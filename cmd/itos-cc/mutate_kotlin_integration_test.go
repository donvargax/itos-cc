package main

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/donvargax/itos-cc/metrics"
	"github.com/donvargax/itos-cc/project"
)

// The scenario of "Rule: Coverage from tests that start other processes, in
// every language" in features/mutate.feature that the
// integration-coverage-kotlin slice holds. Each project's Main.kt is run by
// its tests only in a JVM they start, java -cp <classes> board.MainKt N,
// which prints describe(N): no test calls it in-process. The one test of
// the first project runs it with 1, so it takes the positive branch, lines
// 4 and 5, and never line 7; it notices `0` → `1` and `+` → `-`, and not
// `>` → `>=`. The JVM starts with the agent ITOS_CC_JACOCO_AGENT names,
// writing to ITOS_CC_JACOCO_DESTDIR, as a harness opts in, and when
// JVM_RECORD names a file the test writes the destination there. Gradle's
// jacoco plugin fetches neither the runtime agent nor jacococli, so the
// build resolves both into Gradle's cache with a configuration of its own,
// which a build of the project fills before itos-cc runs, as a project's
// earlier builds would. They need a JDK and Gradle, and skip without them;
// CI runs them on ubuntu-latest, where the language tools are installed.

const (
	ktMainIfLine       = 4
	ktMainPositiveLine = 5
	ktMainOtherLine    = 7
)

const ktMainSource = "package board\n\n" +
	"fun describe(n: Int): Int {\n" +
	"    if (n > 0) {\n" +
	"        return n + 100\n" +
	"    }\n" +
	"    return n - 100\n" +
	"}\n\n" +
	"fun main(args: Array<String>) {\n" +
	"    println(describe(args[0].toInt()))\n" +
	"}\n"

// ktIntegrationBuild is the Gradle build: JaCoCo 0.8.15, and the
// configuration that resolves its runtime agent and jacococli into
// Gradle's cache when the task jacocoJars runs.
const ktIntegrationBuild = "plugins {\n    kotlin(\"jvm\") version \"2.4.21\"\n    jacoco\n}\n\n" +
	"repositories {\n    mavenCentral()\n}\n\n" +
	"dependencies {\n    testImplementation(kotlin(\"test\"))\n" +
	"    testRuntimeOnly(\"org.junit.platform:junit-platform-launcher\")\n}\n\n" +
	"jacoco {\n    toolVersion = \"0.8.15\"\n}\n\n" +
	"tasks.test {\n    useJUnitPlatform()\n}\n\n" +
	"tasks.jacocoTestReport {\n    reports {\n        xml.required = true\n    }\n}\n\n" +
	"val itosJacoco by configurations.creating {\n    isTransitive = false\n}\n\n" +
	"dependencies {\n" +
	"    itosJacoco(\"org.jacoco:org.jacoco.agent:0.8.15:runtime\")\n" +
	"    itosJacoco(\"org.jacoco:org.jacoco.cli:0.8.15:nodeps\")\n}\n\n" +
	"tasks.register(\"jacocoJars\") {\n    val jars: FileCollection = itosJacoco\n" +
	"    inputs.files(jars)\n    doLast {\n        jars.files.forEach { println(it) }\n    }\n}\n"

// ktRunner is the head of the test class class, whose run(name, arg) is
// what Main.kt prints in a JVM it starts. harness is how that JVM gets the
// agent: "agent", with the
// destination ITOS_CC_JACOCO_DESTDIR names; "split", with
// <ITOS_CC_TEST_COVERDIR>/<test> where that is set; "none", never.
func ktRunner(class, harness string) string {
	var agent string
	switch harness {
	case "agent", "split":
		dest := "System.getenv(\"ITOS_CC_JACOCO_DESTDIR\")"
		if harness == "split" {
			dest = "System.getenv(\"ITOS_CC_TEST_COVERDIR\")?.takeIf { it.isNotEmpty() }?.let { File(it, name).path } ?: " + dest
		}
		agent = "        val agent = System.getenv(\"ITOS_CC_JACOCO_AGENT\")\n" +
			"        val dest = " + dest + "\n" +
			"        if (!agent.isNullOrEmpty() && !dest.isNullOrEmpty()) {\n" +
			"            System.getenv(\"JVM_RECORD\")?.let { File(it).appendText(dest + \"\\n\") }\n" +
			"            command += \"-javaagent:$agent=destfile=$dest/$name-${UUID.randomUUID()}.exec\"\n" +
			"        }\n"
	}
	return "package board\n\nimport java.io.File\nimport java.util.UUID\nimport kotlin.test.Test\nimport kotlin.test.assertEquals\n\n" +
		"class " + class + " {\n" +
		"    private fun run(name: String, arg: String): String {\n" +
		"        val command = mutableListOf(File(System.getProperty(\"java.home\"), \"bin/java\").path)\n" + agent +
		"        val stdlib = File(KotlinVersion::class.java.protectionDomain.codeSource.location.toURI()).path\n" +
		"        val classes = File(\"build/classes/kotlin/main\").absolutePath\n" +
		"        command += listOf(\"-cp\", classes + File.pathSeparator + stdlib, \"board.MainKt\", arg)\n" +
		"        val process = ProcessBuilder(command).redirectErrorStream(true).start()\n" +
		"        val out = process.inputStream.bufferedReader().readText().trim()\n" +
		"        process.waitFor()\n" +
		"        return out\n" +
		"    }\n"
}

const (
	ktPositiveTest = "\n    @Test\n    fun positive() {\n        assertEquals(\"101\", run(\"positive\", \"1\"))\n    }\n"
	ktOtherTest    = "\n    @Test\n    fun other() {\n        assertEquals(\"-101\", run(\"other\", \"-1\"))\n    }\n"
)

// ktIntegrationFiles is the project whose one test starts Main.kt's JVM
// with 1, as harness says.
func ktIntegrationFiles(harness string) map[string]string {
	return map[string]string{
		".gitignore":                        ".gradle\nbuild\n",
		"settings.gradle.kts":               "rootProject.name = \"board\"\n",
		"build.gradle.kts":                  ktIntegrationBuild,
		"src/main/kotlin/board/Main.kt":     ktMainSource,
		"src/test/kotlin/board/MainTest.kt": ktRunner("MainTest", harness) + ktPositiveTest + "}\n",
	}
}

// kotlinIntegrationRepo makes a project of files in a new directory, has
// Gradle resolve the build's JaCoCo jars into its cache, makes the
// directory the test's (useDir), and returns the file the tests record the
// destination of their JVMs' agent in.
func kotlinIntegrationRepo(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, text := range files {
		writeFile(t, filepath.Join(dir, filepath.FromSlash(name)), text)
	}
	if out, err := exec.Command("gradle", "-q", "-p", dir, "jacocoJars").CombinedOutput(); err != nil {
		t.Fatalf("resolving the JaCoCo agent and jacococli into Gradle's cache: %v\n%s", err, out)
	}
	useDir(t, dir)
	record := filepath.Join(t.TempDir(), "record")
	useEnv(t, "JVM_RECORD", record)
	return record
}

// ktMainMutants is the mutants of describe in Main.kt in o's --json
// output, as plain maps.
func ktMainMutants(t *testing.T, o outcome) []map[string]any {
	t.Helper()
	for _, f := range o.rawFiles(t) {
		if filepath.ToSlash(f["file"].(string)) != "src/main/kotlin/board/Main.kt" {
			continue
		}
		var out []map[string]any
		list, _ := f["mutants"].([]any)
		for _, m := range list {
			if m := m.(map[string]any); mutantLine(m) >= ktMainIfLine && mutantLine(m) <= ktMainOtherLine {
				out = append(out, m)
			}
		}
		if len(out) != 4 {
			t.Fatalf("%d mutants of describe in Main.kt, want 4\n%s\nstderr:\n%s", len(out), o.stdout, o.stderr)
		}
		return out
	}
	t.Fatalf("no src/main/kotlin/board/Main.kt in files:\n%s\nstderr:\n%s", o.stdout, o.stderr)
	return nil
}

// ktMainMutant describes a mutant of Main.kt.
func ktMainMutant(m map[string]any) string {
	return strings.Replace(describe(m), greetSource, "Main.kt", 1)
}

// @ID-MUT-230
func TestLinesAKotlinTestReachesThroughAJVMItStartsAreCovered(t *testing.T) {
	t.Parallel()
	languageTools(t, "kotlin")
	main := filepath.FromSlash("src/main/kotlin/board/Main.kt")
	t.Run("whole suite", func(t *testing.T) {
		t.Parallel()
		// Given a Kotlin project built with Gradle whose only test starts
		// its main class in a JVM with the agent ITOS_CC_JACOCO_AGENT
		// names, and checks one branch of its output
		record := kotlinIntegrationRepo(t, ktIntegrationFiles("agent"))
		out := filepath.Join(metrics.DirOf(project.RootOf(wd(t))), "coverage")

		// When I run "itos-cc mutation run --all-tests --fail-uncovered --json"
		o := mutateCovered(t, "--all-tests", "--fail-uncovered", "--json", main)
		logRun(t, &o)

		// Then the lines the started JVM ran are covered, with "coverage"
		// listing "integration", and their mutants run
		// And a mutant the test notices is killed, one it does not notice
		// survives, and only mutants on lines never run are uncovered
		want := map[int]map[string]string{
			ktMainIfLine:       {">=": "survived", "1": "killed"},
			ktMainPositiveLine: {"-": "killed"},
			ktMainOtherLine:    {"+": "uncovered"},
		}
		for _, m := range ktMainMutants(t, o) {
			replacement, _ := m["replacement"].(string)
			if w, ok := want[mutantLine(m)][replacement]; !ok || m["outcome"] != w {
				t.Errorf("%s, want %q", ktMainMutant(m), w)
			}
			got := cliCoverage(m)
			if mutantLine(m) == ktMainOtherLine {
				if got != nil {
					t.Errorf("%s has \"coverage\" %q, want none", ktMainMutant(m), got)
				}
				continue
			}
			if !slices.Equal(got, []string{"integration"}) {
				t.Errorf("%s: \"coverage\" %q, want [\"integration\"]: only the started JVM ran its line, not the test's own", ktMainMutant(m), got)
			}
		}
		// The strict line evidence counts the started JVM's lines too: only
		// the line no JVM ran is an uncovered statement.
		if got, want := strictLineFindings(t, o), []string{"mutation.uncovered-statement src/main/kotlin/board/Main.kt board#describe:7"}; !slices.Equal(got, want) {
			t.Errorf("strict findings %q, want %q", got, want)
		}
		if o.code != 1 {
			t.Errorf("exit %d, want 1: the line never run holds an uncovered mutant", o.code)
		}

		// The JVM's coverage data stays with the run: under the run's own
		// run-* directory of .metrics/coverage/, and gone once it ends.
		data, err := os.ReadFile(record)
		if err != nil {
			t.Fatalf("the JVM was started without the agent, as ITOS_CC_JACOCO_AGENT or ITOS_CC_JACOCO_DESTDIR was unset: %v", err)
		}
		dir, _, _ := strings.Cut(string(data), "\n")
		rel, err := filepath.Rel(out, dir)
		first, _, _ := strings.Cut(filepath.ToSlash(rel), "/")
		if err != nil || !strings.HasPrefix(first, "run-") || first == rel {
			t.Fatalf("ITOS_CC_JACOCO_DESTDIR %s, want a directory under a run-* directory of %s", dir, out)
		}
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Errorf("ITOS_CC_JACOCO_DESTDIR %s is still there after the run (%v)", dir, err)
		}
		if _, err := os.Stat(filepath.Join(out, first)); !os.IsNotExist(err) {
			t.Errorf("the run's directory %s is still there after the run (%v)", first, err)
		}
		filepath.WalkDir(wd(t), func(path string, d fs.DirEntry, err error) error {
			if err == nil && d.IsDir() && (d.Name() == "build" || d.Name() == ".gradle") {
				return filepath.SkipDir
			}
			if err == nil && strings.HasSuffix(d.Name(), ".exec") {
				t.Errorf("%s is left in the project", path)
			}
			return nil
		})
	})
	// And with mutation.tests listing two tests that run different
	// branches, each line is covered by the IDs of the tests that reached it
	for _, split := range []bool{false, true} {
		name, harness := "listed tests, each run alone", "agent"
		if split {
			name, harness = "listed tests, split by the harness", "split"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			files := ktIntegrationFiles(harness)
			delete(files, "src/test/kotlin/board/MainTest.kt")
			files["src/test/kotlin/board/ListedTest.kt"] = ktRunner("ListedTest", harness) + ktPositiveTest + ktOtherTest + "}\n"
			files["tests.txt"] = "positive\tsrc/test/kotlin/board/ListedTest.kt\nother\tsrc/test/kotlin/board/ListedTest.kt\n"
			files["itos-cc.yaml"] = "mutation:\n  tests:\n" +
				"    list: cat tests.txt\n" +
				"    run: gradle -q -p . test --rerun {pattern}\n" +
				"    ids_pattern: \"{ids}\"\n" +
				"    join:\n      each: \"--tests board.ListedTest.{id}\"\n      sep: \" \"\n"
			kotlinIntegrationRepo(t, files)

			o := mutateCovered(t, "--json", main)
			logRun(t, &o)
			wantTests := map[int][]string{
				ktMainIfLine:       {"positive", "other"},
				ktMainPositiveLine: {"positive"},
				ktMainOtherLine:    {"other"},
			}
			for _, m := range ktMainMutants(t, o) {
				want := wantTests[mutantLine(m)]
				if got := listedTests(m); m["scope"] != "listed" || !slices.Equal(got, want) {
					t.Errorf("%s: scope %v, tests %q, want \"listed\" and %q, the tests that reach its line", ktMainMutant(m), m["scope"], got, want)
				}
			}
			var runs []string
			for _, l := range strings.Split(o.stderr, "\n") {
				if strings.HasPrefix(l, "itos-cc: coverage ") && strings.Contains(l, "--tests board.ListedTest.") {
					runs = append(runs, l)
				}
			}
			if wantRuns := map[bool]int{false: 3, true: 1}[split]; len(runs) != wantRuns {
				t.Errorf("coverage ran the listed tests as %q, want %d runs", runs, wantRuns)
			}
		})
	}
	// But a test that starts its JVM without the agent leaves those lines
	// uncovered, as before
	t.Run("without the agent", func(t *testing.T) {
		t.Parallel()
		kotlinIntegrationRepo(t, ktIntegrationFiles("none"))

		o := mutateCovered(t, "--all-tests", "--json", main)
		logRun(t, &o)
		for _, m := range ktMainMutants(t, o) {
			if m["outcome"] != "uncovered" || cliCoverage(m) != nil {
				t.Errorf("%s with \"coverage\" %q, want uncovered with none, as before: the JVM the test started was not measured", ktMainMutant(m), cliCoverage(m))
			}
		}
		for _, p := range o.json(t).Problems {
			if p["rule"] == "coverage.tool-missing" {
				t.Errorf("a run whose JVMs write nothing reports %v", p)
			}
		}
	})
}
