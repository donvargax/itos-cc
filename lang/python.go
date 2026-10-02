package lang

import (
	"path/filepath"
	"strings"
	"unsafe"

	sitter "github.com/tree-sitter/go-tree-sitter"
	python "github.com/tree-sitter/tree-sitter-python/bindings/go"
)

// Python units are functions and methods. Functions nested inside another
// function stay inside it.
func init() {
	register(&Spec{
		Name:       "python",
		Extensions: []string{".py"},
		Separator:  ".",
		Grammar:    func(string) unsafe.Pointer { return python.Language() },
		Namespace: func(path string, _ *sitter.Node, _ []byte) string {
			return strings.TrimSuffix(modulePath(path, "pyproject.toml", "setup.py", "setup.cfg"), ".__init__")
		},
		Unit: func(n *sitter.Node, src []byte) (string, bool) {
			if n.Kind() == "function_definition" {
				return fieldText(n, "name", src), true
			}
			return "", false
		},
		Container: func(n *sitter.Node, src []byte) (string, bool) {
			if n.Kind() == "class_definition" {
				return fieldText(n, "name", src), true
			}
			return "", false
		},
		Syntax: Syntax{
			Calls:       map[string]string{"call": "function"},
			Members:     map[string]string{"attribute": "attribute"},
			Identifiers: set("identifier"),
			Literals:    set("string", "concatenated_string", "integer", "float", "true", "false", "none"),
		},
		Mutations: Mutations{
			Swaps:         swaps(map[string]string{"and": "or", "or": "and"}),
			SwapParents:   set("binary_operator", "comparison_operator", "boolean_operator"),
			Deletions:     set("not", "-"),
			DeleteParents: set("not_operator", "unary_operator"),
			Literals:      map[string]string{"True": "False", "False": "True", "0": "1", "1": "0"},
			LiteralKinds:  set("true", "false", "integer"),
		},
		Comment: "#",
		Decision: func(n *sitter.Node, _ []byte) bool {
			return kindIn(n, "if_statement", "elif_clause", "for_statement", "while_statement",
				"except_clause", "case_clause", "conditional_expression", "boolean_operator",
				"for_in_clause", "if_clause")
		},
		BodyLine: func(n *sitter.Node) int {
			if body := n.ChildByFieldName("body"); body != nil {
				return int(body.StartPosition().Row) + 1
			}
			return 0
		},
		IsTest: func(path string) bool {
			base := filepath.Base(path)
			return base == "conftest.py" || strings.HasPrefix(base, "test_") ||
				strings.HasSuffix(base, "_test.py") || underDir(path, "test", "tests")
		},
		// A leading underscore is private; dunder methods such as __init__ are not.
		Private: func(_ *sitter.Node, name string, _ []byte) bool {
			return strings.HasPrefix(name, "_") && !strings.HasSuffix(name, "__")
		},
	})
}
