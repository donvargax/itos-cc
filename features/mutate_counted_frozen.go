package features

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/cucumber/godog"
)

func (q *countedMutation) frozenSteps(sc *godog.ScenarioContext) {
	sc.Step(`^a committed project has a source function, tests and measurement settings$`, q.frozenProject)
	sc.Step(`^those files have different staged, unstaged or untracked working-tree versions$`, q.dirtyInputs)
	sc.Step(`^a count-one run judges a committed selection$`, q.judge)
	sc.Step(`^discovery, coverage, clean baselines and mutation tests use the resolved committed inputs$`, q.committedExecutions)
	sc.Step(`^its report names that commit and logical source paths$`, q.committedReport)
	sc.Step(`^the live working tree is not rewritten or mistaken for committed proof$`, q.unchangedDirty)
	sc.Step(`^unsupported scopes fail rather than silently following live project inputs$`, q.unsupportedInputs)
	sc.Step(`^a multi-site committed function has no complete mutation results$`, q.noCompleteProof)
	sc.Step(`^a count-one run completes successfully$`, func() error {
		var err error
		q.primary, err = q.count("--count", "1", "--seed", q.seed, "--workers", "1", "main.go")
		return err
	})
	sc.Step(`^no mutation snapshot, source annotation or persistent coverage cache is published$`, q.noPublishedProof)
	sc.Step(`^complete mutation check still reports its missing or stale complete proof without running tests$`, q.checkMissingProof)
	sc.Step(`^preexisting snapshot and annotated source bytes remain unchanged$`, q.completeBytesUnchanged)
	sc.Step(`^any genuinely valid complete cache remains a separate unchanged source of proof$`, q.independentCompleteProof)
}
func (q *countedMutation) frozenProject(ctx context.Context) error {
	if err := q.project(ownedOneSource); err != nil {
		return err
	}
	var err error
	q.head, err = q.e.m.w.d.Head(ctx, q.e.m.w.dir)
	return err
}
func (q *countedMutation) dirtyInputs(ctx context.Context) error {
	w := q.e.m.w
	for file, text := range map[string]string{"main.go": "package broken\n", "main_test.go": "staged tests cannot compile\n", "itos-cc.yaml": "mutation:\n  test_command: false\n"} {
		if err := w.write(file, text); err != nil {
			return err
		}
	}
	if err := w.d.Git(w.dir, "add", "main.go", "main_test.go", "itos-cc.yaml"); err != nil {
		return err
	}
	if err := w.write("main.go", "unstaged source cannot compile\n"); err != nil {
		return err
	}
	if err := w.write("untracked.go", "untracked source cannot compile\n"); err != nil {
		return err
	}
	var err error
	q.before, err = q.tree(ctx, w.dir)
	return err
}
func countedSHA(text string) string {
	h := sha256.Sum256([]byte(text))
	return hex.EncodeToString(h[:])
}
func (q *countedMutation) committedExecutions() error {
	o := q.primary
	s, err := countedSelected(o)
	if err != nil {
		return err
	}
	if o.result.Exit != 0 || s["outcome"] != "killed" || countedSampling(o)["executed"] != float64(1) {
		return fmt.Errorf("committed execution outcome: %v", o.object)
	}
	original := countedSHA(ownedOneSource)
	mutated := countedSHA(strings.Replace(ownedOneSource, ">", ">=", 1))
	coverage, baseline, trial := 0, 0, 0
	for _, r := range o.traces {
		if r["cwd"] == "" || r["original_root"] == "" || r["cwd"] == r["original_root"] {
			return fmt.Errorf("committed execution used live root: %v", r)
		}
		switch r["source_sha256"] {
		case original:
			if r["coverage"] == true {
				coverage++
			} else {
				baseline++
			}
		case mutated:
			if r["coverage"] == true {
				return fmt.Errorf("mutant used for coverage: %v", r)
			}
			trial++
		default:
			return fmt.Errorf("execution did not use committed source: %v", r)
		}
	}
	if coverage < 1 || baseline != 1 || trial != 1 {
		return fmt.Errorf("committed actual execution population coverage%d baseline%d trial%d: %v", coverage, baseline, trial, o.traces)
	}
	return nil
}
func (q *countedMutation) committedReport() error {
	s, err := countedSelected(q.primary)
	if err != nil {
		return err
	}
	if countedSampling(q.primary)["commit"] != q.head || s["file"] != "main.go" || !strictNamed(fmt.Sprint(s["function"]), "Compare") {
		return fmt.Errorf("committed logical report: %v", q.primary.object)
	}
	return nil
}
func (q *countedMutation) unchangedDirty(ctx context.Context) error {
	return q.sameTree(ctx, q.e.m.w.dir, q.before)
}
func (q *countedMutation) unsupportedInputs() error {
	other, err := q.otherProject(ownedOneSource)
	if err != nil {
		return err
	}
	w := other.e.m.w
	if err = w.d.Link(filepath.Join(w.dir, "main.go"), filepath.Join(w.dir, "alias.go")); err != nil {
		return err
	}
	if err = w.d.Git(w.dir, "add", "alias.go"); err != nil {
		return err
	}
	if err = w.d.Git(w.dir, "commit", "-qm", "unsupported linked source"); err != nil {
		return err
	}
	o, err := other.count("--count", "1")
	if err != nil {
		return err
	}
	if err = countedProblem(o, "count.unsupported-scope", 2); err != nil {
		return err
	}
	if len(o.traces) != 0 {
		return fmt.Errorf("unsupported scope executed fixture: %v", o.traces)
	}
	return strictNoCommands(o.result.Stderr)
}

const countedMultiSource = "package main\n\nfunc Compare(n int) bool { return n > 5 && n < 9 }\n"
const countedMultiProfile = "mode: set\nexample.com/ownership/main.go:3.28,3.51 1 1\n"

func (q *countedMutation) multiProject() error {
	if err := q.project(countedMultiSource); err != nil {
		return err
	}
	w := q.e.m.w
	tests := strings.NewReplacer("TRACE_PATH", fmt.Sprintf("%q", filepath.Join(q.e.outside, "trace.jsonl")), "CONTROL_PATH", fmt.Sprintf("%q", filepath.Join(q.e.outside, "counted-control.json")), "ORIGINAL_ROOT", fmt.Sprintf("%q", w.dir)).Replace(countedGoTests)
	tests = strings.Replace(tests, "!Compare(6)||Compare(5)", "!Compare(6)||Compare(5)||Compare(9)", 1)
	if err := w.write("main_test.go", tests+"\nfunc TestOther(t *testing.T){if !Other(6)||Other(5){t.Fatal(\"Other\")}}\n"); err != nil {
		return err
	}
	if err := w.write("other.go", "package main\n\nfunc Other(n int) bool { return n > 5 }\n"); err != nil {
		return err
	}
	if err := w.d.Git(w.dir, "add", "main_test.go", "other.go"); err != nil {
		return err
	}
	if err := w.d.Git(w.dir, "commit", "-qm", "independent complete proof"); err != nil {
		return err
	}
	report := countedMultiProfile + "example.com/ownership/other.go:3.26,3.40 1 1\n"
	if err := q.auditProfile(report); err != nil {
		return err
	}
	return nil
}
func (q *countedMutation) noCompleteProof(ctx context.Context) error {
	if err := q.multiProject(); err != nil {
		return err
	}
	w := q.e.m.w
	complete, err := q.observe("mutation", "run", "--json", "--workers", "1", "other.go")
	if err != nil {
		return err
	}
	if complete.result.Exit != 0 {
		return fmt.Errorf("independent complete setup: %v", complete.object)
	}
	if w.d.Exists(filepath.Join(w.dir, ".metrics", "mutate", "main.go.json")) {
		return fmt.Errorf("multi-site function already has complete proof")
	}
	q.saved["complete-other"] = complete
	q.before, err = q.tree(ctx, w.dir)
	return err
}
func (q *countedMutation) noPublishedProof(ctx context.Context) error {
	s, err := countedSelected(q.primary)
	if err != nil {
		return err
	}
	if q.primary.result.Exit != 0 || s["outcome"] != "killed" || countedSampling(q.primary)["eligible"] != float64(3) || countedSampling(q.primary)["executed"] != float64(1) {
		return fmt.Errorf("sampled multi-site judgment: %v", q.primary.object)
	}
	return q.sameTree(ctx, q.e.m.w.dir, q.before)
}
func (q *countedMutation) checkMissingProof() error {
	o, err := q.observe("mutation", "check", "--json", "main.go")
	if err != nil {
		return err
	}
	found := false
	for _, p := range checkProblems(o.object) {
		if p["rule"] == "mutation.missing" && p["file"] == "main.go" {
			found = true
		}
	}
	if o.result.Exit != 1 || !found || len(o.traces) != 0 {
		return fmt.Errorf("sampled work mistaken for complete proof: %v trace%v", o.object, o.traces)
	}
	return strictNoCommands(o.result.Stderr)
}
func (q *countedMutation) completeBytesUnchanged(ctx context.Context) error {
	for _, file := range []string{".metrics/mutate/other.go.json", "other.go"} {
		before, ok := q.before[file]
		if !ok || before == "" {
			return fmt.Errorf("preexisting complete proof bytes absent: %s", file)
		}
		current, err := q.e.m.w.d.Read(filepath.Join(q.e.m.w.dir, filepath.FromSlash(file)))
		if err != nil {
			return err
		}
		if string(current) != before {
			return fmt.Errorf("preexisting snapshot/annotation changed: %s", file)
		}
		if file == "other.go" && (!strings.Contains(before, "itos-cc") || !strings.Contains(before, "killed")) {
			return fmt.Errorf("preexisting source annotation was not genuine: %s", before)
		}
	}
	return q.sameTree(ctx, q.e.m.w.dir, q.before)
}
func (q *countedMutation) independentCompleteProof() error {
	o, err := q.observe("mutation", "check", "--json", "other.go")
	if err != nil {
		return err
	}
	if o.result.Exit != 0 || o.object["ok"] != true || len(checkProblems(o.object)) != 0 || len(o.traces) != 0 {
		return fmt.Errorf("independent complete proof lost: %v", o.object)
	}
	f, err := executionFile(o.object, "other.go")
	if err != nil {
		return err
	}
	mu := objectList(f, "functions")
	if len(mu) != 1 || mu[0]["state"] != "fresh" || mu[0]["killed"] != float64(1) || mu[0]["survived"] != float64(0) {
		return fmt.Errorf("independent complete outcome: %v", f)
	}
	return strictNoCommands(o.result.Stderr)
}
