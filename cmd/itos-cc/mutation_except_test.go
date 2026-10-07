package main

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

// The scenarios of "Rule: Equivalent mutants excepted, each with a reason"
// in features/mutate.feature. They use the module of mutate_since_test.go:
// Board#place's one mutant is killed by TestPlace, and Board#clear's one
// mutant, `<` → `<=` at src/board.go:11:11, survives whenever it runs, since
// no test calls clear. It stands for the equivalent mutant. The scenarios
// about mutation run and mutation check write their entry into itos-cc.yaml
// by hand, as a person reviewing it could, so each fails at its own step
// until the behaviour is there.

// exceptionYAML is one entry under mutation.exceptions in itos-cc.yaml.
type exceptionYAML struct {
	File           string `yaml:"file"`
	Function       string `yaml:"function"`
	Hash           string `yaml:"hash"`
	LineInFunction int    `yaml:"line_in_function"`
	Column         int    `yaml:"column"`
	Original       string `yaml:"original"`
	Replacement    string `yaml:"replacement"`
	Reason         string `yaml:"reason,omitempty"`
}

// The site of clear's mutant: its line counted from clear's first line, 10.
const clearSite, clearLineInFunction = "11:11", 2

// mutationExcept runs itos-cc mutation except with args.
func mutationExcept(t *testing.T, args ...string) outcome {
	t.Helper()
	return cli(t, append([]string{"mutation", "except"}, args...)...)
}

// clearException is the entry that excepts clear's survivor with reason,
// with clear's hash as the snapshot of src/board.go records it.
func clearException(t *testing.T, reason string) exceptionYAML {
	t.Helper()
	u, ok := boardUnits(t)[clearID]
	if !ok {
		t.Fatalf("the snapshot of %s records no %s", boardSource, clearID)
	}
	return exceptionYAML{File: "src/board.go", Function: clearID, Hash: u.Hash, LineInFunction: clearLineInFunction,
		Column: clearColumn, Original: "<", Replacement: "<=", Reason: reason}
}

// writeExceptions writes itos-cc.yaml: head, then entries under
// mutation.exceptions.
func writeExceptions(t *testing.T, head string, entries ...exceptionYAML) {
	t.Helper()
	var file struct {
		Mutation struct {
			Exceptions []exceptionYAML `yaml:"exceptions"`
		} `yaml:"mutation"`
	}
	file.Mutation.Exceptions = entries
	data, err := yaml.Marshal(file)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, "itos-cc.yaml", head+string(data))
}

// readExceptions is the entries under mutation.exceptions in itos-cc.yaml,
// which must be there.
func readExceptions(t *testing.T) []exceptionYAML {
	t.Helper()
	data, err := os.ReadFile("itos-cc.yaml")
	if err != nil {
		t.Fatalf("no itos-cc.yaml: %v", err)
	}
	var file struct {
		Mutation struct {
			Exceptions []exceptionYAML `yaml:"exceptions"`
		} `yaml:"mutation"`
	}
	if err := yaml.Unmarshal(data, &file); err != nil {
		t.Fatalf("itos-cc.yaml: %v\n%s", err, data)
	}
	return file.Mutation.Exceptions
}

// survivorRun runs mutation run over src/board.go, which records clear's
// survivor.
func survivorRun(t *testing.T) {
	t.Helper()
	if o := mutateRun(t, boardSource); o.code != 1 {
		t.Fatalf("the first run: exit %d, want 1 for clear's survivor\n%s%s", o.code, o.stdout, o.stderr)
	}
}

// mutantOf is the one mutant of function a --json run lists in src/board.go.
func mutantOf(t *testing.T, o outcome, function string) mutantJSON {
	t.Helper()
	var found []mutantJSON
	for _, m := range o.json(t).file(t, boardSource).mutants(t) {
		if m.Function == function {
			found = append(found, m)
		}
	}
	if len(found) != 1 {
		t.Fatalf("mutants of %s: %+v, want one\n%s", function, found, o.stdout)
	}
	return found[0]
}

// noItosCcYAML fails t when itos-cc.yaml was written.
func noItosCcYAML(t *testing.T) {
	t.Helper()
	if data, err := os.ReadFile("itos-cc.yaml"); err == nil {
		t.Errorf("itos-cc.yaml was written:\n%s", data)
	}
}

// @ID-MUT-85
func TestExceptingASurvivorRecordsItWithItsReason(t *testing.T) {
	boardRepo(t, nil)
	survivorRun(t)

	o := mutationExcept(t, boardSource+":"+clearSite, "--reason", "clear is never called")
	want := []exceptionYAML{clearException(t, "clear is never called")}
	if o.code != 0 {
		t.Fatalf("exit %d, want 0\n%s%s", o.code, o.stdout, o.stderr)
	}
	if got := readExceptions(t); !slices.Equal(got, want) {
		t.Errorf("mutation.exceptions:\n%+v\nwant:\n%+v", got, want)
	}
}

// @ID-MUT-86
func TestOnlyARecordedSurvivorCanBeExcepted(t *testing.T) {
	boardRepo(t, nil)
	// With coverage, place's mutant is killed and clear's is uncovered.
	if o := mutateCovered(t, boardSource); o.code != 0 {
		t.Fatalf("the run: exit %d, want 0\n%s%s", o.code, o.stdout, o.stderr)
	}
	if u := boardUnits(t); u[placeID].Killed != 1 || u[clearID].Uncovered != 1 {
		t.Fatalf("the snapshot: %+v, want place's mutant killed and clear's uncovered", u)
	}

	// Killed, uncovered, and nothing there: line 3 is the type Board.
	for _, site := range []struct{ line, column int }{{7, 11}, {clearLine, clearColumn}, {3, 1}} {
		arg := fmt.Sprintf("%s:%d:%d", boardSource, site.line, site.column)
		o := mutationExcept(t, "--json", arg, "--reason", "x")
		wantProblem(t, o.json(t).problem("exception.no-survivor"),
			map[string]any{"file": boardSource, "line": float64(site.line), "column": float64(site.column)}, o.stdout)
		if o.code != 2 {
			t.Errorf("%s: exit %d, want 2", arg, o.code)
		}
	}
	noItosCcYAML(t)
}

// @ID-MUT-87
func TestAReasonIsRequired(t *testing.T) {
	boardRepo(t, nil)
	survivorRun(t)

	o := mutationExcept(t, "--json", boardSource+":"+clearSite)
	wantProblem(t, o.json(t).problem("flags.value-missing"), map[string]any{"flag": "--reason"}, o.stdout)
	if o.code != 2 {
		t.Errorf("no --reason: exit %d, want 2", o.code)
	}
	o = mutationExcept(t, "--json", boardSource+":"+clearSite, "--reason", "")
	wantProblem(t, o.json(t).problem("flags.value-invalid"), map[string]any{"flag": "--reason", "value": ""}, o.stdout)
	if o.code != 2 {
		t.Errorf("--reason '': exit %d, want 2", o.code)
	}
	noItosCcYAML(t)
}

// @ID-MUT-88
func TestTheRestOfItosCcYamlIsKept(t *testing.T) {
	boardRepo(t, nil)
	survivorRun(t)
	const comment = "# Equivalent mutants, each reviewed like code.\n"
	writeExceptions(t, comment+"other:\n  kept: true\n", clearException(t, "first"))

	o := mutationExcept(t, boardSource+":"+clearSite, "--reason", "reworded")
	if o.code != 0 {
		t.Fatalf("exit %d, want 0\n%s%s", o.code, o.stdout, o.stderr)
	}
	want := []exceptionYAML{clearException(t, "reworded")}
	if got := readExceptions(t); !slices.Equal(got, want) {
		t.Errorf("mutation.exceptions:\n%+v\nwant the one entry, reworded:\n%+v", got, want)
	}
	data, _ := os.ReadFile("itos-cc.yaml")
	if !strings.HasPrefix(string(data), comment) {
		t.Errorf("itos-cc.yaml:\n%s\nwant it to start with the comment %q", data, comment)
	}
	var file struct {
		Other map[string]bool `yaml:"other"`
	}
	if err := yaml.Unmarshal(data, &file); err != nil || !file.Other["kept"] || len(file.Other) != 1 {
		t.Errorf("other: %v (%v), want {kept: true}\n%s", file.Other, err, data)
	}
}

// @ID-MUT-89
func TestAnExceptedSurvivorFailsNothing(t *testing.T) {
	boardRepo(t, nil)
	survivorRun(t)
	writeExceptions(t, "", clearException(t, "clear is never called"))
	// With no snapshot, every mutant runs.
	if err := os.RemoveAll(filepath.Join(".metrics", "mutate")); err != nil {
		t.Fatal(err)
	}

	o := mutateRun(t, boardSource)
	if want := boardSource + ": 1 killed, 0 survived, 1 excepted, 0 uncovered (ran 2, reused 0)\n"; !strings.Contains(o.stdout, want) {
		t.Errorf("stdout:\n%s\nwant it to say %q", o.stdout, want)
	}
	if strings.Contains(o.stdout, "survived "+boardSource) {
		t.Errorf("stdout:\n%s\nwant clear's mutant not listed as survived", o.stdout)
	}
	if o.code != 0 {
		t.Errorf("exit %d, want 0\n%s%s", o.code, o.stdout, o.stderr)
	}
}

// @ID-MUT-90
func TestAnExceptedSurvivorIsReusedWhileNothingChanged(t *testing.T) {
	boardRepo(t, nil)
	survivorRun(t)
	writeExceptions(t, "", clearException(t, "clear is never called"))

	o := mutateRun(t, "--json", boardSource)
	if m := mutantOf(t, o, clearID); !m.Reused || m.Excepted != "clear is never called" {
		t.Errorf("clear's mutant %+v, want it reused and excepted", m)
	}
	if n := mutantsRun(o.stderr); n != 0 {
		t.Errorf("%d mutants ran, want none: place's kill and clear's excepted survivor are reused\n%s", n, o.stderr)
	}
	if f := o.json(t).file(t, boardSource); f.Excepted == nil || *f.Excepted != 1 || f.Survived != 0 {
		t.Errorf("file %+v, want clear's mutant counted excepted, not survived", f)
	}
	if o.code != 0 {
		t.Errorf("exit %d, want 0", o.code)
	}

	o = mutateRun(t, "--json", "--mutate-all", boardSource)
	if m := mutantOf(t, o, clearID); m.Reused || m.Excepted != "clear is never called" {
		t.Errorf("with --mutate-all, clear's mutant %+v, want it run, and excepted", m)
	}
	if n := mutantsRun(o.stderr); n != 2 {
		t.Errorf("with --mutate-all, %d mutants ran, want 2\n%s", n, o.stderr)
	}
}

// @ID-MUT-91
func TestAfterItsTestsChangeAnExceptedMutantThatStillSurvivesStaysExcepted(t *testing.T) {
	boardRepo(t, nil)
	survivorRun(t)
	writeExceptions(t, "", clearException(t, "clear is never called"))
	changeBoardTest(t)

	o := mutateRun(t, "--json", boardSource)
	if m := mutantOf(t, o, clearID); m.Reused || m.Outcome != "survived" || m.Excepted != "clear is never called" {
		t.Errorf("clear's mutant %+v, want it run again, survived, and excepted", m)
	}
	if f := o.json(t).file(t, boardSource); f.Excepted == nil || *f.Excepted != 1 || f.Survived != 0 {
		t.Errorf("file %+v, want clear's mutant counted excepted", f)
	}
	if o.code != 0 {
		t.Errorf("exit %d, want 0\n%s", o.code, o.stdout)
	}
}

// @ID-MUT-92
func TestAnExceptedMutantThatIsNowKilledMakesItsEntryStale(t *testing.T) {
	boardRepo(t, nil)
	survivorRun(t)
	writeExceptions(t, "", clearException(t, "clear is never called"))
	// TestClear now kills it.
	writeFile(t, filepath.FromSlash("src/board_test.go"), killedTests["src/board_test.go"])

	o := mutateRun(t, "--json", boardSource)
	wantProblem(t, o.json(t).problem("mutation.exception-stale"), map[string]any{"file": boardSource, "function": clearID,
		"line": float64(clearLine), "column": float64(clearColumn), "original": "<", "replacement": "<=", "why": "killed"}, o.stdout)
	if o.code != 1 {
		t.Errorf("exit %d, want 1", o.code)
	}
}

// @ID-MUT-93
func TestAnEntryWhoseFunctionChangedIsStale(t *testing.T) {
	boardRepo(t, nil)
	survivorRun(t)
	writeExceptions(t, "", clearException(t, "clear is never called"))
	// A move is not a change.
	moveDown(t)
	if o := mutateRun(t, "--json", boardSource); o.code != 0 {
		t.Fatalf("after a move: exit %d, want 0: the entry still holds\n%s", o.code, o.stdout)
	}

	edit(t, boardSource, "i < 3", "i < 4")
	o := mutateRun(t, "--json", boardSource)
	m := o.json(t)
	wantProblem(t, m.problem("mutation.exception-stale"), map[string]any{"file": boardSource, "function": clearID, "why": "changed"}, o.stdout)
	// Its mutant is judged as if it had no entry.
	wantProblem(t, m.problem("mutation.survived"), map[string]any{"function": clearID, "line": float64(clearLine + 1),
		"column": float64(clearColumn)}, o.stdout)
	if o.code != 1 {
		t.Errorf("exit %d, want 1", o.code)
	}
}

// @ID-MUT-94
func TestAnEntryWhoseSiteIsGoneIsStale(t *testing.T) {
	boardRepo(t, nil)
	survivorRun(t)
	writeExceptions(t, "", clearException(t, "clear is never called"))

	// clear no longer has the site: `==` is no `<`.
	edit(t, boardSource, "i < 3", "i == 3")
	o := mutateRun(t, "--json", boardSource)
	wantProblem(t, o.json(t).problem("mutation.exception-stale"), map[string]any{"file": boardSource, "function": clearID, "why": "gone"}, o.stdout)
	if o.code != 1 {
		t.Errorf("no such site: exit %d, want 1", o.code)
	}

	// clear is deleted.
	edit(t, boardSource, "\nfunc (b *Board) clear(i int) bool {\n\treturn i == 3\n}\n", "")
	o = mutateRun(t, "--json", boardSource)
	wantProblem(t, o.json(t).problem("mutation.exception-stale"), map[string]any{"file": boardSource, "function": clearID, "why": "gone"}, o.stdout)
	if o.code != 1 {
		t.Errorf("no such function: exit %d, want 1", o.code)
	}
}

// @ID-MUT-95
func TestAnEntryDoesNotExcuseAnUncoveredMutant(t *testing.T) {
	boardRepo(t, nil)
	// With coverage, clear's mutant is uncovered.
	if o := mutateCovered(t, boardSource); o.code != 0 {
		t.Fatalf("the run: exit %d, want 0\n%s%s", o.code, o.stdout, o.stderr)
	}
	writeExceptions(t, "", clearException(t, "clear is never called"))

	o := mutateCovered(t, "--fail-uncovered", "--json", boardSource)
	m := o.json(t)
	wantProblem(t, m.problem("mutation.uncovered"), map[string]any{"file": boardSource, "function": clearID,
		"line": float64(clearLine), "column": float64(clearColumn)}, o.stdout)
	if !slices.Equal(m.rules(), []string{"mutation.uncovered"}) {
		t.Errorf("problems %v, want clear's uncovered mutant alone", m.Problems)
	}
	if f := m.file(t, boardSource); f.Excepted == nil || *f.Excepted != 0 || f.Uncovered != 1 {
		t.Errorf("file %+v, want clear's mutant counted uncovered, and none excepted", f)
	}
	if mu := mutantOf(t, o, clearID); mu.Outcome != "uncovered" || mu.Excepted != "" {
		t.Errorf("clear's mutant %+v, want it uncovered and not excepted", mu)
	}
	if o.code != 1 {
		t.Errorf("exit %d, want 1", o.code)
	}
}

// @ID-MUT-96
func TestMutationCheckPassesAnExceptedSurvivorAndFailsAStaleEntry(t *testing.T) {
	boardRepo(t, nil)
	survivorRun(t)
	writeExceptions(t, "", clearException(t, "clear is never called"))

	if o := mutationCheck(t, boardSource); o.code != 0 {
		t.Errorf("exit %d, want 0: clear's survivor is excepted\n%s%s", o.code, o.stdout, o.stderr)
	}

	edit(t, boardSource, "i < 3", "i < 4")
	o := mutationCheck(t, "--json", boardSource)
	wantProblem(t, o.json(t).problem("mutation.exception-stale"), map[string]any{"file": boardSource, "function": clearID, "why": "changed"}, o.stdout)
	if o.code != 1 {
		t.Errorf("after clear changed: exit %d, want 1", o.code)
	}
}

// @ID-MUT-97
func TestWithSinceOnlyTheJudgedFunctionsEntriesAreChecked(t *testing.T) {
	boardRepo(t, nil)
	survivorRun(t)
	// Written for another version of clear, the entry is stale.
	stale := clearException(t, "clear is never called")
	stale.Hash = "0000000000000000"
	writeExceptions(t, "", stale)
	changePlace(t)

	o := mutateRun(t, "--json", "--since", "base")
	if p := o.json(t).problem("mutation.exception-stale"); p != nil || o.code != 0 {
		t.Errorf("mutation run --since: exit %d, problem %v, want exit 0 and none: clear is not judged", o.code, p)
	}
	o = mutationCheck(t, "--json", "--since", "base")
	if p := o.json(t).problem("mutation.exception-stale"); p != nil || o.code != 0 {
		t.Errorf("mutation check --since: exit %d, problem %v, want exit 0 and none: clear is not checked", o.code, p)
	}

	// Without --since, clear is checked, and its entry is stale.
	o = mutationCheck(t, "--json")
	wantProblem(t, o.json(t).problem("mutation.exception-stale"), map[string]any{"function": clearID, "why": "changed"}, o.stdout)
}

// @ID-MUT-98
func TestAnInvalidItosCcYamlIsAConfigError(t *testing.T) {
	boardRepo(t, nil)
	writeExceptions(t, "", exceptionYAML{File: "src/board.go", Function: clearID, Hash: "0000000000000000",
		LineInFunction: clearLineInFunction, Column: clearColumn, Original: "<", Replacement: "<="})

	o := mutateRun(t, "--json", boardSource)
	wantProblem(t, o.json(t).problem("config.invalid"), map[string]any{"file": "itos-cc.yaml"}, o.stdout)
	if o.code != 2 {
		t.Errorf("an entry with no reason: exit %d, want 2", o.code)
	}
	if n := mutantsRun(o.stderr); n != 0 {
		t.Errorf("%d mutants ran, want none\n%s", n, o.stderr)
	}

	// Nor is a file that is no YAML read, by mutation check either.
	writeFile(t, "itos-cc.yaml", "mutation: [\n")
	o = mutationCheck(t, "--json", boardSource)
	wantProblem(t, o.json(t).problem("config.invalid"), map[string]any{"file": "itos-cc.yaml"}, o.stdout)
	if o.code != 2 {
		t.Errorf("no YAML: exit %d, want 2", o.code)
	}
}

// @ID-MUT-99
func TestExceptedMutantsAsJSON(t *testing.T) {
	boardRepo(t, nil)
	survivorRun(t)
	writeExceptions(t, "", clearException(t, "clear is never called"))

	o := mutateRun(t, "--json", boardSource)
	m := o.json(t)
	if f := m.file(t, boardSource); f.Excepted == nil || *f.Excepted != 1 || f.Killed != 1 || f.Survived != 0 || f.Uncovered != 0 {
		t.Errorf("file %+v, want 1 killed, 0 survived, 1 excepted, and 0 uncovered", f)
	}
	if mu := mutantOf(t, o, clearID); mu.Outcome != "survived" || mu.Excepted != "clear is never called" {
		t.Errorf("clear's mutant %+v, want outcome survived and excepted, the entry's reason", mu)
	}
	for _, f := range o.rawFiles(t) {
		for _, mu := range f["mutants"].([]any) {
			if mu := mu.(map[string]any); mu["function"] == placeID {
				if _, ok := mu["excepted"]; ok {
					t.Errorf("place's mutant %v has \"excepted\", want it only on an excepted mutant", mu)
				}
			}
		}
	}
	if !m.OK || o.code != 0 {
		t.Errorf("ok %v, exit %d, want ok and exit 0", m.OK, o.code)
	}
}
