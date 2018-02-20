// Copyright 2018 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package vgo

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"cmd/go/internal/base"
)

var CmdVendor = &base.Command{
	UsageLine: "vendor",
	Run:       runVendor,
	Short:     "vendor dependencies of current module",
	Long: `
Vendor resets the module's vendor directory to include all
packages needed to builds and test all packages in the module
and their dependencies.
	`,
}

func runVendor(cmd *base.Command, args []string) {
	if Init(); !Enabled() {
		base.Fatalf("vgo vendor: cannot use -m outside module")
	}
	if len(args) != 0 {
		base.Fatalf("vgo vendor: vendor takes no arguments")
	}
	InitMod()
	// TODO(rsc): This should scan directories with more permissive build tags.
	pkgs := ImportPaths([]string{"all"})

	vdir := filepath.Join(ModRoot, "vendor")
	if err := os.RemoveAll(vdir); err != nil {
		base.Fatalf("vgo vendor: %v", err)
	}

	for _, pkg := range pkgs {
		vendorPkg(vdir, pkg)
	}

	var buf bytes.Buffer
	printListM(&buf)
	if err := ioutil.WriteFile(filepath.Join(vdir, "vgo.list"), buf.Bytes(), 0666); err != nil {
		base.Fatalf("vgo vendor: %v", err)
	}
}

func vendorPkg(vdir, pkg string) {
	if hasPathPrefix(pkg, Target.Path) {
		return
	}
	realPath := importmap[pkg]
	if realPath != pkg && importmap[realPath] != "" {
		fmt.Fprintf(os.Stderr, "warning: %s imported as both %s and %s; making two copies.\n", realPath, realPath, pkg)
	}

	dst := filepath.Join(vdir, pkg)
	src := pkgdir[realPath]
	if src == "" {
		fmt.Fprintf(os.Stderr, "internal error: no pkg for %s -> %s\n", pkg, realPath)
	}
	copyDir(dst, src, false)
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
