package testinputs_test

import (
	"maps"
	"os"
	"path/filepath"
	"testing"

	"github.com/donvargax/itos-cc/internal/mutants/porttest"
	"github.com/donvargax/itos-cc/internal/testinputs"
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
