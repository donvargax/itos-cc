package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"testing"

	mutation "github.com/donvargax/itos-cc/internal/mutants"
)

// The scenarios of "Rule: Fresh counted mutation judges a bounded committed
// selection without claiming full proof" in features/mutation.feature that the
// mutation-counted-run slice holds, whose holders still assert what their
// scenarios do not. Each reads the --json object as raw maps. Local
// trials stay sparse: most fixtures have no test reaching the selected site,
// so its measured-uncovered judgment runs no mutant at all.

// countedJSON is the counted mode's --json object.
type countedJSON struct {
	OK       bool                        `json:"ok"`
	Sampling map[string]any              `json:"sampling"`
	Selected []map[string]any            `json:"selected"`
	Subjects map[string][]map[string]any `json:"subjects"`
	Stages   []map[string]any            `json:"stages"`
	Files    []map[string]any            `json:"files"`
	Problems []map[string]any            `json:"problems"`
}

func (o outcome) counted(t *testing.T) countedJSON {
	t.Helper()
	var out countedJSON
	if err := json.Unmarshal([]byte(o.stdout), &out); err != nil {
		t.Fatalf("stdout is not one JSON object: %v\n%s\nstderr:\n%s", err, o.stdout, o.stderr)
	}
	return out
}

func (c countedJSON) problem(rule string) map[string]any {
	for _, p := range c.Problems {
		if p["rule"] == rule {
			return p
		}
	}
	return nil
}

func (c countedJSON) problems(rule string) []map[string]any {
	var out []map[string]any
	for _, p := range c.Problems {
		if p["rule"] == rule {
			out = append(out, p)
		}
	}
	return out
}

func (c countedJSON) number(key string) int {
	n, _ := c.Sampling[key].(float64)
	return int(n)
}

func (c countedJSON) stage(name string) map[string]any {
	for _, s := range c.Stages {
		if s["name"] == name {
			return s
		}
	}
	return nil
}

// countedRun runs mutation run --json with args, one worker unless args
// say otherwise.
func countedRun(t *testing.T, args ...string) outcome {
	t.Helper()
	return itosCc(t, append([]string{"mutation", "run", "--json"}, args...)...)
}

// requireCountedPlatform skips where counted mode refuses to run: it runs
// on Linux and macOS, and Windows is #29.
func requireCountedPlatform(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("counted mutation run supports Linux and macOS; native Windows support is #29")
	}
}

func gitOut(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git %v: %v", args, err)
	}
	return strings.TrimSpace(string(out))
}

// seedSelecting finds, statically and without running a test, a seed whose
// count-site selection want accepts.
func seedSelecting(t *testing.T, repo string, count int, want func([]mutation.FreshCandidate) bool) string {
	t.Helper()
	for i := 0; i < 500; i++ {
		seed := fmt.Sprintf("seed-%d", i)
		plan, err := planFreshForTest(t.Context(), repo, nil, "", count, seed)
		if err != nil {
			t.Fatal(err)
		}
		ok := want(plan.Selected)
		releaseFreshPlan(t.Context(), plan)
		if ok {
			return seed
		}
	}
	t.Fatal("no seed selects the wanted sites")
	return ""
}

func inFunction(function string) func(mutation.FreshCandidate) bool {
	return func(c mutation.FreshCandidate) bool { return strings.HasSuffix(c.Function, "#"+function) }
}

// tree is every regular file under dir but .git, with its bytes.
func tree(t *testing.T, dir string) map[string]string {
	t.Helper()
	files := map[string]string{}
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && d.Name() == ".git" {
			return filepath.SkipDir
		}
		if d.Type().IsRegular() {
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			rel, _ := filepath.Rel(dir, path)
			files[filepath.ToSlash(rel)] = string(data)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

// trialLines counts the progress lines of mutant trials on stderr.
func trialLines(stderr string) int {
	return len(regexp.MustCompile(`(?m)^itos-cc: \[\d+/\d+\]`).FindAllString(stderr, -1))
}

var (
	// compareFiles has one site, Compare's `>`, which TestCompare kills.
	compareFiles = map[string]string{
		"go.mod":       "module example.com/counted\n\ngo 1.22\n",
		"main.go":      "package main\n\nfunc Compare(n int) bool { return n > 5 }\n\nfunc main() {}\n",
		"main_test.go": "package main\n\nimport \"testing\"\n\nfunc TestCompare(t *testing.T) {\n\tif !Compare(6) || Compare(5) {\n\t\tt.Fatal(\"Compare\")\n\t}\n}\n",
	}
	// untestedFiles has two sites, in Half and Third, and no test at all, so
	// every site is measured uncovered and no mutant ever runs. Dormant has
	// executable statements and no site, Empty neither, and Win is built only
	// on Windows.
	untestedFiles = map[string]string{
		"go.mod":           "module example.com/untested\n\ngo 1.22\n",
		"main.go":          "package main\n\nfunc Half(i int) bool { return i > 2 }\n\nfunc Third(i int) int { return i / 3 }\n\nfunc Dormant(flag bool) string {\n\tif flag {\n\t\treturn \"yes\"\n\t}\n\treturn \"no\"\n}\n\nfunc Empty() {}\n\nfunc main() {}\n",
		"other_windows.go": "package main\n\nfunc Win(flag bool) string {\n\tif flag {\n\t\treturn \"w\"\n\t}\n\treturn \"x\"\n}\n",
	}
)

func withFiles(base map[string]string, extra map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range base {
		out[k] = v
	}
	for k, v := range extra {
		out[k] = v
	}
	return out
}

// @ID-MUT-174. Kept: the run leaves the staged paths as they were, and the
// refused committed symlink writes no .metrics, which no scenario asserts:
// ID-MUT-174's compares the files outside .git and its refusal's commands
// (T-130).
func TestCountedExecutionEvaluatesFrozenCommittedInputs(t *testing.T) {
	t.Parallel()
	requireCountedPlatform(t)
	dir := moduleRepo(t, compareFiles)
	head := gitOut(t, dir, "rev-parse", "HEAD")
	// The live tree differs from the commit in every way: an unstaged
	// source edit, a staged test that does not compile, an untracked source
	// that is not Go, and an untracked config whose list command fails.
	writeFile(t, filepath.Join(dir, "main.go"), "package main\n\nfunc Compare(n int) bool { return n >= 0 }\n\nfunc main() {}\n")
	writeFile(t, filepath.Join(dir, "main_test.go"), "package main\n\nfunc broken( {\n")
	gitIn(t, dir, "add", "main_test.go")
	writeFile(t, filepath.Join(dir, "extra.go"), "package main\n\nthis is not Go\n")
	writeFile(t, filepath.Join(dir, "itos-cc.yaml"), "mutation:\n  tests:\n    list: \"exit 7\"\n    run: \"exit 7 {pattern}\"\n    ids_pattern: \"{ids}\"\n    join: {each: \"{id}\", sep: \",\"}\n")
	before := tree(t, dir)
	staged := gitOut(t, dir, "diff", "--cached", "--name-only")

	o := countedRun(t, "--count", "1", "--workers", "1")
	c := o.counted(t)
	if o.code != 0 || !c.OK {
		t.Fatalf("count-one run over committed inputs: exit %d\n%s%s", o.code, o.stdout, o.stderr)
	}
	if c.Sampling["commit"] != head {
		t.Errorf("report names commit %v, want HEAD %s", c.Sampling["commit"], head)
	}
	if len(c.Selected) != 1 || c.Selected[0]["file"] != "main.go" || c.Selected[0]["outcome"] != "killed" || c.number("executed") != 1 {
		t.Errorf("selected = %+v, executed %d; want main.go's committed site killed by the committed test", c.Selected, c.number("executed"))
	}
	if after := tree(t, dir); !maps2Equal(before, after) {
		t.Errorf("the live working tree changed:\nbefore %v\nafter  %v", keys(before), keys(after))
	}
	if now := gitOut(t, dir, "diff", "--cached", "--name-only"); now != staged {
		t.Errorf("staged paths changed from %q to %q", staged, now)
	}

	// A committed symlink is an unsupported scope: refused, never followed.
	link := moduleRepo(t, compareFiles)
	if err := os.Symlink("main.go", filepath.Join(link, "alias.go")); err != nil {
		t.Skip("symlinks unavailable: ", err)
	}
	gitIn(t, link, "add", "alias.go")
	gitIn(t, link, "commit", "-qm", "alias")
	o = countedRun(t, "--count", "1")
	if p := o.counted(t).problem("count.unsupported-scope"); o.code != 80 || p == nil {
		t.Errorf("committed symlink: exit %d, want count.unsupported-scope\n%s%s", o.code, o.stdout, o.stderr)
	}
	if _, err := os.Stat(filepath.Join(link, ".metrics")); !os.IsNotExist(err) {
		t.Errorf("an unsupported counted run wrote .metrics: %v", err)
	}
}

func maps2Equal(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if w, ok := b[k]; !ok || w != v {
			return false
		}
	}
	return true
}

func keys(m map[string]string) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

// @ID-MUT-177. Kept: mutation run --help names --seed TEXT, --since REF,
// --fail-uncovered and --all-tests, and mutation sample --help keeps its
// --count's meaning, which no scenario asserts: ID-MUT-177's reads only
// --count N in each (T-130).
func TestCountedModeLeavesCompleteAndCachedSampleContractsIntact(t *testing.T) {
	t.Parallel()
	useDir(t, t.TempDir())
	help := itosCc(t, "mutation", "run", "--help").stdout
	for _, want := range []string{"--count N", "--seed TEXT", "--since REF", "--fail-uncovered", "--all-tests"} {
		if !strings.Contains(help, want) {
			t.Errorf("mutation run --help lacks %q", want)
		}
	}
	sampleHelp := itosCc(t, "mutation", "sample", "--help").stdout
	if !strings.Contains(sampleHelp, "--count N") || !strings.Contains(sampleHelp, "how many mutants to run, at least 1") {
		t.Errorf("mutation sample --help changed its --count:\n%s", sampleHelp)
	}

	dir := moduleRepo(t, compareFiles)
	// Without the opt-in, a complete run caches and annotates: one trial.
	o := itosCc(t, "mutation", "run", "--json", "--workers", "1", "main.go")
	if o.code != 0 || len(o.json(t).Files) != 1 {
		t.Fatalf("complete run: exit %d\n%s%s", o.code, o.stdout, o.stderr)
	}
	if _, err := os.Stat(filepath.Join(dir, ".metrics", "mutate", "main.go.json")); err != nil {
		t.Errorf("complete run wrote no snapshot: %v", err)
	}
	if source, _ := os.ReadFile(filepath.Join(dir, "main.go")); !strings.Contains(string(source), "itos-cc") {
		t.Errorf("complete run did not annotate main.go:\n%s", source)
	}
	// Cached sample rechecks its recorded outcome: one trial.
	sample := itosCc(t, "mutation", "sample", "--json", "--count", "5", "--workers", "1")
	if sample.code != 0 || len(sample.sampled(t)) != 1 {
		t.Errorf("cached sample: exit %d\n%s%s", sample.code, sample.stdout, sample.stderr)
	}

	// With no cache, sample has nothing to sample, while counted mode draws
	// from the committed sites: measured uncovered here, so no trial.
	moduleRepo(t, untestedFiles)
	nothing := itosCc(t, "mutation", "sample", "--json")
	if nothing.code != 0 || len(nothing.sample(t).Files) != 0 || !strings.Contains(nothing.stderr, "no cached mutant to sample") {
		t.Errorf("sample without a cache: exit %d\n%s%s", nothing.code, nothing.stdout, nothing.stderr)
	}
	if runtime.GOOS != "windows" {
		c := countedRun(t, "--count", "1").counted(t)
		if c.number("eligible") != 2 || c.number("selected") != 1 || c.Sampling["completion"] == "not-applicable" {
			t.Errorf("counted mode without a cache: %+v, want 2 eligible committed sites, 1 selected", c.Sampling)
		}
	}
}

// @ID-MUT-180. Kept: a function only Windows builds is
// mutation.coverage-missing in a strict counted run, which no scenario
// asserts. ID-MUT-173, 175, 176 and 184 to 186's holders went, every
// assertion theirs or another scenario's, ID-MUT-173's platform guard to
// mutation_platform_test.go (T-130).
func TestMeasuredUncoveredSelectionIsExplicitAndIsNeverRedrawn(t *testing.T) {
	t.Parallel()
	requireCountedPlatform(t)
	moduleRepo(t, untestedFiles)
	o := countedRun(t, "--count", "1", "--seed", "fixed")
	c := o.counted(t)
	if o.code != 0 {
		t.Fatalf("uncovered selection without --fail-uncovered: exit %d\n%s%s", o.code, o.stdout, o.stderr)
	}
	if len(c.Selected) != 1 || c.Selected[0]["outcome"] != "uncovered" || c.Selected[0]["state"] != "uncovered" {
		t.Fatalf("selected = %+v, want its one site reported uncovered", c.Selected)
	}
	if c.number("executed") != 0 || c.number("selected") != 1 || c.number("eligible") != 2 || c.Sampling["completion"] == "not-applicable" {
		t.Errorf("sampling = %+v, want 2 eligible, 1 selected, 0 executed, not a site-free range", c.Sampling)
	}
	identity := c.Selected[0]["identity"]

	o = countedRun(t, "--count", "1", "--seed", "fixed", "--fail-uncovered")
	strict := o.counted(t)
	if o.code != 1 {
		t.Errorf("--fail-uncovered: exit %d, want 1\n%s%s", o.code, o.stdout, o.stderr)
	}
	if len(strict.Selected) != 1 || strict.Selected[0]["identity"] != identity || strict.number("executed") != 0 {
		t.Errorf("selected %+v executed %d, want the same %v redrawn never and run never", strict.Selected, strict.number("executed"), identity)
	}
	if p := strict.problem("mutation.uncovered"); p == nil || p["file"] != "main.go" {
		t.Errorf("no mutation.uncovered for the selected site: %+v", strict.Problems)
	}
	functions := map[string]map[string]bool{}
	for _, rule := range []string{"mutation.uncovered-statement", "mutation.coverage-missing"} {
		functions[rule] = map[string]bool{}
		for _, p := range strict.problems(rule) {
			function, _ := p["function"].(string)
			functions[rule][function] = true
		}
	}
	if !functions["mutation.uncovered-statement"]["example.com/untested#Dormant"] {
		t.Errorf("zero-site Dormant is not checked: %+v", strict.Problems)
	}
	if functions["mutation.uncovered-statement"]["example.com/untested#Empty"] || functions["mutation.coverage-missing"]["example.com/untested#Empty"] {
		t.Errorf("empty-bodied Empty has an obligation: %+v", strict.Problems)
	}
	if !functions["mutation.coverage-missing"]["example.com/untested#Win"] {
		t.Errorf("Win, built only on Windows, is not coverage-missing: %+v", strict.Problems)
	}
	if strict.stage("strict-go-inventory") == nil || strict.stage("strict-go-inventory")["state"] != "complete" {
		t.Errorf("a missing-evidence finding did not leave the strict inventory complete: %+v", strict.Stages)
	}
}
