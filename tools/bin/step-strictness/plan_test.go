package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fixturePlan(t *testing.T) (string, string, plan) {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, "features")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(dir, "steps.go")
	// Include legitimate uncommitted text and executable permission in the baseline.
	if err := os.WriteFile(source, []byte("dirty WIP\nwant := 1\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	p := plan{Version: 1, Root: ".", DeadlineSeconds: 600, Probes: []probe{{Name: "count", ScenarioID: "ID-DEMO-01", SourcePath: "features/steps.go", Original: "want := 1", Replacement: "want := 2", Assertion: reviewedProbe().Assertion}}}
	return filepath.Join(root, "plan.json"), source, p
}
func savePlan(t *testing.T, path string, p plan) {
	t.Helper()
	b, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, b, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestManifestValidationBeforeEdits(t *testing.T) {
	for _, tc := range []struct {
		name  string
		alter func(*plan)
	}{
		{"unknown version", func(p *plan) { p.Version = 2 }},
		{"duplicate labels", func(p *plan) { p.Probes = append(p.Probes, p.Probes[0]) }},
		{"missing target", func(p *plan) { p.Probes[0].Original = "not here" }},
		{"ambiguous target", func(p *plan) { p.Probes[0].Original = "\n" }},
		{"empty edit", func(p *plan) { p.Probes[0].Replacement = "" }},
		{"same edit", func(p *plan) { p.Probes[0].Replacement = p.Probes[0].Original }},
		{"product source", func(p *plan) { p.Probes[0].SourcePath = "mutate.go" }},
		{"traversal", func(p *plan) { p.Probes[0].SourcePath = "../outside.go" }},
		{"output overlap", func(p *plan) { p.OutputDir = "features/steps.go" }},
		{"formatter separator in output", func(p *plan) { p.OutputDir = "../evidence,pretty" }},
		{"invalid deadline", func(p *plan) { p.DeadlineSeconds = 601 }},
		{"scenario regex", func(p *plan) { p.Probes[0].ScenarioID = "ID-DEMO-.*" }},
		{"no assertion token", func(p *plan) { p.Probes[0].Assertion.ErrorToken = " " }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path, source, p := fixturePlan(t)
			before, err := os.ReadFile(source)
			if err != nil {
				t.Fatal(err)
			}
			tc.alter(&p)
			savePlan(t, path, p)
			if _, err = prepare(path); err == nil {
				t.Fatal("invalid manifest accepted")
			}
			after, err := os.ReadFile(source)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(before, after) {
				t.Fatal("validation changed source")
			}
		})
	}
}

func TestManifestRejectsUnknownDuplicateAndNullKeys(t *testing.T) {
	path, _, p := fixturePlan(t)
	savePlan(t, path, p)
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, data := range []string{
		strings.Replace(string(b), `"version":1`, `"version":1,"version":1`, 1),
		strings.Replace(string(b), `"name":"count"`, `"name":"count","extra":1`, 1),
		strings.Replace(string(b), `"error_token":"wrong count"`, `"error_token":"wrong count","errror":"typo"`, 1),
		strings.Replace(string(b), `"deadline_seconds":600`, `"deadline_seconds":null`, 1),
	} {
		if err = os.WriteFile(path, []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err = prepare(path); err == nil {
			t.Fatalf("accepted %s", data)
		}
	}
}

func TestManifestRejectsSymlinkedSources(t *testing.T) {
	path, source, p := fixturePlan(t)
	link := filepath.Join(filepath.Dir(source), "linked.go")
	if err := os.Symlink(source, link); err != nil {
		t.Skipf("native symlink unavailable: %v", err)
	}
	p.Probes[0].SourcePath = "features/linked.go"
	savePlan(t, path, p)
	if _, err := prepare(path); err == nil {
		t.Fatal("symlinked edit accepted")
	}
}

func TestSpecialModesAreRefused(t *testing.T) {
	for _, mode := range []os.FileMode{os.ModeSetuid, os.ModeSetgid, os.ModeSticky} {
		if err := sourceMode(0o700 | mode); err == nil {
			t.Fatalf("accepted special mode %v", mode)
		}
	}
	if err := sourceMode(0o700); err != nil {
		t.Fatal(err)
	}
}

func TestWindowsExecutionRefusedBeforeWriting(t *testing.T) {
	path, source, p := fixturePlan(t)
	savePlan(t, path, p)
	before, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	var out, stderr bytes.Buffer
	if code := command(context.Background(), []string{"-plan", path}, &out, &stderr, "windows"); code != 2 || !strings.Contains(stderr.String(), "no files were written") {
		t.Fatalf("code %d: %s", code, stderr.String())
	}
	after, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != len(before) {
		t.Fatal("refusal created files")
	}
	b, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "dirty WIP\nwant := 1\n" {
		t.Fatal("refusal changed source")
	}
	if code := command(context.Background(), []string{"-plan", path, "-print-plan"}, &out, &stderr, "windows"); code != 0 {
		t.Fatalf("portable validation failed: %s", stderr.String())
	}
	if !strings.Contains(out.String(), `"deadline_seconds": 600`) {
		t.Fatal("defaults not visible")
	}
}
