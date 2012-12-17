// Copyright 2011 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	xmpp ".."
	"crypto/tls"
	"flag"
	"fmt"
	"log"
	"os"
)

type StdLogger struct {
}

func (s *StdLogger) Log(v ...interface{}) {
	log.Println(v)
}

func (s *StdLogger) Logf(fmt string, v ...interface{}) {
	log.Printf(fmt, v)
}

func init() {
	logger := &StdLogger{}
	// xmpp.Debug = logger
	xmpp.Info = logger
	xmpp.Warn = logger

	xmpp.TlsConfig = tls.Config{InsecureSkipVerify: true}
}

// Demonstrate the API, and allow the user to interact with an XMPP
// server via the terminal.
func main() {
	var jid xmpp.JID
	flag.Var(&jid, "jid", "JID to log in as")
	var pw *string = flag.String("pw", "", "password")
	flag.Parse()
	if jid.Domain == "" || *pw == "" {
		flag.Usage()
		os.Exit(2)
	}

	c, err := xmpp.NewClient(&jid, *pw, nil)
	if err != nil {
		log.Fatalf("NewClient(%v): %v", jid, err)
	}
	defer close(c.Out)

	err = c.StartSession(true, &xmpp.Presence{})
	if err != nil {
		log.Fatalf("StartSession: %v", err)
	}
	roster := xmpp.Roster(c)
	fmt.Printf("%d roster entries:\n", len(roster))
	for i, entry := range(roster) {
		fmt.Printf("%d: %v\n", i, entry)
	}

	go func(ch <-chan xmpp.Stanza) {
		for obj := range ch {
			fmt.Printf("s: %v\n", obj)
		}
		fmt.Println("done reading")
	}(c.In)

	p := make([]byte, 1024)
	for {
		nr, _ := os.Stdin.Read(p)
		if nr == 0 {
			break
		}
		s := string(p)
		stan, err := xmpp.ParseStanza(s)
		if err == nil {
			c.Out <- stan
		} else {
			fmt.Printf("Parse error: %v\n", err)
			break
		}
	}
	fmt.Println("done sending")
}
