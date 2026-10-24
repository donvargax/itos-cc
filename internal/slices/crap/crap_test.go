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

	"github.com/donvargax/itos-cc/internal/cli"
	"github.com/donvargax/itos-cc/internal/coverage"
	covport "github.com/donvargax/itos-cc/internal/coverage/port"
	covporttest "github.com/donvargax/itos-cc/internal/coverage/port/porttest"
	langport "github.com/donvargax/itos-cc/internal/lang/port"
	langporttest "github.com/donvargax/itos-cc/internal/lang/port/porttest"
	"github.com/donvargax/itos-cc/internal/selection"
	"github.com/donvargax/itos-cc/internal/selection/port"
	"github.com/donvargax/itos-cc/internal/selection/port/porttest"
	"github.com/donvargax/itos-cc/internal/slices/crap/domain"
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
func handler(m coverage.Measurer, store *snapporttest.Store) Handler {
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

// ports are the Ports cmd's adapters give the handler, held in memory: the
// project's tree, the store, the coverage measured by measurer, files named
// from /work/src, and the exclusion the settings give or the failure
// reading them meets.
type ports struct {
	store   *snapporttest.Store
	exclude selection.Exclusion
	failure error
}

func (p ports) SelectionTree() port.Tree { return porttest.TreeOf(at(""), files, nil) }
func (p ports) LangSurroundings() langport.Surroundings {
	return langporttest.Surroundings{Dir: "/work/src"}
}
func (p ports) SnapshotStore() snapport.Store       { return p.store }
func (p ports) CoverageMeasurer() coverage.Measurer { return measurer() }
func (p ports) Rel(path string) string              { return fromSrc(path) }
func (p ports) SelectionExclusion(context.Context) (selection.Exclusion, error) {
	return p.exclude, p.failure
}

// NewHandler builds the handler over its Ports: the sources their tree
// holds, less those their settings exclude, measured by their Measurer,
// named by their Rel and recorded in their store. A failure reading the
// settings is returned as the Ports gave it, never wrapped, so cmd's
// config.invalid is the problem the person reads.
func TestNewHandlerIsBuiltOverItsPorts(t *testing.T) {
	store := &snapporttest.Store{RootDir: at("")}
	h, err := NewHandler(t.Context(), ports{store: store})
	if err != nil {
		t.Fatal(err)
	}
	q := CLI{CoverageReport: []string{"none.info"}}.query()
	chosen, err := h.Select(t.Context(), q)
	if err != nil {
		t.Fatal(err)
	}
	found, err := h.Handle(t.Context(), q, chosen.Sources)
	if err != nil || len(found.Entries) != 2 || found.Entries[0].File != "board.go" || found.Entries[0].CRAP == nil || store.Snapshots["crap.json"] == "" {
		t.Errorf("Handle() = %+v, %v, recorded %v; want board.go's functions scored, named by Rel, in the store", found, err, store.Snapshots)
	}
	h, err = NewHandler(t.Context(), ports{store: store, exclude: selection.Exclusion{Root: at(""), Exclude: []string{"**/board.go"}}})
	if err != nil {
		t.Fatal(err)
	}
	if chosen, err := h.Select(t.Context(), q); err != nil || len(chosen.Sources) != 0 || chosen.Excluded != 1 {
		t.Errorf("Select() = %+v, %v; want board.go left out by the Ports' exclusion", chosen, err)
	}
	failure := cli.NewProblem(cli.CodeUsage, "config.invalid", "itos-cc.yaml: cannot be read", "Fix it.")
	if _, err := NewHandler(t.Context(), ports{store: store, failure: failure}); err != failure {
		t.Errorf("a settings failure: %v, want the Ports' own %v", err, failure)
	}
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
	Entries    []domain.Entry
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
		Entries []domain.Entry
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

// failedMeasurer is the project's measurer whose every report also holds
// failed: build roots whose coverage command failed after writing it.
type failedMeasurer struct {
	covporttest.Measurer
	failed []coverage.Unmeasured
}

func (m failedMeasurer) Measure(ctx context.Context, req covport.Request) (*coverage.Report, covport.Failure) {
	r, failure := m.Measurer.Measure(ctx, req)
	if failure == nil {
		r.SetFailed(m.failed...)
	}
	return r, failure
}

// A build root whose coverage command failed after writing its report is
// read as measured, and warned of without a threshold only when a Go
// package's tests timed out in it, naming the package (slice-47); another
// failure stays silent, as before, and so does a run with a threshold.
func TestWithoutAThresholdAGoTimeoutIsWarnedOf(t *testing.T) {
	timedOut := coverage.Unmeasured{Dir: at("src"), Language: "go", Cause: covport.MeasuredNothing{}, Reason: "its coverage command failed"}.
		WithTimeouts([]coverage.GoTimeout{{Package: "example.com/slow", Limit: "1s"}})
	other := coverage.Unmeasured{Dir: at(""), Language: "go", Cause: covport.MeasuredNothing{}, Reason: "its coverage command failed"}
	m := failedMeasurer{Measurer: measurer(), failed: []coverage.Unmeasured{other, timedOut}}
	code, _, stderr := run(t, CLI{CoverageReport: []string{"none.info"}}, handler(m, &snapporttest.Store{RootDir: at("")}))
	want := "itos-cc: no coverage for . (go): its coverage command failed; example.com/slow timed out after 1s, go test's limit per package: raise it with GOFLAGS=-timeout=<duration>\n"
	if code != 0 || stderr != want {
		t.Errorf("exit %d, stderr %q; want 0 and %q", code, stderr, want)
	}
	_, _, stderr = run(t, CLI{CoverageReport: []string{"none.info"}, Threshold: ptr(30)}, handler(m, &snapporttest.Store{RootDir: at("")}))
	if strings.Contains(stderr, "timed out") {
		t.Errorf("with a threshold, stderr %q warns of the timeout", stderr)
	}
}
