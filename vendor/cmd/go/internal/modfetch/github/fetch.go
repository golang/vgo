// Copyright 2018 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package github

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
	if len(f) < 3 || f[0] != "github.com" {
		return nil, fmt.Errorf("github repo must be github.com/org/project")
	}

	// Check for moved, renamed, or incorrect case in repository.
	// Otherwise the API calls all appear to work.
	// We need to do better here, but it's unclear exactly what.
	var data struct {
		FullName string `json:"full_name"`
	}
	err := web.Get("https://api.github.com/repos/"+url.PathEscape(f[1])+"/"+url.PathEscape(f[2]), web.DecodeJSON(&data))
	if err != nil {
		return nil, err
	}
	myFullName := f[1] + "/" + f[2]
	if myFullName != data.FullName {
		why := "moved"
		if strings.EqualFold(myFullName, data.FullName) {
			why = "wrong case"
		}
		return nil, fmt.Errorf("module path of repo is github.com/%s, not %s (%s)", data.FullName, path, why)
	}

	return newRepo(f[1], f[2]), nil
}

func newRepo(owner, repository string) codehost.Repo {
	return &repo{owner, repository}
}

type repo struct {
	owner string
	repo  string
}

func (r *repo) Root() string {
	return "github.com/" + r.owner + "/" + r.repo
}

func (r *repo) Tags(prefix string) ([]string, error) {
	var tags []string
	u := "https://api.github.com/repos/" + url.PathEscape(r.owner) + "/" + url.PathEscape(r.repo) + "/tags"
	for u != "" {
		var data []struct {
			Name string
		}
		var hdr http.Header
		err := web.Get(u, web.Header(&hdr), web.DecodeJSON(&data))
		if err != nil {
			return nil, err
		}
		for _, t := range data {
			if strings.HasPrefix(t.Name, prefix) {
				tags = append(tags, t.Name)
			}
		}
		last := u
		u = ""
		for _, link := range parseLinks(hdr.Get("Link")) {
			if link.Rel == "next" && link.URL != last {
				u = link.URL
			}
		}
	}
	return tags, nil
}

func (r *repo) LatestAt(t time.Time, branch string) (*codehost.RevInfo, error) {
	var commits []struct {
		SHA    string
		Commit struct {
			Committer struct {
				Date string
			}
		}
	}
	err := web.Get(
		"https://api.github.com/repos/"+url.PathEscape(r.owner)+"/"+url.PathEscape(r.repo)+"/commits?sha="+url.QueryEscape(branch)+"&until="+url.QueryEscape(t.UTC().Format("2006-01-02T15:04:05Z"))+"&per_page=2",
		web.DecodeJSON(&commits),
	)
	if err != nil {
		return nil, err
	}
	if len(commits) == 0 {
		return nil, fmt.Errorf("no commits")
	}
	d, err := time.Parse("2006-01-02T15:04:05Z", commits[0].Commit.Committer.Date)
	if err != nil {
		return nil, err
	}

	info := &codehost.RevInfo{
		Name:  commits[0].SHA,
		Short: codehost.ShortenSHA1(commits[0].SHA),
		Time:  d,
	}
	return info, nil
}

var refKinds = []string{"tags", "heads"}

func (r *repo) Stat(rev string) (*codehost.RevInfo, error) {
	var tag string
	if !codehost.AllHex(rev) {
		// Resolve tag to rev
		tag = rev
		var ref struct {
			Ref    string
			Object struct {
				Type string
				SHA  string
				URL  string
			}
		}

		var firstErr error
		for _, kind := range refKinds {
			err := web.Get(
				"https://api.github.com/repos/"+url.PathEscape(r.owner)+"/"+url.PathEscape(r.repo)+"/git/refs/"+kind+"/"+tag,
				web.DecodeJSON(&ref),
			)
			if err != nil {
				if firstErr == nil {
					firstErr = err
				}
				continue
			}
			switch ref.Object.Type {
			default:
				return nil, fmt.Errorf("invalid ref %q: not a commit or tag (%q)", tag, ref.Object.Type)

			case "commit":
				rev = ref.Object.SHA

			case "tag":
				var info struct {
					Object struct {
						SHA  string
						Type string
					}
				}
				err = web.Get(
					ref.Object.URL,
					web.DecodeJSON(&info),
				)
				if err != nil {
					return nil, err
				}
				if info.Object.Type != "commit" {
					return nil, fmt.Errorf("invalid annotated tag %q: not a commit (%q)", tag, info.Object.Type)
				}
				rev = info.Object.SHA
			}
			if rev == "" {
				return nil, fmt.Errorf("invalid ref %q: missing SHA in GitHub response", tag)
			}
			break
		}
		if rev == "" {
			return nil, fmt.Errorf("unknown ref %q (%v)", tag, firstErr)
		}
	}

	var commits []struct {
		SHA    string
		Commit struct {
			Committer struct {
				Date string
			}
		}
	}
	err := web.Get(
		"https://api.github.com/repos/"+url.PathEscape(r.owner)+"/"+url.PathEscape(r.repo)+"/commits?sha="+url.QueryEscape(rev)+"&per_page=2",
		web.DecodeJSON(&commits),
	)
	if err != nil {
		return nil, err
	}
	if len(commits) == 0 {
		return nil, fmt.Errorf("no commits")
	}
	if !strings.HasPrefix(commits[0].SHA, rev) {
		return nil, fmt.Errorf("wrong rev returned by server")
	}
	d, err := time.Parse("2006-01-02T15:04:05Z", commits[0].Commit.Committer.Date)
	if err != nil {
		return nil, err
	}
	info := &codehost.RevInfo{
		Name:    commits[0].SHA,
		Short:   codehost.ShortenSHA1(commits[0].SHA),
		Version: tag,
		Time:    d,
	}
	return info, nil
}

func (r *repo) ReadFile(rev, file string, maxSize int64) ([]byte, error) {
	var meta struct {
		Type        string
		Size        int64
		Name        string
		DownloadURL string `json:"download_url"`
	}
	err := web.Get(
		"https://api.github.com/repos/"+url.PathEscape(r.owner)+"/"+url.PathEscape(r.repo)+"/contents/"+url.PathEscape(file)+"?ref="+url.QueryEscape(rev),
		web.DecodeJSON(&meta),
	)
	if err != nil {
		return nil, err
	}
	if meta.DownloadURL == "" {
		return nil, fmt.Errorf("no download URL")
	}

	// TODO: Use maxSize.
	var body []byte
	err = web.Get(meta.DownloadURL, web.ReadAllBody(&body))
	if err != nil {
		return nil, err
	}
	return body, nil
}

func (r *repo) ReadZip(rev, subdir string, maxSize int64) (zip io.ReadCloser, actualSubdir string, err error) {
	// TODO: Make web.Get copy to file for us, with limit.
	var body io.ReadCloser
	err = web.Get(
		"https://api.github.com/repos/"+url.PathEscape(r.owner)+"/"+url.PathEscape(r.repo)+"/zipball/"+url.PathEscape(rev),
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

type link struct {
	URL string
	Rel string
}

func parseLinks(text string) []link {
	var links []link
	for text != "" {
		text = strings.TrimSpace(text)
		if !strings.HasPrefix(text, "<") {
			break
		}
		i := strings.Index(text, ">")
		if i < 0 {
			break
		}
		u := text[1:i]
		text = strings.TrimSpace(text[i+1:])
	Attrs:
		for strings.HasPrefix(text, ";") {
			text = text[1:]
			j := 0
			for j < len(text) && text[j] != '=' {
				if text[j] == ',' || text[j] == ';' || text[j] == '"' {
					break Attrs
				}
				j++
			}
			if j >= len(text) {
				break Attrs
			}
			key := strings.TrimSpace(text[:j])
			text = strings.TrimSpace(text[j+1:])
			var val string
			if len(text) > 0 && text[0] == '"' {
				k := 1
				for k < len(text) && text[k] != '"' {
					if text[k] == '\\' && k+1 < len(text) {
						k++
					}
					k++
				}
				if k >= len(text) {
					break Attrs
				}
				val, text = text[1:k], text[k+1:]
			} else {
				k := 0
				for k < len(text) && text[k] != ';' && text[k] != ',' {
					if text[k] == '"' {
						break Attrs
					}
					k++
				}
				if k >= len(text) {
					break Attrs
				}
				val, text = text[:k], text[k:]
			}
			if key == "rel" {
				links = append(links, link{URL: u, Rel: strings.TrimSpace(val)})
			}
		}
		text = strings.TrimSpace(text)
		if !strings.HasPrefix(text, ",") {
			break
		}
		text = text[1:]
	}
	return links
}
