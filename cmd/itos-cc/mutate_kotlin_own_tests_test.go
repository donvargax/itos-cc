package main

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

// The Kotlin example of "Rule: A file's own tests are the tests that reach
// it, in every language" in features/mutate.feature, own-tests-kotlin's
// slice. Its project's test classes each append their name and the
// directory they ran in to a log through Record, a test-support object in
// src/test that declares no test, so the steps see which test classes each
// command ran: coverage runs in the project's tree, and the baseline and
// mutants in worker copies, which are gone once the run returns. ATest, in
// A.kt's package, calls low without importing it; BTest reaches only B.kt
// and fails; and CTest reaches no file the graph sees: it calls A.kt's
// spare by reflection, by names it spells in strings, the way a test that
// runs code through a subprocess or the CLI executes code it names in no
// import or reference. It needs a real JDK with Gradle, or Maven, and skips,
// naming the missing one, without them; CI runs it on ubuntu-latest (T-13).

// kotlinOwnSources are the project's sources and tests, by path from its
// root, each test logging its runs to runs.
func kotlinOwnSources(t *testing.T, runs string) map[string]string {
	t.Helper()
	literal, err := json.Marshal(runs)
	if err != nil {
		t.Fatal(err)
	}
	test := func(name, body string) string {
		return "package own\n\nimport kotlin.test.Test\nimport kotlin.test.assertEquals\nimport kotlin.test.assertFalse\n" +
			"import kotlin.test.assertTrue\nimport kotlin.test.fail\n\nclass " + name + " {\n    @Test\n    fun judges() {\n" +
			"        Record.ran(\"" + name + "\")\n" + body + "    }\n}\n"
	}
	return map[string]string{
		"src/main/kotlin/own/A.kt": "package own\n\nfun low(i: Int): Boolean {\n    return i == 3\n}\n\n" +
			"fun spare(i: Int): Boolean {\n    return i == 7\n}\n",
		"src/main/kotlin/own/B.kt": "package own\n\nfun high(i: Int): Boolean {\n    return i == 9\n}\n",
		"src/test/kotlin/own/Record.kt": "package own\n\nimport java.io.File\n\nobject Record {\n" +
			"    fun ran(name: String) {\n        File(" + string(literal) + ").appendText(name + \"\\t\" + File(\".\").canonicalPath + \"\\n\")\n    }\n}\n",
		"src/test/kotlin/own/ATest.kt": test("ATest", "        assertTrue(low(3))\n        assertFalse(low(4))\n"),
		"src/test/kotlin/own/BTest.kt": test("BTest", "        assertTrue(high(9))\n        fail(\"BTest fails\")\n"),
		"src/test/kotlin/own/CTest.kt": test("CTest", "        val method = Class.forName(\"own.\" + \"AKt\").getMethod(\"sp\" + \"are\", Int::class.javaPrimitiveType)\n"+
			"        assertEquals(true, method.invoke(null, 7))\n        assertEquals(false, method.invoke(null, 8))\n"),
	}
}

// kotlinOwnBuilds are the build files of the project by build tool, each
// measuring coverage with JaCoCo at a pinned version.
var kotlinOwnBuilds = map[string]map[string]string{
	"gradle": {
		"settings.gradle.kts": "rootProject.name = \"own\"\n",
		"build.gradle.kts": "plugins {\n    kotlin(\"jvm\") version \"2.4.21\"\n    jacoco\n}\n\n" +
			"repositories {\n    mavenCentral()\n}\n\n" +
			"dependencies {\n    testImplementation(kotlin(\"test\"))\n" +
			"    testRuntimeOnly(\"org.junit.platform:junit-platform-launcher\")\n}\n\n" +
			"jacoco {\n    toolVersion = \"0.8.15\"\n}\n\n" +
			"tasks.test {\n    useJUnitPlatform()\n}\n\n" +
			"tasks.jacocoTestReport {\n    reports {\n        xml.required = true\n    }\n}\n",
	},
	"maven": {
		"pom.xml": `<?xml version="1.0" encoding="UTF-8"?>
<project xmlns="http://maven.apache.org/POM/4.0.0">
  <modelVersion>4.0.0</modelVersion>
  <groupId>own</groupId>
  <artifactId>own</artifactId>
  <version>0.0.0</version>
  <properties>
    <project.build.sourceEncoding>UTF-8</project.build.sourceEncoding>
    <kotlin.version>2.4.21</kotlin.version>
  </properties>
  <dependencies>
    <dependency>
      <groupId>org.jetbrains.kotlin</groupId>
      <artifactId>kotlin-stdlib</artifactId>
      <version>${kotlin.version}</version>
    </dependency>
    <dependency>
      <groupId>org.jetbrains.kotlin</groupId>
      <artifactId>kotlin-test-junit5</artifactId>
      <version>${kotlin.version}</version>
      <scope>test</scope>
    </dependency>
  </dependencies>
  <build>
    <sourceDirectory>src/main/kotlin</sourceDirectory>
    <testSourceDirectory>src/test/kotlin</testSourceDirectory>
    <plugins>
      <plugin>
        <groupId>org.jetbrains.kotlin</groupId>
        <artifactId>kotlin-maven-plugin</artifactId>
        <version>${kotlin.version}</version>
        <executions>
          <execution><id>compile</id><goals><goal>compile</goal></goals></execution>
          <execution><id>test-compile</id><goals><goal>test-compile</goal></goals></execution>
        </executions>
      </plugin>
      <plugin>
        <groupId>org.apache.maven.plugins</groupId>
        <artifactId>maven-surefire-plugin</artifactId>
        <version>3.5.4</version>
      </plugin>
      <plugin>
        <groupId>org.jacoco</groupId>
        <artifactId>jacoco-maven-plugin</artifactId>
        <version>0.8.15</version>
      </plugin>
    </plugins>
  </build>
</project>
`,
	},
}

// @ID-MUT-217
func TestAKotlinFilesOwnTestsAreTheTestClassesThatReachIt(t *testing.T) {
	t.Parallel()
	for _, build := range []string{"gradle", "maven"} {
		t.Run(build, func(t *testing.T) {
			t.Parallel()
			languageTools(t, "kotlin")
			if build == "maven" {
				languageTools(t, "maven")
			}
			// Given a Kotlin project built with <build> where ATest, in
			// A.kt's package, uses A without importing it, BTest reaches
			// only B.kt, and BTest fails.
			dir := t.TempDir()
			runs := filepath.Join(t.TempDir(), "runs")
			for name, text := range kotlinOwnSources(t, runs) {
				writeFile(t, filepath.Join(dir, filepath.FromSlash(name)), text)
			}
			for name, text := range kotlinOwnBuilds[build] {
				writeFile(t, filepath.Join(dir, name), text)
			}
			useDir(t, dir)
			resolved, err := filepath.EvalSymlinks(dir)
			if err != nil {
				t.Fatal(err)
			}

			// When I run "itos-cc mutation run --json" for A.kt, coverage
			// measured.
			source := filepath.Join("src", "main", "kotlin", "own", "A.kt")
			o := mutateCovered(t, "--json", source)
			logPytestRun(t, &o)

			// Then A.kt's baseline passes and its mutant is judged by ATest
			// alone, with coverage measured from ATest alone: low's mutant
			// ATest kills, and spare, which only CTest executes, is
			// uncovered.
			f := o.json(t).file(t, source)
			if f.Baseline != "passed" {
				t.Errorf("A.kt: baseline %q, want passed", f.Baseline)
			}
			if m := ownMutantOf(t, f, "low"); m.Outcome != "killed" {
				t.Errorf("low's mutant: %+v, want it killed by ATest", m)
			}
			if m := ownMutantOf(t, f, "spare"); m.Outcome != "uncovered" {
				t.Errorf("spare's mutant: %+v, want it uncovered, as ATest never executes spare", m)
			}
			var inTree, inCopies []string
			for _, run := range ownTestRuns(t, runs) {
				if run[1] == resolved {
					inTree = append(inTree, run[0])
				} else {
					inCopies = append(inCopies, run[0])
				}
			}
			if !onlyTest(inTree, "ATest") {
				t.Errorf("coverage in the project's tree ran %v, want ATest alone", inTree)
			}
			if len(inCopies) < 2 || !onlyTest(inCopies, "ATest") {
				t.Errorf("the worker copies ran %v, want ATest alone, for the baseline and the mutant", inCopies)
			}

			// And BTest never runs.
			for _, run := range ownTestRuns(t, runs) {
				if run[0] == "BTest" {
					t.Errorf("BTest ran in %s", run[1])
				}
			}
		})
	}
}
