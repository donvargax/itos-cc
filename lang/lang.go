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

	// BodyLine is the first line whose coverage belongs to the unit. It is
	// StartLine except where defining the unit executes its first lines.
	BodyLine int `json:"-"`

	// Node is the unit's syntax tree. It is valid until its File is closed.
	Node *sitter.Node `json:"-"`
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

	// Decision reports whether n adds a path through a unit: a branch, a
	// loop, a catch, a case, or a short-circuit operator.
	Decision func(n *sitter.Node, src []byte) bool

	// BodyLine returns the first line of n whose coverage reflects calls to
	// the unit, or 0 to use the unit's first line. Python needs it: importing
	// a module executes every def line, so those lines are always covered.
	BodyLine func(n *sitter.Node) int

	// IsTest reports whether path is test code rather than production code.
	IsTest func(path string) bool

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

// File is a parsed source file. Close it to free the syntax tree.
type File struct {
	Path  string
	Spec  *Spec
	Src   []byte
	Units []Unit
	Root  *sitter.Node

	tree *sitter.Tree
}

// Close frees the syntax tree. Units' nodes are invalid afterwards.
func (f *File) Close() {
	if f.tree != nil {
		f.tree.Close()
		f.tree = nil
	}
}

// ParseFile reads and parses path.
func ParseFile(path string) (*File, error) {
	spec := Detect(path)
	if spec == nil {
		return nil, fmt.Errorf("unsupported language: %s", path)
	}
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Parse(spec, path, src)
}

// Parse parses src as spec's language. path names the file and decides its
// namespace; it does not have to exist unless the language reads a project
// file such as go.mod to name it.
func Parse(spec *Spec, path string, src []byte) (*File, error) {
	tree, err := ParseTree(spec, path, src)
	if err != nil {
		return nil, err
	}
	root := tree.RootNode()
	w := walker{spec: spec, path: path, src: src}
	w.walk(root, spec.Namespace(path, root, src), false)
	return &File{Path: path, Spec: spec, Src: src, Units: w.units, Root: root, tree: tree}, nil
}

// ParseTree parses src without finding units. The caller closes the tree.
func ParseTree(spec *Spec, path string, src []byte) (*sitter.Tree, error) {
	parser := sitter.NewParser()
	defer parser.Close()
	if err := parser.SetLanguage(sitter.NewLanguage(spec.Grammar(path))); err != nil {
		return nil, fmt.Errorf("%s: %w", spec.Name, err)
	}
	tree := parser.Parse(src, nil)
	if tree == nil {
		return nil, fmt.Errorf("%s: parse failed", path)
	}
	return tree, nil
}

// UnitsInFile parses path and returns its units without their nodes.
func UnitsInFile(path string) ([]Unit, error) {
	f, err := ParseFile(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	units := f.Units
	for i := range units {
		units[i].Node = nil
	}
	return units, nil
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
	start := int(n.StartPosition().Row) + 1
	body := start
	if w.spec.BodyLine != nil {
		if line := w.spec.BodyLine(n); line > 0 {
			body = line
		}
	}
	return Unit{
		BodyLine:  body,
		File:      w.path,
		Language:  w.spec.Name,
		Namespace: ns,
		Name:      name,
		Kind:      kind,
		Private:   w.spec.Private(n, name, w.src),
		StartLine: start,
		EndLine:   int(n.EndPosition().Row) + 1,
		Node:      n,
	}
}

// Complexity is the cyclomatic complexity of u: one plus each decision inside
// it, including decisions in callbacks and closures it contains.
func (f *File) Complexity(u Unit) int {
	cc := 1
	Walk(u.Node, func(n *sitter.Node) bool {
		if n != u.Node && f.Spec.Decision(n, f.Src) {
			cc++
		}
		return true
	})
	return cc
}

// Walk visits n and its descendants depth-first, in source order. Returning
// false from visit skips that node's children.
func Walk(n *sitter.Node, visit func(*sitter.Node) bool) {
	if !visit(n) {
		return
	}
	for i := uint(0); i < n.ChildCount(); i++ {
		Walk(n.Child(i), visit)
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

// operatorIn reports whether n's operator field is one of ops.
func operatorIn(n *sitter.Node, src []byte, ops ...string) bool {
	op := fieldText(n, "operator", src)
	for _, o := range ops {
		if op == o {
			return true
		}
	}
	return false
}

// kindIn reports whether n's kind is one of kinds.
func kindIn(n *sitter.Node, kinds ...string) bool {
	k := n.Kind()
	for _, want := range kinds {
		if k == want {
			return true
		}
	}
	return false
}

// modulePath names a module by its path from the nearest directory holding
// one of markers, without a leading src/ or lib/: src/demo/board.ts under
// package.json is demo.board. Without a marker the path is used as given.
func modulePath(path string, markers ...string) string {
	rel := path
	if root := FindUp(path, markers...); root != "" {
		if abs, err := filepath.Abs(path); err == nil {
			if r, err := filepath.Rel(root, abs); err == nil {
				rel = r
			}
		}
	}
	p := filepath.ToSlash(strings.TrimSuffix(rel, filepath.Ext(rel)))
	p = strings.TrimPrefix(p, "./")
	for _, dir := range []string{"src/", "lib/"} {
		p = strings.TrimPrefix(p, dir)
	}
	return strings.ReplaceAll(p, "/", ".")
}

// FindUp returns the nearest directory at or above path's directory that
// contains one of markers, or "" when none does.
func FindUp(path string, markers ...string) string {
	dir, err := filepath.Abs(filepath.Dir(path))
	if err != nil {
		return ""
	}
	for {
		for _, m := range markers {
			if _, err := os.Stat(filepath.Join(dir, m)); err == nil {
				return dir
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// underDir reports whether any directory in path is one of dirs.
func underDir(path string, dirs ...string) bool {
	parts := strings.Split(filepath.ToSlash(filepath.Dir(path)), "/")
	for _, p := range parts {
		for _, d := range dirs {
			if p == d {
				return true
			}
		}
	}
	return false
}
