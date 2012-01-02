// Copyright 2011 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package xmpp

import (
	"reflect"
	"strings"
	"testing"
	"xml"
)

// This is mostly just tests of the roster data structures.

func TestRosterIqMarshal(t *testing.T) {
	iq := &Iq{From: "from", Lang: "en", Nested: RosterQuery{}}
	exp := `<iq from="from" xml:lang="en"><query xmlns="` +
		NsRoster + `"></query></iq>`
	assertMarshal(t, exp, iq)
}

func TestRosterIqUnmarshal(t *testing.T) {
	str := `<iq from="from" xml:lang="en"><query xmlns="` +
		NsRoster + `"><item jid="a@b.c"/></query></iq>`
	r := strings.NewReader(str)
	var st Stanza = &Iq{}
	xml.Unmarshal(r, st)
	err := parseExtended(st, newRosterQuery)
	if err != nil {
		t.Fatalf("parseExtended: %v", err)
	}
	assertEquals(t, "iq", st.GetName())
	assertEquals(t, "from", st.GetFrom())
	assertEquals(t, "en", st.GetLang())
	nested := st.GetNested()
	if nested == nil {
		t.Fatalf("nested nil")
	}
	rq, ok := nested.(*RosterQuery)
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
