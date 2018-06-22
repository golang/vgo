// Copyright 2018 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package Main_test

import (
	"bytes"
	"internal/testenv"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	"cmd/go/internal/modconv"
	"cmd/go/internal/vgo"
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

func TestModEdit(t *testing.T) {
	// Test that local replacements work
	// and that they can use a dummy name
	// that isn't resolvable and need not even
	// include a dot. See golang.org/issue/24100.
	tg := testgo(t)
	defer tg.cleanup()
	tg.makeTempdir()
	tg.cd(tg.path("."))
	tg.must(os.MkdirAll(tg.path("w"), 0777))
	tg.must(ioutil.WriteFile(tg.path("x.go"), []byte("package x\n"), 0666))
	tg.must(ioutil.WriteFile(tg.path("w/w.go"), []byte("package w\n"), 0666))

	mustHaveGoMod := func(text string) {
		data, err := ioutil.ReadFile(tg.path("go.mod"))
		tg.must(err)
		if string(data) != text {
			t.Fatalf("go.mod mismatch:\nhave:<<<\n%s>>>\nwant:<<<\n%s\n", string(data), text)
		}
	}

	tg.runFail("-vgo", "mod", "-init")
	tg.grepStderr(`cannot determine module path`, "")
	_, err := os.Stat(tg.path("go.mod"))
	if err == nil {
		t.Fatalf("failed go mod -init created go.mod")
	}

	tg.run("-vgo", "mod", "-init", "-module", "x.x/y/z")
	tg.grepStderr("creating new go.mod: module x.x/y/z", "")
	mustHaveGoMod(`module x.x/y/z
`)

	tg.runFail("-vgo", "mod", "-init")
	mustHaveGoMod(`module x.x/y/z
`)

	tg.run("-vgo", "mod",
		"-droprequire=x.1",
		"-require=x.1@v1.0.0",
		"-require=x.2@v1.1.0",
		"-droprequire=x.2",
		"-exclude=x.1 @ v1.2.0",
		"-exclude=x.1@v1.2.1",
		"-replace=x.1@v1.3.0=>y.1@v1.4.0",
		"-replace=x.1@v1.4.0 => ../z",
	)
	mustHaveGoMod(`module x.x/y/z

require x.1 v1.0.0

exclude (
	x.1 v1.2.0
	x.1 v1.2.1
)

replace (
	x.1 v1.3.0 => y.1 v1.4.0
	x.1 v1.4.0 => ../z
)
`)

	tg.run("-vgo", "mod",
		"-droprequire=x.1",
		"-dropexclude=x.1@v1.2.1",
		"-dropreplace=x.1@v1.3.0",
		"-require=x.3@v1.99.0",
	)
	mustHaveGoMod(`module x.x/y/z

exclude x.1 v1.2.0

replace x.1 v1.4.0 => ../z

require x.3 v1.99.0
`)

	tg.run("-vgo", "mod", "-json")
	want := `{
	"Module": {
		"Path": "x.x/y/z",
		"Version": ""
	},
	"Require": [
		{
			"Path": "x.3",
			"Version": "v1.99.0"
		}
	],
	"Exclude": [
		{
			"Path": "x.1",
			"Version": "v1.2.0"
		}
	],
	"Replace": [
		{
			"Old": {
				"Path": "x.1",
				"Version": "v1.4.0"
			},
			"New": {
				"Path": "../z",
				"Version": ""
			}
		}
	]
}
`
	if have := tg.getStdout(); have != want {
		t.Fatalf("go mod -json mismatch:\nhave:<<<\n%s>>>\nwant:<<<\n%s\n", have, want)
	}

	tg.run("-vgo", "mod", "-packages")
	want = `x.x/y/z
x.x/y/z/w
`
	if have := tg.getStdout(); have != want {
		t.Fatalf("go mod -packages mismatch:\nhave:<<<\n%s>>>\nwant:<<<\n%s\n", have, want)
	}

	data, err := ioutil.ReadFile(tg.path("go.mod"))
	tg.must(err)
	data = bytes.Replace(data, []byte("\n"), []byte("\r\n"), -1)
	data = append(data, "    \n"...)
	tg.must(ioutil.WriteFile(tg.path("go.mod"), data, 0666))

	tg.run("-vgo", "mod", "-fmt")
	mustHaveGoMod(`module x.x/y/z

exclude x.1 v1.2.0

replace x.1 v1.4.0 => ../z

require x.3 v1.99.0
`)
}

// TODO(rsc): Test mod -sync, mod -fix (network required).

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
	tg.grepStderr("build constraints exclude all Go files", "no Go source files without tags")

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
	tg.run("-vgo", "list", "-m", "all")
	tg.grepStdout("github.com/gobuffalo/uuid.*v0.0.0-20180207211247-3a9fb6c5c481", "did downgrade to v0.0.0-*")

	tooSlow(t)

	tg.must(ioutil.WriteFile(tg.path("x/go.mod"), []byte(`
		module x
		require github.com/gobuffalo/uuid v1.2.0
	`), 0666))
	tg.run("-vgo", "get", "github.com/gobuffalo/uuid@v1.1.0")
	tg.run("-vgo", "list", "-m", "-u", "all")
	tg.grepStdout(`github.com/gobuffalo/uuid v1.1.0`, "did downgrade to v1.1.0")
	tg.grepStdout(`github.com/gobuffalo/uuid v1.1.0 \[v1`, "did show upgrade to v1.2.0 or later")

	tg.must(ioutil.WriteFile(tg.path("x/go.mod"), []byte(`
		module x
		require github.com/gobuffalo/uuid v1.1.0
	`), 0666))
	tg.run("-vgo", "get", "github.com/gobuffalo/uuid@v1.2.0")
	tg.run("-vgo", "list", "-m", "all")
	tg.grepStdout("github.com/gobuffalo/uuid.*v1.2.0", "did upgrade to v1.2.0")

	// @7f39a6fea4fe9364 should resolve,
	// and also there should be no build error about not having Go files in the root.
	tg.run("-vgo", "get", "golang.org/x/crypto@7f39a6fea4fe9364")

	// @7f39a6fea4fe9364 should resolve.
	// Now there should be no build at all.
	tg.run("-vgo", "get", "-m", "golang.org/x/crypto@7f39a6fea4fe9364")

	// TODO(rsc): These should work, but "go get" needs more work
	// regarding packages versus modules.

	// @7f39a6fea4fe9364 should resolve.
	// Now there should be no build at all.
	// tg.run("-vgo", "get", "-m", "-x", "golang.org/x/crypto/pbkdf2@7f39a6fea4fe9364")
	// tg.grepStderrNot("compile", "should not see compile steps")

	// @7f39a6fea4fe9364 should resolve.
	// Now there should be a build
	// tg.run("-vgo", "get", "-x", "golang.org/x/crypto/pbkdf2@7f39a6fea4fe9364")
	// tg.grepStderr("compile", "should see compile steps")

	// .../pbkdf2@7f39a6fea4fe9364 should NOT resolve:
	// we are using -m and .../pbkdf2 is not a module path.
	tg.runFail("-vgo", "get", "-m", "golang.org/x/crypto/pbkdf2@7f39a6fea4fe9364")
}

func TestGetModuleUpgrade(t *testing.T) {
	testenv.MustHaveExternalNetwork(t)

	tg := testgo(t)
	defer tg.cleanup()
	tg.makeTempdir()

	tg.setenv(homeEnvName(), tg.path("home"))
	tg.must(os.MkdirAll(tg.path("x"), 0777))
	tg.cd(tg.path("x"))
	tg.must(ioutil.WriteFile(tg.path("x/x.go"), []byte(`package x; import _ "rsc.io/quote"`), 0666))

	tg.must(ioutil.WriteFile(tg.path("x/go.mod"), []byte(`
		module x
		require rsc.io/quote v1.5.1
	`), 0666))

	tg.run("-vgo", "get", "-x", "-u")
	tg.run("-vgo", "list", "-m", "all")
	tg.grepStdout(`quote v1.5.2$`, "should have upgraded only to v1.5.2")

	tg.run("-vgo", "get", "-m", "rsc.io/quote@dd9747d")
	tg.run("-vgo", "list", "-m", "all")
	tg.grepStdout(`quote v0.0.0-20180628003336-dd9747d19b04$`, "should have moved to pseudo-commit")

	tg.run("-vgo", "get", "-m", "-u")
	tg.run("-vgo", "list", "-m", "all")
	tg.grepStdout(`quote v0.0.0-20180628003336-dd9747d19b04$`, "should have stayed on pseudo-commit")

	tg.run("-vgo", "get", "-m", "rsc.io/quote@23179ee8a")
	tg.run("-vgo", "list", "-m", "all")
	tg.grepStdout(`quote v0.0.0-20180214005840-23179ee8a569$`, "should have moved to new pseudo-commit")

	tg.run("-vgo", "get", "-m", "-u")
	tg.run("-vgo", "list", "-m", "all")
	tg.grepStdout(`quote v1.5.2$`, "should have moved off pseudo-commit")

	tg.must(ioutil.WriteFile(tg.path("x/go.mod"), []byte(`
		module x
	`), 0666))
	tg.run("-vgo", "list")
	tg.grepStderr(`adding rsc.io/quote v1.5.2`, "should have added quote v1.5.2")
	tg.grepStderrNot(`v1.5.3-pre1`, "should not mention v1.5.3-pre1")
}

func TestVgoBadDomain(t *testing.T) {
	tg := testgo(t)
	defer tg.cleanup()
	wd, _ := os.Getwd()
	tg.cd(filepath.Join(wd, "testdata/badmod"))

	tg.runFail("-vgo", "get", "appengine")
	tg.grepStderr(`unrecognized import path \"appengine\"`, "expected appengine error ")
	tg.runFail("-vgo", "get", "x/y.z")
	tg.grepStderr(`unrecognized import path \"x/y.z\" \(import path does not begin with hostname\)`, "expected domain error")

	tg.runFail("-vgo", "build")
	tg.grepStderrNot("unknown module appengine: not a domain name", "expected nothing about appengine")
	tg.grepStderr("tcp.*nonexistent.rsc.io", "expected error for nonexistent.rsc.io")
}

func TestVgoSync(t *testing.T) {
	tg := testgo(t)
	defer tg.cleanup()
	tg.makeTempdir()

	write := func(name, text string) {
		name = tg.path(name)
		dir := filepath.Dir(name)
		tg.must(os.MkdirAll(dir, 0777))
		tg.must(ioutil.WriteFile(name, []byte(text), 0666))
	}

	write("m/go.mod", `
module m

require (
	x.1 v1.0.0
	y.1 v1.0.0
	w.1 v1.2.0
)

replace x.1 v1.0.0 => ../x
replace y.1 v1.0.0 => ../y
replace z.1 v1.1.0 => ../z
replace z.1 v1.2.0 => ../z
replace w.1 v1.1.0 => ../w
replace w.1 v1.2.0 => ../w
`)
	write("m/m.go", `
package m

import _ "x.1"
import _ "z.1/sub"
`)

	write("w/go.mod", `
module w
`)
	write("w/w.go", `
package w
`)

	write("x/go.mod", `
module x
require w.1 v1.1.0
require z.1 v1.1.0
`)
	write("x/x.go", `
package x

import _ "w.1"
`)

	write("y/go.mod", `
module y
require z.1 v1.2.0
`)

	write("z/go.mod", `
module z
`)
	write("z/sub/sub.go", `
package sub
`)

	tg.cd(tg.path("m"))
	tg.run("-vgo", "mod", "-sync", "-v")
	tg.grepStderr(`^unused y.1`, "need y.1 unused")
	tg.grepStderrNot(`^unused [^y]`, "only y.1 should be unused")

	tg.run("-vgo", "list", "-m", "all")
	tg.grepStdoutNot(`^y.1`, "y should be gone")
	tg.grepStdout(`^w.1\s+v1.2.0`, "need w.1 to stay at v1.2.0")
	tg.grepStdout(`^z.1\s+v1.2.0`, "need z.1 to stay at v1.2.0 even though y is gone")
}

func TestVgoVendor(t *testing.T) {
	tg := testgo(t)
	defer tg.cleanup()

	wd, _ := os.Getwd()
	tg.cd(filepath.Join(wd, "testdata/vendormod"))
	defer os.RemoveAll(filepath.Join(wd, "testdata/vendormod/vendor"))

	tg.run("-vgo", "list", "-m", "all")
	tg.grepStdout(`^x`, "expected to see module x")
	tg.grepStdout(`=> ./x`, "expected to see replacement for module x")
	tg.grepStdout(`^w`, "expected to see module w")

	tg.must(os.RemoveAll(filepath.Join(wd, "testdata/vendormod/vendor")))
	if !testing.Short() {
		tg.run("-vgo", "build")
		tg.runFail("-vgo", "build", "-getmode=vendor")
	}

	tg.run("-vgo", "list", "-f={{.Dir}}", "x")
	tg.grepStdout(`vendormod[/\\]x$`, "expected x in vendormod/x")

	tg.run("-vgo", "mod", "-vendor", "-v")
	tg.grepStderr(`^# x v1.0.0 => ./x`, "expected to see module x with replacement")
	tg.grepStderr(`^x`, "expected to see package x")
	tg.grepStderr(`^# y v1.0.0 => ./y`, "expected to see module y with replacement")
	tg.grepStderr(`^y`, "expected to see package y")
	tg.grepStderr(`^# z v1.0.0 => ./z`, "expected to see module z with replacement")
	tg.grepStderr(`^z`, "expected to see package z")
	tg.grepStderrNot(`w`, "expected NOT to see unused module w")

	tg.run("-vgo", "list", "-f={{.Dir}}", "x")
	tg.grepStdout(`vendormod[/\\]x$`, "expected x in vendormod/x")

	tg.run("-vgo", "list", "-f={{.Dir}}", "-m", "x")
	tg.grepStdout(`vendormod[/\\]x$`, "expected x in vendormod/x")

	tg.run("-vgo", "list", "-getmode=vendor", "-f={{.Dir}}", "x")
	tg.grepStdout(`vendormod[/\\]vendor[/\\]x$`, "expected x in vendormod/vendor/x in -get=vendor mode")

	tg.run("-vgo", "list", "-getmode=vendor", "-f={{.Dir}}", "-m", "x")
	tg.grepStdout(`vendormod[/\\]vendor[/\\]x$`, "expected x in vendormod/vendor/x in -get=vendor mode")

	tg.run("-vgo", "list", "-f={{.Dir}}", "w")
	tg.grepStdout(`vendormod[/\\]w$`, "expected w in vendormod/w")
	tg.runFail("-vgo", "list", "-getmode=vendor", "-f={{.Dir}}", "w")
	tg.grepStderr(`vendormod[/\\]vendor[/\\]w`, "want error about vendormod/vendor/w not existing")

	tg.run("-vgo", "list", "-getmode=local", "-f={{.Dir}}", "w")
	tg.grepStdout(`vendormod[/\\]w`, "expected w in vendormod/w")

	tg.runFail("-vgo", "list", "-getmode=local", "-f={{.Dir}}", "newpkg")
	tg.grepStderr(`disabled by -getmode=local`, "expected -getmode=local to avoid network")

	if !testing.Short() {
		tg.run("-vgo", "build")
		tg.run("-vgo", "build", "-getmode=vendor")
		tg.cd(filepath.Join(wd, "testdata/vendormod/vendor"))
		tg.run("-vgo", "test", "-getmode=vendor", "./...")
	}
}

func TestFillGoMod(t *testing.T) {
	testenv.MustHaveExternalNetwork(t)
	tg := testgo(t)
	defer tg.cleanup()
	tg.makeTempdir()

	tg.setenv(homeEnvName(), tg.path("."))
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
	tg.run("-vgo", "list", "-m", "all")
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

func TestRequireExcluded(t *testing.T) {
	tg := testgo(t)
	defer tg.cleanup()
	tg.makeTempdir()

	tg.must(os.MkdirAll(tg.path("x"), 0777))
	tg.must(ioutil.WriteFile(tg.path("x/x.go"), []byte(`package x; import _ "github.com/gorilla/mux"`), 0666))

	tg.setenv(homeEnvName(), tg.path("home"))
	tg.cd(tg.path("x"))

	tg.must(ioutil.WriteFile(tg.path("x/go.mod"), []byte(`
		module x
		exclude github.com/gorilla/mux latest
		require github.com/gorilla/mux latest
	`), 0666))
	tg.runFail("-vgo", "build")
	tg.grepStderr("no newer version available", "only available version excluded")

	tg.must(ioutil.WriteFile(tg.path("x/go.mod"), []byte(`
		module x
		exclude github.com/gorilla/mux v1.6.1
		require github.com/gorilla/mux v1.6.1
	`), 0666))
	tg.run("-vgo", "build")
	tg.grepStderr("github.com/gorilla/mux v1.6.2", "find version 1.6.2")

	tg.must(ioutil.WriteFile(tg.path("x/go.mod"), []byte(`
		module x
		exclude github.com/gorilla/mux v1.6.2
		require github.com/gorilla/mux v1.6.1
	`), 0666))
	tg.run("-vgo", "build")
	tg.grepStderr("github.com/gorilla/mux v1.6.1", "find version 1.6.1")
}

func TestConvertLegacyConfig(t *testing.T) {
	testenv.MustHaveExternalNetwork(t)
	tg := testgo(t)
	defer tg.cleanup()
	tg.makeTempdir()

	tg.setenv(homeEnvName(), tg.path("."))

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
	tg.run("-vgo", "list", "-m", "all")

	// If the conversion just ignored the Gopkg.lock entirely
	// it would choose a newer version (like v0.8.0 or maybe
	// something even newer). Check for the older version to
	// make sure Gopkg.lock was properly used.
	tg.grepStdout("v0.6.0", "expected github.com/pkg/errors at v0.6.0")
}

func TestVerify(t *testing.T) {
	testenv.MustHaveExternalNetwork(t)
	tg := testgo(t)
	defer tg.cleanup()
	tg.makeTempdir()
	gopath := tg.path("gp")
	tg.setenv("GOPATH", gopath)
	tg.must(os.MkdirAll(tg.path("x"), 0777))
	tg.must(ioutil.WriteFile(tg.path("x/go.mod"), []byte(`
		module x
		require github.com/pkg/errors v0.8.0
	`), 0666))
	tg.must(ioutil.WriteFile(tg.path("x/x.go"), []byte(`package x; import _ "github.com/pkg/errors"`), 0666))

	// With correct go.sum,verify succeeds but avoids download.
	tg.must(ioutil.WriteFile(tg.path("x/go.sum"), []byte(`github.com/pkg/errors v0.8.0 h1:WdK/asTD0HN+q6hsWO3/vpuAkAr+tw6aNJNDFFf0+qw=
`), 0666))
	tg.cd(tg.path("x"))
	tg.run("-vgo", "mod", "-verify")
	tg.mustNotExist(filepath.Join(gopath, "src/mod/cache/download/github.com/pkg/errors/@v/v0.8.0.zip"))
	tg.mustNotExist(filepath.Join(gopath, "src/mod/github.com/pkg"))

	// With incorrect sum, sync (which must download) fails.
	// Even if the incorrect sum is in the old legacy go.modverify file.
	tg.must(ioutil.WriteFile(tg.path("x/go.sum"), []byte(`
`), 0666))
	tg.must(ioutil.WriteFile(tg.path("x/go.modverify"), []byte(`github.com/pkg/errors v0.8.0 h1:WdK/asTD0HN+q6hsWO3/vpuAkAr+tw6aNJNDFFf1+qw=
`), 0666))
	tg.runFail("-vgo", "mod", "-sync") // downloads pkg/errors
	tg.grepStderr("checksum mismatch", "must detect mismatch")
	tg.mustNotExist(filepath.Join(gopath, "src/mod/cache/download/github.com/pkg/errors/@v/v0.8.0.zip"))
	tg.mustNotExist(filepath.Join(gopath, "src/mod/github.com/pkg"))

	// With corrected sum, sync works.
	tg.must(ioutil.WriteFile(tg.path("x/go.modverify"), []byte(`github.com/pkg/errors v0.8.0 h1:WdK/asTD0HN+q6hsWO3/vpuAkAr+tw6aNJNDFFf0+qw=
`), 0666))
	tg.run("-vgo", "mod", "-sync")
	tg.mustExist(filepath.Join(gopath, "src/mod/cache/download/github.com/pkg/errors/@v/v0.8.0.zip"))
	tg.mustExist(filepath.Join(gopath, "src/mod/github.com/pkg"))
	tg.mustNotExist(tg.path("x/go.modverify")) // moved into go.sum

	// Sync should have added sum for go.mod.
	data, err := ioutil.ReadFile(tg.path("x/go.sum"))
	if !strings.Contains(string(data), "\ngithub.com/pkg/errors v0.8.0/go.mod ") {
		t.Fatalf("cannot find go.mod hash in go.sum: %v\n%s", err, data)
	}

	// Verify should work too.
	tg.run("-vgo", "mod", "-verify")

	// Even the most basic attempt to load the module graph should detect incorrect go.mod files.
	tg.run("-vgo", "mod", "-graph") // loads module graph, is OK
	tg.must(ioutil.WriteFile(tg.path("x/go.sum"), []byte(`github.com/pkg/errors v0.8.0 h1:WdK/asTD0HN+q6hsWO3/vpuAkAr+tw6aNJNDFFf0+qw=
github.com/pkg/errors v0.8.0/go.mod h1:bwawxfHBFNV+L2hUp1rHADufV3IMtnDRdf1r5NINEl1=
`), 0666))
	tg.runFail("-vgo", "mod", "-graph") // loads module graph, fails (even though sum is in old go.modverify file)
	tg.grepStderr("go.mod: checksum mismatch", "must detect mismatch")

	// go.sum should be created and updated automatically.
	tg.must(os.Remove(tg.path("x/go.sum")))
	tg.run("-vgo", "mod", "-graph")
	tg.mustExist(tg.path("x/go.sum"))
	data, err = ioutil.ReadFile(tg.path("x/go.sum"))
	if !strings.Contains(string(data), " v0.8.0/go.mod ") {
		t.Fatalf("cannot find go.mod hash in go.sum: %v\n%s", err, data)
	}
	if strings.Contains(string(data), " v0.8.0 ") {
		t.Fatalf("unexpected module tree hash in go.sum: %v\n%s", err, data)
	}
	tg.run("-vgo", "mod", "-sync")
	data, err = ioutil.ReadFile(tg.path("x/go.sum"))
	if !strings.Contains(string(data), " v0.8.0/go.mod ") {
		t.Fatalf("cannot find go.mod hash in go.sum: %v\n%s", err, data)
	}
	if !strings.Contains(string(data), " v0.8.0 ") {
		t.Fatalf("cannot find module tree hash in go.sum: %v\n%s", err, data)
	}

	tg.must(os.Remove(filepath.Join(gopath, "src/mod/cache/download/github.com/pkg/errors/@v/v0.8.0.ziphash")))
	tg.run("-vgo", "mod", "-sync") // ignores missing ziphash file for ordinary go.sum validation

	tg.runFail("-vgo", "mod", "-verify") // explicit verify fails with missing ziphash
}

func TestVendorWithoutDeps(t *testing.T) {
	tg := testgo(t)
	defer tg.cleanup()
	tg.makeTempdir()

	tg.must(os.MkdirAll(tg.path("x"), 0777))
	tg.must(ioutil.WriteFile(tg.path("x/main.go"), []byte(`package x`), 0666))
	tg.must(ioutil.WriteFile(tg.path("x/go.mod"), []byte(`module x`), 0666))
	tg.cd(tg.path("x"))
	tg.run("-vgo", "mod", "-vendor")
	tg.grepStderr("vgo: no dependencies to vendor", "print vendor info")
}

func TestVersionWithoutModule(t *testing.T) {
	tg := testgo(t)
	defer tg.cleanup()
	tg.makeTempdir()

	tg.cd(tg.path("."))
	tg.run("-vgo", "version")
}

func TestImportDir(t *testing.T) {
	testenv.MustHaveExternalNetwork(t)
	tg := testgo(t)
	defer tg.cleanup()
	tg.makeTempdir()

	tg.setenv("GOPATH", tg.path("."))
	tg.must(os.MkdirAll(tg.path("x"), 0777))
	tg.must(ioutil.WriteFile(tg.path("x/main.go"), []byte(`
		package x
		import _ "goji.io"`), 0666))
	tg.must(ioutil.WriteFile(tg.path("x/go.mod"), []byte("module x"), 0666))
	tg.must(os.MkdirAll(filepath.Join(runtime.GOROOT(), "src", "goji.io"), 0777))
	tg.cd(tg.path("x"))
	tg.run("-vgo", "build")
}

func TestModSyncPrintJson(t *testing.T) {
	testenv.MustHaveExternalNetwork(t)
	tg := testgo(t)
	defer tg.cleanup()
	tg.makeTempdir()

	tg.setenv("GOPATH", tg.path("."))
	tg.must(os.MkdirAll(tg.path("x"), 0777))
	tg.must(ioutil.WriteFile(tg.path("x/main.go"), []byte(`
		package x
		import "github.com/gorilla/mux"
		func main() {
			_ = mux.NewRouter()
		}`), 0666))
	tg.must(ioutil.WriteFile(tg.path("x/go.mod"), []byte("module x"), 0666))
	tg.cd(tg.path("x"))
	tg.run("-vgo", "mod", "-sync", "-json")
	count := tg.grepCountBoth(`"Path": "github.com/gorilla/mux",`)
	if count != 1 {
		t.Fatal("produces duplicate imports")
	}
	// test quoted module path
	tg.must(ioutil.WriteFile(tg.path("x/go.mod"), []byte(`
		module x
		require (
			"github.com/gorilla/context" v1.1.1
			"github.com/gorilla/mux" v1.6.2
	)`), 0666))
	tg.run("-vgo", "mod", "-sync", "-json")
	count = tg.grepCountBoth(`"Path": "github.com/gorilla/mux",`)
	if count != 1 {
		t.Fatal("produces duplicate imports")
	}

}
