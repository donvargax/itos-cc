package features

// The committed public fixtures preserve the changed-line examples.
type changedLinesExample struct {
	language string
	file     string
	files    map[string]string // the project at the base
	// changed and changedTest change the function's first line, and what
	// the test expects of it, as the change since the base does.
	changed, changedTest [2]string
	// added adds the new branch, as [old, new]. Its first line is unlike
	// the old branch's, so git diff cannot slide its hunk onto the old one.
	added [2]string
	// oldLine is where plain --fail-uncovered reports the old branch's
	// line that never runs, once the change is committed: Go's block starts
	// at its first statement, so either language reports that line.
	oldLine int
	// addedLine is where the added branch's line that never runs is
	// reported: one of the lines the further change added.
	addedLine int
}

var changedLinesExamples = []changedLinesExample{
	{
		language: "go",
		file:     "value.go",
		files: map[string]string{
			"go.mod": "module example.com/lines\n\ngo 1.22\n",
			"value.go": "package lines\n\n// Value is the default, or zero when flag is set.\n" +
				"func Value(flag bool) int {\n\tn := 1\n\tif flag {\n\t\tn = 0\n\t}\n\treturn n\n}\n",
			"value_test.go": "package lines\n\nimport \"testing\"\n\n" +
				"func TestValue(t *testing.T) {\n\tif Value(false) != 1 {\n\t\tt.Fatal(\"Value(false) is not the default\")\n\t}\n}\n",
		},
		changed:     [2]string{"\tn := 1\n", "\tn := 2\n"},
		changedTest: [2]string{"Value(false) != 1", "Value(false) != 2"},
		added:       [2]string{"\tn := 2\n", "\tn := 2\n\tif flag { // never taken either\n\t\tn = 5\n\t}\n"},
		oldLine:     7,
		addedLine:   7,
	},
	{
		language: "python",
		file:     "calc.py",
		files: map[string]string{
			"pyproject.toml":     "[project]\nname = \"m\"\nversion = \"0.0.0\"\n",
			"calc.py":            "def value(flag):\n    n = 1\n    if flag:\n        n = 0\n    return n\n",
			"tests/test_calc.py": "from calc import value\n\n\ndef test_value():\n    assert value(False) == 1\n",
		},
		changed:     [2]string{"    n = 1\n", "    n = 2\n"},
		changedTest: [2]string{"value(False) == 1", "value(False) == 2"},
		added:       [2]string{"    n = 2\n", "    n = 2\n    if flag:  # never taken either\n        n = 5\n"},
		oldLine:     4,
		addedLine:   4,
	},
}
