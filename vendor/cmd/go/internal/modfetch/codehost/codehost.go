// Copyright 2018 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package codehost

import (
	"io"
	"time"
)

// Downloaded size limits.
const (
	MaxGoMod   = 16 << 20
	MaxLICENSE = 16 << 20
	MaxZipFile = 100 << 20
)

// A Repo represents a source code repository on a code-hosting service.
type Repo interface {
	// Root returns the import path of the root directory of the repository.
	Root() string

	// List lists all tags with the given prefix.
	Tags(prefix string) (tags []string, err error)

	// Stat returns information about the revision rev.
	// A revision can be any identifier known to the underlying service:
	// commit hash, branch, tag, and so on.
	Stat(rev string) (*RevInfo, error)

	// LatestAt returns the latest revision at the given time.
	// If branch is non-empty, it restricts the query to revisions
	// on the named branch. The meaning of "branch" depends
	// on the underlying implementation.
	LatestAt(t time.Time, branch string) (*RevInfo, error)

	// ReadFile reads the given file in the file tree corresponding to revision rev.
	// It should refuse to read more than maxSize bytes.
	ReadFile(rev, file string, maxSize int64) (data []byte, err error)

	// ReadZip downloads a zip file for the subdir subdirectory
	// of the given revision to a new file in a given temporary directory.
	// It should refuse to read more than maxSize bytes.
	// It returns a ReadCloser for a streamed copy of the zip file,
	// along with the actual subdirectory (possibly shorter than subdir)
	// contained in the zip file. All files in the zip file are expected to be
	// nested in a single top-level directory, whose name is not specified.
	ReadZip(rev, subdir string, maxSize int64) (zip io.ReadCloser, actualSubdir string, err error)
}

// A Rev describes a single revision in a source code repository.
type RevInfo struct {
	Name    string    // complete ID in underlying repository
	Short   string    // shortened ID, for use in pseudo-version
	Version string    // TODO what is this?
	Time    time.Time // commit time
}

func AllHex(rev string) bool {
	for i := 0; i < len(rev); i++ {
		c := rev[i]
		if '0' <= c && c <= '9' || 'a' <= c && c <= 'f' {
			continue
		}
		return false
	}
	return true
}

func ShortenSHA1(rev string) string {
	if AllHex(rev) && len(rev) == 40 {
		return rev[:12]
	}
	return rev
}
