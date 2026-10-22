// Strict coverage scenarios observe public commands and their stored evidence.
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

type strictMutation struct {
	namespace          string
	warm, warmCheck    map[string]any
	scopeCheckWrites   []string
	scopeRuns          []strictFlagObservation
	scopeNames         []string
	flags              []strictFlagObservation
	changed            *changedLinesExample
	externalReads      func() ([]string, error)
	language, testFile string
	selected, expected []string
	plain, plainCheck  map[string]any

	outside                   []string
	reads                     func() ([]string, error)
	m                         *mutateState
	s                         *sinceMutation
	id, file, function, input string
	wantLine                  int
	saved, partial            map[string]any
	observe                   func() ([]string, error)
	log                       string
	env                       []string
}

func strictMutationSteps(sc *godog.ScenarioContext, m *mutateState, s *sinceMutation) {
	q := &strictMutation{m: m, s: s}
	sc.Before(func(ctx context.Context, scenario *godog.Scenario) (context.Context, error) {
		*q = strictMutation{m: m, s: s, file: "main.go", function: "Choose", wantLine: 4}
		for _, tag := range scenario.Tags {
			if strings.HasPrefix(tag.Name, "@ID-MUT-") {
				q.id = strings.TrimPrefix(tag.Name, "@ID-MUT-")
			}
		}
		if !slices.Contains([]string{"156", "157", "158", "159", "160", "161", "162", "163", "164", "221", "222", "223", "224", "225", "226", "228", "229", "230", "254", "255", "259", "271", "272", "273"}, q.id) {
			return ctx, nil
		}
		m.w.prepare = func(args []string) []string {
			if len(args) > 1 && args[0] == "mutation" && args[1] == "run" {
				if !slices.Contains(args, "--no-annotate") {
					args = append(args, "--no-annotate")
				}
				if !slices.Contains(args, "--workers") {
					args = append(args, "--workers", "2")
				}
			}
			return args
		}
		m.forFile = func(args string) error { return q.run(strings.ReplaceAll(args, "<base>", "base") + " " + q.paths()) }
		return ctx, nil
	})
	sc.After(func(ctx context.Context, _ *godog.Scenario, _ error) (context.Context, error) {
		for _, dir := range q.outside {
			if err := m.w.d.Remove(dir); err != nil {
				return ctx, err
			}
		}
		return ctx, nil
	})
	q.integrationSteps(sc)
	q.refusalSteps(sc)
	q.changedSteps(sc)
	q.lineSteps(sc)
	q.strictDartSteps(sc)
	q.strictPnpmSteps(sc)
	q.goSteps(sc)
}
func (q *strictMutation) run(args string) error {
	return q.m.w.runWith(append(q.m.w.env(), q.env...), args)
}
func (q *strictMutation) capture(args string) (map[string]any, string, int, error) {
	// Per-command environment is product input, including GOWORK and GOFLAGS.
	words, err := commandWords(args)
	if err != nil {
		return nil, "", 0, err
	}
	code, out, log, err := q.m.w.capture(q.m.w.dir, append(q.m.w.env(), q.env...), words...)
	if err != nil {
		return nil, log, code, err
	}
	var a map[string]any
	err = json.Unmarshal([]byte(out), &a)
	return a, log, code, err
}
func (q *strictMutation) object() (map[string]any, error) {
	a, _, err := q.m.w.lastObject()
	return a, err
}
func (q *strictMutation) snapshot() (map[string]any, error) { return q.s.snapshot(q.file) }
func (q *strictMutation) writeSnapshot(a map[string]any) error {
	b, err := json.MarshalIndent(a, "", "  ")
	if err != nil {
		return err
	}
	return q.m.w.write(".metrics/mutate/"+q.file+".json", string(b)+"\n")
}
func (q *strictMutation) observed() error {
	paths := []string{filepath.Join(q.m.w.dir, ".metrics", "mutate", filepath.FromSlash(q.file)+".json"), filepath.Join(q.m.w.dir, filepath.FromSlash(q.file))}
	var err error
	q.observe, err = q.m.w.d.ObserveWrites(paths...)
	return err
}
func strictNoCommands(log string) error {
	for _, word := range []string{"itos-cc: coverage ", "itos-cc: baseline ", "itos-cc: tests ", "itos-cc: list "} {
		if strings.Contains(log, word) {
			return fmt.Errorf("cached command executed work %q: %s", word, log)
		}
	}
	return nil
}
func (q *strictMutation) quiet(log string) error {
	if err := strictNoCommands(log); err != nil {
		return err
	}
	if q.observe == nil {
		return fmt.Errorf("missing write observation")
	}
	changed, err := q.observe()
	if err != nil {
		return err
	}
	if len(changed) > 0 {
		return fmt.Errorf("cached check wrote %v", changed)
	}
	return nil
}
func (q *strictMutation) findings(a map[string]any, rule, name string) []map[string]any {
	var out []map[string]any
	for _, p := range checkProblems(a) {
		if p["rule"] == rule && (name == "" || strictNamed(fmt.Sprint(p["function"]), name)) && (name == "" || q.namespace == "" || p["function"] == q.namespace+"#"+name) {
			out = append(out, p)
		}
	}
	return out
}
func (q *strictMutation) finding(a map[string]any, rule, name string, line int) error {
	ps := q.findings(a, rule, name)
	if len(ps) == 0 {
		return fmt.Errorf("missing %s for %s: %v", rule, name, a)
	}
	for _, p := range ps {
		if filepath.ToSlash(fmt.Sprint(p["file"])) == q.file && (line == 0 || p["line"] == float64(line)) {
			return nil
		}
	}
	return fmt.Errorf("%s has wrong file/line, want %s:%d: %v", rule, q.file, line, ps)
}
func (q *strictMutation) fresh(args string, want int) (map[string]any, error) {
	a, log, code, err := q.capture(args)
	if err != nil {
		return nil, err
	}
	if code != want {
		return nil, fmt.Errorf("%s exit%d want%d: %v\n%s", args, code, want, a, log)
	}
	return a, nil
}
func strictEvidence(u map[string]any) (map[string]any, error) {
	hash, hashOK := u["hash"].(string)
	e, ok := u["go_coverage"].(map[string]any)
	producer, producerOK := e["producer"].(string)
	inputs, inputsOK := e["inputs"].(map[string]any)
	if !hashOK || hash == "" || !producerOK || producer == "" || !inputsOK || len(inputs) == 0 {
		return nil, fmt.Errorf("missing nonempty Go provenance: %v", u)
	}
	if !ok || e["complete"] != true || e["function_hash"] != u["hash"] || len(objectList(e, "blocks")) == 0 {
		return nil, fmt.Errorf("missing complete nonempty owned Go evidence: %v", u)
	}
	return e, nil
}
func (q *strictMutation) allEvidence() error {
	snap, err := q.snapshot()
	if err != nil {
		return err
	}
	count := 0
	for _, u := range objectList(snap, "units") {
		if u["name"] == "main" || u["name"] == "Empty" || u["name"] == "CommentsOnly" {
			continue
		}
		if _, err := strictEvidence(u); err != nil {
			return err
		}
		count++
	}
	if count == 0 {
		return fmt.Errorf("no executable units: %v", snap)
	}
	return nil
}
func (q *strictMutation) save() error    { var err error; q.saved, err = q.snapshot(); return err }
func (q *strictMutation) source() string { return q.m.read(q.file) }
func (q *strictMutation) declareGo(report string) error {
	w := q.m.w
	if err := w.d.Tests(w.dir, Tests{Language: "go", Command: "go test -count=1 -failfast ./."}); err != nil {
		return err
	}
	return w.d.Coverage(w.dir, Coverage{Language: "go", Report: report})
}
func (q *strictMutation) goProject(source, test, report string) error {
	q.namespace = "example.com/strict"
	for file, text := range map[string]string{"go.mod": "module example.com/strict\n\ngo 1.22\n", q.file: source, "main_test.go": test} {
		if err := q.m.w.write(file, text); err != nil {
			return err
		}
	}
	return q.declareGo(report)
}

const strictBaseGo = `package main

func Compare(n int) bool { return n > 5 }
func Choose(b bool) string { if b { return "miss" }; return "hit" }
func main() {}
`
const strictBaseTest = `package main
import "testing"
func TestValues(t *testing.T){if Compare(5){t.Fatal("Compare")};if Choose(false)!="hit"{t.Fatal("Choose")}}
`
const strictBaseReport = `mode: set
example.com/strict/main.go:3.28,3.42 1 1
example.com/strict/main.go:4.30,4.35 1 1
example.com/strict/main.go:4.37,4.52 1 0
example.com/strict/main.go:4.54,4.66 1 1
`

func (q *strictMutation) goSteps(sc *godog.ScenarioContext) {
	sc.Step(`^a Go function with a tested comparison whose mutants are killed and an unexecuted identifier-conditioned branch returning a string with no mutation site$`, func() error {
		q.wantLine = 3
		return q.goProject(`package main

func Choose(n int,b bool) string { if n > 5 { return "high" }; if b { return "miss" }; return "hit" }
func main(){}
`, `package main
import "testing"
func TestChoose(t *testing.T){if Choose(5,false)!="hit" || Choose(6,false)!="high" {t.Fatal("Choose")}}
`, `mode: set
example.com/strict/main.go:3.36,3.45 1 1
example.com/strict/main.go:3.47,3.62 1 1
example.com/strict/main.go:3.64,3.69 1 1
example.com/strict/main.go:3.71,3.86 1 0
example.com/strict/main.go:3.88,3.100 1 1
`)
	})
	sc.Step(`^I run mutation run with --fail-uncovered --json for its source file$`, func() error { return q.run("mutation run --fail-uncovered --json " + q.file) })
	sc.Step(`^the problem is "mutation.uncovered-statement", with file, function and the unexecuted block's line$`, func() error {
		a, err := q.object()
		if err != nil {
			return err
		}
		return q.finding(a, "mutation.uncovered-statement", q.function, q.wantLine)
	})
	sc.Step(`^no original or replacement is invented for that problem$`, func() error {
		a, err := q.object()
		if err != nil {
			return err
		}
		ps := q.findings(a, "mutation.uncovered-statement", q.function)
		if len(ps) == 0 {
			return fmt.Errorf("missing statement finding")
		}
		for _, p := range ps {
			if _, ok := p["original"]; ok {
				return fmt.Errorf("invented original: %v", p)
			}
			if _, ok := p["replacement"]; ok {
				return fmt.Errorf("invented replacement: %v", p)
			}
		}
		return nil
	})
	sc.Step(`^the function's mutant counts remain killed mutants only, with zero uncovered mutants$`, func() error {
		a, err := q.object()
		if err != nil {
			return err
		}
		f, err := languageRunFile(a, q.file)
		if err != nil {
			return err
		}
		if f["killed"] != float64(1) || f["survived"] != float64(0) || f["uncovered"] != float64(0) {
			return fmt.Errorf("strict counts %v", f)
		}
		return nil
	})
	sc.Step(`^I run mutation check with --fail-uncovered --json for the file$`, func() error {
		if err := q.observed(); err != nil {
			return err
		}
		return q.run("mutation check --fail-uncovered --json " + q.file)
	})
	sc.Step(`^it reports the same uncovered statement and exits 1 without running commands or writing files$`, func() error {
		a, err := q.object()
		if err != nil {
			return err
		}
		if err = q.finding(a, "mutation.uncovered-statement", q.function, q.wantLine); err != nil {
			return err
		}
		if err = q.m.w.exitsWith(1); err != nil {
			return err
		}
		return q.quiet(q.m.w.stderr)
	})
	q.zeroSteps(sc)
	q.reuseSteps(sc)
	q.partialSteps(sc)
	q.scopeSteps(sc)
	q.modeSteps(sc)
}

func strictNamed(function, name string) bool {
	return function == name || strings.HasSuffix(function, "#"+name)
}

func (q *strictMutation) paths() string {
	if len(q.selected) > 0 {
		return strings.Join(q.selected, " ")
	}
	return q.file
}
