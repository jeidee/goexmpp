// Copyright 2011 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"cjyar/xmpp"
	"flag"
	"fmt"
	"log"
	"os"
	)

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

	c, err := xmpp.NewClient(&jid, *pw)
	if err != nil {
		log.Fatalf("NewClient(%v): %v", jid, err)
	}
	defer c.Close()

	err = c.StartSession(true, &xmpp.Presence{})
	if err != nil {
		log.Fatalf("StartSession: %v", err)
	}
	roster := c.Roster()
	fmt.Printf("%d roster entries:\n", len(roster))
	for jid, entry := range(roster) {
		fmt.Printf("%s: %v\n", jid, entry)
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
