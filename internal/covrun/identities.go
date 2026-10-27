package covrun

import (
	"context"
	"path/filepath"
	"sort"
	"strings"

	"github.com/donvargax/itos-cc/internal/coverage"
	"github.com/donvargax/itos-cc/internal/lang"
	"github.com/donvargax/itos-cc/internal/project"
)

// planKey is a plan's group of sources: its language, its build root and,
// for a Python, Dart or Kotlin source measured by the tests that reach it,
// those tests as PythonTests or DartTests runs them, or the test classes
// of its own module KotlinTests finds in them, joined, each after a NUL: a
// group of its own, one of no test unreached.
type planKey struct{ lang, dir, tests string }

// groups is sources by the plan that measures each, as Plans makes them.
func groups(ctx context.Context, sources []string, scope Scope, reach Reach) map[planKey][]string {
	narrow := scope == OwnTests && reach != nil
	out := map[planKey][]string{}
	for _, s := range sources {
		spec := lang.Detect(s)
		if spec == nil {
			continue
		}
		dir := project.FindUp(ctx, s, markers[spec.Name]...)
		if dir == "" {
			dir = filepath.Dir(s)
		}
		k := planKey{spec.Name, dir, ""}
		switch {
		case spec.Name == "python" && narrow:
			k.tests = "\x00" + strings.Join(PythonTests(dir, reach(s)), "\x00")
		case spec.Name == "kotlin" && narrow:
			k.tests = "\x00" + strings.Join(KotlinTests(ctx, reach(s))[dir], "\x00")
		case spec.Name == "dart" && narrow:
			k.tests = "\x00" + strings.Join(DartTests(dir, reach(s)), "\x00")
		}
		out[k] = append(out[k], s)
	}
	return out
}

// Identities are the plans Plans makes of sources, as the coverage domain
// names them (coverage.Plan), without composing their commands, sorted as
// Plans sorts them.
func Identities(ctx context.Context, sources []string, scope Scope, reach Reach) []coverage.Plan {
	var out []coverage.Plan
	for k, sources := range groups(ctx, sources, scope, reach) {
		p := coverage.Plan{Language: k.lang, Dir: k.dir, Sources: sources}
		if k.tests != "" {
			p.Tests = strings.Split(k.tests, "\x00")[1:]
			if p.Tests[0] == "" {
				p.Tests, p.Unreached = nil, true
			}
		}
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool {
		if a, b := out[i].Language+out[i].Dir, out[j].Language+out[j].Dir; a != b {
			return a < b
		}
		return out[i].Sources[0] < out[j].Sources[0]
	})
	return out
}
