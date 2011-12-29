// Copyright 2011 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// This package implements a simple XMPP client according to RFCs 3920
// and 3921, plus the various XEPs at http://xmpp.org/protocols/.
package xmpp

// BUG(cjyar) Figure out why the library doesn't exit when the server
// closes its stream to us.

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"sync"
	"xml"
)

const (
	// Version of RFC 3920 that we implement.
	Version = "1.0"

	// Various XML namespaces.
	nsStreams = "urn:ietf:params:xml:ns:xmpp-streams"
	nsStream = "http://etherx.jabber.org/streams"
	nsTLS = "urn:ietf:params:xml:ns:xmpp-tls"
	nsSASL = "urn:ietf:params:xml:ns:xmpp-sasl"
	nsBind = "urn:ietf:params:xml:ns:xmpp-bind"
	nsSession = "urn:ietf:params:xml:ns:xmpp-session"

	// DNS SRV names
	serverSrv = "xmpp-server"
	clientSrv = "xmpp-client"

	// BUG(cjyar) Make this a parameter to NewClient, not a
	// constant. We should have both a log level and a
	// syslog.Writer, so the app can control how much time we
	// spend generating log messages, as well as where they go.
	debug = true
)

// The client in a client-server XMPP connection.
type Client struct {
	// This client's JID. This will be updated asynchronously by
	// the time StartSession() returns.
	Jid JID
	password string
	socket net.Conn
	socketSync sync.WaitGroup
	saslExpected string
	authDone bool
	idMutex sync.Mutex
	nextId int64
	handlers chan *stanzaHandler
	inputControl chan int
	// This channel may be used as a convenient way to generate a
	// unique id for an iq, message, or presence stanza.
	Id <-chan string
	// Incoming XMPP stanzas from the server will be published on
	// this channel. Information which is only used by this
	// library to set up the XMPP stream will not appear here.
	In <-chan Stanza
	// Outgoing XMPP stanzas to the server should be sent to this
	// channel.
	Out chan<- Stanza
	xmlOut chan<- interface{}
	// Features advertised by the remote. This will be updated
	// asynchronously as new features are received throughout the
	// connection process. It should not be updated once
	// StartSession() returns.
	Features *Features
}
var _ io.Closer = &Client{}

// Connect to the appropriate server and authenticate as the given JID
// with the given password. This function will return as soon as a TCP
// connection has been established, but before XMPP stream negotiation
// has completed. The negotiation will occur asynchronously, and any
// send operation to Client.Out will block until negotiation (resource
// binding) is complete.
func NewClient(jid *JID, password string) (*Client, os.Error) {
	// Resolve the domain in the JID.
	_, srvs, err := net.LookupSRV(clientSrv, "tcp", jid.Domain)
	if err != nil {
		return nil, os.NewError("LookupSrv " + jid.Domain +
			": " + err.String())
	}

	var tcp *net.TCPConn
	for _, srv := range srvs {
		addrStr := fmt.Sprintf("%s:%d", srv.Target, srv.Port)
		addr, err := net.ResolveTCPAddr("tcp", addrStr)
		if err != nil {
			err = os.NewError(fmt.Sprintf("ResolveTCPAddr(%s): %s",
				addrStr, err.String()))
			continue
		}
		tcp, err = net.DialTCP("tcp", nil, addr)
		if err != nil {
			err = os.NewError(fmt.Sprintf("DialTCP(%s): %s",
				addr, err.String()))
			continue
		}
	}
	if tcp == nil {
		return nil, err
	}

	cl := new(Client)
	cl.password = password
	cl.Jid = *jid
	cl.socket = tcp
	cl.handlers = make(chan *stanzaHandler, 1)
	cl.inputControl = make(chan int)
	idCh := make(chan string)
	cl.Id = idCh

	// Start the unique id generator.
	go makeIds(idCh)

	// Start the transport handler, initially unencrypted.
	tlsr, tlsw := cl.startTransport()

	// Start the reader and writers that convert to and from XML.
	xmlIn := startXmlReader(tlsr)
	cl.xmlOut = startXmlWriter(tlsw)

	// Start the XMPP stream handler which filters stream-level
	// events and responds to them.
	clIn := cl.startStreamReader(xmlIn, cl.xmlOut)
	clOut := cl.startStreamWriter(cl.xmlOut)

	// Initial handshake.
	hsOut := &stream{To: jid.Domain, Version: Version}
	cl.xmlOut <- hsOut

	cl.In = clIn
	cl.Out = clOut

	return cl, nil
}

func (c *Client) Close() os.Error {
	tryClose(c.In, c.Out)
	return nil
}

func (cl *Client) startTransport() (io.Reader, io.Writer) {
	inr, inw := io.Pipe()
	outr, outw := io.Pipe()
	go cl.readTransport(inw)
	go cl.writeTransport(outr)
	return inr, outw
}

func startXmlReader(r io.Reader) <-chan interface{} {
	ch := make(chan interface{})
	go readXml(r, ch)
	return ch
}

func startXmlWriter(w io.Writer) chan<- interface{} {
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

func tee(r io.Reader, w io.Writer, prefix string) {
	defer tryClose(r, w)

	buf := bytes.NewBuffer(nil)
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
			fmt.Printf("%s%s\n", prefix, buf.String())
			buf.Reset()
		}
	}
	leftover := buf.String()
	if leftover != "" {
		fmt.Printf("%s%s\n", prefix, leftover)
	}
}

func tryClose(xs ...interface{}) {
	f1 := func(ch chan<- interface{}) {
		defer func() {
			recover()
		}()
		close(ch)
	}
	f2 := func(ch <-chan interface{}) {
		defer func() {
			recover()
		}()
		close(ch)
	}

	for _, x := range xs {
		if c, ok := x.(io.Closer) ; ok {
			c.Close()
		} else if ch, ok := x.(chan<- interface{}) ; ok {
			f1(ch)
		} else if ch, ok := x.(<-chan interface{}) ; ok {
			f2(ch)
		}
	}
}

func makeIds(ch chan<- string) {
	id := int64(1)
	for {
		str := fmt.Sprintf("id_%d", id)
		ch <- str
		id++
	}
}

// bindDone is called when we've finished resource binding (and all
// the negotiations that precede it). Now we can start accepting
// traffic from the app.
func (cl *Client) bindDone() {
	cl.inputControl <- 1
}

// Start an XMPP session. This should typically be done immediately
// after creating the new Client. Once the session has been
// established, pr will be sent as an initial presence; nil means
// don't send initial presence. The initial presence can be a
// newly-initialized Presence struct. See RFC 3921, Section 3.
func (cl *Client) StartSession(pr *Presence) os.Error {
	id := <- cl.Id
	iq := &Iq{To: cl.Jid.Domain, Id: id, Type: "set", Any:
		&Generic{XMLName: xml.Name{Space: nsSession, Local:
				"session"}}}
	ch := make(chan os.Error)
	f := func(st Stanza) bool {
		if st.XType() == "error" {
			log.Printf("Can't start session: %v", st)
			ch <- st.XError()
			return false
		}
		if pr != nil {
			cl.Out <- pr
		}
		ch <- nil
		return false
	}
	cl.HandleStanza(id, f)
	cl.Out <- iq
	// Now wait until the callback is called.
	return <-ch
}
