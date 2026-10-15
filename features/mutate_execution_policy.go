package features

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/cucumber/godog"
)

type executionObservation struct {
	Object   map[string]any
	Exit     int
	Out, Log string
}

func (q *mutationExecution) capture(args ...string) (executionObservation, error) {
	code, out, log, err := q.m.w.capture(q.m.w.dir, q.m.w.env(), args...)
	r := executionObservation{Exit: code, Out: out, Log: log}
	if err == nil && slices.Contains(args, "--json") {
		err = json.Unmarshal([]byte(out), &r.Object)
	}
	return r, err
}
func (q *mutationExecution) save(key string, o executionObservation) {
	if q.observations == nil {
		q.observations = map[string]executionObservation{}
	}
	q.observations[key] = o
}
func (q *mutationExecution) policySteps(sc *godog.ScenarioContext) {
	sc.Step(`^selected mutants that are killed, time out, are validly excepted survivors, or survive their own tests but are killed by applicable listed tests$`, q.nonActionableProject)
	sc.Step(`^every selected mutant is judged and the run succeeds$`, q.allNonActionable)
	sc.Step(`^a survivor waits for its applicable listed tests before it can stop the run$`, q.listedBeforeStop)
	sc.Step(`^a fail-fast run that stops with completed, cancelled, unattempted and blocked work$`, func() error {
		if err := q.listedProject("idle", "quick", "wait", "label", "spare"); err != nil {
			return err
		}
		return q.failListed(true)
	})
	sc.Step(`^its report is emitted as JSON and as text$`, q.emitReports)
	sc.Step(`^JSON is one object with a stop naming the trigger's rule and subject$`, q.listedBlocked)
	sc.Step(`^it gives disjoint completed, cancelled, unattempted and blocked counts and every selected file's state$`, func() error {
		return q.disjointWork(map[string]int{"completed": 2, "cancelled": 1, "unattempted": 1, "blocked": 1})
	})
	sc.Step(`^plain output says the run stopped early and why$`, q.plainStopped)
	sc.Step(`^a run without --fail-fast keeps its existing output exactly$`, q.aggregateShape)
	sc.Step(`^a fail-fast run that ran one mutant of a file and reused another before a failing listed selection blocked a third$`, q.blockedCountsProject)
	sc.Step(`^that file reports "ran" 1 and "reused" 1$`, q.blockedCounts)
	sc.Step(`^those agree with its completed work count$`, q.countsAgree)
	sc.Step(`^a run without --fail-fast reports the same file as before$`, q.aggregateBlocked)
}
func (q *mutationExecution) addSelected(names ...string) error {
	source := q.source["main.go"]
	for _, name := range names {
		head := "func " + name + "(n int) bool {\n"
		source = strings.Replace(source, head, head+"\t// newly selected\n", 1)
	}
	q.source["main.go"] = source
	if err := q.m.w.write("main.go", source); err != nil {
		return err
	}
	if err := q.m.w.d.Git(q.m.w.dir, "add", "main.go", "itos-cc.yaml"); err != nil {
		return err
	}
	if err := q.m.w.d.Git(q.m.w.dir, "commit", "-qm", "select more functions"); err != nil {
		return err
	}
	if err := q.listedDeclarations(nil); err != nil {
		return err
	}
	q.listedSelected = append(q.listedSelected, names...)
	return q.measureListedCoverage()
}
func (q *mutationExecution) nonActionableProject() error {
	if err := q.listedProject("label"); err != nil {
		return err
	}
	if err := q.listedRun(false, 1); err != nil {
		return err
	}
	label, err := executionMutant(q.result, "main.go", "label")
	if err != nil {
		return err
	}
	if q.m.w.exit != 1 || label["outcome"] != "survived" {
		return fmt.Errorf("reviewed label survivor: %v", q.result)
	}
	location := fmt.Sprintf("main.go:%.0f:%.0f", label["line"], label["column"])
	o, err := q.capture("mutation", "except", location, "--reason", "equivalent on tested input", "--json")
	if err != nil {
		return err
	}
	if o.Exit != 0 {
		return fmt.Errorf("record reviewed exception: %v", o)
	}
	if err = q.addSelected("shown", "quick", "wait"); err != nil {
		return err
	}
	q.args = "--since base"
	return nil
}
func (q *mutationExecution) allNonActionable() error {
	if q.m.w.exit != 0 || q.result["ok"] != true || executionMap(q.result, "stop")["stopped"] != false || executionMap(q.result, "work")["completed"] != float64(4) {
		return fmt.Errorf("non-actionable run stopped: %v", q.result)
	}
	for name, want := range map[string]string{"quick": "killed", "wait": "timeout", "shown": "killed", "label": "survived"} {
		mu, err := executionMutant(q.result, "main.go", name)
		if err != nil {
			return err
		}
		if mu["state"] != "completed" || mu["outcome"] != want || (name == "label" && mu["excepted"] == nil) {
			return fmt.Errorf("non-actionable judgment %s: %v", name, mu)
		}
	}
	return nil
}
func (q *mutationExecution) listedBeforeStop() error {
	mu, err := executionMutant(q.result, "main.go", "shown")
	if err != nil {
		return err
	}
	if mu["scope"] != "listed" || fmt.Sprint(mu["tests"]) != "[ID-A-01 ID-A-02]" || mu["outcome"] != "killed" || executionMap(q.result, "stop")["stopped"] != false {
		return fmt.Errorf("survivor did not wait for applicable listed judgment: %v", q.result)
	}
	return nil
}
func (q *mutationExecution) emitReports() error {
	workers := 2
	if q.id == "211" {
		workers = 1
	}
	if err := q.listedRun(true, workers); err != nil {
		return err
	}
	target := q
	if q.twin != nil {
		target = q.twin
	}
	if err := target.m.w.d.Remove(filepath.Join(target.outside, "started")); err != nil {
		return err
	}
	o, err := target.capture("mutation", "run", "--workers", strconv.Itoa(workers), "--timeout-factor", "10000", "--since", "base", "--fail-fast")
	if err != nil {
		return err
	}
	q.save("plain", o)
	return nil
}
func (q *mutationExecution) plainStopped() error {
	o := q.observations["plain"]
	if o.Exit != 1 || !strings.Contains(o.Out, "stopped early") || !strings.Contains(o.Out, "tests.selection-failed") {
		return fmt.Errorf("plain stopped report: %+v", o)
	}
	return nil
}
func (q *mutationExecution) aggregateShape() error {
	if err := q.m.w.d.Remove(filepath.Join(q.m.w.dir, "itos-cc.yaml")); err != nil {
		return err
	}
	if err := q.three(); err != nil {
		return err
	}
	plain, err := q.capture("mutation", "run", "--no-coverage", "--no-annotate", "--workers", "1", "--test-command", q.command)
	if err != nil {
		return err
	}
	const want = "main.go: 2 killed, 1 survived, 0 uncovered (ran 3, reused 0)\n  survived main.go:3:34 `>` → `>=` in example.com/ownership#Weak\n"
	if plain.Exit != 1 || plain.Out != want {
		return fmt.Errorf("aggregate exact plain output: exit%d got%q want%q", plain.Exit, plain.Out, want)
	}
	o, err := q.capture("mutation", "run", "--json", "--no-coverage", "--no-annotate", "--mutate-all", "--workers", "1", "--test-command", q.command)
	if err != nil {
		return err
	}
	var keys []string
	for key := range o.Object {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	if o.Exit != 1 || !slices.Equal(keys, []string{"files", "ok", "problems", "schema"}) {
		return fmt.Errorf("aggregate exact JSON keys: %+v", o)
	}
	for _, file := range objectList(o.Object, "files") {
		if file["state"] != nil {
			return fmt.Errorf("aggregate invented file state: %v", file)
		}
		for _, mu := range objectList(file, "mutants") {
			if mu["state"] != nil {
				return fmt.Errorf("aggregate invented mutant state: %v", mu)
			}
		}
	}
	return nil
}
func (q *mutationExecution) countFixture() error {
	if err := q.listedProject("shown"); err != nil {
		return err
	}
	if err := q.listedRun(false, 1); err != nil {
		return err
	}
	mu, err := executionMutant(q.result, "main.go", "shown")
	if err != nil {
		return err
	}
	if q.m.w.exit != 0 || mu["outcome"] != "killed" || mu["scope"] != "listed" {
		return fmt.Errorf("prior listed kill: %v", q.result)
	}
	if err = q.addSelected("quick", "label"); err != nil {
		return err
	}
	return q.failListed(false)
}
func (q *mutationExecution) blockedCountsProject() error {
	if err := q.countFixture(); err != nil {
		return err
	}
	// Aggregate failing-baseline controls do not publish judgments or overwrite
	// the prior cache. They precede the fail-fast When, with actual inputs intact.
	o, err := q.capture("mutation", "run", "--json", "--no-annotate", "--since", "base", "--workers", "1")
	if err != nil {
		return err
	}
	q.save("aggregate", o)
	o, err = q.capture("mutation", "run", "--no-annotate", "--since", "base", "--workers", "1")
	if err != nil {
		return err
	}
	q.save("aggregatePlain", o)
	dir, err := q.m.w.d.Folder()
	if err != nil {
		return err
	}
	q.extraDirs = append(q.extraDirs, dir)
	w := &world{d: q.m.w.d, dir: dir}
	twin := &mutationExecution{m: &mutateState{w: w}, s: &sinceMutation{}, command: q.command, id: q.id}
	twin.s.m = twin.m
	if err = twin.countFixture(); err != nil {
		return err
	}
	q.extraDirs = append(q.extraDirs, twin.outside)
	q.twin = twin
	return nil
}
func (q *mutationExecution) blockedCounts() error {
	if err := q.listedBlocked(); err != nil {
		return err
	}
	file, err := executionFile(q.result, "main.go")
	if err != nil {
		return err
	}
	if file["ran"] != float64(1) || file["reused"] != float64(1) {
		return fmt.Errorf("blocked counted work: %v", file)
	}
	for name, state := range map[string]string{"shown": "completed", "quick": "completed", "label": "blocked"} {
		mu, err := executionMutant(q.result, "main.go", name)
		if err != nil {
			return err
		}
		if mu["state"] != state || (name == "shown" && mu["reused"] != true) || (name == "quick" && mu["outcome"] != "killed") || (name == "label" && mu["outcome"] != nil) {
			return fmt.Errorf("blocked count identity %s: %v", name, mu)
		}
	}
	o := q.observations["plain"]
	if o.Exit != 1 || !strings.Contains(o.Out, "blocked") || !strings.Contains(o.Out, "ran 1, reused 1") || !strings.Contains(o.Out, "2 completed") {
		return fmt.Errorf("blocked plain counts: %+v", o)
	}
	return nil
}
func (q *mutationExecution) countsAgree() error {
	file, err := executionFile(q.result, "main.go")
	if err != nil {
		return err
	}
	work := executionMap(file, "work")
	if work["completed"] != float64(2) || work["blocked"] != float64(1) || file["ran"].(float64)+file["reused"].(float64) != work["completed"] {
		return fmt.Errorf("blocked completed work accounting: %v", file)
	}
	return nil
}
func (q *mutationExecution) aggregateBlocked() error {
	o := q.observations["aggregate"]
	file, err := executionFile(o.Object, "main.go")
	if err != nil {
		return err
	}
	if o.Exit != 1 || file["state"] != nil || file["baseline"] != "failed" || file["ran"] != float64(0) || file["reused"] != float64(0) || len(objectList(file, "mutants")) != 0 {
		return fmt.Errorf("aggregate blocked baseline: %+v", o)
	}
	plain := q.observations["aggregatePlain"]
	if plain.Exit != 1 || !strings.Contains(plain.Out, "main.go: listed tests its mutants run fail without any mutant; snapshot not updated") {
		return fmt.Errorf("aggregate blocked plain output: %+v", plain)
	}
	return nil
}
