package features

import (
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"slices"
	"strings"

	"github.com/cucumber/godog"
)

func (q *strictMutation) scopeSteps(sc *godog.ScenarioContext) {
	sc.Step(`^a Go function has fresh strict coverage evidence$`, func() error {
		if err := q.positiveProject(); err != nil {
			return err
		}
		_, err := q.fresh("mutation run --fail-uncovered --json main.go", 0)
		return err
	})
	sc.Step(`^an active go.work is introduced for its measurement environment$`, func() error { return q.m.w.write("go.work", "go 1.22\n\nuse .\n") })
	sc.Step(`^strict run and cached strict check fail with "mutation.coverage-unsupported", naming the workspace$`, func() error { return q.unsupportedGo("go.work") })
	sc.Step(`^cached check does not run coverage or tests or refresh its evidence$`, func() error {
		if err := q.observed(); err != nil {
			return err
		}
		_, log, code, err := q.capture("mutation check --fail-uncovered --json main.go")
		if err != nil {
			return err
		}
		if code != 1 {
			return fmt.Errorf("unsupported check exit%d", code)
		}
		return q.quiet(log)
	})
	sc.Step(`^with GOWORK=off the same inventoried module remains supported$`, func() error {
		q.env = []string{"GOWORK=off"}
		for _, cmd := range []string{"check", "run"} {
			if _, err := q.fresh("mutation "+cmd+" --fail-uncovered --json main.go", 0); err != nil {
				return err
			}
		}
		q.env = nil
		return nil
	})
	sc.Step(`^its go.mod names a local replacement outside the inventoried module$`, func() error {
		if err := q.m.w.d.Remove(filepath.Join(q.m.w.dir, "go.work")); err != nil {
			return err
		}
		dir, err := q.m.w.d.Folder()
		if err != nil {
			return err
		}
		q.outside = append(q.outside, dir)
		if err := q.externalReplacement(dir); err != nil {
			return err
		}
		q.externalReads = q.reads
		return q.replacement(dir)
	})
	sc.Step(`^strict run and cached strict check fail with "mutation.coverage-unsupported", naming the replacement$`, func() error { return q.unsupportedGo(filepath.ToSlash(q.outside[len(q.outside)-1])) })
	sc.Step(`^a replacement beneath an excluded nested module is refused too$`, func() error {
		dir := filepath.Join(q.m.w.dir, "replacement")
		if err := q.externalReplacement(dir); err != nil {
			return err
		}

		if err := q.m.w.write("go.mod", "module example.com/strict\n\ngo 1.22\n"); err != nil {
			return err
		}
		control, log, code, err := q.capture("mutation check --fail-uncovered --json main.go")
		if err != nil {
			return err
		}
		read, err := q.reads()
		if err != nil {
			return err
		}
		want := []string{dir, filepath.Join(dir, "go.mod"), filepath.Join(dir, "replacement.go")}
		slices.Sort(want)
		if code != 0 || !slices.Equal(read, want) || len(checkProblems(control)) != 0 {
			return fmt.Errorf("no-replacement discovery control exit%d reads%v want%v: %v", code, read, want, control)
		}
		if err := q.quiet(log); err != nil {
			return err
		}
		if err := q.externalReplacement(dir); err != nil {
			return err
		}
		if err := q.replacement("./replacement"); err != nil {
			return err
		}
		return q.unsupportedGo("./replacement")
	})
	sc.Step(`^no external dependency tree is followed or admitted as fresh coverage$`, func() error {
		if q.externalReads == nil {
			return fmt.Errorf("missing external read observation")
		}
		read, err := q.externalReads()
		if err != nil {
			return err
		}
		if len(read) != 0 {
			return fmt.Errorf("product read external replacement: %v", read)
		}
		// The current snapshot must retain the old inputs; unsupported commands
		// cannot stamp the lexical replacement as a measured dependency.
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
		for key := range e["inputs"].(map[string]any) {
			if strings.Contains(key, "replacement") {
				return fmt.Errorf("external tree admitted: %v", e)
			}
		}
		return nil
	})
	sc.Step(`^replacement inputs already inside the actual inventory remain supported$`, func() error {
		if err := q.replacement("."); err != nil {
			return err
		}
		for _, cmd := range []string{"run", "check"} {
			if _, err := q.fresh("mutation "+cmd+" --fail-uncovered --json main.go", 0); err != nil {
				return err
			}
		}
		return nil
	})
	sc.Step(`^non-strict commands retain their existing behavior$`, func() error {
		if err := q.m.w.write("go.work", "go 1.22\n\nuse .\n"); err != nil {
			return err
		}
		for _, cmd := range []string{"run", "check"} {
			if _, err := q.fresh("mutation "+cmd+" --json main.go", 0); err != nil {
				return err
			}
		}
		return nil
	})
}
func (q *strictMutation) replacement(target string) error {
	return q.m.w.write("go.mod", "module example.com/strict\n\ngo 1.22\n\nreplace example.com/replacement => "+filepath.ToSlash(target)+"\n")
}
func (q *strictMutation) externalReplacement(dir string) error {
	for name, text := range map[string]string{"go.mod": "module example.com/replacement\n\ngo 1.22\n", "replacement.go": "package replacement\nfunc Value() string {return \"outside\"}\n"} {
		if err := q.m.w.d.Write(filepath.Join(dir, name), []byte(text)); err != nil {
			return err
		}
	}
	var err error
	q.reads, err = q.m.w.d.ObserveReads(dir, filepath.Join(dir, "go.mod"), filepath.Join(dir, "replacement.go"))
	if errors.Is(err, ErrReadObservationUnsupported) {
		fmt.Printf("read observation omission: %v\n", err)
		return godog.ErrSkip
	}
	return err
}
func (q *strictMutation) unsupportedGo(scope string) error {
	before, err := q.snapshot()
	if err != nil {
		return err
	}
	u, err := languageUnit(before, "Positive")
	if err != nil {
		return err
	}
	if _, err := strictEvidence(u); err != nil {
		return err
	}
	if err := q.observed(); err != nil {
		return err
	}
	for _, cmd := range []string{"run", "check"} {
		a, log, code, err := q.capture("mutation " + cmd + " --fail-uncovered --json main.go")
		if err != nil {
			return err
		}
		ps := q.findings(a, "mutation.coverage-unsupported", "Positive")
		if code != 1 || len(ps) != 1 || !strings.Contains(filepath.ToSlash(fmt.Sprint(ps[0]["message"])), scope) {
			return fmt.Errorf("unsupported %s exit%d scope%s: %v\n%s", cmd, code, scope, a, log)
		}
		if err := q.quiet(log); err != nil {
			return err
		}
		after, err := q.snapshot()
		if err != nil {
			return err
		}
		if !reflect.DeepEqual(before, after) {
			return fmt.Errorf("unsupported %s refreshed independent artifact: %v vs %v", cmd, before, after)
		}
	}
	return nil
}
