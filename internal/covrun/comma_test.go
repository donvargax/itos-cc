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
