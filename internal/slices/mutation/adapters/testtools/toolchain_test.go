package testtools_test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
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

// Where go env cannot run, here in a folder that is not there, the
// platform is the GOOS and GOARCH the toolchain's environment sets, an
// empty one left to the system itos-cc runs on. No scenario can arrange
// it: every Go fixture needs go env to run.
func TestGoPlatformWithoutGoEnvIsWhatTheEnvironmentSets(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing")
	tools := testtools.Toolchain{Env: []string{"GOOS=plan9", "GOARCH="}}
	got, failure := tools.GoPlatform(context.Background(), missing)
	if failure != nil {
		t.Fatal(failure)
	}
	if got.OS != "plan9" || got.Arch != runtime.GOARCH {
		t.Errorf("the platform is %s/%s, not plan9/%s", got.OS, got.Arch, runtime.GOARCH)
	}
}
