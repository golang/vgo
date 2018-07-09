// Copyright 2018 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package modload

import (
	"fmt"
	pathpkg "path"

	"cmd/go/internal/cfg"
	"cmd/go/internal/modfetch"
	"cmd/go/internal/module"
)

// Import returns the module repo and version to use to satisfy the given import path.
// It considers a sequence of module paths starting with the import path and
// removing successive path elements from the end. It stops when it finds a module
// path for which the latest version of the module provides the expected package.
// If non-nil, the allowed function is used to filter known versions of a given module
// before determining which one is "latest".
func Import(path string, allowed func(module.Version) bool) (modfetch.Repo, *modfetch.RevInfo, error) {
	if cfg.BuildGetmode != "" {
		return nil, nil, fmt.Errorf("import resolution disabled by -getmode=%s", cfg.BuildGetmode)
	}

	try := func(path string) (modfetch.Repo, *modfetch.RevInfo, error) {
		r, err := modfetch.Lookup(path)
		if err != nil {
			return nil, nil, err
		}
		info, err := Query(path, "latest", allowed)
		if err != nil {
			return nil, nil, err
		}
		_, err = r.GoMod(info.Version)
		if err != nil {
			return nil, nil, err
		}
		// TODO(rsc): Do what the docs promise: download the module
		// source code and check that it actually contains code for the
		// target import path. To do that efficiently we will need to move
		// the unzipped code cache out of ../modload into this package.
		// TODO(rsc): When this happens, look carefully at the use of
		// modfetch.Import in modget.getQuery.
		return r, info, nil
	}

	// Find enclosing module by walking up path element by element.
	var firstErr error
	for {
		r, info, err := try(path)
		if err == nil {
			return r, info, nil
		}
		if firstErr == nil {
			firstErr = err
		}
		p := pathpkg.Dir(path)
		if p == "." {
			break
		}
		path = p
	}
	return nil, nil, firstErr
}
