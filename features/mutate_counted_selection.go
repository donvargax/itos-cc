package features

import (
	"context"
	"fmt"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/cucumber/godog"
)

func (q *countedMutation) selectionSteps(sc *godog.ScenarioContext) {
	sc.Step(`^committed functions include same-name units and more eligible sites than the budget$`, q.sameNames)
	sc.Step(`^two count-one runs use the same TEXT seed and committed inputs$`, q.reordered)
	sc.Step(`^they select the same fully identified site regardless of discovery ordering$`, q.sameSelection)
	sc.Step(`^a default seed is the resolved HEAD commit ID$`, q.defaultSeed)
	sc.Step(`^reports include the seed and deterministic selection algorithm version$`, q.selectionProvenance)
	sc.Step(`^alternate-seed selection can be checked without executing mutation trials$`, q.alternateSeed)
	sc.Step(`^a seeded selected site is reached by neither own nor listed tests$`, q.uncoveredProject)
	sc.Step(`^count-one preparation measures its coverage successfully$`, q.judge)
	sc.Step(`^that site is reported uncovered with zero mutation trials executed$`, q.uncovered)
	sc.Step(`^selection remains unchanged rather than drawing another site$`, q.notRedrawn)
	sc.Step(`^the run is not described as a range with no eligible sites$`, q.applicable)
	sc.Step(`^fail-uncovered fails that judgment$`, q.strictUncovered)
	sc.Step(`^strict Go still checks executable zero-site functions and exempts empty bodies$`, q.zeroSiteObligations)
	sc.Step(`^a supported committed selection contains no eligible mutation sites$`, q.siteFree)
	sc.Step(`^counted execution completes its applicable admission and strict obligations$`, func() error { var err error; q.primary, err = q.count("--count", "3", "--workers", "1"); return err })
	sc.Step(`^the report explicitly says not applicable with zero eligible and selected sites$`, q.notApplicable)
	sc.Step(`^it does not run an avoidable mutation prepass$`, func() error { return q.noTrials(q.primary) })
	sc.Step(`^absent cache, missing tests, unsupported input scope and failed coverage or baseline are not site-free success$`, q.notSiteFreeFailures)
}

func (q *countedMutation) measureProfile(expected string) error {
	w := q.e.m.w
	if err := w.d.Coverage(w.dir, Coverage{Language: "go", Report: expected}); err != nil {
		return err
	}
	code, out, log, err := w.capture(w.dir, w.env(), "crap", "--json", "--coverage-command", "go test -count=1 -coverprofile=T81.observed ./...", "--coverage-report", "T81.observed")
	if err != nil {
		return err
	}
	if code != 0 && code != 1 {
		return fmt.Errorf("the Go profile observation exit%d: %s%s", code, out, log)
	}
	data, err := w.d.Read(filepath.Join(w.dir, "T81.observed"))
	if err != nil {
		return err
	}
	lines := func(text string) []string {
		a := strings.Split(strings.TrimSpace(text), "\n")
		slices.Sort(a)
		return a
	}
	if !slices.Equal(lines(string(data)), lines(expected)) {
		return fmt.Errorf("the Go profile declaration differs from actual compiler inventory:\n%s\nexpected:\n%s", data, expected)
	}
	if err = w.d.Remove(filepath.Join(w.dir, "T81.observed")); err != nil {
		return err
	}
	return nil
}
func (q *countedMutation) sameNames() error {
	if err := q.project("package main\n"); err != nil {
		return err
	}
	w := q.e.m.w
	files := map[string]string{"main.go": "package main\n", "a/value.go": countedSameA, "b/value.go": countedSameB}
	for _, file := range []string{"main.go", "a/value.go", "b/value.go"} {
		pkg := "main"
		if file != "main.go" {
			pkg = strings.Split(file, "/")[0]
		}
		files[strings.TrimSuffix(file, ".go")+"_test.go"] = q.unreachedTests(pkg, file)
	}
	for file, text := range files {
		if err := w.write(file, text); err != nil {
			return err
		}
	}
	if err := w.d.Git(w.dir, "add", "main.go", "main_test.go", "a/value.go", "a/value_test.go", "b/value.go", "b/value_test.go"); err != nil {
		return err
	}
	if err := w.d.Git(w.dir, "commit", "-qm", "same-name counted sites"); err != nil {
		return err
	}
	for i, pkg := range []string{".", "./a", "./b"} {
		source := []string{"main.go", "a/value.go", "b/value.go"}[i]
		if err := w.d.Tests(w.dir, Tests{Language: "go", Command: "go test -count=1 -failfast " + pkg, Elapsed: 100 * time.Millisecond, Trace: &TestTrace{File: filepath.Join(q.e.outside, "trace.jsonl"), Source: source, Coverage: true}}); err != nil {
			return err
		}
	}
	if err := q.originalHashes("main.go", "a/value.go", "b/value.go"); err != nil {
		return err
	}
	if err := q.auditProfile(countedSameProfile); err != nil {
		return err
	}
	sites, err := q.observe("mutation", "list", "--json")
	if err != nil {
		return err
	}
	if sites.result.Exit != 0 || len(objectList(sites.object, "sites")) != 5 {
		return fmt.Errorf("same-name public site inventory: %v", sites.object)
	}
	q.saved["sites"] = sites
	return nil
}
func (q *countedMutation) reordered() error {
	first, err := q.count("--count", "1", "--seed", q.seed, "--workers", "1")
	if err != nil {
		return err
	}
	second, err := q.count("--count", "1", "--seed", q.seed, "--workers", "1", "b", "a")
	if err != nil {
		return err
	}
	q.primary = first
	q.saved["reordered"] = second
	return nil
}
func (q *countedMutation) sameSelection() error {
	a, err := countedSelected(q.primary)
	if err != nil {
		return err
	}
	b, err := countedSelected(q.saved["reordered"])
	if err != nil {
		return err
	}
	if q.primary.result.Exit != 0 || q.saved["reordered"].result.Exit != 0 || !reflect.DeepEqual(a, b) || a["identity"] == "" || a["identity"] == nil {
		return fmt.Errorf("same-name deterministic selected identity: %v vs%v", a, b)
	}
	matches := 0
	for _, site := range objectList(q.saved["sites"].object, "sites") {
		same := true
		for _, key := range []string{"file", "line", "column", "function", "original", "replacement"} {
			same = same && site[key] == a[key]
		}
		if same {
			matches++
		}
	}
	if matches != 1 || countedSampling(q.primary)["eligible"] != float64(5) {
		return fmt.Errorf("fully identified selected site does not match exactly one public site: %v", a)
	}
	return q.noTrials(q.primary)
}
func (q *countedMutation) defaultSeed(ctx context.Context) error {
	head, err := q.e.m.w.d.Head(ctx, q.e.m.w.dir)
	if err != nil {
		return err
	}
	o, err := q.count("--count", "1", "--workers", "1")
	if err != nil {
		return err
	}
	if o.result.Exit != 0 || countedSampling(o)["commit"] != head || countedSampling(o)["seed"] != head {
		return fmt.Errorf("resolved HEAD default seed: %v", o.object)
	}
	q.saved["default"] = o
	return q.noTrials(o)
}
func (q *countedMutation) selectionProvenance() error {
	for _, o := range []countedObservation{q.primary, q.saved["reordered"]} {
		s := countedSampling(o)
		if s["seed"] != q.seed || s["algorithm"] != "fresh-plan-v2" {
			return fmt.Errorf("counted seed/algorithm provenance: %v", s)
		}
	}
	return nil
}
func (q *countedMutation) alternateSeed() error {
	wanted, err := countedSelected(q.primary)
	if err != nil {
		return err
	}
	population, err := q.count("--count", "5", "--seed", q.seed, "--workers", "1")
	if err != nil {
		return err
	}
	identities := map[string]bool{}
	for _, site := range objectList(population.object, "selected") {
		id, ok := site["identity"].(string)
		if !ok || id == "" || identities[id] || site["outcome"] != "uncovered" {
			return fmt.Errorf("distinct complete public identity population: %v", population.object)
		}
		identities[id] = true
	}
	if len(identities) != 5 || population.result.Exit != 0 {
		return fmt.Errorf("same-name public selection population: %v", population.object)
	}
	if err = q.noTrials(population); err != nil {
		return err
	}
	for i := 0; i < 32; i++ {
		o, err := q.count("--count", "1", "--seed", fmt.Sprintf("T81-alternate-%d", i), "--workers", "1")
		if err != nil {
			return err
		}
		if o.result.Exit != 0 {
			return fmt.Errorf("alternate seed query failed: %v", o.object)
		}
		if err = q.noTrials(o); err != nil {
			return err
		}
		site, err := countedSelected(o)
		if err != nil {
			return err
		}
		if !identities[fmt.Sprint(site["identity"])] {
			return fmt.Errorf("alternate selection not in actual population: %v", site)
		}
		if site["identity"] != wanted["identity"] {
			return nil
		}
	}
	return fmt.Errorf("alternate public seeds did not select a different eligible identity")
}
func (q *countedMutation) uncoveredProject() error {
	if err := q.project(countedZeroSource); err != nil {
		return err
	}
	if err := q.setMode("uncovered"); err != nil {
		return err
	}
	return q.auditProfile(countedZeroProfile)
}
func (q *countedMutation) uncovered() error {
	site, err := countedSelected(q.primary)
	if err != nil {
		return err
	}
	stage, err := countedStage(q.primary, "coverage")
	if err != nil {
		return err
	}
	if q.primary.result.Exit != 0 || site["outcome"] != "uncovered" || site["state"] != "uncovered" || stage["state"] != "complete" {
		return fmt.Errorf("measured uncovered judgment: %v", q.primary.object)
	}
	return q.noTrials(q.primary)
}
func (q *countedMutation) notRedrawn() error {
	o, err := q.count("--count", "1", "--seed", q.seed, "--workers", "1")
	if err != nil {
		return err
	}
	a, err := countedSelected(q.primary)
	if err != nil {
		return err
	}
	b, err := countedSelected(o)
	if err != nil {
		return err
	}
	if o.result.Exit != 0 || !reflect.DeepEqual(a, b) || countedSampling(o)["selected"] != float64(1) {
		return fmt.Errorf("uncovered selection was redrawn: %v vs%v", a, b)
	}
	return q.noTrials(o)
}
func (q *countedMutation) applicable() error {
	s := countedSampling(q.primary)
	if s["eligible"] != float64(1) || s["selected"] != float64(1) || s["completion"] == "not-applicable" {
		return fmt.Errorf("uncovered range incorrectly not-applicable: %v", s)
	}
	return nil
}
func (q *countedMutation) strictUncovered() error {
	o, err := q.count("--count", "1", "--seed", q.seed, "--workers", "1", "--fail-uncovered")
	if err != nil {
		return err
	}
	q.saved["strict"] = o
	if o.result.Exit != 1 || o.object["ok"] != false {
		return fmt.Errorf("strict uncovered exit: %v", o.object)
	}
	a, err := countedSelected(q.primary)
	if err != nil {
		return err
	}
	b, err := countedSelected(o)
	if err != nil {
		return err
	}
	found := false
	for _, p := range checkProblems(o.object) {
		if p["rule"] == "mutation.uncovered" && p["file"] == a["file"] && p["function"] == a["function"] && p["line"] == a["line"] && p["column"] == a["column"] {
			found = true
		}
	}
	if !found || a["identity"] != b["identity"] || b["outcome"] != "uncovered" {
		return fmt.Errorf("strict selected uncovered attribution: %v", o.object)
	}
	return q.noTrials(o)
}
func (q *countedMutation) zeroSiteObligations() error {
	o := q.saved["strict"]
	dormant := false
	for _, p := range checkProblems(o.object) {
		if p["file"] != "main.go" {
			return fmt.Errorf("unexpected strict fixture file: %v", p)
		}
		if strictNamed(fmt.Sprint(p["function"]), "Dormant") && p["rule"] == "mutation.uncovered-statement" {
			dormant = true
		}
		if strictNamed(fmt.Sprint(p["function"]), "Empty") {
			return fmt.Errorf("empty body has strict obligation: %v", p)
		}
	}
	stage, err := countedStage(o, "strict-go-inventory")
	if err != nil {
		return err
	}
	if !dormant || stage["state"] != "complete" {
		return fmt.Errorf("zero-site executable inventory missing: %v", o.object)
	}
	return nil
}
func (q *countedMutation) siteFree() error {
	if err := q.project(countedSiteFreeSource); err != nil {
		return err
	}
	w := q.e.m.w
	if err := w.write("main_test.go", q.unreachedTests("main", "main.go")); err != nil {
		return err
	}
	if err := w.d.Git(w.dir, "add", "main_test.go"); err != nil {
		return err
	}
	if err := w.d.Git(w.dir, "commit", "-qm", "site-free tests"); err != nil {
		return err
	}
	if err := w.d.Tests(w.dir, Tests{Language: "go", Command: q.e.command, Elapsed: 100 * time.Millisecond, Trace: &TestTrace{File: filepath.Join(q.e.outside, "trace.jsonl"), Source: "main.go", Coverage: true}}); err != nil {
		return err
	}
	return q.auditProfile(countedSiteFreeProfile)
}
func (q *countedMutation) notApplicable() error {
	s := countedSampling(q.primary)
	if q.primary.result.Exit != 0 || s["completion"] != "not-applicable" || s["eligible"] != float64(0) || s["selected"] != float64(0) || len(objectList(q.primary.object, "selected")) != 0 {
		return fmt.Errorf("site-free applicability: %v", q.primary.object)
	}
	w := q.e.m.w
	code, out, log, err := w.capture(w.dir, w.env(), "mutation", "run", "--count", "3")
	if err != nil {
		return err
	}
	if code != 0 || !strings.Contains(out, "not applicable") {
		return fmt.Errorf("site-free plain applicability exit%d: %s%s", code, out, log)
	}
	strict, err := q.count("--count", "3", "--fail-uncovered")
	if err != nil {
		return err
	}
	found := false
	for _, p := range checkProblems(strict.object) {
		if p["rule"] == "mutation.uncovered-statement" && strictNamed(fmt.Sprint(p["function"]), "Dormant") {
			found = true
		}
	}
	if strict.result.Exit != 1 || !found {
		return fmt.Errorf("site-free strict obligations erased: %v", strict.object)
	}
	return nil
}
func (q *countedMutation) notSiteFreeFailures() error {
	for _, mode := range []string{"covered", "uncovered", "coverage-fail", "baseline-fail"} {
		other, err := q.otherProject(ownedOneSource)
		if err != nil {
			return err
		}
		if mode == "uncovered" {
			w := other.e.m.w
			if err = w.d.Remove(filepath.Join(w.dir, "main_test.go")); err != nil {
				return err
			}
			if err = w.d.Git(w.dir, "add", "main_test.go"); err != nil {
				return err
			}
			if err = w.d.Git(w.dir, "commit", "-qm", "no tests"); err != nil {
				return err
			}
			if err = w.d.Tests(w.dir, Tests{Language: "go", Command: other.e.command}); err != nil {
				return err
			}
			if err = w.d.Coverage(w.dir, Coverage{Language: "go", Report: strings.ReplaceAll(countedOneProfile, " 1 1\n", " 1 0\n")}); err != nil {
				return err
			}
		} else if err = other.setMode(mode); err != nil {
			return err
		}
		o, err := other.count("--count", "1", "--workers", "1")
		if err != nil {
			return err
		}
		if countedSampling(o)["completion"] == "not-applicable" || countedSampling(o)["eligible"] != float64(1) {
			return fmt.Errorf("%s conflated with site-free range: %v", mode, o.object)
		}
		if mode == "covered" || mode == "uncovered" {
			if o.result.Exit != 0 {
				return fmt.Errorf("%s no-cache/no-tests result: %v", mode, o.object)
			}
		} else {
			if err = countedProblem(o, "count.preparation-failed", 1); err != nil {
				return err
			}
			want := "coverage"
			if mode == "baseline-fail" {
				want = "baseline"
			}
			if checkProblems(o.object)[0]["stage"] != want {
				return fmt.Errorf("%s failed preparation stage: %v", mode, o.object)
			}
		}
	}
	other, err := q.otherProject(ownedOneSource)
	if err != nil {
		return err
	}
	w := other.e.m.w
	if err = w.d.Link(filepath.Join(w.dir, "main.go"), filepath.Join(w.dir, "alias.go")); err != nil {
		return err
	}
	if err = w.d.Git(w.dir, "add", "alias.go"); err != nil {
		return err
	}
	if err = w.d.Git(w.dir, "commit", "-qm", "unsupported alias"); err != nil {
		return err
	}
	o, err := other.count("--count", "1")
	if err != nil {
		return err
	}
	if err = countedProblem(o, "count.unsupported-scope", 2); err != nil {
		return err
	}
	return strictNoCommands(o.result.Stderr)
}

const countedSameA = "package a\n\nfunc Value(x int) int { return x + 1 }\n"
const countedSameB = "package b\n\ntype T struct{}\n\nfunc (T) Value(x int) int { return x - 1 }\n\nfunc Value(x int) int { return x * 2 }\n"
const countedSameProfile = "mode: set\nexample.com/ownership/a/value.go:3.25,3.39 1 0\nexample.com/ownership/b/value.go:5.29,5.43 1 0\nexample.com/ownership/b/value.go:7.25,7.39 1 0\n"
const countedZeroSource = "package main\n\nfunc Compare(n int) bool { return n > 5 }\n\nfunc Dormant(flag bool) string {\n if flag { return \"yes\" }\n return \"no\"\n}\n\nfunc Empty() {}\n"
const countedZeroProfile = "mode: set\nexample.com/ownership/main.go:3.28,3.42 1 0\nexample.com/ownership/main.go:6.2,6.10 1 0\nexample.com/ownership/main.go:6.12,6.26 1 0\nexample.com/ownership/main.go:7.2,7.13 1 0\nexample.com/ownership/main.go:10.15,10.15 0 0\n"
const countedSiteFreeSource = "package main\n\nfunc Name() string { return \"x\" }\n\nfunc Dormant(flag bool) string {\n if flag { return \"yes\" }\n return \"no\"\n}\n"
const countedSiteFreeProfile = "mode: set\nexample.com/ownership/main.go:3.22,3.34 1 0\nexample.com/ownership/main.go:6.2,6.10 1 0\nexample.com/ownership/main.go:6.12,6.26 1 0\nexample.com/ownership/main.go:7.2,7.13 1 0\n"

func (q *countedMutation) unreachedTests(pkg, file string) string {
	return strings.NewReplacer("PACKAGE", pkg, "ORIGINAL_ROOT", strconv.Quote(q.e.m.w.dir), "SOURCE_BASE", strconv.Quote(filepath.Base(file)), "SOURCE_PATH", strconv.Quote(file), "TRACE_PATH", strconv.Quote(filepath.Join(q.e.outside, "trace.jsonl"))).Replace(countedUnreachedTests)
}

func (q *countedMutation) auditProfile(expected string) error {
	if err := q.measureProfile(expected); err != nil {
		return err
	}
	return q.e.m.w.d.Remove(filepath.Join(q.e.m.w.dir, ".metrics"))
}
