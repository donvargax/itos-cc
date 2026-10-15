package mutants

import (
	"sort"
)

// HasBroadOutcome says whether units record an outcome of a broad scope,
// whose freshness needs SuiteTestHashes.
func HasBroadOutcome(units []UnitResult) bool {
	for _, u := range units {
		for _, m := range u.Mutants {
			if BroadScope(m.TestScope()) {
				return true
			}
		}
	}
	return false
}

func BroadScope(Scope string) bool {
	return Scope == ScopeAllTests || Scope != "" && Scope != ScopeOwn && Scope != ScopeListed
}

func RecordSuiteEvidence(s *Snapshot, Tests, support map[string]string) {
	if Tests == nil {
		Tests = map[string]string{}
	}
	if support == nil {
		support = map[string]string{}
	}
	for i := range s.Units {
		for j := range s.Units[i].Mutants {
			m := &s.Units[i].Mutants[j]
			if BroadScope(m.TestScope()) {
				m.SuiteEvidence = &SuiteEvidence{Tests: CloneHashes(Tests), Support: CloneHashes(support)}
			}
		}
	}
}

func CloneHashes(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for path, hash := range in {
		out[path] = hash
	}
	return out
}

// BroadChanges is the changed evidence of a broad-scope outcome, of any
// language: tests are SuiteTestHashes of its source now, support the
// configured support files' hashes now.
func BroadChanges(m Mutant, Tests, support map[string]string) []string {
	if !BroadScope(m.TestScope()) {
		return nil
	}
	if m.SuiteEvidence == nil || m.SuiteEvidence.Tests == nil || m.SuiteEvidence.Support == nil {
		return []string{"whole-suite evidence"}
	}
	return ChangedFiles(m.SuiteEvidence.Tests, Tests, m.SuiteEvidence.Support, support)
}

// BroadChangesForUnit returns the changed inputs of broad-scope outcomes in
// one unit, sorted and unique.
func BroadChangesForUnit(mutants []Mutant, Tests, support map[string]string) []string {
	var changed []string
	for _, m := range mutants {
		changed = append(changed, BroadChanges(m, Tests, support)...)
	}
	return UniqueSorted(changed)
}

func ChangedFiles(oldTests, Tests, oldSupport, support map[string]string) []string {
	testChanges := CompareTests(oldTests, Tests)
	changed := append([]string{}, testChanges.Changed...)
	changed = append(changed, testChanges.Added...)
	changed = append(changed, testChanges.Removed...)
	changed = append(changed, CompareTests(oldSupport, support).Changed...)
	d := CompareTests(oldSupport, support)
	changed = append(changed, d.Added...)
	changed = append(changed, d.Removed...)
	return UniqueSorted(changed)
}

func UniqueSorted(paths []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(paths))
	for _, path := range paths {
		if !seen[path] {
			seen[path] = true
			out = append(out, path)
		}
	}
	// compareTests already sorts each group; sort the combined evidence.
	sort.Strings(out)
	return out
}
