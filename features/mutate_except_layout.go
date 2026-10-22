package features

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/cucumber/godog"
	"go.yaml.in/yaml/v3"
)

// layoutSteps are slice-20's: an itos-cc.yaml laid out by hand, and the
// bytes mutation except and --renew keep of it. layoutPrefix and
// layoutSuffix are the text before and after the lines the command may
// change.
func (e *exceptionMutation) layoutSteps(sc *godog.ScenarioContext) {
	step := func(text string, fn any) { sc.Step("^"+regexp.QuoteMeta(text)+"$", fn) }
	step("itos-cc.yaml is laid out by hand: a \"---\" line, blank lines between keys, `other: { each: \"{id}\", sep: \"|\" }`, a block four spaces deep, a comment two spaces after a value, and an exceptions list at its key's indentation with one entry for another function, whose reason is folded", e.layoutNew)
	step("itos-cc.yaml is the text it was, with the new entry's lines after the last entry and no other byte changed", e.layoutAppended)
	step(`itos-cc.yaml is laid out by hand, its entry for src/board.ts:7:19 with reason "first" between two entries for other functions, a comment above it`, e.layoutBetween)
	step(`only the lines of that entry changed in itos-cc.yaml, its reason now "reworded" and the comment above it kept`, e.layoutReplaced)
	step("itos-cc.yaml is laid out by hand around that entry: a \"---\" line, blank lines between keys, `other: { each: \"{id}\", sep: \"|\" }`, and a comment two spaces after a value", e.layoutRenew)
	step(`only the lines of the renewed entry changed in itos-cc.yaml`, e.layoutRenewed)
}

// layoutHash is the recorded hash of Board's function name.
func (e *exceptionMutation) layoutHash(name string) (string, error) {
	snap, err := e.s.snapshot("src/board.ts")
	if err != nil {
		return "", err
	}
	u := e.s.units(snap, "Board#"+name)
	if len(u) != 1 {
		return "", fmt.Errorf("no recorded unit %s: %v", name, u)
	}
	return fmt.Sprint(u[0]["hash"]), nil
}

func (e *exceptionMutation) layoutNew() error {
	hash, err := e.layoutHash("k1")
	if err != nil {
		return err
	}
	text := "---\n# Settings laid out by hand.\n\nexclude:\n    - \"**/gen/**\"\n\nother: { each: \"{id}\", sep: \"|\" }\nlimit: 30  # two spaces before this comment\n\n" +
		"mutation:\n  exceptions:\n  - file: src/board.ts\n    function: board.Board#k1\n    hash: " + hash + "\n    line_in_function: 1\n    column: 17\n" +
		"    original: '>'\n    replacement: '>='\n    reason: >-\n      a folded\n      reason\n"
	e.layoutPrefix, e.layoutSuffix = text, ""
	return e.s.m.w.write("itos-cc.yaml", text)
}

// layoutMiddle is the text of itos-cc.yaml between layoutPrefix and
// layoutSuffix, which must both be there byte for byte.
func (e *exceptionMutation) layoutMiddle() (string, error) {
	after := e.s.m.read("itos-cc.yaml")
	if !strings.HasPrefix(after, e.layoutPrefix) || !strings.HasSuffix(after, e.layoutSuffix) || len(after) < len(e.layoutPrefix)+len(e.layoutSuffix) {
		return "", fmt.Errorf("itos-cc.yaml changed outside the entry's lines:\n%s\nwant it to begin with:\n%s\nand end with:\n%s", after, e.layoutPrefix, e.layoutSuffix)
	}
	return after[len(e.layoutPrefix) : len(after)-len(e.layoutSuffix)], nil
}

// layoutEntry is the one block entry middle holds, its dash indent spaces
// in, its fields checked against want.
func layoutEntry(middle string, indent int, want map[string]any) (map[string]any, error) {
	if !strings.HasPrefix(middle, strings.Repeat(" ", indent)+"- ") || strings.Count(middle, "\n") < 2 {
		return nil, fmt.Errorf("entry %q is not a block entry with its dash %d spaces in", middle, indent)
	}
	var entries []map[string]any
	if err := yaml.Unmarshal([]byte(middle), &entries); err != nil {
		return nil, fmt.Errorf("entry %q: %w", middle, err)
	}
	if len(entries) != 1 {
		return nil, fmt.Errorf("entry lines %q hold %d entries", middle, len(entries))
	}
	return entries[0], exceptionFields(entries[0], want)
}

func (e *exceptionMutation) layoutAppended() error {
	middle, err := e.layoutMiddle()
	if err != nil {
		return err
	}
	hash, err := e.layoutHash("count")
	if err != nil {
		return err
	}
	_, err = layoutEntry(middle, 2, map[string]any{"file": "src/board.ts", "function": "board.Board#count", "hash": hash, "line_in_function": 1, "column": 19,
		"original": "<", "replacement": "<=", "reason": e.reason, "line_text": "count(x){ return x<5; }"})
	return err
}

func (e *exceptionMutation) layoutBetween() error {
	if err := e.review("first"); err != nil {
		return err
	}
	k1, err := e.layoutHash("k1")
	if err != nil {
		return err
	}
	k2, err := e.layoutHash("k2")
	if err != nil {
		return err
	}
	e.layoutPrefix = "# Settings laid out by hand.\n\nother: { each: \"{id}\", sep: \"|\" }\n\nmutation:\n    exceptions:\n" +
		"      - file: src/board.ts\n        function: board.Board#k1\n        hash: " + k1 + "\n        line_in_function: 1\n        column: 17\n" +
		"        original: '>'\n        replacement: '>='\n        reason: kept as written   # three spaces\n\n" +
		"      # The count loop only counts up.\n"
	entry := fmt.Sprintf("      - file: %v\n        function: %v\n        hash: %v\n        line_in_function: %v\n        column: %v\n"+
		"        original: \"%v\"\n        replacement: \"%v\"\n        reason: first\n        line_text: \"%v\"\n",
		e.entry["file"], e.entry["function"], e.entry["hash"], e.entry["line_in_function"], e.entry["column"], e.entry["original"], e.entry["replacement"], e.entry["line_text"])
	e.layoutSuffix = "      - {file: src/board.ts, function: board.Board#k2, hash: " + k2 + ", line_in_function: 1, column: 17, original: '>', replacement: '>=', reason: a flow entry}\n" +
		"# the end\n"
	return e.s.m.w.write("itos-cc.yaml", e.layoutPrefix+entry+e.layoutSuffix)
}

func (e *exceptionMutation) layoutReplaced() error {
	middle, err := e.layoutMiddle()
	if err != nil {
		return err
	}
	want := map[string]any{}
	for key, v := range e.entry {
		want[key] = v
	}
	want["reason"] = "reworded"
	_, err = layoutEntry(middle, 6, want)
	return err
}

// layoutRenew lays out the entries mutation except wrote by hand around
// them: a "---" line and keys before them, and a blank line between each.
func (e *exceptionMutation) layoutRenew() error {
	written := e.s.m.read("itos-cc.yaml")
	if !strings.HasPrefix(written, "mutation:\n  exceptions:\n    - file: label.go\n") {
		return fmt.Errorf("unexpected entries as written:\n%s", written)
	}
	text := "---\n# Settings laid out by hand.\n\nother: { each: \"{id}\", sep: \"|\" }\nlimit: 30  # two spaces before this comment\n\n" +
		strings.ReplaceAll(written, "\n    - file:", "\n\n    - file:")
	lines := strings.SplitAfter(text, "\n")
	start := -1
	for i, line := range lines {
		if line == "    - file: label.go\n" {
			start = i
		}
	}
	end := start + 1
	for end < len(lines) && lines[end] != "\n" {
		end++
	}
	e.layoutPrefix, e.layoutSuffix = strings.Join(lines[:start], ""), strings.Join(lines[end:], "")
	return e.s.m.w.write("itos-cc.yaml", text)
}

func (e *exceptionMutation) layoutRenewed() error {
	middle, err := e.layoutMiddle()
	if err != nil {
		return err
	}
	old := e.renewBefore[renewModule+"#Label"]
	want := map[string]any{}
	for key, v := range old {
		if key != "hash" {
			want[key] = v
		}
	}
	entry, err := layoutEntry(middle, 4, want)
	if err != nil {
		return err
	}
	if entry["hash"] == old["hash"] {
		return fmt.Errorf("the entry was not renewed: %v", entry)
	}
	return nil
}
