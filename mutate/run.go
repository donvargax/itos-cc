package mutate

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"github.com/donvargax/itos-cc/config"
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
	// Judge, when set, says which functions of the file at path to judge,
	// by namespace#name. The others never run: they keep what their
	// snapshot records.
	Judge func(path, function string) bool
	// Renamed, when set, is the path the file at path had before a rename,
	// or "": its snapshot moves to the new path, and the old one is removed.
	Renamed func(path string) string
	// Tests, when set, lists the test files that import the file at path,
	// whose hashes its snapshot records: a kill is reused only while they
	// are those recorded. Unset, a file has none.
	Tests func(path string) []string
	// Exceptions are the survivors itos-cc.yaml excepts: each one of a
	// function judged that still survives is excepted, and one that no
	// longer holds is stale.
	Exceptions []config.Exception
	Log        io.Writer
}

// FileResult is the outcome for one source file.
type FileResult struct {
	Rel string
	// Snapshot is what the run records of the file, written to .metrics
	// unless the baseline failed or no function was judged.
	Snapshot       Snapshot
	Ran, Reused    int
	BaselineFailed bool
	BaselineOutput string
	// Judged holds the namespace#name of each function judged, in the
	// file's order, or is nil when every function was. Functions is how
	// many the file has.
	Judged    []string
	Functions int
	// Mutants is every mutant of the functions judged, in site order (line,
	// then column), with its outcome; empty when the baseline failed.
	Mutants []MutantResult
	// StaleExceptions is each exception of the functions judged that no
	// longer holds: those whose function changed or whose site is gone,
	// then those whose mutant the tests now kill.
	StaleExceptions []StaleException
}

// MutantResult is how one mutant was decided in this run.
type MutantResult struct {
	Site
	Function string // namespace#name
	Outcome  string // Killed, Survived, Timeout, or Uncovered
	// Reused is true when the outcome came from the snapshot without
	// running.
	Reused bool
	// Excepted is the reason itos-cc.yaml gives when it excepts the mutant
	// and the mutant survived; empty otherwise.
	Excepted string
}

// skipped marks a site of a function not judged that has no outcome to keep:
// it neither runs nor goes into the snapshot.
const skipped = "skipped"

// minTimeout keeps fast suites from timing out on scheduling noise.
const minTimeout = 2 * time.Second

type fileState struct {
	file     *lang.File
	rel      string
	sites    []Site
	outcomes []string // "" while pending
	reused   []bool   // the outcome came from the previous snapshot
	command  Command
	result   *FileResult
	previous *Snapshot         // the snapshot before this run, or nil when there is none or its tests differ
	tests    map[string]string // the tests that import the file, as the snapshot records them
	judged   map[string]bool   // by namespace#name, or nil when every function is
	excepted fileExceptions    // the exceptions of the functions judged
	moved    string            // the path the file had before a rename, whose snapshot moves to rel
	moving   *Snapshot         // that snapshot, under rel, when rel had none of its own
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
		fmt.Fprintln(opt.Log, "itos-cc: no mutations to test")
	}

	var results []FileResult
	for _, s := range states {
		if !s.result.BaselineFailed {
			s.result.Mutants = s.decided()
			s.result.Snapshot = build(s.file, s.rel, s.tests, s.sites, s.outcomes)
			if s.judged != nil {
				s.result.Snapshot.Units = keepUnjudged(s.result.Snapshot.Units, s.judged, s.previous)
			}
			markExcepted(s.result.Snapshot.Units, s.result.Mutants)
			if s.judged != nil && len(s.judged) == 0 {
				// Nothing in the file was judged, so nothing ran and the
				// file is left as it was: no new results, no comment. A
				// renamed file's snapshot still moves, as it was.
				if s.moving != nil {
					if err := metrics.Write(SnapshotName(s.rel), s.moving); err != nil {
						return nil, err
					}
				}
				if err := s.removeMoved(); err != nil {
					return nil, err
				}
				results = append(results, *s.result)
				continue
			}
			if err := metrics.Write(SnapshotName(s.rel), s.result.Snapshot); err != nil {
				return nil, err
			}
			if err := s.removeMoved(); err != nil {
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

// follow takes the snapshot of a file renamed from a path that holds no
// file now: as the file's own when it has none yet, as *snap. The old
// snapshot is removed once the new one is written.
func (s *fileState) follow(opt Options, path string, snap **Snapshot) error {
	if opt.Renamed == nil {
		return nil
	}
	old := opt.Renamed(path)
	if old == "" {
		return nil
	}
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		return nil
	}
	s.moved = project.Rel(old)
	if *snap != nil {
		return nil
	}
	moving, err := LoadSnapshot(s.moved)
	if err != nil {
		return fmt.Errorf("%s: %w", SnapshotName(s.moved), err)
	}
	if moving != nil {
		moving.File = filepath.ToSlash(s.rel)
		*snap, s.moving = moving, moving
	}
	return nil
}

// removeMoved removes the snapshot of the path the file had before a
// rename, if any.
func (s *fileState) removeMoved() error {
	if s.moved == "" {
		return nil
	}
	return metrics.Remove(SnapshotName(s.moved))
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
		if err := s.follow(opt, path, &snap); err != nil {
			return states, err
		}
		var tests []string
		if opt.Tests != nil {
			tests = opt.Tests(path)
		}
		if s.tests, err = TestHashes(tests); err != nil {
			return states, err
		}
		s.previous = usable(snap, s.tests)
		prev := remembered(s.previous, f)
		s.result.Functions = len(f.Units)
		if opt.Judge != nil {
			s.judged, s.result.Judged = map[string]bool{}, []string{}
			for _, u := range f.Units {
				if id := unitID(u.Namespace, u.Name); opt.Judge(path, id) {
					s.judged[id] = true
					s.result.Judged = append(s.result.Judged, id)
				}
			}
		}
		var judge func(function string) bool
		if s.judged != nil {
			judge = func(function string) bool { return s.judged[function] }
		}
		s.excepted = applyExceptions(opt.Exceptions, rel, f, s.sites, judge)
		s.result.StaleExceptions = s.excepted.stale
		s.outcomes = make([]string, len(s.sites))
		s.reused = make([]bool, len(s.sites))
		for i, site := range s.sites {
			u := f.Units[site.Unit]
			if id := unitID(u.Namespace, u.Name); s.judged != nil && !s.judged[id] {
				// Not judged: it never runs, and keeps the outcome its
				// snapshot records for its unchanged function, if any.
				s.outcomes[i] = skipped
				if outcome := prev[id][site.Key()]; outcome != "" {
					s.outcomes[i] = outcome
				}
				continue
			}
			_, excepted := s.excepted.held[i]
			if outcome, ok := prev.kept(f, site, excepted); ok && !opt.MutateAll {
				s.outcomes[i] = outcome
				s.reused[i] = true
				s.result.Reused++
			}
		}
	}
	return states, nil
}

// decided lists the mutants of the functions judged with their outcomes, in
// site order, each excepted survivor with its reason. A function not judged
// was not decided in this run, whatever its snapshot keeps for it. An
// exception whose mutant the tests noticed is added to the result's stale
// ones.
func (s *fileState) decided() []MutantResult {
	out := []MutantResult{}
	for i, site := range s.sites {
		u := s.file.Units[site.Unit]
		id := unitID(u.Namespace, u.Name)
		if s.judged != nil && !s.judged[id] {
			continue
		}
		reason, stale := s.excepted.judge(i, site, s.outcomes[i])
		if stale != nil {
			s.result.StaleExceptions = append(s.result.StaleExceptions, *stale)
		}
		out = append(out, MutantResult{Site: site, Function: id, Outcome: s.outcomes[i], Reused: s.reused[i], Excepted: reason})
	}
	slices.SortStableFunc(out, func(a, b MutantResult) int { return LineOrder(a.Site, b.Site) })
	return out
}

// markUncovered settles pending sites on lines no test executes. A file the
// report never mentions is uncovered when coverage measured its language:
// a coverage command for it succeeded, even with a report that names no file
// of the run, or the report measured other files of the run in it. The tests
// ran and never loaded it.
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
		if report.Has(s.file.Path) || report.Measures(s.file.Spec.Name) {
			measured[s.file.Spec.Name] = true
		}
	}
	for _, s := range states {
		has := report.Has(s.file.Path)
		if !has && !measured[s.file.Spec.Name] {
			fmt.Fprintf(opt.Log, "itos-cc: no coverage for %s; running every mutant\n", s.rel)
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
				fmt.Fprintf(opt.Log, "itos-cc: baseline %s$ %s\n", project.Rel(s.command.Dir), s.command)
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
				fmt.Fprintf(opt.Log, "itos-cc: [%d/%d] %s:%d %s → %s %s (%.1fs)\n", done, len(jobs),
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
