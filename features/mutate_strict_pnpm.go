package features

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/cucumber/godog"
)

// Strict TypeScript coverage beside pnpm's files (slice-18, issue #48): a
// pnpm-workspace.yaml with no packages key is the package's settings, and a
// pnpm workspace member with no local dependency is proven as a package of
// its own, its workspace root's settings fingerprinted beside it.

// strictPnpmState is what MUT-281's and MUT-282's steps keep: the folder
// the run ran in, from the project root, and the input a step changed with
// its text before.
type strictPnpmState struct {
	q                   *strictMutation
	folder, pkg         string
	changed, changedWas string
}

// strictPnpm* are the fixtures' package: a judged function its Vitest test
// runs whole, as Vitest's LCOV of it says, and the package.json naming the
// pinned Vitest the binary driver links.
const (
	strictPnpmLabel   = "export function label(name: string): string {\n  return name;\n}\n"
	strictPnpmTest    = "import { expect, test } from \"vitest\";\nimport { label } from \"./label\";\n\ntest(\"label\", () => {\n  expect(label(\"value\")).toBe(\"value\");\n});\n"
	strictPnpmReport  = "SF:src/label.ts\nDA:2,1\nend_of_record\n"
	strictPnpmPackage = `{"name": "label", "private": true, "type": "module", ` +
		`"devDependencies": {"vitest": "5.0.2", "@vitest/coverage-v8": "5.0.2"}}` + "\n"
)

// strictPnpmChanges are the workspace root's inputs MUT-282 changes, each
// with its text after the change.
var strictPnpmChanges = map[string]string{
	"pnpm-lock.yaml":      "lockfileVersion: '9.0'\n\nsettings:\n  autoInstallPeers: false\n",
	"pnpm-workspace.yaml": "packages:\n  - packages/*\nminimumReleaseAge: 1440\n",
	"package.json":        `{"name": "repro", "private": true, "description": "changed"}` + "\n",
	"tsconfig.base.json":  `{"compilerOptions": {"strict": false}}` + "\n",
	".npmrc":              "auto-install-peers=true\nsave-exact=true\n",
}

func (q *strictMutation) strictPnpmSteps(sc *godog.ScenarioContext) {
	p := &strictPnpmState{q: q}
	sc.Before(func(ctx context.Context, scenario *godog.Scenario) (context.Context, error) {
		*p = strictPnpmState{q: q}
		for _, tag := range scenario.Tags {
			if tag.Name == "@ID-MUT-281" || tag.Name == "@ID-MUT-282" {
				q.m.w.prepare = func(args []string) []string {
					if len(args) > 1 && args[0] == "mutation" && args[1] == "run" {
						args = append(args, "--no-annotate", "--workers", "2")
					}
					return args
				}
			}
		}
		return ctx, nil
	})
	sc.Step(`^a TypeScript package tested with Vitest whose own pnpm-workspace\.yaml holds "([^"]*)" and no packages key$`, p.settingsPackage)
	sc.Step(`^I run "itos-cc (mutation run --fail-uncovered --json)" for its source$`, func(args string) error { return p.run(".", args) })
	sc.Step(`^no function is "mutation\.coverage-unsupported", and the exit code is 0$`, p.admitted)
	sc.Step(`^"itos-cc mutation check --fail-uncovered --json" exits 0 and runs no test$`, p.freshCheck)
	sc.Step(`^after pnpm-workspace\.yaml's settings change, the check reports "mutation\.coverage-stale" for each judged function, naming pnpm-workspace\.yaml, and runs no test$`, func() error {
		if err := p.change("pnpm-workspace.yaml", "minimumReleaseAge: 2880\n"); err != nil {
			return err
		}
		return p.stale("pnpm-workspace.yaml")
	})
	sc.Step(`^a pnpm workspace whose root lists packages/\* and holds pnpm-lock\.yaml and tsconfig\.base\.json, with the members packages/label, tested with Vitest and depending on no workspace:, file: or link: package, and packages/other$`, p.workspace)
	sc.Step(`^I run "itos-cc (mutation run --fail-uncovered --json)" for packages/label/src/label\.ts, from (the project root|packages/label)$`, func(args, folder string) error {
		if folder == "the project root" {
			folder = "."
		}
		return p.run(folder, args)
	})
	sc.Step(`^after the workspace root's (pnpm-lock\.yaml|pnpm-workspace\.yaml|package\.json|tsconfig\.base\.json|\.npmrc) changes, "itos-cc mutation check --fail-uncovered --json" reports "mutation\.coverage-stale" for each judged function, naming (\S+), and runs no test$`, func(file, named string) error {
		if err := p.change(file, strictPnpmChanges[file]); err != nil {
			return err
		}
		return p.stale(named)
	})
	sc.Step(`^a change to packages/other/src/other\.ts alone leaves its evidence fresh$`, p.siblingFresh)
}

// settingsPackage writes MUT-281's package at the project root, its own
// pnpm-workspace.yaml holding settings alone, and declares what its Vitest
// measures and runs.
func (p *strictPnpmState) settingsPackage(settings string) error {
	p.pkg = "."
	return p.declare(map[string]string{"pnpm-workspace.yaml": settings + "\n"})
}

// workspace writes MUT-282's pnpm workspace, the member packages/label
// tested with Vitest and depending on no local package, and
// packages/other, in a repository, so a run from packages/label has the
// workspace root for its project root.
func (p *strictPnpmState) workspace() error {
	p.pkg = "packages/label"
	files := map[string]string{
		".gitignore":                  "node_modules\ncoverage\n.metrics\n",
		"package.json":                `{"name": "repro", "private": true}` + "\n",
		"pnpm-workspace.yaml":         "packages:\n  - packages/*\n",
		"pnpm-lock.yaml":              "lockfileVersion: '9.0'\n",
		"tsconfig.base.json":          `{"compilerOptions": {"strict": true}}` + "\n",
		".npmrc":                      "auto-install-peers=true\n",
		"packages/other/package.json": `{"name": "other", "private": true, "type": "module"}` + "\n",
		"packages/other/src/other.ts": "export function other(n: number): number {\n  return n;\n}\n",
	}
	if err := p.declare(files); err != nil {
		return err
	}
	if err := p.q.m.w.git("init", "-q"); err != nil {
		return err
	}
	return p.q.s.commit("a pnpm workspace")
}

// declare writes the package at p.pkg beside files and declares what its
// Vitest measures there and how its tests run.
func (p *strictPnpmState) declare(files map[string]string) error {
	q, w := p.q, p.q.m.w
	q.language = "typescript"
	in := func(rel string) string { return path.Join(p.pkg, rel) }
	q.file, q.testFile = in("src/label.ts"), in("src/label.test.ts")
	q.selected = []string{q.file}
	files[in("package.json")] = strictPnpmPackage
	files[q.file] = strictPnpmLabel
	files[q.testFile] = strictPnpmTest
	if err := q.m.writeAll(files); err != nil {
		return err
	}
	w.imports(map[string][]string{q.testFile: {q.file}})
	dir := filepath.Join(w.dir, filepath.FromSlash(p.pkg))
	if err := w.d.Coverage(dir, Coverage{Language: "typescript", Report: strictPnpmReport}); err != nil {
		return err
	}
	return w.d.Tests(dir, Tests{Language: "typescript", Command: vitestCommand + "src/label.ts"})
}

// run runs itos-cc with args from folder, a slash-separated folder of the
// project, for the judged source, named from there.
func (p *strictPnpmState) run(folder, args string) error {
	p.folder = folder
	words, err := commandWords(args)
	if err != nil {
		return err
	}
	w := p.q.m.w
	return w.runEnv(filepath.Join(w.dir, filepath.FromSlash(folder)), w.env(), append(words, p.source())...)
}

// source is the judged source as itos-cc names it from the folder the run
// ran in, slash-separated.
func (p *strictPnpmState) source() string {
	folder := p.folder
	if folder == "" {
		folder = "."
	}
	rel, err := filepath.Rel(filepath.FromSlash(folder), filepath.FromSlash(p.q.file))
	if err != nil {
		return p.q.file
	}
	return filepath.ToSlash(rel)
}

// check runs a strict mutation check of the judged source from the folder
// the run ran in.
func (p *strictPnpmState) check() (map[string]any, string, int, error) {
	w := p.q.m.w
	code, out, log, err := w.capture(filepath.Join(w.dir, filepath.FromSlash(p.folder)), w.env(), "mutation", "check", "--fail-uncovered", "--json", p.source())
	if err != nil {
		return nil, log, code, err
	}
	var a map[string]any
	if err := json.Unmarshal([]byte(out), &a); err != nil {
		return nil, log, code, fmt.Errorf("check output %q: %w\n%s", out, err, log)
	}
	return a, log, code, nil
}

// admitted is whether the last run reported no function as
// mutation.coverage-unsupported, recorded complete line coverage evidence
// for the judged function, and exited 0.
func (p *strictPnpmState) admitted() error {
	q := p.q
	a, err := q.object()
	if err != nil {
		return err
	}
	if ps := q.findings(a, "mutation.coverage-unsupported", ""); len(ps) != 0 {
		return fmt.Errorf("strict coverage refused the package: %v", ps)
	}
	if err := q.m.w.exitsWith(0); err != nil {
		return err
	}
	snap, err := q.snapshot()
	if err != nil {
		return err
	}
	units := objectList(snap, "units")
	if len(units) != 1 || units[0]["name"] != "label" {
		return fmt.Errorf("judged population: %v", snap)
	}
	if e, ok := units[0]["line_coverage"].(map[string]any); !ok || e["complete"] != true {
		return fmt.Errorf("no complete line coverage evidence: %v", units[0])
	}
	return nil
}

// freshCheck is whether a strict check of the judged source exits 0, with
// no strict coverage finding, running no test and writing nothing.
func (p *strictPnpmState) freshCheck() error {
	q := p.q
	if err := q.observed(); err != nil {
		return err
	}
	a, log, code, err := p.check()
	if err != nil {
		return err
	}
	if code != 0 || len(strictLineFacts(a)) != 0 {
		return fmt.Errorf("strict check exit%d: %v\n%s", code, a, log)
	}
	return q.quiet(log)
}

// change writes text to file, from the project root, keeping its text
// before to restore.
func (p *strictPnpmState) change(file, text string) error {
	p.changed, p.changedWas = file, p.q.m.read(file)
	return p.q.m.w.write(file, text)
}

// stale is whether a strict check reports the judged function's evidence
// stale, naming file among the inputs that changed, running no test and
// writing nothing.
func (p *strictPnpmState) stale(file string) error {
	q := p.q
	if err := q.observed(); err != nil {
		return err
	}
	a, log, code, err := p.check()
	if err != nil {
		return err
	}
	ps := q.findings(a, "mutation.coverage-stale", "")
	if code != 1 || len(ps) != 1 || len(strictLineFacts(a)) != 1 || filepath.ToSlash(fmt.Sprint(ps[0]["file"])) != p.source() || !strictNamed(fmt.Sprint(ps[0]["function"]), "label") {
		return fmt.Errorf("stale strict evidence exit%d: %v\n%s", code, a, log)
	}
	_, changed, _ := strings.Cut(fmt.Sprint(ps[0]["message"]), "inputs changed: ")
	if !slices.Contains(strings.Split(changed, ", "), file) {
		return fmt.Errorf("stale evidence of %s does not name %s: %v", q.file, file, ps[0])
	}
	return q.quiet(log)
}

// siblingFresh restores the changed input, changes the sibling member's
// source alone, and is whether a strict check finds the evidence fresh.
func (p *strictPnpmState) siblingFresh() error {
	q := p.q
	if err := q.m.w.write(p.changed, p.changedWas); err != nil {
		return err
	}
	const sibling = "packages/other/src/other.ts"
	if err := q.m.w.write(sibling, q.m.read(sibling)+"\nexport const changed = true;\n"); err != nil {
		return err
	}
	return p.freshCheck()
}
