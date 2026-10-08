package main

import (
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/donvargax/itos-cc/mutate"
)

// The scenarios of "Rule: Re-running a sample of cached results" in
// features/mutate.feature. Most use the module of mutate_since_test.go,
// whose snapshots a mutation run writes first; mutation sample then re-runs
// some of their mutants and compares. A snapshot that should disagree with
// the tests is rewritten by hand, as the cache the command is there to catch
// would have been. The scenarios that need many mutants use TypeScript files
// and a test command that always passes, so every mutant survives, each run
// is fast, and no test runner is needed.

// mutationSample runs itos-cc mutation sample with args, on one worker.
func mutationSample(t *testing.T, args ...string) outcome {
	t.Helper()
	return cli(t, append([]string{"mutation", "sample", "--workers", "1"}, args...)...)
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
var sampledKeys = []string{"column", "function", "line", "original", "outcome", "recorded", "replacement", "scope"}

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
	snap, err := mutate.LoadSnapshot(rel)
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
			case mutate.Killed, mutate.Timeout:
				u.Killed++
			case mutate.Survived:
				u.Survived++
			case mutate.Uncovered:
				u.Uncovered++
			}
		}
	}
	if !found {
		t.Fatalf("the snapshot of %s records no mutant of %s in %s", rel, quote(original), function)
	}
	data, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(".metrics", "mutate", rel+".json"), string(data)+"\n")
}

// recorded is the outcome the snapshot of src/board.go records for
// function's mutant of original.
func recorded(t *testing.T, function, original string) string {
	t.Helper()
	for _, m := range boardUnits(t)[function].Mutants {
		if m.Original == original {
			return m.Outcome
		}
	}
	t.Fatalf("the snapshot records no mutant of %s in %s", quote(original), function)
	return ""
}

// mutantsRun counts the mutants a run says on stderr that it ran.
func mutantsRun(stderr string) int {
	return strings.Count(stderr, "itos-cc: [")
}

// passing is a test command that always passes, on every platform.
const passing = "go version"

// sampleRepo writes five TypeScript files of ten functions, one mutation
// site each, in a git repository whose working directory it becomes, and
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
	t.Chdir(dir)
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

// recordSurvivors runs every mutant under the working directory with a test
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

// head is the id of the working directory's HEAD commit.
func head(t *testing.T) string {
	t.Helper()
	out, err := exec.Command("git", "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(out))
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

// @ID-MUT-71
func TestASampleWhoseOutcomesHoldPassesAndWritesNothing(t *testing.T) {
	// Board#place's one mutant is killed and Board#clear's survives.
	dir := boardRepo(t, nil)
	// Every test run of package board appends its working directory to the
	// marker. It is in place before the run, since a test changed after it
	// would make its results stale.
	marker := filepath.Join(dir, "tests-ran")
	appendTo(t, filepath.FromSlash("src/board_test.go"), fmt.Sprintf(`
func init() {
	wd, _ := os.Getwd()
	f, _ := os.OpenFile(%s, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	f.WriteString(wd + "\n")
	f.Close()
}
`, strconv.Quote(filepath.ToSlash(marker))))
	edit(t, filepath.FromSlash("src/board_test.go"), `import "testing"`, "import (\n\t\"os\"\n\t\"testing\"\n)")
	if o := mutateRun(t, boardSource); o.code != 1 {
		t.Fatalf("the run: exit %d, want 1 for clear's survivor\n%s%s", o.code, o.stdout, o.stderr)
	}
	if err := os.Remove(marker); err != nil {
		t.Fatalf("the run's tests wrote no marker: %v", err)
	}
	snapshot := filepath.Join(".metrics", "mutate", "src", "board.go.json")
	snapBefore, err := os.ReadFile(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	srcBefore, err := os.ReadFile(boardSource)
	if err != nil {
		t.Fatal(err)
	}

	o := mutationSample(t, boardSource)
	data, _ := os.ReadFile(marker)
	var ran []string
	for _, wd := range strings.Split(string(data), "\n") {
		if wd != "" {
			ran = append(ran, wd)
		}
	}
	if len(ran) != 3 {
		t.Errorf("the tests ran %d times, want 3: the baseline, then each of the two mutants\nstderr:\n%s", len(ran), o.stderr)
	}
	real, err := os.Stat(filepath.Join(dir, "src"))
	if err != nil {
		t.Fatal(err)
	}
	for _, wd := range ran {
		if info, err := os.Stat(wd); err == nil && os.SameFile(info, real) {
			t.Errorf("the tests ran in %s, the real tree: want a worker's copy", wd)
		}
	}
	baseline, first := strings.Index(o.stderr, "itos-cc: baseline"), strings.Index(o.stderr, "itos-cc: [1/")
	if baseline < 0 || first < 0 || baseline > first {
		t.Errorf("stderr:\n%s\nwant the baseline, then each sampled mutant", o.stderr)
	}
	if _, err := os.Stat(filepath.Join(".metrics", "coverage")); err == nil {
		t.Errorf(".metrics/coverage was written: want no coverage command run")
	}
	if after, _ := os.ReadFile(snapshot); string(after) != string(snapBefore) {
		t.Errorf("the snapshot changed: want mutation sample to write nothing")
	}
	if after, _ := os.ReadFile(boardSource); string(after) != string(srcBefore) {
		t.Errorf("%s changed: want no summary comment written", boardSource)
	}
	if o.code != 0 {
		t.Errorf("exit %d, want 0\n%s%s", o.code, o.stdout, o.stderr)
	}
}

// @ID-MUT-72
func TestARecordedKillThatNowSurvivesIsAMismatch(t *testing.T) {
	// TestPlace checks only place(6), so `>` → `>=` survives.
	boardRepo(t, map[string]string{
		"src/board_test.go": "package board\n\nimport \"testing\"\n\nfunc TestPlace(t *testing.T) {\n\tvar b Board\n\tif !b.place(6) {\n\t\tt.Fatal(\"place\")\n\t}\n}\n",
	})
	if o := mutateRun(t, boardSource); o.code != 1 {
		t.Fatalf("the run: exit %d, want 1 for the survivors\n%s%s", o.code, o.stdout, o.stderr)
	}
	recordAs(t, boardSource, placeID, ">", mutate.Killed)

	o := mutationSample(t, "--json", boardSource)
	m := o.json(t)
	wantProblem(t, m.problem("mutation.mismatch"), map[string]any{"file": boardSource, "line": 7.0, "column": 11.0,
		"function": placeID, "original": ">", "replacement": ">=", "recorded": "killed", "outcome": "survived"}, o.stdout)
	// clear's recorded survivor still survives, which is no mismatch.
	if !slices.Equal(m.rules(), []string{"mutation.mismatch"}) {
		t.Errorf("problems %v, want place's mismatch alone", m.Problems)
	}
	if o.code != 1 {
		t.Errorf("exit %d, want 1", o.code)
	}
}

// @ID-MUT-73
func TestARecordedSurvivorThatIsNowKilledIsAMismatchToo(t *testing.T) {
	boardRepo(t, nil)
	if o := mutateRun(t, boardSource); o.code != 1 {
		t.Fatalf("the run: exit %d, want 1 for clear's survivor\n%s%s", o.code, o.stdout, o.stderr)
	}
	recordAs(t, boardSource, placeID, ">", mutate.Survived)

	o := mutationSample(t, "--json", boardSource)
	m := o.json(t)
	wantProblem(t, m.problem("mutation.mismatch"), map[string]any{"file": boardSource, "function": placeID,
		"recorded": "survived", "outcome": "killed"}, o.stdout)
	if !slices.Equal(m.rules(), []string{"mutation.mismatch"}) {
		t.Errorf("problems %v, want place's mismatch alone", m.Problems)
	}
	if o.code != 1 {
		t.Errorf("exit %d, want 1", o.code)
	}
}

// @ID-MUT-74
func TestKilledAndTimeoutAgree(t *testing.T) {
	// Deleting the ! of place makes it sleep an hour where its test expects
	// it to return at once, so that mutant can only time out, as in
	// @ID-MUT-53; TestClear kills clear's one mutant.
	boardRepo(t, map[string]string{
		"src/board.go": `package board

import "time"

type Board struct{}

// place waits for a board that is not still.
func (b *Board) place(still bool) bool {
	if !still {
		time.Sleep(time.Hour)
	}
	return true
}

func (b *Board) clear(i int) bool {
	return i < 3
}
`,
		"src/board_test.go": `package board

import "testing"

func TestPlace(t *testing.T) {
	var b Board
	if !b.place(true) {
		t.Fatal("place")
	}
}

func TestClear(t *testing.T) {
	var b Board
	if !b.clear(2) || b.clear(3) {
		t.Fatal("clear")
	}
}
`,
	})
	// The timeout is then the baseline's duration plus 5s.
	if o := mutateRun(t, "--timeout-factor", "1", boardSource); o.code != 0 {
		t.Fatalf("the run: exit %d, want every mutant killed or timed out\n%s%s", o.code, o.stdout, o.stderr)
	}
	if got := recorded(t, placeID, "!"); got != mutate.Timeout {
		t.Fatalf("place's mutant of ! is recorded %s, want timeout", got)
	}
	// Recorded killed, it now times out; and a recorded timeout is now
	// killed.
	recordAs(t, boardSource, placeID, "!", mutate.Killed)
	recordAs(t, boardSource, clearID, "<", mutate.Timeout)

	o := mutationSample(t, "--json", "--timeout-factor", "1", boardSource)
	if m := o.json(t); len(m.Problems) != 0 {
		t.Errorf("problems %v, want none", m.Problems)
	}
	pairs := map[string]string{}
	for _, m := range o.sampled(t) {
		pairs[m.Function+" "+m.Original] = m.Recorded + " → " + m.Outcome
	}
	for mutant, want := range map[string]string{placeID + " !": "killed → timeout", clearID + " <": "timeout → killed"} {
		if pairs[mutant] != want {
			t.Errorf("%s: %q, want recorded and outcome %q; sampled %v\n%s", mutant, pairs[mutant], want, pairs, o.stderr)
		}
	}
	if o.code != 0 {
		t.Errorf("exit %d, want 0", o.code)
	}
}

// @ID-MUT-75
func TestOnlyFreshMutantsThatRanAreSampled(t *testing.T) {
	// Under coverage, place's `>` on line 6 survives, as no test tells 9
	// from 10; its `false` on line 7 is uncovered; and its `>` on line 9 is
	// killed. No test executes clear.
	boardRepo(t, map[string]string{
		"src/board.go": `package board

type Board struct{}

func (b *Board) place(i int) bool {
	if i > 9 {
		return false
	}
	return i > 5
}

func (b *Board) clear(i int) bool {
	return i < 3
}
`,
	})
	mutateCovered(t, boardSource)
	if u := boardUnits(t)[placeID]; u.Killed != 1 || u.Survived != 1 || u.Uncovered != 1 {
		t.Fatalf("place in the snapshot: %+v, want one mutant killed, one survived, and one uncovered", u)
	}
	// clear changes; reset is added after the run.
	edit(t, boardSource, "i < 3", "i < 4")
	appendTo(t, boardSource, resetSource)

	o := mutationSample(t, "--json", "--count", "100", boardSource)
	var got []string
	for _, m := range o.sampled(t) {
		if m.Function != placeID {
			t.Errorf("sampled %+v, want only mutants of %s", m, placeID)
		}
		got = append(got, m.Original+" "+m.Recorded)
	}
	slices.Sort(got)
	if want := []string{"> killed", "> survived"}; !slices.Equal(got, want) {
		t.Errorf("sampled %q, want place's killed and survived mutants, %q", got, want)
	}
	if n := mutantsRun(o.stderr); n != 2 {
		t.Errorf("%d mutants ran, want 2\n%s", n, o.stderr)
	}
	for _, p := range o.json(t).Problems {
		if p["function"] == clearID || p["function"] == resetID {
			t.Errorf("problem %v, want none of clear or reset: they are mutation check's", p)
		}
	}
	if o.code != 0 {
		t.Errorf("exit %d, want 0\n%s", o.code, o.stdout)
	}
}

// @ID-MUT-76
func TestCountSaysHowMany20ByDefault(t *testing.T) {
	sampleRepo(t)
	for _, c := range []struct {
		args []string
		want int
	}{
		{nil, 20},
		{[]string{"--count", "5"}, 5},
		{[]string{"--count", "80"}, 50},
	} {
		o := mutationSample(t, append([]string{"--json", "--test-command", passing}, c.args...)...)
		if got := len(o.sampled(t)); got != c.want || mutantsRun(o.stderr) != c.want {
			t.Errorf("%q: sampled %d and ran %d, want %d\n%s", c.args, got, mutantsRun(o.stderr), c.want, o.stdout)
		}
		if o.code != 0 {
			t.Errorf("%q: exit %d, want 0", c.args, o.code)
		}
	}
}

// @ID-MUT-77
func TestARerunOfOneCommitSamplesTheSameMutants(t *testing.T) {
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

// @ID-MUT-78
func TestSeedReproducesARun(t *testing.T) {
	sampleRepo(t)
	first := mutationSample(t, "--json", "--test-command", passing)
	seed := first.sample(t).Seed
	if seed == nil {
		t.Fatalf("no seed in:\n%s", first.stdout)
	}
	edit(t, "README.md", "# sample\n", "# sample\n\nFifty mutants.\n")
	commitAll(t, "describe the sample")
	// At the new HEAD, the draw is another.
	moved := mutationSample(t, "--json", "--test-command", passing)
	if slices.Equal(ids(moved.sampled(t)), ids(first.sampled(t))) {
		t.Fatalf("the new HEAD sampled the mutants of the old one:\n%q", ids(moved.sampled(t)))
	}

	o := mutationSample(t, "--json", "--test-command", passing, "--seed", *seed)
	if got, want := ids(o.sampled(t)), ids(first.sampled(t)); !slices.Equal(got, want) {
		t.Errorf("--seed %s sampled:\n%q\nwant what the run printing it sampled:\n%q", *seed, got, want)
	}
}

// @ID-MUT-79
func TestOutsideAGitRepositoryTheSeedMustBeGiven(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go is not installed")
	}
	dir := t.TempDir()
	t.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(dir))
	writeSampleFiles(t, dir)
	t.Chdir(dir)
	recordSurvivors(t, 50)

	o := mutationSample(t, "--json", "--test-command", passing)
	if p := o.json(t).problem("sample.no-git"); p == nil || o.code != 3 {
		t.Errorf("exit %d, stdout:\n%s\nwant exit 3 and sample.no-git", o.code, o.stdout)
	}
	o = mutationSample(t, "--json", "--test-command", passing, "--seed", "1")
	if n := len(o.sampled(t)); n != 20 || o.code != 0 {
		t.Errorf("--seed 1: exit %d, sampled %d, want exit 0 and 20 sampled\n%s", o.code, n, o.stdout)
	}
}

// @ID-MUT-80
func TestACountBelow1IsAUsageError(t *testing.T) {
	inEmptyDir(t)
	o := mutationSample(t, "--json", "--count", "0")
	wantProblem(t, o.json(t).problem("flags.value-invalid"), map[string]any{"flag": "--count", "value": "0"}, o.stdout)
	if o.code != 2 {
		t.Errorf("exit %d, want 2", o.code)
	}
}

// @ID-MUT-81
func TestWithSinceOnlyTheFunctionsTheRangeChangedAreSampled(t *testing.T) {
	boardRepo(t, killedTests)
	if o := mutateRun(t, boardSource); o.code != 0 {
		t.Fatalf("the first run: exit %d, want every mutant killed\n%s%s", o.code, o.stdout, o.stderr)
	}
	changePlace(t)
	if o := mutateRun(t, "--since", "base"); o.code != 0 {
		t.Fatalf("the run since base: exit %d, want place killed\n%s%s", o.code, o.stdout, o.stderr)
	}
	// Without --since, clear's fresh results are sampled too.
	functions := map[string]bool{}
	for _, m := range mutationSample(t, "--json", "--count", "100").sampled(t) {
		functions[m.Function] = true
	}
	if !functions[clearID] {
		t.Fatalf("without --since, sampled %v, want clear's mutant among them", slices.Collect(maps.Keys(functions)))
	}

	o := mutationSample(t, "--json", "--since", "base", "--count", "100")
	got := o.sampled(t)
	if len(got) == 0 {
		t.Errorf("nothing sampled, want place's mutant\n%s%s", o.stdout, o.stderr)
	}
	for _, m := range got {
		if m.Function != placeID {
			t.Errorf("sampled %+v, want only mutants of %s, the function the range changed", m, placeID)
		}
	}
}

// @ID-MUT-82
func TestNothingToSample(t *testing.T) {
	// No run has recorded anything: every function is missing.
	boardRepo(t, nil)

	o := mutationSample(t)
	if !strings.Contains(o.stderr, "itos-cc: no cached mutant to sample\n") {
		t.Errorf("stderr:\n%s\nwant it to say there is no cached mutant to sample", o.stderr)
	}
	if strings.Contains(o.stderr, "baseline") {
		t.Errorf("stderr:\n%s\nwant no test command run", o.stderr)
	}
	if o.code != 0 {
		t.Errorf("exit %d, want 0", o.code)
	}
}

// @ID-MUT-83
func TestAFailingBaselineInASample(t *testing.T) {
	// The tests fail when BOARD_BROKEN is set, which changes no file whose
	// hash the snapshot records, so its results stay fresh.
	boardRepo(t, map[string]string{
		"src/board_test.go": `package board

import (
	"os"
	"testing"
)

func TestPlace(t *testing.T) {
	if os.Getenv("BOARD_BROKEN") != "" {
		t.Fatal("broken")
	}
	var b Board
	if b.place(5) || !b.place(6) {
		t.Fatal("place")
	}
}
`,
	})
	if o := mutateRun(t, boardSource); o.code != 1 {
		t.Fatalf("the run: exit %d, want 1 for clear's survivor\n%s%s", o.code, o.stdout, o.stderr)
	}
	t.Setenv("BOARD_BROKEN", "1")

	o := mutationSample(t, "--json", boardSource)
	wantProblem(t, o.json(t).problem("mutation.baseline-failed"), map[string]any{"file": boardSource}, o.stdout)
	if n := mutantsRun(o.stderr); n != 0 {
		t.Errorf("%d mutants ran, want none\n%s", n, o.stderr)
	}
	if got := o.sampled(t); len(got) != 0 {
		t.Errorf("mutants %+v, want none listed: none ran", got)
	}
	if o.code != 1 {
		t.Errorf("exit %d, want 1", o.code)
	}
}

// @ID-MUT-84
func TestTheSampleAsJSON(t *testing.T) {
	boardRepo(t, nil)
	if o := mutateRun(t, boardSource); o.code != 1 {
		t.Fatalf("the run: exit %d, want 1 for clear's survivor\n%s%s", o.code, o.stdout, o.stderr)
	}
	recordAs(t, boardSource, placeID, ">", mutate.Survived)

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
		fromJSON = append(fromJSON, fmt.Sprintf("  mismatch %s:%v:%v %s → %s in %s: recorded %s, now %s\n",
			p["file"], p["line"], p["column"], quote(p["original"].(string)), quote(p["replacement"].(string)),
			p["function"], p["recorded"], p["outcome"]))
	}
	if !slices.Equal(listed, fromJSON) {
		t.Errorf("plain output lists:\n%q\nwant the problems of --json:\n%q", listed, fromJSON)
	}
	if plain.code != o.code || o.code != 1 {
		t.Errorf("exit %d plain, %d with --json, want 1 both", plain.code, o.code)
	}
}

func TestAMismatchsFixRerunsInItsScope(t *testing.T) {
	for scope, want := range map[string]string{
		"own":          "itos-cc mutation run --mutate-all src/b.ts",
		"listed":       "itos-cc mutation run --mutate-all src/b.ts",
		"all-tests":    "itos-cc mutation run --mutate-all --all-tests src/b.ts",
		"make test":    "itos-cc mutation run --mutate-all --test-command 'make test' src/b.ts",
		"echo 'a b' x": `itos-cc mutation run --mutate-all --test-command 'echo '\''a b'\'' x' src/b.ts`,
	} {
		if got := rerunIn(scope, "src/b.ts"); got != want {
			t.Errorf("scope %q: %q, want %q", scope, got, want)
		}
	}
}
