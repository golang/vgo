// Copyright 2018 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package modload

import (
	"internal/testenv"
	"strings"
	"testing"
)

var importTests = []struct {
	path  string
	mpath string
	err   string
}{
	{
		path:  "golang.org/x/net/context",
		mpath: "golang.org/x/net",
	},
	{
		path:  "github.com/rsc/quote/buggy",
		mpath: "github.com/rsc/quote",
	},
	{
		path:  "golang.org/x/net",
		mpath: "golang.org/x/net",
	},
	{
		path:  "github.com/rsc/quote",
		mpath: "github.com/rsc/quote",
	},
	{
		path: "golang.org/x/foo/bar",
		// TODO(rsc): This error comes from old go get and is terrible. Fix.
		err: `unrecognized import path "golang.org/x/foo/bar" (parse https://golang.org/x/foo/bar?go-get=1: no go-import meta tags ())`,
	},
}

func TestImport(t *testing.T) {
	testenv.MustHaveExternalNetwork(t)

	for _, tt := range importTests {
		t.Run(strings.Replace(tt.path, "/", "_", -1), func(t *testing.T) {
			repo, info, err := Import(tt.path, nil)
			if tt.err != "" {
				if err != nil && err.Error() == tt.err {
					return
				}
				t.Fatalf("Import(%q): %v, want error %q", tt.path, err, tt.err)
			}
			if err != nil {
				t.Fatalf("Import(%q): %v", tt.path, err)
			}
			if mpath := repo.ModulePath(); mpath != tt.mpath {
				t.Errorf("repo.ModulePath() = %q (%v), want %q", mpath, info.Version, tt.mpath)
			}
		})
	}
}
