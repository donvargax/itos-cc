// Result scenarios observe only public command output and fixture artifacts.
package features

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/cucumber/godog"
)

type resultMutation struct {
	s                                             *sinceMutation
	p                                             *sampleMutation
	id, file                                      string
	annotate                                      bool
	held                                          map[string]string
	observe                                       func() ([]string, error)
	temp, killed, live, outside, externalSnapshot string
	release                                       func() error
	beforeBlock                                   string
}

func resultMutationSteps(sc *godog.ScenarioContext, m *mutateState, s *sinceMutation, p *sampleMutation) {
	r := &resultMutation{s: s, p: p}
	sc.Before(func(ctx context.Context, scenario *godog.Scenario) (context.Context, error) {
		*r = resultMutation{s: s, p: p}
		for _, tag := range scenario.Tags {
			if strings.HasPrefix(tag.Name, "@ID-MUT-") {
				r.id = strings.TrimPrefix(tag.Name, "@ID-MUT-")
			}
		}
		if !slices.Contains([]string{"28", "29", "30", "31", "32", "33", "34", "35", "36", "52", "53", "54", "55", "56", "145", "231", "232", "233", "234", "235", "236", "237", "238", "239", "240", "241"}, r.id) {
			return ctx, nil
		}
		r.options()
		switch r.id {
		case "28":
			return ctx, r.summaryFixture()
		case "30":
			return ctx, r.mixed()
		case "31":
			return ctx, r.ts("src/billing/invoice.ts", sinceBoardTS, strings.ReplaceAll(sinceBoardTestTS, `"./board"`, `"./invoice"`), []string{"Board#clear"}, nil)
		case "32", "33", "34", "145":
			r.annotate = true
			return ctx, r.python("x.py", "def a(x):\n    return x > 0\n", "test_x.py", "from x import a\n\ndef test_a():\n    assert a(1)\n", false)
		case "35":
			if err := r.strong(); err != nil {
				return ctx, err
			}
			r.held = map[string]string{s.file: m.read(s.file)}
			var err error
			r.observe, err = m.w.d.ObserveWrites(filepath.Join(m.w.dir, s.file))
			return ctx, err
		case "54", "55":
			if err := s.project(true); err != nil {
				return ctx, err
			}
			r.file = s.file
			if r.id == "55" {
				return ctx, s.previous()
			}
		case "56":
			if err := r.strong(); err != nil {
				return ctx, err
			}
			m.failingSample = r.breakBaseline
		}
		return ctx, nil
	})
	sc.After(func(ctx context.Context, _ *godog.Scenario, _ error) (context.Context, error) {
		var errs []error
		if r.release != nil {
			errs = append(errs, r.release())
			r.release = nil
		}
		for _, dir := range []string{r.temp, r.outside} {
			if dir != "" {
				errs = append(errs, m.w.d.Remove(dir))
			}
		}
		return ctx, errors.Join(errs...)
	})
	sc.Step(`^each survivor is listed as "([^"]*)"$`, r.survivorLine)
	sc.Step(`^(every covered mutant is killed|a baseline fails|a mutant survives)$`, r.situation)
	sc.Step(`^each survivor is a "mutation\.survived" problem with file, line, column, function, original, and replacement$`, r.jsonSurvivors)
	sc.Step(`^\.metrics/mutate/src/billing/invoice\.ts\.json records each function's hash and the outcome of every mutant$`, r.snapshotHolds)
	sc.Step("^its mutants run and \"> → >=\" survives while one other mutant is killed$", r.annotationRun)
	sc.Step(`^the file ends with:$`, r.endsBlock)
	sc.Step(`^a file that already ends with an itos-cc mutation comment$`, r.oldAnnotation)
	sc.Step(`^its mutants run again$`, r.annotationRun)
	sc.Step(`^the file holds exactly one, updated summary comment$`, r.updatedBlock)
	sc.Step(`^the file is only written when the summary changed$`, r.commentWrites)
	sc.Step(`^an itos-cc mutation comment block followed by more code$`, r.followedBlock)
	sc.Step(`^the summary is rewritten$`, r.annotationRun)
	sc.Step(`^that block is left in place$`, r.blockLeft)
	sc.Step(`^no source file is modified$`, r.sourcesHeld)
	sc.Step(`^the file "x\.py" ends with a summary comment opening "# itos-cc mutate:" and closing "# end itos-cc mutate"$`, r.oldMarkers)
	sc.Step(`^the file holds exactly one summary comment$`, r.oneBlock)
	sc.Step(`^it opens "# itos-cc mutation:" and closes "# end itos-cc mutation"$`, r.newMarkers)
	sc.Step(`^src/board\.ts has mutants that are killed, one that survives, and one on a line no test executes$`, r.mixed)
	sc.Step(`^a mutant of src/board\.ts runs past its timeout$`, func() error {
		p.id = "74"
		if err := p.project(); err != nil {
			return err
		}
		r.file = s.file
		previous := m.w.prepare
		m.w.prepare = func(args []string) []string {
			args = previous(args)
			if slices.Contains(args, "run") {
				args = append(args, "--timeout-factor", "0.2")
			}
			return args
		}
		return nil
	})
	sc.Step(`^"Board#place" has not changed since, while "Board#clear" has$`, func() error {
		if err := s.unchangedPlace(); err != nil {
			return err
		}
		return s.replace(s.file, "return i < 3", "// changed clear\n    return i < 3")
	})
	sc.Step(`^its file in "files" has "mutants", one for each site in site order$`, r.siteInventory)
	sc.Step(`^each has line, column, function, original, replacement, outcome, reused, and scope$`, r.mutantFields)
	sc.Step(`^their outcomes are "killed", "survived", and "uncovered" as each was decided$`, r.mixedOutcomes)
	sc.Step(`^its outcome is "timeout"$`, r.timeoutOutcome)
	sc.Step(`^it is counted in "killed"$`, r.timeoutCount)
	sc.Step(`^the mutants of "Board#place" are "killed" with reused true$`, func() error { return r.reuse("Board#place", true) })
	sc.Step(`^the mutants of "Board#clear" have reused false$`, func() error { return r.reuse("Board#clear", false) })
	sc.Step(`^the "mutants" of src/board\.ts all have function "Board#place"$`, r.onlyPlace)
	sc.Step(`^its file has "baseline": "failed" and "mutants": \[\]$`, r.baselineFailed)
	r.lateSteps(sc)
}
func (r *resultMutation) options() {
	w := r.s.m.w
	previous := w.prepare
	w.prepare = func(args []string) []string {
		if previous != nil {
			args = previous(args)
		}
		if slices.Contains(args, "mutation") && slices.Contains(args, "run") {
			has := func(flag string) bool {
				return slices.ContainsFunc(args, func(arg string) bool { return arg == flag || strings.HasPrefix(arg, flag+"=") })
			}
			if !has("--workers") {
				args = append(args, "--workers", "2")
			}
			if !r.annotate && !has("--no-annotate") {
				args = append(args, "--no-annotate")
			}
		}
		return args
	}
}
func (r *resultMutation) run(args string) error { return r.s.m.w.runWith(r.s.m.w.env(), args) }
func (r *resultMutation) object() (map[string]any, error) {
	obj, _, err := r.s.m.w.lastObject()
	return obj, err
}
func (r *resultMutation) fileObject() (map[string]any, error) {
	obj, err := r.object()
	if err != nil {
		return nil, err
	}
	for _, raw := range obj["files"].([]any) {
		f := raw.(map[string]any)
		if filepath.ToSlash(fmt.Sprint(f["file"])) == r.file {
			return f, nil
		}
	}
	return nil, fmt.Errorf("no file %s in %v", r.file, obj)
}
func (r *resultMutation) entries() ([]map[string]any, error) {
	f, err := r.fileObject()
	if err != nil {
		return nil, err
	}
	raw, ok := f["mutants"].([]any)
	if !ok {
		return nil, fmt.Errorf("mutants not a list: %v", f)
	}
	out := []map[string]any{}
	for _, v := range raw {
		out = append(out, v.(map[string]any))
	}
	return out, nil
}
func (r *resultMutation) survivorLine(want string) error {
	var got []string
	for _, line := range strings.Split(r.s.m.w.stdout, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "survived ") {
			got = append(got, strings.TrimSpace(line))
		}
	}
	if !slices.Equal(got, []string{want}) {
		return fmt.Errorf("survivor lines %q, want %q", got, want)
	}
	return nil
}
func (r *resultMutation) situation(what string) error {
	if what == "a mutant survives" {
		if err := r.s.project(true); err != nil {
			return err
		}
		r.file = r.s.file
		return nil
	}
	if err := r.strong(); err != nil {
		return err
	}
	if what == "a baseline fails" {
		return r.breakBaseline()
	}
	return nil
}
func (r *resultMutation) breakBaseline() error {
	if err := r.s.m.w.write("src/board.test.ts", `import {test} from "vitest"; import {Board} from "./board"; test("broken",()=>{new Board().place(6);throw new Error("fixture broken")});`); err != nil {
		return err
	}
	return r.s.m.w.d.Tests(r.s.m.w.dir, Tests{Language: "typescript", Command: vitestCommand + r.file, Baseline: "fixture broken"})
}
func (r *resultMutation) jsonSurvivors() error {
	obj, err := r.object()
	if err != nil {
		return err
	}
	problems := checkProblems(obj)
	sites, err := r.s.m.sitesOf(r.file)
	if err != nil {
		return err
	}
	n := 0
	for _, problem := range problems {
		if problem["rule"] != "mutation.survived" {
			return fmt.Errorf("unexpected problem: %v", problem)
		}
		n++
		if problem["file"] != r.file || !slices.ContainsFunc(sites, func(site listedSite) bool {
			return problem["line"] == float64(site.line) && problem["column"] == float64(site.column) && problem["function"] == site.function && problem["original"] == site.original && problem["replacement"] == site.replacement
		}) {
			return fmt.Errorf("survivor subject differs from public sites: %v", problem)
		}
	}
	if n != 1 {
		return fmt.Errorf("survivors %d, want one", n)
	}
	return r.mixedOutcomes()
}
func (r *resultMutation) siteInventory() error {
	entries, err := r.entries()
	if err != nil {
		return err
	}
	sites, err := r.s.m.sitesOf(r.file)
	if err != nil {
		return err
	}
	if len(entries) != len(sites) || len(sites) != 3 {
		return fmt.Errorf("mutants %v, sites %v", entries, sites)
	}
	for i, site := range sites {
		mu := entries[i]
		if mu["line"] != float64(site.line) || mu["column"] != float64(site.column) || mu["function"] != site.function || mu["original"] != site.original || mu["replacement"] != site.replacement {
			return fmt.Errorf("site order/subject %v, want %v", mu, site)
		}
	}
	return nil
}
func (r *resultMutation) mutantFields() error {
	entries, err := r.entries()
	if err != nil {
		return err
	}
	if len(entries) != 3 {
		return fmt.Errorf("mutants %v", entries)
	}
	for _, mu := range entries {
		for _, key := range []string{"line", "column", "function", "original", "replacement", "outcome", "reused", "scope"} {
			if _, ok := mu[key]; !ok {
				return fmt.Errorf("mutant lacks %s: %v", key, mu)
			}
		}
		if mu["reused"] != false || mu["scope"] != "own" {
			return fmt.Errorf("new own mutant %v", mu)
		}
	}
	return nil
}
func (r *resultMutation) mixedOutcomes() error {
	entries, err := r.entries()
	if err != nil {
		return err
	}
	want := map[string]string{"Board#place": "killed", "Board#full": "survived", "Board#clear": "uncovered"}
	matched := 0
	for _, mu := range entries {
		for name, outcome := range want {
			if sinceFunction(listedSite{function: fmt.Sprint(mu["function"])}, name) {
				if mu["outcome"] != outcome {
					return fmt.Errorf("outcome %v, want %s", mu, outcome)
				}
				matched++
			}
		}
	}
	f, err := r.fileObject()
	if err != nil {
		return err
	}
	if matched != 3 || f["killed"] != float64(1) || f["survived"] != float64(1) || f["uncovered"] != float64(1) || f["ran"] != float64(2) || f["reused"] != float64(0) || f["baseline"] != "passed" {
		return fmt.Errorf("mixed result %v", f)
	}
	return nil
}
func (r *resultMutation) timeoutOutcome() error {
	entries, err := r.entries()
	if err != nil {
		return err
	}
	n := 0
	for _, mu := range entries {
		if mu["original"] == "!" {
			n++
			if mu["outcome"] != "timeout" {
				return fmt.Errorf("timeout mutant %v", mu)
			}
		}
	}
	if n != 1 {
		return fmt.Errorf("timeout subjects %d", n)
	}
	return nil
}
func (r *resultMutation) timeoutCount() error {
	f, err := r.fileObject()
	if err != nil {
		return err
	}
	if f["killed"] != float64(2) || f["survived"] != float64(0) {
		return fmt.Errorf("timeout counts %v", f)
	}
	return nil
}
func (r *resultMutation) reuse(name string, want bool) error {
	entries, err := r.entries()
	if err != nil {
		return err
	}
	n := 0
	for _, mu := range entries {
		if sinceFunction(listedSite{function: fmt.Sprint(mu["function"])}, name) {
			n++
			if mu["reused"] != want || want && mu["outcome"] != "killed" {
				return fmt.Errorf("reuse %v, want %v", mu, want)
			}
		}
	}
	if n != 1 {
		return fmt.Errorf("reuse subjects %d", n)
	}
	return nil
}
func (r *resultMutation) onlyPlace() error {
	entries, err := r.entries()
	if err != nil {
		return err
	}
	if len(entries) != 1 || !sinceFunction(listedSite{function: fmt.Sprint(entries[0]["function"])}, "Board#place") {
		return fmt.Errorf("judged mutants %v", entries)
	}
	return nil
}
func (r *resultMutation) baselineFailed() error {
	f, err := r.fileObject()
	if err != nil {
		return err
	}
	entries, err := r.entries()
	if err != nil {
		return err
	}
	if f["baseline"] != "failed" || len(entries) != 0 || r.s.m.w.exit != 1 {
		return fmt.Errorf("failed baseline result %v exit%d", f, r.s.m.w.exit)
	}
	return nil
}
func (r *resultMutation) snapshotHolds() error {
	snap, err := r.s.snapshot(r.file)
	if err != nil {
		return err
	}
	entries, err := r.entries()
	if err != nil {
		return err
	}
	recorded := snapshotEntries(snap)
	if len(recorded) != len(entries) || len(recorded) != 2 {
		return fmt.Errorf("snapshot mutants %v, output %v", recorded, entries)
	}
	for _, raw := range snap["units"].([]any) {
		u := raw.(map[string]any)
		if hash, ok := u["hash"].(string); !ok || hash == "" {
			return fmt.Errorf("unit lacks hash %v", u)
		}
	}
	for _, mu := range entries {
		if !slices.ContainsFunc(recorded, func(record map[string]any) bool {
			return record["line"] == mu["line"] && record["column"] == mu["column"] && record["original"] == mu["original"] && record["replacement"] == mu["replacement"] && record["outcome"] == mu["outcome"]
		}) {
			return fmt.Errorf("not recorded: %v", mu)
		}
	}
	code, out, stderr, err := r.s.m.w.capture(r.s.m.w.dir, r.s.m.w.env(), "mutation", "check", "--json", r.file)
	if err != nil {
		return err
	}
	var obj map[string]any
	if err := json.Unmarshal([]byte(out), &obj); err != nil {
		return err
	}
	for _, problem := range checkProblems(obj) {
		if problem["rule"] != "mutation.survived" {
			return fmt.Errorf("cached hash is not fresh: %v", problem)
		}
	}
	if code != 1 || len(checkProblems(obj)) != 1 {
		return fmt.Errorf("fresh cache check exit%d: %s%s", code, out, stderr)
	}
	return nil
}

const originalAnnotationSource = "def a(x):\n    return x > 0\n"
const oldAnnotation = "\n# itos-cc mutation: 2 killed, 0 survived, 0 uncovered\n# end itos-cc mutation\n"

func (r *resultMutation) oldAnnotation() error {
	w := r.s.m.w
	if err := w.write(r.file, originalAnnotationSource+oldAnnotation); err != nil {
		return err
	}
	var err error
	r.observe, err = w.d.ObserveWrites(filepath.Join(w.dir, r.file))
	return err
}
func (r *resultMutation) annotationRun() error { return r.run("mutation run --json " + r.file) }
func (r *resultMutation) endsBlock(text *godog.DocString) error {
	got := r.s.m.read(r.file)
	if !strings.HasSuffix(got, text.Content+"\n") {
		return fmt.Errorf("annotation:\n%s\nwant suffix:\n%s", got, text.Content)
	}
	return nil
}
func (r *resultMutation) oneBlock() error {
	got := r.s.m.read(r.file)
	if strings.Count(got, "# itos-cc mutat") != 1 || strings.Count(got, "# end itos-cc mutat") != 1 {
		return fmt.Errorf("summary blocks: %s", got)
	}
	return nil
}
func (r *resultMutation) updatedBlock() error {
	if err := r.oneBlock(); err != nil {
		return err
	}
	if !strings.Contains(r.s.m.read(r.file), "# itos-cc mutation: 1 killed, 1 survived, 0 uncovered") {
		return fmt.Errorf("summary not updated: %s", r.s.m.read(r.file))
	}
	return nil
}
func (r *resultMutation) commentWrites() error {
	written, err := r.observe()
	if err != nil {
		return err
	}
	file := filepath.Join(r.s.m.w.dir, r.file)
	if !slices.Equal(written, []string{file}) {
		return fmt.Errorf("changed summary writes %v", written)
	}
	held := r.s.m.read(r.file)
	r.observe, err = r.s.m.w.d.ObserveWrites(file)
	if err != nil {
		return err
	}
	if err := r.annotationRun(); err != nil {
		return err
	}
	written, err = r.observe()
	if err != nil {
		return err
	}
	if len(written) != 0 || r.s.m.read(r.file) != held {
		return fmt.Errorf("unchanged summary writes %v", written)
	}
	return nil
}
func (r *resultMutation) followedBlock() error {
	r.beforeBlock = originalAnnotationSource + oldAnnotation + "\nmore_code = 7\n"
	return r.s.m.w.write(r.file, r.beforeBlock)
}
func (r *resultMutation) blockLeft() error {
	got := r.s.m.read(r.file)
	if !strings.HasPrefix(got, r.beforeBlock) || strings.Count(got, oldAnnotation) != 1 {
		return fmt.Errorf("followed block changed: %s", got)
	}
	return nil
}
func (r *resultMutation) sourcesHeld() error {
	for file, want := range r.held {
		if got := r.s.m.read(file); got != want {
			return fmt.Errorf("source %s changed", file)
		}
	}
	written, err := r.observe()
	if err != nil {
		return err
	}
	if len(written) != 0 {
		return fmt.Errorf("--no-annotate wrote source files: %v", written)
	}

	return nil
}
func (r *resultMutation) oldMarkers() error {
	return r.s.m.w.write(r.file, originalAnnotationSource+strings.ReplaceAll(strings.ReplaceAll(oldAnnotation, "mutation:", "mutate:"), "end itos-cc mutation", "end itos-cc mutate"))
}
func (r *resultMutation) newMarkers() error {
	got := r.s.m.read(r.file)
	start := strings.Index(got, "# itos-cc mutat")
	if start < 0 || !strings.HasPrefix(got[start:], "# itos-cc mutation:") || !strings.HasSuffix(got, "# end itos-cc mutation\n") {
		return fmt.Errorf("markers not replaced: %s", got)
	}
	return nil
}
