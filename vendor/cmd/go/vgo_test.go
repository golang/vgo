// Copyright 2018 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package Main_test

import (
	"cmd/go/internal/modconv"
	"cmd/go/internal/vgo"
	"internal/testenv"
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

func TestFindModuleRoot(t *testing.T) {
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

func TestFindModulePath(t *testing.T) {
	tg := testgo(t)
	defer tg.cleanup()
	tg.makeTempdir()

	tg.must(os.MkdirAll(tg.path("x"), 0777))
	tg.must(ioutil.WriteFile(tg.path("x/x.go"), []byte("package x // import \"x\"\n"), 0666))
	path, err := vgo.FindModulePath(tg.path("x"))
	if err != nil {
		t.Fatal(err)
	}
	if path != "x" {
		t.Fatalf("FindModulePath = %q, want %q", path, "x")
	}

	// Windows line-ending.
	tg.must(ioutil.WriteFile(tg.path("x/x.go"), []byte("package x // import \"x\"\r\n"), 0666))
	path, err = vgo.FindModulePath(tg.path("x"))
	if err != nil {
		t.Fatal(err)
	}
	if path != "x" {
		t.Fatalf("FindModulePath = %q, want %q", path, "x")
	}
}

func TestLocalModule(t *testing.T) {
	// Test that local replacements work
	// and that they can use a dummy name
	// that isn't resolvable and need not even
	// include a dot. See golang.org/issue/24100.
	tg := testgo(t)
	defer tg.cleanup()
	tg.makeTempdir()

	tg.must(os.MkdirAll(tg.path("x/y"), 0777))
	tg.must(os.MkdirAll(tg.path("x/z"), 0777))
	tg.must(ioutil.WriteFile(tg.path("x/y/go.mod"), []byte(`
		module x/y
		require zz v1.0.0
		replace zz v1.0.0 => ../z
	`), 0666))
	tg.must(ioutil.WriteFile(tg.path("x/y/y.go"), []byte(`package y; import _ "zz"`), 0666))
	tg.must(ioutil.WriteFile(tg.path("x/z/go.mod"), []byte(`
		module x/z
	`), 0666))
	tg.must(ioutil.WriteFile(tg.path("x/z/z.go"), []byte(`package z`), 0666))
	tg.cd(tg.path("x/y"))
	tg.run("-vgo", "build")
}

func TestTags(t *testing.T) {
	// Test that build tags are used. See golang.org/issue/24053.
	tg := testgo(t)
	defer tg.cleanup()
	tg.makeTempdir()

	tg.must(os.MkdirAll(tg.path("x"), 0777))
	tg.must(ioutil.WriteFile(tg.path("x/go.mod"), []byte(`
		module x
	`), 0666))
	tg.must(ioutil.WriteFile(tg.path("x/x.go"), []byte(`// +build tag1

		package y
	`), 0666))
	tg.must(ioutil.WriteFile(tg.path("x/y.go"), []byte(`// +build tag2

		package y
	`), 0666))
	tg.cd(tg.path("x"))

	tg.runFail("-vgo", "list", "-f={{.GoFiles}}")
	tg.grepStderr("no Go source files", "no Go source files without tags")

	tg.run("-vgo", "list", "-f={{.GoFiles}}", "-tags=tag1")
	tg.grepStdout(`\[x.go\]`, "Go source files for tag1")

	tg.run("-vgo", "list", "-f={{.GoFiles}}", "-tags", "tag2")
	tg.grepStdout(`\[y.go\]`, "Go source files for tag2")

	tg.run("-vgo", "list", "-f={{.GoFiles}}", "-tags", "tag1 tag2")
	tg.grepStdout(`\[x.go y.go\]`, "Go source files for tag1 and tag2")
}

func TestFillGoMod(t *testing.T) {
	testenv.MustHaveExternalNetwork(t)
	tg := testgo(t)
	defer tg.cleanup()
	tg.makeTempdir()

	tg.setenv("HOME", tg.path("."))
	tg.must(os.MkdirAll(tg.path("x"), 0777))
	tg.must(ioutil.WriteFile(tg.path("x/x.go"), []byte(`
		package x
	`), 0666))

	tg.must(ioutil.WriteFile(tg.path("x/go.mod"), []byte(`
		module x
	`), 0666))
	tg.must(ioutil.WriteFile(tg.path("x/Gopkg.lock"), []byte(`
[[projects]]
  name = "rsc.io/sampler"
  version = "v1.0.0"
	`), 0666))

	tg.cd(tg.path("x"))
	tg.run("-vgo", "build", "-v")
	tg.grepStderr("copying requirements from .*Gopkg.lock", "did not copy Gopkg.lock")
	tg.run("-vgo", "list", "-m")
	tg.grepStderrNot("copying requirements from .*Gopkg.lock", "should not copy Gopkg.lock again")
	tg.grepStdout("rsc.io/sampler.*v1.0.0", "did not copy Gopkg.lock")

	tg.must(ioutil.WriteFile(tg.path("x/go.mod"), []byte(`
		module x
	`), 0666))
	tg.must(ioutil.WriteFile(tg.path("x/Gopkg.lock"), []byte(`
	`), 0666))

	tg.run("-vgo", "list")
	tg.grepStderr("copying requirements from .*Gopkg.lock", "did not copy Gopkg.lock")
	tg.run("-vgo", "list")
	tg.grepStderrNot("copying requirements from .*Gopkg.lock", "should not copy Gopkg.lock again")

}
