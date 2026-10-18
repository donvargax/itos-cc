// The steps of units.feature: the functions and methods itos-cc units lists,
// judged from what it prints and, where a scenario says what other commands
// make of a unit, from what crap, dry and mutation print and the snapshot
// mutation run writes. Like every step here, they read the binary's output
// alone, never its code.
package features

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/cucumber/godog"
)

// unitsState is one units.feature scenario's state beside the world's.
type unitsState struct {
	w *world

	// testFiles are the sample project's test files, by slash-separated
	// path, when the scenario runs in it.
	testFiles []string
	// file is the file the scenario's Given wrote, and source its text.
	file, source string
	// class and method are those of the Kotlin class the scenario wrote.
	class, method string
	// alone is the folder with no project marker in it or above it that a
	// scenario runs in instead of the scratch folder, and args the words
	// it last ran there.
	alone string
	args  string
}

// sample is the project a units.feature scenario runs in unless it writes
// its own files: production and test code in TypeScript and Python, by
// slash-separated path. Its test files are the ones whose name says so.
var sample = map[string]string{
	"src/shop/cart.ts": "export function total(prices: number[]): number {\n" +
		"  return prices.reduce((sum, p) => sum + p, 0);\n" +
		"}\n\n" +
		"export class Cart {\n" +
		"  items: number[] = [];\n" +
		"  add(price: number) { this.items.push(price); }\n" +
		"}\n",
	"src/shop/cart.test.ts": "import { total } from \"./cart\";\n\n" +
		"function pricesOf(n: number): number[] {\n" +
		"  return Array.from({ length: n }, () => 1);\n" +
		"}\n\n" +
		"test(\"total\", () => {\n" +
		"  expect(total(pricesOf(3))).toBe(3);\n" +
		"});\n",
	"src/shop/price.py": "def discount(price, rate):\n" +
		"    if rate > 1:\n" +
		"        raise ValueError(rate)\n" +
		"    return price * (1 - rate)\n",
	"tests/test_price.py": "from shop.price import discount\n\n\n" +
		"def test_discount():\n" +
		"    assert discount(10, 0.5) == 5\n",
}

func unitSteps(sc *godog.ScenarioContext, w *world) {
	u := &unitsState{w: w}
	sc.Before(func(ctx context.Context, s *godog.Scenario) (context.Context, error) {
		*u = unitsState{w: w}
		if filepath.Base(s.Uri) != "units.feature" {
			return ctx, nil
		}
		// units.feature's scenarios run in the sample project, as its
		// first ones list it; the ones that write their own files list
		// those alone.
		for name, text := range sample {
			if err := w.write(name, text); err != nil {
				return ctx, err
			}
			if strings.Contains(name, "test") {
				u.testFiles = append(u.testFiles, name)
			}
		}
		return ctx, nil
	})
	sc.After(func(ctx context.Context, _ *godog.Scenario, _ error) (context.Context, error) {
		if u.alone != "" {
			_ = w.d.Remove(filepath.Dir(u.alone))
		}
		return ctx, nil
	})

	sc.Step(`^stdout lists each unit as "file:start-end  namespace#name  kind"$`, u.listsEachUnitAsALine)
	sc.Step(`^each unit has (.+)$`, u.eachUnitHas)
	sc.Step(`^only units from test files are listed$`, u.onlyTestFiles)
	sc.Step(`^the selection holds no supported files$`, u.noSupportedFiles)

	sc.Step(`^the file "([^"]*)":$`, func(name string, text *godog.DocString) error {
		u.file, u.source = name, text.Content+"\n"
		return w.write(name, u.source)
	})
	sc.Step(`^its units are listed$`, func() error { return u.list(u.file) })
	sc.Step(`^the units of (\S+) are listed$`, u.list)
	sc.Step(`^the units are:$`, u.unitsAre)
	sc.Step(`^nested arrow functions, callbacks, and abstract methods are not units$`, u.nestedAreNotUnits)
	sc.Step(`^(.+) on any object are routes$`, u.methodsAreRoutes)
	sc.Step(`^a route's complexity, mutation sites, duplication, coverage, and hash are its own, not mount's$`, u.routesOwnTheirMeasures)
	sc.Step(`^the line a route starts on is mount's, since mounting runs it$`, u.routeStartLineIsMounts)
	sc.Step(`^test files have no route units$`, u.testFilesHaveNoRoutes)

	sc.Step(`^the file "([^"]*)" with a component that returns JSX$`, func(name string) error {
		u.file = name
		return w.write(name, "export function Cell(props: { value: number }) {\n  return <td className=\"cell\">{props.value}</td>;\n}\n")
	})
	sc.Step(`^the component "([^"]*)" is a unit$`, u.isAUnit)
	sc.Step(`^the file "([^"]*)" defining "([^"]*)"$`, func(name, function string) error {
		u.file = name
		return w.write(name, "def "+function+"():\n    return 0\n")
	})
	sc.Step(`^the unit is "([^"]*)"$`, u.theUnitIs)
	sc.Step(`^a Kotlin class annotated with both @Configuration and @EnableWebSecurity$`, u.annotatedKotlinClass)
	sc.Step(`^its method is a unit of that class$`, u.methodOfThatClass)
	sc.Step(`^the Go module "([^"]*)" with package "([^"]*)"$`, u.goModule)

	sc.Step(`^x\.ts declares function place, in a directory with no package\.json, go\.mod or \.git in it or above it$`, u.aloneFolder)
	sc.Step(`^I run "itos-cc ([^"]*)" from that directory$`, func(args string) error {
		u.args = args
		return u.runIn(u.alone, args)
	})
	sc.Step(`^it is "([^"]*)" from a copy of that directory at another path$`, u.fromACopy)

	// Sources itos-cc.yaml excludes, with the step writing it (steps.go).
	sc.Step(`^no unit of (\S+) is listed$`, u.noUnitOf)
	sc.Step(`^a unit of (\S+) is listed$`, u.aUnitOf)
}

// unit is one unit as itos-cc units lists it. The plain lines give no
// language and say nothing of privacy.
type unit struct {
	File      string `json:"file"`
	Language  string `json:"language"`
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	Private   bool   `json:"private"`
	StartLine int    `json:"start_line"`
	EndLine   int    `json:"end_line"`
}

func (x unit) id() string { return x.Namespace + "#" + x.Name }

// plainUnit is one plain line: file:start-end, namespace#name and kind,
// two spaces apart. A name may hold spaces and #, as a route's does.
var plainUnit = regexp.MustCompile(`^(.+):(\d+)-(\d+)  (.+)  (function|method)$`)

// listed is what the last run listed, from its --json object or its plain
// lines, the run having succeeded.
func (u *unitsState) listed() ([]unit, error) {
	w := u.w
	if w.exit != 0 {
		return nil, fmt.Errorf("itos-cc exited %d\n%s", w.exit, w.report())
	}
	if strings.HasPrefix(w.stdout, "{") {
		var object struct{ Units []unit }
		if err := json.Unmarshal([]byte(w.stdout), &object); err != nil {
			return nil, fmt.Errorf("standard output is not the units object: %v\n%s", err, w.report())
		}
		return object.Units, nil
	}
	var all []unit
	for _, line := range outputLines(w.stdout) {
		if line == "" {
			continue
		}
		m := plainUnit.FindStringSubmatch(strings.TrimSuffix(line, "\r"))
		if m == nil {
			return nil, fmt.Errorf("%q is no unit's line\n%s", line, w.report())
		}
		start, _ := strconv.Atoi(m[2])
		end, _ := strconv.Atoi(m[3])
		namespace, name, _ := strings.Cut(m[4], "#")
		all = append(all, unit{File: m[1], Namespace: namespace, Name: name, Kind: m[5], StartLine: start, EndLine: end})
	}
	return all, nil
}

// noUnitOf is whether the run succeeded and listed no unit of file, a
// slash-separated path from the scratch folder.
func (u *unitsState) noUnitOf(file string) error {
	all, err := u.listed()
	if err != nil {
		return err
	}
	for _, x := range all {
		if filepath.ToSlash(x.File) == file {
			return fmt.Errorf("%s#%s of %s is listed\n%s", x.Namespace, x.Name, file, u.w.report())
		}
	}
	return nil
}

// aUnitOf is whether the run listed a unit of file, a slash-separated path
// from the scratch folder.
func (u *unitsState) aUnitOf(file string) error {
	all, err := u.listed()
	if err != nil {
		return err
	}
	for _, x := range all {
		if filepath.ToSlash(x.File) == file {
			return nil
		}
	}
	return fmt.Errorf("no unit of %s is listed\n%s", file, u.w.report())
}

// listsEachUnitAsALine is whether the run listed units, each a line of the
// form the scenario gives.
func (u *unitsState) listsEachUnitAsALine() error {
	all, err := u.listed()
	if err != nil {
		return err
	}
	if len(all) == 0 || strings.HasPrefix(u.w.stdout, "{") {
		return fmt.Errorf("no unit is listed as a line\n%s", u.w.report())
	}
	return nil
}

// eachUnitHas is whether the object lists units, each with every key the
// list names ("a, b, and c").
func (u *unitsState) eachUnitHas(list string) error {
	w := u.w
	var object struct{ Units []map[string]any }
	if err := json.Unmarshal([]byte(w.stdout), &object); err != nil || len(object.Units) == 0 {
		return fmt.Errorf("the object lists no unit: %v\n%s", err, w.report())
	}
	keys := strings.Split(strings.ReplaceAll(list, ", and ", ", "), ", ")
	for i, one := range object.Units {
		for _, key := range keys {
			if _, ok := one[key]; !ok {
				return fmt.Errorf("unit %d has no %q\n%s", i+1, key, w.report())
			}
		}
	}
	return nil
}

// onlyTestFiles is whether the run listed units, each of them in one of the
// sample project's test files.
func (u *unitsState) onlyTestFiles() error {
	all, err := u.listed()
	if err != nil {
		return err
	}
	if len(all) == 0 {
		return fmt.Errorf("no unit is listed\n%s", u.w.report())
	}
	for _, x := range all {
		if !slices.Contains(u.testFiles, filepath.ToSlash(x.File)) {
			return fmt.Errorf("%s is listed, and %s is no test file\n%s", x.id(), x.File, u.w.report())
		}
	}
	return nil
}

// noSupportedFiles leaves the scratch folder with files of no supported
// language alone: what the sample wrote there is removed, each folder of
// it whole.
func (u *unitsState) noSupportedFiles() error {
	for name := range sample {
		top, _, _ := strings.Cut(name, "/")
		if err := u.w.d.Remove(filepath.Join(u.w.dir, top)); err != nil {
			return err
		}
	}
	u.testFiles = nil
	for name, text := range map[string]string{"README.md": "# notes\n", "docs/notes.txt": "nothing to measure\n"} {
		if err := u.w.write(name, text); err != nil {
			return err
		}
	}
	return nil
}

// list lists the units of path with --json.
func (u *unitsState) list(path string) error {
	return u.w.runWith(u.w.env(), "units --json "+path)
}

// unitsAre is whether the run listed exactly the table's units, in its
// order: each row's kind, namespace and name, its lines (start-end) where
// the table has the column, and whether it is private.
func (u *unitsState) unitsAre(table *godog.Table) error {
	all, err := u.listed()
	if err != nil {
		return err
	}
	header := table.Rows[0].Cells
	var want, got []string
	for _, row := range table.Rows[1:] {
		var cells []string
		for _, c := range row.Cells {
			cells = append(cells, c.Value)
		}
		want = append(want, strings.Join(cells, " | "))
	}
	for _, x := range all {
		var cells []string
		for _, h := range header {
			switch h.Value {
			case "kind":
				cells = append(cells, x.Kind)
			case "namespace":
				cells = append(cells, x.Namespace)
			case "name":
				cells = append(cells, x.Name)
			case "lines":
				cells = append(cells, fmt.Sprintf("%d-%d", x.StartLine, x.EndLine))
			case "private":
				cells = append(cells, map[bool]string{true: "yes", false: "no"}[x.Private])
			default:
				return fmt.Errorf("the table has a column %q no step reads", h.Value)
			}
		}
		got = append(got, strings.Join(cells, " | "))
	}
	if !slices.Equal(got, want) {
		return fmt.Errorf("the units listed are\n  %s\nnot\n  %s", strings.Join(got, "\n  "), strings.Join(want, "\n  "))
	}
	return nil
}

// arrowOrFunction is a line declaring an arrow function or a function
// expression, and abstractMethod one declaring a method with no body.
var (
	arrowOrFunction = regexp.MustCompile(`=>|\bfunction\b`)
	abstractMethod  = regexp.MustCompile(`^\s*abstract\s+\w+\s*\(`)
)

// nestedAreNotUnits is whether no unit starts on a line of the file that
// declares a function the listing leaves out: an arrow function or a
// function expression no unit starts on, nested or a callback, and an
// abstract method. The file must declare some, so the sentence is not
// true of a file without any.
func (u *unitsState) nestedAreNotUnits() error {
	all, err := u.listed()
	if err != nil {
		return err
	}
	starts := map[int]bool{}
	for _, x := range all {
		starts[x.StartLine] = true
	}
	var left []int
	for i, line := range strings.Split(u.source, "\n") {
		if abstractMethod.MatchString(line) || arrowOrFunction.MatchString(line) && !starts[i+1] {
			left = append(left, i+1)
		}
	}
	if len(left) == 0 {
		return fmt.Errorf("%s declares no nested function, callback or abstract method", u.file)
	}
	for _, x := range all {
		if slices.Contains(left, x.StartLine) {
			return fmt.Errorf("%s, on line %d, is a unit: it is nested, a callback or abstract\n%s", x.id(), x.StartLine, u.w.report())
		}
	}
	return nil
}

// methodsAreRoutes is whether a callback each method of the list
// ("a, b, and c") registers on an object of its own, with a path, is a
// route unit named for the method and the path: the file the scenario
// wrote calls only some of them.
func (u *unitsState) methodsAreRoutes(list string) error {
	methods := strings.Split(strings.ReplaceAll(list, ", and ", ", "), ", ")
	var source strings.Builder
	var want []string
	for i, m := range methods {
		fmt.Fprintf(&source, "object%d.%s(\"/%s\", (req, res) => res.end());\n", i, m, m)
		want = append(want, "verbs#"+strings.ToUpper(m)+" /"+m)
	}
	if err := u.w.write("verbs.js", source.String()); err != nil {
		return err
	}
	if err := u.list("verbs.js"); err != nil {
		return err
	}
	all, err := u.listed()
	if err != nil {
		return err
	}
	var got []string
	for _, x := range all {
		got = append(got, x.id())
	}
	if !slices.Equal(got, want) {
		return fmt.Errorf("the units of\n%s\nare %q, not %q", source.String(), got, want)
	}
	return nil
}

// isAUnit is whether the run listed the unit id (namespace#name).
func (u *unitsState) isAUnit(id string) error {
	if err := u.list(u.file); err != nil {
		return err
	}
	all, err := u.listed()
	if err != nil {
		return err
	}
	for _, x := range all {
		if x.id() == id {
			return nil
		}
	}
	return fmt.Errorf("%s is not among the units of %s\n%s", id, u.file, u.w.report())
}

// theUnitIs is whether the run listed one unit, id (namespace#name), from
// its --json object or its plain line.
func (u *unitsState) theUnitIs(id string) error {
	all, err := u.listed()
	if err != nil {
		return err
	}
	var got []string
	for _, x := range all {
		got = append(got, x.id())
	}
	if len(got) != 1 || got[0] != id {
		return fmt.Errorf("the units listed are %q, not %s alone\n%s", got, id, u.w.report())
	}
	return nil
}

// annotatedKotlinClass writes a Kotlin class carrying both annotations
// above its declaration, with one method.
func (u *unitsState) annotatedKotlinClass() error {
	u.file, u.class, u.method = "src/main/kotlin/demo/security/SecurityConfig.kt", "demo.security.SecurityConfig", "filterChain"
	return u.w.write(u.file, "package demo.security\n\n"+
		"import org.springframework.context.annotation.Configuration\n"+
		"import org.springframework.security.config.annotation.web.configuration.EnableWebSecurity\n\n"+
		"@Configuration\n"+
		"@EnableWebSecurity\n"+
		"class SecurityConfig {\n"+
		"    fun filterChain(): String = \"chain\"\n"+
		"}\n")
}

// methodOfThatClass is whether the class's method is a method unit of the
// class.
func (u *unitsState) methodOfThatClass() error {
	if err := u.list(u.file); err != nil {
		return err
	}
	all, err := u.listed()
	if err != nil {
		return err
	}
	for _, x := range all {
		if x.Namespace == u.class && x.Name == u.method && x.Kind == "method" {
			return nil
		}
	}
	return fmt.Errorf("%s#%s is no method unit of %s\n%s", u.class, u.method, u.file, u.w.report())
}

// goModule writes the module's go.mod and its package's board.go: a type,
// a constructor, an exported and an unexported method, and an unexported
// function, in that order.
func (u *unitsState) goModule(module, pkg string) error {
	if err := u.w.write("go.mod", "module "+module+"\n\ngo 1.22\n"); err != nil {
		return err
	}
	return u.w.write(pkg+"/board.go", "package "+pkg+"\n\n"+
		"type Board struct{ cells []int }\n\n"+
		"func New() *Board { return &Board{} }\n\n"+
		"func (b *Board) Place(n int) { b.cells = append(b.cells, n) }\n\n"+
		"func (b *Board) size() int { return len(b.cells) }\n\n"+
		"func helper() int { return 1 }\n")
}

// markers are the files that make a folder a project root for some
// language: one in a folder or above it names its files from there.
var markers = []string{"package.json", "tsconfig.json", "go.mod", ".git"}

// aloneFolder makes a folder holding x.ts, which declares place, with no
// project marker in it or above it. Where the temporary directory has one
// above it the scenario cannot be set up, and is skipped.
func (u *unitsState) aloneFolder() error {
	parent, err := u.w.d.Folder()
	if err != nil {
		return err
	}
	u.alone = filepath.Join(parent, "here")
	if above := u.markerAbove(u.alone); above != "" {
		_ = u.w.d.Remove(parent)
		u.alone = ""
		return fmt.Errorf("%s is above the temporary directory: %w", above, godog.ErrSkip)
	}
	return u.w.writeIn(u.alone, "x.ts", "export function place(a: number): boolean {\n  return a > 0;\n}\n")
}

// markerAbove is the first project marker in dir or a folder above it, or
// "".
func (u *unitsState) markerAbove(dir string) string {
	for {
		for _, m := range markers {
			if u.w.d.Exists(filepath.Join(dir, m)) {
				return filepath.Join(dir, m)
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// runIn runs itos-cc with args in dir.
func (u *unitsState) runIn(dir, args string) error {
	if dir == "" {
		return errors.New("no folder to run in")
	}
	fields, err := commandWords(args)
	if err != nil {
		return err
	}
	return u.w.runEnv(dir, u.w.env(), fields...)
}

// fromACopy is whether the last command, run in a copy of the folder at
// another path, lists one unit, id.
func (u *unitsState) fromACopy(id string) error {
	parent, err := u.w.d.Folder()
	if err != nil {
		return err
	}
	defer func() { _ = u.w.d.Remove(parent) }()
	copied := filepath.Join(parent, "elsewhere")
	if above := u.markerAbove(copied); above != "" {
		return fmt.Errorf("%s is above the copy: %w", above, godog.ErrSkip)
	}
	text, err := u.w.readIn(u.alone, "x.ts")
	if err != nil {
		return err
	}
	if err := u.w.writeIn(copied, "x.ts", string(text)); err != nil {
		return err
	}
	if err := u.runIn(copied, u.args); err != nil {
		return err
	}
	return u.theUnitIs(id)
}
