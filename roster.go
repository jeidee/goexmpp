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
	// Should always be NsRoster, "query"
	XMLName xml.Name
	Item []RosterItem
}

// See RFC 3921, Section 7.1.
type RosterItem struct {
	// Should always be "item"
	XMLName xml.Name
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
		Nested: RosterQuery{XMLName: xml.Name{Local: "query",
			Space: NsRoster}}}
	ch := make(chan os.Error)
	f := func(st Stanza) bool {
		if iq.Type == "error" {
			ch <- iq.Error
			return false
		}
		rq, ok := st.XNested().(*RosterQuery)
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

// BUG(cjyar) The roster isn't actually updated when things change.

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
