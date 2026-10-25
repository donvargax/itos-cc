package features

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
)

// identityFlipping is board/board_test.go deciding Count's mutant, the
// one Count(5) tells from the source, by the control file at @CONTROL@,
// outside the module: each run of the mutant takes the next of its "flip"
// outcomes, cycled, "killed" failing the test, and counts its runs there,
// so nothing the test hashes changes. Unmutated, it passes as
// identityTest does.
const identityFlipping = `package board

import (
	"encoding/json"
	"os"
	"testing"
)

func TestCount(t *testing.T) {
	if !Count(4) || Count(6) {
		t.Fatal("Count")
	}
	if !Count(5) {
		return
	}
	data, err := os.ReadFile(@CONTROL@)
	if err != nil {
		t.Fatal(err)
	}
	var control struct {
		Flip []string ` + "`json:\"flip\"`" + `
		Runs int      ` + "`json:\"runs\"`" + `
	}
	if err := json.Unmarshal(data, &control); err != nil {
		t.Fatal(err)
	}
	outcome := control.Flip[control.Runs%len(control.Flip)]
	control.Runs++
	if data, err = json.Marshal(control); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(@CONTROL@, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if outcome == "killed" {
		t.Fatal("Count(5)")
	}
}
`

// flipping commits board_test.go deciding Count's mutant by outcomes, in
// turn, run after run, and declares the same of its tests, for a counted
// run to judge.
func (i *identityException) flipping(outcomes ...string) error {
	w := i.w
	if err := w.d.Counted(); err != nil {
		return err
	}
	var err error
	if i.external, err = w.d.Folder(); err != nil {
		return err
	}
	control := filepath.Join(i.external, "control.json")
	data, err := json.Marshal(map[string]any{"flip": outcomes})
	if err != nil {
		return err
	}
	if err := w.d.Write(control, data); err != nil {
		return err
	}
	if err := w.write("board/board_test.go", strings.ReplaceAll(identityFlipping, "@CONTROL@", strconv.Quote(filepath.ToSlash(control)))); err != nil {
		return err
	}
	if err := w.git("add", "board/board_test.go"); err != nil {
		return err
	}
	if err := w.git("commit", "-qm", "flip Count's mutant"); err != nil {
		return err
	}
	site := Site{File: identityFile, Line: 5, Original: "<", Replacement: "<="}
	return w.d.Tests(w.dir, Tests{Language: "go", Command: identityCommand, Flipping: map[Site][]string{site: outcomes}})
}

// countedFlaky is whether the last run, a counted run, reports one
// problem, a mutation.flaky of Count's mutant at board/board.go:5:11, exit
// 1, with outcomes, its runs' in run order, and a fix in its tests.
func (i *identityException) countedFlaky(outcomes ...string) error {
	w := i.w
	obj, report, err := w.lastObject()
	if err != nil {
		return err
	}
	problems := checkProblems(obj)
	if len(problems) != 1 {
		return fmt.Errorf("counted problems %v, want one mutation.flaky\n%s", problems, report)
	}
	mu := problems[0]
	if mu["rule"] != "mutation.flaky" || mu["exit"] != float64(1) || filepath.ToSlash(fmt.Sprint(mu["file"])) != identityFile || mu["line"] != float64(5) || mu["column"] != float64(11) ||
		mu["original"] != "<" || mu["replacement"] != "<=" || !strings.Contains(fmt.Sprint(mu["fix"]), "deterministic") {
		return fmt.Errorf("unexpected flaky problem %v\n%s", mu, report)
	}
	if got := fmt.Sprint(mu["outcomes"]); got != fmt.Sprint(outcomes) {
		return fmt.Errorf("flaky outcomes %s, want %v\n%s", got, outcomes, report)
	}
	if w.exit != 1 {
		return fmt.Errorf("counted run exited %d, want 1\n%s", w.exit, report)
	}
	return nil
}
