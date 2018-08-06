# Versioned Go Command (vgo)

This repository holds a standalone implementation of a version-aware `go` command,
allowing users with a Go 1.10 toolchain to use the new Go 1.11 module support.

The code in this repo is auto-generated from and should behave exactly like
the Go 1.11 `go` command, with two changes:

  - It behaves as if the `GO111MODULE` variable defaults to `on`.
  - When using a Go 1.10 toolchain, `go` `vet` during `go` `test` is disabled.

## Download/Install

Use `go get -u golang.org/x/vgo`.

You can also manually
git clone the repository to `$GOPATH/src/golang.org/x/vgo`.

## Report Issues / Send Patches

See [CONTRIBUTING.md](CONTRIBUTING.md).

Please file bugs in the main Go issue tracker,
[golang.org/issue](https://golang.org/issue),
and put the prefix `x/vgo:` in the issue title,
or `cmd/go:` if you have confirmed that the same
bug is present in the Go 1.11 module support.

Thank you.
