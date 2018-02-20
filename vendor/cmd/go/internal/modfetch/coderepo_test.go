// Copyright 2018 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package modfetch

import (
	"archive/zip"
	"cmd/go/internal/webtest"
	"io/ioutil"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

func init() {
	isTest = true
}

var codeRepoTests = []struct {
	path     string
	rev      string
	err      string
	version  string
	name     string
	short    string
	time     time.Time
	gomod    string
	gomoderr string
	zip      []string
	ziperr   string
}{
	{
		path:    "github.com/rsc/vgotest1",
		rev:     "v0.0.0",
		version: "v0.0.0",
		name:    "80d85c5d4d17598a0e9055e7c175a32b415d6128",
		short:   "80d85c5d4d17",
		time:    time.Date(2018, 2, 19, 23, 10, 6, 0, time.UTC),
		zip: []string{
			"LICENSE",
			"README.md",
			"pkg/p.go",
		},
	},
	{
		path:    "github.com/rsc/vgotest1",
		rev:     "v1.0.0",
		version: "v1.0.0",
		name:    "80d85c5d4d17598a0e9055e7c175a32b415d6128",
		short:   "80d85c5d4d17",
		time:    time.Date(2018, 2, 19, 23, 10, 6, 0, time.UTC),
		zip: []string{
			"LICENSE",
			"README.md",
			"pkg/p.go",
		},
	},
	{
		path:    "github.com/rsc/vgotest1/v2",
		rev:     "v2.0.0",
		version: "v2.0.0",
		name:    "80d85c5d4d17598a0e9055e7c175a32b415d6128",
		short:   "80d85c5d4d17",
		time:    time.Date(2018, 2, 19, 23, 10, 6, 0, time.UTC),
		ziperr:  "missing go.mod",
	},
	{
		path:    "github.com/rsc/vgotest1",
		rev:     "80d85",
		version: "v0.0.0-20180219231006-80d85c5d4d17",
		name:    "80d85c5d4d17598a0e9055e7c175a32b415d6128",
		short:   "80d85c5d4d17",
		time:    time.Date(2018, 2, 19, 23, 10, 6, 0, time.UTC),
		zip: []string{
			"LICENSE",
			"README.md",
			"pkg/p.go",
		},
	},
	{
		path:    "github.com/rsc/vgotest1",
		rev:     "mytag",
		version: "v0.0.0-20180219231006-80d85c5d4d17",
		name:    "80d85c5d4d17598a0e9055e7c175a32b415d6128",
		short:   "80d85c5d4d17",
		time:    time.Date(2018, 2, 19, 23, 10, 6, 0, time.UTC),
		zip: []string{
			"LICENSE",
			"README.md",
			"pkg/p.go",
		},
	},
	{
		path:     "github.com/rsc/vgotest1/v2",
		rev:      "80d85",
		version:  "v2.0.0-20180219231006-80d85c5d4d17",
		name:     "80d85c5d4d17598a0e9055e7c175a32b415d6128",
		short:    "80d85c5d4d17",
		time:     time.Date(2018, 2, 19, 23, 10, 6, 0, time.UTC),
		gomoderr: "missing go.mod",
		ziperr:   "missing go.mod",
	},
	{
		path:    "github.com/rsc/vgotest1/v54321",
		rev:     "80d85",
		version: "v54321.0.0-20180219231006-80d85c5d4d17",
		name:    "80d85c5d4d17598a0e9055e7c175a32b415d6128",
		short:   "80d85c5d4d17",
		time:    time.Date(2018, 2, 19, 23, 10, 6, 0, time.UTC),
		ziperr:  "missing go.mod",
	},
	{
		path: "github.com/rsc/vgotest1/submod",
		rev:  "v1.0.0",
		err:  "404 Not Found", // TODO
	},
	{
		path: "github.com/rsc/vgotest1/submod",
		rev:  "v1.0.3",
		err:  "404 Not Found", // TODO
	},
	{
		path:    "github.com/rsc/vgotest1/submod",
		rev:     "v1.0.4",
		version: "v1.0.4",
		name:    "8afe2b2efed96e0880ecd2a69b98a53b8c2738b6",
		short:   "8afe2b2efed9",
		time:    time.Date(2018, 2, 19, 23, 12, 7, 0, time.UTC),
		gomod:   "module \"github.com/vgotest1/submod\" // submod/go.mod\n",
		zip: []string{
			"go.mod",
			"pkg/p.go",
			"LICENSE",
		},
	},
	{
		path:    "github.com/rsc/vgotest1",
		rev:     "v1.1.0",
		version: "v1.1.0",
		name:    "b769f2de407a4db81af9c5de0a06016d60d2ea09",
		short:   "b769f2de407a",
		time:    time.Date(2018, 2, 19, 23, 13, 36, 0, time.UTC),
		gomod:   "module \"github.com/rsc/vgotest1\" // root go.mod\nrequire \"github.com/rsc/vgotest1/submod\" v1.0.5\n",
		zip: []string{
			"LICENSE",
			"README.md",
			"go.mod",
			"pkg/p.go",
		},
	},
	{
		path:    "github.com/rsc/vgotest1/v2",
		rev:     "v2.0.1",
		version: "v2.0.1",
		name:    "ea65f87c8f52c15ea68f3bdd9925ef17e20d91e9",
		short:   "ea65f87c8f52",
		time:    time.Date(2018, 2, 19, 23, 14, 23, 0, time.UTC),
		gomod:   "module \"github.com/rsc/vgotest1/v2\" // root go.mod\n",
	},
	{
		path:     "github.com/rsc/vgotest1/v2",
		rev:      "v2.0.3",
		version:  "v2.0.3",
		name:     "f18795870fb14388a21ef3ebc1d75911c8694f31",
		short:    "f18795870fb1",
		time:     time.Date(2018, 2, 19, 23, 16, 4, 0, time.UTC),
		gomoderr: "v2/go.mod has non-.../v2 module path",
	},
	{
		path:     "github.com/rsc/vgotest1/v2",
		rev:      "v2.0.4",
		version:  "v2.0.4",
		name:     "1f863feb76bc7029b78b21c5375644838962f88d",
		short:    "1f863feb76bc",
		time:     time.Date(2018, 2, 20, 0, 3, 38, 0, time.UTC),
		gomoderr: "both go.mod and v2/go.mod claim .../v2 module",
	},
	{
		path:    "github.com/rsc/vgotest1/v2",
		rev:     "v2.0.5",
		version: "v2.0.5",
		name:    "2f615117ce481c8efef46e0cc0b4b4dccfac8fea",
		short:   "2f615117ce48",
		time:    time.Date(2018, 2, 20, 0, 3, 59, 0, time.UTC),
		gomod:   "module \"github.com/rsc/vgotest1/v2\" // v2/go.mod\n",
	},
	{
		path:    "go.googlesource.com/scratch",
		rev:     "0f302529858",
		version: "v0.0.0-20180220024720-0f3025298580",
		name:    "0f30252985809011f026b5a2d5cf456e021623da",
		short:   "0f3025298580",
		time:    time.Date(2018, 2, 20, 2, 47, 20, 0, time.UTC),
		gomod:   "//vgo 0.0.3\n\nmodule \"go.googlesource.com/scratch\"\n",
	},
	{
		path:    "go.googlesource.com/scratch/rsc",
		rev:     "0f302529858",
		version: "v0.0.0-20180220024720-0f3025298580",
		name:    "0f30252985809011f026b5a2d5cf456e021623da",
		short:   "0f3025298580",
		time:    time.Date(2018, 2, 20, 2, 47, 20, 0, time.UTC),
		gomod:   "",
	},
	{
		path:     "go.googlesource.com/scratch/cbro",
		rev:      "0f302529858",
		version:  "v0.0.0-20180220024720-0f3025298580",
		name:     "0f30252985809011f026b5a2d5cf456e021623da",
		short:    "0f3025298580",
		time:     time.Date(2018, 2, 20, 2, 47, 20, 0, time.UTC),
		gomoderr: "missing go.mod",
	},
	{
		// redirect to github
		path:    "rsc.io/quote",
		rev:     "v1.0.0",
		version: "v1.0.0",
		name:    "f488df80bcdbd3e5bafdc24ad7d1e79e83edd7e6",
		short:   "f488df80bcdb",
		time:    time.Date(2018, 2, 14, 0, 45, 20, 0, time.UTC),
		gomod:   "module \"rsc.io/quote\"\n",
	},
	{
		// redirect to static hosting proxy
		path:    "swtch.com/testmod",
		rev:     "v1.0.0",
		version: "v1.0.0",
		name:    "v1.0.0",
		short:   "v1.0.0",
		time:    time.Date(1972, 7, 18, 12, 34, 56, 0, time.UTC),
		gomod:   "module \"swtch.com/testmod\"\n",
	},
	{
		// redirect to googlesource
		path:    "golang.org/x/text",
		rev:     "4e4a3210bb",
		version: "v0.0.0-20180208041248-4e4a3210bb54",
		name:    "4e4a3210bb54bb31f6ab2cdca2edcc0b50c420c1",
		short:   "4e4a3210bb54",
		time:    time.Date(2018, 2, 8, 4, 12, 48, 0, time.UTC),
	},
	{
		path:    "github.com/pkg/errors",
		rev:     "v0.8.0",
		version: "v0.8.0",
		name:    "645ef00459ed84a119197bfb8d8205042c6df63d",
		short:   "645ef00459ed",
		time:    time.Date(2016, 9, 29, 1, 48, 1, 0, time.UTC),
	},
}

func TestCodeRepo(t *testing.T) {
	webtest.LoadOnce("testdata/webtest.txt")
	webtest.Hook()
	defer webtest.Unhook()

	tmpdir, err := ioutil.TempDir("", "vgo-modfetch-test-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpdir)
	for _, tt := range codeRepoTests {
		t.Run(strings.Replace(tt.path, "/", "_", -1)+"/"+tt.rev, func(t *testing.T) {
			repo, err := Lookup(tt.path)
			if err != nil {
				t.Fatalf("Lookup(%q): %v", tt.path, err)
			}
			if mpath := repo.ModulePath(); mpath != tt.path {
				t.Errorf("repo.ModulePath() = %q, want %q", mpath, tt.path)
			}
			info, err := repo.Stat(tt.rev)
			if err != nil {
				if tt.err != "" {
					if !strings.Contains(err.Error(), tt.err) {
						t.Fatalf("repoStat(%q): %v, wanted %q", tt.rev, err, tt.err)
					}
					return
				}
				t.Fatalf("repo.Stat(%q): %v", tt.rev, err)
			}
			if tt.err != "" {
				t.Errorf("repo.Stat(%q): success, wanted error", tt.rev)
			}
			if info.Version != tt.version {
				t.Errorf("info.Version = %q, want %q", info.Version, tt.version)
			}
			if info.Name != tt.name {
				t.Errorf("info.Name = %q, want %q", info.Name, tt.name)
			}
			if info.Short != tt.short {
				t.Errorf("info.Short = %q, want %q", info.Short, tt.short)
			}
			if info.Time != tt.time {
				t.Errorf("info.Time = %v, want %v", info.Time, tt.time)
			}
			if tt.gomod != "" || tt.gomoderr != "" {
				data, err := repo.GoMod(tt.version)
				if err != nil && tt.gomoderr == "" {
					t.Errorf("repo.GoMod(%q): %v", tt.version, err)
				} else if err != nil && tt.gomoderr != "" {
					if err.Error() != tt.gomoderr {
						t.Errorf("repo.GoMod(%q): %v, want %q", tt.version, err, tt.gomoderr)
					}
				} else if tt.gomoderr != "" {
					t.Errorf("repo.GoMod(%q) = %q, want error %q", tt.version, data, tt.gomoderr)
				} else if string(data) != tt.gomod {
					t.Errorf("repo.GoMod(%q) = %q, want %q", tt.version, data, tt.gomod)
				}
			}
			if tt.zip != nil || tt.ziperr != "" {
				zipfile, err := repo.Zip(tt.version, tmpdir)
				if err != nil {
					if tt.ziperr != "" {
						if err.Error() == tt.ziperr {
							return
						}
						t.Fatalf("repo.Zip(%q): %v, want error %q", tt.version, err, tt.ziperr)
					}
					t.Fatalf("repo.Zip(%q): %v", tt.version, err)
				}
				if tt.ziperr != "" {
					t.Errorf("repo.Zip(%q): success, want error %q", tt.version, tt.ziperr)
				}
				prefix := tt.path + "@" + tt.version + "/"
				z, err := zip.OpenReader(zipfile)
				if err != nil {
					t.Fatalf("open zip %s: %v", zipfile, err)
				}
				var names []string
				for _, file := range z.File {
					if !strings.HasPrefix(file.Name, prefix) {
						t.Errorf("zip entry %v does not start with prefix %v", file.Name, prefix)
						continue
					}
					names = append(names, file.Name[len(prefix):])
				}
				z.Close()
				if !reflect.DeepEqual(names, tt.zip) {
					t.Fatalf("zip = %v\nwant %v\n", names, tt.zip)
				}
			}
		})
	}
}

var codeRepoVersionsTests = []struct {
	path     string
	prefix   string
	versions []string
}{
	// TODO: Why do we allow a prefix here at all?
	{
		path:     "github.com/rsc/vgotest1",
		versions: []string{"v0.0.0", "v0.0.1", "v1.0.0", "v1.0.1", "v1.0.2", "v1.0.3", "v1.1.0"},
	},
	{
		path:     "github.com/rsc/vgotest1",
		prefix:   "v1.0",
		versions: []string{"v1.0.0", "v1.0.1", "v1.0.2", "v1.0.3"},
	},
	{
		path:     "github.com/rsc/vgotest1/v2",
		versions: []string{"v2.0.0", "v2.0.1", "v2.0.2", "v2.0.3", "v2.0.4", "v2.0.5", "v2.0.6"},
	},
	{
		path:     "swtch.com/testmod",
		versions: []string{"v1.0.0", "v1.1.1"},
	},
}

func TestCodeRepoVersions(t *testing.T) {
	webtest.LoadOnce("testdata/webtest.txt")
	webtest.Hook()
	defer webtest.Unhook()

	tmpdir, err := ioutil.TempDir("", "vgo-modfetch-test-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpdir)
	for _, tt := range codeRepoVersionsTests {
		t.Run(strings.Replace(tt.path, "/", "_", -1), func(t *testing.T) {
			repo, err := Lookup(tt.path)
			if err != nil {
				t.Fatalf("Lookup(%q): %v", tt.path, err)
			}
			list, err := repo.Versions(tt.prefix)
			if err != nil {
				t.Fatalf("Versions(%q): %v", tt.prefix, err)
			}
			if !reflect.DeepEqual(list, tt.versions) {
				t.Fatalf("Versions(%q):\nhave %v\nwant %v", tt.prefix, list, tt.versions)
			}
		})
	}
}

var latestAtTests = []struct {
	path    string
	time    time.Time
	branch  string
	version string
	err     string
}{
	{
		path: "github.com/rsc/vgotest1",
		time: time.Date(2018, 1, 20, 0, 0, 0, 0, time.UTC),
		err:  "no commits",
	},
	{
		path:    "github.com/rsc/vgotest1",
		time:    time.Date(2018, 2, 20, 0, 0, 0, 0, time.UTC),
		version: "v0.0.0-20180219223237-a08abb797a67",
	},
	{
		path:    "github.com/rsc/vgotest1",
		time:    time.Date(2018, 2, 20, 0, 0, 0, 0, time.UTC),
		branch:  "mybranch",
		version: "v0.0.0-20180219231006-80d85c5d4d17",
	},
	{
		path:    "swtch.com/testmod",
		time:    time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC),
		version: "v1.0.0",
	},
	{
		path:    "swtch.com/testmod",
		time:    time.Date(3000, 1, 1, 0, 0, 0, 0, time.UTC),
		version: "v1.1.1",
	},
	{
		path:   "swtch.com/testmod",
		time:   time.Date(3000, 1, 1, 0, 0, 0, 0, time.UTC),
		branch: "branch",
		err:    "latest on branch not supported",
	},
}

func TestLatestAt(t *testing.T) {
	webtest.LoadOnce("testdata/webtest.txt")
	webtest.Hook()
	defer webtest.Unhook()

	tmpdir, err := ioutil.TempDir("", "vgo-modfetch-test-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpdir)
	for _, tt := range latestAtTests {
		name := strings.Replace(tt.path, "/", "_", -1) + "/" + tt.time.Format("2006-01-02_15:04:05")
		if tt.branch != "" {
			name += "/" + tt.branch
		}
		t.Run(name, func(t *testing.T) {
			repo, err := Lookup(tt.path)
			if err != nil {
				t.Fatalf("Lookup(%q): %v", tt.path, err)
			}
			info, err := repo.LatestAt(tt.time, tt.branch)
			if err != nil {
				if tt.err != "" {
					if err.Error() == tt.err {
						return
					}
					t.Fatalf("LatestAt(%v, %q): %v, want %q", tt.time, tt.branch, err, tt.err)
				}
				t.Fatalf("LatestAt(%v, %q): %v", tt.time, tt.branch, err)
			}
			if info.Version != tt.version {
				t.Fatalf("LatestAt(%v, %q) = %v, want %v", tt.time, tt.branch, info.Version, tt.version)
			}
		})
	}
}
