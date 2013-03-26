// Copyright 2011 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Control over logging from the XMPP library.

package xmpp

var (
	// If any of these are non-nil when NewClient() is called,
	// they will be used to log messages of the indicated
	// severity.
	Warn  Logger = &noLog{}
	Info  Logger = &noLog{}
	Debug Logger = &noLog{}
)

// Anything implementing Logger can receive log messages from the XMPP
// library. The default implementation doesn't log anything; it
// efficiently discards all messages.
type Logger interface {
	Log(v ...interface{})
	Logf(fmt string, v ...interface{})
}

type noLog struct {
	flags  int
	prefix string
}

var _ Logger = &noLog{}

func (l *noLog) Log(v ...interface{}) {
}

func (l *noLog) Logf(fmt string, v ...interface{}) {
}
