package main

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The scenarios of "Rule: What itos-cc says while it measures" in
// features/coverage.feature. Each runs crap over a/a.go in a small Go module,
// whose coverage runs go test, and reads what stderr says.

// coverageModule makes a Go module whose package a holds a/a.go, with a
// test that passes, or fails when failing is true, and makes it the test's
// directory (useDir).
func coverageModule(t *testing.T, failing bool) {
	t.Helper()
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go is not installed")
	}
	dir := t.TempDir()
	want := "1"
	if failing {
		want = "2"
	}
	for name, text := range map[string]string{
		"go.mod": "module example.com/c\n\ngo 1.22\n",
		"a/a.go": "package a\n\nfunc A(x int) int {\n\tif x > 0 {\n\t\treturn 1\n\t}\n\treturn 0\n}\n",
		"a/a_test.go": "package a\n\nimport \"testing\"\n\nfunc TestA(t *testing.T) {\n\tif A(1) != " + want +
			" {\n\t\tt.Fatal(\"A\")\n\t}\n}\n",
	} {
		writeFile(t, filepath.Join(dir, filepath.FromSlash(name)), text)
	}
	useDir(t, dir)
}

// stderrLines is each line of stderr.
func stderrLines(stderr string) []string {
	return strings.Split(strings.TrimRight(stderr, "\n"), "\n")
}

// @ID-COV-38
func TestTheCoverageCommandIsAnnouncedAsItosCcs(t *testing.T) {
	t.Parallel()
	coverageModule(t, false)

	o := cli(t, "crap", filepath.FromSlash("a/a.go"))
	if o.code != 0 {
		t.Fatalf("exit %d, want 0\n%s%s", o.code, o.stdout, o.stderr)
	}
	announced := false
	for _, l := range stderrLines(o.stderr) {
		if rest, ok := strings.CutPrefix(l, "itos-cc: coverage "); ok {
			if _, command, ok := strings.Cut(rest, "$ "); ok && strings.HasPrefix(command, "go test ") {
				announced = true
			}
		}
	}
	if !announced {
		t.Errorf("stderr:\n%s\nwant a line \"itos-cc: coverage <dir>$ go test …\"", o.stderr)
	}
	for _, l := range stderrLines(o.stderr) {
		if strings.HasPrefix(l, "coverage:") {
			t.Errorf("stderr:\n%s\nholds the line %q, which begins \"coverage:\"", o.stderr, l)
		}
	}
}

// @ID-COV-39
func TestACoverageCommandThatFailsIsReportedAsItosCcs(t *testing.T) {
	t.Parallel()
	coverageModule(t, true)

	o := cli(t, "crap", filepath.FromSlash("a/a.go"))
	reported := false
	for _, l := range stderrLines(o.stderr) {
		if strings.HasPrefix(l, "itos-cc: coverage: go: ") && strings.Contains(l, "exit status") {
			reported = true
		}
	}
	if !reported {
		t.Errorf("stderr:\n%s\nwant a line beginning \"itos-cc: coverage: go: \" with the command's exit status", o.stderr)
	}
}
