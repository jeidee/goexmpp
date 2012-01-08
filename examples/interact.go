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
	"time"
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

	time.Sleep(1e9 * 5)
	fmt.Println("Shutting down.")
	close(c.Out)
	time.Sleep(1e9 * 5)
	select {}
}
