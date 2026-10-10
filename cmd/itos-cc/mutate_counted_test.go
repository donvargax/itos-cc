package main

import (
	"bytes"
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

	"github.com/donvargax/itos-cc/mutate"
)

// The scenarios of "Rule: Fresh counted mutation judges a bounded committed
// selection without claiming full proof" in features/mutate.feature that the
// mutation-counted-run slice holds. Each reads the --json object as raw maps,
// so the steps compile against a product that has no counted mode yet. Local
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
	return cli(t, append([]string{"mutation", "run", "--json"}, args...)...)
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
func seedSelecting(t *testing.T, repo string, count int, want func([]mutate.FreshCandidate) bool) string {
	t.Helper()
	for i := 0; i < 500; i++ {
		seed := fmt.Sprintf("seed-%d", i)
		plan, err := mutate.PlanFresh(repo, nil, "", count, seed)
		if err != nil {
			t.Fatal(err)
		}
		ok := want(plan.Selected)
		if err := plan.Close(); err != nil {
			t.Fatal(err)
		}
		if ok {
			return seed
		}
	}
	t.Fatal("no seed selects the wanted sites")
	return ""
}

func inFunction(function string) func(mutate.FreshCandidate) bool {
	return func(c mutate.FreshCandidate) bool { return strings.HasSuffix(c.Function, "#"+function) }
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

// @ID-MUT-173
func TestCountedExecutionHasExplicitAdmissionAndPlatformBoundaries(t *testing.T) {
	dir := moduleRepo(t, untestedFiles)
	for _, args := range [][]string{{"--count", "0"}, {"--count=-2"}, {"--count", "two"}} {
		o := countedRun(t, args...)
		p := o.counted(t).problem("flags.value-invalid")
		if o.code != 2 || p == nil || p["flag"] != "--count" {
			t.Errorf("mutation run %v: exit %d, want a --count usage error before any trial\n%s%s", args, o.code, o.stdout, o.stderr)
		}
	}
	o := countedRun(t, "--seed", "abc")
	if p := o.counted(t).problem("flags.conflict"); o.code != 2 || p == nil || p["flag"] != "--seed" {
		t.Errorf("--seed without --count: exit %d, want a --seed usage conflict\n%s%s", o.code, o.stdout, o.stderr)
	}
	if _, err := os.Stat(filepath.Join(dir, ".metrics")); !os.IsNotExist(err) {
		t.Errorf("a refused counted run wrote .metrics: %v", err)
	}

	if runtime.GOOS == "windows" {
		o := countedRun(t, "--count", "1")
		if p := o.counted(t).problem("count.platform"); o.code != 3 || p == nil {
			t.Errorf("counted mode on %s: exit %d, want count.platform before launching commands\n%s%s", runtime.GOOS, o.code, o.stdout, o.stderr)
		}
		if strings.Contains(o.stderr, "itos-cc: coverage") || strings.Contains(o.stderr, "itos-cc: baseline") {
			t.Errorf("counted mode launched commands on %s:\n%s", runtime.GOOS, o.stderr)
		}
		return
	}

	// The same refusal on Linux and macOS, with the platform pretended away.
	useEnv(t, platformEnv, "windows")
	o = countedRun(t, "--count", "1")
	useEnv(t, platformEnv, "")
	if p := o.counted(t).problem("count.platform"); o.code != 3 || p == nil || strings.Contains(o.stderr, "itos-cc: coverage") {
		t.Errorf("counted mode on an unsupported platform: exit %d, want count.platform before launching commands\n%s%s", o.code, o.stdout, o.stderr)
	}

	// Outside any Git repository, and in one whose HEAD is unborn.
	plain := t.TempDir()
	writeFile(t, filepath.Join(plain, "go.mod"), "module example.com/plain\n\ngo 1.22\n")
	writeFile(t, filepath.Join(plain, "main.go"), "package main\n\nfunc Half(i int) bool { return i > 2 }\n\nfunc main() {}\n")
	t.Chdir(plain)
	o = countedRun(t, "--count", "1")
	if p := o.counted(t).problem("count.no-git"); o.code != 3 || p == nil {
		t.Errorf("counted mode outside Git: exit %d, want count.no-git\n%s%s", o.code, o.stdout, o.stderr)
	}
	gitIn(t, plain, "init", "-q")
	o = countedRun(t, "--count", "1")
	if p := o.counted(t).problem("count.no-git"); o.code != 3 || p == nil {
		t.Errorf("counted mode with an unborn HEAD: exit %d, want count.no-git\n%s%s", o.code, o.stdout, o.stderr)
	}
	if _, err := os.Stat(filepath.Join(plain, ".metrics")); !os.IsNotExist(err) {
		t.Errorf("a refused counted run wrote .metrics: %v", err)
	}
}

// @ID-MUT-174
func TestCountedExecutionEvaluatesFrozenCommittedInputs(t *testing.T) {
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
	if p := o.counted(t).problem("count.unsupported-scope"); o.code != 2 || p == nil {
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

// @ID-MUT-175
func TestFreshSelectionIsReproducibleAcrossPathsAndSameNameUnits(t *testing.T) {
	requireCountedPlatform(t)
	dir := moduleRepo(t, map[string]string{
		"go.mod":     "module example.com/same\n\ngo 1.22\n",
		"main.go":    "package main\n\nfunc main() {}\n",
		"a/value.go": "package a\n\nfunc Value(x int) int { return x + 1 }\n",
		"b/value.go": "package b\n\ntype T struct{}\n\nfunc (T) Value(x int) int { return x - 1 }\n\nfunc Value(x int) int { return x * 2 }\n",
	})
	head := gitOut(t, dir, "rev-parse", "HEAD")

	// Static checks, no trial: every eligible site is fully identified, and
	// another seed can be checked to draw another site.
	plan, err := mutate.PlanFresh(dir, nil, "", 1, "fixed")
	if err != nil {
		t.Fatal(err)
	}
	identities := map[string]bool{}
	for _, c := range plan.Eligible {
		identities[c.Identity] = true
	}
	fixed := plan.Selected[0].Identity
	plan.Close()
	if len(identities) != 5 {
		t.Fatalf("eligible identities %v, want 5 distinct sites across same-name units", identities)
	}
	seedSelecting(t, dir, 1, func(s []mutate.FreshCandidate) bool { return s[0].Identity != fixed })

	first := countedRun(t, "--count", "1", "--seed", "fixed").counted(t)
	reordered := countedRun(t, "--count", "1", "--seed", "fixed", "b", "a").counted(t)
	if len(first.Selected) != 1 || len(reordered.Selected) != 1 {
		t.Fatalf("selected %+v and %+v, want one site each", first.Selected, reordered.Selected)
	}
	if first.Selected[0]["identity"] != fixed || reordered.Selected[0]["identity"] != fixed {
		t.Errorf("selected %v and %v, want the planned %s whatever the path order", first.Selected[0]["identity"], reordered.Selected[0]["identity"], fixed)
	}
	if first.Sampling["seed"] != "fixed" || first.Sampling["algorithm"] != mutate.FreshPlanVersion {
		t.Errorf("sampling = %+v, want seed fixed and algorithm %s", first.Sampling, mutate.FreshPlanVersion)
	}
	if first.number("executed") != 0 {
		t.Errorf("untested selection executed %d trials, want 0", first.number("executed"))
	}
	byDefault := countedRun(t, "--count", "1").counted(t)
	if byDefault.Sampling["seed"] != head {
		t.Errorf("default seed %v, want the resolved HEAD %s", byDefault.Sampling["seed"], head)
	}
}

// @ID-MUT-176
func TestTheGlobalFreshBudgetHoldsEvenWhenEverySelectedMutantIsKilled(t *testing.T) {
	requireCountedPlatform(t)
	dir := moduleRepo(t, map[string]string{
		"go.mod":      "module example.com/budget\n\ngo 1.22\n",
		"main.go":     "package main\n\nfunc main() {}\n",
		"a/a.go":      "package a\n\nfunc Less(x, y int) bool { return x < y }\n",
		"a/a_test.go": "package a\n\nimport \"testing\"\n\nfunc TestLess(t *testing.T) {\n\tif !Less(1, 2) || Less(2, 2) {\n\t\tt.Fatal(\"Less\")\n\t}\n}\n",
		"b/b.go":      "package b\n\nfunc Add(x, y int) int { return x + y }\n\nfunc Sub(x, y int) int { return x - y }\n",
		"b/b_test.go": "package b\n\nimport \"testing\"\n\nfunc TestB(t *testing.T) {\n\tif Add(2, 3) != 5 || Sub(5, 3) != 2 {\n\t\tt.Fatal(\"b\")\n\t}\n}\n",
	})
	// A complete run caches a/a.go's one kill: one trial.
	if o := mutateCovered(t, filepath.FromSlash("a/a.go")); o.code != 0 {
		t.Fatalf("complete run of a/a.go: exit %d\n%s%s", o.code, o.stdout, o.stderr)
	}
	snapshot := filepath.Join(dir, ".metrics", "mutate", "a", "a.go.json")
	cached, err := os.ReadFile(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	// A later commit adds a function the cache has never seen.
	writeFile(t, filepath.Join(dir, "a", "a.go"), "package a\n\nfunc Less(x, y int) bool { return x < y }\n\nfunc Twice(x int) int { return x * 2 }\n")
	gitIn(t, dir, "add", "a/a.go")
	gitIn(t, dir, "commit", "-qm", "twice")
	seed := seedSelecting(t, dir, 1, func(s []mutate.FreshCandidate) bool { return inFunction("Less")(s[0]) })

	o := countedRun(t, "--count", "1", "--seed", seed, "--workers", "4")
	c := o.counted(t)
	if o.code != 0 {
		t.Fatalf("counted run: exit %d\n%s%s", o.code, o.stdout, o.stderr)
	}
	if c.number("eligible") != 4 || c.number("selected") != 1 || c.number("executed") != 1 || c.number("omitted") != 3 || c.number("budget") != 1 {
		t.Errorf("sampling = %+v, want budget 1, 4 eligible (missing and stale entries admitted), 1 selected, 1 executed, 3 omitted", c.Sampling)
	}
	if len(c.Selected) != 1 || c.Selected[0]["outcome"] != "killed" || c.Selected[0]["state"] != "judged" || c.Selected[0]["reused"] == true {
		t.Errorf("selected = %+v, want Less's mutant judged fresh and killed", c.Selected)
	}
	if n := trialLines(o.stderr); n != 1 {
		t.Errorf("%d mutant trials ran, want exactly 1 across the whole selection:\n%s", n, o.stderr)
	}
	if omitted := c.Subjects["omitted"]; len(omitted) != 3 {
		t.Errorf("omitted subjects %+v, want Twice, Add and Sub", omitted)
	}
	if now, err := os.ReadFile(snapshot); err != nil || !bytes.Equal(now, cached) {
		t.Errorf("the cached snapshot changed: %v", err)
	}
}

// @ID-MUT-177
func TestCountedModeLeavesCompleteAndCachedSampleContractsIntact(t *testing.T) {
	help := cli(t, "mutation", "run", "--help").stdout
	for _, want := range []string{"--count N", "--seed TEXT", "--since REF", "--fail-uncovered", "--all-tests"} {
		if !strings.Contains(help, want) {
			t.Errorf("mutation run --help lacks %q", want)
		}
	}
	sampleHelp := cli(t, "mutation", "sample", "--help").stdout
	if !strings.Contains(sampleHelp, "--count N") || !strings.Contains(sampleHelp, "how many mutants to run, at least 1") {
		t.Errorf("mutation sample --help changed its --count:\n%s", sampleHelp)
	}

	dir := moduleRepo(t, compareFiles)
	// Without the opt-in, a complete run caches and annotates: one trial.
	o := cli(t, "mutation", "run", "--json", "--workers", "1", "main.go")
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
	sample := cli(t, "mutation", "sample", "--json", "--count", "5", "--workers", "1")
	if sample.code != 0 || len(sample.sampled(t)) != 1 {
		t.Errorf("cached sample: exit %d\n%s%s", sample.code, sample.stdout, sample.stderr)
	}

	// With no cache, sample has nothing to sample, while counted mode draws
	// from the committed sites: measured uncovered here, so no trial.
	moduleRepo(t, untestedFiles)
	nothing := cli(t, "mutation", "sample", "--json")
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

// @ID-MUT-180
func TestMeasuredUncoveredSelectionIsExplicitAndIsNeverRedrawn(t *testing.T) {
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

// @ID-MUT-184
func TestMachineAndTextOutputDistinguishSampledWorkFromPopulationCompleteness(t *testing.T) {
	requireCountedPlatform(t)
	dir := moduleRepo(t, withFiles(compareFiles, map[string]string{
		"main.go": "package main\n\nfunc Compare(n int) bool { return n > 5 }\n\nfunc Half(i int) bool { return i > 2 }\n\nfunc Third(i int) int { return i / 3 }\n\nfunc main() {}\n",
	}))
	head := gitOut(t, dir, "rev-parse", "HEAD")
	seed := seedSelecting(t, dir, 2, func(s []mutate.FreshCandidate) bool {
		return slices.ContainsFunc(s, inFunction("Compare")) && !slices.ContainsFunc(s, inFunction("Third"))
	})
	o := countedRun(t, "--count", "2", "--seed", seed, "--workers", "1")
	c := o.counted(t) // one JSON object, or it fails here
	if o.code != 0 {
		t.Fatalf("counted run: exit %d\n%s%s", o.code, o.stdout, o.stderr)
	}
	want := map[string]int{"budget": 2, "eligible": 3, "selected": 2, "executed": 1, "omitted": 1}
	for key, n := range want {
		if c.number(key) != n {
			t.Errorf("sampling.%s = %v, want %d", key, c.Sampling[key], n)
		}
	}
	if c.Sampling["commit"] != head || c.Sampling["seed"] != seed || c.Sampling["algorithm"] != mutate.FreshPlanVersion {
		t.Errorf("provenance = %+v", c.Sampling)
	}
	if c.Sampling["assurance"] != "sampled" || c.Sampling["completion"] != "completed" {
		t.Errorf("assurance %v completion %v, want sampled and completed", c.Sampling["assurance"], c.Sampling["completion"])
	}
	outcomes := map[string]string{}
	for _, s := range c.Selected {
		for _, key := range []string{"identity", "file", "line", "column", "function", "original", "replacement", "state"} {
			if _, ok := s[key]; !ok {
				t.Errorf("selected site lacks %q: %+v", key, s)
			}
		}
		state, _ := s["state"].(string)
		outcome, has := s["outcome"].(string)
		if state != "judged" && state != "uncovered" && has {
			t.Errorf("%s site has a fabricated outcome %q", state, outcome)
		}
		function, _ := s["function"].(string)
		outcomes[function] = outcome
	}
	if outcomes["example.com/counted#Compare"] != "killed" || outcomes["example.com/counted#Half"] != "uncovered" {
		t.Errorf("outcomes = %v, want Compare killed and Half uncovered", outcomes)
	}
	judged, omitted := c.Subjects["judged"], c.Subjects["omitted"]
	if len(judged) != 2 || len(omitted) != 1 || omitted[0]["function"] != "example.com/counted#Third" {
		t.Errorf("subjects judged %+v omitted %+v", judged, omitted)
	}
	for _, name := range []string{"coverage", "baseline", "trials"} {
		if s := c.stage(name); s == nil || s["state"] != "complete" {
			t.Errorf("stage %s = %+v in %+v", name, s, c.Stages)
		}
	}

	// Plain output, on a selection that runs no trial.
	moduleRepo(t, untestedFiles)
	text := cli(t, "mutation", "run", "--count", "1")
	for _, want := range []string{"sampled", "not a complete", "bounds mutant trials, not discovery"} {
		if !strings.Contains(text.stdout, want) {
			t.Errorf("plain output lacks %q:\n%s", want, text.stdout)
		}
	}
}

// @ID-MUT-185
func TestSampledExecutionPublishesNoPersistentCompleteProof(t *testing.T) {
	requireCountedPlatform(t)
	dir := moduleRepo(t, map[string]string{
		"go.mod":      "module example.com/publish\n\ngo 1.22\n",
		"main.go":     "package main\n\nfunc main() {}\n",
		"x/x.go":      "package x\n\nfunc Less(x, y int) bool { return x < y }\n",
		"x/x_test.go": "package x\n\nimport \"testing\"\n\nfunc TestLess(t *testing.T) {\n\tif !Less(1, 2) || Less(2, 2) {\n\t\tt.Fatal(\"Less\")\n\t}\n}\n",
		"y/y.go":      "package y\n\nfunc Mix(x, y int) int { return x*y + 1 }\n",
		"y/y_test.go": "package y\n\nimport \"testing\"\n\nfunc TestMix(t *testing.T) {\n\tif Mix(2, 3) != 7 {\n\t\tt.Fatal(\"Mix\")\n\t}\n}\n",
	})
	// A genuine complete cache of x/x.go, annotated: one trial.
	if o := cli(t, "mutation", "run", "--workers", "1", filepath.FromSlash("x/x.go")); o.code != 0 {
		t.Fatalf("complete run of x/x.go: exit %d\n%s%s", o.code, o.stdout, o.stderr)
	}
	gitIn(t, dir, "add", "x/x.go")
	gitIn(t, dir, "commit", "-qm", "annotated")
	before := tree(t, dir)

	o := countedRun(t, "--count", "1", "--workers", "1", "y")
	if c := o.counted(t); o.code != 0 || c.number("executed") != 1 {
		t.Fatalf("count-one run of y: exit %d\n%s%s", o.code, o.stdout, o.stderr)
	}
	if after := tree(t, dir); !maps2Equal(before, after) {
		t.Errorf("counted run published files:\nbefore %v\nafter  %v", keys(before), keys(after))
	}
	check := cli(t, "mutation", "check", "--json", filepath.FromSlash("y/y.go"))
	if p := check.json(t).problem("mutation.missing"); check.code != 1 || p == nil {
		t.Errorf("complete check of y/y.go: exit %d, want its missing complete proof\n%s", check.code, check.stdout)
	}
	if strings.Contains(check.stderr, "itos-cc: coverage") || strings.Contains(check.stderr, "itos-cc: baseline") {
		t.Errorf("mutation check ran tests:\n%s", check.stderr)
	}
	if valid := cli(t, "mutation", "check", filepath.FromSlash("x/x.go")); valid.code != 0 {
		t.Errorf("the genuine complete cache of x/x.go no longer holds: exit %d\n%s%s", valid.code, valid.stdout, valid.stderr)
	}
}

// @ID-MUT-186
func TestOnlyAGenuinelySiteFreeRangeIsNotApplicable(t *testing.T) {
	requireCountedPlatform(t)
	moduleRepo(t, map[string]string{
		"go.mod":  "module example.com/sitefree\n\ngo 1.22\n",
		"main.go": "package main\n\nfunc Name() string { return \"x\" }\n\nfunc Dormant(flag bool) string {\n\tif flag {\n\t\treturn \"yes\"\n\t}\n\treturn \"no\"\n}\n\nfunc main() {}\n",
	})
	o := countedRun(t, "--count", "3")
	c := o.counted(t)
	if o.code != 0 || c.Sampling["completion"] != "not-applicable" || c.number("eligible") != 0 || c.number("selected") != 0 {
		t.Errorf("site-free range: exit %d sampling %+v, want not applicable with 0 eligible and selected\n%s", o.code, c.Sampling, o.stderr)
	}
	if c.number("executed") != 0 || trialLines(o.stderr) != 0 {
		t.Errorf("site-free range ran trials:\n%s", o.stderr)
	}
	if text := cli(t, "mutation", "run", "--count", "3"); !strings.Contains(text.stdout, "not applicable") {
		t.Errorf("plain output does not say not applicable:\n%s", text.stdout)
	}
	// Strict obligations still apply to a site-free range.
	strict := countedRun(t, "--count", "3", "--fail-uncovered")
	if p := strict.counted(t).problem("mutation.uncovered-statement"); strict.code != 1 || p == nil {
		t.Errorf("strict site-free range: exit %d, want Dormant's uncovered statement\n%s", strict.code, strict.stdout)
	}

	// A range with sites whose tests cannot run is no site-free success.
	moduleRepo(t, withFiles(compareFiles, map[string]string{
		"main_test.go": "package main\n\nimport \"testing\"\n\nfunc TestCompare(t *testing.T) { Compare(undefined) }\n",
	}))
	broken := countedRun(t, "--count", "1")
	b := broken.counted(t)
	if p := b.problem("count.preparation-failed"); broken.code != 1 || p == nil || p["stage"] == nil {
		t.Errorf("failing measurement: exit %d, want count.preparation-failed naming its stage\n%s", broken.code, broken.stdout)
	}
	if b.Sampling["completion"] == "not-applicable" || b.number("executed") != 0 {
		t.Errorf("failing measurement reported %+v", b.Sampling)
	}
}
