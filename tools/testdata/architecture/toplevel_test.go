package architecture_test

import (
	"os/exec"
	"strings"
	"testing"
)

// toplevelFixture is a package TestArchitecture writes in its temporary
// module for architecture-check --toplevel: layer/name importing the
// module's imported, from its code or its tests, refused or accepted.
type toplevelFixture struct {
	layer, name, imported string
	test, refused         bool
}

// Every package but cmd lives under internal/ (ADR-0061), so a package
// outside internal/ and cmd/ imports none under it, from its code or its
// tests, but the rows docs/go.md names: features internal/project alone,
// tools/bin/release-notes and release-version internal/release alone, its
// releasetest with it, and, until graph-snapshot, graph and server
// whatever they import; metrics', coverage's and config's rows went when
// they moved under internal/.
var toplevelFixtures = []toplevelFixture{
	{"features", "toplevel_project_code", "internal/project", false, false},
	{"features", "toplevel_project_test", "internal/project", true, false},
	{"tools/bin/release-notes", "toplevel_release_code", "internal/release", false, false},
	{"tools/bin/release-version", "toplevel_releasetest_test", "internal/release/releasetest", true, false},
	{"server", "toplevel_lang_code", "internal/lang", false, false},
	{"server", "toplevel_lang_test", "internal/lang", true, false},
	{"library", "toplevel_lang_code", "internal/lang", false, true},
	{"library", "toplevel_lang_test", "internal/lang", true, true},
	{"features", "toplevel_lang_code", "internal/lang", false, true},
	{"features", "toplevel_lang_test", "internal/lang", true, true},
	{"tools/bin/release-notes", "toplevel_lang_code", "internal/lang", false, true},
	{"tools/bin/release-version", "toplevel_lang_test", "internal/lang", true, true},
	{"tools/bin/other", "toplevel_release_code", "internal/release", false, true},
	{"metrics", "toplevel_snapshot_code", "internal/snapshot", false, true},
	{"coverage", "toplevel_coverage_code", "internal/coverage", false, true},
	{"config", "toplevel_mutation_code", "internal/slices/mutation/domain", false, true},
}

// addToplevel writes toplevelFixtures through TestArchitecture's add, each
// one depguard lets by, since no rule holds a package outside internal/.
func addToplevel(module string, add func(name, layer, imported, symbol, rule string, test bool)) {
	for _, fixture := range toplevelFixtures {
		add(fixture.name, fixture.layer, module+"/"+fixture.imported, "Value", "", fixture.test)
	}
}

// checkToplevel runs architecture-check --toplevel over TestArchitecture's
// fixtures in dir, the temporary module, with env, its environment: cmd,
// what is under internal/ and each named row accepted, the rest refused,
// naming both packages and whether from the importer's tests. Beside
// toplevelFixtures it runs over the fixtures the other checks wrote:
// cmd/probe importing every package under internal/, graph's row, and
// features importing dry's domain.
func checkToplevel(t *testing.T, check, dir string, env []string, module string) {
	t.Helper()
	type run struct{ pkg, imported, from string }
	runs := []run{
		{"./cmd/probe/...", "", ""},
		{"./internal/slices/units/...", "", ""},
		{"./graph/...", "", ""},
		{"./features/dry_domain", "internal/slices/dry/domain", ""},
	}
	for _, fixture := range toplevelFixtures {
		c := run{pkg: "./" + fixture.layer + "/" + fixture.name}
		if fixture.refused {
			c.imported = fixture.imported
			if fixture.test {
				c.from = " from its tests"
			}
		}
		runs = append(runs, c)
	}
	for _, c := range runs {
		t.Run("toplevel "+c.pkg, func(t *testing.T) {
			command := exec.Command("sh", check, "--toplevel", c.pkg)
			command.Dir = dir
			command.Env = env
			output, err := command.CombinedOutput()
			if c.imported == "" {
				if err != nil {
					t.Errorf("refused cmd, a package under internal/ or a row importing what it allows: %v\n%s", err, output)
				}
				return
			}
			importer := module + "/" + strings.TrimPrefix(c.pkg, "./")
			imported := module + "/" + c.imported
			if exit, ok := err.(*exec.ExitError); !ok || exit.ExitCode() != 1 {
				t.Fatalf("missing refusal of %s importing %s: %v\n%s", importer, imported, err, output)
			}
			for _, text := range []string{importer + " imports " + imported + c.from + ";", "ADR-0061"} {
				if !strings.Contains(string(output), text) {
					t.Errorf("the refusal does not say %q:\n%s", text, output)
				}
			}
		})
	}
}
