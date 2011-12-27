// Copyright 2011 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package xmpp

// This file contains data structures.

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"xml"
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
var _ flag.Value = &JID{}

// XMPP's <stream:stream> XML element
type Stream struct {
	To string `xml:"attr"`
	From string `xml:"attr"`
	Id string `xml:"attr"`
	Lang string `xml:"attr"`
	Version string `xml:"attr"`
}
var _ xml.Marshaler = &Stream{}
var _ fmt.Stringer = &Stream{}

// <stream:error>
type StreamError struct {
	Any definedCondition
	Text *errText
}
var _ xml.Marshaler = &StreamError{}

type definedCondition struct {
	// Must always be in namespace nsStreams
	XMLName xml.Name
	Chardata string `xml:"chardata"`
}

type errText struct {
	Lang string `xml:"attr"`
	Text string `xml:"chardata"`
}
var _ xml.Marshaler = &errText{}

type Features struct {
	Starttls *starttls
	Mechanisms mechs
}

type starttls struct {
	XMLName xml.Name
	required *string
}

type mechs struct {
	Mechanism []string
}

type auth struct {
	XMLName xml.Name
	Chardata string `xml:"chardata"`
	Mechanism string `xml:"attr"`
	Any *Unrecognized
}

type Unrecognized struct {
	XMLName xml.Name
}
var _ fmt.Stringer = &Unrecognized{}

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

func (jid *JID) Set(val string) bool {
	r := regexp.MustCompile("^(([^@/]+)@)?([^@/]+)(/([^@/]+))?$")
	parts := r.FindStringSubmatch(val)
	if parts == nil {
		return false
	}
	if parts[2] == "" {
		jid.Node = nil
	} else {
		jid.Node = &parts[2]
	}
	jid.Domain = parts[3]
	if parts[5] == "" {
		jid.Resource = nil
	} else {
		jid.Resource = &parts[5]
	}
	return true
}

func (s *Stream) MarshalXML() ([]byte, os.Error) {
	buf := bytes.NewBuffer(nil)
	buf.WriteString("<stream:stream")
	writeField(buf, "xmlns", "jabber:client")
	writeField(buf, "xmlns:stream", nsStream)
	writeField(buf, "to", s.To)
	writeField(buf, "from", s.From)
	writeField(buf, "id", s.Id)
	writeField(buf, "xml:lang", s.Lang)
	writeField(buf, "version", s.Version)
	buf.WriteString(">")
	// We never write </stream:stream>
	return buf.Bytes(), nil
}

func (s *Stream) String() string {
	result, _ := s.MarshalXML()
	return string(result)
}

func parseStream(se xml.StartElement) (*Stream, os.Error) {
	s := &Stream{}
	for _, attr := range se.Attr {
		switch strings.ToLower(attr.Name.Local) {
		case "to":
			s.To = attr.Value
		case "from":
			s.From = attr.Value
		case "id":
			s.Id = attr.Value
		case "lang":
			s.Lang = attr.Value
		case "version":
			s.Version = attr.Value
		}
	}
	return s, nil
}

func (s *StreamError) MarshalXML() ([]byte, os.Error) {
	buf := bytes.NewBuffer(nil)
	buf.WriteString("<stream:error>")
	xml.Marshal(buf, s.Any)
	if s.Text != nil {
		xml.Marshal(buf, s.Text)
	}
	buf.WriteString("</stream:error>")
	return buf.Bytes(), nil
}

func (e *errText) MarshalXML() ([]byte, os.Error) {
	buf := bytes.NewBuffer(nil)
	buf.WriteString("<text")
	writeField(buf, "xmlns", nsStreams)
	writeField(buf, "xml:lang", e.Lang)
	buf.WriteString(">")
	xml.Escape(buf, []byte(e.Text))
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

func (u *Unrecognized) String() string {
	return fmt.Sprintf("unrecognized{%s %s}", u.XMLName.Space,
		u.XMLName.Local)
}
