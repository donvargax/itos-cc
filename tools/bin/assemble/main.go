// Command assemble writes cmd/itos-cc/commands_gen.go, the assembly of the
// commands the slices under internal/slices declare (ADR-0051), so an
// ordinary new command needs no hand edit outside its slice (T-146).
//
// A slice is a folder right under internal/slices whose own Go files, its
// tests and subfolders left out, hold the two things cmd assembles it by: a
// package-level
//
//	var Command = cli.Declaration{Name: "units", Help: "…", New: …}
//
// whose Name is a string literal, and a type Ports interface, the ports its
// handler is built over, which cmd's adapters satisfy by methods. Beside them
// the slice has NewHandler(ctx context.Context, p Ports) (Handler, error),
// one shape for every slice, so the file holds the same provider for each;
// the compiler, not this tool, holds a slice to that shape and adapters to
// its Ports. A slice with no declaration, a declaration with no Ports, a
// Name that is no string literal and two slices with one name are refused,
// each naming its slice, and nothing is written.
//
// The file imports the slices sorted by path, registers each command with
// kong's DynamicCommand in its name's order, the order the top-level help
// and the guess for an unknown command keep, keeps kong.PostBuild(byName),
// since kong adds a dynamic command after commandLine's fields, and binds
// each slice's handler through one provider over cmd's adapters. It is
// formatted by go/format and carries the standard "Code generated … DO NOT
// EDIT." line, so it is never edited by hand: after adding, removing or
// renaming a slice, run
//
//	go run ./tools/bin/assemble
//
// from the repository's root and commit the file. With -check it writes
// nothing and fails, naming the file, when the committed one differs from
// what it would write, as CI's static step runs it (itos.yaml).
//
// Exit status: 0 written or current, 1 a slice refused or the file not
// current, 2 the tree could not be read or the file written.
package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"io"
	"io/fs"
	"os"
	"path"
	"slices"
	"strconv"
	"strings"
)

const self = "assemble"

// output is the file assemble writes, from the repository's root.
const output = "cmd/itos-cc/commands_gen.go"

// sliceRoot is the folder each slice is a folder of, from the repository's
// root.
const sliceRoot = "internal/slices"

func main() {
	check := flag.Bool("check", false, "write nothing; fail when "+output+" is not what assemble writes")
	flag.Parse()
	os.Exit(run(".", *check, flag.Args(), os.Stderr))
}

// refused is a slice assemble cannot assemble: exit 1, as a stale file is.
type refused struct{ reason string }

func (r refused) Error() string { return r.reason }

// run writes the assembly of the slices of the repository at dir, or with
// check holds the committed file to it, and returns the exit status.
func run(dir string, check bool, args []string, stderr io.Writer) int {
	if len(args) > 0 {
		say(stderr, "%s: unexpected argument %q\n", self, args[0])
		return 2
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		say(stderr, "%s: %v\n", self, err)
		return 2
	}
	defer func() { _ = root.Close() }()
	source, err := assemble(root.FS())
	var r refused
	switch {
	case errors.As(err, &r):
		say(stderr, "%s: %s\n", self, r.reason)
		return 1
	case err != nil:
		say(stderr, "%s: %v\n", self, err)
		return 2
	case check:
		committed, err := root.ReadFile(output)
		if err != nil || !bytes.Equal(committed, source) {
			say(stderr, "%s: %s is not what the slices under %s declare: run go run ./tools/bin/assemble and commit it\n", self, output, sliceRoot)
			return 1
		}
		return 0
	}
	if err := root.WriteFile(output, source, 0o644); err != nil {
		say(stderr, "%s: %v\n", self, err)
		return 2
	}
	return 0
}

func say(w io.Writer, format string, a ...any) { _, _ = fmt.Fprintf(w, format, a...) }

// slice is one slice assemble found: its folder under internal/slices, its
// package's name and the name its declaration gives its command.
type slice struct {
	dir, pkg, name string
}

// assemble is the source of the file over the slices under tree's
// internal/slices, or why one of them is refused.
func assemble(tree fs.FS) ([]byte, error) {
	module, err := modulePath(tree)
	if err != nil {
		return nil, err
	}
	found, err := find(tree, module)
	if err != nil {
		return nil, err
	}
	return render(module, found)
}

// modulePath is the module path go.mod at tree's root names.
func modulePath(tree fs.FS) (string, error) {
	mod, err := fs.ReadFile(tree, "go.mod")
	if err != nil {
		return "", err
	}
	for line := range strings.Lines(string(mod)) {
		if rest, ok := strings.CutPrefix(strings.TrimSpace(line), "module "); ok {
			return strings.Trim(strings.TrimSpace(rest), `"`), nil
		}
	}
	return "", errors.New("go.mod names no module")
}

// find is every slice under tree's internal/slices, in their names' order,
// or why one is refused.
func find(tree fs.FS, module string) ([]slice, error) {
	entries, err := fs.ReadDir(tree, sliceRoot)
	if err != nil {
		return nil, err
	}
	var found []slice
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		s, err := read(tree, module, e.Name())
		if err != nil {
			return nil, err
		}
		if i := slices.IndexFunc(found, func(f slice) bool { return f.name == s.name }); i >= 0 {
			return nil, refused{fmt.Sprintf("%s/%s and %s/%s both declare the command %q: a name runs one command", sliceRoot, found[i].dir, sliceRoot, s.dir, s.name)}
		}
		found = append(found, s)
	}
	slices.SortFunc(found, func(x, y slice) int { return strings.Compare(x.name, y.name) })
	return found, nil
}

// read is the slice in folder dir under internal/slices, from its own Go
// files, or why it is refused.
func read(tree fs.FS, module, dir string) (slice, error) {
	folder := sliceRoot + "/" + dir
	entries, err := fs.ReadDir(tree, folder)
	if err != nil {
		return slice{}, err
	}
	s := slice{dir: dir}
	var declared, ports bool
	fset := token.NewFileSet()
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		file := folder + "/" + e.Name()
		src, err := fs.ReadFile(tree, file)
		if err != nil {
			return slice{}, err
		}
		f, err := parser.ParseFile(fset, file, src, parser.SkipObjectResolution)
		if err != nil {
			return slice{}, err
		}
		s.pkg = f.Name.Name
		name, ok, err := declaration(f, module)
		if err != nil {
			return slice{}, refused{fmt.Sprintf("%s: %v", folder, err)}
		}
		if ok {
			s.name, declared = name, true
		}
		ports = ports || hasPorts(f)
	}
	switch {
	case !declared:
		return slice{}, refused{fmt.Sprintf("%s declares no command: give it a package-level var Command = cli.Declaration{Name: …, Help: …, New: …}", folder)}
	case !ports:
		return slice{}, refused{fmt.Sprintf("%s declares its command but no Ports: give it a type Ports interface of the ports its handler is built over", folder)}
	}
	return s, nil
}

// declaration is the name f's var Command = cli.Declaration{…} gives its
// command, ok false when f holds none, and an error when its Name is no
// string literal.
func declaration(f *ast.File, module string) (string, bool, error) {
	cli := importName(f, module+"/internal/cli")
	for _, d := range f.Decls {
		g, isGen := d.(*ast.GenDecl)
		if !isGen || g.Tok != token.VAR {
			continue
		}
		for _, spec := range g.Specs {
			v := spec.(*ast.ValueSpec)
			if len(v.Names) != 1 || v.Names[0].Name != "Command" || len(v.Values) != 1 {
				continue
			}
			lit, isLit := v.Values[0].(*ast.CompositeLit)
			if !isLit || !isSelector(lit.Type, cli, "Declaration") {
				continue
			}
			for _, el := range lit.Elts {
				kv, isKV := el.(*ast.KeyValueExpr)
				if !isKV {
					continue
				}
				if key, isIdent := kv.Key.(*ast.Ident); !isIdent || key.Name != "Name" {
					continue
				}
				if name, isString := stringOf(kv.Value); isString && name != "" {
					return name, true, nil
				}
			}
			return "", false, errors.New("its Command names its command by no string literal: give cli.Declaration a Name such as Name: \"units\"")
		}
	}
	return "", false, nil
}

// importName is the name f refers to the package at path by, "" when f
// does not import it.
func importName(f *ast.File, importPath string) string {
	for _, spec := range f.Imports {
		if p, _ := strconv.Unquote(spec.Path.Value); p == importPath {
			if spec.Name != nil {
				return spec.Name.Name
			}
			return path.Base(p)
		}
	}
	return ""
}

// isSelector is whether e is pkg.name, pkg not "".
func isSelector(e ast.Expr, pkg, name string) bool {
	sel, ok := e.(*ast.SelectorExpr)
	if !ok || pkg == "" {
		return false
	}
	x, ok := sel.X.(*ast.Ident)
	return ok && x.Name == pkg && sel.Sel.Name == name
}

// stringOf is e's value when e is a string literal.
func stringOf(e ast.Expr) (string, bool) {
	lit, ok := e.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", false
	}
	s, err := strconv.Unquote(lit.Value)
	return s, err == nil
}

// hasPorts is whether f declares type Ports interface{…} at package level.
func hasPorts(f *ast.File) bool {
	for _, d := range f.Decls {
		g, ok := d.(*ast.GenDecl)
		if !ok || g.Tok != token.TYPE {
			continue
		}
		for _, spec := range g.Specs {
			t := spec.(*ast.TypeSpec)
			if _, isInterface := t.Type.(*ast.InterfaceType); isInterface && t.Name.Name == "Ports" {
				return true
			}
		}
	}
	return false
}

// taken are the names the file uses beside the slices' packages: a slice
// whose package has one is imported under an alias.
var taken = []string{"a", "adapters", "assembled", "byName", "context", "ctx", "kong"}

// header is the file's first lines, the standard line go tooling and
// golangci-lint read a generated file by among them.
const header = `// Code generated by tools/bin/assemble from internal/slices; DO NOT EDIT.
// Run go run ./tools/bin/assemble after adding, removing or renaming a slice.

package main

`

// doc is assembled's comment.
const doc = `// assembled are the options that add the command each slice under
// internal/slices declares to the model, a fresh command line each
// (Declaration.New), in their names' order, then put the model's commands in
// their names' order (byName), since kong adds a declared command after
// commandLine's fields, and the providers of each slice's handler over a, as
// the slice's Ports, which kong calls only when the Run it calls asks for its
// value, under that Run's context. A provider returns NewHandler's error as
// it is, so a problem it meets, config.invalid among them, is the one kong
// returns from that command's Run alone, never wrapped as a dependent
// provider's error.
`

// render is the file's source over found, in their names' order, formatted
// by go/format.
func render(module string, found []slice) ([]byte, error) {
	var b strings.Builder
	b.WriteString(header)
	b.WriteString("import (\n")
	if len(found) > 0 {
		b.WriteString("\t\"context\"\n\n")
	}
	b.WriteString("\t\"github.com/donvargax/kong\"\n")
	byPath := slices.Clone(found)
	slices.SortFunc(byPath, func(x, y slice) int { return strings.Compare(x.dir, y.dir) })
	for i, s := range byPath {
		if i == 0 {
			b.WriteString("\n")
		}
		importPath := module + "/" + sliceRoot + "/" + s.dir
		if alias := aliasOf(s); alias != path.Base(importPath) || alias != s.pkg {
			fmt.Fprintf(&b, "\t%s %q\n", alias, importPath)
			continue
		}
		fmt.Fprintf(&b, "\t%q\n", importPath)
	}
	b.WriteString(")\n\n")
	b.WriteString(doc)
	b.WriteString("func assembled(a adapters) []kong.Option {\n\treturn []kong.Option{\n")
	for _, s := range found {
		p := aliasOf(s)
		fmt.Fprintf(&b, "\t\tkong.DynamicCommand(%[1]s.Command.Name, %[1]s.Command.Help, \"\", %[1]s.Command.New()),\n", p)
	}
	b.WriteString("\t\tkong.PostBuild(byName),\n")
	for _, s := range found {
		p := aliasOf(s)
		fmt.Fprintf(&b, "\t\tkong.BindToProvider(func(ctx context.Context) (%[1]s.Handler, error) { return %[1]s.NewHandler(ctx, a) }),\n", p)
	}
	b.WriteString("\t}\n}\n")
	return format.Source([]byte(b.String()))
}

// aliasOf is the name the file refers to s's package by: its own, unless
// the file uses that name for something else.
func aliasOf(s slice) string {
	if slices.Contains(taken, s.pkg) {
		return s.pkg + "slice"
	}
	return s.pkg
}
