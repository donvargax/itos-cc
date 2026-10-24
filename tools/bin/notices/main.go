// Command notices writes THIRD_PARTY_NOTICES, the license and notice texts
// of every Go module the itos-cc binary compiles in and of the toolchain
// parts the release links into it, which the release archives carry beside
// LICENSE (T-173, T-176): MIT, BSD and Apache ask their notices to travel
// with every copy, binaries included.
//
// It runs go list -deps for ./cmd/itos-cc under each platform the release
// builds (release.yml's build matrix: linux and darwin on amd64 and arm64
// and windows on amd64, CGO_ENABLED=1, no build tags), so a module one
// platform alone compiles in is named too. go list
// reads build constraints only and compiles nothing, so it lists a foreign
// platform's cgo packages without that platform's C toolchain. It takes the
// union of the modules those packages are in, the main module and the
// standard library left out; a module replaced by another module is named
// and read as its replacement, one replaced by a directory is named by its
// own path and read from that directory.
//
// For each module, in module path order, the file holds a header naming
// the module's path, never its version, so a bump that keeps a module's
// texts leaves the file as it was, then every file in the module's root
// whose name starts with LICENSE, LICENCE, COPYING or NOTICE, case ignored,
// in name order, each under a line naming it, verbatim but for its line
// endings: written with LF, as .gitattributes checks out every text file.
// A module with no such file is refused, each named, and nothing is
// written: a notice is never skipped.
//
// The toolchain's parts follow the modules, each under a header naming it,
// in a fixed order: Go's runtime and standard library, in every build, by
// the LICENSE of the GOROOT go env names, read when the file is written, so
// a Go bump that keeps its text leaves the file as it was; then the C
// runtimes the Linux and Windows builds link statically, their texts
// committed verbatim under toolchain/ (toolchain, below, names where each
// came from and its version). macOS links its system libraries
// dynamically, so they need nothing here. A GOROOT with no LICENSE, as a
// distribution's Go may have, is refused: run it with go.dev's Go, which
// GOTOOLCHAIN set to go.mod's toolchain fetches.
//
// The viewer's npm packages follow the toolchain (slice-42): the bundle
// vite builds from viewer/, which a release's binary embeds and serve
// serves, carries them, and its minified JavaScript keeps none of their
// notices. They are the packages viewer/package-lock.json lists with no
// dev or devOptional flag, the @types scope left out, as it holds type
// declarations alone: the set vite's own license list gives. Each is named
// by its place under node_modules, its npm name for a top-level package,
// never its version, and its notice files are read from viewer/node_modules
// by the rule above, a package with none refused. A package offered under a
// choice of licences is named under the one itos-cc takes (choices, below),
// whose text follows its own files; the lockfile naming another offer for
// it is refused, so the choice is made again.
//
//	go run ./tools/bin/notices [-check]
//
// from the repository's root writes the file; commit it. With -check it
// writes nothing and fails, naming each module, toolchain part or viewer
// package added, removed or changed, when the committed file is not what it
// would write.
//
// go list needs the modules, from the module cache or the network to the
// module proxy, and the viewer's packages need npm ci in viewer/. It
// imports nothing but the standard library. Exit status: 0 written or
// current, 1 a module, viewer package or Go's LICENSE refused or the file
// not current, 2 go list or go env failed, viewer/node_modules is missing,
// or a file could not be read or written.
package main

import (
	"bytes"
	"embed"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
)

const (
	self = "notices"
	// output is the file written, from the repository's root.
	output = "THIRD_PARTY_NOTICES"
	// target is the package whose dependencies the binary compiles in.
	target = "./cmd/itos-cc"
)

// platform is one build the release makes: its GOOS and its GOARCH.
type platform struct{ goos, goarch string }

// platforms are the builds of release.yml's build matrix, none built with
// tags.
var platforms = []platform{
	{"linux", "amd64"},
	{"linux", "arm64"},
	{"darwin", "amd64"},
	{"darwin", "arm64"},
	{"windows", "amd64"},
}

// goPart is the header of Go's own notice, its LICENSE read from GOROOT.
const goPart = "Go, the runtime and standard library of every build"

// toolchainTexts holds the C runtimes' texts, committed verbatim.
//
//go:embed toolchain
var toolchainTexts embed.FS

// toolchain is the C runtimes the release links in statically, after Go,
// in this order: each part's header and its text's path in toolchainTexts.
var toolchain = []struct{ name, file string }{
	// musl's COPYRIGHT at v1.2.4,
	// https://git.musl-libc.org/cgit/musl/plain/COPYRIGHT?h=v1.2.4: the musl
	// of Ubuntu 24.04's musl-tools 1.2.4-2, whose musl-gcc release.yml's
	// Linux builds link with on ubuntu-latest (24.04) and ubuntu-24.04-arm.
	{"musl, the C library of the Linux builds", "toolchain/musl/COPYRIGHT"},
	// MinGW-w64's runtime licence at v13.0.0,
	// https://github.com/mingw-w64/mingw-w64/blob/v13.0.0/COPYING.MinGW-w64-runtime/COPYING.MinGW-w64-runtime.txt:
	// the runtime of windows-latest's MinGW, niXman's mingw-builds
	// 15.2.0-rt_v13 (gcc 15.2.0, runtime v13), which the Windows build links.
	{"MinGW-w64 runtime, linked into the Windows build", "toolchain/mingw-w64-runtime/COPYING.MinGW-w64-runtime.txt"},
	// winpthreads' COPYING at the same tag,
	// https://github.com/mingw-w64/mingw-w64/blob/v13.0.0/mingw-w64-libraries/winpthreads/COPYING.
	{"MinGW-w64 winpthreads, linked into the Windows build", "toolchain/winpthreads/COPYING"},
}

// viewer is the viewer's folder, from the repository's root: its lockfile
// and node_modules.
const viewer = "viewer"

// licenceTexts holds the texts of the licences choices takes, committed
// verbatim.
//
//go:embed licences
var licenceTexts embed.FS

// choice is the licence itos-cc takes a package under that offers several:
// the offer the lockfile names, which the choice was made for; the licence
// taken; the note under the package's header; and the taken licence's text,
// by its path in licenceTexts.
type choice struct{ offered, taken, note, file string }

// choices are the viewer's packages offered under a choice of licences, by
// name.
var choices = map[string]choice{
	// The person, q-87 (2026-10-24): elkjs is taken under GPL-3.0-or-later,
	// which GPLv3's section 13 lets combine with itos-cc's AGPL-3.0, so the
	// binary stays AGPL-3.0. Its package ships EPL-2.0's text alone. GPLv3's
	// is gnu.org's, https://www.gnu.org/licenses/gpl-3.0.txt, SHA-256
	// 3972dc9744f6499f0f9b2dbf76696f2ae7ad8af9b23dde66d6af86c9dfb36986.
	"elkjs": {
		offered: "EPL-2.0 OR GPL-3.0-or-later",
		note: "elkjs is dual-licensed EPL-2.0 OR GPL-3.0-or-later. itos-cc takes it\n" +
			"under GPL-3.0-or-later, which GPLv3's section 13 lets combine with\n" +
			"itos-cc's AGPL-3.0; its text follows elkjs's own LICENSE.md, which holds\n" +
			"EPL-2.0's.\n",
		taken: "GPL-3.0-or-later",
		file:  "licences/gpl-3.0.txt",
	},
}

// prefixes are what a notice file's name starts with, case ignored.
var prefixes = []string{"LICENSE", "LICENCE", "COPYING", "NOTICE"}

func main() {
	check := flag.Bool("check", false, "write nothing; fail, naming each module added, removed or changed, when "+output+" is not what notices writes")
	flag.Parse()
	os.Exit(run(".", *check, flag.Args(), goroot, os.Stderr))
}

// refused is a module notices cannot write the notices of: exit 1.
type refused struct{ reason string }

func (r refused) Error() string { return r.reason }

// run writes the notices of the module at dir, Go's read from the GOROOT
// root gives, or with check holds the committed file to them, and returns
// the exit status.
func run(dir string, check bool, args []string, root func(dir string) (string, error), stderr io.Writer) int {
	if len(args) > 0 {
		say(stderr, "%s: unexpected argument %q\n", self, args[0])
		return 2
	}
	mods, err := union(dir)
	if err != nil {
		say(stderr, "%s: %v\n", self, err)
		return 2
	}
	notices, err := read(mods)
	if err == nil {
		var parts []notice
		parts, err = toolchainNotices(dir, root)
		notices = append(notices, parts...)
	}
	if err == nil {
		var parts []notice
		parts, err = viewerNotices(dir)
		notices = append(notices, parts...)
	}
	var r refused
	if errors.As(err, &r) {
		say(stderr, "%s: %s\n", self, r.reason)
		return 1
	}
	if err != nil {
		say(stderr, "%s: %v\n", self, err)
		return 2
	}
	text := render(notices)
	path := filepath.Join(dir, output)
	if check {
		committed, err := os.ReadFile(path) //nolint:gosec // the repository's own THIRD_PARTY_NOTICES
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			say(stderr, "%s: %v\n", self, err)
			return 2
		}
		if bytes.Equal(committed, text) {
			return 0
		}
		say(stderr, "%s: %s is not what the modules %s compiles in, the toolchain and the viewer's packages give: run go run ./tools/bin/notices and commit it\n", self, output, target)
		for _, line := range differences(committed, text) {
			say(stderr, "  %s\n", line)
		}
		return 1
	}
	if err := os.WriteFile(path, text, 0o644); err != nil { //nolint:gosec // a text file every reader of the archive reads
		say(stderr, "%s: %v\n", self, err)
		return 2
	}
	return 0
}

func say(w io.Writer, format string, a ...any) { _, _ = fmt.Fprintf(w, format, a...) }

// module is a module the binary compiles in: the path its notices are
// named by and the directory they are read from.
type module struct{ path, dir string }

// union is every module the target compiles in under any platform, in
// module path order.
func union(dir string) ([]module, error) {
	byPath := map[string]module{}
	for _, p := range platforms {
		listed, err := list(dir, p)
		if err != nil {
			return nil, err
		}
		for _, m := range listed {
			byPath[m.path] = m
		}
	}
	mods := make([]module, 0, len(byPath))
	for _, m := range byPath {
		mods = append(mods, m)
	}
	slices.SortFunc(mods, func(a, b module) int { return strings.Compare(a.path, b.path) })
	return mods, nil
}

// list is the modules the target's packages are in, built for p.
func list(dir string, p platform) ([]module, error) {
	cmd := exec.Command("go", "list", "-deps", "-json=ImportPath,Module", target)
	cmd.Dir = dir
	// Later entries win, so these hold whatever the caller's environment sets.
	cmd.Env = append(os.Environ(), "GOOS="+p.goos, "GOARCH="+p.goarch, "CGO_ENABLED=1")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("go list -deps %s for %s/%s: %v\n%s", target, p.goos, p.goarch, err, stderr.String())
	}
	mods, err := decode(bytes.NewReader(out))
	if err != nil {
		return nil, fmt.Errorf("go list -deps %s for %s/%s: %v", target, p.goos, p.goarch, err)
	}
	return mods, nil
}

// listedModule is the part of go list's Module that says which module a
// package is in and where its files are.
type listedModule struct {
	Path, Version, Dir string
	Main               bool
	Replace            *listedModule
}

// decode reads go list -json's stream of packages into the modules they
// are in, each once: a standard package, in no module, and the main
// module are left out.
func decode(r io.Reader) ([]module, error) {
	var mods []module
	seen := map[string]bool{}
	dec := json.NewDecoder(r)
	for {
		var pkg struct{ Module *listedModule }
		err := dec.Decode(&pkg)
		if errors.Is(err, io.EOF) {
			return mods, nil
		}
		if err != nil {
			return nil, err
		}
		m := pkg.Module
		if m == nil || m.Main {
			continue
		}
		listed := module{path: m.Path, dir: m.Dir}
		if rep := m.Replace; rep != nil {
			listed.dir = rep.Dir
			if rep.Version != "" {
				listed.path = rep.Path
			}
		}
		if listed.dir == "" {
			return nil, fmt.Errorf("go list gives no directory for %s", listed.path)
		}
		if !seen[listed.path] {
			seen[listed.path] = true
			mods = append(mods, listed)
		}
	}
}

// notice is a module's notice files, in name order, and the note under its
// header, "" for none.
type notice struct {
	path  string
	files []noticeFile
	note  string
}

type noticeFile struct {
	name string
	text []byte
}

// isNotice says whether a file of a module's root is a notice by its name.
func isNotice(name string) bool {
	upper := strings.ToUpper(name)
	return slices.ContainsFunc(prefixes, func(p string) bool { return strings.HasPrefix(upper, p) })
}

// read is each module's notice files, or refused naming every module with
// none.
func read(mods []module) ([]notice, error) {
	var notices []notice
	var none []string
	for _, m := range mods {
		entries, err := os.ReadDir(m.dir)
		if err != nil {
			return nil, err
		}
		n := notice{path: m.path}
		for _, e := range entries {
			if !e.Type().IsRegular() || !isNotice(e.Name()) {
				continue
			}
			text, err := os.ReadFile(filepath.Join(m.dir, e.Name())) //nolint:gosec // a notice file of a module go list names
			if err != nil {
				return nil, err
			}
			n.files = append(n.files, noticeFile{e.Name(), text})
		}
		if len(n.files) == 0 {
			none = append(none, m.path+" ("+m.dir+")")
			continue
		}
		notices = append(notices, n)
	}
	if len(none) > 0 {
		return nil, refused{"no LICENSE, LICENCE, COPYING or NOTICE file in the root of " + strings.Join(none, ", ") + ": find its license and say how its notice ships before releasing it"}
	}
	return notices, nil
}

// goroot is the GOROOT go env names, run in dir.
func goroot(dir string) (string, error) {
	cmd := exec.Command("go", "env", "GOROOT")
	cmd.Dir = dir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("go env GOROOT: %v\n%s", err, stderr.String())
	}
	return strings.TrimSpace(string(out)), nil
}

// toolchainNotices is the toolchain's parts: Go's LICENSE, from the GOROOT
// root gives for dir, or refused when it has none; then each C runtime's
// committed text.
func toolchainNotices(dir string, root func(dir string) (string, error)) ([]notice, error) {
	goRoot, err := root(dir)
	if err != nil {
		return nil, err
	}
	license, err := os.ReadFile(filepath.Join(goRoot, "LICENSE")) //nolint:gosec // Go's LICENSE, in the GOROOT go env names
	if errors.Is(err, os.ErrNotExist) {
		return nil, refused{"no LICENSE in GOROOT, " + goRoot + ", as a distribution's Go may leave it out: run it with go.dev's Go, which GOTOOLCHAIN=<go.mod's toolchain> go run ./tools/bin/notices fetches"}
	}
	if err != nil {
		return nil, err
	}
	parts := []notice{{path: goPart, files: []noticeFile{{"LICENSE", license}}}}
	for _, p := range toolchain {
		text, err := toolchainTexts.ReadFile(p.file)
		if err != nil {
			return nil, err
		}
		parts = append(parts, notice{path: p.name, files: []noticeFile{{filepath.Base(p.file), text}}})
	}
	return parts, nil
}

// lockedPackage is the part of a package-lock.json package entry that says
// whether the bundle can carry it and under which licence it is offered.
type lockedPackage struct {
	License          string
	Dev, DevOptional bool
}

// viewerNotices is the notice files of each npm package the viewer's
// bundle carries, by its place under node_modules, read from the viewer's
// node_modules in dir, with the licence choices takes a package under.
func viewerNotices(dir string) ([]notice, error) {
	lock, err := os.ReadFile(filepath.Join(dir, viewer, "package-lock.json")) //nolint:gosec // the repository's own viewer lockfile
	if err != nil {
		return nil, err
	}
	var locked struct{ Packages map[string]lockedPackage }
	if err := json.Unmarshal(lock, &locked); err != nil {
		return nil, fmt.Errorf("%s/package-lock.json: %v", viewer, err)
	}
	if _, err := os.Stat(filepath.Join(dir, viewer, "node_modules")); err != nil {
		return nil, fmt.Errorf("no %s/node_modules to read the viewer's notices from: run npm ci in %s/ first (%v)", viewer, viewer, err)
	}
	var pkgs []module
	for _, key := range slices.Sorted(maps.Keys(locked.Packages)) {
		p := locked.Packages[key]
		name, ok := strings.CutPrefix(key, "node_modules/")
		if !ok || p.Dev || p.DevOptional || strings.HasPrefix(name, "@types/") {
			continue
		}
		if c, ok := choices[name]; ok && p.License != c.offered {
			return nil, refused{fmt.Sprintf("%s/package-lock.json offers %s under %q, not the %q itos-cc took it under %s from: decide again under which licence it ships, in tools/bin/notices' choices", viewer, name, p.License, c.offered, c.taken)}
		}
		pkgs = append(pkgs, module{path: name, dir: filepath.Join(dir, viewer, filepath.FromSlash(key))})
	}
	notices, err := read(pkgs)
	if err != nil {
		return nil, err
	}
	return notices, chosen(licenceTexts, notices)
}

// chosen gives each of notices that choices names its choice's note, and
// the text of the licence taken, read from texts, after its own files.
func chosen(texts fs.FS, notices []notice) error {
	for i, n := range notices {
		c, ok := choices[n.path]
		if !ok {
			continue
		}
		text, err := fs.ReadFile(texts, c.file)
		if err != nil {
			return err
		}
		notices[i].note = c.note
		notices[i].files = append(n.files, noticeFile{c.taken + " (" + filepath.Base(c.file) + ")", text})
	}
	return nil
}

// rule is the line above and below a module's header.
var rule = strings.Repeat("=", 80)

const preamble = `THIRD-PARTY NOTICES

The itos-cc binary compiles in the Go modules below, then links in Go's
runtime and standard library, and the C runtime of a statically linked
build: musl on Linux, MinGW-w64's runtime and winpthreads on Windows. A
release's binary also carries the architecture viewer serve serves, whose
bundle holds the npm packages after them, each named by its place under
node_modules; each package's source is its npm package at the version
viewer/package-lock.json pins. Each one's license and notice files follow,
verbatim, under a header naming it.

Written by go run ./tools/bin/notices from the modules ./cmd/itos-cc compiles
in on every platform released, the toolchain's texts and the viewer's
packages; never edited by hand.
`

// render is the file: the preamble, then each module's or toolchain part's
// header and files.
func render(notices []notice) []byte {
	var b bytes.Buffer
	b.WriteString(preamble)
	for _, n := range notices {
		b.WriteString(header(n.path))
		if n.note != "" {
			b.WriteString("\n" + n.note)
		}
		for _, f := range n.files {
			fmt.Fprintf(&b, "\n--- %s ---\n\n", f.name)
			text := bytes.ReplaceAll(f.text, []byte("\r\n"), []byte("\n"))
			b.Write(text)
			if !bytes.HasSuffix(text, []byte("\n")) {
				b.WriteString("\n")
			}
		}
	}
	return b.Bytes()
}

// header is the lines a module's notices start with.
func header(path string) string {
	return "\n" + rule + "\n" + path + "\n" + rule + "\n"
}

// sections is a file's text cut into each module's part, by the module's
// path, and the text before the first module.
func sections(text []byte) (string, map[string]string) {
	parts := strings.Split(string(text), "\n"+rule+"\n")
	mods := map[string]string{}
	// parts alternate: before, path, body, path, body…; a path sits between
	// two rules.
	for i := 1; i+1 < len(parts); i += 2 {
		mods[parts[i]] = parts[i+1]
	}
	return parts[0], mods
}

// differences names what makes the committed file not what notices
// writes: each module added, removed or changed, in module path order, or
// the text around them.
func differences(committed, written []byte) []string {
	before, had := sections(committed)
	wantBefore, want := sections(written)
	paths := map[string]bool{}
	for p := range had {
		paths[p] = true
	}
	for p := range want {
		paths[p] = true
	}
	var lines []string
	for _, p := range slices.Sorted(maps.Keys(paths)) {
		text, in := had[p]
		wanted, ought := want[p]
		switch {
		case !in:
			lines = append(lines, "added "+p)
		case !ought:
			lines = append(lines, "removed "+p)
		case text != wanted:
			lines = append(lines, "changed "+p)
		}
	}
	if len(lines) == 0 || before != wantBefore {
		lines = append(lines, "the modules' order or the text around their notices differs")
	}
	return lines
}
