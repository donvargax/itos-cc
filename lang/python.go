package lang

import (
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
			return strings.TrimSuffix(modulePath(path), ".__init__")
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
		// A leading underscore is private; dunder methods such as __init__ are not.
		Private: func(_ *sitter.Node, name string, _ []byte) bool {
			return strings.HasPrefix(name, "_") && !strings.HasSuffix(name, "__")
		},
	})
}
