// The tests are in the slice's own package: a slice imports no other slice,
// and depguard cannot tell a slice's own import from another's. The
// scenarios of features/crap.feature, coverage.feature, complexity.feature
// and file_selection.feature hold the command through both drivers; these
// hold what Select, Handle and Run do with what the ports give and fail
// with, through porttest's fakes.
package crap

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/donvargax/itos-cc/internal/changerisk"
	"github.com/donvargax/itos-cc/internal/cli"
	"github.com/donvargax/itos-cc/internal/coverage"
	covport "github.com/donvargax/itos-cc/internal/coverage/port"
	covporttest "github.com/donvargax/itos-cc/internal/coverage/port/porttest"
	langporttest "github.com/donvargax/itos-cc/internal/lang/port/porttest"
	"github.com/donvargax/itos-cc/internal/selection/port/porttest"
	snapport "github.com/donvargax/itos-cc/internal/snapshot/port"
	snapporttest "github.com/donvargax/itos-cc/internal/snapshot/port/porttest"
)

// board is a Go source whose place makes one decision, and half none.
const board = "package board\n\nfunc place(a int) int {\n\tif a > 0 {\n\t\treturn 1\n\t}\n\treturn 0\n}\n\nfunc half(a int) int {\n\treturn a / 2\n}\n"

// at is a path of the project at /work, as the system writes it.
func at(rel string) string { return filepath.FromSlash("/work/" + rel) }

// files are the project's files: board.go in src/, its test, and the
// reports a run may name.
var files = map[string]string{
	"src/board.go":      board,
	"src/board_test.go": "package board\n",
	"none.info":         "SF:src/board.go\nDA:4,0\nDA:5,0\nDA:7,0\nDA:11,0\nend_of_record\n",
}

// fromSrc names a file as a person in /work/src reads it, so names and
// crap.json's keys, from /work, differ.
func fromSrc(path string) string {
	rel, err := filepath.Rel(at("src"), path)
	if err != nil {
		return path
	}
	return filepath.ToSlash(rel)
}

// handler is a handler over the project, its coverage measured by m, its
// snapshots kept in store.
func handler(m covport.Measurer[*coverage.Report], store *snapporttest.Store) Handler {
	return Handler{Tree: porttest.TreeOf(at(""), files, nil), Surroundings: langporttest.Surroundings{Dir: "/work/src"},
		Store: store, Coverage: m, Name: fromSrc}
}

// measurer reads the project's reports from /work.
func measurer() covporttest.Measurer {
	held := map[string]string{}
	for name, text := range files {
		held[at(name)] = text
	}
	return covporttest.Measurer{Dir: at(""), Files: held}
}

// run is c run over h, and what it printed on stdout and stderr.
func run(t *testing.T, c CLI, h Handler) (code int, stdout, stderr string) {
	t.Helper()
	var out, errs bytes.Buffer
	code = c.Run(t.Context(), &cli.UI{Stdout: &out, Stderr: &errs, Dir: at("src")}, h)
	return code, out.String(), errs.String()
}

// object is the --json object of a run.
type object struct {
	Entries    []changerisk.Entry
	Unmeasured []map[string]any
	Problems   []map[string]any
}

func objectOf(t *testing.T, stdout string) object {
	t.Helper()
	var o object
	if err := json.Unmarshal([]byte(stdout), &o); err != nil {
		t.Fatalf("stdout is no object: %v\n%s", err, stdout)
	}
	return o
}

func ptr(f float64) *float64 { return &f }

// Each function is named by Name and scored by the report named, worst
// first, and crap.json records every one from the project root.
func TestRunScoresAndRecordsEveryFunction(t *testing.T) {
	store := &snapporttest.Store{RootDir: at("")}
	code, stdout, stderr := run(t, CLI{CoverageReport: []string{"none.info"}}, handler(measurer(), store))
	lines := strings.Split(strings.TrimSuffix(stdout, "\n"), "\n")
	if code != 0 || stderr != "" || len(lines) != 3 || !strings.Contains(lines[1], "6.0") || !strings.HasSuffix(lines[1], "board#place  board.go:3") ||
		!strings.HasSuffix(lines[2], "board#half  board.go:10") {
		t.Errorf("exit %d, stdout\n%s\nstderr %q", code, stdout, stderr)
	}
	var recorded struct {
		Version int
		Entries []changerisk.Entry
	}
	if err := json.Unmarshal([]byte(store.Snapshots["crap.json"]), &recorded); err != nil {
		t.Fatal(err)
	}
	if recorded.Version != 1 || len(recorded.Entries) != 2 || recorded.Entries[0].File != "src/board.go" {
		t.Errorf("crap.json holds %+v, want both functions from the root", recorded)
	}
}

// --top prints the worst only; --no-coverage measures nothing and leaves
// every function unscored.
func TestRunPrintsTheWorstAndMeasuresNothingWithoutCoverage(t *testing.T) {
	_, stdout, _ := run(t, CLI{CoverageReport: []string{"none.info"}, Top: 1, JSON: true}, handler(measurer(), &snapporttest.Store{RootDir: at("")}))
	if o := objectOf(t, stdout); len(o.Entries) != 1 || o.Entries[0].Name != "place" {
		t.Errorf("--top 1 printed %+v, want place alone", o.Entries)
	}
	failing := covporttest.Measurer{}
	_, stdout, _ = run(t, CLI{NoCoverage: true, JSON: true}, handler(failing, &snapporttest.Store{RootDir: at("")}))
	if o := objectOf(t, stdout); len(o.Entries) != 2 || o.Entries[0].CRAP != nil {
		t.Errorf("--no-coverage printed %+v, want both unscored", o.Entries)
	}
}

// A threshold of 0 is a threshold: every scored function is above it.
// Without one, a report not read is a warning; with one, it is a problem,
// which the UI's classifiers word.
func TestTheThresholdJudgesEveryScoreAndEveryGap(t *testing.T) {
	c := CLI{CoverageReport: []string{"none.info", "absent.info"}, Threshold: ptr(0), JSON: true}
	code, stdout, stderr := run(t, c, handler(measurer(), &snapporttest.Store{RootDir: at("")}))
	o := objectOf(t, stdout)
	var rules []string
	for _, p := range o.Problems {
		rules = append(rules, p["rule"].(string))
	}
	if code != cli.CodeInternal || strings.Join(rules, " ") != "internal crap.threshold crap.threshold" || len(o.Unmeasured) != 1 {
		t.Errorf("exit %d, rules %q, object %+v\nstderr %s", code, rules, o, stderr)
	}
	c.Threshold = nil
	code, _, stderr = run(t, c, handler(measurer(), &snapporttest.Store{RootDir: at("")}))
	if code != 0 || !strings.HasSuffix(stderr, "itos-cc: no coverage for absent.info: open absent.info: file does not exist\n") {
		t.Errorf("without a threshold: exit %d, stderr %q", code, stderr)
	}
}

// Nothing selected says so before a command with no report is refused;
// sources selected and a command with no report is a usage problem.
func TestNoSourceComesBeforeACommandWithNoReport(t *testing.T) {
	c := CLI{Selection: cli.Selection{Paths: []string{"board_test"}}, CoverageCommand: "x"}
	code, _, stderr := run(t, c, handler(measurer(), &snapporttest.Store{}))
	if code != 0 || stderr != "itos-cc: no source files to score\n" {
		t.Errorf("no source: exit %d, stderr %q", code, stderr)
	}
	c.Paths = nil
	code, _, stderr = run(t, c, handler(measurer(), &snapporttest.Store{}))
	if code != cli.CodeUsage || !strings.Contains(stderr, "--coverage-command needs --coverage-report") {
		t.Errorf("a command with no report: exit %d, stderr %q", code, stderr)
	}
}

// A measurement, a snapshot or a selection that fails stops the run, with
// the empty object.
func TestAFailureStopsTheRun(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	h := handler(measurer(), &snapporttest.Store{RootDir: at("")})
	if _, err := h.Handle(ctx, Query{Measure: true}, []string{at("src/board.go")}); err != (covport.Cancelled{Cause: context.Canceled}) {
		t.Errorf("a done context: %v, want the coverage port's Cancelled", err)
	}
	full := &snapport.Unwritable{Name: "crap.json", Reason: "no space left on device"}
	_, err := handler(measurer(), &snapporttest.Store{RootDir: at(""), Failures: map[string]snapport.Failure{"crap.json": full}}).
		Handle(t.Context(), Query{}, []string{at("src/board.go")})
	if !errors.Is(err, full) {
		t.Errorf("an unwritable snapshot: %v", err)
	}
	code, stdout, _ := run(t, CLI{JSON: true}, Handler{Tree: porttest.Tree{Dir: at("")}, Coverage: measurer(), Name: fromSrc,
		Store: &snapporttest.Store{RootDir: at("")}, Surroundings: langporttest.Surroundings{}})
	if o := objectOf(t, stdout); code != 0 || len(o.Entries) != 0 {
		t.Errorf("an empty tree: exit %d, %+v", code, o)
	}
}
