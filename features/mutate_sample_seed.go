package features

import (
	"context"
	"fmt"
	"slices"
	"strings"
)

// How a sample's seed draws its mutants: HEAD's by default, the same draw for
// the same seed, and a count it refuses.

func (p *sampleMutation) sameDraw() error {
	e := p.s.m.w.each
	if len(e) != 2 {
		return fmt.Errorf("sample runs %d", len(e))
	}
	a, b := p.trialKeys(e[0].stderr), p.trialKeys(e[1].stderr)
	if len(a) != 20 || !slices.Equal(a, b) {
		return fmt.Errorf("sample draws differ: %v / %v", a, b)
	}
	return nil
}

func (p *sampleMutation) seedIsHead(ctx context.Context) error {
	w := p.s.m.w
	head, err := w.d.Head(ctx, w.dir)
	if err != nil {
		return err
	}
	if len(head) != 40 {
		return fmt.Errorf("HEAD %q is not a full commit id", head)
	}
	for _, run := range w.each {
		if !strings.Contains(run.stdout, head) {
			return fmt.Errorf("sample output does not name HEAD %s: %s", head, run.stdout)
		}
	}
	return nil
}

func (p *sampleMutation) firstSeed() error {
	if err := p.fifty(true); err != nil {
		return err
	}
	if err := p.s.m.w.runWith(p.s.m.w.env(), "mutation sample --seed 4813e48"); err != nil {
		return err
	}
	if err := p.ranCount(20); err != nil {
		return err
	}
	if !strings.Contains(p.s.m.w.stdout, "4813e48") {
		return fmt.Errorf("first sample did not print its seed")
	}
	p.firstTrials = p.trialKeys(p.s.m.w.stderr)
	return nil
}

func (p *sampleMutation) moveHead() error {
	if err := p.s.m.w.write("README.md", "# Fifty mutants\nAnother commit.\n"); err != nil {
		return err
	}
	return p.s.commit("move HEAD")
}

func (p *sampleMutation) seedReproduces() error {
	got := p.trialKeys(p.s.m.w.stderr)
	if len(got) != 20 || !slices.Equal(got, p.firstTrials) {
		return fmt.Errorf("seed draw %v, want %v", got, p.firstTrials)
	}
	return nil
}

func (p *sampleMutation) seedOutside() error {
	if err := p.s.m.w.runWith(p.s.m.w.env(), "mutation sample --seed 1"); err != nil {
		return err
	}
	return p.ranCount(20)
}

func (p *sampleMutation) badCount() error {
	obj, err := p.object()
	if err != nil {
		return err
	}
	problems := checkProblems(obj)
	if len(problems) != 1 || problems[0]["rule"] != "flags.value-invalid" || problems[0]["flag"] != "--count" || problems[0]["value"] != "0" {
		return fmt.Errorf("invalid-count problem %v", problems)
	}
	return nil
}
