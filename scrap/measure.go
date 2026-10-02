package scrap

import (
	"strings"
	"unicode"
	"unicode/utf8"

	sitter "github.com/tree-sitter/go-tree-sitter"

	"itos-cc/lang"
)

// Metrics are what one example is measured by.
type Metrics struct {
	Lines      int `json:"lines"`      // code lines, not counting multi-line string fixtures
	RawLines   int `json:"raw_lines"`  // every line the example spans
	Decisions  int `json:"decisions"`  // branches and loops the test itself takes
	Assertions int `json:"assertions"` // expect, assert, t.Error, shouldBe, assertion helpers, …
	Mocks      int `json:"mocks"`      // mocks, spies, and patches it sets up
}

// measurer classifies the nodes of one test file.
type measurer struct {
	f *lang.File
	// helpers are functions of the file that assert: calling one is an
	// assertion, however the helper is named.
	helpers map[string]bool
}

func newMeasurer(f *lang.File) *measurer {
	m := &measurer{f: f, helpers: map[string]bool{}}
	// Twice, so a helper that only calls another helper is found too.
	for range 2 {
		for _, u := range f.Units {
			if m.helpers[u.Name] {
				continue
			}
			lang.Walk(u.Node, func(n *sitter.Node) bool {
				if n != u.Node && m.isAssertion(n) {
					m.helpers[u.Name] = true
				}
				return !m.helpers[u.Name]
			})
		}
	}
	return m
}

// measure walks an example. An assertion is counted once and not looked
// into, so `assert a and b` asserts rather than branches; a loop over cases
// whose body asserts is a hand-written table, not logic.
func (m *measurer) measure(ex Example) (Metrics, bool) {
	raw := ex.EndLine - ex.StartLine + 1
	met := Metrics{RawLines: raw, Lines: raw - m.fixtureLines(ex.Node)}
	table := false
	lang.Walk(ex.Node, func(n *sitter.Node) bool {
		switch {
		case m.isAssertion(n):
			met.Assertions++
			return false
		case m.isGuardAssertion(n):
			met.Assertions += m.count(n.ChildByFieldName("consequence"), m.isAssertion)
			return false
		case m.isMock(n):
			met.Mocks++
		case m.isTableLoop(n):
			table = true
		case m.f.Spec.Decision(n, m.f.Src):
			met.Decisions++
		}
		return true
	})
	return met, table
}

func (m *measurer) count(n *sitter.Node, match func(*sitter.Node) bool) int {
	c := 0
	if n == nil {
		return 0
	}
	lang.Walk(n, func(x *sitter.Node) bool {
		if match(x) {
			c++
			return false
		}
		return true
	})
	return c
}

// fixtureLines are lines inside multi-line string literals: test data, not
// test code.
func (m *measurer) fixtureLines(n *sitter.Node) int {
	lines := 0
	lang.Walk(n, func(x *sitter.Node) bool {
		if m.f.Spec.Syntax.Literals[x.Kind()] {
			lines += int(x.EndPosition().Row - x.StartPosition().Row)
			return false
		}
		return true
	})
	return lines
}

// callee splits a call into the object it is called on, if any, and the
// called name: ("vi", "fn") for vi.fn(), ("", "expect") for expect(x).
func (m *measurer) callee(n *sitter.Node) (object, name string, ok bool) {
	object, name, ok = m.f.Spec.Syntax.Callee(n, m.f.Src)
	if object != "" {
		object = lastSegment(object)
	}
	return object, name, ok
}

// lastSegment keeps the final name of a dotted object: patch for
// mock.patch, so patch.object reads the same however patch was imported.
func lastSegment(s string) string {
	if i := strings.LastIndexAny(s, ".)"); i >= 0 && i < len(s)-1 {
		return s[i+1:]
	}
	return s
}

// assertive reports whether a name reads as an assertion helper: assert,
// expect, check, verify, or must, alone or followed by a word boundary, so
// checkLimits counts and checkout does not.
func assertive(name string) bool {
	for _, prefix := range []string{"assert", "expect", "check", "verify", "must"} {
		rest, ok := strings.CutPrefix(name, prefix)
		if !ok {
			rest, ok = strings.CutPrefix(name, strings.ToUpper(prefix[:1])+prefix[1:])
		}
		if !ok {
			continue
		}
		r, _ := utf8.DecodeRuneInString(rest)
		if rest == "" || r == '_' || unicode.IsUpper(r) {
			return true
		}
	}
	return false
}

var goFailures = map[string]bool{"Error": true, "Errorf": true, "Fatal": true, "Fatalf": true, "Fail": true, "FailNow": true}

func (m *measurer) isAssertion(n *sitter.Node) bool {
	switch m.f.Spec.Name {
	case "python":
		if n.Kind() == "assert_statement" {
			return true
		}
	case "kotlin":
		if n.Kind() == "infix_expression" && n.NamedChildCount() == 3 {
			return strings.HasPrefix(n.NamedChild(1).Utf8Text(m.f.Src), "should")
		}
	}
	object, name, ok := m.callee(n)
	if !ok {
		return false
	}
	if object == "" && (m.helpers[name] || assertive(name)) {
		return true
	}
	switch m.f.Spec.Name {
	case "typescript":
		return object == "expect" || object == "assert"
	case "python":
		return assertive(name) || (object == "pytest" && (name == "raises" || name == "warns"))
	case "go":
		return goFailures[name] || object == "assert" || object == "require"
	case "kotlin":
		return assertive(name) || name == "coVerify" || strings.HasPrefix(name, "should")
	}
	return false
}

var (
	tsMockCalls = map[string]bool{"mock": true, "fn": true, "spyOn": true, "doMock": true,
		"stubGlobal": true, "stubEnv": true, "mocked": true}
	pyMocks = map[string]bool{"patch": true, "MagicMock": true, "Mock": true, "AsyncMock": true,
		"create_autospec": true, "PropertyMock": true}
	ktMocks = map[string]bool{"mockk": true, "spyk": true, "every": true, "coEvery": true, "mock": true,
		"spy": true, "whenever": true, "`when`": true, "mockkObject": true, "mockkStatic": true,
		"mockkConstructor": true}
)

func (m *measurer) isMock(n *sitter.Node) bool {
	object, name, ok := m.callee(n)
	if !ok {
		return false
	}
	switch m.f.Spec.Name {
	case "typescript":
		return ((object == "vi" || object == "jest") && tsMockCalls[name]) ||
			(object != "" && strings.HasPrefix(name, "mock"))
	case "python":
		return pyMocks[name] || object == "patch" || object == "mocker" || object == "monkeypatch"
	case "go":
		return name == "EXPECT" || name == "NewController" || strings.HasPrefix(name, "NewMock")
	case "kotlin":
		return ktMocks[name]
	}
	return false
}

// isGuardAssertion is an if whose only job is to fail the test, Go's
// assertion idiom: `if got != want { t.Errorf(…) }`. Its condition is part
// of the assertion, not logic.
func (m *measurer) isGuardAssertion(n *sitter.Node) bool {
	if n.Kind() != "if_statement" || n.ChildByFieldName("alternative") != nil {
		return false
	}
	block := n.ChildByFieldName("consequence")
	if block == nil {
		return false
	}
	stmts := children(block)
	if len(stmts) > 0 && stmts[0].Kind() == "statement_list" {
		stmts = children(stmts[0])
	}
	if len(stmts) == 0 {
		return false
	}
	for _, s := range stmts {
		if s.Kind() != "expression_statement" || s.NamedChildCount() == 0 || !m.isAssertion(s.NamedChild(0)) {
			return false
		}
	}
	return true
}

// loopKinds iterate over a collection, the shape of a hand-written table.
var loopKinds = map[string]bool{"for_statement": true, "for_in_statement": true}

// isTableLoop is a loop over cases whose body asserts or runs a subtest:
// the table of a table-driven test, not logic in a test.
func (m *measurer) isTableLoop(n *sitter.Node) bool {
	if !loopKinds[n.Kind()] {
		return false
	}
	if m.f.Spec.Name == "go" && !hasChild(n, "range_clause") {
		return false
	}
	found := false
	lang.Walk(n, func(x *sitter.Node) bool {
		if found {
			return false
		}
		if _, name, ok := m.callee(x); ok && name == "Run" {
			found = true
		}
		found = found || m.isAssertion(x) || m.isGuardAssertion(x)
		return true
	})
	return found
}

func hasChild(n *sitter.Node, kind string) bool {
	for _, c := range children(n) {
		if c.Kind() == kind {
			return true
		}
	}
	return false
}
