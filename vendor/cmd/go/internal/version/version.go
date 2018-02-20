// Copyright 2011 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package version implements the ``go version'' command.
package version

import (
	"fmt"
	"runtime"

	"cmd/go/internal/base"
	"cmd/go/internal/work"
)

const vgoVersion = "2018-02-20.1"

var CmdVersion = &base.Command{
	Run:       runVersion,
	UsageLine: "version",
	Short:     "print Go version",
	Long:      `Version prints the Go version, as reported by runtime.Version.`,
}

func runVersion(cmd *base.Command, args []string) {
	if len(args) != 0 {
		cmd.Usage()
	}

	fmt.Printf("go version %s %s/%s vgo:%s\n", work.RuntimeVersion, runtime.GOOS, runtime.GOARCH, vgoVersion)
}
