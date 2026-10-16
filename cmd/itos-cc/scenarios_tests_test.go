package main

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/donvargax/itos-cc/features"
)

// Mutation execution moves in S7. This declaration contract already holds
// different commands and same-command replacement without scripted runs.
func TestMemoryTestDeclarationsKeepCommandsIndependent(t *testing.T) {
	m := newMemory(t)
	dir, err := m.Folder()
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Write(filepath.Join(dir, "src", "board.ts"), []byte("export function board() { return true; }\n")); err != nil {
		t.Fatal(err)
	}
	own := features.Tests{Language: "typescript", Command: "vitest related src/board.ts", Baseline: "old"}
	all := features.Tests{Language: "typescript", Command: "make test", Trace: &features.TestTrace{File: filepath.Join(dir, "trace"), Source: "src/board.ts"}}
	for _, tests := range []features.Tests{own, all} {
		if err := m.Tests(dir, tests); err != nil {
			t.Fatal(err)
		}
	}
	own.Baseline = "new"
	if err := m.Tests(dir, own); err != nil {
		t.Fatal(err)
	}
	want := map[string]features.Tests{own.Command: own, all.Command: all}
	if !reflect.DeepEqual(m.tests[dir], want) {
		t.Fatalf("declarations %v, want both commands with only own replaced: %v", m.tests[dir], want)
	}
}
