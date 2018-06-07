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
	"path/filepath"
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

func TestFSPatterns(t *testing.T) {
	tg := testgo(t)
	defer tg.cleanup()
	tg.makeTempdir()

	tg.must(os.MkdirAll(tg.path("x/vendor/v"), 0777))
	tg.must(os.MkdirAll(tg.path("x/y/z/w"), 0777))
	tg.must(ioutil.WriteFile(tg.path("x/go.mod"), []byte(`
		module m
	`), 0666))

	tg.must(ioutil.WriteFile(tg.path("x/x.go"), []byte(`package x`), 0666))
	tg.must(ioutil.WriteFile(tg.path("x/vendor/v/v.go"), []byte(`package v; import "golang.org/x/crypto"`), 0666))
	tg.must(ioutil.WriteFile(tg.path("x/vendor/v.go"), []byte(`package main`), 0666))
	tg.must(ioutil.WriteFile(tg.path("x/y/y.go"), []byte(`package y`), 0666))
	tg.must(ioutil.WriteFile(tg.path("x/y/z/go.mod"), []byte(`syntax error`), 0666))
	tg.must(ioutil.WriteFile(tg.path("x/y/z/z.go"), []byte(`package z`), 0666))
	tg.must(ioutil.WriteFile(tg.path("x/y/z/w/w.go"), []byte(`package w`), 0666))

	tg.cd(tg.path("x"))
	tg.run("-vgo", "list", "all")
	tg.grepStdout(`^m$`, "expected m")
	tg.grepStdout(`^m/vendor$`, "must see package named vendor")
	tg.grepStdoutNot(`vendor/`, "must not see vendored packages")
	tg.grepStdout(`^m/y$`, "expected m/y")
	tg.grepStdoutNot(`^m/y/z`, "should ignore submodule m/y/z...")
}

func TestGetModuleVersion(t *testing.T) {
	testenv.MustHaveExternalNetwork(t)

	tg := testgo(t)
	defer tg.cleanup()
	tg.makeTempdir()

	tg.setenv(homeEnvName(), tg.path("home"))
	tg.must(os.MkdirAll(tg.path("x"), 0777))
	tg.cd(tg.path("x"))
	tg.must(ioutil.WriteFile(tg.path("x/x.go"), []byte(`package x`), 0666))

	tg.must(ioutil.WriteFile(tg.path("x/go.mod"), []byte(`
		module x
		require github.com/gobuffalo/uuid v1.1.0
	`), 0666))
	tg.run("-vgo", "get", "github.com/gobuffalo/uuid@v2.0.0")
	tg.run("-vgo", "list", "-m")
	tg.grepStdout("github.com/gobuffalo/uuid.*v0.0.0-20180207211247-3a9fb6c5c481", "did downgrade to v0.0.0-*")

	tooSlow(t)

	tg.must(ioutil.WriteFile(tg.path("x/go.mod"), []byte(`
		module x
		require github.com/gobuffalo/uuid v1.2.0
	`), 0666))
	tg.run("-vgo", "get", "github.com/gobuffalo/uuid@v1.1.0")
	tg.run("-vgo", "list", "-m")
	tg.grepStdout("github.com/gobuffalo/uuid.*v1.1.0", "did downgrade to v1.1.0")

	tg.must(ioutil.WriteFile(tg.path("x/go.mod"), []byte(`
		module x
		require github.com/gobuffalo/uuid v1.1.0
	`), 0666))
	tg.run("-vgo", "get", "github.com/gobuffalo/uuid@v1.2.0")
	tg.run("-vgo", "list", "-m")
	tg.grepStdout("github.com/gobuffalo/uuid.*v1.2.0", "did upgrade to v1.2.0")
}

func TestVgoBadDomain(t *testing.T) {
	tg := testgo(t)
	defer tg.cleanup()
	wd, _ := os.Getwd()
	tg.cd(filepath.Join(wd, "testdata/badmod"))

	tg.runFail("-vgo", "get", "appengine")
	tg.grepStderr("unknown module appengine: not a domain name", "expected domain error")
	tg.runFail("-vgo", "get", "x/y.z")
	tg.grepStderr("unknown module x/y.z: not a domain name", "expected domain error")

	tg.runFail("-vgo", "build")
	tg.grepStderr("unknown module appengine: not a domain name", "expected domain error")
	tg.grepStderr("tcp.*nonexistent.rsc.io", "expected error for nonexistent.rsc.io")
}

func TestVgoVendor(t *testing.T) {
	tg := testgo(t)
	defer tg.cleanup()

	wd, _ := os.Getwd()
	tg.cd(filepath.Join(wd, "testdata/vendormod"))
	tg.run("-vgo", "list", "-m")
	tg.grepStdout(`^x`, "expected to see module x")
	tg.grepStdout(`=> ./x`, "expected to see replacement for module x")
	tg.grepStdout(`^w`, "expected to see module w")

	tg.run("-vgo", "vendor", "-v")
	tg.grepStderr(`^# x v1.0.0 => ./x`, "expected to see module x with replacement")
	tg.grepStderr(`^x`, "expected to see package x")
	tg.grepStderr(`^# y v1.0.0 => ./y`, "expected to see module y with replacement")
	tg.grepStderr(`^y`, "expected to see package y")
	tg.grepStderr(`^# z v1.0.0 => ./z`, "expected to see module z with replacement")
	tg.grepStderr(`^z`, "expected to see package z")
	tg.grepStderrNot(`w`, "expected NOT to see unused module w")

	tg.must(os.RemoveAll(filepath.Join(wd, "testdata/vendormod/vendor")))
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

func TestQueryExcluded(t *testing.T) {
	tg := testgo(t)
	defer tg.cleanup()
	tg.makeTempdir()

	tg.must(os.MkdirAll(tg.path("x"), 0777))
	tg.must(ioutil.WriteFile(tg.path("x/x.go"), []byte(`package x; import _ "github.com/gorilla/mux"`), 0666))
	gomod := []byte(`
		module x

		exclude github.com/gorilla/mux v1.6.0
	`)

	tg.setenv(homeEnvName(), tg.path("home"))
	tg.cd(tg.path("x"))

	tg.must(ioutil.WriteFile(tg.path("x/go.mod"), gomod, 0666))
	tg.runFail("-vgo", "get", "github.com/gorilla/mux@v1.6.0")
	tg.grepStderr("github.com/gorilla/mux@v1.6.0 excluded", "print version excluded")

	tg.must(ioutil.WriteFile(tg.path("x/go.mod"), gomod, 0666))
	tg.run("-vgo", "get", "github.com/gorilla/mux@v1.6.1")
	tg.grepStderr("finding github.com/gorilla/mux v1.6.1", "find version 1.6.1")

	tg.must(ioutil.WriteFile(tg.path("x/go.mod"), gomod, 0666))
	tg.runFail("-vgo", "get", "github.com/gorilla/mux@v1.6")
	tg.grepStderr("github.com/gorilla/mux@v1.6.0 excluded", "print version excluded")
}

func TestConvertLegacyConfig(t *testing.T) {
	testenv.MustHaveExternalNetwork(t)
	tg := testgo(t)
	defer tg.cleanup()
	tg.makeTempdir()

	// Testing that on Windows the path x/Gopkg.lock turning into x\Gopkg.lock does not confuse converter.
	tg.must(os.MkdirAll(tg.path("x"), 0777))
	tg.must(ioutil.WriteFile(tg.path("x/Gopkg.lock"), []byte(`
	  [[projects]]
		name = "github.com/pkg/errors"
		packages = ["."]
		revision = "645ef00459ed84a119197bfb8d8205042c6df63d"
		version = "v0.6.0"`), 0666))
	tg.must(ioutil.WriteFile(tg.path("x/main.go"), []byte("package x // import \"x\"\n import _ \"github.com/pkg/errors\""), 0666))
	tg.cd(tg.path("x"))
	tg.run("-vgo", "list", "-m")

	// If the conversion just ignored the Gopkg.lock entirely
	// it would choose a newer version (like v0.8.0 or maybe
	// something even newer). Check for the older version to
	// make sure Gopkg.lock was properly used.
	tg.grepStderr("v0.6.0", "expected github.com/pkg/errors at v0.6.0")
}
