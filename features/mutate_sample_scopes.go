package features

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
)

// The sample scenarios about the test scopes a sampled mutant runs with, and
// the false kill a narrowed scope would make.

func scopeOf(mu map[string]any) string {
	if mu["scope"] == nil {
		return "own"
	}
	return fmt.Sprint(mu["scope"])
}

func (p *sampleMutation) scopes(want string) error {
	snap, err := p.s.snapshot(p.s.file)
	if err != nil {
		return err
	}
	entries := snapshotEntries(snap)
	if len(entries) == 0 {
		return fmt.Errorf("no recorded scopes")
	}
	for _, mu := range entries {
		if scopeOf(mu) != want {
			return fmt.Errorf("scope %s, want %s: %v", scopeOf(mu), want, mu)
		}
	}
	return nil
}

func (p *sampleMutation) scopeVariants() error {
	for _, row := range []struct{ flags, scope string }{{"--test-command 'make test'", "make test"}, {"", "own"}} {
		if err := p.s.m.w.runWith(p.s.m.w.env(), "mutation run --mutate-all "+row.flags+" src/board.ts"); err != nil {
			return err
		}
		if err := p.scopes(row.scope); err != nil {
			return err
		}
	}
	return nil
}

func (p *sampleMutation) reusedScope() error {
	if err := p.s.reusedPlace(); err != nil {
		return err
	}
	return p.scopes("all-tests")
}

func (p *sampleMutation) wholeSuite() error {
	if err := p.commandIs("node_modules/.bin/vitest run --bail=1"); err != nil {
		return err
	}
	obj, err := p.object()
	if err != nil {
		return err
	}
	for _, mu := range sampleEntries(obj) {
		if sinceFunction(listedSite{function: fmt.Sprint(mu["function"])}, p.s.place) {
			if mu["outcome"] != "killed" {
				return fmt.Errorf("whole suite did not kill place: %v", mu)
			}
			return nil
		}
	}
	return fmt.Errorf("place not sampled")
}

func (p *sampleMutation) commandIs(command string) error {
	runs := baselines(p.s.m.w.stderr)
	got := ""
	if len(runs) == 1 {
		got = strings.TrimPrefix(filepath.ToSlash(runs[0].command), filepath.ToSlash(p.s.m.w.dir)+"/")
		// Windows runs an installed tool by its .cmd shim.
		if program, rest, _ := strings.Cut(got, " "); strings.HasSuffix(program, ".cmd") {
			got = strings.TrimSuffix(program, ".cmd") + " " + rest
		}
	}
	if len(runs) != 1 || got != command {
		return fmt.Errorf("sample baselines %v, want %s", runs, command)
	}
	// Observe the original When before the JSON diagnostic rerun adds executions.
	if p.id == "112" {
		if err := p.copyExecutions(); err != nil {
			return err
		}
	}
	obj, err := p.object()
	if err != nil {
		return err
	}
	entries := sampleEntries(obj)
	if len(entries) == 0 {
		return fmt.Errorf("no sampled mutant")
	}
	want := command
	if command == vitestCommand+p.s.file {
		want = "own"
	}
	if command == "node_modules/.bin/vitest run --bail=1" {
		want = "all-tests"
	}
	for _, mu := range entries {
		if scopeOf(mu) != want {
			return fmt.Errorf("sample scope %s, want %s", scopeOf(mu), want)
		}
	}
	return nil
}

func (p *sampleMutation) stripScopes() error {
	if err := p.record("--all-tests"); err != nil {
		return err
	}
	snap, err := p.s.snapshot(p.s.file)
	if err != nil {
		return err
	}
	for _, mu := range snapshotEntries(snap) {
		delete(mu, "scope")
	}
	if err := (&checkMutation{s: p.s}).writeSnapshot(snap); err != nil {
		return err
	}
	if err := p.setControl("all_fail", true); err != nil {
		return err
	}
	w := p.s.m.w
	code, out, stderr, err := w.capture(w.dir, w.env(), "mutation", "sample", "--all-tests", "--json", p.s.file)
	if err != nil {
		return err
	}
	var obj map[string]any
	if err := json.Unmarshal([]byte(out), &obj); err != nil {
		return fmt.Errorf("whole-suite counterfactual JSON: %w: %s%s", err, out, stderr)
	}
	problems := checkProblems(obj)
	if code != 1 || len(problems) != 1 || problems[0]["rule"] != "mutation.baseline-failed" || len(trialsOf(stderr, p.s.file)) != 0 {
		return fmt.Errorf("whole-suite counterfactual exit%d problems%v: %s%s", code, problems, out, stderr)
	}
	return nil
}

func (p *sampleMutation) falseKill(flags string) error {
	switch flags {
	case "without flags":
		flags = ""
	case "with --all-tests":
		flags = "--all-tests"
	case "with --test-command 'make test'":
		flags = "--test-command 'make test'"
	default:
		return fmt.Errorf("unknown fixture flags %s", flags)
	}
	if err := p.record(flags); err != nil {
		return err
	}
	return p.setOutcome(p.s.clear, "<", "killed")
}

func (p *sampleMutation) scopeProblem(scope string) error {
	obj, err := p.object()
	if err != nil {
		return err
	}
	problems := checkProblems(obj)
	if len(problems) != 1 || problems[0]["rule"] != "mutation.mismatch" || problems[0]["scope"] != scope || problems[0]["recorded"] != "killed" || problems[0]["outcome"] != "survived" {
		return fmt.Errorf("scoped mismatch %v, want %s", problems, scope)
	}
	return nil
}

func (p *sampleMutation) fixCommand(command string) error {
	obj, err := p.object()
	if err != nil {
		return err
	}
	problems := checkProblems(obj)
	if len(problems) != 1 || !strings.HasSuffix(filepath.ToSlash(fmt.Sprint(problems[0]["fix"])), ": "+command) {
		return fmt.Errorf("mismatch fix does not run %s: %v", command, problems)
	}
	return nil
}

// mismatchFix is the fix of the one problem the last run's --json object
// reports, a mutation.mismatch, its paths written with slashes.
func (p *sampleMutation) mismatchFix() (string, error) {
	obj, err := p.object()
	if err != nil {
		return "", err
	}
	problems := checkProblems(obj)
	if len(problems) != 1 || problems[0]["rule"] != "mutation.mismatch" {
		return "", fmt.Errorf("want one mutation.mismatch problem: %v", problems)
	}
	return filepath.ToSlash(fmt.Sprint(problems[0]["fix"])), nil
}

// fixEndsWith is whether the mismatch's fix ends with ending.
func (p *sampleMutation) fixEndsWith(ending string) error {
	fix, err := p.mismatchFix()
	if err != nil {
		return err
	}
	if !strings.HasSuffix(fix, ending) {
		return fmt.Errorf("the mismatch's fix %q does not end with %q", fix, ending)
	}
	return nil
}

// fixQuotesOnlyTestCommand is whether the mismatch's fix holds no quote,
// single, double or back, but the two single quotes around make test.
func (p *sampleMutation) fixQuotesOnlyTestCommand() error {
	fix, err := p.mismatchFix()
	if err != nil {
		return err
	}
	rest := strings.Replace(fix, "'make test'", "make test", 1)
	if rest == fix || strings.ContainsAny(rest, "'\"`") {
		return fmt.Errorf("the mismatch's fix %q holds a quote but the two around make test", fix)
	}
	return nil
}
