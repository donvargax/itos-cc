package main

import (
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// The scenarios of slice-6, itos-cc.yaml's exclude, in
// features/mutate.feature, which godog does not run yet; those of
// file_selection.feature (ID-SEL-17 to 19) run through godog (T-62). Each
// uses the module of mutate_since_test.go with a fake beside it in a
// package of its own, src/port/porttest, which no test reaches, and reads
// the --json objects as raw maps and the itos-cc.yaml it writes as text,
// so the tests compile against a product that has no exclude yet.

// fakeFile is the fake, a source of test support, by its path from the
// module's root; fakeSource is the same path as itos-cc names it.
const fakeFile = "src/port/porttest/fake.go"

var fakeSource = filepath.FromSlash(fakeFile)

// fakeText is the fake. Its line 7 has three mutation sites, the first the
// 0 at column 9.
const fakeText = `package porttest

// Fake stands in for a board in tests.
type Fake struct{ n int }

func (f *Fake) Over(i int) bool {
	return 0 < i-f.n
}
`

// excludePorttest is an itos-cc.yaml that excludes every porttest package.
const excludePorttest = "exclude: [\"**/porttest/**\"]\n"

// fakeException is an entry of itos-cc.yaml excepting the survivor at
// line 7, column 9 of the fake, written as a person or mutation except
// writes one.
const fakeException = `mutation:
  exceptions:
    - file: src/port/porttest/fake.go
      function: example.com/m/src/port/porttest.Fake#Over
      hash: "0000000000000000"
      line_in_function: 2
      column: 9
      original: "0"
      replacement: "1"
      reason: the fake's bound is never read
`

// withFake is the files of every map in extra, the fake added.
func withFake(extra ...map[string]string) map[string]string {
	files := map[string]string{fakeFile: fakeText}
	for _, m := range extra {
		maps.Copy(files, m)
	}
	return files
}

// problemOf is the problem of rule in o's --json object, nil when it has
// none.
func problemOf(t *testing.T, o outcome, rule string) map[string]any {
	t.Helper()
	var out struct {
		Problems []map[string]any `json:"problems"`
	}
	if err := json.Unmarshal([]byte(o.stdout), &out); err != nil {
		t.Fatalf("stdout is not one JSON object: %v\n%s\nstderr:\n%s", err, o.stdout, o.stderr)
	}
	for _, p := range out.Problems {
		if p["rule"] == rule {
			return p
		}
	}
	return nil
}

// orphansOf is the "orphans" of o's --json object.
func orphansOf(t *testing.T, o outcome) []map[string]any {
	t.Helper()
	var out struct {
		Orphans []map[string]any `json:"orphans"`
	}
	if err := json.Unmarshal([]byte(o.stdout), &out); err != nil {
		t.Fatalf("stdout is not one JSON object: %v\n%s\nstderr:\n%s", err, o.stdout, o.stderr)
	}
	return out.Orphans
}

// wantExcludedOrphan fails t unless orphans name the fake's snapshot, with
// why excluded.
func wantExcludedOrphan(t *testing.T, what string, o outcome) {
	t.Helper()
	for _, orphan := range orphansOf(t, o) {
		if orphan["file"] == fakeSource {
			if orphan["why"] != "excluded" {
				t.Errorf("%s names the fake's snapshot %v, want \"why\": \"excluded\"", what, orphan)
			}
			return
		}
	}
	t.Errorf("%s names no orphan %s, want its snapshot with \"why\": \"excluded\"\n%s\nstderr:\n%s", what, fakeSource, o.stdout, o.stderr)
}

// @ID-MUT-243
func TestAGateJudgesNoExcludedSourceTheRangeChanged(t *testing.T) {
	t.Parallel()
	boardRepo(t, withFake(map[string]string{"itos-cc.yaml": excludePorttest}))
	gitIn(t, ".", "checkout", "-qb", "work")
	edit(t, fakeSource, "0 < i-f.n", "1 < i-f.n")
	commitAll(t, "change the fake")

	run := itosCc(t, "mutation", "run", "--since", "main", "--fail-uncovered", "--no-annotate", "--workers", "1")
	if want := "itos-cc: no source files to mutate"; !strings.Contains(run.stderr, want) {
		t.Errorf("mutation run --since main --fail-uncovered: stderr does not say %q\nstdout:\n%s\nstderr:\n%s", want, run.stdout, run.stderr)
	}
	if run.code != 0 {
		t.Errorf("mutation run --since main --fail-uncovered: exit %d, want 0", run.code)
	}
	if check := mutationCheck(t, "--since", "main", "--fail-uncovered"); check.code != 0 {
		t.Errorf("mutation check --since main --fail-uncovered: exit %d, want 0\nstdout:\n%s\nstderr:\n%s", check.code, check.stdout, check.stderr)
	}
	list := itosCc(t, "mutation", "list", "--json")
	var sites struct {
		Sites []map[string]any `json:"sites"`
	}
	if err := json.Unmarshal([]byte(list.stdout), &sites); err != nil {
		t.Fatalf("mutation list --json: stdout is not one JSON object: %v\n%s", err, list.stdout)
	}
	for _, s := range sites.Sites {
		if s["file"] == fakeSource {
			t.Errorf("mutation list names the site %v in %s, which itos-cc.yaml excludes", s, fakeSource)
		}
	}
	if len(sites.Sites) == 0 {
		t.Errorf("mutation list names no site, want those of %s\n%s", boardSource, list.stderr)
	}
	sample := itosCc(t, "mutation", "sample", "--count", "5", "--json")
	if strings.Contains(sample.stdout, "fake.go") {
		t.Errorf("mutation sample --count 5 names %s, which itos-cc.yaml excludes:\n%s", fakeSource, sample.stdout)
	}
}

// @ID-MUT-244
func TestTheSnapshotOfASourceNewlyExcludedIsAnOrphan(t *testing.T) {
	t.Parallel()
	boardRepo(t, withFake(killedTests))
	mutateRun(t)
	snapshot := filepath.Join(".metrics", "mutate", fakeSource+".json")
	if _, err := os.Stat(inWD(t, snapshot)); err != nil {
		t.Fatalf("mutation run wrote no snapshot of %s: %v", fakeSource, err)
	}
	writeFile(t, "itos-cc.yaml", excludePorttest)

	check := mutationCheck(t, "--json")
	wantExcludedOrphan(t, "mutation check --json", check)
	if check.code != 0 {
		t.Errorf("mutation check --json: exit %d, want 0\n%s\nstderr:\n%s", check.code, check.stdout, check.stderr)
	}
	run := mutateRun(t, "--json")
	wantExcludedOrphan(t, "mutation run --json", run)
	if _, err := os.Stat(inWD(t, snapshot)); err == nil {
		t.Errorf("mutation run left %s, the snapshot of a source itos-cc.yaml excludes", snapshot)
	}
}

// @ID-MUT-245
func TestAnExceptionNamingAnExcludedSourceIsStaleAndNoneCanBeAdded(t *testing.T) {
	t.Parallel()
	boardRepo(t, withFake(killedTests, map[string]string{"itos-cc.yaml": excludePorttest + fakeException}))
	mutateRun(t, boardSource)

	check := mutationCheck(t, "--json")
	stale := problemOf(t, check, "mutation.exception-stale")
	if stale == nil || stale["why"] != "excluded" {
		t.Errorf("mutation check --json reports %v, want mutation.exception-stale with \"why\": \"excluded\"\n%s", stale, check.stdout)
	}

	site := fakeFile + ":7:9"
	plain := itosCc(t, "mutation", "except", site, "--reason", "x")
	if plain.code != 2 {
		t.Errorf("mutation except %s: exit %d, want 2\n%s%s", site, plain.code, plain.stdout, plain.stderr)
	}
	o := itosCc(t, "mutation", "except", site, "--reason", "x", "--json")
	p := problemOf(t, o, "exception.excluded")
	if p == nil {
		t.Fatalf("mutation except %s --json reports no exception.excluded\n%s", site, o.stdout)
	}
	if p["file"] != fakeSource {
		t.Errorf("exception.excluded %v, want \"file\": %q", p, fakeSource)
	}
	if message, _ := p["message"].(string); message == "" || !strings.Contains(plain.stderr, message) {
		t.Errorf("plain stderr does not print exception.excluded's message %q:\n%s", message, plain.stderr)
	}
	if o.code != 2 {
		t.Errorf("mutation except %s --json: exit %d, want 2", site, o.code)
	}
	data, err := os.ReadFile(inWD(t, "itos-cc.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if got := string(data); got != excludePorttest+fakeException {
		t.Errorf("mutation except rewrote itos-cc.yaml:\n%s", got)
	}
}

// @ID-MUT-246
func TestACountedRunSelectsNoSiteInAnExcludedSource(t *testing.T) {
	t.Parallel()
	requireCountedPlatform(t)
	boardRepo(t, withFake(map[string]string{"itos-cc.yaml": excludePorttest}))

	o := countedRun(t, "--count", "20")
	c := o.counted(t)
	var files []string
	for _, s := range c.Selected {
		files = append(files, fmt.Sprint(s["file"]))
	}
	if slices.ContainsFunc(files, func(f string) bool { return strings.HasSuffix(filepath.ToSlash(f), fakeFile) }) {
		t.Errorf("mutation run --count 20 selects sites in %s, which HEAD's itos-cc.yaml excludes: %v", fakeFile, files)
	}
	if len(files) == 0 {
		t.Errorf("mutation run --count 20 selects no site, want those of %s\n%s\nstderr:\n%s", boardSource, o.stdout, o.stderr)
	}
}
