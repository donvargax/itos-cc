package lang

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"
	"unsafe"

	sitter "github.com/tree-sitter/go-tree-sitter"
	golang "github.com/tree-sitter/tree-sitter-go/bindings/go"
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
func goImportPath(path string, root *sitter.Node, src []byte) string {
	dir, err := filepath.Abs(filepath.Dir(path))
	if err == nil {
		for d := dir; ; d = filepath.Dir(d) {
			if module := goModule(filepath.Join(d, "go.mod")); module != "" {
				rel, _ := filepath.Rel(d, dir)
				if rel == "." {
					return module
				}
				return module + "/" + filepath.ToSlash(rel)
			}
			if filepath.Dir(d) == d {
				break
			}
		}
	}
	for i := uint(0); i < root.NamedChildCount(); i++ {
		if c := root.NamedChild(i); c.Kind() == "package_clause" {
			return strings.TrimSpace(strings.TrimPrefix(c.Utf8Text(src), "package"))
		}
	}
	return ""
}

func goModule(gomod string) string {
	f, err := os.Open(gomod)
	if err != nil {
		return ""
	}
	defer f.Close()
	s := bufio.NewScanner(f)
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
