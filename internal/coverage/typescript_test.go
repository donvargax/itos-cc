package coverage_test

import (
	"testing"

	"github.com/donvargax/itos-cc/internal/coverage"
)

func TestVitestRunsAPackageThatDeclaresItOrNamesItsResolvableBin(t *testing.T) {
	for _, c := range []struct {
		name string
		p    coverage.TypeScriptPackage
		want bool
	}{
		{"declared", coverage.TypeScriptPackage{Vitest: true}, true},
		{"declared beside jest", coverage.TypeScriptPackage{Vitest: true, Jest: true}, true},
		{"vitest run", coverage.TypeScriptPackage{VitestBin: true, TestScript: "vitest run"}, true},
		{"vp test", coverage.TypeScriptPackage{VitestBin: true, TestScript: "vp test"}, true},
		{"vite", coverage.TypeScriptPackage{VitestBin: true, TestScript: "tsc && vite test"}, true},
		{"by path", coverage.TypeScriptPackage{VitestBin: true, TestScript: "CI=1 ./node_modules/.bin/vitest --run"}, true},
		{"a config beside it", coverage.TypeScriptPackage{VitestBin: true, TestScript: "node --test", Config: true}, true},
		{"hoisted only", coverage.TypeScriptPackage{VitestBin: true, TestScript: "node --test"}, false},
		{"a word that only contains vitest", coverage.TypeScriptPackage{VitestBin: true, TestScript: "vitest-runner && vite-node a.ts"}, false},
		{"no bin", coverage.TypeScriptPackage{TestScript: "vitest run", Config: true}, false},
		{"jest declared", coverage.TypeScriptPackage{Jest: true, VitestBin: true, TestScript: "vitest run", Config: true}, false},
	} {
		if got := c.p.UsesVitest(); got != c.want {
			t.Errorf("%s: UsesVitest(%+v) = %t, want %t", c.name, c.p, got, c.want)
		}
	}
}
