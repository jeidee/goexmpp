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
	"log"
	"net"
	"os"
	"xml"
)

const (
	serverSrv = "xmpp-server"
	clientSrv = "xmpp-client"
	debug = false
)

// The client in a client-server XMPP connection.
type Client struct {
	In <-chan interface{}
	Out chan<- interface{}
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

	// Start the transport handler, initially unencrypted.
	tlsr, tlsw := startTransport(tcp)

	// Start the reader and writers that convert to and from XML.
	xmlIn := startXmlReader(tlsr)
	xmlOut := startXmlWriter(tlsw)
	textOut := startTextWriter(tlsw)

	// Start the XMPP stream handler which filters stream-level
	// events and responds to them.
	clIn := startStreamReader(xmlIn)
	clOut := startStreamWriter(xmlOut)

	// Initial handshake.
	hsOut := &Stream{To: jid.Domain, Version: Version}
	xmlOut <- hsOut

	// TODO Wait for initialization to finish.

	// Make the Client and init its fields.
	cl := new(Client)
	cl.In = clIn
	cl.Out = clOut
	cl.TextOut = textOut

	return cl, nil
}

func (c *Client) Close() os.Error {
	tryClose(c.In, c.Out, c.TextOut)
	return nil
}

func startTransport(tcp io.ReadWriter) (io.Reader, io.Writer) {
	f := func(r io.Reader, w io.Writer, dir string) {
		defer tryClose(r, w)
		p := make([]byte, 1024)
		for {
			nr, err := r.Read(p)
			if nr == 0 {
				log.Printf("%s: %s", dir, err.String())
				break
			}
			nw, err := w.Write(p[:nr])
			if nw < nr {
				log.Println("%s: %s", dir, err.String())
				break
			}
		}
	}
	inr, inw := io.Pipe()
	outr, outw := io.Pipe()
	go f(tcp, inw, "read")
	go f(outr, tcp, "write")
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

func startStreamReader(xmlIn <-chan interface{}) <-chan interface{} {
	ch := make(chan interface{})
	go readStream(xmlIn, ch)
	return ch
}

func startStreamWriter(xmlOut chan<- interface{}) chan<- interface{} {
	ch := make(chan interface{})
	go writeStream(xmlOut, ch)
	return ch
}

func readXml(r io.Reader, ch chan<- interface{}) {
	if debug {
		pr, pw := io.Pipe()
		go tee(r, pw, "S: ")
		r = pr
	}
	defer tryClose(r, ch)

	p := xml.NewParser(r)
	for {
		// Sniff the next token on the stream.
		t, err := p.Token()
		if t == nil {
			if err != os.EOF {
				log.Printf("read: %v", err)
			}
			break
		}
		var se xml.StartElement
		var ok bool
		if se, ok = t.(xml.StartElement) ; !ok {
			continue
		}

		// Allocate the appropriate structure for this token.
		var obj interface{}
		switch se.Name.Space + " " + se.Name.Local {
		case nsStream + " stream":
			st, err := parseStream(se)
			if err != nil {
				log.Printf("unmarshal stream: %v",
					err)
				break
			}
			ch <- st
			continue
		case "stream error", nsStream + " error":
			obj = &StreamError{}
		case nsStream + " features":
			obj = &Features{}
		default:
			obj = &Unrecognized{}
			log.Printf("Ignoring unrecognized: %s %s\n",
				se.Name.Space, se.Name.Local)
		}

		// Read the complete XML stanza.
		err = p.Unmarshal(obj, &se)
		if err != nil {
			log.Printf("unmarshal: %v", err)
			break
		}

		// Put it on the channel.
		ch <- obj
	}
}

func writeXml(w io.Writer, ch <-chan interface{}) {
	if debug {
		pr, pw := io.Pipe()
		go tee(pr, w, "C: ")
		w = pw
	}
	defer tryClose(w, ch)

	for obj := range ch {
		err := xml.Marshal(w, obj)
		if err != nil {
			log.Printf("write: %v", err)
			break
		}
	}
}

func writeText(w io.Writer, ch <-chan *string) {
	if debug {
		pr, pw := io.Pipe()
		go tee(pr, w, "C: ")
		w = pw
	}
	defer tryClose(w, ch)

	for str := range ch {
		_, err := w.Write([]byte(*str))
		if err != nil {
			log.Printf("writeStr: %v", err)
			break
		}
	}
}

func readStream(srvIn <-chan interface{}, cliOut chan<- interface{}) {
	defer tryClose(srvIn, cliOut)

	for x := range srvIn {
		cliOut <- x
	}
}

func writeStream(srvOut chan<- interface{}, cliIn <-chan interface{}) {
	defer tryClose(srvOut, cliIn)

	for x := range cliIn {
		srvOut <- x
	}
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
