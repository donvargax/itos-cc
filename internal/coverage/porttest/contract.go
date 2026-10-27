package porttest

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/donvargax/itos-cc/internal/coverage"
	"github.com/donvargax/itos-cc/internal/coverage/port"
	aggregate "github.com/donvargax/itos-cc/internal/coverage/port/porttest"
)

// MutationMeasurerContract holds the listed entry point to the same named-report
// formats, matching, merging, gaps and cancellation as aggregate measurement.
// The deliberately invalid listed commands must never run for named reports.
// Physical collectors and their split/fallback protocols remain adapter guards.
func MutationMeasurerContract(t *testing.T, open func(*testing.T, map[string]string) (coverage.MutationMeasurer, string)) {
	t.Helper()
	aggregate.MeasurerContract(t, func(t *testing.T, files map[string]string) (coverage.Measurer, string) {
		m, dir := open(t, files)
		return listedEntry{m, dir}, dir
	})
	t.Run("inventory stops on the actual caller cancellation", func(t *testing.T) {
		m, _ := open(t, map[string]string{})
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		if failure := m.Inventory(ctx, &coverage.Report{}, "python"); failure == nil {
			t.Fatal("cancelled inventory succeeded")
		}
	})
}

type listedEntry struct {
	coverage.MutationMeasurer
	root string
}

func (m listedEntry) Measure(ctx context.Context, req port.Request) (*coverage.Report, port.Failure) {
	return m.MeasureListed(ctx, req, &coverage.PerTest{Root: m.root, Tests: []coverage.Test{{ID: "not-run"}}, All: "T90-named-report-must-not-start-this-command", Select: func([]string) string { return "T90-no-selector" }})
}

// ListedContract exercises built-in collection using independent fixture
// artifacts in memory and real covered Go processes at the adapter. split
// controls whether the actual harness supplies per-ID data in one run, and
// fail names the harness's run that fails, "" for none: "all", the run of
// every test, or a test's ID, its run alone. A run that fails is
// TestsFailed, naming its command and exit code, and gives no report.
func ListedContract(t *testing.T, open func(t *testing.T, split bool, fail string) (coverage.MutationMeasurer, string)) {
	tests := func(root string) *coverage.PerTest {
		return &coverage.PerTest{Root: root, Tests: []coverage.Test{{ID: "A"}, {ID: "B"}, {ID: "C"}}, All: "go run ./harness all", Select: func(ids []string) string { return "go run ./harness " + ids[0] }}
	}
	for _, split := range []bool{true, false} {
		name := "fallback"
		if split {
			name = "split"
		}
		t.Run(name, func(t *testing.T) {
			m, root := open(t, split, "")
			file := filepath.Join(root, "main.go")
			r, failure := m.MeasureListed(t.Context(), port.Request{Sources: []string{file}, Scope: port.OwnTests}, tests(root))
			if failure != nil {
				t.Fatal(failure)
			}
			if !reflect.DeepEqual(r.LineTests(file, 5), []string{"A"}) || !reflect.DeepEqual(r.LineTests(file, 7), []string{"B"}) {
				t.Fatalf("actual per-ID coverage: A=%v B=%v", r.LineTests(file, 5), r.LineTests(file, 7))
			}
			if len(r.LineTests(file, 1)) != 0 {
				t.Fatal("absent/nonexecuted line acquired an ID")
			}
		})
	}
	t.Run("a served source runs no plan and the listed tests measure it", func(t *testing.T) {
		m, root := open(t, true, "")
		file := filepath.Join(root, "main.go")
		req := port.Request{Sources: []string{file}, Scope: port.OwnTests}
		if plans := m.Plans(t.Context(), req); !reflect.DeepEqual(plans, []coverage.Plan{{Language: "go", Dir: root, Sources: []string{file}}}) {
			t.Fatalf("plans %+v, want the module's one", plans)
		}
		if plans := m.Plans(t.Context(), port.Request{Sources: []string{file}, Reports: []string{"c.out"}}); plans != nil {
			t.Fatalf("a request of reports has plans %+v", plans)
		}
		req.Served = []string{file}
		r, failure := m.MeasureListed(t.Context(), req, tests(root))
		if failure != nil {
			t.Fatal(failure)
		}
		if r.Has(file) || r.Measures("go") {
			t.Fatal("a plan measured the served source")
		}
		if !reflect.DeepEqual(r.LineTests(file, 5), []string{"A"}) {
			t.Fatalf("the listed tests' coverage of the served source: A=%v", r.LineTests(file, 5))
		}
	})
	for _, c := range []struct {
		name, fail, command string
		split               bool
	}{
		{name: "split run of every test fails", fail: "all", command: "go run ./harness all", split: true},
		{name: "fallback run of one test fails", fail: "B", command: "go run ./harness B"},
	} {
		t.Run(c.name, func(t *testing.T) {
			m, root := open(t, c.split, c.fail)
			r, failure := m.MeasureListed(t.Context(), port.Request{Sources: []string{filepath.Join(root, "main.go")}, Scope: port.OwnTests}, tests(root))
			failed, ok := failure.(port.TestsFailed)
			if !ok || r != nil {
				t.Fatalf("failed listed run: report %v, failure %T %v; want TestsFailed alone", r, failure, failure)
			}
			if failed.Command != c.command || failed.ExitCode != 1 || failed.Reason == "" {
				t.Fatalf("failed listed run = %+v; want %q, exit code 1 and a reason", failed, c.command)
			}
		})
	}
}

// StaticInventoryContract proves only actual unloaded Python files, including
// an explicitly empty inventory, without adding hits or touching measured peers.
func StaticInventoryContract(t *testing.T, open func(*testing.T) (coverage.MutationMeasurer, string)) {
	m, root := open(t)
	hit, miss, empty := filepath.Join(root, "hit.py"), filepath.Join(root, "miss.py"), filepath.Join(root, "empty.py")
	r := coverage.FromLines("python", map[string][]coverage.Line{hit: {{Line: 1, Covered: true}}})
	r.Prove("python", root, []string{hit, miss, empty})
	if failure := m.Inventory(t.Context(), r, "python"); failure != nil {
		t.Fatal(failure)
	}
	lines, ok := r.Lines(miss)
	if !ok || len(lines) != 2 || lines[0].Line != 1 || lines[1].Line != 2 || lines[0].Covered || lines[1].Covered {
		t.Fatalf("static inventory: %v %v", lines, ok)
	}
	lines, ok = r.Lines(empty)
	if !ok || len(lines) != 0 {
		t.Fatalf("empty inventory: %v %v", lines, ok)
	}
	if hit, ok := r.LineCovered(hit, 1); !hit || !ok {
		t.Fatal("measured sibling changed")
	}
}

// ListedMain is an executable fixture with two independently observable arms.
const ListedMain = `package main
import "os"
func main() {
 if os.Args[1]=="A" {
  println("A")
 } else {
  println("B")
 }
}
`

// ListedHarness runs real covered processes. Split is an independently configured
// fixture choice; it never supplies a fabricated profile or uses a desired verdict.
const ListedHarness = `package main
import("fmt";"os";"os/exec";"path/filepath")
func main(){
 id:=os.Args[1]
 if os.Getenv("T90_FAIL")==id{os.Exit(1)}
 split:=os.Getenv("T90_SPLIT")=="true"
 whole:=os.Getenv("ITOS_CC_TEST_COVERDIR")
 if whole!="" && !split{return}
 ids:=[]string{id};if id=="all"{ids=[]string{"A","B","C"}}
 for _,id:=range ids{
  if id=="C"{continue}
  dir:=os.Getenv("GOCOVERDIR");if whole!=""{dir=filepath.Join(whole,id)}
  if dir!=""{if err:=os.MkdirAll(dir,0700);err!=nil{panic(err)}}
  cmd:=exec.Command("./app.exe",id)
  cmd.Env=append(os.Environ(),"GOCOVERDIR="+dir)
  if out,err:=cmd.CombinedOutput();err!=nil{panic(fmt.Sprintf("%s: %v",out,err))}
 }
}
`
