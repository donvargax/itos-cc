package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

// The scenarios of "Rule: Each selection of listed tests has its own
// baseline" in features/mutate.feature. They use the module of
// mutate_listed_test.go, whose harness also records, on each run, the
// SHA-256 of the main.go it builds the binary from: a run whose hash is that
// of the main.go written ran without a mutant. ID-A-01 alone reaches the
// lines of greet's n == 1 branch, ID-A-02 alone those of n-1 (line 17), and
// both the rest of greet and main.

// The harness's run record, and what it becomes: the same line, with the
// hash of ../main.go.
const (
	harnessRecord   = `fmt.Fprintf(f, "run tests=%s coverdir=%s\n", *selected, os.Getenv("ITOS_CC_TEST_COVERDIR"))`
	harnessRecorded = `src, _ := os.ReadFile(filepath.Join("..", "main.go"))
		fmt.Fprintf(f, "run tests=%s coverdir=%s main=%x\n", *selected, os.Getenv("ITOS_CC_TEST_COVERDIR"), sha256.Sum256(src))`
)

// selectionSleep is how long ID-A-02 takes once the binary passed it, in
// the module of ID-MUT-135, where it is the slow test.
const selectionSleep = 2 * time.Second

// selectionRepo makes the module of mutate_listed_test.go, with the harness
// recording main.go's hash and each of changes made, the first of its
// text in the file it names, as listedRepo does. It returns the record file
// and the text of main.go.
func selectionRepo(t *testing.T, changes ...[3]string) (string, string) {
	t.Helper()
	files := map[string]string{}
	for name, text := range listedFiles {
		files[name] = text
	}
	changes = append([][3]string{
		{"e2e/e2e_test.go", `import (` + "\n", "import (\n\t\"crypto/sha256\"\n"},
		{"e2e/e2e_test.go", harnessRecord, harnessRecorded},
	}, changes...)
	for _, c := range changes {
		if !strings.Contains(files[c[0]], c[1]) {
			t.Fatalf("%s holds no %q", c[0], c[1])
		}
		files[c[0]] = strings.Replace(files[c[0]], c[1], c[2], 1)
	}
	moduleRepo(t, files)
	record := filepath.Join(t.TempDir(), "record")
	t.Setenv("GREET_RECORD", record)
	return record, files["main.go"]
}

// unmutatedRuns is how many runs of record's harness selected ids alone,
// with no ITOS_CC_TEST_COVERDIR, on a main.go that is main: without a
// mutant, outside coverage. It also returns the index of each such run and
// of the first run selecting ids on any other main.go, -1 when none did.
func unmutatedRuns(lines []string, ids, main string) (int, []int, int) {
	prefix := "run tests=" + ids + " coverdir= main="
	clean := prefix + fmt.Sprintf("%x", sha256.Sum256([]byte(main)))
	var at []int
	mutated := -1
	for i, l := range lines {
		switch {
		case l == clean:
			at = append(at, i)
		case strings.HasPrefix(l, prefix) && mutated < 0:
			mutated = i
		}
	}
	return len(at), at, mutated
}

// lineOf is the line of text, from 1, that holds s.
func lineOf(t *testing.T, text, s string) int {
	t.Helper()
	for i, l := range strings.Split(text, "\n") {
		if strings.Contains(l, s) {
			return i + 1
		}
	}
	t.Fatalf("no line holds %q", s)
	return 0
}

// mutationRun runs mutation run with args, no source file annotated, and
// returns what it printed.
func mutationRun(t *testing.T, args ...string) outcome {
	t.Helper()
	return cli(t, append([]string{"mutation", "run", "--no-annotate"}, args...)...)
}

// mainMutants is the mutants of main.go in o's --json output, as plain maps,
// with the file's own entry.
func mainMutants(t *testing.T, o outcome) ([]map[string]any, map[string]any) {
	t.Helper()
	for _, f := range o.rawFiles(t) {
		if f["file"] != greetSource {
			continue
		}
		var out []map[string]any
		list, _ := f["mutants"].([]any)
		for _, m := range list {
			out = append(out, m.(map[string]any))
		}
		return out, f
	}
	t.Fatalf("no %s in files:\n%s\nstderr:\n%s", greetSource, o.stdout, o.stderr)
	return nil, nil
}

// seconds reads a duration printed as seconds, such as "1.2".
func seconds(t *testing.T, s string) time.Duration {
	t.Helper()
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		t.Fatal(err)
	}
	return time.Duration(f * float64(time.Second))
}

// selectionBaseline is how long the baseline of the selection ids took and
// the timeout it set, as stderr says.
func selectionBaseline(t *testing.T, stderr, ids string) (time.Duration, time.Duration) {
	t.Helper()
	re := regexp.MustCompile(`(?m)^itos-cc: baseline of ` + regexp.QuoteMeta(ids) + ` passed in ([0-9.]+)s: a mutant running it times out after ([0-9.]+)s$`)
	m := re.FindStringSubmatch(stderr)
	if m == nil {
		t.Fatalf("stderr says nothing of a baseline of %s passing, with its time and timeout:\n%s", ids, stderr)
	}
	return seconds(t, m[1]), seconds(t, m[2])
}

// @ID-MUT-135
func TestASelectionsTimeoutComesFromItsOwnRun(t *testing.T) {
	record, main := selectionRepo(t,
		// The branch ID-A-01 alone reaches holds a loop its mutants make
		// endless: n > 1 never holds there.
		[3]string{"main.go", "\t\"os\"\n)", "\t\"os\"\n\t\"time\"\n)"},
		[3]string{"main.go", "if n == 1 {\n", "if n == 1 {\n\t\tfor n > 1 {\n\t\t\ttime.Sleep(time.Second)\n\t\t}\n"},
		// ID-A-02 is the slow test.
		[3]string{"e2e/e2e_test.go", "\t\"testing\"\n)", "\t\"testing\"\n\t\"time\"\n)"},
		[3]string{"e2e/e2e_test.go", "\t\t\tt.Errorf(\"%s: greet printed %q, want %q\", f.id, got, f.want)\n\t\t}\n",
			"\t\t\tt.Errorf(\"%s: greet printed %q, want %q\", f.id, got, f.want)\n\t\t} else if f.id == \"ID-A-02\" {\n\t\t\ttime.Sleep(" +
				strconv.Itoa(int(selectionSleep/time.Second)) + " * time.Second)\n\t\t}\n"})
	hang := lineOf(t, main, "for n > 1 {")

	o := mutationRun(t, "--workers", "1", "--timeout-factor", "3", "--json", greetSource)
	mutants, file := mainMutants(t, o)
	defer func() {
		if t.Failed() {
			t.Logf("stdout:\n%s\nstderr:\n%s", o.stdout, o.stderr)
		}
	}()

	// ID-A-01 runs once alone without a mutant, before any mutant runs it
	// alone.
	n, at, mutated := unmutatedRuns(harnessRuns(t, record), "ID-A-01", main)
	if n != 1 || mutated < 0 || at[0] > mutated {
		t.Fatalf("ID-A-01 ran alone without a mutant %d times, at %v, and with one first at %d: want once, before any mutant's run", n, at, mutated)
	}

	// Each mutant of the loop times out after three times that run, at
	// least 2 seconds, not after three times both tests' time, which is at
	// least ID-A-01's and ID-A-02's sleep.
	took, timeout := selectionBaseline(t, o.stderr, "ID-A-01")
	if want := max(2*time.Second, 3*took); timeout < want-250*time.Millisecond || timeout > want+250*time.Millisecond {
		t.Errorf("ID-A-01's baseline took %v and set a timeout of %v, want %v: three times it, at least 2s", took, timeout, want)
	}
	both := 3 * (took + selectionSleep)
	re := regexp.MustCompile(fmt.Sprintf(`(?m) %s:%d .* then timeout with ID-A-01 \(([0-9.]+)s, the listed tests ([0-9.]+)s\)$`,
		regexp.QuoteMeta(greetSource), hang))
	found := re.FindAllStringSubmatch(o.stderr, -1)
	if len(found) != 2 {
		t.Fatalf("stderr shows %d mutants of line %d timing out with ID-A-01 and the listed tests' time, want 2", len(found), hang)
	}
	for _, m := range found {
		// A timed-out command is killed at once on Unix; on Windows its
		// children may hold its output up to 2s more.
		if listed := seconds(t, m[2]); listed < timeout-150*time.Millisecond || listed > timeout+3500*time.Millisecond || listed >= both {
			t.Errorf("a mutant of line %d ran ID-A-01 for %v: want it timed out after %v, well before %v, three times both tests' time", hang, listed, timeout, both)
		}
	}

	// Its outcome is "timeout", counted killed.
	killed := 0
	for _, m := range mutants {
		if m["outcome"] == "killed" || m["outcome"] == "timeout" {
			killed++
		}
		if mutantLine(m) == hang && (m["outcome"] != "timeout" || m["scope"] != "listed" || !slices.Equal(listedTests(m), []string{"ID-A-01"})) {
			t.Errorf("%s: scope %v, tests %v, want timeout with ID-A-01", describe(m), m["scope"], m["tests"])
		}
	}
	if file["killed"] != float64(killed) {
		t.Errorf("main.go's killed %v, want %d, the killed and timed-out mutants", file["killed"], killed)
	}
}

// @ID-MUT-136
func TestASelectionIsRunWithoutAMutantOncePerRun(t *testing.T) {
	record, main := selectionRepo(t)

	// Two workers, so the two mutants of line 17 may need ID-A-02 at once.
	o := mutationRun(t, "--workers", "2", "--json", greetSource)
	mutants, _ := mainMutants(t, o)
	many := 0
	for _, m := range mutants {
		if mutantLine(m) == listedManyLine {
			many++
			if m["outcome"] != "killed" || m["scope"] != "listed" {
				t.Errorf("%s: scope %v, want both mutants of line %d surviving their own tests, then killed by ID-A-02", describe(m), m["scope"], listedManyLine)
			}
		}
	}
	if many != 2 {
		t.Errorf("%d mutants on line %d, want 2", many, listedManyLine)
	}

	if n, at, _ := unmutatedRuns(harnessRuns(t, record), "ID-A-02", main); n != 1 {
		t.Errorf("ID-A-02 ran alone without a mutant %d times (at %v), want exactly once", n, at)
	}
	if t.Failed() {
		t.Logf("stdout:\n%s\nstderr:\n%s", o.stdout, o.stderr)
	}
}

// @ID-MUT-137
func TestASelectionThatFailsWithoutAMutantDecidesNoneOfItsMutants(t *testing.T) {
	selectionRepo(t, [3]string{"e2e/e2e_test.go", "\tsplit := os.Getenv(\"ITOS_CC_TEST_COVERDIR\")\n",
		"\tif os.Getenv(\"GREET_FAIL_ALONE\") != \"\" && *selected == \"ID-A-02\" {\n\t\tt.Fatal(\"ID-A-02 fails when it runs alone\")\n\t}\n" +
			"\tsplit := os.Getenv(\"ITOS_CC_TEST_COVERDIR\")\n"})
	// A first run records the mutants of line 17 as killed by ID-A-02; the
	// snapshot is then made to hold them survived, so the next run retries
	// them.
	if o := mutationRun(t, "--workers", "1", "--json", greetSource); o.code != 1 {
		t.Fatalf("the first run: exit %d, want 1 for the survivor of line %d\n%s%s", o.code, listedStderrLine, o.stdout, o.stderr)
	}
	path := filepath.Join(".metrics", "mutate", "main.go.json")
	snap := rawSnapshot(t)
	many := 0
	for _, m := range snapshotMutants(snap) {
		if line, _ := m["line"].(float64); int(line) == listedManyLine {
			if m["outcome"] != "killed" {
				t.Fatalf("the first run recorded %v, want line %d killed by ID-A-02", m, listedManyLine)
			}
			m["outcome"] = "survived"
			many++
		}
	}
	if many != 2 {
		t.Fatalf("%d mutants on line %d in the snapshot, want 2", many, listedManyLine)
	}
	data, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	held := string(data) + "\n"
	writeFile(t, path, held)
	t.Setenv("GREET_FAIL_ALONE", "1")

	o := mutationRun(t, "--workers", "1", "--json", greetSource)
	defer func() {
		if t.Failed() {
			t.Logf("stdout:\n%s\nstderr:\n%s", o.stdout, o.stderr)
		}
	}()

	p := o.json(t).problem("tests.selection-failed")
	if p == nil {
		t.Fatal("no tests.selection-failed problem")
	}
	if fmt.Sprint(p["ids"]) != "[ID-A-02]" {
		t.Errorf("problem %v, want its ids [ID-A-02]", p)
	}

	mutants, _ := mainMutants(t, o)
	for _, m := range mutants {
		if mutantLine(m) == listedManyLine && (m["outcome"] == "killed" || m["outcome"] == "timeout") {
			t.Errorf("%s: want no mutant that would run ID-A-02 recorded killed", describe(m))
		}
	}
	now, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(now) != held {
		t.Errorf("the snapshot of main.go changed:\n%s\nwant what it held:\n%s", now, held)
	}

	if o.code != 1 {
		t.Errorf("exit %d, want 1", o.code)
	}
}
