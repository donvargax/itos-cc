package features

import "path/filepath"

// slice-16, issue #46: a Python project whose test fails only under
// coverage.py, after its file's every line has run, so the coverage command
// fails after writing a report that names the file, as a floor the command
// line cannot clear makes it fail.

// floorTestPython is calc.py's test: it runs both of above's outcomes, then
// fails where coverage.py runs in its process, which only itos-cc's
// coverage run does.
const floorTestPython = "import sys\n\nfrom calc import above\n\n\ndef test_above():\n    assert above(2, 1)\n    assert not above(1, 2)\n" +
	"    assert \"coverage\" not in sys.modules, \"coverage.py runs this test\"\n"

func (l *languageMutation) failingUnderCoverage() error {
	w := l.m.w
	l.language, l.file = "python", "app/calc.py"
	if err := l.m.writeProject(map[string][]string{"app/test_calc.py": {l.file}}, map[string]string{
		"app/pyproject.toml": "[project]\nname = \"app\"\nversion = \"0.0.0\"\n",
		l.file:               "def above(a, b):\n    return a > b\n",
		"app/test_calc.py":   floorTestPython,
	}); err != nil {
		return err
	}
	app := filepath.Join(w.dir, "app")
	if err := w.d.Coverage(app, Coverage{Language: "python", Report: "SF:calc.py\nDA:1,1\nDA:2,1\nend_of_record\n", CommandExit: 1, CommandOutput: "1 failed"}); err != nil {
		return err
	}
	if err := w.d.Tests(app, Tests{Language: "python", Command: "python3 -m pytest -q -x -p no:cacheprovider"}); err != nil {
		return err
	}
	if err := l.m.pythonProbe(true); err != nil {
		return err
	}
	w.prepare = func(args []string) []string {
		if len(args) >= 2 && args[0] == "mutation" && args[1] == "run" {
			args = append(args, "--no-annotate", "--workers", "2")
		}
		return args
	}
	l.m.forFile = func(args string) error { return l.run(args + " " + l.file) }
	return nil
}
