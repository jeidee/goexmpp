// Copyright 2011 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package xmpp

import (
	"fmt"
	"os"
	"xml"
)

// This file contains support for roster management, RFC 3921, Section 7.

// Roster query/result
type RosterQuery struct {
	XMLName xml.Name `xml:"jabber:iq:roster query"`
	Item []RosterItem
}

// See RFC 3921, Section 7.1.
type RosterItem struct {
	XMLName xml.Name `xml:"item"`
	Jid string `xml:"attr"`
	Subscription string `xml:"attr"`
	Name string `xml:"attr"`
	Group []string
}

// Implicitly becomes part of NewClient's extStanza arg.
func newRosterQuery(name *xml.Name) interface{} {
	return &RosterQuery{}
}

// Synchronously fetch this entity's roster from the server and cache
// that information.
func (cl *Client) fetchRoster() os.Error {
	iq := &Iq{From: cl.Jid.String(), Id: <- cl.Id, Type: "get",
		Nested: RosterQuery{}}
	ch := make(chan os.Error)
	f := func(st Stanza) bool {
		if iq.Type == "error" {
			ch <- iq.Error
			return false
		}
		rq, ok := st.GetNested().(*RosterQuery)
		if !ok {
			ch <- os.NewError(fmt.Sprintf(
				"Roster query result not query: %v", st))
			return false
		}
		cl.roster = make(map[string] *RosterItem, len(rq.Item))
		for i, item := range(rq.Item) {
			cl.roster[item.Jid] = &rq.Item[i]
		}
		ch <- nil
		return false
	}
	cl.HandleStanza(iq.Id, f)
	cl.Out <- iq
	// Wait for f to complete.
	return <- ch
}

// Returns the current roster of other entities which this one has a
// relationship with. Changes to the roster will be signaled by an
// appropriate Iq appearing on Client.In. See RFC 3921, Section 7.4.
func (cl *Client) Roster() map[string] *RosterItem {
	r := make(map[string] *RosterItem)
	for key, val := range(cl.roster) {
		r[key] = val
	}
	return r
}

// The roster filter updates the Client's representation of the
// roster, but it lets the relevant stanzas through.
func (cl *Client) startRosterFilter() {
	out := make(chan Stanza)
	in := cl.AddFilter(out)
	go func(in <-chan Stanza, out chan<- Stanza) {
		defer close(out)
		for st := range(in) {
			cl.maybeUpdateRoster(st)
			out <- st
		}
	}(in, out)
}

// BUG(cjyar) This isn't getting updates.
// BUG(cjyar) This isn't actually thread safe, though it's unlikely it
// will fail in practice. Either the roster should be protected with a
// mutex, or we should make the roster available on a channel instead
// of via a method call.
// BUG(cjyar) RFC 3921, Section 7.4 says we need to reply.
func (cl *Client) maybeUpdateRoster(st Stanza) {
	rq, ok := st.GetNested().(*RosterQuery)
	if st.GetName() == "iq" && st.GetType() == "set" && ok {
		for i, item := range(rq.Item) {
			if item.Subscription == "remove" {
				cl.roster[item.Jid] = nil
			} else {
				cl.roster[item.Jid] = &rq.Item[i]
			}
		}
	}
}
