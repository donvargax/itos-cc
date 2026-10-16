package features

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestBinaryToolSetupObservesActualInstalledEntries(t *testing.T) {
	root := t.TempDir()
	driver := binary{}
	if err := driver.Tools(t.Context(), root, ToolSetup{NodeBins: []string{"vitest"}}); err == nil {
		t.Fatal("unobserved installed entry accepted")
	}
	for _, name := range []string{"vitest", "vitest.cmd"} {
		path := filepath.Join(root, "node_modules", ".bin", name)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		// The existing capability checks Stat, not executable permissions.
		if err := os.WriteFile(path, []byte("independent installed marker"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := driver.Tools(t.Context(), root, ToolSetup{NodeBins: []string{"vitest"}}); err != nil {
		t.Fatal(err)
	}
	if err := driver.Tools(t.Context(), root, ToolSetup{NodeBins: []string{"../escape"}}); err == nil {
		t.Fatal("unsupported installed name accepted")
	}
	cause := errors.New("caller stopped setup")
	ctx, cancel := context.WithCancelCause(t.Context())
	cancel(cause)
	if err := driver.Tools(ctx, root, ToolSetup{NodeBins: []string{"missing"}, Python: &PythonProbe{Interpreter: "missing"}}); !errors.Is(err, cause) {
		t.Fatalf("cancellation did not precede observations: %v", err)
	}
}
