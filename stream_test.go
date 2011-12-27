// Copyright 2011 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package xmpp

import (
	"testing"
)

func TestSaslDigest(t *testing.T) {
	// These values are from RFC2831, section 4.
	obs := saslDigestResponse("chris", "elwood.innosoft.com",
		"secret", "OA6MG9tEQGm2hh", "OA6MHXh6VqTrRk",
		"AUTHENTICATE", "imap/elwood.innosoft.com",
		"00000001")
	exp := "d388dad90d4bbd760a152321f2143af7"
	assertEquals(t, exp, obs)
}
