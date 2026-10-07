package mutate

import (
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/donvargax/itos-cc/coverage"
	"github.com/donvargax/itos-cc/lang"
	"github.com/donvargax/itos-cc/metrics"
	"github.com/donvargax/itos-cc/project"
)

// Options control a mutation run.
type Options struct {
	Workers       int
	MutateAll     bool    // rerun killed mutants of unchanged functions too
	TimeoutFactor float64 // a mutant may run this many times the baseline
	TestCommand   string  // shell command that replaces the per-language default
	AllTests      bool    // kill mutants with the whole suite, integration and end-to-end tests included
	Annotate      bool    // write a summary comment at the end of each file
	// Coverage is called only when some mutant has to run. It returns nil
	// to run every mutant regardless of coverage.
	Coverage func(sources []string) *coverage.Report
	Log      io.Writer
}

// FileResult is the outcome for one source file.
type FileResult struct {
	Rel            string
	Snapshot       Snapshot
	Ran, Reused    int
	BaselineFailed bool
	BaselineOutput string
}

// minTimeout keeps fast suites from timing out on scheduling noise.
const minTimeout = 2 * time.Second

type fileState struct {
	file     *lang.File
	rel      string
	sites    []Site
	outcomes []string // "" while pending
	command  Command
	result   *FileResult
}

// Run mutates files and writes their snapshots. It returns one result per
// file, in the order given.
func Run(files []string, opt Options) ([]FileResult, error) {
	states, err := plan(files, opt)
	defer func() {
		for _, s := range states {
			s.file.Close()
		}
	}()
	if err != nil {
		return nil, err
	}

	pending := 0
	for _, s := range states {
		for _, o := range s.outcomes {
			if o == "" {
				pending++
			}
		}
	}
	if pending > 0 {
		markUncovered(states, opt)
		if err := execute(states, opt); err != nil {
			return nil, err
		}
	} else {
		fmt.Fprintln(opt.Log, "mutate: no mutations to test")
	}

	var results []FileResult
	for _, s := range states {
		if !s.result.BaselineFailed {
			s.result.Snapshot = build(s.file, s.rel, s.sites, s.outcomes)
			if err := metrics.Write(SnapshotName(s.rel), s.result.Snapshot); err != nil {
				return nil, err
			}
			if opt.Annotate {
				if err := annotate(s.file.Path, s.file.Spec, s.result.Snapshot); err != nil {
					return nil, err
				}
			}
		}
		results = append(results, *s.result)
	}
	return results, nil
}

// plan parses each file, finds its sites, and keeps the outcomes the
// previous snapshot allows.
func plan(files []string, opt Options) ([]*fileState, error) {
	var states []*fileState
	for _, path := range files {
		f, err := lang.ParseFile(path)
		if err != nil {
			return states, err
		}
		rel := project.Rel(path)
		s := &fileState{file: f, rel: rel, sites: Sites(f), command: TestCommand(path, opt.TestCommand, opt.AllTests),
			result: &FileResult{Rel: rel}}
		states = append(states, s)
		snap, err := LoadSnapshot(rel)
		if err != nil {
			return states, fmt.Errorf("%s: %w", SnapshotName(rel), err)
		}
		prev := remembered(snap, f)
		s.outcomes = make([]string, len(s.sites))
		for i, site := range s.sites {
			if outcome, ok := prev.kept(f, site); ok && !opt.MutateAll {
				s.outcomes[i] = outcome
				s.result.Reused++
			}
		}
	}
	return states, nil
}

// markUncovered settles pending sites on lines no test executes. A file the
// report never mentions is uncovered when the report measured other files
// of its language: the tests ran and never loaded it.
func markUncovered(states []*fileState, opt Options) {
	if opt.Coverage == nil {
		return
	}
	var sources []string
	for _, s := range states {
		sources = append(sources, s.file.Path)
	}
	report := opt.Coverage(sources)
	if report == nil {
		return
	}
	measured := map[string]bool{}
	for _, s := range states {
		if report.Has(s.file.Path) {
			measured[s.file.Spec.Name] = true
		}
	}
	for _, s := range states {
		has := report.Has(s.file.Path)
		if !has && !measured[s.file.Spec.Name] {
			fmt.Fprintf(opt.Log, "mutate: no coverage for %s; running every mutant\n", s.rel)
			continue
		}
		for i, site := range s.sites {
			if s.outcomes[i] != "" {
				continue
			}
			covered, lineMeasured := report.LineCovered(s.file.Path, site.Line)
			if !has || (lineMeasured && !covered) {
				s.outcomes[i] = Uncovered
			}
		}
	}
}

type job struct {
	state *fileState
	site  int
}

// execute runs each command's baseline, then every pending mutant across
// the workers.
func execute(states []*fileState, opt Options) error {
	base, err := os.MkdirTemp("", "itos-cc-mutate-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(base)
	workers := make([]*worker, max(1, opt.Workers))
	for i := range workers {
		workers[i] = &worker{dir: fmt.Sprintf("%s/w%d", base, i), copies: map[string]string{}}
	}

	// One baseline per distinct command, run inside a worker's copy: passing
	// proves the copy is a working tree, and its time sets the timeout.
	timeouts := map[string]time.Duration{}
	failed := map[string]string{}
	var jobs []job
	for _, s := range states {
		for i, o := range s.outcomes {
			if o != "" {
				continue
			}
			key := s.command.Key()
			if _, done := timeouts[key]; !done && failed[key] == "" {
				fmt.Fprintf(opt.Log, "mutate: baseline %s$ %s\n", project.Rel(s.command.Dir), s.command)
				r, err := workers[0].run(s.command, 0)
				if err != nil {
					return err
				}
				if !r.passed {
					failed[key] = r.output
				} else {
					timeouts[key] = max(minTimeout, time.Duration(float64(r.elapsed)*opt.TimeoutFactor))
				}
			}
			if out, bad := failed[key]; bad {
				s.result.BaselineFailed, s.result.BaselineOutput = true, out
				continue
			}
			jobs = append(jobs, job{s, i})
		}
	}

	queue := make(chan job)
	var mu sync.Mutex
	var firstErr error
	done := 0
	var wg sync.WaitGroup
	for _, w := range workers {
		wg.Add(1)
		go func(w *worker) {
			defer wg.Done()
			for j := range queue {
				outcome, elapsed, err := runMutant(w, j, timeouts[j.state.command.Key()])
				mu.Lock()
				if err != nil && firstErr == nil {
					firstErr = err
				}
				j.state.outcomes[j.site] = outcome
				j.state.result.Ran++
				done++
				site := j.state.sites[j.site]
				fmt.Fprintf(opt.Log, "mutate: [%d/%d] %s:%d %s → %s %s (%.1fs)\n", done, len(jobs),
					j.state.rel, site.Line, show(site.Original), show(site.Replacement), outcome, elapsed.Seconds())
				mu.Unlock()
			}
		}(w)
	}
	for _, j := range jobs {
		queue <- j
	}
	close(queue)
	wg.Wait()
	return firstErr
}

func runMutant(w *worker, j job, timeout time.Duration) (string, time.Duration, error) {
	s := j.state
	site := s.sites[j.site]
	r, err := w.withMutant(s.command.Root, s.file.Path, s.file.Src, site.Apply(s.file.Src), func() (result, error) {
		return w.run(s.command, timeout)
	})
	switch {
	case err != nil:
		return "", r.elapsed, err
	case r.timedOut:
		return Timeout, r.elapsed, nil
	case r.passed:
		return Survived, r.elapsed, nil
	}
	return Killed, r.elapsed, nil
}

// show renders an empty replacement, a deleted operator, visibly.
func show(s string) string {
	if s == "" {
		return "(deleted)"
	}
	return "`" + s + "`"
}
