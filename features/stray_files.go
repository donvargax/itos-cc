package features

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"
)

// What a run of itos-cc leaves (T-236, T-240): each of a scenario's runs is
// given a temporary folder and a marks folder of the scenario's own. A path a
// run added to the scenario's project tree, but under .metrics/, or left in
// the temporary folder fails the scenario, naming it, unless it is a report
// the scenario named with --coverage-report, is in a folder of a language's
// own tooling (toolFolders), or an allowance names the scenario's tag and the
// path. Only new paths are held to this: a file the scenario wrote may change,
// and a path a later run removed is not left. What a fixture project's own
// tests write on purpose goes to the marks folder, never to the project.

// allowance lets the scenarios its tags name leave one path, with the reason
// their subject is that path. A path ending in a slash is the folder of that
// name and everything below it; no allowance is a pattern.
type allowance struct {
	tags         []string
	path, reason string
}

// projectAllowances are the paths scenarios may add to their project tree.
var projectAllowances = []allowance{
	{[]string{
		"@ID-MUT-85", "@ID-MUT-88", "@ID-MUT-89", "@ID-MUT-90", "@ID-MUT-91", "@ID-MUT-92", "@ID-MUT-93",
		"@ID-MUT-94", "@ID-MUT-96", "@ID-MUT-97", "@ID-MUT-98", "@ID-MUT-99", "@ID-MUT-138",
		"@ID-MUT-139", "@ID-MUT-140", "@ID-MUT-141", "@ID-MUT-142", "@ID-MUT-143", "@ID-MUT-144",
		"@ID-MUT-146", "@ID-MUT-163", "@ID-MUT-195", "@ID-MUT-227", "@ID-MUT-242", "@ID-MUT-291", "@ID-MUT-292",
		"@ID-MUT-293", "@ID-MUT-295", "@ID-MUT-297", "@ID-MUT-298", "@ID-MUT-318", "@ID-MUT-319",
		"@ID-MUT-320", "@ID-MUT-322", "@ID-MUT-344", "@ID-MUT-357", "@ID-MUT-373", "@ID-MUT-391",
		"@ID-MUT-394", "@ID-MUT-397", "@ID-MUT-399", "@ID-MUT-401", "@ID-MUT-410", "@ID-MUT-411",
		"@ID-MUT-412", "@ID-MUT-413", "@ID-MUT-414", "@ID-MUT-415", "@ID-MUT-416", "@ID-MUT-417",
		"@ID-MUT-418", "@ID-MUT-420", "@ID-MUT-422",
		"@ID-SEL-37"}, "itos-cc.yaml",
		"their subject is mutation except, which writes the exception it records into itos-cc.yaml, making the file when there is none, or the scenario records one the same way before judging it"},
	{[]string{"@ID-MUT-325"}, "testdata/left-behind",
		"its subject is the file its project's test leaves in the project's own tree while coverage runs, which must never reach a worker's copy"},
	{[]string{"@ID-MUT-209", "@ID-MUT-210", "@ID-MUT-216"}, ".coverage",
		"the scenario's setup run names the coverage command, coverage.py, whose data file it writes, as the project's own tool does"},
	{[]string{"@ID-MUT-209", "@ID-MUT-210", "@ID-MUT-216"}, "__pycache__/",
		"the scenario's setup run names a coverage command that runs pytest and compileall in the project, which write bytecode there as a person's own run does; the scenario's subject starts from that tree"},
	{[]string{"@ID-MUT-209", "@ID-MUT-216"}, ".venv/",
		"the virtual environment the scenario's setup command makes, to have a project of its own with the tools its tests need"},
}

// A path in a folder a language's own tooling owns and every project ignores
// is not the project's tree: itos-cc runs the tool in place, and the tool
// leaves its output there. This one list names them, each with its reason. A
// cache beside the sources (__pycache__, .pytest_cache, .coverage) is not
// here: a run that leaves one is fixed in the product.
type toolFolder struct {
	// folder is the name of a folder, at the project's root or nested in it,
	// and temp whether it is in the temporary folder, not the project's tree.
	folder string
	temp   bool
	reason string
}

var toolFolders = []toolFolder{
	{"node_modules", false, "npm's own folder, which node tools (vitest's .vite results, c8) write their caches into"},
	{"build", false, "the output folder of Gradle and Flutter builds, which they write when the project's own tests run"},
	{".gradle", false, "Gradle's cache of the project, which it makes when the project's tests run"},
	{"target", false, "Maven's output folder, which it writes when the project's tests run"},
	{"coverage", false, "the report folder of vitest's coverage provider, where it writes by default"},
	{"node-compile-cache", true, "Node's compile cache, which it writes into the temporary folder it is given"},
	{"ssr", true, "vite's server-side transform cache, which vitest writes in a random folder of the temporary folder"},
}

// inToolFolder is whether file, a slash path, is below a folder of toolFolders
// of the kind temp says.
func inToolFolder(file string, temp bool) bool {
	segments := strings.Split(file, "/")
	for _, tool := range toolFolders {
		if tool.temp == temp && slices.Contains(segments[:len(segments)-1], tool.folder) {
			return true
		}
	}
	return false
}

// tempAllowances are the paths scenarios may leave in their temporary folder.
var tempAllowances = []allowance{
	{[]string{"@ID-MUT-354"}, tempLeft,
		"its subject is the file a package's test leaves in its temporary folder: the scenario's own step holds that none reaches the system's"},
}

// permitted is whether one of allowances lets a scenario with tags leave path.
func permitted(allowances []allowance, tags []string, path string) bool {
	return slices.ContainsFunc(allowances, func(a allowance) bool {
		if !slices.ContainsFunc(a.tags, func(tag string) bool { return slices.Contains(tags, tag) }) {
			return false
		}
		if folder, ok := strings.CutSuffix(a.path, "/"); ok {
			return path == folder || strings.HasPrefix(path, a.path)
		}
		return path == a.path
	})
}

// tempEnv are the variables that give a run the scenario's temporary folder,
// TMPDIR on Unix and TMP and TEMP on Windows, all three set for a scenario's
// own variables to set again or unset.
func (w *world) tempEnv(env []string) []string {
	return append([]string{"TMPDIR=" + w.tmp, "TMP=" + w.tmp, "TEMP=" + w.tmp, marksEnv + "=" + w.marks}, env...)
}

// marksEnv names the variable that gives the tests of a fixture project the
// folder of marks the scenario owns, outside the project and its temporary
// folder: what a project's own tests write on purpose is the project's, so
// they write their marks there, and the steps read them there.
const marksEnv = "FIXTURE_MARKS"

// mark is the path of the mark name in the scenario's marks folder.
func (w *world) mark(name string) string { return filepath.Join(w.marks, name) }

// named records the reports r asks for with --coverage-report, which the
// scenario names itself, so the project's coverage command writing them is
// its subject.
func (w *world) named(r Run) {
	if w.reports == nil {
		w.reports = map[string]bool{}
	}
	for i, arg := range r.Args {
		report, ok := strings.CutPrefix(arg, "--coverage-report=")
		if arg == "--coverage-report" && i+1 < len(r.Args) {
			report, ok = r.Args[i+1], true
		}
		if !ok {
			continue
		}
		if !filepath.IsAbs(report) {
			report = filepath.Join(r.Dir, report)
		}
		if rel, err := filepath.Rel(w.dir, report); err == nil {
			w.reports[filepath.ToSlash(rel)] = true
		}
	}
}

// around makes one run of itos-cc, run, holding it to what it leaves: the
// project tree is fingerprinted just before it, so what the scenario's steps
// wrote since the last run is the scenario's, and again just after, so a path
// new then is the run's. A run started and left running is held to what it
// wrote by the time Start returns, no later.
func (w *world) around(ctx context.Context, run func() error) error {
	before, err := w.paths(ctx, w.dir)
	if err != nil {
		return err
	}
	w.ran = true
	if w.added == nil {
		w.added = map[string]bool{}
	}
	runErr := run()
	after, err := w.paths(ctx, w.dir)
	if err != nil {
		return err
	}
	for file := range after {
		if !before[file] {
			w.added[file] = true
		}
	}
	return runErr
}

// paths are the files of dir as a set, none when dir is not there.
func (w *world) paths(ctx context.Context, dir string) (map[string]bool, error) {
	files, err := w.d.Files(ctx, dir)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	set := map[string]bool{}
	for _, file := range files {
		set[file] = true
	}
	return set, nil
}

// strays are the paths a scenario's runs left that no allowance lets,
// naming the first, as the scenario's failure; nil when it ran nothing.
func (w *world) strays(ctx context.Context) error {
	if !w.ran {
		return nil
	}
	now, err := w.paths(ctx, w.dir)
	if err != nil {
		return err
	}
	var in, temp []string
	for _, file := range sortedKeys(w.added) {
		if now[file] && !underMetrics(file) && !w.reports[file] && !inToolFolder(file, false) && !permitted(projectAllowances, w.tags, file) {
			in = append(in, file)
		}
	}
	left, err := w.paths(ctx, w.tmp)
	if err != nil {
		return err
	}
	for _, file := range sortedKeys(left) {
		if !inToolFolder(file, true) && !permitted(tempAllowances, w.tags, file) {
			temp = append(temp, file)
		}
	}
	var problems []string
	if len(in) > 0 {
		problems = append(problems, fmt.Sprintf("%s is new in the project: a run of itos-cc must leave the project as it found it but for .metrics/", strings.Join(in, ", ")))
	}
	if len(temp) > 0 {
		problems = append(problems, fmt.Sprintf("%s is left in the run's temporary folder: a run of itos-cc must leave its temporary folder empty", strings.Join(temp, ", ")))
	}
	if len(problems) > 0 {
		return errors.New(strings.Join(problems, "; "))
	}
	return nil
}

// underMetrics is whether file is under a .metrics folder, the project's or
// a nested project's, where runs keep what they measured.
func underMetrics(file string) bool {
	return strings.HasPrefix(file, ".metrics/") || strings.Contains(file, "/.metrics/")
}

// sortedKeys are the names of set, in order.
func sortedKeys(set map[string]bool) []string {
	names := make([]string, 0, len(set))
	for name := range set {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

// run is the driver's run of r, made the way every run of a scenario is: with
// the scenario's temporary folder, and held to what it leaves (around).
func (w *world) run(r Run) (res Result, err error) {
	w.named(r)
	r.Env = w.tempEnv(r.Env)
	err = w.around(context.Background(), func() (runErr error) {
		res, runErr = w.d.Run(r)
		return runErr
	})
	return res, err
}

// start is the driver's asynchronous run of r, made as run makes one.
func (w *world) start(ctx context.Context, r Run) (Running, error) {
	w.named(r)
	r.Env = w.tempEnv(r.Env)
	var p Running
	err := w.around(ctx, func() (runErr error) {
		p, runErr = w.d.Start(ctx, r)
		return runErr
	})
	return p, err
}
