package main

import (
	"encoding/json"
	"fmt"
	"maps"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/donvargax/itos-cc/internal/metricsdir"
	mutationslice "github.com/donvargax/itos-cc/internal/slices/mutation"
	"github.com/donvargax/itos-cc/internal/slices/mutation/adapters/snapshots"
	mutation "github.com/donvargax/itos-cc/internal/slices/mutation/domain"
)

// Tests of "Rule: Re-running a sample of cached results" in
// features/mutate.feature. ID-MUT-71 to 76 and 78 to 83 hold their old
// tests whole (T-101); each test left holds an assertion no scenario does.
// Most use the module of mutate_since_test.go, whose snapshots a mutation
// run writes first; mutation sample then re-runs some of their mutants and
// compares. A snapshot that should disagree with
// the tests is rewritten by hand, as the cache the command is there to catch
// would have been. The scenarios that need many mutants use TypeScript files
// and a test command that always passes, so every mutant survives, each run
// is fast, and no test runner is needed.

// mutationSample runs itos-cc mutation sample with args, on one worker.
func mutationSample(t *testing.T, args ...string) outcome {
	t.Helper()
	return itosCc(t, append([]string{"mutation", "sample", "--workers", "1"}, args...)...)
}

// sampledJSON is one entry of a file's "mutants" in mutation sample --json.
type sampledJSON struct {
	File        string `json:"-"`
	Line        int    `json:"line"`
	Column      int    `json:"column"`
	Function    string `json:"function"`
	Original    string `json:"original"`
	Replacement string `json:"replacement"`
	Recorded    string `json:"recorded"`
	Outcome     string `json:"outcome"`
}

// sampledKeys are the keys of each sampled mutant.
var sampledKeys = []string{"column", "function", "line", "original", "outcome", "outcomes", "recorded", "replacement", "scope"}

type sampleJSON struct {
	OK    bool    `json:"ok"`
	Seed  *string `json:"seed"`
	Files []struct {
		File    string         `json:"file"`
		Mutants *[]sampledJSON `json:"mutants"`
	} `json:"files"`
}

// sample is the --json output of mutation sample.
func (o outcome) sample(t *testing.T) sampleJSON {
	t.Helper()
	var out sampleJSON
	if err := json.Unmarshal([]byte(o.stdout), &out); err != nil {
		t.Fatalf("stdout is not one JSON object: %v\n%s\nstderr:\n%s", err, o.stdout, o.stderr)
	}
	return out
}

// sampled is every mutant the --json output lists, file by file.
func (o outcome) sampled(t *testing.T) []sampledJSON {
	t.Helper()
	var all []sampledJSON
	for _, f := range o.sample(t).Files {
		if f.Mutants == nil {
			t.Fatalf("%s has no \"mutants\" in:\n%s", f.File, o.stdout)
		}
		for _, m := range *f.Mutants {
			m.File = f.File
			all = append(all, m)
		}
	}
	return all
}

// recordAs rewrites the snapshot of rel so that function's mutant of
// original is recorded with outcome, its counts to match, as a snapshot
// written by hand would be.
func recordAs(t *testing.T, rel, function, original, outcome string) {
	t.Helper()
	snap, err := loadMutationSnapshot(t.Context(), metricsdir.Store{RootDir: wd(t)}, rel)
	if err != nil || snap == nil {
		t.Fatalf("no snapshot of %s: %v", rel, err)
	}
	found := false
	for i := range snap.Units {
		u := &snap.Units[i]
		if u.Namespace+"#"+u.Name != function {
			continue
		}
		u.Killed, u.Survived, u.Uncovered = 0, 0, 0
		for j := range u.Mutants {
			m := &u.Mutants[j]
			if m.Original == original {
				m.Outcome, found = outcome, true
			}
			switch m.Outcome {
			case mutation.Killed, mutation.Timeout:
				u.Killed++
			case mutation.Survived:
				u.Survived++
			case mutation.Uncovered:
				u.Uncovered++
			}
		}
	}
	if !found {
		t.Fatalf("the snapshot of %s records no mutant of %s in %s", rel, mutationslice.Quote(original), function)
	}
	data, err := json.MarshalIndent(snapshots.SnapshotJSON(*snap), "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(".metrics", "mutate", rel+".json"), string(data)+"\n")
}

// mutantsRun counts the mutants a run says on stderr that it ran.
func mutantsRun(stderr string) int {
	return strings.Count(stderr, "itos-cc: [")
}

// passing is a test command that always passes, on every platform.
const passing = "go version"

// sampleRepo writes five TypeScript files of ten functions, one mutation
// site each, in a git repository it makes the test's directory (useDir), and
// records every mutant survived with a test command that always passes.
func sampleRepo(t *testing.T) {
	t.Helper()
	for _, tool := range []string{"git", "go"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skip(tool + " is not installed")
		}
	}
	dir := t.TempDir()
	writeSampleFiles(t, dir)
	writeFile(t, filepath.Join(dir, "README.md"), "# sample\n")
	gitIn(t, dir, "init", "-q", "-b", "main")
	gitIn(t, dir, "add", "-A")
	gitIn(t, dir, "commit", "-qm", "base")
	useDir(t, dir)
	recordSurvivors(t, 50)
}

func writeSampleFiles(t *testing.T, dir string) {
	t.Helper()
	writeFile(t, filepath.Join(dir, "package.json"), `{"name": "m"}`+"\n")
	for f := 1; f <= 5; f++ {
		var b strings.Builder
		for n := 1; n <= 10; n++ {
			fmt.Fprintf(&b, "export function f%d(a: number): boolean {\n  return a > %d;\n}\n\n", n, n+1)
		}
		writeFile(t, filepath.Join(dir, "src", fmt.Sprintf("part%d.ts", f)), b.String())
	}
}

// recordSurvivors runs every mutant under the test's directory with a test
// command that always passes and checks that want of them were recorded.
func recordSurvivors(t *testing.T, want int) {
	t.Helper()
	o := mutateRun(t, "--json", "--test-command", passing)
	survived := 0
	for _, f := range o.json(t).Files {
		survived += f.Survived
	}
	if survived != want {
		t.Fatalf("the run recorded %d survivors, want %d\n%s%s", survived, want, o.stdout, o.stderr)
	}
}

// head is the id of the test's directory's HEAD commit.
func head(t *testing.T) string {
	t.Helper()
	return gitOut(t, wd(t), "rev-parse", "HEAD")
}

// ids names each sampled mutant by its file, function, and site.
func ids(mutants []sampledJSON) []string {
	var out []string
	for _, m := range mutants {
		out = append(out, fmt.Sprintf("%s:%d:%d %s %s>%s", m.File, m.Line, m.Column, m.Function, m.Original, m.Replacement))
	}
	slices.Sort(out)
	return out
}

// @ID-MUT-77. Kept: the "seed" --json gives is the HEAD commit's id, which
// no scenario asserts; the scenario reads the plain output's.
func TestARerunOfOneCommitSamplesTheSameMutants(t *testing.T) {
	t.Parallel()
	sampleRepo(t)

	first := mutationSample(t, "--json", "--test-command", passing)
	second := mutationSample(t, "--json", "--test-command", passing)
	a, b := ids(first.sampled(t)), ids(second.sampled(t))
	if len(a) != 20 || !slices.Equal(a, b) {
		t.Errorf("the runs sampled:\n%q\nand\n%q\nwant the same 20 mutants", a, b)
	}
	id := head(t)
	if s := first.sample(t).Seed; s == nil || *s != id {
		t.Errorf("seed %v, want the HEAD commit's id %s", s, id)
	}
	plain := mutationSample(t, "--test-command", passing)
	if !strings.Contains(plain.stdout, id) {
		t.Errorf("stdout:\n%s\nwant it to name the seed, %s", plain.stdout, id)
	}
}

// @ID-MUT-84. Kept: each sampled mutant holds exactly its nine keys, where
// the scenario asserts only that each is there.
func TestTheSampleAsJSON(t *testing.T) {
	t.Parallel()
	boardRepo(t, nil)
	if o := mutateRun(t, boardSource); o.code != 1 {
		t.Fatalf("the run: exit %d, want 1 for clear's survivor\n%s%s", o.code, o.stdout, o.stderr)
	}
	recordAs(t, boardSource, placeID, ">", mutation.Survived)

	o := mutationSample(t, "--json", boardSource)
	var top map[string]any
	if err := json.Unmarshal([]byte(o.stdout), &top); err != nil {
		t.Fatalf("stdout is not one JSON object: %v\n%s", err, o.stdout)
	}
	for _, key := range []string{"schema", "ok", "seed", "files"} {
		if _, ok := top[key]; !ok {
			t.Errorf("no %q in:\n%s", key, o.stdout)
		}
	}
	if top["schema"] != 1.0 {
		t.Errorf("schema %v, want 1", top["schema"])
	}
	if got := len(o.sampled(t)); got != 2 {
		t.Errorf("%d mutants listed, want the two sampled", got)
	}
	for _, f := range o.rawFiles(t) {
		list, _ := f["mutants"].([]any)
		for _, m := range list {
			if keys := slices.Sorted(maps.Keys(m.(map[string]any))); !slices.Equal(keys, sampledKeys) {
				t.Errorf("mutant keys %q, want %q", keys, sampledKeys)
			}
		}
	}
	m := o.json(t)
	if !slices.Equal(m.rules(), []string{"mutation.mismatch"}) {
		t.Fatalf("problems %v, want place's mismatch", m.Problems)
	}

	plain := mutationSample(t, boardSource)
	var listed []string
	for _, l := range strings.SplitAfter(plain.stdout, "\n") {
		if strings.HasPrefix(l, "  ") {
			listed = append(listed, l)
		}
	}
	var fromJSON []string
	for _, p := range m.Problems {
		fromJSON = append(fromJSON, fmt.Sprintf("  mismatch %s → %s in %s, %s line %v column %v: recorded %s, now %s\n",
			mutationslice.Quote(p["original"].(string)), mutationslice.Quote(p["replacement"].(string)), p["function"],
			p["file"], p["line"], p["column"], p["recorded"], p["outcome"]))
	}
	if !slices.Equal(listed, fromJSON) {
		t.Errorf("plain output lists:\n%q\nwant the problems of --json:\n%q", listed, fromJSON)
	}
	if plain.code != o.code || o.code != 1 {
		t.Errorf("exit %d plain, %d with --json, want 1 both", plain.code, o.code)
	}
}
