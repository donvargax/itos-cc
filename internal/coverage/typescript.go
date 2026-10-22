package coverage

import (
	"path"
	"strings"
	"unicode"
)

// TypeScriptPackage is what decides whether Vitest runs a TypeScript
// package's tests, as its runner finds it (slice-17, issue #45).
type TypeScriptPackage struct {
	// Vitest and Jest are whether its package.json declares them, in its
	// dependencies or devDependencies.
	Vitest, Jest bool
	// VitestBin is whether a vitest resolves from its node_modules/.bin,
	// or one above it, as an installed tool is found.
	VitestBin bool
	// TestScript is its package.json's test script.
	TestScript string
	// Config is whether one of ViteConfigs sits beside its package.json.
	Config bool
}

// ViteConfigs are the config files of Vite and Vitest, in each extension
// they load, whose presence beside a package.json says Vitest runs its
// tests.
var ViteConfigs = func() []string {
	var names []string
	for _, base := range []string{"vite.config", "vitest.config"} {
		for _, ext := range []string{"js", "mjs", "cjs", "ts", "mts", "cts"} {
			names = append(names, base+"."+ext)
		}
	}
	return names
}()

// UsesVitest is whether Vitest runs the package's tests: it declares
// vitest, or, declaring no jest, a vitest resolves for it and its test
// script names vitest, vp or vite, or a Vite or Vitest config sits beside
// its package.json. Vitest that comes in through another package, such as
// a toolchain's, is never named in package.json; a resolvable bin alone is
// not enough, since npm and yarn hoist every transitive dependency's bin.
// No toolchain is known by name.
func (p TypeScriptPackage) UsesVitest() bool {
	return p.Vitest || !p.Jest && p.VitestBin && (p.Config || namesVitest(p.TestScript))
}

// namesVitest is whether script runs a program named vitest, vp or vite,
// by name or by path.
func namesVitest(script string) bool {
	words := strings.FieldsFunc(script, func(r rune) bool {
		return unicode.IsSpace(r) || strings.ContainsRune(";&|()<>'\"`", r)
	})
	for _, word := range words {
		switch path.Base(strings.ReplaceAll(word, `\`, "/")) {
		case "vitest", "vp", "vite":
			return true
		}
	}
	return false
}
