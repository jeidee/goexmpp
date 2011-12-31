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
	assertEquals(t, "user", jid.Node)
	assertEquals(t, "domain", jid.Domain)
	assertEquals(t, "res", jid.Resource)
	assertEquals(t, str, jid.String())

	str = "domain.tld"
	if !jid.Set(str) {
		t.Errorf("Set(%s) failed\n", str)
	}
	if jid.Node != "" {
		t.Errorf("Node: %v\n", jid.Node)
	}
	assertEquals(t, "domain.tld", jid.Domain)
	if jid.Resource != "" {
		t.Errorf("Resource: %v\n", jid.Resource)
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
	s := &stream{To: "bob"}
	exp := `<stream:stream xmlns="jabber:client"` +
		` xmlns:stream="` + NsStream + `" to="bob">`
	assertMarshal(t, exp, s)

	s = &stream{To: "bob", From: "alice", Id: "#3", Version: "5.3"}
	exp = `<stream:stream xmlns="jabber:client"` +
		` xmlns:stream="` + NsStream + `" to="bob" from="alice"` +
		` id="#3" version="5.3">`
	assertMarshal(t, exp, s)

	s = &stream{Lang: "en_US"}
	exp = `<stream:stream xmlns="jabber:client"` +
		` xmlns:stream="` + NsStream + `" xml:lang="en_US">`
	assertMarshal(t, exp, s)
}

func TestStreamErrorMarshal(t *testing.T) {
	name := xml.Name{Space: NsStreams, Local: "ack"}
	e := &streamError{Any: Generic{XMLName: name}}
	exp := `<stream:error><ack xmlns="` + NsStreams +
		`"></ack></stream:error>`;
	assertMarshal(t, exp, e)

	txt := errText{Lang: "pt", Text: "things happen"}
	e = &streamError{Any: Generic{XMLName: name}, Text: &txt}
	exp = `<stream:error><ack xmlns="` + NsStreams +
		`"></ack><text xmlns="` + NsStreams +
		`" xml:lang="pt">things happen</text></stream:error>`
	assertMarshal(t, exp, e)
}

func TestIqMarshal(t *testing.T) {
	iq := &Iq{Type: "set", Id: "3", Any: &Generic{XMLName:
			xml.Name{Space: NsBind, Local: "bind"}}}
	exp := `<iq id="3" type="set"><bind xmlns="` + NsBind +
		`"></bind></iq>`
	assertMarshal(t, exp, iq)
}

func TestParseStanza(t *testing.T) {
	str := `<iq to="alice" from="bob" id="1" type="A"` +
		` xml:lang="en"><foo>text</foo></iq>`
	st, err := ParseStanza(str)
	if err != nil {
		t.Fatalf("iq: %v", err)
	}
	assertEquals(t, "iq", st.XName())
	assertEquals(t, "alice", st.XTo())
	assertEquals(t, "bob", st.XFrom())
	assertEquals(t, "1", st.XId())
	assertEquals(t, "A", st.XType())
	assertEquals(t, "en", st.XLang())
	if st.XError() != nil {
		t.Errorf("iq: error %v", st.XError())
	}
	if st.XChild() == nil {
		t.Errorf("iq: nil child")
	}
	assertEquals(t, "foo", st.XChild().XMLName.Local)
	assertEquals(t, "text", st.XChild().Chardata)

	str = `<message to="alice" from="bob"/>`
	st, err = ParseStanza(str)
	if err != nil {
		t.Fatalf("message: %v", err)
	}
	assertEquals(t, "message", st.XName())
	assertEquals(t, "alice", st.XTo())
	assertEquals(t, "bob", st.XFrom())
	assertEquals(t, "", st.XId())
	assertEquals(t, "", st.XLang())
	if st.XError() != nil {
		t.Errorf("message: error %v", st.XError())
	}
	if st.XChild() != nil {
		t.Errorf("message: child %v", st.XChild())
	}

	str = `<presence/>`
	st, err = ParseStanza(str)
	if err != nil {
		t.Fatalf("presence: %v", err)
	}
	assertEquals(t, "presence", st.XName())
}
