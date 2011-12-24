// Copyright 2011 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// This package implements a simple XMPP client according to RFCs 3920
// and 3921, plus the various XEPs at http://xmpp.org/protocols/.
package xmpp

import (
	"fmt"
	"net"
	"os"
)

const (
	serverSrv = "xmpp-server"
	clientSrv = "xmpp-client"
)

// The client in a client-server XMPP connection.
type Client struct {
	//In <-chan *Stanza
	//Out chan<- *Stanza
	tcp *net.TCPConn
}

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
	return &cl, nil
}
