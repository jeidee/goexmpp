// Copyright 2011 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// This package implements a simple XMPP client according to RFCs 3920
// and 3921, plus the various XEPs at http://xmpp.org/protocols/.
package xmpp

import (
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

	cl := Client{}
	cl.tcp = c
	cl.in = make(chan interface{})
	cl.In = cl.in
	// TODO Send readXml a reader that we can close when we
	// negotiate TLS.
	go readXml(cl.tcp, cl.in)
	// TODO go writeXml(&cl)

	return &cl, nil
}

func (c *Client) Close() os.Error {
	return c.tcp.Close()
}

func readXml(r io.Reader, ch chan<- interface{}) {
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
		switch se.Name.Space + se.Name.Local {
		case "stream stream":
			st, err := parseStream(se)
			if err != nil {
				log.Printf("unmarshal stream: %v",
					err)
				break
			}
			ch <- st
			continue
		case nsStreams + " stream:error":
			obj = &StreamError{}
		default:
			obj = &Unrecognized{}
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
