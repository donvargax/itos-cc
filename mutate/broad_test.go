package mutate

import (
	"reflect"
	"testing"
)

func TestBroadEvidenceAppliesToEveryBroadOutcomeAndNoNarrowOutcome(t *testing.T) {
	oldTests := map[string]string{"app_test.go": "before"}
	nowTests := map[string]string{"app_test.go": "after"}
	oldSupport := map[string]string{"features/steps.go": "before"}
	nowSupport := map[string]string{"features/steps.go": "after"}
	mutants := []Mutant{
		{Scope: ScopeOwn, Outcome: Killed},
		{Scope: ScopeListed, Outcome: Killed, Tests: []string{"ID-1"}},
		{Scope: ScopeAllTests, Outcome: Survived, GoEvidence: &GoEvidence{Tests: oldTests, Support: oldSupport}},
		{Scope: "go test ./...", Outcome: Survived, Excepted: "equivalent", GoEvidence: &GoEvidence{Tests: oldTests, Support: oldSupport}},
	}
	changed := BroadChangesForUnit(mutants, nowTests, nowSupport)
	want := []string{"app_test.go", "features/steps.go"}
	if !reflect.DeepEqual(changed, want) {
		t.Fatalf("broad changes %v, want %v", changed, want)
	}
	if got := BroadChanges(mutants[0], nowTests, nowSupport); len(got) != 0 {
		t.Fatalf("own outcome depends on broad inputs: %v", got)
	}
	if got := BroadChanges(mutants[1], nowTests, nowSupport); len(got) != 0 {
		t.Fatalf("listed outcome depends on Go module inputs: %v", got)
	}
}
