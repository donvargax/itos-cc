package features

import (
	"fmt"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

// Running mutants: the private copy of the project each mutant runs in.

// makeProject writes a module in src/, its build root, with a Makefile
// whose test target runs its tests, passing them a word that says make ran
// them, and a test that records where it ran.
func (m *mutateState) makeProject() error {
	records, err := m.testRecords()
	if err != nil {
		return err
	}
	m.file = "src/board.go"
	test := goRecorder("board", records, filepath.Join(m.w.dir, "src"), "slices") + `
func TestPlace(t *testing.T) {
	record(t, map[string]any{"make": slices.Contains(os.Args, "make-ran")})
	var b Board
	if b.place(5) || !b.place(6) {
		t.Fatal("place")
	}
}
`
	if err := m.writeAll(map[string]string{
		"src/go.mod": mutateModule, m.file: boardGo, "src/board_test.go": test,
		"src/Makefile": "test:\n\tgo test -count=1 ./... -args make-ran\n",
	}); err != nil {
		return err
	}
	if err := m.w.d.Coverage(filepath.Join(m.w.dir, "src"), Coverage{Language: "go", Report: "SF:board.go\nDA:7,1\nDA:11,0\nend_of_record\n"}); err != nil {
		return err
	}
	return m.w.d.Tests(filepath.Join(m.w.dir, "src"), Tests{Language: "go", Command: "make test",
		Surviving: []Site{{"board.go", 11, "<", "<="}}})
}

// shellFromBuildRoot is whether the last run announced command as its
// baseline, from the file's build root, and the command, make's recipe,
// ran the tests in a copy of that folder.
func (m *mutateState) shellFromBuildRoot(command string) error {
	root := path.Dir(m.file)
	if !slices.ContainsFunc(baselines(m.w.stderr), func(a announced) bool { return a.command == command && a.dir == root }) {
		return fmt.Errorf("the run announced no baseline %q from %s\n%s", command, root, m.w.report())
	}
	runs, err := m.runsRecorded()
	if err != nil {
		return err
	}
	ran := 0
	for _, r := range runs {
		if r.Real {
			// coverage, measured in the real tree
			continue
		}
		ran++
		if !r.Make || filepath.Base(r.Cwd) != path.Base(root) {
			return fmt.Errorf("a run of the tests in %s did not come from make's recipe in a copy of %s: %+v\n%s", r.Cwd, root, r, m.w.report())
		}
	}
	if ran == 0 {
		return fmt.Errorf("no run of the tests came from %q\n%s", command, m.w.report())
	}
	return nil
}

// The project of "The real tree is never modified while tests run": a
// TypeScript source whose test script, check.js, records where it ran,
// which of node_modules and .venv are links there, and whether the real
// tree's src/board.ts was as written before and after it, then fails when
// its own copy of src/board.ts is mutated.
const (
	copiedTS = "export function place(i: number, j: number): boolean {\n  return i > 1 && j < 5;\n}\n"
	checkJS  = `const fs = require("fs");
const path = require("path");
const records = %s;
const real = %s;
const original = %s;
const unchanged = () => fs.readFileSync(path.join(real, "src", "board.ts"), "utf8") === original;
const here = fs.realpathSync(process.cwd());
const entry = {
  cwd: here,
  real: here === fs.realpathSync(real),
  start: Date.now(),
  links: ["node_modules", ".venv"].filter((dir) => fs.lstatSync(dir).isSymbolicLink()),
  unchanged: unchanged(),
};
const until = Date.now() + 300;
while (Date.now() < until) {}
entry.end = Date.now();
entry.unchanged = entry.unchanged && unchanged();
fs.appendFileSync(path.join(records, "runs"), JSON.stringify(entry) + "\n");
process.exit(fs.readFileSync(path.join("src", "board.ts"), "utf8") === original ? 0 : 1);
`
)

// copiedProject writes the project, with a node_modules and a .venv of
// its own.
func (m *mutateState) copiedProject() error {
	records, err := m.testRecords()
	if err != nil {
		return err
	}
	m.file, m.original = "src/board.ts", copiedTS
	if err := m.writeAll(map[string]string{
		"package.json":                  `{"name": "board", "scripts": {"test": "node check.js"}}` + "\n",
		m.file:                          copiedTS,
		"check.js":                      fmt.Sprintf(checkJS, strconv.Quote(records), strconv.Quote(m.w.dir), strconv.Quote(copiedTS)),
		"node_modules/dep/index.js":     "module.exports = 1;\n",
		"node_modules/dep/package.json": `{"name": "dep", "version": "1.0.0"}` + "\n",
		".venv/pyvenv.cfg":              "include-system-site-packages = false\n",
	}); err != nil {
		return err
	}
	return m.w.d.Tests(m.w.dir, Tests{Language: "typescript", Command: "npm run test --silent"})
}

// mutantRuns are the runs the project's tests recorded in a worker's
// copy, at least one a mutant's beside the baseline.
func (m *mutateState) mutantRuns() ([]testRecord, error) {
	runs, err := m.runsRecorded()
	if err != nil {
		return nil, err
	}
	copies := slices.DeleteFunc(slices.Clone(runs), func(r testRecord) bool { return r.Real })
	if len(copies) < 2 {
		return nil, fmt.Errorf("the tests ran %d times outside the real tree, not a baseline and a mutant's: %+v\n%s", len(copies), runs, m.w.report())
	}
	return copies, nil
}

// privateCopies is whether every run of the tests ran in a worker's copy,
// none in the real tree, and no two at once in one copy.
func (m *mutateState) privateCopies() error {
	runs, err := m.runsRecorded()
	if err != nil {
		return err
	}
	if len(runs) < 2 {
		return fmt.Errorf("the tests ran %d times, not a baseline and a mutant's\n%s", len(runs), m.w.report())
	}
	for i, r := range runs {
		if r.Real {
			return fmt.Errorf("a run of the tests ran in the real tree, %s\n%s", r.Cwd, m.w.report())
		}
		for _, other := range runs[i+1:] {
			if other.Cwd == r.Cwd && other.Start < r.End && r.Start < other.End {
				return fmt.Errorf("two runs of the tests ran in %s at once\n%s", r.Cwd, m.w.report())
			}
		}
	}
	return nil
}

// dependenciesLinked is whether every run of the tests found each folder a
// link in its copy.
func (m *mutateState) dependenciesLinked(first, second string) error {
	runs, err := m.mutantRuns()
	if err != nil {
		return err
	}
	for _, r := range runs {
		for _, dir := range []string{first, second} {
			if !slices.Contains(r.Links, dir) {
				return fmt.Errorf("%s is no link in the copy %s: the links are %v", dir, r.Cwd, r.Links)
			}
		}
	}
	return nil
}

// treeUnchanged is whether every run of the tests found the real tree's
// source as written before and after it ran. The run annotates it once
// the tests are done.
func (m *mutateState) treeUnchanged() error {
	runs, err := m.mutantRuns()
	if err != nil {
		return err
	}
	for _, r := range runs {
		if !r.Unchanged {
			return fmt.Errorf("the run of the tests in %s found %s changed in the real tree\n%s", r.Cwd, m.file, m.w.report())
		}
	}
	return nil
}

// editableTest records where it ran, PYTHONPATH and the file board was
// imported from, each as the system resolves it.
const editableTest = `import json
import os

import board

RECORDS = %s
REAL = %s


def test_place():
    here = os.path.realpath(os.getcwd())
    entry = {
        "cwd": here,
        "real": here == os.path.realpath(REAL),
        "path": [os.path.realpath(p) for p in os.environ.get("PYTHONPATH", "").split(os.pathsep) if p],
        "file": os.path.realpath(board.__file__),
    }
    with open(os.path.join(RECORDS, "runs"), "a") as f:
        f.write(json.dumps(entry) + "\n")
    assert board.place(1, 2) and not board.place(0, 2)
`

// editableProject writes a Python project in src/ layout, and makes it
// installed in editable mode in its own virtualenv: a .pth file in the
// virtualenv's site-packages names the real tree's src/, as pip install -e
// writes one, for each Python 3 a virtualenv may hold.
func (m *mutateState) editableProject() error {
	records, err := m.testRecords()
	if err != nil {
		return err
	}
	m.file = "src/board.py"
	files := map[string]string{
		"pyproject.toml": pyproject, m.file: pyBoard,
		"tests/test_board.py": fmt.Sprintf(editableTest, strconv.Quote(records), strconv.Quote(m.w.dir)),
	}
	if err := m.writeAll(files); err != nil {
		return err
	}
	if err := m.w.d.Coverage(m.w.dir, Coverage{Language: "python", Venv: true}); err != nil {
		return err
	}
	pth := map[string]string{".venv/pyvenv.cfg": "include-system-site-packages = true\n"}
	for minor := 9; minor <= 20; minor++ {
		pth[fmt.Sprintf(".venv/lib/python3.%d/site-packages/__editable__.board-0.pth", minor)] = filepath.Join(m.w.dir, "src") + "\n"
	}
	if err := m.writeAll(pth); err != nil {
		return err
	}
	return m.w.d.Tests(m.w.dir, Tests{Language: "python", Command: ".venv/bin/python -m pytest -q -x -p no:cacheprovider tests/test_board.py"})
}

// pythonPathFirst is whether every run of the tests in a worker's copy had
// PYTHONPATH begin with the copy's first and second folders, and imported
// the source from the copy.
func (m *mutateState) pythonPathFirst(first, second string) error {
	runs, err := m.mutantRuns()
	if err != nil {
		return err
	}
	for _, r := range runs {
		want := []string{filepath.Join(r.Cwd, first), filepath.Join(r.Cwd, second)}
		if len(r.Path) < 2 || !slices.Equal(r.Path[:2], want) {
			return fmt.Errorf("PYTHONPATH in the copy %s is %v, not %v first", r.Cwd, r.Path, want)
		}
		if !strings.HasPrefix(r.File, r.Cwd+string(filepath.Separator)) {
			return fmt.Errorf("the tests in the copy %s imported %s", r.Cwd, r.File)
		}
	}
	return nil
}
