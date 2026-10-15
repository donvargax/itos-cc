// Real executable fixture harvested from cmd/itos-cc/mutate_fail_fast_test.go.
// Its Go tests and e2e harness decide outcomes; steps observe only public output.
package features

const executionListedMain = `package main

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

// executionListedFiles is a module whose binary an e2e harness tests by listed
// test ID, as mutate_listed_test.go's does: -tests=<IDs, by commas>, and
// with ITOS_CC_TEST_COVERDIR set, GOCOVERDIR=<it>/<ID> for each test's
// binary. With FF_FAIL_ALONE set, the harness fails whenever it runs
// without coverage, as the clean baseline of a selection runs it, once the
// file FF_FAIL_AFTER names exists, if it names one. TestWait, on wait's
// mutant, marks $FF_DIR/started and waits for a release that never comes.
var executionListedFiles = map[string]string{
	"go.mod":  "module example.com/ff\n\ngo 1.22\n",
	"main.go": executionListedMain,
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

// Exact compiler reports observed through the fixture public crap setup.
var executionListedReports = map[string]string{
	"idle,label,quick,spare,wait": `mode: set
example.com/ff/main.go:9.2,12.1 3 0
example.com/ff/main.go:17.2,18.1 1 0
example.com/ff/main.go:23.2,24.1 1 0
example.com/ff/main.go:29.2,30.1 1 1
example.com/ff/main.go:35.2,36.1 1 1
example.com/ff/main.go:41.2,42.1 1 0
example.com/ff/main.go:47.2,48.1 1 1
`,
	"label": `mode: set
example.com/ff/main.go:9.2,12.1 3 0
example.com/ff/main.go:17.2,18.1 1 0
example.com/ff/main.go:22.2,23.1 1 0
example.com/ff/main.go:27.2,28.1 1 1
example.com/ff/main.go:32.2,33.1 1 1
example.com/ff/main.go:38.2,39.1 1 0
example.com/ff/main.go:43.2,44.1 1 1
`,
	"label,quick,shown": `mode: set
example.com/ff/main.go:9.2,12.1 3 0
example.com/ff/main.go:18.2,19.1 1 0
example.com/ff/main.go:23.2,24.1 1 0
example.com/ff/main.go:29.2,30.1 1 1
example.com/ff/main.go:34.2,35.1 1 1
example.com/ff/main.go:40.2,41.1 1 0
example.com/ff/main.go:45.2,46.1 1 1
`,
	"label,quick,shown,wait": `mode: set
example.com/ff/main.go:9.2,12.1 3 0
example.com/ff/main.go:18.2,19.1 1 0
example.com/ff/main.go:23.2,24.1 1 0
example.com/ff/main.go:29.2,30.1 1 1
example.com/ff/main.go:35.2,36.1 1 1
example.com/ff/main.go:41.2,42.1 1 0
example.com/ff/main.go:46.2,47.1 1 1
`,
	"shown": `mode: set
example.com/ff/main.go:9.2,12.1 3 0
example.com/ff/main.go:18.2,19.1 1 0
example.com/ff/main.go:23.2,24.1 1 0
example.com/ff/main.go:28.2,29.1 1 1
example.com/ff/main.go:33.2,34.1 1 1
example.com/ff/main.go:38.2,39.1 1 0
example.com/ff/main.go:43.2,44.1 1 1
`,
}
