// Copyright 2018 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package Main_test

import (
	"cmd/go/internal/modconv"
	"cmd/go/internal/vgo"
	"io/ioutil"
	"os"
	"runtime"
	"sort"
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

func TestFindModRoot(t *testing.T) {
	tg := testgo(t)
	defer tg.cleanup()
	tg.makeTempdir()

	tg.must(os.MkdirAll(tg.path("x/Godeps"), 0777))
	tg.must(os.MkdirAll(tg.path("x/vendor"), 0777))
	tg.must(os.MkdirAll(tg.path("x/y/z"), 0777))
	tg.must(os.MkdirAll(tg.path("x/.git"), 0777))
	var files []string
	for file := range modconv.Converters {
		files = append(files, file)
	}
	files = append(files, "go.mod")
	files = append(files, ".git/config")
	sort.Strings(files)

	for file := range modconv.Converters {
		tg.must(ioutil.WriteFile(tg.path("x/"+file), []byte{}, 0666))
		root, file1 := vgo.FindModuleRoot(tg.path("x/y/z"), tg.path("."), true)
		if root != tg.path("x") || file1 != file {
			t.Errorf("%s: findModuleRoot = %q, %q, want %q, %q", file, root, file1, tg.path("x"), file)
		}
		tg.must(os.Remove(tg.path("x/" + file)))
	}
}
