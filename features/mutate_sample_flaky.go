package features

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

// flipping has the tests decide the place mutant at its boundary by
// outcomes, in turn, run after run, through the control file outside the
// project alone: the source, the tests and the snapshot stay as recorded,
// and sample is watched for writing them.
func (p *sampleMutation) flipping(outcomes ...string) error {
	p.flip = outcomes
	if err := p.writeControl(); err != nil {
		return err
	}
	if err := p.declare(); err != nil {
		return err
	}
	if err := p.s.m.fixtureUnchanged(); err != nil {
		return err
	}
	w := p.s.m.w
	var err error
	p.observe, err = w.d.ObserveWrites(filepath.Join(w.dir, p.s.file), filepath.Join(w.dir, ".metrics/mutate/"+p.s.file+".json"))
	return err
}

// flaky is whether the last run reports one problem, a mutation.flaky of
// the place mutant at its boundary, recorded as recorded, with outcomes,
// in run order, and the fields of a mismatch beside them: outcome, its
// first, and scope, the file's own tests.
func (p *sampleMutation) flaky(recorded string, outcomes []string, full bool) error {
	obj, err := p.object()
	if err != nil {
		return err
	}
	problems := checkProblems(obj)
	if len(problems) != 1 {
		return fmt.Errorf("sample problems %v, want one mutation.flaky\n%s", problems, p.s.m.w.report())
	}
	mu := problems[0]
	if mu["rule"] != "mutation.flaky" || mu["recorded"] != recorded || mu["outcome"] != outcomes[0] || mu["scope"] != "own" || filepath.ToSlash(fmt.Sprint(mu["file"])) != p.s.file || !sinceFunction(listedSite{function: fmt.Sprint(mu["function"])}, p.s.place) {
		return fmt.Errorf("unexpected flaky problem %v\n%s", mu, p.s.m.w.report())
	}
	if got := fmt.Sprint(mu["outcomes"]); got != fmt.Sprint(outcomes) {
		return fmt.Errorf("flaky outcomes %s, want %v", got, outcomes)
	}
	if mu["exit"] != float64(1) {
		return fmt.Errorf("flaky exit %v, want 1", mu["exit"])
	}
	if full {
		for key, want := range map[string]any{"line": float64(5), "column": float64(9), "original": ">", "replacement": ">="} {
			if mu[key] != want {
				return fmt.Errorf("flaky %s=%v, want %v", key, mu[key], want)
			}
		}
	}
	return nil
}

// runsOf are how many times the last run says each sampled mutant ran,
// by its line, original and replacement: its progress lines on stderr,
// which must agree with the outcomes --json lists for it.
func (p *sampleMutation) runsOf() (map[string]int, error) {
	w := p.s.m.w
	ran := map[string]int{}
	for _, t := range trialsOf(w.stderr, p.s.file) {
		ran[fmt.Sprintf("%d %s → %s", t.line, t.original, t.replacement)]++
	}
	obj, err := p.object()
	if err != nil {
		return nil, err
	}
	entries := sampleEntries(obj)
	if len(entries) != len(ran) {
		return nil, fmt.Errorf("sampled %v, ran %v\n%s", entries, ran, w.report())
	}
	for _, mu := range entries {
		key := fmt.Sprintf("%v %v → %v", mu["line"], mu["original"], mu["replacement"])
		outcomes, _ := mu["outcomes"].([]any)
		if len(outcomes) == 0 || len(outcomes) != ran[key] || outcomes[0] != mu["outcome"] {
			return nil, fmt.Errorf("sampled mutant %v ran %d times\n%s", mu, ran[key], w.report())
		}
	}
	return ran, nil
}

func (p *sampleMutation) ranThrice() error {
	ran, err := p.runsOf()
	if err != nil {
		return err
	}
	if ran["5 > → >="] != 3 {
		return fmt.Errorf("the mismatched mutant ran %d times, want 3: %v\n%s", ran["5 > → >="], ran, p.s.m.w.report())
	}
	return nil
}

func (p *sampleMutation) ranOnce() error {
	ran, err := p.runsOf()
	if err != nil {
		return err
	}
	if len(ran) != 2 {
		return fmt.Errorf("sampled %v, want both mutants", ran)
	}
	for site, n := range ran {
		if n != 1 {
			return fmt.Errorf("%s ran %d times, want once\n%s", site, n, p.s.m.w.report())
		}
	}
	return nil
}

func (p *sampleMutation) outcomesListed() error {
	obj, _, err := p.s.m.w.lastObject()
	if err != nil {
		return err
	}
	entries := sampleEntries(obj)
	if len(entries) != 1 || entries[0]["original"] != ">" || entries[0]["line"] != float64(5) {
		return fmt.Errorf("sampled %v, want the mutant at the boundary alone", entries)
	}
	if got := fmt.Sprint(entries[0]["outcomes"]); got != "[survived killed]" {
		return fmt.Errorf("sampled mutant %v lists outcomes %s, want [survived killed]", entries[0], got)
	}
	return nil
}

func (p *sampleMutation) deterministicFix() error {
	obj, err := p.object()
	if err != nil {
		return err
	}
	problems := checkProblems(obj)
	if len(problems) != 1 || problems[0]["rule"] != "mutation.flaky" {
		return fmt.Errorf("sample problems %v, want one mutation.flaky", problems)
	}
	fix := fmt.Sprint(problems[0]["fix"])
	if !strings.Contains(fix, "deterministic") || strings.Contains(fix, "mutation run") || strings.Contains(fix, "itos-cc") {
		return fmt.Errorf("flaky fix %q", fix)
	}
	return nil
}

// stdoutSays is whether stdout says text, where "Board#place" may stand
// for the function under the namespace its language gives it.
func (p *sampleMutation) stdoutSays(text string) error {
	name := regexp.QuoteMeta(p.s.place)
	pattern := strings.ReplaceAll(regexp.QuoteMeta(text), " in "+name, ` in (?:\S*[./])?`+name)
	if regexp.MustCompile(pattern).MatchString(filepath.ToSlash(p.s.m.w.stdout)) {
		return nil
	}
	return fmt.Errorf("stdout does not say %q\n%s", text, p.s.m.w.report())
}
