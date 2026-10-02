// Package lang finds the units every tool measures: the functions and methods
// of a source file, named by namespace and function name.
package lang

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unsafe"

	sitter "github.com/tree-sitter/go-tree-sitter"
)

// Unit is one function or method. Namespace + Name is the join key every
// metrics snapshot uses.
type Unit struct {
	File      string `json:"file"`
	Language  string `json:"language"`
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Kind      string `json:"kind"` // "function" or "method"
	Private   bool   `json:"private"`
	StartLine int    `json:"start_line"`
	EndLine   int    `json:"end_line"`
}

// Spec describes one language to the generic walker.
type Spec struct {
	Name       string
	Extensions []string
	Grammar    func(path string) unsafe.Pointer

	// Namespace of the file, before any container names are appended.
	Namespace func(path string, root *sitter.Node, src []byte) string

	// Unit returns the unit's name when n is a function or method, and false
	// otherwise. The walker does not descend into a unit.
	Unit func(n *sitter.Node, src []byte) (string, bool)

	// Container returns the class-like name n adds to the namespace of the
	// units inside it, and false when n is not a container.
	Container func(n *sitter.Node, src []byte) (string, bool)

	// Receiver returns the type a unit belongs to when the language declares
	// it on the unit itself (Go methods). Nil for languages that nest methods.
	Receiver func(n *sitter.Node, src []byte) string

	// Private reports whether the unit named name, declared by n, is private.
	Private func(n *sitter.Node, name string, src []byte) bool

	// Separator joins namespace segments ("." for most languages).
	Separator string
}

var specs = map[string]*Spec{}

func register(s *Spec) {
	for _, ext := range s.Extensions {
		specs[ext] = s
	}
}

// Detect returns the spec for path, or nil when the language is unsupported.
func Detect(path string) *Spec {
	if strings.HasSuffix(path, ".d.ts") {
		return nil
	}
	return specs[filepath.Ext(path)]
}

// UnitsInFile parses path and returns its units in source order.
func UnitsInFile(path string) ([]Unit, error) {
	spec := Detect(path)
	if spec == nil {
		return nil, fmt.Errorf("unsupported language: %s", path)
	}
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Units(spec, path, src)
}

// Units parses src as spec's language and returns its units in source order.
func Units(spec *Spec, path string, src []byte) ([]Unit, error) {
	parser := sitter.NewParser()
	defer parser.Close()
	if err := parser.SetLanguage(sitter.NewLanguage(spec.Grammar(path))); err != nil {
		return nil, fmt.Errorf("%s: %w", spec.Name, err)
	}
	tree := parser.Parse(src, nil)
	defer tree.Close()
	root := tree.RootNode()

	w := walker{spec: spec, path: path, src: src}
	w.walk(root, spec.Namespace(path, root, src), false)
	return w.units, nil
}

type walker struct {
	spec  *Spec
	path  string
	src   []byte
	units []Unit
}

func (w *walker) walk(n *sitter.Node, ns string, inClass bool) {
	if name, ok := w.spec.Unit(n, w.src); ok {
		w.units = append(w.units, w.unit(n, ns, name, inClass))
		return
	}
	if name, ok := w.spec.Container(n, w.src); ok {
		ns = join(ns, name, w.spec.Separator)
		inClass = true
	}
	for i := uint(0); i < n.NamedChildCount(); i++ {
		w.walk(n.NamedChild(i), ns, inClass)
	}
}

func (w *walker) unit(n *sitter.Node, ns, name string, inClass bool) Unit {
	if w.spec.Receiver != nil {
		if recv := w.spec.Receiver(n, w.src); recv != "" {
			ns = join(ns, recv, w.spec.Separator)
			inClass = true
		}
	}
	kind := "function"
	if inClass {
		kind = "method"
	}
	return Unit{
		File:      w.path,
		Language:  w.spec.Name,
		Namespace: ns,
		Name:      name,
		Kind:      kind,
		Private:   w.spec.Private(n, name, w.src),
		StartLine: int(n.StartPosition().Row) + 1,
		EndLine:   int(n.EndPosition().Row) + 1,
	}
}

func join(ns, name, sep string) string {
	if ns == "" {
		return name
	}
	return ns + sep + name
}

// fieldText returns the text of n's field, or "" when the field is absent.
func fieldText(n *sitter.Node, field string, src []byte) string {
	if c := n.ChildByFieldName(field); c != nil {
		return c.Utf8Text(src)
	}
	return ""
}

// modulePath turns src/demo/board.ts into demo.board, dropping a leading
// src/ or lib/ segment the way crapper names TypeScript and Python modules.
func modulePath(path string) string {
	p := filepath.ToSlash(strings.TrimSuffix(path, filepath.Ext(path)))
	p = strings.TrimPrefix(p, "./")
	for _, root := range []string{"src/", "lib/"} {
		p = strings.TrimPrefix(p, root)
	}
	return strings.ReplaceAll(p, "/", ".")
}
