package main

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// The line-order scenario of "Rule: Mutation sites" in
// features/mutate.feature. src/app.ts holds routes, whose sites sit on line 2,
// before the route callback app.get("/u", …), and on line 7, after it; the
// callback is a unit of its own, with its sites on line 4.

var appSource = filepath.FromSlash("src/app.ts")

const appText = `export function routes(app, a) {
  if (a > 1) {
    app.get("/u", (req, res) => {
      return req.x < 2;
    });
  }
  return a === 0;
}
`

// appRepo makes a TypeScript project holding src/app.ts in a git repository,
// and makes it the test's directory (useDir).
func appRepo(t *testing.T) {
	t.Helper()
	for _, tool := range []string{"git", "go"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skip(tool + " is not installed")
		}
	}
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "package.json"), `{"name": "m"}`+"\n")
	writeFile(t, filepath.Join(dir, appSource), appText)
	gitIn(t, dir, "init", "-q", "-b", "main")
	gitIn(t, dir, "add", "-A")
	gitIn(t, dir, "commit", "-qm", "base")
	useDir(t, dir)
}

// siteLine is how plain mutation list prints a site.
func siteLine(s mutateSite) string {
	return fmt.Sprintf("%s:%d:%d %s → %s in %s", s.File, s.Line, s.Column, quote(s.Original), quote(s.Replacement), s.Function)
}

// @ID-MUT-101
func TestSitesAreListedInLineOrderInlineCallbacksIncluded(t *testing.T) {
	t.Parallel()
	appRepo(t)

	sites := scanned(t, appSource)
	byLine := map[int]string{} // the function of each line's sites
	for _, s := range sites {
		if f, ok := byLine[s.Line]; ok && f != s.Function {
			t.Fatalf("sites %+v, want each line's sites in one function", sites)
		}
		byLine[s.Line] = s.Function
	}
	if len(byLine) != 3 || byLine[2] == "" || byLine[2] != byLine[7] || byLine[4] == "" || byLine[4] == byLine[2] {
		t.Fatalf("sites %+v, want those of lines 2 and 7 in routes and those of line 4 in the callback", sites)
	}

	o := cli(t, "mutation", "list", appSource)
	var want []string
	for _, s := range slices.SortedStableFunc(slices.Values(sites), func(a, b mutateSite) int {
		if a.Line != b.Line {
			return a.Line - b.Line
		}
		return a.Column - b.Column
	}) {
		want = append(want, siteLine(s))
	}
	if got := strings.Split(strings.TrimSuffix(o.stdout, "\n"), "\n"); !slices.Equal(got, want) {
		t.Errorf("mutation list printed:\n%s\nwant, in line and column order:\n%s", o.stdout, strings.Join(want, "\n"))
	}
	if o.code != 0 {
		t.Errorf("mutation list: exit %d, want 0\n%s", o.code, o.stderr)
	}

	r := mutateRun(t, "--json", "--test-command", passing, appSource)
	var mutants []string
	for _, m := range r.json(t).file(t, appSource).mutants(t) {
		mutants = append(mutants, siteLine(mutateSite{File: appSource, Line: m.Line, Column: m.Column,
			Function: m.Function, Original: m.Original, Replacement: m.Replacement}))
	}
	var listed []string
	for _, s := range sites {
		listed = append(listed, siteLine(s))
	}
	if !slices.Equal(listed, mutants) {
		t.Errorf("mutation list --json lists:\n%s\nwant the order of mutation run --json's \"mutants\":\n%s",
			strings.Join(listed, "\n"), strings.Join(mutants, "\n"))
	}
}
