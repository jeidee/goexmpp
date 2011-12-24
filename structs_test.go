// Copyright 2011 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package xmpp

import (
	"bytes"
	"testing"
	"xml"
)

func assertEquals(t *testing.T, expected, observed string) {
	if expected != observed {
		t.Errorf("Expected:\n%s\nObserved:\n%s\n", expected,
			observed)
	}
}

func TestJid(t *testing.T) {
	str := "user@domain/res"
	jid := &JID{}
	if !jid.Set(str) {
		t.Errorf("Set(%s) failed\n", str)
	}
	assertEquals(t, "user", *jid.Node)
	assertEquals(t, "domain", jid.Domain)
	assertEquals(t, "res", *jid.Resource)
	assertEquals(t, str, jid.String())

	str = "domain.tld"
	if !jid.Set(str) {
		t.Errorf("Set(%s) failed\n", str)
	}
	if jid.Node != nil {
		t.Errorf("Node: %v\n", *jid.Node)
	}
	assertEquals(t, "domain.tld", jid.Domain)
	if jid.Resource != nil {
		t.Errorf("Resource: %v\n", *jid.Resource)
	}
	assertEquals(t, str, jid.String())
}

func assertMarshal(t *testing.T, expected string, marshal interface{}) {
	buf := bytes.NewBuffer(nil)
	xml.Marshal(buf, marshal)
	observed := string(buf.Bytes())
	assertEquals(t, expected, observed)
}

func TestStreamMarshal(t *testing.T) {
	s := &Stream{to: "bob"}
	exp := `<stream:stream to="bob">`
	assertMarshal(t, exp, s)

	s = &Stream{to: "bob", from: "alice", id: "#3", version: "5.3"}
	exp = `<stream:stream to="bob" from="alice" id="#3" version="5.3">`
	assertMarshal(t, exp, s)

	s = &Stream{lang: "en_US"}
	exp = `<stream:stream xml:lang="en_US">`
	assertMarshal(t, exp, s)
}

func TestStreamErrorMarshal(t *testing.T) {
	name := xml.Name{Space: nsStreams, Local: "ack"}
	e := &StreamError{cond: definedCondition{name}}
	exp := `<stream:error><ack xmlns="` + nsStreams +
		`"></ack></stream:error>`;
	assertMarshal(t, exp, e)

	txt := errText{Lang: "pt", text: "things happen"}
	e = &StreamError{cond: definedCondition{name}, text: &txt}
	exp = `<stream:error><ack xmlns="` + nsStreams +
		`"></ack><text xmlns="` + nsStreams +
		`" xml:lang="pt">things happen</text></stream:error>`
	assertMarshal(t, exp, e)
}
