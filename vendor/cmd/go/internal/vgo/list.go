// Copyright 2018 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package vgo

import (
	"fmt"
	"os"
	"strings"

	"cmd/go/internal/base"
	"cmd/go/internal/modinfo"
	"cmd/go/internal/search"
)

func ListModules(args []string) []*modinfo.ModulePublic {
	LoadBuildList()
	if len(args) == 0 {
		return []*modinfo.ModulePublic{moduleInfo(buildList[0], true)}
	}

	var mods []*modinfo.ModulePublic
	matchedBuildList := make([]bool, len(buildList))
	for _, arg := range args {
		if strings.Contains(arg, `\`) {
			base.Fatalf("vgo: module paths never use backslash")
		}
		if search.IsRelativePath(arg) {
			base.Fatalf("vgo: cannot use relative path %s to specify module", arg)
		}
		if strings.Contains(arg, "@") {
			// TODO(rsc): Add support for 'go list -m golang.org/x/text@v0.3.0'
			base.Fatalf("vgo: list path@version not implemented")
		}

		// Module path or pattern.
		var match func(string) bool
		if arg == "all" {
			match = func(string) bool { return true }
		} else {
			match = search.MatchPattern(arg)
		}
		matched := false
		for i, m := range buildList {
			if match(m.Path) {
				matched = true
				if !matchedBuildList[i] {
					matchedBuildList[i] = true
					mods = append(mods, moduleInfo(m, true))
				}
			}
		}
		if !matched {
			fmt.Fprintf(os.Stderr, "warning: pattern %q matched no module dependencies", arg)
		}
	}

	return mods
}
