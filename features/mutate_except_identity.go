package features

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strings"

	"github.com/cucumber/godog"
	"go.yaml.in/yaml/v3"
)

// The steps of the Rule "A survivor a CI report names is excepted from its
// identity, with no mutation run" (slice-41, issue #66). The fixture is a
// committed Go module, example.com/fx, whose package board holds Count,
// one site, `<` → `<=` at board/board.go:5:11, which its test survives:
// it checks Count(4) and Count(6), never 5. The identity a counted run
// reports for it is identityOfCount, whose hash b1a8f41702cd975f was
// measured from that source. The steps read only what itos-cc prints and
// writes: its --json objects, itos-cc.yaml and .metrics/mutate.

const (
	identityModule = "module example.com/fx\n\ngo 1.22\n"
	// identitySource is board/board.go: Count's one site is `<` on line 5,
	// column 11.
	identitySource = "package board\n\n// Count says whether x is under five.\nfunc Count(x int) bool {\n\treturn x < 5\n}\n"
	// identityTest survives `<` → `<=`, checking Count(4) and Count(6);
	// identityKilling checks Count(5) too, killing it.
	identityTest    = "package board\n\nimport \"testing\"\n\nfunc TestCount(t *testing.T) {\n\tif !Count(4) || Count(6) {\n\t\tt.Fatal(\"Count\")\n\t}\n}\n"
	identityKilling = "package board\n\nimport \"testing\"\n\nfunc TestCount(t *testing.T) {\n\tif !Count(4) || Count(6) || Count(5) {\n\t\tt.Fatal(\"Count\")\n\t}\n}\n"
	// identityProfile is go test's coverprofile of identitySource, its one
	// block executed.
	identityProfile = "mode: set\nexample.com/fx/board/board.go:5.2,6.1 1 1\n"
	identityCommand = "go test -count=1 -failfast ./board"
	identityFile    = "board/board.go"
	identityReason  = "x is never 5"
)

// identityUnit is the part of an identity that names its function's lines
// and hash and the site's offset.
var identityUnit = regexp.MustCompile(`@(\d+):(\d+):([0-9a-f]+):(\d+):`)

type identityException struct {
	w *world
	// counted is the identity the counted run of MUT-318's Given reported.
	counted string
	// external is the folder outside the module of a flipping test's
	// control file, or "".
	external string
}

func identityExceptionSteps(sc *godog.ScenarioContext, w *world) {
	i := &identityException{w: w}
	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		*i = identityException{w: w}
		return ctx, nil
	})
	sc.After(func(ctx context.Context, _ *godog.Scenario, _ error) (context.Context, error) {
		if i.external != "" {
			return ctx, w.d.Remove(i.external)
		}
		return ctx, nil
	})
	sc.Step(`^a committed Go module whose "example\.com/fx/board#Count" survives its test$`, func() error { return i.module(identityTest, true) })
	sc.Step("^a committed Go module whose test also checks Count\\(5\\), so `<` → `<=` is killed$", func() error { return i.module(identityKilling, false) })
	sc.Step(`^its test kills that mutant on one run and misses it on the next, with nothing it hashes changed$`, func() error { return i.flipping("killed", "survived") })
	sc.Step(`^the problem is "mutation\.flaky", with outcomes "([^"]*)", "([^"]*)"$`, func(first, second string) error { return i.countedFlaky(first, second) })
	sc.Step(`^\.metrics/mutate holds no snapshot of board/board\.go$`, i.noSnapshot)
	sc.Step(`^a snapshot of board/board\.go is stale, a later commit having changed board/board_test\.go$`, i.staleSnapshot)
	sc.Step("^\"itos-cc mutation run --count 5 --json\" reported the survivor `<` → `<=` at board/board\\.go:5:11 with its identity$", i.reported)
	sc.Step(`^I except that identity with --reason 'x is never 5'$`, func() error {
		if i.counted == "" {
			return fmt.Errorf("no counted run reported an identity")
		}
		return w.runWith(w.env(), "mutation except '"+i.counted+"' --reason '"+identityReason+"'")
	})
	sc.Step("^it records board/board\\.go, \"example\\.com/fx/board#Count\", the identity's hash, line_in_function 2, column 11, `<`, `<=`, the reason and line_text \"return x < 5\"$", i.records)
	sc.Step(`^once "itos-cc mutation run board/board\.go" has recorded the survivor, "itos-cc mutation except board/board\.go:5:11 --reason 'x is never 5'" says "excepted again" and leaves itos-cc\.yaml byte for byte as it was$`, i.exceptedAgain)
	sc.Step(`^I run "itos-cc ([^"]*)" with no Go toolchain on the PATH$`, func(args string) error {
		w.without = append(slices.Clone(w.without), "go")
		return w.runWith(w.env(), args)
	})
	sc.Step(`^no test or coverage command ran, and \.metrics/mutate was not written$`, i.nothingRan)
	sc.Step(`^with --json it prints \{"exception": the entry itos-cc\.yaml holds, "replaced": true\}$`, i.printsEntry)
	sc.Step(`^a blank line was added above Count's comment since the report$`, func() error {
		return w.write(identityFile, strings.Replace(identitySource, "// Count", "\n// Count", 1))
	})
	sc.Step(`^Count's "x < 5" became "x < 6"$`, func() error {
		return w.write(identityFile, strings.Replace(identitySource, "x < 5", "x < 6", 1))
	})
	sc.Step(`^Count was renamed Below$`, func() error {
		return w.write(identityFile, strings.ReplaceAll(identitySource, "Count", "Below"))
	})
	sc.Step(`^nothing changed$`, func() error { return nil })
	sc.Step(`^its "why" is "(changed|gone)", its "identity" the argument, and its fix names a report of the code as it is now$`, i.staleWhy)
	sc.Step(`^itos-cc\.yaml was not written$`, i.notWritten)
	sc.Step(`^given "--reason ''" it is "flags\.value-invalid", exit 80, and given --renew "args\.unexpected", exit 80$`, i.refusals)
	sc.Step(`^given "--reason 'x is never 5'" it writes the entry, exit 0$`, i.writesWithReason)
	sc.Step(`^"itos-cc mutation run board/board\.go" recorded it killed$`, func() error { return i.recorded("mutation run "+identityFile, "killed", 0) })
	sc.Step(`^its message says the mutant is recorded killed, at board/board\.go:5:11$`, i.killedMessage)
	sc.Step("^its standard error names the survivor `<` → `<=` at board/board\\.go:5:11 with its identity \"([^\"]*)\"$", i.stderrNames)
	sc.Step(`^with --json, the "mutation\.survived" problem's fix names "(.*)"$`, i.survivedFix)
	i.namedSiteSteps(sc)
}

// module writes and commits the module, board_test.go holding test, and
// declares what its tests do and cover: survive `<` → `<=` when survives,
// else kill every mutant.
func (i *identityException) module(test string, survives bool) error {
	w := i.w
	for name, text := range map[string]string{"go.mod": identityModule, identityFile: identitySource, "board/board_test.go": test} {
		if err := w.write(name, text); err != nil {
			return err
		}
	}
	if err := w.git("init", "-q"); err != nil {
		return err
	}
	if err := w.git("add", "go.mod", identityFile, "board/board_test.go"); err != nil {
		return err
	}
	if err := w.git("commit", "-qm", "board"); err != nil {
		return err
	}
	t := Tests{Language: "go", Command: identityCommand}
	if survives {
		t.Surviving = []Site{{File: identityFile, Line: 5, Original: "<", Replacement: "<="}}
	}
	if err := w.d.Tests(w.dir, t); err != nil {
		return err
	}
	return w.d.Coverage(w.dir, Coverage{Language: "go", Report: identityProfile})
}

func (i *identityException) snapshotPath() string {
	return filepath.Join(i.w.dir, ".metrics", "mutate", "board", "board.go.json")
}

func (i *identityException) noSnapshot() error {
	if i.w.d.Exists(i.snapshotPath()) {
		return fmt.Errorf(".metrics/mutate holds a snapshot of board/board.go")
	}
	return nil
}

// staleSnapshot records the survivor with a complete run, then commits a
// change to board_test.go that still survives it, which leaves the
// snapshot stale, as mutation check says.
func (i *identityException) staleSnapshot() error {
	w := i.w
	if err := i.recorded("mutation run --no-annotate "+identityFile, "survived", 1); err != nil {
		return err
	}
	if err := w.write("board/board_test.go", identityTest+"\nfunc TestCountTwo(t *testing.T) {\n\tif !Count(2) {\n\t\tt.Fatal(\"Count(2)\")\n\t}\n}\n"); err != nil {
		return err
	}
	if err := w.git("add", "board/board_test.go"); err != nil {
		return err
	}
	if err := w.git("commit", "-qm", "test Count(2)"); err != nil {
		return err
	}
	code, out, errOut, err := w.capture(w.dir, w.env(), "mutation", "check", "--json", identityFile)
	if err != nil {
		return err
	}
	if code != 1 || !strings.Contains(out, `"mutation.stale"`) {
		return fmt.Errorf("the snapshot of board/board.go is not stale: exit %d\n%s%s", code, out, errOut)
	}
	return nil
}

// recorded runs args, a complete run of board/board.go, which must exit code
// and leave a snapshot recording Count's mutant at 5:11 as outcome.
func (i *identityException) recorded(args, outcome string, code int) error {
	w := i.w
	if err := w.runWith(w.env(), args); err != nil {
		return err
	}
	if w.exit != code {
		return fmt.Errorf("itos-cc %s exited %d, not %d\n%s", args, w.exit, code, w.report())
	}
	return i.snapshotRecords(outcome)
}

// snapshotRecords is whether the snapshot of board/board.go records
// Count's mutant at 5:11 as outcome.
func (i *identityException) snapshotRecords(outcome string) error {
	data, err := i.w.d.Read(i.snapshotPath())
	if err != nil {
		return err
	}
	var snap struct {
		Units []struct {
			Name    string `json:"name"`
			Mutants []struct {
				Line, Column int
				Original     string `json:"original"`
				Replacement  string `json:"replacement"`
				Outcome      string `json:"outcome"`
			} `json:"mutants"`
		} `json:"units"`
	}
	if err := json.Unmarshal(data, &snap); err != nil {
		return err
	}
	for _, u := range snap.Units {
		for _, m := range u.Mutants {
			if u.Name == "Count" && m.Line == 5 && m.Column == 11 && m.Original == "<" && m.Replacement == "<=" && m.Outcome == outcome {
				return nil
			}
		}
	}
	return fmt.Errorf("the snapshot of board/board.go does not record Count's mutant at 5:11 %s\n%s", outcome, data)
}

// reported runs the counted run and keeps the identity it reports for the
// survivor at board/board.go:5:11.
func (i *identityException) reported() error {
	w := i.w
	if err := w.d.Counted(); err != nil {
		return err
	}
	if err := w.runWith(w.env(), "mutation run --count 5 --json"); err != nil {
		return err
	}
	object, err := w.json()
	if err != nil {
		return err
	}
	for _, s := range objectList(object, "selected") {
		file, _ := s["file"].(string)
		if filepath.ToSlash(file) == identityFile && s["line"] == float64(5) && s["column"] == float64(11) && s["original"] == "<" && s["replacement"] == "<=" && s["outcome"] == "survived" {
			i.counted, _ = s["identity"].(string)
		}
	}
	if i.counted == "" || w.exit != 1 {
		return fmt.Errorf("the counted run reported no survivor at board/board.go:5:11 with an identity\n%s", w.report())
	}
	return nil
}

// identity is the argument of the last run that is an identity.
func (i *identityException) identity() (string, error) {
	if i.w.last == nil {
		return "", fmt.Errorf("no run of itos-cc to read")
	}
	for _, arg := range i.w.last.args {
		if identityUnit.MatchString(arg) {
			return arg, nil
		}
	}
	return "", fmt.Errorf("the last run %q names no identity", i.w.last.args)
}

// entries are the entries of itos-cc.yaml under mutation.exceptions.
func (i *identityException) entries() ([]map[string]any, error) {
	data, err := i.w.readIn(i.w.dir, "itos-cc.yaml")
	if err != nil {
		return nil, err
	}
	var doc struct {
		Mutation struct {
			Exceptions []map[string]any `yaml:"exceptions"`
		} `yaml:"mutation"`
	}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	return doc.Mutation.Exceptions, nil
}

// entry is the one entry of itos-cc.yaml, with Count's site and reason.
func (i *identityException) entry(hash string) (map[string]any, error) {
	a, err := i.entries()
	if err != nil {
		return nil, err
	}
	if len(a) != 1 {
		return nil, fmt.Errorf("itos-cc.yaml holds %d entries, not one: %v\n%s", len(a), a, i.w.report())
	}
	want := map[string]any{"file": identityFile, "function": "example.com/fx/board#Count", "line_in_function": 2, "column": 11,
		"original": "<", "replacement": "<=", "reason": identityReason, "line_text": "return x < 5"}
	if hash != "" {
		want["hash"] = hash
	}
	if err := exceptionFields(a[0], want); err != nil {
		return nil, err
	}
	return a[0], nil
}

func (i *identityException) records() error {
	id, err := i.identity()
	if err != nil {
		return err
	}
	_, err = i.entry(identityUnit.FindStringSubmatch(id)[3])
	return err
}

func (i *identityException) exceptedAgain() error {
	w := i.w
	before, err := w.readIn(w.dir, "itos-cc.yaml")
	if err != nil {
		return err
	}
	if err := i.recorded("mutation run "+identityFile, "survived", 0); err != nil {
		return err
	}
	if err := w.runWith(w.env(), "mutation except "+identityFile+":5:11 --reason '"+identityReason+"'"); err != nil {
		return err
	}
	if w.exit != 0 || !strings.Contains(w.stdout, "excepted again") {
		return fmt.Errorf("excepting the site again does not say \"excepted again\"\n%s", w.report())
	}
	after, err := w.readIn(w.dir, "itos-cc.yaml")
	if err != nil {
		return err
	}
	if string(after) != string(before) {
		return fmt.Errorf("itos-cc.yaml changed\n--- before\n%s--- after\n%s", before, after)
	}
	return nil
}

func (i *identityException) nothingRan(ctx context.Context) error {
	if err := strictNoCommands(i.w.stderr); err != nil {
		return err
	}
	files, err := i.w.d.Files(ctx, i.w.dir)
	if err != nil {
		return err
	}
	for _, file := range files {
		if strings.HasPrefix(file, ".metrics/mutate/") {
			return fmt.Errorf("except wrote %s", file)
		}
	}
	return nil
}

// rerun runs the last run again with more words, and gives its --json
// object.
func (i *identityException) rerun(more ...string) (map[string]any, error) {
	w := i.w
	if w.last == nil {
		return nil, fmt.Errorf("no run of itos-cc to run again")
	}
	args := slices.Clone(w.last.args)
	if !slices.Contains(args, "--json") {
		args = append(args, "--json")
	}
	if err := w.runEnv(w.last.dir, w.last.env, append(args, more...)...); err != nil {
		return nil, err
	}
	return w.json()
}

func (i *identityException) printsEntry() error {
	object, err := i.rerun()
	if err != nil {
		return err
	}
	entry, err := i.entry("")
	if err != nil {
		return err
	}
	// The entry as JSON reads it, its numbers floats.
	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	var want map[string]any
	if err := json.Unmarshal(data, &want); err != nil {
		return err
	}
	if i.w.exit != 0 || object["replaced"] != true || !reflect.DeepEqual(object["exception"], want) {
		return fmt.Errorf("--json does not print the entry %v, replaced\n%s", want, i.w.report())
	}
	return nil
}

func (i *identityException) staleWhy(why string) error {
	p := i.w.problem
	if p == nil {
		return fmt.Errorf("no problem found to read")
	}
	id, err := i.identity()
	if err != nil {
		return err
	}
	fix, _ := p["fix"].(string)
	if p["why"] != why || p["identity"] != id || !strings.Contains(fix, "a report of the code as it is now") {
		return fmt.Errorf("the problem %v does not name why %q, the identity %q and a report of the code as it is now", p, why, id)
	}
	return nil
}

func (i *identityException) notWritten() error {
	if i.w.d.Exists(filepath.Join(i.w.dir, "itos-cc.yaml")) {
		data, _ := i.w.readIn(i.w.dir, "itos-cc.yaml")
		return fmt.Errorf("itos-cc.yaml was written\n%s", data)
	}
	return nil
}

func (i *identityException) refusals() error {
	last := *i.w.last
	for _, refused := range []struct {
		more []string
		rule string
	}{{[]string{"--reason", ""}, "flags.value-invalid"}, {[]string{"--renew"}, "args.unexpected"}} {
		i.w.last = &last
		object, err := i.rerun(refused.more...)
		if err != nil {
			return err
		}
		p := checkProblems(object)
		if i.w.exit != 80 || len(p) != 1 || p[0]["rule"] != refused.rule {
			return fmt.Errorf("given %q, not %s, exit 80\n%s", refused.more, refused.rule, i.w.report())
		}
		if err := i.notWritten(); err != nil {
			return err
		}
	}
	// The But runs the When's command again, not the last refusal's.
	i.w.last = &last
	return nil
}

func (i *identityException) writesWithReason() error {
	if _, err := i.rerun("--reason", identityReason); err != nil {
		return err
	}
	if i.w.exit != 0 {
		return fmt.Errorf("given a reason, exit %d\n%s", i.w.exit, i.w.report())
	}
	id, err := i.identity()
	if err != nil {
		return err
	}
	_, err = i.entry(identityUnit.FindStringSubmatch(id)[3])
	return err
}

func (i *identityException) killedMessage() error {
	message, _ := i.w.problem["message"].(string)
	if !strings.Contains(message, identityFile+":5:11") || !strings.Contains(message, "recorded killed") {
		return fmt.Errorf("the message %q does not say the mutant at board/board.go:5:11 is recorded killed", message)
	}
	return nil
}

func (i *identityException) stderrNames(id string) error {
	if err := i.w.d.Counted(); err != nil {
		return err
	}
	for _, line := range outputLines(i.w.stderr) {
		if strings.Contains(line, identityFile+":5:11") && strings.Contains(line, "`<` → `<=`") && strings.Contains(line, id) {
			return nil
		}
	}
	return fmt.Errorf("no line of standard error names the survivor at board/board.go:5:11 with its identity %q\n%s", id, i.w.report())
}

func (i *identityException) survivedFix(text string) error {
	object, err := i.rerun()
	if err != nil {
		return err
	}
	for _, p := range checkProblems(object) {
		file, _ := p["file"].(string)
		if p["rule"] == "mutation.survived" && filepath.ToSlash(file) == identityFile && p["line"] == float64(5) {
			if fix, _ := p["fix"].(string); !strings.Contains(fix, text) {
				return fmt.Errorf("the fix %q does not name %q", fix, text)
			}
			return nil
		}
	}
	return fmt.Errorf("no mutation.survived problem at board/board.go:5\n%s", i.w.report())
}
