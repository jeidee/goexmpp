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
	Node string
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
type streamError struct {
	Any Generic
	Text *errText
}
var _ xml.Marshaler = &streamError{}

type errText struct {
	Lang string `xml:"attr"`
	Text string `xml:"chardata"`
}
var _ xml.Marshaler = &errText{}

type Features struct {
	Starttls *starttls
	Mechanisms mechs
	Bind *bindIq
	Session *Generic
	Any *Generic
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
	GetName() string
	// The to attribute.
	GetTo() string
	// The from attribute.
	GetFrom() string
	// The id attribute.
	GetId() string
	// The type attribute.
	GetType() string
	// The xml:lang attribute.
	GetLang() string
	// A nested error element, if any.
	GetError() *Error
	// A (non-error) nested element, if any.
	GetNested() interface{}
	setNested(interface{})
	generic() *Generic
	innerxml() string
}

// message stanza
type Message struct {
	To string `xml:"attr"`
	From string `xml:"attr"`
	Id string `xml:"attr"`
	Type string `xml:"attr"`
	Lang string `xml:"attr"`
	Innerxml string `xml:"innerxml"`
	Error *Error
	Subject *Generic
	Body *Generic
	Thread *Generic
	Any *Generic
	Nested interface{}
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
	Innerxml string `xml:"innerxml"`
	Error *Error
	Show *Generic
	Status *Generic
	Priority *Generic
	Any *Generic
	Nested interface{}
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
	Innerxml string `xml:"innerxml"`
	Error *Error
	Any *Generic
	Nested interface{}
}
var _ xml.Marshaler = &Iq{}
var _ Stanza = &Iq{}

// Describes an XMPP stanza error. See RFC 3920, Section 9.3.
type Error struct {
	XMLName xml.Name `xml:"error"`
	// The error type attribute.
	Type string `xml:"attr"`
	// Any nested element, if present.
	Any *Generic
}
var _ os.Error = &Error{}

// Used for resource binding as a nested element inside <iq/>.
type bindIq struct {
	XMLName xml.Name `xml:"urn:ietf:params:xml:ns:xmpp-bind bind"`
	Resource *string `xml:"resource"`
	Jid *string `xml:"jid"`
}

// Holds an XML element not described by the more specific types.
type Generic struct {
	XMLName xml.Name
	Any *Generic
	Chardata string `xml:"chardata"`
}
var _ fmt.Stringer = &Generic{}

func (jid *JID) String() string {
	result := jid.Domain
	if jid.Node != "" {
		result = jid.Node + "@" + result
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
	jid.Node = parts[2]
	jid.Domain = parts[3]
	jid.Resource = parts[5]
	return true
}

func (s *stream) MarshalXML() ([]byte, os.Error) {
	buf := bytes.NewBuffer(nil)
	buf.WriteString("<stream:stream")
	writeField(buf, "xmlns", "jabber:client")
	writeField(buf, "xmlns:stream", NsStream)
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

func (s *streamError) MarshalXML() ([]byte, os.Error) {
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
	writeField(buf, "xmlns", NsStreams)
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
	buf.WriteString(st.GetName())
	if st.GetTo() != "" {
		writeField(buf, "to", st.GetTo())
	}
	if st.GetFrom() != "" {
		writeField(buf, "from", st.GetFrom())
	}
	if st.GetId() != "" {
		writeField(buf, "id", st.GetId())
	}
	if st.GetType() != "" {
		writeField(buf, "type", st.GetType())
	}
	if st.GetLang() != "" {
		writeField(buf, "xml:lang", st.GetLang())
	}
	buf.WriteString(">")

	if st.GetNested() != nil {
		xml.Marshal(buf, st.GetNested())
	} else if st.generic() != nil {
		xml.Marshal(buf, st.generic())
	} else if st.innerxml() != "" {
		buf.WriteString(st.innerxml())
	}

	buf.WriteString("</")
	buf.WriteString(st.GetName())
	buf.WriteString(">")
	return buf.Bytes(), nil
}

func (er *Error) String() string {
	buf := bytes.NewBuffer(nil)
	xml.Marshal(buf, er)
	return buf.String()
}

func (m *Message) GetName() string {
	return "message"
}

func (m *Message) GetTo() string {
	return m.To
}

func (m *Message) GetFrom() string {
	return m.From
}

func (m *Message) GetId() string {
	return m.Id
}

func (m *Message) GetType() string {
	return m.Type
	}

func (m *Message) GetLang() string {
	return m.Lang
}

func (m *Message) GetError() *Error {
	return m.Error
}

func (m *Message) GetNested() interface{} {
	return m.Nested
}

func (m *Message) setNested(n interface{}) {
	m.Nested = n
}

func (m *Message) generic() *Generic {
	return m.Any
}

func (m *Message) innerxml() string {
	return m.Innerxml
}

func (m *Message) MarshalXML() ([]byte, os.Error) {
	return marshalXML(m)
}

func (m *Message) InnerMarshal(w io.Writer) os.Error {
	err := xml.Marshal(w, m.Subject)
	if err != nil {
		return err
	}
	err = xml.Marshal(w, m.Body)
	if err != nil {
		return err
	}
	err = xml.Marshal(w, m.Thread)
	if err != nil {
		return err
	}
	return nil
}

func (p *Presence) GetName() string {
	return "presence"
}

func (p *Presence) GetTo() string {
	return p.To
}

func (p *Presence) GetFrom() string {
	return p.From
}

func (p *Presence) GetId() string {
	return p.Id
}

func (p *Presence) GetType() string {
	return p.Type
	}

func (p *Presence) GetLang() string {
	return p.Lang
}

func (p *Presence) GetError() *Error {
	return p.Error
}

func (p *Presence) GetNested() interface{} {
	return p.Nested
}

func (p *Presence) setNested(n interface{}) {
	p.Nested = n
}

func (p *Presence) generic() *Generic {
	return p.Any
}

func (p *Presence) innerxml() string {
	return p.Innerxml
}

func (p *Presence) MarshalXML() ([]byte, os.Error) {
	return marshalXML(p)
}

func (p *Presence) InnerMarshal(w io.Writer) os.Error {
	err := xml.Marshal(w, p.Show)
	if err != nil {
		return err
	}
	err = xml.Marshal(w, p.Status)
	if err != nil {
		return err
	}
	err = xml.Marshal(w, p.Priority)
	if err != nil {
		return err
	}
	return nil
}

func (iq *Iq) GetName() string {
	return "iq"
}

func (iq *Iq) GetTo() string {
	return iq.To
}

func (iq *Iq) GetFrom() string {
	return iq.From
}

func (iq *Iq) GetId() string {
	return iq.Id
}

func (iq *Iq) GetType() string {
	return iq.Type
	}

func (iq *Iq) GetLang() string {
	return iq.Lang
}

func (iq *Iq) GetError() *Error {
	return iq.Error
}

func (iq *Iq) GetNested() interface{} {
	return iq.Nested
}

func (iq *Iq) setNested(n interface{}) {
	iq.Nested = n
}

func (iq *Iq) generic() *Generic {
	return iq.Any
}

func (iq *Iq) innerxml() string {
	return iq.Innerxml
}

func (iq *Iq) MarshalXML() ([]byte, os.Error) {
	return marshalXML(iq)
}

// Parse a string into a struct implementing Stanza -- this will be
// either an Iq, a Message, or a Presence.
func ParseStanza(str string) (Stanza, os.Error) {
	r := strings.NewReader(str)
	p := xml.NewParser(r)
	tok, err := p.Token()
	if err != nil {
		return nil, err
	}
	se, ok := tok.(xml.StartElement)
	if !ok {
		return nil, os.NewError("Not a start element")
	}
	var stan Stanza
	switch se.Name.Local {
	case "iq":
		stan = &Iq{}
	case "message":
		stan = &Message{}
	case "presence":
		stan = &Presence{}
	default:
		return nil, os.NewError("Not iq, message, or presence")
	}
	err = p.Unmarshal(stan, &se)
	if err != nil {
		return nil, err
	}
	return stan, nil
}

func newBind(name *xml.Name) interface{} {
	return &bindIq{}
}
