package features

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/cucumber/godog"
)

func (l *languageMutation) ownLanguageSteps(sc *godog.ScenarioContext) {
	sc.Step(`^a Python project run with (pytest|unittest) where test_a reaches a\.py, test_b reaches only b\.py, and test_b fails$`, l.ownPython)
	sc.Step(`^I run "itos-cc ([^"]*)" for (a\.py|A\.kt)$`, func(args, _ string) error { return l.run(args + " " + l.file) })
	sc.Step(`^a\.py's baseline passes and its mutant is judged by test_a alone, with coverage measured from test_a alone$`, func() error { return l.ownJudgment("low", "spare", "test_a") })
	sc.Step(`^test_b never runs$`, func() error { return l.onlyOwnTests("test_a") })
	sc.Step(`^a Python file no test reaches has its mutants reported uncovered, and no test command runs for it$`, l.unreachedOwnPython)
	sc.Step(`^a Kotlin project built with (gradle|maven) where ATest, in A\.kt's package, uses A without importing it, BTest reaches only B\.kt, and BTest fails$`, l.ownKotlin)
	sc.Step(`^A\.kt's baseline passes and its mutant is judged by ATest alone, with coverage measured from ATest alone$`, func() error { return l.ownJudgment("low", "spare", "ATest") })
	sc.Step(`^BTest never runs$`, func() error { return l.onlyOwnTests("ATest") })
	sc.Step(`^a Python project whose test_a\.py uses a conftest\.py fixture that imports a\.py, and no test imports a\.py$`, l.ownConftest)
	sc.Step(`^a\.py's baseline runs test_a\.py, its coverage is measured from test_a\.py, and its mutant is killed, not reported uncovered$`, l.conftestJudgment)
}
func (l *languageMutation) ownTrace() error {
	dir, err := l.m.w.d.Folder()
	if err != nil {
		return err
	}
	l.trace = filepath.Join(dir, "own.jsonl")
	return l.m.w.d.Write(l.trace, nil)
}
func (l *languageMutation) ownPython(runner string) error {
	w := l.m.w
	l.file = "a.py"
	l.language = "python"
	if err := l.ownTrace(); err != nil {
		return err
	}
	trace, _ := json.Marshal(l.trace)
	root, _ := json.Marshal(w.dir)
	record := func(name string) string {
		return "import pathlib, json, unittest, hashlib\nHERE = pathlib.Path(__file__).resolve().parent\nORIGINAL = pathlib.Path(" + string(root) + ").resolve()\ndef record():\n    with open(" + string(trace) + ", 'a') as f: f.write(json.dumps({'test': '" + name + "', 'cwd': str(HERE), 'original_root': str(ORIGINAL), 'source_sha256': hashlib.sha256((HERE / 'a.py').read_bytes()).hexdigest()}) + '\\n')\n"
	}
	for name, text := range map[string]string{
		"pyproject.toml": "[project]\nname = \"board\"\nversion = \"0.0.0\"\n",
		"a.py":           "def low(i):\n    return i == 3\n\ndef spare(i):\n    return i == 7\n",
		"b.py":           "def high(i):\n    return i == 9\n", "u.py": "def mid(i):\n    return i == 5\n",
		"test_a.py": record("test_a") + "from a import low\nclass TestA(unittest.TestCase):\n    def test_low(self):\n        record()\n        self.assertTrue(low(3))\n        self.assertFalse(low(4))\n",
		"test_b.py": record("test_b") + "from b import high\nclass TestB(unittest.TestCase):\n    def test_high(self):\n        record()\n        self.assertTrue(high(9))\n        self.fail('test_b fails')\n",
		"test_c.py": record("test_c") + "import importlib\nclass TestC(unittest.TestCase):\n    def test_loaded(self):\n        record()\n        a,u = (importlib.import_module(n) for n in ('a','u'))\n        self.assertTrue(a.spare(7))\n        self.assertTrue(u.mid(5))\n",
	} {
		if err := w.write(name, text); err != nil {
			return err
		}
	}
	w.variables = append(w.variables, "PYTEST_DISABLE_PLUGIN_AUTOLOAD=1")
	if err := w.d.Coverage(w.dir, Coverage{Language: "python", Report: "SF:a.py\nDA:1,1\nDA:2,1\nDA:4,1\nDA:5,0\nend_of_record\n"}); err != nil {
		return err
	}
	command := "python3 -m pytest -q -x -p no:cacheprovider test_a.py"
	if runner == "unittest" {
		setup := "import pathlib, sys, venv\nvenv.EnvBuilder(system_site_packages=True, with_pip=False).create('.venv')\nsite = pathlib.Path('.venv') / 'lib' / ('python' + str(sys.version_info.major) + '.' + str(sys.version_info.minor)) / 'site-packages'\nsite.mkdir(parents=True, exist_ok=True)\n(site / 'no_pytest.pth').write_text('import sys; sys.modules[\"pytest\"] = None\\n')\n"
		if err := w.write("setup_runner.py", setup); err != nil {
			return err
		}
		a, log, code, err := l.capture(`crap --json --coverage-command 'python3 setup_runner.py && .venv/bin/python -m coverage run -m unittest test_a && .venv/bin/python -m coverage lcov -o own.lcov' --coverage-report own.lcov a.py`)
		if err != nil {
			return err
		}
		if code != 0 || a["ok"] != true {
			return fmt.Errorf("unittest fixture setup exit%d: %v\n%s", code, a, log)
		}
		command = filepath.Join(w.dir, ".venv/bin/python") + " -m unittest test_a"
		if err := w.d.Write(l.trace, nil); err != nil {
			return err
		}
	}
	return w.d.Tests(w.dir, Tests{Language: "python", Command: command, Trace: &TestTrace{File: l.trace, Source: l.file, Tests: ownPythonInventory(runner)}})
}
func (l *languageMutation) onlyOwnTests(name string) error {
	records, err := l.pythonTrace()
	if err != nil {
		return err
	}
	tree, workers := 0, 0
	for _, r := range records {
		if len(fmt.Sprint(r["source_sha256"])) != 64 {
			return fmt.Errorf("missing actual source hash: %v", r)
		}
		if r["test"] != name {
			return fmt.Errorf("unrelated test actually ran: %v", r)
		}
		if r["cwd"] == r["original_root"] {
			tree++
		} else {
			workers++
		}
	}
	if tree < 1 || workers < 2 {
		return fmt.Errorf("missing actual coverage/baseline/trial test observations: %v", records)
	}
	return nil
}
func (l *languageMutation) ownJudgment(killed, uncovered, test string) error {
	a, err := l.object()
	if err != nil {
		return err
	}
	f, err := languageRunFile(a, l.file)
	if err != nil {
		return err
	}
	if f["baseline"] != "passed" {
		return fmt.Errorf("own baseline: %v", f)
	}
	seen := map[string]int{}
	for _, m := range objectList(f, "mutants") {
		name := fmt.Sprint(m["function"])
		want := ""
		if strings.HasSuffix(name, "#"+killed) {
			want = "killed"
			seen[killed]++
		}
		if strings.HasSuffix(name, "#"+uncovered) {
			want = "uncovered"
			seen[uncovered]++
		}
		if want == "" || m["outcome"] != want {
			return fmt.Errorf("own-test mutant: %v", m)
		}
	}
	if seen[killed] != 1 || seen[uncovered] != 1 {
		return fmt.Errorf("missing expected own-test mutant population: %v", f)
	}
	return l.onlyOwnTests(test)
}
func (l *languageMutation) unreachedOwnPython() error {
	w := l.m.w
	if err := w.d.Write(l.trace, nil); err != nil {
		return err
	}
	a, log, _, err := l.capture("mutation run --json u.py")
	if err != nil {
		return err
	}
	f, err := languageRunFile(a, "u.py")
	if err != nil {
		return err
	}
	b, err := w.d.Read(l.trace)
	if err != nil {
		return err
	}
	if f["baseline"] != "not-run" || f["uncovered"] != float64(1) || f["ran"] != float64(0) || len(b) != 0 || strings.Contains(log, "$ ") {
		return fmt.Errorf("unreached Python executed tests: %v trace%s\n%s", f, b, log)
	}
	return nil
}
func (l *languageMutation) ownKotlin(build string) error {
	w := l.m.w
	l.language = "kotlin"
	l.file = "src/main/kotlin/own/A.kt"
	if err := l.ownTrace(); err != nil {
		return err
	}
	for file, text := range languageKotlinBuilds[build] {
		if err := w.write(file, text); err != nil {
			return err
		}
	}
	trace, _ := json.Marshal(l.trace)
	root, _ := json.Marshal(w.dir)
	record := "package own\nimport java.io.File\nimport java.security.MessageDigest\nobject Record { fun ran(name: String) { val digest = MessageDigest.getInstance(\"SHA-256\").digest(File(\"src/main/kotlin/own/A.kt\").readBytes()).joinToString(\"\") { \"%02x\".format(it) }; File(" + string(trace) + ").appendText(\"{\\\"test\\\":\\\"\" + name + \"\\\",\\\"cwd\\\":\" + quote(File(\".\").canonicalPath) + \",\\\"original_root\\\":\" + quote(File(" + string(root) + ").canonicalPath) + \",\\\"source_sha256\\\":\" + quote(digest) + \"}\\n\") }; fun quote(s: String): String = \"\\\"\" + s.replace(\"\\\\\",\"\\\\\\\\\") + \"\\\"\" }\n"
	test := func(name, body string) string {
		return "package own\nimport kotlin.test.*\nclass " + name + " { @Test fun judges() { Record.ran(\"" + name + "\"); " + body + " } }\n"
	}
	for file, text := range map[string]string{
		l.file:                          "package own\n\nfun low(i: Int): Boolean {\n    return i == 3\n}\n\nfun spare(i: Int): Boolean {\n    return i == 7\n}\n",
		"src/main/kotlin/own/B.kt":      "package own\nfun high(i: Int): Boolean { return i == 9 }\n",
		"src/test/kotlin/own/Record.kt": record,
		"src/test/kotlin/own/ATest.kt":  test("ATest", "assertTrue(low(3)); assertFalse(low(4));"),
		"src/test/kotlin/own/BTest.kt":  test("BTest", "assertTrue(high(9)); fail(\"BTest fails\");"),
		"src/test/kotlin/own/CTest.kt":  test("CTest", "val m = Class.forName(\"own.\" + \"AKt\").getMethod(\"sp\" + \"are\", Int::class.javaPrimitiveType); assertEquals(true,m.invoke(null,7));"),
	} {
		if err := w.write(file, text); err != nil {
			return err
		}
	}
	program := "gradle"
	command := "gradle -p . test --fail-fast --tests own.ATest"
	if build == "maven" {
		program = "mvn"
		command = "mvn -q test -Dtest=own.ATest -Dsurefire.failIfNoSpecifiedTests=false"
	}
	if err := w.d.Coverage(w.dir, Coverage{Language: "kotlin", Program: program, Report: `<report><package name="own"><sourcefile name="A.kt"><line nr="4" mi="0" ci="1"/><line nr="8" mi="1" ci="0"/></sourcefile></package></report>`}); err != nil {
		return err
	}
	return w.d.Tests(w.dir, Tests{Language: "kotlin", Command: command, Trace: &TestTrace{File: l.trace, Source: l.file, Tests: map[string]string{"own.ATest": "ATest", "own.BTest": "BTest", "own.CTest": "CTest"}}})
}
func (l *languageMutation) ownConftest() error {
	w := l.m.w
	l.file = "a.py"
	l.language = "python"
	for file, text := range map[string]string{
		"pyproject.toml":    "[project]\nname = \"board\"\nversion = \"0.0.0\"\n",
		"a.py":              languagePythonSource,
		"tests/conftest.py": "import pytest\nfrom a import low\n@pytest.fixture\ndef check():\n    return low\n",
		"tests/test_a.py":   "def test_low(check):\n    assert check(3)\n    assert not check(4)\n",
		"test_other.py":     "def test_other():\n    assert False\n",
	} {
		if err := w.write(file, text); err != nil {
			return err
		}
	}
	w.variables = append(w.variables, "PYTEST_DISABLE_PLUGIN_AUTOLOAD=1")
	if err := w.d.Coverage(w.dir, Coverage{Language: "python", Report: "SF:a.py\nDA:1,1\nDA:2,1\nend_of_record\n"}); err != nil {
		return err
	}
	return w.d.Tests(w.dir, Tests{Language: "python", Command: "python3 -m pytest -q -x -p no:cacheprovider tests/test_a.py"})
}
func (l *languageMutation) conftestJudgment() error {
	a, err := l.object()
	if err != nil {
		return err
	}
	f, err := languageRunFile(a, l.file)
	if err != nil {
		return err
	}
	if f["baseline"] != "passed" || f["killed"] != float64(1) || f["uncovered"] != float64(0) {
		return fmt.Errorf("conftest judgment: %v", f)
	}
	seen := map[string]bool{}
	for _, line := range strings.Split(l.m.w.stderr, "\n") {
		if !strings.Contains(line, " -m pytest") {
			continue
		}
		stage, _, _ := strings.Cut(strings.TrimPrefix(line, "itos-cc: "), " ")
		seen[stage] = true
		if !strings.HasSuffix(line, " tests/test_a.py") {
			return fmt.Errorf("conftest selection ran extra tests: %s", line)
		}
	}
	if !seen["baseline"] || !seen["coverage"] {
		return fmt.Errorf("missing actual conftest runner stages: %v", seen)
	}
	return nil
}

func ownPythonInventory(runner string) map[string]string {
	suffix := ".py"
	if runner == "unittest" {
		suffix = ""
	}
	return map[string]string{"test_a" + suffix: "test_a", "test_b" + suffix: "test_b", "test_c" + suffix: "test_c"}
}
