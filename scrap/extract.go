// Package scrap measures the structure of test code the way crap measures
// production code: which tests are too big, too clever, too mocked, too
// weakly asserted, or repeated, and whether a test file is worth refactoring
// at all. Its verdicts are meant to steer an AI assistant: when to leave a
// file alone, when to rewrite repeated examples as a table, when to clean up
// in place, and when to split the file first.
package scrap

import (
	"strings"

	sitter "github.com/tree-sitter/go-tree-sitter"

	"github.com/donvargax/itos-cc/lang"
)

// Example is one test case.
type Example struct {
	Name      string
	Group     string // describe blocks, test classes, or the parent Go test
	StartLine int
	EndLine   int
	Table     bool // already table-driven: it.each, parametrize, a t.Run loop
	// Data are the case tables written beside the example: it.each's
	// arguments, a parametrize decorator, a @CsvSource. They are fixtures,
	// not test code.
	Data []*sitter.Node
	Node *sitter.Node
	// Body is the code the example runs, without its name and decorators.
	Body *sitter.Node
}

// Extract finds the examples of a test file and the lines spent on shared
// setup (beforeEach, fixtures, setUp, @BeforeEach).
func Extract(f *lang.File) (examples []Example, setupLines int) {
	x := extractor{f: f}
	switch f.Spec.Name {
	case "typescript":
		x.typescript(f.Root, nil)
	case "python":
		x.python(f.Root, nil)
	case "go":
		x.golang(f.Root)
	case "kotlin":
		x.kotlin(f.Root, nil)
	}
	return x.examples, x.setup
}

type extractor struct {
	f        *lang.File
	examples []Example
	setup    int
}

func (x *extractor) text(n *sitter.Node) string { return n.Utf8Text(x.f.Src) }

func (x *extractor) add(name string, groups []string, n, body *sitter.Node, table bool, data ...*sitter.Node) {
	x.examples = append(x.examples, Example{
		Name:      name,
		Group:     strings.Join(groups, " › "),
		StartLine: int(n.StartPosition().Row) + 1,
		EndLine:   int(n.EndPosition().Row) + 1,
		Table:     table || len(data) > 0,
		Data:      data,
		Node:      n,
		Body:      body,
	})
}

func lines(n *sitter.Node) int {
	return int(n.EndPosition().Row-n.StartPosition().Row) + 1
}

func children(n *sitter.Node) []*sitter.Node {
	out := make([]*sitter.Node, 0, n.NamedChildCount())
	for i := uint(0); i < n.NamedChildCount(); i++ {
		out = append(out, n.NamedChild(i))
	}
	return out
}

// --- TypeScript: Vitest, Jest, Mocha ---

var (
	tsGroups   = map[string]bool{"describe": true, "context": true, "suite": true}
	tsExamples = map[string]bool{"it": true, "test": true, "specify": true}
	tsSetup    = map[string]bool{"beforeEach": true, "beforeAll": true, "afterEach": true, "afterAll": true}
)

func (x *extractor) typescript(n *sitter.Node, groups []string) {
	if n.Kind() == "call_expression" {
		base, cases := tsCallee(n.ChildByFieldName("function"), x.f.Src)
		args := n.ChildByFieldName("arguments")
		body := lastFunctionArg(args)
		switch {
		case tsGroups[base] && body != nil:
			x.typescript(body, append(groups, firstStringArg(args, x.f.Src)))
			return
		case tsExamples[base] && body != nil && cases != nil:
			x.add(firstStringArg(args, x.f.Src), groups, n, body, true, cases)
			return
		case tsExamples[base] && body != nil:
			x.add(firstStringArg(args, x.f.Src), groups, n, body, false)
			return
		case tsSetup[base]:
			x.setup += lines(n)
			return
		}
	}
	for _, c := range children(n) {
		x.typescript(c, groups)
	}
}

// tsCallee names the test function a call invokes: it for it(...),
// it.only(...), and it.each(table)(...), returning the cases .each was given
// when there is one.
func tsCallee(fn *sitter.Node, src []byte) (base string, cases *sitter.Node) {
	var call *sitter.Node
	for fn != nil {
		switch fn.Kind() {
		case "identifier":
			return fn.Utf8Text(src), cases
		case "member_expression":
			if fieldText(fn, "property", src) == "each" && call != nil {
				cases = call.ChildByFieldName("arguments")
			}
			fn = fn.ChildByFieldName("object")
		case "call_expression":
			call, fn = fn, fn.ChildByFieldName("function")
		default:
			return "", nil
		}
	}
	return "", nil
}

func lastFunctionArg(args *sitter.Node) *sitter.Node {
	if args == nil {
		return nil
	}
	for i := int(args.NamedChildCount()) - 1; i >= 0; i-- {
		a := args.NamedChild(uint(i))
		if a.Kind() == "arrow_function" || a.Kind() == "function_expression" {
			return a.ChildByFieldName("body")
		}
	}
	return nil
}

func firstStringArg(args *sitter.Node, src []byte) string {
	if args == nil || args.NamedChildCount() == 0 {
		return ""
	}
	return strings.Trim(args.NamedChild(0).Utf8Text(src), "\"'`")
}

func fieldText(n *sitter.Node, field string, src []byte) string {
	if c := n.ChildByFieldName(field); c != nil {
		return c.Utf8Text(src)
	}
	return ""
}

// --- Python: pytest and unittest ---

func (x *extractor) python(n *sitter.Node, groups []string) {
	for _, c := range children(n) {
		def, decorators := c, ""
		var cases []*sitter.Node
		if c.Kind() == "decorated_definition" {
			def = c.ChildByFieldName("definition")
			for _, d := range children(c) {
				if d.Kind() == "decorator" {
					decorators += x.text(d) + "\n"
					if strings.Contains(x.text(d), "parametrize") {
						cases = append(cases, d)
					}
				}
			}
		}
		if def == nil {
			continue
		}
		name := fieldText(def, "name", x.f.Src)
		switch {
		case def.Kind() == "class_definition":
			x.python(def.ChildByFieldName("body"), append(groups, name))
		case def.Kind() != "function_definition":
		case strings.HasPrefix(name, "test"):
			x.add(name, groups, c, def.ChildByFieldName("body"), false, cases...)
		case strings.Contains(decorators, "fixture") || pySetup[name]:
			x.setup += lines(c)
		}
	}
}

var pySetup = map[string]bool{
	"setUp": true, "tearDown": true, "setUpClass": true, "tearDownClass": true,
	"setup_method": true, "teardown_method": true, "setup_class": true, "teardown_class": true,
	"setup_function": true, "teardown_function": true,
}

// --- Go: testing ---

func (x *extractor) golang(root *sitter.Node) {
	for _, fn := range children(root) {
		name := fieldText(fn, "name", x.f.Src)
		body := fn.ChildByFieldName("body")
		if fn.Kind() != "function_declaration" || body == nil || !strings.HasPrefix(name, "Test") {
			continue
		}
		if name == "TestMain" {
			x.setup += lines(fn)
			continue
		}
		subtests, table := x.goSubtests(body)
		if table || len(subtests) == 0 {
			x.add(name, nil, fn, body, table)
			continue
		}
		for _, st := range subtests {
			x.add(st.name, []string{name}, st.node, st.body, false)
		}
	}
}

type subtest struct {
	name       string
	node, body *sitter.Node
}

// goSubtests finds t.Run calls. One inside a loop makes the whole test a
// table: the loop runs it once per case.
func (x *extractor) goSubtests(body *sitter.Node) (subtests []subtest, table bool) {
	var walk func(n *sitter.Node, inLoop bool)
	walk = func(n *sitter.Node, inLoop bool) {
		if n.Kind() == "call_expression" {
			fn := n.ChildByFieldName("function")
			args := n.ChildByFieldName("arguments")
			if fn != nil && fn.Kind() == "selector_expression" && fieldText(fn, "field", x.f.Src) == "Run" &&
				args != nil && args.NamedChildCount() == 2 && args.NamedChild(1).Kind() == "func_literal" {
				if inLoop {
					table = true
					return
				}
				subtests = append(subtests, subtest{
					name: strings.Trim(x.text(args.NamedChild(0)), "\"`"),
					node: n,
					body: args.NamedChild(1).ChildByFieldName("body"),
				})
				return
			}
		}
		for _, c := range children(n) {
			walk(c, inLoop || n.Kind() == "for_statement")
		}
	}
	walk(body, false)
	return subtests, table
}

// --- Kotlin: JUnit and kotest ---

var (
	ktExampleAnnotations = map[string]bool{"Test": true, "ParameterizedTest": true, "RepeatedTest": true, "TestFactory": true}
	ktSetupAnnotations   = map[string]bool{"BeforeEach": true, "BeforeAll": true, "AfterEach": true, "AfterAll": true,
		"Before": true, "After": true, "BeforeClass": true, "AfterClass": true}
	ktGroups   = map[string]bool{"describe": true, "context": true, "feature": true, "given": true}
	ktExamples = map[string]bool{"test": true, "it": true, "should": true, "scenario": true, "then": true}
)

func (x *extractor) kotlin(n *sitter.Node, groups []string) {
	switch n.Kind() {
	case "class_declaration", "object_declaration":
		groups = append(groups, ktName(n, x.f.Src))
	case "function_declaration":
		annotations := x.ktAnnotations(n)
		switch {
		case annotations["Test"] || annotations["RepeatedTest"] || annotations["TestFactory"]:
			x.add(ktName(n, x.f.Src), groups, n, ktBody(n), false)
		case annotations["ParameterizedTest"]:
			x.add(ktName(n, x.f.Src), groups, n, ktBody(n), true, x.ktSources(n)...)
		default:
			for a := range annotations {
				if ktSetupAnnotations[a] {
					x.setup += lines(n)
				}
			}
		}
		return
	case "call_expression":
		// kotest: test("name") { … } inside a spec's init block or constructor lambda.
		if name, lambda := x.ktCall(n); lambda != nil {
			switch {
			case ktGroups[name]:
				x.kotlin(lambda, append(groups, x.ktTitle(n)))
				return
			case ktExamples[name]:
				x.add(x.ktTitle(n), groups, n, lambda, false)
				return
			}
		}
	}
	for _, c := range children(n) {
		x.kotlin(c, groups)
	}
}

func (x *extractor) ktAnnotations(fn *sitter.Node) map[string]bool {
	out := map[string]bool{}
	for _, c := range children(fn) {
		if c.Kind() != "modifiers" {
			continue
		}
		for _, a := range children(c) {
			if a.Kind() == "annotation" {
				name := strings.TrimPrefix(x.text(a), "@")
				if i := strings.IndexAny(name, "(\n "); i >= 0 {
					name = name[:i]
				}
				out[name[strings.LastIndex(name, ".")+1:]] = true
			}
		}
	}
	return out
}

// ktSources are a parameterized test's argument sources, @CsvSource,
// @ValueSource, and the like: its case table.
func (x *extractor) ktSources(fn *sitter.Node) []*sitter.Node {
	var out []*sitter.Node
	for _, c := range children(fn) {
		if c.Kind() != "modifiers" {
			continue
		}
		for _, a := range children(c) {
			name, _, _ := strings.Cut(strings.TrimPrefix(x.text(a), "@"), "(")
			if a.Kind() == "annotation" && strings.HasSuffix(strings.TrimSpace(name), "Source") {
				out = append(out, a)
			}
		}
	}
	return out
}

func ktBody(fn *sitter.Node) *sitter.Node {
	for _, c := range children(fn) {
		if c.Kind() == "function_body" {
			return c
		}
	}
	return fn
}

// ktName is a Kotlin declaration's name: its first identifier child.
func ktName(n *sitter.Node, src []byte) string {
	for _, c := range children(n) {
		if c.Kind() == "simple_identifier" || c.Kind() == "type_identifier" {
			return c.Utf8Text(src)
		}
	}
	return ""
}

// ktCall returns the called name and trailing lambda of a kotest-style
// call, test("name") { … }, or a nil lambda. The arguments and the lambda
// share the call's suffix.
func (x *extractor) ktCall(call *sitter.Node) (string, *sitter.Node) {
	kids := children(call)
	if len(kids) != 2 || kids[0].Kind() != "simple_identifier" || kids[1].Kind() != "call_suffix" {
		return "", nil
	}
	for _, c := range children(kids[1]) {
		if c.Kind() == "annotated_lambda" {
			return x.text(kids[0]), c
		}
	}
	return "", nil
}

// ktTitle is the first argument of a kotest-style call.
func (x *extractor) ktTitle(call *sitter.Node) string {
	for _, suffix := range children(call) {
		if suffix.Kind() != "call_suffix" {
			continue
		}
		for _, args := range children(suffix) {
			if args.Kind() == "value_arguments" && args.NamedChildCount() > 0 {
				return strings.Trim(x.text(args.NamedChild(0)), "\"")
			}
		}
	}
	return ""
}
