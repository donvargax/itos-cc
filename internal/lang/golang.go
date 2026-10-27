package lang

import (
	"bufio"
	"bytes"
	"context"
	"path/filepath"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
	"unsafe"

	sitter "github.com/tree-sitter/go-tree-sitter"
	golang "github.com/tree-sitter/tree-sitter-go/bindings/go"

	"github.com/donvargax/itos-cc/internal/lang/port"
)

// Go units are functions and methods with a body. A method's namespace is
// the package import path plus its receiver type; function literals stay
// inside the enclosing function.
func init() {
	register(&Spec{
		Name:       "go",
		Extensions: []string{".go"},
		Separator:  ".",
		Grammar:    func(string) unsafe.Pointer { return golang.Language() },
		Namespace:  goImportPath,
		Unit: func(n *sitter.Node, src []byte) (string, bool) {
			switch n.Kind() {
			case "function_declaration", "method_declaration":
				return fieldText(n, "name", src), n.ChildByFieldName("body") != nil
			}
			return "", false
		},
		Container: func(*sitter.Node, []byte) (string, bool) { return "", false },
		Receiver:  goReceiver,
		Private: func(_ *sitter.Node, name string, _ []byte) bool {
			r, _ := utf8.DecodeRuneInString(name)
			return !unicode.IsUpper(r)
		},
		Syntax: Syntax{
			Calls:       map[string]string{"call_expression": "function"},
			Members:     map[string]string{"selector_expression": "field"},
			Identifiers: set("identifier", "field_identifier", "type_identifier", "package_identifier"),
			Literals: set("interpreted_string_literal", "raw_string_literal", "int_literal",
				"float_literal", "imaginary_literal", "rune_literal", "true", "false", "nil"),
			Comments: set("comment"),
		},
		Mutations: Mutations{
			Swaps:         swaps(map[string]string{"&&": "||", "||": "&&"}),
			SwapParents:   set("binary_expression"),
			Deletions:     set("!", "-"),
			DeleteParents: set("unary_expression"),
			Literals:      cLikeLiterals,
			LiteralKinds:  set("true", "false", "int_literal"),
		},
		Comment: "//",
		Imports: func(root *sitter.Node, src []byte) []Import {
			var out []Import
			Walk(root, func(n *sitter.Node) bool {
				if n.Kind() == "import_spec" {
					if p := n.ChildByFieldName("path"); p != nil {
						out = append(out, Import{Path: unquote(p.Utf8Text(src)), Line: line(n)})
					}
					return false
				}
				return n.Kind() == "source_file" || n.Kind() == "import_declaration" || n.Kind() == "import_spec_list"
			})
			return out
		},
		Decision: func(n *sitter.Node, src []byte) bool {
			if n.Kind() == "binary_expression" {
				return operatorIn(n, src, "&&", "||")
			}
			return kindIn(n, "if_statement", "for_statement", "expression_case", "type_case",
				"communication_case")
		},
		IsTest: func(path string) bool { return strings.HasSuffix(path, "_test.go") },
	})
}

// goImportPath is the module path from the nearest go.mod joined with the
// file's directory, falling back to the package clause without a go.mod.
// around says what each directory up from the file's holds.
func goImportPath(ctx context.Context, around port.Surroundings, path string, root *sitter.Node, src []byte) string {
	if pkg := GoModulePackage(ctx, around, filepath.Dir(path)); pkg != "" {
		return pkg
	}
	for i := uint(0); i < root.NamedChildCount(); i++ {
		if c := root.NamedChild(i); c.Kind() == "package_clause" {
			return strings.TrimSpace(strings.TrimPrefix(c.Utf8Text(src), "package"))
		}
	}
	return ""
}

// GoModulePackage is the import path of the Go package in dir: the module
// path of the nearest go.mod up from dir that names one, joined with dir's
// folder below it; "" when no go.mod up from dir names a module. around
// says what each directory up from dir holds.
func GoModulePackage(ctx context.Context, around port.Surroundings, dir string) string {
	abs, ok := around.Abs(ctx, dir)
	if !ok {
		return ""
	}
	for d := abs; ; d = filepath.Dir(d) {
		if module := goModule(ctx, around, filepath.Join(d, "go.mod")); module != "" {
			rel, _ := filepath.Rel(d, abs)
			if rel == "." {
				return module
			}
			return module + "/" + filepath.ToSlash(rel)
		}
		if filepath.Dir(d) == d {
			return ""
		}
	}
}

// GoPackages is the import paths of the packages of the Go files among
// sources, sorted, each its GoModulePackage, and whether there is any such
// file. They are none when one of those files has no go.mod up from it
// that names a module: its package's path is unknown.
func GoPackages(ctx context.Context, around port.Surroundings, sources []string) ([]string, bool) {
	var packages []string
	measured := false
	for _, s := range sources {
		if spec := Detect(s); spec == nil || spec.Name != "go" {
			continue
		}
		measured = true
		pkg := GoModulePackage(ctx, around, filepath.Dir(s))
		if pkg == "" {
			return nil, true
		}
		if !slices.Contains(packages, pkg) {
			packages = append(packages, pkg)
		}
	}
	slices.Sort(packages)
	return packages, measured
}

// goModule is the module a go.mod's module line names, or "" when gomod
// cannot be read or has none. Its lines are scanned as they always were, a
// line too long for the scanner ending the scan.
func goModule(ctx context.Context, around port.Surroundings, gomod string) string {
	text, ok := around.Read(ctx, gomod)
	if !ok {
		return ""
	}
	s := bufio.NewScanner(bytes.NewReader(text))
	for s.Scan() {
		if rest, ok := strings.CutPrefix(strings.TrimSpace(s.Text()), "module "); ok {
			return strings.Trim(strings.TrimSpace(rest), `"`)
		}
	}
	return ""
}

// goReceiver returns the receiver type name, without pointer or type
// parameters: `func (b *Board[T]) Place()` is Board.
func goReceiver(n *sitter.Node, src []byte) string {
	if n.Kind() != "method_declaration" {
		return ""
	}
	recv := n.ChildByFieldName("receiver")
	if recv == nil || recv.NamedChildCount() == 0 {
		return ""
	}
	t := recv.NamedChild(0).ChildByFieldName("type")
	if t == nil {
		return ""
	}
	name := strings.TrimPrefix(t.Utf8Text(src), "*")
	if i := strings.IndexByte(name, '['); i >= 0 {
		name = name[:i]
	}
	return name
}
