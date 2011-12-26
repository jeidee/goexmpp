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
	debug = true
)

// The client in a client-server XMPP connection.
type Client struct {
	In <-chan interface{}
	in chan interface{}
	Out chan<- interface{}
	out chan interface{}
	tcp *net.TCPConn
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

	var c *net.TCPConn
	for _, srv := range srvs {
		addrStr := fmt.Sprintf("%s:%d", srv.Target, srv.Port)
		addr, err := net.ResolveTCPAddr("tcp", addrStr)
		if err != nil {
			err = os.NewError(fmt.Sprintf("ResolveTCPAddr(%s): %s",
				addrStr, err.String()))
			continue
		}
		c, err = net.DialTCP("tcp", nil, addr)
		if err != nil {
			err = os.NewError(fmt.Sprintf("DialTCP(%s): %s",
				addr, err.String()))
			continue
		}
	}
	if c == nil {
		return nil, err
	}

	cl := new(Client)
	cl.tcp = c
	cl.in = make(chan interface{})
	cl.In = cl.in
	cl.out = make(chan interface{})
	cl.Out = cl.out
	// TODO Send readXml a reader that we can close when we
	// negotiate TLS.
	go readXml(cl.tcp, cl.in, debug)
	go writeXml(cl.tcp, cl.out, debug)

	// Initial handshake.
	hsOut := &Stream{To: jid.Domain, Version: Version}
	cl.Out <- hsOut

	return cl, nil
}

func (c *Client) Close() os.Error {
	close(c.in)
	close(c.out)
	return c.tcp.Close()
}

// TODO Delete; for use only by interact.go:
func ReadXml(r io.ReadCloser, ch chan<- interface{}, dbg bool) {
	readXml(r, ch, dbg)
}

func readXml(r io.Reader, ch chan<- interface{}, dbg bool) {
	defer close(ch)
	if dbg {
		pr, pw := io.Pipe()
		go tee(r, pw, "S: ")
		r = pr
	}

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
		case "stream error":
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

func writeXml(w io.Writer, ch <-chan interface{}, dbg bool) {
	if dbg {
		pr, pw := io.Pipe()
		go tee(pr, w, "C: ")
		w = pw
	}

	for obj := range ch {
		err := xml.Marshal(w, obj)
		if err != nil {
			log.Printf("write: %v", err)
			break
		}
	}
}

func tee(r io.Reader, w io.Writer, prefix string) {
	defer func(xs ...interface{}) {
		for _, x := range xs {
			if c, ok := x.(io.Closer) ; ok {
				c.Close()
			}
		}
	}(r, w)

	buf := bytes.NewBuffer(nil)
	for {
		var c [1]byte
		n, _ := r.Read(c[:])
		if n == 0 {
			break
		}
		n, _ = w.Write(c[:])
		if n == 0 {
			break
		}
		buf.Write(c[:])
		if c[0] == '\n' {
			fmt.Printf("%s%s", prefix, buf.String())
			buf.Reset()
		}
	}
	leftover := buf.String()
	if leftover != "" {
		fmt.Printf("%s%s\n", prefix, leftover)
	}
}
