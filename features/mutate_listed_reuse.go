package features

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// What a later listed run reuses, reruns and finds stale, and the import graph
// its tests agree with.

func (l *listedMutation) reuses() error {
	if err := l.m.w.runWith(l.m.w.env(), "mutation run --json main.go"); err != nil {
		return err
	}
	entries, err := l.m.greetMutants()
	if err != nil {
		return err
	}
	return l.assertLines(entries, listedManyLine, func(e map[string]any) bool { return e["reused"] == true && e["scope"] == "listed" })
}

func (l *listedMutation) checkObject() (map[string]any, error) {
	object, _, err := l.m.w.lastObject()
	return object, err
}

func (l *listedMutation) stale(functions []string, file string) error {
	object, err := l.checkObject()
	if err != nil {
		return err
	}
	problems, _ := object["problems"].([]any)
	for _, fn := range functions {
		found := false
		for _, p := range problems {
			entry := p.(map[string]any)
			if entry["rule"] == "mutation.stale" && entry["function"] == "example.com/greet#"+fn && strings.Contains(fmt.Sprint(entry["message"]), file) {
				found = true
			}
		}
		if !found {
			return fmt.Errorf("no stale %s naming %s\n%s", fn, file, l.m.w.report())
		}
	}
	return nil
}

func (l *listedMutation) ownFresh() error {
	object, err := l.checkObject()
	if err != nil {
		return err
	}
	files, _ := object["files"].([]any)
	for _, f := range files {
		functions, _ := f.(map[string]any)["functions"].([]any)
		for _, fn := range functions {
			v := fn.(map[string]any)
			if v["function"] == "example.com/greet#half" && v["state"] == "fresh" {
				return nil
			}
		}
	}
	return fmt.Errorf("half is not fresh\n%s", l.m.w.report())
}

func (l *listedMutation) reruns() error {
	if err := l.m.w.runWith(l.m.w.env(), "mutation run --json main.go"); err != nil {
		return err
	}
	entries, err := l.m.greetMutants()
	if err != nil {
		return err
	}
	if err := l.assertLines(entries, listedManyLine, func(e map[string]any) bool { return e["reused"] != true && e["outcome"] == "killed" }); err != nil {
		return err
	}
	return l.ranAfterOwn(listedManyLine, "ID-A-02", "killed")
}

func (l *listedMutation) buildGraph(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	var err error
	l.graph, err = l.m.w.d.Graph(ctx, Run{Dir: l.m.w.dir, Env: l.m.w.env()})
	return err
}

func (l *listedMutation) graphAgrees() error {
	var graph struct {
		Nodes []struct {
			Units []struct {
				File, Namespace, Name string
				Stale                 bool
			}
		}
	}
	if err := json.Unmarshal(l.graph, &graph); err != nil {
		return err
	}
	states := map[string]bool{}
	for _, n := range graph.Nodes {
		for _, u := range n.Units {
			if u.File == "main.go" {
				states[u.Namespace+"#"+u.Name] = u.Stale
			}
		}
	}
	if !states["example.com/greet#greet"] || states["example.com/greet#half"] {
		return fmt.Errorf("graph freshness %v", states)
	}
	if err := l.m.w.runWith(l.m.w.env(), "mutation check --json main.go"); err != nil {
		return err
	}
	if err := l.stale([]string{"greet"}, "a.feature"); err != nil {
		return err
	}
	return l.ownFresh()
}
