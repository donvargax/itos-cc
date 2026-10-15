package mutate

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestSuiteTestHashesFollowEachLanguagesBuildRoot(t *testing.T) {
	root := t.TempDir()
	for name, text := range map[string]string{
		"ts/package.json":                        "{}",
		"ts/src/app.ts":                          "",
		"ts/e2e/app.test.ts":                     "",
		"ts/test/helper.ts":                      "",
		"ts/node_modules/dep/dep.test.ts":        "",
		"ts/nested/package.json":                 "{}",
		"ts/nested/nested.test.ts":               "",
		"ts/tests/test_other.py":                 "",
		"py/setup.cfg":                           "",
		"py/pkg/app.py":                          "",
		"py/tests/test_app.py":                   "",
		"py/conftest.py":                         "",
		"py/.venv/lib/test_dep.py":               "",
		"kt/settings.gradle":                     "",
		"kt/app/build.gradle.kts":                "",
		"kt/app/src/main/kotlin/App.kt":          "",
		"kt/app/src/test/kotlin/AppTest.kt":      "",
		"kt/lib/src/test/kotlin/LibTest.kt":      "",
		"mvn/pom.xml":                            "",
		"mvn/src/main/kotlin/App.kt":             "",
		"mvn/src/test/kotlin/AppTest.kt":         "",
		"mvn/child/pom.xml":                      "",
		"mvn/child/src/test/kotlin/ChildTest.kt": "",
		"loose/app.ts":                           "",
		"loose/app.test.ts":                      "",
	} {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for source, want := range map[string][]string{
		"ts/src/app.ts":                 {"ts/e2e/app.test.ts", "ts/test/helper.ts"},
		"py/pkg/app.py":                 {"py/conftest.py", "py/tests/test_app.py"},
		"kt/app/src/main/kotlin/App.kt": {"kt/app/src/test/kotlin/AppTest.kt", "kt/lib/src/test/kotlin/LibTest.kt"},
		"mvn/src/main/kotlin/App.kt":    {"mvn/src/test/kotlin/AppTest.kt"},
		"loose/app.ts":                  {},
	} {
		tests, err := SuiteTestHashes(t.Context(), filepath.Join(root, filepath.FromSlash(source)), root)
		if err != nil {
			t.Fatal(err)
		}
		got := slices.Sorted(func(yield func(string) bool) {
			for name := range tests {
				if !yield(name) {
					return
				}
			}
		})
		if !slices.Equal(got, want) && !(len(got) == 0 && len(want) == 0) {
			t.Errorf("tests of %s's build root: %q, want %q", source, got, want)
		}
	}
}

func TestGoEvidenceReadsAsSuiteEvidenceAndIsWrittenUnderTheNeutralKey(t *testing.T) {
	var m Mutant
	if err := readMutantJSON([]byte(`{"outcome":"killed","scope":"all-tests","go_evidence":{"tests":{"a_test.go":"h"},"support":{}}}`), &m); err != nil {
		t.Fatal(err)
	}
	if m.SuiteEvidence == nil || m.SuiteEvidence.Tests["a_test.go"] != "h" {
		t.Fatalf("go_evidence read as %+v, want the suite evidence", m.SuiteEvidence)
	}
	data, err := json.Marshal(MutantJSON(m))
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	if _, ok := raw["go_evidence"]; ok || raw["suite_evidence"] == nil {
		t.Errorf("written as %s, want suite_evidence alone", data)
	}
}
