package features

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/cucumber/godog"
)

func (q *strictMutation) modeSteps(sc *godog.ScenarioContext) {
	sc.Step(`^I combine --fail-uncovered with --no-coverage, even for an empty selection$`, func() error { return q.run("mutation run --fail-uncovered --no-coverage --json") })
	sc.Step(`^the problem is "flags.conflict" for "--no-coverage" and the exit code is 80$`, func() error {
		a, err := q.object()
		if err != nil {
			return err
		}
		ps := q.findings(a, "flags.conflict", "")
		if len(ps) != 1 || ps[0]["flag"] != "--no-coverage" {
			return fmt.Errorf("empty strict conflict: %v", a)
		}
		return q.m.w.exitsWith(80)
	})
	sc.Step(`^strict Go run is combined with --coverage-report, --use-existing-coverage or --coverage-command$`, func() error {
		if err := q.goProject(strictBaseGo, strictBaseTest, strictBaseReport); err != nil {
			return err
		}
		if err := q.m.w.write("missing.lcov", "this is deliberately not a coverage report\n"); err != nil {
			return err
		}
		var err error
		q.observe, err = q.m.w.d.ObserveWrites(filepath.Join(q.m.w.dir, q.file), filepath.Join(q.m.w.dir, "missing.lcov"))
		if err != nil {
			return err
		}
		return q.reportFlags()
	})
	sc.Step(`^the problem is "flags.conflict" naming the incompatible report flag and the exit code is 80$`, func() error {
		// Each report flag's exact subject and exit was captured by the preceding
		// operation; repeat here as the scenario's assertion, preserving every result.
		if len(q.flags) != 3 {
			return fmt.Errorf("missing report flag observations")
		}
		for _, r := range q.flags {
			if r.code != 80 || len(checkProblems(r.object)) != 1 || checkProblems(r.object)[0]["rule"] != "flags.conflict" || checkProblems(r.object)[0]["flag"] != r.flag {
				return fmt.Errorf("incompatible report flag %s exit%d: %v", r.flag, r.code, r.object)
			}
		}
		return nil
	})
	sc.Step(`^no report, coverage command or mutant is consumed to bless strict evidence$`, func() error {
		for _, r := range q.flags {
			if strings.Contains(r.log, "itos-cc: coverage ") || len(baselines(r.log)) != 0 || len(trialsOf(r.log, q.file)) != 0 {
				return fmt.Errorf("incompatible flag executed work: %s", r.log)
			}
		}
		changed, err := q.observe()
		if err != nil {
			return err
		}
		if len(changed) != 0 || q.m.w.d.Exists(filepath.Join(q.m.w.dir, ".metrics")) {
			return fmt.Errorf("strict flags consumed inputs or wrote evidence: %v", changed)
		}
		return nil
	})
	sc.Step(`^non-strict explicit-report and no-coverage runs retain their current behavior$`, func() error {
		if err := q.m.w.write("coverage.out", strictBaseReport); err != nil {
			return err
		}
		for _, option := range []string{"--coverage-report coverage.out", "--no-coverage"} {
			a, log, code, err := q.capture("mutation run --json " + option + " main.go")
			if err != nil {
				return err
			}
			f, err := languageRunFile(a, q.file)
			if err != nil {
				return err
			}
			if code != 0 || f["killed"] != float64(1) || f["uncovered"] != float64(0) || len(strictLineFacts(a)) != 0 || strings.Contains(log, "itos-cc: coverage ") {
				return fmt.Errorf("non-strict option %s exit%d: %v\n%s", option, code, a, log)
			}
		}
		return nil
	})
	sc.Step(`^other-language statement coverage, ordinary graph/sample freshness and survivor exceptions are unchanged$`, q.otherModes)
	sc.Step(`^a survivor exception cannot excuse a strict Go uncovered-statement finding$`, func() error {
		a, _, code, err := q.capture("mutation run --fail-uncovered --json main.go")
		if err != nil {
			return err
		}
		f, err := languageRunFile(a, q.file)
		if err != nil {
			return err
		}
		if code != 1 || f["excepted"] != float64(1) || f["survived"] != float64(0) || len(q.findings(a, "mutation.survived", "")) != 0 {
			return fmt.Errorf("strict exception behavior exit%d: %v", code, a)
		}
		return q.finding(a, "mutation.uncovered-statement", "Choose", 4)
	})
}

type strictFlagObservation struct {
	flag, log string
	code      int
	object    map[string]any
}

func (q *strictMutation) otherModes(ctx context.Context) error {
	// Each real language collector is observed in its own owned project root.
	// Sharing a source folder's declaration must not replace another language's.
	w := q.m.w
	original := w.dir
	for _, language := range []string{"python", "typescript", "kotlin"} {
		dir, err := w.d.Folder()
		if err != nil {
			return err
		}
		q.outside = append(q.outside, dir)
		w.dir = dir
		other := strictMutation{m: q.m, s: q.s}
		err = other.lineProject(language)
		if err == nil {
			err = other.run("mutation run --fail-uncovered --json " + other.paths())
		}
		if err == nil {
			err = other.strictLines()
		}
		w.dir = original
		if err != nil {
			return err
		}
	}
	q.file = "main.go"
	q.selected = nil
	tests := strings.Replace(strictBaseTest, "Compare(5)", "!Compare(6)", 1)
	if err := w.write("main_test.go", tests); err != nil {
		return err
	}
	if err := w.d.Tests(w.dir, Tests{Language: "go", Command: "go test -count=1 -failfast ./.", Surviving: []Site{{File: q.file, Line: 3, Original: ">", Replacement: ">="}}}); err != nil {
		return err
	}
	if _, err := q.fresh("mutation run --json main.go", 1); err != nil {
		return err
	}
	if _, err := q.fresh("mutation except main.go:3:37 --reason 'reviewed comparison' --json", 0); err != nil {
		return err
	}
	a, _, code, err := q.capture("mutation check --json main.go")
	if err != nil {
		return err
	}
	if code != 0 || len(checkProblems(a)) != 0 {
		return fmt.Errorf("reviewed survivor check exit%d: %v", code, a)
	}
	if err := q.graphStates(ctx, map[string]bool{"Compare": false, "Choose": false, "main": false}); err != nil {
		return err
	}
	sample, _, code, err := q.capture("mutation sample --count 50 --seed 4813e48 --json main.go")
	if err != nil {
		return err
	}
	f, err := languageRunFile(sample, q.file)
	if err != nil {
		return err
	}
	mutants := objectList(f, "mutants")
	if code != 0 || len(mutants) != 1 || !strictNamed(fmt.Sprint(mutants[0]["function"]), "Compare") || mutants[0]["outcome"] != "survived" {
		return fmt.Errorf("fresh ordinary sample exit%d: %v", code, sample)
	}
	if err := w.write("main_test.go", tests+"\n// importer changed\n"); err != nil {
		return err
	}
	a, _, code, err = q.capture("mutation check --json main.go")
	if err != nil {
		return err
	}
	if code != 1 || len(q.findings(a, "mutation.stale", "Compare")) != 1 {
		return fmt.Errorf("ordinary stale check exit%d: %v", code, a)
	}
	if err := q.graphStates(ctx, map[string]bool{"Compare": true, "Choose": true, "main": true}); err != nil {
		return err
	}
	sample, _, code, err = q.capture("mutation sample --count 50 --seed 4813e48 --json main.go")
	if err != nil {
		return err
	}
	if code != 0 || len(objectList(sample, "files")) != 0 {
		return fmt.Errorf("ordinary stale sample admitted outcomes exit%d: %v", code, sample)
	}
	return nil
}
func (q *strictMutation) graphStates(ctx context.Context, want map[string]bool) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	raw, err := q.m.w.d.Graph(ctx, Run{Dir: q.m.w.dir, Env: q.m.w.env()})
	if err != nil {
		return err
	}
	var graph map[string]any
	if err = json.Unmarshal(raw, &graph); err != nil {
		return err
	}
	got := map[string]bool{}
	for _, node := range objectList(graph, "nodes") {
		for _, u := range objectList(node, "units") {
			if filepath.ToSlash(fmt.Sprint(u["file"])) != q.file {
				continue
			}
			stale, ok := u["stale"].(bool)
			if !ok {
				return fmt.Errorf("graph lacks stale observation: %v", u)
			}
			name := fmt.Sprint(u["name"])
			if _, ok := got[name]; ok {
				return fmt.Errorf("duplicate graph unit %s", name)
			}
			got[name] = stale
		}
	}
	if !reflect.DeepEqual(got, want) {
		return fmt.Errorf("graph complete population %v want%v", got, want)
	}
	return nil
}
