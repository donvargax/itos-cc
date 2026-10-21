package architecture_test

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// probeSlice is a fixture command, probe, laid out as
// internal/slices/version is: its Command, a CLI with --json, its
// Contract, a Handler, Ports declaring only BuildVersion, a method cmd's
// adapters already answer on disk and in process alike, NewHandler and Run.
const probeSlice = `// Package probe is a fixture command, written as a person adds one.
package probe

import (
	"context"
	"fmt"

	"github.com/donvargax/itos-cc/internal/cli"
)

// Command is the probe command as cmd assembles it.
var Command = cli.Declaration{
	Name: "probe",
	Help: "probe the assembly of a new command",
	New:  func() any { return &CLI{} },
}

// CLI is the probe command's command line.
type CLI struct {
	JSON bool ` + "`" + `name:"json" help:"print one JSON object on stdout instead of text (shape below)"` + "`" + `
}

// Contract is what probe's help says beyond its flags.
func (CLI) Contract() cli.Contract {
	return cli.Contract{
		About: "Prints \"probe <version>\", the version the binary says it is.",
		JSON:  ` + "`" + `"probed"` + "`" + `,
		Exits: []cli.Exit{
			{Code: 0, Meaning: "success"},
			{Code: cli.CodeUsage, Meaning: "a usage error: a bad flag or an argument"},
		},
		Examples: []string{"itos-cc probe", "itos-cc probe --json"},
	}
}

// Handler is the probe command's application: the version it probed.
type Handler struct {
	Probed string
}

// Ports are what the probe command reaches the world through.
type Ports interface {
	BuildVersion() string
}

// NewHandler is the handler over p's version.
func NewHandler(_ context.Context, p Ports) (Handler, error) {
	return Handler{Probed: p.BuildVersion()}, nil
}

// printed is the object --json prints beside schema and ok.
type printed struct {
	Probed string ` + "`" + `json:"probed"` + "`" + `
}

// Run prints the version h probed.
func (c CLI) Run(ui *cli.UI, h Handler) int {
	if !c.JSON {
		_, _ = fmt.Fprintln(ui.Stdout, "probe", h.Probed)
	}
	return ui.Emit(c.JSON, printed(h), nil)
}
`

// probeFeature is the fixture command's feature file: one live scenario,
// in sentences features/ defines already.
const probeFeature = `Feature: Probe
  probe prints the version the binary says it is, a fixture command.

  @ID-PROBE-01
  Scenario: probe prints one line
    When I run "itos-cc probe"
    Then it exits with code 0
    And its standard output is one line
`

// probeSteps declares probe.feature's steps, none of its own.
const probeSteps = `package features

import "github.com/cucumber/godog"

// probeFeature declares probe.feature's steps, probeSteps, which
// tools/bin/assemble lists in steps_gen.go.
var probeFeature = featureFile{Name: "probe.feature", Steps: probeSteps}

// probeSteps are probe.feature's own steps: none, its sentences being the
// world's.
func probeSteps(*godog.ScenarioContext, *world) {}
`

// probeSmoke is probe.feature's entry in the smoke set.
const probeSmoke = `- file: probe.feature
  scenarios:
    - id: "@ID-PROBE-01"
      why: the fixture command run, exit 0 and one line
`

// An ordinary new command needs no hand edit outside its slice, its
// feature file and steps, the smoke set and the files tools/bin/assemble
// writes (ADR-0051, T-169): in a copy of the module, probe is added as a
// person would, and it assembles, builds, runs, passes its scenario in
// process, and passes the sibling, outside and lint rules with no rule
// edit. A port no adapters method answers is assembly's edit, which go
// build refuses, naming it.
func TestANewCommandTouchesOnlyItsSliceAndTheGeneratedFiles(t *testing.T) {
	root := os.Getenv("ARCHITECTURE_ROOT")
	linter := os.Getenv("ARCHITECTURE_LINTER")
	if root == "" || linter == "" {
		t.Fatal("run tools/bin/architecture-check")
	}
	dir := t.TempDir()
	copied := filepath.Join(dir, "module")
	bin := filepath.Join(dir, "bin")
	scratch := filepath.Join(dir, "tmp")
	for _, d := range []string{bin, scratch} {
		if err := os.Mkdir(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	// Every temporary file the go command, the tests and the linter make
	// is under dir; the build cache is shared, so the test stays fast.
	env := append(os.Environ(), "GOWORK=off", "GOPROXY=off", "GOTMPDIR="+scratch, "TMPDIR="+scratch, "TMP="+scratch, "TEMP="+scratch)
	tracked := copyTracked(t, root, copied)
	added := map[string]string{
		"internal/slices/probe/probe.go": probeSlice,
		"features/probe.feature":         probeFeature,
		"features/probe.go":              probeSteps,
	}
	for path, text := range added {
		writeFile(t, filepath.Join(copied, filepath.FromSlash(path)), []byte(text))
	}
	smoke := filepath.Join(copied, "features", "smoke.yaml")
	listed, err := os.ReadFile(smoke)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, smoke, append(listed, probeSmoke...))
	run := func(name string, args ...string) (string, error) {
		t.Helper()
		cmd := exec.Command(name, args...)
		cmd.Dir = copied
		cmd.Env = env
		output, err := cmd.CombinedOutput()
		return string(output), err
	}
	for _, args := range [][]string{{"run", "./tools/bin/assemble"}, {"run", "./tools/bin/assemble", "-check"}} {
		if output, err := run("go", args...); err != nil {
			t.Fatalf("go %s: %v\n%s", strings.Join(args, " "), err, output)
		}
	}
	generated := []string{"cmd/itos-cc/commands_gen.go", "features/steps_gen.go"}
	changed := changedFiles(t, root, copied, tracked)
	want := []string{"features/smoke.yaml"}
	for path := range added {
		want = append(want, path)
	}
	want = append(want, generated...)
	slices.Sort(want)
	if !slices.Equal(changed, want) {
		t.Fatalf("the files a new command changed or added = %q, want %q", changed, want)
	}
	for _, path := range generated {
		addsOnlyProbe(t, filepath.Join(root, path), filepath.Join(copied, path))
	}
	t.Run("built and run", func(t *testing.T) {
		exe, err := run("go", "env", "GOEXE")
		if err != nil {
			t.Fatalf("go env GOEXE: %v\n%s", err, exe)
		}
		binary := filepath.Join(bin, "itos-cc"+strings.TrimSpace(exe))
		if output, err := run("go", "build", "-o", binary, "./cmd/itos-cc"); err != nil {
			t.Fatalf("go build ./cmd/itos-cc: %v\n%s", err, output)
		}
		helpLists(t, mustRun(t, run, binary, "--help"))
		if output := mustRun(t, run, binary, "probe"); !strings.HasPrefix(output, "probe ") || strings.Count(output, "\n") != 1 {
			t.Errorf("itos-cc probe printed %q, want one line, probe and the version", output)
		}
		page := mustRun(t, run, binary, "help", "probe")
		for _, text := range []string{
			"usage: itos-cc probe [options]\n\nPrints \"probe <version>\", the version the binary says it is.\n",
			`{"schema": 1, "ok": true|false, "probed",`,
			"  0   success\n  80  a usage error: a bad flag or an argument\n",
			"  itos-cc probe\n  itos-cc probe --json\n",
		} {
			if !strings.Contains(page, text) {
				t.Errorf("itos-cc help probe does not say %q:\n%s", text, page)
			}
		}
	})
	t.Run("scenario in process", func(t *testing.T) {
		output, err := run("go", "test", "./cmd/itos-cc", "-count=1", "-run", "^TestScenarios$", "-scenarios=^@ID-PROBE-01$", "-v")
		if err != nil {
			t.Fatalf("@ID-PROBE-01 failed in process: %v\n%s", err, output)
		}
		if !strings.Contains(output, "--- PASS: TestScenarios/probe_prints_one_line") || strings.Contains(output, "not run here") {
			t.Errorf("@ID-PROBE-01 was not run in process:\n%s", output)
		}
	})
	t.Run("architecture rules", func(t *testing.T) {
		check := filepath.Join(copied, "tools", "bin", "architecture-check")
		for _, args := range [][]string{{"--siblings", "./internal/slices/probe/..."}, {"--outside", "./..."}} {
			if output, err := run("sh", append([]string{check}, args...)...); err != nil {
				t.Errorf("architecture-check %s: %v\n%s", strings.Join(args, " "), err, output)
			}
		}
		lintsClean(t, run, linter, copied, scratch)
	})
	// Last, as it edits the slice: a Ports method no adapters method
	// answers fails the build, naming the method.
	t.Run("a new port", func(t *testing.T) {
		path := filepath.Join(copied, "internal", "slices", "probe", "probe.go")
		writeFile(t, path, []byte(strings.Replace(probeSlice, "\tBuildVersion() string\n", "\tBuildVersion() string\n\tProbeUnanswered() string\n", 1)))
		output, err := run("go", "build", "-o", filepath.Join(bin, "unanswered"), "./cmd/itos-cc")
		if err == nil {
			t.Fatal("go build passed a Ports method no adapters method answers")
		}
		if !strings.Contains(output, "missing method ProbeUnanswered") {
			t.Errorf("go build's failure does not name ProbeUnanswered:\n%s", output)
		}
	})
}

// copyTracked copies every file git ls-files lists at root into dir, as
// it is in root's working tree, and returns their paths, slash-separated.
func copyTracked(t *testing.T, root, dir string) []string {
	t.Helper()
	ls := exec.Command("git", "ls-files", "-z")
	ls.Dir = root
	listed, err := ls.Output()
	if err != nil {
		t.Fatalf("git ls-files: %v", err)
	}
	var paths []string
	for _, path := range strings.Split(strings.TrimSuffix(string(listed), "\x00"), "\x00") {
		from := filepath.Join(root, filepath.FromSlash(path))
		info, err := os.Lstat(from)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		to := filepath.Join(dir, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(to), 0o700); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, path)
		if info.Mode()&fs.ModeSymlink != 0 {
			target, err := os.Readlink(from)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(target, to); err != nil {
				t.Fatal(err)
			}
			continue
		}
		data, err := os.ReadFile(from)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(to, data, info.Mode().Perm()|0o600); err != nil {
			t.Fatal(err)
		}
	}
	return paths
}

// writeFile writes data to path, making the folders above it.
func writeFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

// changedFiles are the files of dir differing from root's, or new, and
// the tracked ones gone from dir, slash-separated and sorted.
func changedFiles(t *testing.T, root, dir string, tracked []string) []string {
	t.Helper()
	var changed []string
	seen := map[string]bool{}
	known := map[string]bool{}
	for _, path := range tracked {
		known[path] = true
	}
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		seen[rel] = true
		if !known[rel] {
			changed = append(changed, rel)
			return nil
		}
		if d.Type()&fs.ModeSymlink != 0 {
			return nil
		}
		now, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		was, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			return err
		}
		if !bytes.Equal(now, was) {
			changed = append(changed, rel)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range tracked {
		if !seen[path] {
			changed = append(changed, path)
		}
	}
	slices.Sort(changed)
	return changed
}

// addsOnlyProbe holds the generated file at now to the one at was: was's
// lines all there, in their order, and every line added naming probe.
func addsOnlyProbe(t *testing.T, was, now string) {
	t.Helper()
	before, after := lines(t, was), lines(t, now)
	i := 0
	var added []string
	for _, line := range after {
		if i < len(before) && line == before[i] {
			i++
			continue
		}
		added = append(added, line)
		if !strings.Contains(line, "probe") {
			t.Errorf("%s adds a line naming no probe: %q", now, line)
		}
	}
	if i < len(before) {
		t.Errorf("%s lost or moved the line %q", now, before[i])
	}
	if len(added) == 0 {
		t.Errorf("%s adds no line for probe", now)
	}
}

// lines are the lines of the file at path.
func lines(t *testing.T, path string) []string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		out = append(out, scanner.Text())
	}
	return out
}

// mustRun is run's output of name and args, which must exit 0.
func mustRun(t *testing.T, run func(string, ...string) (string, error), name string, args ...string) string {
	t.Helper()
	output, err := run(name, args...)
	if err != nil {
		t.Fatalf("%s %s: %v\n%s", filepath.Base(name), strings.Join(args, " "), err, output)
	}
	return output
}

// helpLists holds the top-level help to listing probe by its help line,
// between mutation's commands and scrap.
func helpLists(t *testing.T, help string) {
	t.Helper()
	var commands []string
	for _, line := range strings.Split(help, "\n") {
		if strings.HasPrefix(line, "  ") && !strings.HasPrefix(line, "   ") {
			commands = append(commands, strings.Fields(line)[0])
		}
		if line == "Paths are files, directories, or fragments of a path under the working" {
			break
		}
	}
	at := slices.Index(commands, "probe")
	if at < 1 || at+1 >= len(commands) || commands[at-1] != "mutation" || commands[at+1] != "scrap" {
		t.Errorf("--help does not list probe between mutation and scrap: %q\n%s", commands, help)
	}
	if !strings.Contains(help, "\n  probe           probe the assembly of a new command\n") {
		t.Errorf("--help does not list probe by its help line:\n%s", help)
	}
}

// lintsClean runs the pinned linter with dir's .golangci.yml over the
// probe slice, which must report nothing: depguard holds a new slice with
// no rule edit.
func lintsClean(t *testing.T, run func(string, ...string) (string, error), linter, dir, scratch string) {
	t.Helper()
	outputPath := filepath.Join(scratch, "probe-issues.json")
	output, err := run(linter, "run", "--config", filepath.Join(dir, ".golangci.yml"), "--output.json.path", outputPath, "--path-mode=abs", "--max-issues-per-linter=0", "--max-same-issues=0", "./internal/slices/probe/...")
	if err != nil {
		t.Errorf("the linter refused the probe slice or failed: %v\n%s", err, output)
	}
	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read the linter's JSON: %v\n%s", err, output)
	}
	var report struct {
		Issues []struct {
			FromLinter, Text string
			Pos              struct{ Filename string }
		}
	}
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatal(err)
	}
	for _, issue := range report.Issues {
		t.Errorf("the linter reports %s: %s: %s", issue.Pos.Filename, issue.FromLinter, issue.Text)
	}
}
