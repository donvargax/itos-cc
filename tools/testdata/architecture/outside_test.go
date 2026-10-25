package architecture_test

import (
	"os/exec"
	"strings"
	"testing"
)

// checkOutside runs architecture-check --outside over TestArchitecture's
// fixtures in dir, the temporary module, with env, its environment: cmd
// accepted, every other import refused. config's row went when it became
// mutation's settings adapter under internal/slices, and graph's when it
// became a slice (T-208).
func checkOutside(t *testing.T, check, dir string, env []string, module string) {
	t.Helper()
	outsideCases := []struct{ pkg, imported, from string }{
		{"./cmd/probe/...", "", ""},
		{"./cmd/slices", "", ""},
		{"./internal/project/units", "units", ""},
		{"./internal/covrun/mutation_domain", "mutation/domain", " from its tests"},
		{"./features/dry_domain", "dry/domain", ""},
		{"./server/toplevel_graph_code", "graph", ""},
		{"./server/toplevel_graph_test", "graph", " from its tests"},
	}
	for _, c := range outsideCases {
		t.Run("outside "+c.pkg, func(t *testing.T) {
			run := exec.Command("sh", check, "--outside", c.pkg)
			run.Dir = dir
			run.Env = env
			output, err := run.CombinedOutput()
			if c.imported == "" {
				if err != nil {
					t.Errorf("refused cmd importing the slices: %v\n%s", err, output)
				}
				return
			}
			importer := module + "/" + strings.TrimPrefix(c.pkg, "./")
			imported := module + "/internal/slices/" + c.imported
			if exit, ok := err.(*exec.ExitError); !ok || exit.ExitCode() != 1 {
				t.Fatalf("missing refusal of %s importing %s: %v\n%s", importer, imported, err, output)
			}
			for _, text := range []string{importer + " imports " + imported + c.from + ";", "ADR-0051"} {
				if !strings.Contains(string(output), text) {
					t.Errorf("the refusal does not say %q:\n%s", text, output)
				}
			}
		})
	}
}
