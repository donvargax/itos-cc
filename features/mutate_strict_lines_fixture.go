package features

// Real collector fixtures retained from the legacy strict-line command guards.

var strictPythonFiles = map[string]string{
	"pyproject.toml":     "[project]\nname = \"m\"\nversion = \"0.0.0\"\n",
	"calc.py":            "def reached(i, strict):\n    if strict:\n        raise ValueError(\"strict\")\n    return i > 5\n\n\ndef dormant(name):\n    label = name.strip()\n    return label\n",
	"tests/test_calc.py": "from calc import reached\n\n\ndef test_reached():\n    assert reached(6, False)\n    assert not reached(5, False)\n",
	"lonely.py":          "def lonely(name):\n    return name.upper()\n",
}

var strictPythonLines = []string{
	"mutation.uncovered-statement calc.py calc#dormant:8",
	"mutation.uncovered-statement calc.py calc#dormant:9",
	"mutation.uncovered-statement calc.py calc#reached:3",
	"mutation.uncovered-statement lonely.py lonely#lonely:2",
}

var strictTypeScriptFiles = map[string]string{
	"package.json": `{"name": "strict", "private": true, "type": "module", ` +
		`"devDependencies": {"vitest": "5.0.2", "@vitest/coverage-v8": "5.0.2"}}` + "\n",
	"src/calc.ts": "export function reached(i: number, strict: boolean): boolean {\n  if (strict) {\n    throw new Error(\"strict\");\n  }\n  return i > 5;\n}\n\n" +
		"export function dormant(name: string): string {\n  const label = name.trim();\n  return label;\n}\n",
	"src/calc.test.ts": "import { expect, test } from \"vitest\";\nimport { reached } from \"./calc\";\n\n" +
		"test(\"reached\", () => {\n  expect(reached(6, false)).toBe(true);\n  expect(reached(5, false)).toBe(false);\n});\n",
	"src/lonely.ts": "export function lonely(name: string): string {\n  return name.toUpperCase();\n}\n",
}

var strictTypeScriptLines = []string{
	"mutation.uncovered-statement src/calc.ts calc#dormant:10",
	"mutation.uncovered-statement src/calc.ts calc#dormant:9",
	"mutation.uncovered-statement src/calc.ts calc#reached:3",
	"mutation.uncovered-statement src/lonely.ts lonely#lonely:2",
}

var strictKotlinFiles = map[string]string{
	"src/main/kotlin/calc/Calc.kt": "package calc\n\n" +
		"fun reached(i: Int, strict: Boolean, label: String = \"strict\"): Boolean {\n" +
		"    if (strict) {\n        throw IllegalStateException(label)\n    }\n" +
		"    return i > 5 && i < 100\n}\n\n" +
		"fun dormant(name: String, suffix: String = \"!\"): String {\n" +
		"    val label = name.trim() + suffix\n    return label\n}\n\n" +
		"data class Span(val from: String, val to: String)\n",
	"src/test/kotlin/calc/CalcTest.kt": "package calc\n\nimport kotlin.test.Test\nimport kotlin.test.assertFalse\nimport kotlin.test.assertTrue\n\n" +
		"class CalcTest {\n    @Test\n    fun reaches() {\n        assertTrue(reached(6, false))\n        assertFalse(reached(5, false))\n    }\n}\n",
	"src/main/kotlin/other/Lonely.kt": "package other\n\nfun lonely(name: String): String {\n    return name.uppercase()\n}\n",
}

var strictKotlinLines = []string{
	"mutation.uncovered-statement src/main/kotlin/calc/Calc.kt calc#dormant:11",
	"mutation.uncovered-statement src/main/kotlin/calc/Calc.kt calc#dormant:12",
	"mutation.uncovered-statement src/main/kotlin/calc/Calc.kt calc#reached:5",
	"mutation.uncovered-statement src/main/kotlin/other/Lonely.kt other#lonely:4",
}

// strictDartFiles are q-D's Flutter package: reached's line 3 never runs,
// line 5 runs only its first operand for reached(5, false), and dormant,
// which has no mutation site, never runs.
var strictDartFiles = map[string]string{
	"pubspec.yaml": coverageDartFlutterPubspec("calc", false),
	"lib/calc.dart": "bool reached(int i, bool strict) {\n  if (strict) {\n    throw ArgumentError('strict');\n  }\n  return i > 5 && i < 100;\n}\n\n" +
		"String dormant(String name) {\n  final label = name.trim();\n  return label;\n}\n",
	"test/calc_test.dart": "import 'package:flutter_test/flutter_test.dart';\nimport 'package:calc/calc.dart';\n\n" +
		"void main() {\n  test('reached', () {\n    expect(reached(6, false), isTrue);\n    expect(reached(5, false), isFalse);\n  });\n}\n",
}

var strictDartLines = []string{
	"mutation.uncovered-statement lib/calc.dart calc.calc#dormant:8",
	"mutation.uncovered-statement lib/calc.dart calc.calc#dormant:9",
	"mutation.uncovered-statement lib/calc.dart calc.calc#reached:3",
}

// strictDartFlutterTest is the flutter test command a Dart file's own
// tests run with, before their paths.
const strictDartFlutterTest = "flutter --suppress-analytics --no-version-check test --no-pub "

// strictDartLonely is MUT-272's package: strictDartFiles with
// lib/lonely.dart, which no test imports, judged too.
const strictDartLonely = "dart with lonely.dart"

const strictDartLonelySource = "int lonely(int i) {\n  return i + 1;\n}\n"

// strictDartLonelyMissing is what a strict run reports of lib/lonely.dart:
// flutter test's LCOV never names a library no test loads.
var strictDartLonelyMissing = []string{
	"mutation.coverage-missing lib/lonely.dart calc.lonely#lonely:1",
}
