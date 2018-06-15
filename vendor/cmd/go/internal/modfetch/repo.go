// Copyright 2018 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package modfetch

import (
	"errors"
	"fmt"
	"os"
	pathpkg "path"
	"sort"
	"strings"
	"time"

	"cmd/go/internal/cfg"
	"cmd/go/internal/modfetch/bitbucket"
	"cmd/go/internal/modfetch/codehost"
	"cmd/go/internal/modfetch/github"
	"cmd/go/internal/modfetch/googlesource"
	"cmd/go/internal/module"
	"cmd/go/internal/par"
	"cmd/go/internal/semver"
)

const traceRepo = false // trace all repo actions, for debugging

// A Repo represents a repository storing all versions of a single module.
type Repo interface {
	// ModulePath returns the module path.
	ModulePath() string

	// Versions lists all known versions with the given prefix.
	// Pseudo-versions are not included.
	// Versions should be returned sorted in semver order
	// (implementations can use SortVersions).
	Versions(prefix string) (tags []string, err error)

	// Stat returns information about the revision rev.
	// A revision can be any identifier known to the underlying service:
	// commit hash, branch, tag, and so on.
	Stat(rev string) (*RevInfo, error)

	// Latest returns the latest revision on the default branch,
	// whatever that means in the underlying source code repository.
	// It is only used when there are no tagged versions.
	Latest() (*RevInfo, error)

	// GoMod returns the go.mod file for the given version.
	GoMod(version string) (data []byte, err error)

	// Zip downloads a zip file for the given version
	// to a new file in a given temporary directory.
	// It returns the name of the new file.
	// The caller should remove the file when finished with it.
	Zip(version, tmpdir string) (tmpfile string, err error)
}

// A Rev describes a single revision in a module repository.
type RevInfo struct {
	Version string    // version string
	Name    string    // complete ID in underlying repository
	Short   string    // shortened ID, for use in pseudo-version
	Time    time.Time // commit time
}

var lookupCache par.Cache

// Lookup returns the module with the given module path.
func Lookup(path string) (Repo, error) {
	if traceRepo {
		defer logCall("Lookup(%q)", path)()
	}

	type cached struct {
		r   Repo
		err error
	}
	c := lookupCache.Do(path, func() interface{} {
		r, err := lookup(path)
		if err == nil {
			if traceRepo {
				r = newLoggingRepo(r)
			}
			r = newCachingRepo(r)
		}
		return cached{r, err}
	}).(cached)

	return c.r, c.err
}

// lookup returns the module with the given module path.
func lookup(path string) (r Repo, err error) {
	if cfg.BuildGetmode != "" {
		return nil, fmt.Errorf("module lookup disabled by -getmode=%s", cfg.BuildGetmode)
	}
	if proxyURL != "" {
		return lookupProxy(path)
	}
	if code, err := lookupCodeHost(path, false); err != errNotHosted {
		if err != nil {
			return nil, err
		}
		return newCodeRepo(code, path)
	}
	return lookupCustomDomain(path)
}

func Import(path string, allowed func(module.Version) bool) (Repo, *RevInfo, error) {
	if traceRepo {
		defer logCall("Import(%q, ...)", path)()
	}
	try := func(path string) (Repo, *RevInfo, error) {
		r, err := Lookup(path)
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
		return r, info, nil
	}

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

var errNotHosted = errors.New("not hosted")

var isTest bool

func lookupCodeHost(path string, customDomain bool) (codehost.Repo, error) {
	switch {
	case strings.HasPrefix(path, "github.com/"):
		return github.Lookup(path)
	case strings.HasPrefix(path, "bitbucket.org/"):
		return bitbucket.Lookup(path)
	case customDomain && strings.HasSuffix(path[:strings.Index(path, "/")+1], ".googlesource.com/") ||
		isTest && strings.HasPrefix(path, "go.googlesource.com/scratch"):
		return googlesource.Lookup(path)
	case strings.HasPrefix(path, "gopkg.in/"):
		return gopkginLookup(path)
	}
	return nil, errNotHosted
}

func SortVersions(list []string) {
	sort.Slice(list, func(i, j int) bool {
		cmp := semver.Compare(list[i], list[j])
		if cmp != 0 {
			return cmp < 0
		}
		return list[i] < list[j]
	})
}

// A loggingRepo is a wrapper around an underlying Repo
// that prints a log message at the start and end of each call.
// It can be inserted when debugging.
type loggingRepo struct {
	r Repo
}

func newLoggingRepo(r Repo) *loggingRepo {
	return &loggingRepo{r}
}

// logCall prints a log message using format and args and then
// also returns a function that will print the same message again,
// along with the elapsed time.
// Typical usage is:
//
//	defer logCall("hello %s", arg)()
//
// Note the final ().
func logCall(format string, args ...interface{}) func() {
	start := time.Now()
	fmt.Fprintf(os.Stderr, "+++ %s\n", fmt.Sprintf(format, args...))
	return func() {
		fmt.Fprintf(os.Stderr, "%.3fs %s\n", time.Since(start).Seconds(), fmt.Sprintf(format, args...))
	}
}

func (l *loggingRepo) ModulePath() string {
	return l.r.ModulePath()
}

func (l *loggingRepo) Versions(prefix string) (tags []string, err error) {
	defer logCall("Repo[%s]: Versions(%q)", l.r.ModulePath(), prefix)()
	return l.r.Versions(prefix)
}

func (l *loggingRepo) Stat(rev string) (*RevInfo, error) {
	defer logCall("Repo[%s]: Stat(%q)", l.r.ModulePath(), rev)()
	return l.r.Stat(rev)
}

func (l *loggingRepo) Latest() (*RevInfo, error) {
	defer logCall("Repo[%s]: Latest()", l.r.ModulePath())()
	return l.r.Latest()
}

func (l *loggingRepo) GoMod(version string) ([]byte, error) {
	defer logCall("Repo[%s]: GoMod(%q)", l.r.ModulePath(), version)()
	return l.r.GoMod(version)
}

func (l *loggingRepo) Zip(version, tmpdir string) (string, error) {
	defer logCall("Repo[%s]: Zip(%q, %q)", l.r.ModulePath(), version, tmpdir)()
	return l.r.Zip(version, tmpdir)
}
