package features

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/cucumber/godog"
)

func (q *mutationExecution) planningSteps(sc *godog.ScenarioContext) {
	sc.Step(`^a selection with a stale exception, or with an uncovered mutant or a strict Go coverage finding under --fail-uncovered$`, q.stalePolicy)
	sc.Step(`^mutation run --fail-fast runs it$`, q.planningRun)
	sc.Step(`^it fails with the same rule the aggregate run reports$`, q.samePlanningRule)
	sc.Step(`^no mutant trial starts when planning already knew the failure$`, q.noPlanningTrials)
	sc.Step(`^an uncovered mutant without --fail-uncovered does not stop the run$`, q.uncoveredContinues)
	sc.Step(`^a selected file whose own baseline fails, or a survivor whose listed selection baseline fails without any mutant$`, q.baselineProject)
	sc.Step(`^mutation run --fail-fast reaches it$`, func() error {
		return q.run("mutation run --json --fail-fast --no-coverage --no-annotate --workers 1 --test-command '" + q.command + "' a/main.go b/main.go")
	})
	sc.Step(`^it fails with mutation\.baseline-failed or tests\.selection-failed$`, q.baselineProblem)
	sc.Step(`^the affected mutants have no invented killed, timed-out or survived outcome$`, q.baselineNoOutcomes)
	sc.Step(`^no stage reports a baseline as passed that never ran$`, q.baselineStages)
}
func (q *mutationExecution) stalePolicy() error {
	if err := q.three(); err != nil {
		return err
	}
	if err := q.run("mutation run --json --no-coverage --no-annotate --workers 1 --test-command '" + q.command + "'"); err != nil {
		return err
	}
	mu, err := executionMutant(q.result, "main.go", "Weak")
	if err != nil {
		return err
	}
	if q.m.w.exit != 1 || mu["outcome"] != "survived" {
		return fmt.Errorf("stale policy reviewed survivor: %v", q.result)
	}
	location := fmt.Sprintf("main.go:%.0f:%.0f", mu["line"], mu["column"])
	o, err := q.capture("mutation", "except", location, "--reason", "reviewed weak boundary", "--json")
	if err != nil {
		return err
	}
	if o.Exit != 0 {
		return fmt.Errorf("policy exception setup: %+v", o)
	}
	q.source["main.go"] = strings.Replace(q.source["main.go"], "n > 5", "n > 8", 1)
	if err = q.m.w.write("main.go", q.source["main.go"]); err != nil {
		return err
	}
	if err = q.m.w.d.Remove(filepath.Join(q.m.w.dir, ".metrics", "mutate", "main.go.json")); err != nil {
		return err
	}
	return q.declare("Weak")
}
func (q *mutationExecution) planningRun() error {
	if err := q.m.w.d.Write(filepath.Join(q.outside, "trace.jsonl"), nil); err != nil {
		return err
	}
	if err := q.run("mutation run --json --fail-fast --no-coverage --no-annotate --workers 1 --test-command '" + q.command + "'"); err != nil {
		return err
	}
	trace, err := q.m.w.d.Read(filepath.Join(q.outside, "trace.jsonl"))
	if err != nil {
		return err
	}
	q.save("planningTrace", executionObservation{Out: string(trace), Log: q.m.w.stderr})
	return nil
}
func (q *mutationExecution) samePlanningRule() error {
	if q.m.w.exit != 1 {
		return fmt.Errorf("planning stop exit%d", q.m.w.exit)
	}
	if err := executionStop(q.result, "mutation.exception-stale"); err != nil {
		return err
	}
	subject := executionMap(executionMap(q.result, "stop"), "subject")
	if subject["why"] != "changed" || !strictNamed(fmt.Sprint(subject["function"]), "Weak") {
		return fmt.Errorf("planning stale subject: %v", subject)
	}
	o, err := q.capture("mutation", "run", "--json", "--no-coverage", "--no-annotate", "--workers", "1", "--test-command", q.command)
	if err != nil {
		return err
	}
	found := false
	for _, p := range checkProblems(o.Object) {
		if p["rule"] == "mutation.exception-stale" && p["why"] == "changed" && strictNamed(fmt.Sprint(p["function"]), "Weak") {
			found = true
		}
	}
	if o.Exit != 1 || !found {
		return fmt.Errorf("aggregate planning rule differs: %+v", o)
	}
	return q.strictPlanningAlternatives()
}
func (q *mutationExecution) noPlanningTrials() error {
	o := q.observations["planningTrace"]
	if len(o.Out) != 0 {
		return fmt.Errorf("planning failure started fixture commands: %s", o.Out)
	}
	if err := strictNoCommands(o.Log); err != nil {
		return err
	}
	for _, name := range []string{"Weak", "Compare", "Other"} {
		mu, err := executionMutant(q.result, "main.go", name)
		if err != nil {
			return err
		}
		if mu["state"] != "unattempted" || mu["outcome"] != nil {
			return fmt.Errorf("known failure admitted mutant: %v", mu)
		}
	}
	return nil
}

const executionUncoveredSource = executionThreeSource + "\nfunc Dormant(n int) bool { return n > 9 }\n"
const executionUncoveredReport = executionThreeReport + "example.com/ownership/main.go:9.28,9.42 1 0\n"

func (q *mutationExecution) uncoveredContinues() error {
	if err := q.m.w.d.Remove(filepath.Join(q.m.w.dir, "itos-cc.yaml")); err != nil {
		return err
	}
	if err := q.project(map[string]string{"main.go": executionUncoveredSource}); err != nil {
		return err
	}
	fixture := strings.Replace(executionTests, "if !Weak(50)", "if Weak(5)||!Weak(50)", 1)
	if err := q.m.w.write("main_test.go", fixture); err != nil {
		return err
	}
	command := "go test -count=1 -failfast ./."
	if err := q.m.w.d.Tests(q.m.w.dir, Tests{Language: "go", Command: command, Elapsed: 100 * time.Millisecond, Trace: &TestTrace{File: filepath.Join(q.outside, "trace.jsonl"), Source: "main.go"}}); err != nil {
		return err
	}
	if err := q.m.w.d.Coverage(q.m.w.dir, Coverage{Language: "go", Report: executionUncoveredReport}); err != nil {
		return err
	}
	o, err := q.capture("mutation", "run", "--json", "--no-annotate", "--workers", "1", "--fail-fast")
	if err != nil {
		return err
	}
	if o.Exit != 0 || o.Object["ok"] != true || executionMap(o.Object, "stop")["stopped"] != false || executionMap(o.Object, "work")["completed"] != float64(4) {
		return fmt.Errorf("uncovered without strict policy stopped: %+v", o)
	}
	mu, err := executionMutant(o.Object, "main.go", "Dormant")
	if err != nil {
		return err
	}
	if mu["outcome"] != "uncovered" || mu["state"] != "completed" {
		return fmt.Errorf("uncovered completed judgment: %v", mu)
	}
	for _, name := range []string{"Weak", "Compare", "Other"} {
		mu, err := executionMutant(o.Object, "main.go", name)
		if err != nil {
			return err
		}
		if mu["outcome"] != "killed" || mu["state"] != "completed" {
			return fmt.Errorf("uncovered did not allow later judgment %s: %v", name, mu)
		}
	}
	return nil
}
func (q *mutationExecution) baselineProject() error {
	if err := q.three(); err != nil {
		return err
	}
	if err := q.m.w.d.Remove(filepath.Join(q.m.w.dir, "main.go")); err != nil {
		return err
	}
	files := map[string]string{
		"a/go.mod":  "module example.com/a\n\ngo 1.22\n",
		"a/main.go": "package main\n\nfunc A(n int) bool { return n > 5 }\n",
		"a/main_test.go": `package main
import "testing"
func TestA(t *testing.T){t.Fatal("owned baseline fails")}
`,
		"a/trace_test.go": executionPlanningTrace,
		"b/trace_test.go": executionPlanningTrace,
		"b/go.mod":        "module example.com/b\n\ngo 1.22\n",
		"b/main.go":       "package main\n\nfunc B(n int) bool { return n > 6 }\n",
		"b/main_test.go": `package main
import "testing"
func TestB(t *testing.T){if B(6){t.Fatal("B boundary")}}
`,
	}
	for name, text := range files {
		if err := q.m.w.write(name, text); err != nil {
			return err
		}
	}
	if err := q.m.w.d.Git(q.m.w.dir, "add", "a/go.mod", "a/main.go", "a/main_test.go", "a/trace_test.go", "b/trace_test.go", "b/go.mod", "b/main.go", "b/main_test.go", "main.go"); err != nil {
		return err
	}
	if err := q.m.w.d.Git(q.m.w.dir, "commit", "-qm", "separate baseline scopes"); err != nil {
		return err
	}
	if err := q.m.w.d.Tests(filepath.Join(q.m.w.dir, "a"), Tests{Language: "go", Command: q.command, Baseline: "owned baseline fails", Elapsed: 100 * time.Millisecond, Trace: &TestTrace{File: filepath.Join(q.outside, "trace.jsonl"), Source: "main.go"}}); err != nil {
		return err
	}
	return q.m.w.d.Tests(filepath.Join(q.m.w.dir, "b"), Tests{Language: "go", Command: q.command, Elapsed: 100 * time.Millisecond, Trace: &TestTrace{File: filepath.Join(q.outside, "trace.jsonl"), Source: "main.go"}})
}
func (q *mutationExecution) baselineProblem() error {
	if q.m.w.exit != 1 {
		return fmt.Errorf("baseline stop exit%d", q.m.w.exit)
	}
	if err := executionStop(q.result, "mutation.baseline-failed"); err != nil {
		return err
	}
	subject := executionMap(executionMap(q.result, "stop"), "subject")
	if subject["file"] != "a/main.go" {
		return fmt.Errorf("baseline stop subject: %v", subject)
	}
	return nil
}
func (q *mutationExecution) baselineNoOutcomes() error {
	for file, name := range map[string]string{"a/main.go": "A", "b/main.go": "B"} {
		mu, err := executionMutant(q.result, file, name)
		if err != nil {
			return err
		}
		if mu["outcome"] != nil {
			return fmt.Errorf("baseline invented outcome: %v", mu)
		}
	}
	return nil
}
func (q *mutationExecution) baselineStages() error {
	for file, want := range map[string]string{"a/main.go": "failed", "b/main.go": "not-run"} {
		f, err := executionFile(q.result, file)
		if err != nil {
			return err
		}
		if f["baseline"] != want {
			return fmt.Errorf("baseline stage %s: %v", file, f)
		}
	}
	trace, err := q.m.w.d.Read(filepath.Join(q.outside, "trace.jsonl"))
	if err != nil {
		return err
	}
	source, err := q.m.w.d.Read(filepath.Join(q.m.w.dir, "a", "main.go"))
	if err != nil {
		return err
	}
	// The failing baseline ran in a worker's copy, and once more in the
	// project's own folder (slice-19): each run recorded the source as it
	// is, unmutated.
	for _, line := range strings.Split(strings.TrimSpace(string(trace)), "\n") {
		var record map[string]string
		if err = json.Unmarshal([]byte(line), &record); err != nil {
			return fmt.Errorf("baseline command inventory: %w: %s", err, trace)
		}
		if record["source_sha256"] != fmt.Sprintf("%x", sha256.Sum256(source)) || record["cwd"] == "" || record["original_root"] == "" {
			return fmt.Errorf("baseline command inventory: %s", trace)
		}
	}
	return nil
}

func (q *mutationExecution) coldProject(source string) (*mutationExecution, error) {
	dir, err := q.m.w.d.Folder()
	if err != nil {
		return nil, err
	}
	q.extraDirs = append(q.extraDirs, dir)
	w := &world{d: q.m.w.d, dir: dir}
	child := &mutationExecution{m: &mutateState{w: w}, command: "go test -count=1 -failfast ./."}
	err = child.project(map[string]string{"main.go": source})
	if child.outside != "" {
		q.extraDirs = append(q.extraDirs, child.outside)
	}
	return child, err
}
func (q *mutationExecution) strictPlanningAlternatives() error {
	for _, zeroSite := range []bool{false, true} {
		source, fixture, report := executionUncoveredSource, strings.Replace(executionTests, "if !Weak(50)", "if Weak(5)||!Weak(50)", 1), executionUncoveredReport
		covered := []string{"Weak", "Compare", "Other"}
		if zeroSite {
			source, fixture, report = strictBaseGo, strictBaseTest, strictBaseReport
			covered = []string{"Compare"}
		}
		child, err := q.coldProject(source)
		if err != nil {
			return err
		}
		if err = child.m.w.write("main_test.go", fixture); err != nil {
			return err
		}
		if zeroSite {
			if err = child.m.w.write("trace_test.go", executionPlanningTrace); err != nil {
				return err
			}
		}
		if err = child.m.w.d.Tests(child.m.w.dir, Tests{Language: "go", Command: child.command, Elapsed: 100 * time.Millisecond, Trace: &TestTrace{File: filepath.Join(child.outside, "trace.jsonl"), Source: "main.go"}}); err != nil {
			return err
		}
		if err = child.m.w.d.Coverage(child.m.w.dir, Coverage{Language: "go", Report: report}); err != nil {
			return err
		}
		fast, err := child.capture("mutation", "run", "--json", "--no-annotate", "--workers", "1", "--fail-uncovered", "--fail-fast")
		if err != nil {
			return err
		}
		stop := executionMap(fast.Object, "stop")
		rule, _ := stop["rule"].(string)
		if fast.Exit != 1 || (rule != "mutation.uncovered" && rule != "mutation.uncovered-statement") || (zeroSite && rule != "mutation.uncovered-statement") {
			return fmt.Errorf("strict policy planning stop: %+v", fast)
		}
		if err = executionStop(fast.Object, rule); err != nil {
			return err
		}
		if strings.Contains(fast.Log, "itos-cc: baseline ") || strings.Contains(fast.Log, "itos-cc: [") {
			return fmt.Errorf("strict planning admitted baseline/trial: %s", fast.Log)
		}
		trace, err := child.m.w.d.Read(filepath.Join(child.outside, "trace.jsonl"))
		if err != nil {
			return err
		}
		rows := strings.Split(strings.TrimSpace(string(trace)), "\n")
		if len(trace) == 0 {
			return fmt.Errorf("strict planning coverage positive control absent")
		}
		hash := fmt.Sprintf("%x", sha256.Sum256([]byte(source)))
		for _, row := range rows {
			var r map[string]string
			if err = json.Unmarshal([]byte(row), &r); err != nil {
				return err
			}
			if r["source_sha256"] != hash || r["cwd"] == "" || r["original_root"] == "" {
				return fmt.Errorf("strict planning executed mutant source: %v", r)
			}
		}
		for _, name := range covered {
			mu, err := executionMutant(fast.Object, "main.go", name)
			if err != nil {
				return err
			}
			if mu["state"] != "unattempted" || mu["outcome"] != nil {
				return fmt.Errorf("strict known failure admitted %s: %v", name, mu)
			}
		}
		aggregate, err := child.capture("mutation", "run", "--json", "--no-annotate", "--workers", "1", "--fail-uncovered")
		if err != nil {
			return err
		}
		found := false
		for _, p := range checkProblems(aggregate.Object) {
			if p["rule"] == rule {
				found = true
			}
		}
		if aggregate.Exit != 1 || !found {
			return fmt.Errorf("strict aggregate planning rule differs: %+v", aggregate)
		}
	}
	return nil
}

const executionPlanningTrace = `package main
import("testing";"os";"path/filepath";"crypto/sha256";"encoding/json";"fmt")
func TestMain(m *testing.M){
 source,err:=os.ReadFile("main.go");if err!=nil{panic(err)}
 cwd,err:=os.Getwd();if err!=nil{panic(err)};cwd,err=filepath.EvalSymlinks(cwd);if err!=nil{panic(err)}
 original,err:=filepath.EvalSymlinks(os.Getenv("T80_ORIGINAL"));if err!=nil{panic(err)}
 f,err:=os.OpenFile(os.Getenv("T80_TRACE"),os.O_WRONLY|os.O_APPEND|os.O_CREATE,0600);if err!=nil{panic(err)}
 if err=json.NewEncoder(f).Encode(map[string]string{"cwd":cwd,"original_root":original,"source_sha256":fmt.Sprintf("%x",sha256.Sum256(source))});err!=nil{panic(err)};if err=f.Close();err!=nil{panic(err)}
 os.Exit(m.Run())
}
`
