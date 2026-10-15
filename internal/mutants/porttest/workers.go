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
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/donvargax/itos-cc/internal/mutants"
)

// Tests describes one fixture's real command, keyed by directory and command.
// It is fixture configuration, not a sequence of expected calls or events.
type Tests struct {
	Baseline string
	Elapsed  time.Duration
	Verdicts []Verdict
	Trace    *Trace
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
}

// Workers holds the actual virtual tree and each directory/command declaration.
// Write, when set, publishes trace bytes through the driver's actual file writer.
type Workers struct {
	Files map[string]string
	Tests map[string]map[string]Tests
	Write func(path string, data []byte) error
	mu    sync.Mutex
	next  int
}

var _ mutants.Workers = (*Workers)(nil)

func (f *Workers) Open(ctx context.Context, purpose mutants.WorkPurpose, count int) (mutants.WorkerPool, mutants.WorkerFailure) {
	if ctx.Err() != nil {
		return nil, mutants.WorkerCancelled{Cause: context.Cause(ctx)}
	}
	if purpose != mutants.MutationRun && purpose != mutants.FrozenTrials {
		return nil, failed("unknown worker purpose")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.next++
	p := &pool{factory: f}
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
	factory *Workers
	workers []*worker
	closed  bool
}

func (p *pool) Worker(index int) mutants.Worker { return p.workers[index] }
func (p *pool) Abort()                          {} // no physical cleanup clock; actual context cancellation governs requests
func (p *pool) Close(context.Context) mutants.WorkerFailure {
	p.factory.mu.Lock()
	defer p.factory.mu.Unlock()
	p.closed = true
	for _, w := range p.workers {
		w.copies = nil
	}
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
	f := w.pool.factory
	f.mu.Lock()
	defer f.mu.Unlock()
	if w.pool.closed {
		return mutants.CommandResult{}, failed("worker pool is closed")
	}
	tests, ok := f.Tests[filepath.Clean(c.Dir)][c.String()]
	if !ok {
		return mutants.CommandResult{}, failed("no tests declared for " + c.Dir + "$ " + c.String())
	}
	root := filepath.Clean(c.Root)
	copy := w.copies[root]
	if copy == nil {
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
	if tests.Trace != nil {
		if err := w.trace(c, copy, tests.Trace); err != nil {
			return mutants.CommandResult{}, mutants.WorkerFailed{Cause: err}
		}
	}
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

func (w *worker) trace(c mutants.Command, copy map[string]string, t *Trace) error {
	source := filepath.Join(c.Root, filepath.FromSlash(t.Source))
	text, ok := copy[source]
	if !ok {
		return fmt.Errorf("trace source is absent: %s", source)
	}
	names, err := selectedTests(c, t.Tests)
	if err != nil {
		return err
	}
	hash := sha256.Sum256([]byte(text))
	rel, err := filepath.Rel(c.Root, c.Dir)
	if err != nil {
		return err
	}
	cwd := filepath.Join(w.dir, rel)
	if cwd == filepath.Clean(c.Root) {
		return errors.New("virtual worker is the original root")
	}
	for _, name := range names {
		record := map[string]string{"cwd": cwd, "original_root": filepath.Clean(c.Root), "source_sha256": hex.EncodeToString(hash[:])}
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
		f := w.pool.factory
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

// selectedTests supports the concrete runnable selector shapes declared by the
// fixture inventory. Unsupported requests error rather than silently emitting
// no observations. A broad command executes the entire inventory.
func selectedTests(c mutants.Command, inventory map[string]string) ([]string, error) {
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
	// The Python runner's non-option operands are actual test selectors. A
	// misspelled file/module must not silently broaden to the whole inventory.
	if len(selectors) == 0 && !whole {
		module := -1
		for i := 0; i+1 < len(args); i++ {
			if args[i] == "-m" && (args[i+1] == "pytest" || args[i+1] == "unittest") {
				module = i + 1
				break
			}
		}
		if module >= 0 {
			for i := module + 1; i < len(args); i++ {
				a := args[i]
				if a == "-p" || a == "-k" {
					i++
					continue
				}
				if !strings.HasPrefix(a, "-") {
					selectors = append(selectors, a)
				}
			}
			whole = len(selectors) == 0
		} else {
			// With no explicit class selector, these runners execute the suite.
			for _, a := range args {
				if a == "gradle" || a == "mvn" {
					whole = true
				}
			}
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
