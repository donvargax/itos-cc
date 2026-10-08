package mutate

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// GoModuleTestHashes returns every Go test file in source's nearest module,
// relative to projectRoot. Nested modules are separate suites and are skipped.
func GoModuleTestHashes(source, projectRoot string) (map[string]string, error) {
	abs, err := filepath.Abs(source)
	if err != nil {
		return nil, err
	}
	module := filepath.Dir(abs)
	for {
		if _, err := os.Stat(filepath.Join(module, "go.mod")); err == nil {
			break
		}
		parent := filepath.Dir(module)
		if parent == module {
			return map[string]string{}, nil
		}
		module = parent
	}
	return moduleTestsUnder(module, module, projectRoot)
}

func moduleTestsUnder(module, dir, projectRoot string) (map[string]string, error) {
	out := map[string]string{}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		path := filepath.Join(dir, entry.Name())
		if entry.IsDir() {
			if entry.Name() == ".git" {
				continue
			}
			if path != module {
				if _, err := os.Stat(filepath.Join(path, "go.mod")); err == nil {
					continue
				}
			}
			nested, err := moduleTestsUnder(module, path, projectRoot)
			if err != nil {
				return nil, err
			}
			for name, hash := range nested {
				out[name] = hash
			}
			continue
		}
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), "_test.go") {
			rel, err := filepath.Rel(projectRoot, path)
			if err != nil {
				return nil, err
			}
			out[filepath.ToSlash(rel)] = fileHash(path)
		}
	}
	return out, nil
}

func broadScope(scope string) bool {
	return scope == ScopeAllTests || scope != "" && scope != ScopeOwn && scope != ScopeListed
}

func recordGoEvidence(s *Snapshot, tests, support map[string]string) {
	if tests == nil {
		tests = map[string]string{}
	}
	if support == nil {
		support = map[string]string{}
	}
	for i := range s.Units {
		for j := range s.Units[i].Mutants {
			m := &s.Units[i].Mutants[j]
			if broadScope(m.TestScope()) {
				m.GoEvidence = &GoEvidence{Tests: cloneHashes(tests), Support: cloneHashes(support)}
			}
		}
	}
}

func cloneHashes(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for path, hash := range in {
		out[path] = hash
	}
	return out
}

func (s *fileState) markBroadStale() {
	if s.judged == nil || s.stored == nil {
		return
	}
	ids, hashes := fileKeys(s.file)
	for i, entry := range EntriesOf(ids, hashes, s.stored.Units) {
		if s.judged[i] || entry < 0 {
			continue
		}
		unit := &s.result.Snapshot.Units[i]
		for _, m := range s.stored.Units[entry].Mutants {
			if len(BroadChanges(m, s.moduleTests, s.currentSupport)) > 0 {
				unit.Stale = true
				break
			}
		}
	}
}

// BroadChanges is the changed evidence of a Go broad-scope outcome.
func BroadChanges(m Mutant, tests, support map[string]string) []string {
	if !broadScope(m.TestScope()) {
		return nil
	}
	if m.GoEvidence == nil || m.GoEvidence.Tests == nil || m.GoEvidence.Support == nil {
		return []string{"whole-suite evidence"}
	}
	return changedFiles(m.GoEvidence.Tests, tests, m.GoEvidence.Support, support)
}

// BroadChangesForUnit returns the changed inputs of broad-scope outcomes in
// one unit, sorted and unique.
func BroadChangesForUnit(mutants []Mutant, tests, support map[string]string) []string {
	var changed []string
	for _, m := range mutants {
		changed = append(changed, BroadChanges(m, tests, support)...)
	}
	return uniqueSorted(changed)
}

func changedFiles(oldTests, tests, oldSupport, support map[string]string) []string {
	testChanges := compareTests(oldTests, tests)
	changed := append([]string{}, testChanges.Changed...)
	changed = append(changed, testChanges.Added...)
	changed = append(changed, testChanges.Removed...)
	changed = append(changed, compareTests(oldSupport, support).Changed...)
	d := compareTests(oldSupport, support)
	changed = append(changed, d.Added...)
	changed = append(changed, d.Removed...)
	return uniqueSorted(changed)
}

func uniqueSorted(paths []string) []string {
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
