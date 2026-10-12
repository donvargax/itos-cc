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
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
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

	// Sources itos-cc.yaml excludes, with the step writing it (steps_test.go).
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

// positiveGo and fakePositiveGo are one Go function of nine lines, its
// names changed, the second in a package of fakes, as a fake that copies
// the code it stands in for is.
const (
	positiveGo = `package a

func total(xs []int) int {
	sum := 0
	for _, x := range xs {
		if x > 0 {
			sum += x
		}
	}
	return sum
}
`
	fakePositiveGo = `package porttest

func count(items []int) int {
	kept := 0
	for _, item := range items {
		if item > 0 {
			kept += item
		}
	}
	return kept
}
`
)

// The fixtures. Each TypeScript function is ten lines or fewer and scored
// as its comment says; the scores were read from dry itself.
const (
	// totalTs is ten lines and 68 nodes.
	totalTs = `export function total(lines) {
  let sum = 0;
  for (const line of lines) {
    if (line.quantity > 0 && !line.void) {
      sum += price(line) * line.quantity;
    }
  }
  const tax = taxOn(sum);
  return round(sum + tax);
}
`
	// loadTs is eight lines and 74 nodes, unlike totalTs and fetchTs.
	loadTs = `export function load(path, options) {
  const text = readFile(path, options.encoding);
  if (!text) {
    throw new Error(missing(path));
  }
  const rows = text.split(options.separator).map(parseRow);
  return rows.filter((row) => row.length > 0);
}
`
	// fetchTs is twelve lines and 74 nodes.
	fetchTs = `export async function fetchAll(urls, retries) {
  const results = [];
  while (urls.length > 0) {
    const url = urls.shift();
    try {
      results.push(await get(url));
    } catch (error) {
      if (retries-- > 0) urls.push(url);
    }
  }
  return results;
}
`
	totalPy = `def total(lines):
    sum = 0
    for line in lines:
        if line.quantity > 0 and not line.void:
            sum += price(line) * line.quantity
    tax = tax_on(sum)
    return round(sum + tax)
`
)

// invoicePy, receiptPy and parsePy are the project of "Near copies are
// reported": receipt's render is invoice's with one more line.
const (
	invoicePy = `def render(invoice):
    lines = []
    for item in invoice.items:
        lines.append(format_line(item.name, item.price))
    total = sum(item.price for item in invoice.items)
    lines.append(format_total(total))
    return "\n".join(lines)
`
	receiptPy = `def render(receipt):
    out = []
    for entry in receipt.entries:
        out.append(format_line(entry.label, entry.amount))
    paid = sum(entry.amount for entry in receipt.entries)
    out.append(format_total(paid))
    out.append(format_footer(receipt.store))
    return "\n".join(out)
`
	parsePy = `def parse(text):
    if not text:
        raise ValueError("empty")
    head, _, rest = text.partition(":")
    return {"head": head.strip(), "rest": rest.strip()}
`
)

// scoredA and scoredB score 0.947: b's sum is a's total renamed, its tax
// declared with let instead of const. a's function is on lines 3-12, b's on
// 7-16.
const (
	scoredA = `import { price, taxOn, round } from "./money";

` + totalTs
	scoredB = `import { price, taxOn, round } from "./money";

// The sum of a receipt,
// its tax included,
// rounded to the cent.

export function sum(rows) {
  let acc = 0;
  for (const row of rows) {
    if (row.quantity > 0 && !row.void) {
      acc += price(row) * row.quantity;
    }
  }
  let tax = taxOn(acc);
  return round(acc + tax);
}
`
)

// fourCopies are a.go to d.go, each holding f on lines 1-9: a and b score
// 1, as do c and d, which call factor where a and b call weight, and every
// other pair scores 0.902. The package clause follows the function so it
// can start on line 1; the parser reads it anywhere, and with no go.mod it
// names the namespace.
func fourCopies() map[string]string {
	f := `func f(xs []int, limit int) (int, error) {
	total, count := 0, 0
	for i, x := range xs {
		if x > 0 && i < limit {
			total += scale(x) * weight(i)
			count++
		}
	}
	return total, check(count) }
`
	b := strings.NewReplacer("total", "sum", "xs", "items", "limit", "most").Replace(f)
	c := strings.ReplaceAll(f, "weight(i)", "factor(i)")
	d := strings.ReplaceAll(c, "count", "n")
	files := map[string]string{}
	for name, text := range map[string]string{"a": f, "b": b, "c": c, "d": d} {
		files[name+".go"] = text + "\npackage " + name + "\n"
	}
	return files
}

// The sample, laid out for the scenarios that only say When I run: by
// slash-separated path, TypeScript functions whose pairs fall on either side
// of every bound the scenarios name, one at the bound and one just past it,
// so a bound written one step off fails. totalTs in cart.ts and its near
// copies in receipt.ts (0.947) and invoice.ts (0.897 with cart.ts, 0.85 with
// receipt.ts) are ten lines and 68 nodes, fetchTs in fetch.ts and its copy
// calling head in head.ts (0.903) twelve lines and 74. Each other file holds
// two functions, which score 1 but in near.ts (0.825) and far.ts (0.818):
// in load.ts eight lines of 74 nodes, in lines7.ts seven of 101, in lines4.ts
// four of 58, in lines3.ts three of 49, in nodes40.ts and nodes39.ts ten
// lines of 40 and 39 nodes, in nodes20.ts and nodes19.ts five of 20 and 19,
// in near.ts and far.ts nine of 43 and 42.
var drySample = map[string]string{
	"src/cart.ts":    totalTs,
	"src/receipt.ts": strings.Replace(scoredB, "export function sum", "export function receipt", 1),
	"src/invoice.ts": strings.Replace(totalTs, "round(sum", "fix(sum", 1),
	"src/fetch.ts":   fetchTs,
	"src/head.ts":    strings.Replace(fetchTs, "await get(", "await head(", 1),
	"src/load.ts":    loadTs + "\n" + strings.Replace(loadTs, "function load", "function reload", 1),
	"src/lines7.ts": `export function tint(a, b, t) {
  const r = mix(a.red, b.red, t) + bias(a, b) * t;
  const g = mix(a.green, b.green, t) - bias(b, a) * t;
  const l = luma(r, g, a.blue);
  log(l, r, g);
  return rgb(clamp(r, 0, 255), clamp(g, 0, 255), a.blue);
}

export function hue(x, y, k) {
  const r = mix(x.red, y.red, k) + bias(x, y) * k;
  const g = mix(x.green, y.green, k) - bias(y, x) * k;
  const l = luma(r, g, x.blue);
  log(l, r, g);
  return rgb(clamp(r, 0, 255), clamp(g, 0, 255), x.blue);
}
`,
	"src/lines4.ts": `export function blend(a, b, t) {
  const r = mix(a.red, b.red, t) + bias(a, b) * t;
  return rgb(clamp(r, 0, 255), clamp(a.green, 0, 255), a.blue);
}

export function shade(x, y, k) {
  const r = mix(x.red, y.red, k) + bias(x, y) * k;
  return rgb(clamp(r, 0, 255), clamp(x.green, 0, 255), x.blue);
}
`,
	"src/lines3.ts": `export function score(a, b) {
  return weigh(a.x, b.x) + weigh(a.y, b.y) * factor(a, b) - offset(a.z, b.z);
}

export function rank(p, q) {
  return weigh(p.x, q.x) + weigh(p.y, q.y) * factor(p, q) - offset(p.z, q.z);
}
`,
	"src/nodes40.ts": calls("start", "a()", "b()", "c()", "d()", "e()", "f()", "g()", "h(x)") + "\n" +
		calls("stop", "a()", "b()", "c()", "d()", "e()", "f()", "g()", "h(x)"),
	"src/nodes39.ts": calls("open", "a()", "b()", "c()", "d()", "e()", "f()", "g()", "h()") + "\n" +
		calls("close", "a()", "b()", "c()", "d()", "e()", "f()", "g()", "h()"),
	"src/nodes20.ts": calls("wait", "a()", "b()", "c(x)") + "\n" + calls("pause", "a()", "b()", "c(x)"),
	"src/nodes19.ts": calls("idle", "a()", "b()", "c()") + "\n" + calls("rest", "a()", "b()", "c()"),
	"src/near.ts": calls("near", "c1(x)", "c2(x)", "c3(x)", "c4(x)", "c5(x)", "c6(x)", "c7(x, x)") + "\n" +
		calls("nearer", "c1(x)", "c2(x)", "c3(x)", "c4(x)", "c5(x)", "c6(x)", "z(x, x)"),
	"src/far.ts": calls("far", "d1(x)", "d2(x)", "d3(x)", "d4(x)", "d5(x)", "d6(x)", "d7(x)") + "\n" +
		calls("farther", "d1(x)", "d2(x)", "d3(x)", "d4(x)", "d5(x)", "d6(x)", "y(x)"),
}

// calls is a TypeScript function of one parameter that makes each call, one
// to a line: seven nodes and four or five more for each call.
func calls(name string, each ...string) string {
	var text strings.Builder
	text.WriteString("export function " + name + "(x) {\n")
	for _, call := range each {
		text.WriteString("  " + call + ";\n")
	}
	text.WriteString("}\n")
	return text.String()
}

// sampleChanged is the sample's file changed since its commit: committed
// as a copy of cart.ts renamed, it is then rewritten as the sample has it.
const sampleChanged = "src/receipt.ts"

// layOutSample writes the sample in a git repository, every file committed
// and then sampleChanged changed.
func (d *dryState) layOutSample() error {
	w := d.w
	for name, text := range drySample {
		if name == sampleChanged {
			text = strings.Replace(totalTs, "export function total", "export function receipt", 1)
		}
		if err := w.write(name, text); err != nil {
			return err
		}
	}
	for _, args := range [][]string{{"init", "-q"}, {"add", "-A"}, {"commit", "-q", "-m", "sample"}} {
		if err := w.git(args...); err != nil {
			return err
		}
	}
	d.changed = []string{sampleChanged}
	return w.write(sampleChanged, drySample[sampleChanged])
}

// writeFile writes one file of the scenario's.
func (d *dryState) writeFile(name, text string) error {
	d.files = append(d.files, name)
	return d.w.write(name, text)
}

// writeFiles writes the scenario's files, in the order of their names.
func (d *dryState) writeFiles(files map[string]string) error {
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		if err := d.writeFile(name, files[name]); err != nil {
			return err
		}
	}
	return nil
}

// extensions are the file extensions of the languages a scenario writes
// functions in, and headers what a file of the language holds before them.
var (
	extensions = map[string]string{"TypeScript": ".ts", "Go": ".go", "Kotlin": ".kt", "Python": ".py"}
	headers    = map[string]string{"Go": "package p\n\n"}
)

// writeFunctions writes source, the scenario's functions, to a file of the
// language.
func (d *dryState) writeFunctions(language, source string) error {
	d.language, d.file = language, "functions"+extensions[language]
	return d.w.write(d.file, headers[language]+source+"\n")
}

// The changes a copy is made with, as the scenario's table words them.
var (
	calledInstead  = regexp.MustCompile(`^(\w+) called instead of (\w+)$`)
	subtractedFrom = regexp.MustCompile(`^"([^"]*)" subtracted from the return value$`)
	returned       = regexp.MustCompile(`return ([^;]+);`)
)

// copyWith adds to the file a copy of the function with the change: a
// called function named instead of another, or a text put after the
// returned value.
func (d *dryState) copyWith(change string) error {
	if d.first == "" {
		return errors.New("no function to copy")
	}
	var copied string
	if m := calledInstead.FindStringSubmatch(change); m != nil {
		called, instead := m[1], m[2]
		if !strings.Contains(d.first, instead+"(") {
			return fmt.Errorf("%q calls no %s", d.first, instead)
		}
		copied = strings.ReplaceAll(d.first, instead+"(", called+"(")
	} else if m := subtractedFrom.FindStringSubmatch(change); m != nil {
		if !returned.MatchString(d.first) {
			return fmt.Errorf("%q returns no value", d.first)
		}
		copied = returned.ReplaceAllStringFunc(d.first, func(r string) string {
			return strings.TrimSuffix(r, ";") + m[1] + ";"
		})
	} else {
		return fmt.Errorf("no step makes a copy with %s", change)
	}
	return d.writeFunctions(d.language, d.first+"\n"+copied)
}

// compareDirectly scores the file's two functions with nothing left out,
// --threshold 0 --min-lines 1 --min-nodes 1, which reports every pair.
func (d *dryState) compareDirectly() error {
	w := d.w
	if err := w.runWith(w.env(), "dry --json --threshold 0 --min-lines 1 --min-nodes 1 "+d.file); err != nil {
		return err
	}
	if w.exit != 0 {
		return fmt.Errorf("itos-cc exited %d\n%s", w.exit, w.report())
	}
	var object dryObject
	if err := json.Unmarshal([]byte(w.stdout), &object); err != nil {
		return fmt.Errorf("standard output is not dry's object: %v\n%s", err, w.report())
	}
	if len(object.Candidates) != 1 {
		return fmt.Errorf("the file holds not two functions but a pair for each of %d\n%s", len(object.Candidates), w.report())
	}
	d.score, d.compared = object.Candidates[0].Score, true
	return nil
}

func (d *dryState) mustHaveCompared() error {
	if !d.compared {
		return errors.New("no two functions were compared")
	}
	return nil
}

// drySide is one function of a pair or member of a group, as --json and
// .metrics/dry.json give it.
type drySide struct {
	File      string `json:"file"`
	StartLine int    `json:"start_line"`
	EndLine   int    `json:"end_line"`
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Nodes     int    `json:"nodes"`
}

func (s drySide) lines() int { return s.EndLine - s.StartLine + 1 }

// String is the side as the text output lists it.
func (s drySide) String() string {
	return fmt.Sprintf("%s:%d-%d  %s#%s", filepath.ToSlash(s.File), s.StartLine, s.EndLine, s.Namespace, s.Name)
}

type dryPair struct {
	Score    float64 `json:"score"`
	Language string  `json:"language"`
	Left     drySide `json:"left"`
	Right    drySide `json:"right"`
}

func (p dryPair) String() string { return fmt.Sprintf("%v %s | %s", p.Score, p.Left, p.Right) }

// key is the pair whatever its order.
func (p dryPair) key() string {
	a, b := p.Left.String(), p.Right.String()
	if b < a {
		a, b = b, a
	}
	return a + " | " + b
}

// in is whether a side of the pair is in one of files.
func (p dryPair) in(files []string) bool {
	return slices.Contains(files, filepath.ToSlash(p.Left.File)) || slices.Contains(files, filepath.ToSlash(p.Right.File))
}

// both is whether both sides of the pair are what keep says.
func (p dryPair) both(keep func(drySide) bool) bool { return keep(p.Left) && keep(p.Right) }

type dryGroup struct {
	Language string    `json:"language"`
	MinScore float64   `json:"min_score"`
	MaxScore float64   `json:"max_score"`
	Members  []drySide `json:"members"`
}

// dryObject is dry's --json object.
type dryObject struct {
	Threshold  float64    `json:"threshold"`
	Candidates []dryPair  `json:"candidates"`
	Groups     []dryGroup `json:"groups"`
}

// entry is one entry of the text output: its score, a range where the
// group's pairs differ, its language and its members' lines.
type entry struct {
	min, max float64
	language string
	members  []string
}

var (
	entryHead   = regexp.MustCompile(`^DUPLICATE score=(\d\.\d\d)(?:–(\d\.\d\d))? (\S+)$`)
	entryMember = regexp.MustCompile(`^  (\S.*:\d+-\d+  .*#.+)$`)
)

// entries reads the text output, each line an entry's head or one of its
// members.
func entries(stdout string) ([]entry, error) {
	var all []entry
	for _, line := range outputLines(filepath.ToSlash(stdout)) {
		line = strings.TrimSuffix(line, "\r")
		if m := entryHead.FindStringSubmatch(line); m != nil {
			e := entry{language: m[3]}
			e.min, _ = strconv.ParseFloat(m[1], 64)
			e.max = e.min
			if m[2] != "" {
				e.max, _ = strconv.ParseFloat(m[2], 64)
			}
			all = append(all, e)
			continue
		}
		if m := entryMember.FindStringSubmatch(line); m != nil && len(all) > 0 {
			all[len(all)-1].members = append(all[len(all)-1].members, m[1])
			continue
		}
		if line != "" {
			return nil, fmt.Errorf("%q is no line of an entry", line)
		}
	}
	return all, nil
}

// reported are the pairs the scenario's run reported: from its --json
// object, or with the text output from the .metrics/dry.json it wrote, the
// text listing the functions of those pairs, an entry for each group.
func (d *dryState) reported() ([]dryPair, error) {
	w := d.w
	if w.last == nil {
		return nil, errors.New("itos-cc dry was not run")
	}
	if d.reportedRun != w.last {
		d.reportedRun = w.last
		d.reportedPairs, d.reportedErr = d.readReported()
	}
	return d.reportedPairs, d.reportedErr
}

func (d *dryState) readReported() ([]dryPair, error) {
	w := d.w
	if w.exit != 0 {
		return nil, fmt.Errorf("itos-cc exited %d\n%s", w.exit, w.report())
	}
	var object dryObject
	if strings.HasPrefix(strings.TrimSpace(w.stdout), "{") {
		if err := json.Unmarshal([]byte(w.stdout), &object); err != nil {
			return nil, fmt.Errorf("standard output is not dry's object: %v\n%s", err, w.report())
		}
		return object.Candidates, nil
	}
	raw, err := os.ReadFile(filepath.Join(w.last.dir, ".metrics", "dry.json"))
	if err != nil {
		return nil, fmt.Errorf("the run wrote no .metrics/dry.json: %v\n%s", err, w.report())
	}
	if err := json.Unmarshal(raw, &object); err != nil {
		return nil, fmt.Errorf(".metrics/dry.json is not dry's snapshot: %v\n%s", err, raw)
	}
	listed, err := entries(w.stdout)
	if err != nil {
		return nil, fmt.Errorf("%v\n%s", err, w.report())
	}
	var printed, recorded []string
	for _, e := range listed {
		printed = append(printed, e.members...)
	}
	for _, p := range object.Candidates {
		recorded = append(recorded, p.Left.String(), p.Right.String())
	}
	slices.Sort(printed)
	slices.Sort(recorded)
	if !slices.Equal(printed, slices.Compact(recorded)) || len(listed) != len(object.Groups) {
		return nil, fmt.Errorf("stdout lists %d entries of\n  %s\nand .metrics/dry.json %d groups of\n  %s\n%s",
			len(listed), strings.Join(printed, "\n  "), len(object.Groups), strings.Join(recorded, "\n  "), w.report())
	}
	return object.Candidates, nil
}

// dryIn runs itos-cc dry with args and --json in dir, after reading the
// scenario's own run, and gives its object.
func (d *dryState) dryIn(dir string, args ...string) (dryObject, error) {
	_, _ = d.reported()
	var object dryObject
	args = append([]string{"dry", "--json"}, args...)
	code, stdout, stderr, err := d.w.capture(dir, d.w.env(), d.w.bin, args...)
	if err != nil {
		return object, err
	}
	if code != 0 {
		return object, fmt.Errorf("itos-cc %s exited %d\n%s%s", strings.Join(args, " "), code, stdout, stderr)
	}
	if err := json.Unmarshal([]byte(stdout), &object); err != nil {
		return object, fmt.Errorf("itos-cc %s printed no JSON object: %v\n%s", strings.Join(args, " "), err, stdout)
	}
	return object, nil
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
		code, stdout, stderr, err := d.w.capture(d.w.last.dir, d.w.env(), d.w.bin, "units", "--json", file)
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
	alone, err := os.MkdirTemp("", "itos-cc-features-dry-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(alone) }()
	for _, name := range d.files {
		data, err := os.ReadFile(filepath.Join(d.w.dir, filepath.FromSlash(name)))
		if err != nil {
			return err
		}
		if err := writeIn(alone, name, string(data)); err != nil {
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
		if err := writeIn(dir, file, text); err != nil {
			return nil, object, err
		}
	}
	code, stdout, stderr, err := w.capture(dir, w.env(), w.bin, w.last.args...)
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

// ordered are groups of TypeScript functions in a folder of their own: in
// c1 to c3 three copies of totalTs, in b1 and b2 two of loadTs, both groups
// scoring 1, and in a1 and a2 fetchTs and a copy calling head for get,
// scoring 0.903. Best first, then largest first, is the reverse of their
// files' order.
var ordered = map[string]string{
	"a1.ts": fetchTs, "a2.ts": strings.Replace(fetchTs, "await get(", "await head(", 1),
	"b1.ts": loadTs, "b2.ts": loadTs,
	"c1.ts": totalTs, "c2.ts": totalTs, "c3.ts": totalTs,
}

// pairsBestFirst is whether the entries and the pairs are listed from the
// highest score down, the scenario's command run over ordered.
func (d *dryState) pairsBestFirst() error {
	listed, object, err := d.otherFolder("ordered", ordered)
	if err != nil {
		return err
	}
	var entryScores, pairScores []float64
	for _, e := range listed {
		entryScores = append(entryScores, e.max)
	}
	for _, p := range object.Candidates {
		pairScores = append(pairScores, p.Score)
	}
	for _, s := range [][]float64{entryScores, pairScores} {
		if len(slices.Compact(slices.Clone(s))) < 2 {
			return fmt.Errorf("the fixture gives scores %v, too few to order", s)
		}
		if !slices.IsSortedFunc(s, func(a, b float64) int { return cmp.Compare(b, a) }) {
			return fmt.Errorf("listed with scores %v, not best first", s)
		}
	}
	return nil
}

// groupsBestThenLargest is whether the entries are listed by their best
// score, highest first, then by their members, most first, the scenario's
// command run over ordered.
func (d *dryState) groupsBestThenLargest() error {
	listed, _, err := d.otherFolder("ordered", ordered)
	if err != nil {
		return err
	}
	tie := false
	for i := 1; i < len(listed); i++ {
		a, b := listed[i-1], listed[i]
		if a.max == b.max && len(a.members) != len(b.members) {
			tie = true
		}
		if a.max < b.max || a.max == b.max && len(a.members) < len(b.members) {
			return fmt.Errorf("entry %d, %v with %d members, is listed before %v with %d", i, a.max, len(a.members), b.max, len(b.members))
		}
	}
	if !tie || len(listed) < 3 {
		return fmt.Errorf("the fixture lists %d entries, without two of one score and unlike sizes", len(listed))
	}
	return nil
}

// linked are TypeScript functions in a folder of their own: b is totalTs,
// a calls fix for round and c subtracts the tax, each 0.897 with b and
// 0.804 with each other; e, with two changes of its own, is 0.818 with b and
// 0.731 with a and c, so it joins them at any threshold below the default;
// d is unlike them all.
var linked = map[string]string{
	"a.ts": strings.Replace(totalTs, "round(sum", "fix(sum", 1),
	"b.ts": totalTs,
	"c.ts": strings.Replace(totalTs, "sum + tax", "sum - tax", 1),
	"d.ts": loadTs,
	"e.ts": strings.NewReplacer("!line.void", "line.ok", "> 0", ">= 0").Replace(totalTs),
}

// groupHoldsLinked is whether each entry, the scenario's command run over
// linked, holds the functions linked by pairs at or above its threshold,
// every one of them, and no other: what dry scores with nothing left out,
// joined. The fixture must have a group with two members whose own pair is
// below the threshold, and a function in none.
func (d *dryState) groupHoldsLinked() error {
	listed, object, err := d.otherFolder("linked", linked)
	if err != nil {
		return err
	}
	all, err := d.everyPair(filepath.Join(d.w.dir, "linked"))
	if err != nil {
		return err
	}
	group := map[string]string{} // each function's group, by a member's name
	var find func(string) string
	find = func(s string) string {
		if group[s] == "" || group[s] == s {
			return s
		}
		return find(group[s])
	}
	functions := map[string]bool{}
	for _, p := range all {
		functions[p.Left.String()], functions[p.Right.String()] = true, true
		if p.Score >= object.Threshold {
			group[find(p.Left.String())] = find(p.Right.String())
		}
	}
	members := map[string][]string{}
	for f := range functions {
		members[find(f)] = append(members[find(f)], f)
	}
	var want, got []string
	alone := false
	for _, m := range members {
		if len(m) == 1 {
			alone = true
			continue
		}
		slices.Sort(m)
		want = append(want, strings.Join(m, ", "))
	}
	for _, e := range listed {
		m := slices.Clone(e.members)
		slices.Sort(m)
		got = append(got, strings.Join(m, ", "))
	}
	slices.Sort(want)
	slices.Sort(got)
	if !slices.Equal(got, want) {
		return fmt.Errorf("the groups are\n  %s\nnot\n  %s", strings.Join(got, "\n  "), strings.Join(want, "\n  "))
	}
	if !alone {
		return errors.New("the fixture has no function in no group")
	}
	return needs(all, "below the threshold of two members of one group", func(p dryPair) bool {
		return p.Score < object.Threshold && find(p.Left.String()) == find(p.Right.String())
	})
}

// numbers are the counts a scenario writes as words.
var numbers = map[string]int{"one": 1, "two": 2, "three": 3, "four": 4, "five": 5, "six": 6}

// oneEntryNotPairs is whether stdout is the text, one entry, and the run
// reported as many pairs as the scenario says.
func (d *dryState) oneEntryNotPairs(count string, text *godog.DocString) error {
	if err := d.w.stdoutShows(text); err != nil {
		return err
	}
	listed, err := entries(d.w.stdout)
	if err != nil {
		return err
	}
	if len(listed) != 1 {
		return fmt.Errorf("stdout shows %d entries, not one", len(listed))
	}
	got, err := d.reported()
	if err != nil {
		return err
	}
	if want, ok := numbers[count]; !ok || len(got) != want {
		return fmt.Errorf("the run reported %d pairs, not %s:\n  %s", len(got), count, strings.Join(keys(got), "\n  "))
	}
	return nil
}

// snapshotHolds is whether .metrics/dry.json holds a version, and the
// threshold, the candidates and the groups stdout's object holds.
func (d *dryState) snapshotHolds() error {
	w := d.w
	raw, err := os.ReadFile(filepath.Join(w.last.dir, ".metrics", "dry.json"))
	if err != nil {
		return fmt.Errorf("the run wrote no .metrics/dry.json: %v", err)
	}
	var snapshot, object map[string]any
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		return fmt.Errorf(".metrics/dry.json is no JSON object: %v\n%s", err, raw)
	}
	if err := json.Unmarshal([]byte(w.stdout), &object); err != nil {
		return fmt.Errorf("standard output is no JSON object: %v\n%s", err, w.report())
	}
	if version, ok := snapshot["version"].(float64); !ok || version < 1 {
		return fmt.Errorf(".metrics/dry.json has no version: %v", snapshot["version"])
	}
	// stdout names a file from the working directory with the system's
	// separator, the snapshot with slashes: an escaped backslash in the
	// JSON text reads as a slash.
	for _, key := range []string{"threshold", "candidates", "groups"} {
		want, _ := json.Marshal(object[key])
		got, _ := json.Marshal(snapshot[key])
		if object[key] == nil || string(got) != strings.ReplaceAll(string(want), `\\`, "/") {
			return fmt.Errorf(".metrics/dry.json's %s is %s, not stdout's %s", key, got, want)
		}
	}
	return nil
}

// sentenceKeys are the keys a sentence lists ("a, b, and c"), each of "lines"
// the start and end lines.
func sentenceKeys(list string) []string {
	var out []string
	for _, key := range strings.Split(strings.ReplaceAll(strings.ReplaceAll(list, ", and ", ", "), " and ", ", "), ", ") {
		if key == "lines" {
			out = append(out, "start_line", "end_line")
			continue
		}
		out = append(out, key)
	}
	return out
}

// jsonList is the list key of stdout's object, which must hold something.
func (d *dryState) jsonList(key string) ([]map[string]any, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(d.w.stdout), &raw); err != nil {
		return nil, fmt.Errorf("standard output is no JSON object: %v\n%s", err, d.w.report())
	}
	var list []map[string]any
	if err := json.Unmarshal(raw[key], &list); err != nil || len(list) == 0 {
		return nil, fmt.Errorf("the object's %s is no list of objects with something in it: %v\n%s", key, err, d.w.report())
	}
	return list, nil
}

// hasKeys is whether the object has every key, what a sentence calls it
// for the error.
func hasKeys(object map[string]any, what string, keys []string) error {
	for _, key := range keys {
		if _, ok := object[key]; !ok {
			return fmt.Errorf("%s has no %q: %v", what, key, object)
		}
	}
	return nil
}

// eachCandidateHas is whether stdout's object lists candidates, each with
// every key of first and a left and a right side with every key of sides.
func (d *dryState) eachCandidateHas(first, sides string) error {
	candidates, err := d.jsonList("candidates")
	if err != nil {
		return err
	}
	for i, c := range candidates {
		if err := hasKeys(c, fmt.Sprintf("candidate %d", i+1), sentenceKeys(first)); err != nil {
			return err
		}
		for _, side := range []string{"left", "right"} {
			s, ok := c[side].(map[string]any)
			if !ok {
				return fmt.Errorf("candidate %d has no %s side: %v", i+1, side, c)
			}
			if err := hasKeys(s, fmt.Sprintf("candidate %d's %s side", i+1, side), sentenceKeys(sides)); err != nil {
				return err
			}
		}
	}
	return nil
}

// eachGroupHas is whether stdout's object lists groups, each with every
// key of list and members, a list with something in it.
func (d *dryState) eachGroupHas(list string) error {
	groups, err := d.jsonList("groups")
	if err != nil {
		return err
	}
	for i, g := range groups {
		if err := hasKeys(g, fmt.Sprintf("group %d", i+1), sentenceKeys(list)); err != nil {
			return err
		}
		if members, ok := g["members"].([]any); !ok || len(members) == 0 {
			return fmt.Errorf("group %d has no members: %v", i+1, g)
		}
	}
	return nil
}
