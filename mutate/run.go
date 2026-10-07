package mutate

import (
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
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
	// to run every mutant regardless of coverage, and an error to stop the
	// run before any mutant runs or any snapshot is written.
	Coverage func(sources []string) (*coverage.Report, error)
	// Support is the hashes of the support files of the listed tests now
	// (SupportHashes): what a listed outcome records, and holds while they
	// are unchanged.
	Support map[string]string
	// Listed, when set, runs the listed tests whose coverage reaches a
	// mutant's line (coverage.Report.LineTests) once the file's own tests
	// survived it.
	Listed *Listed
	// Judge, when set, says which functions of the file at path to judge,
	// by namespace#name and, to tell apart functions sharing a name, hash
	// (UnitHash). The others never run: they keep what their snapshot
	// records.
	Judge func(path, function, hash string) bool
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

	// listedTimeout is how long a run of listed tests may take, or 0 while
	// no run of them has been timed.
	listedTimeout time.Duration
}

// Listed is how to run a selection of the tests a project lists in
// itos-cc.yaml (mutation.tests).
type Listed struct {
	// Root is where the commands run: the project root.
	Root string
	// Select is the shell command line that runs the tests ids.
	Select func(ids []string) string
	// Files is the file that defines each test, by its ID, as the list
	// command names it, "" for none: what a snapshot records of the tests
	// its outcomes name.
	Files map[string]string
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
	// Scope is the scope of the tests that decided Outcome: this run's,
	// or, reused, the one its snapshot records. See RunScope.
	Scope string
	// Excepted is the reason itos-cc.yaml gives when it excepts the mutant
	// and the mutant survived; empty otherwise.
	Excepted string
	// Coverage names the sources of this run's coverage that executed the
	// mutant's line, "in-process", "integration", or both; empty when
	// coverage was not measured or did not execute it.
	Coverage []string
	// Tests is, with Scope ScopeListed, the IDs of the listed tests that
	// decided Outcome.
	Tests []string
}

// skipped marks a site of a function not judged that has no outcome to keep:
// it neither runs nor goes into the snapshot.
const skipped = "skipped"

// minTimeout keeps fast suites from timing out on scheduling noise.
const minTimeout = 2 * time.Second

type fileState struct {
	file     *lang.File
	rel      string // the file's path as output names it, from the working directory
	key      string // the file's path as its snapshot names it, from the project root
	sites    []Site
	outcomes []string   // "" while pending
	reused   []bool     // the outcome came from the previous snapshot
	scopes   []string   // the scope that decided each outcome, or decides it in this run
	coverage [][]string // the sources of coverage that executed each site's line, or nil
	reach    [][]string // the listed tests that execute each site's line, or nil
	ran      [][]string // the listed tests that decided each outcome of ScopeListed
	// listedChange is how the files the stored snapshot's listed outcomes
	// rest on changed since, or nil.
	listedChange *ListedChange
	command      Command
	result       *FileResult
	previous     *Snapshot         // the snapshot before this run, or nil when there is none or its tests differ
	stored       *Snapshot         // the snapshot before this run, whatever tests it records, or nil when there is none
	tests        map[string]string // the tests that import the file, as the snapshot records them
	judged       map[int]bool      // by the index of the unit, or nil when every function is
	excepted     fileExceptions    // the exceptions of the functions judged
	moved        string            // the path from the root the file had before a rename, whose snapshot moves to key
	moving       *Snapshot         // that snapshot, under key, when key had none of its own
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
		if err := markUncovered(states, &opt); err != nil {
			return nil, err
		}
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
			s.result.Snapshot = buildScoped(s.file, s.key, s.tests, s.sites, s.outcomes, s.scopes, s.ran)
			if s.judged != nil {
				s.result.Snapshot.Units = keepUnjudged(s.result.Snapshot.Units, s.judged, s.stored, s.previous != nil)
			}
			var files map[string]string
			if opt.Listed != nil {
				files = opt.Listed.Files
			}
			s.result.Snapshot.Listed = listedOf(s.result.Snapshot.Units, files, s.stored)
			s.markListedStale()
			s.result.Snapshot.recordListed(project.Root(), opt.Support)
			markExcepted(s.result.Snapshot.Units, s.result.Mutants)
			if s.judged != nil && len(s.judged) == 0 {
				// Nothing in the file was judged, so nothing ran and the
				// file is left as it was: no new results, no comment. A
				// renamed file's snapshot still moves, as it was.
				if s.moving != nil {
					if err := metrics.Write(SnapshotName(s.key), s.moving); err != nil {
						return nil, err
					}
				}
				if err := s.removeMoved(); err != nil {
					return nil, err
				}
				results = append(results, *s.result)
				continue
			}
			if err := metrics.Write(SnapshotName(s.key), s.result.Snapshot); err != nil {
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
	s.moved = project.FromRoot(old)
	if *snap != nil {
		return nil
	}
	moving, err := LoadSnapshot(s.moved)
	if err != nil {
		return fmt.Errorf("%s: %w", SnapshotName(s.moved), err)
	}
	if moving != nil {
		moving.File = s.key
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
		rel, key := project.Rel(path), project.FromRoot(path)
		s := &fileState{file: f, rel: rel, key: key, sites: Sites(f), command: TestCommand(path, opt.TestCommand, opt.AllTests),
			result: &FileResult{Rel: rel}}
		states = append(states, s)
		snap, err := LoadSnapshot(key)
		if err != nil {
			return states, fmt.Errorf("%s: %w", SnapshotName(key), err)
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
		s.stored, s.previous = snap, usable(snap, s.tests)
		prev, prevScopes := rememberedWithScopes(s.previous, f)
		s.listedChange = s.stored.ListedChanged(project.Root(), opt.Support)
		s.result.Functions = len(f.Units)
		var judge func(function string) bool
		if opt.Judge != nil {
			s.judged, s.result.Judged = map[int]bool{}, []string{}
			names := map[string]bool{}
			for i, u := range f.Units {
				if id := unitID(u.Namespace, u.Name); opt.Judge(path, id, UnitHash(f, u)) {
					s.judged[i], names[id] = true, true
					s.result.Judged = append(s.result.Judged, id)
				}
			}
			// An exception names its function, so it counts when any
			// function of that name is judged.
			judge = func(function string) bool { return names[function] }
		}
		s.excepted = applyExceptions(opt.Exceptions, key, f, s.sites, judge)
		s.result.StaleExceptions = s.excepted.stale
		s.outcomes = make([]string, len(s.sites))
		s.reused = make([]bool, len(s.sites))
		s.scopes = make([]string, len(s.sites))
		s.ran = make([][]string, len(s.sites))
		scope := RunScope(opt.TestCommand, opt.AllTests)
		for i, site := range s.sites {
			// A kept outcome keeps the scope it was decided with.
			s.scopes[i] = scope
			if s.judged != nil && !s.judged[site.Unit] {
				// Not judged: it never runs, and keeps the outcome its
				// snapshot records for its unchanged function, if any.
				s.outcomes[i] = skipped
				if outcome := prev[site.Unit][site.Key()]; outcome != "" {
					d := prevScopes[site.Unit][site.Key()]
					s.outcomes[i], s.scopes[i], s.ran[i] = outcome, d.scope, d.tests
				}
				continue
			}
			_, excepted := s.excepted.held[i]
			d := prevScopes[site.Unit][site.Key()]
			// A listed outcome holds while the files it rests on do.
			restsOnChange := len(s.listedChange.files([]Mutant{{Scope: d.scope, Tests: d.tests}})) > 0
			if outcome, ok := prev.kept(f, site, excepted); ok && !opt.MutateAll && !restsOnChange {
				s.outcomes[i], s.scopes[i], s.ran[i] = outcome, d.scope, d.tests
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
		if s.judged != nil && !s.judged[site.Unit] {
			continue
		}
		reason, stale := s.excepted.judge(i, site, s.outcomes[i])
		if stale != nil {
			s.result.StaleExceptions = append(s.result.StaleExceptions, *stale)
		}
		var covered []string
		if s.coverage != nil && s.outcomes[i] != Uncovered {
			covered = s.coverage[i]
		}
		var ran []string
		if s.scopes[i] == ScopeListed {
			ran = s.ran[i]
		}
		out = append(out, MutantResult{Site: site, Function: id, Outcome: s.outcomes[i], Reused: s.reused[i], Scope: s.scopes[i],
			Excepted: reason, Coverage: covered, Tests: ran})
	}
	slices.SortStableFunc(out, func(a, b MutantResult) int { return LineOrder(a.Site, b.Site) })
	return out
}

// markUncovered settles pending sites on lines no test executes, and records
// which sources of coverage, and which listed tests, executed each site's
// line. A file the report never mentions is uncovered when coverage
// measured its language: a coverage command for it succeeded, even with a
// report that names no file of the run, or the report measured other files
// of the run in it. The tests ran and never loaded it. A line a listed test
// executes is never uncovered. It times the listed tests' runs from their
// coverage run.
func markUncovered(states []*fileState, opt *Options) error {
	if opt.Coverage == nil {
		return nil
	}
	var sources []string
	for _, s := range states {
		sources = append(sources, s.file.Path)
	}
	report, err := opt.Coverage(sources)
	if err != nil || report == nil {
		return err
	}
	if elapsed := report.TestsElapsed(); elapsed > 0 {
		opt.listedTimeout = max(minTimeout, time.Duration(float64(elapsed)*opt.TimeoutFactor))
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
		s.coverage = make([][]string, len(s.sites))
		s.reach = make([][]string, len(s.sites))
		for i, site := range s.sites {
			s.coverage[i] = report.LineSources(s.file.Path, site.Line)
			if opt.Listed != nil {
				s.reach[i] = report.LineTests(s.file.Path, site.Line)
			}
			if s.outcomes[i] != "" || len(s.reach[i]) > 0 {
				continue
			}
			covered, lineMeasured := report.LineCovered(s.file.Path, site.Line)
			if !has || (lineMeasured && !covered) {
				s.outcomes[i] = Uncovered
			}
		}
	}
	return nil
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

	jobs, err = listedBaseline(workers[0], jobs, &opt)
	if err != nil {
		return err
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
				outcome, elapsed, ran, err := runMutant(w, j, timeouts[j.state.command.Key()], opt)
				mu.Lock()
				if err != nil && firstErr == nil {
					firstErr = err
				}
				j.state.outcomes[j.site] = outcome
				said := outcome
				if ran != nil {
					j.state.scopes[j.site], j.state.ran[j.site] = ScopeListed, ran
					said = fmt.Sprintf("survived its own tests, then %s with %s", outcome, strings.Join(ran, " "))
				}
				j.state.result.Ran++
				done++
				site := j.state.sites[j.site]
				fmt.Fprintf(opt.Log, "itos-cc: [%d/%d] %s:%d %s → %s %s (%.1fs)\n", done, len(jobs),
					j.state.rel, site.Line, show(site.Original), show(site.Replacement), said, elapsed.Seconds())
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

// runMutant runs the file's own tests on the mutant, then, when it survives
// them, the listed tests that reach its line, whose IDs it returns.
func runMutant(w *worker, j job, timeout time.Duration, opt Options) (string, time.Duration, []string, error) {
	s := j.state
	site := s.sites[j.site]
	mutated := site.Apply(s.file.Src)
	r, err := w.withMutant(s.command.Root, s.file.Path, s.file.Src, mutated, func() (result, error) {
		return w.run(s.command, timeout)
	})
	outcome := judged(r)
	if err != nil || outcome != Survived || opt.Listed == nil || s.reach == nil || len(s.reach[j.site]) == 0 {
		return outcome, r.elapsed, nil, err
	}
	ids := s.reach[j.site]
	listed, err := w.withMutant(opt.Listed.Root, s.file.Path, s.file.Src, mutated, func() (result, error) {
		return w.run(opt.Listed.command(ids), opt.listedTimeout)
	})
	return judged(listed), r.elapsed + listed.elapsed, ids, err
}

// judged is the outcome of a test run on a mutant.
func judged(r result) string {
	switch {
	case r.timedOut:
		return Timeout
	case r.passed:
		return Survived
	}
	return Killed
}

// command runs the listed tests ids.
func (l *Listed) command(ids []string) Command {
	return Command{Root: l.Root, Dir: l.Root, Shell: l.Select(ids)}
}

// listedBaseline runs, unless coverage timed them already, the listed tests
// any job may run, with no mutant, in w's copy: passing proves they hold,
// and their time sets the timeout. When they fail, the files whose mutants
// would run them are not judged, as with a failing baseline, and their jobs
// are dropped.
func listedBaseline(w *worker, jobs []job, opt *Options) ([]job, error) {
	if opt.Listed == nil || opt.listedTimeout > 0 {
		return jobs, nil
	}
	var ids []string
	for _, j := range jobs {
		if j.state.reach != nil {
			for _, id := range j.state.reach[j.site] {
				if !slices.Contains(ids, id) {
					ids = append(ids, id)
				}
			}
		}
	}
	if len(ids) == 0 {
		return jobs, nil
	}
	c := opt.Listed.command(ids)
	fmt.Fprintf(opt.Log, "itos-cc: baseline %s$ %s\n", project.Rel(c.Dir), c)
	r, err := w.run(c, 0)
	if err != nil {
		return nil, err
	}
	if r.passed {
		opt.listedTimeout = max(minTimeout, time.Duration(float64(r.elapsed)*opt.TimeoutFactor))
		return jobs, nil
	}
	var kept []job
	for _, j := range jobs {
		if j.state.reach == nil || len(j.state.reach[j.site]) == 0 {
			kept = append(kept, j)
			continue
		}
		j.state.result.BaselineFailed, j.state.result.BaselineOutput = true, r.output
	}
	return kept, nil
}

// show renders an empty replacement, a deleted operator, visibly.
func show(s string) string {
	if s == "" {
		return "(deleted)"
	}
	return "`" + s + "`"
}
