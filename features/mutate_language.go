// Language rules observe public commands and the fixtures those commands run.
package features

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/cucumber/godog"
)

type languageMutation struct {
	m                              *mutateState
	s                              *sinceMutation
	id, language, file, trace      string
	before                         map[string]string
	dependencyTarget, frozenSource string
}

func languageMutationSteps(sc *godog.ScenarioContext, m *mutateState, s *sinceMutation) {
	l := &languageMutation{m: m, s: s}
	sc.Before(func(ctx context.Context, scenario *godog.Scenario) (context.Context, error) {
		*l = languageMutation{m: m, s: s}
		for _, tag := range scenario.Tags {
			if strings.HasPrefix(tag.Name, "@ID-MUT-") {
				l.id = strings.TrimPrefix(tag.Name, "@ID-MUT-")
			}
		}
		if !slices.Contains([]string{"150", "151", "152", "153", "154", "155", "203", "204", "205", "206", "207", "208", "209", "210", "213", "214", "215", "216", "217", "218", "219", "220", "260", "261"}, l.id) {
			return ctx, nil
		}
		m.w.prepare = func(args []string) []string {
			if len(args) >= 2 && args[0] == "mutation" && args[1] == "run" {
				if !slices.Contains(args, "--no-annotate") {
					args = append(args, "--no-annotate")
				}
				if !slices.Contains(args, "--workers") {
					args = append(args, "--workers", "2")
				}
			}
			return args
		}
		m.forFile = func(args string) error { return l.run(args + " " + l.file) }
		return ctx, nil
	})
	sc.After(func(ctx context.Context, _ *godog.Scenario, _ error) (context.Context, error) {
		if l.trace != "" {
			return ctx, m.w.d.Remove(filepath.Dir(l.trace))
		}
		return ctx, nil
	})
	l.pythonCacheSteps(sc)
	l.ownLanguageSteps(sc)
	l.reachSteps(sc)
	l.broadSteps(sc)
	l.failureSteps(sc)
	l.toolSteps(sc)
	l.suiteSteps(sc)
}

func (l *languageMutation) run(args string) error { return l.m.w.runWith(l.m.w.env(), args) }
func (l *languageMutation) object() (map[string]any, error) {
	a, _, err := l.m.w.lastObject()
	return a, err
}
func (l *languageMutation) capture(args string) (map[string]any, string, int, error) {
	w := l.m.w
	words, err := commandWords(args)
	if err != nil {
		return nil, "", 0, err
	}
	code, out, log, err := w.capture(w.dir, w.env(), words...)
	if err != nil {
		return nil, log, code, err
	}
	var a map[string]any
	err = json.Unmarshal([]byte(out), &a)
	return a, log, code, err
}
func (l *languageMutation) files(ctx context.Context) (map[string]string, error) {
	w := l.m.w
	names, err := w.d.Files(ctx, w.dir)
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, name := range names {
		if name == ".metrics" || strings.HasPrefix(name, ".metrics/") {
			continue
		}
		b, err := w.d.Read(filepath.Join(w.dir, filepath.FromSlash(name)))
		if err != nil {
			return nil, err
		}
		out[name] = string(b)
	}
	return out, nil
}
func languageRunFile(a map[string]any, file string) (map[string]any, error) {
	for _, f := range objectList(a, "files") {
		if filepath.ToSlash(fmt.Sprint(f["file"])) == file {
			return f, nil
		}
	}
	return nil, fmt.Errorf("missing file %s: %v", file, a)
}
func objectList(a map[string]any, key string) []map[string]any {
	var out []map[string]any
	raw, _ := a[key].([]any)
	for _, v := range raw {
		if m, ok := v.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

func languageUnit(snap map[string]any, name string) (map[string]any, error) {
	var found []map[string]any
	for _, u := range objectList(snap, "units") {
		if u["name"] == name {
			found = append(found, u)
		}
	}
	if len(found) != 1 {
		return nil, fmt.Errorf("snapshot has %d units named%s, want exactly one: %v", len(found), name, snap)
	}
	return found[0], nil
}
