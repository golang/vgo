// Copyright 2018 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// TODO: Figure out what gopkg.in should do.

package modfetch

import (
	"cmd/go/internal/modfetch/codehost"
	"cmd/go/internal/modfetch/github"
	"cmd/go/internal/semver"
	"fmt"
	"io"
	"strings"
)

func ParseGopkgIn(path string) (root, repo, major, subdir string, ok bool) {
	if !strings.HasPrefix(path, "gopkg.in/") {
		return
	}
	f := strings.Split(path, "/")
	if len(f) >= 2 {
		if elem, v, ok := dotV(f[1]); ok {
			root = strings.Join(f[:2], "/")
			repo = "github.com/go-" + elem + "/" + elem
			major = v
			subdir = strings.Join(f[2:], "/")
			return root, repo, major, subdir, true
		}
	}
	if len(f) >= 3 {
		if elem, v, ok := dotV(f[2]); ok {
			root = strings.Join(f[:3], "/")
			repo = "github.com/" + f[1] + "/" + elem
			major = v
			subdir = strings.Join(f[3:], "/")
			return root, repo, major, subdir, true
		}
	}
	return
}

func dotV(name string) (elem, v string, ok bool) {
	i := len(name) - 1
	for i >= 0 && '0' <= name[i] && name[i] <= '9' {
		i--
	}
	if i <= 2 || i+1 >= len(name) || name[i-1] != '.' || name[i] != 'v' || name[i+1] == '0' && len(name) != i+2 {
		return "", "", false
	}
	return name[:i-1], name[i:], true
}

func gopkginLookup(path string) (codehost.Repo, error) {
	root, repo, major, subdir, ok := ParseGopkgIn(path)
	if !ok {
		return nil, fmt.Errorf("invalid gopkg.in/ path: %q", path)
	}
	gh, err := github.Lookup(repo)
	if err != nil {
		return nil, err
	}
	return &gopkgin{gh, root, repo, major, subdir}, nil
}

type gopkgin struct {
	gh     codehost.Repo
	root   string
	repo   string
	major  string
	subdir string
}

func (r *gopkgin) Root() string {
	return r.root
}

func (r *gopkgin) Tags(prefix string) ([]string, error) {
	p := r.major + "."
	list, err := r.gh.Tags(p)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, v := range list {
		if !strings.HasPrefix(v, p) || !semver.IsValid(v) {
			continue
		}
		out = append(out, "v1"+v[len(r.major):]+"-gopkgin-"+v)
	}
	return out, nil
}

func (r *gopkgin) Stat(rev string) (*codehost.RevInfo, error) {
	ghRev, err := r.unconvert(rev)
	if err != nil {
		return nil, err
	}
	return r.convert(r.gh.Stat(ghRev))
}

func (r *gopkgin) Latest() (*codehost.RevInfo, error) {
	if r.major == "v0" {
		return r.convert(r.gh.Stat("master"))
	}
	return r.convert(r.gh.Stat(r.major))
}

func (r *gopkgin) ReadFile(rev, file string, maxSize int64) ([]byte, error) {
	ghRev, err := r.unconvert(rev)
	if err != nil {
		return nil, err
	}
	return r.gh.ReadFile(ghRev, file, maxSize)
}

func (r *gopkgin) ReadZip(rev, subdir string, maxSize int64) (io.ReadCloser, string, error) {
	ghRev, err := r.unconvert(rev)
	if err != nil {
		return nil, "", err
	}
	return r.gh.ReadZip(ghRev, subdir, maxSize)
}

func (r *gopkgin) convert(info *codehost.RevInfo, err error) (*codehost.RevInfo, error) {
	if err != nil {
		return nil, err
	}
	v := info.Version
	if !semver.IsValid(v) {
		return info, nil
	}
	if !strings.HasPrefix(v, r.major+".") {
		info.Version = PseudoVersion("v0", info.Time, info.Short)
		return info, nil
	}
	info.Version = "v1" + v[len(r.major):] + "-gopkgin-" + v
	return info, nil
}

func (r *gopkgin) unconvert(rev string) (ghRev string, err error) {
	i := strings.Index(rev, "-gopkgin-")
	if i < 0 {
		return rev, nil
	}
	fake, real := rev[:i], rev[i+len("-gopkgin-"):]
	if strings.HasPrefix(real, r.major+".") && fake == "v1"+real[len(r.major):] {
		return real, nil
	}
	return "", fmt.Errorf("malformed gopkgin tag")
}
