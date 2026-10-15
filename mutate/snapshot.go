package mutate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"

	snapport "github.com/donvargax/itos-cc/internal/snapshot/port"
	"github.com/donvargax/itos-cc/project"
)

// scopeCommand is the command that runs path's tests in scope, tests the
// test files that reach it.
func scopeCommand(ctx context.Context, path, scope string, tests []string) Command {
	switch scope {
	case ScopeOwn, ScopeListed, "":
		return TestCommand(ctx, path, "", false, tests)
	case ScopeAllTests:
		return TestCommand(ctx, path, "", true, nil)
	}
	return TestCommand(ctx, path, scope, false, nil)
}

// SnapshotName is where the snapshot of the file at rel, its path from the
// project root (project.FromRoot), lives under .metrics.
func SnapshotName(rel string) string {
	return filepath.Join("mutate", filepath.ToSlash(rel)+".json")
}

// LoadSnapshotOf reads the snapshot of the file at rel, its path from the
// root of the project whose snapshots store holds, which need not be the
// working directory's, or returns nil when there is none: the snapshot
// SnapshotName(rel), which holds while it names the file rel. One that
// names another file is none, so the file's functions are missing. Every
// reader looks a file's snapshot up here, mutation check and the
// architecture graph alike. A snapshot that cannot be read is the store's
// failure.
//
// An outcome it records as its file's own tests', where those run the whole
// suite (OwnScope), reads as ScopeAllTests, the scope that decided it: one
// recorded before that rule has no whole-suite evidence, so it reads stale
// once.
func LoadSnapshotOf(ctx context.Context, store snapport.Store, rel string) (*Snapshot, error) {
	root, failure := store.Root(ctx)
	if failure != nil {
		return nil, failure
	}
	var encoded snapshotWire
	ok, failure := store.Read(ctx, SnapshotName(rel), &encoded)
	if failure != nil {
		return nil, failure
	}
	s := encoded.domain()
	if !ok || s.File != filepath.ToSlash(rel) {
		return nil, nil
	}
	if OwnScope(ctx, filepath.Join(root, filepath.FromSlash(rel))) == ScopeAllTests {
		for i := range s.Units {
			for j := range s.Units[i].Mutants {
				if m := &s.Units[i].Mutants[j]; m.TestScope() == ScopeOwn {
					m.Scope = ScopeAllTests
				}
			}
		}
	}
	return &s, nil
}

// TestHashes is what a snapshot records of the test files tests: the
// SHA-256 of each one's content, by its slash-separated path from the
// project root (project.FromRoot).
func TestHashes(tests []string) (map[string]string, error) {
	return testHashes(tests, project.FromRoot)
}

// TestHashesUnder is TestHashes for the project at root, which need not be
// the working directory's: each test file by its slash-separated path from
// root, as a run in that project records it.
func TestHashesUnder(root string, tests []string) (map[string]string, error) {
	return testHashes(tests, func(path string) string {
		if rel, err := filepath.Rel(root, path); err == nil && !strings.HasPrefix(rel, "..") {
			return rel
		}
		return path
	})
}

func testHashes(tests []string, rel func(string) string) (map[string]string, error) {
	out := map[string]string{}
	for _, path := range tests {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		sum := sha256.Sum256(data)
		out[filepath.ToSlash(rel(path))] = hex.EncodeToString(sum[:])
	}
	return out, nil
}
