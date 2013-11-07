// Copyright 2011 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// This package implements a simple XMPP client according to RFCs 3920
// and 3921, plus the various XEPs at http://xmpp.org/protocols/.
package xmpp

import (
	"bytes"
	"crypto/tls"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
)

const (
	// Version of RFC 3920 that we implement.
	Version = "1.0"

	// Various XML namespaces.
	NsClient  = "jabber:client"
	NsStreams = "urn:ietf:params:xml:ns:xmpp-streams"
	NsStream  = "http://etherx.jabber.org/streams"
	NsTLS     = "urn:ietf:params:xml:ns:xmpp-tls"
	NsSASL    = "urn:ietf:params:xml:ns:xmpp-sasl"
	NsBind    = "urn:ietf:params:xml:ns:xmpp-bind"
	NsSession = "urn:ietf:params:xml:ns:xmpp-session"
	NsRoster  = "jabber:iq:roster"

	// DNS SRV names
	serverSrv = "xmpp-server"
	clientSrv = "xmpp-client"
)

// This channel may be used as a convenient way to generate a unique
// id for an iq, message, or presence stanza.
var Id <-chan string

func init() {
	// Start the unique id generator.
	idCh := make(chan string)
	Id = idCh
	go func(ch chan<- string) {
		id := int64(1)
		for {
			str := fmt.Sprintf("id_%d", id)
			ch <- str
			id++
		}
	}(idCh)
}

// Extensions can add stanza filters and/or new XML element types.
type Extension struct {
	StanzaHandlers map[string]func(*xml.Name) interface{}
	Start          func(*Client)
}

// Allows the user to override the TLS configuration.
var TlsConfig tls.Config

// The client in a client-server XMPP connection.
type Client struct {
	// This client's unique ID. It's unique within the context of
	// this process, so if multiple Client objects exist, each
	// will be distinguishable by its Uid.
	Uid string
	// This client's JID. This will be updated asynchronously by
	// the time StartSession() returns.
	Jid          JID
	password     string
	socket       net.Conn
	socketSync   sync.WaitGroup
	saslExpected string
	authDone     bool
	handlers     chan *stanzaHandler
	inputControl chan int
	// Incoming XMPP stanzas from the server will be published on
	// this channel. Information which is only used by this
	// library to set up the XMPP stream will not appear here.
	In <-chan Stanza
	// Outgoing XMPP stanzas to the server should be sent to this
	// channel.
	Out    chan<- Stanza
	xmlOut chan<- interface{}
	// Features advertised by the remote. This will be updated
	// asynchronously as new features are received throughout the
	// connection process. It should not be updated once
	// StartSession() returns.
	Features  *Features
	filterOut chan<- <-chan Stanza
	filterIn  <-chan <-chan Stanza
}

// Connect to the appropriate server and authenticate as the given JID
// with the given password. This function will return as soon as a TCP
// connection has been established, but before XMPP stream negotiation
// has completed. The negotiation will occur asynchronously, and any
// send operation to Client.Out will block until negotiation (resource
// binding) is complete.
func NewClient(jid *JID, password string, exts []Extension) (*Client, error) {
	// Resolve the domain in the JID.
	_, srvs, err := net.LookupSRV(clientSrv, "tcp", jid.Domain)
	if err != nil {
		return nil, errors.New("LookupSrv " + jid.Domain +
			": " + err.Error())
	}

	var tcp *net.TCPConn
	for _, srv := range srvs {
		addrStr := fmt.Sprintf("%s:%d", srv.Target, srv.Port)
		addr, err := net.ResolveTCPAddr("tcp", addrStr)
		if err != nil {
			err = fmt.Errorf("ResolveTCPAddr(%s): %s",
				addrStr, err.Error())
			continue
		}
		tcp, err = net.DialTCP("tcp", nil, addr)
		if err != nil {
			err = fmt.Errorf("DialTCP(%s): %s",
				addr, err)
			continue
		}
	}
	if tcp == nil {
		return nil, err
	}

	return newClient(tcp, jid, password, exts)
}

// Connect to the specified host and port. This is otherwise identical
// to NewClient.
func NewClientFromHost(jid *JID, password string, exts []Extension, host string, port int) (*Client, error) {
	addrStr := fmt.Sprintf("%s:%d", host, port)
	addr, err := net.ResolveTCPAddr("tcp", addrStr)
	if err != nil {
		return nil, err
	}
	tcp, err := net.DialTCP("tcp", nil, addr)
	if err != nil {
		return nil, err
	}

	return newClient(tcp, jid, password, exts)
}

func newClient(tcp *net.TCPConn, jid *JID, password string, exts []Extension) (*Client, error) {
	// Include the mandatory extensions.
	exts = append(exts, rosterExt)
	exts = append(exts, bindExt)

	cl := new(Client)
	cl.Uid = <-Id
	cl.password = password
	cl.Jid = *jid
	cl.socket = tcp
	cl.handlers = make(chan *stanzaHandler, 100)
	cl.inputControl = make(chan int)

	extStanza := make(map[string]func(*xml.Name) interface{})
	for _, ext := range exts {
		for k, v := range ext.StanzaHandlers {
			extStanza[k] = v
		}
	}

	// Start the transport handler, initially unencrypted.
	tlsr, tlsw := cl.startTransport()

	// Start the reader and writers that convert to and from XML.
	xmlIn := startXmlReader(tlsr, extStanza)
	cl.xmlOut = startXmlWriter(tlsw)

	// Start the XMPP stream handler which filters stream-level
	// events and responds to them.
	stIn := cl.startStreamReader(xmlIn, cl.xmlOut)
	clOut := cl.startStreamWriter(cl.xmlOut)
	cl.Out = clOut

	// Start the manager for the filters that can modify what the
	// app sees.
	clIn := cl.startFilter(stIn)
	cl.In = clIn

	// Add filters for our extensions.
	for _, ext := range exts {
		ext.Start(cl)
	}

	// Initial handshake.
	hsOut := &stream{To: jid.Domain, Version: Version}
	cl.xmlOut <- hsOut

	return cl, nil
}

func (cl *Client) startTransport() (io.Reader, io.WriteCloser) {
	inr, inw := io.Pipe()
	outr, outw := io.Pipe()
	go cl.readTransport(inw)
	go cl.writeTransport(outr)
	return inr, outw
}

func startXmlReader(r io.Reader,
	extStanza map[string]func(*xml.Name) interface{}) <-chan interface{} {
	ch := make(chan interface{})
	go readXml(r, ch, extStanza)
	return ch
}

func startXmlWriter(w io.WriteCloser) chan<- interface{} {
	ch := make(chan interface{})
	go writeXml(w, ch)
	return ch
}

func (cl *Client) startStreamReader(xmlIn <-chan interface{}, srvOut chan<- interface{}) <-chan Stanza {
	ch := make(chan Stanza)
	go cl.readStream(xmlIn, ch)
	return ch
}

func (cl *Client) startStreamWriter(xmlOut chan<- interface{}) chan<- Stanza {
	ch := make(chan Stanza)
	go writeStream(xmlOut, ch, cl.inputControl)
	return ch
}

func (cl *Client) startFilter(srvIn <-chan Stanza) <-chan Stanza {
	cliIn := make(chan Stanza)
	filterOut := make(chan (<-chan Stanza))
	filterIn := make(chan (<-chan Stanza))
	nullFilter := make(chan Stanza)
	go filterBottom(srvIn, nullFilter)
	go filterTop(filterOut, filterIn, nullFilter, cliIn)
	cl.filterOut = filterOut
	cl.filterIn = filterIn
	return cliIn
}

func tee(r io.Reader, w io.Writer, prefix string) {
	defer func(w io.Writer) {
		if c, ok := w.(io.Closer); ok {
			c.Close()
		}
	}(w)

	buf := bytes.NewBuffer([]uint8(prefix))
	for {
		var c [1]byte
		n, _ := r.Read(c[:])
		if n == 0 {
			break
		}
		n, _ = w.Write(c[:n])
		if n == 0 {
			break
		}
		buf.Write(c[:n])
		if c[0] == '\n' || c[0] == '>' {
			Debug.Log(buf)
			buf = bytes.NewBuffer([]uint8(prefix))
		}
	}
	leftover := buf.String()
	if leftover != "" {
		Debug.Log(buf)
	}
}

// bindDone is called when we've finished resource binding (and all
// the negotiations that precede it). Now we can start accepting
// traffic from the app.
func (cl *Client) bindDone() {
	cl.inputControl <- 1
}

// Start an XMPP session. A typical XMPP client should call this
// immediately after creating the Client in order to start the
// session, retrieve the roster, and broadcast an initial
// presence. The presence can be as simple as a newly-initialized
// Presence struct.  See RFC 3921, Section 3.
func (cl *Client) StartSession(getRoster bool, pr *Presence) error {
	id := <-Id
	iq := &Iq{Header: Header{To: cl.Jid.Domain, Id: id, Type: "set",
		Nested: []interface{}{Generic{XMLName: xml.Name{Space: NsSession, Local: "session"}}}}}
	ch := make(chan error)
	f := func(st Stanza) bool {
		iq, ok := st.(*Iq)
		if !ok {
			Warn.Log("iq reply not iq; can't start session")
			ch <- errors.New("bad session start reply")
			return false
		}
		if iq.Type == "error" {
			Warn.Logf("Can't start session: %v", iq)
			ch <- iq.Error
			return false
		}
		ch <- nil
		return false
	}
	cl.HandleStanza(id, f)
	cl.Out <- iq

	// Now wait until the callback is called.
	if err := <-ch; err != nil {
		return err
	}
	if getRoster {
		err := fetchRoster(cl)
		if err != nil {
			return err
		}
	}
	if pr != nil {
		cl.Out <- pr
	}
	return nil
}

// AddFilter adds a new filter to the top of the stack through which
// incoming stanzas travel on their way up to the client. The new
// filter's output channel is given to this function, and it returns a
// new input channel which the filter should read from. When its input
// channel closes, the filter should close its output channel.
func (cl *Client) AddFilter(out <-chan Stanza) <-chan Stanza {
	cl.filterOut <- out
	return <-cl.filterIn
}
