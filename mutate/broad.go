package mutate

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/donvargax/itos-cc/lang"
	"github.com/donvargax/itos-cc/project"
)

// SuiteTestHashes returns every test file of source's language beneath its
// build root, relative to projectRoot: what a whole-suite outcome of source
// depends on besides its support files (ADR-0016). Go's is its nearest
// module (GoModuleTestHashes). TypeScript's is the nearest package.json,
// Python's the nearest pyproject.toml, setup.py or setup.cfg, and Kotlin's
// the Gradle build root, which holds settings.gradle or settings.gradle.kts,
// or else the Maven module, the nearest pom.xml. Tests are those project
// discovery classifies as tests, so node_modules, .venv and the like are
// never walked, and conftest.py counts; a test beneath a nested build root
// of the language belongs to that root. A source with no build root has
// none.
func SuiteTestHashes(source, projectRoot string) (map[string]string, error) {
	spec := lang.Detect(source)
	if spec == nil {
		return map[string]string{}, nil
	}
	if spec.Name == "go" {
		return GoModuleTestHashes(source, projectRoot)
	}
	root := suiteRoot(spec.Name, source)
	if root == "" {
		return map[string]string{}, nil
	}
	files, err := project.Discover([]string{root})
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, test := range files.Tests {
		if s := lang.Detect(test); s == nil || s.Name != spec.Name || suiteRoot(spec.Name, test) != root {
			continue
		}
		rel, err := filepath.Rel(projectRoot, test)
		if err != nil {
			return nil, err
		}
		out[filepath.ToSlash(rel)] = fileHash(test)
	}
	return out, nil
}

// suiteRoot is the build root of path, a file of language, as
// SuiteTestHashes names it; "" for none.
func suiteRoot(language, path string) string {
	switch language {
	case "typescript":
		return lang.FindUp(path, "package.json")
	case "python":
		return lang.FindUp(path, "pyproject.toml", "setup.py", "setup.cfg")
	case "kotlin":
		if root := lang.FindUp(path, "settings.gradle.kts", "settings.gradle"); root != "" {
			return root
		}
		return lang.FindUp(path, "pom.xml")
	}
	return ""
}

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

// HasBroadOutcome says whether units record an outcome of a broad scope,
// whose freshness needs SuiteTestHashes.
func HasBroadOutcome(units []UnitResult) bool {
	for _, u := range units {
		for _, m := range u.Mutants {
			if broadScope(m.TestScope()) {
				return true
			}
		}
	}
	return false
}

func broadScope(scope string) bool {
	return scope == ScopeAllTests || scope != "" && scope != ScopeOwn && scope != ScopeListed
}

func recordSuiteEvidence(s *Snapshot, tests, support map[string]string) {
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
				m.SuiteEvidence = &SuiteEvidence{Tests: cloneHashes(tests), Support: cloneHashes(support)}
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

// BroadChanges is the changed evidence of a broad-scope outcome, of any
// language: tests are SuiteTestHashes of its source now, support the
// configured support files' hashes now.
func BroadChanges(m Mutant, tests, support map[string]string) []string {
	if !broadScope(m.TestScope()) {
		return nil
	}
	if m.SuiteEvidence == nil || m.SuiteEvidence.Tests == nil || m.SuiteEvidence.Support == nil {
		return []string{"whole-suite evidence"}
	}
	return changedFiles(m.SuiteEvidence.Tests, tests, m.SuiteEvidence.Support, support)
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
