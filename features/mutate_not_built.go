package features

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"github.com/cucumber/godog"
)

// The steps of the Rule "Code only another system builds is not judged on
// this one" (slice-25, issue #18). The fixture is a Go package whose
// function Probe is split by a build constraint into a file only another
// system builds and one this system builds. "Another system" is named from
// the driver's platform, windows on Linux and macOS and linux on Windows,
// and another architecture from the host's, so every system runs them. The
// steps read the run's --json object, its stderr and the snapshots it
// leaves, never its code.

// notBuiltState is one such scenario's state beside mutateState's.
type notBuiltState struct {
	m *mutateState
	// other is the file only another system builds, built the file this
	// one builds; excluded is the GOOS or GOARCH the other names, which
	// the built file rules out, and line the line of the other's Probe.
	other, built, excluded string
	line                   int
	// args is the command line the When ran, for the But to run again.
	args string
	// selected are the files a counted run selected a site in, one for
	// each seed it ran with.
	selected []string
}

const (
	// notBuiltModule is the fixture's go.mod.
	notBuiltModule = "module example.com/probe\n\ngo 1.22\n"
	// notBuiltTest reaches Probe as this system builds it, killing its
	// one mutant there.
	notBuiltTest = "package probe\n\nimport \"testing\"\n\nfunc TestProbe(t *testing.T) {\n\tif !Probe(4) || Probe(3) {\n\t\tt.Fatal(\"Probe\")\n\t}\n}\n"
	// notBuiltOther is Probe as another system builds it, below its
	// header: on that system the test kills its mutants but `>` → `>=`.
	notBuiltOther = "package probe\n\nfunc Probe(n int) bool { return n > 2 && n%2 == 0 }\n"
	// notBuiltHere is Probe as this system builds it, below its
	// constraint, its one site on line 5.
	notBuiltHere = "package probe\n\nfunc Probe(n int) bool { return n > 3 }\n"
	// notBuiltHereProfile is go test's coverprofile of notBuiltHere: its
	// one block, executed.
	notBuiltHereProfile = "mode: set\nexample.com/probe/probe_here.go:5.26,5.40 1 1\n"
)

func notBuiltSteps(sc *godog.ScenarioContext, m *mutateState) {
	n := &notBuiltState{m: m}
	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		*n = notBuiltState{m: m}
		return ctx, nil
	})
	sc.Step(`^a Go package whose function Probe is split by (its file name's suffix|a //go:build line naming another system|a //go:build line naming another architecture) into a file only another system builds and a file this one builds, whose test reaches the second$`, n.split)
	sc.Step(`^I run "itos-cc ([^"]*)" for both files$`, func(args string) error {
		n.args = args + " " + n.other + " " + n.built
		return n.run(n.args)
	})
	sc.Step(`^the first is in "not_built" with this system's platform, and not in "files"$`, n.notBuiltHere)
	sc.Step(`^stderr says it is not built on this system's platform, so not judged here$`, n.saysNotBuilt)
	sc.Step(`^the second's mutants are judged as its tests decide, and the exit code is 0$`, n.builtJudged)
	sc.Step(`^when the second also branches on runtime\.GOOS for another system, its mutants on that branch are uncovered, and the exit code is 1$`, n.branching)
	sc.Step(`^the package of MUT-304, its snapshots committed, and a commit since main that changes only the file another system builds$`, n.changedOther)
	sc.Step(`^that file is in "not_built", and no problem names it$`, n.unjudged)
	sc.Step(`^the package of MUT-304, where the snapshot of the file another system builds records killed mutants and one survivor, as a run on that system recorded them$`, n.recordedElsewhere)
	sc.Step(`^that file is in "not_built", no problem names it, and the exit code is 0$`, func() error {
		if err := n.unjudged(); err != nil {
			return err
		}
		return m.w.exitsWith(0)
	})
	sc.Step(`^"itos-cc mutation sample --count 20" runs none of its mutants and exits 0$`, n.sampleSkips)
	sc.Step(`^"itos-cc mutation run" leaves its snapshot byte for byte, and names no orphan$`, n.snapshotKept)
	sc.Step(`^the package of MUT-304, committed, whose only eligible sites are in the file another system builds and in a function of the other file$`, n.committed)
	sc.Step(`^a count-one run judges it, with several seeds$`, n.counted)
	sc.Step(`^no selected site is in the file another system builds$`, n.noneSelected)
}

// otherSystem is the system the fixture names as another: windows, or
// linux on Windows.
func (n *notBuiltState) otherSystem() string {
	if n.m.w.d.Platform() == "windows" {
		return "linux"
	}
	return "windows"
}

// otherArchitecture is the architecture the fixture names as another:
// arm64, or amd64 on arm64.
func otherArchitecture() string {
	if runtime.GOARCH == "arm64" {
		return "amd64"
	}
	return "arm64"
}

// platform is this system's, as go env names it: GOOS/GOARCH.
func (n *notBuiltState) platform() string {
	return n.m.w.d.Platform() + "/" + runtime.GOARCH
}

// run runs itos-cc with args in the scratch folder.
func (n *notBuiltState) run(args string) error {
	return n.m.w.runWith(n.m.w.env(), args)
}

// split writes the package, its Probe split by constraint, and declares
// what its tests do and what they cover.
func (n *notBuiltState) split(constraint string) error {
	header := ""
	switch constraint {
	case "its file name's suffix":
		n.excluded = n.otherSystem()
		n.other = "probe_" + n.excluded + ".go"
	case "a //go:build line naming another system":
		n.excluded = n.otherSystem()
		n.other, header = "probe_other.go", "//go:build "+n.excluded
	case "a //go:build line naming another architecture":
		n.excluded = otherArchitecture()
		n.other, header = "probe_other.go", "//go:build "+n.excluded
	default:
		return fmt.Errorf("no package is split by %s", constraint)
	}
	n.built = "probe_here.go"
	return n.write(header, "//go:build !"+n.excluded)
}

// write writes the module with the other file under header, none when it
// is "", and the built file under builtHeader, then declares their tests.
func (n *notBuiltState) write(header, builtHeader string) error {
	other := notBuiltOther
	n.line = 3
	if header != "" {
		other = header + "\n\n" + other
		n.line = 5
	}
	if err := n.m.writeAll(map[string]string{"go.mod": notBuiltModule, "probe_test.go": notBuiltTest,
		n.other: other, n.built: builtHeader + "\n\n" + notBuiltHere}); err != nil {
		return err
	}
	return n.declare(n.otherSites(), notBuiltHereProfile)
}

// otherSites are the sites of the other file's Probe.
func (n *notBuiltState) otherSites() []Site {
	return []Site{{n.other, n.line, ">", ">="}, {n.other, n.line, "&&", "||"}, {n.other, n.line, "==", "!="}, {n.other, n.line, "0", "1"}}
}

// declare says the package's tests survive the mutants of surviving and
// kill every other, and that go test measures profile.
func (n *notBuiltState) declare(surviving []Site, profile string) error {
	w := n.m.w
	if err := w.d.Tests(w.dir, Tests{Language: "go", Command: "go test -count=1 -failfast ./.", Surviving: surviving}); err != nil {
		return err
	}
	return w.d.Coverage(w.dir, Coverage{Language: "go", Report: profile})
}

// notBuiltOf is the "not_built" list of a --json object, by its files,
// slash-separated, each with its platform.
func notBuiltOf(object map[string]any) map[string]string {
	out := map[string]string{}
	for _, entry := range objectList(object, "not_built") {
		file, _ := entry["file"].(string)
		platform, _ := entry["platform"].(string)
		out[filepath.ToSlash(file)] = platform
	}
	return out
}

// inFiles is whether the "files" list of a --json object holds file.
func inFiles(object map[string]any, file string) bool {
	for _, entry := range objectList(object, "files") {
		if got, _ := entry["file"].(string); filepath.ToSlash(got) == file {
			return true
		}
	}
	return false
}

// notBuiltHere is whether the last run lists the other file in
// "not_built" with this system's platform, and not in "files".
func (n *notBuiltState) notBuiltHere() error {
	object, err := n.m.w.json()
	if err != nil {
		return err
	}
	if got, ok := notBuiltOf(object)[n.other]; !ok || got != n.platform() {
		return fmt.Errorf("\"not_built\" does not list %s with platform %s: %v\n%s", n.other, n.platform(), object["not_built"], n.m.w.report())
	}
	if inFiles(object, n.other) {
		return fmt.Errorf("\"files\" lists %s\n%s", n.other, n.m.w.report())
	}
	return nil
}

// saysNotBuilt is whether stderr says the other file is not built here.
func (n *notBuiltState) saysNotBuilt() error {
	want := fmt.Sprintf("itos-cc: not built on %s, so %s is not judged here", n.platform(), n.other)
	if !strings.Contains(n.m.w.stderr, want) {
		return fmt.Errorf("stderr does not say %q\n%s", want, n.m.w.report())
	}
	return nil
}

// builtJudged is whether every mutant of the built file is killed, as its
// test decides, and the run exited 0.
func (n *notBuiltState) builtJudged() error {
	object, err := n.m.w.json()
	if err != nil {
		return err
	}
	mutants, err := notBuiltMutants(object, n.built)
	if err != nil {
		return fmt.Errorf("%v\n%s", err, n.m.w.report())
	}
	for _, mu := range mutants {
		if mu["outcome"] != "killed" {
			return fmt.Errorf("the mutant %v is not killed, as its test decides\n%s", mu, n.m.w.report())
		}
	}
	return n.m.w.exitsWith(0)
}

// notBuiltMutants is the mutants a --json object lists of file, at least
// one.
func notBuiltMutants(object map[string]any, file string) ([]map[string]any, error) {
	for _, entry := range objectList(object, "files") {
		if got, _ := entry["file"].(string); filepath.ToSlash(got) == file {
			mutants := objectList(entry, "mutants")
			if len(mutants) == 0 {
				return nil, fmt.Errorf("%s lists no mutant", file)
			}
			return mutants, nil
		}
	}
	return nil, fmt.Errorf("\"files\" does not list %s", file)
}

// branching makes the built file branch on runtime.GOOS for the other
// system, runs the When's command again, and holds the branch's mutant
// uncovered: no test on this system executes its line. With
// --no-coverage, which measures nothing, it runs and survives, as nothing
// executes it. The other file is still not built here, and the run fails.
func (n *notBuiltState) branching() error {
	condition := fmt.Sprintf("if runtime.GOOS == %q {", n.otherSystem())
	source := fmt.Sprintf("//go:build !%s\n\npackage probe\n\nimport \"runtime\"\n\nfunc Probe(n int) bool {\n\t%s\n\t\treturn n > 2\n\t}\n\treturn n > 3\n}\n", n.excluded, condition)
	if err := n.m.w.write(n.built, source); err != nil {
		return err
	}
	branch := Site{n.built, 9, ">", ">="}
	profile := fmt.Sprintf("mode: set\nexample.com/probe/probe_here.go:8.2,8.%d 1 1\nexample.com/probe/probe_here.go:9.3,10.1 1 0\nexample.com/probe/probe_here.go:11.2,11.14 1 1\n", len(condition)+2)
	if err := n.declare(append(n.otherSites(), branch), profile); err != nil {
		return err
	}
	if err := n.run(n.args); err != nil {
		return err
	}
	if err := n.notBuiltHere(); err != nil {
		return err
	}
	object, err := n.m.w.json()
	if err != nil {
		return err
	}
	mutants, err := notBuiltMutants(object, n.built)
	if err != nil {
		return fmt.Errorf("%v\n%s", err, n.m.w.report())
	}
	want := "uncovered"
	if slices.Contains(strings.Fields(n.args), "--no-coverage") {
		want = "survived"
	}
	found := false
	for _, mu := range mutants {
		if mu["line"] == float64(branch.Line) {
			found = true
			if mu["outcome"] != want {
				return fmt.Errorf("the branch's mutant %v is not %s\n%s", mu, want, n.m.w.report())
			}
		}
	}
	if !found {
		return fmt.Errorf("no mutant is listed on the branch's line %d\n%s", branch.Line, n.m.w.report())
	}
	return n.m.w.exitsWith(1)
}

// commit commits every file of the scratch folder with message.
func (n *notBuiltState) commit(message string) error {
	if err := n.m.w.git("add", "-A"); err != nil {
		return err
	}
	return n.m.w.git("commit", "-qm", message)
}

// recordRun runs args, which must exit with code.
func (n *notBuiltState) recordRun(args string, code int) error {
	if err := n.run(args); err != nil {
		return err
	}
	if n.m.w.exit != code {
		return fmt.Errorf("the fixture's run of itos-cc %s exited %d, not %d\n%s", args, n.m.w.exit, code, n.m.w.report())
	}
	return nil
}

// changedOther writes the package split by its file name's suffix, records
// and commits its snapshots on main, then commits on a branch a change to
// the other file alone.
func (n *notBuiltState) changedOther() error {
	if err := n.split("its file name's suffix"); err != nil {
		return err
	}
	if err := n.m.w.git("init", "-q", "-b", "main"); err != nil {
		return err
	}
	if err := n.recordRun("mutation run --no-annotate", 0); err != nil {
		return err
	}
	if err := n.commit("probe"); err != nil {
		return err
	}
	if err := n.m.w.git("checkout", "-qb", "work"); err != nil {
		return err
	}
	if err := n.m.w.write(n.other, strings.Replace(notBuiltOther, "n > 2", "n > 1", 1)); err != nil {
		return err
	}
	return n.commit("change the other system's probe")
}

// unjudged is whether the last run lists the other file in "not_built",
// and no problem it reports names it.
func (n *notBuiltState) unjudged() error {
	object, err := n.m.w.json()
	if err != nil {
		return err
	}
	if _, ok := notBuiltOf(object)[n.other]; !ok {
		return fmt.Errorf("\"not_built\" does not list %s: %v\n%s", n.other, object["not_built"], n.m.w.report())
	}
	for _, p := range checkProblems(object) {
		if file, _ := p["file"].(string); filepath.ToSlash(file) == n.other {
			return fmt.Errorf("a problem names %s: %v\n%s", n.other, p, n.m.w.report())
		}
	}
	return nil
}

// snapshotOf is the path of file's snapshot, slash-separated from the
// scratch folder.
func snapshotOf(file string) string { return ".metrics/mutate/" + file + ".json" }

// recordedElsewhere writes the package split by a //go:build line naming
// another system and records the other file's snapshot as a run on that
// system does: with the constraints swapped, so this system builds the
// other file alone, a run judges it, its test killing its mutants but one,
// before they are put back, the lines unchanged. The built file's snapshot
// is then recorded here, and the whole committed.
func (n *notBuiltState) recordedElsewhere() error {
	if err := n.split("a //go:build line naming another system"); err != nil {
		return err
	}
	if err := n.m.writeAll(map[string]string{n.other: "// Probe as another system builds it\n\n" + notBuiltOther,
		n.built: "//go:build " + n.excluded + "\n\n" + notBuiltHere}); err != nil {
		return err
	}
	if err := n.declare(n.otherSites()[:1], notBuiltHereProfile); err != nil {
		return err
	}
	if err := n.recordRun("mutation run --no-coverage --no-annotate --json "+n.other, 1); err != nil {
		return err
	}
	snapshot := n.m.read(snapshotOf(n.other))
	if !strings.Contains(snapshot, `"killed": 3`) || !strings.Contains(snapshot, `"survived": 1`) {
		return fmt.Errorf("the run on the other system did not record 3 killed and 1 survived\n%s\n%s", snapshot, n.m.w.report())
	}
	if err := n.write("//go:build "+n.excluded, "//go:build !"+n.excluded); err != nil {
		return err
	}
	if err := n.recordRun("mutation run --no-annotate --json "+n.built, 0); err != nil {
		return err
	}
	if err := n.m.w.git("init", "-q", "-b", "main"); err != nil {
		return err
	}
	return n.commit("probe")
}

// sampleSkips samples 20 mutants, which must exit 0 with none of the other
// file's among them.
func (n *notBuiltState) sampleSkips() error {
	if err := n.run("mutation sample --count 20 --json"); err != nil {
		return err
	}
	object, err := n.m.w.json()
	if err != nil {
		return err
	}
	if inFiles(object, n.other) {
		return fmt.Errorf("the sample ran mutants of %s\n%s", n.other, n.m.w.report())
	}
	return n.m.w.exitsWith(0)
}

// snapshotKept runs a whole mutation run, which must leave the other file's
// snapshot as it was and name no orphan.
func (n *notBuiltState) snapshotKept() error {
	before := n.m.read(snapshotOf(n.other))
	if before == "" {
		return fmt.Errorf("%s has no snapshot", n.other)
	}
	if err := n.run("mutation run --json"); err != nil {
		return err
	}
	if after := n.m.read(snapshotOf(n.other)); after != before {
		return fmt.Errorf("the run rewrote the snapshot of %s\n--- before\n%s--- after\n%s\n%s", n.other, before, after, n.m.w.report())
	}
	var object struct {
		Orphans json.RawMessage `json:"orphans"`
	}
	if err := json.Unmarshal([]byte(n.m.w.stdout), &object); err != nil {
		return fmt.Errorf("standard output is not one JSON object: %v\n%s", err, n.m.w.report())
	}
	if object.Orphans != nil {
		return fmt.Errorf("the run names orphans: %s\n%s", object.Orphans, n.m.w.report())
	}
	return nil
}

// committed writes the package split by its file name's suffix and commits
// it.
func (n *notBuiltState) committed() error {
	if err := n.split("its file name's suffix"); err != nil {
		return err
	}
	if err := n.m.w.git("init", "-q", "-b", "main"); err != nil {
		return err
	}
	return n.commit("probe")
}

// notBuiltSeeds are the seeds the counted runs draw with: of the five
// eligible sites before slice-25, four the other file's, one of them
// selects the built file's alone with odds of 1 in 5^8.
var notBuiltSeeds = []string{"1", "2", "3", "4", "5", "6", "7", "8"}

// counted makes a count-one run with each seed, keeping the file of the
// site each selected.
func (n *notBuiltState) counted() error {
	if err := n.m.w.d.Counted(); err != nil {
		return err
	}
	for _, seed := range notBuiltSeeds {
		if err := n.run("mutation run --count 1 --seed " + seed + " --workers 1 --json"); err != nil {
			return err
		}
		object, err := n.m.w.json()
		if err != nil {
			return err
		}
		selected := objectList(object, "selected")
		if len(selected) != 1 {
			return fmt.Errorf("the run with seed %s selected %d sites, not 1\n%s", seed, len(selected), n.m.w.report())
		}
		file, _ := selected[0]["file"].(string)
		n.selected = append(n.selected, filepath.ToSlash(file))
	}
	return nil
}

// noneSelected is whether no counted run selected a site in the other
// file.
func (n *notBuiltState) noneSelected() error {
	if slices.Contains(n.selected, n.other) {
		return fmt.Errorf("the runs with seeds %v selected sites in %v, %s among them", notBuiltSeeds, n.selected, n.other)
	}
	return nil
}
