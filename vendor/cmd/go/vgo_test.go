// Copyright 2018 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package Main_test

import (
	"runtime"
	"testing"
)

func TestVGOROOT(t *testing.T) {
	tg := testgo(t)
	defer tg.cleanup()

	tg.setenv("GOROOT", "/bad")
	tg.runFail("env")

	tg.setenv("VGOROOT", runtime.GOROOT())
	tg.run("env")
}
