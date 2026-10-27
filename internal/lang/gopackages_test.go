package lang

import (
	"slices"
	"testing"

	"github.com/donvargax/itos-cc/internal/lang/port/porttest"
)

// The packages a listed tests' Go conversion is limited to (slice-65): each
// Go file's package is the module path of the nearest go.mod up from it
// that names one, joined with its folder; a file with none leaves the
// packages unknown.
func TestGoPackagesNameEachGoFilesPackageByItsNearestModule(t *testing.T) {
	files := map[string]string{
		"/m/go.mod":         "module example.com/m\n\ngo 1.22\n",
		"/m/main.go":        "package main\n",
		"/m/a/a.go":         "package a\n",
		"/m/a/b.go":         "package a\n",
		"/m/sub/go.mod":     "module example.com/sub\n",
		"/m/sub/c/c.go":     "package c\n",
		"/m/empty/go.mod":   "go 1.22\n",
		"/m/empty/d/d.go":   "package d\n",
		"/m/web/label.ts":   "export {}\n",
		"/loose/x/loose.go": "package x\n",
	}
	around := porttest.Surroundings{Dir: "/m", Files: files}
	for _, c := range []struct {
		name     string
		sources  []string
		want     []string
		measured bool
	}{
		{"a package, its files once", []string{"/m/a/b.go", "/m/a/a.go"}, []string{"example.com/m/a"}, true},
		{"a main package at the module's root", []string{"/m/main.go"}, []string{"example.com/m"}, true},
		{"a nested module, sorted", []string{"/m/sub/c/c.go", "/m/a/a.go"}, []string{"example.com/m/a", "example.com/sub/c"}, true},
		{"a go.mod naming no module is passed by", []string{"/m/empty/d/d.go"}, []string{"example.com/m/empty/d"}, true},
		{"a relative path, from the working directory", []string{"a/a.go"}, []string{"example.com/m/a"}, true},
		{"a file with no go.mod above it", []string{"/m/a/a.go", "/loose/x/loose.go"}, nil, true},
		{"no Go file", []string{"/m/web/label.ts", "/m/README"}, nil, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			got, measured := GoPackages(t.Context(), around, c.sources)
			if !slices.Equal(got, c.want) || measured != c.measured {
				t.Errorf("GoPackages(%q) = %q, %v; want %q, %v", c.sources, got, measured, c.want, c.measured)
			}
		})
	}
}

func TestGoModulePackageIsUnknownWhereNoPathCanBeMadeAbsolute(t *testing.T) {
	around := porttest.Surroundings{Files: map[string]string{"/m/go.mod": "module example.com/m\n"}}
	if got := GoModulePackage(t.Context(), around, "a"); got != "" {
		t.Errorf("GoModulePackage of a relative folder with no working directory = %q, want none", got)
	}
}
