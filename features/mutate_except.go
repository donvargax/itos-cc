// Exception scenarios inspect public commands, configuration and snapshots.
package features

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"
	"slices"
	"strings"

	"github.com/cucumber/godog"
	"go.yaml.in/yaml/v3"
)

type exceptionMutation struct {
	s                  *sinceMutation
	r                  *resultMutation
	id                 string
	recorded, reviewed bool
	reason             string
	entry              map[string]any
	testBefore         string
	renewBefore        map[string]map[string]any
	renewFiles         map[string]string
	renewObject        map[string]any
}

func exceptionMutationSteps(sc *godog.ScenarioContext, m *mutateState, s *sinceMutation) {
	e := &exceptionMutation{s: s}
	sc.Before(func(ctx context.Context, scenario *godog.Scenario) (context.Context, error) {
		*e = exceptionMutation{s: s, reason: "the loop only counts up", r: &resultMutation{s: s}}
		for _, tag := range scenario.Tags {
			if strings.HasPrefix(tag.Name, "@ID-MUT-") {
				e.id = strings.TrimPrefix(tag.Name, "@ID-MUT-")
			}
		}
		if !slices.Contains([]string{"85", "86", "87", "88", "89", "90", "91", "92", "93", "94", "95", "96", "97", "98", "99", "138", "139", "140", "141", "142", "143", "144", "146", "227", "242"}, e.id) {
			return ctx, nil
		}
		e.r.annotate = slices.Contains([]string{"138", "139", "140"}, e.id)
		e.r.options()
		if e.id == "227" {
			return ctx, e.renewProject()
		}
		if err := e.project(); err != nil {
			return ctx, err
		}
		if e.id == "86" {
			m.w.problemSubject = e.refused
		}
		if e.id == "97" {
			if err := e.review(e.reason); err != nil {
				return ctx, err
			}
			if err := e.changeCount(); err != nil {
				return ctx, err
			}
			if err := s.commit("count changed before the selected range"); err != nil {
				return ctx, err
			}
			if err := m.w.git("tag", "-d", "base"); err != nil {
				return ctx, err
			}
			return ctx, m.w.git("tag", "base")
		}
		return ctx, nil
	})
	sc.Step("^a fresh run recorded the survivor `<` → `<=` at src/board\\.ts:7:19 in \"Board#count\"$", e.record)
	sc.Step(`^itos-cc\.yaml has one entry under mutation\.exceptions$`, e.oneEntry)
	sc.Step("^it records src/board\\.ts, \"Board#count\", the function's hash, the site's line within the function, its column, `<`, `<=`, and the reason$", e.entryFields)
	sc.Step(`^the snapshot records the mutant at src/board\.ts:5:9 as killed, and the one at src/board\.ts:9:3 as uncovered$`, e.refusedFixture)
	sc.Step(`^excepting src/board\.ts:9:3 is refused the same way$`, e.refusedUncovered)
	sc.Step(`^"--reason ''" is "flags\.value-invalid", exit 2$`, e.emptyReason)
	sc.Step(`^itos-cc\.yaml holds a comment, another key, and an entry for src/board\.ts:7:19$`, e.commented)
	sc.Step(`^the entry's reason is "reworded", and it is still the only one for that site$`, e.reworded)
	sc.Step(`^the comment and the other key are unchanged$`, e.restKept)
	sc.Step(`^itos-cc\.yaml excepts the survivor of "Board#count", and every other mutant of src/board\.ts is killed$`, e.reviewAndForget)
	sc.Step(`^a run recorded the excepted survivor of "Board#count"$`, e.recordExcepted)
	sc.Step(`^neither "Board#count" nor the tests that import src/board\.ts have changed since$`, e.unchanged)
	sc.Step(`^the excepted mutant does not run$`, func() error { return e.executed(false) })
	sc.Step(`^it is counted "excepted"$`, e.counted)
	sc.Step(`^"itos-cc mutation run --mutate-all src/board\.ts" runs it$`, e.mutateAll)
	sc.Step(`^src/board\.test\.ts changed since, and still does not kill it$`, func() error { return e.changeTests(false) })
	sc.Step(`^src/board\.test\.ts changed since, and now kills it$`, func() error { return e.changeTests(true) })
	sc.Step(`^the excepted mutant runs$`, func() error { return e.executed(true) })
	sc.Step(`^the problem is "mutation\.exception-stale", with file, function, line, column, original, replacement and why "killed"$`, e.killedEntry)
	sc.Step(`^itos-cc\.yaml excepts (?:a|the) survivor of "Board#count"(?: in src/board\.ts)?$`, func() error { return e.review(e.reason) })
	sc.Step(`^"Board#count" changed since the entry was written(?:, and the mutant at its site still survives)?$`, e.changeCount)
	sc.Step(`^the problem is "mutation\.exception-stale", with why "(changed|gone)"$`, func(why string) error { _, err := e.stale(map[string]any{"why": why}); return err })
	sc.Step(`^the mutant at its site, if it survives, is a "mutation\.survived" problem too$`, e.survivingProblem)
	sc.Step(`^itos-cc\.yaml excepts a survivor in "Board#count"$`, func() error { return e.review(e.reason) })
	sc.Step(`^"Board#count" was deleted, or no longer has that site$`, e.deleteCount)
	sc.Step(`^itos-cc\.yaml holds an entry for a site whose mutant is uncovered$`, e.uncoveredEntry)
	sc.Step(`^the problem is "mutation\.uncovered" for that site$`, e.uncoveredProblem)
	sc.Step(`^fresh results for src/board\.ts with one survivor, which itos-cc\.yaml excepts$`, func() error { return e.review(e.reason) })
	sc.Step(`^after "Board#count" changes, mutation check reports "mutation\.exception-stale" and exits 1$`, e.changedCheck)
	sc.Step(`^itos-cc\.yaml holds an entry for "Board#count" that would be stale$`, e.outsideRange)
	sc.Step(`^itos-cc\.yaml holds an entry with no reason$`, e.invalid)
	sc.Step(`^I run "itos-cc mutation run --json src/board\.ts" with one excepted survivor$`, func() error {
		if err := e.review(e.reason); err != nil {
			return err
		}
		return e.run("mutation run --json src/board.ts")
	})
	sc.Step(`^its file has "excepted": 1 beside "killed", "survived" and "uncovered"$`, e.counted)
	sc.Step(`^that mutant has outcome "survived" and "excepted", the entry's reason$`, e.jsonExcepted)
	sc.Step("^itos-cc\\.yaml excepts the survivor `<` → `<=` at src/board\\.ts:7:19 in \"Board#count\" with reason \"the loop only counts up\", and every other mutant of src/board\\.ts is killed$", func() error { return e.review(e.reason) })
	sc.Step(`^src/board\.ts ends with:$`, e.endsWith)
	sc.Step(`^the summary comment of src/board\.ts counts it as survived and lists it on a "survived:" line$`, e.survivingComment)
	sc.Step(`^the comment holds no "excepted" count and no "excepted:" line$`, e.noExceptedComment)
	sc.Step(`^itos-cc\.yaml excepts the survivor of "Board#count" with reason "the loop\\n  only counts up"$`, func() error { return e.review("the loop\n  only counts up") })
	sc.Step(`^the summary comment of src/board\.ts has the line "([^"]*)"$`, e.commentLine)
	sc.Step(`^every line of the comment begins with "//"$`, e.commentLines)
	sc.Step(`^src/board\.ts and its tests were deleted$`, e.deleteBoard)
	sc.Step(`^the problem is "mutation\.exception-stale", with file "src/board\.ts", function "Board#count", no line, and why "gone"$`, e.goneFile)
	sc.Step(`^fresh results for every source file, src/board\.ts having been deleted$`, e.freshAfterDeletion)
	sc.Step(`^the problem is "mutation\.exception-stale", with file "src/board\.ts" and why "gone"$`, func() error { _, err := e.stale(map[string]any{"file": "src/board.ts", "why": "gone"}); return err })
	sc.Step(`^a commit after "base" deleted src/board\.ts and its tests$`, func() error {
		if err := e.deleteBoard(); err != nil {
			return err
		}
		return s.commit("delete board and its tests")
	})
	sc.Step("^itos-cc\\.yaml excepts the survivor `<` → `<=` at src/board\\.ts:7:19 in \"Board#count\"$", func() error { return e.review(e.reason) })
	sc.Step(`^a commit after "base" renamed src/board\.ts to src/grid\.ts, changing nothing else$`, func() error { return e.rename(true) })
	sc.Step(`^src/board\.ts was renamed to src/grid\.ts, changing nothing else$`, func() error { return e.rename(false) })
	sc.Step(`^the problem is "mutation\.exception-stale", with file "src/board\.ts", why "moved", and new file "src/grid\.ts"$`, e.moved)
	sc.Step(`^the problem is "mutation\.exception-stale", with why "moved" and new file "src/grid\.ts"$`, e.moved)
	sc.Step(`^its fix says to change the entry's file to src/grid\.ts$`, e.moveFix)
	sc.Step(`^there is no "mutation\.survived" problem$`, e.noSurvivor)
	sc.Step(`^itos-cc\.yaml still names src/board\.ts$`, e.oldPathKept)
	sc.Step(`^itos-cc\.yaml excepts a survivor of "Board#count", and "Board#count" changed since the entry was written$`, func() error {
		if err := e.review(e.reason); err != nil {
			return err
		}
		return e.changeCount()
	})
	sc.Step(`^the "mutation\.exception-stale" problem, with why "changed", has a fix naming "itos-cc mutation except --renew" before "--reason"$`, e.renewFix)
	sc.Step(`^"itos-cc mutation check src/board\.ts --json" gives it the same fix$`, e.sameFix)
	e.renewSteps(sc)
}

func (e *exceptionMutation) run(args string) error { return e.s.m.w.runWith(e.s.m.w.env(), args) }
func (e *exceptionMutation) object() (map[string]any, error) {
	a, _, err := e.s.m.w.lastObject()
	return a, err
}
func (e *exceptionMutation) config() (map[string]any, error) {
	b, err := e.s.m.w.readIn(e.s.m.w.dir, "itos-cc.yaml")
	if err != nil {
		return nil, err
	}
	a := map[string]any{}
	err = yaml.Unmarshal(b, &a)
	return a, err
}
func (e *exceptionMutation) entries() ([]map[string]any, error) {
	a, err := e.config()
	if err != nil {
		return nil, err
	}
	m, ok := a["mutation"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("no mutation settings: %v", a)
	}
	raw, ok := m["exceptions"].([]any)
	if !ok {
		return nil, fmt.Errorf("no exception list: %v", m)
	}
	out := []map[string]any{}
	for _, v := range raw {
		entry, ok := v.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("invalid entry: %v", v)
		}
		out = append(out, entry)
	}
	return out, nil
}
func (e *exceptionMutation) writeEntries(entries []map[string]any) error {
	a := map[string]any{"mutation": map[string]any{"exceptions": entries}}
	b, err := yaml.Marshal(a)
	if err != nil {
		return err
	}
	return e.s.m.w.write("itos-cc.yaml", string(b))
}
func (e *exceptionMutation) oneEntry() error {
	a, err := e.entries()
	if err != nil {
		return err
	}
	if len(a) != 1 {
		return fmt.Errorf("entries %v, want exactly one", a)
	}
	return nil
}

// exceptionFields is whether a holds each field of want, its paths with
// their separators made slashes, as Windows names them with backslashes.
func exceptionFields(a, want map[string]any) error {
	for key, v := range want {
		got := a[key]
		if text, ok := got.(string); ok && (key == "file" || key == "new_file") {
			got = filepath.ToSlash(text)
		}
		if !reflect.DeepEqual(got, v) {
			return fmt.Errorf("%s is %v, want %v in %v", key, a[key], v, a)
		}
	}
	return nil
}
func (e *exceptionMutation) entryFields() error {
	if err := e.oneEntry(); err != nil {
		return err
	}
	a, _ := e.entries()
	snap, err := e.s.snapshot("src/board.ts")
	if err != nil {
		return err
	}
	u := e.s.units(snap, "Board#count")
	if len(u) != 1 || u[0]["hash"] == "" {
		return fmt.Errorf("no recorded count hash: %v", u)
	}
	return exceptionFields(a[0], map[string]any{"file": "src/board.ts", "function": u[0]["namespace"].(string) + "#count", "hash": u[0]["hash"], "line_in_function": 1, "column": 19, "original": "<", "replacement": "<=", "reason": e.reason, "line_text": "count(x){ return x<5; }"})
}
func (e *exceptionMutation) stale(want map[string]any) (map[string]any, error) {
	a, err := e.object()
	if err != nil {
		return nil, err
	}
	want["rule"] = "mutation.exception-stale"
	for _, p := range checkProblems(a) {
		if exceptionFields(p, want) == nil {
			return p, nil
		}
	}
	return nil, fmt.Errorf("no problem matching %v: %v", want, a)
}
func (e *exceptionMutation) refused() error { return e.refusal(5, 9) }
func (e *exceptionMutation) refusal(line, column int) error {
	a, err := e.object()
	if err != nil {
		return err
	}
	p := checkProblems(a)
	if len(p) != 1 {
		return fmt.Errorf("refusal problems %v", p)
	}
	if err := exceptionFields(p[0], map[string]any{"rule": "exception.no-survivor", "file": "src/board.ts", "line": float64(line), "column": float64(column)}); err != nil {
		return err
	}
	if e.s.m.w.d.Exists(filepath.Join(e.s.m.w.dir, "itos-cc.yaml")) {
		return fmt.Errorf("refusal wrote configuration")
	}
	return nil
}
func (e *exceptionMutation) refusedUncovered() error {
	if err := e.run("mutation except src/board.ts:9:3 --reason x --json"); err != nil {
		return err
	}
	if e.s.m.w.exit != 2 {
		return fmt.Errorf("refused uncovered exit %d", e.s.m.w.exit)
	}
	return e.refusal(9, 3)
}
func (e *exceptionMutation) emptyReason() error {
	if err := e.run("mutation except src/board.ts:7:19 --reason '' --json"); err != nil {
		return err
	}
	a, err := e.object()
	if err != nil {
		return err
	}
	p := checkProblems(a)
	if e.s.m.w.exit != 2 || len(p) != 1 {
		return fmt.Errorf("invalid reason: exit%d %v", e.s.m.w.exit, a)
	}
	if err := exceptionFields(p[0], map[string]any{"rule": "flags.value-invalid", "flag": "--reason", "value": ""}); err != nil {
		return err
	}
	if e.s.m.w.d.Exists(filepath.Join(e.s.m.w.dir, "itos-cc.yaml")) {
		return fmt.Errorf("invalid reason wrote config")
	}
	return nil
}
func (e *exceptionMutation) reworded() error {
	if err := e.oneEntry(); err != nil {
		return err
	}
	a, _ := e.entries()
	return exceptionFields(a[0], map[string]any{"file": "src/board.ts", "line_in_function": 1, "column": 19, "reason": "reworded"})
}
func (e *exceptionMutation) restKept() error {
	a, err := e.config()
	if err != nil {
		return err
	}
	if !strings.HasPrefix(e.s.m.read("itos-cc.yaml"), "# a reviewed exception\n") || !reflect.DeepEqual(a["other"], map[string]any{"kept": true}) {
		return fmt.Errorf("comment/other key changed: %v", a)
	}
	return nil
}
func (e *exceptionMutation) executed(want bool) error {
	n := 0
	for _, t := range trialsOf(e.s.m.w.stderr, "src/board.ts") {
		if t.line == 7 {
			n++
		}
	}
	expected := 0
	if want {
		expected = 1
	}
	if n != expected {
		return fmt.Errorf("count trial executions %d, want %d\n%s", n, expected, e.s.m.w.report())
	}
	return nil
}
func (e *exceptionMutation) counted() error {
	f, err := e.r.fileObject()
	if err != nil {
		return err
	}
	return exceptionFields(f, map[string]any{"killed": float64(14), "survived": float64(0), "excepted": float64(1), "uncovered": float64(0)})
}
func (e *exceptionMutation) mutateAll() error {
	if err := e.run("mutation run --mutate-all src/board.ts"); err != nil {
		return err
	}
	if err := e.executed(true); err != nil {
		return err
	}
	if e.s.m.w.exit != 0 {
		return fmt.Errorf("mutate-all exit %d", e.s.m.w.exit)
	}
	return e.counted()
}
func (e *exceptionMutation) killedEntry() error {
	_, err := e.stale(map[string]any{"file": "src/board.ts", "function": "board.Board#count", "line": float64(7), "column": float64(19), "original": "<", "replacement": "<=", "why": "killed"})
	return err
}
func (e *exceptionMutation) survivingProblem() error {
	a, err := e.object()
	if err != nil {
		return err
	}
	for _, p := range checkProblems(a) {
		if exceptionFields(p, map[string]any{"rule": "mutation.survived", "file": "src/board.ts", "function": "board.Board#count", "line": float64(7), "column": float64(19), "original": "<", "replacement": "<="}) == nil {
			return nil
		}
	}
	return fmt.Errorf("missing current survivor: %v", a)
}
func (e *exceptionMutation) uncoveredProblem() error {
	a, err := e.object()
	if err != nil {
		return err
	}
	for _, p := range checkProblems(a) {
		if exceptionFields(p, map[string]any{"rule": "mutation.uncovered", "file": "src/board.ts", "function": "board.Board#clear", "line": float64(9), "column": float64(3)}) == nil {
			return nil
		}
	}
	return fmt.Errorf("missing uncovered site: %v", a)
}
func (e *exceptionMutation) changedCheck() error {
	if err := e.changeCount(); err != nil {
		return err
	}
	if err := e.run("mutation check --json src/board.ts"); err != nil {
		return err
	}
	if e.s.m.w.exit != 1 {
		return fmt.Errorf("changed check exit %d", e.s.m.w.exit)
	}
	_, err := e.stale(map[string]any{"why": "changed"})
	return err
}
func (e *exceptionMutation) outsideRange() error { return e.counterfactualStale() }
func (e *exceptionMutation) jsonExcepted() error {
	a, err := e.r.entries()
	if err != nil {
		return err
	}
	var found []map[string]any
	for _, m := range a {
		if m["function"] == "board.Board#count" {
			found = append(found, m)
		}
	}
	if len(found) != 1 {
		return fmt.Errorf("count mutants %v", found)
	}
	return exceptionFields(found[0], map[string]any{"outcome": "survived", "excepted": e.reason, "reused": true, "line": float64(7), "column": float64(19), "original": "<", "replacement": "<="})
}
func (e *exceptionMutation) endsWith(doc *godog.DocString) error {
	if !strings.HasSuffix(e.s.m.read("src/board.ts"), doc.Content+"\n") {
		return fmt.Errorf("source lacks exact suffix\n%s", e.s.m.read("src/board.ts"))
	}
	return nil
}
func (e *exceptionMutation) comment() string {
	_, b, _ := strings.Cut(e.s.m.read("src/board.ts"), "// itos-cc mutation:")
	return "// itos-cc mutation:" + b
}
func (e *exceptionMutation) survivingComment() error {
	c := e.comment()
	if !strings.Contains(c, "14 killed, 1 survived, 0 uncovered") || !strings.Contains(c, "// survived: line 7 `<` → `<=` in count\n") {
		return fmt.Errorf("surviving comment %q", c)
	}
	return nil
}
func (e *exceptionMutation) noExceptedComment() error {
	if strings.Contains(e.comment(), "excepted") {
		return fmt.Errorf("comment still excepts survivor: %s", e.comment())
	}
	return nil
}
func (e *exceptionMutation) commentLine(want string) error {
	if !slices.Contains(strings.Split(e.comment(), "\n"), want) {
		return fmt.Errorf("no line %q in %s", want, e.comment())
	}
	return nil
}
func (e *exceptionMutation) commentLines() error {
	c := strings.TrimSuffix(e.comment(), "\n")
	if len(strings.Split(c, "\n")) != 3 {
		return fmt.Errorf("comment lines %q", c)
	}
	for _, line := range strings.Split(c, "\n") {
		if !strings.HasPrefix(line, "//") {
			return fmt.Errorf("uncommented line %q", line)
		}
	}
	return nil
}
func (e *exceptionMutation) goneFile() error {
	p, err := e.stale(map[string]any{"file": "src/board.ts", "function": "board.Board#count", "why": "gone"})
	if err != nil {
		return err
	}
	if _, ok := p["line"]; ok {
		return fmt.Errorf("gone file names a line: %v", p)
	}
	return nil
}
func (e *exceptionMutation) moved() error {
	_, err := e.stale(map[string]any{"file": "src/board.ts", "function": "board.Board#count", "why": "moved", "new_file": "src/grid.ts"})
	if err != nil {
		return err
	}
	f, err := e.r.fileObject()
	if err != nil {
		return err
	}
	if e.id == "144" {
		if err := exceptionFields(f, map[string]any{"baseline": "not-run", "judged": []any{}, "mutants": []any{}, "killed": float64(0), "survived": float64(0), "excepted": float64(0), "uncovered": float64(0), "ran": float64(0), "reused": float64(0)}); err != nil {
			return err
		}
		return e.continuingException()
	}
	if f["excepted"] != float64(1) {
		return fmt.Errorf("moved mutant not excepted: %v", f)
	}
	return nil
}

// An unchanged --since rename judges nothing. Observe the continuing entry
// through a real whole-project judgment, preserving the original When's output.
func (e *exceptionMutation) continuingException() error {
	w := e.s.m.w
	args := w.prepare([]string{"mutation", "run", "--json"})
	code, out, stderr, err := w.capture(w.dir, w.env(), args...)
	if err != nil {
		return err
	}
	var object map[string]any
	if err := json.Unmarshal([]byte(out), &object); err != nil {
		return fmt.Errorf("continuing exception output: %w\n%s\n%s", err, out, stderr)
	}
	if code != 1 {
		return fmt.Errorf("moved-entry judgment exit%d\n%s\n%s", code, out, stderr)
	}
	for _, p := range checkProblems(object) {
		if p["rule"] == "mutation.survived" {
			return fmt.Errorf("continuing entry left a survivor: %v", p)
		}
	}
	files, _ := object["files"].([]any)
	if len(files) != 1 {
		return fmt.Errorf("continuing judgment files: %v", object)
	}
	f, ok := files[0].(map[string]any)
	if !ok {
		return fmt.Errorf("continuing judgment file: %v", files[0])
	}
	if err := exceptionFields(f, map[string]any{"file": "src/grid.ts", "killed": float64(14), "survived": float64(0), "excepted": float64(1), "uncovered": float64(0)}); err != nil {
		return err
	}
	var count []map[string]any
	mutants, _ := f["mutants"].([]any)
	for _, raw := range mutants {
		m, ok := raw.(map[string]any)
		if ok && m["function"] == "grid.Board#count" {
			count = append(count, m)
		}
	}
	if len(count) != 1 {
		return fmt.Errorf("continuing count judgment: %v", count)
	}
	return exceptionFields(count[0], map[string]any{"line": float64(7), "column": float64(19), "original": "<", "replacement": "<=", "outcome": "survived", "excepted": e.reason})
}
func (e *exceptionMutation) moveFix() error {
	p, err := e.stale(map[string]any{"why": "moved"})
	if err != nil {
		return err
	}
	fix, _ := p["fix"].(string)
	if !strings.Contains(fix, "src/grid.ts") || !strings.Contains(fix, "file") {
		return fmt.Errorf("move fix %q", fix)
	}
	return nil
}
func (e *exceptionMutation) noSurvivor() error {
	a, err := e.object()
	if err != nil {
		return err
	}
	for _, p := range checkProblems(a) {
		if p["rule"] == "mutation.survived" {
			return fmt.Errorf("survivor %v", p)
		}
	}
	return nil
}
func (e *exceptionMutation) oldPathKept() error {
	a, err := e.entries()
	if err != nil {
		return err
	}
	if len(a) != 1 || a[0]["file"] != "src/board.ts" || !reflect.DeepEqual(a[0], e.entry) {
		return fmt.Errorf("entry changed: %v, before %v", a, e.entry)
	}
	return nil
}
func (e *exceptionMutation) renewFix() error {
	p, err := e.stale(map[string]any{"why": "changed"})
	if err != nil {
		return err
	}
	fix, _ := p["fix"].(string)
	at := strings.Index(fix, "itos-cc mutation except --renew")
	if at < 0 || strings.Index(fix, "--reason") <= at {
		return fmt.Errorf("renewal not before reason: %q", fix)
	}
	e.renewObject = p
	return nil
}
func (e *exceptionMutation) sameFix() error {
	old := e.renewObject["fix"]
	if err := e.run("mutation check src/board.ts --json"); err != nil {
		return err
	}
	p, err := e.stale(map[string]any{"why": "changed"})
	if err != nil {
		return err
	}
	if p["fix"] != old || e.s.m.w.exit != 1 {
		return fmt.Errorf("check fix %v, want %v, exit%d", p, old, e.s.m.w.exit)
	}
	return nil
}
