package mutants

import (
	"context"
	"slices"
	"sync"
	"time"

	"github.com/donvargax/itos-cc/internal/coverage"
	"github.com/donvargax/itos-cc/internal/lang"
	langport "github.com/donvargax/itos-cc/internal/lang/port"
	selected "github.com/donvargax/itos-cc/internal/selection"
	selport "github.com/donvargax/itos-cc/internal/selection/port"
	"github.com/donvargax/itos-cc/internal/snapshot"
)

// Options control a mutation run.
type Options struct {
	// Store holds the project's snapshots: each file's is read from it, and
	// written back to it, under the run's context.
	Store        MutationSnapshots
	Root, Dir    string
	Tree         selport.Tree
	Surroundings langport.Surroundings
	Source       SourceEdits
	// WorkerFactory owns private command copies; cmd supplies the real binding.
	WorkerFactory Workers
	// Toolchain and Fingerprints are required actual observation bindings.
	// Compatibility composition stays outside the domain.
	Toolchain     Toolchain
	Fingerprints  Fingerprints
	Workers       int
	MutateAll     bool    // rerun killed mutants of unchanged functions too
	TimeoutFactor float64 // a mutant may run this many times the baseline
	TestCommand   string  // shell command that replaces the per-language default
	AllTests      bool    // kill mutants with the whole suite, integration and end-to-end tests included
	Annotate      bool    // write a summary comment at the end of each file
	// Coverage is an already admitted report; Measurement requests lazy collection
	// only when pending work requires it. Neither carries a live I/O closure.
	Coverage    *coverage.Report
	Measurement *RunMeasurement
	// StatementCoverage is a successful independently admitted coverage
	// measurement for strict mode, of every strict language
	// (StrictCoverage). It is attached per unit of a file of such a
	// language, including units with no mutation sites.
	StatementCoverage    *coverage.Report
	CoverageProducer     string
	RecordCoverageInputs bool
	CachedCoverage       func(path, function, hash string) *CoverageEvidence
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
	// Tests, when set, lists the test files that reach the file at path
	// (graph.TestsImporting), whose hashes its snapshot records: a kill is
	// reused only while they are those recorded. In Python they are also
	// the tests its own command runs (TestCommand). Unset, a file has none.
	Tests func(path string) []string
	// Exceptions are the survivors itos-cc.yaml excepts: each one of a
	// function judged that still survives is excepted, and one that no
	// longer holds is stale.
	Exceptions []Exception
	Progress   Progress
}

// Listed is how to run a selection of the tests a project lists in
// itos-cc.yaml (mutation.tests).
type Listed struct {
	// Root is where the commands run: the project root.
	Root string
	List string
	All  func(ids []string) string
	// Select is the shell command line that runs the tests ids.
	Select func(ids []string) string
	// Files is the file that defines each test, by its ID, as the list
	// command names it, "" for none: what a snapshot records of the tests
	// its outcomes name.
	Files map[string]string
}

type Execution struct {
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
	listedChange   *ListedChange
	command        Command
	result         *FileResult
	previous       *Snapshot         // the snapshot before this run, or nil when there is none or its tests differ
	stored         *Snapshot         // the snapshot before this run, whatever tests it records, or nil when there is none
	tests          map[string]string // the tests that import the file, as the snapshot records them
	moduleTests    map[string]string // every test beneath the source's Build root (SuiteTestHashes)
	currentSupport map[string]string
	judged         map[int]bool   // by the index of the unit, or nil when every function is
	excepted       FileExceptions // the exceptions of the functions judged
	moved          string         // the path from the root the file had before a rename, whose snapshot moves to key
	moving         *Snapshot      // that snapshot, under key, when key had none of its own
	// failFast is set in a fail-fast run, and undecided holds, there, the
	// state of each site the run left with no outcome, StateCancelled or
	// StateBlocked, or "" (see stateOf).
	failFast  bool
	undecided []string
	// fresh is, by site of a function judged, the outcome the snapshot
	// before this run records for it that still holds, whether or not this
	// run reuses it, with what decided it; zero where none does. A
	// fail-fast run keeps it for a site it left undecided (preserved).
	fresh []freshOutcome
}

// freshOutcome is an outcome a snapshot records that still holds: the
// function's hash, the tests that import its file and the files its listed
// or broad evidence rests on are as they were.
type freshOutcome struct {
	outcome, scope string
	tests          []string
}

// Run mutates files and writes their snapshots. It returns one FileResult per
// file, in the order given.
func Run(ctx context.Context, files []string, opt Options) ([]FileResult, error) {
	results, _, err := run(ctx, files, opt, nil)
	return results, err
}

// RunFailFast is Run stopping at the first actionable final judgment, as
// ff says. It returns the stop too, nil when nothing stopped the run
// before every selected mutant was decided. Each FileResult's Baseline is set,
// and each mutant listed has its State: a file the stop cut short lists
// the mutants of its functions judged, decided or not.
func RunFailFast(ctx context.Context, files []string, opt Options, ff FailFast) ([]FileResult, *Stop, error) {
	return run(ctx, files, opt, &ff)
}

func run(ctx context.Context, files []string, opt Options, ff *FailFast) ([]FileResult, *Stop, error) {
	if ctx.Err() != nil {
		return nil, nil, context.Cause(ctx)
	}
	if err := opt.bound(); err != nil {
		return nil, nil, err
	}
	states, err := plan(ctx, files, opt)
	defer func() {
		for _, s := range states {
			s.file.Close()
		}
	}()
	if err = stopped(ctx, err); err != nil {
		return nil, nil, err
	}

	pending := 0
	for _, s := range states {
		s.failFast = ff != nil
		for _, o := range s.outcomes {
			if o == "" {
				pending++
			}
		}
	}
	var stop *Stop
	if pending > 0 {
		// A failure known before any mutant runs stops a fail-fast run
		// before the work it makes pointless: a stale exception before
		// coverage, an uncovered mutant or strict coverage finding before any
		// baseline or mutant.
		if ff != nil {
			if stop = ff.Known; stop == nil {
				stop = knownStop(states)
			}
		}
		if stop == nil {
			if err := markUncovered(ctx, states, opt); err != nil {
				return nil, nil, err
			}
			if ff != nil {
				if stop, err = coverageStop(ctx, states, opt, ff); err != nil {
					return nil, nil, err
				}
			}
		}
		if stop != nil {
			announce(opt.Progress, stop)
		} else if stop, err = execute(ctx, states, opt, ff); err != nil {
			return nil, nil, err
		}
	} else {
		report(opt.Progress, RunEvent{Kind: "nothing"})
	}
	// A run whose own context stopped writes nothing: what it left
	// undecided is the stop, never an outcome. A fail-fast stop cancels
	// only the context execute derives.
	if err := stopped(ctx, nil); err != nil {
		return nil, nil, err
	}
	for _, s := range states {
		// No baseline ran for the file: none of its mutants needed one,
		// nor any of another file sharing its command.
		if s.result.Baseline == "" {
			s.result.Baseline = BaselineNotRun
		}
	}

	var results []FileResult
	for _, s := range states {
		blocked := s.result.BaselineFailed || len(s.result.FailedSelections) > 0
		if s.failFast || !blocked {
			// In a fail-fast run every mutant of the functions judged is
			// listed with its state, written or not.
			s.result.Mutants = s.decided()
		}
		if s.failFast {
			// A failing baseline leaves nothing judged to keep, so the
			// snapshot stays as it was, and the cache holds what it held.
			s.result.Cached = s.cacheComplete(!s.result.BaselineFailed)
		}
		if s.result.BaselineFailed || (blocked && !s.failFast) {
			// Not written: a mutant is not decided.
			results = append(results, *s.result)
			continue
		}
		// A fail-fast stop that cut the file short, or a selection of
		// listed tests that failed in a fail-fast run, leaves sites
		// undecided: the file is still written, with what the run judged
		// and what still holds of the snapshot before (ADR-0022), and
		// without its summary comment.
		incomplete := s.failFast && s.incomplete()
		outcomes, scopes, ran := s.outcomes, s.scopes, s.ran
		if incomplete {
			outcomes, scopes, ran = s.preserved()
		}
		s.result.Snapshot = BuildScoped(s.file, s.key, s.tests, s.sites, outcomes, scopes, ran)
		if (opt.StatementCoverage != nil || opt.CachedCoverage != nil) && StrictCoverage(s.file.Spec.Name) {
			evidence, err := s.coverageEvidence(ctx, opt)
			if err != nil {
				return nil, nil, err
			}
			for i := range s.file.Units {
				s.result.Snapshot.Units[i].Coverage = evidence[i]
			}
		}
		if incomplete {
			s.keepCoverage()
		}
		RecordSuiteEvidence(&s.result.Snapshot, s.moduleTests, opt.Support)
		if s.judged != nil {
			s.result.Snapshot.Units = KeepUnjudged(s.result.Snapshot.Units, s.judged, s.stored, s.previous != nil)
			s.markBroadStale()
		}
		var files map[string]string
		if opt.Listed != nil {
			files = opt.Listed.Files
		}
		s.result.Snapshot.Listed = ListedOf(s.result.Snapshot.Units, files, s.stored)
		s.markListedStale()
		recordListedWithInputs(ctx, &s.result.Snapshot, opt.Root, opt.Support, opt.Fingerprints)
		MarkExcepted(s.result.Snapshot.Units, s.result.Mutants)
		if incomplete {
			s.result.Incomplete = true
			if s.judgedAnew() {
				if failure := opt.Store.Write(ctx, s.key, s.result.Snapshot); failure != nil {
					return nil, nil, failure
				}
				if err := s.removeMoved(ctx, opt.Store); err != nil {
					return nil, nil, err
				}
				s.result.Preserved = true
			}
			results = append(results, *s.result)
			continue
		}
		if s.judged != nil && len(s.judged) == 0 {
			// Nothing in the file was judged, so nothing ran and the
			// file is left as it was: no new results, no comment. A
			// renamed file's snapshot still moves, as it was.
			if s.moving != nil {
				if failure := opt.Store.Write(ctx, s.key, *s.moving); failure != nil {
					return nil, nil, failure
				}
			}
			if err := s.removeMoved(ctx, opt.Store); err != nil {
				return nil, nil, err
			}
			results = append(results, *s.result)
			continue
		}
		if failure := opt.Store.Write(ctx, s.key, s.result.Snapshot); failure != nil {
			return nil, nil, failure
		}
		if err := s.removeMoved(ctx, opt.Store); err != nil {
			return nil, nil, err
		}
		if opt.Annotate {
			// The comment excepts the survivors of every function,
			// judged or not, as a full run would.
			excepted := HeldReasons(opt.Exceptions, s.key, s.file, s.sites)
			if err := opt.Source.Annotate(ctx, s.file.Path, s.file.Spec.Comment, Annotation(s.result.Snapshot, s.file.Spec.Comment, excepted)); err != nil {
				return nil, nil, err
			}
		}
		results = append(results, *s.result)
	}
	return results, stop, nil
}

// follow takes the snapshot of a file renamed from a path that holds no
// file now: as the file's own when it has none yet, as *snap. The old
// snapshot is removed once the new one is written.
func (s *Execution) follow(ctx context.Context, opt Options, path string, snap **Snapshot) error {
	if opt.Renamed == nil {
		return nil
	}
	old := opt.Renamed(path)
	if old == "" {
		return nil
	}
	missing, failure := opt.Source.Missing(ctx, old)
	if failure != nil {
		return failure
	}
	if !missing {
		return nil
	}
	s.moved = snapshot.Key(opt.Root, old)
	if *snap != nil {
		return nil
	}
	moving, err := LoadSnapshot(ctx, opt.Store, s.moved, opt.Toolchain)
	if err != nil {
		return SnapshotReadFailure{Key: s.moved, Cause: err}
	}
	if moving != nil {
		moving.File = s.key
		*snap, s.moving = moving, moving
	}
	return nil
}

// removeMoved removes the snapshot of the path the file had before a
// rename, if any, from store, and the folders that leaves empty.
func (s *Execution) removeMoved(ctx context.Context, store MutationSnapshots) error {
	if s.moved == "" {
		return nil
	}
	if failure := store.Remove(ctx, s.moved); failure != nil {
		return failure
	}
	return nil
}

// plan parses each file, finds its sites, and keeps the outcomes the
// previous snapshot allows.
func plan(ctx context.Context, files []string, opt Options) ([]*Execution, error) {
	var states []*Execution
	for _, path := range files {
		f, err := selected.Parse(ctx, opt.Tree, opt.Surroundings, path)
		if err != nil {
			return states, err
		}
		rel, key := relativeName(opt.Dir, path), snapshot.Key(opt.Root, path)
		var tests []string
		if opt.Tests != nil {
			tests = opt.Tests(path)
		}
		c, failure := ObserveCommand(ctx, opt.Toolchain, CommandRequest{Path: path, Shell: opt.TestCommand, All: opt.AllTests, Tests: tests})
		if failure != nil {
			f.Close()
			return states, failure
		}

		s := &Execution{file: f, rel: rel, key: key, sites: Sites(f), command: c, currentSupport: opt.Support,
			result: &FileResult{Rel: rel}}
		states = append(states, s)
		snap, err := LoadSnapshot(ctx, opt.Store, key, opt.Toolchain)
		if err != nil {
			return states, SnapshotReadFailure{Key: key, Cause: err}
		}
		if err := s.follow(ctx, opt, path, &snap); err != nil {
			return states, err
		}
		s.tests, err = opt.Fingerprints.Tests(ctx, opt.Root, tests)
		if err != nil {
			return states, err
		}
		// Only a broad-scope outcome, judged now or recorded before,
		// depends on the Build root's tests.
		if BroadScope(ScopeForCommand(opt.TestCommand, opt.AllTests, s.command)) || snap != nil && HasBroadOutcome(snap.Units) {
			s.moduleTests, err = opt.Fingerprints.Suite(ctx, path, opt.Root)
			if err != nil {
				return states, err
			}
		}
		s.stored, s.previous = snap, Usable(snap, s.tests)
		prev, prevScopes := RememberedWithScopes(s.previous, f)
		s.listedChange = listedChangedWithInputs(ctx, s.stored, opt.Root, opt.Support, opt.Fingerprints)
		s.result.Functions = len(f.Units)
		var judge func(function string) bool
		if opt.Judge != nil {
			s.judged, s.result.Judged = map[int]bool{}, []string{}
			names := map[string]bool{}
			for i, u := range f.Units {
				if id := UnitID(u.Namespace, u.Name); opt.Judge(path, id, UnitHash(f, u)) {
					s.judged[i], names[id] = true, true
					s.result.Judged = append(s.result.Judged, id)
				}
			}
			// An exception names its function, so it counts when any
			// function of that name is judged.
			judge = func(function string) bool { return names[function] }
		}
		s.excepted = ApplyExceptions(opt.Exceptions, key, f, s.sites, judge)
		s.result.StaleExceptions = s.excepted.Stale
		s.outcomes = make([]string, len(s.sites))
		s.reused = make([]bool, len(s.sites))
		s.scopes = make([]string, len(s.sites))
		s.ran = make([][]string, len(s.sites))
		s.fresh = make([]freshOutcome, len(s.sites))
		scope := ScopeForCommand(opt.TestCommand, opt.AllTests, s.command)
		for i, site := range s.sites {
			// A kept outcome keeps the scope it was decided with.
			s.scopes[i] = scope
			if s.judged != nil && !s.judged[site.Unit] {
				// Not judged: it never runs, and keeps the outcome its
				// snapshot records for its unchanged function, if any.
				s.outcomes[i] = Skipped
				if outcome := prev[site.Unit][site.Key()]; outcome != "" {
					d := prevScopes[site.Unit][site.Key()]
					s.outcomes[i], s.scopes[i], s.ran[i] = outcome, d.Scope, d.Tests
				}
				continue
			}
			_, excepted := s.excepted.Held[i]
			d := prevScopes[site.Unit][site.Key()]
			// A listed outcome holds while the files it rests on do.
			restsOnChange := len(s.listedChange.Files([]Mutant{{Scope: d.Scope, Tests: d.Tests}})) > 0
			broadStale := len(BroadChanges(Mutant{Scope: d.Scope, SuiteEvidence: d.Evidence}, s.moduleTests, opt.Support)) > 0
			if outcome := prev[site.Unit][site.Key()]; outcome != "" && !restsOnChange && !broadStale {
				s.fresh[i] = freshOutcome{outcome: outcome, scope: d.Scope, tests: d.Tests}
			}
			if outcome, ok := prev.Kept(f, site, excepted); ok && !opt.MutateAll && !restsOnChange && !broadStale {
				s.outcomes[i], s.scopes[i], s.ran[i] = outcome, d.Scope, d.Tests
				s.reused[i] = true
				s.result.Reused++
			}
		}
	}
	return states, nil
}

// coverageEvidence is the measured coverage evidence of each unit of the
// file, of a strict language, as its snapshot records it: from the run's
// statement coverage, or from a matching independent cache, which wins;
// nil where neither has any.
func (s *Execution) coverageEvidence(ctx context.Context, opt Options) ([]*CoverageEvidence, error) {
	out := make([]*CoverageEvidence, len(s.file.Units))
	if !StrictCoverage(s.file.Spec.Name) {
		return out, nil
	}
	if opt.StatementCoverage != nil {
		producer, inputs := opt.CoverageProducer, map[string]string{}
		if opt.RecordCoverageInputs {
			var err error
			producer, inputs, err = CoverageInputsFor(ctx, opt.Toolchain, opt.Fingerprints, s.file.Path, opt.Root, opt.AllTests, opt.TestCommand != "", opt.Support)
			if err != nil {
				return nil, err
			}
		}
		for i, unit := range s.file.Units {
			out[i] = CoverageEvidenceFor(s.key, s.file, unit, opt.StatementCoverage, producer, inputs)
		}
	}
	if opt.CachedCoverage != nil {
		ids, hashes := FileKeys(s.file)
		for i := range s.file.Units {
			if evidence := opt.CachedCoverage(s.file.Path, ids[i], hashes[i]); evidence != nil {
				out[i] = evidence
			}
		}
	}
	return out, nil
}

// decided lists the mutants of the functions judged with their outcomes, in
// site order, each excepted survivor with its reason. A function not judged
// was not decided in this run, whatever its snapshot keeps for it. An
// exception whose mutant the tests noticed is added to the result's stale
// ones. In a fail-fast run each has its state, and one with no outcome has
// no scope either.
func (s *Execution) decided() []MutantResult {
	out := []MutantResult{}
	for i, site := range s.sites {
		u := s.file.Units[site.Unit]
		id := UnitID(u.Namespace, u.Name)
		if s.judged != nil && !s.judged[site.Unit] {
			continue
		}
		reason, stale := s.excepted.Judge(i, site, s.outcomes[i])
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
		scope, state := s.scopes[i], ""
		if s.failFast {
			state = s.stateOf(i)
			if s.outcomes[i] == "" {
				scope, ran = "", nil
			}
		}
		out = append(out, MutantResult{Site: site, Function: id, Outcome: s.outcomes[i], Reused: s.reused[i], Scope: scope,
			Excepted: reason, Coverage: covered, Tests: ran, State: state})
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
// executes is never uncovered.
func markUncovered(ctx context.Context, states []*Execution, opt Options) error {
	if opt.Coverage == nil && opt.Measurement == nil {
		return nil
	}
	var sources []string
	for _, s := range states {
		sources = append(sources, s.file.Path)
	}
	measuredReport, err := opt.Measure(ctx, sources, false)
	if err != nil || measuredReport == nil {
		return err
	}
	measured := map[string]bool{}
	for _, s := range states {
		if measuredReport.Has(s.file.Path) || measuredReport.Measures(s.file.Spec.Name) {
			measured[s.file.Spec.Name] = true
		}
	}
	for _, s := range states {
		has := measuredReport.Has(s.file.Path)
		if !has && !measured[s.file.Spec.Name] {
			report(opt.Progress, RunEvent{Kind: "no-coverage", File: s.rel})
			continue
		}
		s.coverage = make([][]string, len(s.sites))
		s.reach = make([][]string, len(s.sites))
		for i, site := range s.sites {
			s.coverage[i] = measuredReport.LineSources(s.file.Path, site.Line)
			if opt.Listed != nil {
				s.reach[i] = measuredReport.LineTests(s.file.Path, site.Line)
			}
			if s.outcomes[i] != "" || len(s.reach[i]) > 0 {
				continue
			}
			covered, lineMeasured := measuredReport.LineCovered(s.file.Path, site.Line)
			if !has || (lineMeasured && !covered) {
				s.outcomes[i] = Uncovered
			}
		}
	}
	return nil
}

type job struct {
	state *Execution
	site  int
}

// execute runs each command's baseline, then every pending mutant across
// the workers, each selection of listed tests' baseline the first time a
// mutant needs it. With ff, it stops at the first actionable final
// judgment, which it returns: every command runs under one context the
// stop cancels, and the workers share one cleanup deadline from the stop.
// The worker copies are removed only once every worker returned, its
// owned commands joined.
func execute(ctx context.Context, states []*Execution, opt Options, ff *FailFast) (*Stop, error) {
	purpose := MutationRun
	if ff != nil {
		purpose = FailFastJudgments
	}
	pool, err := opt.WorkerFactory.Open(ctx, purpose, max(1, opt.Workers))
	if err != nil {
		return nil, err
	}
	defer pool.Close(ctx)
	var st *stopper
	if ff != nil {
		ctx, st = newStopper(ctx, opt.Progress)
		defer st.cancel(nil)
		st.abort = pool.Abort
	}
	workers := make([]Worker, max(1, opt.Workers))
	for i := range workers {
		workers[i] = pool.Worker(i)
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
			if s.command.RunsNothing() {
				// No test reaches the file: none runs, so its mutant is
				// uncovered, unless listed tests reach its line, which then
				// judge it alone.
				if s.reach == nil || len(s.reach[i]) == 0 {
					s.outcomes[i] = Uncovered
					continue
				}
				if !st.stopped() {
					jobs = append(jobs, job{s, i})
				}
				continue
			}
			key := s.command.Key()
			if _, done := timeouts[key]; !done && failed[key] == "" && !st.stopped() {
				report(opt.Progress, RunEvent{Kind: "baseline", Command: s.command})
				r, err := workers[0].Run(ctx, s.command, nil, 0)
				if err != nil {
					return nil, err
				}
				if !r.Passed {
					failed[key] = r.Output
				} else {
					timeouts[key] = TimeoutOf(r.Elapsed, opt.TimeoutFactor)
				}
			}
			if out, bad := failed[key]; bad {
				s.result.BaselineFailed, s.result.BaselineOutput = true, out
				if st != nil {
					st.trigger(&Stop{Rule: "mutation.baseline-failed", File: s.rel})
				}
				continue
			}
			if _, ran := timeouts[key]; !ran {
				// The run stopped before this baseline: nothing of the
				// file is admitted.
				continue
			}
			jobs = append(jobs, job{s, i})
		}
	}
	// A baseline that ran is every file's sharing its command, whether or
	// not one of its own mutants needed it.
	for _, s := range states {
		if s.command.RunsNothing() {
			// No test reaches the file: no baseline runs for it in any
			// run, and run says BaselineNotRun.
			continue
		}
		if _, ran := timeouts[s.command.Key()]; ran {
			s.result.Baseline = "passed"
		} else if s.result.BaselineFailed {
			s.result.Baseline = "failed"
		}
	}

	// Each selection of listed tests runs without a mutant the first time
	// a mutant needs it, in that mutant's worker.
	var sels *SelectionBaselines
	if opt.Listed != nil {
		sels = NewSelectionBaselines(opt.Listed, opt.TimeoutFactor, opt.Progress)
	}

	queue := make(chan job)
	var mu sync.Mutex
	var firstErr error
	done := 0
	var wg sync.WaitGroup
	for _, w := range workers {
		wg.Add(1)
		go func(ctx context.Context, w Worker) {
			defer wg.Done()
			for j := range queue {
				r, err := runMutant(ctx, w, j, timeouts[j.state.command.Key()], sels)
				mu.Lock()
				if err != nil && firstErr == nil {
					firstErr = err
				}
				site := j.state.sites[j.site]
				if r.cancelled {
					// The stop came first: no outcome, and a judgment that
					// never started stays unattempted.
					if r.started {
						j.state.leave(j.site, StateCancelled)
						report(opt.Progress, RunEvent{Kind: "cancelled", File: j.state.rel, Site: site, Failure: context.Cause(ctx), Elapsed: r.elapsed})
					}
					mu.Unlock()
					continue
				}
				switch {
				case r.failed != nil:
					// Undecided: its outcome stays pending, it counts as no
					// mutant run, and its file is not written, but in a
					// fail-fast run, which keeps what else it judged.
					j.state.result.AddFailed(r.failed)
					if st != nil {
						j.state.leave(j.site, StateBlocked)
					}
				case r.ids != nil:
					j.state.outcomes[j.site] = r.outcome
					j.state.scopes[j.site], j.state.ran[j.site] = ScopeListed, r.ids
				default:
					j.state.outcomes[j.site] = r.outcome
				}
				if r.failed == nil {
					j.state.result.Ran++
				}
				done++
				kind := "judged"
				if r.failed != nil {
					kind = "selection-blocked"
				}
				report(opt.Progress, RunEvent{Kind: kind, Done: done, Total: len(jobs), File: j.state.rel, Site: site, Outcome: r.outcome, IDs: r.ids, Elapsed: r.elapsed, Listed: r.listed})
				if st != nil && err == nil {
					if r.failed != nil {
						st.trigger(&Stop{Rule: "tests.selection-failed", File: j.state.rel, Selection: r.failed})
					} else if stop := j.state.actionable(j.site); stop != nil {
						st.trigger(stop)
					}
				}
				mu.Unlock()
			}
		}(ctx, w)
	}
	// After a stop no job is admitted; the Done of a run without fail-fast
	// never closes.
dispatch:
	for _, j := range jobs {
		select {
		case queue <- j:
		case <-ctx.Done():
			break dispatch
		}
	}
	close(queue)
	wg.Wait()
	return st.result(), firstErr
}

// mutantRun is how a mutant's runs went.
type mutantRun struct {
	outcome string
	elapsed time.Duration // its runs' time in all
	listed  time.Duration // the listed tests' time
	// ids is the listed tests it ran once its own tests survived it, or
	// nil when it ran none.
	ids []string
	// failed, when set, is the selection ids, which fails without any
	// mutant: the mutant is not decided.
	failed *FailedSelection
	// cancelled is set when a fail-fast stop cut the mutant's judgment
	// short, or came before it: it has no outcome. started says whether
	// any of its commands had been admitted.
	cancelled, started bool
}

// runMutant runs the file's own tests on the mutant, then, when it survives
// them, the listed tests that reach its line, after their selection's
// baseline the first time any mutant needs it. Every command runs under
// ctx: once a fail-fast stop cancels it, none starts, and a judgment it cut
// short is cancelled, never killed or timed out.
func runMutant(ctx context.Context, w Worker, j job, timeout time.Duration, sels *SelectionBaselines) (mutantRun, error) {
	if ctx.Err() != nil {
		return mutantRun{cancelled: true}, nil
	}
	s := j.state
	site := s.sites[j.site]
	// Own tests that run nothing survive every mutant without running.
	run, err := mutantRun{outcome: Survived, started: true}, error(nil)
	if !s.command.RunsNothing() {
		var r CommandResult
		r, err = w.Run(ctx, s.command, &Mutation{Path: s.file.Path, Source: s.file.Src, Site: site}, timeout)
		run = mutantRun{outcome: Judged(r), elapsed: r.Elapsed, started: true}
		if r.Cancelled || cancelledBy(ctx, err) {
			return mutantRun{cancelled: true, started: true, elapsed: r.Elapsed}, nil
		}
	}
	if err != nil || run.outcome != Survived || sels == nil || s.reach == nil || len(s.reach[j.site]) == 0 {
		return run, err
	}
	run.ids = s.reach[j.site]
	cut := mutantRun{cancelled: true, started: true, elapsed: run.elapsed}
	if ctx.Err() != nil {
		return cut, nil
	}
	sel := sels.baseline(ctx, w, run.ids)
	if cancelledBy(ctx, sel.err) {
		return cut, nil
	}
	if sel.err != nil || sel.failed != nil {
		run.failed = sel.failed
		return run, sel.err
	}
	listed, err := w.Run(ctx, sels.listed.Command(run.ids), &Mutation{Path: s.file.Path, Source: s.file.Src, Site: site}, sel.timeout)
	if listed.Cancelled || cancelledBy(ctx, err) {
		cut.elapsed += listed.Elapsed
		return cut, nil
	}
	run.outcome, run.listed = Judged(listed), listed.Elapsed
	run.elapsed += listed.Elapsed
	return run, err
}

// judged is the outcome of a test run on a mutant.
func Judged(r CommandResult) string {
	switch {
	case r.TimedOut:
		return Timeout
	case r.Passed:
		return Survived
	}
	return Killed
}

// command runs the listed tests ids.
func (l *Listed) Command(ids []string) Command {
	return Command{Root: l.Root, Dir: l.Root, Shell: l.Select(ids)}
}
