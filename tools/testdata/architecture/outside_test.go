package architecture_test

import (
	"os/exec"
	"strings"
	"testing"
)

// checkOutside runs architecture-check --outside over TestArchitecture's
// fixtures in dir, the temporary module, with env, its environment: cmd
// and graph's and config's rows accepted, every other import refused.
func checkOutside(t *testing.T, check, dir string, env []string, module string) {
	t.Helper()
	outsideCases := []struct{ pkg, imported, from string }{
		{"./cmd/probe/...", "", ""},
		{"./cmd/slices", "", ""},
		{"./graph/mutation", "", ""},
		{"./config/mutation", "", ""},
		{"./graph/testtools", "", ""},
		{"./config/fake", "", ""},
		{"./internal/project/units", "units", ""},
		{"./coverage/mutation_domain", "mutation/domain", " from its tests"},
		{"./features/dry_domain", "dry/domain", ""},
		{"./graph/crap_domain", "crap/domain", ""},
		{"./graph/snapshots", "mutation/adapters/snapshots", ""},
		{"./config/units", "units", ""},
	}
	for _, c := range outsideCases {
		t.Run("outside "+c.pkg, func(t *testing.T) {
			run := exec.Command("sh", check, "--outside", c.pkg)
			run.Dir = dir
			run.Env = env
			output, err := run.CombinedOutput()
			if c.imported == "" {
				if err != nil {
					t.Errorf("refused cmd or a legacy consumer importing what its row allows: %v\n%s", err, output)
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
