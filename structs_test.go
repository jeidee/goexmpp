// Copyright 2011 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package xmpp

import (
	"bytes"
	"encoding/xml"
	"testing"
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
	if err := jid.Set(str); err != nil {
		t.Errorf("Set(%s) failed: %s", str, err)
	}
	assertEquals(t, "user", jid.Node)
	assertEquals(t, "domain", jid.Domain)
	assertEquals(t, "res", jid.Resource)
	assertEquals(t, str, jid.String())

	str = "domain.tld"
	if err := jid.Set(str); err != nil {
		t.Errorf("Set(%s) failed: %s", str, err)
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
	var buf bytes.Buffer
	enc := xml.NewEncoder(&buf)
	enc.Context.Map[NsClient] = ""
	enc.Context.Map[NsStream] = "stream"
	err := enc.Encode(marshal)
	if err != nil {
		t.Errorf("Marshal error for %s: %s", marshal, err)
	}
	observed := buf.String()
	assertEquals(t, expected, observed)
}

func TestStreamMarshal(t *testing.T) {
	s := &stream{To: "bob"}
	exp := `<stream:stream xmlns="` + NsClient +
		`" xmlns:stream="` + NsStream + `" to="bob">`
	assertEquals(t, exp, s.String())

	s = &stream{To: "bob", From: "alice", Id: "#3", Version: "5.3"}
	exp = `<stream:stream xmlns="` + NsClient +
		`" xmlns:stream="` + NsStream + `" to="bob" from="alice"` +
		` id="#3" version="5.3">`
	assertEquals(t, exp, s.String())

	s = &stream{Lang: "en_US"}
	exp = `<stream:stream xmlns="` + NsClient +
		`" xmlns:stream="` + NsStream + `" xml:lang="en_US">`
	assertEquals(t, exp, s.String())
}

func TestStreamErrorMarshal(t *testing.T) {
	name := xml.Name{Space: NsStreams, Local: "ack"}
	e := &streamError{Any: Generic{XMLName: name}}
	exp := `<stream:error><ack xmlns="` + NsStreams +
		`"></ack></stream:error>`
	assertMarshal(t, exp, e)

	txt := errText{Lang: "pt", Text: "things happen"}
	e = &streamError{Any: Generic{XMLName: name}, Text: &txt}
	exp = `<stream:error><ack xmlns="` + NsStreams +
		`"></ack><text xmlns="` + NsStreams +
		`" xml:lang="pt">things happen</text></stream:error>`
	assertMarshal(t, exp, e)
}

func TestIqMarshal(t *testing.T) {
	iq := &Iq{Type: "set", Id: "3", Nested: []interface{}{Generic{XMLName: xml.Name{Space: NsBind,
		Local: "bind"}}}}
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
	iq, ok := st.(*Iq)
	if !ok {
		t.Fatalf("not iq: %v", st)
	}
	assertEquals(t, "iq", iq.XMLName.Local)
	assertEquals(t, "alice", iq.To)
	assertEquals(t, "bob", iq.From)
	assertEquals(t, "1", iq.Id)
	assertEquals(t, "A", iq.Type)
	assertEquals(t, "en", iq.Lang)
	if iq.Error != nil {
		t.Errorf("iq: error %v", iq.Error)
	}
	if st.innerxml() == "" {
		t.Errorf("iq: empty child")
	}
	assertEquals(t, "<foo>text</foo>", st.innerxml())
	assertEquals(t, st.innerxml(), iq.Innerxml)

	str = `<message to="alice" from="bob"/>`
	st, err = ParseStanza(str)
	if err != nil {
		t.Fatalf("message: %v", err)
	}
	m, ok := st.(*Message)
	if !ok {
		t.Fatalf("not message: %v", st)
	}
	assertEquals(t, "message", m.XMLName.Local)
	assertEquals(t, "alice", m.To)
	assertEquals(t, "bob", m.From)
	assertEquals(t, "", m.Id)
	assertEquals(t, "", m.Lang)
	if m.Error != nil {
		t.Errorf("message: error %v", m.Error)
	}
	if st.innerxml() != "" {
		t.Errorf("message: child %v", st.innerxml())
	}

	str = `<presence/>`
	st, err = ParseStanza(str)
	if err != nil {
		t.Fatalf("presence: %v", err)
	}
	_, ok = st.(*Presence)
	if !ok {
		t.Fatalf("not presence: %v", st)
	}
}

func TestMarshalEscaping(t *testing.T) {
	msg := &Message{Body: &Generic{XMLName: xml.Name{Local: "body"},
		Chardata: `&<!-- "`}}
	exp := `<message><body>&amp;&lt;!-- &#34;</body></message>`
	assertMarshal(t, exp, msg)
}
