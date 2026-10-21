package freshenv

import (
	"context"
	"runtime"
	"slices"
	"testing"

	mutants "github.com/donvargax/itos-cc/internal/slices/mutation/domain"
	"github.com/donvargax/itos-cc/internal/testtools"
	"github.com/donvargax/itos-cc/internal/workdir"
	"github.com/donvargax/itos-cc/internal/worktrees"
)

// The Kotlin twin of @ID-MUT-232: a counted trial of a Kotlin file no own
// test class reaches has no own-test command, so it runs no own stage, and
// the listed tests that reach its line judge it after their selection's
// clean baseline, as a complete run judges it. The listed test is a shell
// line that passes on the committed file and fails on the mutant, so no
// Gradle runs.
func TestAFreshTrialOfAFileNoOwnTestReachesIsJudgedByItsListedTests(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the listed test is a POSIX shell line; counted runs refuse Windows (#29)")
	}
	repo := freshFixture(t, map[string]string{
		"build.gradle.kts":       "plugins { jacoco }\n",
		"src/main/kotlin/a/A.kt": "package a\n\nfun low(i: Int): Boolean {\n    return i == 3\n}\n",
	})
	plan, err := PlanFresh(t.Context(), repo, nil, "", 1, "unreached")
	if err != nil {
		t.Fatal(err)
	}
	cleanupFreshPlan(t, plan)
	if len(plan.Selected) != 1 || plan.Selected[0].Site.Original != "==" {
		t.Fatalf("selected %+v, want A.kt's one site", plan.Selected)
	}
	c := plan.Selected[0]
	listed := &mutants.Listed{Root: plan.FrozenRoot, Select: func([]string) string {
		return "grep -q 'i == 3' src/main/kotlin/a/A.kt"
	}}
	trials := []mutants.FreshTrial{
		{Candidate: c, Baseline: FreshBaseline{Paths: []string{c.Path}}, Reach: []string{"low"}},
		{Candidate: c, Baseline: FreshBaseline{Paths: []string{c.Path}}},
	}
	results, err := mutants.RunFreshTrials(context.Background(), plan, trials, mutants.FreshTrialOptions{WorkerFactory: worktrees.Workers{}, Tree: workdir.Tree{}, Toolchain: testtools.Toolchain{}, Workers: 1, TimeoutFactor: 10, Scope: mutants.ScopeOwn, Listed: listed})
	if err != nil {
		t.Fatal(err)
	}
	judgedByListed := results[0]
	var stages []string
	for _, s := range judgedByListed.Stages {
		stages = append(stages, s.Name+" "+s.State)
	}
	if judgedByListed.Err != nil || judgedByListed.Outcome != mutants.Killed || judgedByListed.Scope != mutants.ScopeListed ||
		!slices.Equal(judgedByListed.Tests, []string{"low"}) || judgedByListed.RanOwn() {
		t.Errorf("the trial reached by a listed test: %+v, want it killed by [low], with no own stage", judgedByListed)
	}
	if want := []string{"listed-baseline complete", "listed complete"}; !slices.Equal(stages, want) {
		t.Errorf("stages %q, want %q", stages, want)
	}
	if unreached := results[1]; unreached.Err == nil || unreached.Outcome != "" || len(unreached.Stages) != 0 {
		t.Errorf("the trial no listed test reaches: %+v, want it not judged, with nothing run", unreached)
	}
}
