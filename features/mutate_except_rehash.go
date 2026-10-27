package features

import (
	"fmt"
	"strings"

	"github.com/cucumber/godog"
)

// The steps of the Rule "An exception written under itos-cc's previous
// hash is rehashed without a run" (slice-68, issue #65). They reuse
// slice-41's module, example.com/fx, with one comment line in Count's body
// above its "return x < 5", so its one site, `<` → `<=`, is at
// board/board.go:6:11, and slice-63's Over below it, whose `>` → `>=` at
// 11:11 its test survives too. Count's hash before 0.18.0, which kept
// comments in, is rehashOld, of "func Count(x int) bool {\n\t// below five
// counts\n\treturn x < 5\n}"; its hash now, without the comment, is
// rehashNew, slice-41's. Over holds no comment, so its hash, rehashOver, is
// the same under both. The steps carry these literals, measured by hand
// with sha256sum, and read only what itos-cc prints and writes.

const (
	rehashOld  = "af13bea3308d9ebf"
	rehashNew  = "b1a8f41702cd975f"
	rehashOver = "fb46828f16ec8698"
	// rehashSource is board/board.go: Count's comment line, then Over.
	rehashSource = "package board\n\n// Count says whether x is under five.\nfunc Count(x int) bool {\n\t// below five counts\n\treturn x < 5\n}\n" + siteOver
	// rehashCount is Count's entry, under its hash before 0.18.0, written
	// before entries had line_text, with a comment between its keys and an
	// x- key, which only a rewrite of the whole entry would lose.
	rehashCount = "  - file: board/board.go\n" +
		"    function: example.com/fx/board#Count\n" +
		"    hash: " + rehashOld + "\n" +
		"    # the place within Count\n" +
		"    line_in_function: 3\n" +
		"    column: 11\n" +
		"    original: <\n" +
		"    replacement: <=\n" +
		"    reason: x is never 5\n" +
		"    x-ticket: QA-12\n"
	// rehashOverEntry is Over's entry, which holds.
	rehashOverEntry = "  - file: board/board.go\n" +
		"    function: example.com/fx/board#Over\n" +
		"    hash: " + rehashOver + "\n" +
		"    line_in_function: 2\n" +
		"    column: 11\n" +
		"    original: '>'\n" +
		"    replacement: '>='\n" +
		"    reason: x is never 10\n" +
		"    line_text: return x > 10\n"
)

var (
	rehashCountSite = Site{File: identityFile, Line: 6, Original: "<", Replacement: "<="}
	rehashOverSite  = Site{File: identityFile, Line: 11, Original: ">", Replacement: ">="}
	rehashProfile   = "mode: set\nexample.com/fx/board/board.go:6.2,7.1 1 1\nexample.com/fx/board/board.go:11.2,12.1 1 1\n"
)

func (i *identityException) rehashSteps(sc *godog.ScenarioContext) {
	sc.Step("^a Go module whose \"example\\.com/fx/board#Count\" holds \"// below five counts\" above its \"return x < 5\", so its `<` → `<=` is at board/board\\.go:6:11$", i.rehashModule)
	sc.Step(`^itos-cc\.yaml excepts that mutant under Count's hash before 0\.18\.0, "af13bea3308d9ebf", with line_in_function 3 and "x-ticket: QA-12"$`, func() error {
		return i.rehashConfig(rehashCount)
	})
	sc.Step(`^it also excepts a survivor of "example\.com/fx/board#Over", which holds$`, func() error {
		return i.rehashConfig(rehashCount + rehashOverEntry)
	})
	sc.Step(`^the module and the entry of ID-MUT-377$`, func() error {
		if err := i.rehashModule(); err != nil {
			return err
		}
		return i.rehashConfig(rehashCount + rehashOverEntry)
	})
	sc.Step(`^the entry's hash is "([0-9a-f]+)", and no other line of itos-cc\.yaml changed$`, i.rehashedEntry)
	sc.Step(`^"rehashed" lists it with "old_hash" "([0-9a-f]+)", "new_hash" "([0-9a-f]+)", line (\d+) and column (\d+)$`, i.rehashedListed)
	sc.Step(`^"not_rehashed" is empty, and the exit code is 0$`, i.nothingLeft)
	sc.Step(`^without --json, on the file as it was, its standard output says "(.*)"$`, i.rehashPlain)
	sc.Step(`^"itos-cc mutation check board/board\.go --json" then reports no "mutation\.exception-stale" problem$`, i.rehashChecked)
	sc.Step(`^the problem is "mutation\.exception-stale", with function "([^"]*)" and why "([^"]*)", and its fix names "([^"]*)"$`, i.rehashStale)
	sc.Step(`^"not_rehashed" lists the entry with why "([^"]*)"$`, func(why string) error { return i.leftWith("not_rehashed", why) })
	sc.Step(`^"itos-cc mutation run board/board\.go" has recorded the mutant surviving$`, i.rehashRecorded)
	sc.Step(`^the "mutation\.exception-stale" problem, with why "changed", has a fix naming "itos-cc mutation except --rehash" before "--renew"$`, func() error {
		_, err := i.rehashFix("--renew")
		return err
	})
	sc.Step(`^its message says the entry was written under itos-cc's previous hash$`, i.previousHashMessage)
	sc.Step(`^"itos-cc mutation run board/board\.go --json" gives it the same fix$`, i.rehashSameFix)
	sc.Step(`^once Count's "x < 5" became "x < 6", its fix is ID-MUT-242's, naming no "--rehash"$`, i.renewFixOnce)
	sc.Step(`^"not_renewed" lists the entry with why "no-results"$`, func() error { return i.leftWith("not_renewed", "no-results") })
	sc.Step(`^its "mutation\.exception-stale" problem's fix names "itos-cc mutation except --rehash" before "mutation run"$`, func() error {
		_, err := i.rehashFix("mutation run")
		return err
	})
}

// rehashModule writes and commits the module, Count with its comment line
// and Over, and declares what its tests do and cover: both survive.
func (i *identityException) rehashModule() error {
	w := i.w
	for name, text := range map[string]string{"go.mod": identityModule, identityFile: rehashSource, "board/board_test.go": identityTest + siteOverTest} {
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
	if err := w.d.Tests(w.dir, Tests{Language: "go", Command: identityCommand, Surviving: []Site{rehashCountSite, rehashOverSite}}); err != nil {
		return err
	}
	return w.d.Coverage(w.dir, Coverage{Language: "go", Report: rehashProfile})
}

// rehashConfig writes itos-cc.yaml holding entries, and keeps it as the
// text a later step compares with.
func (i *identityException) rehashConfig(entries string) error {
	text := "version: 1\nmutation:\n  exceptions:\n" + entries
	if err := i.w.write("itos-cc.yaml", text); err != nil {
		return err
	}
	i.w.config = text
	return nil
}

// rehashedEntry is whether itos-cc.yaml is the text the Given wrote, with
// Count's hash line alone now naming hash.
func (i *identityException) rehashedEntry(hash string) error {
	w := i.w
	want := strings.Replace(w.config, "    hash: "+rehashOld+"\n", "    hash: "+hash+"\n", 1)
	after, err := w.readIn(w.dir, "itos-cc.yaml")
	if err != nil {
		return err
	}
	if string(after) != want {
		return fmt.Errorf("itos-cc.yaml is:\n%s\nwant:\n%s\n%s", after, want, w.report())
	}
	return nil
}

func (i *identityException) rehashedListed(oldHash, newHash string, line, column int) error {
	object, err := i.w.json()
	if err != nil {
		return err
	}
	rehashed := objectList(object, "rehashed")
	if len(rehashed) != 1 {
		return fmt.Errorf("rehashed %v, want Count's entry alone\n%s", rehashed, i.w.report())
	}
	return exceptionFields(rehashed[0], map[string]any{"file": identityFile, "function": "example.com/fx/board#Count",
		"old_hash": oldHash, "new_hash": newHash, "line": float64(line), "column": float64(column), "line_in_function": float64(3),
		"original": "<", "replacement": "<=", "reason": identityReason})
}

func (i *identityException) nothingLeft() error {
	object, err := i.w.json()
	if err != nil {
		return err
	}
	left, ok := object["not_rehashed"].([]any)
	if !ok || len(left) != 0 || i.w.exit != 0 {
		return fmt.Errorf("not_rehashed %v, exit %d, want an empty list, exit 0\n%s", object["not_rehashed"], i.w.exit, i.w.report())
	}
	return nil
}

// rehashPlain writes itos-cc.yaml back as the Given wrote it, runs the
// last run again without --json, and reads its standard output.
func (i *identityException) rehashPlain(text string) error {
	w := i.w
	if err := w.write("itos-cc.yaml", w.config); err != nil {
		return err
	}
	if err := w.runEnv(w.last.dir, w.last.env, "mutation", "except", "--rehash"); err != nil {
		return err
	}
	if w.exit != 0 || !strings.Contains(w.stdout, text) {
		return fmt.Errorf("standard output does not say %q, exit 0\n%s", text, w.report())
	}
	return nil
}

// rehashChecked runs mutation check with the Go toolchain on the PATH.
func (i *identityException) rehashChecked() error {
	w := i.w
	w.without = nil
	if err := w.runWith(w.env(), "mutation check "+identityFile+" --json"); err != nil {
		return err
	}
	object, err := w.json()
	if err != nil {
		return err
	}
	for _, p := range checkProblems(object) {
		if p["rule"] == "mutation.exception-stale" {
			return fmt.Errorf("still stale: %v\n%s", p, w.report())
		}
	}
	return nil
}

func (i *identityException) rehashStale(function, why, fix string) error {
	w := i.w
	if err := w.problemIs("mutation.exception-stale", "function", function); err != nil {
		return err
	}
	got, _ := w.problem["fix"].(string)
	if w.problem["why"] != why || !strings.Contains(got, fix) {
		return fmt.Errorf("the problem %v does not have why %q and a fix naming %q", w.problem, why, fix)
	}
	return nil
}

// leftWith is whether list, of the last --json object, holds Count's
// entry alone, with why.
func (i *identityException) leftWith(list, why string) error {
	object, err := i.w.json()
	if err != nil {
		return err
	}
	left := objectList(object, list)
	if len(left) != 1 {
		return fmt.Errorf("%s %v, want Count's entry alone\n%s", list, left, i.w.report())
	}
	return exceptionFields(left[0], map[string]any{"file": identityFile, "function": "example.com/fx/board#Count", "hash": rehashOld,
		"line_in_function": float64(3), "column": float64(11), "original": "<", "replacement": "<=", "reason": identityReason, "why": why})
}

// rehashRecorded runs a complete run of board/board.go, which fails on
// Count's stale entry and its survivor, and checks its snapshot records
// Count's mutant at 6:11 survived.
func (i *identityException) rehashRecorded() error {
	w := i.w
	if err := w.runWith(w.env(), "mutation run "+identityFile); err != nil {
		return err
	}
	if w.exit != 1 {
		return fmt.Errorf("itos-cc mutation run %s exited %d, not 1\n%s", identityFile, w.exit, w.report())
	}
	return i.snapshotRecordsAt(6, "survived")
}

// staleCount is the mutation.exception-stale problem of Count's entry, why
// changed, in the last --json object.
func (i *identityException) staleCount() (map[string]any, error) {
	object, err := i.w.json()
	if err != nil {
		return nil, err
	}
	for _, p := range checkProblems(object) {
		if p["rule"] == "mutation.exception-stale" && p["function"] == "example.com/fx/board#Count" && p["why"] == "changed" {
			return p, nil
		}
	}
	return nil, fmt.Errorf("no mutation.exception-stale problem of Count, why changed\n%s", i.w.report())
}

// rehashFix is the stale problem of Count's entry, whose fix names
// "itos-cc mutation except --rehash" before later.
func (i *identityException) rehashFix(later string) (map[string]any, error) {
	p, err := i.staleCount()
	if err != nil {
		p, err = i.noResults()
	}
	if err != nil {
		return nil, err
	}
	fix, _ := p["fix"].(string)
	at := strings.Index(fix, "itos-cc mutation except --rehash")
	if at < 0 || !strings.Contains(fix[at+len("itos-cc mutation except --rehash"):], later) {
		return nil, fmt.Errorf("the fix %q does not name \"itos-cc mutation except --rehash\" before %q", fix, later)
	}
	i.w.problem = p
	return p, nil
}

// noResults is the mutation.exception-stale problem of Count's entry, why
// no-results, in the last --json object.
func (i *identityException) noResults() (map[string]any, error) {
	object, err := i.w.json()
	if err != nil {
		return nil, err
	}
	for _, p := range checkProblems(object) {
		if p["rule"] == "mutation.exception-stale" && p["function"] == "example.com/fx/board#Count" && p["why"] == "no-results" {
			return p, nil
		}
	}
	return nil, fmt.Errorf("no mutation.exception-stale problem of Count, why changed or no-results\n%s", i.w.report())
}

func (i *identityException) previousHashMessage() error {
	message, _ := i.w.problem["message"].(string)
	if !strings.Contains(message, "written under itos-cc's previous hash") {
		return fmt.Errorf("the message %q does not say the entry was written under itos-cc's previous hash", message)
	}
	return nil
}

func (i *identityException) rehashSameFix() error {
	w := i.w
	old := w.problem["fix"]
	if err := w.runWith(w.env(), "mutation run "+identityFile+" --json"); err != nil {
		return err
	}
	p, err := i.staleCount()
	if err != nil {
		return err
	}
	if p["fix"] != old || w.exit != 1 {
		return fmt.Errorf("mutation run's fix %q, exit %d, want %q, exit 1", p["fix"], w.exit, old)
	}
	return nil
}

// renewFixOnce changes Count's "x < 5" to "x < 6" and runs mutation check
// again: the stale entry's fix names --renew before --reason, as ID-MUT-242
// says, and no --rehash.
func (i *identityException) renewFixOnce() error {
	w := i.w
	if err := i.countChanged(); err != nil {
		return err
	}
	if err := w.runWith(w.env(), "mutation check "+identityFile+" --json"); err != nil {
		return err
	}
	p, err := i.staleCount()
	if err != nil {
		return err
	}
	fix, _ := p["fix"].(string)
	at := strings.Index(fix, "itos-cc mutation except --renew")
	if at < 0 || strings.Index(fix, "--reason") <= at || strings.Contains(fix, "--rehash") {
		return fmt.Errorf("the fix %q does not name --renew before --reason, or names --rehash", fix)
	}
	return nil
}

// countChanged changes Count's "x < 5" to "x < 6" in board/board.go as it
// is.
func (i *identityException) countChanged() error {
	return i.edit(func(text string) string { return strings.Replace(text, "x < 5", "x < 6", 1) })
}

// edit writes board/board.go as change makes it from its text now.
func (i *identityException) edit(change func(string) string) error {
	data, err := i.w.readIn(i.w.dir, identityFile)
	if err != nil {
		return err
	}
	return i.w.write(identityFile, change(string(data)))
}
