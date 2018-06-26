// Copyright 2018 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package vgo

import (
	"bytes"
	"fmt"
	"go/build"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"cmd/go/internal/base"
	"cmd/go/internal/cfg"
	"cmd/go/internal/imports"
	"cmd/go/internal/modfetch"
	"cmd/go/internal/modfile"
	"cmd/go/internal/module"
	"cmd/go/internal/mvs"
	"cmd/go/internal/par"
	"cmd/go/internal/search"
	"cmd/go/internal/semver"
)

type importLevel int

const (
	levelNone          importLevel = 0
	levelBuild         importLevel = 1
	levelTest          importLevel = 2
	levelTestRecursive importLevel = 3
)

var (
	buildList []module.Version
	tags      map[string]bool
	importmap map[string]string
	pkgdir    map[string]string
	pkgmod    map[string]module.Version
)

func AddImports(gofiles []string) {
	if Init(); !Enabled() {
		return
	}
	InitMod()

	imports, testImports, err := imports.ScanFiles(gofiles, tags)
	if err != nil {
		base.Fatalf("vgo: %v", err)
	}

	iterate(func(ld *loader) {
		ld.importList(imports, levelBuild)
		ld.importList(testImports, levelBuild)
	})
	WriteGoMod()
}

// LoadBuildList loads and returns the build list from go.mod.
// The loading of the build list happens automatically in ImportPaths:
// LoadBuildList need only be called if ImportPaths is not
// (typically in commands that care about the module but
// no particular package).
func LoadBuildList() []module.Version {
	if Init(); !Enabled() {
		base.Fatalf("vgo: LoadBuildList called but vgo not enabled")
	}
	InitMod()
	iterate(func(*loader) {})
	WriteGoMod()
	return buildList
}

// BuildList returns the module build list,
// typically constructed by a previous call to
// LoadBuildList or ImportPaths.
func BuildList() []module.Version {
	return buildList
}

// SetBuildList sets the module build list.
// The caller is responsible for ensuring that the list is valid.
func SetBuildList(list []module.Version) {
	buildList = list
}

// ImportMap returns the actual package import path
// for an import path found in source code.
// If the given import path does not appear in the source code
// for the packages that have been loaded, ImportMap returns the empty string.
func ImportMap(path string) string {
	return importmap[path]
}

// PackageDir returns the directory containing the source code
// for the package named by the import path.
func PackageDir(path string) string {
	return pkgdir[path]
}

// PackageModule returns the module providing the package named by the import path.
func PackageModule(path string) module.Version {
	return pkgmod[path]
}

func ImportPaths(args []string) []string {
	if Init(); !Enabled() {
		return search.ImportPaths(args)
	}
	InitMod()

	paths := importPaths(args)
	WriteGoMod()
	return paths
}

func importPaths(args []string) []string {
	level := levelBuild
	switch cfg.CmdName {
	case "test", "vet":
		level = levelTest
	}
	isALL := len(args) == 1 && args[0] == "ALL"
	cleaned := search.CleanImportPaths(args)
	iterate(func(ld *loader) {
		if isALL {
			ld.tags = map[string]bool{"*": true}
		}
		args = expandImportPaths(cleaned)
		for i, pkg := range args {
			if pkg == "." || pkg == ".." || strings.HasPrefix(pkg, "./") || strings.HasPrefix(pkg, "../") {
				dir := filepath.Join(cwd, pkg)
				if dir == ModRoot {
					pkg = Target.Path
				} else if strings.HasPrefix(dir, ModRoot+string(filepath.Separator)) {
					pkg = Target.Path + filepath.ToSlash(dir[len(ModRoot):])
				} else {
					base.Errorf("vgo: package %s outside module root", pkg)
					continue
				}
				args[i] = pkg
			}
			ld.importPkg(pkg, level)
		}
	})
	return args
}

func Lookup(parentPath, path string) (dir, realPath string, err error) {
	realPath = importmap[path]
	if realPath == "" {
		if isStandardImportPath(path) {
			dir := filepath.Join(cfg.GOROOT, "src", path)
			if _, err := os.Stat(dir); err == nil {
				return dir, path, nil
			}
		}
		return "", "", fmt.Errorf("no such package in module")
	}
	return pkgdir[realPath], realPath, nil
}

func iterate(doImports func(*loader)) {
	var err error
	mvsOp := mvs.BuildList
	if *getU {
		mvsOp = mvs.UpgradeAll
	}
	buildList, err = mvsOp(Target, newReqs())
	if err != nil {
		base.Fatalf("vgo: %v", err)
	}

	var ld *loader
	for {
		ld = newLoader()
		doImports(ld)
		if len(ld.missing) == 0 {
			break
		}
		for _, m := range ld.missing {
			findMissing(m)
		}
		base.ExitIfErrors()
		buildList, err = mvsOp(Target, newReqs())
		if err != nil {
			base.Fatalf("vgo: %v", err)
		}
	}
	base.ExitIfErrors()

	importmap = ld.importmap
	pkgdir = ld.pkgdir
	pkgmod = ld.pkgmod
}

type loader struct {
	imported  map[string]importLevel
	importmap map[string]string
	pkgdir    map[string]string
	pkgmod    map[string]module.Version
	tags      map[string]bool
	missing   []missing
	imports   []string
	stack     []string
}

type missing struct {
	path  string
	stack string
}

func newLoader() *loader {
	ld := &loader{
		imported:  make(map[string]importLevel),
		importmap: make(map[string]string),
		pkgdir:    make(map[string]string),
		pkgmod:    make(map[string]module.Version),
		tags:      imports.Tags(),
	}
	ld.imported["C"] = 100
	return ld
}

func (ld *loader) stackText() string {
	var buf bytes.Buffer
	for _, p := range ld.stack[:len(ld.stack)-1] {
		fmt.Fprintf(&buf, "import %q ->\n\t", p)
	}
	fmt.Fprintf(&buf, "import %q", ld.stack[len(ld.stack)-1])
	return buf.String()
}

func (ld *loader) importList(pkgs []string, level importLevel) {
	for _, pkg := range pkgs {
		ld.importPkg(pkg, level)
	}
}

func (ld *loader) importPkg(path string, level importLevel) {
	if ld.imported[path] >= level {
		return
	}

	ld.stack = append(ld.stack, path)
	defer func() {
		ld.stack = ld.stack[:len(ld.stack)-1]
	}()

	// Any rewritings go here.
	realPath := path

	ld.imported[path] = level
	ld.importmap[path] = realPath
	if realPath != path && ld.imported[realPath] >= level {
		// Already handled.
		return
	}

	dir := ld.importDir(realPath)
	if dir == "" {
		return
	}

	ld.pkgdir[realPath] = dir

	imports, testImports, err := scanDir(dir, ld.tags)
	if err != nil {
		if strings.HasPrefix(err.Error(), "no Go ") {
			// Don't print about directories with no Go source files.
			// Let the eventual real package load do that.
			return
		}
		base.Errorf("vgo: %s [%s]: %v", ld.stackText(), dir, err)
		return
	}
	nextLevel := level
	if level == levelTest {
		nextLevel = levelBuild
	}
	for _, pkg := range imports {
		ld.importPkg(pkg, nextLevel)
	}
	if level >= levelTest {
		for _, pkg := range testImports {
			ld.importPkg(pkg, nextLevel)
		}
	}
}

func (ld *loader) importDir(path string) string {
	if importPathInModule(path, Target.Path) {
		dir := ModRoot
		if len(path) > len(Target.Path) {
			dir = filepath.Join(dir, path[len(Target.Path)+1:])
		}
		ld.pkgmod[path] = Target
		return dir
	}

	if search.IsStandardImportPath(path) {
		if strings.HasPrefix(path, "golang_org/") {
			return filepath.Join(cfg.GOROOT, "src/vendor", path)
		}
		dir := filepath.Join(cfg.GOROOT, "src", path)
		if _, err := os.Stat(dir); err == nil {
			return dir
		}
	}

	if cfg.BuildGetmode == "vendor" {
		// Using -getmode=vendor, everything the module needs
		// (beyond the current module and standard library)
		// must be in the module's vendor directory.
		return filepath.Join(ModRoot, "vendor", path)
	}

	var mod1 module.Version
	var dir1 string
	for _, mod := range buildList {
		if !importPathInModule(path, mod.Path) {
			continue
		}
		dir, err := fetch(mod)
		if err != nil {
			base.Errorf("vgo: %s: %v", ld.stackText(), err)
			return ""
		}
		if len(path) > len(mod.Path) {
			dir = filepath.Join(dir, path[len(mod.Path)+1:])
		}
		if dir1 != "" {
			base.Errorf("vgo: %s: found in both %v %v and %v %v", ld.stackText(),
				mod1.Path, mod1.Version, mod.Path, mod.Version)
			return ""
		}
		dir1 = dir
		mod1 = mod
	}
	if dir1 != "" {
		ld.pkgmod[path] = mod1
		return dir1
	}
	ld.missing = append(ld.missing, missing{path, ld.stackText()})
	return ""
}

// Replacement returns the replacement for mod, if any, from go.mod.
// If there is no replacement for mod, Replacement returns
// a module.Version with Path == "".
func Replacement(mod module.Version) module.Version {
	var found *modfile.Replace
	for _, r := range modFile.Replace {
		if r.Old == mod {
			found = r // keep going
		}
	}
	if found == nil {
		return module.Version{}
	}
	return found.New
}

func importPathInModule(path, mpath string) bool {
	return mpath == path ||
		len(path) > len(mpath) && path[len(mpath)] == '/' && path[:len(mpath)] == mpath
}

var found = make(map[string]bool)

func findMissing(m missing) {
	for _, mod := range buildList {
		if importPathInModule(m.path, mod.Path) {
			// Leave for ordinary build to complain about the missing import.
			return
		}
	}
	if build.IsLocalImport(m.path) {
		base.Errorf("vgo: relative import is not supported: %s", m.path)
		return
	}
	fmt.Fprintf(os.Stderr, "vgo: resolving import %q\n", m.path)
	repo, info, err := modfetch.Import(m.path, allowed)
	if err != nil {
		base.Errorf("vgo: %s: %v", m.stack, err)
		return
	}
	root := repo.ModulePath()
	fmt.Fprintf(os.Stderr, "vgo: finding %s (latest)\n", root)
	if found[root] {
		base.Fatalf("internal error: findmissing loop on %s", root)
	}
	found[root] = true
	fmt.Fprintf(os.Stderr, "vgo: adding %s %s\n", root, info.Version)
	buildList = append(buildList, module.Version{Path: root, Version: info.Version})
	modFile.AddRequire(root, info.Version)
}

// mvsReqs implements mvs.Reqs for vgo's semantic versions, with any exclusions
// or replacements applied internally.
type mvsReqs struct {
	extra []module.Version
	cache par.Cache
}

func newReqs(extra ...module.Version) *mvsReqs {
	r := &mvsReqs{
		extra: extra,
	}
	return r
}

// Reqs returns the module requirement graph.
func Reqs() mvs.Reqs {
	return newReqs()
}

func (r *mvsReqs) Required(mod module.Version) ([]module.Version, error) {
	type cached struct {
		list []module.Version
		err  error
	}

	c := r.cache.Do(mod, func() interface{} {
		list, err := r.required(mod)
		if err != nil {
			return cached{nil, err}
		}
		for i, mv := range list {
			for excluded[mv] {
				mv1, err := r.next(mv)
				if err != nil {
					return cached{nil, err}
				}
				if mv1.Version == "none" {
					return cached{nil, fmt.Errorf("%s(%s) depends on excluded %s(%s) with no newer version available", mod.Path, mod.Version, mv.Path, mv.Version)}
				}
				mv = mv1
			}
			list[i] = mv
		}

		return cached{list, nil}
	}).(cached)

	return c.list, c.err
}

func (r *mvsReqs) required(mod module.Version) ([]module.Version, error) {
	if mod == Target {
		var list []module.Version
		if buildList != nil {
			list = append(list, buildList[1:]...)
			return list, nil
		}
		for _, r := range modFile.Require {
			list = append(list, r.Mod)
		}
		list = append(list, r.extra...)
		return list, nil
	}

	origPath := mod.Path
	if repl := Replacement(mod); repl.Path != "" {
		if repl.Version == "" {
			// TODO: need to slip the new version into the tags list etc.
			dir := repl.Path
			if !filepath.IsAbs(dir) {
				dir = filepath.Join(ModRoot, dir)
			}
			gomod := filepath.Join(dir, "go.mod")
			data, err := ioutil.ReadFile(gomod)
			if err != nil {
				return nil, err
			}
			f, err := modfile.Parse(gomod, data, nil)
			if err != nil {
				return nil, err
			}
			var list []module.Version
			for _, r := range f.Require {
				list = append(list, r.Mod)
			}
			return list, nil
		}
		mod = repl
	}

	if mod.Version == "none" {
		return nil, nil
	}

	if !semver.IsValid(mod.Version) {
		// Disallow the broader queries supported by fetch.Lookup.
		panic(fmt.Errorf("invalid semantic version %q for %s", mod.Version, mod.Path))
		// TODO: return nil, fmt.Errorf("invalid semantic version %q", mod.Version)
	}

	data, err := modfetch.GoMod(mod.Path, mod.Version)
	if err != nil {
		base.Errorf("vgo: %s %s: %v\n", mod.Path, mod.Version, err)
		return nil, err
	}
	f, err := modfile.Parse("go.mod", data, nil)
	if err != nil {
		return nil, fmt.Errorf("parsing downloaded go.mod: %v", err)
	}

	if f.Module == nil {
		return nil, fmt.Errorf("%v@%v go.mod: missing module line", mod.Path, mod.Version)
	}
	if mpath := f.Module.Mod.Path; mpath != origPath && mpath != mod.Path {
		return nil, fmt.Errorf("downloaded %q and got module %q", mod.Path, mpath)
	}

	var list []module.Version
	for _, req := range f.Require {
		list = append(list, req.Mod)
	}
	if false {
		fmt.Fprintf(os.Stderr, "REQLIST %v:\n", mod)
		for _, req := range list {
			fmt.Fprintf(os.Stderr, "\t%v\n", req)
		}
	}
	return list, nil
}

func (*mvsReqs) Max(v1, v2 string) string {
	if v1 != "" && semver.Compare(v1, v2) == -1 {
		return v2
	}
	return v1
}

// Upgrade returns the desired upgrade for m.
// If m is a tagged version, then Upgrade returns the latest tagged version.
// If m is a pseudo-version, then Upgrade returns the latest tagged version
// when that version has a time-stamp newer than m.
// Otherwise Upgrade returns m (preserving the pseudo-version).
// This special case prevents accidental downgrades
// when already using a pseudo-version newer than the latest tagged version.
func (*mvsReqs) Upgrade(m module.Version) (module.Version, error) {
	// Note that query "latest" is not the same as
	// using repo.Latest.
	// The query only falls back to untagged versions
	// if nothing is tagged. The Latest method
	// only ever returns untagged versions,
	// which is not what we want.
	fmt.Fprintf(os.Stderr, "vgo: finding %s latest\n", m.Path)
	info, err := modfetch.Query(m.Path, "latest", allowed)
	if err != nil {
		return module.Version{}, err
	}

	// If we're on a later prerelease, keep using it,
	// even though normally an Upgrade will ignore prereleases.
	if semver.Compare(info.Version, m.Version) < 0 {
		return m, nil
	}

	// If we're on a pseudo-version chronologically after the latest tagged version, keep using it.
	// This avoids accidental downgrades.
	if mTime, err := modfetch.PseudoVersionTime(m.Version); err == nil && info.Time.Before(mTime) {
		return m, nil
	}
	return module.Version{Path: m.Path, Version: info.Version}, nil
}

func versions(path string) ([]string, error) {
	// Note: modfetch.Lookup and repo.Versions are cached,
	// so there's no need for us to add extra caching here.
	repo, err := modfetch.Lookup(path)
	if err != nil {
		return nil, err
	}
	return repo.Versions("")
}

// Previous returns the tagged version of m.Path immediately prior to
// m.Version, or version "none" if no prior version is tagged.
func (*mvsReqs) Previous(m module.Version) (module.Version, error) {
	list, err := versions(m.Path)
	if err != nil {
		return module.Version{}, err
	}
	i := sort.Search(len(list), func(i int) bool { return semver.Compare(list[i], m.Version) >= 0 })
	if i > 0 {
		return module.Version{Path: m.Path, Version: list[i-1]}, nil
	}
	return module.Version{Path: m.Path, Version: "none"}, nil
}

// next returns the next version of m.Path after m.Version.
// It is only used by the exclusion processing in the Required method,
// not called directly by MVS.
func (*mvsReqs) next(m module.Version) (module.Version, error) {
	list, err := versions(m.Path)
	if err != nil {
		return module.Version{}, err
	}
	i := sort.Search(len(list), func(i int) bool { return semver.Compare(list[i], m.Version) > 0 })
	if i < len(list) {
		return module.Version{Path: m.Path, Version: list[i]}, nil
	}
	return module.Version{Path: m.Path, Version: "none"}, nil
}

// scanDir is like imports.ScanDir but elides known magic imports from the list,
// so that vgo does not go looking for packages that don't really exist.
//
// The only known magic imports are appengine and appengine/*.
// These are so old that they predate "go get" and did not use URL-like paths.
// Most code today now uses google.golang.org/appengine instead,
// but not all code has been so updated. When we mostly ignore build tags
// during "vgo vendor", we look into "// +build appengine" files and
// may see these legacy imports. We drop them so that the module
// search does not look for modules to try to satisfy them.
func scanDir(path string, tags map[string]bool) (imports_, testImports []string, err error) {
	imports_, testImports, err = imports.ScanDir(path, tags)

	filter := func(x []string) []string {
		w := 0
		for _, pkg := range x {
			if pkg != "appengine" && !strings.HasPrefix(pkg, "appengine/") &&
				pkg != "appengine_internal" && !strings.HasPrefix(pkg, "appengine_internal/") {
				x[w] = pkg
				w++
			}
		}
		return x
	}

	return filter(imports_), filter(testImports), err
}

func fetch(mod module.Version) (dir string, err error) {
	if r := Replacement(mod); r.Path != "" {
		if r.Version == "" {
			dir = r.Path
			if !filepath.IsAbs(dir) {
				dir = filepath.Join(ModRoot, dir)
			}
			return dir, nil
		}
		mod = r
	}

	return modfetch.Download(mod)
}
