package features

import (
	"fmt"
	"slices"
	"strings"

	"github.com/cucumber/godog"
)

func (q *strictMutation) integrationSteps(sc *godog.ScenarioContext) {
	sc.Step(`^a Python project whose only test runs its module in a subprocess and checks one branch of its output$`, func() error { return q.integrationProject("python", false) })
	sc.Step(`^a TypeScript project with Vitest and c8 installed whose only test runs its CLI in a child Node process and checks one branch of its output$`, func() error { return q.integrationProject("typescript", false) })
	sc.Step(`^a Kotlin project built with Gradle whose only test starts its main class in a JVM with the agent ITOS_CC_JACOCO_AGENT names, and checks one branch of its output$`, func() error { return q.integrationProject("kotlin", false) })
	sc.Step(`^the lines the (?:subprocess|child process|started JVM) ran are covered, with "coverage" listing "integration", and their mutants run$`, q.integrationCoverage)
	sc.Step(`^a mutant the test notices is killed, one it does not notice survives, and only mutants on lines never run are uncovered$`, q.integrationOutcomes)
	sc.Step(`^with mutation.tests listing two tests that run different branches, each line is covered by the IDs of the tests that reached it$`, q.integrationListed)
	sc.Step(`^without c8 the run logs that integration coverage needs it and goes on, and a strict run fails with "coverage.tool-missing"$`, q.missingC8)
	sc.Step(`^a test that starts its JVM without the agent leaves those lines uncovered, as before$`, q.noAgent)
}
func (q *strictMutation) integrationProject(language string, without bool) error {
	q.language = language
	q.namespace = "cli"
	if language == "kotlin" {
		q.namespace = "board"
	}
	files := strictPyPythonIntegrationFiles
	c := Coverage{Language: language}
	command := ""
	ifLine := 6
	switch language {
	case "python":
		q.file = "cli.py"
		q.testFile = "test_cli.py"
		q.m.w.variables = append(q.m.w.variables, "PYTEST_DISABLE_PLUGIN_AUTOLOAD=1")
		command = "python3 -m pytest -q -x -p no:cacheprovider"
		c.Report = "SF:cli.py\nDA:6,0\nDA:7,0\nDA:8,0\nend_of_record\n"
		c.IntegrationReport = "SF:cli.py\nDA:6,1\nDA:7,1\nDA:8,0\nend_of_record\n"
	case "typescript":
		files = strictTsTypescriptIntegrationFiles
		q.file = "src/cli.ts"
		q.testFile = "src/cli.test.ts"
		ifLine = 4
		command = "node_modules/.bin/vitest run"
		c.Report = "SF:src/cli.ts\nDA:4,0\nDA:5,0\nDA:7,0\nend_of_record\n"
		c.IntegrationReport = "SF:src/cli.ts\nDA:4,1\nDA:5,1\nDA:7,0\nend_of_record\n"
		if without {
			c.WithoutC8 = true
			c.IntegrationReport = ""
		}
	case "kotlin":
		harness := "agent"
		if without {
			harness = "none"
		}
		files = strictKtKtIntegrationFiles(harness)
		q.file = "src/main/kotlin/board/Main.kt"
		q.testFile = "src/test/kotlin/board/MainTest.kt"
		ifLine = 4
		command = "gradle -p . test --fail-fast"
		c.Program = "gradle"
		c.Report = `<report><package name="board"><sourcefile name="Main.kt"><line nr="4" mi="1" ci="0"/><line nr="5" mi="1" ci="0"/><line nr="7" mi="1" ci="0"/><line nr="11" mi="1" ci="0"/></sourcefile></package></report>`
		c.IntegrationReport = `<report><package name="board"><sourcefile name="Main.kt"><line nr="4" mi="0" ci="1"/><line nr="5" mi="0" ci="1"/><line nr="7" mi="1" ci="0"/><line nr="11" mi="0" ci="1"/></sourcefile></package></report>`
		if without {
			c.IntegrationReport = ""
		}
	default:
		return fmt.Errorf("unknown integration language %s", language)
	}
	for file, text := range files {
		if err := q.m.w.write(file, text); err != nil {
			return err
		}
	}
	if err := q.m.w.d.Coverage(q.m.w.dir, c); err != nil {
		return err
	}
	t := Tests{Language: language, Command: command, Surviving: []Site{{File: q.file, Line: ifLine, Original: ">", Replacement: ">="}}}
	if err := q.m.w.d.Tests(q.m.w.dir, t); err != nil {
		return err
	}
	if language == "kotlin" {
		// This explicit public setup command resolves the build's pinned agent/CLI
		// into its cache, as the project's earlier builds would. The measured
		// mutation command itself still discovers and uses those actual jars.
		a, log, code, err := q.capture("crap --json --coverage-command 'gradle -q -p . jacocoJars test jacocoTestReport' --coverage-report build/reports/jacoco/test/jacocoTestReport.xml " + q.file)
		if err != nil {
			return err
		}
		if code != 0 {
			return fmt.Errorf("pinned JaCoCo fixture setup exit%d: %v\n%s", code, a, log)
		}
	}
	return nil
}
func (q *strictMutation) integrationMutants(a map[string]any) ([]map[string]any, error) {
	f, err := languageRunFile(a, q.file)
	if err != nil {
		return nil, err
	}
	out := objectList(f, "mutants")
	described, main := 0, 0
	for _, m := range out {
		if strictNamed(fmt.Sprint(m["function"]), "describe") {
			described++
		} else if q.language == "kotlin" && strictNamed(fmt.Sprint(m["function"]), "main") && m["line"] == float64(11) && m["original"] == "0" && m["replacement"] == "1" {
			main++
		} else {
			return nil, fmt.Errorf("unexpected integration mutant: %v", m)
		}
	}
	wantMain := 0
	if q.language == "kotlin" {
		wantMain = 1
	}
	if described != 4 || main != wantMain {
		return nil, fmt.Errorf("integration inventory describe%d/main%d want4/%d: %v", described, main, wantMain, f)
	}
	return out, nil
}
func (q *strictMutation) integrationCoverage() error {
	a, err := q.object()
	if err != nil {
		return err
	}
	ms, err := q.integrationMutants(a)
	if err != nil {
		return err
	}
	other := 7
	if q.language == "python" {
		other = 8
	}
	ran := 0
	for _, m := range ms {
		coverage, err := strictStringList(m["coverage"])
		if err != nil {
			return err
		}
		if m["line"] == float64(other) {
			if len(coverage) != 0 {
				return fmt.Errorf("unexecuted line claims coverage: %v", m)
			}
			continue
		}
		if !slices.Equal(coverage, []string{"integration"}) || m["outcome"] == "uncovered" || m["reused"] == true {
			return fmt.Errorf("subprocess line not actually judged with integration coverage: %v", m)
		}
		found := false
		for _, trial := range trialsOf(q.m.w.stderr, q.file) {
			if trial.line == int(m["line"].(float64)) && trial.original == m["original"] && trial.replacement == m["replacement"] {
				found = true
			}
		}
		if !found {
			return fmt.Errorf("missing actual integration mutant trial: %v", m)
		}
		ran++
	}
	wantRan := 3
	if q.language == "kotlin" {
		wantRan = 4
	}
	if ran != wantRan {
		return fmt.Errorf("integration judged population %d", ran)
	}
	return nil
}
func (q *strictMutation) integrationOutcomes() error {
	a, err := q.object()
	if err != nil {
		return err
	}
	ms, err := q.integrationMutants(a)
	if err != nil {
		return err
	}
	want := map[string]string{">=": "survived", "1": "killed", "-": "killed", "+": "uncovered"}
	for _, m := range ms {
		if m["outcome"] != want[fmt.Sprint(m["replacement"])] {
			return fmt.Errorf("integration outcome: %v want%s", m, want[fmt.Sprint(m["replacement"])])
		}
	}
	line := 7
	if q.language == "python" {
		line = 8
	}
	if err := q.finding(a, "mutation.uncovered-statement", "describe", line); err != nil {
		return err
	}
	return q.m.w.exitsWith(1)
}
func (q *strictMutation) integrationListed() error { return q.listedIntegrationRun() }
func (q *strictMutation) newIntegrationRoot() error {
	dir, err := q.m.w.d.Folder()
	if err != nil {
		return err
	}
	q.outside = append(q.outside, dir)
	q.m.w.dir = dir
	return nil
}
func (q *strictMutation) missingC8() error {
	old := q.m.w.dir
	defer func() { q.m.w.dir = old }()
	if err := q.newIntegrationRoot(); err != nil {
		return err
	}
	if err := q.integrationProject("typescript", true); err != nil {
		return err
	}
	strict, log, code, err := q.capture("mutation run --all-tests --fail-uncovered --json " + q.file)
	if err != nil {
		return err
	}
	found := false
	for _, p := range checkProblems(strict) {
		if p["rule"] == "coverage.tool-missing" && p["language"] == "typescript" && strings.Contains(fmt.Sprint(p["message"]), "c8") {
			found = true
		}
	}
	if code != 3 || !found || len(baselines(log)) != 0 {
		return fmt.Errorf("missing-c8 strict exit%d: %v\n%s", code, strict, log)
	}
	plain, log, code, err := q.capture("mutation run --all-tests --json " + q.file)
	if err != nil {
		return err
	}
	if code != 0 || !strings.Contains(log, "integration coverage") || !strings.Contains(log, "c8") {
		return fmt.Errorf("missing-c8 plain exit%d: %v\n%s", code, plain, log)
	}
	ms, err := q.integrationMutants(plain)
	if err != nil {
		return err
	}
	for _, m := range ms {
		if m["outcome"] != "uncovered" || m["coverage"] != nil {
			return fmt.Errorf("missing collector invented coverage: %v", m)
		}
	}
	return nil
}
func (q *strictMutation) noAgent() error {
	old := q.m.w.dir
	defer func() { q.m.w.dir = old }()
	if err := q.newIntegrationRoot(); err != nil {
		return err
	}
	if err := q.integrationProject("kotlin", true); err != nil {
		return err
	}
	a, _, code, err := q.capture("mutation run --all-tests --json " + q.file)
	if err != nil {
		return err
	}
	if code != 0 || len(q.findings(a, "coverage.tool-missing", "")) != 0 {
		return fmt.Errorf("no-agent control exit%d: %v", code, a)
	}
	ms, err := q.integrationMutants(a)
	if err != nil {
		return err
	}
	for _, m := range ms {
		if m["outcome"] != "uncovered" || m["coverage"] != nil {
			return fmt.Errorf("JVM without agent invented coverage: %v", m)
		}
	}
	return nil
}
