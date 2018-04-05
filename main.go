// Copyright 2018 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Vgo is a prototype of what the go command
// might look like with integrated support for package versioning.
//
// Download and install with:
//
//	go get -u golang.org/x/vgo
//
// Then run "vgo" instead of "go".
//
// See https://research.swtch.com/vgo-intro for an overview
// and the documents linked at https://research.swtch.com/vgo
// for additional details.
//
// This is still a very early prototype.
// You are likely to run into bugs.
// Please file bugs in the main Go issue tracker,
// https://golang.org/issue,
// and put the prefix `x/vgo:` in the issue title.
//
// Thank you.
//
package main

import Main "cmd/go"

func main() {
	Main.Main()
}
