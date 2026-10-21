package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fixture is a module tree, no network: example.com/app, whose
// cmd/itos-cc imports modules each replaced by a folder under deps/, some
// only on one platform, and the folders holding each module's files, by
// their paths from the tree's root. It returns the root.
func fixture(t *testing.T, files map[string]string) string {
	t.Helper()
	t.Setenv("GOPROXY", "off")
	t.Setenv("GOWORK", "off")
	t.Setenv("GOFLAGS", "-mod=mod")
	root := t.TempDir()
	var mod strings.Builder
	mod.WriteString("module example.com/app\n\ngo 1.24\n")
	for name := range files {
		if dep, ok := strings.CutPrefix(name, "deps/"); ok && strings.HasSuffix(dep, "/go.mod") {
			dep = strings.TrimSuffix(dep, "/go.mod")
			mod.WriteString("\nrequire example.com/" + dep + " v0.0.0\nreplace example.com/" + dep + " => ./deps/" + dep + "\n")
		}
	}
	files["go.mod"] = mod.String()
	for name, text := range files {
		p := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// dep is a module example.com/<name>'s go.mod and one package at its root.
func dep(files map[string]string, name string) {
	files["deps/"+name+"/go.mod"] = "module example.com/" + name + "\n\ngo 1.24\n"
	files["deps/"+name+"/"+name+".go"] = "package " + name + "\n"
}

// imports is a Go file of package main importing each path, under a
// build constraint unless it is "".
func imports(constraint string, paths ...string) string {
	var b strings.Builder
	if constraint != "" {
		b.WriteString("//go:build " + constraint + "\n\n")
	}
	b.WriteString("package main\n\n")
	for _, p := range paths {
		b.WriteString("import _ \"" + p + "\"\n")
	}
	return b.String()
}

// tree is the fixture every case starts from: zeta and alpha imported on
// every platform, alpha's sub-package too, win on windows with cgo alone,
// mac on darwin with cgo alone, net on linux with cgo alone, unused
// required but never imported; each with a notice, alpha with several
// files that are and are not.
func tree() map[string]string {
	files := map[string]string{
		"cmd/itos-cc/main.go":    imports("", "example.com/zeta", "example.com/alpha", "example.com/alpha/sub") + "\nfunc main() {}\n",
		"cmd/itos-cc/windows.go": imports("windows && cgo", "example.com/win"),
		"cmd/itos-cc/darwin.go":  imports("darwin && cgo", "example.com/mac"),
		"cmd/itos-cc/linux.go":   imports("linux && cgo", "example.com/net"),
	}
	for _, name := range []string{"zeta", "alpha", "win", "mac", "net", "unused"} {
		dep(files, name)
		files["deps/"+name+"/LICENSE"] = "The " + name + " license.\n"
	}
	delete(files, "deps/alpha/LICENSE")
	files["deps/alpha/sub/sub.go"] = "package sub\n"
	files["deps/alpha/sub/LICENSE"] = "Not alpha's root.\n"
	files["deps/alpha/license.md"] = "alpha's license,\r\nwith CRLF and no last newline."
	files["deps/alpha/NOTICE"] = "alpha's notice.\n"
	files["deps/alpha/Copying.LESSER"] = "alpha's copying.\n"
	files["deps/alpha/Licence-MIT"] = "alpha's licence.\n"
	files["deps/alpha/PATENTS"] = "Not a notice.\n"
	files["deps/alpha/README.md"] = "Not a notice.\n"
	files["deps/alpha/MY-LICENSE"] = "Not a notice.\n"
	files["deps/alpha/LICENSES/MIT.txt"] = "A folder, not a file.\n"
	return files
}

// goWith is a GOROOT holding license as its LICENSE, or none when license
// is "", and the function run asks for it.
func goWith(t *testing.T, license string) func(string) (string, error) {
	t.Helper()
	dir := t.TempDir()
	if license != "" {
		if err := os.WriteFile(filepath.Join(dir, "LICENSE"), []byte(license), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return func(string) (string, error) { return dir, nil }
}

// toolchainWant is the toolchain's parts as the file holds them, after the
// modules: Go's LICENSE, then musl's, MinGW-w64's runtime's and
// winpthreads' committed texts, each under its header.
func toolchainWant(t *testing.T, goLicense string) string {
	t.Helper()
	want := header("Go, the runtime and standard library of every build") + "\n--- LICENSE ---\n\n" + goLicense
	for _, p := range []struct{ name, file string }{
		{"musl, the C library of the Linux builds", "musl/COPYRIGHT"},
		{"MinGW-w64 runtime, linked into the Windows build", "mingw-w64-runtime/COPYING.MinGW-w64-runtime.txt"},
		{"MinGW-w64 winpthreads, linked into the Windows build", "winpthreads/COPYING"},
	} {
		text, err := os.ReadFile(filepath.Join("toolchain", filepath.FromSlash(p.file)))
		if err != nil {
			t.Fatal(err)
		}
		want += header(p.name) + "\n--- " + filepath.Base(p.file) + " ---\n\n" + string(text)
	}
	return want
}

// The file: the preamble, then each module the binary compiles in on any
// platform, in path order, under a header naming its path and no version,
// with every notice file of its root in name order, verbatim but for its
// line endings; a module required but never imported, a sub-package's
// license and a file named otherwise are not in it. The toolchain's parts
// follow, Go's LICENSE from GOROOT first.
func TestNotices(t *testing.T) {
	root := fixture(t, tree())
	var stderr bytes.Buffer
	if code := run(root, false, nil, goWith(t, "The Go license.\n"), &stderr); code != 0 {
		t.Fatalf("run = %d, want 0: %s", code, stderr.String())
	}
	got, err := os.ReadFile(filepath.Join(root, output))
	if err != nil {
		t.Fatal(err)
	}
	want := preamble +
		header("example.com/alpha") +
		"\n--- Copying.LESSER ---\n\nalpha's copying.\n" +
		"\n--- Licence-MIT ---\n\nalpha's licence.\n" +
		"\n--- NOTICE ---\n\nalpha's notice.\n" +
		"\n--- license.md ---\n\nalpha's license,\nwith CRLF and no last newline.\n" +
		header("example.com/mac") + "\n--- LICENSE ---\n\nThe mac license.\n" +
		header("example.com/net") + "\n--- LICENSE ---\n\nThe net license.\n" +
		header("example.com/win") + "\n--- LICENSE ---\n\nThe win license.\n" +
		header("example.com/zeta") + "\n--- LICENSE ---\n\nThe zeta license.\n" +
		toolchainWant(t, "The Go license.\n")
	if string(got) != want {
		t.Errorf("%s:\n%s\nwant:\n%s", output, got, want)
	}
	if !strings.Contains(header("example.com/zeta"), "\nexample.com/zeta\n") || strings.Contains(string(got), "v0.0.0") {
		t.Errorf("a header names more than the module's path:\n%s", got)
	}
}

// A notice file is one whose name starts with LICENSE, LICENCE, COPYING
// or NOTICE, case ignored.
func TestIsNotice(t *testing.T) {
	for name, want := range map[string]bool{
		"LICENSE": true, "license.txt": true, "Licence": true, "LICENSE-APACHE": true,
		"COPYING": true, "copying.lesser": true, "NOTICE": true, "Notice.md": true,
		"README.md": false, "PATENTS": false, "MY-LICENSE": false, "LICENS": false, "": false,
	} {
		if got := isNotice(name); got != want {
			t.Errorf("isNotice(%q) = %v, want %v", name, got, want)
		}
	}
}

// A module with no notice file in its root fails the generator, each such
// module named, and nothing is written.
func TestAModuleWithNoNoticeFails(t *testing.T) {
	files := tree()
	delete(files, "deps/zeta/LICENSE")
	delete(files, "deps/win/LICENSE")
	files["deps/zeta/README.md"] = "No license here.\n"
	root := fixture(t, files)
	var stderr bytes.Buffer
	if code := run(root, false, nil, goWith(t, "The Go license.\n"), &stderr); code != 1 {
		t.Fatalf("run = %d, want 1: %s", code, stderr.String())
	}
	for _, name := range []string{"example.com/zeta", "example.com/win"} {
		if !strings.Contains(stderr.String(), name+" (") {
			t.Errorf("the refusal does not name %s: %s", name, stderr.String())
		}
	}
	if strings.Contains(stderr.String(), "example.com/alpha") {
		t.Errorf("the refusal names a module with a notice: %s", stderr.String())
	}
	if _, err := os.Stat(filepath.Join(root, output)); !os.IsNotExist(err) {
		t.Errorf("%s was written: %v", output, err)
	}
}

// Go's LICENSE missing from GOROOT, as a distribution's Go may leave it
// out, fails the generator, saying how to run it, and nothing is written.
func TestAGoWithNoLicenseFails(t *testing.T) {
	root := fixture(t, tree())
	var stderr bytes.Buffer
	if code := run(root, false, nil, goWith(t, ""), &stderr); code != 1 {
		t.Fatalf("run = %d, want 1: %s", code, stderr.String())
	}
	for _, want := range []string{"no LICENSE in GOROOT", "GOTOOLCHAIN"} {
		if !strings.Contains(stderr.String(), want) {
			t.Errorf("the refusal does not say %q: %s", want, stderr.String())
		}
	}
	if _, err := os.Stat(filepath.Join(root, output)); !os.IsNotExist(err) {
		t.Errorf("%s was written: %v", output, err)
	}
}

// Each C runtime's committed text is the file it names, verbatim: its
// first line is upstream's.
func TestToolchainTexts(t *testing.T) {
	firsts := map[string]string{
		"toolchain/musl/COPYRIGHT":                                  "musl as a whole is licensed under the following standard MIT license:",
		"toolchain/mingw-w64-runtime/COPYING.MinGW-w64-runtime.txt": "MinGW-w64 runtime licensing",
		"toolchain/winpthreads/COPYING":                             "Copyright (c) 2011 mingw-w64 project",
	}
	if len(toolchain) != len(firsts) {
		t.Errorf("toolchain names %d texts, want %d", len(toolchain), len(firsts))
	}
	for _, p := range toolchain {
		text, err := toolchainTexts.ReadFile(p.file)
		if err != nil {
			t.Fatal(err)
		}
		if first, _, _ := strings.Cut(string(text), "\n"); first != firsts[p.file] {
			t.Errorf("%s starts %q, want %q", p.file, first, firsts[p.file])
		}
	}
}

// -check writes nothing, passes on a current file, and fails naming each
// module or toolchain part added, removed or changed.
func TestCheck(t *testing.T) {
	root := fixture(t, tree())
	path := filepath.Join(root, output)
	goRoot := goWith(t, "The Go license.\n")
	var stderr bytes.Buffer
	if code := run(root, true, nil, goRoot, &stderr); code != 1 || !strings.Contains(stderr.String(), "added example.com/alpha\n") {
		t.Errorf("-check with no file = %d, want 1 naming every module added: %s", code, stderr.String())
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("-check wrote %s: %v", output, err)
	}
	if code := run(root, false, nil, goRoot, &stderr); code != 0 {
		t.Fatalf("run = %d: %s", code, stderr.String())
	}
	current, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	stderr.Reset()
	if code := run(root, true, nil, goRoot, &stderr); code != 0 {
		t.Errorf("-check on a current file = %d, want 0: %s", code, stderr.String())
	}

	stale := strings.Replace(string(current), "The mac license.", "The old mac license.", 1)
	stale = strings.Replace(stale, header("example.com/zeta")+"\n--- LICENSE ---\n\nThe zeta license.\n", "", 1)
	stale += header("example.com/gone") + "\n--- LICENSE ---\n\nGone.\n"
	if err := os.WriteFile(path, []byte(stale), 0o600); err != nil {
		t.Fatal(err)
	}
	stderr.Reset()
	if code := run(root, true, nil, goRoot, &stderr); code != 1 {
		t.Errorf("-check on a stale file = %d, want 1", code)
	}
	for _, want := range []string{"\n  removed example.com/gone\n  changed example.com/mac\n  added example.com/zeta\n", "run go run ./tools/bin/notices"} {
		if !strings.Contains(stderr.String(), want) {
			t.Errorf("-check says %q, not %q", stderr.String(), want)
		}
	}
	if strings.Contains(stderr.String(), "alpha") || strings.Contains(stderr.String(), "around") {
		t.Errorf("-check names what did not change: %s", stderr.String())
	}
	if after, _ := os.ReadFile(path); string(after) != stale {
		t.Errorf("-check wrote %s", output)
	}

	if err := os.WriteFile(path, current, 0o600); err != nil {
		t.Fatal(err)
	}
	stderr.Reset()
	if code := run(root, true, nil, goWith(t, "Another Go license.\n"), &stderr); code != 1 || !strings.HasSuffix(stderr.String(), "\n  changed Go, the runtime and standard library of every build\n") {
		t.Errorf("-check under a Go whose LICENSE changed = %d, want 1 naming Go alone: %s", code, stderr.String())
	}

	if err := os.WriteFile(path, []byte("A note.\n"+string(current)), 0o600); err != nil {
		t.Fatal(err)
	}
	stderr.Reset()
	if code := run(root, true, nil, goRoot, &stderr); code != 1 || !strings.Contains(stderr.String(), "the text around their notices differs") {
		t.Errorf("-check on an edited preamble = %d: %s", code, stderr.String())
	}
}

// A module replaced by another module is named and read as its
// replacement; one replaced by a folder keeps its path and is read from
// the folder; the main module and the standard library are no module to
// name.
func TestDecode(t *testing.T) {
	stream := `{"ImportPath":"fmt"}
{"ImportPath":"example.com/app/cmd","Module":{"Path":"example.com/app","Main":true,"Dir":"/app"}}
{"ImportPath":"example.com/a","Module":{"Path":"example.com/a","Version":"v1.0.0","Dir":"/cache/b@v1.1.0","Replace":{"Path":"example.com/b","Version":"v1.1.0","Dir":"/cache/b@v1.1.0"}}}
{"ImportPath":"example.com/a/sub","Module":{"Path":"example.com/a","Version":"v1.0.0","Dir":"/cache/b@v1.1.0","Replace":{"Path":"example.com/b","Version":"v1.1.0","Dir":"/cache/b@v1.1.0"}}}
{"ImportPath":"example.com/c","Module":{"Path":"example.com/c","Version":"v0.0.0","Dir":"/app/c","Replace":{"Path":"./c","Dir":"/app/c"}}}
{"ImportPath":"example.com/d","Module":{"Path":"example.com/d","Version":"v2.0.0","Dir":"/cache/d@v2.0.0"}}
`
	got, err := decode(strings.NewReader(stream))
	if err != nil {
		t.Fatal(err)
	}
	want := []module{{"example.com/b", "/cache/b@v1.1.0"}, {"example.com/c", "/app/c"}, {"example.com/d", "/cache/d@v2.0.0"}}
	if len(got) != len(want) {
		t.Fatalf("decode = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("decode = %v, want %v", got, want)
		}
	}
}
