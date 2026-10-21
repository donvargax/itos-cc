package lang

import (
	"bufio"
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"unsafe"

	dart "github.com/UserNobody14/tree-sitter-dart/bindings/go"
	sitter "github.com/tree-sitter/go-tree-sitter"

	"github.com/donvargax/itos-cc/internal/lang/port"
)

// Dart uses UserNobody14's grammar, the one maintained Dart grammar, pinned
// to a commit as it has no tagged release. It gives a function a signature
// and a body as two siblings, so a unit is a signature followed by its body
// (Spec.Body), starting on the signature's line, never an annotation's.
//
// Dart units are the declarations with a body, a block or =>: functions,
// methods, getters, setters, operators and constructors. Abstract and
// external members and a constructor ending in ; have none, so are not
// units; closures and local functions stay inside the unit around them.
// They are named under the library, the nearest pubspec.yaml's name and the
// file's path under lib/, and any enclosing class, mixin, named extension,
// extension type or enum.
func init() {
	register(&Spec{
		Name:       "dart",
		Extensions: []string{".dart"},
		Separator:  ".",
		Grammar:    func(string) unsafe.Pointer { return dart.Language() },
		Namespace:  dartLibrary,
		Unit:       dartUnit,
		Body:       dartBody,
		Container:  dartContainer,
		Private: func(_ *sitter.Node, name string, _ []byte) bool {
			own := name[strings.LastIndex(name, ".")+1:]
			return strings.HasPrefix(name, "_") || strings.HasPrefix(own, "_")
		},
		Syntax: Syntax{
			Identifiers: set("identifier", "type_identifier"),
			Comments:    set("comment", "documentation_comment"),
		},
		Comment:  "//",
		Decision: dartDecision,
		IsTest:   dartIsTest,
	})
}

// dartSignatures are the signatures a unit starts with: a top-level
// function, getter or setter, or a class-like member's method_signature,
// which wraps a function, getter, setter, operator or constructor's.
var dartSignatures = set("function_signature", "getter_signature", "setter_signature", "method_signature")

// dartUnit names n when it is a signature a body follows. A function_signature
// inside a method_signature or a local function is not one: the method's is,
// and a local function stays in its unit.
func dartUnit(n *sitter.Node, src []byte) (string, bool) {
	if !dartSignatures[n.Kind()] || dartBody(n) == nil {
		return "", false
	}
	if p := n.Parent(); p == nil || n.Kind() != "method_signature" && p.Kind() != "program" {
		return "", false
	}
	sig := n
	if n.Kind() == "method_signature" {
		if sig = dartInnerSignature(n); sig == nil {
			return "", false
		}
	}
	return dartName(sig, src), true
}

// dartBody is the function_body after signature n, past any comment
// between them, or nil when the signature has none.
func dartBody(n *sitter.Node) *sitter.Node {
	for s := n.NextNamedSibling(); s != nil; s = s.NextNamedSibling() {
		switch s.Kind() {
		case "function_body":
			return s
		case "comment", "documentation_comment":
			continue
		}
		return nil
	}
	return nil
}

// dartInnerSignature is the signature a method_signature wraps.
func dartInnerSignature(n *sitter.Node) *sitter.Node {
	for i := uint(0); i < n.NamedChildCount(); i++ {
		if c := n.NamedChild(i); strings.HasSuffix(c.Kind(), "_signature") {
			return c
		}
	}
	return nil
}

// dartName is a signature's unit name: a constructor's C or C.named, a
// setter's s=, an operator's operator+, and otherwise its name.
func dartName(sig *sitter.Node, src []byte) string {
	switch sig.Kind() {
	case "constructor_signature", "factory_constructor_signature":
		var b strings.Builder
		for i := uint(0); i < sig.ChildCount(); i++ {
			c := sig.Child(i)
			if c.Kind() == "formal_parameter_list" {
				break
			}
			if kindIn(c, "identifier", ".", "new") {
				b.WriteString(c.Utf8Text(src))
			}
		}
		return b.String()
	case "operator_signature":
		var b strings.Builder
		after := false
		for i := uint(0); i < sig.ChildCount(); i++ {
			c := sig.Child(i)
			switch {
			case c.Kind() == "formal_parameter_list":
				return b.String()
			case c.Kind() == "operator":
				after = true
				b.WriteString("operator")
			case after:
				b.WriteString(strings.Join(strings.Fields(c.Utf8Text(src)), ""))
			}
		}
		return b.String()
	case "setter_signature":
		return fieldText(sig, "name", src) + "="
	}
	return fieldText(sig, "name", src)
}

// dartContainer names a class, mixin, named extension, extension type or
// enum. An unnamed extension is a container with no name: its members are
// methods in the file's namespace.
func dartContainer(n *sitter.Node, src []byte) (string, bool) {
	switch n.Kind() {
	case "class_definition", "mixin_declaration", "extension_declaration", "extension_type_declaration",
		"enum_declaration":
		if id := firstChildOfKind(n, "identifier"); id != nil {
			return id.Utf8Text(src), true
		}
		return "", n.Kind() == "extension_declaration"
	}
	return "", false
}

// dartDecision counts as DCM does but for a switch's last case: only
// default and the bare _ arm are left out, as in TypeScript and Kotlin.
// Each on or catch handler counts once, so a catch after on T does not; a
// when guard does not, as its case already counts.
func dartDecision(n *sitter.Node, src []byte) bool {
	switch n.Kind() {
	case "on":
		return n.Parent() != nil && n.Parent().Kind() == "try_statement"
	case "catch_clause":
		prev := n.PrevNamedSibling()
		for prev != nil && kindIn(prev, "comment", "documentation_comment") {
			prev = prev.PrevNamedSibling()
		}
		return prev == nil || prev.Kind() == "block"
	case "switch_expression_case":
		pattern, _, _ := strings.Cut(n.Utf8Text(src), "=>")
		return strings.TrimSpace(pattern) != "_"
	case "?..":
		return n.Parent() != nil && n.Parent().Kind() == "cascade_section"
	case "?":
		return n.Parent() != nil && n.Parent().Kind() == "spread_element"
	}
	return kindIn(n, "if_statement", "if_element", "for_statement", "for_element", "while_statement",
		"do_statement", "conditional_expression", "switch_statement_case", "logical_and_operator",
		"logical_or_operator", "if_null_expression", "??=", "conditional_assignable_selector")
}

// dartIsTest covers a _test.dart file anywhere and every file under test/,
// integration_test/ or test_driver/, helpers and flutter_test_config.dart
// among them.
func dartIsTest(path string) bool {
	return strings.HasSuffix(filepath.Base(path), "_test.dart") ||
		underDir(path, "test", "integration_test", "test_driver")
}

// dartLibrary names a file as a package: URI names its library: the nearest
// pubspec.yaml's name, then the path under lib/, or, outside lib/, the path
// from the package directory, dotted. Without a pubspec.yaml the file is
// named by its path, as modulePath names one with no marker.
func dartLibrary(ctx context.Context, around port.Surroundings, path string, _ *sitter.Node, _ []byte) string {
	root := Nearest(ctx, around, path, "pubspec.yaml")
	abs, ok := around.Abs(ctx, path)
	if root == "" || !ok {
		return modulePath(ctx, around, path)
	}
	rel, err := filepath.Rel(root, abs)
	if err != nil {
		return modulePath(ctx, around, path)
	}
	p := strings.TrimPrefix(filepath.ToSlash(strings.TrimSuffix(rel, filepath.Ext(rel))), "lib/")
	return join(pubspecName(ctx, around, filepath.Join(root, "pubspec.yaml")), strings.ReplaceAll(p, "/", "."), ".")
}

// pubspecName is the name a pubspec.yaml's top-level name key gives, or ""
// when it cannot be read or has none.
func pubspecName(ctx context.Context, around port.Surroundings, pubspec string) string {
	text, ok := around.Read(ctx, pubspec)
	if !ok {
		return ""
	}
	s := bufio.NewScanner(bytes.NewReader(text))
	for s.Scan() {
		if rest, ok := strings.CutPrefix(s.Text(), "name:"); ok {
			if i := strings.Index(rest, "#"); i >= 0 {
				rest = rest[:i]
			}
			return strings.Trim(strings.TrimSpace(rest), `"'`)
		}
	}
	return ""
}
