package testinputs_test

import (
	"crypto/sha256"
	"encoding/hex"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"

	"github.com/donvargax/itos-cc/internal/slices/mutation/adapters/testinputs"
	"github.com/donvargax/itos-cc/internal/slices/mutation/domain/porttest"
)

func TestRealFingerprintsContract(t *testing.T) {
	porttest.FingerprintsContract(t, func(t *testing.T, files map[string]string, _ map[string][]string, env map[string]string) porttest.InputBindings {
		t.Helper()
		root := filepath.Join(t.TempDir(), "project")
		for name, text := range files {
			path := filepath.Join(root, filepath.FromSlash(name))
			if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(text), 0644); err != nil {
				t.Fatal(err)
			}
		}
		if err := os.MkdirAll(root, 0755); err != nil {
			t.Fatal(err)
		}
		runtime := testinputs.CurrentRuntime()
		runtime.Env = maps.Clone(env)
		return porttest.InputBindings{Root: root, Fingerprints: testinputs.Fingerprints{Runtime: runtime}}
	})
}

// What only a real repository can hold: in one, the Go files are those git
// tracks, read from the working tree, so an untracked or ignored file is no
// input, nor a tracked one deleted, and a committed testdata file still
// none.
func TestGoCoverageInputsInARepositoryAreTheTrackedFiles(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	root := t.TempDir()
	for name, text := range map[string]string{
		"go.mod": "module board\n", "a.go": "package board\n", "gone.go": "package board\n",
		"sub/b.go": "package sub\n", "testdata/fixture.go": "package fixture\n", ".gitignore": "dist/\n",
	} {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, args := range [][]string{{"init", "-q"}, {"add", "-A"}, {"-c", "user.name=t", "-c", "user.email=t@t", "commit", "-qm", "base"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	for _, name := range []string{"untracked.go", "dist/x.go"} {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("package x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Remove(filepath.Join(root, "gone.go")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "a.go"), []byte("package board\n\nvar A = 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := testinputs.GoCoverageInputs(t.Context(), testinputs.CurrentRuntime(), filepath.Join(root, "a.go"), root, "producer", nil)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"@go-env", "@producer", "a.go", "go.mod", "itos-cc.yaml", "sub/b.go"}; !slices.Equal(slices.Sorted(maps.Keys(got)), want) {
		t.Errorf("inputs %v, want the keys %v", slices.Sorted(maps.Keys(got)), want)
	}
	sum := sha256.Sum256([]byte("package board\n\nvar A = 1\n"))
	if got["a.go"] != hex.EncodeToString(sum[:]) {
		t.Errorf("a.go hashed %s, not its working tree's bytes", got["a.go"])
	}
}

// What only a disk can hold: an itos-cc.yaml the system cannot read, here
// a folder of that name, fails Go's coverage inputs and every other
// language's, never reading as no file. Every command refuses such a file
// before it fingerprints anything, so no scenario reaches this.
func TestCoverageInputsFailOnAnUnreadableItosCCYAML(t *testing.T) {
	for source, files := range map[string]map[string]string{
		"a.go": {"go.mod": "module board\n", "a.go": "package board\n"},
		"a.py": {"pyproject.toml": "", "a.py": "x = 1\n"},
	} {
		root := t.TempDir()
		for name, text := range files {
			if err := os.WriteFile(filepath.Join(root, name), []byte(text), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		if err := os.Mkdir(filepath.Join(root, "itos-cc.yaml"), 0o755); err != nil {
			t.Fatal(err)
		}
		f := testinputs.Fingerprints{Runtime: testinputs.CurrentRuntime()}
		if got, failure := f.Coverage(t.Context(), filepath.Join(root, source), root, "producer", nil); failure == nil {
			t.Errorf("%s: an itos-cc.yaml folder was measured: %v", source, got)
		}
	}
}
