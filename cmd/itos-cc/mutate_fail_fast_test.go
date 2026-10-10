//go:build !windows

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/donvargax/itos-cc/mutate"
)

// The scenarios of "Rule: mutation run --fail-fast stops at the first
// actionable failure" in features/mutate.feature, the fail-fast-run slice.
// --fail-fast runs on Linux and macOS only, so these build only where Unix
// process groups exist; ID-MUT-199, the admission boundaries, builds
// everywhere. Each drives the CLI and reads its --json object as raw maps,
// so the steps compile against a product with no --fail-fast yet.
//
// Most fixtures judge their mutants with a shell script, test.sh, given as
// --test-command: it runs at the module root of a worker copy, appends a
// line to $FF_DIR/runs on every run, and decides each mutant by grepping
// the source for its original text, so a trial takes milliseconds and its
// timing is under the fixture's control through marker files in $FF_DIR.
// The listed-test cases use ffListedFiles, a small module whose e2e
// harness runs its built binary by listed test ID.

// ffJSON is the --json object of a mutation run, fail-fast keys included.
type ffJSON struct {
	OK       bool             `json:"ok"`
	Stop     map[string]any   `json:"stop"`
	Work     map[string]any   `json:"work"`
	Files    []map[string]any `json:"files"`
	Problems []map[string]any `json:"problems"`
}

func (o outcome) ff(t *testing.T) ffJSON {
	t.Helper()
	var out ffJSON
	if err := json.Unmarshal([]byte(o.stdout), &out); err != nil {
		t.Fatalf("stdout is not one JSON object: %v\n%s\nstderr:\n%s", err, o.stdout, o.stderr)
	}
	return out
}

func (f ffJSON) problem(rule string) map[string]any {
	for _, p := range f.Problems {
		if p["rule"] == rule {
			return p
		}
	}
	return nil
}

func (f ffJSON) rules() []string {
	var out []string
	for _, p := range f.Problems {
		r, _ := p["rule"].(string)
		out = append(out, r)
	}
	return out
}

// file is the entry of file in "files", or nil.
func (f ffJSON) file(name string) map[string]any {
	for _, file := range f.Files {
		if file["file"] == name {
			return file
		}
	}
	return nil
}

// work is the count of work in state, from the run's "work".
func (f ffJSON) work(state string) int {
	n, _ := f.Work[state].(float64)
	return int(n)
}

// stopped says whether the run reports that it stopped early.
func (f ffJSON) stopped() bool {
	return f.Stop["stopped"] == true
}

// stopRule is the rule of the run's stop, or "".
func (f ffJSON) stopRule() string {
	r, _ := f.Stop["rule"].(string)
	return r
}

// stopSubject is the subject of the run's stop, or nil.
func (f ffJSON) stopSubject() map[string]any {
	s, _ := f.Stop["subject"].(map[string]any)
	return s
}

// mutantsOf is the "mutants" of a file entry, as plain maps.
func mutantsOf(file map[string]any) []map[string]any {
	var out []map[string]any
	list, _ := file["mutants"].([]any)
	for _, m := range list {
		out = append(out, m.(map[string]any))
	}
	return out
}

// ffMutant is the mutant of file in function, by the name after its "#".
func ffMutant(file map[string]any, function string) map[string]any {
	for _, m := range mutantsOf(file) {
		if f, _ := m["function"].(string); strings.HasSuffix(f, "#"+function) {
			return m
		}
	}
	return nil
}

// undecided says whether m is reported in state with no mutation outcome.
func undecided(m map[string]any, state string) bool {
	_, has := m["outcome"]
	return m != nil && m["state"] == state && !has
}

// ffScript is test.sh: each line of body after the line recording the run.
func ffScript(body ...string) string {
	return "echo run >> \"$FF_DIR/runs\"\n" + strings.Join(body, "\n") + "\n"
}

// ffRepo writes files in a Git repository, makes it the test's directory
// (useDir) and points FF_DIR at a fresh directory for test.sh's markers. It returns
// the repository and that directory.
func ffRepo(t *testing.T, files map[string]string) (string, string) {
	t.Helper()
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh is not installed")
	}
	dir := moduleRepo(t, files)
	ff := t.TempDir()
	useEnv(t, "FF_DIR", ff)
	return dir, ff
}

// shellRun runs mutation run --json with test.sh as the test command, no
// coverage, and args.
func shellRun(t *testing.T, args ...string) outcome {
	t.Helper()
	return cli(t, append([]string{"mutation", "run", "--json", "--no-coverage", "--test-command", "sh test.sh"}, args...)...)
}

// runsIn is how many times test.sh ran, as $FF_DIR/runs records.
func runsIn(ff string) int {
	return lineCount(filepath.Join(ff, "runs"))
}

func logOutcome(t *testing.T, o *outcome) {
	t.Helper()
	t.Cleanup(func() {
		if t.Failed() {
			t.Logf("exit %d\nstdout:\n%s\nstderr:\n%s", o.code, o.stdout, o.stderr)
		}
	})
}

// threeFiles is a module of three one-site functions: Weak's mutant
// survives test.sh, which kills Compare's and Other's.
var threeFiles = map[string]string{
	"go.mod":  "module example.com/ff\n\ngo 1.22\n",
	"main.go": "package main\n\n// Weak's mutant survives: the tests never check it.\nfunc Weak(n int) bool { return n > 4 }\n\nfunc Compare(n int) bool { return n > 5 }\n\nfunc Other(n int) bool { return n > 6 }\n\nfunc main() {}\n",
	"test.sh": ffScript("grep -q 'n > 5' main.go || exit 1", "grep -q 'n > 6' main.go || exit 1", "exit 0"),
}

// @ID-MUT-192
func TestTheFirstActionableSurvivorStopsAdmissionOfLaterMutants(t *testing.T) {
	_, ff := ffRepo(t, threeFiles)

	// With --fail-fast, one worker judges Weak's mutant first.
	o := shellRun(t, "--no-annotate", "--workers", "1", "--fail-fast")
	logOutcome(t, &o)
	f := o.ff(t)
	p := f.problem("mutation.survived")
	if o.code != 1 || p == nil || !strings.HasSuffix(fmt.Sprint(p["function"]), "#Weak") {
		t.Errorf("exit %d, problems %v: want 1 with mutation.survived for Weak's mutant", o.code, f.Problems)
	}
	if !f.stopped() || f.stopRule() != "mutation.survived" || !strings.HasSuffix(fmt.Sprint(f.stopSubject()["function"]), "#Weak") ||
		f.stopSubject()["line"] != float64(4) {
		t.Errorf("stop = %v: want the run stopped by mutation.survived for Weak's mutant at line 4", f.Stop)
	}
	// No later mutant starts: the baseline and Weak's mutant are the only
	// runs, and the others are unattempted, with no outcome.
	if n := runsIn(ff); n != 2 {
		t.Errorf("test.sh ran %d times, want twice: the baseline and Weak's mutant", n)
	}
	m := f.file("main.go")
	for _, function := range []string{"Compare", "Other"} {
		if got := ffMutant(m, function); !undecided(got, "unattempted") {
			t.Errorf("%s's mutant = %v: want it unattempted, with no outcome", function, got)
		}
	}
	if got := ffMutant(m, "Weak"); got["outcome"] != "survived" || got["state"] != "completed" {
		t.Errorf("Weak's mutant = %v: want it completed, survived", got)
	}
	os.Remove(filepath.Join(ff, "runs"))

	// Without --fail-fast every mutant is judged and the survivor is
	// reported among them: a guardrail of the aggregate run, after the
	// fail-fast one, which kept of the file it cut short only Weak's
	// survivor, which every run tries again.
	all := shellRun(t, "--no-annotate", "--workers", "1")
	logOutcome(t, &all)
	agg := all.ff(t)
	if all.code != 1 || agg.problem("mutation.survived") == nil || runsIn(ff) != 4 {
		t.Errorf("without --fail-fast: exit %d, rules %v, %d runs: want 1 for the survivor, the baseline and all three mutants run",
			all.code, agg.rules(), runsIn(ff))
	}
	if m := agg.file("main.go"); m == nil || len(mutantsOf(m)) != 3 || ffMutant(m, "Weak")["outcome"] != "survived" ||
		ffMutant(m, "Compare")["outcome"] != "killed" || ffMutant(m, "Other")["outcome"] != "killed" {
		t.Errorf("without --fail-fast, main.go = %v: want Weak's mutant survived, the other two killed", m)
	}
}

// holdFiles is the module of ID-MUT-193. Hold's mutant makes test.sh start
// owned descendants (a sleeping child, a shell with a sleeping grandchild,
// and a watcher that marks the worker copy's main.go disappearing while it
// still runs), record their PIDs, mark that it started and wait for a
// release that never comes. Surv's mutant survives once Hold's started, and
// marks that it survived; test.sh kills Later's.
var holdFiles = map[string]string{
	"go.mod":  "module example.com/ff\n\ngo 1.22\n",
	"main.go": "package main\n\nfunc Hold(n int) bool { return n > 5 }\n\nfunc Surv(n int) bool { return n > 6 }\n\nfunc Later(n int) bool { return n > 7 }\n\nfunc main() {}\n",
	"test.sh": ffScript(
		"if ! grep -q 'n > 5' main.go; then",
		"\tsleep 300 &",
		"\techo $! >> \"$FF_DIR/pids\"",
		"\tsh -c 'sleep 300 & echo $! > \"$FF_DIR/grandchild\"; wait' &",
		"\techo $! >> \"$FF_DIR/pids\"",
		"\t(while [ -e main.go ]; do sleep 0.01; done; touch \"$FF_DIR/removed-while-alive\") &",
		"\techo $! >> \"$FF_DIR/pids\"",
		"\twhile [ ! -s \"$FF_DIR/grandchild\" ]; do sleep 0.01; done",
		"\tcat \"$FF_DIR/grandchild\" >> \"$FF_DIR/pids\"",
		"\techo $$ >> \"$FF_DIR/pids\"",
		"\ttouch \"$FF_DIR/started\"",
		"\twhile [ ! -e \"$FF_DIR/release\" ]; do sleep 0.01; done",
		"\texit 0",
		"fi",
		"if ! grep -q 'n > 6' main.go; then",
		"\twhile [ ! -e \"$FF_DIR/started\" ]; do sleep 0.01; done",
		"\ttouch \"$FF_DIR/survived\"",
		"\texit 0",
		"fi",
		"grep -q 'n > 7' main.go || exit 1",
		"exit 0"),
}

// @ID-MUT-193
func TestInFlightJudgmentsAreCancelledAndTheirOwnedProcessesCleanedUp(t *testing.T) {
	bin := buildItosCc(t)
	dir, ff := ffRepo(t, holdFiles)
	tmp := t.TempDir()

	// An unrelated process the run must leave alone.
	unrelated := exec.Command("sleep", "300")
	unrelated.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := unrelated.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { unrelated.Process.Kill(); unrelated.Wait() })
	pidsOf := func() []int {
		var pids []int
		data, _ := os.ReadFile(filepath.Join(ff, "pids"))
		for _, field := range strings.Fields(string(data)) {
			if pid, err := strconv.Atoi(field); err == nil {
				pids = append(pids, pid)
			}
		}
		return pids
	}
	// Whatever the run leaves behind is stopped after the test.
	t.Cleanup(func() {
		for _, pid := range pidsOf() {
			if alive(pid) {
				syscall.Kill(pid, syscall.SIGKILL)
			}
		}
		os.WriteFile(filepath.Join(ff, "release"), nil, 0o644)
	})

	// Two workers: Hold's mutant goes to one and Surv's to the other. A
	// timeout factor this large means a run that drained Hold's judgment
	// instead of cancelling it would outlast the checks below by far.
	var stdout bytes.Buffer
	stderr, err := os.Create(filepath.Join(t.TempDir(), "stderr"))
	if err != nil {
		t.Fatal(err)
	}
	defer stderr.Close()
	run := exec.Command(bin, "mutation", "run", "--json", "--fail-fast", "--no-coverage", "--no-annotate",
		"--test-command", "sh test.sh", "--workers", "2", "--timeout-factor", "10000")
	run.Dir, run.Stdout, run.Stderr = dir, &stdout, stderr
	run.Env = append(testEnv(t), "TMPDIR="+tmp)
	if err := run.Start(); err != nil {
		t.Fatal(err)
	}
	exited := make(chan struct{})
	go func() { run.Wait(); close(exited) }()
	t.Cleanup(func() {
		select {
		case <-exited:
		default:
			run.Process.Kill()
			<-exited
		}
	})
	defer func() {
		if t.Failed() {
			text, _ := os.ReadFile(stderr.Name())
			t.Logf("stdout:\n%s\nstderr:\n%s", stdout.String(), text)
		}
	}()

	// When the other worker's mutant becomes an actionable survivor.
	waitFor(t, "survivor", 3*time.Minute, exited, func() bool {
		_, err := os.Stat(filepath.Join(ff, "survived"))
		return err == nil
	})
	stop := time.Now()
	pids := pidsOf()
	if len(pids) != 5 {
		t.Fatalf("recorded descendants %v, want the child, shell, watcher, grandchild and script", pids)
	}
	select {
	case <-exited:
	case <-time.After(time.Minute):
		t.Fatal("itos-cc did not exit within a minute of the stop")
	}
	took := time.Since(stop)

	// Then the running judgment is cancelled with no outcome.
	o := outcome{code: run.ProcessState.ExitCode(), stdout: stdout.String()}
	f := o.ff(t)
	m := f.file("main.go")
	if hold := ffMutant(m, "Hold"); !undecided(hold, "cancelled") {
		t.Errorf("Hold's mutant = %v: want it cancelled, with no killed, timed-out or survived outcome", hold)
	}
	if surv := ffMutant(m, "Surv"); surv["outcome"] != "survived" {
		t.Errorf("Surv's mutant = %v: want it survived", surv)
	}
	if later := ffMutant(m, "Later"); !undecided(later, "unattempted") {
		t.Errorf("Later's mutant = %v: want it unattempted", later)
	}
	if o.code != 1 || f.stopRule() != "mutation.survived" || f.problem("mutation.survived") == nil {
		t.Errorf("exit %d, stop %v: want 1, stopped by mutation.survived", o.code, f.Stop)
	}
	if n := runsIn(ff); n != 3 {
		t.Errorf("test.sh ran %d times, want 3: the baseline, Hold's and Surv's mutants, and nothing after the stop", n)
	}
	// Its owned processes are terminated and joined within one shared
	// five-second deadline from the stop, before the worker copies go.
	for _, pid := range pids {
		if alive(pid) {
			t.Errorf("owned process %d outlived the run", pid)
		}
	}
	if took > 15*time.Second {
		t.Errorf("the run took %v to finish after the stop, want its cleanup within the five-second deadline", took)
	}
	if _, err := os.Stat(filepath.Join(ff, "removed-while-alive")); err == nil {
		t.Error("a worker copy was removed while an owned process still ran")
	}
	if left, _ := filepath.Glob(filepath.Join(tmp, "itos-cc-mutate-*")); len(left) > 0 {
		t.Errorf("the worker copies %v were left behind", left)
	}
	if !alive(unrelated.Process.Pid) {
		t.Error("an unrelated process was killed")
	}
}

// ffListedMain is the source of ffListedFiles. The binary prints shown(n)
// and label(n) for its n arguments; ID-A-01 runs it with two and wants
// "false" first, ID-A-02 with three and wants "true" first. The own tests
// check quick and spare, call wait, and never call shown, idle or label.
const ffListedMain = `package main

import (
	"flag"
	"fmt"
)

func main() {
	flag.Parse()
	n := flag.NArg()
	fmt.Println(shown(n), label(n))
}

// shown is checked by the listed tests: its mutant survives the own tests,
// and ID-A-01 kills it.
func shown(n int) bool {
	return n > 2
}

// idle is never called: its mutant is uncovered.
func idle(n int) bool {
	return n > 3
}

// quick is checked by the own test, which kills its mutant.
func quick(n int) bool {
	return n > 4
}

// wait is called by the own test, where its mutant blocks.
func wait(n int) bool {
	return n > 5
}

// label is printed and never checked: its mutant survives every test.
func label(n int) bool {
	return n > 6
}

// spare is checked by the own test, which kills its mutant.
func spare(n int) bool {
	return n > 8
}
`

// ffListedFiles is a module whose binary an e2e harness tests by listed
// test ID, as mutate_listed_test.go's does: -tests=<IDs, by commas>, and
// with ITOS_CC_TEST_COVERDIR set, GOCOVERDIR=<it>/<ID> for each test's
// binary. With FF_FAIL_ALONE set, the harness fails whenever it runs
// without coverage, as the clean baseline of a selection runs it, once the
// file FF_FAIL_AFTER names exists, if it names one. TestWait, on wait's
// mutant, marks $FF_DIR/started and waits for a release that never comes.
var ffListedFiles = map[string]string{
	"go.mod":  "module example.com/ff\n\ngo 1.22\n",
	"main.go": ffListedMain,
	"main_test.go": `package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestQuick(t *testing.T) {
	if !quick(5) || quick(4) {
		t.Fatal("quick")
	}
}

func TestWait(t *testing.T) {
	if !wait(5) {
		return
	}
	dir := os.Getenv("FF_DIR")
	os.WriteFile(filepath.Join(dir, "started"), nil, 0o644)
	for {
		if _, err := os.Stat(filepath.Join(dir, "release")); err == nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestSpare(t *testing.T) {
	if !spare(9) || spare(8) {
		t.Fatal("spare")
	}
}
`,
	"e2e/e2e_test.go": `package e2e

import (
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

var selected = flag.String("tests", "", "the IDs of the tests to run, by commas: all of them when empty")

var features = []struct {
	id   string
	args []string
	want string
}{
	{"ID-A-01", []string{"x", "y"}, "false"},
	{"ID-A-02", []string{"x", "y", "z"}, "true"},
}

func TestFeatures(t *testing.T) {
	split := os.Getenv("ITOS_CC_TEST_COVERDIR")
	covered := split != "" || os.Getenv("GOCOVERDIR") != ""
	if !covered && os.Getenv("FF_FAIL_ALONE") != "" {
		for after := os.Getenv("FF_FAIL_AFTER"); after != ""; time.Sleep(10 * time.Millisecond) {
			if _, err := os.Stat(after); err == nil {
				break
			}
		}
		t.Fatal("the listed tests fail when they run without coverage")
	}
	bin := filepath.Join(t.TempDir(), "ff.exe")
	build := []string{"build", "-o", bin, ".."}
	if covered {
		build = []string{"build", "-cover", "-o", bin, ".."}
	}
	if out, err := exec.Command("go", build...).CombinedOutput(); err != nil {
		t.Fatalf("go %v: %v\n%s", build, err, out)
	}
	for _, f := range features {
		if *selected != "" && !slices.Contains(strings.Split(*selected, ","), f.id) {
			continue
		}
		cmd := exec.Command(bin, f.args...)
		if split != "" {
			dir := filepath.Join(split, f.id)
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			cmd.Env = append(os.Environ(), "GOCOVERDIR="+dir)
		}
		out, err := cmd.Output()
		if err != nil {
			t.Errorf("%s: %v", f.id, err)
			continue
		}
		if got := strings.Fields(string(out)); len(got) == 0 || got[0] != f.want {
			t.Errorf("%s: printed %q, want %s first", f.id, out, f.want)
		}
	}
}
`,
	"itos-cc.yaml": `mutation:
  tests:
    list: printf "ID-A-01\nID-A-02\n"
    run: go test -count=1 ./e2e -args -tests={pattern}
    ids_pattern: "{ids}"
    join:
      each: "{id}"
      sep: ","
`,
}

// ffListedRepo makes ffListedFiles a Git repository and the test's
// directory (useDir), then commits a comment inside each function of judged, so a
// run with --since HEAD~1 judges those alone. FF_DIR points at a fresh
// directory, which it returns.
func ffListedRepo(t *testing.T, judged ...string) string {
	t.Helper()
	moduleRepo(t, ffListedFiles)
	for _, function := range judged {
		head := "func " + function + "(n int) bool {\n"
		edit(t, "main.go", head, head+"\t// judged\n")
	}
	commitAll(t, "judge "+strings.Join(judged, " "))
	ff := t.TempDir()
	useEnv(t, "FF_DIR", ff)
	return ff
}

// listedRun runs mutation run --json --since HEAD~1 with args.
func listedRun(t *testing.T, args ...string) outcome {
	t.Helper()
	return cli(t, append([]string{"mutation", "run", "--json", "--no-annotate", "--since", "HEAD~1"}, args...)...)
}

// ffException is an itos-cc.yaml exception entry for the one site of
// function in the committed repository at dir, with hash instead of its
// function's when hash is not empty.
func ffException(t *testing.T, dir, function, hash string) string {
	t.Helper()
	plan, err := mutate.PlanFresh(dir, nil, "", 1, "any")
	if err != nil {
		t.Fatal(err)
	}
	var c *mutate.FreshCandidate
	for _, e := range plan.Eligible {
		if strings.HasSuffix(e.Function, "#"+function) {
			c = &e
		}
	}
	plan.Close()
	if c == nil {
		t.Fatalf("no site of %s", function)
	}
	return exceptionEntry(t, dir, *c, hash, "no test can tell")
}

// @ID-MUT-194
func TestNonActionableJudgmentsNeverStopTheRun(t *testing.T) {
	t.Run("killed, timed out and excepted", func(t *testing.T) {
		// Compare's mutant is killed, Slow's makes test.sh sleep past its
		// timeout, and Same's survives, which itos-cc.yaml excepts.
		dir, ff := ffRepo(t, map[string]string{
			"go.mod":  "module example.com/ff\n\ngo 1.22\n",
			"main.go": "package main\n\nfunc Compare(n int) bool { return n > 5 }\n\nfunc Slow(n int) bool { return n > 6 }\n\nfunc Same(n int) bool { return n > 7 }\n\nfunc main() {}\n",
			"test.sh": ffScript("grep -q 'n > 5' main.go || exit 1", "grep -q 'n > 6' main.go || { sleep 60; exit 0; }", "exit 0"),
		})
		writeFile(t, filepath.Join(dir, "itos-cc.yaml"), "mutation:\n  exceptions:\n"+ffException(t, dir, "Same", ""))

		o := shellRun(t, "--no-annotate", "--workers", "1", "--fail-fast")
		logOutcome(t, &o)
		f := o.ff(t)
		m := f.file("main.go")
		if got := ffMutant(m, "Compare"); got["outcome"] != "killed" || got["state"] != "completed" {
			t.Errorf("Compare's mutant = %v: want it completed, killed", got)
		}
		if got := ffMutant(m, "Slow"); got["outcome"] != "timeout" || got["state"] != "completed" {
			t.Errorf("Slow's mutant = %v: want it completed, timed out", got)
		}
		if got := ffMutant(m, "Same"); got["outcome"] != "survived" || got["excepted"] == nil || got["state"] != "completed" {
			t.Errorf("Same's mutant = %v: want it completed, an excepted survivor", got)
		}
		// Every selected mutant is judged and the run succeeds.
		if o.code != 0 || !f.OK || f.Stop == nil || f.stopped() || f.work("completed") != 3 || runsIn(ff) != 4 {
			t.Errorf("exit %d, stop %v, work %v, %d runs: want 0, not stopped, all three mutants completed after the baseline",
				o.code, f.Stop, f.Work, runsIn(ff))
		}
	})

	t.Run("killed by listed tests", func(t *testing.T) {
		// shown's mutant survives its own tests, then ID-A-01 kills it.
		ffListedRepo(t, "shown")
		o := listedRun(t, "--workers", "1", "--fail-fast")
		logOutcome(t, &o)
		f := o.ff(t)
		got := ffMutant(f.file("main.go"), "shown")
		if got["outcome"] != "killed" || got["scope"] != "listed" || got["state"] != "completed" {
			t.Errorf("shown's mutant = %v: want it completed, killed by the listed tests once it survived its own", got)
		}
		// A survivor waits for its applicable listed tests before it can
		// stop the run, and once they kill it, it never does.
		if o.code != 0 || !f.OK || f.Stop == nil || f.stopped() || f.work("completed") != 1 {
			t.Errorf("exit %d, stop %v, work %v: want 0, not stopped, its one mutant completed", o.code, f.Stop, f.Work)
		}
	})
}

// compareTwins is a module whose test kills Compare's mutant, beside
// functions no test runs: Half, with a site, and Dormant, with executable
// statements and no site.
func compareTwins(extra string) map[string]string {
	return map[string]string{
		"go.mod":       "module example.com/ff\n\ngo 1.22\n",
		"main.go":      "package main\n\nfunc Compare(n int) bool { return n > 5 }\n\n" + extra + "\nfunc main() {}\n",
		"main_test.go": "package main\n\nimport \"testing\"\n\nfunc TestCompare(t *testing.T) {\n\tif !Compare(6) || Compare(5) {\n\t\tt.Fatal(\"Compare\")\n\t}\n}\n",
	}
}

const (
	halfSource    = "func Half(i int) bool { return i > 2 }\n"
	dormantSource = "func Dormant(flag bool) string {\n\tif flag {\n\t\treturn \"yes\"\n\t}\n\treturn \"no\"\n}\n"
)

// @ID-MUT-195
func TestKnownPolicyFailuresStopBeforeAvoidableMutantWork(t *testing.T) {
	t.Run("stale exception", func(t *testing.T) {
		// The entry for Compare's mutant names another hash: it is stale
		// before anything runs.
		dir, ff := ffRepo(t, map[string]string{
			"go.mod":  "module example.com/ff\n\ngo 1.22\n",
			"main.go": "package main\n\nfunc Compare(n int) bool { return n > 5 }\n\nfunc main() {}\n",
			"test.sh": ffScript("grep -q 'n > 5' main.go || exit 1", "exit 0"),
		})
		writeFile(t, filepath.Join(dir, "itos-cc.yaml"), "mutation:\n  exceptions:\n"+ffException(t, dir, "Compare", "0000000000000000"))

		o := shellRun(t, "--no-annotate", "--workers", "1", "--fail-fast")
		logOutcome(t, &o)
		f := o.ff(t)
		p := f.problem("mutation.exception-stale")
		if o.code != 1 || p == nil || p["why"] != "changed" {
			t.Errorf("exit %d, problems %v: want 1 with mutation.exception-stale, why changed", o.code, f.Problems)
		}
		if !f.stopped() || f.stopRule() != "mutation.exception-stale" || f.stopSubject()["why"] != "changed" {
			t.Errorf("stop = %v: want the run stopped by the stale exception", f.Stop)
		}
		if n := runsIn(ff); n != 0 {
			t.Errorf("test.sh ran %d times, want never: planning knew the failure", n)
		}
		if got := ffMutant(f.file("main.go"), "Compare"); !undecided(got, "unattempted") {
			t.Errorf("Compare's mutant = %v: want it unattempted", got)
		}

		// The run without --fail-fast, after it, reports the same rule.
		all := shellRun(t, "--no-annotate", "--workers", "1")
		logOutcome(t, &all)
		if all.code != 1 || all.ff(t).problem("mutation.exception-stale") == nil || runsIn(ff) == 0 {
			t.Errorf("without --fail-fast: exit %d, %d runs: want 1 with mutation.exception-stale, its mutant run", all.code, runsIn(ff))
		}
	})

	for _, c := range []struct {
		name, extra string
		rules       []string
	}{
		{"strict Go coverage finding", dormantSource, []string{"mutation.uncovered-statement"}},
		{"uncovered mutant", halfSource, []string{"mutation.uncovered", "mutation.uncovered-statement"}},
	} {
		t.Run(c.name+" under --fail-uncovered", func(t *testing.T) {
			moduleRepo(t, compareTwins(c.extra))
			o := cli(t, "mutation", "run", "--json", "--no-annotate", "--workers", "1", "--fail-uncovered", "--fail-fast")
			logOutcome(t, &o)
			f := o.ff(t)
			// The run without --fail-fast, after it, judges Compare's mutant.
			all := cli(t, "mutation", "run", "--json", "--no-annotate", "--workers", "1", "--fail-uncovered")
			logOutcome(t, &all)
			agg := all.ff(t)
			if all.code != 1 || trialLines(all.stderr) != 1 {
				t.Errorf("without --fail-fast: exit %d, %d trials: want 1, with Compare's mutant run", all.code, trialLines(all.stderr))
			}
			// It fails with the same rule the aggregate run reports.
			if o.code != 1 || !f.stopped() || !slices.Contains(c.rules, f.stopRule()) || !slices.Contains(agg.rules(), f.stopRule()) ||
				f.problem(f.stopRule()) == nil {
				t.Errorf("exit %d, stop %v, rules %v, aggregate rules %v: want 1, stopped by one of %v that both report",
					o.code, f.Stop, f.rules(), agg.rules(), c.rules)
			}
			// No mutant trial starts, nor its baseline.
			if trialLines(o.stderr) != 0 || strings.Contains(o.stderr, "itos-cc: baseline") {
				t.Errorf("a mutant or its baseline ran after planning knew the failure:\n%s", o.stderr)
			}
			if got := ffMutant(f.file("main.go"), "Compare"); !undecided(got, "unattempted") {
				t.Errorf("Compare's mutant = %v: want it unattempted", got)
			}
		})
	}

	t.Run("uncovered mutant without --fail-uncovered", func(t *testing.T) {
		moduleRepo(t, compareTwins(halfSource))
		o := cli(t, "mutation", "run", "--json", "--no-annotate", "--workers", "1", "--fail-fast")
		logOutcome(t, &o)
		f := o.ff(t)
		m := f.file("main.go")
		if half := ffMutant(m, "Half"); half["outcome"] != "uncovered" || half["state"] != "completed" {
			t.Errorf("Half's mutant = %v: want it completed, uncovered", half)
		}
		if got := ffMutant(m, "Compare"); got["outcome"] != "killed" {
			t.Errorf("Compare's mutant = %v: want it judged, killed", got)
		}
		if o.code != 0 || f.Stop == nil || f.stopped() || f.work("completed") != 2 {
			t.Errorf("exit %d, stop %v, work %v: want 0, not stopped, both mutants completed", o.code, f.Stop, f.Work)
		}
	})
}

// @ID-MUT-196
func TestAFailingBaselineStopsTheRunWithoutInventingOutcomes(t *testing.T) {
	t.Run("own baseline", func(t *testing.T) {
		// Two modules, so two baselines: a's test.sh fails on the source
		// as it is, which marks itself broken; b's would kill its mutant.
		_, ff := ffRepo(t, map[string]string{
			"a/go.mod":  "module example.com/a\n\ngo 1.22\n",
			"a/main.go": "package main\n\n// BROKEN\nfunc A(n int) bool { return n > 5 }\n\nfunc main() {}\n",
			"a/test.sh": ffScript("grep -q BROKEN main.go && exit 1", "exit 0"),
			"b/go.mod":  "module example.com/b\n\ngo 1.22\n",
			"b/main.go": "package main\n\nfunc B(n int) bool { return n > 6 }\n\nfunc main() {}\n",
			"b/test.sh": ffScript("grep -q 'n > 6' main.go || exit 1", "exit 0"),
		})
		o := shellRun(t, "--no-annotate", "--workers", "1", "--fail-fast", "a/main.go", "b/main.go")
		logOutcome(t, &o)
		f := o.ff(t)
		p := f.problem("mutation.baseline-failed")
		if o.code != 1 || p == nil || p["file"] != "a/main.go" {
			t.Errorf("exit %d, problems %v: want 1 with mutation.baseline-failed for a/main.go", o.code, f.Problems)
		}
		if !f.stopped() || f.stopRule() != "mutation.baseline-failed" || f.stopSubject()["file"] != "a/main.go" {
			t.Errorf("stop = %v: want the run stopped by a/main.go's baseline", f.Stop)
		}
		// The affected mutants have no invented outcome.
		a, b := f.file("a/main.go"), f.file("b/main.go")
		if a == nil || a["baseline"] != "failed" || a["state"] != "blocked" || !undecided(ffMutant(a, "A"), "blocked") {
			t.Errorf("a/main.go = %v: want its baseline failed, and the file and its mutant blocked with no outcome", a)
		}
		// No stage reports a baseline as passed that never ran: b's never did.
		if b == nil || b["baseline"] != "not-run" || b["state"] != "unattempted" || !undecided(ffMutant(b, "B"), "unattempted") {
			t.Errorf("b/main.go = %v: want its baseline not-run, and the file and its mutant unattempted", b)
		}
		if n := runsIn(ff); n != 1 {
			t.Errorf("test.sh ran %d times, want once: a's baseline alone", n)
		}
	})

	t.Run("listed selection baseline", func(t *testing.T) {
		// label's mutant survives its own tests; the clean baseline of the
		// listed tests that reach it fails.
		ffListedRepo(t, "label")
		useEnv(t, "FF_FAIL_ALONE", "1")
		o := listedRun(t, "--workers", "1", "--fail-fast")
		logOutcome(t, &o)
		f := o.ff(t)
		p := f.problem("tests.selection-failed")
		if o.code != 1 || p == nil || fmt.Sprint(p["ids"]) != "[ID-A-01 ID-A-02]" {
			t.Errorf("exit %d, problems %v: want 1 with tests.selection-failed for [ID-A-01 ID-A-02]", o.code, f.Problems)
		}
		if !f.stopped() || f.stopRule() != "tests.selection-failed" || fmt.Sprint(f.stopSubject()["ids"]) != "[ID-A-01 ID-A-02]" {
			t.Errorf("stop = %v: want the run stopped by the failing selection", f.Stop)
		}
		m := f.file("main.go")
		if got := ffMutant(m, "label"); !undecided(got, "blocked") {
			t.Errorf("label's mutant = %v: want it blocked, with no killed, timed-out or survived outcome", got)
		}
		if strings.Contains(o.stderr, "baseline of ID-A-01 ID-A-02 passed") {
			t.Errorf("stderr reports the failing selection's baseline passed:\n%s", o.stderr)
		}
		if m == nil || m["baseline"] != "passed" || m["state"] != "blocked" {
			t.Errorf("main.go = %v: want its own baseline passed, as it ran, and the file blocked", m)
		}
	})
}

// @ID-MUT-197
func TestOutputDistinguishesCompletedWorkFromCancelledAndUnattemptedWork(t *testing.T) {
	// Two workers judge idle (uncovered), quick (killed), wait (held),
	// label (its selection fails, once wait's mutant started) and spare.
	// quick's goes to one worker and wait's to the other; label's follows
	// quick's, and its failing selection stops the run while wait's is
	// still held; spare's is never started.
	ff := ffListedRepo(t, "idle", "quick", "wait", "label", "spare")
	useEnv(t, "FF_FAIL_ALONE", "1")
	useEnv(t, "FF_FAIL_AFTER", filepath.Join(ff, "started"))
	t.Cleanup(func() { os.WriteFile(filepath.Join(ff, "release"), nil, 0o644) })

	o := listedRun(t, "--workers", "2", "--fail-fast")
	logOutcome(t, &o)
	f := o.ff(t)
	// JSON is one object with a stop naming the trigger's rule and subject.
	if o.code != 1 || !f.stopped() || f.stopRule() != "tests.selection-failed" || fmt.Sprint(f.stopSubject()["ids"]) != "[ID-A-01 ID-A-02]" {
		t.Errorf("exit %d, stop %v: want 1, stopped by tests.selection-failed for [ID-A-01 ID-A-02]", o.code, f.Stop)
	}
	// Disjoint completed, cancelled, unattempted and blocked counts, and
	// every selected file's state.
	want := map[string]int{"completed": 2, "cancelled": 1, "unattempted": 1, "blocked": 1}
	for state, n := range want {
		if f.work(state) != n {
			t.Errorf("work = %v: want %v", f.Work, want)
			break
		}
	}
	m := f.file("main.go")
	if len(f.Files) != 1 || m == nil || m["state"] != "blocked" || m["baseline"] != "passed" {
		t.Errorf("files = %v: want main.go alone, blocked, its own baseline passed", f.Files)
	}
	states := map[string]string{"idle": "completed", "quick": "completed", "wait": "cancelled", "label": "blocked", "spare": "unattempted"}
	outcomes := map[string]string{"idle": "uncovered", "quick": "killed"}
	for function, state := range states {
		got := ffMutant(m, function)
		if got["state"] != state || (outcomes[function] == "" && !undecided(got, state)) ||
			(outcomes[function] != "" && got["outcome"] != outcomes[function]) {
			t.Errorf("%s's mutant = %v: want it %s, %s", function, got, state, cmpOr(outcomes[function], "with no outcome"))
		}
	}

	// Plain output says the run stopped early and why.
	os.Remove(filepath.Join(ff, "started"))
	text := cli(t, "mutation", "run", "--no-annotate", "--since", "HEAD~1", "--workers", "2", "--fail-fast")
	logOutcome(t, &text)
	if text.code != 1 || !strings.Contains(text.stdout, "stopped early") || !strings.Contains(text.stdout, "tests.selection-failed") {
		t.Errorf("plain output, exit %d:\n%s\nwant it to say the run stopped early, at tests.selection-failed", text.code, text.stdout)
	}

	// A run without --fail-fast keeps its existing output exactly: no new
	// key, and the plain lines it always printed.
	_, _ = ffRepo(t, threeFiles)
	plain := cli(t, "mutation", "run", "--no-coverage", "--test-command", "sh test.sh", "--no-annotate", "--workers", "1")
	if wantText := "main.go: 2 killed, 1 survived, 0 uncovered (ran 3, reused 0)\n  survived main.go:4:34 `>` → `>=` in example.com/ff#Weak\n"; plain.stdout != wantText {
		t.Errorf("plain output without --fail-fast:\n%q\nwant exactly\n%q", plain.stdout, wantText)
	}
	agg := shellRun(t, "--no-annotate", "--workers", "1", "--mutate-all")
	var raw map[string]any
	if err := json.Unmarshal([]byte(agg.stdout), &raw); err != nil {
		t.Fatal(err)
	}
	var keys []string
	for k := range raw {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	if !slices.Equal(keys, []string{"files", "ok", "problems", "schema"}) {
		t.Errorf("--json keys without --fail-fast = %v, want files, ok, problems and schema", keys)
	}
	for _, file := range agg.ff(t).Files {
		if _, has := file["state"]; has || file["baseline"] != "passed" {
			t.Errorf("file without --fail-fast = %v: want no state and its baseline passed", file)
		}
		for _, mu := range mutantsOf(file) {
			if _, has := mu["state"]; has {
				t.Errorf("mutant without --fail-fast = %v: want no state", mu)
			}
		}
	}
}

func cmpOr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// @ID-MUT-198
func TestAFileTheStopCutShortPublishesNoPartialProof(t *testing.T) {
	// One worker judges a.go's mutant, killed, then b.go's: B's survives,
	// and C's is never started.
	_, ff := ffRepo(t, map[string]string{
		"go.mod":  "module example.com/ff\n\ngo 1.22\n",
		"a.go":    "package main\n\nfunc A(n int) bool { return n > 5 }\n\nfunc main() {}\n",
		"b.go":    "package main\n\nfunc B(n int) bool { return n > 6 }\n\nfunc C(n int) bool { return n > 7 }\n",
		"test.sh": ffScript("grep -q 'n > 5' a.go || exit 1", "grep -q 'n > 7' b.go || exit 1", "exit 0"),
	})
	read := func(name string) string {
		data, _ := os.ReadFile(inWD(t, name))
		return string(data)
	}
	aSnapshot, bSnapshot := filepath.Join(".metrics", "mutate", "a.go.json"), filepath.Join(".metrics", "mutate", "b.go.json")
	aSource, bSource := read("a.go"), read("b.go")

	o := shellRun(t, "--workers", "1", "--fail-fast")
	logOutcome(t, &o)
	f := o.ff(t)
	if o.code != 1 || f.stopRule() != "mutation.survived" || f.file("a.go")["state"] != "completed" || f.file("b.go")["state"] != "stopped" {
		t.Errorf("exit %d, stop %v, files %v: want 1, stopped by B's survivor, a.go completed and b.go stopped", o.code, f.Stop, f.Files)
	}
	// The completed file's snapshot and annotation are written.
	aWritten, aAnnotated := read(aSnapshot), read("a.go")
	if aWritten == "" || aAnnotated == aSource {
		t.Errorf("a.go's snapshot %q and source: want both written, the source annotated", aWritten)
	}
	// The stopped file's annotated source bytes are unchanged. Its snapshot
	// publishes no proof of the site the stop left: since fail-fast-partial
	// (ADR-0022) it keeps B's completed judgment, and has no entry for C's.
	if snap := ffSnapshot(t, "b.go"); len(ffRecorded(snap, "C")) != 0 {
		t.Errorf("b.go's snapshot records C's unattempted mutant:\n%s", read(bSnapshot))
	}
	if read("b.go") != bSource {
		t.Errorf("b.go was annotated:\n%s", read("b.go"))
	}

	// What a.go got is what a run without --fail-fast writes.
	again := shellRun(t, "--workers", "1", "--mutate-all", "a.go")
	logOutcome(t, &again)
	if again.code != 0 || read(aSnapshot) != aWritten || read("a.go") != aAnnotated {
		t.Errorf("without --fail-fast, exit %d, a.go's snapshot:\n%s\nsource:\n%s\nwant 0 and the bytes the fail-fast run wrote:\n%s\n%s",
			again.code, read(aSnapshot), read("a.go"), aWritten, aAnnotated)
	}

	// mutation check still reports the stopped file's missing or stale
	// results without running tests: C's, which no valid result records.
	runs := runsIn(ff)
	check := cli(t, "mutation", "check", "--json", "b.go")
	logOutcome(t, &check)
	if rules := ffCheckRules(t, check); check.code != 1 || !unjudgedRule(rules["C"]) || runsIn(ff) != runs {
		t.Errorf("mutation check b.go: exit %d, rules %v, %d runs: want 1, C missing or stale, and no test run", check.code, rules, runsIn(ff)-runs)
	}

	// A later run reuses only outcomes whose inputs are still fresh: a.go's
	// published kill, and nothing of b.go's.
	later := shellRun(t, "--workers", "1", "--no-annotate")
	logOutcome(t, &later)
	l := later.ff(t)
	if a, b := l.file("a.go"), l.file("b.go"); a["reused"] != float64(1) || a["ran"] != float64(0) || b["reused"] != float64(0) || b["ran"] != float64(2) {
		t.Errorf("later files = %v: want a.go's mutant reused, b.go's two run", l.Files)
	}
}
