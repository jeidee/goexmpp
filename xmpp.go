// Copyright 2011 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// This package implements a simple XMPP client according to RFCs 3920
// and 3921, plus the various XEPs at http://xmpp.org/protocols/.
package xmpp

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

	debug = true
)

// The client in a client-server XMPP connection.
type Client struct {
	Jid JID
	password string
	socket net.Conn
	socketSync sync.WaitGroup
	saslExpected string
	authDone bool
	idMutex sync.Mutex
	nextId int64
	handlers chan *stanzaHandler
	In <-chan interface{}
	Out chan<- interface{}
	xmlOut chan<- interface{}
	TextOut chan<- *string
}
var _ io.Closer = &Client{}

// Connect to the appropriate server and authenticate as the given JID
// with the given password.
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
	cl.handlers = make(chan *stanzaHandler)

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
	hsOut := &Stream{To: jid.Domain, Version: Version}
	cl.xmlOut <- hsOut

	// TODO Wait for initialization to finish.

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

func (cl *Client) startStreamReader(xmlIn <-chan interface{}, srvOut chan<- interface{}) <-chan interface{} {
	ch := make(chan interface{})
	go cl.readStream(xmlIn, ch)
	return ch
}

func startStreamWriter(xmlOut chan<- interface{}) chan<- interface{} {
	ch := make(chan interface{})
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

func (cl *Client) NextId() string {
	cl.idMutex.Lock()
	defer cl.idMutex.Unlock()
	id := cl.nextId
	cl.nextId++
	return fmt.Sprintf("id_%d", id)
}
