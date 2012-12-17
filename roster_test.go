// Copyright 2011 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package xmpp

import (
	"encoding/xml"
	"reflect"
	"testing"
)

// This is mostly just tests of the roster data structures.

func TestRosterIqMarshal(t *testing.T) {
	iq := &Iq{Header: Header{From: "from", Lang: "en",
		Nested: []interface{}{RosterQuery{}}}}
	exp := `<iq from="from" xml:lang="en"><query xmlns="` +
		NsRoster + `"></query></iq>`
	assertMarshal(t, exp, iq)
}

func TestRosterIqUnmarshal(t *testing.T) {
	str := `<iq from="from" xml:lang="en"><query xmlns="` +
		NsRoster + `"><item jid="a@b.c"/></query></iq>`
	iq := Iq{}
	xml.Unmarshal([]byte(str), &iq)
	m := map[string]func(*xml.Name) interface{}{NsRoster: newRosterQuery}
	err := parseExtended(&iq.Header, m)
	if err != nil {
		t.Fatalf("parseExtended: %v", err)
	}
	assertEquals(t, "iq", iq.XMLName.Local)
	assertEquals(t, "from", iq.From)
	assertEquals(t, "en", iq.Lang)
	nested := iq.Nested
	if nested == nil {
		t.Fatalf("nested nil")
	}
	if len(nested) != 1 {
		t.Fatalf("wrong size nested(%d): %v", len(nested),
			nested)
	}
	var rq *RosterQuery
	rq, ok := nested[0].(*RosterQuery)
	if !ok {
		t.Fatalf("nested not RosterQuery: %v",
			reflect.TypeOf(nested))
	}
	if len(rq.Item) != 1 {
		t.Fatalf("Wrong # items: %v", rq.Item)
	}
	item := rq.Item[0]
	assertEquals(t, "a@b.c", item.Jid)
}
