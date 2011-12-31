# Copyright 2009 The Go Authors.  All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

include $(GOROOT)/src/Make.inc

TARG=cjyar/xmpp
GOFILES=\
	xmpp.go \
	roster.go \
	stream.go \
	structs.go \

examples: install
	gomake -C examples all

clean: clean-examples

clean-examples:
	gomake -C examples clean

include $(GOROOT)/src/Make.pkg
