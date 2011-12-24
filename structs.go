// Copyright 2011 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package xmpp

// This file contains data structures.

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"xml"
)

const (
	// Version of RFC 3920 that we implement.
	Version = "1.0"
	nsStreams = "urn:ietf:params:xml:ns:xmpp-streams"
	nsStream = "http://etherx.jabber.org/streams"
	nsTLS = "urn:ietf:params:xml:ns:xmpp-tls"
)

// JID represents an entity that can communicate with other
// entities. It looks like node@domain/resource. Node and resource are
// sometimes optional.
type JID struct {
	Node *string
	Domain string
	Resource *string
}
var _ fmt.Stringer = &JID{}

// XMPP's <stream:stream> XML element
type Stream struct {
	to string `xml:"attr"`
	from string `xml:"attr"`
	id string `xml:"attr"`
	lang string `xml:"attr"`
	version string `xml:"attr"`
}
var _ xml.Marshaler = &Stream{}

type StreamError struct {
	cond definedCondition
	text errText
}
var _ xml.Marshaler = &StreamError{}

type definedCondition struct {
	// Must always be in namespace nsStreams
	XMLName xml.Name
}

type errText struct {
	XMLName xml.Name
	Lang string
	text string `xml:"chardata"`
}
var _ xml.Marshaler = &errText{}

func (jid *JID) String() string {
	result := jid.Domain
	if jid.Node != nil {
		result = *jid.Node + "@" + result
	}
	if jid.Resource != nil {
		result = result + "/" + *jid.Resource
	}
	return result
}

func (s *Stream) MarshalXML() ([]byte, os.Error) {
	buf := bytes.NewBuffer(nil)
	buf.WriteString("<stream:stream")
	writeField(buf, "to", s.to)
	writeField(buf, "from", s.from)
	writeField(buf, "id", s.id)
	writeField(buf, "xml:lang", s.lang)
	writeField(buf, "version", s.version)
	buf.WriteString(">")
	// We never write </stream:stream>
	return buf.Bytes(), nil
}

func (s *StreamError) MarshalXML() ([]byte, os.Error) {
	buf := bytes.NewBuffer(nil)
	buf.WriteString("<stream:error>")
	xml.Marshal(buf, s.cond)
	xml.Marshal(buf, s.text)
	buf.WriteString("</stream:error>")
	return buf.Bytes(), nil
}

func (e *errText) MarshalXML() ([]byte, os.Error) {
	buf := bytes.NewBuffer(nil)
	buf.WriteString("<text")
	writeField(buf, "xmlns", nsStreams)
	writeField(buf, "xml:lang", e.Lang)
	buf.WriteString(">")
	xml.Escape(buf, []byte(e.text))
	buf.WriteString("</text>")
	return buf.Bytes(), nil
}

func writeField(w io.Writer, field, value string) {
	if value != "" {
		io.WriteString(w, " ")
		io.WriteString(w, field)
		io.WriteString(w, `="`)
		xml.Escape(w, []byte(value))
		io.WriteString(w, `"`)
	}
}
