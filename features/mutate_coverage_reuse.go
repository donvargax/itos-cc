package features

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"maps"
	"path/filepath"
	"reflect"
	"slices"
	"strings"

	"github.com/cucumber/godog"
)

// The steps of the Rule "The listed tests' coverage is reused while nothing
// it was measured from changed" (slice-69, issue #67). They reuse
// ID-MUT-122's module (listedMutation), slice-63's named-site module and
// slice-65's module of packages a and b, and read only what itos-cc prints
// and writes: the coverage commands and the lines its stderr says, its
// --json objects, the snapshots and the folder .metrics/coverage/cache.

const (
	// reusedLine is what a run that reuses the cached listed tests'
	// coverage says in place of their coverage command.
	reusedLine = "itos-cc: coverage: the listed tests' coverage reused from .metrics/coverage/cache, its inputs unchanged"
	// unservedLine starts the line saying why the cache does not serve a
	// run, before the run measures.
	unservedLine = "itos-cc: coverage: the cached listed tests' coverage does not serve this run: "
	// cacheFolder is where the cached coverage lives, from the scratch
	// folder.
	cacheFolder = ".metrics/coverage/cache"
)

// coverageReuse is the state the steps keep: what the cache held after the
// Given's run, and the input a change step changed, as the run names it.
type coverageReuse struct {
	l *listedMutation
	// held is each file under the cache folder, by its path from the
	// scratch folder, as the Given's run left it.
	held map[string]string
	// named is what the line saying the cache does not serve names of the
	// input a change step changed.
	named string
}

func coverageReuseSteps(sc *godog.ScenarioContext, l *listedMutation) {
	r := &coverageReuse{l: l}
	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		*r = coverageReuse{l: l}
		return ctx, nil
	})
	sc.Step(`^"itos-cc ([^"]*)" has run$`, r.hasRun)
	sc.Step(`^"itos-cc mutation run board/board\.go" has run, nothing uncommitted$`, r.siteRecorded)
	sc.Step(`^"itos-cc mutation run --count 1", committed has run$`, func(ctx context.Context) error {
		return r.counted(ctx, "mutation run --count 1")
	})
	sc.Step(`^"itos-cc mutation run --site" naming the mutant only ID-A-02 kills, committed has run$`, r.namedRun)
	sc.Step(`^"itos-cc mutation run" whose listed tests' coverage run failed \(ID-MUT-137's Given\) has run$`, r.failedRun)
	sc.Step(`^the module of ID-MUT-122, with a second package b that the binary links$`, r.withB)

	sc.Step(`^a Go file of the module, no file the run mutates, changed$`, func(ctx context.Context) error {
		return r.changeAfter(ctx, "doc.go", "doc.go", "// Package main greets.\npackage main\n")
	})
	sc.Step(`^a test file of the module changed$`, func() error { return r.change("main_test.go", "main_test.go") })
	sc.Step(`^a\.feature, the file the list names for ID-A-01, changed$`, func(ctx context.Context) error {
		return r.changeAfter(ctx, "a.feature", "a.feature", "Feature: a\n")
	})
	sc.Step(`^a file a mutation\.tests support glob matches changed$`, r.supportChanged)
	sc.Step(`^the list command now also prints "ID-A-03"$`, func() error {
		r.named = "ID-A-03"
		return l.slice39SetList("ID-A-01\ta.feature\nID-A-02\nID-A-03\n")
	})
	sc.Step(`^itos-cc\.yaml's mutation\.tests\.whole changed$`, func() error {
		r.named = "itos-cc.yaml"
		// The whole run is the run of every ID the fixture made before,
		// so only the setting changed, not the command that runs.
		if err := l.replace("itos-cc.yaml", "    run: ", "    whole: "+listedRun("ID-A-01", "ID-A-02")+"\n    run: "); err != nil {
			return err
		}
		return l.declare()
	})
	sc.Step(`^\.metrics/coverage/cache's listed entry holds "not json"$`, func() error {
		r.named = "cannot be read"
		return l.m.w.write(cacheFolder+"/listed.json", "not json")
	})

	sc.Step(`^the list command runs once, and the listed tests do not run for coverage$`, func() error {
		if err := l.listOnce(); err != nil {
			return err
		}
		return r.notMeasured()
	})
	sc.Step(`^the listed tests do not run for coverage$`, r.notMeasured)
	sc.Step(`^the mutant only ID-A-02 kills runs its own tests, survives them, then runs the run command selecting ID-A-02 alone, and is killed$`, l.listedKilled)
	sc.Step(`^stderr says the cached listed tests' coverage does not serve this run, naming (that file|that test file|a\.feature|that support file|ID-A-03|itos-cc\.yaml|that it cannot be read), before the listed tests' coverage command$`, r.unservedBefore)
	sc.Step(`^"itos-cc mutation run --mutate-all" after it reuses the coverage it measured$`, r.reusesAfter)
	sc.Step(`^the listed tests run once for coverage, and stderr says the cached coverage measured no file of b$`, r.lacksB)
	sc.Step(`^"itos-cc (mutation run --mutate-all main\.go b)" after it does not run the listed tests for coverage$`, r.notMeasuredAfter)
	sc.Step(`^no listed test ran for coverage, and stderr says the listed tests' coverage was reused$`, r.reused)
	sc.Step(`^\.metrics/coverage/cache is byte for byte as it was$`, r.cacheKept)
	sc.Step(`^the listed tests run once for coverage, and \.metrics/coverage/cache is byte for byte as it was$`, func(ctx context.Context) error {
		if err := l.splitCoverage(); err != nil {
			return fmt.Errorf("%w\n%s", err, l.m.w.report())
		}
		return r.cacheKept(ctx)
	})
	sc.Step(`^the problem is "([^"]*)", with flag "([^"]*)", exit 80$`, func(rule, flag string) error {
		if err := l.m.w.problemIs(rule, "flag", flag); err != nil {
			return err
		}
		return l.m.w.exitsWith(80)
	})
	sc.Step(`^every judged function's coverage evidence is what "itos-cc ([^"]*)" would record, with no uncovered statement it did not have$`, r.sameEvidence)
	sc.Step(`^once a/a\.go's test changed, the strict run measures again and records evidence of the new inputs$`, r.strictAgain)
	exceptionCoverageSteps(sc, r)
}

// run runs args, a mutation run a Given makes, which must judge or fail as
// a check fails, never be refused, and keeps what the cache holds after it.
func (r *coverageReuse) run(ctx context.Context, args string) error {
	w := r.l.m.w
	if err := w.runWith(w.env(), args); err != nil {
		return err
	}
	if w.exit != 0 && w.exit != 1 {
		return fmt.Errorf("itos-cc %s exited %d\n%s", args, w.exit, w.report())
	}
	if r.l.made {
		if err := r.l.clearRecords(); err != nil {
			return err
		}
	}
	held, err := r.cache(ctx)
	r.held = held
	return err
}

func (r *coverageReuse) hasRun(ctx context.Context, args string) error {
	return r.run(ctx, args)
}

// cache is each file under the cache folder, by its path from the scratch
// folder, with its text.
func (r *coverageReuse) cache(ctx context.Context) (map[string]string, error) {
	w := r.l.m.w
	files, err := w.d.Files(ctx, w.dir)
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, file := range files {
		if !strings.HasPrefix(file, cacheFolder+"/") {
			continue
		}
		data, err := w.readIn(w.dir, file)
		if err != nil {
			return nil, err
		}
		out[file] = string(data)
	}
	return out, nil
}

func (r *coverageReuse) cacheKept(ctx context.Context) error {
	w := r.l.m.w
	if len(r.held) == 0 {
		return fmt.Errorf("the Given's run left nothing in %s to keep\n%s", cacheFolder, w.report())
	}
	now, err := r.cache(ctx)
	if err != nil {
		return err
	}
	if !maps.Equal(now, r.held) {
		return fmt.Errorf("%s changed: it holds %v, it held %v\n%s", cacheFolder, slices.Sorted(maps.Keys(now)), slices.Sorted(maps.Keys(r.held)), w.report())
	}
	return nil
}

// measuring is each line of the last run's stderr that runs listed tests
// for coverage: a coverage command selecting them.
func (r *coverageReuse) measuring() []string {
	var out []string
	for _, line := range outputLines(r.l.m.w.stderr) {
		if strings.HasPrefix(line, "itos-cc: coverage ") && strings.Contains(line, "$ ") && strings.Contains(line, "-tests=") {
			out = append(out, line)
		}
	}
	return out
}

func (r *coverageReuse) notMeasured() error {
	if runs := r.measuring(); len(runs) > 0 {
		return fmt.Errorf("the listed tests ran for coverage: %q\n%s", runs, r.l.m.w.report())
	}
	return nil
}

func (r *coverageReuse) says(text string) error {
	if !slices.Contains(outputLines(r.l.m.w.stderr), text) {
		return fmt.Errorf("stderr has no line %q\n%s", text, r.l.m.w.report())
	}
	return nil
}

// unserved is the index of the last run's stderr line saying the cache does
// not serve it naming text, -1 for none.
func (r *coverageReuse) unserved(text string) int {
	for i, line := range outputLines(r.l.m.w.stderr) {
		if strings.HasPrefix(line, unservedLine) && strings.Contains(filepath.ToSlash(line), text) {
			return i
		}
	}
	return -1
}

func (r *coverageReuse) unservedBefore(_ string) error {
	w := r.l.m.w
	at := r.unserved(r.named)
	if at < 0 {
		return fmt.Errorf("no line says the cache does not serve the run, naming %q\n%s", r.named, w.report())
	}
	runs := r.measuring()
	if len(runs) == 0 {
		return fmt.Errorf("the listed tests did not run for coverage\n%s", w.report())
	}
	if slices.Index(outputLines(w.stderr), runs[0]) < at {
		return fmt.Errorf("the listed tests ran for coverage before the line naming %q\n%s", r.named, w.report())
	}
	return nil
}

func (r *coverageReuse) reusesAfter() error {
	w := r.l.m.w
	if err := w.runWith(w.env(), "mutation run --mutate-all"); err != nil {
		return err
	}
	if err := r.notMeasured(); err != nil {
		return err
	}
	return r.says(reusedLine)
}

func (r *coverageReuse) notMeasuredAfter(args string) error {
	w := r.l.m.w
	if err := w.runWith(w.env(), args); err != nil {
		return err
	}
	if w.exit != 0 && w.exit != 1 {
		return fmt.Errorf("itos-cc %s exited %d\n%s", args, w.exit, w.report())
	}
	return r.notMeasured()
}

func (r *coverageReuse) lacksB() error {
	if err := r.l.splitCoverage(); err != nil {
		return fmt.Errorf("%w\n%s", err, r.l.m.w.report())
	}
	if r.unserved("measured no b/b.go") < 0 {
		return fmt.Errorf("no line says the cached coverage measured no b/b.go\n%s", r.l.m.w.report())
	}
	return nil
}

// change appends a line to name, named as the line names it.
func (r *coverageReuse) change(name, named string) error {
	r.named = named
	return r.l.edit(name)
}

// changeAfter writes name as text and commits it, measures again so the
// cache holds it as it is, then changes it.
func (r *coverageReuse) changeAfter(ctx context.Context, name, named, text string) error {
	l := r.l
	if err := l.m.w.write(name, text); err != nil {
		return err
	}
	if err := r.commit("add "+name, name); err != nil {
		return err
	}
	if err := r.run(ctx, "mutation run"); err != nil {
		return err
	}
	return r.change(name, named)
}

// commit commits names.
func (r *coverageReuse) commit(message string, names ...string) error {
	w := r.l.m.w
	if err := w.git(append([]string{"add"}, names...)...); err != nil {
		return err
	}
	return w.git("commit", "-qm", message)
}

// supportChanged lists support/*.txt under mutation.tests.support, commits a
// file it matches, measures again, then changes that file.
func (r *coverageReuse) supportChanged(ctx context.Context) error {
	l := r.l
	if err := l.replace("itos-cc.yaml", "      sep: \",\"\n", "      sep: \",\"\n    support: [\"support/*.txt\"]\n"); err != nil {
		return err
	}
	if err := l.declare(); err != nil {
		return err
	}
	if err := l.m.w.write("support/checks.txt", "what the listed tests check\n"); err != nil {
		return err
	}
	if err := r.commit("support", "itos-cc.yaml", "support/checks.txt"); err != nil {
		return err
	}
	if err := r.run(ctx, "mutation run"); err != nil {
		return err
	}
	return r.change("support/checks.txt", "support/checks.txt")
}

// counted runs args, a counted run, where itos-cc makes one.
func (r *coverageReuse) counted(ctx context.Context, args string) error {
	if err := r.l.m.w.d.Counted(); err != nil {
		return err
	}
	return r.run(ctx, args)
}

// namedRun runs --site naming the mutant of `-` → `+` in n-1, which only
// ID-A-02 reaches and kills.
func (r *coverageReuse) namedRun(ctx context.Context) error {
	line := strings.Split(r.l.m.read("main.go"), "\n")[listedManyLine-1]
	column := strings.Index(line, "n-1") + 2
	return r.counted(ctx, fmt.Sprintf("mutation run --site main.go:%d:%d", listedManyLine, column))
}

// failedRun is ID-MUT-137's Given and a run whose listed tests' coverage run
// failed, after which the listed tests pass again.
func (r *coverageReuse) failedRun(ctx context.Context) error {
	l := r.l
	if err := l.failingCoverage(); err != nil {
		return err
	}
	if err := r.run(ctx, "mutation run --json"); err != nil {
		return err
	}
	if err := l.coverageProblem("ID-A-01", "ID-A-02"); err != nil {
		return err
	}
	l.m.w.variables = slices.DeleteFunc(l.m.w.variables, func(v string) bool { return v == failCoverageVariable })
	return l.declare()
}

// siteRecorded runs a complete run of board/board.go, then commits the
// summary comment it wrote, so nothing it judges is uncommitted.
func (r *coverageReuse) siteRecorded(ctx context.Context) error {
	w := r.l.m.w
	if err := w.d.Counted(); err != nil {
		return err
	}
	before, err := w.readIn(w.dir, identityFile)
	if err != nil {
		return err
	}
	if err := r.run(ctx, "mutation run "+identityFile); err != nil {
		return err
	}
	after, err := w.readIn(w.dir, identityFile)
	if err != nil {
		return err
	}
	if string(after) == string(before) {
		return nil
	}
	return r.commit("summary comment", identityFile)
}

// withB is ID-MUT-122's module whose binary also prints b.Twice(n), of a
// package b its own test never calls and both listed tests reach,
// committed. No line of main.go moves.
func (r *coverageReuse) withB() error {
	l := r.l
	if err := l.project(); err != nil {
		return err
	}
	if err := l.replace("main.go", "\t\"os\"\n", "\t\"os\"; \"example.com/greet/b\"\n"); err != nil {
		return err
	}
	if err := l.replace("main.go", "half(n))", "half(n), b.Twice(n))"); err != nil {
		return err
	}
	if err := l.m.w.write(listedBFile, listedB); err != nil {
		return err
	}
	if err := l.m.w.write(listedBTestFile, listedBTest); err != nil {
		return err
	}
	if err := r.commit("b", "main.go", listedBFile, listedBTestFile); err != nil {
		return err
	}
	return l.declare()
}

// evidence is the strict coverage evidence of each function of a/a.go the
// snapshot records, by its name.
func (r *coverageReuse) evidence() (map[string]any, error) {
	w := r.l.m.w
	data, err := w.readIn(w.dir, ".metrics/mutate/a/a.go.json")
	if err != nil {
		return nil, fmt.Errorf("%v\n%s", err, w.report())
	}
	var snap struct {
		Units []struct {
			Name     string `json:"name"`
			Coverage any    `json:"go_coverage"`
		} `json:"units"`
	}
	if err := json.Unmarshal(data, &snap); err != nil {
		return nil, err
	}
	out := map[string]any{}
	for _, u := range snap.Units {
		if u.Coverage == nil {
			return nil, fmt.Errorf("function %s has no coverage evidence\n%s", u.Name, w.report())
		}
		out[u.Name] = u.Coverage
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("a/a.go's snapshot records no function\n%s", w.report())
	}
	return out, nil
}

// sameEvidence compares the evidence the run recorded with what a strict
// run measuring afresh, its cache bypassed, records over the same tree:
// what first, run over it, would have recorded.
func (r *coverageReuse) sameEvidence(_ string) error {
	w := r.l.m.w
	reused, err := r.evidence()
	if err != nil {
		return err
	}
	if err := w.runWith(w.env(), "mutation run --mutate-all --fail-uncovered a/a.go --json --no-coverage-cache"); err != nil {
		return err
	}
	if err := w.exitsWith(0); err != nil {
		return err
	}
	measured, err := r.evidence()
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(reused, measured) {
		return fmt.Errorf("the reused evidence %v is not what a fresh measurement records, %v\n%s", reused, measured, w.report())
	}
	return nil
}

// strictAgain changes a/a.go's test, runs the strict run again, and reads
// the evidence it records.
func (r *coverageReuse) strictAgain() error {
	w := r.l.m.w
	test := r.l.m.read("a/a_test.go") + "\n// changed\n"
	if err := w.write("a/a_test.go", test); err != nil {
		return err
	}
	if err := w.runWith(w.env(), "mutation run --mutate-all --fail-uncovered a/a.go --json"); err != nil {
		return err
	}
	if len(r.measuring()) == 0 {
		return fmt.Errorf("the strict run did not measure the listed tests again\n%s", w.report())
	}
	evidence, err := r.evidence()
	if err != nil {
		return err
	}
	sum := sha256.Sum256([]byte(test))
	for name, e := range evidence {
		inputs, _ := e.(map[string]any)["inputs"].(map[string]any)
		if inputs["a/a_test.go"] != hex.EncodeToString(sum[:]) {
			return fmt.Errorf("%s's evidence records a/a_test.go as %v, not the changed test\n%s", name, inputs["a/a_test.go"], w.report())
		}
	}
	return nil
}

// listedBFile is package b's one file: Twice, whose `*` → `/` the binary
// runs for every listed test, and no test checks.
const listedBFile = "b/b.go"

// listedBLine is the line of Twice's site.
const listedBLine = 5

// listedBTestFile is b's own test, which records its runs as main_test.go
// does and calls nothing of b.
const listedBTestFile = "b/b_test.go"

const listedBTest = `package b

import (
	"encoding/json"
	"os"
	"testing"
)

func TestNothing(t *testing.T) {
	if record := os.Getenv("GREET_RECORD"); record != "" {
		f, err := os.OpenFile(record, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.NewEncoder(f).Encode(map[string]string{"test": "own", "coverdir": os.Getenv("ITOS_CC_TEST_COVERDIR")}); err != nil {
			t.Fatal(err)
		}
		f.Close()
	}
}
`

const listedB = "package b\n\n// Twice is n doubled.\nfunc Twice(n int) int {\n\treturn n * 2\n}\n"
