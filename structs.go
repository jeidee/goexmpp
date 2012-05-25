// Copyright 2011 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package xmpp

// This file contains data structures.

import (
	"bytes"
	"encoding/xml"
	"errors"
	"flag"
	"fmt"
	"io"
	"regexp"
	"strings"
)

// JID represents an entity that can communicate with other
// entities. It looks like node@domain/resource. Node and resource are
// sometimes optional.
type JID struct {
	Node     string
	Domain   string
	Resource string
}

var _ fmt.Stringer = &JID{}
var _ flag.Value = &JID{}

// XMPP's <stream:stream> XML element
type stream struct {
	XMLName xml.Name `xml:"stream http://etherx.jabber.org/streams"`
	To      string `xml:"to,attr,omitempty"`
	From    string `xml:"from,attr,omitempty"`
	Id      string `xml:"id,attr,omitempty"`
	Lang    string `xml:"lang,attr,omitempty"`
	Version string `xml:"version,attr,omitempty"`
}

var _ fmt.Stringer = &stream{}

// <stream:error>
type streamError struct {
	XMLName xml.Name `xml:"stream:error"`
	Any  Generic  `xml:",any"`
	Text *errText `xml:"text"`
}

type errText struct {
	XMLName xml.Name `xml:"urn:ietf:params:xml:ns:xmpp-streams text"`
	Lang string `xml:"lang,attr"`
	Text string `xml:",chardata"`
}

type Features struct {
	Starttls   *starttls `xml:"starttls"`
	Mechanisms mechs     `xml:"mechanisms"`
	Bind       *bindIq   `xml:"bind"`
	Session    *Generic  `xml:"session"`
	Any        *Generic  `xml:",any"`
}

type starttls struct {
	XMLName  xml.Name
	Required *string `xml:"required"`
}

type mechs struct {
	Mechanism []string `xml:"mechanism"`
}

type auth struct {
	XMLName   xml.Name
	Chardata  string   `xml:",chardata"`
	Mechanism string   `xml:"mechanism,attr"`
	Any       *Generic `xml:",any"`
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
	// Zero or more (non-error) nested elements. These will be in
	// namespaces managed by extensions.
	GetNested() []interface{}
	addNested(interface{})
	innerxml() string
}

// message stanza
type Message struct {
	XMLName  xml.Name `xml:"message"`
	To       string   `xml:"to,attr,omitempty"`
	From     string   `xml:"from,attr,omitempty"`
	Id       string   `xml:"id,attr,omitempty"`
	Type     string   `xml:"type,attr,omitempty"`
	Lang     string   `xml:"lang,attr,omitempty"`
	Innerxml string   `xml:",innerxml"`
	Error    *Error   `xml:"error"`
	Subject  *Generic `xml:"subject"`
	Body     *Generic `xml:"body"`
	Thread   *Generic `xml:"thread"`
	Nested   []interface{}
}

var _ Stanza = &Message{}

// presence stanza
type Presence struct {
	XMLName  xml.Name `xml:"presence"`
	To       string   `xml:"to,attr,omitempty"`
	From     string   `xml:"from,attr,omitempty"`
	Id       string   `xml:"id,attr,omitempty"`
	Type     string   `xml:"type,attr,omitempty"`
	Lang     string   `xml:"lang,attr,omitempty"`
	Innerxml string   `xml:",innerxml"`
	Error    *Error   `xml:"error"`
	Show     *Generic `xml:"show"`
	Status   *Generic `xml:"status"`
	Priority *Generic `xml:"priority"`
	Nested   []interface{}
}

var _ Stanza = &Presence{}

// iq stanza
type Iq struct {
	XMLName  xml.Name `xml:"iq"`
	To       string `xml:"to,attr,omitempty"`
	From     string `xml:"from,attr,omitempty"`
	Id       string `xml:"id,attr,omitempty"`
	Type     string `xml:"type,attr,omitempty"`
	Lang     string `xml:"xml lang,attr,omitempty"`
	Innerxml string `xml:",innerxml"`
	Error    *Error `xml:"error"`
	Nested   []interface{}
}

var _ Stanza = &Iq{}

// Describes an XMPP stanza error. See RFC 3920, Section 9.3.
type Error struct {
	XMLName xml.Name `xml:"error"`
	// The error type attribute.
	Type string `xml:"type,attr"`
	// Any nested element, if present.
	Any *Generic `xml:",any"`
}

var _ error = &Error{}

// Used for resource binding as a nested element inside <iq/>.
type bindIq struct {
	XMLName  xml.Name `xml:"urn:ietf:params:xml:ns:xmpp-bind bind"`
	Resource *string  `xml:"resource"`
	Jid      *string  `xml:"jid"`
}

// Holds an XML element not described by the more specific types.
type Generic struct {
	XMLName  xml.Name
	Any      *Generic `xml:",any"`
	Chardata string   `xml:",chardata"`
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

// Set implements flag.Value. It returns true if it successfully
// parses the string.
func (jid *JID) Set(val string) error {
	r := regexp.MustCompile("^(([^@/]+)@)?([^@/]+)(/([^@/]+))?$")
	parts := r.FindStringSubmatch(val)
	if parts == nil {
		return errors.New("Can't parse as JID: " + val)
	}
	jid.Node = parts[2]
	jid.Domain = parts[3]
	jid.Resource = parts[5]
	return nil
}

func (s *stream) String() string {
	buf := bytes.NewBuffer(nil)
	buf.WriteString("<stream:stream")
	writeField(buf, "to", s.To)
	writeField(buf, "from", s.From)
	writeField(buf, "id", s.Id)
	writeField(buf, "xml:lang", s.Lang)
	writeField(buf, "version", s.Version)
	buf.WriteString(">")
	return buf.String()
}

func parseStream(se xml.StartElement) (*stream, error) {
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
	if u == nil {
		return "nil"
	}
	var sub string
	if u.Any != nil {
		sub = u.Any.String()
	}
	return fmt.Sprintf("<%s %s>%s%s</%s %s>", u.XMLName.Space,
		u.XMLName.Local, sub, u.Chardata, u.XMLName.Space,
		u.XMLName.Local)
}

func (er *Error) Error() string {
	buf := bytes.NewBuffer(nil)
	xml.NewEncoder(buf).Encode(er)
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

func (m *Message) GetNested() []interface{} {
	return m.Nested
}

func (m *Message) addNested(n interface{}) {
	m.Nested = append(m.Nested, n)
}

func (m *Message) innerxml() string {
	return m.Innerxml
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

func (p *Presence) GetNested() []interface{} {
	return p.Nested
}

func (p *Presence) addNested(n interface{}) {
	p.Nested = append(p.Nested, n)
}

func (p *Presence) innerxml() string {
	return p.Innerxml
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

func (iq *Iq) GetNested() []interface{} {
	return iq.Nested
}

func (iq *Iq) addNested(n interface{}) {
	iq.Nested = append(iq.Nested, n)
}

func (iq *Iq) innerxml() string {
	return iq.Innerxml
}

// Parse a string into a struct implementing Stanza -- this will be
// either an Iq, a Message, or a Presence.
func ParseStanza(str string) (Stanza, error) {
	r := strings.NewReader(str)
	p := xml.NewDecoder(r)
	tok, err := p.Token()
	if err != nil {
		return nil, err
	}
	se, ok := tok.(xml.StartElement)
	if !ok {
		return nil, errors.New("Not a start element")
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
		return nil, errors.New("Not iq, message, or presence")
	}
	err = p.DecodeElement(stan, &se)
	if err != nil {
		return nil, err
	}
	return stan, nil
}

var bindExt Extension = Extension{StanzaHandlers: map[string]func(*xml.Name) interface{}{NsBind: newBind},
	Start: func(cl *Client) {}}

func newBind(name *xml.Name) interface{} {
	return &bindIq{}
}
