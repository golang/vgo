// Copyright 2018 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bitbucket

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"cmd/go/internal/modfetch/codehost"
	web "cmd/go/internal/web2"
)

func Lookup(path string) (codehost.Repo, error) {
	f := strings.Split(path, "/")
	if len(f) < 3 || f[0] != "bitbucket.org" {
		return nil, fmt.Errorf("bitbucket repo must be bitbucket.org/org/project")
	}

	// Check for moved, renamed, or incorrect case in repository.
	// Otherwise the API calls all appear to work.
	// We need to do better here, but it's unclear exactly what.
	var data struct {
		FullName string `json:"full_name"`
	}
	err := web.Get("https://api.bitbucket.org/2.0/repositories/"+url.PathEscape(f[1])+"/"+url.PathEscape(f[2]), web.DecodeJSON(&data))
	if err != nil {
		return nil, err
	}
	myFullName := f[1] + "/" + f[2]
	if myFullName != data.FullName {
		why := "moved"
		if strings.EqualFold(myFullName, data.FullName) {
			why = "wrong case"
		}
		return nil, fmt.Errorf("module path of repo is bitbucket.org/%s, not %s (%s)", data.FullName, path, why)
	}

	return newRepo(f[1], f[2]), nil
}

func newRepo(owner, repository string) codehost.Repo {
	return &repo{owner: owner, repo: repository}
}

type repo struct {
	owner string
	repo  string
}

func (r *repo) Root() string {
	return "bitbucket.org/" + r.owner + "/" + r.repo
}

func (r *repo) Tags(prefix string) ([]string, error) {
	var tags []string
	u := "https://api.bitbucket.org/2.0/repositories/" + url.PathEscape(r.owner) + "/" + url.PathEscape(r.repo) + "/refs/tags"
	var data struct {
		Values []struct {
			Name string `json:"name"`
		} `json:"values"`
	}
	var hdr http.Header
	err := web.Get(u, web.Header(&hdr), web.DecodeJSON(&data))
	if err != nil {
		return nil, err
	}
	for _, t := range data.Values {
		if strings.HasPrefix(t.Name, prefix) {
			tags = append(tags, t.Name)
		}
	}
	return tags, nil
}

func (r *repo) LatestAt(t time.Time, branch string) (*codehost.RevInfo, error) {
	u := "https://api.bitbucket.org/2.0/repositories/" + url.PathEscape(r.owner) + "/" + url.PathEscape(r.repo) + "/commits/" + url.QueryEscape(branch) + "?pagelen=10"
	for u != "" {
		var commits struct {
			Values []struct {
				Hash string `json:"hash"`
				Date string `json:"date"`
			} `json:"values"`
			Next string `json:"next"`
		}
		err := web.Get(u, web.DecodeJSON(&commits))
		if err != nil {
			return nil, err
		}
		if len(commits.Values) == 0 {
			return nil, fmt.Errorf("no commits")
		}
		for _, commit := range commits.Values {
			d, err := time.Parse(time.RFC3339, commit.Date)
			if err != nil {
				return nil, err
			}
			if d.Before(t) {
				info := &codehost.RevInfo{
					Name:  commit.Hash,
					Short: codehost.ShortenSHA1(commit.Hash),
					Time:  d,
				}
				return info, nil
			}
		}
		u = commits.Next
	}
	return nil, fmt.Errorf("no commits")
}

func (r *repo) Stat(rev string) (*codehost.RevInfo, error) {
	var tag string
	if !codehost.AllHex(rev) {
		tag = rev
	}
	var commit struct {
		Hash string `json:"hash"`
		Type string `json:"type"`
		Date string `json:"date"`
	}
	err := web.Get(
		"https://api.bitbucket.org/2.0/repositories/"+url.PathEscape(r.owner)+"/"+url.PathEscape(r.repo)+"/commit/"+rev,
		web.DecodeJSON(&commit),
	)
	if err != nil {
		return nil, err
	}
	rev = commit.Hash
	if rev == "" {
		return nil, fmt.Errorf("no commits")
	}
	d, err := time.Parse(time.RFC3339, commit.Date)
	if err != nil {
		return nil, err
	}
	info := &codehost.RevInfo{
		Name:    rev,
		Short:   codehost.ShortenSHA1(rev),
		Version: tag,
		Time:    d,
	}
	return info, nil
}

func (r *repo) ReadFile(rev, file string, maxSize int64) ([]byte, error) {
	// TODO: Use maxSize.
	// TODO: I could not find an API endpoint for getting information about an
	// individual file, and I do not know if the raw file download endpoint is
	// a stable API.
	var body []byte
	err := web.Get(
		"https://bitbucket.org/"+url.PathEscape(r.owner)+"/"+url.PathEscape(r.repo)+"/raw/"+url.PathEscape(rev)+"/"+url.PathEscape(file),
		web.ReadAllBody(&body),
	)
	return body, err
}

func (r *repo) ReadZip(rev, subdir string, maxSize int64) (zip io.ReadCloser, actualSubdir string, err error) {
	// TODO: Make web.Get copy to file for us, with limit.
	var body io.ReadCloser
	err = web.Get(
		"https://bitbucket.org/"+url.PathEscape(r.owner)+"/"+url.PathEscape(r.repo)+"/get/"+url.PathEscape(rev)+".zip",
		web.Body(&body),
	)
	if err != nil {
		if body != nil {
			body.Close()
		}
		return nil, "", err
	}
	return body, "", nil
}
