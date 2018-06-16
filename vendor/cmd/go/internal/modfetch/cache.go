// Copyright 2018 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package modfetch

import (
	"sync"

	"cmd/go/internal/par"
)

// A cachingRepo is a cache around an underlying Repo,
// avoiding redundant calls to ModulePath, Versions, Stat, Latest, and GoMod (but not Zip).
// It is also safe for simultaneous use by multiple goroutines
// (so that it can be returned from Lookup multiple times).
// It serializes calls to the underlying Repo.
type cachingRepo struct {
	path  string
	cache par.Cache // cache for all operations

	mu sync.Mutex // protects r's methods
	r  Repo
}

func newCachingRepo(r Repo) *cachingRepo {
	return &cachingRepo{
		r:    r,
		path: r.ModulePath(),
	}
}

func (r *cachingRepo) ModulePath() string {
	return r.path
}

func (r *cachingRepo) Versions(prefix string) ([]string, error) {
	type cached struct {
		list []string
		err  error
	}
	c := r.cache.Do("versions:"+prefix, func() interface{} {
		r.mu.Lock()
		defer r.mu.Unlock()
		list, err := r.r.Versions(prefix)
		return cached{list, err}
	}).(cached)

	if c.err != nil {
		return nil, c.err
	}
	return append([]string(nil), c.list...), nil
}

func (r *cachingRepo) Stat(rev string) (*RevInfo, error) {
	type cached struct {
		info *RevInfo
		err  error
	}
	c := r.cache.Do("stat:"+rev, func() interface{} {
		r.mu.Lock()
		defer r.mu.Unlock()
		info, err := r.r.Stat(rev)
		return cached{info, err}
	}).(cached)

	if c.err != nil {
		return nil, c.err
	}
	info := *c.info
	return &info, nil
}

func (r *cachingRepo) Latest() (*RevInfo, error) {
	type cached struct {
		info *RevInfo
		err  error
	}
	c := r.cache.Do("latest:", func() interface{} {
		r.mu.Lock()
		defer r.mu.Unlock()
		info, err := r.r.Latest()
		return cached{info, err}
	}).(cached)

	if c.err != nil {
		return nil, c.err
	}
	info := *c.info
	return &info, nil
}

func (r *cachingRepo) GoMod(rev string) ([]byte, error) {
	type cached struct {
		text []byte
		err  error
	}
	c := r.cache.Do("gomod:"+rev, func() interface{} {
		r.mu.Lock()
		defer r.mu.Unlock()
		text, err := r.r.GoMod(rev)
		return cached{text, err}
	}).(cached)

	if c.err != nil {
		return nil, c.err
	}
	return append([]byte(nil), c.text...), nil
}

func (r *cachingRepo) Zip(version, tmpdir string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.r.Zip(version, tmpdir)
}
