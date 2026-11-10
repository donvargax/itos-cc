package features

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/cucumber/godog"
)

// The steps of slice-78 (issue #86): a stale entry's problem names the
// identity that re-points it with no mutation run. They reuse slice-41's
// module and slice-76's signature change, carry the hashes measured by hand
// with sha256sum, and read only what itos-cc prints and writes, decoding its
// --json, so a problem's raw text may hold < or <.

const (
	// staleMovedHash is Count's hash once "_ = x" is above its "return x < 5",
	// and staleMovedIdentity its mutant's identity then: at line_in_function 3.
	staleMovedHash     = "edded670ff042bc8"
	staleMovedIdentity = "board/board.go:example.com/fx/board#Count@4:7:" + staleMovedHash + ":42:<><="
	// staleTwice is Count with two sites its entry's line text could be, none
	// at its place.
	staleTwice = "package board\n\n// Count says whether x is under five.\nfunc Count(x int) bool {\n\tif x == 0 {\n\t\treturn x < 5\n\t}\n\treturn x < 5\n}\n"
	// stalePlainStart is what the plain line of the stale entry says before
	// its why.
	stalePlainStart = "  exception-stale `<` → `<=` in example.com/fx/board#Count, board/board.go line 5 column 11 "
)

// staleLanguage is a language's fixture for ID-MUT-421: a source whose
// Board#count holds one site, `<` → `<=`, on its second line, and its text and
// hash before and after a line below the site changes.
type staleLanguage struct {
	file, source, changed, hash, current string
	column                               int
	lineText                             string
}

var staleLanguages = map[string]staleLanguage{
	"TypeScript": {
		file:     "src/board.ts",
		source:   "export class Board {\n  count(x) {\n    const under = x < 5;\n    return under;\n  }\n}\n",
		changed:  "export class Board {\n  count(x) {\n    const under = x < 5;\n    return !!under;\n  }\n}\n",
		hash:     "843e3a83ab72972c",
		current:  "ccd4c2a52c429f49",
		column:   21,
		lineText: "const under = x < 5;",
	},
	"Python": {
		file:     "src/board.py",
		source:   "class Board:\n    def count(self, x):\n        under = x < 5\n        return under\n",
		changed:  "class Board:\n    def count(self, x):\n        under = x < 5\n        return bool(under)\n",
		hash:     "af47a4ebbc8b6eac",
		current:  "8c0483aa26dcdf06",
		column:   19,
		lineText: "under = x < 5",
	},
}

// staleIdentity is the state of the steps: the language of ID-MUT-421.
type staleIdentity struct {
	b        *byHash
	language staleLanguage
}

func (b *byHash) staleIdentitySteps(sc *godog.ScenarioContext) {
	s := &staleIdentity{b: b}
	i := b.i
	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		*s = staleIdentity{b: b}
		return ctx, nil
	})
	sc.Step(`^a commit made Count's signature "func Count\(x int\) \(ok bool\)"$`, s.committedSignature)
	sc.Step(`^its "identity" is that mutant's as the code is now, with Count's current hash$`, func() error { return s.identityIs(byHashIdentity) })
	sc.Step(`^its "identity" is that mutant's as the code is now$`, func() error { return s.identityIs(byHashIdentity) })
	sc.Step(`^its fix names "itos-cc mutation except '<its identity>'", saying that this runs nothing and keeps the entry's reason, and to read the function first$`, s.fixNames)
	sc.Step(`^its fix still names "itos-cc mutation except --renew" after that$`, s.renewAfter)
	sc.Step(`^running the named command, with no --reason and no Go toolchain on the PATH, exits 0 with the entry at Count's current hash and the reason "x is never 5"$`, s.runNamed)
	sc.Step(`^the line "_ = x" was added above Count's "return x < 5"$`, func() error {
		return i.edit(func(text string) string { return strings.Replace(text, "\treturn x < 5", "\t_ = x\n\treturn x < 5", 1) })
	})
	sc.Step(`^the "mutation\.exception-stale" problem's "identity" names that mutant at line_in_function 3, as the code is now$`, func() error { return s.identityIs(staleMovedIdentity) })
	sc.Step(`^excepting that identity with no --reason records line_in_function 3 and the reason "x is never 5"$`, s.exceptMoved)
	sc.Step(`^Count became "if x == 0 \{ return x < 5 \}" over three lines, then "return x < 5"$`, func() error { return i.w.write(identityFile, staleTwice) })
	sc.Step(`^Count's "x < 5" became "x == 5"$`, func() error {
		return i.edit(func(text string) string { return strings.Replace(text, "x < 5", "x == 5", 1) })
	})
	sc.Step(`^it has no "identity" key$`, s.noIdentity)
	sc.Step(`^its fix does not name an identity$`, s.fixNamesNoIdentity)
	sc.Step("^its standard output has the line \""+regexp.QuoteMeta(stalePlainStart)+"\\(changed\\), identity <that mutant's identity>\"$", func() error {
		return s.plainLine(stalePlainStart + "(changed), identity " + byHashIdentity)
	})
	sc.Step(`^when the entry's line changed too, the line ends "\(changed\)"$`, s.plainEnds)
	sc.Step(`^its fix names "itos-cc mutation except '<its identity>'" ahead of what it says today$`, s.fixAhead)
	sc.Step(`^I run "itos-cc mutation check board\.go --json" in the folder board$`, func() error {
		return i.w.runEnv(filepath.Join(i.w.dir, "board"), i.w.env(), "mutation", "check", "board.go", "--json")
	})
	sc.Step(`^the "mutation\.exception-stale" problem's "file" is "board\.go"$`, s.fileIsFromHere)
	sc.Step(`^its "identity" starts "board/board\.go:example\.com/fx/board#Count@"$`, s.identityFromRoot)
	sc.Step(`^excepting that identity from the folder board exits 0, keeping the reason "x is never 5"$`, s.exceptFromFolder)
	sc.Step("^(TypeScript|Python) source whose Board#count has the `<` → `<=` site itos-cc\\.yaml excepts with reason \"x is never 5\"$", s.languageSource)
	sc.Step(`^Board#count changed in a line after the site, leaving the site's line as it was$`, s.languageChanged)
	sc.Step(`^the "mutation\.exception-stale" problem, with why "changed", has an "identity"$`, s.languageIdentity)
	sc.Step(`^excepting that identity with no --reason exits 0, the entry at Board#count's current hash with the reason "x is never 5"$`, s.exceptLanguage)
	sc.Step(`^"itos-cc mutation check (\S+) --json" (?:then )?reports no "mutation\.exception-stale"$`, s.noneStale)
	s.twoMovedSteps(sc)
}

// committedSignature is slice-76's signature change, committed, as a counted
// run judges HEAD.
func (s *staleIdentity) committedSignature() error {
	w := s.b.i.w
	if err := s.b.signature(); err != nil {
		return err
	}
	if err := w.git("add", identityFile); err != nil {
		return err
	}
	return w.git("commit", "-qm", "name Count's result")
}

// problem is the mutation.exception-stale problem of the last run, read from
// its --json object, made again with --json when the run had none.
func (s *staleIdentity) problem() (map[string]any, error) {
	object, report, err := s.b.i.w.lastObject()
	if err != nil {
		return nil, err
	}
	for _, p := range checkProblems(object) {
		if p["rule"] == "mutation.exception-stale" {
			return p, nil
		}
	}
	return nil, fmt.Errorf("no mutation.exception-stale problem\n%s", report)
}

// named is the stale problem's identity, its fix and the command that takes
// it, which the fix must name.
func (s *staleIdentity) named() (identity, fix, command string, err error) {
	p, err := s.problem()
	if err != nil {
		return "", "", "", err
	}
	identity, _ = p["identity"].(string)
	fix, _ = p["fix"].(string)
	if identity == "" {
		return "", "", "", fmt.Errorf("the problem %v has no \"identity\"", p)
	}
	return identity, fix, "itos-cc mutation except '" + identity + "'", nil
}

func (s *staleIdentity) identityIs(want string) error {
	got, _, _, err := s.named()
	if err != nil {
		return err
	}
	if got != want {
		return fmt.Errorf("its \"identity\" is %q, not %q", got, want)
	}
	return nil
}

func (s *staleIdentity) fixNames() error {
	_, fix, command, err := s.named()
	if err != nil {
		return err
	}
	for _, text := range []string{command, "runs nothing", "keeps the entry's reason", "ead the function first"} {
		if !strings.Contains(fix, text) {
			return fmt.Errorf("the fix %q does not say %q", fix, text)
		}
	}
	return nil
}

func (s *staleIdentity) renewAfter() error {
	_, fix, command, err := s.named()
	if err != nil {
		return err
	}
	at := strings.Index(fix, command)
	if at < 0 || !strings.Contains(fix[at+len(command):], "itos-cc mutation except --renew") {
		return fmt.Errorf("the fix %q does not name \"itos-cc mutation except --renew\" after %q", fix, command)
	}
	return nil
}

// runNamed runs the command the fix names as it names it, with no Go on the
// PATH.
func (s *staleIdentity) runNamed() error {
	i := s.b.i
	w := i.w
	identity, _, _, err := s.named()
	if err != nil {
		return err
	}
	w.without = append(slices.Clone(w.without), "go")
	if err := w.runEnv(w.dir, w.env(), "mutation", "except", identity); err != nil {
		return err
	}
	if w.exit != 0 {
		return fmt.Errorf("the named command exited %d\n%s", w.exit, w.report())
	}
	_, err = i.entry(byHashCurrent)
	return err
}

func (s *staleIdentity) exceptMoved() error {
	i := s.b.i
	w := i.w
	identity, _, _, err := s.named()
	if err != nil {
		return err
	}
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
	if len(entries) != 1 {
		return fmt.Errorf("itos-cc.yaml holds %d entries, not one: %v", len(entries), entries)
	}
	return exceptionFields(entries[0], map[string]any{"hash": staleMovedHash, "line_in_function": 3, "column": 11, "reason": identityReason, "line_text": "return x < 5"})
}

func (s *staleIdentity) noIdentity() error {
	p, err := s.problem()
	if err != nil {
		return err
	}
	if _, has := p["identity"]; has {
		return fmt.Errorf("the problem %v has an \"identity\"", p)
	}
	return nil
}

func (s *staleIdentity) fixNamesNoIdentity() error {
	p, err := s.problem()
	if err != nil {
		return err
	}
	fix, _ := p["fix"].(string)
	if identityUnit.MatchString(fix) || strings.Contains(fix, "'<identity>'") {
		return fmt.Errorf("the fix %q names an identity", fix)
	}
	return nil
}

// plainLine is whether the last run's standard output has the line want.
func (s *staleIdentity) plainLine(want string) error {
	w := s.b.i.w
	if !slices.Contains(strings.Split(strings.ReplaceAll(w.stdout, "\r\n", "\n"), "\n"), want) {
		return fmt.Errorf("standard output has no line %q\n%s", want, w.report())
	}
	return nil
}

func (s *staleIdentity) plainEnds() error {
	i := s.b.i
	if err := i.edit(func(text string) string { return strings.Replace(text, "x < 5", "x < 6", 1) }); err != nil {
		return err
	}
	if err := i.w.runWith(i.w.env(), "mutation check "+identityFile); err != nil {
		return err
	}
	return s.plainLine(stalePlainStart + "(changed)")
}

func (s *staleIdentity) fixAhead() error {
	_, fix, command, err := s.named()
	if err != nil {
		return err
	}
	at := strings.Index(fix, command)
	if at < 0 || !strings.Contains(fix[at+len(command):], "itos-cc mutation run") {
		return fmt.Errorf("the fix %q does not name %q ahead of \"itos-cc mutation run\"", fix, command)
	}
	return nil
}

func (s *staleIdentity) fileIsFromHere() error {
	p, err := s.problem()
	if err != nil {
		return err
	}
	if file, _ := p["file"].(string); filepath.ToSlash(file) != "board.go" {
		return fmt.Errorf("the problem's \"file\" is %q, not board.go", file)
	}
	return nil
}

func (s *staleIdentity) identityFromRoot() error {
	identity, _, _, err := s.named()
	if err != nil {
		return err
	}
	if !strings.HasPrefix(identity, "board/board.go:example.com/fx/board#Count@") {
		return fmt.Errorf("the identity %q is not from the project root", identity)
	}
	return nil
}

func (s *staleIdentity) exceptFromFolder() error {
	i := s.b.i
	w := i.w
	identity, _, _, err := s.named()
	if err != nil {
		return err
	}
	if err := w.runEnv(filepath.Join(w.dir, "board"), w.env(), "mutation", "except", identity); err != nil {
		return err
	}
	if w.exit != 0 {
		return fmt.Errorf("excepting from the folder board exited %d\n%s", w.exit, w.report())
	}
	_, err = i.entry(byHashCurrent)
	return err
}

// languageSource writes the language's source and the entry for its site,
// and checks that the entry holds before the source changes.
func (s *staleIdentity) languageSource(language string) error {
	s.language = staleLanguages[language]
	l := s.language
	w := s.b.i.w
	if err := w.write(l.file, l.source); err != nil {
		return err
	}
	if err := w.git("init", "-q"); err != nil {
		return err
	}
	entry := fmt.Sprintf("mutation:\n  exceptions:\n    - file: %s\n      function: board.Board#count\n      hash: %s\n      line_in_function: 2\n      column: %d\n      original: <\n      replacement: <=\n      reason: %s\n      line_text: %s\n",
		l.file, l.hash, l.column, identityReason, l.lineText)
	if err := w.write("itos-cc.yaml", entry); err != nil {
		return err
	}
	return s.noneStale(l.file)
}

func (s *staleIdentity) languageChanged() error {
	return s.b.i.w.write(s.language.file, s.language.changed)
}

func (s *staleIdentity) languageIdentity() error {
	p, err := s.problem()
	if err != nil {
		return err
	}
	if identity, _ := p["identity"].(string); identity == "" || p["why"] != "changed" {
		return fmt.Errorf("the problem %v has no \"identity\", with why changed", p)
	}
	return nil
}

func (s *staleIdentity) exceptLanguage() error {
	w := s.b.i.w
	identity, _, _, err := s.named()
	if err != nil {
		return err
	}
	if err := w.runEnv(w.dir, w.env(), "mutation", "except", identity); err != nil {
		return err
	}
	if w.exit != 0 {
		return fmt.Errorf("excepting %s exited %d\n%s", identity, w.exit, w.report())
	}
	entries, err := s.b.i.entries()
	if err != nil {
		return err
	}
	if len(entries) != 1 {
		return fmt.Errorf("itos-cc.yaml holds %d entries, not one: %v", len(entries), entries)
	}
	return exceptionFields(entries[0], map[string]any{"file": s.language.file, "hash": s.language.current, "reason": identityReason})
}

// noneStale runs mutation check of file with --json and is whether it reports
// no stale entry, the toolchain back on the PATH.
func (s *staleIdentity) noneStale(file string) error {
	w := s.b.i.w
	w.without = nil
	if err := w.runWith(w.env(), "mutation check "+file+" --json"); err != nil {
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
