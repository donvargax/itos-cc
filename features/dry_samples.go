package features

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// The dry scenarios' sample projects, and the copies of functions they write.

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
	extensions = map[string]string{"TypeScript": ".ts", "Go": ".go", "Kotlin": ".kt", "Python": ".py", "Dart": ".dart"}
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
