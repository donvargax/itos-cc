// Counted scenarios observe public reports and actual owned Go fixtures.
package features

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/cucumber/godog"
)

type countedObservation struct {
	object map[string]any
	result Result
	traces []map[string]any
}

type countedMutation struct {
	e                    *mutationExecution
	id, head, seed, mode string
	workers              int
	primary              countedObservation
	controls             []countedObservation
	before               map[string]string
	mainRoot, linkedRoot string
	worktreeParent       string
	traceBefore          []byte
	sourceHashes         map[string]string
	children             []*countedMutation
	saved                map[string]countedObservation
	texts                map[string]Result
	// committed is the committed text of each file a scenario changes
	// without committing, by its slash path.
	committed map[string]string
}

func countedMutationSteps(sc *godog.ScenarioContext, m *mutateState, s *sinceMutation) {
	q := &countedMutation{}
	sc.Before(func(ctx context.Context, scenario *godog.Scenario) (context.Context, error) {
		*q = countedMutation{e: &mutationExecution{m: m, s: s, command: "go test -count=1 -failfast ./."}, seed: "T81-fixed", saved: map[string]countedObservation{}, texts: map[string]Result{}}
		for _, tag := range scenario.Tags {
			if strings.HasPrefix(tag.Name, "@ID-MUT-") {
				q.id = strings.TrimPrefix(tag.Name, "@ID-MUT-")
			}
		}
		if slices.Contains([]string{"173", "174", "175", "176", "177", "178", "179", "180", "181", "182", "183", "184", "185", "186", "212", "275"}, q.id) {
			m.w.beforeRemove = q.cleanup
		}
		if slices.Contains(interruptIDs, q.id) {
			// The interrupt's steps share the ownership and fail-fast steps'
			// state, whose sentences they reuse.
			q.e = m.execution
			m.w.beforeRemove = q.cleanup
		}
		return ctx, nil
	})
	q.admissionSteps(sc)
	q.selectionSteps(sc)
	q.frozenSteps(sc)
	q.budgetSteps(sc)
	q.interruptSteps(sc)
	q.runInterruptSteps(sc)
	q.worktreeSteps(sc)
	q.listedSteps(sc)
	q.preparationSteps(sc)
	q.outcomeSteps(sc)
	q.uncommittedSteps(sc)
}

func (q *countedMutation) cleanup(ctx context.Context) error {
	for _, child := range q.children {
		if err := child.cleanup(ctx); err != nil {
			return err
		}
	}
	if err := q.e.cleanup(ctx); err != nil {
		return err
	}
	if q.linkedRoot != "" {
		if err := q.e.m.w.d.Git(q.mainRoot, "worktree", "remove", "--force", q.linkedRoot); err != nil {
			return err
		}
		if err := q.e.m.w.d.Remove(q.mainRoot); err != nil {
			return err
		}
	}
	if q.worktreeParent != "" {
		return q.e.m.w.d.Remove(q.worktreeParent)
	}
	return nil
}

func (q *countedMutation) project(source string) error {
	w := q.e.m.w
	if err := w.d.Counted(); err != nil {
		return err
	}
	if err := q.e.project(map[string]string{"main.go": source}); err != nil {
		return err
	}
	fixture := strings.NewReplacer("TRACE_PATH", strconv.Quote(filepath.Join(q.e.outside, "trace.jsonl")), "CONTROL_PATH", strconv.Quote(filepath.Join(q.e.outside, "counted-control.json")), "ORIGINAL_ROOT", strconv.Quote(w.dir)).Replace(countedGoTests)
	if err := w.write("main_test.go", fixture); err != nil {
		return err
	}
	if err := w.d.Git(w.dir, "add", "main_test.go"); err != nil {
		return err
	}
	if err := w.d.Git(w.dir, "commit", "-qm", "counted fixture"); err != nil {
		return err
	}
	if err := q.originalHashes("main.go"); err != nil {
		return err
	}
	return q.setMode("covered")
}

func (q *countedMutation) setMode(mode string) error {
	w := q.e.m.w
	q.mode = mode
	data, err := json.Marshal(map[string]string{"mode": mode})
	if err != nil {
		return err
	}
	if err = w.d.Write(filepath.Join(q.e.outside, "counted-control.json"), data); err != nil {
		return err
	}
	t := Tests{Language: "go", Command: q.e.command, Elapsed: 100 * time.Millisecond, Trace: &TestTrace{File: filepath.Join(q.e.outside, "trace.jsonl"), Source: "main.go", Coverage: true}}
	site := Site{File: "main.go", Line: 3, Original: ">", Replacement: ">="}
	if mode == "survive" {
		t.Surviving = []Site{site}
	}
	if mode == "timeout" {
		t.TimingOut = []Site{site}
	}
	if mode == "baseline-fail" {
		t.Baseline = "T81 baseline fails"
	}
	if err = w.d.Tests(w.dir, t); err != nil {
		return err
	}
	c := Coverage{Language: "go", Report: countedOneProfile}
	if mode == "uncovered" {
		c.Report = strings.ReplaceAll(c.Report, " 1 1\n", " 1 0\n")
	}
	if mode == "coverage-fail" {
		c.CommandExit = 1
		c.CommandOutput = "T81 coverage fails after report"
	}
	return w.d.Coverage(w.dir, c)
}

func (q *countedMutation) observe(args ...string) (countedObservation, error) {
	return q.observeAt(q.e.m.w.dir, args...)
}
func (q *countedMutation) observeAt(dir string, args ...string) (countedObservation, error) {
	w := q.e.m.w
	before, err := q.readTrace()
	if err != nil {
		return countedObservation{}, err
	}
	code, out, log, err := w.capture(dir, w.env(), args...)
	if err != nil {
		return countedObservation{}, err
	}
	o := countedObservation{result: Result{Exit: code, Stdout: out, Stderr: log}}
	after, traceErr := q.readTrace()
	if traceErr != nil {
		return o, traceErr
	}
	if len(after) < len(before) || string(after[:len(before)]) != string(before) {
		return o, fmt.Errorf("actual counted trace was replaced")
	}
	for _, line := range strings.Split(strings.TrimSpace(string(after[len(before):])), "\n") {
		if line == "" {
			continue
		}
		var record map[string]any
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			return o, err
		}
		o.traces = append(o.traces, record)
	}
	if err = json.Unmarshal([]byte(out), &o.object); err != nil {
		return o, fmt.Errorf("counted public JSON: %w: %s\n%s", err, out, log)
	}
	if o.object["schema"] != float64(1) {
		return o, fmt.Errorf("counted public schema: %v", o.object)
	}
	return o, nil
}
func (q *countedMutation) count(args ...string) (countedObservation, error) {
	return q.observe(append([]string{"mutation", "run", "--json"}, args...)...)
}
func (q *countedMutation) otherProject(source string) (*countedMutation, error) {
	d := q.e.m.w.d
	dir, err := d.Folder()
	if err != nil {
		return nil, err
	}
	q.e.extraDirs = append(q.e.extraDirs, dir)
	w := &world{d: d, dir: dir}
	o := &countedMutation{e: &mutationExecution{m: &mutateState{w: w}, command: "go test -count=1 -failfast ./."}, seed: q.seed, saved: map[string]countedObservation{}, texts: map[string]Result{}}
	if err = o.project(source); err != nil {
		return nil, err
	}
	q.e.extraDirs = append(q.e.extraDirs, o.e.outside)
	return o, nil
}
func (q *countedMutation) judge() error {
	var err error
	q.primary, err = q.count("--count", "1", "--workers", "1", "--seed", q.seed)
	return err
}
func countedSampling(o countedObservation) map[string]any { return executionMap(o.object, "sampling") }
func countedSelected(o countedObservation) (map[string]any, error) {
	a := objectList(o.object, "selected")
	if len(a) != 1 {
		return nil, fmt.Errorf("counted selected population: %v", a)
	}
	return a[0], nil
}
func countedProblem(o countedObservation, rule string, code int) error {
	if o.result.Exit != code || o.object["ok"] != false {
		return fmt.Errorf("counted problem %s exit%d: %v", rule, o.result.Exit, o.object)
	}
	ps := checkProblems(o.object)
	if len(ps) != 1 || ps[0]["rule"] != rule {
		return fmt.Errorf("counted problem expected %s: %v", rule, ps)
	}
	return nil
}
func countedStage(o countedObservation, name string) (map[string]any, error) {
	var matches []map[string]any
	for _, s := range objectList(o.object, "stages") {
		if s["name"] == name {
			matches = append(matches, s)
		}
	}
	if len(matches) != 1 {
		return nil, fmt.Errorf("counted stage %s: %v", name, matches)
	}
	return matches[0], nil
}
func countedNoTrials(o countedObservation) error {
	if countedSampling(o)["executed"] != float64(0) || strings.Contains(o.result.Stderr, "itos-cc: [") {
		return fmt.Errorf("counted unexpected trial: %v\n%s", o.object, o.result.Stderr)
	}
	return nil
}
func (q *countedMutation) tree(ctx context.Context, dir string) (map[string]string, error) {
	d := q.e.m.w.d
	files, err := d.Files(ctx, dir)
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, file := range files {
		if strings.HasPrefix(file, ".git/") {
			continue
		}
		data, err := d.Read(filepath.Join(dir, filepath.FromSlash(file)))
		if err != nil {
			return nil, err
		}
		out[file] = string(data)
	}
	return out, nil
}

func (q *countedMutation) sameTree(ctx context.Context, dir string, before map[string]string) error {
	after, err := q.tree(ctx, dir)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(before, after) {
		return fmt.Errorf("counted live tree changed: before%v after%v", before, after)
	}
	return nil
}

func (q *countedMutation) readTrace() ([]byte, error) {
	if q.e.outside == "" {
		return nil, nil
	}
	b, err := q.e.m.w.d.Read(filepath.Join(q.e.outside, "trace.jsonl"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	return b, err
}
func (q *countedMutation) originalHashes(files ...string) error {
	q.sourceHashes = map[string]string{}
	for _, file := range files {
		b, err := q.e.m.w.d.Read(filepath.Join(q.e.m.w.dir, filepath.FromSlash(file)))
		if err != nil {
			return err
		}
		sum := sha256.Sum256(b)
		q.sourceHashes[file] = hex.EncodeToString(sum[:])
	}
	return nil
}
func (q *countedMutation) noTrials(o countedObservation) error {
	if err := countedNoTrials(o); err != nil {
		return err
	}
	if len(o.traces) == 0 && countedSampling(o)["eligible"] != float64(0) {
		return fmt.Errorf("counted no-trial observation has no actual coverage executions")
	}
	for _, r := range o.traces {
		file, _ := r["source"].(string)
		if file == "" {
			file = "main.go"
		}
		expected, ok := q.sourceHashes[file]
		if !ok || r["source_sha256"] != expected || r["cwd"] == "" || r["original_root"] == "" {
			return fmt.Errorf("counted actual trial or wrong original source: %v expected%v", r, q.sourceHashes)
		}
	}
	return nil
}
