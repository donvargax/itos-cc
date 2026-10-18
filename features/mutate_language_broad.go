package features

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/cucumber/godog"
)

type broadLanguage struct {
	l             *languageMutation
	scope, change string
	before        map[string]any
}

func (l *languageMutation) broadSteps(sc *godog.ScenarioContext) {
	b := &broadLanguage{l: l}
	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		*b = broadLanguage{l: l}
		return ctx, nil
	})
	sc.Step(`^a Go module with a production function and a separate end-to-end _test\.go file that runs its built binary without importing its package$`, b.binaryProject)
	sc.Step(`^a mutation run with (--all-tests|--test-command "go test -count=1 ./\.\.\.") recorded a mutant killed only by that end-to-end test$`, func(scope string) error {
		b.scope = scope
		if err := b.record(scope); err != nil {
			return err
		}
		u, err := languageUnit(b.before, "Low")
		if err != nil {
			return err
		}
		mutants := objectList(u, "mutants")
		if len(mutants) != 1 || mutants[0]["outcome"] != "killed" {
			return fmt.Errorf("initial binary-only kill missing: %v", u)
		}
		return nil
	})
	sc.Step(`^only that test file changed since, so it no longer kills the mutant$`, b.weakenBinaryTest)
	sc.Step(`^its function is stale, the problem is "mutation\.stale", and its message names the changed end-to-end test file$`, func() error { return b.stale("Low", "e2e/e2e_test.go", nil) })
	sc.Step(`^I run mutation run with (--all-tests|--test-command "go test -count=1 ./\.\.\."|--test-command "\./run\.sh") for the production file$`, func(scope string) error { return l.run("mutation run --json --no-coverage " + scope + " " + l.file) })
	sc.Step(`^that mutant runs again rather than reusing its recorded kill$`, func() error { return b.executed("Low", "survived") })
	sc.Step(`^it survives$`, func() error { return b.executed("Low", "survived") })
	sc.Step(`^a Go whole-suite outcome recorded with (all-tests|test-command) and support globs matching a feature file$`, func(scope string) error {
		if err := b.goProject(false); err != nil {
			return err
		}
		if err := l.m.w.write("itos-cc.yaml", "mutation:\n  tests:\n    list: go version\n    run: go version {pattern}\n    ids_pattern: \"{ids}\"\n    join:\n      each: \"{id}\"\n      sep: \",\"\n    support: [\"features/*.feature\"]\n"); err != nil {
			return err
		}
		if err := l.m.w.write("features/example.feature", "Feature: original\n"); err != nil {
			return err
		}
		b.scope = "--all-tests"
		if scope == "test-command" {
			b.scope = `--test-command "go test -count=1 ./..."`
		}
		return b.record(b.scope)
	})
	sc.Step(`^a support file was (added|changed|removed) since that outcome was recorded$`, b.supportChange)
	sc.Step(`^the function is stale and its "mutation\.stale" message names the support file$`, func() error { return b.stale("Low", "features/example.feature", nil) })
	sc.Step(`^no test or list command runs$`, b.noCommands)
	sc.Step(`^Go kills recorded with all-tests and test-command scopes and current module-test and support hashes$`, b.twoScopes)
	sc.Step(`^none of the source, importer tests, module tests or support files changed$`, b.unchanged)
	sc.Step(`^I run mutation run without --all-tests or --test-command$`, func() error { return l.run("mutation run --json --no-coverage " + l.file) })
	sc.Step(`^those kills are reused$`, b.allReused)
	sc.Step(`^each retains its recorded scope and freshness evidence$`, b.evidenceKept)
	sc.Step(`^mutation check still reports them fresh$`, func() error { return b.freshAll(nil) })
	sc.Step(`^Go mutation outcomes recorded with own, listed, all-tests and test-command scopes$`, b.mixed)
	sc.Step(`^importer tests, listed covering tests and listed support files are unchanged$`, b.unchanged)
	sc.Step(`^only a non-importing module _test\.go file changed$`, func() error { return l.appendComment("e2e/support_test.go") })
	sc.Step(`^their freshness is checked$`, func() error { return l.run("mutation check --json " + l.file) })
	sc.Step(`^the own and listed outcomes remain usable$`, func() error { return b.expectStates(map[string]string{"Own": "fresh", "Listed": "fresh"}, nil) })
	sc.Step(`^every all-tests and test-command outcome is stale, including a survivor and an excepted survivor$`, func() error {
		return b.expectStates(map[string]string{"AllTests": "stale", "TestCommand": "stale", "Survivor": "stale", "Excepted": "stale"}, nil)
	})
	sc.Step(`^mutation check and the graph agree on each affected function's freshness$`, func(ctx context.Context) error {
		return b.graph(ctx, map[string]string{"Own": "fresh", "Listed": "fresh", "AllTests": "stale", "TestCommand": "stale", "Survivor": "stale", "Excepted": "stale"})
	})
	sc.Step(`^a Go file with two functions whose outcomes were recorded with whole-suite scopes$`, b.partialProject)
	sc.Step(`^a non-importing module test changed since both outcomes were recorded$`, func() error { return l.appendComment("e2e/support_test.go") })
	sc.Step(`^only one function changed since "base"$`, b.changeOne)

	sc.Step(`^the judged function's outcomes are rerun$`, func() error { return b.executed("Low", "killed") })
	sc.Step(`^the unjudged function's outcomes remain stale$`, b.unjudgedKept)
	sc.Step(`^mutation check reports the unjudged function stale$`, func() error { return b.stale("Other", "", nil) })
	sc.Step(`^mutation sample excludes its stale outcomes$`, b.sampleFresh)
	sc.Step(`^the graph marks the unjudged function stale too$`, func(ctx context.Context) error {
		return b.graph(ctx, map[string]string{"Low": "fresh", "Other": "stale"})
	})
	sc.Step(`^a Go broad-scope kill whose source and importer hashes match but whose snapshot records no whole-suite freshness evidence$`, b.legacy)
	sc.Step(`^I run mutation check for its production file$`, func() error { return l.run("mutation check --json " + l.file) })
	sc.Step(`^its function is stale and mutation run must rerun that outcome before it can be reused$`, b.legacyRerun)
	sc.Step(`^a fresh broad-scope outcome in the outer Go module$`, func() error { return b.record("--all-tests") })
	sc.Step(`^a build-tagged _test\.go file is added beneath that module$`, func() error {
		return l.m.w.write("tagged_test.go", "//go:build freshness_test\n\npackage main\nimport \"testing\"\nfunc TestTagged(t *testing.T) {}\n")
	})
	sc.Step(`^that outcome is stale$`, func() error { return b.stale("Low", "tagged_test.go", nil) })
	sc.Step(`^adding a _test\.go file only beneath a nested Go module leaves that outcome fresh$`, b.nested)
}
func (b *broadLanguage) goProject(two bool) error {
	l, w := b.l, b.l.m.w
	l.file = "main.go"
	l.language = "go"
	source := "package main\n\nfunc Low() bool { return true }\n"
	test := "package main\nimport \"testing\"\nfunc TestLow(t *testing.T) { if !Low() { t.Fatal(\"Low\") } }\n"
	if two {
		source += "func Other() bool { return false }\n"
		test += "func TestOther(t *testing.T) { if Other() { t.Fatal(\"Other\") } }\n"
	}
	for file, text := range map[string]string{"go.mod": "module example.com/language\n\ngo 1.22\n", l.file: source, "main_test.go": test, "e2e/support_test.go": "package e2e\nimport \"testing\"\nfunc TestSupport(t *testing.T) {}\n"} {
		if err := w.write(file, text); err != nil {
			return err
		}
	}
	// The file's own tests, its package's, beside the whole suite's.
	if err := w.d.Tests(w.dir, Tests{Language: "go", Command: "go test -count=1 -failfast ./."}); err != nil {
		return err
	}
	if err := w.d.Tests(w.dir, Tests{Language: "go", Command: "go test -count=1 -failfast ./..."}); err != nil {
		return err
	}
	return w.d.Tests(w.dir, Tests{Language: "go", Command: "go test -count=1 ./..."})
}
func (b *broadLanguage) binaryProject() error {
	l, w := b.l, b.l.m.w
	l.file = "main.go"
	l.language = "go"
	source := "package main\nimport \"fmt\"\nfunc Low() bool { return true }\nfunc main() { fmt.Println(Low()) }\n"
	test := `package e2e
import("testing";"os/exec";"path/filepath";"strings")
func TestCLI(t *testing.T) { bin:=filepath.Join(t.TempDir(),"app.exe"); if out,err:=exec.Command("go","build","-o",bin,"..").CombinedOutput();err!=nil {t.Fatalf("build: %v %s",err,out)};out,err:=exec.Command(bin).Output();if err!=nil{t.Fatal(err)};if strings.TrimSpace(string(out))!="true"{t.Fatalf("output %s",out)} }
`
	for file, text := range map[string]string{"go.mod": "module example.com/language\n\ngo 1.22\n", l.file: source, "e2e/e2e_test.go": test} {
		if err := w.write(file, text); err != nil {
			return err
		}
	}
	if err := w.d.Tests(w.dir, Tests{Language: "go", Command: "go test -count=1 -failfast ./..."}); err != nil {
		return err
	}
	return w.d.Tests(w.dir, Tests{Language: "go", Command: "go test -count=1 ./..."})
}
func (b *broadLanguage) record(scope string) error {
	l := b.l
	a, log, code, err := l.capture("mutation run --json --no-coverage " + scope + " " + l.file)
	if err != nil {
		return err
	}
	if code > 1 {
		return fmt.Errorf("broad record exit%d %v\n%s", code, a, log)
	}
	b.before, err = l.s.snapshot(l.file)
	return err
}
func (b *broadLanguage) weakenBinaryTest() error {
	l, w := b.l, b.l.m.w
	old := l.m.read("e2e/e2e_test.go")
	next := strings.Replace(old, `strings.TrimSpace(string(out))!="true"`, `strings.TrimSpace(string(out))!="true" && strings.TrimSpace(string(out))!="false"`, 1)
	if next == old {
		return fmt.Errorf("no binary assertion to weaken")
	}
	if err := w.write("e2e/e2e_test.go", next); err != nil {
		return err
	}
	t := Tests{Language: "go", Command: "go test -count=1 -failfast ./...", Surviving: []Site{{File: l.file, Line: 3, Original: "true", Replacement: "false"}}}
	if err := w.d.Tests(w.dir, t); err != nil {
		return err
	}
	t.Command = "go test -count=1 ./..."
	return w.d.Tests(w.dir, t)
}
func (b *broadLanguage) statesOf(a map[string]any) (map[string]string, error) {
	f, err := languageRunFile(a, b.l.file)
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, u := range objectList(f, "functions") {
		_, name, _ := strings.Cut(fmt.Sprint(u["function"]), "#")
		out[name] = fmt.Sprint(u["state"])
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no checked functions: %v", a)
	}
	return out, nil
}
func (b *broadLanguage) expectStates(want map[string]string, a map[string]any) error {
	if a == nil {
		var err error
		a, err = b.l.object()
		if err != nil {
			return err
		}
	}
	states, err := b.statesOf(a)
	if err != nil {
		return err
	}
	for name, state := range want {
		if states[name] != state {
			return fmt.Errorf("%s state%q want%s in%v", name, states[name], state, a)
		}
	}
	return nil
}
func (b *broadLanguage) stale(name, changed string, a map[string]any) error {
	if a == nil {
		var err error
		a, _, _, err = b.l.capture("mutation check --json " + b.l.file)
		if err != nil {
			return err
		}
	}
	if err := b.expectStates(map[string]string{name: "stale"}, a); err != nil {
		return err
	}
	for _, p := range checkProblems(a) {
		if p["rule"] == "mutation.stale" && strings.HasSuffix(fmt.Sprint(p["function"]), "#"+name) && strings.Contains(fmt.Sprint(p["message"]), changed) {
			return nil
		}
	}
	return fmt.Errorf("no stale %s naming%s: %v", name, changed, a)
}
func (b *broadLanguage) executed(name, outcome string) error {
	w := b.l.m.w
	plain := w.last != nil && !slices.Contains(w.last.args, "--json")
	actual := trialsOf(w.stderr, b.l.file)
	a, err := b.l.object()
	if err != nil {
		return err
	}
	f, err := languageRunFile(a, b.l.file)
	if err != nil {
		return err
	}
	found := 0
	for _, m := range objectList(f, "mutants") {
		if strings.HasSuffix(fmt.Sprint(m["function"]), "#"+name) {
			found++
			observed := false
			for _, trial := range actual {
				if float64(trial.line) == m["line"] && trial.original == m["original"] && trial.replacement == m["replacement"] && trial.outcome == outcome {
					observed = true
				}
			}
			if !observed || (!plain && m["reused"] == true) || m["outcome"] != outcome {
				return fmt.Errorf("expected actual %s %s: %v", name, outcome, m)
			}
		}
	}
	if found == 0 {
		return fmt.Errorf("missing executed%s: %v", name, f)
	}
	return nil
}
func (b *broadLanguage) supportChange(change string) error {
	l := b.l
	b.change = change
	if change == "added" {
		if err := l.m.w.d.Remove(filepath.Join(l.m.w.dir, "features/example.feature")); err != nil {
			return err
		}
		if err := b.record(b.scope); err != nil {
			return err
		}
	}
	if change == "removed" {
		return l.m.w.d.Remove(filepath.Join(l.m.w.dir, "features/example.feature"))
	}
	return l.m.w.write("features/example.feature", "Feature: "+change+"\n")
}
func (b *broadLanguage) noCommands() error {
	if strings.Contains(b.l.m.w.stderr, "$ ") || len(baselines(b.l.m.w.stderr)) != 0 {
		return fmt.Errorf("check ran command: %s", b.l.m.w.stderr)
	}
	return nil
}
func (b *broadLanguage) twoScopes() error {
	if err := b.goProject(true); err != nil {
		return err
	}
	if err := b.record("--all-tests"); err != nil {
		return err
	}
	snap := b.before
	// Model separately recorded broad scopes with the actual current evidence.
	for _, u := range objectList(snap, "units") {
		if u["name"] == "Other" {
			for _, m := range objectList(u, "mutants") {
				m["scope"] = "go test -count=1 ./..."
			}
		}
	}
	if err := b.l.writeSnapshot(b.l.file, snap); err != nil {
		return err
	}
	b.before = snap
	return nil
}
func (b *broadLanguage) unchanged() error {
	snap, err := b.l.s.snapshot(b.l.file)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(snap, b.before) {
		return fmt.Errorf("recorded state unexpectedly changed")
	}
	return nil
}
func (b *broadLanguage) allReused() error {
	a, err := b.l.object()
	if err != nil {
		return err
	}
	f, err := languageRunFile(a, b.l.file)
	if err != nil {
		return err
	}
	mutants := objectList(f, "mutants")
	if len(mutants) != 2 || f["ran"] != float64(0) || f["reused"] != float64(2) {
		return fmt.Errorf("broad reuse counts: %v", f)
	}
	for _, m := range mutants {
		if m["reused"] != true || m["outcome"] != "killed" {
			return fmt.Errorf("broad kill not reused: %v", m)
		}
	}
	return nil
}
func (b *broadLanguage) evidenceKept() error {
	a, err := b.l.s.snapshot(b.l.file)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(a, b.before) {
		return fmt.Errorf("reuse changed recorded scopes/evidence\nbefore%v\nafter%v", b.before, a)
	}
	return nil
}
func (b *broadLanguage) freshAll(a map[string]any) error {
	if a == nil {
		var err error
		a, _, _, err = b.l.capture("mutation check --json " + b.l.file)
		if err != nil {
			return err
		}
	}
	states, err := b.statesOf(a)
	if err != nil {
		return err
	}
	for name, state := range states {
		if state != "fresh" {
			return fmt.Errorf("%s not fresh: %v", name, a)
		}
	}
	return nil
}
func (b *broadLanguage) mixed() error {
	if err := b.goProject(false); err != nil {
		return err
	}
	l := b.l
	source := "package main\n\nfunc Own() bool { return true }\nfunc Listed() bool { return false }\nfunc AllTests() bool { return true }\nfunc TestCommand() bool { return false }\nfunc Survivor() bool { return true }\nfunc Excepted() bool { return false }\n"
	if err := l.m.w.write(l.file, source); err != nil {
		return err
	}
	if err := l.m.w.write("main_test.go", "package main\nimport \"testing\"\nfunc TestOwn(t *testing.T){if !Own(){t.Fatal(\"Own\")}}\n"); err != nil {
		return err
	}
	definition := "Feature: listed test definition\n"
	if err := l.m.w.write("features/listed.feature", definition); err != nil {
		return err
	}
	if err := l.m.w.write("itos-cc.yaml", "mutation:\n  tests:\n    list: go version\n    run: go version {pattern}\n    ids_pattern: \"{ids}\"\n    join:\n      each: \"{id}\"\n      sep: \",\"\n    support: [\"features/*.feature\"]\n"); err != nil {
		return err
	}
	if err := b.record("--all-tests"); err != nil {
		return err
	}
	a := b.before
	for _, u := range objectList(a, "units") {
		name := fmt.Sprint(u["name"])
		for _, m := range objectList(u, "mutants") {
			switch name {
			case "Own":
				delete(m, "scope")
				delete(m, "suite_evidence")
				delete(m, "go_evidence")
			case "Listed":
				m["scope"] = "listed"
				m["tests"] = []string{"ID-A-01"}
				delete(m, "suite_evidence")
				delete(m, "go_evidence")
			case "TestCommand", "Excepted":
				m["scope"] = "go test -count=1 ./..."
			}
			if name == "Excepted" {
				site := fmt.Sprintf("%s:%.0f:%.0f", l.file, m["line"], m["column"])
				o, log, code, err := l.capture("mutation except --json " + site + " --reason equivalent")
				if err != nil {
					return err
				}
				if code != 0 {
					return fmt.Errorf("review mixed survivor: %v %s", o, log)
				}
			}
		}
	}
	hash := sha256.Sum256([]byte(definition))
	a["listed"] = map[string]string{"ID-A-01": "features/listed.feature"}
	a["listed_files"] = map[string]string{"features/listed.feature": fmt.Sprintf("%x", hash)}
	a["support"] = map[string]string{"features/listed.feature": fmt.Sprintf("%x", hash)}
	if err := l.writeSnapshot(l.file, a); err != nil {
		return err
	}
	var err error
	b.before, err = l.s.snapshot(l.file)
	return err
}
func (b *broadLanguage) graph(ctx context.Context, want map[string]string) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	w := b.l.m.w
	raw, err := w.d.Graph(ctx, Run{Dir: w.dir, Env: w.env()})
	if err != nil {
		return err
	}
	var a map[string]any
	if err := json.Unmarshal(raw, &a); err != nil {
		return err
	}
	states := map[string]bool{}
	for _, n := range objectList(a, "nodes") {
		for _, u := range objectList(n, "units") {
			if filepath.ToSlash(fmt.Sprint(u["file"])) == b.l.file {
				v, ok := u["stale"].(bool)
				if !ok {
					return fmt.Errorf("graph lacks stale fact: %v", u)
				}
				states[fmt.Sprint(u["name"])] = v
			}
		}
	}
	for name, state := range want {
		v, ok := states[name]
		if !ok || v != (state == "stale") {
			return fmt.Errorf("graph %s present%v stale%v want%s; all%v", name, ok, v, state, states)
		}
	}
	return nil
}
func (b *broadLanguage) partialProject() error {
	if err := b.goProject(true); err != nil {
		return err
	}
	w := b.l.m.w
	if err := w.git("init", "-q"); err != nil {
		return err
	}
	if err := b.l.s.commit("base fixture"); err != nil {
		return err
	}
	if err := w.git("tag", "base"); err != nil {
		return err
	}
	return b.record("--all-tests")
}
func (b *broadLanguage) changeOne() error {
	l := b.l
	if err := l.m.w.write(l.file, strings.Replace(l.m.read(l.file), "func Low() bool { return true }", "func Low() bool { /* changed */ return true }", 1)); err != nil {
		return err
	}
	return l.s.commit("one function and module test changed")
}
func (b *broadLanguage) unjudgedKept() error {
	a, err := b.l.s.snapshot(b.l.file)
	if err != nil {
		return err
	}
	after, err := languageUnit(a, "Other")
	if err != nil {
		return err
	}
	before, err := languageUnit(b.before, "Other")
	if err != nil {
		return err
	}
	if len(objectList(after, "mutants")) != 1 || after["stale"] != true || !reflect.DeepEqual(languageJudgment(after), languageJudgment(before)) {
		return fmt.Errorf("partial run changed the unjudged judgment/site/hash or lost stale state: before%v after%v", before, after)
	}
	return b.stale("Other", "", nil)
}

// Compare the recorded judgment while checking staleness separately. The exact
// original dependency-evidence contract is also held by MUT207 and MUT247.
func languageJudgment(unit map[string]any) map[string]any {
	out := map[string]any{}
	for _, key := range []string{"name", "namespace", "hash", "sites", "killed", "survived", "uncovered"} {
		out[key] = unit[key]
	}
	mutants := []map[string]any{}
	for _, m := range objectList(unit, "mutants") {
		copy := map[string]any{}
		for key, value := range m {
			if key != "suite_evidence" && key != "go_evidence" {
				copy[key] = value
			}
		}
		mutants = append(mutants, copy)
	}
	out["mutants"] = mutants
	return out
}
func (b *broadLanguage) sampleFresh() error {
	a, _, code, err := b.l.capture("mutation sample --count 50 --seed 4813e48 --json " + b.l.file)
	if err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("sample exit%d: %v", code, a)
	}
	text, _ := json.Marshal(a)
	if strings.Contains(string(text), "#Other") {
		return fmt.Errorf("stale Other included in sample: %s", text)
	}
	if !strings.Contains(string(text), "#Low") {
		return fmt.Errorf("fresh Low missing from sample: %s", text)
	}
	return nil
}
func (b *broadLanguage) legacy() error {
	if err := b.goProject(false); err != nil {
		return err
	}
	if err := b.record("--all-tests"); err != nil {
		return err
	}
	a := b.before
	for _, u := range objectList(a, "units") {
		for _, m := range objectList(u, "mutants") {
			delete(m, "suite_evidence")
			delete(m, "go_evidence")
		}
	}
	return b.l.writeSnapshot(b.l.file, a)
}
func (b *broadLanguage) legacyRerun() error {
	if err := b.stale("Low", "whole-suite evidence", nil); err != nil {
		return err
	}
	if err := b.l.run("mutation run --json --no-coverage " + b.l.file); err != nil {
		return err
	}
	return b.executed("Low", "killed")
}
func (b *broadLanguage) nested() error {
	if err := b.record("--all-tests"); err != nil {
		return err
	}
	w := b.l.m.w
	if err := w.write("nested/go.mod", "module example.com/nested\n\ngo 1.22\n"); err != nil {
		return err
	}
	if err := w.write("nested/nested_test.go", "package nested\nimport \"testing\"\nfunc TestNested(t *testing.T){}\n"); err != nil {
		return err
	}
	return b.freshAll(nil)
}
