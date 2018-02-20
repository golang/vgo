// Copyright 2018 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package googlesource

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"cmd/go/internal/modfetch/codehost"
	web "cmd/go/internal/web2"
)

func Lookup(path string) (codehost.Repo, error) {
	i := strings.Index(path, "/")
	if i+1 == len(path) || !strings.HasSuffix(path[:i+1], ".googlesource.com/") {
		return nil, fmt.Errorf("not *.googlesource.com/*")
	}
	j := strings.Index(path[i+1:], "/")
	if j >= 0 {
		path = path[:i+1+j]
	}
	r := &repo{
		root: path,
		base: "https://" + path,
	}
	return r, nil
}

type repo struct {
	base string
	root string
}

func (r *repo) Root() string {
	return r.root
}

func (r *repo) Tags(prefix string) ([]string, error) {
	var data []byte
	err := web.Get(r.base+"/+refs/tags/?format=TEXT", web.ReadAllBody(&data))
	if err != nil {
		return nil, err
	}
	prefix = "refs/tags/" + prefix
	var tags []string
	for _, line := range strings.Split(string(data), "\n") {
		f := strings.Fields(line)
		if len(f) == 2 && len(f[0]) == 40 && strings.HasPrefix(f[1], prefix) {
			tags = append(tags, strings.TrimPrefix(f[1], "refs/tags/"))
		}
	}
	return tags, nil
}

func (r *repo) LatestAt(limit time.Time, branch string) (*codehost.RevInfo, error) {
	u := r.base + "/+log/" + url.PathEscape(branch) + "?format=JSON&n=2"
	var n int
	for u != "" {
		var body io.ReadCloser
		err := web.Get(u, web.Body(&body))
		if err != nil {
			return nil, err
		}
		b := make([]byte, 1)
		for {
			_, err := body.Read(b)
			if err != nil {
				body.Close()
				return nil, err
			}
			if b[0] == '\n' {
				break
			}
		}
		var data struct {
			Log []struct {
				Commit    string
				Committer struct {
					Time string
				}
			}
			Next string
		}
		err = json.NewDecoder(body).Decode(&data)
		body.Close()
		if err != nil {
			return nil, err
		}
		for i := range data.Log {
			t, err := time.Parse("Mon Jan 02 15:04:05 2006 -0700", data.Log[i].Committer.Time)
			if err != nil {
				return nil, err
			}
			if !t.After(limit) {
				info := &codehost.RevInfo{
					Time:  t.UTC(),
					Name:  data.Log[i].Commit,
					Short: codehost.ShortenSHA1(data.Log[i].Commit),
				}
				return info, nil
			}
		}
		u = ""
		if data.Next != "" {
			if n == 0 {
				n = 10
			} else if n < 1000 {
				n *= 2
			}
			u = r.base + "/+log/" + url.PathEscape(data.Next) + "?format=JSON&n=" + fmt.Sprint(n)
		}
	}
	return nil, fmt.Errorf("no commits")
}

func (r *repo) Stat(rev string) (*codehost.RevInfo, error) {
	if !codehost.AllHex(rev) || len(rev) != 40 {
		return r.LatestAt(time.Date(9999, 1, 1, 0, 0, 0, 0, time.UTC), rev)
	}

	var body io.ReadCloser
	u := r.base + "/+show/" + url.PathEscape(rev) + "?format=TEXT"
	if err := web.Get(u, web.Body(&body)); err != nil {
		return nil, err
	}
	defer body.Close()
	b := bufio.NewReader(base64.NewDecoder(base64.StdEncoding, body))
	for {
		line, err := b.ReadSlice('\n')
		if err != nil {
			return nil, err
		}
		s := string(line)
		if s == "\n" {
			return nil, fmt.Errorf("malformed commit: no committer")
		}
		if strings.HasPrefix(s, "committer ") {
			f := strings.Fields(s)
			if len(f) >= 3 {
				v, err := strconv.ParseUint(f[len(f)-2], 10, 64)
				if err == nil {
					info := &codehost.RevInfo{
						Time:  time.Unix(int64(v), 0).UTC(),
						Name:  rev,
						Short: codehost.ShortenSHA1(rev),
					}
					return info, nil
				}
			}
		}
	}
}

func (r *repo) ReadFile(rev, file string, maxSize int64) ([]byte, error) {
	u := r.base + "/+show/" + url.PathEscape(rev) + "/" + file + "?format=TEXT"
	var body io.ReadCloser
	if err := web.Get(u, web.Body(&body)); err != nil {
		return nil, err
	}
	defer body.Close()
	lr := &io.LimitedReader{R: base64.NewDecoder(base64.StdEncoding, body), N: maxSize + 1}
	data, err := ioutil.ReadAll(lr)
	if lr.N <= 0 {
		return data, fmt.Errorf("too long")
	}
	return data, err
}

type closeRemover struct {
	*os.File
}

func (c *closeRemover) Close() error {
	c.File.Close()
	os.Remove(c.File.Name())
	return nil
}

func (r *repo) ReadZip(rev, subdir string, maxSize int64) (zipstream io.ReadCloser, actualSubdir string, err error) {
	// Start download of tgz for subdir.
	if subdir != "" {
		subdir = "/" + strings.TrimSuffix(subdir, "/")
	}
	u := r.base + "/+archive/" + url.PathEscape(rev) + subdir + ".tar.gz"
	var body io.ReadCloser
	if err := web.Get(u, web.Body(&body)); err != nil {
		return nil, "", err
	}
	defer body.Close()
	gz, err := gzip.NewReader(body)
	if err != nil {
		return nil, "", err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)

	// Start temporary zip file.
	f, err := ioutil.TempFile("", "vgo-")
	if err != nil {
		return nil, "", err
	}
	defer func() {
		f.Close()
		if err != nil {
			os.Remove(f.Name())
		}
	}()
	z := zip.NewWriter(f)

	// Copy files from tgz to zip file.
	prefix := "googlesource/"
	haveLICENSE := false
	for {
		hdr, err := tr.Next()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, "", fmt.Errorf("reading tgz from gitiles: %v", err)
		}
		maxSize -= 512
		switch hdr.Typeflag {
		case tar.TypeDir:
			// ok
		case tar.TypeReg:
			if maxSize < hdr.Size {
				return nil, "", fmt.Errorf("module source tree too big")
			}
			maxSize -= hdr.Size
			if hdr.Name == "LICENSE" {
				haveLICENSE = true
			}
			fw, err := z.Create(prefix + hdr.Name)
			if err != nil {
				return nil, "", err
			}
			if _, err := io.Copy(fw, tr); err != nil {
				return nil, "", err
			}
		}
	}

	// Add LICENSE from parent directory if needed.
	if !haveLICENSE && subdir != "" {
		if data, err := r.ReadFile(rev, "LICENSE", codehost.MaxLICENSE); err == nil {
			fw, err := z.Create(prefix + "LICENSE")
			if err != nil {
				return nil, "", err
			}
			if _, err := fw.Write(data); err != nil {
				return nil, "", err
			}
		}
	}

	// Finish.
	if err := z.Close(); err != nil {
		return nil, "", err
	}
	if err := f.Close(); err != nil {
		return nil, "", err
	}

	fr, err := os.Open(f.Name())
	if err != nil {
		return nil, "", err
	}
	return &closeRemover{fr}, subdir, nil
}
