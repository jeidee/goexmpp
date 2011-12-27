// Copyright 2011 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// This file contains the three layers of processing for the
// communication with the server: transport (where TLS happens), XML
// (where strings are converted to go structures), and Stream (where
// we respond to XMPP events on behalf of the library client).

package xmpp

import (
	"crypto/tls"
	"io"
	"log"
	"net"
	"os"
	"time"
	"xml"
)

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
		default:
			obj = &Unrecognized{}
			log.Printf("Ignoring unrecognized: %s %s\n",
				se.Name.Space, se.Name.Local)
		}

		// Read the complete XML stanza.
		err = p.Unmarshal(obj, &se)
		if err != nil {
			log.Printf("unmarshal: %v", err)
			break
		}

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

func (cl *Client) readStream(srvIn <-chan interface{}, srvOut, cliOut chan<- interface{}) {
	defer tryClose(srvIn, cliOut)

	for x := range srvIn {
		switch obj := x.(type) {
		case *Stream:
			handleStream(obj)
		case *Features:
			handleFeatures(obj, srvOut)
		case *starttls:
			cl.handleTls(obj)
		default:
			cliOut <- x
		}
	}
}

func writeStream(srvOut chan<- interface{}, cliIn <-chan interface{}) {
	defer tryClose(srvOut, cliIn)

	for x := range cliIn {
		srvOut <- x
	}
}

func handleStream(ss *Stream) {
}

func handleFeatures(fe *Features, srvOut chan<- interface{}) {
	if fe.Starttls != nil {
		start := &starttls{XMLName: xml.Name{Space: nsTLS,
			Local: "starttls"}}
		srvOut <- start
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
	hsOut := &Stream{To: cl.Jid.Domain, Version: Version}
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
