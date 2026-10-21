package features

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/cucumber/godog"
)

// What scrap counts as an example, and the measures and smells it gives each.

// isAnExampleWith is whether the file's examples include group/name with
// that many assertions.
func (d *scrapState) isAnExampleWith(group, name string, assertions int) error {
	r, err := d.measuredReport(d.file)
	if err != nil {
		return err
	}
	for _, e := range r.Details {
		if e.Group == group && e.Name == name {
			if e.Assertions != assertions {
				return fmt.Errorf("%s/%s has %d assertions, not %d", group, name, e.Assertions, assertions)
			}
			return nil
		}
	}
	return fmt.Errorf("%s/%s is no example of %s: %+v", group, name, d.file, r.Details)
}

// examplesAre is whether the file's examples are the table's, in order:
// each row's name and every other column the table has.
func (d *scrapState) examplesAre(table *godog.Table) error {
	r, err := d.measuredReport(d.file)
	if err != nil {
		return err
	}
	header := table.Rows[0].Cells
	var want, got []string
	for _, row := range table.Rows[1:] {
		var cells []string
		for _, c := range row.Cells {
			cells = append(cells, c.Value)
		}
		want = append(want, strings.Join(cells, " | "))
	}
	for _, e := range r.Details {
		var cells []string
		for _, h := range header {
			switch h.Value {
			case "example":
				cells = append(cells, scrapExampleName(e))
			case "assertions":
				cells = append(cells, strconv.Itoa(e.Assertions))
			case "decisions":
				cells = append(cells, strconv.Itoa(e.Decisions))
			case "mocks":
				cells = append(cells, strconv.Itoa(e.Mocks))
			case "lines":
				cells = append(cells, strconv.Itoa(e.Lines))
			case "table":
				cells = append(cells, map[bool]string{true: "yes", false: "no"}[e.Table])
			default:
				return fmt.Errorf("the table has a column %q no step reads", h.Value)
			}
		}
		got = append(got, strings.Join(cells, " | "))
	}
	if !slices.Equal(got, want) {
		return fmt.Errorf("the examples are\n  %s\nnot\n  %s", strings.Join(got, "\n  "), strings.Join(want, "\n  "))
	}
	return nil
}

// scrapExampleName is how a scenario names an example: by its name under its
// group, joined by /, as Kotlin's Class/method and Dart's group/title.
func scrapExampleName(e scrapExample) string {
	if e.Group == "" {
		return e.Name
	}
	return e.Group + "/" + e.Name
}

// exampleCallingHelpers writes the Go file of the scenario: its helper,
// failing only as the scenario says, an asserting helper, and a test
// calling each as often as it says.
func (d *scrapState) exampleCallingHelpers(helper, times, asserting string) error {
	if helper != d.helper {
		return fmt.Errorf("no helper %q was written", helper)
	}
	calls, err := countOf(times)
	if err != nil {
		return err
	}
	checks, err := countOf(asserting)
	if err != nil {
		return err
	}
	var test strings.Builder
	test.WriteString("func TestParse(t *testing.T) {\n")
	for i := range calls {
		fmt.Fprintf(&test, "    %s(t, \"%c\")\n", helper, 'a'+i)
	}
	for range checks {
		test.WriteString("    same(t, parse(\"a\"), 1)\n")
	}
	test.WriteString("}\n")
	return d.writeFile("helpers_test.go", "package a\n\nfunc "+helper+"(t *testing.T, path string) {\n    "+d.helperFailure+"\n}\n\n"+
		"func same(t *testing.T, got, want int) {\n    if got != want {\n        t.Errorf(\"got %d, want %d\", got, want)\n    }\n}\n\n"+test.String())
}

// exampleContaining writes one example of the language holding the code
// alone, a block it opens given a body.
func (d *scrapState) exampleContaining(language, code string) error {
	switch language {
	case "TypeScript":
		return d.writeFile("checks.test.ts", "it(\"checks\", () => {\n  "+code+";\n});\n")
	case "Python":
		if strings.HasSuffix(code, ":") {
			code += "\n        parse(\"x\")"
		}
		return d.writeFile("test_checks.py", "def test_checks():\n    "+code+"\n")
	case "Go":
		return d.writeFile("checks_test.go", "package a\n\nfunc TestChecks(t *testing.T) {\n    "+code+"\n}\n")
	case "Kotlin":
		return d.writeFile("ChecksTest.kt", "class ChecksTest {\n    @Test\n    fun checks() {\n        "+code+"\n    }\n}\n")
	case "Dart":
		return d.writeFile("test/checks_test.dart", "void main() {\n  test('checks', () {\n    "+code+";\n  });\n}\n")
	}
	return fmt.Errorf("no example is written in %s", language)
}

// callCounts is whether the one example's call is counted as an
// assertion, its only one, or not at all.
func (d *scrapState) callCounts(counts bool) error {
	e, err := d.onlyExample()
	if err != nil {
		return err
	}
	if want := map[bool]int{true: 1, false: 0}[counts]; e.Assertions != want {
		return fmt.Errorf("%q has %d assertions, not %d", e.Name, e.Assertions, want)
	}
	return nil
}

// caseTables are, for each way a scenario says a test's cases are written,
// a test of each form it names over count cases, by file.
func caseTables(count int) map[string]map[string]string {
	rows := func(row string) string { return strings.Repeat(row+"\n", count) }
	return map[string]map[string]string{
		"a Go []struct literal it ranges over, inline or named, with or without t.Run": {
			"inline_test.go": "package a\n\nfunc TestInc(t *testing.T) {\n    for _, c := range []struct{ in, want int }{\n" +
				rows("        {1, 2},") + "    } {\n        if got := inc(c.in); got != c.want {\n            t.Errorf(\"inc(%d) = %d\", c.in, got)\n        }\n    }\n}\n",
			"inline_run_test.go": "package a\n\nfunc TestInc(t *testing.T) {\n    for _, c := range []struct{ in, want int }{\n" +
				rows("        {1, 2},") + "    } {\n        t.Run(\"\", func(t *testing.T) {\n            if inc(c.in) != c.want {\n                t.Fail()\n            }\n        })\n    }\n}\n",
			"named_test.go": "package a\n\nfunc TestInc(t *testing.T) {\n    tests := []struct {\n        in, want int\n    }{\n" +
				rows("        {1, 2},") + "    }\n    for _, tt := range tests {\n        if got := inc(tt.in); got != tt.want {\n            t.Errorf(\"inc(%d) = %d\", tt.in, got)\n        }\n    }\n}\n",
			"named_run_test.go": "package a\n\nfunc TestInc(t *testing.T) {\n    tests := []struct {\n        in, want int\n    }{\n" +
				rows("        {1, 2},") + "    }\n    for _, tt := range tests {\n        t.Run(\"\", func(t *testing.T) {\n            if inc(tt.in) != tt.want {\n                t.Fail()\n            }\n        })\n    }\n}\n",
		},
		"a Python list it loops over, or a parametrize decorator": {
			"test_loop.py":        "def test_inc():\n    cases = [\n" + rows("        (1, 2),") + "    ]\n    for a, b in cases:\n        assert inc(a) == b\n",
			"test_parametrize.py": "@pytest.mark.parametrize(\"a,b\", [\n" + rows("    (1, 2),") + "])\ndef test_inc(a, b):\n    assert inc(a) == b\n",
		},
		"a TypeScript array it loops over with for…of, or it.each's argument": {
			"loop.test.ts": "it(\"incs\", () => {\n  const cases = [\n" + rows("    [1, 2],") + "  ];\n  for (const [a, b] of cases) {\n    expect(inc(a)).toBe(b);\n  }\n});\n",
			"each.test.ts": "it.each([\n" + rows("  [1, 2],") + "])(\"incs %i\", (a, b) => {\n  expect(inc(a)).toBe(b);\n});\n",
		},
		"a Kotlin listOf it loops over, or a @CsvSource": {
			"LoopTest.kt": "class LoopTest {\n  @Test fun inc() {\n    val cases = listOf(\n" + rows("      1 to 2,") + "    )\n    for ((a, b) in cases) { assertEquals(b, inc(a)) }\n  }\n}\n",
			"CsvTest.kt":  "class CsvTest {\n  @ParameterizedTest\n  @CsvSource(\n" + rows("    \"1, 2\",") + "  )\n  fun inc(a: Int, b: Int) { assertEquals(b, inc(a)) }\n}\n",
		},
		"a Dart list it loops over with for…in, calling test for each case": {
			"test/loop_test.dart": "void main() {\n  for (final (a, b) in [\n" + rows("    (1, 2),") + "  ]) {\n    test('inc $a', () {\n      expect(inc(a), b);\n    });\n  }\n}\n",
			"test/group_loop_test.dart": "void main() {\n  group('inc', () {\n    for (final c in [\n" + rows("      (in: 1, want: 2),") +
				"    ]) {\n      test('${c.in}', () => expect(inc(c.in), c.want));\n    }\n  });\n}\n",
		},
	}
}

// caseTables writes a test of each form the phrase names, over count cases.
func (d *scrapState) caseTables(count int, phrase string) error {
	files, ok := caseTables(count)[phrase]
	if !ok {
		return fmt.Errorf("no test is written whose cases are %s", phrase)
	}
	return d.writeFiles(files)
}

// fewLines is the most code lines a table of cases is "a few" of: what a
// screenful's quarter holds.
const fewLines = 12

// tablesAreSmall is whether each file's one example is a table of a few
// code lines, though its raw lines hold every case, and is not large.
func (d *scrapState) tablesAreSmall() error {
	for _, file := range d.files {
		r, err := d.measuredReport(file)
		if err != nil {
			return err
		}
		if len(r.Details) != 1 {
			return fmt.Errorf("%s has %d examples, not one", file, len(r.Details))
		}
		e := r.Details[0]
		if !e.Table || e.Lines > fewLines || e.RawLines <= fewLines || slices.Contains(e.Smells, "large") {
			return fmt.Errorf("%q of %s: table %v, %d code lines of %d raw, smells %v", e.Name, file, e.Table, e.Lines, e.RawLines, e.Smells)
		}
	}
	return nil
}

// noSplit is whether no file's action or recommendation asks to split it.
func (d *scrapState) noSplit() error {
	for _, file := range d.files {
		r, err := d.measuredReport(file)
		if err != nil {
			return err
		}
		if r.Action == "MANUAL_SPLIT" {
			return fmt.Errorf("%s's action is MANUAL_SPLIT", file)
		}
		for _, rec := range r.Recommendations {
			if strings.Contains(rec.Text, "split") {
				return fmt.Errorf("%s is asked to split: %s", file, rec.Text)
			}
		}
	}
	return nil
}

// tsExample is a TypeScript example of m's code lines, decisions, mocks and
// assertions, each one line, the rest calls that are none of them.
func tsExample(name string, m scrapMetrics) (string, error) {
	pad := m.Lines - 2 - m.Decisions - m.Mocks - m.Assertions
	if pad < 0 {
		return "", fmt.Errorf("%d lines hold no example of %d decisions, %d mocks and %d assertions", m.Lines, m.Decisions, m.Mocks, m.Assertions)
	}
	var body strings.Builder
	fmt.Fprintf(&body, "it(%q, () => {\n", name)
	for i := range m.Decisions {
		fmt.Fprintf(&body, "  if (flag%d) { run%d(); }\n", i, i)
	}
	for i := range m.Mocks {
		fmt.Fprintf(&body, "  const mock%d = vi.fn();\n", i)
	}
	for i := range m.Assertions {
		fmt.Fprintf(&body, "  expect(value%d).toBe(%d);\n", i, i)
	}
	for i := range pad {
		fmt.Fprintf(&body, "  step%d();\n", i)
	}
	body.WriteString("});\n")
	return body.String(), nil
}

// scored measures the example and checks it has what the scenario says.
func (d *scrapState) scored() error {
	if err := d.measure(); err != nil {
		return err
	}
	e, err := d.onlyExample()
	if err != nil {
		return err
	}
	if s := d.stated; s != nil && (e.Lines != s.Lines || e.Decisions != s.Decisions || e.Mocks != s.Mocks || e.Assertions != s.Assertions) {
		return fmt.Errorf("the example measures %+v, not %+v", e.scrapMetrics, *s)
	}
	return nil
}

// measureSmells are the measures a scenario gives an example a smell by:
// the example with the measure, and one just short of it.
var measureSmells = []struct {
	measure       *regexp.Regexp
	with, without func(n int) scrapMetrics
}{
	{regexp.MustCompile(`^no assertions$`),
		func(int) scrapMetrics { return scrapMetrics{Lines: 5} },
		func(int) scrapMetrics { return scrapMetrics{Lines: 5, Assertions: 1} }},
	{regexp.MustCompile(`^more than (\d+) code lines$`),
		func(n int) scrapMetrics { return scrapMetrics{Lines: n + 1, Assertions: 1} },
		func(n int) scrapMetrics { return scrapMetrics{Lines: n, Assertions: 1} }},
	{regexp.MustCompile(`^a branch or loop$`),
		func(int) scrapMetrics { return scrapMetrics{Lines: 5, Decisions: 1, Assertions: 1} },
		func(int) scrapMetrics { return scrapMetrics{Lines: 5, Assertions: 1} }},
	{regexp.MustCompile(`^(\d+) or more mocks$`),
		func(n int) scrapMetrics { return scrapMetrics{Lines: n + 3, Mocks: n, Assertions: 1} },
		func(n int) scrapMetrics { return scrapMetrics{Lines: n + 3, Mocks: n - 1, Assertions: 1} }},
	{regexp.MustCompile(`^more than (\d+) assertions$`),
		func(n int) scrapMetrics { return scrapMetrics{Lines: n + 3, Assertions: n + 1} },
		func(n int) scrapMetrics { return scrapMetrics{Lines: n + 3, Assertions: n} }},
}

// exampleWithMeasure writes an example with the measure, "with", and one
// just short of it, "without", each in a file of its own.
func (d *scrapState) exampleWithMeasure(measure string) error {
	for _, s := range measureSmells {
		m := s.measure.FindStringSubmatch(measure)
		if m == nil {
			continue
		}
		n := 0
		if len(m) > 1 {
			n, _ = strconv.Atoi(m[1])
		}
		with, err := tsExample("with", s.with(n))
		if err != nil {
			return err
		}
		without, err := tsExample("without", s.without(n))
		if err != nil {
			return err
		}
		if err := d.writeFile("with.test.ts", with); err != nil {
			return err
		}
		return d.writeFile("without.test.ts", without)
	}
	return fmt.Errorf("no example is written with %s", measure)
}

// hasSmell is whether the example with the measure has the smell and the
// one just short of it does not.
func (d *scrapState) hasSmell(smell string) error {
	for file, want := range map[string]bool{"with.test.ts": true, "without.test.ts": false} {
		r, err := d.measuredReport(file)
		if err != nil {
			return err
		}
		if len(r.Details) != 1 {
			return fmt.Errorf("%s has %d examples, not one", file, len(r.Details))
		}
		if e := r.Details[0]; slices.Contains(e.Smells, smell) != want {
			return fmt.Errorf("%q, %+v, has the smells %v: %q is %v there", e.Name, e.scrapMetrics, e.Smells, smell, want)
		}
	}
	return nil
}
