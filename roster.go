// Copyright 2011 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package xmpp

import (
	"fmt"
	"io"
	"os"
	"xml"
)

// This file contains support for roster management, RFC 3921, Section 7.

type RosterIq struct {
	Iq
	Query RosterQuery
}
var _ ExtendedStanza = &RosterIq{}

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

func (riq *RosterIq) InnerMarshal(w io.Writer) os.Error {
	return xml.Marshal(w, riq.Query)
}

// Implicitly becomes part of NewClient's extStanza arg.
func rosterStanza(name *xml.Name) ExtendedStanza {
	return &RosterIq{}
}

// Synchronously fetch this entity's roster from the server and cache
// that information.
func (cl *Client) fetchRoster() os.Error {
	iq := &RosterIq{Iq: Iq{From: cl.Jid.String(), Id: <- cl.Id,
		Type: "get"}, Query: RosterQuery{XMLName:
			xml.Name{Local: "query", Space: NsRoster}}}
	ch := make(chan os.Error)
	f := func(st Stanza) bool {
		iq, ok := st.(*RosterIq)
		if !ok {
			ch <- os.NewError(fmt.Sprintf(
				"Roster query result not iq: %v", st))
			return false
		}
		if iq.Type == "error" {
			ch <- iq.Error
			return false
		}
		q := iq.Query
		cl.roster = make(map[string] *RosterItem, len(q.Item))
		for _, item := range(q.Item) {
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
