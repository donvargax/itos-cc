// The steps of dry.feature: the duplicate functions itos-cc dry reports,
// judged from what it prints and the .metrics/dry.json it writes. What a
// scenario says of every pair is held against the pairs dry itself scores
// with nothing left out (--threshold 0 --min-lines 1 --min-nodes 1), and
// each such step first finds in the fixture the pairs that make its
// sentence worth checking, so a fixture that cannot tell right from wrong
// fails instead of passing. Like every step here, they read the binary's
// output alone, never its code.
package features

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/cucumber/godog"
)

// dryState is one dry.feature scenario's state beside the world's.
type dryState struct {
	w *world

	// file is the file the scenario's functions are written to, language
	// the language they are in and first the function a copy is made of.
	file, language, first string
	// score is the similarity the two functions were compared at, compared
	// whether they were.
	score    float64
	compared bool
	// files are the files the scenario's Given wrote, which its Then names
	// as "they".
	files []string
	// changed are the files the sample changed since its commit.
	changed []string

	// reportedRun is the run reportedPairs and reportedErr were read from:
	// the steps run dry again over the same folder, which writes
	// .metrics/dry.json again, so the scenario's own run is read once.
	reportedRun   *run
	reportedPairs []dryPair
	reportedErr   error
}

func drySteps(sc *godog.ScenarioContext, w *world) {
	d := &dryState{w: w}
	sc.Before(func(ctx context.Context, s *godog.Scenario) (context.Context, error) {
		*d = dryState{w: w}
		if filepath.Base(s.Uri) != "dry.feature" || len(s.Steps) == 0 || !strings.HasPrefix(s.Steps[0].Text, `I run "itos-cc`) {
			return ctx, nil
		}
		// The scenarios that only say When I run run in the sample.
		return ctx, d.layOutSample()
	})

	// Two functions compared directly.
	sc.Step(`^the (TypeScript|Go|Kotlin|Python) functions:$`, func(language string, text *godog.DocString) error {
		return d.writeFunctions(language, text.Content)
	})
	sc.Step(`^the (TypeScript|Go|Kotlin|Python) function "(.*)"$`, func(language, source string) error {
		d.first = source
		return d.writeFunctions(language, source)
	})
	sc.Step(`^a copy of it with (.+)$`, d.copyWith)
	sc.Step(`^the two functions are compared directly$`, d.compareDirectly)
	sc.Step(`^their similarity is (\d+\.\d\d)$`, func(want string) error {
		if err := d.mustHaveCompared(); err != nil {
			return err
		}
		if got := strconv.FormatFloat(d.score, 'f', 2, 64); got != want {
			return fmt.Errorf("their similarity is %s (%v), not %s\n%s", got, d.score, want, w.report())
		}
		return nil
	})
	sc.Step(`^their similarity is above (\d+(?:\.\d+)?) and below (\d+(?:\.\d+)?)$`, func(low, high float64) error {
		if err := d.mustHaveCompared(); err != nil {
			return err
		}
		if d.score <= low || d.score >= high {
			return fmt.Errorf("their similarity is %v, not above %v and below %v\n%s", d.score, low, high, w.report())
		}
		return nil
	})
	sc.Step(`^their similarity is at most (\d+(?:\.\d+)?)$`, func(most float64) error {
		if err := d.mustHaveCompared(); err != nil {
			return err
		}
		if d.score > most {
			return fmt.Errorf("their similarity is %v, above %v\n%s", d.score, most, w.report())
		}
		return nil
	})

	// The defaults and their tuning.
	sc.Step(`^pairs scoring at least (\d+(?:\.\d+)?) are reported$`, d.pairsScoringAtLeastAreReported)
	sc.Step(`^functions shorter than (\d+) lines or with fewer than (\d+) normalized nodes are not candidates$`, d.smallAreNotCandidates)
	sc.Step(`^only pairs scoring at least (\d+(?:\.\d+)?) among functions of (\d+)\+ lines and (\d+)\+ nodes are reported$`, d.onlyPairsReported)

	// Files of a project.
	sc.Step(`^invoice\.py renders an invoice's lines and total$`, func() error { return d.writeFile("invoice.py", invoicePy) })
	sc.Step(`^receipt\.py renders a receipt the same way, plus one footer line$`, func() error { return d.writeFile("receipt.py", receiptPy) })
	sc.Step(`^parse\.py holds an unrelated parser$`, func() error { return d.writeFile("parse.py", parsePy) })
	sc.Step(`^exactly one pair is reported: (\S+)#(\S+) and (\S+)#(\S+)$`, d.exactlyOnePair)
	sc.Step(`^its score is at least (\d+(?:\.\d+)?) but below (\d+(?:\.\d+)?)$`, d.itsScoreIs)
	sc.Step(`^a TypeScript function and a Python function with the same logic$`, func() error {
		return d.writeFiles(map[string]string{"src/total.ts": totalTs, "src/total.py": totalPy})
	})
	sc.Step(`^they are not compared$`, d.notCompared)
	sc.Step(`^two identical (\d+)-line functions$`, d.identicalFunctions)
	sc.Step(`^they are not reported$`, d.theyAreNotReported)
	sc.Step(`^src/new\.ts duplicates a function in src/old\.ts$`, func() error {
		return d.writeFiles(map[string]string{"src/old.ts": loadTs + "\n" + totalTs, "src/new.ts": strings.ReplaceAll(totalTs, "total(", "sum(")})
	})
	sc.Step(`^src/x\.ts and src/y\.ts duplicate each other$`, func() error {
		return d.writeFiles(map[string]string{"src/x.ts": fetchTs, "src/y.ts": fetchTs})
	})
	sc.Step(`^the pair from (\S+) and (\S+) is reported$`, d.pairIsReported)
	sc.Step(`^the pair from (\S+) and (\S+) is not reported$`, d.pairIsNotReported)
	sc.Step(`^only pairs with at least one function in a changed file are reported$`, d.onlyChangedPairs)
	sc.Step(`^those functions are compared against every source in the project$`, d.changedAgainstAll)
	sc.Step(`^no pair reaches the threshold$`, d.noPairReaches)
	sc.Step(`^(\S+) is a directory holding no source files$`, func(dir string) error {
		// The project has sources, a pair of them duplicates, all outside
		// the directory.
		return d.writeFiles(map[string]string{dir + "/notes.md": "# Notes\n", "src/cart.ts": totalTs, "src/receipt.ts": totalTs})
	})

	// Sources itos-cc.yaml excludes, with the step writing it (steps.go).
	sc.Step(`^src/a\.go and src/port/porttest/fake\.go hold the same function with its names changed$`, func() error {
		return d.writeFiles(map[string]string{"src/a.go": positiveGo, "src/port/porttest/fake.go": fakePositiveGo})
	})
	sc.Step(`^no pair is reported$`, d.noPairReported)
	sc.Step(`^"itos-cc ([^"]*)" reports no pair$`, func(args string) error {
		if err := w.runWith(w.env(), args); err != nil {
			return err
		}
		return d.noPairReported()
	})

	// The text output.
	sc.Step(`^two functions that score (\d\.\d\d) in TypeScript$`, func(score string) error {
		if score != "0.95" {
			return fmt.Errorf("the fixture is tuned to score 0.95, not %s", score)
		}
		return d.writeFiles(map[string]string{"src/a.ts": scoredA, "src/b.ts": scoredB})
	})
	sc.Step(`^pairs are listed best first$`, d.pairsBestFirst)
	sc.Step(`^four copies of one helper, every pair of them scoring between (\d\.\d\d) and (\d\.\d\d)$`, func(low, high string) error {
		if low != "0.90" || high != "1.00" {
			return fmt.Errorf("the fixture is tuned to score between 0.90 and 1.00, not %s and %s", low, high)
		}
		return d.writeFiles(fourCopies())
	})
	sc.Step(`^stdout shows one entry, not (\w+) pairs:$`, d.oneEntryNotPairs)
	sc.Step(`^a group holds every function linked to another member by a pair at or above the threshold$`, d.groupHoldsLinked)
	sc.Step(`^groups are listed best first, then largest first$`, d.groupsBestThenLargest)

	// The JSON output and the snapshot.
	sc.Step(`^\.metrics/dry\.json holds the version, the threshold, the candidates, and the groups$`, d.snapshotHolds)
	sc.Step(`^each candidate has (.+), and left and right sides with (.+)$`, d.eachCandidateHas)
	sc.Step(`^each group has (.+), and its members$`, d.eachGroupHas)
}

// everyPair is every pair of functions in the same language in dir, with
// nothing left out.
func (d *dryState) everyPair(dir string) ([]dryPair, error) {
	object, err := d.dryIn(dir, "--threshold", "0", "--min-lines", "1", "--min-nodes", "1")
	return object.Candidates, err
}

// valued are dry's flags that take a value.
var valued = []string{"--threshold", "--min-lines", "--min-nodes"}

// options are the scenario's run's flags that take a value, but those
// named in leave, each with its value: the run's words without its paths,
// --changed and --json.
func (d *dryState) options(leave ...string) []string {
	var kept []string
	args := d.w.last.args
	for i := 1; i < len(args); i++ {
		name, _, hasValue := strings.Cut(args[i], "=")
		if !slices.Contains(valued, name) {
			continue
		}
		taken := []string{args[i]}
		if !hasValue && i+1 < len(args) {
			i++
			taken = append(taken, args[i])
		}
		if !slices.Contains(leave, name) {
			kept = append(kept, taken...)
		}
	}
	return kept
}

// wholeProject is what the scenario's run reports of the whole project:
// the same run without its paths and --changed.
func (d *dryState) wholeProject() ([]dryPair, error) {
	object, err := d.dryIn(d.w.last.dir, d.options()...)
	return object.Candidates, err
}

// keys are the pairs' keys, sorted.
func keys(pairs []dryPair) []string {
	var all []string
	for _, p := range pairs {
		all = append(all, p.key())
	}
	slices.Sort(all)
	return all
}

// samePairs is whether got are the pairs of want, whatever their order.
func samePairs(got, want []dryPair, what string) error {
	if g, w := keys(got), keys(want); !slices.Equal(g, w) {
		return fmt.Errorf("the pairs reported are\n  %s\nnot %s,\n  %s", strings.Join(g, "\n  "), what, strings.Join(w, "\n  "))
	}
	return nil
}

func filtered(pairs []dryPair, keep func(dryPair) bool) []dryPair {
	var out []dryPair
	for _, p := range pairs {
		if keep(p) {
			out = append(out, p)
		}
	}
	return out
}

// needs is an error when the fixture has no pair keep holds of, which the
// sentence needs to be told from its opposite.
func needs(pairs []dryPair, what string, keep func(dryPair) bool) error {
	if len(filtered(pairs, keep)) == 0 {
		return fmt.Errorf("the fixture has no pair %s, so the step cannot fail", what)
	}
	return nil
}

// pairsScoringAtLeastAreReported is whether the run reported exactly the
// pairs scoring at least threshold of the functions its sizes admit, which
// dry gives with --threshold 0 and the run's other options. The fixture
// must hold such pairs on both sides of the threshold.
func (d *dryState) pairsScoringAtLeastAreReported(threshold float64) error {
	got, err := d.reported()
	if err != nil {
		return err
	}
	admitted, err := d.dryIn(d.w.last.dir, append(d.options("--threshold"), "--threshold", "0")...)
	if err != nil {
		return err
	}
	for _, need := range []struct {
		what string
		keep func(dryPair) bool
	}{
		{fmt.Sprintf("scoring at least %v", threshold), func(p dryPair) bool { return p.Score >= threshold }},
		{fmt.Sprintf("scoring below %v", threshold), func(p dryPair) bool { return p.Score < threshold }},
	} {
		if err := needs(admitted.Candidates, need.what, need.keep); err != nil {
			return err
		}
	}
	return samePairs(got, filtered(admitted.Candidates, func(p dryPair) bool { return p.Score >= threshold }),
		fmt.Sprintf("those scoring at least %v", threshold))
}

// smallAreNotCandidates is whether the functions the run's sizes admit,
// those dry pairs with --threshold 0 and the run's other options, are
// exactly those of lines lines and nodes nodes or more, and whether no pair
// the run reported holds another. The fixture must hold identical
// functions too short, and too few nodes, alone.
func (d *dryState) smallAreNotCandidates(lines, nodes int) error {
	got, err := d.reported()
	if err != nil {
		return err
	}
	big := func(s drySide) bool { return s.lines() >= lines && s.Nodes >= nodes }
	for _, p := range got {
		if !p.both(big) {
			return fmt.Errorf("%s is reported\n%s", p, d.w.report())
		}
	}
	all, err := d.everyPair(d.w.last.dir)
	if err != nil {
		return err
	}
	if err := needs(all, fmt.Sprintf("scoring 1 of a function shorter than %d lines", lines), func(p dryPair) bool {
		return p.Score == 1 && p.both(func(s drySide) bool { return s.Nodes >= nodes }) && !p.both(big)
	}); err != nil {
		return err
	}
	if err := needs(all, fmt.Sprintf("scoring 1 of a function with fewer than %d nodes", nodes), func(p dryPair) bool {
		return p.Score == 1 && p.both(func(s drySide) bool { return s.lines() >= lines }) && !p.both(big)
	}); err != nil {
		return err
	}
	admitted, err := d.dryIn(d.w.last.dir, append(d.options("--threshold"), "--threshold", "0")...)
	if err != nil {
		return err
	}
	sides := func(pairs []dryPair, keep func(drySide) bool) []string {
		var out []string
		for _, p := range pairs {
			for _, s := range []drySide{p.Left, p.Right} {
				if keep(s) && !slices.Contains(out, s.String()) {
					out = append(out, s.String())
				}
			}
		}
		slices.Sort(out)
		return out
	}
	candidates, want := sides(admitted.Candidates, func(drySide) bool { return true }), sides(all, big)
	if !slices.Equal(candidates, want) {
		return fmt.Errorf("the candidates are\n  %s\nnot those of %d lines and %d nodes or more,\n  %s",
			strings.Join(candidates, "\n  "), lines, nodes, strings.Join(want, "\n  "))
	}
	return nil
}

// onlyPairsReported is whether the run reported exactly the pairs scoring
// at least threshold of functions of lines lines and nodes nodes or more.
// The fixture must hold a pair left out by each bound alone.
func (d *dryState) onlyPairsReported(threshold float64, lines, nodes int) error {
	got, err := d.reported()
	if err != nil {
		return err
	}
	all, err := d.everyPair(d.w.last.dir)
	if err != nil {
		return err
	}
	long := func(s drySide) bool { return s.lines() >= lines }
	many := func(s drySide) bool { return s.Nodes >= nodes }
	big := func(s drySide) bool { return long(s) && many(s) }
	for _, need := range []struct {
		what string
		keep func(dryPair) bool
	}{
		{"reported", func(p dryPair) bool { return p.Score >= threshold && p.both(big) }},
		{fmt.Sprintf("left out by its score below %v alone", threshold), func(p dryPair) bool { return p.Score < threshold && p.both(big) }},
		{fmt.Sprintf("left out by a function shorter than %d lines alone", lines), func(p dryPair) bool {
			return p.Score >= threshold && p.both(many) && !p.both(long)
		}},
		{fmt.Sprintf("left out by a function with fewer than %d nodes alone", nodes), func(p dryPair) bool {
			return p.Score >= threshold && p.both(long) && !p.both(many)
		}},
	} {
		if err := needs(all, need.what, need.keep); err != nil {
			return err
		}
	}
	return samePairs(got, filtered(all, func(p dryPair) bool { return p.Score >= threshold && p.both(big) }),
		fmt.Sprintf("those scoring at least %v of functions of %d+ lines and %d+ nodes", threshold, lines, nodes))
}

// exactlyOnePair is whether the run reported one pair, of the two
// functions named file#name.
func (d *dryState) exactlyOnePair(leftFile, leftName, rightFile, rightName string) error {
	got, err := d.reported()
	if err != nil {
		return err
	}
	want := []string{leftFile + "#" + leftName, rightFile + "#" + rightName}
	slices.Sort(want)
	if len(got) == 1 {
		sides := []string{filepath.ToSlash(got[0].Left.File) + "#" + got[0].Left.Name, filepath.ToSlash(got[0].Right.File) + "#" + got[0].Right.Name}
		slices.Sort(sides)
		if slices.Equal(sides, want) {
			return nil
		}
	}
	return fmt.Errorf("the pairs reported are\n  %s\nnot %s alone", strings.Join(keys(got), "\n  "), strings.Join(want, " and "))
}

// itsScoreIs is whether the one pair reported scores at least low and
// below high.
func (d *dryState) itsScoreIs(low, high float64) error {
	got, err := d.reported()
	if err != nil {
		return err
	}
	if len(got) != 1 {
		return fmt.Errorf("%d pairs are reported, not one", len(got))
	}
	if s := got[0].Score; s < low || s >= high {
		return fmt.Errorf("its score is %v, not at least %v and below %v", s, low, high)
	}
	return nil
}

// notCompared is whether the scenario's two functions, in two languages,
// are listed as units and dry, with nothing left out, pairs them with
// nothing: every pair it compares it reports at --threshold 0.
func (d *dryState) notCompared() error {
	if _, err := d.reported(); err != nil {
		return err
	}
	for _, file := range d.files {
		code, stdout, stderr, err := d.w.capture(d.w.last.dir, d.w.env(), "units", "--json", file)
		if err != nil {
			return err
		}
		var object struct{ Units []unit }
		if code != 0 || json.Unmarshal([]byte(stdout), &object) != nil || len(object.Units) == 0 {
			return fmt.Errorf("%s has no unit\n%s%s", file, stdout, stderr)
		}
	}
	all, err := d.everyPair(d.w.last.dir)
	if err != nil {
		return err
	}
	if len(all) != 0 {
		return fmt.Errorf("compared:\n  %s", strings.Join(keys(all), "\n  "))
	}
	return nil
}

// identicalFunctions writes two files, each holding the same function of
// count lines, many nodes to each line.
func (d *dryState) identicalFunctions(count int) error {
	if count < 2 {
		return fmt.Errorf("a function takes 2 lines or more, not %d", count)
	}
	var body strings.Builder
	body.WriteString("def f(xs):\n")
	for range count - 2 {
		body.WriteString("    xs = [weigh(x) + bias(x) * scale(x) for x in xs if x > 0 and x < limit(xs)]\n")
	}
	body.WriteString("    return [weigh(x) + bias(x) * scale(x) for x in xs if x > 0 and x < limit(xs)]\n")
	return d.writeFiles(map[string]string{"src/a.py": body.String(), "src/b.py": body.String()})
}

// theyAreNotReported is whether no pair reported holds two of the
// scenario's functions, which dry scores 1 with nothing left out.
// noPairReported is whether the run reported no pair, while the files the
// scenario wrote, laid out alone in a folder of their own with nothing
// left out, hold a pair scoring 1: only what the run leaves out keeps that
// pair from it.
func (d *dryState) noPairReported() error {
	got, err := d.reported()
	if err != nil {
		return err
	}
	if len(got) > 0 {
		return fmt.Errorf("reported:\n  %s\n%s", strings.Join(keys(got), "\n  "), d.w.report())
	}
	alone, err := d.w.d.Folder()
	if err != nil {
		return err
	}
	defer func() { _ = d.w.d.Remove(alone) }()
	for _, name := range d.files {
		data, err := d.w.readIn(d.w.dir, name)
		if err != nil {
			return err
		}
		if err := d.w.writeIn(alone, name, string(data)); err != nil {
			return err
		}
	}
	all, err := d.everyPair(alone)
	if err != nil {
		return err
	}
	theirs := func(p dryPair) bool {
		l, r := filepath.ToSlash(p.Left.File), filepath.ToSlash(p.Right.File)
		return l != r && slices.Contains(d.files, l) && slices.Contains(d.files, r)
	}
	return needs(all, "of the files written scoring 1", func(p dryPair) bool { return theirs(p) && p.Score == 1 })
}

func (d *dryState) theyAreNotReported() error {
	got, err := d.reported()
	if err != nil {
		return err
	}
	theirs := func(p dryPair) bool {
		return slices.Contains(d.files, filepath.ToSlash(p.Left.File)) && slices.Contains(d.files, filepath.ToSlash(p.Right.File))
	}
	if mine := filtered(got, theirs); len(mine) > 0 {
		return fmt.Errorf("reported:\n  %s", strings.Join(keys(mine), "\n  "))
	}
	all, err := d.everyPair(d.w.last.dir)
	if err != nil {
		return err
	}
	return needs(all, "of them scoring 1", func(p dryPair) bool { return theirs(p) && p.Score == 1 })
}

// between are the pairs of a function in a and one in b.
func between(pairs []dryPair, a, b string) []dryPair {
	return filtered(pairs, func(p dryPair) bool {
		l, r := filepath.ToSlash(p.Left.File), filepath.ToSlash(p.Right.File)
		return l == a && r == b || l == b && r == a
	})
}

// pairIsReported is whether the run reported a pair of a function in a and
// one in b.
func (d *dryState) pairIsReported(a, b string) error {
	got, err := d.reported()
	if err != nil {
		return err
	}
	if len(between(got, a, b)) == 0 {
		return fmt.Errorf("no pair from %s and %s is reported:\n  %s", a, b, strings.Join(keys(got), "\n  "))
	}
	return nil
}

// pairIsNotReported is whether the run reported no pair of a function in a
// and one in b, though the whole project's run reports one.
func (d *dryState) pairIsNotReported(a, b string) error {
	got, err := d.reported()
	if err != nil {
		return err
	}
	if mine := between(got, a, b); len(mine) > 0 {
		return fmt.Errorf("reported:\n  %s", strings.Join(keys(mine), "\n  "))
	}
	whole, err := d.wholeProject()
	if err != nil {
		return err
	}
	return needs(whole, "from "+a+" and "+b+" in the whole project", func(p dryPair) bool { return len(between([]dryPair{p}, a, b)) > 0 })
}

// onlyChangedPairs is whether every pair reported has a function in a
// changed file, though the whole project's run reports a pair with none.
func (d *dryState) onlyChangedPairs() error {
	got, err := d.reported()
	if err != nil {
		return err
	}
	if len(d.changed) == 0 {
		return errors.New("no file is changed")
	}
	if len(got) == 0 {
		return fmt.Errorf("no pair is reported\n%s", d.w.report())
	}
	for _, p := range got {
		if !p.in(d.changed) {
			return fmt.Errorf("%s is reported, in no changed file of %s", p, strings.Join(d.changed, " "))
		}
	}
	whole, err := d.wholeProject()
	if err != nil {
		return err
	}
	return needs(whole, "in no changed file", func(p dryPair) bool { return !p.in(d.changed) })
}

// changedAgainstAll is whether the run reported every pair of the whole
// project's run with a function in a changed file, one of them with a
// function in a file not changed.
func (d *dryState) changedAgainstAll() error {
	got, err := d.reported()
	if err != nil {
		return err
	}
	whole, err := d.wholeProject()
	if err != nil {
		return err
	}
	if err := needs(got, "of a changed file's function and an unchanged file's", func(p dryPair) bool {
		return !slices.Contains(d.changed, filepath.ToSlash(p.Left.File)) || !slices.Contains(d.changed, filepath.ToSlash(p.Right.File))
	}); err != nil {
		return err
	}
	return samePairs(got, filtered(whole, func(p dryPair) bool { return p.in(d.changed) }),
		"those of the whole project with a function in a changed file")
}

// noPairReaches writes two unlike functions of the sizes dry compares by
// default, which it pairs at --threshold 0.
func (d *dryState) noPairReaches() error {
	if err := d.writeFiles(map[string]string{"src/load.ts": loadTs, "src/fetch.ts": fetchTs}); err != nil {
		return err
	}
	object, err := d.dryIn(d.w.dir, "--threshold", "0")
	if err != nil {
		return err
	}
	if len(object.Candidates) == 0 {
		return errors.New("dry compares none of the functions written")
	}
	return nil
}

// otherFolder runs the scenario's command in a folder of its own holding
// files, and gives its entries and, with --json added, its object.
func (d *dryState) otherFolder(name string, files map[string]string) ([]entry, dryObject, error) {
	w := d.w
	var object dryObject
	if w.last == nil || slices.Contains(w.last.args, "--json") {
		return nil, object, errors.New("itos-cc dry was not run with its text output")
	}
	_, _ = d.reported()
	dir := filepath.Join(w.dir, name)
	for file, text := range files {
		if err := w.writeIn(dir, file, text); err != nil {
			return nil, object, err
		}
	}
	code, stdout, stderr, err := w.capture(dir, w.env(), w.last.args...)
	if err != nil {
		return nil, object, err
	}
	if code != 0 {
		return nil, object, fmt.Errorf("itos-cc %s exited %d in %s\n%s%s", strings.Join(w.last.args, " "), code, name, stdout, stderr)
	}
	listed, err := entries(stdout)
	if err != nil {
		return nil, object, err
	}
	object, err = d.dryIn(dir, w.last.args[1:]...)
	return listed, object, err
}
