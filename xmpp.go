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
	"net"
	"os"
	"sync"
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
	// This client's JID. This will be updated asynchronously when
	// resource binding completes; at that time an iq stanza will
	// be published on the In channel:
	// <iq><bind><jid>jid</jid></bind></iq>
	Jid JID
	password string
	socket net.Conn
	socketSync sync.WaitGroup
	saslExpected string
	authDone bool
	idMutex sync.Mutex
	nextId int64
	handlers chan *stanzaHandler
	// Incoming XMPP stanzas from the server will be published on
	// this channel. Information which is only used by this
	// library to set up the XMPP stream will not appear here.
	In <-chan Stanza
	// Outgoing XMPP stanzas to the server should be sent to this
	// channel.
	Out chan<- Stanza
	xmlOut chan<- interface{}
	// BUG(cjyar) Remove this. Make a Stanza parser method
	// available for use by interact.go and similar applications.
	TextOut chan<- *string
}
var _ io.Closer = &Client{}

// Connect to the appropriate server and authenticate as the given JID
// with the given password. This function will return as soon as a TCP
// connection has been established, but before XMPP stream negotiation
// has completed. The negotiation will occur asynchronously, and sends
// to Client.Out will block until negotiation is complete.
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

	// Start the transport handler, initially unencrypted.
	tlsr, tlsw := cl.startTransport()

	// Start the reader and writers that convert to and from XML.
	xmlIn := startXmlReader(tlsr)
	cl.xmlOut = startXmlWriter(tlsw)
	textOut := startTextWriter(tlsw)

	// Start the XMPP stream handler which filters stream-level
	// events and responds to them.
	clIn := cl.startStreamReader(xmlIn, cl.xmlOut)
	clOut := startStreamWriter(cl.xmlOut)

	// Initial handshake.
	hsOut := &stream{To: jid.Domain, Version: Version}
	cl.xmlOut <- hsOut

	cl.In = clIn
	cl.Out = clOut
	cl.TextOut = textOut

	return cl, nil
}

func (c *Client) Close() os.Error {
	tryClose(c.In, c.Out, c.TextOut)
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

func startTextWriter(w io.Writer) chan<- *string {
	ch := make(chan *string)
	go writeText(w, ch)
	return ch
}

func (cl *Client) startStreamReader(xmlIn <-chan interface{}, srvOut chan<- interface{}) <-chan Stanza {
	ch := make(chan Stanza)
	go cl.readStream(xmlIn, ch)
	return ch
}

func startStreamWriter(xmlOut chan<- interface{}) chan<- Stanza {
	ch := make(chan Stanza)
	go writeStream(xmlOut, ch)
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

// This convenience function may be used to generate a unique id for
// use in the Id fields of iq, message, and presence stanzas.
func (cl *Client) NextId() string {
	cl.idMutex.Lock()
	defer cl.idMutex.Unlock()
	id := cl.nextId
	cl.nextId++
	return fmt.Sprintf("id_%d", id)
}
