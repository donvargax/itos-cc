package features

import (
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/cucumber/godog"
)

func (q *mutationExecution) partialSteps(sc *godog.ScenarioContext) {
	sc.Step(`^a fail-fast run that completes one file and stops inside another$`, func() error {
		if err := q.partialProject(); err != nil {
			return err
		}
		return q.marker("simple")
	})
	sc.Step(`^it returns$`, q.partialReturns)
	sc.Step(`^the completed file's snapshot and annotation are written as without --fail-fast$`, q.completedFileWritten)
	sc.Step(`^the stopped file's annotated source bytes are unchanged, and its snapshot records no outcome for a site the stop left undecided$`, q.stoppedFileUnchanged)
	sc.Step(`^mutation check still reports the stopped file's missing or stale results without running tests$`, q.partialCheck)
	sc.Step(`^a later run reuses only outcomes whose inputs are still fresh$`, q.laterFreshOnly)
	sc.Step(`^a fail-fast run judges some mutants of a file and the stop cancels or leaves the rest$`, q.priorPartial)
	sc.Step(`^the file's snapshot keeps each completed or reused judgment with its original scope and freshness evidence$`, q.partialProof)
	sc.Step(`^cancelled, blocked and unattempted sites get no outcome and no new freshness proof$`, q.unfinishedProof)
	sc.Step(`^mutation check still fails the functions with missing valid site entries$`, q.partialCheck)
	sc.Step(`^the file's annotated source bytes are unchanged$`, func() error { return q.sourceUnchanged("a.go") })
	sc.Step(`^a file with fresh complete mutation results$`, q.completePrior)
	sc.Step(`^a fail-fast --mutate-all rerun of it is cancelled by a stop elsewhere$`, q.cancelForced)
	sc.Step(`^its prior fresh results still satisfy mutation check$`, q.priorCacheUsable)
	sc.Step(`^the run's report says its own work was incomplete, separately from the cache's completeness$`, q.incompleteWorkCompleteCache)
	sc.Step(`^a stopped fail-fast run preserved some judgments of a file$`, q.stoppedPrior)
	sc.Step(`^the same run is repeated with unchanged inputs$`, q.repeatPartial)
	sc.Step(`^the preserved judgments are reused and only the unjudged sites run$`, q.onlyUnjudgedRuns)
	sc.Step(`^a preserved judgment whose source, tests or support inputs changed runs again$`, q.changedPartialInputs)
}
func (q *mutationExecution) partialProject() error {
	if err := q.three(); err != nil {
		return err
	}
	q.source = map[string]string{"a.go": executionPartialA, "b.go": executionPartialB}
	if err := q.m.w.d.Remove(filepath.Join(q.m.w.dir, "main.go")); err != nil {
		return err
	}
	for name, text := range q.source {
		if err := q.m.w.write(name, text); err != nil {
			return err
		}
	}
	if err := q.m.w.write("main_test.go", strings.ReplaceAll(executionPartialTests, "RECORD_PATH", strconv.Quote(q.outside))); err != nil {
		return err
	}
	if err := q.m.w.write("testdata/list.go", `package main
import("os")
func main(){f,err:=os.OpenFile(os.Getenv("T80_LIST"),os.O_CREATE|os.O_APPEND|os.O_WRONLY,0600);if err!=nil{panic(err)};if _,err=f.WriteString("list\n");err!=nil{panic(err)};if err=f.Close();err!=nil{panic(err)}}
`); err != nil {
		return err
	}
	if err := q.m.w.write("support/dep.txt", "original support\n"); err != nil {
		return err
	}
	if err := q.m.w.write("itos-cc.yaml", `mutation:
  tests:
    list: go run ./testdata/list.go
    run: go test -count=1 -run '{pattern}' .
    ids_pattern: "^({ids})$"
    join:
      each: "{id}"
      sep: "|"
    support: ["support/*.txt"]
`); err != nil {
		return err
	}
	q.m.w.variables = append(q.m.w.variables, "T80_LIST="+filepath.Join(q.outside, "list.log"))
	if err := q.m.w.d.Write(filepath.Join(q.outside, "list.log"), nil); err != nil {
		return err
	}
	if err := q.m.w.d.Git(q.m.w.dir, "add", "a.go", "b.go", "main.go", "main_test.go", "testdata/list.go", "support/dep.txt", "itos-cc.yaml"); err != nil {
		return err
	}
	if err := q.m.w.d.Git(q.m.w.dir, "commit", "-qm", "partial cache fixture"); err != nil {
		return err
	}
	if err := q.m.w.d.Git(q.m.w.dir, "tag", "base"); err != nil {
		return err
	}
	q.before = map[string]string{"a.go": q.source["a.go"], "b.go": q.source["b.go"]}
	return q.partialDeclaration(false, false)
}
func (q *mutationExecution) marker(name string) error {
	return q.m.w.d.Write(filepath.Join(q.outside, name), []byte(name+"\n"))
}
func (q *mutationExecution) partialDeclaration(hold, simple bool) error {
	t := Tests{Language: "go", Command: q.command, Elapsed: 100 * time.Millisecond, Trace: &TestTrace{File: filepath.Join(q.outside, "trace.jsonl"), Source: "a.go"}, Listed: &ListedTests{Files: map[string]string{}, Lines: map[string]map[int][]string{}}}
	if hold {
		t.TimingOut = []Site{{File: "a.go", Line: 7, Original: ">", Replacement: ">="}}
	}
	if hold || simple {
		t.Surviving = []Site{{File: "b.go", Line: 3, Original: ">", Replacement: ">="}}
	}
	return q.m.w.d.Tests(q.m.w.dir, t)
}
func (q *mutationExecution) partialRun(workers int, force, annotate bool) error {
	args := []string{"mutation", "run", "--json", "--no-coverage", "--workers", strconv.Itoa(workers), "--timeout-factor", "10000", "--test-command", q.command, "--fail-fast"}
	if force {
		args = append(args, "--mutate-all")
	}
	if !annotate {
		args = append(args, "--no-annotate")
	}
	return q.run(strings.Join(quoteWords(args), " "))
}
func (q *mutationExecution) partialReturns() error {
	if q.id == "198" {
		if err := q.partialDeclaration(false, true); err != nil {
			return err
		}
		return q.partialRun(1, false, true)
	}
	return q.partialRun(2, false, true)
}
func (q *mutationExecution) recordFile(file, name string) (map[string]any, []map[string]any, error) {
	a, err := q.s.snapshot(file)
	if err != nil {
		return nil, nil, err
	}
	var found []map[string]any
	for _, u := range objectList(a, "units") {
		if strictNamed(fmt.Sprint(u["name"]), name) {
			found = append(found, u)
		}
	}
	if len(found) != 1 {
		return nil, nil, fmt.Errorf("snapshot %s#%s expected one unit: %v", file, name, found)
	}
	return found[0], objectList(found[0], "mutants"), nil
}
func (q *mutationExecution) completedFileWritten() error {
	if q.m.w.exit != 1 {
		return fmt.Errorf("stopped run exit%d", q.m.w.exit)
	}
	if err := executionStop(q.result, "mutation.survived"); err != nil {
		return err
	}
	f, err := executionFile(q.result, "a.go")
	if err != nil {
		return err
	}
	if f["state"] != "completed" {
		return fmt.Errorf("completed file: %v", f)
	}
	source, err := q.m.w.d.Read(filepath.Join(q.m.w.dir, "a.go"))
	if err != nil {
		return err
	}
	snap, err := q.snapshotBytes("a.go")
	if err != nil {
		return err
	}
	if len(snap) == 0 || string(source) == q.before["a.go"] || !strings.Contains(string(source), "itos-cc mutation:") {
		return fmt.Errorf("completed snapshot/annotation not published")
	}
	o, err := q.capture("mutation", "run", "--json", "--no-coverage", "--workers", "1", "--mutate-all", "--test-command", q.command, "a.go")
	if err != nil {
		return err
	}
	again, err := q.snapshotBytes("a.go")
	if err != nil {
		return err
	}
	after, err := q.m.w.d.Read(filepath.Join(q.m.w.dir, "a.go"))
	if err != nil {
		return err
	}
	if o.Exit != 0 || snap != again || string(source) != string(after) {
		return fmt.Errorf("completed file differs from aggregate publication: %+v", o)
	}
	return nil
}
func (q *mutationExecution) stoppedFileUnchanged() error {
	f, err := executionFile(q.result, "b.go")
	if err != nil {
		return err
	}
	if f["state"] != "stopped" {
		return fmt.Errorf("stopped file state: %v", f)
	}
	_, entries, err := q.recordFile("b.go", "Later")
	if err != nil {
		return err
	}
	if len(entries) != 0 {
		return fmt.Errorf("unattempted site published proof: %v", entries)
	}
	mu, err := executionMutant(q.result, "b.go", "Later")
	if err != nil {
		return err
	}
	if mu["state"] != "unattempted" || mu["outcome"] != nil {
		return fmt.Errorf("unattempted Later: %v", mu)
	}
	return q.sourceUnchanged("b.go")
}
func (q *mutationExecution) partialCheck() error {
	file, name := "a.go", "A2"
	if q.id == "198" {
		file, name = "b.go", "Later"
	}
	mark, err := q.m.w.d.ObserveWrites(filepath.Join(q.outside, "trace.jsonl"))
	if err != nil {
		return err
	}
	o, err := q.capture("mutation", "check", "--json", file)
	if err != nil {
		return err
	}
	if o.Exit != 1 {
		return fmt.Errorf("partial check exit%d: %v", o.Exit, o.Object)
	}
	found := false
	for _, p := range checkProblems(o.Object) {
		if p["file"] == file && strictNamed(fmt.Sprint(p["function"]), name) && (p["rule"] == "mutation.missing" || p["rule"] == "mutation.stale") {
			found = true
		}
	}
	if !found {
		return fmt.Errorf("missing unfinished function check: %v", o.Object)
	}
	if q.id == "200" {
		for _, p := range checkProblems(o.Object) {
			for _, done := range []string{"A1", "Done"} {
				if strictNamed(fmt.Sprint(p["function"]), done) {
					return fmt.Errorf("completed proof rejected by public check: %v", p)
				}
			}
		}
	}

	if err = strictNoCommands(o.Log); err != nil {
		return err
	}
	writes, err := mark()
	if err != nil {
		return err
	}
	if len(writes) != 0 {
		return fmt.Errorf("partial check executed tests: %v", writes)
	}
	return nil
}
func (q *mutationExecution) laterFreshOnly() error {
	o, err := q.capture("mutation", "run", "--json", "--no-coverage", "--no-annotate", "--workers", "1", "--test-command", q.command)
	if err != nil {
		return err
	}
	a, err := executionFile(o.Object, "a.go")
	if err != nil {
		return err
	}
	b, err := executionFile(o.Object, "b.go")
	if err != nil {
		return err
	}
	if o.Exit != 1 || a["ran"] != float64(0) || a["reused"] != float64(3) || b["ran"] != float64(2) || b["reused"] != float64(0) {
		return fmt.Errorf("later fresh-only reuse: %+v", o)
	}
	return nil
}
func (q *mutationExecution) priorPartial() error {
	if err := q.partialProject(); err != nil {
		return err
	}
	q.source["a.go"] = strings.Replace(q.source["a.go"], "func A1(n int) bool {", "func A1(n int) bool { /* initially selected */", 1)
	if err := q.m.w.write("a.go", q.source["a.go"]); err != nil {
		return err
	}
	if err := q.m.w.d.Git(q.m.w.dir, "add", "a.go"); err != nil {
		return err
	}
	if err := q.m.w.d.Git(q.m.w.dir, "commit", "-qm", "record A1"); err != nil {
		return err
	}
	o, err := q.capture("mutation", "run", "--json", "--no-coverage", "--no-annotate", "--since", "base", "--test-command", q.command)
	if err != nil {
		return err
	}
	unit, entries, err := q.recordFile("a.go", "A1")
	if err != nil {
		return err
	}
	if o.Exit != 0 || len(entries) != 1 || entries[0]["outcome"] != "killed" || entries[0]["scope"] != q.command {
		return fmt.Errorf("prior A1 proof: %+v %v", o, entries)
	}
	q.save("priorA1", executionObservation{Object: entries[0]})
	q.save("priorA1Unit", executionObservation{Object: map[string]any{"hash": unit["hash"]}})
	q.before["a.go"] = q.source["a.go"]
	if err = q.marker("hold"); err != nil {
		return err
	}
	return q.partialDeclaration(true, false)
}
func (q *mutationExecution) partialProof() error {
	if q.m.w.exit != 1 {
		return fmt.Errorf("partial exit%d", q.m.w.exit)
	}
	if err := executionStop(q.result, "mutation.survived"); err != nil {
		return err
	}
	for _, name := range []string{"A1", "Done"} {
		unit, entries, err := q.recordFile("a.go", name)
		if err != nil {
			return err
		}
		if len(entries) != 1 || entries[0]["outcome"] != "killed" || entries[0]["scope"] != q.command {
			return fmt.Errorf("partial retained %s: %v", name, entries)
		}
		e := executionMap(entries[0], "suite_evidence")
		if len(executionMap(e, "tests")) == 0 || len(executionMap(e, "support")) == 0 {
			return fmt.Errorf("partial nonempty freshness %s: %v", name, entries)
		}
		hash, ok := unit["hash"].(string)
		if !ok || hash == "" || (name == "A1" && hash != q.observations["priorA1Unit"].Object["hash"]) {
			return fmt.Errorf("partial source hash %s: %v", name, unit)
		}
		for kind, file := range map[string]string{"tests": "main_test.go", "support": "support/dep.txt"} {
			bytes, err := q.m.w.d.Read(filepath.Join(q.m.w.dir, file))
			if err != nil {
				return err
			}
			expected := fmt.Sprintf("%x", sha256.Sum256(bytes))
			if executionMap(e, kind)[file] != expected {
				return fmt.Errorf("partial %s evidence %s differs from actual input: %v", name, kind, e)
			}
		}
		if name == "A1" && !executionJSONEqual(entries[0], q.observations["priorA1"].Object) {
			return fmt.Errorf("reused A1 proof changed: %v", entries)
		}
	}
	return nil
}
func (q *mutationExecution) unfinishedProof() error {
	for _, name := range []string{"A2"} {
		_, entries, err := q.recordFile("a.go", name)
		if err != nil {
			return err
		}
		if len(entries) != 0 {
			return fmt.Errorf("unfinished site got freshness proof: %v", entries)
		}
		mu, err := executionMutant(q.result, "a.go", name)
		if err != nil {
			return err
		}
		if mu["state"] != "cancelled" || mu["outcome"] != nil {
			return fmt.Errorf("unfinished reported outcome: %v", mu)
		}
	}
	return nil
}
func (q *mutationExecution) completePrior() error {
	if err := q.partialProject(); err != nil {
		return err
	}
	o, err := q.capture("mutation", "run", "--json", "--no-coverage", "--no-annotate", "--workers", "1", "--test-command", q.command)
	if err != nil {
		return err
	}
	if o.Exit != 0 {
		return fmt.Errorf("fresh prior run: %+v", o)
	}
	return q.priorCacheUsable()
}
func (q *mutationExecution) cancelForced() error {
	if err := q.marker("hold"); err != nil {
		return err
	}
	if err := q.partialDeclaration(true, false); err != nil {
		return err
	}
	return q.partialRun(2, true, false)
}
func (q *mutationExecution) priorCacheUsable() error {
	o, err := q.capture("mutation", "check", "--json", "a.go")
	if err != nil {
		return err
	}
	if o.Exit != 0 || o.Object["ok"] != true || len(checkProblems(o.Object)) != 0 {
		return fmt.Errorf("prior fresh cache rejected: %+v", o)
	}
	return nil
}
func (q *mutationExecution) incompleteWorkCompleteCache() error {
	if q.m.w.exit != 1 {
		return fmt.Errorf("forced partial exit%d", q.m.w.exit)
	}
	if err := executionStop(q.result, "mutation.survived"); err != nil {
		return err
	}
	a, err := executionFile(q.result, "a.go")
	if err != nil {
		return err
	}
	mu, err := executionMutant(q.result, "a.go", "A2")
	if err != nil {
		return err
	}
	if a["state"] != "stopped" || a["cache"] != "complete" || executionMap(a, "work")["cancelled"] != float64(1) || mu["state"] != "cancelled" || mu["outcome"] != nil {
		return fmt.Errorf("incomplete work/complete cache distinction: %v", a)
	}
	return nil
}
func (q *mutationExecution) stoppedPrior() error {
	if err := q.partialProject(); err != nil {
		return err
	}
	if err := q.marker("hold"); err != nil {
		return err
	}
	if err := q.partialDeclaration(true, false); err != nil {
		return err
	}
	if err := q.partialRun(2, false, false); err != nil {
		return err
	}
	if err := q.unfinishedProof(); err != nil {
		return err
	}
	for _, name := range []string{"A1", "Done"} {
		_, entries, err := q.recordFile("a.go", name)
		if err != nil {
			return err
		}
		if len(entries) != 1 || entries[0]["outcome"] != "killed" {
			return fmt.Errorf("prior stopped completed %s: %v", name, entries)
		}
	}
	return nil
}
func (q *mutationExecution) repeatPartial() error {
	if err := q.m.w.d.Remove(filepath.Join(q.outside, "hold")); err != nil {
		return err
	}
	if err := q.m.w.d.Remove(filepath.Join(q.outside, "started")); err != nil {
		return err
	}
	if err := q.partialDeclaration(false, false); err != nil {
		return err
	}
	return q.partialRun(2, false, false)
}
func (q *mutationExecution) onlyUnjudgedRuns() error {
	if q.m.w.exit != 0 {
		return fmt.Errorf("continued run exit%d", q.m.w.exit)
	}
	a, err := executionFile(q.result, "a.go")
	if err != nil {
		return err
	}
	if a["ran"] != float64(1) || a["reused"] != float64(2) {
		return fmt.Errorf("continued counts: %v", a)
	}
	for _, name := range []string{"A1", "Done", "A2"} {
		mu, err := executionMutant(q.result, "a.go", name)
		if err != nil {
			return err
		}
		if mu["outcome"] != "killed" || mu["reused"] != (name != "A2") {
			return fmt.Errorf("continued identity %s: %v", name, mu)
		}
	}
	return nil
}
func (q *mutationExecution) changedPartialInputs() error {
	for _, kind := range []string{"source", "tests", "support"} {
		switch kind {
		case "source":
			q.source["a.go"] = strings.Replace(q.source["a.go"], "func A1(n int) bool {", "func A1(n int) bool { /* changed input */", 1)
			if err := q.m.w.write("a.go", q.source["a.go"]); err != nil {
				return err
			}
		case "tests":
			b, err := q.m.w.d.Read(filepath.Join(q.m.w.dir, "main_test.go"))
			if err != nil {
				return err
			}
			if err = q.m.w.write("main_test.go", string(b)+"\n// changed test input\n"); err != nil {
				return err
			}
		case "support":
			if err := q.m.w.write("support/dep.txt", "changed support\n"); err != nil {
				return err
			}
		}
		o, err := q.capture("mutation", "run", "--json", "--no-coverage", "--no-annotate", "--workers", "1", "--test-command", q.command, "--fail-fast")
		if err != nil {
			return err
		}
		if o.Exit != 0 {
			return fmt.Errorf("changed %s input: %+v", kind, o)
		}
		for _, name := range []string{"A1", "Done", "A2"} {
			mu, err := executionMutant(o.Object, "a.go", name)
			if err != nil {
				return err
			}
			wantReuse := kind == "source" && name != "A1"
			if mu["outcome"] != "killed" || mu["reused"] != wantReuse {
				return fmt.Errorf("changed %s freshness %s: %v", kind, name, mu)
			}
		}
	}
	return nil
}
