package features

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/cucumber/godog"
)

// pruneSteps are slice-21's: mutation except --prune and what it leaves of
// itos-cc.yaml. e.written is the text mutation except wrote when the
// fixture excepted its survivor.
func (e *exceptionMutation) pruneSteps(sc *godog.ScenarioContext) {
	sc.Step(`^"itos-cc mutation run src/board\.ts" has run since$`, func() error { return e.run("mutation run --json src/board.ts") })
	sc.Step(`^itos-cc\.yaml holds no entry for "Board#count", and no other byte of it changed$`, func() error {
		return e.pruneLeft(e.written, "src/board.ts", "board.Board#count")
	})
	sc.Step(`^the removal is reported with the entry's file, function, column, original, replacement, reason and why "(gone|killed)"$`, func(why string) error {
		return e.pruneReported(map[string]any{"file": "src/board.ts", "function": "board.Board#count", "column": float64(19),
			"original": "<", "replacement": "<=", "reason": e.reason, "why": why})
	})
	sc.Step(`^"itos-cc (mutation check(?: src/board\.ts)? --json)" reports no "mutation\.exception-stale" problem$`, e.pruneNoStale)
	sc.Step(`^before it, "itos-cc mutation check src/board\.ts --json" gave the entry a fix naming "itos-cc mutation except --prune"$`, e.pruneFixBefore)
	sc.Step(`^itos-cc\.yaml holds no entry for src/port/porttest/fake\.go, still excludes "\*\*/porttest/\*\*", and no other byte of it changed$`, func() error {
		fake := "src/port/porttest/fake.go"
		return e.pruneLeft("exclude: ["+strconv.Quote("**/porttest/**")+"]\n"+fmt.Sprintf(fakeException, fake), fake, "")
	})
	sc.Step(`^the removal is reported with why "excluded"$`, func() error {
		return e.pruneReported(map[string]any{"file": "src/port/porttest/fake.go", "why": "excluded"})
	})
	sc.Step(`^it reports nothing pruned, and the exit code is 0$`, e.pruneNothing)
	sc.Step(`^itos-cc\.yaml is byte for byte as it was$`, func() error {
		if after := e.s.m.read("itos-cc.yaml"); after != e.written {
			return fmt.Errorf("itos-cc.yaml changed:\n%s\nwas:\n%s", after, e.written)
		}
		return nil
	})
	sc.Step(`^"itos-cc mutation except --prune --reason x" is "flags\.conflict", with flag "--reason", exit 80$`, func() error {
		return e.pruneRefused("mutation except --prune --reason x --json", map[string]any{"rule": "flags.conflict", "flag": "--reason"})
	})
	sc.Step(`^"itos-cc mutation except --prune --renew" is "flags\.conflict", exit 80$`, func() error {
		return e.pruneRefused("mutation except --prune --renew --json", map[string]any{"rule": "flags.conflict"})
	})
}

// pruneLines is text less the lines of its entry for file, and function
// when it is given: from the entry's dash to the line before the next
// line as little indented, or the end.
func pruneLines(text, file, function string) (string, error) {
	lines := strings.SplitAfter(text, "\n")
	for i, line := range lines {
		dash := strings.Index(line, "- file: "+file+"\n")
		if dash < 0 || strings.TrimSpace(line[:dash]) != "" {
			continue
		}
		end := i + 1
		for end < len(lines) && len(lines[end])-len(strings.TrimLeft(lines[end], " ")) > dash {
			end++
		}
		if function != "" && !strings.Contains(strings.Join(lines[i:end], ""), "function: "+function+"\n") {
			continue
		}
		return strings.Join(lines[:i], "") + strings.Join(lines[end:], ""), nil
	}
	return "", fmt.Errorf("no entry for %s %s in:\n%s", file, function, text)
}

func (e *exceptionMutation) pruneLeft(before, file, function string) error {
	if before == "" {
		return fmt.Errorf("no itos-cc.yaml was written before")
	}
	want, err := pruneLines(before, file, function)
	if err != nil {
		return err
	}
	if after := e.s.m.read("itos-cc.yaml"); after != want {
		return fmt.Errorf("itos-cc.yaml is:\n%s\nwant:\n%s\n%s", after, want, e.s.m.w.report())
	}
	return nil
}

// pruned is the "pruned" list of the last --json object.
func (e *exceptionMutation) pruned() ([]map[string]any, error) {
	a, err := e.object()
	if err != nil {
		return nil, err
	}
	raw, ok := a["pruned"].([]any)
	if !ok {
		return nil, fmt.Errorf("no pruned list: %v\n%s", a, e.s.m.w.report())
	}
	var out []map[string]any
	for _, v := range raw {
		p, ok := v.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("pruned entry %v", v)
		}
		out = append(out, p)
	}
	return out, nil
}

func (e *exceptionMutation) pruneReported(want map[string]any) error {
	pruned, err := e.pruned()
	if err != nil {
		return err
	}
	if len(pruned) != 1 {
		return fmt.Errorf("pruned %v, want one entry", pruned)
	}
	return exceptionFields(pruned[0], want)
}

func (e *exceptionMutation) pruneNothing() error {
	pruned, err := e.pruned()
	if err != nil {
		return err
	}
	if len(pruned) != 0 || e.s.m.w.exit != 0 {
		return fmt.Errorf("pruned %v, exit %d, want none, exit 0\n%s", pruned, e.s.m.w.exit, e.s.m.w.report())
	}
	return nil
}

func (e *exceptionMutation) pruneNoStale(args string) error {
	if err := e.run(args); err != nil {
		return err
	}
	a, err := e.object()
	if err != nil {
		return err
	}
	for _, p := range checkProblems(a) {
		if p["rule"] == "mutation.exception-stale" {
			return fmt.Errorf("still stale: %v", p)
		}
	}
	return nil
}

// pruneFixBefore puts back itos-cc.yaml as it was before --prune, checks
// the stale entry's fix there, and leaves the file as --prune wrote it.
func (e *exceptionMutation) pruneFixBefore() error {
	after := e.s.m.read("itos-cc.yaml")
	if err := e.s.m.w.write("itos-cc.yaml", e.written); err != nil {
		return err
	}
	err := e.run("mutation check src/board.ts --json")
	var p map[string]any
	if err == nil {
		p, err = e.stale(map[string]any{"function": "board.Board#count"})
	}
	if werr := e.s.m.w.write("itos-cc.yaml", after); werr != nil {
		return werr
	}
	if err != nil {
		return err
	}
	if fix, _ := p["fix"].(string); !strings.Contains(fix, "itos-cc mutation except --prune") {
		return fmt.Errorf("the fix %q does not name --prune", fix)
	}
	return nil
}

func (e *exceptionMutation) pruneRefused(args string, want map[string]any) error {
	if err := e.run(args); err != nil {
		return err
	}
	a, err := e.object()
	if err != nil {
		return err
	}
	p := checkProblems(a)
	if e.s.m.w.exit != 80 || len(p) != 1 {
		return fmt.Errorf("%s: exit %d, problems %v, want one, exit 80", args, e.s.m.w.exit, p)
	}
	return exceptionFields(p[0], want)
}
