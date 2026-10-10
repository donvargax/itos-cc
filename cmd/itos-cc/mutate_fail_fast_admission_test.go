package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// The admission scenario of "Rule: mutation run --fail-fast stops at the
// first actionable failure" in features/mutate.feature. It builds on every
// platform: on Windows it sees the refusal itself, elsewhere with the
// platform pretended, as ID-MUT-191 does for counted mode. No mutant runs.

// @ID-MUT-199
func TestFailFastAdmissionBoundaries(t *testing.T) {
	readme, err := os.ReadFile(filepath.Join("..", "..", "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	docs, err := os.ReadFile(filepath.Join("..", "..", "docs", "CLI.md"))
	if err != nil {
		t.Fatal(err)
	}
	moduleRepo(t, compareFiles)
	problemOf := func(o outcome, rule string) map[string]any {
		t.Helper()
		var out struct {
			Problems []map[string]any `json:"problems"`
		}
		if err := json.Unmarshal([]byte(o.stdout), &out); err != nil {
			t.Fatalf("stdout is not one JSON object: %v\n%s\nstderr:\n%s", err, o.stdout, o.stderr)
		}
		for _, p := range out.Problems {
			if p["rule"] == rule {
				return p
			}
		}
		return nil
	}

	// --count with --fail-fast is a flags.conflict usage error.
	o := cli(t, "mutation", "run", "--json", "--count", "1", "--fail-fast")
	if p := problemOf(o, "flags.conflict"); o.code != 2 || p == nil || p["flag"] != "--fail-fast" {
		t.Errorf("--count with --fail-fast: exit %d, want 2 with flags.conflict for --fail-fast\n%s%s", o.code, o.stdout, o.stderr)
	}

	// On Windows it fails with fail-fast.platform before launching commands.
	if runtime.GOOS != "windows" {
		countedPlatform = "windows"
		defer func() { countedPlatform = runtime.GOOS }()
	}
	o = cli(t, "mutation", "run", "--json", "--fail-fast")
	countedPlatform = runtime.GOOS
	if p := problemOf(o, "fail-fast.platform"); o.code != 3 || p == nil || p["platform"] != "windows" {
		t.Errorf("--fail-fast on Windows: exit %d, want 3 with fail-fast.platform for windows\n%s%s", o.code, o.stdout, o.stderr)
	}
	for _, launched := range []string{"itos-cc: coverage", "itos-cc: baseline", "itos-cc: ["} {
		if strings.Contains(o.stderr, launched) {
			t.Errorf("--fail-fast on Windows launched a command (%q):\n%s", launched, o.stderr)
		}
	}
	if _, err := os.Stat(filepath.Join(".metrics", "mutate", "main.go.json")); err == nil {
		t.Error("--fail-fast on Windows wrote a snapshot")
	}

	// The help, README and docs/CLI.md describe --fail-fast, its rule and
	// its platforms.
	help := cli(t, "mutation", "run", "--help").stdout
	for name, text := range map[string]string{"mutation run --help": help, "README.md": string(readme), "docs/CLI.md": string(docs)} {
		for _, want := range []string{"--fail-fast", "fail-fast.platform", "Linux and macOS", `"stop"`, "unattempted"} {
			if !strings.Contains(text, want) {
				t.Errorf("%s does not describe %s", name, want)
			}
		}
	}
}
