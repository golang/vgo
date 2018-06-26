// Copyright 2018 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package modfetch

import (
	"cmd/go/internal/module"
	"cmd/go/internal/semver"
	"fmt"
	"strings"
)

// Query looks up a revision of a given module given a version query string.
// The module must be a complete module path.
// The version must take one of the following forms:
//
//	- the literal string "latest", denoting the latest available, allowed tagged version,
//	  with non-prereleases preferred over prereleases
//	- v1.2.3, a semantic version string
//	- v1 or v1.2, an abbreviated semantic version string completed by adding zeroes (v1.0.0 or v1.2.0)
//	- >v1.2.3, denoting the earliest available version after v1.2.3 (including prereleases)
//	- <v1.2.3, denoting the latest available version before v1.2.3 (including prereleases)
//	- a repository commit identifier, denoting that version
//
// If the allowed function is non-nil, Query excludes any versions for which allowed returns false.
//
func Query(path, vers string, allowed func(module.Version) bool) (*RevInfo, error) {
	if allowed == nil {
		allowed = func(module.Version) bool { return true }
	}
	if semver.IsValid(vers) {
		// TODO: This turns query for "v2" into Stat "v2.0.0",
		// but probably it should allow checking for a branch named "v2".
		vers = semver.Canonical(vers)
		if !allowed(module.Version{Path: path, Version: vers}) {
			return nil, fmt.Errorf("%s@%s excluded", path, vers)
		}

		// Fast path that avoids network overhead of Lookup (resolving path to repo host),
		// if we already have this stat information cached on disk.
		info, err := Stat(path, vers)
		if err == nil {
			return info, nil
		}
	}

	repo, err := Lookup(path)
	if err != nil {
		return nil, err
	}

	if semver.IsValid(vers) {
		return repo.Stat(vers)
	}
	if strings.HasPrefix(vers, ">") || strings.HasPrefix(vers, "<") || vers == "latest" {
		var op string
		if vers != "latest" {
			if !semver.IsValid(vers[1:]) {
				return nil, fmt.Errorf("invalid semantic version in range %s", vers)
			}
			op, vers = vers[:1], vers[1:]
		}
		versions, err := repo.Versions("")
		if err != nil {
			return nil, err
		}
		if len(versions) == 0 && vers == "latest" {
			return repo.Latest()
		}
		if vers == "latest" {
			// Prefer a proper (non-prerelease) release.
			for i := len(versions) - 1; i >= 0; i-- {
				if semver.Prerelease(versions[i]) == "" && allowed(module.Version{Path: path, Version: versions[i]}) {
					return repo.Stat(versions[i])
				}
			}
			// Fall back to pre-releases if that's all we have.
			for i := len(versions) - 1; i >= 0; i-- {
				if semver.Prerelease(versions[i]) != "" && allowed(module.Version{Path: path, Version: versions[i]}) {
					return repo.Stat(versions[i])
				}
			}
		} else if op == "<" {
			for i := len(versions) - 1; i >= 0; i-- {
				if semver.Compare(versions[i], vers) < 0 && allowed(module.Version{Path: path, Version: versions[i]}) {
					return repo.Stat(versions[i])
				}
			}
		} else {
			for i := 0; i < len(versions); i++ {
				if semver.Compare(versions[i], vers) > 0 && allowed(module.Version{Path: path, Version: versions[i]}) {
					return repo.Stat(versions[i])
				}
			}
		}
		return nil, fmt.Errorf("no matching versions for %s%s", op, vers)
	}

	return repo.Stat(vers)
}
