package mutation

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/donvargax/itos-cc/internal/coverage"
	covport "github.com/donvargax/itos-cc/internal/coverage/port"
	"github.com/donvargax/itos-cc/internal/lang"
	"github.com/donvargax/itos-cc/internal/mutants"
)

// CountedQuery carries a counted run's domain values once: the paths, the
// --since ref and the seed as written, "" when omitted, the budget, the
// scope its trials run, how uncovered sites judge and the run's limits.
// Progress is where its observations go as they happen, its report among
// them.
type CountedQuery struct {
	Paths                    []string
	Since, Seed              string
	Count                    int
	AllTests                 bool
	FailUncovered, LinesOnly bool
	Workers                  int
	TimeoutFactor            float64
	Progress                 CountedProgress
}

// CountedProgress receives a counted run's observations while it works:
// each run event, and its report once it is as complete as it gets, under
// the run's own context, while the run still owns its frozen inputs.
type CountedProgress interface {
	mutants.Progress
	Counted(ctx context.Context, found CountedFound)
}

// CountedFound is a counted run's report and what stopped it: an interrupt
// (its cause) before any trial, preparation failing at a stage, the
// integration coverage strict coverage could not measure, or, once its
// trials ran, the stale exceptions and the plan and preparation strict Go
// coverage is judged by.
type CountedFound struct {
	Report             *countedResult
	Interrupted        error
	PreparationStage   string
	PreparationFailure mutants.FreshFailure
	Integration        []coverage.Unmeasured
	Judged             bool
	Stale              []mutants.StaleException
	Plan               *mutants.FreshPlan
	Prep               *mutants.FreshPreparation
	Strict, StrictGo   bool
	LinesOnly          bool
}

// Counted judges at most Count committed sites freshly, drawn with the seed
// from the HEAD commit's inputs. It writes no snapshot, annotation or
// coverage cache. It observes in the cmd handler's order: the repository's
// top level, the paths from the working directory, then under its own
// context, which an interrupt cancels, the plan, preparation, strict
// integration coverage, the exceptions and the trials. A refusal is a
// Failure the UI words; the report reaches Progress before the frozen
// inputs are released.
func (h Handler) Counted(ctx context.Context, q CountedQuery) error {
	result := &countedResult{Selected: []countedSite{}, Subjects: countedSubjects{Judged: []countedSubject{}, Omitted: []countedSubject{}},
		Stages: []preparationStage{}}
	if h.Repository == nil {
		return errors.New("mutation repository binding is missing")
	}
	root, err := h.Repository(ctx)
	if err != nil {
		return CountedNoGit{Cause: err}
	}
	paths, err := h.countedPaths(ctx, root, q.Paths)
	if err != nil {
		return err
	}
	fresh := h.Fresh
	if fresh == nil {
		return errors.New("mutation Fresh binding is missing")
	}
	plan, planFailure := fresh.Plan(ctx, mutants.FreshRequest{Root: root, Paths: paths, Since: q.Since, Count: q.Count, Seed: q.Seed})
	if planFailure != nil {
		if ctx.Err() != nil {
			result.Sampling = countedSampling{Budget: q.Count, Assurance: "sampled", Completion: "interrupted",
				Stop: "interrupted while planning the selection", Bounds: countedBounds}
			q.present(ctx, CountedFound{Report: result, Interrupted: context.Cause(ctx)})
			return nil
		}
		return CountedPlanFailed{Since: q.Since, Failure: planFailure}
	}
	defer fresh.Release(ctx, plan)
	s := &result.Sampling
	*s = countedSampling{Budget: q.Count, Eligible: len(plan.Eligible), Selected: len(plan.Selected), Omitted: len(plan.Omitted),
		Seed: plan.Seed, Algorithm: plan.Algorithm, Commit: plan.Commit, Since: plan.SinceRef, SinceBase: plan.SinceBase,
		Assurance: "sampled", Completion: "completed", Bounds: countedBounds}
	result.Subjects = countedSubjectsOf(plan)
	for _, c := range plan.Selected {
		result.Selected = append(result.Selected, countedSite{Identity: c.Identity, File: c.Path, Line: c.Site.Line, Column: c.Site.Column,
			Function: c.Function, Original: c.Site.Original, Replacement: c.Site.Replacement, State: "unattempted"})
	}
	strict := q.FailUncovered
	strictGo := strict && hasGoUnit(plan.Units)
	if len(plan.Eligible) == 0 {
		s.Assurance, s.Completion = "not-applicable", "not-applicable"
	}
	if len(plan.Eligible) == 0 && !strictGo {
		result.Stages = append(result.Stages, preparationStage{Name: "preparation", State: "skipped"})
		q.present(ctx, CountedFound{Report: result})
		return nil
	}

	scope := covport.OwnTests
	if q.AllTests {
		scope = covport.AllTests
	}
	// One owner, so preparation and trials share one run-abort cleanup
	// deadline.
	prep, prepFailure := fresh.Prepare(ctx, plan, scope)
	if prep != nil {
		for _, stage := range prep.Stages {
			result.Stages = append(result.Stages, preparationStage{Name: stage.Name, State: stage.State, Error: stage.Error})
		}
	}
	if prepFailure != nil {
		failed := "preparation"
		if prep != nil && len(prep.Stages) > 0 {
			failed = prep.Stages[len(prep.Stages)-1].Name
		}
		if ctx.Err() != nil {
			s.Completion, s.Stop = "interrupted", "interrupted during preparation, at "+failed
			q.present(ctx, CountedFound{Report: result, Interrupted: context.Cause(ctx)})
			return nil
		}
		if s.Completion != "not-applicable" {
			s.Completion = "stopped"
		}
		s.Stop = "preparation failed at " + failed
		q.present(ctx, CountedFound{Report: result, PreparationStage: failed, PreparationFailure: prepFailure})
		return nil
	}

	if strict {
		// A collector that could not measure the processes the tests
		// start fails a strict run where its language has a mutant to
		// judge, as a complete run does (integrationToJudge).
		var files []string
		for _, c := range plan.Selected {
			files = append(files, filepath.Join(plan.FrozenRoot, filepath.FromSlash(c.Path)))
		}
		if gaps := h.integrationToJudge(prep.Report, files); len(gaps) > 0 {
			s.Completion, s.Stop = "stopped", "integration coverage could not be measured"
			q.present(ctx, CountedFound{Report: result, Integration: gaps})
			return nil
		}
	}

	exceptions, exceptionErr := mutants.PlanExceptions(ctx, plan, prep.Exceptions, h.Tree, h.Surroundings)
	if exceptionErr != nil {
		return exceptionErr
	}
	stale := slices.Clone(exceptions.Stale)
	if err := h.trials(ctx, q, plan, prep, exceptions, result, &stale); err != nil {
		return err
	}
	interrupted := ctx.Err() != nil
	found := CountedFound{Report: result, Judged: true, Stale: stale, Plan: plan, Prep: prep, Strict: strict, StrictGo: strictGo, LinesOnly: q.LinesOnly}
	if interrupted {
		s.Completion, s.Stop = "interrupted", "interrupted while selected sites were judged"
		found.Interrupted = context.Cause(ctx)
	}
	for _, site := range result.Selected {
		switch {
		case s.Stop != "":
		case site.State == "failed" || site.State == "unattempted":
			s.Completion, s.Stop = "stopped", "a selected trial could not run"
		case site.State == "blocked":
			s.Completion, s.Stop = "stopped", "the listed tests a selected site needs fail without any mutant"
		}
	}
	q.present(ctx, found)
	return nil
}

// present hands found to Progress, when there is one.
func (q CountedQuery) present(ctx context.Context, found CountedFound) {
	if q.Progress != nil {
		q.Progress.Counted(ctx, found)
	}
}

// trials judges each selected site that has a clean baseline and is not
// uncovered, recording each site's state, its stages and the stale
// exceptions its outcome makes in result and stale.
func (h Handler) trials(ctx context.Context, q CountedQuery, plan *mutants.FreshPlan, prep *mutants.FreshPreparation, exceptions *mutants.FreshExceptions, result *countedResult, stale *[]mutants.StaleException) error {
	s := &result.Sampling
	var listed *mutants.Listed
	if prep.Tests != nil {
		listed = &mutants.Listed{Root: plan.FrozenRoot, Select: prep.Tests.Select}
	}
	var trials []mutants.FreshTrial
	var trialSites []int
	for i, c := range plan.Selected {
		path := filepath.Join(plan.FrozenRoot, filepath.FromSlash(c.Path))
		site := &result.Selected[i]
		if countedUncovered(prep.Report, path, c.Site.Line) {
			site.State, site.Outcome = "uncovered", mutants.Uncovered
			continue
		}
		var reach []string
		if listed != nil {
			reach = prep.Report.LineTests(path, c.Site.Line)
		}
		baseline, ok := baselineOf(prep.Baselines, c.Path)
		if !ok && len(reach) > 0 {
			// Preparation runs no own baseline for a Python or Kotlin file
			// whose own tests run nothing, as no own test reaches it: its
			// mutant goes straight to the listed tests that reach its line,
			// after their selection's clean baseline, as a complete run
			// judges it.
			baseline, ok = mutants.FreshBaseline{Paths: []string{c.Path}}, true
		}
		if !ok {
			site.State, site.Reason = "failed", "no clean baseline was prepared for its file"
			continue
		}
		trials = append(trials, mutants.FreshTrial{Candidate: c, Baseline: baseline, Reach: reach})
		trialSites = append(trialSites, i)
	}
	if len(trials) == 0 {
		result.Stages = append(result.Stages, preparationStage{Name: "trials", State: "skipped"})
		return nil
	}
	factory := h.FreshWorkers
	if factory == nil {
		return errors.New("mutation Workers binding is missing")
	}
	var progress mutants.Progress
	if q.Progress != nil {
		progress = q.Progress
	}
	results, err := mutants.RunFreshTrials(ctx, plan, trials, mutants.FreshTrialOptions{
		WorkerFactory: factory,
		Workers:       q.Workers, TimeoutFactor: q.TimeoutFactor, Scope: mutants.RunScope("", q.AllTests),
		Listed: listed, Tree: h.Tree, Toolchain: h.Toolchain, Progress: progress})
	if err != nil {
		return err
	}
	trialStage := preparationStage{Name: "trials", State: "complete"}
	var selectionStage *preparationStage
	for n, r := range results {
		site := &result.Selected[trialSites[n]]
		for _, stage := range r.Stages {
			site.Stages = append(site.Stages, countedStage{Name: stage.Name, State: stage.State, Outcome: stage.Outcome, Tests: stage.Tests, Error: stage.Error})
			switch {
			case stage.Name == "own" && stage.State == "complete",
				stage.Name == "listed" && stage.State == "complete" && !r.RanOwn():
				// The mutant ran: its trial spent the budget, whatever
				// its later stages could judge. A file no own test
				// reaches runs it under the listed tests alone.
				s.Executed++
			case stage.Name == "listed-baseline" && selectionStage == nil:
				selectionStage = &preparationStage{Name: stage.Name, State: stage.State, Error: stage.Error}
			case stage.Name == "listed-baseline" && stage.State == "failed" && selectionStage.State != "failed":
				selectionStage.State, selectionStage.Error = stage.State, stage.Error
			}
		}
		var cleanup mutants.WorkerCleanupFailed
		switch {
		case errors.As(r.Err, &cleanup):
			// Its owned cleanup did not finish: no outcome, and the run
			// reports the group.
			site.State, site.Reason = "cancelled", "its owned process group's cleanup did not finish"
			result.cleanups = append(result.cleanups, cleanup)
			continue
		case r.Err != nil && ctx.Err() != nil && len(r.Stages) == 0:
			// Never started: nothing is admitted after the abort.
			site.State = "unattempted"
			continue
		case r.Err != nil && ctx.Err() != nil:
			site.State, site.Reason = "cancelled", "the run was interrupted while it was judged"
			continue
		case r.Err != nil:
			site.State, site.Reason = "failed", r.Err.Error()
		case r.FailedSelection != nil:
			site.State, site.selection = "blocked", r.FailedSelection
			own := "it survived its own tests"
			if !r.RanOwn() {
				own = "no own test reaches its file"
			}
			site.Reason = fmt.Sprintf("%s, and its listed tests %s fail without any mutant, so they could not judge it",
				own, strings.Join(r.FailedSelection.IDs, " "))
		default:
			site.State, site.Outcome, site.Scope, site.Tests = "judged", r.Outcome, r.Scope, r.Tests
			reason, gone := exceptions.Judge(r.Trial.Candidate, r.Outcome)
			site.Excepted = reason
			if gone != nil {
				*stale = append(*stale, *gone)
			}
			continue
		}
		if trialStage.State == "complete" {
			trialStage.State, trialStage.Error = "failed", site.Reason
		}
	}
	if ctx.Err() != nil {
		trialStage.State, trialStage.Error = "aborted", context.Cause(ctx).Error()
	}
	if selectionStage != nil {
		result.Stages = append(result.Stages, *selectionStage)
	}
	result.Stages = append(result.Stages, trialStage)
	return nil
}

// countedPaths turns the paths given, relative to the working directory,
// into slash paths relative to root.
func (h Handler) countedPaths(ctx context.Context, root string, args []string) ([]string, error) {
	if len(args) == 0 {
		return nil, nil
	}
	if h.PhysicalDir == nil {
		return nil, errors.New("mutation working directory binding is missing")
	}
	wd, err := h.PhysicalDir(ctx)
	if err != nil {
		return nil, err
	}
	var paths []string
	for _, arg := range args {
		abs := arg
		if !filepath.IsAbs(abs) {
			abs = filepath.Join(wd, arg)
		}
		rel, err := filepath.Rel(root, filepath.Clean(abs))
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return nil, OutsideRepository{Argument: arg}
		}
		paths = append(paths, filepath.ToSlash(rel))
	}
	return paths, nil
}

func countedSubjectsOf(plan *mutants.FreshPlan) countedSubjects {
	out := countedSubjects{Judged: []countedSubject{}, Omitted: []countedSubject{}}
	judged := map[countedSubject]bool{}
	for _, c := range plan.Selected {
		judged[countedSubject{File: c.Path, Function: c.Function}] = true
	}
	omitted := map[countedSubject]bool{}
	for _, c := range plan.Omitted {
		if subject := (countedSubject{File: c.Path, Function: c.Function}); !judged[subject] {
			omitted[subject] = true
		}
	}
	for subject := range judged {
		out.Judged = append(out.Judged, subject)
	}
	for subject := range omitted {
		out.Omitted = append(out.Omitted, subject)
	}
	for _, list := range [][]countedSubject{out.Judged, out.Omitted} {
		sort.Slice(list, func(i, j int) bool {
			if list[i].File != list[j].File {
				return list[i].File < list[j].File
			}
			return list[i].Function < list[j].Function
		})
	}
	return out
}

// countedUncovered says whether the fresh measurement shows no test, own
// or listed, executing line of path, as complete mode marks a mutant
// uncovered.
func countedUncovered(report *coverage.Report, path string, line int) bool {
	if len(report.LineTests(path, line)) > 0 {
		return false
	}
	if !report.Has(path) {
		spec := lang.Detect(path)
		return spec != nil && report.Measures(spec.Name)
	}
	covered, measured := report.LineCovered(path, line)
	return measured && !covered
}

func baselineOf(baselines []mutants.FreshBaseline, path string) (mutants.FreshBaseline, bool) {
	for _, b := range baselines {
		for _, p := range b.Paths {
			if p == path {
				return b, true
			}
		}
	}
	return mutants.FreshBaseline{}, false
}

func hasGoUnit(units []mutants.FreshUnit) bool {
	for _, unit := range units {
		if strings.HasSuffix(unit.Path, ".go") {
			return true
		}
	}
	return false
}
