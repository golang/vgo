// Copyright 2018 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package vgo

import (
	"fmt"
	"os"
	"strings"

	"cmd/go/internal/base"
	"cmd/go/internal/modfetch"
	"cmd/go/internal/modinfo"
	"cmd/go/internal/module"
	"cmd/go/internal/par"
	"cmd/go/internal/search"
)

func ListModules(args []string, listU, listVersions bool) []*modinfo.ModulePublic {
	mods := listModules(args)
	if listU || listVersions {
		var work par.Work
		for _, m := range mods {
			work.Add(m)
			if m.Replace != nil {
				work.Add(m.Replace)
			}
		}
		work.Do(10, func(item interface{}) {
			m := item.(*modinfo.ModulePublic)
			if listU {
				addUpdate(m)
			}
			if listVersions {
				addVersions(m)
			}
		})
	}
	return mods
}

func listModules(args []string) []*modinfo.ModulePublic {
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
		if i := strings.Index(arg, "@"); i >= 0 {
			info, err := modfetch.Query(arg[:i], arg[i+1:], nil)
			if err != nil {
				mods = append(mods, &modinfo.ModulePublic{
					Path:    arg[:i],
					Version: arg[i+1:],
					Error: &modinfo.ModuleError{
						Err: err.Error(),
					},
				})
				continue
			}
			mods = append(mods, moduleInfo(module.Version{Path: arg[:i], Version: info.Version}, false))
			continue
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
			fmt.Fprintf(os.Stderr, "warning: pattern %q matched no module dependencies\n", arg)
		}
	}

	return mods
}
