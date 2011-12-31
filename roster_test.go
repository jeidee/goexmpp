// Copyright 2011 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package xmpp

import (
	"testing"
	"xml"
)

// This is mostly just tests of the roster data structures.

func TestRosterIqMarshal(t *testing.T) {
	iq := &RosterIq{Iq: Iq{From: "from", Lang: "en"}, Query:
		RosterQuery{XMLName: xml.Name{Space: NsRoster, Local:
				"query"}, Item: []RosterItem{}}}
	var s Stanza = iq
	if _, ok := s.(ExtendedStanza) ; !ok {
		t.Errorf("Not an ExtendedStanza")
	}
	exp := `<iq from="from" xml:lang="en"><query xmlns="` +
		NsRoster + `"></query></iq>`
	assertMarshal(t, exp, iq)
}
