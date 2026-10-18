package features

import (
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/cucumber/godog"
)

const strictPartialGo = `package main

func Edited(n int) bool { return n > 0 }
func Stable(flag bool) string { if flag { return "yes" }; return "no" }
func init() { _ = Stable }
func init() { _ = Edited }
func main() {}
`
const strictPartialTest = `package main
import "testing"
func TestFunctions(t *testing.T){if !Edited(1)||Edited(0)||Stable(true)!="yes"{t.Fatal("functions")}}
`

const strictMovedPartialReport = `mode: set
example.com/strict/main.go:3.27,3.42 1 1
example.com/strict/main.go:6.33,6.41 1 1
example.com/strict/main.go:6.43,6.57 1 1
example.com/strict/main.go:6.59,6.70 1 0
example.com/strict/main.go:7.15,7.27 1 1
example.com/strict/main.go:8.15,8.27 1 1
`

func (q *strictMutation) partialSteps(sc *godog.ScenarioContext) {
	sc.Step(`^two Go functions have fresh independent coverage evidence$`, func() error {
		if err := q.goProject(strictPartialGo, strictPartialTest, `mode: set
example.com/strict/main.go:3.27,3.41 1 1
example.com/strict/main.go:4.33,4.41 1 1
example.com/strict/main.go:4.43,4.57 1 1
example.com/strict/main.go:4.59,4.70 1 0
example.com/strict/main.go:5.15,5.27 1 1
example.com/strict/main.go:6.15,6.27 1 1
`); err != nil {
			return err
		}
		if _, err := q.fresh("mutation run --fail-uncovered --json main.go", 1); err != nil {
			return err
		}
		if err := q.allEvidence(); err != nil {
			return err
		}
		if err := q.m.w.git("init", "-q", "-b", "main"); err != nil {
			return err
		}
		if err := q.s.commit("base"); err != nil {
			return err
		}
		if err := q.m.w.git("tag", "base"); err != nil {
			return err
		}
		return q.save()
	})
	sc.Step(`^a module test changed since, making both inventories stale$`, func() error {
		tests := strings.Replace(strictPartialTest, "Edited(0)", "!Edited(0)", 1)
		if err := q.m.w.write("main_test.go", tests); err != nil {
			return err
		}
		a, _, code, err := q.capture("mutation check --fail-uncovered --json main.go")
		if err != nil {
			return err
		}
		if code != 1 {
			return fmt.Errorf("changed test check exit%d %v", code, a)
		}
		for _, name := range []string{"Edited", "Stable"} {
			if len(q.findings(a, "mutation.coverage-stale", name)) != 1 {
				return fmt.Errorf("missing stale %s: %v", name, a)
			}
		}
		return nil
	})
	sc.Step(`^only one function changed in commits since "base"$`, func() error {
		source := strings.Replace(strictPartialGo, "return n > 0", "return n >= 0", 1)
		source = strings.Replace(source, "func Stable", "\n// Stable moves without changing its body.\nfunc Stable", 1)
		if err := q.m.w.write(q.file, source); err != nil {
			return err
		}
		if err := q.m.w.d.Tests(q.m.w.dir, Tests{Language: "go", Command: "go test -count=1 -failfast ./."}); err != nil {
			return err
		}
		if err := q.m.w.d.Coverage(q.m.w.dir, Coverage{Language: "go", Report: strictMovedPartialReport}); err != nil {
			return err
		}
		return q.s.commit("change Edited and its test")
	})
	sc.Step(`^I run mutation run with --since base --fail-uncovered$`, func() error { return q.run("mutation run --since base --fail-uncovered --json main.go") })
	sc.Step(`^only the changed function is judged and gets newly measured evidence$`, func() error {
		a, err := q.object()
		if err != nil {
			return err
		}
		f, err := languageRunFile(a, q.file)
		if err != nil {
			return err
		}
		raw, ok := f["judged"].([]any)
		if !ok || len(raw) != 1 || !strictNamed(fmt.Sprint(raw[0]), "Edited") {
			return fmt.Errorf("wrong partial judgment: %v", f)
		}
		if err := q.m.w.exitsWith(0); err != nil {
			return err
		}
		snap, err := q.snapshot()
		if err != nil {
			return err
		}
		old, err := languageUnit(q.saved, "Edited")
		if err != nil {
			return err
		}
		now, err := languageUnit(snap, "Edited")
		if err != nil {
			return err
		}
		if old["hash"] == now["hash"] {
			return errors.New("edited hash unchanged")
		}
		if _, err := strictEvidence(now); err != nil {
			return err
		}
		if reflect.DeepEqual(old["go_coverage"], now["go_coverage"]) {
			return errors.New("edited reused old evidence")
		}
		return nil
	})
	sc.Step(`^the unjudged function keeps its original stale evidence$`, func() error {
		snap, err := q.snapshot()
		if err != nil {
			return err
		}
		old, err := languageUnit(q.saved, "Stable")
		if err != nil {
			return err
		}
		now, err := languageUnit(snap, "Stable")
		if err != nil {
			return err
		}
		if _, err := strictEvidence(old); err != nil {
			return err
		}
		if old["hash"] != now["hash"] || !reflect.DeepEqual(old["go_coverage"], now["go_coverage"]) {
			return fmt.Errorf("unjudged Stable evidence changed: %v vs %v", old, now)
		}
		q.partial = snap
		return nil
	})
	sc.Step(`^strict check of the unjudged function cannot pass$`, func() error {
		a, _, code, err := q.capture("mutation check --fail-uncovered --json main.go")
		if err != nil {
			return err
		}
		if code != 1 {
			return fmt.Errorf("unjudged strict check exit%d", code)
		}
		return q.finding(a, "mutation.coverage-stale", "Stable", 6)
	})
	sc.Step(`^moving a function down within its file reports uncovered blocks at their current lines after current measurement$`, func() error {
		a, log, code, err := q.capture("mutation run --fail-uncovered --json main.go")
		if err != nil {
			return err
		}
		if code != 1 || !strings.Contains(log, "itos-cc: coverage ") {
			return fmt.Errorf("moved evidence not measured exit%d: %v\n%s", code, a, log)
		}
		return q.finding(a, "mutation.uncovered-statement", "Stable", 6)
	})
	sc.Step(`^repeated init functions retain the correct separate inventories when paired by name and hash$`, q.initPairs)
}

func (q *strictMutation) initPairs() error {
	snap, err := q.snapshot()
	if err != nil {
		return err
	}
	currentEdited, err := languageUnit(snap, "Edited")
	if err != nil {
		return err
	}
	currentEvidence, err := strictEvidence(currentEdited)
	if err != nil {
		return err
	}
	old := map[string]map[string]any{}
	for _, u := range objectList(q.saved, "units") {
		if u["name"] == "init" {
			old[fmt.Sprint(u["hash"])] = u
		}
	}
	if len(old) != 2 {
		return fmt.Errorf("missing distinct original init identities: %v", old)
	}
	// The partial run must preserve each original inventory even while its
	// source has moved. The later full measurement uses current inputs and
	// current positions, including both endpoints of each independently saved span.
	for stage, state := range map[string]map[string]any{"partial": q.partial, "full": snap} {
		seen := map[string]bool{}
		for _, u := range objectList(state, "units") {
			if u["name"] != "init" {
				continue
			}
			hash := fmt.Sprint(u["hash"])
			delta := float64(0)
			if stage == "full" {
				delta = 2
			}
			before, ok := old[hash]
			if !ok || seen[hash] || u["namespace"] != before["namespace"] || u["start_line"] != before["start_line"].(float64)+delta {
				return fmt.Errorf("%s init identity/position not paired: %v", stage, u)
			}
			seen[hash] = true
			was, err := strictEvidence(before)
			if err != nil {
				return err
			}
			now, err := strictEvidence(u)
			if err != nil {
				return err
			}
			if stage == "partial" {
				if !reflect.DeepEqual(was, now) {
					return fmt.Errorf("partial init evidence changed for %s: %v vs %v", hash, was, now)
				}
				continue
			}
			blocks := objectList(was, "blocks")
			if len(blocks) != 1 || blocks[0]["line"] != before["start_line"] || blocks[0]["covered"] != true || blocks[0]["span"] != fmt.Sprintf("%v.15,%v.27", before["start_line"], before["start_line"]) {
				return fmt.Errorf("original init population not independently recorded: %v", before)
			}
			want := map[string]any{}
			for k, v := range blocks[0] {
				want[k] = v
			}
			want["line"] = blocks[0]["line"].(float64) + 2
			want["span"] = fmt.Sprintf("%v.15,%v.27", want["line"], want["line"])
			if !reflect.DeepEqual(objectList(now, "blocks"), []map[string]any{want}) || now["producer"] != was["producer"] || !reflect.DeepEqual(now["inputs"], currentEvidence["inputs"]) {
				return fmt.Errorf("full init inventory/provenance not paired for %s: %v want block %v/current inputs %v", hash, now, want, currentEvidence["inputs"])
			}
		}
		if len(seen) != 2 {
			return fmt.Errorf("%s init population %d, want 2", stage, len(seen))
		}
	}
	return nil
}
