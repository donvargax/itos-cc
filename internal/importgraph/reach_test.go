package importgraph_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/donvargax/itos-cc/internal/importgraph"
	"github.com/donvargax/itos-cc/internal/slices/mutation/domain/porttest"
)

func TestRealReachContract(t *testing.T) {
	porttest.ReachContract(t, func(t *testing.T, files map[string]string, _ map[string][]string, _ map[string]string) porttest.InputBindings {
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
		return porttest.InputBindings{Root: root, Reach: importgraph.Reach{}}
	})
}
