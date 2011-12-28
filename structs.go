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
	// BUG(cjyar) Make this not a pointer.
	Node *string
	Domain string
	Resource string
}
var _ fmt.Stringer = &JID{}
var _ flag.Value = &JID{}

// XMPP's <stream:stream> XML element
type stream struct {
	To string `xml:"attr"`
	From string `xml:"attr"`
	Id string `xml:"attr"`
	Lang string `xml:"attr"`
	Version string `xml:"attr"`
}
var _ xml.Marshaler = &stream{}
var _ fmt.Stringer = &stream{}

// <stream:error>
// BUG(cjyar) Can this be consolidated with Error? Hide it if not.
type StreamError struct {
	Any definedCondition
	Text *errText
}
var _ xml.Marshaler = &StreamError{}

// BUG(cjyar) Replace this with Generic.
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

// BUG(cjyar) Store this in Client and make it available to the app.
type Features struct {
	Starttls *starttls
	Mechanisms mechs
	Bind *Generic
}

type starttls struct {
	XMLName xml.Name
	Required *string
}

type mechs struct {
	Mechanism []string
}

type auth struct {
	XMLName xml.Name
	Chardata string `xml:"chardata"`
	Mechanism string `xml:"attr"`
	Any *Generic
}

// One of the three core XMPP stanza types: iq, message, presence. See
// RFC3920, section 9.
type Stanza interface {
	// Returns "iq", "message", or "presence".
	XName() string
	// The to attribute.
	XTo() string
	// The from attribute.
	XFrom() string
	// The id attribute.
	XId() string
	// The type attribute.
	XType() string
	// The xml:lang attribute.
	XLang() string
	// A nested error element, if any.
	XError() *Error
	// A (non-error) nested element, if any.
	XChild() *Generic
}

// message stanza
type Message struct {
	To string `xml:"attr"`
	From string `xml:"attr"`
	Id string `xml:"attr"`
	Type string `xml:"attr"`
	Lang string `xml:"attr"`
	Error *Error
	Any *Generic
}
var _ xml.Marshaler = &Message{}
var _ Stanza = &Message{}

// presence stanza
type Presence struct {
	To string `xml:"attr"`
	From string `xml:"attr"`
	Id string `xml:"attr"`
	Type string `xml:"attr"`
	Lang string `xml:"attr"`
	Error *Error
	Any *Generic
}
var _ xml.Marshaler = &Presence{}
var _ Stanza = &Presence{}

// iq stanza
type Iq struct {
	To string `xml:"attr"`
	From string `xml:"attr"`
	Id string `xml:"attr"`
	Type string `xml:"attr"`
	Lang string `xml:"attr"`
	Error *Error
	Any *Generic
}
var _ xml.Marshaler = &Iq{}
var _ Stanza = &Iq{}

// Describes an XMPP stanza error. See RFC 3920, Section 9.3.
type Error struct {
	// The error type attribute.
	Type string `xml:"attr"`
	// Any nested element, if present.
	Any *Generic
}
var _ xml.Marshaler = &Error{}

// Holds an XML element not described by the more specific types.
type Generic struct {
	XMLName xml.Name
	Any *Generic
	Chardata string `xml:"chardata"`
}
var _ fmt.Stringer = &Generic{}

func (jid *JID) String() string {
	result := jid.Domain
	if jid.Node != nil {
		result = *jid.Node + "@" + result
	}
	if jid.Resource != "" {
		result = result + "/" + jid.Resource
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
	jid.Resource = parts[5]
	return true
}

func (s *stream) MarshalXML() ([]byte, os.Error) {
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

func (s *stream) String() string {
	result, _ := s.MarshalXML()
	return string(result)
}

func parseStream(se xml.StartElement) (*stream, os.Error) {
	s := &stream{}
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

func (u *Generic) String() string {
	var sub string
	if u.Any != nil {
		sub = u.Any.String()
	}
	return fmt.Sprintf("<%s %s>%s%s</%s %s>", u.XMLName.Space,
		u.XMLName.Local, sub, u.Chardata, u.XMLName.Space,
		u.XMLName.Local)
}

func marshalXML(st Stanza) ([]byte, os.Error) {
	buf := bytes.NewBuffer(nil)
	buf.WriteString("<")
	buf.WriteString(st.XName())
	if st.XTo() != "" {
		writeField(buf, "to", st.XTo())
	}
	if st.XFrom() != "" {
		writeField(buf, "from", st.XFrom())
	}
	if st.XId() != "" {
		writeField(buf, "id", st.XId())
	}
	if st.XType() != "" {
		writeField(buf, "type", st.XType())
	}
	if st.XLang() != "" {
		writeField(buf, "xml:lang", st.XLang())
	}
	buf.WriteString(">")
	if st.XError() != nil {
		bytes, _ := st.XError().MarshalXML()
		buf.WriteString(string(bytes))
	}
	if st.XChild() != nil {
		xml.Marshal(buf, st.XChild())
	}
	buf.WriteString("</")
	buf.WriteString(st.XName())
	buf.WriteString(">")
	return buf.Bytes(), nil
}

func (er *Error) MarshalXML() ([]byte, os.Error) {
	buf := bytes.NewBuffer(nil)
	buf.WriteString("<error")
	writeField(buf, "type", er.Type)
	buf.WriteString(">")
	if er.Any != nil {
		xml.Marshal(buf, er.Any)
	}
	buf.WriteString("</error>")
	return buf.Bytes(), nil
}

func (m *Message) XName() string {
	return "message"
}

func (m *Message) XTo() string {
	return m.To
}

func (m *Message) XFrom() string {
	return m.From
}

func (m *Message) XId() string {
	return m.Id
}

func (m *Message) XType() string {
	return m.Type
	}

func (m *Message) XLang() string {
	return m.Lang
}

func (m *Message) XError() *Error {
	return m.Error
}

func (m *Message) XChild() *Generic {
	return m.Any
}

func (m *Message) MarshalXML() ([]byte, os.Error) {
	return marshalXML(m)
}

func (p *Presence) XName() string {
	return "presence"
}

func (p *Presence) XTo() string {
	return p.To
}

func (p *Presence) XFrom() string {
	return p.From
}

func (p *Presence) XId() string {
	return p.Id
}

func (p *Presence) XType() string {
	return p.Type
	}

func (p *Presence) XLang() string {
	return p.Lang
}

func (p *Presence) XError() *Error {
	return p.Error
}

func (p *Presence) XChild() *Generic {
	return p.Any
}

func (p *Presence) MarshalXML() ([]byte, os.Error) {
	return marshalXML(p)
}

func (iq *Iq) XName() string {
	return "iq"
}

func (iq *Iq) XTo() string {
	return iq.To
}

func (iq *Iq) XFrom() string {
	return iq.From
}

func (iq *Iq) XId() string {
	return iq.Id
}

func (iq *Iq) XType() string {
	return iq.Type
	}

func (iq *Iq) XLang() string {
	return iq.Lang
}

func (iq *Iq) XError() *Error {
	return iq.Error
}

func (iq *Iq) XChild() *Generic {
	return iq.Any
}

func (iq *Iq) MarshalXML() ([]byte, os.Error) {
	return marshalXML(iq)
}
