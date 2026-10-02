package lang

import (
	"path/filepath"
	"strings"
	"unsafe"

	kotlin "github.com/tree-sitter-grammars/tree-sitter-kotlin/bindings/go"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

// Kotlin units are functions and methods, named under the file's package and
// any enclosing class, object, or companion object. Local functions stay
// inside the function that declares them.
func init() {
	register(&Spec{
		Name:       "kotlin",
		Extensions: []string{".kt", ".kts"},
		Separator:  ".",
		Grammar:    func(string) unsafe.Pointer { return kotlin.Language() },
		Namespace:  ktPackage,
		Unit: func(n *sitter.Node, src []byte) (string, bool) {
			if n.Kind() == "function_declaration" {
				return fieldText(n, "name", src), true
			}
			return "", false
		},
		Container: ktContainer,
		Private: func(n *sitter.Node, _ string, src []byte) bool {
			return ktHasModifier(n, "private", src)
		},
		Syntax: Syntax{
			Calls:       map[string]string{"call_expression": ""},
			Members:     map[string]string{"navigation_expression": ""},
			Identifiers: set("identifier"),
			Literals: set("string_literal", "multiline_string_literal", "number_literal",
				"float_literal", "character_literal", "true", "false", "null"),
		},
		Decision: ktDecision,
		IsTest:   ktIsTest,
	})
}

func ktDecision(n *sitter.Node, src []byte) bool {
	switch n.Kind() {
	case "binary_expression":
		return operatorIn(n, src, "&&", "||", "?:")
	case "when_entry":
		return !strings.HasPrefix(n.Utf8Text(src), "else")
	}
	return kindIn(n, "if_expression", "for_statement", "while_statement", "do_while_statement",
		"catch_block")
}

func ktIsTest(path string) bool {
	p := filepath.ToSlash(path)
	base := strings.TrimSuffix(filepath.Base(p), filepath.Ext(p))
	return strings.Contains(p, "/src/test/") || strings.HasPrefix(p, "src/test/") ||
		underDir(path, "test", "tests") ||
		strings.HasSuffix(base, "Test") || strings.HasSuffix(base, "Tests") || strings.HasSuffix(base, "Spec")
}

func ktPackage(_ string, root *sitter.Node, src []byte) string {
	for i := uint(0); i < root.NamedChildCount(); i++ {
		c := root.NamedChild(i)
		if c.Kind() == "package_header" {
			return strings.TrimSpace(strings.TrimPrefix(c.Utf8Text(src), "package"))
		}
	}
	return ""
}

func ktContainer(n *sitter.Node, src []byte) (string, bool) {
	switch n.Kind() {
	case "class_declaration", "object_declaration":
		return fieldText(n, "name", src), true
	case "companion_object":
		if name := fieldText(n, "name", src); name != "" {
			return name, true
		}
		return "Companion", true
	}
	return "", false
}

func ktHasModifier(n *sitter.Node, modifier string, src []byte) bool {
	for i := uint(0); i < n.NamedChildCount(); i++ {
		c := n.NamedChild(i)
		if c.Kind() != "modifiers" {
			continue
		}
		for _, word := range strings.Fields(c.Utf8Text(src)) {
			if word == modifier {
				return true
			}
		}
	}
	return false
}
