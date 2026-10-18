package features

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/cucumber/godog"
)

func sampleMutationSteps(sc *godog.ScenarioContext, m *mutateState, s *sinceMutation) *sampleMutation {
	p := &sampleMutation{s: s}
	sc.Before(func(ctx context.Context, scenario *godog.Scenario) (context.Context, error) {
		*p = sampleMutation{s: s}
		for _, tag := range scenario.Tags {
			if strings.HasPrefix(tag.Name, "@ID-MUT-") {
				p.id = strings.TrimPrefix(tag.Name, "@ID-MUT-")
			}
		}
		switch p.id {
		case "79":
			return ctx, p.fifty(false)
		case "83":
			if err := p.record(""); err != nil {
				return ctx, err
			}
			m.failingSample = func() error {
				p.controls["fail"] = true
				if err := p.writeControl(); err != nil {
					return err
				}
				return p.declare()
			}
		case "84":
			if err := p.record(""); err != nil {
				return ctx, err
			}
			if err := p.setOutcome(s.place, ">", "survived"); err != nil {
				return ctx, err
			}
			m.samplePlain = p.plainAgrees
		case "109":
			return ctx, p.project()
		}
		return ctx, nil
	})
	sc.After(func(ctx context.Context, _ *godog.Scenario, _ error) (context.Context, error) {
		if p.external != "" {
			return ctx, m.w.d.Remove(p.external)
		}
		return ctx, nil
	})
	sc.Step(`^fresh results for every function of src/board\.ts, with killed mutants and one survivor$`, func() error {
		if err := p.record(""); err != nil {
			return err
		}
		if err := s.unitOutcomes(s.before, s.place, "killed"); err != nil {
			return err
		}
		return s.unitOutcomes(s.before, s.clear, "survived")
	})
	sc.Step(`^the tests still kill and miss the same mutants$`, p.stillSame)
	sc.Step("^the snapshot records the mutant `>` → `>=` at src/board\\.ts:5:9 in \"Board#place\" as killed$", func() error { return p.record("") })
	sc.Step(`^the tests no longer kill it$`, func() error { return p.setControl("place_weak", true) })
	sc.Step(`^the snapshot records a mutant of "Board#place" as survived$`, func() error {
		if err := p.record(""); err != nil {
			return err
		}
		return s.unitOutcomes(s.before, s.place, "survived")
	})
	sc.Step(`^the tests now kill it$`, func() error { return p.setControl("place_weak", false) })
	sc.Step(`^the snapshot records a mutant of "Board#place" as killed$`, func() error {
		if err := p.record(""); err != nil {
			return err
		}
		if err := s.unitOutcomes(s.before, s.place, "timeout"); err != nil {
			return err
		}
		if err := p.setOutcome(s.place, "!", "killed"); err != nil {
			return err
		}
		return p.setOutcome(s.clear, "<", "timeout")
	})
	sc.Step(`^it now runs past its timeout$`, func() error { return p.declare() })
	sc.Step(`^"Board#place" has fresh results, with one mutant recorded uncovered$`, p.coveredRecord)
	sc.Step(`^"Board#clear" changed since its results, and "Board#reset" has none$`, func() error {
		if err := s.replace(s.file, "i<3", "i<4"); err != nil {
			return err
		}
		return (&checkMutation{s: s}).addMethod("reset(i: number): boolean { return i==0; }")
	})
	sc.Step(`^the selection holds 50 fresh mutants that ran(?:, across several files)?$`, func() error { return p.fifty(true) })
	sc.Step(`^a run printed the seed "4813e48"$`, p.firstSeed)
	sc.Step(`^HEAD has moved since, with the same snapshots$`, p.moveHead)
	sc.Step(`^I run "itos-cc mutation sample" twice at the same HEAD commit$`, p.twice)
	sc.Step(`^only "Board#place" changed since "base", and a run since recorded its results$`, p.rangeRecorded)
	sc.Step(`^"Board#clear", unchanged, has fresh results too$`, func() error { return s.unitOutcomes(s.before, s.clear, "killed") })
	sc.Step(`^no fresh mutant that ran in the selection$`, func() error { return s.project(false) })
	sc.Step(`^the baseline runs, then each sampled mutant, in a worker's copy$`, p.copyExecutions)
	sc.Step(`^no coverage command runs$`, p.noCoverage)
	sc.Step(`^\.metrics/mutate/src/board\.ts\.json and src/board\.ts are unchanged$`, p.unchanged)
	sc.Step(`^the problem is "mutation\.mismatch", with file, line, column, function, original, replacement, recorded "([^"]*)" and outcome "([^"]*)"$`, func(recorded, outcome string) error { return p.mismatch(recorded, outcome, true) })
	sc.Step(`^the problem is "mutation\.mismatch", with recorded "([^"]*)" and outcome "([^"]*)"$`, func(recorded, outcome string) error { return p.mismatch(recorded, outcome, false) })
	sc.Step(`^no problem is reported$`, p.noProblems)
	sc.Step(`^only the killed, timeout and survived mutants of "Board#place" run$`, p.freshOnly)
	sc.Step(`^no problem names "Board#clear" or "Board#reset"$`, p.noUnselectedProblem)
	sc.Step(`^(\d+) of them run$`, p.ranCount)
	sc.Step(`^"itos-cc mutation sample --count (\d+)" runs (\d+)$`, p.countRun)
	sc.Step(`^"itos-cc mutation sample --count 80" runs all 50$`, func() error { return p.countRun(80, 50) })
	sc.Step(`^both runs sample the same mutants$`, p.sameDraw)
	sc.Step(`^stdout names the seed, the HEAD commit's id$`, p.seedIsHead)
	sc.Step(`^it samples the mutants that run sampled$`, p.seedReproduces)
	sc.Step(`^"itos-cc mutation sample --seed 1" runs$`, p.seedOutside)
	sc.Step(`^the problem is "flags\.value-invalid", with flag "--count" and value "0"$`, p.badCount)
	sc.Step(`^only mutants of "Board#place" run$`, p.onlyPlace)
	sc.Step(`^none of its mutants runs$`, p.noTrials)
	sc.Step(`^each file has "mutants", each sampled one with line, column, function, original, replacement, recorded, outcome, and scope$`, p.sampleJSON)
	sc.Step(`^each mutant in \.metrics/mutate/src/board\.ts\.json records scope "([^"]*)"$`, p.scopes)
	sc.Step(`^with --test-command 'make test' each records "make test", and with neither each records "own"$`, p.scopeVariants)
	sc.Step(`^a run with --all-tests killed every mutant of "Board#place"$`, func() error {
		if err := p.record("--all-tests"); err != nil {
			return err
		}
		return s.unitOutcomes(s.before, s.place, "killed")
	})
	sc.Step(`^the mutants of "Board#place" are reused and still record scope "all-tests"$`, p.reusedScope)
	sc.Step(`^a run with --all-tests recorded a kill in src/board\.ts that only an end-to-end test makes$`, func() error {
		if err := p.record(""); err != nil {
			return err
		}
		if err := s.unitOutcomes(s.before, s.place, "survived"); err != nil {
			return fmt.Errorf("own tests must miss this kill: %w", err)
		}
		if err := p.record("--all-tests"); err != nil {
			return err
		}
		return s.unitOutcomes(s.before, s.place, "killed")
	})
	sc.Step(`^that mutant runs the whole suite and is killed$`, p.wholeSuite)
	sc.Step(`^outcomes of src/board\.ts recorded with scope "all-tests"$`, func() error { return p.record("--all-tests") })
	sc.Step(`^every sampled mutant runs "make test"$`, func() error { return p.commandIs("make test") })
	sc.Step(`^fresh results for src/board\.ts written before outcomes recorded their scope$`, p.stripScopes)
	sc.Step(`^each sampled mutant runs the file's own tests$`, func() error { return p.commandIs(vitestCommand + s.file) })
	sc.Step(`^a run (without flags|with --all-tests|with --test-command 'make test') recorded a kill in src/board\.ts that those tests no longer make$`, p.falseKill)
	sc.Step(`^outcomes of src/board\.ts recorded with scope "own", one of them a kill the whole suite no longer makes$`, func() error { return p.falseKill("without flags") })
	sc.Step(`^the problem is "mutation\.mismatch", with scope "([^"]*)"$`, p.scopeProblem)
	sc.Step(`^its fix says to run "([^"]*)"$`, p.fixCommand)
	return p
}

func (p *sampleMutation) setControl(name string, value bool) error {
	p.controls[name] = value
	if err := p.writeControl(); err != nil {
		return err
	}
	return p.declare()
}
func (p *sampleMutation) stillSame() error {
	if err := p.s.m.fixtureUnchanged(); err != nil {
		return err
	}
	w := p.s.m.w
	if err := w.d.Write(p.trace, nil); err != nil {
		return err
	}
	if err := w.d.Remove(filepath.Join(w.dir, ".metrics/coverage")); err != nil {
		return err
	}
	var err error
	p.observe, err = w.d.ObserveWrites(filepath.Join(w.dir, p.s.file), filepath.Join(w.dir, ".metrics/mutate/"+p.s.file+".json"))
	return err
}
func (p *sampleMutation) noCoverage() error {
	for _, line := range strings.Split(p.s.m.w.stderr, "\n") {
		if strings.HasPrefix(line, "itos-cc: coverage") {
			return fmt.Errorf("coverage command ran: %s", line)
		}
	}
	if p.s.m.w.d.Exists(filepath.Join(p.s.m.w.dir, ".metrics/coverage")) {
		return fmt.Errorf("sample wrote coverage")
	}
	return nil
}
func (p *sampleMutation) unchanged() error {
	if err := p.s.m.fixtureUnchanged(); err != nil {
		return err
	}
	written, err := p.observe()
	if err != nil {
		return err
	}
	if len(written) != 0 {
		return fmt.Errorf("sample wrote %v", written)
	}
	return nil
}
func (p *sampleMutation) trialKeys(stderr string) []string {
	var out []string
	for _, file := range p.files {
		for _, t := range trialsOf(stderr, file) {
			out = append(out, fmt.Sprintf("%s:%d %s → %s", file, t.line, t.original, t.replacement))
		}
	}
	slices.Sort(out)
	return out
}
func (p *sampleMutation) copyExecutions() error {
	w := p.s.m.w
	data, err := w.d.Read(p.trace)
	if err != nil {
		return err
	}
	var records []struct {
		Cwd  string
		Root string `json:"original_root"`
		SHA  string `json:"source_sha256"`
	}
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		var r struct {
			Cwd  string
			Root string `json:"original_root"`
			SHA  string `json:"source_sha256"`
		}
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			return err
		}
		records = append(records, r)
	}
	ran := trialsOf(w.stderr, p.s.file)
	if len(ran) != 2 || len(records) != len(ran)+1 {
		return fmt.Errorf("executions %v and trials %v, want baseline then two mutants", records, ran)
	}
	if records[0].SHA != fmt.Sprintf("%x", sha256.Sum256([]byte(p.original))) {
		return fmt.Errorf("first execution is not the unmutated baseline: %v", records)
	}
	sites, err := p.s.m.sitesOf(p.s.file)
	if err != nil {
		return err
	}
	var want, got []string
	for _, t := range ran {
		found := false
		for _, site := range sites {
			if site.line == t.line && site.original == t.original && site.replacement == t.replacement {
				// The fixture is ASCII; independently edit the public site in its text.
				lines := strings.SplitAfter(p.original, "\n")
				line := lines[site.line-1]
				at := site.column - 1
				if !strings.HasPrefix(line[at:], site.original) {
					return fmt.Errorf("fixture lacks public site %v", site)
				}
				lines[site.line-1] = line[:at] + site.replacement + line[at+len(site.original):]
				want = append(want, fmt.Sprintf("%x", sha256.Sum256([]byte(strings.Join(lines, "")))))
				found = true
			}
		}
		if !found {
			return fmt.Errorf("trial has no public site: %v", t)
		}
	}
	for i, r := range records {
		if !filepath.IsAbs(r.Root) || r.Root != records[0].Root {
			return fmt.Errorf("unstable canonical original root: %v", records)
		}
		rel, err := filepath.Rel(r.Root, r.Cwd)
		if err != nil {
			return err
		}
		if rel == "." || !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return fmt.Errorf("execution stayed in original project: %s", r.Cwd)
		}
		if i > 0 {
			got = append(got, r.SHA)
		}
	}
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		return fmt.Errorf("trial source digests %v, want %v", got, want)
	}
	return nil
}
func (p *sampleMutation) object() (map[string]any, error) {
	obj, _, err := p.s.m.w.lastObject()
	return obj, err
}
func (p *sampleMutation) mismatch(recorded, outcome string, full bool) error {
	obj, err := p.object()
	if err != nil {
		return err
	}
	problems := checkProblems(obj)
	if len(problems) != 1 {
		return fmt.Errorf("mismatch problems %v, want one", problems)
	}
	mu := problems[0]
	if mu["rule"] != "mutation.mismatch" || mu["recorded"] != recorded || mu["outcome"] != outcome || filepath.ToSlash(fmt.Sprint(mu["file"])) != p.s.file || !sinceFunction(listedSite{function: fmt.Sprint(mu["function"])}, p.s.place) {
		return fmt.Errorf("unexpected mismatch %v", mu)
	}
	if full {
		for key, want := range map[string]any{"line": float64(5), "column": float64(9), "original": ">", "replacement": ">="} {
			if mu[key] != want {
				return fmt.Errorf("mismatch %s=%v, want %v", key, mu[key], want)
			}
		}
	}
	return nil
}
func (p *sampleMutation) noProblems() error {
	obj, err := p.object()
	if err != nil {
		return err
	}
	if problems := checkProblems(obj); len(problems) != 0 {
		return fmt.Errorf("sample problems %v", problems)
	}
	mutants := sampleEntries(obj)
	pairs := map[string]string{}
	for _, mu := range mutants {
		pairs[fmt.Sprint(mu["original"])] = fmt.Sprint(mu["recorded"]) + "/" + fmt.Sprint(mu["outcome"])
	}
	if pairs["!"] != "killed/timeout" || pairs["<"] != "timeout/killed" {
		return fmt.Errorf("timeout agreements %v", pairs)
	}
	return nil
}
func sampleEntries(obj map[string]any) []map[string]any {
	var out []map[string]any
	files, _ := obj["files"].([]any)
	for _, raw := range files {
		file := raw.(map[string]any)
		mutants, _ := file["mutants"].([]any)
		for _, mu := range mutants {
			entry := mu.(map[string]any)
			entry["file"] = filepath.ToSlash(fmt.Sprint(file["file"]))
			out = append(out, entry)
		}
	}
	return out
}
func (p *sampleMutation) freshOnly() error {
	obj, err := p.object()
	if err != nil {
		return err
	}
	entries := sampleEntries(obj)
	if len(entries) != 2 || len(trialsOf(p.s.m.w.stderr, p.s.file)) != 2 {
		return fmt.Errorf("sample entries %v, want two fresh mutants", entries)
	}
	var kinds []string
	for _, mu := range entries {
		if !sinceFunction(listedSite{function: fmt.Sprint(mu["function"])}, p.s.place) || mu["recorded"] == "uncovered" {
			return fmt.Errorf("ineligible sampled mutant %v", mu)
		}
		kinds = append(kinds, fmt.Sprint(mu["recorded"]))
	}
	slices.Sort(kinds)
	if !slices.Equal(kinds, []string{"killed", "survived"}) {
		return fmt.Errorf("sampled outcomes %v", kinds)
	}
	return nil
}
func (p *sampleMutation) noUnselectedProblem() error {
	obj, err := p.object()
	if err != nil {
		return err
	}
	for _, name := range []string{p.s.clear, "Board#reset"} {
		matched := 0
		for _, mu := range checkProblems(obj) {
			if sinceFunction(listedSite{function: fmt.Sprint(mu["function"])}, name) {
				matched++
			}
		}
		if matched != 0 {
			return fmt.Errorf("sample reported %d problems for unselected %s", matched, name)
		}
	}
	return nil
}

func (p *sampleMutation) coveredRecord() error {
	if err := p.record(""); err != nil {
		return err
	}
	units := p.s.units(p.s.before, p.s.place)
	if len(units) != 1 {
		return fmt.Errorf("place entries %v", units)
	}
	for _, key := range []string{"killed", "survived", "uncovered"} {
		if units[0][key] != float64(1) {
			return fmt.Errorf("place %s=%v, want one", key, units[0][key])
		}
	}
	return nil
}
func (p *sampleMutation) ranCount(n int) error {
	got := p.trialKeys(p.s.m.w.stderr)
	if len(got) != n {
		return fmt.Errorf("ran %d, want %d\n%s", len(got), n, p.s.m.w.report())
	}
	return p.s.m.w.exitsWith(0)
}
func (p *sampleMutation) countRun(count, want int) error {
	if err := p.s.m.w.runWith(p.s.m.w.env(), fmt.Sprintf("mutation sample --count %d", count)); err != nil {
		return err
	}
	return p.ranCount(want)
}
func (p *sampleMutation) twice() error { return p.s.m.w.runEach("mutation sample", "mutation sample") }
func (p *sampleMutation) rangeRecorded() error {
	if err := p.s.project(false); err != nil {
		return err
	}
	p.files = []string{p.s.file}
	p.sampleOptions()
	if err := p.s.previous(); err != nil {
		return err
	}
	if err := p.s.changePlace("base"); err != nil {
		return err
	}
	if err := p.s.m.w.runWith(p.s.m.w.env(), "mutation run --since base"); err != nil {
		return err
	}
	var err error
	p.s.before, err = p.s.snapshot(p.s.file)
	return err
}
func (p *sampleMutation) onlyPlace() error {
	ran := trialsOf(p.s.m.w.stderr, p.s.file)
	sites, err := p.s.functionSites(p.s.place)
	if err != nil {
		return err
	}
	if len(ran) == 0 {
		return fmt.Errorf("no place mutant ran")
	}
	for _, t := range ran {
		if !slices.ContainsFunc(sites, func(site listedSite) bool {
			return site.line == t.line && site.original == t.original && site.replacement == t.replacement
		}) {
			return fmt.Errorf("unselected trial %v", t)
		}
	}
	return nil
}
func (p *sampleMutation) baselineProblem() error {
	obj, err := p.object()
	if err != nil {
		return err
	}
	problems := checkProblems(obj)
	if len(problems) != 1 || problems[0]["rule"] != "mutation.baseline-failed" || filepath.ToSlash(fmt.Sprint(problems[0]["file"])) != p.s.file {
		return fmt.Errorf("baseline problems %v", problems)
	}
	return nil
}
func (p *sampleMutation) noTrials() error {
	if err := p.baselineProblem(); err != nil {
		return err
	}
	if ran := trialsOf(p.s.m.w.stderr, p.s.file); len(ran) != 0 {
		return fmt.Errorf("baseline failure ran mutants: %v", ran)
	}
	obj, err := p.object()
	if err != nil {
		return err
	}
	if entries := sampleEntries(obj); len(entries) != 0 {
		return fmt.Errorf("baseline failure listed mutants: %v", entries)
	}
	return nil
}
func (p *sampleMutation) sampleJSON() error {
	obj, err := p.s.m.w.json()
	if err != nil {
		return err
	}
	entries := sampleEntries(obj)
	if len(entries) != 2 {
		return fmt.Errorf("sample mutants %v, want two", entries)
	}
	for _, mu := range entries {
		for _, key := range []string{"line", "column", "function", "original", "replacement", "recorded", "outcome", "scope"} {
			if _, ok := mu[key]; !ok {
				return fmt.Errorf("sampled mutant lacks %s: %v", key, mu)
			}
		}
	}
	return nil
}
func (p *sampleMutation) plainAgrees() error {
	w := p.s.m.w
	obj, err := w.json()
	if err != nil {
		return err
	}
	code, out, stderr, err := w.capture(w.dir, w.env(), "mutation", "sample", p.s.file)
	if err != nil {
		return err
	}
	var got, want []string
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "  ") {
			got = append(got, filepath.ToSlash(line))
		}
	}
	for _, mu := range checkProblems(obj) {
		want = append(want, fmt.Sprintf("  mismatch %s:%v:%v %s → %s in %s: recorded %s, now %s", filepath.ToSlash(fmt.Sprint(mu["file"])), mu["line"], mu["column"], quoteSample(fmt.Sprint(mu["original"])), quoteSample(fmt.Sprint(mu["replacement"])), mu["function"], mu["recorded"], mu["outcome"]))
	}
	if len(want) != 1 || !slices.Equal(got, want) || code != 1 || w.exit != 1 {
		return fmt.Errorf("sample plain problems %q exit%d, JSON %q exit%d\n%s%s", got, code, want, w.exit, out, stderr)
	}
	return nil
}
func quoteSample(s string) string {
	if s == "" {
		return "(deleted)"
	}
	return "`" + s + "`"
}
