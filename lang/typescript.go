package lang

import (
	"path/filepath"
	"strings"
	"unsafe"

	sitter "github.com/tree-sitter/go-tree-sitter"
	javascript "github.com/tree-sitter/tree-sitter-javascript/bindings/go"
	typescript "github.com/tree-sitter/tree-sitter-typescript/bindings/go"
)

// TypeScript units are top-level functions, class methods, and top-level
// arrow or function expressions bound to a name. Callbacks stay inside the
// function that encloses them. JavaScript files are TypeScript to every tool:
// they parse with the JavaScript grammar, whose node kinds TypeScript's
// grammar extends.
func init() {
	register(&Spec{
		Name:       "typescript",
		Extensions: []string{".ts", ".tsx", ".mts", ".cts", ".js", ".jsx", ".mjs", ".cjs"},
		Separator:  ".",
		Grammar: func(path string) unsafe.Pointer {
			switch filepath.Ext(path) {
			case ".tsx":
				return typescript.LanguageTSX()
			case ".js", ".jsx", ".mjs", ".cjs":
				return javascript.Language()
			}
			return typescript.LanguageTypescript()
		},
		Namespace: func(path string, _ *sitter.Node, _ []byte) string {
			return modulePath(path, "package.json", "tsconfig.json")
		},
		Unit:      tsUnit,
		Container: tsContainer,
		Private:   tsPrivate,
		Syntax: Syntax{
			Calls:   map[string]string{"call_expression": "function", "new_expression": "constructor"},
			Members: map[string]string{"member_expression": "property"},
			Identifiers: set("identifier", "property_identifier", "private_property_identifier",
				"shorthand_property_identifier", "shorthand_property_identifier_pattern",
				"type_identifier", "statement_identifier"),
			Literals: set("string", "template_string", "number", "regex", "true", "false", "null", "undefined"),
		},
		Mutations: Mutations{
			Swaps:         swaps(map[string]string{"===": "!==", "!==": "===", "&&": "||", "||": "&&", "??": "||"}),
			SwapParents:   set("binary_expression"),
			Deletions:     set("!", "-"),
			DeleteParents: set("unary_expression"),
			Literals:      cLikeLiterals,
			LiteralKinds:  set("true", "false", "number"),
			Other:         tsOptionalChain,
		},
		Comment:  "//",
		Imports:  tsImports,
		Decision: tsDecision,
		BodyLine: tsBodyLine,
		IsTest:   tsIsTest,
	})
}

// tsImports finds import and export-from statements, require(…), and
// import(…) with a literal specifier. Type-only imports count: they are
// still a dependency of the source.
func tsImports(root *sitter.Node, src []byte) []Import {
	var out []Import
	Walk(root, func(n *sitter.Node) bool {
		switch n.Kind() {
		case "import_statement", "export_statement":
			if s := n.ChildByFieldName("source"); s != nil {
				out = append(out, Import{Path: unquote(s.Utf8Text(src)), Line: line(n)})
			}
			return n.Kind() == "export_statement"
		case "call_expression":
			fn := n.ChildByFieldName("function")
			args := n.ChildByFieldName("arguments")
			if fn != nil && (fn.Kind() == "import" || fn.Utf8Text(src) == "require") &&
				args != nil && args.NamedChildCount() == 1 && args.NamedChild(0).Kind() == "string" {
				out = append(out, Import{Path: unquote(args.NamedChild(0).Utf8Text(src)), Line: line(n)})
			}
		}
		return true
	})
	return out
}

func tsDecision(n *sitter.Node, src []byte) bool {
	if n.Kind() == "binary_expression" {
		return operatorIn(n, src, "&&", "||", "??")
	}
	if isOptionalChain(n) {
		return true
	}
	return kindIn(n, "if_statement", "for_statement", "for_in_statement", "while_statement",
		"do_statement", "catch_clause", "ternary_expression", "switch_case")
}

// isOptionalChain is true for the ?. token of a?.b, a?.(), and a?.[0]. The
// TypeScript grammar wraps it in an optional_chain node, or leaves it bare in
// a call; the JavaScript grammar's optional_chain is the token itself.
func isOptionalChain(n *sitter.Node) bool {
	return n.ChildCount() == 0 && (n.Kind() == "?." || n.Kind() == "optional_chain")
}

// tsOptionalChain mutates a?.b to a.b, and drops the ?. of a call or an
// index: a?.() becomes a(), a?.[0] becomes a[0].
func tsOptionalChain(n *sitter.Node) (string, bool) {
	if !isOptionalChain(n) {
		return "", false
	}
	host := n.Parent()
	if host != nil && host.Kind() == "optional_chain" {
		host = host.Parent()
	}
	switch {
	case host == nil:
		return "", false
	case host.Kind() == "member_expression":
		return ".", true
	case host.Kind() == "call_expression" || host.Kind() == "subscript_expression":
		return "", true
	}
	return "", false
}

// tsBodyLine skips the declaration line: loading a module runs
// `export const f = () => {`, so that line is covered without a call.
func tsBodyLine(n *sitter.Node) int {
	if v := n.ChildByFieldName("value"); isFunctionValue(v) {
		n = v
	}
	if body := n.ChildByFieldName("body"); body != nil {
		return firstStatementLine(body)
	}
	return 0
}

func tsIsTest(path string) bool {
	base := filepath.Base(path)
	return strings.Contains(base, ".test.") || strings.Contains(base, ".spec.") ||
		underDir(path, "__tests__", "test", "tests")
}

func tsUnit(n *sitter.Node, src []byte) (string, bool) {
	switch n.Kind() {
	case "function_declaration", "generator_function_declaration", "method_definition":
		return fieldText(n, "name", src), true
	case "public_field_definition":
		if isFunctionValue(n.ChildByFieldName("value")) {
			return fieldText(n, "name", src), true
		}
	case "variable_declarator":
		if isFunctionValue(n.ChildByFieldName("value")) && isTopLevelDeclaration(n) {
			return fieldText(n, "name", src), true
		}
	}
	return "", false
}

func isFunctionValue(v *sitter.Node) bool {
	return v != nil && (v.Kind() == "arrow_function" || v.Kind() == "function_expression")
}

// isTopLevelDeclaration is true for `const f = …` and `export const f = …`
// at module scope.
func isTopLevelDeclaration(declarator *sitter.Node) bool {
	decl := declarator.Parent()
	if decl == nil {
		return false
	}
	scope := decl.Parent()
	if scope != nil && scope.Kind() == "export_statement" {
		scope = scope.Parent()
	}
	return scope != nil && scope.Kind() == "program"
}

func tsContainer(n *sitter.Node, src []byte) (string, bool) {
	switch n.Kind() {
	case "class_declaration", "abstract_class_declaration":
		return fieldText(n, "name", src), true
	}
	return "", false
}

func tsPrivate(n *sitter.Node, _ string, src []byte) bool {
	if name := n.ChildByFieldName("name"); name != nil && name.Kind() == "private_property_identifier" {
		return true
	}
	for i := uint(0); i < n.NamedChildCount(); i++ {
		c := n.NamedChild(i)
		if c.Kind() == "accessibility_modifier" && c.Utf8Text(src) == "private" {
			return true
		}
	}
	return false
}
