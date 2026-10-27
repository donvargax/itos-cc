package covrun

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/donvargax/itos-cc/internal/lang"
	"github.com/donvargax/itos-cc/internal/project"
)

// The listed tests' Go coverage is converted once-resolved and only for the
// packages the run measures (slice-65, ID-MUT-370 and ID-MUT-371): covdata
// is found once a measure, with "go tool -n covdata", and run directly,
// each conversion limited by -pkg to the measured Go files' packages and
// command-line-arguments. The scenarios run the real covdata through the
// binary; a command's cost is not a thing the CLI can show, so the lookup
// made once is held here, through a recording executor.

// missingCovdata is the covdata a recorder's lookup prints: a path where
// nothing is, so a conversion started directly fails.
var missingCovdata = filepath.Join(os.TempDir(), "itos-cc-no-such-folder", "covdata")

// isCovdataLookup is whether cmd is Go's collector's lookup of covdata.
func isCovdataLookup(cmd *exec.Cmd) bool {
	return slices.Equal(cmd.Args, goCovdata)
}

// printsCovdata has cmd, when it is the lookup of covdata, print path, as
// go tool -n covdata prints where covdata is.
func printsCovdata(cmd *exec.Cmd, path string) {
	if isCovdataLookup(cmd) && cmd.Stdout != nil {
		_, _ = cmd.Stdout.Write([]byte(path + "\n"))
	}
}

// goModule writes a module, example.com/m, at root, with main.go and
// a/a.go, and gives their paths, the sources a run measures.
func goModule(t *testing.T, root string) []string {
	t.Helper()
	writeFiles(t, root, map[string]string{
		"go.mod":  "module example.com/m\n\ngo 1.22\n",
		"main.go": "package main\n\nfunc main() {}\n",
		"a/a.go":  "package a\n",
	})
	return []string{filepath.Join(root, "main.go"), filepath.Join(root, "a", "a.go")}
}

// twoListed is a listed-tests run of tests a and b at root, and a recorder
// whose run of every test writes Go coverage data for both, whose lookup
// prints covdata and whose conversions each write an empty profile.
func twoListed(root, covdata string) (PerTest, *recorder) {
	p := PerTest{Root: root, Tests: []Test{{ID: "a"}, {ID: "b"}}, All: "every test", Select: func([]string) string { return "one test" }}
	return p, &recorder{also: func(cmd *exec.Cmd) {
		printsCovdata(cmd, covdata)
		for _, kv := range cmd.Env {
			if dir, ok := strings.CutPrefix(kv, TestCoverDirEnv+"="); ok {
				for _, id := range []string{"a", "b"} {
					_ = os.MkdirAll(filepath.Join(dir, id), 0o750)
					_ = os.WriteFile(filepath.Join(dir, id, "covmeta.0"), nil, 0o600)
				}
			}
		}
		profiled(cmd)
	}}
}

// lookups and conversions are the commands of rec that looked covdata up
// and that converted with it.
func lookups(rec *recorder) (found int, conversions [][]string) {
	for _, cmd := range rec.commands {
		switch {
		case isCovdataLookup(cmd):
			found++
		case slices.Contains(cmd.Args, "textfmt"):
			conversions = append(conversions, cmd.Args)
		}
	}
	return found, conversions
}

func TestTheSupervisedListedTestsFindCovdataOnceAndConvertOnlyTheMeasuredPackages(t *testing.T) {
	root := t.TempDir()
	sources := goModule(t, root)
	covdata := filepath.Join(root, "bin", "covdata")
	p, rec := twoListed(root, covdata)
	var log bytes.Buffer
	tc, executions, err := MeasureTestsSupervised(context.Background(), p, filepath.Join(root, "data"), sources, &log, rec.execute)
	if err != nil || tc == nil {
		t.Fatalf("listed coverage: %v\n%s", err, log.String())
	}
	found, conversions := lookups(rec)
	if found != 1 || len(conversions) != 2 {
		t.Fatalf("looked covdata up %d times and converted %d times, want once and twice: %q", found, len(conversions), conversions)
	}
	for _, args := range conversions {
		if args[0] != covdata || args[1] != "textfmt" || !slices.Contains(args, "-pkg=command-line-arguments,example.com/m,example.com/m/a") {
			t.Errorf("conversion %q, want %s textfmt limited to command-line-arguments and the measured packages", args, covdata)
		}
	}
	if !slices.ContainsFunc(executions, func(e CommandExecution) bool { return slices.Equal(e.Args, goCovdata) && e.Dir == root }) {
		t.Errorf("the executions %v hold no lookup of covdata", executions)
	}
	inOrder(t, log.String(), `itos-cc: coverage: converted the coverage of 2 listed tests, Go's limited to 2 packages, in [0-9]+\.[0-9]s`)
}

func TestTheListedTestsFindCovdataOnceAndConvertOnlyTheMeasuredPackages(t *testing.T) {
	root := t.TempDir()
	goModule(t, root)
	p, rec := twoListed(root, missingCovdata)
	var log bytes.Buffer
	// covdata is not where the lookup says: each conversion fails, which
	// is logged, and the measure goes on.
	if _, failure := MeasureTests(context.Background(), p, filepath.Join(root, "data"), []string{filepath.Join(root, "a", "a.go")}, &log, rec.execute); failure != nil {
		t.Fatalf("listed coverage: %v\n%s", failure, log.String())
	}
	if found, _ := lookups(rec); found != 1 {
		t.Errorf("looked covdata up %d times, want once", found)
	}
	for _, id := range []string{"a", "b"} {
		if !strings.Contains(log.String(), "itos-cc: coverage: "+id+": "+missingCovdata+" textfmt ") || !strings.Contains(log.String(), "-pkg=command-line-arguments,example.com/m/a") {
			t.Errorf("the log says no conversion of %s by the covdata found, limited to example.com/m/a:\n%s", id, log.String())
		}
	}
	inOrder(t, log.String(), `itos-cc: coverage: converted the coverage of 2 listed tests, Go's limited to 1 package, in [0-9]+\.[0-9]s`)
}

func TestASupervisedLookupOfCovdataThatFailsFailsTheConversion(t *testing.T) {
	for _, c := range []struct {
		name  string
		print string
		fails error
		want  string
	}{
		{"its command fails", "", errors.New("go: no such tool"), "find covdata: go tool -n covdata: go: no such tool"},
		{"it prints no path", " \n", nil, "find covdata: go tool -n covdata: it printed no path"},
	} {
		t.Run(c.name, func(t *testing.T) {
			root := t.TempDir()
			sources := goModule(t, root)
			p, rec := twoListed(root, c.print)
			execute := func(ctx context.Context, cmd *exec.Cmd) error {
				_ = rec.execute(ctx, cmd)
				if isCovdataLookup(cmd) {
					return c.fails
				}
				return nil
			}
			_, executions, err := MeasureTestsSupervised(context.Background(), p, filepath.Join(root, "data"), sources, &bytes.Buffer{}, execute)
			if err == nil || !strings.Contains(err.Error(), "convert listed test a coverage: "+c.want) {
				t.Fatalf("a failed lookup: %v, want %q", err, c.want)
			}
			if last := executions[len(executions)-1]; !slices.Equal(last.Args, goCovdata) || !errors.Is(last.Err, c.fails) {
				t.Errorf("the last execution is %v, want the lookup and its error", last)
			}
		})
	}
}

func TestALookupOfCovdataThatFailsIsLoggedForEachTestAndMadeOnce(t *testing.T) {
	root := t.TempDir()
	goModule(t, root)
	p, rec := twoListed(root, "")
	var log bytes.Buffer
	if _, failure := MeasureTests(context.Background(), p, filepath.Join(root, "data"), []string{filepath.Join(root, "a", "a.go")}, &log, rec.execute); failure != nil {
		t.Fatalf("listed coverage: %v\n%s", failure, log.String())
	}
	if found, conversions := lookups(rec); found != 1 || len(conversions) != 0 {
		t.Errorf("looked covdata up %d times and converted %d times, want once and never", found, len(conversions))
	}
	for _, id := range []string{"a", "b"} {
		if !strings.Contains(log.String(), "itos-cc: coverage: "+id+": find covdata: go tool -n covdata: it printed no path") {
			t.Errorf("the log says nothing of %s's failed lookup:\n%s", id, log.String())
		}
	}
}

func TestGoConversionsAreLimitedToTheMeasuredPackagesUnlessOneIsUnknown(t *testing.T) {
	root := t.TempDir()
	sources := goModule(t, root)
	writeFiles(t, root, map[string]string{"sub/go.mod": "module example.com/sub\n", "sub/c/c.go": "package c\n", "web/label.ts": "export {}\n"})
	loose := t.TempDir()
	writeFiles(t, loose, map[string]string{"x/x.go": "package x\n"})
	unknown := lang.GoModulePackage(t.Context(), project.Surroundings{}, filepath.Join(loose, "x")) == ""
	for _, c := range []struct {
		name    string
		sources []string
		pkg     string // the -pkg argument, "" for none
		none    bool   // no conversion at all
	}{
		{"a module's packages, its main package by the module's path", sources, "-pkg=command-line-arguments,example.com/m,example.com/m/a", false},
		{"a nested module's package", []string{filepath.Join(root, "sub", "c", "c.go")}, "-pkg=command-line-arguments,example.com/sub/c", false},
		{"a Go file with no go.mod above it", append([]string{filepath.Join(loose, "x", "x.go")}, sources...), "", false},
		{"no Go file", []string{filepath.Join(root, "web", "label.ts")}, "", true},
	} {
		t.Run(c.name, func(t *testing.T) {
			if c.pkg == "" && !c.none && !unknown {
				t.Skip("a go.mod above the test's temporary folder names a module")
			}
			args, profile := goTestCollector(t.Context(), root, c.sources).convert("d", "out")
			if c.none {
				if args != nil || profile != "" {
					t.Errorf("converts %q to %q, want no conversion", args, profile)
				}
				return
			}
			want := []string{"covdata", "textfmt", "-i=d", "-o=out.out"}
			if c.pkg != "" {
				want = append(want, c.pkg)
			}
			if !slices.Equal(args, want) || profile != "out.out" {
				t.Errorf("converts with %q to %q, want %q to out.out", args, profile, want)
			}
		})
	}
}

func TestTheConversionsLineSaysTheLimitInEnglish(t *testing.T) {
	for packages, want := range map[int]string{
		0: "itos-cc: coverage: converted the coverage of 3 listed tests in 1.5s\n",
		1: "itos-cc: coverage: converted the coverage of 3 listed tests, Go's limited to 1 package, in 1.5s\n",
		2: "itos-cc: coverage: converted the coverage of 3 listed tests, Go's limited to 2 packages, in 1.5s\n",
	} {
		var log bytes.Buffer
		listedConverted(&log, 3, packages, 1500000000)
		if log.String() != want {
			t.Errorf("limited to %d packages: %q, want %q", packages, log.String(), want)
		}
	}
}

func TestOnlyACollectorWithAToolLooksItUpAndOnlyGoIsLimited(t *testing.T) {
	args := []string{"python", "-c", "combine"}
	run := func(*exec.Cmd) error { t.Fatal("a collector with no tool looked one up"); return nil }
	if got, err := (tools{}).command(t.Context(), testCollector{language: "python"}, args, &bytes.Buffer{}, run); err != nil || !slices.Equal(got, args) {
		t.Errorf("a collector with no tool converts with %q (%v), want %q", got, err, args)
	}
	if n := limited([]testCollector{{language: "python", packages: []string{"p"}}}); n != 0 {
		t.Errorf("limited without Go's collector = %d, want 0", n)
	}
}
