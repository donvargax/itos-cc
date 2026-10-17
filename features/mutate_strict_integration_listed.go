package features

import (
	"fmt"
	"slices"
	"strings"
)

func (q *strictMutation) listedIntegrationRun() error {
	old := q.m.w.dir
	defer func() { q.m.w.dir = old }()
	if err := q.newIntegrationRoot(); err != nil {
		return err
	}
	language := q.language
	// The listed harness reaches the CLI only through processes. There is no
	// source-importing own test that could stand in for the listed collector.
	files := map[string]string{}
	c := Coverage{Language: language}
	first, second := "positive", "other"
	ifLine, positive, other := 4, 5, 7
	commands := []string{}
	var list, run, pattern, each, sep string
	switch language {
	case "python":
		q.file = "cli.py"
		first, second = "test_positive", "test_other"
		ifLine, positive, other = 6, 7, 8
		q.testFile = "test_listed.py"
		files["pyproject.toml"] = strictPyPythonIntegrationFiles["pyproject.toml"]
		files[q.file] = strictPyCliSource
		files[q.testFile] = strictPyCliRunner(true) + "def test_positive():\n    assert run('test_positive','1') == '101'\n\ndef test_other():\n    assert run('test_other','-1') == '-101'\n"
		list = `python3 -c "print(open('tests.txt').read(), end='')"`
		run = `python3 -m pytest -q -p no:cacheprovider test_listed.py -k "{pattern}"`
		pattern, each, sep = "{ids}", "{id}", " or "
		commands = []string{`python3 -m pytest -q -p no:cacheprovider test_listed.py -k "test_positive or test_other"`, `python3 -m pytest -q -p no:cacheprovider test_listed.py -k "test_positive"`, `python3 -m pytest -q -p no:cacheprovider test_listed.py -k "test_other"`}
		c.Report = "SF:cli.py\nDA:6,0\nDA:7,0\nDA:8,0\nend_of_record\n"
		c.IntegrationReport = "SF:cli.py\nDA:6,1\nDA:7,1\nDA:8,1\nend_of_record\n"
	case "typescript":
		q.file = "src/cli.ts"
		q.testFile = "src/listed.test.ts"
		files["package.json"] = strictTsTsPackageJSON
		files[q.file] = strictTsTsCliSource
		files[q.testFile] = strictTsTsCliRunner(true) + "test('positive',()=>expect(run('positive','1')).toBe('101'));\ntest('other',()=>expect(run('other','-1')).toBe('-101'));\n"
		list = `node -e "process.stdout.write(require('fs').readFileSync('tests.txt', 'utf8'))"`
		run = `node_modules/.bin/vitest run src/listed.test.ts -t "{pattern}"`
		pattern, each, sep = "^({ids})$", "{id}", "|"
		for _, p := range []string{"^(positive|other)$", "^(positive)$", "^(other)$"} {
			commands = append(commands, `node_modules/.bin/vitest run src/listed.test.ts -t "`+p+`"`)
		}
		c.Report = "SF:src/cli.ts\nDA:4,0\nDA:5,0\nDA:7,0\nend_of_record\n"
		c.IntegrationReport = "SF:src/cli.ts\nDA:4,1\nDA:5,1\nDA:7,1\nend_of_record\n"
	case "kotlin":
		q.file = "src/main/kotlin/board/Main.kt"
		q.testFile = "src/test/kotlin/board/ListedTest.kt"
		for file, text := range strictKtKtIntegrationFiles("split") {
			if file != "src/test/kotlin/board/MainTest.kt" {
				files[file] = text
			}
		}
		files[q.testFile] = strictKtKtRunner("ListedTest", "split") + strictKtKtPositiveTest + strictKtKtOtherTest + "}\n"
		files["build.gradle.kts"] += "\ntasks.register(\"itosList\") { doLast { println(\"positive\\tsrc/test/kotlin/board/ListedTest.kt\"); println(\"other\\tsrc/test/kotlin/board/ListedTest.kt\") } }\n"
		list = "gradle -q -p . itosList"
		run = "gradle -q -p . test --rerun {pattern}"
		pattern, each, sep = "{ids}", "--tests board.ListedTest.{id}", " "
		commands = []string{"gradle -q -p . test --rerun --tests board.ListedTest.positive --tests board.ListedTest.other", "gradle -q -p . test --rerun --tests board.ListedTest.positive", "gradle -q -p . test --rerun --tests board.ListedTest.other"}
		c.Program = "gradle"
		c.Report = `<report><package name="board"><sourcefile name="Main.kt"><line nr="4" mi="1" ci="0"/><line nr="5" mi="1" ci="0"/><line nr="7" mi="1" ci="0"/><line nr="11" mi="1" ci="0"/></sourcefile></package></report>`
		c.IntegrationReport = `<report><package name="board"><sourcefile name="Main.kt"><line nr="4" mi="0" ci="1"/><line nr="5" mi="0" ci="1"/><line nr="7" mi="0" ci="1"/><line nr="11" mi="0" ci="1"/></sourcefile></package></report>`
	default:
		return fmt.Errorf("unknown listed integration language %s", language)
	}
	files["tests.txt"] = first + "\t" + q.testFile + "\n" + second + "\t" + q.testFile + "\n"
	// JSON strings are YAML scalars too, avoiding a fixture quoting language.
	files["itos-cc.yaml"] = fmt.Sprintf("mutation:\n  tests:\n    list: %q\n    run: %q\n    ids_pattern: %q\n    join:\n      each: %q\n      sep: %q\n", list, run, pattern, each, sep)
	for file, text := range files {
		if err := q.m.w.write(file, text); err != nil {
			return err
		}
	}
	if err := q.m.w.d.Coverage(q.m.w.dir, c); err != nil {
		return err
	}
	surviving := Site{File: q.file, Line: ifLine, Original: ">", Replacement: ">="}
	inventory := &ListedTests{Files: map[string]string{first: q.testFile, second: q.testFile}, Lines: map[string]map[int][]string{q.file: {ifLine: {first, second}, positive: {first}, other: {second}}}, Split: true, Surviving: map[string][]Site{first: {surviving}, second: {surviving, {File: q.file, Line: ifLine, Original: "0", Replacement: "1"}}}}
	if language == "kotlin" {
		inventory.Lines[q.file][11] = []string{first, second}
	}
	// The list command prints tests.txt's lines, in their order.
	inventory.ListOutput = files["tests.txt"]
	for _, command := range commands {
		if err := q.m.w.d.Tests(q.m.w.dir, Tests{Language: language, Command: command, Surviving: []Site{surviving}, Listed: inventory}); err != nil {
			return err
		}
	}
	if language == "typescript" {
		// Vitest's related tests of the CLI are its own tests: no test
		// imports it, so every mutant passes them.
		sites, err := q.m.sitesOf(q.file)
		if err != nil {
			return err
		}
		own := Tests{Language: language, Command: "node_modules/.bin/vitest related --run --bail=1 " + q.file}
		for _, site := range sites {
			own.Surviving = append(own.Surviving, Site{File: q.file, Line: site.line, Original: site.original, Replacement: site.replacement})
		}
		if err := q.m.w.d.Tests(q.m.w.dir, own); err != nil {
			return err
		}
	}
	if language == "kotlin" {
		a, log, code, err := q.capture("crap --json --coverage-command 'gradle -q -p . jacocoJars test jacocoTestReport' --coverage-report build/reports/jacoco/test/jacocoTestReport.xml " + q.file)
		if err != nil {
			return err
		}
		if code != 0 {
			return fmt.Errorf("listed JaCoCo fixture setup exit%d: %v\n%s", code, a, log)
		}
	}
	a, log, code, err := q.capture("mutation run --json " + q.file)
	if err != nil {
		return err
	}
	if code != 1 {
		return fmt.Errorf("listed integration run exit%d: %v\n%s", code, a, log)
	}
	ms, err := q.integrationMutants(a)
	if err != nil {
		return err
	}
	want := map[int][]string{ifLine: {first, second}, positive: {first}, other: {second}}
	if language == "kotlin" {
		want[11] = []string{first, second}
	}
	for _, m := range ms {
		ids, err := strictStringList(m["tests"])
		if err != nil {
			return err
		}
		if m["scope"] != "listed" || !slices.Equal(ids, want[int(m["line"].(float64))]) || m["outcome"] == "uncovered" {
			return fmt.Errorf("listed attribution/provenance: %v want%v", m, want)
		}
	}
	// Prove the split harness really executed once for coverage; the legacy
	// fallback and unsplit/collector compatibility guards stay until T101.
	measured := 0
	for _, line := range strings.Split(log, "\n") {
		if strings.HasPrefix(line, "itos-cc: coverage ") && (strings.Contains(line, "test_listed.py -k") || strings.Contains(line, "src/listed.test.ts -t") || strings.Contains(line, "--tests board.ListedTest.")) {
			measured++
		}
	}
	if measured != 1 {
		return fmt.Errorf("listed split collector executions %d want1\n%s", measured, log)
	}
	return nil
}
func strictStringList(value any) ([]string, error) {
	if value == nil {
		return nil, nil
	}
	list, ok := value.([]any)
	if !ok {
		return nil, fmt.Errorf("not a JSON string list: %v", value)
	}
	out := make([]string, 0, len(list))
	for _, v := range list {
		s, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("nonstring list entry: %v", v)
		}
		out = append(out, s)
	}
	return out, nil
}
