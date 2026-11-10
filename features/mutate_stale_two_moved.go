package features

import (
	"context"
	"fmt"
	"strings"

	"github.com/cucumber/godog"
)

// The steps of slice-80 (idea stale-identity-two-moved): two stale entries of
// one change in one function, neither at its site's place once a line was
// added above both, are each re-pointed by the identity its problem names.
// They need no toolchain and no run: check and an identity's except run
// nothing. The hashes were measured with itos-cc from the sources below.

const (
	// twoMovedSource is board/board.go: Count's two sites of `<` → `<=`, on
	// lines 5 and 6, column 9, line_in_function 2 and 3, its hash
	// twoMovedHash.
	twoMovedSource = "package board\n\n// Count says whether x is under five and y under seven.\nfunc Count(x, y int) bool {\n\ta := x < 5\n\tb := y < 7\n\treturn a && b\n}\n"
	twoMovedHash   = "acff77bccbb468d0"
	// twoMovedCurrent is Count's hash once "_ = x" is above both lines.
	twoMovedCurrent = "41d3c5f118875691"
	// twoMovedSecondReason is the second entry's reason; the first's is
	// identityReason.
	twoMovedSecondReason = "y is never 7"
)

// twoMoved is the state of the steps: the identities the two problems name,
// by the line of their entry's place.
type twoMoved struct {
	s             *staleIdentity
	first, second string
}

func (s *staleIdentity) twoMovedSteps(sc *godog.ScenarioContext) {
	t := &twoMoved{s: s}
	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		*t = twoMoved{s: s}
		return ctx, nil
	})
	sc.Step(`^Go source whose "example\.com/fx/board#Count" has the lines "a := x < 5" and "b := y < 7"$`, t.source)
	sc.Step("^itos-cc\\.yaml excepts each line's `<` → `<=`, with its line_text, the first with reason \"x is never 5\" and the second with reason \"y is never 7\"$", t.entries)
	sc.Step(`^the line "_ = x" was added above both$`, func() error {
		return s.b.i.edit(func(text string) string { return strings.Replace(text, "\ta := x < 5", "\t_ = x\n\ta := x < 5", 1) })
	})
	sc.Step(`^each entry's "mutation\.exception-stale" problem, with why "changed", has an "identity"$`, t.identities)
	sc.Step(`^excepting the first's identity with no --reason exits 0, its entry at Count's current hash and line_in_function 3 with the reason "x is never 5", the second entry as it was$`, t.exceptFirst)
	sc.Step(`^excepting the second's identity with no --reason then exits 0, its entry at line_in_function 4 with the reason "y is never 7"$`, t.exceptSecond)
}

// source writes the module and Count's two sites, in a repository.
func (t *twoMoved) source() error {
	w := t.s.b.i.w
	for name, text := range map[string]string{"go.mod": identityModule, identityFile: twoMovedSource} {
		if err := w.write(name, text); err != nil {
			return err
		}
	}
	return w.git("init", "-q")
}

// twoMovedEntry is the entry of Count's site at line_in_function line, holding for
// hash, with reason and the site's line text.
func twoMovedEntry(hash string, line int, reason, text string) map[string]any {
	return map[string]any{"file": identityFile, "function": "example.com/fx/board#Count", "hash": hash, "line_in_function": line,
		"column": 9, "original": "<", "replacement": "<=", "reason": reason, "line_text": text}
}

// entries writes the two entries, and checks that both hold before the
// source changes.
func (t *twoMoved) entries() error {
	w := t.s.b.i.w
	var b strings.Builder
	b.WriteString("mutation:\n  exceptions:\n")
	for _, e := range []struct {
		line         int
		reason, text string
	}{{2, identityReason, "a := x < 5"}, {3, twoMovedSecondReason, "b := y < 7"}} {
		fmt.Fprintf(&b, "    - file: %s\n      function: example.com/fx/board#Count\n      hash: %s\n      line_in_function: %d\n      column: 9\n      original: <\n      replacement: <=\n      reason: %s\n      line_text: %s\n",
			identityFile, twoMovedHash, e.line, e.reason, e.text)
	}
	if err := w.write("itos-cc.yaml", b.String()); err != nil {
		return err
	}
	return t.s.noneStale(identityFile)
}

// identities keeps the identity of each entry's stale problem: the first's
// at line 5, its old place, the second's at line 6.
func (t *twoMoved) identities() error {
	w := t.s.b.i.w
	object, err := w.json()
	if err != nil {
		return err
	}
	byLine := map[float64]string{}
	for _, p := range checkProblems(object) {
		if p["rule"] != "mutation.exception-stale" {
			continue
		}
		identity, _ := p["identity"].(string)
		line, _ := p["line"].(float64)
		if p["why"] != "changed" || identity == "" || byLine[line] != "" {
			return fmt.Errorf("the problem %v is not one entry's, with why changed and an \"identity\"\n%s", p, w.report())
		}
		byLine[line] = identity
	}
	t.first, t.second = byLine[5], byLine[6]
	if len(byLine) != 2 || t.first == "" || t.second == "" {
		return fmt.Errorf("the stale problems' identities by line are %v, not one each at lines 5 and 6\n%s", byLine, w.report())
	}
	return nil
}

// except runs mutation except identity with no --reason, which must exit 0,
// and checks the two entries itos-cc.yaml then holds.
func (t *twoMoved) except(identity string, first, second map[string]any) error {
	i := t.s.b.i
	w := i.w
	if err := w.runEnv(w.dir, w.env(), "mutation", "except", identity); err != nil {
		return err
	}
	if w.exit != 0 {
		return fmt.Errorf("excepting %s exited %d\n%s", identity, w.exit, w.report())
	}
	entries, err := i.entries()
	if err != nil {
		return err
	}
	if len(entries) != 2 {
		return fmt.Errorf("itos-cc.yaml holds %d entries, not two: %v", len(entries), entries)
	}
	if err := exceptionFields(entries[0], first); err != nil {
		return fmt.Errorf("the first entry: %w", err)
	}
	if err := exceptionFields(entries[1], second); err != nil {
		return fmt.Errorf("the second entry: %w", err)
	}
	return nil
}

func (t *twoMoved) exceptFirst() error {
	return t.except(t.first, twoMovedEntry(twoMovedCurrent, 3, identityReason, "a := x < 5"), twoMovedEntry(twoMovedHash, 3, twoMovedSecondReason, "b := y < 7"))
}

func (t *twoMoved) exceptSecond() error {
	return t.except(t.second, twoMovedEntry(twoMovedCurrent, 3, identityReason, "a := x < 5"), twoMovedEntry(twoMovedCurrent, 4, twoMovedSecondReason, "b := y < 7"))
}
