package features

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"time"
)

// The suites of mixed-language projects: their freshness, states and partial
// runs.

func (u *languageSuites) mixedProjects() error {
	l, w := u.l, u.l.m.w
	for _, language := range []string{"typescript", "python", "kotlin"} {
		p, err := u.project(language, language, true, false)
		if err != nil {
			return err
		}
		u.projects = append(u.projects, p)
	}
	if err := w.git("init", "-q"); err != nil {
		return err
	}
	if err := l.s.commit("base mixed language populations"); err != nil {
		return err
	}
	if err := w.git("tag", "base"); err != nil {
		return err
	}
	for _, p := range u.projects {
		if err := u.record(p, "--all-tests", true); err != nil {
			return err
		}
		a := u.initial[p.file]
		for _, unit := range objectList(a, "units") {
			name := fmt.Sprint(unit["name"])
			for _, m := range objectList(unit, "mutants") {
				if name == "own" {
					delete(m, "scope")
					delete(m, "suite_evidence")
					delete(m, "go_evidence")
				}
				if name == "command" {
					m["scope"] = p.command
				}
			}
		}
		if err := l.writeSnapshot(p.file, a); err != nil {
			return err
		}
		snap, err := l.s.snapshot(p.file)
		u.initial[p.file] = snap
		if err != nil {
			return err
		}
	}
	return nil
}

func suiteKey(file, name string) string { return file + "#" + name }

func (u *languageSuites) checkStates(a map[string]any) map[string]string {
	states := map[string]string{}
	for _, f := range objectList(a, "files") {
		file := filepath.ToSlash(fmt.Sprint(f["file"]))
		for _, unit := range objectList(f, "functions") {
			_, name, _ := strings.Cut(fmt.Sprint(unit["function"]), "#")
			states[suiteKey(file, name)] = fmt.Sprint(unit["state"])
		}
	}
	return states
}

func (u *languageSuites) expected() map[string]string {
	out := map[string]string{}
	for _, p := range u.projects {
		for _, name := range p.names {
			state := "stale"
			if name == "own" {
				state = "fresh"
			}
			out[suiteKey(p.file, name)] = state
		}
	}
	return out
}

func (u *languageSuites) paths() string {
	var paths []string
	for _, p := range u.projects {
		paths = append(paths, p.file)
	}
	return strings.Join(paths, " ")
}

func (u *languageSuites) captureFreshness(ctx context.Context) error {
	l := u.l
	a, _, _, err := l.capture("mutation check --json " + u.paths())
	if err != nil {
		return err
	}
	u.checked = u.checkStates(a)
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	raw, err := l.m.w.d.Graph(ctx, Run{Dir: l.m.w.dir, Env: l.m.w.env()})
	if err != nil {
		return err
	}
	var g map[string]any
	if err := json.Unmarshal(raw, &g); err != nil {
		return err
	}
	for _, node := range objectList(g, "nodes") {
		for _, unit := range objectList(node, "units") {
			file := filepath.ToSlash(fmt.Sprint(unit["file"]))
			stale, ok := unit["stale"].(bool)
			if !ok {
				return fmt.Errorf("graph missing stale field: %v", unit)
			}
			u.graph[suiteKey(file, fmt.Sprint(unit["name"]))] = stale
		}
	}
	sample, _, code, err := l.capture("mutation sample --count 50 --seed 4813e48 --json " + u.paths())
	if err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("fresh population sample exit%d: %v", code, sample)
	}
	for _, f := range objectList(sample, "files") {
		file := filepath.ToSlash(fmt.Sprint(f["file"]))
		for _, m := range objectList(f, "mutants") {
			_, name, _ := strings.Cut(fmt.Sprint(m["function"]), "#")
			u.sampled[suiteKey(file, name)] = true
		}
	}
	return nil
}

func (u *languageSuites) mixedStates() error {
	for key, want := range u.expected() {
		if u.checked[key] != want {
			return fmt.Errorf("missing/incorrect checked population%s: %q want%s; all%v", key, u.checked[key], want, u.checked)
		}
	}
	return nil
}

func (u *languageSuites) observeMixed(ctx context.Context) error {
	if err := u.captureFreshness(ctx); err != nil {
		return err
	}
	return u.executeMixed(ctx)
}

func checkSuitePopulation(want map[string]string, checked map[string]string, graph map[string]bool, sampled map[string]bool) error {
	fresh := 0
	for key, state := range want {
		if checked[key] != state {
			return fmt.Errorf("checked population %s state%q want%s", key, checked[key], state)
		}
		stale, ok := graph[key]
		if !ok || stale != (state == "stale") {
			return fmt.Errorf("graph population%s present%v stale%v want%s", key, ok, stale, state)
		}
		if sampled[key] != (state == "fresh") {
			return fmt.Errorf("sample population%s included%v want fresh=%v;all%v", key, sampled[key], state == "fresh", sampled)
		}
		if state == "fresh" {
			fresh++
		}
	}
	if len(sampled) != fresh {
		return fmt.Errorf("sample actual fresh population %v, want%d", sampled, fresh)
	}
	return nil
}

func (u *languageSuites) postExpected() map[string]string {
	want := u.expected()
	for _, p := range u.projects {
		want[suiteKey(p.file, "helper")] = "fresh"
	}
	return want
}

func (u *languageSuites) mixedAgreement() error {
	if err := checkSuitePopulation(u.expected(), u.checked, u.graph, u.sampled); err != nil {
		return err
	}
	if err := checkSuitePopulation(u.postExpected(), u.postChecked, u.postGraph, u.postSampled); err != nil {
		return err
	}
	for _, p := range u.projects {
		f, err := languageRunFile(u.fullResult, p.file)
		if err != nil {
			return err
		}
		seen := map[string]bool{}
		for _, m := range objectList(f, "mutants") {
			_, name, _ := strings.Cut(fmt.Sprint(m["function"]), "#")
			if !slices.Contains([]string{"own", "all", "command", "helper"}, name) {
				return fmt.Errorf("unexpected final unit: %v", m)
			}
			want := u.postChecked[suiteKey(p.file, name)] == "fresh"
			if m["reused"] != want || m["outcome"] != "killed" {
				return fmt.Errorf("final per-function reuse%s want%v: %v", name, want, m)
			}
			seen[name] = true
		}
		if len(seen) != 4 {
			return fmt.Errorf("missing final function population: %v", f)
		}
	}
	return nil
}

func (u *languageSuites) executeMixed(ctx context.Context) error {
	l := u.l
	for _, p := range u.projects {
		source := l.m.read(p.file)
		helper := "export function helper(): boolean { return true; }\n"
		if p.language == "python" {
			helper = "\ndef helper():\n    return True\n"
		}
		if p.language == "kotlin" {
			helper = "fun helper(): Boolean { return true }\n"
		}
		if err := l.m.w.write(p.file, source+helper); err != nil {
			return err
		}
	}
	paths := []string{"add", "--"}
	for _, p := range u.projects {
		paths = append(paths, p.file, p.other)
	}
	if err := l.m.w.git(paths...); err != nil {
		return err
	}
	if err := l.m.w.git("commit", "-qm", "only new helpers and unrelated tests changed"); err != nil {
		return err
	}
	a, _, code, err := l.capture("mutation run --since base --json --no-coverage " + u.paths())
	if err != nil {
		return err
	}
	if code > 1 {
		return fmt.Errorf("scoped helper run exit%d: %v", code, a)
	}
	u.partialResult = a
	for _, p := range u.projects {
		snap, err := l.s.snapshot(p.file)
		if err != nil {
			return err
		}
		u.partial[p.file] = snap
	}
	checked, graph, sampled := u.checked, u.graph, u.sampled
	u.checked = map[string]string{}
	u.graph = map[string]bool{}
	u.sampled = map[string]bool{}
	if err := u.captureFreshness(ctx); err != nil {
		return err
	}
	u.postChecked, u.postGraph, u.postSampled = u.checked, u.graph, u.sampled
	u.checked, u.graph, u.sampled = checked, graph, sampled
	u.fullResult, _, code, err = l.capture("mutation run --json --no-coverage " + u.paths())
	if err != nil {
		return err
	}
	if code > 1 {
		return fmt.Errorf("full refresh exit%d: %v", code, u.fullResult)
	}
	return nil
}

func (u *languageSuites) partialMixed() error {
	for _, p := range u.projects {
		f, err := languageRunFile(u.partialResult, p.file)
		if err != nil {
			return err
		}
		judged, _ := f["judged"].([]any)
		if len(judged) != 1 || !strings.HasSuffix(fmt.Sprint(judged[0]), "#helper") {
			return fmt.Errorf("scoped run judged old population: %v", f)
		}
		snap := u.partial[p.file]
		if snap == nil {
			return fmt.Errorf("missing actual scoped artifact%s", p.file)
		}
		for _, name := range p.names {
			after, err := languageUnit(snap, name)
			if err != nil {
				return err
			}
			before, err := languageUnit(u.initial[p.file], name)
			if err != nil {
				return err
			}
			mutants := objectList(before, "mutants")
			if before["hash"] == nil || before["hash"] == "" || len(mutants) != 1 || len(objectList(after, "mutants")) != 1 {
				return fmt.Errorf("missing original unjudged hash/judgment: %v -> %v", before, after)
			}
			if name != "own" {
				evidence, _ := mutants[0]["suite_evidence"].(map[string]any)
				tests, _ := evidence["tests"].(map[string]any)
				if len(tests) == 0 || tests[p.other] == nil || tests[p.other] == "" {
					return fmt.Errorf("missing original whole-suite evidence for %s: %v", name, mutants[0])
				}
			}
			if (after["stale"] == true) != (name != "own") {
				return fmt.Errorf("scoped entry %s has incorrect stale marker: %v", suiteKey(p.file, name), after)
			}
			if !reflect.DeepEqual(unjudgedLanguageRecord(after), unjudgedLanguageRecord(before)) {
				return fmt.Errorf("scoped entry %s before%v after%v", suiteKey(p.file, name), before, after)
			}
		}
	}
	return checkSuitePopulation(u.postExpected(), u.postChecked, u.postGraph, u.postSampled)
}

// Stale and absolute source positions may change during a partial run. Every
// recorded judgment and dependency hash must still describe its original run.
func unjudgedLanguageRecord(unit map[string]any) map[string]any {
	out := map[string]any{}
	for key, value := range unit {
		if key != "stale" && key != "start_line" && key != "end_line" && key != "mutants" {
			out[key] = value
		}
	}
	mutants := []map[string]any{}
	for _, mutant := range objectList(unit, "mutants") {
		copy := map[string]any{}
		for key, value := range mutant {
			if key != "line" && key != "column" {
				copy[key] = value
			}
		}
		mutants = append(mutants, copy)
	}
	out["mutants"] = mutants
	return out
}
