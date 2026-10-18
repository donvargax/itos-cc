package features

// Real subprocess collector fixtures retained from the legacy command guards.

const strictPyCliSource = `import os
import sys


def describe(n):
    if n > 0:
        return n + 100
    return n - 100


if __name__ == "__main__":
    record = os.environ.get("CLI_RECORD")
    if record:
        try:
            import coverage
        except ImportError:
            coverage = None
        cov = coverage and coverage.Coverage.current()
        if cov is not None:
            with open(record, "a") as f:
                f.write(cov.get_option("run:data_file") + "\n")
    print(describe(int(sys.argv[1])))
`

// strictPyCliRunner is the head of a test file that runs cli.py in a subprocess:
// run(test, arg) is what it prints. With a harness that splits coverage by
// test, the processes test starts write their coverage.py data to
// COVERAGE_FILE=<ITOS_CC_TEST_COVERDIR>/<test>/.coverage.
func strictPyCliRunner(split bool) string {
	harness := "    split = \"\"\n"
	if split {
		harness = "    split = os.environ.get(\"ITOS_CC_TEST_COVERDIR\")\n"
	}
	return "import os\nimport pathlib\nimport subprocess\nimport sys\n\n" +
		"CLI = pathlib.Path(__file__).resolve().parent / \"cli.py\"\n\n\n" +
		"def run(test, arg):\n" +
		"    env = dict(os.environ)\n" + harness +
		"    if split:\n" +
		"        env[\"COVERAGE_FILE\"] = os.path.join(split, test, \".coverage\")\n" +
		"    done = subprocess.run([sys.executable, str(CLI), arg], env=env, capture_output=True, text=True, check=True)\n" +
		"    return done.stdout.strip()\n\n\n"
}

var strictPyPythonIntegrationFiles = map[string]string{
	"pyproject.toml": "[project]\nname = \"cli\"\nversion = \"0.0.0\"\n",
	"cli.py":         strictPyCliSource,
	"test_cli.py":    strictPyCliRunner(false) + "def test_positive():\n    assert run(\"test_positive\", \"1\") == \"101\"\n",
}

const strictTsTsCliSource = `import { appendFileSync } from "node:fs";

export function describe(n: number): number {
  if (n > 0) {
    return n + 100;
  }
  return n - 100;
}

if (import.meta.main) {
  const record = process.env.CLI_RECORD;
  if (record && process.env.NODE_V8_COVERAGE) {
    appendFileSync(record, process.env.NODE_V8_COVERAGE + "\n");
  }
  console.log(describe(Number(process.argv[2])));
}
`

// strictTsTsCliRunner is the head of a Vitest test file that runs src/cli.ts in a
// child Node process: run(test, arg) is what it prints. With a harness that
// splits coverage by test, the processes test starts write their V8
// coverage to NODE_V8_COVERAGE=<ITOS_CC_TEST_COVERDIR>/<test>.
func strictTsTsCliRunner(split bool) string {
	harness := "  const split = \"\";\n"
	if split {
		harness = "  const split = process.env.ITOS_CC_TEST_COVERDIR;\n"
	}
	return "import { execFileSync } from \"node:child_process\";\nimport { join } from \"node:path\";\n" +
		"import { fileURLToPath } from \"node:url\";\nimport { expect, test } from \"vitest\";\n\n" +
		"const cli = fileURLToPath(new URL(\"./cli.ts\", import.meta.url));\n\n" +
		"function run(name: string, arg: string): string {\n" +
		"  const env = { ...process.env };\n" + harness +
		"  if (split) {\n    env.NODE_V8_COVERAGE = join(split, name);\n  }\n" +
		"  return execFileSync(process.execPath, [cli, arg], { encoding: \"utf8\", env }).trim();\n}\n\n"
}

const strictTsTsPackageJSON = `{"name": "cli", "private": true, "type": "module", ` +
	`"devDependencies": {"vitest": "5.0.2", "@vitest/coverage-v8": "5.0.2", "c8": "12.0.0"}}` + "\n"

var strictTsTypescriptIntegrationFiles = map[string]string{
	"package.json":    strictTsTsPackageJSON,
	"src/cli.ts":      strictTsTsCliSource,
	"src/cli.test.ts": strictTsTsCliRunner(false) + "test(\"positive\", () => {\n  expect(run(\"positive\", \"1\")).toBe(\"101\");\n});\n",
}

const strictKtKtMainSource = "package board\n\n" +
	"fun describe(n: Int): Int {\n" +
	"    if (n > 0) {\n" +
	"        return n + 100\n" +
	"    }\n" +
	"    return n - 100\n" +
	"}\n\n" +
	"fun main(args: Array<String>) {\n" +
	"    println(describe(args[0].toInt()))\n" +
	"}\n"

// strictKtKtIntegrationBuild is the Gradle build: JaCoCo 0.8.15, and the
// configuration that resolves its runtime agent and jacococli into
// Gradle's cache when the task jacocoJars runs.
const strictKtKtIntegrationBuild = "plugins {\n    kotlin(\"jvm\") version \"2.4.21\"\n    jacoco\n}\n\n" +
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

// strictKtKtRunner is the head of the test class class, whose run(name, arg) is
// what Main.kt prints in a JVM it starts. harness is how that JVM gets the
// agent: "agent", with the
// destination ITOS_CC_JACOCO_DESTDIR names; "split", with
// <ITOS_CC_TEST_COVERDIR>/<test> where that is set; "none", never.
func strictKtKtRunner(class, harness string) string {
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
	strictKtKtPositiveTest = "\n    @Test\n    fun positive() {\n        assertEquals(\"101\", run(\"positive\", \"1\"))\n    }\n"
	strictKtKtOtherTest    = "\n    @Test\n    fun other() {\n        assertEquals(\"-101\", run(\"other\", \"-1\"))\n    }\n"
)

// strictKtKtIntegrationFiles is the project whose one test starts Main.kt's JVM
// with 1, as harness says.
func strictKtKtIntegrationFiles(harness string) map[string]string {
	return map[string]string{
		".gitignore":                        ".gradle\nbuild\n",
		"settings.gradle.kts":               "rootProject.name = \"board\"\n",
		"build.gradle.kts":                  strictKtKtIntegrationBuild,
		"src/main/kotlin/board/Main.kt":     strictKtKtMainSource,
		"src/test/kotlin/board/MainTest.kt": strictKtKtRunner("MainTest", harness) + strictKtKtPositiveTest + "}\n",
	}
}
