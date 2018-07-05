// Copyright 2018 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package modget implements the module-aware ``go get'' command.
package modget

import (
	"cmd/go/internal/base"
	"cmd/go/internal/get"
	"cmd/go/internal/load"
	"cmd/go/internal/modfetch"
	"cmd/go/internal/module"
	"cmd/go/internal/mvs"
	"cmd/go/internal/semver"
	"cmd/go/internal/vgo"
	"cmd/go/internal/work"
	"strings"
)

var CmdGet = &base.Command{
	// Note: -d -m -u are listed explicitly because they are the most common get flags.
	// Do not send CLs removing them because they're covered by [get flags].
	UsageLine: "get [-d] [-m] [-p] [-u] [-v] [-insecure] [build flags] [packages]",
	Short:     "add dependencies to current module and install them",
	Long: `
Get resolves and adds dependencies to the current development module
and then optionally downloads, builds, and installs them.

The first step is to resolve which dependencies to add. 

For each named package or package pattern, get must decide which version of
the corresponding module to use. By default, get chooses the latest tagged
release version, such as v0.4.5 or v1.2.3. If there are no tagged release
versions, get chooses the latest tagged prerelease version, such as
v0.0.1-pre1. If there are no tagged versions at all, get chooses the latest
known commit.

This default version selection can be overridden by adding an @version
suffix to the package argument, as in 'go get golang.org/x/text@v0.3.0'.
For modules stored in source control repositories, the version suffix can
also be a commit hash, branch identifier, or other syntax known to the
source control system, as in 'go get golang.org/x/text@master'.
The version suffix @latest explicitly requests the default behavior
described above.

If a module under consideration is already a dependency of the current
development module, then get will update the required version.
Specifying a version earlier than the current required version is valid and
downgrades the dependency. The version suffix @none indicates that the
dependency should be removed entirely.

Although get defaults to using the latest version of the module containing
a named package, it does not use the latest version of that module's
dependencies. Instead it prefers to use the specific dependency versions
requested by that module. For example, if the latest A requires module
B v1.2.3, while B v1.2.4 and v1.3.1 are also available, then 'go get A'
will use the latest A but then use B v1.2.3, as requested by A. (If there
are competing requirements for a particular module, then 'go get' resolves
those requirements by taking the maximum requested version.)

The -u flag instructs get to update dependencies to use newer minor or
point releases when available. Continuing the previous example,
'go get -u A' will use the latest A with B v1.3.1 (not B v1.2.3).

The -u=point flag instructs get to update dependencies to use newer
point releases when available. Continuing the previous example,
'go get -u=point A' will use the latest A with B v1.2.4 (not B v1.2.3).

In general, adding a new dependency may require upgrading
existing dependencies to keep a working build, and 'go get' does
this automatically. Similarly, downgrading one dependency may
require downgrading other dependenceis, and 'go get' does
this automatically as well.

The second step is to download source code for dependencies.
This step is usually skipped, deferring the download until the code
is first needed by a build (at which point it is cached for future use),
so that building one package in a module does not incur the cost of
downloading dependencies for other packages in the module.

The -d flag instructs get to download the source code for all added
modules, including their dependencies, recursively, so that the first
build that needs the code won't have to download it.

The -insecure flag permits fetching from repositories and resolving
custom domains using insecure schemes such as HTTP. Use with caution.

The -m flag instructs get to stop here, after resolving, adding, and
possibly downloading the modules, without building and installing
any named packages.

The third and final step is to build and install the named packages.

If an argument names a module but not a package (because there is no
Go source code in the module's root directory), then the install step
is skipped for that argument, instead of causing a build failure.
For example 'go get golang.org/x/perf' succeeds even though there
is no code corresponding to that import path.

Note that package patterns are allowed and are expanded after resolving
the module versions. For example, 'go get golang.org/x/perf/cmd/...'
adds the latest golang.org/x/perf and then installs the commands in that
latest version.

If 'go get' has no package arguments, then it applies to the current
development module: the -d flag downloads all dependencies of the
current module, and the -u and -u=point flags update all dependencies
of the current module. If there is a Go package in the current directory,
the build and install step applies to that package.

For more about modules, see 'go help modules'.

For more about specifying packages, see 'go help packages'.

This text describes the behavior of get using modules to manage
source code and dependencies.
If instead the go command is running in legacy GOPATH mode,
the details of get's flags and effects change, as does 'go help get'.
See 'go help modules' and 'go help gopath-get'.

See also: go build, go install, go clean, go mod.
	`,
}

// Note that this help text is a stopgap to make the module-aware get help text
// available even in non-module settings. It should be deleted when the old get
// is deleted. It should NOT be considered to set a precedent of having hierarchical
// help names with dashes.
var HelpModuleGet = &base.Command{
	UsageLine: "module-get",
	Short:     "module-aware go get",
	Long: `
The 'go get' command changes behavior depending on whether the
go command is running in module-aware mode or legacy GOPATH mode.
This help text, accessible as 'go help module-get' even in legacy GOPATH mode,
describes 'go get' as it operates in module-aware mode.

Usage: ` + CmdGet.UsageLine + `
` + CmdGet.Long,
}

var (
	getD   = CmdGet.Flag.Bool("d", false, "")
	getF   = CmdGet.Flag.Bool("f", false, "")
	getFix = CmdGet.Flag.Bool("fix", false, "")
	getM   = CmdGet.Flag.Bool("m", false, "")
	getT   = CmdGet.Flag.Bool("t", false, "")

	// -insecure is get.Insecure
	// -u is vgo.GetU
	// -v is cfg.BuildV
)

func init() {
	work.AddBuildFlags(CmdGet)
	CmdGet.Run = runGet // break init loop
	CmdGet.Flag.BoolVar(&get.Insecure, "insecure", get.Insecure, "")
	CmdGet.Flag.BoolVar(&vgo.GetU, "u", vgo.GetU, "")
}

func runGet(cmd *base.Command, args []string) {
	if vgo.GetU && len(args) > 0 {
		base.Fatalf("vgo get: -u not supported with argument list")
	}
	if !vgo.GetU && len(args) == 0 {
		base.Fatalf("vgo get: need arguments or -u")
	}

	if vgo.GetU {
		vgo.LoadBuildList()
		return
	}

	vgo.Init()
	vgo.InitMod()
	var upgrade []module.Version
	var downgrade []module.Version
	var newPkgs []string
	for _, pkg := range args {
		var path, vers string
		/* OLD CODE
		if n := strings.Count(pkg, "(") + strings.Count(pkg, ")"); n > 0 {
			i := strings.Index(pkg, "(")
			j := strings.Index(pkg, ")")
			if n != 2 || i < 0 || j <= i+1 || j != len(pkg)-1 && pkg[j+1] != '/' {
				base.Errorf("vgo get: invalid module version syntax: %s", pkg)
				continue
			}
			path, vers = pkg[:i], pkg[i+1:j]
			pkg = pkg[:i] + pkg[j+1:]
		*/
		if i := strings.Index(pkg, "@"); i >= 0 {
			path, pkg, vers = pkg[:i], pkg[:i], pkg[i+1:]
			if strings.Contains(vers, "@") {
				base.Errorf("vgo get: invalid module version syntax: %s", pkg)
				continue
			}
		} else {
			path = pkg
			vers = "latest"
		}
		if vers == "none" {
			downgrade = append(downgrade, module.Version{Path: path, Version: ""})
		} else {
			info, err := modfetch.Query(path, vers, vgo.Allowed)
			if err != nil {
				base.Errorf("vgo get %v: %v", pkg, err)
				continue
			}
			upgrade = append(upgrade, module.Version{Path: path, Version: info.Version})
			newPkgs = append(newPkgs, pkg)
		}
	}
	args = newPkgs

	// Upgrade.
	var err error
	list, err := mvs.Upgrade(vgo.Target, vgo.Reqs(), upgrade...)
	if err != nil {
		base.Fatalf("vgo get: %v", err)
	}
	vgo.SetBuildList(list)

	vgo.LoadBuildList()

	// Downgrade anything that went too far.
	version := make(map[string]string)
	for _, mod := range vgo.BuildList() {
		version[mod.Path] = mod.Version
	}
	for _, mod := range upgrade {
		if semver.Compare(mod.Version, version[mod.Path]) < 0 {
			downgrade = append(downgrade, mod)
		}
	}

	if len(downgrade) > 0 {
		list, err := mvs.Downgrade(vgo.Target, vgo.Reqs(), downgrade...)
		if err != nil {
			base.Fatalf("vgo get: %v", err)
		}
		vgo.SetBuildList(list)

		// TODO: Check that everything we need to import is still available.
		/*
			local := v.matchPackages("all", v.Reqs[:1])
			for _, path := range local {
				dir, err := v.importDir(path)
				if err != nil {
					return err // TODO
				}
				imports, testImports, err := scanDir(dir, v.Tags)
				for _, path := range imports {
					xxx
				}
				for _, path := range testImports {
					xxx
				}
			}
		*/
	}
	vgo.WriteGoMod()

	if *getD {
		// Download all needed code as side-effect.
		vgo.LoadALL()
	}

	if *getM {
		return
	}

	if len(args) > 0 {
		work.BuildInit()
		var list []string
		for _, p := range load.PackagesAndErrors(args) {
			if p.Error == nil || !strings.HasPrefix(p.Error.Err, "no Go files") {
				list = append(list, p.ImportPath)
			}
		}
		if len(list) > 0 {
			work.InstallPackages(list)
		}
	}
}
