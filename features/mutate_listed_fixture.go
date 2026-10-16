// The real listed-test project, harvested from cmd/itos-cc's acceptance tests.
// Its executable harness is fixture data, never an oracle for expected results.
package features

const listedMain = `package main

import (
	"fmt"
	"os"
)

func main() {
	n := len(os.Args) - 1
	fmt.Println(greet(n))
	fmt.Fprintln(os.Stderr, n*2, half(n))
}

// greet tells one from many.
func greet(n int) string {
	if n > 1 {
		return fmt.Sprint(n-1, " more")
	}
	if n == 1 {
		return "one"
	}
	return "none"
}

// half is run by the binary, and checked by the file's own test.
func half(i int) bool {
	return i > 2
}

// twice is never run.
func twice(i int) int {
	return i * 2
}
`

// The lines of listedMain that hold mutants, and the tests that reach them.
const (
	listedArgsLine   = 9  // n := len(os.Args) - 1: both tests; - → +, 1 → 0, killed
	listedStderrLine = 11 // n*2: both tests, never checked; * → /, survives
	listedIfLine     = 16 // n > 1: both tests; > → >=, 1 → 0, killed by ID-A-01
	listedManyLine   = 17 // n-1: ID-A-02 alone; - → +, 1 → 0, killed by it
	listedOneLine    = 19 // n == 1: ID-A-01 alone; == → !=, 1 → 0, killed by it
	listedHalfLine   = 27 // i > 2: both tests and the own test; > → >=, killed by the own test
	listedTwiceLine  = 32 // i * 2: no test; * → /, uncovered
)

// listedMutants is how many mutants listedMain holds.
const listedMutants = 11

var listedFiles = map[string]string{
	"go.mod":  "module example.com/greet\n\ngo 1.22\n",
	"main.go": listedMain,
	"main_test.go": `package main

import (
    "fmt"
    "os"
    "testing"
)

func TestHalf(t *testing.T) {
    if record := os.Getenv("GREET_RECORD"); record != "" {
        f, err := os.OpenFile(record, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
        if err != nil { t.Fatal(err) }
        fmt.Fprintf(f, "own coverdir=%s\n", os.Getenv("ITOS_CC_TEST_COVERDIR"))
        f.Close()
    }
	if !half(3) || half(2) {
		t.Fatal("half")
	}
}
`,
	"e2e/e2e_test.go": `package e2e

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

var selected = flag.String("tests", "", "the IDs of the tests to run, by commas: all of them when empty")

var features = []struct {
	id   string
	args []string
	want string
}{
	{"ID-A-01", []string{"x"}, "one"},
	{"ID-A-02", []string{"x", "y", "z"}, "2 more"},
}

func TestFeatures(t *testing.T) {
	if record := os.Getenv("GREET_RECORD"); record != "" {
		f, err := os.OpenFile(record, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			t.Fatal(err)
		}
		fmt.Fprintf(f, "run tests=%s coverdir=%s\n", *selected, os.Getenv("ITOS_CC_TEST_COVERDIR"))
		f.Close()
	}
	split := os.Getenv("ITOS_CC_TEST_COVERDIR")
	bin := filepath.Join(t.TempDir(), "greet.exe")
	build := []string{"build", "-buildvcs=false", "-o", bin, ".."}
	if split != "" || os.Getenv("GOCOVERDIR") != "" {
		build = []string{"build", "-buildvcs=false", "-cover", "-o", bin, ".."}
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
		if got := strings.TrimSpace(string(out)); got != f.want {
			t.Errorf("%s: greet printed %q, want %q", f.id, got, f.want)
		}
	}
}
`,
	"testdata/list.go": `package main

import (
	"fmt"
	"os"
)

func main() {
	if record := os.Getenv("GREET_RECORD"); record != "" {
		f, err := os.OpenFile(record, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err == nil {
			fmt.Fprintln(f, "list")
			f.Close()
		}
	}
	if len(os.Args) > 1 && os.Args[1] == "fail" {
		fmt.Fprintln(os.Stderr, "list: failing as asked")
		os.Exit(1)
	}
	fmt.Print("ID-A-01\ta.feature\nID-A-02\n")
}
`,
	"itos-cc.yaml": `mutation:
  tests:
    list: go run ./testdata/list.go
    run: go test -count=1 ./e2e -args -tests={pattern}
    ids_pattern: "{ids}"
    join:
      each: "{id}"
      sep: ","
`,
}
