package main

import (
	"encoding/json"
	"maps"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/donvargax/itos-cc/mutate"
)

// The scenario of "Rule: A reviewed exception can be renewed when its
// mutant is unchanged" in features/mutate.feature that the except-renew
// slice holds. It drives the CLI and reads itos-cc.yaml and the --json
// object as raw data, so it compiles against a product without the feature.
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

// renewEntries is itos-cc.yaml's entries by function.
func renewEntries(t *testing.T) map[string]exceptionYAML {
	t.Helper()
	out := map[string]exceptionYAML{}
	for _, e := range readExceptions(t) {
		out[e.Function] = e
	}
	return out
}

// renewHash is the hash the snapshot of file records for function.
func renewHash(t *testing.T, file, function string) string {
	t.Helper()
	snap, err := mutate.LoadSnapshotOf(wd(t), file)
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

// staleExceptions is each mutation.exception-stale problem of a --json
// object, as "function why", sorted.
func staleExceptions(problems []map[string]any) []string {
	var out []string
	for _, p := range problems {
		if p["rule"] == "mutation.exception-stale" {
			out = append(out, p["function"].(string)+" "+p["why"].(string))
		}
	}
	slices.Sort(out)
	return out
}

// renewedTree is every file under the project but itos-cc.yaml.
func renewedTree(t *testing.T) map[string]string {
	t.Helper()
	files := tree(t, wd(t))
	delete(files, "itos-cc.yaml")
	return files
}

// @ID-MUT-227
func TestARenewedExceptionFollowsItsUnchangedMutantAcrossARename(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go is not installed")
	}
	// Given a Go project with a reviewed exception for an equivalent mutant
	// in a function
	dir := t.TempDir()
	for name, text := range renewFiles {
		writeFile(t, filepath.Join(dir, name), text)
	}
	useDir(t, dir)
	if o := mutateRun(t); o.code != 1 {
		t.Fatalf("the first run: exit %d, want 1 for the survivors\n%s%s", o.code, o.stdout, o.stderr)
	}
	for _, site := range []string{"label.go:6:7", "other.go:6:7", "other.go:15:7", "size.go:6:7"} {
		if o := mutationExcept(t, site, "--reason", "reviewed: "+site); o.code != 0 {
			t.Fatalf("excepting %s: exit %d\n%s%s", site, o.code, o.stdout, o.stderr)
		}
	}
	before := renewEntries(t)
	oldLabel := before[renewLabel].Hash

	// And a commit that renames a string literal on another line of that
	// function
	edit(t, "label.go", `name := "old"`, `name := "new"`)
	edit(t, "label_test.go", `Label(1) != "old+" || Label(-1) != "old"`, `Label(1) != "new+" || Label(-1) != "new"`)
	// And "itos-cc mutation run" has judged the function again and the
	// mutant still survives
	run := mutateRun(t, "--json", "label.go")
	logRun(t, &run)
	if got := staleExceptions(rawProblems(t, run)); !slices.Contains(got, renewLabel+" changed") {
		t.Fatalf("the run after the rename reports stale exceptions %q, want %s changed", got, renewLabel)
	}
	// The stale report stands until the entry is renewed (a guardrail).
	stale := mutationCheck(t, "--json", "label.go")
	if got := staleExceptions(rawProblems(t, stale)); !slices.Equal(got, []string{renewLabel + " changed"}) || stale.code != 1 {
		t.Errorf("the check after the rename: exit %d, stale exceptions %q; want 1 with %s changed", stale.code, got, renewLabel)
	}
	newLabel := renewHash(t, "label.go", renewLabel)
	untouched := renewedTree(t)

	// When I run "itos-cc mutation except --renew --json"
	renew := mutationExcept(t, "--renew", "--json")
	logRun(t, &renew)
	r := renew.renew(t)
	// Then the exception names the function's current hash, line and
	// column and keeps its reason
	got := renewEntries(t)
	want := before[renewLabel]
	want.Hash = newLabel
	if got[renewLabel] != want {
		t.Errorf("the renewed entry %+v, want %+v", got[renewLabel], want)
	}
	for _, function := range []string{renewSign, renewGrade, renewSize} {
		if got[function] != before[function] {
			t.Errorf("the entry of %s, which still held, became %+v; want it as it was, %+v", function, got[function], before[function])
		}
	}
	// And the renewal is reported with the old and new hash and the site
	if len(r.Renewed) != 1 {
		t.Errorf("renewed %v, want Label's entry alone", r.Renewed)
	} else if x := r.Renewed[0]; x["function"] != renewLabel || x["old_hash"] != oldLabel || x["new_hash"] != newLabel ||
		x["file"] != "label.go" || x["line"] != 6.0 || x["column"] != 7.0 || x["original"] != ">" || x["replacement"] != ">=" {
		t.Errorf("the renewal reported %v, want label.go:6:7 `>` → `>=` in %s from %s to %s", x, renewLabel, oldLabel, newLabel)
	}
	if renew.code != 0 || len(r.NotRenewed) != 0 {
		t.Errorf("the renewal: exit %d, not renewed %v; want 0 and none", renew.code, r.NotRenewed)
	}
	if after := renewedTree(t); !maps.Equal(after, untouched) {
		t.Error("the renewal wrote a file other than itos-cc.yaml")
	}
	// And "itos-cc mutation check" passes
	if check := mutationCheck(t, "--json", "label.go"); check.code != 0 {
		t.Errorf("the check after the renewal: exit %d, want 0\n%s", check.code, check.stdout)
	}

	// But an exception whose mutant the fresh run killed, whose line
	// changed, or whose function has no fresh results is not renewed, is
	// reported with why, and stays "mutation.exception-stale"
	edit(t, "other.go", "word := \"old\"\n\tif n < 0", "word := \"new\"\n\tif n < 0")
	edit(t, "label_test.go", `Sign(-1) != "old-" || Sign(1) != "old"`, `Sign(-1) != "new-" || Sign(1) != "new" || Sign(0) != "new"`)
	edit(t, "other.go", "word := \"old\"\n\tif n > 9 {", "word := \"new\"\n\tif n > 9 { // big")
	edit(t, "label_test.go", `Grade(20) != "old!" || Grade(1) != "old"`, `Grade(20) != "new!" || Grade(1) != "new"`)
	edit(t, "size.go", `word := "old"`, `word := "new"`)
	edit(t, "label_test.go", `Size(200) != "old#" || Size(1) != "old"`, `Size(200) != "new#" || Size(1) != "new"`)
	again := mutateRun(t, "--json", "label.go", "other.go")
	logRun(t, &again)
	if again.code != 1 {
		t.Fatalf("the run after the further changes: exit %d, want 1", again.code)
	}
	held := renewEntries(t)
	untouched = renewedTree(t)
	refused := mutationExcept(t, "--renew", "--json")
	logRun(t, &refused)
	n := refused.renew(t)
	var why []string
	for _, x := range n.NotRenewed {
		function, _ := x["function"].(string)
		reason, _ := x["why"].(string)
		why = append(why, function+" "+reason)
	}
	slices.Sort(why)
	wantWhy := []string{renewGrade + " changed", renewSign + " killed", renewSize + " no-results"}
	if !slices.Equal(why, wantWhy) || len(n.Renewed) != 0 || refused.code != 1 {
		t.Errorf("the renewal after the further changes: exit %d, renewed %v, not renewed %q; want 1, none, and %q", refused.code, n.Renewed, why, wantWhy)
	}
	if got := staleExceptions(n.Problems); len(got) != 3 {
		t.Errorf("the renewal reports stale exceptions %q, want one for each entry it did not renew", got)
	}
	if got := renewEntries(t); !maps.Equal(got, held) {
		t.Errorf("entries it did not renew changed: %+v, want %+v", got, held)
	}
	if after := renewedTree(t); !maps.Equal(after, untouched) {
		t.Error("the renewal wrote a file other than itos-cc.yaml")
	}
	check := mutationCheck(t, "--json")
	var still []string
	for _, x := range staleExceptions(rawProblems(t, check)) {
		function, _, _ := strings.Cut(x, " ")
		still = append(still, function)
	}
	if want := []string{renewGrade, renewSign, renewSize}; !slices.Equal(still, want) || check.code != 1 {
		t.Errorf("the check after it: exit %d, stale exceptions of %q; want 1 with %q", check.code, still, want)
	}
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
		if o.code != 2 || !named {
			t.Errorf("mutation except %s: exit %d, problems %v; want 2, %s naming --renew", strings.Join(c.args, " "), o.code, rawProblems(t, o), c.rule)
		}
	}
}
