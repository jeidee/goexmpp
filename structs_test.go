// Copyright 2011 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package xmpp

import (
	"bytes"
	"testing"
	"xml"
)

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

func assertMarshal(t *testing.T, expected string, marshal interface{}) {
	buf := bytes.NewBuffer(nil)
	xml.Marshal(buf, marshal)
	observed := string(buf.Bytes())
	if expected != observed {
		t.Errorf("Expected:\n%s\nObserved:\n%s\n", expected,
			observed)
	}
}
