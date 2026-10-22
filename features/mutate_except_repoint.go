package features

import (
	"fmt"
	"regexp"

	"github.com/cucumber/godog"
)

// repointLine is Board#count once it declares n before its comparison: the
// comparison's `<` moves from column 19 to column 30 of line 7.
const repointLine = "count(x){ const n=x; return n<5; }"

// repointSteps are slice-22's: excepting the same change in the same
// function re-points its stale entry, and run, check and --renew give it
// the same why.
func (e *exceptionMutation) repointSteps(sc *godog.ScenarioContext) {
	step := func(text string, fn any) { sc.Step("^"+regexp.QuoteMeta(text)+"$", fn) }
	step("\"Board#count\" became \""+repointLine+"\", and \"itos-cc mutation run src/board.ts\" recorded its `<` → `<=` at src/board.ts:7:30 surviving", e.repointMoved)
	step(`it names "Board#count"'s current hash, line_in_function 1, column 30 and its line text, and keeps the reason "the loop only counts up"`, func() error {
		return e.repointEntry(e.reason)
	})
	step(`the JSON says "replaced": true`, func() error {
		a, err := e.object()
		if err != nil {
			return err
		}
		if a["replaced"] != true {
			return fmt.Errorf("replaced is %v: %v", a["replaced"], a)
		}
		return nil
	})
	step(`given "--reason 'reworded'", the entry is re-pointed the same way, its reason "reworded"`, func() error {
		if err := e.s.m.w.write("itos-cc.yaml", e.written); err != nil {
			return err
		}
		if err := e.run("mutation except src/board.ts:7:30 --reason 'reworded' --json"); err != nil {
			return err
		}
		if e.s.m.w.exit != 0 {
			return fmt.Errorf("re-pointing with a reason: exit %d\n%s", e.s.m.w.exit, e.s.m.w.report())
		}
		if err := e.oneEntry(); err != nil {
			return err
		}
		return e.repointEntry("reworded")
	})
	step(`"itos-cc mutation run src/board.ts --json" and "itos-cc mutation except --renew --json" name it "changed" too`, func() error {
		return e.repointWhy("changed", false)
	})
	step("once \"Board#count\" has no `<` left and mutation run has judged it, all three name it \"gone\"", func() error {
		if err := e.s.replace("src/board.ts", repointLine, "count(x){ const n=x; return !(n>=5); }"); err != nil {
			return err
		}
		if err := e.declare("src/board.ts", true); err != nil {
			return err
		}
		return e.repointWhy("gone", true)
	})
}

func (e *exceptionMutation) repointMoved() error {
	if err := e.s.replace("src/board.ts", "count(x){ return x<5; }", repointLine); err != nil {
		return err
	}
	if err := e.declare("src/board.ts", true); err != nil {
		return err
	}
	if err := e.run("mutation run --json src/board.ts"); err != nil {
		return err
	}
	mutants, err := e.r.entries()
	if err != nil {
		return err
	}
	for _, m := range mutants {
		if exceptionFields(m, map[string]any{"line": float64(7), "column": float64(30), "original": "<", "replacement": "<=", "outcome": "survived"}) == nil {
			return nil
		}
	}
	return fmt.Errorf("no survivor at src/board.ts:7:30: %v", mutants)
}

func (e *exceptionMutation) repointEntry(reason string) error {
	if err := e.oneEntry(); err != nil {
		return err
	}
	a, _ := e.entries()
	hash, err := e.layoutHash("count")
	if err != nil {
		return err
	}
	if hash == e.entry["hash"] {
		return fmt.Errorf("the function's hash did not change")
	}
	return exceptionFields(a[0], map[string]any{"file": "src/board.ts", "function": e.entry["function"], "hash": hash, "line_in_function": 1, "column": 30,
		"original": "<", "replacement": "<=", "line_text": repointLine, "reason": reason})
}

// repointWhy is whether check, run and --renew all call the entry why;
// with all, the check is run too, else it is the last command's.
func (e *exceptionMutation) repointWhy(why string, all bool) error {
	commands := []string{"mutation run src/board.ts --json"}
	if all {
		commands = append([]string{"mutation check src/board.ts --json"}, commands...)
	}
	for _, command := range commands {
		if err := e.run(command); err != nil {
			return err
		}
		if _, err := e.stale(map[string]any{"function": e.entry["function"], "why": why}); err != nil {
			return fmt.Errorf("%s: %w", command, err)
		}
	}
	if err := e.run("mutation except --renew --json"); err != nil {
		return err
	}
	a, err := e.object()
	if err != nil {
		return err
	}
	left, _ := a["not_renewed"].([]any)
	if len(left) != 1 {
		return fmt.Errorf("--renew left %v, want the entry\n%s", left, e.s.m.w.report())
	}
	entry, _ := left[0].(map[string]any)
	return exceptionFields(entry, map[string]any{"function": e.entry["function"], "why": why})
}
