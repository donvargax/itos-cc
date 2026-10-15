package features

import (
	"context"
	"fmt"
	"reflect"
	"slices"

	"github.com/cucumber/godog"
)

const renewModule = "example.com/renew"

func (e *exceptionMutation) renewSteps(sc *godog.ScenarioContext) {
	sc.Step(`^a Go project with a reviewed exception for an equivalent mutant in a function$`, e.reviewRenew)
	sc.Step(`^a commit that renames a string literal on another line of that function$`, e.renameLiteral)
	sc.Step(`^"itos-cc mutation run" has judged the function again and the mutant still survives$`, e.rejudgeRenew)
	sc.Step(`^the exception names the function's current hash, line and column and keeps its reason$`, e.renewedEntry)
	sc.Step(`^the renewal is reported with the old and new hash and the site$`, e.renewedReport)
	sc.Step(`^"itos-cc mutation check" passes$`, e.renewedCheck)
	sc.Step(`^an exception whose mutant the fresh run killed, whose line changed, or whose function has no fresh results is not renewed, is reported with why, and stays "mutation\.exception-stale"$`, e.unrenewed)
}

func (e *exceptionMutation) renewProject() error {
	e.s.made = true
	e.s.file = "label.go"
	e.r.file = e.s.file
	e.s.m.file = e.s.file
	files := map[string]string{"go.mod": "module " + renewModule + "\n\ngo 1.22\n"}
	files["label.go"] = "package renew\n\n// Label names n.\nfunc Label(n int) string {\n\tname := \"old\"\n\tif n > 0 {\n\t\treturn name + \"+\"\n\t}\n\treturn name\n}\n"
	files["other.go"] = "package renew\n\n// Sign names n.\nfunc Sign(n int) string {\n\tword := \"old\"\n\tif n < 5 {\n\t\treturn word + \"-\"\n\t}\n\treturn word\n}\n\n// Grade names n.\nfunc Grade(n int) string {\n\tword := \"old\"\n\tif n > 9 {\n\t\treturn word + \"!\"\n\t}\n\treturn word\n}\n"
	files["size.go"] = "package renew\n\n// Size names n.\nfunc Size(n int) string {\n\tword := \"old\"\n\tif n > 99 {\n\t\treturn word + \"#\"\n\t}\n\treturn word\n}\n"
	files["label_test.go"] = "package renew\nimport \"testing\"\n" +
		"func TestLabel(t *testing.T) { if Label(1)!=\"old+\" || Label(-1)!=\"old\" {t.Fatal(\"Label\")} }\n" +
		"func TestSign(t *testing.T) { if Sign(4)!=\"old-\" || Sign(6)!=\"old\" {t.Fatal(\"Sign\")} }\n" +
		"func TestGrade(t *testing.T) { if Grade(20)!=\"old!\" || Grade(1)!=\"old\" {t.Fatal(\"Grade\")} }\n" +
		"func TestSize(t *testing.T) { if Size(200)!=\"old#\" || Size(1)!=\"old\" {t.Fatal(\"Size\")} }\n"
	if err := e.s.m.writeAll(files); err != nil {
		return err
	}
	if err := e.declareRenew(false); err != nil {
		return err
	}
	for _, args := range [][]string{{"init", "-q", "-b", "main"}, {"add", "."}, {"commit", "-qm", "review fixture"}} {
		if err := e.s.m.w.git(args...); err != nil {
			return err
		}
	}
	return nil
}
func (e *exceptionMutation) declareRenew(killSign bool) error {
	report := ""
	t := Tests{Language: "go"}
	for _, file := range []string{"label.go", "other.go", "size.go"} {
		sites, err := e.s.m.sitesOf(file)
		if err != nil {
			return err
		}
		report += "SF:" + file + "\n"
		for _, site := range sites {
			report += fmt.Sprintf("DA:%d,1\n", site.line)
			survives := site.original == ">" || site.original == "<" || file == "other.go" && site.original == "0"
			if killSign && site.function == renewModule+"#Sign" {
				survives = false
			}
			if survives {
				t.Surviving = append(t.Surviving, Site{File: file, Line: site.line, Original: site.original, Replacement: site.replacement})
			}
		}
		report += "end_of_record\n"
	}
	if err := e.s.m.w.d.Coverage(e.s.m.w.dir, Coverage{Language: "go", Report: report}); err != nil {
		return err
	}
	for _, command := range []string{"go test -count=1 -failfast .", "go test -count=1 -failfast ./..."} {
		t.Command = command
		if err := e.s.m.w.d.Tests(e.s.m.w.dir, t); err != nil {
			return err
		}
	}
	return nil
}
func (e *exceptionMutation) reviewRenew() error {
	if err := e.run("mutation run --json"); err != nil {
		return err
	}
	if e.s.m.w.exit != 1 {
		return fmt.Errorf("renewal first run exit%d", e.s.m.w.exit)
	}
	for _, site := range []string{"label.go:6:7", "other.go:6:7", "other.go:15:7", "size.go:6:7"} {
		if err := e.run("mutation except " + site + " --reason 'reviewed: " + site + "'"); err != nil {
			return err
		}
		if e.s.m.w.exit != 0 {
			return fmt.Errorf("review %s: %s", site, e.s.m.w.report())
		}
	}
	var err error
	e.renewBefore, err = e.entriesByFunction()
	return err
}
func (e *exceptionMutation) entriesByFunction() (map[string]map[string]any, error) {
	a, err := e.entries()
	if err != nil {
		return nil, err
	}
	out := map[string]map[string]any{}
	for _, v := range a {
		fn, ok := v["function"].(string)
		if !ok || out[fn] != nil {
			return nil, fmt.Errorf("invalid/duplicate exception %v", v)
		}
		out[fn] = v
	}
	return out, nil
}
func (e *exceptionMutation) renameLiteral() error {
	if err := e.s.replace("label.go", `name := "old"`, `name := "new"`); err != nil {
		return err
	}
	if err := e.s.replace("label_test.go", `Label(1)!="old+" || Label(-1)!="old"`, `Label(1)!="new+" || Label(-1)!="new"`); err != nil {
		return err
	}
	return e.s.commit("rename Label string literal")
}
func (e *exceptionMutation) rejudgeRenew(ctx context.Context) error {
	if err := e.run("mutation run --json"); err != nil {
		return err
	}
	if _, err := e.stale(map[string]any{"function": renewModule + "#Label", "why": "changed"}); err != nil {
		return err
	}
	snap, err := e.s.snapshot("label.go")
	if err != nil {
		return err
	}
	u := e.s.units(snap, renewModule+"#Label")
	if len(u) != 1 || u[0]["survived"] != float64(1) {
		return fmt.Errorf("Label not rejudged surviving: %v", u)
	}
	e.renewFiles, err = e.treeWithoutConfig(ctx)
	return err
}
func (e *exceptionMutation) treeWithoutConfig(ctx context.Context) (map[string]string, error) {
	w := e.s.m.w
	files, err := w.d.Files(ctx, w.dir)
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, file := range files {
		if file == "itos-cc.yaml" {
			continue
		}
		b, err := w.readIn(w.dir, file)
		if err != nil {
			return nil, err
		}
		out[file] = string(b)
	}
	return out, nil
}
func (e *exceptionMutation) renewedEntry(ctx context.Context) error {
	a, err := e.entriesByFunction()
	if err != nil {
		return err
	}
	if len(a) != 4 {
		return fmt.Errorf("renewal entries %v", a)
	}
	snap, err := e.s.snapshot("label.go")
	if err != nil {
		return err
	}
	u := e.s.units(snap, renewModule+"#Label")
	if len(u) != 1 {
		return fmt.Errorf("Label unit %v", u)
	}
	want := map[string]any{}
	for key, v := range e.renewBefore[renewModule+"#Label"] {
		want[key] = v
	}
	want["hash"] = u[0]["hash"]
	if want["hash"] == e.renewBefore[renewModule+"#Label"]["hash"] {
		return fmt.Errorf("literal rename did not change hash")
	}
	if !reflect.DeepEqual(a[renewModule+"#Label"], want) {
		return fmt.Errorf("renewed entry %v, want %v", a[renewModule+"#Label"], want)
	}
	for _, fn := range []string{"Sign", "Grade", "Size"} {
		if !reflect.DeepEqual(a[renewModule+"#"+fn], e.renewBefore[renewModule+"#"+fn]) {
			return fmt.Errorf("unrelated entry %s changed", fn)
		}
	}
	after, err := e.treeWithoutConfig(ctx)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(after, e.renewFiles) {
		return fmt.Errorf("renewal changed other fixture files")
	}
	return nil
}
func (e *exceptionMutation) renewedReport() error {
	a, err := e.object()
	if err != nil {
		return err
	}
	e.renewObject = a
	raw, ok := a["renewed"].([]any)
	if !ok || len(raw) != 1 || e.s.m.w.exit != 0 {
		return fmt.Errorf("renewal report %v, exit%d", a, e.s.m.w.exit)
	}
	entries, err := e.entriesByFunction()
	if err != nil {
		return err
	}
	p := raw[0].(map[string]any)
	if err := exceptionFields(p, map[string]any{"function": renewModule + "#Label", "file": "label.go", "line": float64(6), "column": float64(7), "original": ">", "replacement": ">=", "old_hash": e.renewBefore[renewModule+"#Label"]["hash"], "new_hash": entries[renewModule+"#Label"]["hash"]}); err != nil {
		return err
	}
	if n, _ := a["not_renewed"].([]any); len(n) != 0 {
		return fmt.Errorf("unexpected unrenewed %v", n)
	}
	return nil
}
func (e *exceptionMutation) renewedCheck() error {
	if err := e.run("mutation check --json"); err != nil {
		return err
	}
	a, err := e.object()
	if err != nil {
		return err
	}
	if e.s.m.w.exit != 0 || len(checkProblems(a)) != 0 {
		return fmt.Errorf("renewed check: %s", e.s.m.w.report())
	}
	return nil
}
func (e *exceptionMutation) unrenewed(ctx context.Context) error {
	changes := [][3]string{{"other.go", "word := \"old\"\n\tif n < 5", "word := \"new\"\n\tif n < 5"},
		{"label_test.go", `Sign(4)!="old-" || Sign(6)!="old"`, `Sign(4)!="new-" || Sign(6)!="new" || Sign(5)!="new"`},
		{"other.go", "word := \"old\"\n\tif n > 9 {", "word := \"new\"\n\tif n > 9 { // changed site line"},
		{"label_test.go", `Grade(20)!="old!" || Grade(1)!="old"`, `Grade(20)!="new!" || Grade(1)!="new"`},
		{"size.go", `word := "old"`, `word := "new"`},
		{"label_test.go", `Size(200)!="old#" || Size(1)!="old"`, `Size(200)!="new#" || Size(1)!="new"`}}
	for _, c := range changes {
		if err := e.s.replace(c[0], c[1], c[2]); err != nil {
			return err
		}
	}
	if err := e.declareRenew(true); err != nil {
		return err
	}
	if err := e.run("mutation run --json label.go other.go"); err != nil {
		return err
	}
	if e.s.m.w.exit != 1 {
		return fmt.Errorf("refusal control run exit%d", e.s.m.w.exit)
	}
	held, err := e.entriesByFunction()
	if err != nil {
		return err
	}
	tree, err := e.treeWithoutConfig(ctx)
	if err != nil {
		return err
	}
	if err := e.run("mutation except --renew --json"); err != nil {
		return err
	}
	a, err := e.object()
	if err != nil {
		return err
	}
	var why []string
	raw, _ := a["not_renewed"].([]any)
	for _, v := range raw {
		p := v.(map[string]any)
		why = append(why, fmt.Sprint(p["function"])+" "+fmt.Sprint(p["why"]))
	}
	slices.Sort(why)
	want := []string{renewModule + "#Grade changed", renewModule + "#Sign killed", renewModule + "#Size no-results"}
	if !slices.Equal(why, want) || e.s.m.w.exit != 1 {
		return fmt.Errorf("unrenewed %v, want %v, exit%d", why, want, e.s.m.w.exit)
	}
	if renewed, _ := a["renewed"].([]any); len(renewed) != 0 {
		return fmt.Errorf("unexpected renewal %v", renewed)
	}
	problems := checkProblems(a)
	if len(problems) != 3 {
		return fmt.Errorf("unrenewed problems %v", problems)
	}
	for _, p := range problems {
		if p["rule"] != "mutation.exception-stale" {
			return fmt.Errorf("unexpected refusal problem %v", p)
		}
	}
	after, err := e.entriesByFunction()
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(after, held) {
		return fmt.Errorf("unrenewed entries changed: %v", after)
	}
	newTree, err := e.treeWithoutConfig(ctx)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(tree, newTree) {
		return fmt.Errorf("refused renewal changed other files")
	}
	if err := e.run("mutation check --json"); err != nil {
		return err
	}
	check, err := e.object()
	if err != nil {
		return err
	}
	var stale []string
	for _, p := range checkProblems(check) {
		if p["rule"] == "mutation.exception-stale" {
			stale = append(stale, fmt.Sprint(p["function"]))
		}
	}
	slices.Sort(stale)
	if !slices.Equal(stale, []string{renewModule + "#Grade", renewModule + "#Sign", renewModule + "#Size"}) || e.s.m.w.exit != 1 {
		return fmt.Errorf("remaining stale entries %v, exit%d", stale, e.s.m.w.exit)
	}
	return nil
}
