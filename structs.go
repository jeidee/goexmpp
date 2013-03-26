// Copyright 2011 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package xmpp

// This file contains data structures.

import (
	"bytes"
	"encoding/xml"
	"flag"
	"fmt"
	// BUG(cjyar): We should use stringprep
	// "code.google.com/p/go-idn/src/stringprep"
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
	XMLName xml.Name `xml:"stream=http://etherx.jabber.org/streams stream"`
	To      string   `xml:"to,attr"`
	From    string   `xml:"from,attr"`
	Id      string   `xml:"id,attr"`
	Lang    string   `xml:"http://www.w3.org/XML/1998/namespace lang,attr"`
	Version string   `xml:"version,attr"`
}

var _ fmt.Stringer = &stream{}

// <stream:error>
type streamError struct {
	XMLName xml.Name `xml:"http://etherx.jabber.org/streams error"`
	Any     Generic  `xml:",any"`
	Text    *errText
}

type errText struct {
	XMLName xml.Name `xml:"urn:ietf:params:xml:ns:xmpp-streams text"`
	Lang    string   `xml:"http://www.w3.org/XML/1998/namespace lang,attr"`
	Text    string   `xml:",chardata"`
}

type Features struct {
	Starttls   *starttls `xml:"urn:ietf:params:xml:ns:xmpp-tls starttls"`
	Mechanisms mechs     `xml:"urn:ietf:params:xml:ns:xmpp-sasl mechanisms"`
	Bind       *bindIq
	Session    *Generic
	Any        *Generic
}

type starttls struct {
	XMLName  xml.Name
	Required *string
}

type mechs struct {
	Mechanism []string `xml:"urn:ietf:params:xml:ns:xmpp-sasl mechanism"`
}

type auth struct {
	XMLName   xml.Name
	Chardata  string `xml:",chardata"`
	Mechanism string `xml:"mechanism,attr,omitempty"`
	Any       *Generic
}

type Stanza interface {
	GetHeader() *Header
}

// One of the three core XMPP stanza types: iq, message, presence. See
// RFC3920, section 9.
type Header struct {
	To       string `xml:"to,attr,omitempty"`
	From     string `xml:"from,attr,omitempty"`
	Id       string `xml:"id,attr,omitempty"`
	Type     string `xml:"type,attr,omitempty"`
	Lang     string `xml:"http://www.w3.org/XML/1998/namespace lang,attr,omitempty"`
	Innerxml string `xml:",innerxml"`
	Error    *Error
	Nested   []interface{}
}

// message stanza
type Message struct {
	XMLName xml.Name `xml:"jabber:client message"`
	Header
	Subject *Generic `xml:"jabber:client subject"`
	Body    *Generic `xml:"jabber:client body"`
	Thread  *Generic `xml:"jabber:client thread"`
}

var _ Stanza = &Message{}

// presence stanza
type Presence struct {
	XMLName xml.Name `xml:"presence"`
	Header
	Show     *Generic `xml:"jabber:client show"`
	Status   *Generic `xml:"jabber:client status"`
	Priority *Generic `xml:"jabber:client priority"`
}

var _ Stanza = &Presence{}

// iq stanza
type Iq struct {
	XMLName xml.Name `xml:"iq"`
	Header
}

var _ Stanza = &Iq{}

// Describes an XMPP stanza error. See RFC 3920, Section 9.3.
type Error struct {
	XMLName xml.Name `xml:"error"`
	// The error type attribute.
	Type string `xml:"type,attr"`
	// Any nested element, if present.
	Any *Generic
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
		return fmt.Errorf("%s doesn't match user@domain/resource", val)
	}
	// jid.Node = stringprep.Nodeprep(parts[2])
	// jid.Domain = stringprep.Nodeprep(parts[3])
	// jid.Resource = stringprep.Resourceprep(parts[5])
	jid.Node = parts[2]
	jid.Domain = parts[3]
	jid.Resource = parts[5]
	return nil
}

func (s *stream) String() string {
	var buf bytes.Buffer
	buf.WriteString(`<stream:stream xmlns="`)
	buf.WriteString(NsClient)
	buf.WriteString(`" xmlns:stream="`)
	buf.WriteString(NsStream)
	buf.WriteString(`"`)
	if s.To != "" {
		buf.WriteString(` to="`)
		xml.Escape(&buf, []byte(s.To))
		buf.WriteString(`"`)
	}
	if s.From != "" {
		buf.WriteString(` from="`)
		xml.Escape(&buf, []byte(s.From))
		buf.WriteString(`"`)
	}
	if s.Id != "" {
		buf.WriteString(` id="`)
		xml.Escape(&buf, []byte(s.Id))
		buf.WriteString(`"`)
	}
	if s.Lang != "" {
		buf.WriteString(` xml:lang="`)
		xml.Escape(&buf, []byte(s.Lang))
		buf.WriteString(`"`)
	}
	if s.Version != "" {
		buf.WriteString(` version="`)
		xml.Escape(&buf, []byte(s.Version))
		buf.WriteString(`"`)
	}
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

func (iq *Iq) GetHeader() *Header {
	return &iq.Header
}

func (m *Message) GetHeader() *Header {
	return &m.Header
}

func (p *Presence) GetHeader() *Header {
	return &p.Header
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
	buf, err := xml.Marshal(er)
	if err != nil {
		Warn.Log("double bad error: couldn't marshal error")
		return "unreadable error"
	}
	return string(buf)
}

var bindExt Extension = Extension{StanzaHandlers: map[string]func(*xml.Name) interface{}{NsBind: newBind},
	Start: func(cl *Client) {}}

func newBind(name *xml.Name) interface{} {
	return &bindIq{}
}
