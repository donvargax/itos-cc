package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/donvargax/itos-cc/features"
)

// Beside the in-process scenarios of mutate.feature, this focused handler
// regression uses real disposable source discovery and the very Workers
// binding built from memory.Driver.Tests; no test command reaches the host.
func TestMutationHandlerBindsDeclaredWorkersAndActualCopyTrace(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)
	m := newMemory(t)
	src := "package board\nfunc Flip(v bool) bool { return !v }\n"
	for name, text := range map[string]string{"go.mod": "module example.com/board\n\ngo 1.27\n", "board.go": src} {
		path := filepath.Join(root, name)
		if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := m.Write(path, []byte(text)); err != nil {
			t.Fatal(err)
		}
	}
	trace := filepath.Join(root, "outside", "trace.jsonl")
	declared := features.Tests{Language: "go", Command: "go test -count=1 -failfast ./.", Surviving: []features.Site{{File: "board.go", Line: 2, Original: "!", Replacement: ""}}, Trace: &features.TestTrace{File: trace, Source: "board.go"}}
	if err := m.Tests(root, declared); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	a := diskAdapters(&stderr, nil)
	a.Workers = m.workers()
	args := []string{"mutation", "run", "--json", "--no-coverage", "--no-annotate", "--mutate-all", "board.go"}
	invoke := func() (int, string, string) {
		var code int
		stdout.Reset()
		stderr.Reset()
		out, log := captured(t, func() { code = run(t.Context(), args, strings.NewReader(""), &stdout, &stderr, a) })
		return code, out + stdout.String(), log + stderr.String()
	}
	code, out, log := invoke()
	var actual map[string]any
	if err := json.Unmarshal([]byte(out), &actual); err != nil {
		t.Fatalf("JSON: %v %s", err, out)
	}
	if code != 1 || !strings.Contains(out, "mutation.survived") {
		t.Fatalf("declared survivor lost: exit%d %s\n%s", code, out, log)
	}
	lines := strings.Split(strings.TrimSpace(m.files[trace]), "\n")
	if len(lines) != 2 {
		t.Fatalf("actual bound baseline+trial missing: %q", m.files[trace])
	}
	var hashes []string
	for _, line := range lines {
		var r map[string]string
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			t.Fatal(err)
		}
		if r["cwd"] == root || r["original_root"] != root {
			t.Fatalf("not actual virtual copy: %v", r)
		}
		hashes = append(hashes, r["source_sha256"])
	}
	original := sha256.Sum256([]byte(src))
	mutated := sha256.Sum256([]byte(strings.Replace(src, "!v", "v", 1)))
	if !reflect.DeepEqual(hashes, []string{hex.EncodeToString(original[:]), hex.EncodeToString(mutated[:])}) {
		t.Fatalf("actual copied source hashes: %v", hashes)
	}
	if m.files[filepath.Join(root, "board.go")] != src {
		t.Fatal("live memory source changed")
	}
	// Missing injection is an error, never the legacy disk fallback.
	stdout.Reset()
	stderr.Reset()
	a.Workers = nil
	if code, out, _ := invoke(); code != 100 || !strings.Contains(out, "Workers binding is missing") {
		t.Fatalf("missing binding escaped: %d %s", code, out)
	}
	// Wrong command binding is actually reached by the same handler.
	delete(m.tests[root], declared.Command)
	stdout.Reset()
	stderr.Reset()
	a.Workers = m.workers()
	if code, out, _ := invoke(); code != 100 || !strings.Contains(out, "no tests declared") {
		t.Fatalf("wrong binding escaped: %d %s", code, out)
	}
}
