// Copyright 2011 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package xmpp

import (
	"encoding/xml"
	"fmt"
)

// This file contains support for roster management, RFC 3921, Section 7.

var rosterExt Extension = Extension{StanzaHandlers: map[string]func(*xml.Name) interface{}{NsRoster: newRosterQuery}, Start: startRosterFilter}

// Roster query/result
type RosterQuery struct {
	XMLName xml.Name     `xml:"jabber:iq:roster query"`
	Item    []RosterItem `xml:"item"`
}

// See RFC 3921, Section 7.1.
type RosterItem struct {
	XMLName      xml.Name `xml:"jabber:iq:roster item"`
	Jid          string   `xml:"jid,attr"`
	Subscription string   `xml:"subscription,attr"`
	Name         string   `xml:"name,attr"`
	Group        []string
}

type rosterClient struct {
	rosterChan   <-chan []RosterItem
	rosterUpdate chan<- RosterItem
}

var (
	rosterClients = make(map[string]rosterClient)
)

// Implicitly becomes part of NewClient's extStanza arg.
func newRosterQuery(name *xml.Name) interface{} {
	return &RosterQuery{}
}

// Synchronously fetch this entity's roster from the server and cache
// that information. This is called once from a fairly deep call stack
// as part of XMPP negotiation.
func fetchRoster(client *Client) error {
	rosterUpdate := rosterClients[client.Uid].rosterUpdate

	iq := &Iq{Header: Header{From: client.Jid.String(), Type: "get",
		Id: <-Id, Nested: []interface{}{RosterQuery{}}}}
	ch := make(chan error)
	f := func(v Stanza) bool {
		defer close(ch)
		iq, ok := v.(*Iq)
		if !ok {
			ch <- fmt.Errorf("response to iq wasn't iq: %s", v)
			return false
		}
		if iq.Type == "error" {
			ch <- iq.Error
			return false
		}
		var rq *RosterQuery
		for _, ele := range iq.Nested {
			if q, ok := ele.(*RosterQuery); ok {
				rq = q
				break
			}
		}
		if rq == nil {
			ch <- fmt.Errorf(
				"Roster query result not query: %v", v)
			return false
		}
		for _, item := range rq.Item {
			rosterUpdate <- item
		}
		ch <- nil
		return false
	}
	client.HandleStanza(iq.Id, f)
	client.Out <- iq
	// Wait for f to complete.
	return <-ch
}

// The roster filter updates the Client's representation of the
// roster, but it lets the relevant stanzas through. This also starts
// the roster feeder, which is the goroutine that provides data on
// client.Roster.
func startRosterFilter(client *Client) {
	out := make(chan Stanza)
	in := client.AddFilter(out)
	go func(in <-chan Stanza, out chan<- Stanza) {
		defer close(out)
		for st := range in {
			maybeUpdateRoster(client, st)
			out <- st
		}
	}(in, out)

	rosterCh := make(chan []RosterItem)
	rosterUpdate := make(chan RosterItem)
	rosterClients[client.Uid] = rosterClient{rosterChan: rosterCh,
		rosterUpdate: rosterUpdate}
	go feedRoster(rosterCh, rosterUpdate)
}

func maybeUpdateRoster(client *Client, st interface{}) {
	iq, ok := st.(*Iq)
	if !ok {
		return
	}

	rosterUpdate := rosterClients[client.Uid].rosterUpdate

	var rq *RosterQuery
	for _, ele := range iq.Nested {
		if q, ok := ele.(*RosterQuery); ok {
			rq = q
			break
		}
	}
	if iq.Type == "set" && rq != nil {
		for _, item := range rq.Item {
			rosterUpdate <- item
		}
		// Send a reply.
		reply := &Iq{Header: Header{To: iq.From, Id: iq.Id,
			Type: "result"}}
		client.Out <- reply
	}
}

func feedRoster(rosterCh chan<- []RosterItem, rosterUpdate <-chan RosterItem) {
	roster := make(map[string]RosterItem)
	snapshot := []RosterItem{}
	for {
		select {
		case newIt := <-rosterUpdate:
			if newIt.Subscription == "remove" {
				delete(roster, newIt.Jid)
			} else {
				roster[newIt.Jid] = newIt
			}
		case rosterCh <- snapshot:
		}
		snapshot = make([]RosterItem, 0, len(roster))
		for _, v := range roster {
			snapshot = append(snapshot, v)
		}
	}
}

// Retrieve a snapshot of the roster for the given Client.
func Roster(client *Client) []RosterItem {
	rosterChan := rosterClients[client.Uid].rosterChan
	return <-rosterChan
}
