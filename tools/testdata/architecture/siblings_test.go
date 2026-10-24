package architecture_test

import (
	"os/exec"
	"strings"
	"testing"
)

// checkSiblings runs architecture-check --siblings over TestArchitecture's
// fixtures in dir, the temporary module, with env, its environment: a
// slice's own subpackages, an engine, cmd and graph accepted, a slice
// importing a sibling's package refused, naming both.
func checkSiblings(t *testing.T, check, dir string, env []string, module string) {
	t.Helper()
	slicePath := func(name string) string { return module + "/internal/slices/" + name }
	siblingCases := []struct{ pkg, imported string }{
		{"./internal/slices/alpha/...", ""},
		{"./cmd/slices", ""},
		{"./internal/slices/beta", "alpha"},
		{"./internal/slices/gamma", "alpha/store"},
		// Mutation's domain is for its own slice, its porttest, its adapters
		// and what is outside internal/slices, graph, since every adapter of
		// it, its settings adapter last (T-197), is under internal/slices now
		// (T-161); another slice's domain importing it, which depguard lets
		// by, is refused.
		{"./internal/slices/mutation/own_domain_allowed_code", ""},
		{"./internal/slices/mutation/domain/porttest/domain_allowed_code", ""},
		{"./graph/mutation", ""},
	}
	for _, domain := range []string{"dry", "scrap", "crap"} {
		for _, kind := range []string{"code", "test"} {
			siblingCases = append(siblingCases, struct{ pkg, imported string }{"./internal/slices/" + domain + "/domain/mutation_domain_" + kind, "mutation/domain"})
		}
	}
	// The mutation slice's adapters, under internal/slices/mutation/adapters
	// (T-159, T-160, T-161, T-197), are the slice's: each importing the slice's domain, its
	// fake from its tests, passes, as the mutation slice importing its own
	// adapter does here, which slice-adapters refuses instead, in TestArchitecture's lint.
	// Another slice importing one, which slice-adapters refuses too, is
	// refused here, from its code or its tests.
	for _, adapter := range []string{"testtools", "testinputs", "worktrees", "githistory", "importgraph", "freshenv", "snapshots", "exceptions", "sources", "settings"} {
		path := "internal/slices/mutation/adapters/" + adapter
		siblingCases = append(siblingCases,
			struct{ pkg, imported string }{"./" + path + "/internal_slices_mutation_domain_allowed", ""},
			struct{ pkg, imported string }{"./" + path + "/fake_allowed", ""},
		)
		for _, kind := range []string{"code", "test"} {
			fixture := strings.ReplaceAll(path, "/", "_") + "_" + kind
			siblingCases = append(siblingCases, struct{ pkg, imported string }{"./internal/slices/mutation/" + fixture, ""})
			for _, other := range []string{"units", "dry", "scrap", "crap", "version"} {
				siblingCases = append(siblingCases, struct{ pkg, imported string }{"./internal/slices/" + other + "/" + fixture, "mutation/adapters/" + adapter})
			}
		}
	}
	for _, c := range siblingCases {
		t.Run("siblings "+c.pkg, func(t *testing.T) {
			run := exec.Command("sh", check, "--siblings", c.pkg)
			run.Dir = dir
			run.Env = env
			output, err := run.CombinedOutput()
			if c.imported == "" {
				if err != nil {
					t.Errorf("refused a slice's own subpackage, an engine or cmd importing the slices: %v\n%s", err, output)
				}
				return
			}
			importer := module + "/" + strings.TrimPrefix(c.pkg, "./")
			if exit, ok := err.(*exec.ExitError); !ok || exit.ExitCode() != 1 {
				t.Fatalf("missing refusal of %s importing %s: %v\n%s", importer, slicePath(c.imported), err, output)
			}
			for _, text := range []string{importer + " imports " + slicePath(c.imported), "ADR-0051"} {
				if !strings.Contains(string(output), text) {
					t.Errorf("the refusal does not say %q:\n%s", text, output)
				}
			}
		})
	}
}
