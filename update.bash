#!/usr/bin/env bash

set -e
rm -rf ./vendor/cmd/go
cp -a $(go env GOROOT)/src/cmd/go vendor/cmd/go
rm -f vendor/cmd/go/alldocs.go vendor/cmd/go/mkalldocs.sh # docs are in wrong place and describe wrong command
cd vendor/cmd/go
patch -p0 < ../../../patch.txt
vers=$(go version | sed 's/^go version //; s/ [A-Z][a-z][a-z].*//')
echo "package version; const version = \"$vers\"" > internal/version/vgo.go
gofmt -w internal
cd ../../..
rm $(find . -name '*.orig')
go build
./vgo version
rm vgo
git add .
