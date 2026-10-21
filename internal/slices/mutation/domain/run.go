package domain

import (
	"context"

	"github.com/donvargax/itos-cc/internal/coverage"
	"github.com/donvargax/itos-cc/internal/lang"
	langport "github.com/donvargax/itos-cc/internal/lang/port"
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
	// (internal/imports' TestsImporting, through Reach), whose hashes its
	// snapshot records: a kill is reused only while they are those
	// recorded. In Python they are also the tests its own command runs
	// (TestCommand). Unset, a file has none.
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
	// A run whose own context stopped while it judged, as an interrupt
	// stops it, keeps what it finished as a fail-fast stop does (ADR-0022),
	// writing it outside the cancelled context, and so does one whose
	// judgment's owned cleanup failed. A fail-fast stop cancels only the
	// context execute derives.
	var interrupted error
	if ctx.Err() != nil {
		interrupted = Stopped{Cause: context.Cause(ctx)}
		ctx = context.WithoutCancel(ctx)
	}
	for _, s := range states {
		if interrupted != nil || len(s.result.CleanupFailures) > 0 {
			for _, other := range states {
				other.failFast = true
			}
		}
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
	return results, stop, interrupted
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
