package testtools_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/donvargax/itos-cc/internal/slices/mutation/adapters/testtools"
	"github.com/donvargax/itos-cc/internal/slices/mutation/domain/porttest"
)

func TestRealToolchainContract(t *testing.T) {
	porttest.ToolchainContract(t, func(t *testing.T, files map[string]string, _ map[string][]string, env map[string]string) porttest.InputBindings {
		t.Helper()
		root := t.TempDir()
		for name, text := range files {
			path := filepath.Join(root, filepath.FromSlash(name))
			if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(text), 0644); err != nil {
				t.Fatal(err)
			}
		}
		var vars []string
		if env != nil {
			vars = os.Environ()
			for name, value := range env {
				vars = append(vars, name+"="+value)
			}
		}
		return porttest.InputBindings{Root: root, Toolchain: testtools.Toolchain{Dir: root, Env: vars}}
	})
}
