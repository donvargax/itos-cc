// Command notices writes THIRD_PARTY_NOTICES, the license and notice texts
// of every Go module the itos-cc binary compiles in, which the release
// archives carry beside LICENSE (T-173): MIT, BSD and Apache ask their
// notices to travel with every copy, binaries included.
//
// It runs go list -deps for ./cmd/itos-cc under each platform the release
// builds (release.yml's build matrix: linux and darwin on amd64 and arm64
// and windows on amd64, CGO_ENABLED=1 and the build tags each is built
// with), so a module one platform alone compiles in is named too. go list
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
//	go run ./tools/bin/notices [-check]
//
// from the repository's root writes the file; commit it. With -check it
// writes nothing and fails, naming each module added, removed or changed,
// when the committed file is not what it would write.
//
// go list needs the modules, from the module cache or the network to the
// module proxy. It imports nothing but the standard library. Exit status:
// 0 written or current, 1 a module refused or the file not current, 2 go
// list failed or a file could not be read or written.
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
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

// platform is one build the release makes: its GOOS, its GOARCH and the
// build tags release.yml's build job gives it.
type platform struct{ goos, goarch, tags string }

// platforms are the builds of release.yml's build matrix, each with the
// tags its Build step sets.
var platforms = []platform{
	{"linux", "amd64", "netgo,osusergo"},
	{"linux", "arm64", "netgo,osusergo"},
	{"darwin", "amd64", ""},
	{"darwin", "arm64", ""},
	{"windows", "amd64", ""},
}

// prefixes are what a notice file's name starts with, case ignored.
var prefixes = []string{"LICENSE", "LICENCE", "COPYING", "NOTICE"}

func main() {
	check := flag.Bool("check", false, "write nothing; fail, naming each module added, removed or changed, when "+output+" is not what notices writes")
	flag.Parse()
	os.Exit(run(".", *check, flag.Args(), os.Stderr))
}

// refused is a module notices cannot write the notices of: exit 1.
type refused struct{ reason string }

func (r refused) Error() string { return r.reason }

// run writes the notices of the module at dir, or with check holds the
// committed file to them, and returns the exit status.
func run(dir string, check bool, args []string, stderr io.Writer) int {
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
		say(stderr, "%s: %s is not what the modules %s compiles in give: run go run ./tools/bin/notices and commit it\n", self, output, target)
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
	cmd := exec.Command("go", "list", "-deps", "-json=ImportPath,Module", "-tags="+p.tags, target) //nolint:gosec // go list, with arguments this program names
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

// notice is a module's notice files, in name order.
type notice struct {
	path  string
	files []noticeFile
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

// rule is the line above and below a module's header.
var rule = strings.Repeat("=", 80)

const preamble = `THIRD-PARTY NOTICES

The itos-cc binary compiles in the Go modules below. Each module's license
and notice files follow, verbatim, under a header naming the module.

Written by go run ./tools/bin/notices from the modules ./cmd/itos-cc compiles
in on every platform released; never edited by hand.
`

// render is the file: the preamble, then each module's header and files.
func render(notices []notice) []byte {
	var b bytes.Buffer
	b.WriteString(preamble)
	for _, n := range notices {
		b.WriteString(header(n.path))
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
