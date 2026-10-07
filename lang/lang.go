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

	// Inner indexes the inline units directly inside this one in its File's
	// Units. Their code is theirs: it adds nothing to this unit's
	// complexity, mutation sites, fingerprint, coverage, or hash.
	Inner []int `json:"-"`
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

	// Inline returns the name of a unit that can sit anywhere, even inside
	// another unit, such as an Express route callback, and false otherwise.
	// Inline units are named under the file's namespace, and a repeated name
	// gets #2, #3, … in source order. Test files have none. Nil when the
	// language has no inline units.
	Inline func(n *sitter.Node, src []byte) (string, bool)

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

	// Syntax names the node kinds tools normalize across languages.
	Syntax Syntax

	// Mutations are the changes mutation testing makes to this language.
	Mutations Mutations

	// Comment starts a line comment.
	Comment string

	// Imports lists what the file imports, as written.
	Imports func(root *sitter.Node, src []byte) []Import
}

// Import is one imported module as the source names it: a relative or
// package specifier in TypeScript, a dotted module in Python (with leading
// dots when relative), an import path in Go, a qualified name in Kotlin.
type Import struct {
	Path string
	// Names are what a Python `from x import a, b` takes from Path; each may
	// be a submodule.
	Names    []string
	Wildcard bool // Kotlin's import a.b.*
	Line     int
}

// Mutations are small changes a test suite should notice. Operator tokens
// are only changed under the parent kinds listed, so `<` in a type argument
// or `-` in a type never is.
type Mutations struct {
	// Swaps maps an operator to the operator that replaces it, under any of
	// SwapParents.
	Swaps       map[string]string
	SwapParents map[string]bool
	// Deletions are unary operators removed under any of DeleteParents:
	// !x becomes x, -x becomes x.
	Deletions     map[string]bool
	DeleteParents map[string]bool
	// Literals maps a literal's text to its replacement, for leaves of
	// LiteralKinds: true and false, 0 and 1.
	Literals     map[string]string
	LiteralKinds map[string]bool
	// Other returns the replacement for a leaf whose mutant depends on more
	// than its text and parent, such as TypeScript's ?., and false for every
	// other leaf. Nil when the language has none.
	Other func(leaf *sitter.Node) (string, bool)
}

// swaps returns the shared operator swaps plus extra.
func swaps(extra map[string]string) map[string]string {
	m := map[string]string{
		"+": "-", "-": "+", "*": "/", "/": "*",
		"<": "<=", "<=": "<", ">": ">=", ">=": ">",
		"==": "!=", "!=": "==",
	}
	for k, v := range extra {
		m[k] = v
	}
	return m
}

var cLikeLiterals = map[string]string{"true": "false", "false": "true", "0": "1", "1": "0"}

// Syntax names a grammar's node kinds for the structure every language
// shares: calls, member access, names, and literal values.
type Syntax struct {
	// Calls maps a call node kind to the field holding the callee, or "" when
	// the callee is the first named child.
	Calls map[string]string
	// Members maps a member-access node kind to the field holding the member
	// name, or "" when the name is the last named child.
	Members map[string]string
	// Identifiers are leaf kinds that name something.
	Identifiers map[string]bool
	// Literals are kinds whose whole subtree is one literal value.
	Literals map[string]bool
}

func set(kinds ...string) map[string]bool {
	m := make(map[string]bool, len(kinds))
	for _, k := range kinds {
		m[k] = true
	}
	return m
}

// IsCallee reports whether the identifier n names the function a call
// invokes: f in f(x), or m in obj.m(x). Single-child wrappers between them,
// such as Kotlin's navigation_suffix, are looked through.
func (s Syntax) IsCallee(n *sitter.Node) bool {
	id := n
	p := n.Parent()
	for p != nil && !s.isMember(p) && !s.isCall(p) && p.NamedChildCount() == 1 {
		n, p = p, p.Parent()
	}
	if p == nil {
		return false
	}
	if field, ok := s.Members[p.Kind()]; ok {
		if !sameNode(s.memberName(p, field), id) {
			return false
		}
		n, p = p, p.Parent()
		if p == nil {
			return false
		}
	}
	field, ok := s.Calls[p.Kind()]
	return ok && sameNode(callee(p, field), n)
}

func (s Syntax) isMember(n *sitter.Node) bool { _, ok := s.Members[n.Kind()]; return ok }
func (s Syntax) isCall(n *sitter.Node) bool   { _, ok := s.Calls[n.Kind()]; return ok }

// Callee splits a call into the object it is made on, if any, and the
// called name: ("http", "Get") for http.Get(u), ("", "fetch") for fetch(u).
// A call made on a call's result, a.b().c(), has the object text a.b().
func (s Syntax) Callee(n *sitter.Node, src []byte) (object, name string, ok bool) {
	field, isCall := s.Calls[n.Kind()]
	if !isCall {
		return "", "", false
	}
	fn := callee(n, field)
	// Kotlin's trailing lambda nests the call: get("/x") { } is
	// call(call(get, args), lambda).
	if fn != nil && fn.Kind() == n.Kind() && field == "" {
		fn = callee(fn, field)
	}
	if fn == nil {
		return "", "", false
	}
	if s.Identifiers[fn.Kind()] {
		return "", fn.Utf8Text(src), true
	}
	memberField, isMember := s.Members[fn.Kind()]
	if !isMember || fn.NamedChildCount() == 0 {
		return "", "", false
	}
	member := s.memberName(fn, memberField)
	if member == nil {
		return "", "", false
	}
	return fn.NamedChild(0).Utf8Text(src), member.Utf8Text(src), true
}

func callee(call *sitter.Node, field string) *sitter.Node {
	if field != "" {
		return call.ChildByFieldName(field)
	}
	if call.NamedChildCount() == 0 {
		return nil
	}
	return call.NamedChild(0)
}

// memberName is the name in a member access: the field, or the last named
// child, unwrapped down to the identifier inside it.
func (s Syntax) memberName(member *sitter.Node, field string) *sitter.Node {
	var name *sitter.Node
	if field != "" {
		name = member.ChildByFieldName(field)
	} else if n := member.NamedChildCount(); n > 0 {
		name = member.NamedChild(n - 1)
	}
	for name != nil && !s.Identifiers[name.Kind()] && name.NamedChildCount() > 0 {
		name = name.NamedChild(name.NamedChildCount() - 1)
	}
	return name
}

func sameNode(a, b *sitter.Node) bool {
	return a != nil && b != nil && a.Id() == b.Id()
}

var specs = map[string]*Spec{}

func register(s *Spec) {
	for _, ext := range s.Extensions {
		specs[ext] = s
	}
}

// Detect returns the spec for path, or nil when the language is unsupported.
func Detect(path string) *Spec {
	// Declarations and minified bundles are not code anyone wrote here.
	if strings.HasSuffix(path, ".d.ts") || strings.HasSuffix(path, ".min.js") {
		return nil
	}
	return specs[filepath.Ext(path)]
}

// File is a parsed source file. Close it to free the syntax tree.
type File struct {
	Path      string
	Spec      *Spec
	Src       []byte
	Units     []Unit
	Root      *sitter.Node
	Namespace string // the file's own namespace, before any class

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
	ns := spec.Namespace(path, root, src)
	w := walker{spec: spec, path: path, src: src, ns: ns, inline: spec.Inline,
		names: map[string]int{}}
	if spec.IsTest(path) {
		w.inline = nil
	}
	w.walk(root, ns, false)
	return &File{Path: path, Spec: spec, Src: src, Units: w.units, Root: root, Namespace: ns, tree: tree}, nil
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
	spec   *Spec
	path   string
	src    []byte
	ns     string // the file's namespace, which inline units are named under
	inline func(n *sitter.Node, src []byte) (string, bool)
	names  map[string]int // inline names so far, to number repeats
	units  []Unit
}

func (w *walker) walk(n *sitter.Node, ns string, inClass bool) {
	if name, ok := w.inlineName(n); ok {
		w.units = append(w.units, w.unit(n, w.ns, name, false))
		w.walkInner(n, len(w.units)-1)
		return
	}
	if name, ok := w.spec.Unit(n, w.src); ok {
		w.units = append(w.units, w.unit(n, ns, name, inClass))
		w.walkInner(n, len(w.units)-1)
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

// walkInner finds the inline units below n, which is inside the unit at
// index outer, and records the outermost of them as outer's Inner.
func (w *walker) walkInner(n *sitter.Node, outer int) {
	if w.inline == nil {
		return
	}
	for i := uint(0); i < n.NamedChildCount(); i++ {
		c := n.NamedChild(i)
		if name, ok := w.inlineName(c); ok {
			w.units = append(w.units, w.unit(c, w.ns, name, false))
			w.units[outer].Inner = append(w.units[outer].Inner, len(w.units)-1)
			w.walkInner(c, len(w.units)-1)
			continue
		}
		w.walkInner(c, outer)
	}
}

func (w *walker) inlineName(n *sitter.Node) (string, bool) {
	if w.inline == nil {
		return "", false
	}
	name, ok := w.inline(n, w.src)
	if !ok {
		return "", false
	}
	w.names[name]++
	if k := w.names[name]; k > 1 {
		name = fmt.Sprintf("%s#%d", name, k)
	}
	return name, true
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
// it, including decisions in callbacks and closures it contains, but not in
// its inline units.
func (f *File) Complexity(u Unit) int {
	cc := 1
	f.WalkOwn(u, func(n *sitter.Node) bool {
		if n != u.Node && f.Spec.Decision(n, f.Src) {
			cc++
		}
		return true
	})
	return cc
}

// WalkOwn walks u's own code like Walk, skipping its inline units.
func (f *File) WalkOwn(u Unit, visit func(*sitter.Node) bool) {
	inner := f.InnerNodes(u)
	Walk(u.Node, func(n *sitter.Node) bool {
		for _, c := range inner {
			if sameNode(n, c) {
				return false
			}
		}
		return visit(n)
	})
}

// InnerNodes are the syntax trees of u's inline units, in source order.
func (f *File) InnerNodes(u Unit) []*sitter.Node {
	out := make([]*sitter.Node, len(u.Inner))
	for i, j := range u.Inner {
		out[i] = f.Units[j].Node
	}
	return out
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

// Imports lists what the file imports.
func (f *File) Imports() []Import {
	if f.Spec.Imports == nil {
		return nil
	}
	return f.Spec.Imports(f.Root, f.Src)
}

func line(n *sitter.Node) int { return int(n.StartPosition().Row) + 1 }

func unquote(s string) string { return strings.Trim(s, "\"'`") }

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

// firstStatementLine is the line of the first statement in body, skipping
// comments, or the line body starts on when it holds no statement or is an
// expression.
func firstStatementLine(body *sitter.Node) int {
	for i := uint(0); i < body.NamedChildCount(); i++ {
		if c := body.NamedChild(i); c.Kind() != "comment" {
			return int(c.StartPosition().Row) + 1
		}
	}
	return int(body.StartPosition().Row) + 1
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
// package.json is demo.board. Without a marker the working directory stands
// in for that directory, so the name never depends on where the checkout
// lives; a file above the working directory drops the ../ of its path, and
// one the working directory has no path to is named by its file name.
func modulePath(path string, markers ...string) string {
	rel := path
	if root := FindUp(path, markers...); root != "" {
		if abs, err := filepath.Abs(path); err == nil {
			if r, err := filepath.Rel(root, abs); err == nil {
				rel = r
			}
		}
	} else if abs, err := filepath.Abs(path); err == nil {
		rel = filepath.Base(path)
		if wd, err := os.Getwd(); err == nil {
			if r, err := filepath.Rel(wd, abs); err == nil {
				rel = r
			}
		}
	}
	p := filepath.ToSlash(strings.TrimSuffix(rel, filepath.Ext(rel)))
	for strings.HasPrefix(p, "../") {
		p = strings.TrimPrefix(p, "../")
	}
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
