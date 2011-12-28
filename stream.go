// Copyright 2011 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// This file contains the three layers of processing for the
// communication with the server: transport (where TLS happens), XML
// (where strings are converted to go structures), and Stream (where
// we respond to XMPP events on behalf of the library client), or send
// those events to the client.

package xmpp

import (
	"big"
	"crypto/md5"
	"crypto/rand"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"regexp"
	"strings"
	"time"
	"xml"
)

// Callback to handle a stanza with a particular id.
type stanzaHandler struct {
	id string
	// Return true means pass this to the application
	f func(Stanza) bool
}

// BUG(cjyar) Review all these *Client receiver methods. They should
// probably either all be receivers, or none.

func (cl *Client) readTransport(w io.Writer) {
	defer tryClose(cl.socket, w)
	cl.socket.SetReadTimeout(1e8)
	p := make([]byte, 1024)
	for {
		if cl.socket == nil {
			cl.waitForSocket()
		}
		nr, err := cl.socket.Read(p)
		if nr == 0 {
			if errno, ok := err.(*net.OpError) ; ok {
				if errno.Timeout() {
					continue
				}
			}
			log.Printf("read: %s", err.String())
			break
		}
		nw, err := w.Write(p[:nr])
		if nw < nr {
			log.Println("read: %s", err.String())
			break
		}
	}
}

func (cl *Client) writeTransport(r io.Reader) {
	defer tryClose(r, cl.socket)
	p := make([]byte, 1024)
	for {
		nr, err := r.Read(p)
		if nr == 0 {
			log.Printf("write: %s", err.String())
			break
		}
		nw, err := cl.socket.Write(p[:nr])
		if nw < nr {
			log.Println("write: %s", err.String())
			break
		}
	}
}

func readXml(r io.Reader, ch chan<- interface{}) {
	if debug {
		pr, pw := io.Pipe()
		go tee(r, pw, "S: ")
		r = pr
	}
	defer tryClose(r, ch)

	p := xml.NewParser(r)
	for {
		// Sniff the next token on the stream.
		t, err := p.Token()
		if t == nil {
			if err != os.EOF {
				log.Printf("read: %v", err)
			}
			break
		}
		var se xml.StartElement
		var ok bool
		if se, ok = t.(xml.StartElement) ; !ok {
			continue
		}

		// Allocate the appropriate structure for this token.
		var obj interface{}
		switch se.Name.Space + " " + se.Name.Local {
		case nsStream + " stream":
			st, err := parseStream(se)
			if err != nil {
				log.Printf("unmarshal stream: %v",
					err)
				break
			}
			ch <- st
			continue
		case "stream error", nsStream + " error":
			obj = &StreamError{}
		case nsStream + " features":
			obj = &Features{}
		case nsTLS + " proceed", nsTLS + " failure":
			obj = &starttls{}
		case nsSASL + " challenge", nsSASL + " failure",
			nsSASL + " success":
			obj = &auth{}
		case "jabber:client iq":
			obj = &Iq{}
		case "jabber:client message":
			obj = &Message{}
		case "jabber:client presence":
			obj = &Presence{}
		default:
			obj = &Generic{}
			log.Printf("Ignoring unrecognized: %s %s\n",
				se.Name.Space, se.Name.Local)
		}

		// Read the complete XML stanza.
		err = p.Unmarshal(obj, &se)
		if err != nil {
			log.Printf("unmarshal: %v", err)
			break
		}

		// BUG(cjyar) If it's a Stanza, use reflection to
		// search for any Generic elements and fill in
		// their attributes.

		// Put it on the channel.
		ch <- obj
	}
}

func writeXml(w io.Writer, ch <-chan interface{}) {
	if debug {
		pr, pw := io.Pipe()
		go tee(pr, w, "C: ")
		w = pw
	}
	defer tryClose(w, ch)

	for obj := range ch {
		err := xml.Marshal(w, obj)
		if err != nil {
			log.Printf("write: %v", err)
			break
		}
	}
}

// BUG(cjyar) This should go away. We shouldn't allow writing of
// unstructured data.
func writeText(w io.Writer, ch <-chan *string) {
	if debug {
		pr, pw := io.Pipe()
		go tee(pr, w, "C: ")
		w = pw
	}
	defer tryClose(w, ch)

	for str := range ch {
		_, err := w.Write([]byte(*str))
		if err != nil {
			log.Printf("writeStr: %v", err)
			break
		}
	}
}

func (cl *Client) readStream(srvIn <-chan interface{}, cliOut chan<- Stanza) {
	defer tryClose(srvIn, cliOut)

	handlers := make(map[string] func(Stanza) bool)
	// BUG(cjyar) This for loop will never terminate, even when
	// the channels are closed.
	for {
		select {
		case h := <- cl.handlers:
			handlers[h.id] = h.f
		case x := <- srvIn:
			send := false
			switch obj := x.(type) {
			case *stream:
				handleStream(obj)
			case *Features:
				cl.handleFeatures(obj)
			case *starttls:
				cl.handleTls(obj)
			case *auth:
				cl.handleSasl(obj)
			default:
				send = true
			}
			st, ok := x.(Stanza)
			if !ok {
				log.Printf("Unhandled non-stanza: %v",
					x)
				continue
			}
			if handlers[st.XId()] != nil {
				f := handlers[st.XId()]
				handlers[st.XId()] = nil
				send = f(st)
			}
			if send {
				cliOut <- st
			}
		}
	}
}

// BUG(cjyar) Disable this loop until resource binding is
// complete. Otherwise the app might inject something weird into our
// negotiation stream.
func writeStream(srvOut chan<- interface{}, cliIn <-chan Stanza) {
	defer tryClose(srvOut, cliIn)

	for x := range cliIn {
		srvOut <- x
	}
}

func handleStream(ss *stream) {
}

func (cl *Client) handleFeatures(fe *Features) {
	if fe.Starttls != nil {
		start := &starttls{XMLName: xml.Name{Space: nsTLS,
			Local: "starttls"}}
		cl.xmlOut <- start
		return
	}

	if len(fe.Mechanisms.Mechanism) > 0 {
		cl.chooseSasl(fe)
		return
	}

	if fe.Bind != nil {
		cl.bind(fe.Bind)
		return
	}
}

// readTransport() is running concurrently. We need to stop it,
// negotiate TLS, then start it again. It calls waitForSocket() in
// its inner loop; see below.
func (cl *Client) handleTls(t *starttls) {
	tcp := cl.socket

	// Set the socket to nil, and wait for the reader routine to
	// signal that it's paused.
	cl.socket = nil
	cl.socketSync.Add(1)
	cl.socketSync.Wait()

	// Negotiate TLS with the server.
	tls := tls.Client(tcp, nil)

	// Make the TLS connection available to the reader, and wait
	// for it to signal that it's working again.
	cl.socketSync.Add(1)
	cl.socket = tls
	cl.socketSync.Wait()

	// Reset the read timeout on the (underlying) socket so the
	// reader doesn't get woken up unnecessarily.
	tcp.SetReadTimeout(0)

	log.Println("TLS negotiation succeeded.")

	// Now re-send the initial handshake message to start the new
	// session.
	hsOut := &stream{To: cl.Jid.Domain, Version: Version}
	cl.xmlOut <- hsOut
}

// Synchronize with handleTls(). Called from readTransport() when
// cl.socket is nil.
func (cl *Client) waitForSocket() {
	// Signal that we've stopped reading from the socket.
	cl.socketSync.Done()

	// Wait until the socket is available again.
	for cl.socket == nil {
		time.Sleep(1e8)
	}

	// Signal that we're going back to the read loop.
	cl.socketSync.Done()
}

// BUG(cjyar) Doesn't implement TLS/SASL EXTERNAL.
func (cl *Client) chooseSasl(fe *Features) {
	var digestMd5 bool
	for _, m := range(fe.Mechanisms.Mechanism) {
		switch strings.ToLower(m) {
		case "digest-md5":
			digestMd5 = true
		}
	}

	if digestMd5 {
		auth := &auth{XMLName: xml.Name{Space: nsSASL, Local:
				"auth"}, Mechanism: "DIGEST-MD5"}
		cl.xmlOut <- auth
	}
}

func (cl *Client) handleSasl(srv *auth) {
	switch strings.ToLower(srv.XMLName.Local) {
	case "challenge":
		b64 := base64.StdEncoding
		str, err := b64.DecodeString(srv.Chardata)
		if err != nil {
			log.Printf("SASL challenge decode: %s",
				err.String())
			return;
		}
		srvMap := parseSasl(string(str))

		if cl.saslExpected == "" {
			cl.saslDigest1(srvMap)
		} else {
			cl.saslDigest2(srvMap)
		}
	case "failure":
		log.Println("SASL authentication failed")
	case "success":
		log.Println("SASL authentication succeeded")
		ss := &stream{To: cl.Jid.Domain, Version: Version}
		cl.xmlOut <- ss
	}
}

func (cl *Client) saslDigest1(srvMap map[string] string) {
	// Make sure it supports qop=auth
	var hasAuth bool
	for _, qop := range(strings.Fields(srvMap["qop"])) {
		if qop == "auth" {
			hasAuth = true
		}
	}
	if !hasAuth {
		log.Println("Server doesn't support SASL auth")
		return;
	}

	// Pick a realm.
	var realm string
	if srvMap["realm"] != "" {
		realm = strings.Fields(srvMap["realm"])[0]
	}

	passwd := cl.password
	nonce := srvMap["nonce"]
	digestUri := "xmpp/" + cl.Jid.Domain
	nonceCount := int32(1)
	nonceCountStr := fmt.Sprintf("%08x", nonceCount)

	// Begin building the response. Username is
	// user@domain or just domain.
	var username string
	if cl.Jid.Node == "" {
		username = cl.Jid.Domain
	} else {
		username = cl.Jid.Node
	}

	// Generate our own nonce from random data.
	randSize := big.NewInt(0)
	randSize.Lsh(big.NewInt(1), 64)
	cnonce, err := rand.Int(rand.Reader, randSize)
	if err != nil {
		log.Println("SASL rand: %s", err.String())
		return
	}
	cnonceStr := fmt.Sprintf("%016x", cnonce)

	/* Now encode the actual password response, as well as the
	 * expected next challenge from the server. */
	response := saslDigestResponse(username, realm, passwd, nonce,
		cnonceStr, "AUTHENTICATE", digestUri, nonceCountStr)
	next := saslDigestResponse(username, realm, passwd, nonce,
		cnonceStr, "", digestUri, nonceCountStr)
	cl.saslExpected = next

	// Build the map which will be encoded.
	clMap := make(map[string]string)
	clMap["realm"] = `"` + realm + `"`
	clMap["username"] = `"` + username + `"`
	clMap["nonce"] = `"` + nonce + `"`
	clMap["cnonce"] = `"` + cnonceStr + `"`
	clMap["nc"] =  nonceCountStr
	clMap["qop"] = "auth"
	clMap["digest-uri"] = `"` + digestUri + `"`
	clMap["response"] = response
	if srvMap["charset"] == "utf-8" {
		clMap["charset"] = "utf-8"
	}

	// Encode the map and send it.
	clStr := packSasl(clMap)
	b64 := base64.StdEncoding
	clObj := &auth{XMLName: xml.Name{Space: nsSASL, Local:
			"response"}, Chardata:
		b64.EncodeToString([]byte(clStr))}
	cl.xmlOut <- clObj
}

func (cl *Client) saslDigest2(srvMap map[string] string) {
	if cl.saslExpected == srvMap["rspauth"] {
		clObj := &auth{XMLName: xml.Name{Space: nsSASL, Local:
				"response"}}
		cl.xmlOut <- clObj
	} else {
		clObj := &auth{XMLName: xml.Name{Space: nsSASL, Local:
				"failure"}, Any:
			&Generic{XMLName: xml.Name{Space: nsSASL,
				Local: "abort"}}}
		cl.xmlOut <- clObj
	}
}

// Takes a string like `key1=value1,key2="value2"...` and returns a
// key/value map.
func parseSasl(in string) map[string]string {
	re := regexp.MustCompile(`([^=]+)="?([^",]+)"?,?`)
	strs := re.FindAllStringSubmatch(in, -1)
	m := make(map[string]string)
	for _, pair := range(strs) {
		key := strings.ToLower(string(pair[1]))
		value := string(pair[2])
		m[key] = value
	}
	return m
}

// Inverse of parseSasl().
func packSasl(m map[string]string) string {
	var terms []string
	for key, value := range(m) {
		if key == "" || value == "" || value == `""` {
			continue
		}
		terms = append(terms, key + "=" + value)
	}
	return strings.Join(terms, ",")
}

// Computes the response string for digest authentication.
func saslDigestResponse(username, realm, passwd, nonce, cnonceStr,
	authenticate, digestUri, nonceCountStr string) string {
	h := func(text string) []byte {
		h := md5.New()
		h.Write([]byte(text))
		return h.Sum()
	}
	hex := func(bytes []byte) string {
		return fmt.Sprintf("%x", bytes)
	}
	kd := func(secret, data string) []byte {
		return h(secret + ":" + data)
	}

	a1 := string(h(username + ":" + realm + ":" + passwd)) + ":" +
		nonce + ":" + cnonceStr
	a2 := authenticate + ":" + digestUri
	response := hex(kd(hex(h(a1)), nonce + ":" +
		nonceCountStr + ":" + cnonceStr + ":auth:" +
		hex(h(a2))))
	return response
}

// Send a request to bind a resource. RFC 3920, section 7.
func (cl *Client) bind(bind *Generic) {
	res := cl.Jid.Resource
	msg := &Iq{Type: "set", Id: cl.NextId(), Any:
		&Generic{XMLName: xml.Name{Space: nsBind, Local:
					"bind"}}}
	if res != "" {
		msg.Any.Any = &Generic{XMLName: xml.Name{Local:
				"resource"}, Chardata: res}
	}
	f := func(st Stanza) bool {
		if st.XType() == "error" {
			log.Println("Resource binding failed")
			return false
		}
		bind := st.XChild()
		if bind == nil {
			log.Println("nil resource bind")
			return false
		}
		jidEle := bind.Any
		if jidEle == nil {
			log.Println("nil resource")
			return false
		}
		jidStr := jidEle.Chardata
		if jidStr == "" {
			log.Println("empty resource")
			return false
		}
		jid := new(JID)
		if !jid.Set(jidStr) {
			log.Println("Can't parse JID %s", jidStr)
			return false
		}
		cl.Jid = *jid
		log.Printf("Bound resource: %s", cl.Jid.String())
		return true
	}
	cl.HandleStanza(msg.Id, f)
	cl.xmlOut <- msg
}

// Register a callback to handle the next XMPP stanza (iq, message, or
// presence) with a given id. The provided function will not be called
// more than once. If it returns false, the stanza will not be made
// available on the normal Client.In channel.
func (cl *Client) HandleStanza(id string, f func(Stanza) bool) {
	h := &stanzaHandler{id: id, f: f}
	cl.handlers <- h
}
