package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/donvargax/itos-cc/internal/metricsdir"
)

// Guards of "Rule: A reviewed exception can be renewed when its mutant is
// unchanged" in features/mutate.feature that no scenario reaches; ID-MUT-227
// holds the except-renew slice's old test whole (T-101). They drive the CLI
// and read itos-cc.yaml and the --json object as raw data. The rule's
// stale-fix-renew test, at the end, uses the module of
// mutation_except_test.go instead.
//
// Each function of the module has a survivor no test kills, its `>` or
// `<` at the 7th column of its if line, the function's 3rd: Label's, in label.go, stands for the equivalent mutant a
// person reviewed. other.go's Sign and Grade, and size.go's Size, hold the
// exceptions that must not be renewed: Sign's mutant a new test kills,
// Grade's line changes, and Size is never run again.
var renewFiles = map[string]string{
	"go.mod": "module example.com/renew\n\ngo 1.22\n",
	"label.go": "package renew\n\n// Label is n's name, marked when n is positive.\n" +
		"func Label(n int) string {\n\tname := \"old\"\n\tif n > 0 {\n\t\treturn name + \"+\"\n\t}\n\treturn name\n}\n",
	"other.go": "package renew\n\n// Sign says whether n is negative.\n" +
		"func Sign(n int) string {\n\tword := \"old\"\n\tif n < 0 {\n\t\treturn word + \"-\"\n\t}\n\treturn word\n}\n\n" +
		"// Grade says whether n is big.\n" +
		"func Grade(n int) string {\n\tword := \"old\"\n\tif n > 9 {\n\t\treturn word + \"!\"\n\t}\n\treturn word\n}\n",
	"size.go": "package renew\n\n// Size says whether n is large.\n" +
		"func Size(n int) string {\n\tword := \"old\"\n\tif n > 99 {\n\t\treturn word + \"#\"\n\t}\n\treturn word\n}\n",
	"label_test.go": "package renew\n\nimport \"testing\"\n\n" +
		"func TestLabel(t *testing.T) {\n\tif Label(1) != \"old+\" || Label(-1) != \"old\" {\n\t\tt.Fatal(\"Label\")\n\t}\n}\n\n" +
		"func TestSign(t *testing.T) {\n\tif Sign(-1) != \"old-\" || Sign(1) != \"old\" {\n\t\tt.Fatal(\"Sign\")\n\t}\n}\n\n" +
		"func TestGrade(t *testing.T) {\n\tif Grade(20) != \"old!\" || Grade(1) != \"old\" {\n\t\tt.Fatal(\"Grade\")\n\t}\n}\n\n" +
		"func TestSize(t *testing.T) {\n\tif Size(200) != \"old#\" || Size(1) != \"old\" {\n\t\tt.Fatal(\"Size\")\n\t}\n}\n",
}

const (
	renewLabel = "example.com/renew#Label"
	renewSign  = "example.com/renew#Sign"
	renewGrade = "example.com/renew#Grade"
	renewSize  = "example.com/renew#Size"
)

// renewJSON is what the --json object of mutation except --renew holds,
// as raw maps.
type renewJSON struct {
	Renewed    []map[string]any `json:"renewed"`
	NotRenewed []map[string]any `json:"not_renewed"`
	Problems   []map[string]any `json:"problems"`
}

func (o outcome) renew(t *testing.T) renewJSON {
	t.Helper()
	var out renewJSON
	if err := json.Unmarshal([]byte(o.stdout), &out); err != nil {
		t.Fatalf("stdout is not one JSON object: %v\n%s\nstderr:\n%s", err, o.stdout, o.stderr)
	}
	return out
}

// renewHash is the hash the snapshot of file records for function.
func renewHash(t *testing.T, file, function string) string {
	t.Helper()
	snap, err := loadMutationSnapshot(t.Context(), metricsdir.Store{RootDir: wd(t)}, file)
	if err != nil || snap == nil {
		t.Fatalf("no snapshot of %s: %v", file, err)
	}
	for _, u := range snap.Units {
		if u.Namespace+"#"+u.Name == function {
			return u.Hash
		}
	}
	t.Fatalf("the snapshot of %s records no %s", file, function)
	return ""
}

// mutation except --renew takes no site and no reason: it keeps each
// entry's own.
func TestRenewingTakesNoSiteAndNoReason(t *testing.T) {
	t.Parallel()
	inEmptyDir(t)
	for _, c := range []struct {
		args []string
		rule string
	}{
		{[]string{"--renew", "--reason", "why", "--json"}, "flags.conflict"},
		{[]string{"--renew", "label.go:6:7", "--json"}, "args.unexpected"},
	} {
		o := mutationExcept(t, c.args...)
		named := false
		for _, p := range rawProblems(t, o) {
			message, _ := p["message"].(string)
			named = named || p["rule"] == c.rule && strings.Contains(message, "--renew")
		}
		if o.code != 80 || !named {
			t.Errorf("mutation except %s: exit %d, problems %v; want 80, %s naming --renew", strings.Join(c.args, " "), o.code, rawProblems(t, o), c.rule)
		}
	}
}

// An entry written before entries recorded their line's text renews by its
// place in the function, and follows its function across a renamed module,
// which changes the function's namespace, as a template's render does
// (#32).
func TestALegacyExceptionRenewsByItsPlaceAcrossARenamedModule(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go is not installed")
	}
	dir := t.TempDir()
	for name, text := range renewFiles {
		writeFile(t, filepath.Join(dir, name), text)
	}
	useDir(t, dir)
	if o := mutateRun(t); o.code != 1 {
		t.Fatalf("the first run: exit %d, want 1 for the survivors\n%s%s", o.code, o.stdout, o.stderr)
	}
	legacy := exceptionYAML{File: "label.go", Function: renewLabel, Hash: renewHash(t, "label.go", renewLabel), LineInFunction: 3, Column: 7,
		Original: ">", Replacement: ">=", Reason: "reviewed before line texts"}
	writeExceptions(t, "# Reviewed by hand.\n", legacy)
	edit(t, "go.mod", "module example.com/renew", "module example.com/renamed")
	run := mutateRun(t, "--json", "label.go")
	logRun(t, &run)

	renew := mutationExcept(t, "--renew", "--json")
	logRun(t, &renew)
	r := renew.renew(t)
	const renamed = "example.com/renamed#Label"
	if len(r.Renewed) != 1 || r.Renewed[0]["function"] != renamed || r.Renewed[0]["old_function"] != renewLabel || r.Renewed[0]["match"] != "place" || renew.code != 0 {
		t.Fatalf("the renewal: exit %d, renewed %v, not renewed %v; want 0 and Label renewed by its place, from %s to %s", renew.code, r.Renewed, r.NotRenewed, renewLabel, renamed)
	}
	want := legacy
	want.Function, want.Hash = renamed, renewHash(t, "label.go", renamed)
	if got := readExceptions(t); !slices.Equal(got, []exceptionYAML{want}) {
		t.Errorf("mutation.exceptions %+v, want %+v", got, want)
	}
	data, err := os.ReadFile(filepath.Join(dir, "itos-cc.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if data := string(data); !strings.Contains(data, "# Reviewed by hand.") || !strings.Contains(data, "line_text: if n > 0 {") {
		t.Errorf("itos-cc.yaml:\n%s\nwant its comment kept and the renewed entry's line text", data)
	}
	if check := mutationCheck(t, "--json", "label.go"); check.code != 0 {
		t.Errorf("the check after the renewal: exit %d, want 0\n%s", check.code, check.stdout)
	}
}

// @ID-MUT-242. Kept: the fix names the site to except again and
// itos-cc.yaml, which the scenario does not assert (T-101).
func TestAStaleExceptionWhoseFunctionChangedNamesTheRenewalAsItsFix(t *testing.T) {
	t.Parallel()
	// Given itos-cc.yaml excepts a survivor of clear, and clear changed
	// since the entry was written
	boardRepo(t, nil)
	survivorRun(t)
	writeExceptions(t, "", clearException(t, "clear is never called"))
	edit(t, boardSource, "i < 3", "i < 4")

	run := mutateRun(t, "--json", boardSource)
	p := run.json(t).problem("mutation.exception-stale")
	wantProblem(t, p, map[string]any{"file": boardSource, "function": clearID, "why": "changed"}, run.stdout)
	fix, _ := p["fix"].(string)
	renewAt, reasonAt := strings.Index(fix, "itos-cc mutation except --renew"), strings.Index(fix, "--reason")
	if renewAt < 0 || reasonAt < 0 || renewAt > reasonAt {
		t.Errorf("mutation run's fix %q, want it to name 'itos-cc mutation except --renew' before '--reason'", fix)
	}
	if want := "'itos-cc mutation except " + boardSource + ":"; !strings.Contains(fix, want) || !strings.Contains(fix, "itos-cc.yaml") {
		t.Errorf("mutation run's fix %q, want it to name %s<line>:<column> and itos-cc.yaml", fix, want)
	}

	check := mutationCheck(t, "--json", boardSource)
	c := check.json(t).problem("mutation.exception-stale")
	wantProblem(t, c, map[string]any{"file": boardSource, "function": clearID, "why": "changed"}, check.stdout)
	if c["fix"] != fix {
		t.Errorf("mutation check's fix %q, want mutation run's, %q", c["fix"], fix)
	}
}
