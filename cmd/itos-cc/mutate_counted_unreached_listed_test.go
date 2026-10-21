//go:build !windows

package main

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	mutation "github.com/donvargax/itos-cc/internal/slices/mutation/domain"
)

// The scenario of "Rule: Mutation results say what happened, and leave
// nothing stale behind" in features/mutation.feature that the
// counted-unreached-listed-fix slice holds. calc.py has one mutation site,
// mid's == 5. No own test reaches it: e2e_mid.py loads calc with importlib,
// by a name the graph does not see, the way an end-to-end test reaches code,
// and it is a listed test, mid, whose run through pytest executes mid's line
// and notices == → !=. Counted runs refuse Windows (#29), so the file builds
// elsewhere only. It needs a real python3 with pytest and coverage.py 7.13 or
// later, and skips, naming the missing one, without them; CI runs it on
// ubuntu-latest (T-13).

var countedUnreachedFiles = map[string]string{
	".gitignore":     ".metrics\n",
	"pyproject.toml": "[project]\nname = \"calc\"\nversion = \"0.0.0\"\n",
	"calc.py":        "def mid(i):\n    return i == 5\n",
	"e2e_mid.py": "import importlib\n\n\n" +
		"def test_mid():\n    calc = importlib.import_module(\"c\" + \"alc\")\n" +
		"    assert calc.mid(5)\n    assert not calc.mid(4)\n",
	"tests.txt": "mid\te2e_mid.py\n",
	"itos-cc.yaml": "mutation:\n  tests:\n" +
		"    list: cat tests.txt\n" +
		"    run: python3 -m pytest -q -p no:cacheprovider e2e_mid.py -k \"{pattern}\"\n" +
		"    ids_pattern: \"{ids}\"\n" +
		"    join:\n      each: \"{id}\"\n      sep: \" or \"\n",
}

// @ID-MUT-232. Kept: preparation's skipped own baseline and the trial's
// listed stages, which the scenario does not assert (T-101).
func TestACountedRunJudgesAnUnreachedFileByTheListedTestsThatReachIt(t *testing.T) {
	t.Parallel()
	requireCountedPlatform(t)
	languageTools(t, "python")
	useEnv(t, "PYTEST_DISABLE_PLUGIN_AUTOLOAD", "1")
	// Given a committed Python project whose file no own test reaches, but a
	// listed test reaches one of its lines and kills its mutant
	dir := t.TempDir()
	for name, text := range countedUnreachedFiles {
		writeFile(t, filepath.Join(dir, name), text)
	}
	gitIn(t, dir, "init", "-q")
	gitIn(t, dir, "add", ".")
	gitIn(t, dir, "commit", "-qm", "calc")
	useDir(t, dir)
	var chosen []mutation.FreshCandidate
	seed := seedSelecting(t, dir, 1, selecting(&chosen, func(c mutation.FreshCandidate) bool {
		return c.Path == "calc.py" && c.Site.Original == "==" && c.Site.Replacement == "!="
	}))

	// When a count-one run judges that file
	o := countedRun(t, "--count", "1", "--seed", seed, "--workers", "1", "calc.py")
	logOnFailure(t, &o)
	c := o.counted(t)

	// Then its mutant is judged by the listed test and killed, as a complete
	// run judges it
	if len(c.Selected) != 1 {
		t.Fatalf("selected = %+v, want calc.py's one site", c.Selected)
	}
	site := c.Selected[0]
	if site["state"] != "judged" || site["outcome"] != "killed" || site["scope"] != "listed" || !slices.Equal(siteTests(site), []string{"mid"}) {
		t.Errorf("selected %+v, want it judged killed with scope listed by [mid]", site)
	}
	if o.code != 0 || c.number("executed") != 1 || c.Sampling["completion"] != "completed" {
		t.Errorf("exit %d, executed %d, completion %v; want 0, the one trial, completed", o.code, c.number("executed"), c.Sampling["completion"])
	}

	// And no "no clean baseline was prepared" problem is reported
	if strings.Contains(o.stdout, "no clean baseline was prepared") {
		t.Errorf("the run says no clean baseline was prepared:\n%s", o.stdout)
	}
	// No own test ran for calc.py: preparation ran no own baseline, and its
	// trial ran the listed selection's clean baseline, then the mutant under
	// it, with no own stage.
	if b := c.stage("baseline"); b == nil || b["state"] != "skipped" {
		t.Errorf("preparation's baseline stage %v, want skipped: no own test reaches calc.py", b)
	}
	var stages []string
	list, _ := site["stages"].([]any)
	for _, stage := range list {
		m, _ := stage.(map[string]any)
		name, _ := m["name"].(string)
		state, _ := m["state"].(string)
		stages = append(stages, name+" "+state)
	}
	if want := []string{"listed-baseline complete", "listed complete"}; !slices.Equal(stages, want) {
		t.Errorf("the trial's stages %q, want %q", stages, want)
	}
}
