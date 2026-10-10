package lang

import (
	"path/filepath"
	"strings"
	"unsafe"

	kotlin "github.com/fwcd/tree-sitter-kotlin/bindings/go"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

// Kotlin uses fwcd's grammar: the tree-sitter-grammars one misreads a class
// with several annotations and no constructor, losing the class and its
// methods. fwcd's grammar names few fields, so names are found by child
// kind.
//
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
				return ktName(n, src), true
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
			Identifiers: set("simple_identifier", "type_identifier"),
			Literals: set("string_literal", "integer_literal", "real_literal", "long_literal", "hex_literal",
				"bin_literal", "unsigned_literal", "boolean_literal", "character_literal", "null_literal"),
		},
		Mutations: Mutations{
			Swaps: swaps(map[string]string{"===": "!==", "!==": "===", "&&": "||", "||": "&&"}),
			SwapParents: set("additive_expression", "multiplicative_expression", "comparison_expression",
				"equality_expression", "conjunction_expression", "disjunction_expression"),
			Deletions:     set("!", "-"),
			DeleteParents: set("prefix_expression"),
			Literals:      cLikeLiterals,
			// boolean_literal wraps a true or false token.
			LiteralKinds: set("true", "false", "integer_literal"),
		},
		Comment:  "//",
		Imports:  ktImports,
		Decision: ktDecision,
		IsTest:   ktIsTest,
	})
}

// ktName is a declaration's name: the first identifier among its children,
// after any modifiers and receiver type.
func ktName(n *sitter.Node, src []byte) string {
	for i := uint(0); i < n.NamedChildCount(); i++ {
		if c := n.NamedChild(i); c.Kind() == "simple_identifier" || c.Kind() == "type_identifier" {
			return c.Utf8Text(src)
		}
	}
	return ""
}

// ktDecision counts the safe call ?. like the elvis ?:, since a?.b is null
// when a is.
func ktDecision(n *sitter.Node, src []byte) bool {
	if n.Kind() == "when_entry" {
		return !strings.HasPrefix(n.Utf8Text(src), "else")
	}
	return kindIn(n, "if_expression", "for_statement", "while_statement", "do_while_statement",
		"catch_block", "conjunction_expression", "disjunction_expression", "elvis_expression", "?.")
}

// ktIsTest covers Gradle and Maven's src/test/ through its test directory.
func ktIsTest(path string) bool {
	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	return underDir(path, "test", "tests") ||
		strings.HasSuffix(base, "Test") || strings.HasSuffix(base, "Tests") || strings.HasSuffix(base, "Spec")
}

func ktPackage(_ string, root *sitter.Node, src []byte) string {
	if h := firstChildOfKind(root, "package_header"); h != nil {
		if id := firstChildOfKind(h, "identifier"); id != nil {
			return id.Utf8Text(src)
		}
	}
	return ""
}

func ktContainer(n *sitter.Node, src []byte) (string, bool) {
	switch n.Kind() {
	case "class_declaration", "object_declaration":
		return ktName(n, src), true
	case "companion_object":
		if name := ktName(n, src); name != "" {
			return name, true
		}
		return "Companion", true
	}
	return "", false
}

func ktHasModifier(n *sitter.Node, modifier string, src []byte) bool {
	if m := firstChildOfKind(n, "modifiers"); m != nil {
		for _, word := range strings.Fields(m.Utf8Text(src)) {
			if word == modifier {
				return true
			}
		}
	}
	return false
}

func ktImports(root *sitter.Node, src []byte) []Import {
	var out []Import
	Walk(root, func(n *sitter.Node) bool {
		switch n.Kind() {
		case "source_file", "import_list":
			return true
		case "import_header":
			imp := Import{Line: line(n), Wildcard: firstChildOfKind(n, "wildcard_import") != nil}
			if id := firstChildOfKind(n, "identifier"); id != nil {
				imp.Path = id.Utf8Text(src)
				out = append(out, imp)
			}
		}
		return false
	})
	return out
}

// TopLevelNames are the classes, objects, interfaces, functions, and type
// aliases a Kotlin file declares at the top level: what other files import.
func TopLevelNames(f *File) []string {
	var out []string
	for i := uint(0); i < f.Root.NamedChildCount(); i++ {
		n := f.Root.NamedChild(i)
		if kindIn(n, "class_declaration", "object_declaration", "function_declaration", "type_alias") {
			if name := ktName(n, f.Src); name != "" {
				out = append(out, name)
			}
		}
	}
	return out
}

func firstChildOfKind(n *sitter.Node, kind string) *sitter.Node {
	for i := uint(0); i < n.NamedChildCount(); i++ {
		if c := n.NamedChild(i); c.Kind() == kind {
			return c
		}
	}
	return nil
}

// ReferencedNames are the names a Kotlin file uses outside its package and
// import lines. Code in the same package needs no import, so a graph finds
// those dependencies by matching these names to what the package declares.
func ReferencedNames(f *File) []string {
	seen := map[string]bool{}
	var out []string
	Walk(f.Root, func(n *sitter.Node) bool {
		switch n.Kind() {
		case "package_header", "import_list", "import_header":
			return false
		case "simple_identifier", "type_identifier":
			if name := n.Utf8Text(f.Src); !seen[name] {
				seen[name] = true
				out = append(out, name)
			}
			return false
		}
		return true
	})
	return out
}

// TestClasses are the runnable test classes a Kotlin file declares, by fully
// qualified name, in the order it declares them: each top-level class, not
// abstract and not an interface, that a JUnit or Kotest runner would run,
// which is one that
//   - declares a function, in its body or a nested class's, annotated as a
//     test: @Test, an annotation whose name ends in Test (@ParameterizedTest,
//     @RepeatedTest) or names Test through an import alias, @TestFactory or
//     @TestTemplate;
//   - extends a Kotest spec, a supertype whose name ends in Spec; or
//   - extends a supertype and is named as Gradle and Surefire name test
//     classes, ending in Test or Tests: a subclass of an abstract test class
//     inherits its tests and declares none.
//
// A test-support file, such as a helper object or class in src/test that
// declares no test, has none.
func TestClasses(f *File) []string {
	// alias holds the names test annotations are imported as.
	alias := map[string]bool{}
	Walk(f.Root, func(n *sitter.Node) bool {
		switch n.Kind() {
		case "source_file", "import_list":
			return true
		case "import_header":
			id, as := firstChildOfKind(n, "identifier"), firstChildOfKind(n, "import_alias")
			if id == nil || as == nil {
				return false
			}
			path := id.Utf8Text(f.Src)
			if name := firstChildOfKind(as, "type_identifier"); name != nil && ktTestAnnotation(path[strings.LastIndex(path, ".")+1:]) {
				alias[name.Utf8Text(f.Src)] = true
			}
		}
		return false
	})
	prefix := ""
	if f.Namespace != "" {
		prefix = f.Namespace + "."
	}
	var out []string
	for i := uint(0); i < f.Root.NamedChildCount(); i++ {
		n := f.Root.NamedChild(i)
		if n.Kind() != "class_declaration" || ktIsInterface(n) || ktHasModifier(n, "abstract", f.Src) {
			continue
		}
		if name := ktName(n, f.Src); name != "" && (ktDeclaresTest(n, f.Src, alias) || ktRunnableBySupertype(n, name, f.Src)) {
			out = append(out, prefix+name)
		}
	}
	return out
}

// ktTestAnnotation says whether an annotation of this simple name marks a
// test function.
func ktTestAnnotation(name string) bool {
	return strings.HasSuffix(name, "Test") || name == "TestFactory" || name == "TestTemplate"
}

// ktIsInterface says whether class_declaration n declares an interface.
func ktIsInterface(n *sitter.Node) bool {
	for i := uint(0); i < n.ChildCount(); i++ {
		if n.Child(i).Kind() == "interface" {
			return true
		}
	}
	return false
}

// ktDeclaresTest says whether class n declares, in its body or a nested
// class's, a function annotated as a test.
func ktDeclaresTest(n *sitter.Node, src []byte, alias map[string]bool) bool {
	found := false
	Walk(n, func(c *sitter.Node) bool {
		if found {
			return false
		}
		switch c.Kind() {
		case "class_declaration", "class_body":
			return true
		case "function_declaration":
			found = ktTestAnnotated(c, src, alias)
		}
		return false
	})
	return found
}

// ktTestAnnotated says whether function n carries a test annotation.
func ktTestAnnotated(n *sitter.Node, src []byte, alias map[string]bool) bool {
	m := firstChildOfKind(n, "modifiers")
	if m == nil {
		return false
	}
	for i := uint(0); i < m.NamedChildCount(); i++ {
		a := m.NamedChild(i)
		if a.Kind() != "annotation" {
			continue
		}
		t := firstChildOfKind(a, "user_type")
		if t == nil {
			if call := firstChildOfKind(a, "constructor_invocation"); call != nil {
				t = firstChildOfKind(call, "user_type")
			}
		}
		if t == nil || t.NamedChildCount() == 0 {
			continue
		}
		if name := t.NamedChild(t.NamedChildCount() - 1).Utf8Text(src); ktTestAnnotation(name) || alias[name] {
			return true
		}
	}
	return false
}

// ktRunnableBySupertype says whether class n, named name, is a test by what
// it extends: a Kotest spec, or any supertype when name ends in Test or
// Tests.
func ktRunnableBySupertype(n *sitter.Node, name string, src []byte) bool {
	named := strings.HasSuffix(name, "Test") || strings.HasSuffix(name, "Tests")
	for i := uint(0); i < n.NamedChildCount(); i++ {
		d := n.NamedChild(i)
		if d.Kind() != "delegation_specifier" {
			continue
		}
		if named {
			return true
		}
		t := firstChildOfKind(d, "user_type")
		if t == nil {
			if call := firstChildOfKind(d, "constructor_invocation"); call != nil {
				t = firstChildOfKind(call, "user_type")
			}
		}
		if t != nil && t.NamedChildCount() > 0 && strings.HasSuffix(t.NamedChild(t.NamedChildCount()-1).Utf8Text(src), "Spec") {
			return true
		}
	}
	return false
}
