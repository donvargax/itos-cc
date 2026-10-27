package covrun

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/donvargax/itos-cc/internal/coverage"
)

// Go coverage under a project path holding a comma (bug-13, ID-MUT-392):
// go tool covdata reads -i as a comma-separated list, so each conversion
// runs in the parent of the folder it reads, naming it by its bare name.
// These convert real data, from a binary built with -cover at a folder
// named "with,comma", with the real covdata.

// commaMain is the module's main.go: run with no argument, it reaches line
// 13 and never line 10.
const commaMain = `package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) > 1 {
		fmt.Println("more")
		return
	}
	fmt.Println("one")
}
`

// commaModule writes the module example.com/c at a folder named
// "with,comma" and builds its binary with -cover there, giving the folder
// and the binary.
func commaModule(t *testing.T) (root, bin string) {
	t.Helper()
	root = filepath.Join(t.TempDir(), "with,comma")
	writeFiles(t, root, map[string]string{"go.mod": "module example.com/c\n\ngo 1.22\n", "main.go": commaMain})
	bin = filepath.Join(root, "app")
	if strings.HasSuffix(os.Args[0], ".exe") {
		bin += ".exe"
	}
	build := exec.Command("go", "build", "-cover", "-o", bin, ".")
	build.Dir = root
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build -cover: %v\n%s", err, out)
	}
	return root, bin
}

// runCovered runs bin with no argument, writing its coverage to dir.
func runCovered(t *testing.T, bin, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(bin)
	cmd.Env = append(os.Environ(), "GOCOVERDIR="+dir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s: %v\n%s", bin, err, out)
	}
}

func TestGoIntegrationCoverageUnderAPathHoldingACommaIsConverted(t *testing.T) {
	root, bin := commaModule(t)
	saved := Executable
	Executable = "itos-cc"
	t.Cleanup(func() { Executable = saved })
	for name, integrate := range map[string]func(Plan, *bytes.Buffer) (string, error){
		"unsupervised": func(p Plan, log *bytes.Buffer) (string, error) {
			report, _ := p.integrate(log)
			return report, nil
		},
		"supervised": func(p Plan, log *bytes.Buffer) (string, error) {
			report, _, _, err := p.integrateSupervised(t.Context(), log, func(_ context.Context, cmd *exec.Cmd) error { return cmd.Run() })
			return report, err
		},
	} {
		t.Run(name, func(t *testing.T) {
			p := goPlanWithScope(root, filepath.Join(root, ".metrics", name), nil, false, nil, nil)
			runCovered(t, bin, p.CoverDir)
			var log bytes.Buffer
			report, err := integrate(p, &log)
			if err != nil || report == "" {
				t.Fatalf("converted to %q (%v), want the integration profile\n%s", report, err, log.String())
			}
			entries, err := Load(report)
			if err != nil || !slices.ContainsFunc(entries, func(e coverage.Entry) bool { return strings.HasSuffix(e.Path, "main.go") }) {
				t.Errorf("the profile holds %v (%v), want main.go's lines\n%s", entries, err, log.String())
			}
		})
	}
}

func TestGoListedCoverageUnderAPathHoldingACommaIsConverted(t *testing.T) {
	root, bin := commaModule(t)
	file := filepath.Join(root, "main.go")
	for _, c := range []struct {
		name  string
		id    string
		split bool // the harness writes each test's data to <dir>/<id>
	}{
		{"each test alone", "ID-A-01", false},
		{"the harness splitting the run, under an ID holding a comma", "ID-A,01", true},
	} {
		harness := func(_ context.Context, cmd *exec.Cmd) error {
			line := cmd.Args[len(cmd.Args)-1]
			if line != "every test" && line != "one test" {
				return cmd.Run()
			}
			for _, kv := range cmd.Env {
				if dir, ok := strings.CutPrefix(kv, TestCoverDirEnv+"="); ok && c.split {
					runCovered(t, bin, filepath.Join(dir, c.id))
				}
				if dir, ok := strings.CutPrefix(kv, "GOCOVERDIR="); ok && line == "one test" {
					runCovered(t, bin, dir)
				}
			}
			return nil
		}
		p := PerTest{Root: root, Tests: []Test{{ID: c.id}}, All: "every test", Select: func([]string) string { return "one test" }}
		for _, supervised := range []bool{false, true} {
			t.Run(c.name+map[bool]string{false: "", true: ", supervised"}[supervised], func(t *testing.T) {
				dir := filepath.Join(root, ".metrics", "listed")
				var log bytes.Buffer
				var tc *coverage.TestCoverage
				var err any
				if supervised {
					tc, _, err = MeasureTestsSupervised(t.Context(), p, dir, []string{file}, &log, harness)
				} else {
					tc, err = MeasureTests(t.Context(), p, dir, []string{file}, &log, harness)
				}
				if err != nil || tc == nil || tc.Of(c.id) == nil {
					t.Fatalf("measured %v (%v), want %s's coverage\n%s", tc, err, c.id, log.String())
				}
				if covered, _ := tc.Of(c.id).LineCovered(file, 13); !covered {
					t.Errorf("line 13 is not covered by %s, want it covered: the binary ran it\n%s", c.id, log.String())
				}
			})
		}
	}
}

func TestAListedTestsFolderWhoseNameHoldsACommaIsMovedToOneNamedByItsIndex(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{"ID-A,01/covmeta.1": "meta", "ID-A,01/sub/covmeta.2": "another test's", "ID-B/covmeta.3": "kept"})
	moved, err := splitFolder(filepath.Join(dir, "ID-A,01"), dir, 4)
	if want := filepath.Join(dir, ".split", "4"); err != nil || moved != want {
		t.Fatalf("moved to %q (%v), want %q", moved, err, want)
	}
	if text, err := os.ReadFile(filepath.Join(moved, "covmeta.1")); err != nil || string(text) != "meta" {
		t.Errorf("the moved folder holds %q (%v), want the test's covmeta.1", text, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "ID-A,01", "sub", "covmeta.2")); err != nil {
		t.Errorf("another test's folder within it moved: %v", err)
	}
	if _, err := os.Stat(filepath.Join(moved, "sub")); !os.IsNotExist(err) {
		t.Errorf("the folder within it was moved too: %v", err)
	}
	if kept, err := splitFolder(filepath.Join(dir, "ID-B"), dir, 5); err != nil || kept != filepath.Join(dir, "ID-B") {
		t.Errorf("a name without a comma gave %q (%v), want it kept", kept, err)
	}
}

func TestAListedTestsFolderThatCannotBeMovedIsAnError(t *testing.T) {
	for name, files := range map[string]map[string]string{
		"no folder for the moved data":    {"ID-A,01/covmeta.1": "", ".split": "a file"},
		"no folder to move":               {},
		"a moved file that cannot be put": {"ID-A,01/covmeta.1": "", ".split/0/covmeta.1/x": ""},
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			writeFiles(t, dir, files)
			if moved, err := splitFolder(filepath.Join(dir, "ID-A,01"), dir, 0); err == nil {
				t.Errorf("moved to %q, want an error", moved)
			}
		})
	}
}

func TestAListedTestWhoseFolderCannotBeMovedIsLoggedOrFailsTheSupervisedMeasure(t *testing.T) {
	root, bin := commaModule(t)
	file := filepath.Join(root, "main.go")
	// The harness splits the run under an ID holding a comma, and where
	// the moved data would go is a file.
	harness := func(_ context.Context, cmd *exec.Cmd) error {
		switch cmd.Args[len(cmd.Args)-1] {
		case "one test":
			return nil
		case "every test":
		default:
			return cmd.Run()
		}
		for _, kv := range cmd.Env {
			if dir, ok := strings.CutPrefix(kv, TestCoverDirEnv+"="); ok {
				runCovered(t, bin, filepath.Join(dir, "ID-A,01"))
				return os.WriteFile(filepath.Join(dir, ".split"), nil, 0o600)
			}
		}
		return nil
	}
	p := PerTest{Root: root, Tests: []Test{{ID: "ID-A,01"}}, All: "every test", Select: func([]string) string { return "one test" }}
	dir := filepath.Join(root, ".metrics", "listed")
	var log bytes.Buffer
	tc, failed := MeasureTests(t.Context(), p, dir, []string{file}, &log, harness)
	if failed != nil || tc == nil || tc.Of("ID-A,01") != nil || !strings.Contains(log.String(), "itos-cc: coverage: ID-A,01: ") {
		t.Errorf("measured %v (%v), want ID-A,01 left out and logged\n%s", tc, failed, log.String())
	}
	_, _, err := MeasureTestsSupervised(t.Context(), p, dir, []string{file}, &log, harness)
	if err == nil || !strings.Contains(err.Error(), "prepare coverage for listed test ID-A,01") {
		t.Errorf("the supervised measure gave %v, want its failure to move ID-A,01's data", err)
	}
}

func TestOnlyGoConvertsAListedTestsDataFromTheFoldersParent(t *testing.T) {
	root := t.TempDir()
	d := filepath.Join(root, ".metrics", "listed", "ID-A-01")
	if got := goTestCollector(t.Context(), root, nil).convertDir(d); got != filepath.Dir(d) {
		t.Errorf("Go converts in %q, want the folder's parent %q", got, filepath.Dir(d))
	}
	if got := (testCollector{language: "python", dir: root}).convertDir(d); got != root {
		t.Errorf("Python converts in %q, want its own folder %q", got, root)
	}
}
