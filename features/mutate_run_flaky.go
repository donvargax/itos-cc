package features

import (
	"fmt"
	"slices"
)

// boundaryMutant is the place mutant at its boundary, `>` → `>=` at line
// 5, as the snapshot of the sample fixture records it, and every other
// mutant it records.
func (p *sampleMutation) boundaryMutant(snap map[string]any) (boundary map[string]any, others []map[string]any, err error) {
	for _, u := range p.s.units(snap, p.s.place) {
		for _, raw := range u["mutants"].([]any) {
			m := raw.(map[string]any)
			if m["original"] == ">" && m["line"] == float64(5) {
				boundary = m
				continue
			}
			others = append(others, m)
		}
	}
	for _, u := range p.s.units(snap, p.s.clear) {
		for _, raw := range u["mutants"].([]any) {
			others = append(others, raw.(map[string]any))
		}
	}
	if boundary == nil {
		return nil, nil, fmt.Errorf("the snapshot records no mutant `>` → `>=` at line 5 of %s: %v", p.s.place, snap)
	}
	return boundary, others, nil
}

// recordedFlaky is whether the snapshot records the place mutant at its
// boundary with "flaky": true and outcomes, in run order, and no other
// mutant with either key.
func (p *sampleMutation) recordedFlaky(outcomes ...string) error {
	snap, err := p.s.snapshot(p.s.file)
	if err != nil {
		return err
	}
	m, others, err := p.boundaryMutant(snap)
	if err != nil {
		return err
	}
	if m["flaky"] != true || fmt.Sprint(m["outcomes"]) != fmt.Sprint(outcomes) {
		return fmt.Errorf("the mutant at the boundary is recorded %v, want \"flaky\": true and \"outcomes\" %v\n%s", m, outcomes, p.s.m.w.report())
	}
	for _, o := range others {
		if _, ok := o["flaky"]; ok {
			return fmt.Errorf("another mutant is recorded flaky: %v", o)
		}
		if _, ok := o["outcomes"]; ok {
			return fmt.Errorf("another mutant is recorded with outcomes: %v", o)
		}
	}
	return nil
}

// flakyListed is whether the plain output of the last run, made again
// without --json when it had it, which must exit as it did, lists the
// place mutant at its boundary on a "flaky" line in slice-51's site shape,
// and counts 1 flaky in the file's summary line, after its other counts.
func (p *sampleMutation) flakyListed() error {
	w := p.s.m.w
	stdout := w.stdout
	if slices.Contains(w.last.args, "--json") {
		args := slices.DeleteFunc(slices.Clone(w.last.args), func(arg string) bool { return arg == "--json" })
		code, out, errOut, err := w.capture(w.last.dir, w.last.env, args...)
		if err != nil {
			return err
		}
		if code != w.exit {
			return fmt.Errorf("without --json itos-cc exited %d, not %d as with it\n%s\n--- stdout\n%s--- stderr\n%s", code, w.exit, w.report(), out, errOut)
		}
		stdout = out
	}
	saved := w.stdout
	w.stdout = stdout
	defer func() { w.stdout = saved }()
	if err := p.stdoutSays("flaky `>` → `>=` in Board#place, src/board.ts line 5 column 9"); err != nil {
		return err
	}
	return p.stdoutSays(" uncovered, 1 flaky in src/board.ts (ran ")
}

// recordFlaky records the fixture, every mutant killed, then has its
// snapshot record the place mutant at its boundary as a run records a
// flaky one: its first outcome, killed, "flaky": true and its outcomes,
// killed and survived, counted neither killed nor survived.
func (p *sampleMutation) recordFlaky() error {
	if err := p.record(""); err != nil {
		return err
	}
	snap, err := p.s.snapshot(p.s.file)
	if err != nil {
		return err
	}
	m, _, err := p.boundaryMutant(snap)
	if err != nil {
		return err
	}
	if m["outcome"] != "killed" {
		return fmt.Errorf("the mutant at the boundary was recorded %v, want killed before it is made flaky\n%s", m["outcome"], p.s.m.w.report())
	}
	m["flaky"], m["outcomes"] = true, []any{"killed", "survived"}
	u := p.s.units(snap, p.s.place)[0]
	u["killed"] = u["killed"].(float64) - 1
	if err := (&checkMutation{s: p.s}).writeSnapshot(snap); err != nil {
		return err
	}
	name := ".metrics/mutate/" + p.s.file + ".json"
	p.s.m.held[name] = p.s.m.read(name)
	return nil
}

// settled has the tests kill the place mutant at its boundary on every
// run, then is whether mutation run judges it three times, records it
// killed with neither "flaky" nor "outcomes", and mutation check then
// exits 0.
func (p *sampleMutation) settled() error {
	p.flip = nil
	if err := p.writeControl(); err != nil {
		return err
	}
	if err := p.declare(); err != nil {
		return err
	}
	w := p.s.m.w
	if err := w.runWith(w.env(), "mutation run src/board.ts"); err != nil {
		return err
	}
	if w.exit != 0 {
		return fmt.Errorf("mutation run exited %d, want 0\n%s", w.exit, w.report())
	}
	runs := 0
	for _, t := range trialsOf(w.stderr, p.s.file) {
		if t.line == 5 && t.original == ">" {
			runs++
			if t.outcome != "killed" {
				return fmt.Errorf("the mutant at the boundary ran %s, want killed\n%s", t.outcome, w.report())
			}
		}
	}
	if runs != 3 {
		return fmt.Errorf("the mutant at the boundary ran %d times, want 3 before it is settled\n%s", runs, w.report())
	}
	snap, err := p.s.snapshot(p.s.file)
	if err != nil {
		return err
	}
	m, _, err := p.boundaryMutant(snap)
	if err != nil {
		return err
	}
	_, flaky := m["flaky"]
	_, outcomes := m["outcomes"]
	if m["outcome"] != "killed" || flaky || outcomes {
		return fmt.Errorf("the mutant at the boundary is recorded %v, want killed with no \"flaky\" or \"outcomes\"\n%s", m, w.report())
	}
	if err := w.runWith(w.env(), "mutation check src/board.ts"); err != nil {
		return err
	}
	if w.exit != 0 {
		return fmt.Errorf("mutation check exited %d, want 0\n%s", w.exit, w.report())
	}
	return nil
}
