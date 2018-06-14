// Copyright 2018 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package modcmd

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"cmd/go/internal/base"
	"cmd/go/internal/module"
	"cmd/go/internal/vgo"
)

var copiedDir map[string]bool

func runVendor() {
	pkgs := vgo.ImportPaths([]string{"ALL"})

	vdir := filepath.Join(vgo.ModRoot, "vendor")
	if err := os.RemoveAll(vdir); err != nil {
		base.Fatalf("vgo vendor: %v", err)
	}

	modpkgs := make(map[module.Version][]string)
	for _, pkg := range pkgs {
		m := vgo.PackageModule(pkg)
		if m == vgo.Target {
			continue
		}
		modpkgs[m] = append(modpkgs[m], pkg)
	}

	var buf bytes.Buffer
	copiedDir = make(map[string]bool)
	for _, m := range vgo.BuildList()[1:] {
		if pkgs := modpkgs[m]; len(pkgs) > 0 {
			repl := ""
			if r := vgo.Replacement(m); r.Path != "" {
				repl = " => " + r.Path
				if r.Version != "" {
					repl += " " + r.Version
				}
			}
			fmt.Fprintf(&buf, "# %s %s%s\n", m.Path, m.Version, repl)
			if *modV {
				fmt.Fprintf(os.Stderr, "# %s %s%s\n", m.Path, m.Version, repl)
			}
			for _, pkg := range pkgs {
				fmt.Fprintf(&buf, "%s\n", pkg)
				if *modV {
					fmt.Fprintf(os.Stderr, "%s\n", pkg)
				}
				vendorPkg(vdir, pkg)
			}
		}
	}
	if buf.Len() == 0 {
		fmt.Fprintf(os.Stderr, "vgo: no dependencies to vendor\n")
		return
	}
	if err := ioutil.WriteFile(filepath.Join(vdir, "vgo.list"), buf.Bytes(), 0666); err != nil {
		base.Fatalf("vgo vendor: %v", err)
	}
}

func vendorPkg(vdir, pkg string) {
	realPath := vgo.ImportMap(pkg)
	if realPath != pkg && vgo.ImportMap(realPath) != "" {
		fmt.Fprintf(os.Stderr, "warning: %s imported as both %s and %s; making two copies.\n", realPath, realPath, pkg)
	}

	dst := filepath.Join(vdir, pkg)
	src := vgo.PackageDir(realPath)
	if src == "" {
		fmt.Fprintf(os.Stderr, "internal error: no pkg for %s -> %s\n", pkg, realPath)
	}

	copyDir(dst, src, false)
	if m := vgo.PackageModule(realPath); m.Path != "" {
		copyTestdata(m.Path, realPath, dst, src)
	}
}

// Copy the testdata directories in parent directories.
// If the package being vendored is a/b/c,
// try to copy a/b/c/testdata, a/b/testdata and a/testdata to vendor directory,
// up to the module root.
func copyTestdata(modPath, pkg, dst, src string) {
	testdata := func(dir string) string {
		return filepath.Join(dir, "testdata")
	}
	for {
		if copiedDir[dst] {
			break
		}
		copiedDir[dst] = true
		if info, err := os.Stat(testdata(src)); err == nil && info.IsDir() {
			copyDir(testdata(dst), testdata(src), true)
		}
		if modPath == pkg {
			break
		}
		pkg = filepath.Dir(pkg)
		dst = filepath.Dir(dst)
		src = filepath.Dir(src)
	}
}

func copyDir(dst, src string, recursive bool) {
	files, err := ioutil.ReadDir(src)
	if err != nil {
		base.Fatalf("vgo vendor: %v", err)
	}
	if err := os.MkdirAll(dst, 0777); err != nil {
		base.Fatalf("vgo vendor: %v", err)
	}
	for _, file := range files {
		if file.IsDir() {
			if recursive || file.Name() == "testdata" {
				copyDir(filepath.Join(dst, file.Name()), filepath.Join(src, file.Name()), true)
			}
			continue
		}
		if !file.Mode().IsRegular() {
			continue
		}
		r, err := os.Open(filepath.Join(src, file.Name()))
		if err != nil {
			base.Fatalf("vgo vendor: %v", err)
		}
		w, err := os.Create(filepath.Join(dst, file.Name()))
		if err != nil {
			base.Fatalf("vgo vendor: %v", err)
		}
		if _, err := io.Copy(w, r); err != nil {
			base.Fatalf("vgo vendor: %v", err)
		}
		r.Close()
		if err := w.Close(); err != nil {
			base.Fatalf("vgo vendor: %v", err)
		}
	}
}

// hasPathPrefix reports whether the path s begins with the
// elements in prefix.
func hasPathPrefix(s, prefix string) bool {
	switch {
	default:
		return false
	case len(s) == len(prefix):
		return s == prefix
	case len(s) > len(prefix):
		if prefix != "" && prefix[len(prefix)-1] == '/' {
			return strings.HasPrefix(s, prefix)
		}
		return s[len(prefix)] == '/' && s[:len(prefix)] == prefix
	}
}
