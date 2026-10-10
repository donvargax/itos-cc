package main

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

// The scenario of crap-unloaded-measured in features/crap.feature: a file no
// test loads, scored alone. It uses the TypeScript project of
// mutate_unloaded_test.go (ID-MUT-105), whose one test imports src/board.ts
// and never src/unused.ts, and a real Vitest, which finds no test related
// to src/unused.ts, exits 0 and writes an empty lcov.info.

type crapJSON struct {
	Entries []struct {
		Name     string   `json:"name"`
		File     string   `json:"file"`
		Coverage *float64 `json:"coverage"`
		CRAP     *float64 `json:"crap"`
	} `json:"entries"`
	Problems []map[string]any `json:"problems"`
}

// crapOf is crap's --json output in o.
func crapOf(t *testing.T, o outcome) crapJSON {
	t.Helper()
	var out crapJSON
	if err := json.Unmarshal([]byte(o.stdout), &out); err != nil {
		t.Fatalf("stdout is not one JSON object: %v\n%s\nstderr:\n%s", err, o.stdout, o.stderr)
	}
	return out
}

// wantUntested fails t unless every function of src/unused.ts in c has 0%
// coverage.
func wantUntested(t *testing.T, c crapJSON, o outcome) {
	t.Helper()
	n := 0
	for _, e := range c.Entries {
		if filepath.Clean(e.File) != unusedSource {
			continue
		}
		n++
		if e.Coverage == nil || *e.Coverage != 0 {
			t.Errorf("%s: coverage %v, want 0%%\nstdout:\n%s\nstderr:\n%s", e.Name, e.Coverage, o.stdout, o.stderr)
		}
	}
	if n != 2 {
		t.Errorf("%d functions of %s scored, want 2\n%s", n, unusedSource, o.stdout)
	}
}

// @ID-CRAP-14
func TestAFileNoTestLoadsScoredAloneIsUntestedNotUnmeasured(t *testing.T) {
	t.Parallel()
	vitestRepo(t)

	o := cli(t, "crap", "--json", "--threshold", "30", unusedSource)
	c := crapOf(t, o)
	wantUntested(t, c, o)
	for _, p := range c.Problems {
		if p["rule"] == "coverage.measured-nothing" {
			t.Errorf("problem %v, want no coverage.measured-nothing: the coverage run succeeded", p)
		}
	}
	if o.code != 0 {
		t.Errorf("exit %d, want 0: with no branch, each function at 0%% scores 2\n%s%s", o.code, o.stdout, o.stderr)
	}
}

// Without --threshold, the file scores 0% as with one, and stderr warns of
// no missing coverage.
func TestWithoutAThresholdAFileNoTestLoadsIsUntestedWithNoWarning(t *testing.T) {
	t.Parallel()
	vitestRepo(t)

	o := cli(t, "crap", "--json", unusedSource)
	wantUntested(t, crapOf(t, o), o)
	if strings.Contains(o.stderr, "itos-cc: no coverage") {
		t.Errorf("stderr warns of missing coverage:\n%s", o.stderr)
	}
	if o.code != 0 {
		t.Errorf("exit %d, want 0", o.code)
	}
}
