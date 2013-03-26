// Copyright 2011 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package xmpp

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"os"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func assertEquals(t *testing.T, expected, observed string) {
	if expected != observed {
		file := "unknown"
		line := 0
		_, file, line, _ = runtime.Caller(1)
		fmt.Fprintf(os.Stderr, "%s:%d: Expected:\n%s\nObserved:\n%s\n",
			file, line, expected, observed)
		t.Fail()
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
	err := enc.Encode(marshal)
	if err != nil {
		t.Errorf("Marshal error for %s: %s", marshal, err)
	}
	observed := buf.String()
	if expected != observed {
		file := "unknown"
		line := 0
		_, file, line, _ = runtime.Caller(1)
		fmt.Fprintf(os.Stderr, "%s:%d: Expected:\n%s\nObserved:\n%s\n",
			file, line, expected, observed)
		t.Fail()
	}
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
	exp := `<error xmlns="` + NsStream + `"><ack xmlns="` + NsStreams +
		`"></ack></error>`
	assertMarshal(t, exp, e)

	txt := errText{Lang: "pt", Text: "things happen"}
	e = &streamError{Any: Generic{XMLName: name}, Text: &txt}
	exp = `<error xmlns="` + NsStream + `"><ack xmlns="` + NsStreams +
		`"></ack><text xmlns="` + NsStreams +
		`" xml:lang="pt">things happen</text></error>`
	assertMarshal(t, exp, e)
}

func TestIqMarshal(t *testing.T) {
	iq := &Iq{Header: Header{Type: "set", Id: "3",
		Nested: []interface{}{Generic{XMLName: xml.Name{Space: NsBind,
			Local: "bind"}}}}}
	exp := `<iq id="3" type="set"><bind xmlns="` + NsBind +
		`"></bind></iq>`
	assertMarshal(t, exp, iq)
}

func TestMarshalEscaping(t *testing.T) {
	msg := &Message{Body: &Generic{XMLName: xml.Name{Local: "body"},
		Chardata: `&<!-- "`}}
	exp := `<message xmlns="jabber:client"><body>&amp;&lt;!-- &#34;</body></message>`
	assertMarshal(t, exp, msg)
}

func TestUnmarshalMessage(t *testing.T) {
	str := `<message to="a@b.c"><body>foo!</body></message>`
	r := strings.NewReader(str)
	ch := make(chan interface{})
	go readXml(r, ch, make(map[string]func(*xml.Name) interface{}))
	obs := <-ch
	exp := &Message{XMLName: xml.Name{Local: "message", Space: "jabber:client"},
		Header: Header{To: "a@b.c", Innerxml: "<body>foo!</body>"},
		Body: &Generic{XMLName: xml.Name{Local: "body", Space: "jabber:client"},
			Chardata: "foo!"}}
	if !reflect.DeepEqual(obs, exp) {
		t.Errorf("read %s\ngot:  %#v\nwant: %#v\n", str, obs, exp)
	}
	obsMsg, ok := obs.(*Message)
	if !ok {
		t.Fatalf("Not a Message: %T", obs)
	}
	obsBody := obsMsg.Body
	expBody := exp.Body
	if !reflect.DeepEqual(obsBody, expBody) {
		t.Errorf("body\ngot:  %#v\nwant: %#v\n", obsBody, expBody)
	}
}
