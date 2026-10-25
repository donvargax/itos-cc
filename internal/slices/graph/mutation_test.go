package graph

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/donvargax/itos-cc/internal/lang"
)

// judging is a fake of the Mutation port that judges by a snapshot file's
// presence alone: a source whose snapshot file is under its project's
// .metrics/mutate is mutated, each of its units killing what killed says
// of its key; any other has no results. Snapshot fails with snapshotErr and
// Tests with testsErr when they are set.
type judging struct {
	killed      map[string]int
	snapshotErr error
	testsErr    error
}

func (judging) Facts(f *lang.File) any { return len(f.Units) }

func (judging) SnapshotPath(top, key string) string {
	return filepath.Join(top, ".metrics", "mutate", key+".json")
}

func (j judging) Snapshot(_ context.Context, _, key string) (any, error) {
	return key, j.snapshotErr
}

func (judging) Support(context.Context, string) map[string]string { return nil }

func (j judging) Tests(context.Context, string, []string) (map[string]string, error) {
	return map[string]string{}, j.testsErr
}

func (j judging) Judge(_ context.Context, file Judged) []UnitMutation {
	out := make([]UnitMutation, file.Facts.(int))
	if key, ok := file.Snapshot.(string); ok {
		for i := range out {
			out[i] = UnitMutation{Mutated: true, Killed: j.killed[key]}
		}
	}
	return out
}

// A builder needs its Mutation port: none is refused, never read from the
// disk instead.
func TestABuilderWithoutItsMutationPortIsRefused(t *testing.T) {
	if _, err := NewBuilder([]string{"testdata/shop"}, nil); err == nil {
		t.Fatal("a builder without its Mutation port was made")
	}
}

// What the Mutation port fails to read, a file's snapshot or the hashes of
// the tests that import it, fails the build with its error.
func TestTheMutationPortsFailuresFailTheBuild(t *testing.T) {
	root := t.TempDir()
	write := func(rel, text string) {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("go.mod", "module example.com/p\n\ngo 1.22\n")
	write("a.go", "package p\n\nfunc A(i int) bool { return i > 0 }\n")
	write("a_test.go", "package p\n\nimport \"testing\"\n\nfunc TestA(t *testing.T) { _ = A(1) }\n")
	write(".metrics/mutate/a.go.json", "{}")
	failure := errors.New("unbound")
	for name, port := range map[string]judging{"snapshot": {snapshotErr: failure}, "tests": {testsErr: failure}} {
		t.Run(name, func(t *testing.T) {
			b, err := NewBuilder([]string{root}, port)
			if err != nil {
				t.Fatal(err)
			}
			if _, _, err := b.Build(t.Context()); !errors.Is(err, failure) {
				t.Errorf("build: %v, want the port's failure", err)
			}
		})
	}
}
