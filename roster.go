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
		for _, item := range(rq.Item) {
			cl.roster[item.Jid] = &item
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
	go func(inSave <-chan Stanza, outSave chan<- Stanza) {
		defer close(out)
		in := inSave
		var out chan<- Stanza
		var st Stanza
		var ok bool
		for {
			select {
			case st, ok = <- in:
				if !ok {
					break
				}
				cl.maybeUpdateRoster(st)
				in = nil
				out = outSave
			case out <- st:
				out = nil
				in = inSave
			}
		}
	}(in, out)
}

func (cl *Client) maybeUpdateRoster(st Stanza) {
	rq, ok := st.GetNested().(*RosterQuery)
	if st.GetName() == "iq" && st.GetType() == "set" && ok {
		for _, item := range(rq.Item) {
			cl.roster[item.Jid] = &item
		}
	}
}
