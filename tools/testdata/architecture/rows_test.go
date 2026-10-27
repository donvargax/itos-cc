package architecture_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// rowsCase is a run of a copy of architecture-check beside a rows file of
// its own, tools/architecture-allow.txt (T-222): rows, the file's text, nil
// for no file; flag and pkg, what it runs over TestArchitecture's temporary
// module; exit, the exit wanted; texts, what its output must say.
type rowsCase struct {
	name      string
	rows      *string
	flag, pkg string
	exit      int
	texts     []string
}

// rowsText is a rows file's text, for a rowsCase.
func rowsText(lines ...string) *string {
	text := strings.Join(lines, "\n") + "\n"
	return &text
}

// checkRows runs a copy of check, architecture-check, from a temporary
// folder of its own with a tools/architecture-allow.txt beside it, over
// TestArchitecture's fixtures in dir, the temporary module, with env, its
// environment: a row allows what --toplevel or --outside refuses without
// it, a path ending in /... names the package and those under it and one
// without it that package alone, a row holds for its rule alone, and a
// malformed row or one naming an unknown rule is refused, naming its line,
// as a usage error (exit 2). The module's own rows, read by check itself,
// are what checkToplevel's fixtures prove.
func checkRows(t *testing.T, check, dir string, env []string, module string) {
	t.Helper()
	script, err := os.ReadFile(check)
	if err != nil {
		t.Fatal(err)
	}
	library := module + "/library/toplevel_lang_code"
	libraryTests := module + "/library/toplevel_lang_test"
	lang := module + "/internal/lang"
	features := module + "/features/dry_domain"
	dry := module + "/internal/slices/dry/domain"
	cases := []rowsCase{
		{"a toplevel row allows its importer's code", rowsText("toplevel library/... internal/lang/... # the fixture's row"), "--toplevel", "./library/toplevel_lang_code", 0, nil},
		{"a toplevel row allows its importer's tests", rowsText("toplevel library/... internal/lang # the fixture's row"), "--toplevel", "./library/toplevel_lang_test", 0, nil},
		{"an outside row allows its importer", rowsText("", "# a comment", "  outside features/dry_domain internal/slices/units internal/slices/dry/domain   # the fixture's row"), "--outside", "./features/dry_domain", 0, nil},
		{"no row refuses", rowsText("", "# toplevel library/... internal/lang # a comment, no row", "   "), "--toplevel", "./library/toplevel_lang_code", 1, []string{library + " imports " + lang + ";", "ADR-0061"}},
		{"no rows file refuses", nil, "--toplevel", "./library/toplevel_lang_test", 1, []string{libraryTests + " imports " + lang + " from its tests;", "ADR-0061"}},
		{"a row without /... names its package alone", rowsText("toplevel library internal/lang/... # the fixture's row"), "--toplevel", "./library/toplevel_lang_code", 1, []string{library + " imports " + lang + ";"}},
		{"a row's import without /... names its package alone", rowsText("outside features/dry_domain internal/slices/dry # the fixture's row"), "--outside", "./features/dry_domain", 1, []string{features + " imports " + dry + ";", "ADR-0051"}},
		{"an outside row allows nothing to toplevel", rowsText("outside library/... internal/lang/... # the fixture's row"), "--toplevel", "./library/toplevel_lang_code", 1, []string{library + " imports " + lang + ";"}},
		{"a toplevel row allows nothing to outside", rowsText("toplevel features/... internal/slices/dry/... # the fixture's row"), "--outside", "./features/dry_domain", 1, []string{features + " imports " + dry + ";"}},
		{"a row naming no import is malformed", rowsText("# rows", "", "toplevel library/... # no import"), "--toplevel", "./cmd/probe/...", 2, []string{"architecture-allow.txt:3:", "toplevel library/... # no import"}},
		{"a row naming no reason is malformed", rowsText("toplevel features/... internal/project/...", "toplevel library/... internal/lang"), "--outside", "./cmd/probe/...", 2, []string{"architecture-allow.txt:1:", "toplevel features/... internal/project/..."}},
		{"a row naming an empty reason is malformed", rowsText("toplevel library/... internal/lang #   "), "--toplevel", "./cmd/probe/...", 2, []string{"architecture-allow.txt:1:"}},
		{"a row naming an unknown rule is refused", rowsText("toplevel features/... internal/project/... # a row", "inside library/... internal/lang # the fixture's row"), "--toplevel", "./cmd/probe/...", 2, []string{"architecture-allow.txt:2:", "unknown rule", "inside"}},
	}
	for _, c := range cases {
		t.Run("rows "+c.name, func(t *testing.T) {
			folder := t.TempDir()
			copied := filepath.Join(folder, "tools", "bin", "architecture-check")
			if err := os.MkdirAll(filepath.Dir(copied), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(copied, script, 0o600); err != nil {
				t.Fatal(err)
			}
			if c.rows != nil {
				if err := os.WriteFile(filepath.Join(folder, "tools", "architecture-allow.txt"), []byte(*c.rows), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			run := exec.Command("sh", copied, c.flag, c.pkg)
			run.Dir = dir
			run.Env = env
			output, err := run.CombinedOutput()
			exit := 0
			if err != nil {
				failed, ok := err.(*exec.ExitError)
				if !ok {
					t.Fatal(err)
				}
				exit = failed.ExitCode()
			}
			if exit != c.exit {
				t.Fatalf("%s %s exited %d, want %d:\n%s", c.flag, c.pkg, exit, c.exit, output)
			}
			for _, text := range c.texts {
				if !strings.Contains(string(output), text) {
					t.Errorf("the output does not say %q:\n%s", text, output)
				}
			}
		})
	}
}
