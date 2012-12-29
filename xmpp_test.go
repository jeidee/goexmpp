// Copyright 2011 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package xmpp

import (
	"bytes"
	"encoding/xml"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func TestReadError(t *testing.T) {
	r := strings.NewReader(`<stream:error><bad-foo xmlns="blah"/>` +
		`</stream:error>`)
	ch := make(chan interface{})
	go readXml(r, ch, make(map[string]func(*xml.Name) interface{}))
	x := <-ch
	se, ok := x.(*streamError)
	if !ok {
		t.Fatalf("not StreamError: %T", x)
	}
	assertEquals(t, "bad-foo", se.Any.XMLName.Local)
	assertEquals(t, "blah", se.Any.XMLName.Space)
	if se.Text != nil {
		t.Errorf("text not nil: %v", se.Text)
	}

	r = strings.NewReader(`<stream:error><bad-foo xmlns="blah"/>` +
		`<text xml:lang="en" xmlns="` + NsStreams +
		`">Error text</text></stream:error>`)
	ch = make(chan interface{})
	go readXml(r, ch, make(map[string]func(*xml.Name) interface{}))
	x = <-ch
	se, ok = x.(*streamError)
	if !ok {
		t.Fatalf("not StreamError: %v", reflect.TypeOf(x))
	}
	assertEquals(t, "bad-foo", se.Any.XMLName.Local)
	assertEquals(t, "blah", se.Any.XMLName.Space)
	assertEquals(t, "Error text", se.Text.Text)
	assertEquals(t, "en", se.Text.Lang)
}

func TestReadStream(t *testing.T) {
	r := strings.NewReader(`<stream:stream to="foo.com" ` +
		`from="bar.org" id="42"` +
		`xmlns="` + NsClient + `" xmlns:stream="` + NsStream +
		`" version="1.0">`)
	ch := make(chan interface{})
	go readXml(r, ch, make(map[string]func(*xml.Name) interface{}))
	x := <-ch
	ss, ok := x.(*stream)
	if !ok {
		t.Fatalf("not stream: %v", reflect.TypeOf(x))
	}
	assertEquals(t, "foo.com", ss.To)
	assertEquals(t, "bar.org", ss.From)
	assertEquals(t, "42", ss.Id)
	assertEquals(t, "1.0", ss.Version)
}

func testWrite(obj interface{}) string {
	w := bytes.NewBuffer(nil)
	ch := make(chan interface{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		writeXml(w, ch)
	}()
	ch <- obj
	close(ch)
	wg.Wait()
	return w.String()
}

func TestWriteError(t *testing.T) {
	se := &streamError{Any: Generic{XMLName: xml.Name{Local: "blah"}}}
	str := testWrite(se)
	exp := `<error xmlns="` + NsStream + `"><blah></blah></error>`
	assertEquals(t, exp, str)

	se = &streamError{Any: Generic{XMLName: xml.Name{Space: NsStreams, Local: "foo"}}, Text: &errText{Lang: "ru", Text: "Пошёл ты"}}
	str = testWrite(se)
	exp = `<error xmlns="` + NsStream + `"><foo xmlns="` + NsStreams +
		`"></foo><text xmlns="` + NsStreams +
		`" xml:lang="ru">Пошёл ты</text></error>`
	assertEquals(t, exp, str)
}

func TestWriteStream(t *testing.T) {
	ss := &stream{To: "foo.org", From: "bar.com", Id: "42", Lang: "en", Version: "1.0"}
	str := testWrite(ss)
	exp := `<stream:stream xmlns="` + NsClient +
		`" xmlns:stream="` + NsStream + `" to="foo.org"` +
		` from="bar.com" id="42" xml:lang="en" version="1.0">`
	assertEquals(t, exp, str)
}
