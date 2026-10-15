package features

import (
	"fmt"
	"path/filepath"
	"slices"

	"github.com/cucumber/godog"
)

const strictZeroGo = `package main

func Dormant(flag bool) string { if flag { return "yes" }; return "no" }
func Empty() {}
func CommentsOnly() { // no executable work
}
func main() {}
`
const strictZeroTest = `package main
import "testing"
func TestEmpty(t *testing.T) { Empty(); CommentsOnly() }
`
const strictZeroReport = `mode: set
example.com/strict/main.go:3.34,3.42 1 0
example.com/strict/main.go:3.44,3.58 1 0
example.com/strict/main.go:3.60,3.71 1 0
`

const strictUnusedReport = `example.com/strict/unused/unused.go:2.34,2.42 1 0
example.com/strict/unused/unused.go:2.44,2.58 1 0
example.com/strict/unused/unused.go:2.60,2.71 1 0
`

func (q *strictMutation) zeroSteps(sc *godog.ScenarioContext) {
	sc.Step(`^a Go function containing no mutation sites whose identifier-conditioned branch is never executed$`, func() error { return q.goProject(strictZeroGo, strictZeroTest, strictZeroReport) })
	sc.Step(`^an executable zero-site function in a package no test loads$`, func() error {
		if err := q.m.w.write("unused/unused.go", "package unused\nfunc Dormant(flag bool) string { if flag { return \"yes\" }; return \"no\" }\n"); err != nil {
			return err
		}
		return q.declareGo(strictZeroReport + strictUnusedReport)
	})
	sc.Step(`^I run mutation run with --fail-uncovered for each source$`, func() error { return q.run("mutation run --fail-uncovered --json main.go unused/unused.go") })
	sc.Step(`^each unexecuted executable block is an uncovered-statement failure$`, func() error {
		a, err := q.object()
		if err != nil {
			return err
		}
		if err = q.finding(a, "mutation.uncovered-statement", "Dormant", 3); err != nil {
			return err
		}
		for _, p := range checkProblems(a) {
			if p["rule"] == "mutation.uncovered-statement" && filepath.ToSlash(fmt.Sprint(p["file"])) == "unused/unused.go" && p["function"] == "example.com/strict/unused#Dormant" && p["line"] == float64(2) {
				return q.m.w.exitsWith(1)
			}
		}
		return fmt.Errorf("unloaded executable function did not fail: %v", a)
	})
	sc.Step(`^a strict run and cached check of a fully executed zero-site function both pass$`, q.executedZero)
	sc.Step(`^empty or comment-only function bodies add no coverage obligation$`, func() error {
		a, err := q.object()
		if err != nil {
			return err
		}
		for _, name := range []string{"Empty", "CommentsOnly"} {
			for _, p := range checkProblems(a) {
				if strictNamed(fmt.Sprint(p["function"]), name) {
					return fmt.Errorf("empty body has obligation: %v", p)
				}
			}
		}
		snap, err := q.snapshot()
		if err != nil {
			return err
		}
		for _, name := range []string{"Empty", "CommentsOnly"} {
			u, err := languageUnit(snap, name)
			if err != nil {
				return err
			}
			e, ok := u["go_coverage"].(map[string]any)
			if !ok || e["complete"] != true || len(objectList(e, "blocks")) != 0 {
				return fmt.Errorf("empty body evidence %v", u)
			}
		}
		return nil
	})
	sc.Step(`^a non-strict check of a zero-site function still needs no snapshot entry$`, func() error {
		if err := q.m.w.d.Remove(filepath.Join(q.m.w.dir, ".metrics", "mutate")); err != nil {
			return err
		}
		_, err := q.fresh("mutation check --json main.go", 0)
		return err
	})
	sc.Step(`^a Go function has covered and uncovered positive-weight blocks with distinct column spans on the same source line$`, func() error {
		source := `package main

func Branch(call bool) string { if call { return "one" }; return "two" }
func Outer(call bool) string { f := func() string { return "nested" }; if call { return f() }; return "none" }
func main() {
    // A comment and brace-only line have no executable weight.
}
`
		tests := `package main
import "testing"
func TestBranch(t *testing.T){if Branch(true)!="one"{t.Fatal("branch")};if Outer(false)!="none"{t.Fatal("outer")}}
`
		report := `mode: set
example.com/strict/main.go:3.33,3.41 1 1
example.com/strict/main.go:3.43,3.57 1 1
example.com/strict/main.go:3.59,3.71 1 0
example.com/strict/main.go:4.32,4.51 1 1
example.com/strict/main.go:4.53,4.70 1 0
example.com/strict/main.go:4.72,4.80 1 1
example.com/strict/main.go:4.82,4.94 1 0
example.com/strict/main.go:4.96,4.109 1 1
`
		return q.goProject(source, tests, report)
	})
	sc.Step(`^another named Go function contains a never-called function literal$`, func() error {
		if q.source() == "" {
			return fmt.Errorf("missing closure fixture")
		}
		return nil
	})
	sc.Step(`^I run mutation run with --fail-uncovered$`, func() error { return q.run("mutation run --fail-uncovered --json " + q.file) })
	sc.Step(`^the same-line uncovered span fails despite the covered span on that line$`, func() error {
		a, err := q.object()
		if err != nil {
			return err
		}
		if err = q.finding(a, "mutation.uncovered-statement", "Branch", 3); err != nil {
			return err
		}
		snap, err := q.snapshot()
		if err != nil {
			return err
		}
		u, err := languageUnit(snap, "Branch")
		if err != nil {
			return err
		}
		e, err := strictEvidence(u)
		if err != nil {
			return err
		}
		covered, uncovered := []string{}, []string{}
		for _, b := range objectList(e, "blocks") {
			if b["line"] != float64(3) {
				return fmt.Errorf("unexpected branch block line: %v", b)
			}
			if b["covered"] == true {
				covered = append(covered, fmt.Sprint(b["span"]))
			} else {
				uncovered = append(uncovered, fmt.Sprint(b["span"]))
			}
		}
		if len(covered) == 0 || len(uncovered) == 0 {
			return fmt.Errorf("missing covered/uncovered spans: %v", e)
		}
		for _, span := range uncovered {
			if span == "" || slices.Contains(covered, span) {
				return fmt.Errorf("not distinct spans: %v", e)
			}
		}
		return q.m.w.exitsWith(1)
	})
	sc.Step(`^the literal's uncovered block is attributed to its enclosing named function$`, func() error {
		a, err := q.object()
		if err != nil {
			return err
		}
		return q.closureInventory(a)
	})
	sc.Step(`^comments, brace-only lines and zero-statement-weight blocks create no separate findings$`, func() error {
		a, err := q.object()
		if err != nil {
			return err
		}
		for _, p := range q.findings(a, "mutation.uncovered-statement", "") {
			if !slices.Contains([]any{float64(3), float64(4)}, p["line"]) {
				return fmt.Errorf("non-executable finding: %v", p)
			}
		}
		snap, err := q.snapshot()
		if err != nil {
			return err
		}
		u, err := languageUnit(snap, "main")
		if err != nil {
			return err
		}
		e, ok := u["go_coverage"].(map[string]any)
		if !ok || e["complete"] != true || len(objectList(e, "blocks")) != 0 {
			return fmt.Errorf("zero-weight main evidence: %v", u)
		}
		return nil
	})
}
func (q *strictMutation) executedZero() error {
	tests := `package main
import "testing"
func TestAll(t *testing.T){if Dormant(true)!="yes" || Dormant(false)!="no"{t.Fatal("Dormant")};Empty();CommentsOnly()}
`
	if err := q.m.w.write("main_test.go", tests); err != nil {
		return err
	}
	if err := q.declareGo("mode: set\nexample.com/strict/main.go:3.34,3.42 1 1\nexample.com/strict/main.go:3.44,3.58 1 1\nexample.com/strict/main.go:3.60,3.71 1 1\n"); err != nil {
		return err
	}
	if _, err := q.fresh("mutation run --fail-uncovered --json main.go", 0); err != nil {
		return err
	}
	if err := q.observed(); err != nil {
		return err
	}
	a, log, code, err := q.capture("mutation check --fail-uncovered --json main.go")
	if err != nil {
		return err
	}
	if code != 0 || len(checkProblems(a)) != 0 {
		return fmt.Errorf("executed zero check exit%d %v", code, a)
	}
	if err := q.quiet(log); err != nil {
		return err
	}
	return q.run("mutation check --fail-uncovered --json main.go")
}

func (q *strictMutation) closureInventory(a map[string]any) error {
	if err := q.finding(a, "mutation.uncovered-statement", "Outer", 4); err != nil {
		return err
	}
	problems := q.findings(a, "mutation.uncovered-statement", "Outer")
	if len(problems) != 2 || len(checkProblems(a)) != 3 {
		return fmt.Errorf("incomplete Branch/Outer statement population: %v", a)
	}
	for _, p := range problems {
		if filepath.ToSlash(fmt.Sprint(p["file"])) != q.file || p["line"] != float64(4) {
			return fmt.Errorf("closure finding attribution: %v", p)
		}
	}
	snap, err := q.snapshot()
	if err != nil {
		return err
	}
	u, err := languageUnit(snap, "Outer")
	if err != nil {
		return err
	}
	e, err := strictEvidence(u)
	if err != nil {
		return err
	}
	if e["function"] != q.namespace+"#Outer" || e["file"] != q.file {
		return fmt.Errorf("closure inventory owner: %v", e)
	}
	var uncovered []string
	closure := false
	for _, block := range objectList(e, "blocks") {
		if block["covered"] == false {
			uncovered = append(uncovered, fmt.Sprint(block["span"]))
		}
		if block["span"] == "4.53,4.70" && block["column"] == float64(53) && block["line"] == float64(4) && block["weight"] == float64(1) && block["covered"] == false {
			closure = true
		}
	}
	slices.Sort(uncovered)
	if !closure || !slices.Equal(uncovered, []string{"4.53,4.70", "4.82,4.94"}) {
		return fmt.Errorf("closure and outer branch inventories not separate: %v", e)
	}
	return nil
}
