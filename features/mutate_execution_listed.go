package features

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
)

func (q *mutationExecution) listedProject(selected ...string) error {
	if err := q.three(); err != nil {
		return err
	}
	w := q.m.w
	q.command = "go test -count=1 -failfast ./."
	q.source = map[string]string{"main.go": executionListedMain}
	for name, text := range executionListedFiles {
		if name == "itos-cc.yaml" {
			text = strings.Replace(text, `printf "ID-A-01\nID-A-02\n"`, "go run ./testdata/list.go", 1) + "    support: [\"support/*.txt\"]\n"
		}
		if err := w.write(name, text); err != nil {
			return err
		}
	}
	if err := w.write("testdata/list.go", executionList); err != nil {
		return err
	}
	if err := w.write("support/dep.txt", "original support\n"); err != nil {
		return err
	}
	w.variables = append(w.variables, "FF_DIR="+q.outside)
	if err := w.d.Git(w.dir, "add", "go.mod", "main.go", "main_test.go", "e2e/e2e_test.go", "testdata/list.go", "itos-cc.yaml", "support/dep.txt"); err != nil {
		return err
	}
	if err := w.d.Git(w.dir, "commit", "-qm", "listed fixture base"); err != nil {
		return err
	}
	if err := w.d.Git(w.dir, "tag", "base"); err != nil {
		return err
	}
	for _, name := range selected {
		head := "func " + name + "(n int) bool {\n"
		if !strings.Contains(q.source["main.go"], head) {
			return fmt.Errorf("listed fixture has no %s", name)
		}
		q.source["main.go"] = strings.Replace(q.source["main.go"], head, head+"\t// selected since base\n", 1)
	}
	if err := w.write("main.go", q.source["main.go"]); err != nil {
		return err
	}
	if err := w.d.Git(w.dir, "add", "main.go"); err != nil {
		return err
	}
	if err := w.d.Git(w.dir, "commit", "-qm", "selected functions"); err != nil {
		return err
	}
	q.args = "--since base"
	if err := q.listedDeclarations(nil); err != nil {
		return err
	}
	q.listedSelected = append([]string(nil), selected...)
	return q.measureListedCoverage()
}
func (q *mutationExecution) measureListedCoverage() error {
	// This real public setup captures the compiler's actual Go block inventory.
	// The final declaration below reflects these measurements, not guessed LCOV.
	w := q.m.w
	key := slices.Clone(q.listedSelected)
	slices.Sort(key)
	expected, ok := executionListedReports[strings.Join(key, ",")]
	if !ok {
		return fmt.Errorf("undeclared compiler inventory for %v", key)
	}
	if err := w.d.Coverage(w.dir, Coverage{Language: "go", Report: expected}); err != nil {
		return err
	}
	code, _, log, err := w.capture(w.dir, w.env(), "crap", "--json", "--coverage-command", "go test -count=1 -coverprofile=setup.coverage .", "--coverage-report", "setup.coverage", "main.go")
	if err != nil {
		return err
	}
	if code != 0 && code != 1 {
		return fmt.Errorf("listed coverage setup exit%d: %s", code, log)
	}
	report, err := w.d.Read(filepath.Join(w.dir, "setup.coverage"))
	if err != nil {
		return err
	}
	if err = w.d.Write(filepath.Join(q.outside, "go-profile.txt"), report); err != nil {
		return err
	}
	if string(report) != expected {
		return fmt.Errorf("actual compiler inventory differs for %v:\n%s", key, report)
	}
	return nil
}
func (q *mutationExecution) listedDeclarations(failing []string) error {
	w := q.m.w
	source := q.source["main.go"]
	sites := map[string]Site{}
	lines := strings.Split(source, "\n")
	for _, name := range []string{"shown", "idle", "quick", "wait", "label", "spare"} {
		head := "func " + name + "(n int) bool {"
		for i, line := range lines {
			if line == head {
				for j := i + 1; j < len(lines); j++ {
					if strings.Contains(lines[j], "return n >") {
						sites[name] = Site{File: "main.go", Line: j + 1, Original: ">", Replacement: ">="}
						break
					}
				}
			}
		}
		if sites[name].Line == 0 {
			return fmt.Errorf("missing listed site %s", name)
		}
	}
	t := Tests{Language: "go", Command: q.command, Elapsed: 100 * time.Millisecond, Surviving: []Site{sites["shown"], sites["label"]}, TimingOut: []Site{sites["wait"]}}
	if err := w.d.Tests(w.dir, t); err != nil {
		return err
	}
	reached := map[int][]string{}
	for _, name := range []string{"shown", "label"} {
		reached[sites[name].Line] = []string{"ID-A-01", "ID-A-02"}
	}
	listed := &ListedTests{Files: map[string]string{"ID-A-01": "e2e/e2e_test.go", "ID-A-02": "e2e/e2e_test.go"}, Lines: map[string]map[int][]string{"main.go": reached}, Split: true, Failing: failing, Surviving: map[string][]Site{"ID-A-01": {sites["label"]}, "ID-A-02": {sites["shown"], sites["label"]}}}
	return w.d.Tests(w.dir, Tests{Language: "go", Command: "go test -count=1 ./e2e -args -tests={pattern}", Elapsed: 100 * time.Millisecond, Listed: listed})
}
func (q *mutationExecution) failListed(wait bool) error {
	q.m.w.variables = append(q.m.w.variables, "FF_FAIL_ALONE=1")
	if wait {
		q.m.w.variables = append(q.m.w.variables, "FF_FAIL_AFTER="+filepath.Join(q.outside, "started"))
	}
	return q.listedDeclarations([]string{"ID-A-01", "ID-A-02"})
}
func (q *mutationExecution) listedRun(failfast bool, workers int) error {
	args := []string{"mutation", "run", "--json", "--workers", strconv.Itoa(workers), "--timeout-factor", "10000", "--since", "base"}
	if failfast {
		args = append(args, "--fail-fast")
	}
	return q.run(strings.Join(quoteWords(args), " "))
}
func executionStop(a map[string]any, rule string) error {
	stop := executionMap(a, "stop")
	if a["ok"] != false || stop["stopped"] != true || stop["rule"] != rule {
		return fmt.Errorf("expected stop %s: %v", rule, a)
	}
	found := false
	for _, p := range checkProblems(a) {
		if p["rule"] == rule {
			found = true
		}
	}
	if !found {
		return fmt.Errorf("stop has no matching problem %s: %v", rule, a)
	}
	return nil
}
func (q *mutationExecution) listedBlocked() error {
	if q.id == "250" {
		return q.keptStop()
	}
	if q.m.w.exit != 1 {
		return fmt.Errorf("blocked selection exit%d", q.m.w.exit)
	}
	if err := executionStop(q.result, "tests.selection-failed"); err != nil {
		return err
	}
	subject := executionMap(executionMap(q.result, "stop"), "subject")
	if fmt.Sprint(subject["ids"]) != "[ID-A-01 ID-A-02]" {
		return fmt.Errorf("blocked selection subject: %v", subject)
	}
	return nil
}
func (q *mutationExecution) disjointWork(want map[string]int) error {
	work := executionMap(q.result, "work")
	file, err := executionFile(q.result, "main.go")
	if err != nil {
		return err
	}
	total := 0
	for state, n := range want {
		if work[state] != float64(n) {
			return fmt.Errorf("work partition %s: %v want%v", state, work, want)
		}
		total += n
	}
	mutants := objectList(file, "mutants")
	if len(mutants) != total || file["state"] != "blocked" || file["baseline"] != "passed" {
		return fmt.Errorf("blocked file population: %v", file)
	}
	states := map[string]string{"idle": "completed", "quick": "completed", "wait": "cancelled", "label": "blocked", "spare": "unattempted"}
	for name, state := range states {
		mu, err := executionMutant(q.result, "main.go", name)
		if err != nil {
			return err
		}
		if mu["state"] != state {
			return fmt.Errorf("work state %s: %v", name, mu)
		}
		if state != "completed" && mu["outcome"] != nil {
			return fmt.Errorf("unfinished work invented outcome: %v", mu)
		}
	}
	for name, want := range map[string]string{"idle": "uncovered", "quick": "killed"} {
		mu, err := executionMutant(q.result, "main.go", name)
		if err != nil {
			return err
		}
		if mu["outcome"] != want {
			return fmt.Errorf("completed %s: %v", name, mu)
		}
	}
	return nil
}

const executionList = `package main
import("fmt";"os")
func main(){f,err:=os.OpenFile(os.Getenv("FF_DIR")+"/list.log",os.O_WRONLY|os.O_CREATE|os.O_APPEND,0600);if err!=nil{panic(err)};if _,err=f.WriteString("list\n");err!=nil{panic(err)};if err=f.Close();err!=nil{panic(err)};fmt.Println("ID-A-01\te2e/e2e_test.go\nID-A-02\te2e/e2e_test.go")}
`

func (q *mutationExecution) snapshotBytes(file string) (string, error) {
	b, err := q.m.w.d.Read(filepath.Join(q.m.w.dir, ".metrics", "mutate", file+".json"))
	return string(b), err
}
func (q *mutationExecution) sourceUnchanged(file string) error {
	b, err := q.m.w.d.Read(filepath.Join(q.m.w.dir, file))
	if err != nil {
		return err
	}
	if string(b) != q.before[file] {
		return fmt.Errorf("stopped annotated source changed: %s", file)
	}
	return nil
}
func executionJSONEqual(a, b any) bool {
	left, _ := json.Marshal(a)
	right, _ := json.Marshal(b)
	return slices.Equal(left, right)
}

// keptStop is whether a fail-fast run whose other judgment's cleanup failed
// still reports its stop at the survivor that triggered it, Gate.
func (q *mutationExecution) keptStop() error {
	if q.result == nil {
		return fmt.Errorf("the run printed no JSON object\n%s", q.m.w.report())
	}
	if err := executionStop(q.result, "mutation.survived"); err != nil {
		return err
	}
	trigger, err := executionMutant(q.result, "main.go", "Gate")
	if err != nil {
		return err
	}
	subject := executionMap(executionMap(q.result, "stop"), "subject")
	if subject["file"] != "main.go" || subject["function"] != trigger["function"] {
		return fmt.Errorf("the stop's subject: %v", subject)
	}
	return nil
}
