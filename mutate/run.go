package mutate

import (
	"context"
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
	// StatementCoverage is a successful independently admitted coverage
	// measurement for strict mode, of every strict language
	// (StrictCoverage). It is attached per unit of a file of such a
	// language, including units with no mutation sites.
	StatementCoverage *coverage.Report
	CoverageProducer  string
	// CoverageInputs is the producer and input fingerprint of the file at
	// source's evidence, of its language.
	CoverageInputs func(source string) (string, map[string]string, error)
	CachedCoverage func(path, function, hash string) *CoverageEvidence
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
	Exceptions []config.Exception
	Log        io.Writer
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
	Snapshot Snapshot
	// Ran is how many mutants of the functions judged this run decided by
	// running them, and Reused how many it took from the snapshot: a
	// mutant blocked by a failing selection of listed tests counts in
	// neither, nor does a result only kept from before (Preserved).
	Ran, Reused    int
	BaselineFailed bool
	BaselineOutput string
	// FailedSelections is each selection of listed tests that a mutant of
	// the file needed and that failed without any mutant: such a mutant is
	// not decided, so the snapshot is not written, as when the baseline
	// failed.
	FailedSelections []FailedSelection
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
	// Baseline is, in a fail-fast run, "passed" or "failed" when the run
	// ran the file's own tests without any mutant, empty when it did not.
	Baseline string
	// Incomplete is set when a fail-fast stop left a site of a function
	// judged undecided: the run's own work on the file is not complete. The
	// summary comment is not written, and the snapshot only to keep what
	// the run judged (Preserved).
	Incomplete bool
	// Preserved is set when the file is Incomplete and its snapshot was
	// written all the same, as ADR-0022 says: it keeps every completed or
	// reused judgment with the scope and freshness evidence that decided
	// it, and for a site the stop left undecided what the snapshot before
	// recorded that still holds, if anything; such a site gets no outcome
	// of its own. A file the run judged nothing anew in is left as it was.
	Preserved bool
	// Cached is, in a fail-fast run, whether the file's snapshot, as the
	// run leaves it, holds a result that still holds for every mutant of
	// its functions judged: the cache's completeness, which a stopped run
	// can leave complete, apart from the run's own (Incomplete).
	Cached bool
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
	// State is, in a fail-fast run, StateCompleted when Outcome is set, and
	// otherwise why it is not; empty in other runs, where every mutant
	// listed has its outcome.
	State string
}

// skipped marks a site of a function not judged that has no outcome to keep:
// it neither runs nor goes into the snapshot.
const skipped = "skipped"

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
	listedChange   *ListedChange
	command        Command
	result         *FileResult
	previous       *Snapshot         // the snapshot before this run, or nil when there is none or its tests differ
	stored         *Snapshot         // the snapshot before this run, whatever tests it records, or nil when there is none
	tests          map[string]string // the tests that import the file, as the snapshot records them
	moduleTests    map[string]string // every test beneath the source's build root (SuiteTestHashes)
	currentSupport map[string]string
	judged         map[int]bool   // by the index of the unit, or nil when every function is
	excepted       fileExceptions // the exceptions of the functions judged
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

// Run mutates files and writes their snapshots. It returns one result per
// file, in the order given.
func Run(files []string, opt Options) ([]FileResult, error) {
	results, _, err := run(files, opt, nil)
	return results, err
}

// RunFailFast is Run stopping at the first actionable final judgment, as
// ff says. It returns the stop too, nil when nothing stopped the run
// before every selected mutant was decided. Each result's Baseline is set,
// and each mutant listed has its State: a file the stop cut short lists
// the mutants of its functions judged, decided or not.
func RunFailFast(files []string, opt Options, ff FailFast) ([]FileResult, *Stop, error) {
	return run(files, opt, &ff)
}

func run(files []string, opt Options, ff *FailFast) ([]FileResult, *Stop, error) {
	states, err := plan(files, opt)
	defer func() {
		for _, s := range states {
			s.file.Close()
		}
	}()
	if err != nil {
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
			if err := markUncovered(states, opt); err != nil {
				return nil, nil, err
			}
			if ff != nil {
				if stop, err = coverageStop(states, opt, ff); err != nil {
					return nil, nil, err
				}
			}
		}
		if stop != nil {
			announce(opt.Log, stop)
		} else if stop, err = execute(states, opt, ff); err != nil {
			return nil, nil, err
		}
	} else {
		fmt.Fprintln(opt.Log, "itos-cc: no mutations to test")
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
		s.result.Snapshot = buildScoped(s.file, s.key, s.tests, s.sites, outcomes, scopes, ran)
		if (opt.StatementCoverage != nil || opt.CachedCoverage != nil) && StrictCoverage(s.file.Spec.Name) {
			evidence, err := s.coverageEvidence(opt)
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
		recordSuiteEvidence(&s.result.Snapshot, s.moduleTests, opt.Support)
		if s.judged != nil {
			s.result.Snapshot.Units = keepUnjudged(s.result.Snapshot.Units, s.judged, s.stored, s.previous != nil)
			s.markBroadStale()
		}
		var files map[string]string
		if opt.Listed != nil {
			files = opt.Listed.Files
		}
		s.result.Snapshot.Listed = listedOf(s.result.Snapshot.Units, files, s.stored)
		s.markListedStale()
		s.result.Snapshot.recordListed(project.Root(), opt.Support)
		markExcepted(s.result.Snapshot.Units, s.result.Mutants)
		if incomplete {
			s.result.Incomplete = true
			if s.judgedAnew() {
				if err := metrics.Write(SnapshotName(s.key), s.result.Snapshot); err != nil {
					return nil, nil, err
				}
				if err := s.removeMoved(); err != nil {
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
				if err := metrics.Write(SnapshotName(s.key), s.moving); err != nil {
					return nil, nil, err
				}
			}
			if err := s.removeMoved(); err != nil {
				return nil, nil, err
			}
			results = append(results, *s.result)
			continue
		}
		if err := metrics.Write(SnapshotName(s.key), s.result.Snapshot); err != nil {
			return nil, nil, err
		}
		if err := s.removeMoved(); err != nil {
			return nil, nil, err
		}
		if opt.Annotate {
			// The comment excepts the survivors of every function,
			// judged or not, as a full run would.
			excepted := heldReasons(opt.Exceptions, s.key, s.file, s.sites)
			if err := annotate(s.file.Path, s.file.Spec, s.result.Snapshot, excepted); err != nil {
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
		var tests []string
		if opt.Tests != nil {
			tests = opt.Tests(path)
		}
		s := &fileState{file: f, rel: rel, key: key, sites: Sites(f), command: TestCommand(path, opt.TestCommand, opt.AllTests, tests), currentSupport: opt.Support,
			result: &FileResult{Rel: rel}}
		states = append(states, s)
		snap, err := LoadSnapshot(key)
		if err != nil {
			return states, fmt.Errorf("%s: %w", SnapshotName(key), err)
		}
		if err := s.follow(opt, path, &snap); err != nil {
			return states, err
		}
		if s.tests, err = TestHashes(tests); err != nil {
			return states, err
		}
		// Only a broad-scope outcome, judged now or recorded before,
		// depends on the build root's tests.
		if broadScope(FileScope(path, opt.TestCommand, opt.AllTests)) || snap != nil && HasBroadOutcome(snap.Units) {
			if s.moduleTests, err = SuiteTestHashes(path, project.Root()); err != nil {
				return states, err
			}
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
		s.fresh = make([]freshOutcome, len(s.sites))
		scope := FileScope(path, opt.TestCommand, opt.AllTests)
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
			broadStale := len(BroadChanges(Mutant{Scope: d.scope, SuiteEvidence: d.evidence}, s.moduleTests, opt.Support)) > 0
			if outcome := prev[site.Unit][site.Key()]; outcome != "" && !restsOnChange && !broadStale {
				s.fresh[i] = freshOutcome{outcome: outcome, scope: d.scope, tests: d.tests}
			}
			if outcome, ok := prev.kept(f, site, excepted); ok && !opt.MutateAll && !restsOnChange && !broadStale {
				s.outcomes[i], s.scopes[i], s.ran[i] = outcome, d.scope, d.tests
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
func (s *fileState) coverageEvidence(opt Options) ([]*CoverageEvidence, error) {
	out := make([]*CoverageEvidence, len(s.file.Units))
	if !StrictCoverage(s.file.Spec.Name) {
		return out, nil
	}
	if opt.StatementCoverage != nil {
		producer, inputs := opt.CoverageProducer, map[string]string{}
		if opt.CoverageInputs != nil {
			var err error
			producer, inputs, err = opt.CoverageInputs(s.file.Path)
			if err != nil {
				return nil, err
			}
		}
		for i, unit := range s.file.Units {
			out[i] = coverageEvidence(s.file, unit, opt.StatementCoverage, producer, inputs)
		}
	}
	if opt.CachedCoverage != nil {
		ids, hashes := fileKeys(s.file)
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
func markUncovered(states []*fileState, opt Options) error {
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
// the workers, each selection of listed tests' baseline the first time a
// mutant needs it. With ff, it stops at the first actionable final
// judgment, which it returns: every command runs under one context the
// stop cancels, and the workers share one cleanup deadline from the stop.
// The worker copies are removed only once every worker returned, its
// owned commands joined.
func execute(states []*fileState, opt Options, ff *FailFast) (*Stop, error) {
	base, err := os.MkdirTemp("", "itos-cc-mutate-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(base)
	ctx := context.Background()
	var st *stopper
	var runner commandRunner
	if ff != nil {
		st = newStopper(opt.Log)
		defer st.cancel(nil)
		ctx, runner.cleanupBudget = st.ctx, st.budget
	}
	workers := make([]*worker, max(1, opt.Workers))
	for i := range workers {
		workers[i] = &worker{dir: fmt.Sprintf("%s/w%d", base, i), copies: map[string]string{}, runner: runner}
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
				fmt.Fprintf(opt.Log, "itos-cc: baseline %s$ %s\n", project.Rel(s.command.Dir), s.command)
				r, err := workers[0].runContext(ctx, s.command, 0)
				if err != nil {
					return nil, err
				}
				if !r.passed {
					failed[key] = r.output
				} else {
					timeouts[key] = timeoutOf(r.elapsed, opt.TimeoutFactor)
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
	if ff != nil {
		for _, s := range states {
			if _, ran := timeouts[s.command.Key()]; ran {
				s.result.Baseline = "passed"
			} else if s.result.BaselineFailed {
				s.result.Baseline = "failed"
			}
		}
	}

	// Each selection of listed tests runs without a mutant the first time
	// a mutant needs it, in that mutant's worker.
	var sels *selections
	if opt.Listed != nil {
		sels = newSelections(opt.Listed, opt.TimeoutFactor, opt.Log)
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
						fmt.Fprintf(opt.Log, "itos-cc: %s:%d %s → %s cancelled: %v (%.1fs)\n", j.state.rel, site.Line,
							show(site.Original), show(site.Replacement), context.Cause(ctx), r.elapsed.Seconds())
					}
					mu.Unlock()
					continue
				}
				said, took := r.outcome, fmt.Sprintf("%.1fs", r.elapsed.Seconds())
				switch {
				case r.failed != nil:
					// Undecided: its outcome stays pending, it counts as no
					// mutant run, and its file is not written, but in a
					// fail-fast run, which keeps what else it judged.
					j.state.result.addFailed(r.failed)
					said = fmt.Sprintf("survived its own tests, then not judged: %s fail without any mutant", strings.Join(r.ids, " "))
					if st != nil {
						j.state.leave(j.site, StateBlocked)
					}
				case r.ids != nil:
					j.state.outcomes[j.site] = r.outcome
					j.state.scopes[j.site], j.state.ran[j.site] = ScopeListed, r.ids
					said = fmt.Sprintf("survived its own tests, then %s with %s", r.outcome, strings.Join(r.ids, " "))
					took += fmt.Sprintf(", the listed tests %.1fs", r.listed.Seconds())
				default:
					j.state.outcomes[j.site] = r.outcome
				}
				if r.failed == nil {
					j.state.result.Ran++
				}
				done++
				fmt.Fprintf(opt.Log, "itos-cc: [%d/%d] %s:%d %s → %s %s (%s)\n", done, len(jobs),
					j.state.rel, site.Line, show(site.Original), show(site.Replacement), said, took)
				if st != nil && err == nil {
					if r.failed != nil {
						st.trigger(&Stop{Rule: "tests.selection-failed", File: j.state.rel, Selection: r.failed})
					} else if stop := j.state.actionable(j.site); stop != nil {
						st.trigger(stop)
					}
				}
				mu.Unlock()
			}
		}(w)
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
func runMutant(ctx context.Context, w *worker, j job, timeout time.Duration, sels *selections) (mutantRun, error) {
	if ctx.Err() != nil {
		return mutantRun{cancelled: true}, nil
	}
	s := j.state
	site := s.sites[j.site]
	mutated := site.Apply(s.file.Src)
	// Own tests that run nothing survive every mutant without running.
	run, err := mutantRun{outcome: Survived, started: true}, error(nil)
	if !s.command.RunsNothing() {
		var r result
		r, err = w.withMutant(s.command.Root, s.file.Path, s.file.Src, mutated, func() (result, error) {
			return w.runContext(ctx, s.command, timeout)
		})
		run = mutantRun{outcome: judged(r), elapsed: r.elapsed, started: true}
		if r.cancelled || cancelledBy(ctx, err) {
			return mutantRun{cancelled: true, started: true, elapsed: r.elapsed}, nil
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
	sel := sels.baselineContext(ctx, w, run.ids)
	if cancelledBy(ctx, sel.err) {
		return cut, nil
	}
	if sel.err != nil || sel.failed != nil {
		run.failed = sel.failed
		return run, sel.err
	}
	listed, err := w.withMutant(sels.listed.Root, s.file.Path, s.file.Src, mutated, func() (result, error) {
		return w.runContext(ctx, sels.listed.command(run.ids), sel.timeout)
	})
	if listed.cancelled || cancelledBy(ctx, err) {
		cut.elapsed += listed.elapsed
		return cut, nil
	}
	run.outcome, run.listed = judged(listed), listed.elapsed
	run.elapsed += listed.elapsed
	return run, err
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

// show renders an empty replacement, a deleted operator, visibly.
func show(s string) string {
	if s == "" {
		return "(deleted)"
	}
	return "`" + s + "`"
}
