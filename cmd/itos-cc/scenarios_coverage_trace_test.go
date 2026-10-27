package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/donvargax/itos-cc/features"
	"github.com/donvargax/itos-cc/internal/coverage"
	"github.com/donvargax/itos-cc/internal/coverage/port"
	covtest "github.com/donvargax/itos-cc/internal/coverage/porttest"
	"github.com/donvargax/itos-cc/internal/covrun"
	"github.com/donvargax/itos-cc/internal/lang"
	mutation "github.com/donvargax/itos-cc/internal/slices/mutation/domain"
	mutantstest "github.com/donvargax/itos-cc/internal/slices/mutation/domain/porttest"
)

// memoryCoverage observes actual successful/failing built-in measurement requests.
// Unsupported fixture observations remain a Driver.Run error, never a fabricated
// product measurement failure. The actual measurement always delegates unchanged.
type memoryCoverage struct {
	memory         *memory
	base           *covtest.Measurer
	inputs         *mutantstest.Inputs
	observationErr error
}

var _ coverage.MutationMeasurer = (*memoryCoverage)(nil)

func (m *memoryCoverage) Measure(ctx context.Context, req port.Request) (*coverage.Report, port.Failure) {
	return m.MeasureListed(ctx, req, nil)
}
func (m *memoryCoverage) MeasureListed(ctx context.Context, req port.Request, tests *coverage.PerTest) (*coverage.Report, port.Failure) {
	m.observationErr = errors.Join(m.observationErr, m.command(req))
	if len(req.Reports) == 0 && !req.Existing {
		m.announce(ctx, req)
	}
	before := len(m.base.Entered)
	beforeCommands := len(m.base.Commands)
	r, failure := m.base.MeasureListed(ctx, req, tests)
	if failure == nil && len(req.Reports) == 0 && !req.Existing {
		m.observationErr = errors.Join(m.observationErr, m.observe(ctx, req, m.base.Entered[before:]))
		if tests != nil {
			for _, command := range m.base.Commands[beforeCommands:] {
				m.observationErr = errors.Join(m.observationErr, m.observeListed(tests, command))
			}
		}
	}
	return r, failure
}

// command runs a --coverage-command as the project's own tools run it in
// the nearest folder at or above the working directory whose tools the
// scenario declared: they write the report they measure, as declared, where
// the request says the command's report lands. A command in a folder whose
// tools write no declared report writes none, which measured nothing.
func (m *memoryCoverage) command(req port.Request) error {
	if req.Command == "" || len(req.Reports) != 1 {
		return nil
	}
	dir := m.base.Aggregate.Dir
	if m.base.Log != nil {
		_, _ = fmt.Fprintf(m.base.Log, "itos-cc: coverage $ %s\n", req.Command)
	}
	for d := dir; ; d = filepath.Dir(d) {
		if declared, ok := m.memory.coverage[d]; ok {
			// The tools say what a failing run printed, and the shell its
			// exit status, as a real run does.
			if declared.CommandExit != 0 && m.base.Log != nil {
				_, _ = fmt.Fprintf(m.base.Log, "%s\nitos-cc: coverage: exit status %d\n", declared.CommandOutput, declared.CommandExit)
			}
			if declared.Report == "" {
				return nil
			}
			path := req.Reports[0]
			if !filepath.IsAbs(path) {
				path = filepath.Join(dir, filepath.FromSlash(path))
			}
			if err := m.memory.Write(path, []byte(declared.Report)); err != nil {
				return err
			}
			m.base.Aggregate.Files[filepath.Clean(path)] = declared.Report
			return nil
		}
		if filepath.Dir(d) == d {
			return nil
		}
	}
}

// announce says, on the run's stderr, what each build root's own tools run
// to measure req's sources, as the collector says it before it runs them:
// the tests of the sources' own scope, or the whole suite's, by the command
// their runner runs them with, or that no test reaches them. A root whose
// tools the scenario declared none of, or whose runner the memory cannot
// tell, says nothing.
func (m *memoryCoverage) announce(ctx context.Context, req port.Request) {
	if m.base.Log == nil {
		return
	}
	type group struct{ language, root string }
	var order []group
	sources := map[group][]string{}
	for _, source := range req.Sources {
		spec := lang.Detect(source)
		if spec == nil || covrun.Markers(spec.Name) == nil || slices.Contains(req.Served, source) {
			continue
		}
		g := group{spec.Name, m.memory.buildRoot(source, covrun.Markers(spec.Name))}
		if declared, ok := m.memory.coverage[g.root]; !ok || declared.Language != g.language {
			continue
		}
		if sources[g] == nil {
			order = append(order, g)
		}
		sources[g] = append(sources[g], source)
	}
	for _, g := range order {
		request := mutation.CommandRequest{Path: sources[g][0], All: req.Scope != port.OwnTests || req.AllTests || req.Reach == nil}
		if !request.All {
			for _, source := range sources[g] {
				request.Tests = append(request.Tests, req.Reach(source)...)
			}
			slices.Sort(request.Tests)
			request.Tests = slices.Compact(request.Tests)
		}
		c, failure := mutation.ObserveCommand(ctx, m.inputs, request)
		if failure != nil {
			continue
		}
		if c.RunsNothing() {
			var rel []string
			for _, source := range sources[g] {
				rel = append(rel, relTo(g.root, source))
			}
			_, _ = fmt.Fprintf(m.base.Log, "itos-cc: coverage %s: no test reaches %s, so none runs\n", g.root, strings.Join(rel, " "))
			continue
		}
		_, _ = fmt.Fprintf(m.base.Log, "itos-cc: coverage %s$ %s\n", g.root, c.String())
	}
}

// relTo is path from dir, slash-separated, or path when it is not below.
func relTo(dir, path string) string {
	if rel, err := filepath.Rel(dir, path); err == nil && filepath.IsLocal(rel) {
		return filepath.ToSlash(rel)
	}
	return path
}

func (m *memoryCoverage) Plans(ctx context.Context, req port.Request) []coverage.Plan {
	return m.base.Plans(ctx, req)
}

func (m *memoryCoverage) Inventory(ctx context.Context, r *coverage.Report, language string) port.Failure {
	return m.base.Inventory(ctx, r, language)
}

func (m *memoryCoverage) observe(ctx context.Context, req port.Request, measured []coverage.Unloaded) error {
	seen := map[string]bool{}
	for _, source := range req.Sources {
		spec := lang.Detect(source)
		if spec == nil {
			continue
		}
		root := m.memory.buildRoot(source, covrun.Markers(spec.Name))
		if !slices.Contains(measured, coverage.Unloaded{Language: spec.Name, Dir: root}) {
			continue
		}
		declared, ok := m.memory.coverage[root]
		if !ok || declared.Language != spec.Name {
			continue
		}
		// Only the tests the collector runs observe: those its language's
		// test runner runs, never another program's, such as a make target
		// declared beside them. A runner the memory cannot tell, such as a
		// Python interpreter no probe declares, rules none out.
		runner := ""
		if run, failure := mutation.ObserveCommand(ctx, m.inputs, mutation.CommandRequest{Path: source, All: true}); failure == nil {
			runner = run.String()
		}
		for _, command := range slices.Sorted(maps.Keys(m.memory.tests[root])) {
			fixture := m.memory.tests[root][command]
			trace := fixture.Trace
			if trace == nil || runner != "" && !sameProgram(command, runner) {
				continue
			}
			// A given may describe several command modes with one trace. One
			// actual coverage selection observes the matching inventory once.
			key := root + "\x00" + trace.File
			if seen[key] {
				continue
			}
			seen[key] = true
			c := mutation.Command{Root: root, Dir: root, Suite: true}
			if req.Scope == port.OwnTests && !req.AllTests && req.Reach != nil {
				var tests []string
				for _, selected := range req.Sources {
					if selectedSpec := lang.Detect(selected); selectedSpec != nil && selectedSpec.Name == spec.Name && m.memory.buildRoot(selected, covrun.Markers(spec.Name)) == root {
						tests = append(tests, req.Reach(selected)...)
					}
				}
				slices.Sort(tests)
				tests = slices.Compact(tests)
				for _, path := range tests {
					if _, ok := m.memory.files[filepath.Clean(path)]; !ok {
						return fmt.Errorf("coverage trace unknown selected test %s", path)
					}
				}
				var failure mutation.InputFailure
				c, failure = mutation.ObserveCommand(ctx, m.inputs, mutation.CommandRequest{Path: source, Tests: tests})
				if failure != nil {
					return failure
				}
				if c.RunsNothing() {
					continue
				}
			}
			fields := map[string]any{}
			if trace.CoverDir {
				fields["coverdir"] = ""
			}
			if err := m.emit(root, c, trace, fields); err != nil {
				return err
			}

		}
	}
	return nil
}

// sameProgram is whether two command lines run one program, the first word
// of each, as declared and as chosen.
func sameProgram(declared, chosen string) bool {
	d, c := strings.Fields(declared), strings.Fields(chosen)
	return len(d) > 0 && len(c) > 0 && (d[0] == c[0] || filepath.Base(d[0]) == filepath.Base(c[0]))
}

func (m *memory) admitCoverageTrace(root string, t features.Tests) error {
	trace := t.Trace
	if trace == nil {
		return nil
	}
	source := filepath.Join(root, filepath.FromSlash(trace.Source))
	if _, ok := m.files[source]; !ok {
		return fmt.Errorf("trace source missing: %s", source)
	}
	if trace.File == "" {
		return errors.New("trace observation path is empty")
	}
	if trace.Tests == nil {
		return nil
	}
	if len(trace.Tests) == 0 {
		return errors.New("trace inventory is empty")
	}
	for key, name := range trace.Tests {
		if key == "" || name == "" {
			return errors.New("trace inventory selector/name is empty")
		}
		path := filepath.Join(root, filepath.FromSlash(key))
		if _, ok := m.files[path]; ok {
			continue
		}
		module := filepath.Join(root, filepath.FromSlash(strings.ReplaceAll(key, ".", "/")+".py"))
		if _, ok := m.files[module]; ok {
			continue
		}
		// Resolve Kotlin selectors against actual package/class source facts.
		if spec := lang.Detect(source); spec != nil && spec.Name == "kotlin" {
			var files []string
			for path := range m.files {
				if under(path, root) && strings.HasSuffix(path, ".kt") {
					files = append(files, path)
				}
			}
			facts, failure := m.inputs(features.Run{Dir: root}).Observe(m.t.Context(), mutation.CommandRequest{Path: source, Tests: files})
			if failure != nil {
				return failure
			}
			found := false
			for _, names := range facts.Classes {
				found = found || slices.Contains(names, key)
			}
			if found {
				continue
			}
		}
		return fmt.Errorf("trace inventory selector has no actual file/class: %s", key)
	}
	_, err := mutantstest.SelectedTests(mutation.Command{Shell: t.Command, Suite: true}, trace.Tests)
	return err
}

func TestMemoryMeasurementTracesFollowActualRequestsAndBytes(t *testing.T) {
	m := newMemory(t)
	root, err := m.Folder()
	if err != nil {
		t.Fatal(err)
	}
	for file, text := range map[string]string{"pyproject.toml": "[project]\nname='trace'\nversion='1'\n", "board.py": "def value():\n    return 1\n", "test_a.py": "def test_a():\n    assert True\n", "test_b.py": "def test_b():\n    assert True\n"} {
		if err := m.Write(filepath.Join(root, file), []byte(text)); err != nil {
			t.Fatal(err)
		}
	}
	trace := filepath.Join(root, "observations.jsonl")
	if err := m.Tools(t.Context(), root, features.ToolSetup{Python: &features.PythonProbe{Interpreter: "python3", Importable: true}}); err != nil {
		t.Fatal(err)
	}
	declaration := features.Tests{Language: "python", Command: "python3 -m pytest -q -x -p no:cacheprovider test_a.py", Trace: &features.TestTrace{File: trace, Source: "board.py", Tests: map[string]string{"test_a.py": "A", "test_b.py": "B"}}}
	if err := m.Tests(root, declaration); err != nil {
		t.Fatal(err)
	}
	if err := m.Coverage(root, features.Coverage{Language: "python", Report: "SF:board.py\nDA:2,1\nend_of_record\n"}); err != nil {
		t.Fatal(err)
	}
	binding := &memoryCoverage{memory: m, base: m.measurer(root), inputs: m.inputs(features.Run{Dir: root})}
	source := filepath.Join(root, "board.py")
	req := port.Request{Sources: []string{source}, Scope: port.OwnTests, Reach: func(string) []string { return []string{filepath.Join(root, "test_a.py")} }}
	if _, failure := binding.Measure(t.Context(), req); failure != nil || binding.observationErr != nil {
		t.Fatalf("measurement: %v %v", failure, binding.observationErr)
	}
	records := traceRecords(t, m.files[trace])
	if len(records) != 1 || records[0]["test"] != "A" || records[0]["cwd"] != root || records[0]["original_root"] != root {
		t.Fatalf("request selection: %v", records)
	}
	digest := sha256.Sum256([]byte(m.files[source]))
	if records[0]["source_sha256"] != hex.EncodeToString(digest[:]) {
		t.Fatal("trace did not hash actual source")
	}
	if err := m.Write(trace, nil); err != nil {
		t.Fatal(err)
	}
	if err := m.Write(source, []byte("def value():\n    return 2\n")); err != nil {
		t.Fatal(err)
	}
	result, err := m.Run(features.Run{Dir: root, Args: []string{"crap", "--json", "--all-tests", "board.py"}})
	if err != nil || result.Exit != 0 {
		t.Fatalf("actual public binding: %+v %v", result, err)
	}
	records = traceRecords(t, m.files[trace])
	var names []string
	for _, r := range records {
		names = append(names, r["test"])
	}
	if !reflect.DeepEqual(names, []string{"A", "B"}) || records[0]["source_sha256"] == hex.EncodeToString(digest[:]) {
		t.Fatalf("whole suite/current bytes: %v", records)
	}
	if err := m.Write(trace, nil); err != nil {
		t.Fatal(err)
	}
	binding.base.Declarations = map[coverage.Unloaded]covtest.Measurement{}
	if _, failure := binding.Measure(t.Context(), req); failure != nil || m.files[trace] != "" {
		t.Fatal("wrong measurement binding fabricated execution")
	}
	binding.base = m.measurer(root)
	req.Reach = func(string) []string { return []string{filepath.Join(root, "missing.py")} }
	if _, failure := binding.Measure(t.Context(), req); failure != nil || binding.observationErr == nil {
		t.Fatalf("observer error leaked into product failure or vanished: %v %v", failure, binding.observationErr)
	}
	if err := m.Write(trace, nil); err != nil {
		t.Fatal(err)
	}
	failed := m.coverage[root]
	failed.CommandExit = 1
	failed.CommandOutput = "collector ran then failed"
	failed.Report = ""
	if err := m.Coverage(root, failed); err != nil {
		t.Fatal(err)
	}
	binding = &memoryCoverage{memory: m, base: m.measurer(root), inputs: m.inputs(features.Run{Dir: root})}
	req.Reach = func(string) []string { return []string{filepath.Join(root, "test_a.py")} }
	report, failure := binding.Measure(t.Context(), req)
	if failure != nil || binding.observationErr != nil || len(report.Missing()) != 1 || report.Has(source) {
		t.Fatalf("failed collector fabricated proof: %v %v %v", report, failure, binding.observationErr)
	}
	records = traceRecords(t, m.files[trace])
	if len(records) != 1 || records[0]["test"] != "A" {
		t.Fatalf("failed entered collector lost actual request trace: %v", records)
	}
	if err := m.Write(trace, nil); err != nil {
		t.Fatal(err)
	}
	failed.CommandExit = 0
	failed.Report = "SF:board.py\nDA:2,1\nend_of_record\n"
	if err := m.Coverage(root, failed); err != nil {
		t.Fatal(err)
	}
	listed := declaration
	listed.Listed = &features.ListedTests{Files: map[string]string{"A": "test_a.py", "B": "test_b.py"}, Lines: map[string]map[int][]string{"board.py": {2: {"A", "B"}}}}
	if err := m.Tests(root, listed); err != nil {
		t.Fatal(err)
	}
	binding = &memoryCoverage{memory: m, base: m.measurer(root), inputs: m.inputs(features.Run{Dir: root})}
	perTest := &coverage.PerTest{Root: root, Tests: []coverage.Test{{ID: "A", File: "test_a.py"}, {ID: "B", File: "test_b.py"}}, All: "python3 -m pytest -q test_a.py test_b.py", Select: func(ids []string) string {
		if ids[0] == "A" {
			return "python3 -m pytest -q test_a.py"
		}
		return "python3 -m pytest -q test_b.py"
	}}
	if _, failure := binding.MeasureListed(t.Context(), req, perTest); failure != nil || binding.observationErr != nil {
		t.Fatalf("listed trace: %v %v", failure, binding.observationErr)
	}
	records = traceRecords(t, m.files[trace])
	names = nil
	for _, record := range records {
		names = append(names, record["test"])
	}
	if !reflect.DeepEqual(names, []string{"A", "A", "B", "A", "B"}) {
		t.Fatalf("actual aggregate/all/fallback selections: %v", names)
	}
	bad := declaration
	bad.Trace = &features.TestTrace{File: trace, Source: "missing.py"}
	if err := m.Tests(root, bad); err == nil {
		t.Fatal("missing trace source admitted")
	}
	bad = declaration
	bad.Trace = &features.TestTrace{File: trace, Source: "board.py", Tests: map[string]string{"missing.py": "M"}}
	if err := m.Tests(root, bad); err == nil {
		t.Fatal("unknown inventory selector admitted")
	}
}
func traceRecords(t *testing.T, text string) []map[string]string {
	t.Helper()
	var out []map[string]string
	for _, line := range strings.Split(strings.TrimSpace(text), "\n") {
		if line == "" {
			continue
		}
		var record map[string]string
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatal(err)
		}
		out = append(out, record)
	}
	return out
}

func (m *memoryCoverage) observeListed(tests *coverage.PerTest, command string) error {
	seen := map[string]bool{}
	for _, key := range slices.Sorted(maps.Keys(m.memory.tests[tests.Root])) {
		trace := m.memory.tests[tests.Root][key].Trace
		if trace == nil || seen[trace.File] {
			continue
		}
		seen[trace.File] = true
		c := mutation.Command{Root: tests.Root, Dir: tests.Root, Shell: command, Suite: command == tests.All}
		// The listed collection gives its harness a coverage directory,
		// and selects every listed test, or one of them.
		fields := map[string]any{}
		if trace.CoverDir {
			fields["coverdir"] = filepath.Join(tests.Root, ".metrics", "coverage", "virtual-listed")
			var ids []string
			for _, test := range tests.Tests {
				ids = append(ids, test.ID)
				if tests.Select != nil && command != tests.All && tests.Select([]string{test.ID}) == command {
					ids = []string{test.ID}
					break
				}
			}
			fields["tests"] = strings.Join(ids, ",")
		}
		if err := m.emit(tests.Root, c, trace, fields); err != nil {
			return err
		}
	}
	return nil
}
func (m *memoryCoverage) emit(root string, c mutation.Command, trace *features.TestTrace, fields map[string]any) error {
	names, err := mutantstest.SelectedTests(c, trace.Tests)
	if err != nil {
		return fmt.Errorf("coverage trace: %w", err)
	}
	sourcePath := filepath.Join(root, filepath.FromSlash(trace.Source))
	text, ok := m.memory.files[sourcePath]
	if !ok {
		return fmt.Errorf("coverage trace source missing: %s", sourcePath)
	}
	digest := sha256.Sum256([]byte(text))
	for _, name := range names {
		record := map[string]any{"cwd": filepath.Clean(c.Dir), "original_root": root, "source_sha256": hex.EncodeToString(digest[:])}
		if trace.Coverage {
			record["coverage"] = true
		}
		maps.Copy(record, fields)
		if strings.Contains(filepath.ToSlash(trace.Source), "/") {
			record["source"] = filepath.ToSlash(trace.Source)
		}
		if name != "" {
			record["test"] = name
		}
		data, err := json.Marshal(record)
		if err != nil {
			return err
		}
		data = append([]byte(m.memory.files[trace.File]), append(data, '\n')...)
		if err := m.memory.Write(trace.File, data); err != nil {
			return err
		}
	}
	return nil
}
