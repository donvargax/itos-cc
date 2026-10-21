// Package porttest holds our in-memory Workers and the contract it shares with
// worktrees. It imports the domain directly; no production code imports a fake.
package porttest

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	mutants "github.com/donvargax/itos-cc/internal/slices/mutation/domain"
)

// Tests describes one fixture's real command, keyed by directory and command.
// It is fixture configuration, not a sequence of expected calls or events.
type Tests struct {
	Baseline string
	Elapsed  time.Duration
	Verdicts []Verdict
	// Reached, when not nil, are the lines the tests execute, by file
	// relative to the root: a mutant on any other line passes them, as
	// tests that never run it cannot notice it.
	Reached map[string][]int
	// Selected are the IDs of the listed tests the command selects, which
	// its harness records it ran.
	Selected []string
	Trace    *Trace
	Hold     *WorkGate
}

// Verdict is a declared exception to the default killed mutant.
type Verdict struct {
	Path    string
	Site    mutants.Site
	Outcome string
}

// Trace identifies actual execution observations. Tests is the complete runnable
// selector inventory, including tests unrelated to the selected source.
type Trace struct {
	File, Source string
	Tests        map[string]string
	// Coverage says each record tells whether its run measured coverage,
	// as a harness checking testing.CoverMode records it: a test run
	// measures none.
	Coverage bool
	// CoverDir says each record tells the ITOS_CC_TEST_COVERDIR its run
	// was given, as a listed harness records it: a test run is given none.
	CoverDir bool
}

// Workers holds the actual virtual tree and each directory/command declaration.
// Write, when set, publishes trace bytes through the driver's actual file writer.
type Workers struct {
	// Opened retains the semantic purpose of actual pool lifetimes, for binding guards.
	// Lifetimes observe actual pool join/release, independent of result fields.
	Lifetimes []*InputLifetime
	OnRelease func(InputState) error
	// ReleaseFailure models an unavailable owned-input release after actual join.
	ReleaseFailure mutants.WorkerFailure
	Opened         []mutants.WorkPurpose
	Files          map[string]string
	Tests          map[string]map[string]Tests
	Write          func(path string, data []byte) error
	// Env is the actual per-run environment, independent of test verdict facts.
	Env      map[string]string
	Without  []string
	Platform string
	// Environments are actual entered command environments for binding guards.
	Environments []map[string]string
	// Originals are the original project roots of the private roots a
	// command may run from, such as a fresh plan's frozen inputs, by the
	// private root: a trace names the original, as an executed test names
	// the project it was written in.
	Originals map[string]string

	mu   sync.Mutex
	next int
}

var _ mutants.Workers = (*Workers)(nil)

func (f *Workers) Open(ctx context.Context, purpose mutants.WorkPurpose, count int) (mutants.WorkerPool, mutants.WorkerFailure) {
	if ctx.Err() != nil {
		return nil, mutants.WorkerCancelled{Cause: context.Cause(ctx)}
	}
	if purpose != mutants.MutationRun && purpose != mutants.FrozenTrials && purpose != mutants.FailFastJudgments {
		return nil, failed("unknown worker purpose")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.next++
	f.Opened = append(f.Opened, purpose)
	p := &pool{factory: f, lifetime: &InputLifetime{}}
	f.Lifetimes = append(f.Lifetimes, p.lifetime)
	for i := 0; i < max(1, count); i++ {
		p.workers = append(p.workers, &worker{pool: p, dir: filepath.Join(filepath.VolumeName(firstPath(f.Files))+string(filepath.Separator), "virtual-workers", fmt.Sprintf("%d-%d", purpose, f.next), fmt.Sprint(i)), copies: map[string]map[string]string{}})
	}
	return p, nil
}
func firstPath(files map[string]string) string {
	for name := range files {
		return name
	}
	return ""
}

type pool struct {
	factory  *Workers
	workers  []*worker
	closed   bool
	mu       sync.Mutex
	active   sync.WaitGroup
	lifetime *InputLifetime
}

func (p *pool) Worker(index int) mutants.Worker { return p.workers[index] }
func (p *pool) Abort()                          {} // no physical cleanup clock; actual context cancellation governs requests
func (p *pool) Close(context.Context) mutants.WorkerFailure {
	p.mu.Lock()
	p.closed = true
	p.mu.Unlock()
	p.active.Wait()
	p.lifetime.mu.Lock()
	p.lifetime.state.JoinReturned = true
	p.lifetime.mu.Unlock()
	p.factory.mu.Lock()
	defer p.factory.mu.Unlock()
	if p.factory.OnRelease != nil {
		if err := p.factory.OnRelease(p.lifetime.State()); err != nil {
			return mutants.WorkerFailed{Cause: err}
		}
	}
	if p.factory.ReleaseFailure != nil {
		return p.factory.ReleaseFailure
	}
	for _, w := range p.workers {
		w.copies = nil
	}
	p.lifetime.mu.Lock()
	p.lifetime.state.Released = true
	p.lifetime.mu.Unlock()
	return nil
}

type worker struct {
	pool   *pool
	dir    string
	copies map[string]map[string]string
}

func (w *worker) Run(ctx context.Context, c mutants.Command, change *mutants.Mutation, timeout time.Duration) (mutants.CommandResult, mutants.WorkerFailure) {
	if ctx.Err() != nil {
		return mutants.CommandResult{}, mutants.WorkerCancelled{Cause: context.Cause(ctx)}
	}
	p := w.pool
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return mutants.CommandResult{}, failed("worker pool is closed")
	}
	p.active.Add(1)
	p.mu.Unlock()
	defer p.active.Done()
	f := p.factory
	f.mu.Lock()
	defer f.mu.Unlock()

	tests, ok := f.Tests[filepath.Clean(c.Dir)][c.String()]
	if !ok {
		return mutants.CommandResult{}, failed("no tests declared for " + c.Dir + "$ " + c.String())
	}
	root := filepath.Clean(c.Root)
	copy := w.copies[root]
	if copy == nil {
		// A copy's Ready passes unless its program is absent, off the
		// PATH or a path no file is at, or a declaration of it fails it: a
		// setup failure, never a judgment.
		if program := firstWord(c.Ready); program != "" {
			ready := f.Tests[filepath.Clean(c.Dir)][strings.Join(c.Ready, " ")]
			_, there := f.Files[filepath.Clean(program)]
			if hiddenProgram(program, f.Without, f.Platform) || filepath.IsAbs(program) && !there || ready.Baseline != "" {
				return mutants.CommandResult{}, failed("the worker's copy is not readied by " + strings.Join(c.Ready, " "))
			}
		}
		copy = map[string]string{}
		for path, text := range f.Files {
			rel, err := filepath.Rel(root, path)
			if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				copy[filepath.Clean(path)] = text
			}
		}
		w.copies[root] = copy
	}
	if change != nil {
		if _, ok := copy[filepath.Clean(change.Path)]; !ok {
			return mutants.CommandResult{}, failed("mutation source is absent: " + change.Path)
		}
		copy[filepath.Clean(change.Path)] = string(change.Site.Apply(change.Source))
		defer func() { copy[filepath.Clean(change.Path)] = string(change.Source) }()
	}

	environment := maps.Clone(f.Env)
	if environment == nil {
		environment = map[string]string{}
	}
	if c.PathEnv != "" {
		var paths []string
		for _, dir := range c.PathDirs {
			paths = append(paths, filepath.Join(w.dir, filepath.FromSlash(dir)))
		}
		if inherited := environment[c.PathEnv]; inherited != "" {
			paths = append(paths, inherited)
		}
		environment[c.PathEnv] = strings.Join(paths, string(filepath.ListSeparator))
	}
	f.Environments = append(f.Environments, environment)

	if len(c.Args) > 0 && hiddenProgram(c.Args[0], f.Without, f.Platform) {
		return mutants.CommandResult{}, failed("requested executable is absent from actual Run.Without: " + c.Args[0])
	}
	if c.Shell != "" && len(f.Without) > 0 {
		if strings.ContainsAny(c.Shell, "\"'`\\;&|<>()$\n") {
			return mutants.CommandResult{}, failed("unsupported shell Run.Without shape")
		}
		words := strings.Fields(c.Shell)
		if len(words) == 0 {
			return mutants.CommandResult{}, failed("empty shell Run.Without request")
		}
		if hiddenProgram(words[0], f.Without, f.Platform) {
			if f.Platform == "windows" {
				return mutants.CommandResult{}, failed("unsupported Windows shell Run.Without outcome")
			}
			return mutants.CommandResult{Passed: false, ExitCode: 127, Output: words[0] + ": command not found"}, nil
		}
	}

	if tests.Trace != nil {
		if err := w.trace(c, copy, tests.Trace, harnessFields(tests, environment)); err != nil {
			return mutants.CommandResult{}, mutants.WorkerFailed{Cause: err}
		}
	}
	if gate := tests.Hold; gate != nil && (change != nil || !gate.MutationsOnly) {
		entry := Invocation{Command: c, Changed: change != nil, Files: maps.Clone(copy)}
		f.mu.Unlock()
		cancelled := gate.wait(ctx, entry)
		f.mu.Lock()
		if cancelled {
			return mutants.CommandResult{Cancelled: true, ExitCode: -1}, nil
		}
	}
	return declaredResult(c, tests, change, timeout)
}

func declaredResult(c mutants.Command, tests Tests, change *mutants.Mutation, timeout time.Duration) (mutants.CommandResult, mutants.WorkerFailure) {
	r := mutants.CommandResult{Passed: true, ExitCode: 0, Elapsed: tests.Elapsed}
	if change == nil {
		if tests.Baseline != "" {
			r.Passed = false
			r.ExitCode = 1
			r.Output = tests.Baseline
		}
		return r, nil
	}
	r.Passed = false
	r.ExitCode = 1
	rel, err := filepath.Rel(c.Root, change.Path)
	if err != nil {
		return r, mutants.WorkerFailed{Cause: err}
	}
	if tests.Reached != nil && !slices.Contains(tests.Reached[filepath.ToSlash(rel)], change.Site.Line) {
		r.Passed, r.ExitCode = true, 0
		return r, nil
	}
	for _, v := range tests.Verdicts {
		if filepath.ToSlash(rel) == filepath.ToSlash(v.Path) && v.Site.Line == change.Site.Line && v.Site.Original == change.Site.Original && v.Site.Replacement == change.Site.Replacement {
			switch v.Outcome {
			case mutants.Survived:
				r.Passed = true
				r.ExitCode = 0
			case mutants.Timeout:
				r.TimedOut = true
				r.ExitCode = -1
				r.Elapsed = timeout
			default:
				return r, failed("unsupported declared mutant outcome: " + v.Outcome)
			}
			break
		}
	}
	return r, nil
}

func failed(message string) mutants.WorkerFailure {
	return mutants.WorkerFailed{Cause: errors.New(message)}
}

// harnessFields are what a listed harness records of a run beside the
// standard trace: the IDs it selected, and the coverage directory it was
// given where its trace says so.
func harnessFields(tests Tests, environment map[string]string) map[string]any {
	fields := map[string]any{}
	if len(tests.Selected) > 0 {
		fields["tests"] = strings.Join(tests.Selected, ",")
	}
	if tests.Trace != nil && tests.Trace.CoverDir {
		fields["coverdir"] = environment["ITOS_CC_TEST_COVERDIR"]
	}
	return fields
}

func (w *worker) trace(c mutants.Command, copy map[string]string, t *Trace, fields map[string]any) error {
	rel, err := filepath.Rel(c.Root, c.Dir)
	if err != nil {
		return err
	}
	cwd := filepath.Join(w.dir, rel)
	if cwd == filepath.Clean(c.Root) {
		return errors.New("virtual worker is the original root")
	}
	return w.pool.factory.observeTrace(c, copy, t, cwd, w.pool.factory.original(c.Root), fields)
}

// original is the original project root of root, a command's.
func (f *Workers) original(root string) string {
	if original, ok := f.Originals[filepath.Clean(root)]; ok {
		return original
	}
	return filepath.Clean(root)
}

// observeTrace reads actual execution bytes at the supplied execution location.
// Preparation uses the frozen cwd; workers validate their private copy above.
func (f *Workers) observeTrace(c mutants.Command, copy map[string]string, t *Trace, cwd, original string, fields map[string]any) error {
	source := filepath.Join(c.Root, filepath.FromSlash(t.Source))
	text, ok := copy[source]
	if !ok {
		return fmt.Errorf("trace source is absent: %s", source)
	}
	names, err := SelectedTests(c, t.Tests)
	if err != nil {
		return err
	}
	hash := sha256.Sum256([]byte(text))
	for _, name := range names {
		record := map[string]any{"cwd": cwd, "original_root": original, "source_sha256": hex.EncodeToString(hash[:])}
		if t.Coverage {
			record["coverage"] = false
		}
		maps.Copy(record, fields)
		if strings.Contains(filepath.ToSlash(t.Source), "/") {
			record["source"] = filepath.ToSlash(t.Source)
		}
		if name != "" {
			record["test"] = name
		}
		data, err := json.Marshal(record)
		if err != nil {
			return err
		}
		data = append([]byte(f.Files[t.File]), append(data, '\n')...)
		if f.Write != nil {
			if err := f.Write(t.File, data); err != nil {
				return err
			}
		} else {
			f.Files[t.File] = string(data)
		}
	}
	return nil
}

// SelectedTests supports the concrete runnable selector shapes declared by the
// fixture inventory. Unsupported requests error rather than silently emitting
// no observations. A broad command executes the entire inventory.
func SelectedTests(c mutants.Command, inventory map[string]string) ([]string, error) {
	if inventory == nil {
		return []string{""}, nil
	}
	if len(inventory) == 0 {
		return nil, errors.New("trace inventory is empty")
	}
	args := c.Args
	if c.Shell != "" {
		if strings.ContainsAny(c.Shell, "\"'`\\") {
			return nil, errors.New("unsupported quoted trace command")
		}
		args = strings.Fields(c.Shell)
	}

	if !c.Suite {
		if len(args) == 0 {
			return nil, errors.New("trace request has no runner")
		}
		switch filepath.Base(args[0]) {
		case "python", "python3", "python.exe", "python3.exe", "gradle", "gradlew", "gradlew.bat", "mvn", "mvnw", "mvnw.cmd", "go":
		default:
			return nil, fmt.Errorf("unsupported trace runner %s", args[0])
		}
	}
	if len(args) > 0 && filepath.Base(args[0]) == "go" {
		return goSelected(args, inventory)
	}
	whole := c.Suite
	var selectors []string
	for i, a := range args {
		switch {
		case a == "--tests":
			if i+1 >= len(args) {
				return nil, errors.New("missing test selector")
			}
			selectors = append(selectors, args[i+1])
		case strings.HasPrefix(a, "-tests="):
			selectors = append(selectors, strings.Split(strings.TrimPrefix(a, "-tests="), "|")...)
		case strings.HasPrefix(a, "-Dtest="):
			selectors = append(selectors, strings.Split(strings.TrimPrefix(a, "-Dtest="), ",")...)
		case a == "discover" || a == "./...":
			whole = true
		case inventory[a] != "":
			selectors = append(selectors, a)
		}
	}
	// Parse every Python operand, even if an earlier one was known. A
	// misspelled second selector must not vanish behind a valid first one.
	module := -1
	for i := 0; i+1 < len(args); i++ {
		if args[i] == "-m" && (args[i+1] == "pytest" || args[i+1] == "unittest") {
			module = i + 1
			break
		}
	}
	if module >= 0 {
		selectors = nil
		for i := module + 1; i < len(args); i++ {
			a := args[i]
			switch a {
			case "-p":
				if args[module] != "pytest" {
					return nil, errors.New("unsupported unittest trace pattern")
				}
				i++
				if i >= len(args) || args[i] != "no:cacheprovider" {
					return nil, errors.New("unsupported Python trace plugin")
				}
			case "-q", "-v", "--":
			case "-f":
				if args[module] != "unittest" {
					return nil, errors.New("unsupported pytest trace flag -f")
				}
			case "-x", "--disable-warnings":
				if args[module] != "pytest" {
					return nil, fmt.Errorf("unsupported unittest trace flag %s", a)
				}
			case "discover":
				if args[module] != "unittest" {
					return nil, errors.New("unsupported pytest trace discovery operand")
				}
				whole = true
			default:
				if strings.HasPrefix(a, "-") {
					return nil, fmt.Errorf("unsupported Python trace filter %s", a)
				}
				selectors = append(selectors, a)
			}
		}
		if len(selectors) == 0 {
			whole = true
		}
	} else if len(selectors) == 0 && !whole && len(args) > 0 {
		// Existing Kotlin runners and wrappers execute the suite when no class
		// selector is supplied. Recognize the executable, not an incidental word.
		switch filepath.Base(args[0]) {
		case "gradle", "gradlew", "gradlew.bat", "mvn", "mvnw", "mvnw.cmd":
			whole = true
		}
	}
	// A broad declaration cannot hide an explicitly unknown requested selector.
	for _, key := range selectors {
		if _, ok := inventory[key]; !ok {
			return nil, fmt.Errorf("unknown runnable trace selector %s", key)
		}
	}

	keys := slices.Sorted(slices.Values(selectors))
	if whole {
		keys = make([]string, 0, len(inventory))
		for key := range inventory {
			keys = append(keys, key)
		}
		slices.Sort(keys)
	}
	if len(keys) == 0 {
		return nil, fmt.Errorf("unsupported trace selector in %s", c.String())
	}
	var names []string
	for _, key := range slices.Compact(keys) {
		name, ok := inventory[key]
		if !ok {
			return nil, fmt.Errorf("unknown runnable trace selector %s", key)
		}
		names = append(names, name)
	}
	return names, nil
}

// goSelected is the tests go test runs, those of the test files of the
// packages it names, by their inventory keys, file paths from the root:
// every one for ./..., and its own for a package folder. The arguments after
// -args are the tests' own, which select nothing here.
func goSelected(args []string, inventory map[string]string) ([]string, error) {
	var packages []string
	for _, a := range args[1:] {
		if a == "-args" {
			break
		}
		if a == "test" || strings.HasPrefix(a, "-") {
			continue
		}
		packages = append(packages, a)
	}
	var keys []string
	for _, pkg := range packages {
		if pkg == "./..." {
			keys = slices.Collect(maps.Keys(inventory))
			break
		}
		dir := filepath.ToSlash(filepath.Clean(filepath.FromSlash(pkg)))
		for key := range inventory {
			if filepath.ToSlash(filepath.Dir(filepath.FromSlash(key))) == dir {
				keys = append(keys, key)
			}
		}
	}
	if len(keys) == 0 {
		return nil, fmt.Errorf("unsupported trace selector in %s", strings.Join(args, " "))
	}
	slices.Sort(keys)
	var names []string
	for _, key := range slices.Compact(keys) {
		names = append(names, inventory[key])
	}
	return names, nil
}

// PATH hiding affects bare lookups only. Explicit installed paths remain facts.
func hiddenProgram(program string, without []string, platform string) bool {
	if filepath.IsAbs(program) || strings.ContainsAny(program, "/\\") {
		return false
	}
	if platform == "windows" {
		program = strings.TrimSuffix(program, filepath.Ext(program))
		program = strings.ToLower(program)
	}
	for _, name := range without {
		if platform == "windows" {
			name = strings.ToLower(name)
		}
		if prefix, ok := strings.CutSuffix(name, "*"); ok {
			if strings.HasPrefix(program, prefix) {
				return true
			}
		} else if program == name {
			return true
		}
	}
	return false
}

func firstWord(words []string) string {
	if len(words) == 0 {
		return ""
	}
	return words[0]
}
