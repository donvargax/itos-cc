package domain_test

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	selected "github.com/donvargax/itos-cc/internal/selection"
	mutants "github.com/donvargax/itos-cc/internal/slices/mutation/domain"
	"github.com/donvargax/itos-cc/internal/slices/mutation/domain/porttest"
)

// flip has the tests decide site's mutant by outcomes in turn, run after
// run, counted across the runs of f's Workers, and every other mutant as
// the fixture declared it.
func flip(f *runFixture, site mutants.Site, outcomes ...string) {
	for command, facts := range f.workers.Tests[f.root] {
		for i, v := range facts.Verdicts {
			if v.Site.Key() == site.Key() {
				facts.Verdicts[i] = porttest.Verdict{Path: v.Path, Site: v.Site, Outcome: outcomes[0], Outcomes: outcomes}
			}
		}
		f.workers.Tests[f.root][command] = facts
	}
}

// recorded is the unit of board.go's snapshot and the mutant it records at
// site.
func recorded(t *testing.T, f *runFixture, site mutants.Site) (mutants.UnitResult, mutants.Mutant) {
	t.Helper()
	snap := f.snapshots.Values["board.go"]
	for _, u := range snap.Units {
		for _, m := range u.Mutants {
			if m.Key() == site.Key() {
				return u, m
			}
		}
	}
	t.Fatalf("the snapshot records no mutant at %s: %+v", site.Key(), snap)
	return mutants.UnitResult{}, mutants.Mutant{}
}

// decidedAt is the mutant a run's result lists at site.
func decidedAt(t *testing.T, r mutants.FileResult, site mutants.Site) mutants.MutantResult {
	t.Helper()
	for _, m := range r.Mutants {
		if m.Key() == site.Key() {
			return m
		}
	}
	t.Fatalf("the run lists no mutant at %s: %+v", site.Key(), r.Mutants)
	return mutants.MutantResult{}
}

func TestARetriedSurvivorItsTestsDecideDifferentlyIsRecordedFlakyUntilThreeRunsAgree(t *testing.T) {
	f, site := survivingFixture(t)
	survivors := 0
	for _, u := range f.snapshots.Values["board.go"].Units {
		survivors += u.Survived
	}
	f.options.Annotate = true
	flip(f, site, mutants.Killed, mutants.Survived)

	r := f.run(t)[0]
	u, m := recorded(t, f, site)
	if !m.Flaky || !reflect.DeepEqual(m.Outcomes, []string{mutants.Killed, mutants.Survived}) || m.Outcome != mutants.Killed {
		t.Fatalf("recorded %+v, want flaky with outcomes killed, survived and outcome killed", m)
	}
	if u.Killed != 0 || u.Survived != survivors-1 || r.Ran != survivors+1 {
		t.Errorf("killed %d, survived %d, ran %d; want 0, %d, %d", u.Killed, u.Survived, r.Ran, survivors-1, survivors+1)
	}
	if got := decidedAt(t, r, site); !reflect.DeepEqual(got.Outcomes, m.Outcomes) || got.Outcome != mutants.Killed {
		t.Errorf("the run lists %+v, want its outcomes", got)
	}
	comment := f.files[f.path]
	line := fmt.Sprintf("// flaky: line %d %s → %s in Value\n", site.Line, mutants.Show(site.Original), mutants.Show(site.Replacement))
	if !strings.Contains(comment, ", 1 flaky\n") || !strings.Contains(comment, line) {
		t.Errorf("the summary comment does not list the flaky mutant:\n%s", comment)
	}

	// Recorded flaky, it is never reused: its runs disagree again.
	r = f.run(t)[0]
	if _, m := recorded(t, f, site); !m.Flaky || !reflect.DeepEqual(m.Outcomes, []string{mutants.Killed, mutants.Survived}) || r.Reused != 0 {
		t.Fatalf("judged again %+v, reused %d: want flaky again, nothing reused", m, r.Reused)
	}

	// Once three runs agree, it is recorded as their outcome, not flaky.
	flip(f, site, mutants.Killed)
	r = f.run(t)[0]
	u, m = recorded(t, f, site)
	if m.Flaky || m.Outcomes != nil || m.Outcome != mutants.Killed || u.Killed != 1 || r.Ran != survivors-1+3 {
		t.Fatalf("settled %+v, killed %d, ran %d: want killed, not flaky, after 3 runs", m, u.Killed, r.Ran)
	}
	if strings.Contains(f.files[f.path], "flaky") {
		t.Errorf("the summary comment still names a flaky mutant:\n%s", f.files[f.path])
	}
	if r = f.run(t)[0]; r.Reused != 1 {
		t.Errorf("a settled kill reused %d, want it reused", r.Reused)
	}
}

func TestAMutateAllKillItsTestsDecideDifferentlyIsFlakyAndOneThatStaysSurvivedIsNot(t *testing.T) {
	f, site := survivingFixture(t)
	flip(f, site, mutants.Killed)
	f.run(t)
	if _, m := recorded(t, f, site); m.Outcome != mutants.Killed {
		t.Fatalf("recorded %+v, want killed", m)
	}
	f.options.MutateAll = true
	flip(f, site, mutants.Survived, mutants.Survived, mutants.Killed)
	f.run(t)
	u, m := recorded(t, f, site)
	if !m.Flaky || !reflect.DeepEqual(m.Outcomes, []string{mutants.Survived, mutants.Survived, mutants.Killed}) || m.Outcome != mutants.Survived || u.Survived != len(u.Mutants)-1 || u.Killed != 0 {
		t.Fatalf("recorded %+v, killed %d, survived %d: want flaky, outcomes survived, survived, killed, counted neither", m, u.Killed, u.Survived)
	}
	checks, err := mutants.Check(t.Context(), []string{f.path}, checkOptions(f))
	if err != nil {
		t.Fatal(err)
	}
	if !mutants.NeedsMutationCoverage(checks, false) {
		t.Error("a recorded flaky mutant needs no run")
	}
	flip(f, site, mutants.Survived)
	f.run(t)
	if _, m := recorded(t, f, site); m.Flaky || m.Outcome != mutants.Survived {
		t.Fatalf("recorded %+v, want a survivor, not flaky", m)
	}
}

func TestAFailFastRunStopsAtAFlakyMutantOnceItsRunsAreDone(t *testing.T) {
	f, site := survivingFixture(t)
	flip(f, site, mutants.Killed, mutants.Survived)
	r, stop, err := mutants.RunFailFast(t.Context(), []string{f.path}, f.options, mutants.FailFast{})
	if err != nil {
		t.Fatal(err)
	}
	if stop == nil || stop.Rule != "mutation.flaky" || stop.Site == nil || stop.Site.Key() != site.Key() {
		t.Fatalf("stop %+v, want mutation.flaky at %s", stop, site.Key())
	}
	if m := decidedAt(t, r[0], site); m.State != mutants.StateCompleted || len(m.Outcomes) != 2 {
		t.Errorf("the flaky mutant is listed %+v, want completed with its outcomes", m)
	}
}

func TestAFailFastStopKeepsAFlakyMutantItLeftUndecided(t *testing.T) {
	f, first := survivingFixture(t)
	parsed := f.snapshots.Values["board.go"].Units[0].Mutants
	last := parsed[len(parsed)-1]
	site := mutants.Site{Offset: last.Offset, Original: last.Original, Replacement: last.Replacement}
	if site.Key() == first.Key() {
		t.Fatal("the fixture has one site")
	}
	flip(f, site, mutants.Killed, mutants.Survived)
	f.run(t)
	if _, m := recorded(t, f, site); !m.Flaky {
		t.Fatalf("recorded %+v, want flaky", m)
	}
	// The first survivor stops the run before the flaky mutant is judged.
	r, stop, err := mutants.RunFailFast(t.Context(), []string{f.path}, f.options, mutants.FailFast{})
	if err != nil || stop == nil || stop.Rule != "mutation.survived" || !r[0].Incomplete {
		t.Fatalf("results %+v, stop %+v, %v: want a survivor's stop leaving the file incomplete", r, stop, err)
	}
	if _, m := recorded(t, f, site); !m.Flaky || !reflect.DeepEqual(m.Outcomes, []string{mutants.Killed, mutants.Survived}) {
		t.Fatalf("kept %+v, want it flaky as recorded", m)
	}
}

func TestAFunctionNotJudgedKeepsItsFlakyMutant(t *testing.T) {
	f, site := survivingFixture(t)
	flip(f, site, mutants.Killed, mutants.Survived)
	f.run(t)
	f.options.Judge = func(string, string, string) bool { return false }
	r := f.run(t)[0]
	for _, u := range r.Snapshot.Units {
		for _, m := range u.Mutants {
			if m.Key() == site.Key() && (!m.Flaky || u.Killed != 0) {
				t.Fatalf("kept %+v, killed %d: want it flaky, counted neither", m, u.Killed)
			}
		}
	}
}

func TestNoExceptionTakesAFlakyMutantNorGoesStaleByIt(t *testing.T) {
	f, site := survivingFixture(t)
	flip(f, site, mutants.Killed, mutants.Survived)
	f.run(t)
	parsed, err := selected.Parse(t.Context(), f.options.Tree, f.inputs, f.path)
	if err != nil {
		t.Fatal(err)
	}
	defer parsed.Close()
	u := parsed.Units[site.Unit]
	f.options.Exceptions = []mutants.Exception{{File: "board.go", Function: mutants.UnitID(u.Namespace, u.Name), Hash: mutants.UnitHash(parsed, u),
		LineInFunction: site.Line - u.StartLine + 1, Column: site.Column, Original: site.Original, Replacement: site.Replacement, Reason: "equivalent"}}
	// Its first run kills it again, which would stale its exception.
	r := f.run(t)[0]
	if m := decidedAt(t, r, site); m.Excepted != "" || len(m.Outcomes) != 2 || len(r.StaleExceptions) != 0 {
		t.Fatalf("listed %+v, stale %+v: want it flaky, neither excepted nor staling its exception", m, r.StaleExceptions)
	}
	checks, err := mutants.Check(t.Context(), []string{f.path}, checkOptions(f))
	if err != nil {
		t.Fatal(err)
	}
	for _, fn := range checks[0].Functions {
		for _, m := range fn.Mutants {
			if m.Key() == site.Key() && (!m.Flaky || m.Excepted != "") {
				t.Errorf("check reads %+v, want it flaky and not excepted", m)
			}
		}
	}
	if len(checks[0].StaleExceptions) != 0 {
		t.Errorf("check stales %+v, want none for a flaky mutant", checks[0].StaleExceptions)
	}
}

// anew gives f Workers of their own, over the same files and tests, whose
// runs count from none.
func anew(f *runFixture) {
	f.workers = &porttest.Workers{Files: f.files, Tests: f.workers.Tests}
	f.options.WorkerFactory = f.workers
}

func TestAFailFastRunWaitsForAFlakyMutantsRunsBeforeItStops(t *testing.T) {
	f, site := survivingFixture(t)
	flip(f, site, mutants.Killed)
	f.run(t)
	anew(f)
	f.options.MutateAll = true
	// Its first run survives, which would stop the run as a survivor.
	flip(f, site, mutants.Survived, mutants.Killed)
	_, stop, err := mutants.RunFailFast(t.Context(), []string{f.path}, f.options, mutants.FailFast{})
	if err != nil || stop == nil || stop.Rule != "mutation.flaky" {
		t.Fatalf("stop %+v, %v: want mutation.flaky", stop, err)
	}
}

func TestAMutantWithNoRecordedOutcomeRunsOnceBesideARecheckedOne(t *testing.T) {
	f, site := survivingFixture(t)
	snap := f.snapshots.Values["board.go"]
	u := &snap.Units[0]
	var other mutants.Mutant
	for i, m := range u.Mutants {
		if m.Key() != site.Key() {
			other = m
			u.Mutants = append(u.Mutants[:i:i], u.Mutants[i+1:]...)
			u.Survived--
			u.Sites--
			break
		}
	}
	f.snapshots.Values["board.go"] = snap
	// The recorded survivor is now killed, and runs again; the other one,
	// recorded nowhere, is killed and runs once.
	for command, facts := range f.workers.Tests[f.root] {
		facts.Verdicts = nil
		f.workers.Tests[f.root][command] = facts
	}
	r := f.run(t)[0]
	if r.Ran != 3+1 || decidedAt(t, r, mutants.Site{Offset: other.Offset, Original: other.Original, Replacement: other.Replacement}).Outcome != mutants.Killed {
		t.Fatalf("ran %d, %+v: want the recorded survivor 3 times and the other once", r.Ran, r.Mutants)
	}
}
