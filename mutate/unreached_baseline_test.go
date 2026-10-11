package mutate

import (
	"io"
	"os"
	"path/filepath"
	"testing"
)

// The Kotlin twin of @ID-MUT-231: a Kotlin file none of whose reaching
// tests declares a test class runs nothing, as a Python file no test
// reaches does, so its baseline is not run, in a run with or without
// --fail-fast. No Gradle runs: no command does.
func TestAKotlinFileNoTestClassReachesReportsItsBaselineAsNotRun(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"build.gradle.kts":               "plugins { jacoco }\n",
		"src/main/kotlin/a/A.kt":         "package a\n\nfun low(i: Int): Boolean {\n    return i == 3\n}\n",
		"src/test/kotlin/a/Helper.kt":    "package a\n\nobject Helper\n",
		"src/test/kotlin/a/Unrelated.kt": "package a\n\nclass Unrelated\n",
	}
	for name, text := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	t.Chdir(dir)
	src := filepath.Join(dir, "src", "main", "kotlin", "a", "A.kt")
	helper := filepath.Join(dir, "src", "test", "kotlin", "a", "Helper.kt")
	opt := Options{Workers: 1, Log: io.Discard, Tests: func(string) []string { return []string{helper} }}
	if c := TestCommand(src, "", false, []string{helper}); !c.RunsNothing() {
		t.Fatalf("A.kt's own tests run %q, want nothing: Helper.kt declares no test class", c)
	}
	for _, failFast := range []bool{false, true} {
		var results []FileResult
		var err error
		if failFast {
			results, _, err = RunFailFast([]string{src}, opt, FailFast{})
		} else {
			results, err = Run([]string{src}, opt)
		}
		if err != nil {
			t.Fatalf("fail-fast %v: %v", failFast, err)
		}
		if len(results) != 1 || results[0].Baseline != BaselineNotRun || results[0].BaselineFailed {
			t.Fatalf("fail-fast %v: results %+v, want A.kt's baseline %q", failFast, results, BaselineNotRun)
		}
		for _, m := range results[0].Mutants {
			if m.Outcome != Uncovered {
				t.Errorf("fail-fast %v: %+v, want uncovered", failFast, m)
			}
		}
		os.RemoveAll(filepath.Join(dir, ".metrics"))
	}
}
