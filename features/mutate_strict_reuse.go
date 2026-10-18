package features

import (
	"fmt"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/cucumber/godog"
)

const strictPositiveGo = `package main

func Positive(n int) bool { return n > 0 }
func main() {}
`
const strictPositiveTest = `package main
import "testing"
func TestPositive(t *testing.T){if !Positive(1)||Positive(0){t.Fatal("Positive")}}
`
const strictPositiveReport = `mode: set
example.com/strict/main.go:3.29,3.43 1 1
`

func (q *strictMutation) positiveProject() error {
	q.function = "Positive"
	return q.goProject(strictPositiveGo, strictPositiveTest, strictPositiveReport)
}
func (q *strictMutation) reuseSteps(sc *godog.ScenarioContext) {
	sc.Step(`^a non-strict no-coverage run recorded all mutants killed and no independent statement evidence$`, func() error {
		if err := q.positiveProject(); err != nil {
			return err
		}
		a, err := q.fresh("mutation run --no-coverage --json main.go", 0)
		if err != nil {
			return err
		}
		f, err := languageRunFile(a, q.file)
		if err != nil {
			return err
		}
		if f["killed"] != float64(2) || f["survived"] != float64(0) {
			return fmt.Errorf("initial outcomes %v", f)
		}
		snap, err := q.snapshot()
		if err != nil {
			return err
		}
		u, err := languageUnit(snap, "Positive")
		if err != nil {
			return err
		}
		if u["go_coverage"] != nil {
			return fmt.Errorf("no-coverage created evidence: %v", u)
		}
		return nil
	})
	sc.Step(`^I run mutation run with --fail-uncovered and fresh inputs$`, func() error { return q.run("mutation run --fail-uncovered --json " + q.file) })
	sc.Step(`^coverage is measured even though those mutants can be reused$`, func() error {
		a, err := q.object()
		if err != nil {
			return err
		}
		f, err := languageRunFile(a, q.file)
		if err != nil {
			return err
		}
		if len(trialsOf(q.m.w.stderr, q.file)) != 0 || f["killed"] != float64(2) || !strings.Contains(q.m.w.stderr, "itos-cc: coverage ") {
			return fmt.Errorf("missing measurement/reuse: %v\n%s", f, q.m.w.stderr)
		}
		return q.m.w.exitsWith(0)
	})
	sc.Step(`^current independent evidence is recorded for every judged executable Go function$`, q.allEvidence)
	sc.Step(`^I repeat the strict run and cached check without any input change$`, func() error {
		_, log, code, err := q.capture("mutation run --fail-uncovered --json " + q.file)
		if err != nil {
			return err
		}
		if code != 0 {
			return fmt.Errorf("repeat strict exit%d", code)
		}
		q.log = log
		if err := q.observed(); err != nil {
			return err
		}
		return q.run("mutation check --fail-uncovered --json " + q.file)
	})
	sc.Step(`^the fresh evidence can be reused without another coverage measurement$`, func() error {
		if strings.Contains(q.log, "itos-cc: coverage ") {
			return fmt.Errorf("fresh evidence remeasured: %s", q.log)
		}
		return q.m.w.exitsWith(0)
	})
	sc.Step(`^cached check runs no test, list or coverage command and writes nothing$`, func() error { return q.quiet(q.m.w.stderr) })
	sc.Step(`^stale independent evidence makes a strict run measure again even if its mutant outcomes remain reusable$`, func() error {
		q.env = []string{"GOFLAGS=-buildvcs=false"}
		a, log, code, err := q.capture("mutation run --fail-uncovered --json " + q.file)
		if err != nil {
			return err
		}
		if code != 0 || !strings.Contains(log, "itos-cc: coverage ") || len(trialsOf(log, q.file)) != 0 {
			return fmt.Errorf("stale evidence did not remeasure with reused mutants exit%d: %v\n%s", code, a, log)
		}
		return q.allEvidence()
	})
	sc.Step(`^a Go function's mutation outcomes are fresh but its independent coverage evidence is (.+)$`, q.incomplete)
	sc.Step(`^I run mutation check with --fail-uncovered --json$`, func() error {
		if err := q.observed(); err != nil {
			return err
		}
		return q.run("mutation check --fail-uncovered --json " + q.file)
	})
	sc.Step(`^its problem is "mutation.coverage-missing", with file, function and line$`, func() error {
		a, err := q.object()
		if err != nil {
			return err
		}
		return q.finding(a, "mutation.coverage-missing", "Positive", 3)
	})
	sc.Step(`^no test, list or coverage command runs and no file is written$`, func() error { return q.quiet(q.m.w.stderr) })
	sc.Step(`^fresh strict coverage evidence for an unchanged Go function$`, q.unchangedEvidence)
	sc.Step(`^a (.+) input was (changed|added|removed) without changing that function$`, q.changeInput)
	sc.Step(`^I run mutation check with --fail-uncovered --json for the function's file$`, func() error { return q.run("mutation check --fail-uncovered --json " + q.file) })
	sc.Step(`^its problem is "mutation.coverage-stale", naming the changed input$`, func() error {
		a, err := q.object()
		if err != nil {
			return err
		}
		ps := q.findings(a, "mutation.coverage-stale", "Positive")
		if len(ps) != 1 || !strings.Contains(fmt.Sprint(ps[0]["message"]), q.input) {
			return fmt.Errorf("stale finding does not name %s: %v", q.input, ps)
		}
		return nil
	})
	q.blessSteps(sc)
	q.portableSteps(sc)
	q.commentSteps(sc)
}

// unchangedEvidence records strict evidence for Positive, beside a module
// helper, a support file and a listed harness, in the run's environment.
func (q *strictMutation) unchangedEvidence() error {
	if err := q.positiveProject(); err != nil {
		return err
	}
	for file, text := range map[string]string{"other.go": "package main\nvar Other = 1\n", "support/dep.txt": "initial\n", "itos-cc.yaml": "mutation:\n  tests:\n    list: go version\n    run: go version\n    support: ['support/**']\n"} {
		if err := q.m.w.write(file, text); err != nil {
			return err
		}
	}
	// A non-listed run supplies the measurement and source hashes. The support
	// configuration participates in evidence even though no mutant is listed.
	if err := q.m.w.write("itos-cc.yaml", "mutation:\n  tests:\n    list: go version\n    run: go version\n    support: ['support/**']\n"); err != nil {
		return err
	}
	// Listed harness setup is supplied below using an actual Go list program.
	return q.fingerprintSetup()
}

func (q *strictMutation) blessSteps(sc *godog.ScenarioContext) {
	sc.Step(`^a later strict run cannot bless the old profile with new fingerprints$`, func() error {
		before, err := q.snapshot()
		if err != nil {
			return err
		}
		u, err := languageUnit(before, "Positive")
		if err != nil {
			return err
		}
		old, err := strictEvidence(u)
		if err != nil {
			return err
		}
		a, log, code, err := q.capture("mutation run --fail-uncovered --json " + q.file)
		if err != nil {
			return err
		}
		if code != 0 || !strings.Contains(log, "itos-cc: coverage ") {
			return fmt.Errorf("stale profile not remeasured exit%d: %v\n%s", code, a, log)
		}
		after, err := q.snapshot()
		if err != nil {
			return err
		}
		u, err = languageUnit(after, "Positive")
		if err != nil {
			return err
		}
		current, err := strictEvidence(u)
		if err != nil {
			return err
		}
		if reflect.DeepEqual(old["inputs"], current["inputs"]) {
			return fmt.Errorf("measurement inputs not updated: %v", current)
		}
		return nil
	})
}
func (q *strictMutation) incomplete(mode string) error {
	if err := q.positiveProject(); err != nil {
		return err
	}
	if _, err := q.fresh("mutation run --fail-uncovered --json main.go", 0); err != nil {
		return err
	}
	snap, err := q.snapshot()
	if err != nil {
		return err
	}
	u, err := languageUnit(snap, "Positive")
	if err != nil {
		return err
	}
	e, err := strictEvidence(u)
	if err != nil {
		return err
	}
	switch mode {
	case "absent in a legacy snapshot":
		delete(u, "go_coverage")
	case "null":
		u["go_coverage"] = nil
	case "missing its executable function's block inventory":
		e["complete"] = false
		e["blocks"] = []any{}
	case "supplied only by measurement of an unrelated Go module":
		unrelated, err := q.unrelatedEvidence()
		if err != nil {
			return err
		}
		u["go_coverage"] = unrelated
	default:
		return fmt.Errorf("unknown incomplete evidence %q", mode)
	}
	return q.writeSnapshot(snap)
}

func (q *strictMutation) fingerprintSetup() error {
	if err := q.m.w.write("harness/main.go", `package main
import "fmt"
func main(){fmt.Println("TestPositive\tmain_test.go")}
`); err != nil {
		return err
	}
	config := `mutation:
  tests:
    list: go run ./harness
    run: go test -count=1 -failfast -run '{pattern}' .
    ids_pattern: '^({ids})$'
    join:
      each: '{id}'
      sep: '|'
    support: ['support/**']
`
	if err := q.m.w.write("itos-cc.yaml", config); err != nil {
		return err
	}
	t := Tests{Language: "go", Command: "go test -count=1 -failfast -run '^TestPositive$' .", Listed: &ListedTests{
		Files: map[string]string{"TestPositive": "main_test.go"}, Lines: map[string]map[int][]string{"main.go": {3: {"TestPositive"}}},
	}}
	if err := q.m.w.d.Tests(q.m.w.dir, t); err != nil {
		return err
	}
	if _, err := q.fresh("mutation run --fail-uncovered --json main.go", 0); err != nil {
		return err
	}
	return q.save()
}
func (q *strictMutation) changeInput(input, change string) error {
	q.input = ""
	switch input {
	case "module production helper":
		q.input = "other.go"
		if err := q.m.w.write("other.go", "package main\nvar Other = 2\n"); err != nil {
			return err
		}
	case "non-importing module test":
		q.input = "unrelated/unrelated_test.go"
		if err := q.m.w.write(q.input, "package unrelated\nimport \"testing\"\nfunc TestOther(t *testing.T){}\n"); err != nil {
			return err
		}
	case "configured support file":
		q.input = "support:support/dep.txt"
		if err := q.m.w.d.Remove(filepath.Join(q.m.w.dir, "support", "dep.txt")); err != nil {
			return err
		}
	case "module go.mod":
		q.input = "go.mod"
		if err := q.m.w.write("go.mod", q.m.read("go.mod")+"\n// measurement input changed\n"); err != nil {
			return err
		}
	case "recorded coverage build option":
		q.input = "@go-env"
		q.env = []string{"GOFLAGS=-buildvcs=false"}
	default:
		return fmt.Errorf("unknown input %s %s", input, change)
	}
	old, err := languageUnit(q.saved, "Positive")
	if err != nil {
		return err
	}
	snap, err := q.snapshot()
	if err != nil {
		return err
	}
	now, err := languageUnit(snap, "Positive")
	if err != nil {
		return err
	}
	if old["hash"] != now["hash"] || !strings.Contains(q.source(), "return n > 0") {
		return fmt.Errorf("fingerprint fixture changed judged function")
	}
	return nil
}

func (q *strictMutation) unrelatedEvidence() (map[string]any, error) {
	w := q.m.w
	original := w.dir
	dir, err := w.d.Folder()
	if err != nil {
		return nil, err
	}
	q.outside = append(q.outside, dir)
	w.dir = dir
	defer func() { w.dir = original }()
	other := strictMutation{m: q.m, s: q.s, file: "main.go"}
	if err := other.goProject(`package main

func Other(n int)bool{return n > 5}
func main(){}
`, `package main
import "testing"
func TestOther(t *testing.T){if Other(5)||!Other(6){t.Fatal("Other")}}
`, "mode: set\nexample.com/unrelated/main.go:3.23,3.36 1 1\n"); err != nil {
		return nil, err
	}
	if err := w.write("go.mod", "module example.com/unrelated\n\ngo 1.22\n"); err != nil {
		return nil, err
	}
	if _, err := other.fresh("mutation run --fail-uncovered --json main.go", 0); err != nil {
		return nil, err
	}
	snap, err := other.snapshot()
	if err != nil {
		return nil, err
	}
	u, err := languageUnit(snap, "Other")
	if err != nil {
		return nil, err
	}
	evidence, err := strictEvidence(u)
	if err != nil {
		return nil, err
	}
	return evidence, nil
}
