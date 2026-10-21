package features

import (
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
)

// How scrap groups a test file's examples: by what encloses them, and by the
// setup they share.

// enclosure is a construct around an example, of a kind a sentence names:
// a describe block, a test class, a parent test.
type enclosure struct{ kind, name string }

// grouped is an example of a fixture and what encloses it, outermost first.
type grouped struct {
	name string
	in   []enclosure
}

// groupFixture is a test file of one framework and the examples it holds.
type groupFixture struct {
	file, source string
	examples     []grouped
}

// tsGrouped is a TypeScript test file of a framework, its import header
// and the words it writes its examples with: an example alone, describe,
// context and suite blocks around others, and one in a group call, a block
// no framework here groups by.
func tsGrouped(file, header, alone, inner string) groupFixture {
	source := header + "\n" +
		alone + `("stands alone", () => { expect(one()).toBe(1); });

describe("cart", () => {
  ` + inner + `("adds", () => { expect(add(1)).toBe(1); });
  context("when empty", () => {
    ` + inner + `("is zero", () => { expect(size()).toBe(0); });
  });
});

suite("totals", () => {
  ` + inner + `("sums", () => { expect(sum([1])).toBe(1); });
});

group("helpers", () => {
  ` + inner + `("formats", () => { expect(fmt(1)).toBe("1"); });
});
`
	cart := enclosure{"describe", "cart"}
	return groupFixture{file, source, []grouped{
		{"stands alone", nil},
		{"adds", []enclosure{cart}},
		{"is zero", []enclosure{cart, {"context", "when empty"}}},
		{"sums", []enclosure{{"suite", "totals"}}},
		{"formats", []enclosure{{"group", "helpers"}}},
	}}
}

// groupFixtures are the test files of each framework a scenario names.
var groupFixtures = map[string]map[string]groupFixture{
	"TypeScript": {
		"Vitest": tsGrouped("vitest.test.ts", `import { describe, it, test, expect } from "vitest";`+"\n", "test", "it"),
		"Jest":   tsGrouped("jest.test.ts", `import { describe, test, expect } from "@jest/globals";`+"\n", "test", "test"),
		"Mocha":  tsGrouped("mocha.test.ts", `import { expect } from "chai";`+"\n", "specify", "it"),
	},
	"Python": {
		"pytest": {"test_pytest_style.py", `def test_stands_alone():
    assert one() == 1


class TestCart:
    def test_adds(self):
        assert add(1) == 1

    class TestWhenEmpty:
        def test_is_zero(self):
            assert size() == 0
`, []grouped{
			{"test_stands_alone", nil},
			{"test_adds", []enclosure{{"class", "TestCart"}}},
			{"test_is_zero", []enclosure{{"class", "TestCart"}, {"class", "TestWhenEmpty"}}},
		}},
		"unittest": {"test_unittest_style.py", `import unittest


class TestParse(unittest.TestCase):
    def test_parses(self):
        self.assertEqual(parse("1"), 1)
`, []grouped{{"test_parses", []enclosure{{"class", "TestParse"}}}}},
	},
	"Go": {
		"testing": {"cart_test.go", `package cart

import "testing"

func TestStandsAlone(t *testing.T) {
    if One() != 1 {
        t.Fatal("one")
    }
}

func TestCart(t *testing.T) {
    t.Run("adds", func(t *testing.T) {
        if Add(1) != 1 {
            t.Fatal("add")
        }
    })
    t.Run("removes", func(t *testing.T) {
        if Remove(1) != 0 {
            t.Fatal("remove")
        }
    })
}
`, []grouped{
			{"TestStandsAlone", nil},
			{"adds", []enclosure{{"parent test", "TestCart"}}},
			{"removes", []enclosure{{"parent test", "TestCart"}}},
		}},
	},
	"Kotlin": {
		"JUnit": {"CartTest.kt", `class CartTest {
    @Test
    fun adds() {
        assertEquals(1, add(1))
    }

    @Nested
    inner class WhenEmpty {
        @Test
        fun isZero() {
            assertEquals(0, size())
        }
    }
}
`, []grouped{
			{"adds", []enclosure{{"class", "CartTest"}}},
			{"isZero", []enclosure{{"class", "CartTest"}, {"class", "WhenEmpty"}}},
		}},
		"kotest": {"CartSpec.kt", `class CartSpec : DescribeSpec({
    describe("cart") {
        it("adds") { add(1) shouldBe 1 }
        context("when empty") {
            it("is zero") { size() shouldBe 0 }
        }
    }
})

class TotalsSpec : FeatureSpec({
    feature("totals") {
        scenario("sums") { sum(listOf(1)) shouldBe 1 }
    }
})

class PriceSpec : BehaviorSpec({
    given("a price") {
        then("it formats") { fmt(1) shouldBe "1" }
    }
})

class AloneSpec : FunSpec({
    test("stands alone") { one() shouldBe 1 }
})
`, []grouped{
			{"adds", []enclosure{{"class", "CartSpec"}, {"describe", "cart"}}},
			{"is zero", []enclosure{{"class", "CartSpec"}, {"describe", "cart"}, {"context", "when empty"}}},
			{"sums", []enclosure{{"class", "TotalsSpec"}, {"feature", "totals"}}},
			{"it formats", []enclosure{{"class", "PriceSpec"}, {"given", "a price"}}},
			{"stands alone", []enclosure{{"class", "AloneSpec"}}},
		}},
	},
	"Dart": {
		"flutter_test": {"test/counter_test.dart", `import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  testWidgets('stands alone', (tester) async {
    await tester.pumpWidget(const MaterialApp(home: Text('1')));
    expect(find.text('1'), findsOneWidget);
  });

  group('cart', () {
    testWidgets('adds', (tester) async {
      expect(add(1), 1);
    });
    group('when empty', () {
      testWidgets('is zero', (tester) async {
        expect(size(), 0);
      });
    });
  });
}
`, []grouped{
			{"stands alone", nil},
			{"adds", []enclosure{{"group", "cart"}}},
			{"is zero", []enclosure{{"group", "cart"}, {"group", "when empty"}}},
		}},
		"package:test": {"test/totals_test.dart", `import 'package:test/test.dart';

void main() {
  test('stands alone', () {
    expect(one(), 1);
  });

  group('totals', () {
    test('sums', () => expect(sum([1]), 1));
  });
}
`, []grouped{
			{"stands alone", nil},
			{"sums", []enclosure{{"group", "totals"}}},
		}},
	},
}

// listItems are the items of a sentence's list: "a, b, or c", "a and b".
func listItems(list string) []string {
	list = strings.NewReplacer(", or ", ", ", ", and ", ", ", " or ", ", ", " and ", ", ").Replace(list)
	return strings.Split(list, ", ")
}

// testFileUsing writes the test files of each framework the list names, in
// the language.
func (d *scrapState) testFileUsing(language, frameworks string) error {
	d.expected = map[string][]grouped{}
	for _, framework := range listItems(frameworks) {
		f, ok := groupFixtures[language][framework]
		if !ok {
			return fmt.Errorf("no %s test file is written with %s", language, framework)
		}
		if err := d.writeFile(f.file, f.source); err != nil {
			return err
		}
		d.expected[f.file] = f.examples
	}
	return nil
}

// enclosingKinds are the kinds of construct a sentence names as an
// example's group: "describe, context, or suite block", "test class",
// "parent test, for t.Run subtests", "test class, or kotest describe …".
func enclosingKinds(phrase string) ([]string, error) {
	var kinds []string
	for _, item := range listItems(phrase) {
		item = strings.TrimSuffix(strings.TrimPrefix(item, "kotest "), " block")
		switch {
		case item == "test class":
			kinds = append(kinds, "class")
		case item == "for t.Run subtests":
			if !slices.Contains(kinds, "parent test") {
				return nil, fmt.Errorf("%q follows no parent test", item)
			}
		case strings.HasPrefix(item, "for "):
			return nil, fmt.Errorf("no fixture groups %q", item)
		default:
			kinds = append(kinds, item)
		}
	}
	return kinds, nil
}

// groupedBy is whether the examples of each file are exactly the fixture's,
// each grouped by the constructs around it of the kinds named, outermost
// first and joined by " › ", and by no other. The fixtures must hold a
// construct of each kind named.
func (d *scrapState) groupedBy(phrase string) error {
	kinds, err := enclosingKinds(phrase)
	if err != nil {
		return err
	}
	for _, kind := range kinds {
		found := false
		for _, examples := range d.expected {
			for _, e := range examples {
				found = found || slices.ContainsFunc(e.in, func(x enclosure) bool { return x.kind == kind })
			}
		}
		if !found {
			return fmt.Errorf("the fixtures hold no %s, so the step cannot fail", kind)
		}
	}
	if d.w.exit != 0 {
		return fmt.Errorf("itos-cc exited %d\n%s", d.w.exit, d.w.report())
	}
	reports, err := d.scrapIn(d.w.dir)
	if err != nil {
		return err
	}
	for file, examples := range d.expected {
		var want, got []string
		for _, e := range examples {
			var names []string
			for _, x := range e.in {
				if slices.Contains(kinds, x.kind) {
					names = append(names, x.name)
				}
			}
			want = append(want, strings.Join(names, " › ")+"/"+e.name)
		}
		for _, e := range reports[file].Details {
			got = append(got, e.Group+"/"+e.Name)
		}
		if !slices.Equal(got, want) {
			return fmt.Errorf("the examples of %s are\n  %s\nnot\n  %s", file, strings.Join(got, "\n  "), strings.Join(want, "\n  "))
		}
		if !slices.ContainsFunc(outputLines(filepath.ToSlash(d.w.stdout)), func(line string) bool {
			return strings.HasPrefix(line, file+"  ") && strings.Contains(line, fmt.Sprintf("  examples=%d  ", len(examples)))
		}) {
			return fmt.Errorf("the run prints no line of %s with its %d examples\n%s", file, len(examples), d.w.report())
		}
	}
	return nil
}

// goTableTest is a Go test that loops over its cases, asserting in the
// loop's body.
const goTableTest = `package a

func TestInc(t *testing.T) {
    cases := []struct{ in, want int }{
        {1, 2},
        {2, 3},
    }
    for _, c := range cases {
        if got := inc(c.in); got != c.want {
            t.Errorf("inc(%d) = %d", c.in, got)
        }
    }
}
`

// setupFixture is a test file with shared setup of setupLines lines, and
// the same file without it.
type setupFixture struct {
	file, with, without string
	setupLines          int
}

var setupFixtures = map[string]setupFixture{
	"beforeEach": {"cart.test.ts", `import { beforeEach, it, expect } from "vitest";

let cart;
beforeEach(() => {
  cart = new Cart();
});

it("adds", () => {
  expect(cart.add(1)).toBe(1);
});
`, `import { beforeEach, it, expect } from "vitest";

let cart;

it("adds", () => {
  expect(cart.add(1)).toBe(1);
});
`, 3},
	"fixtures": {"test_fixture_cart.py", `import pytest

@pytest.fixture
def cart():
    return Cart()

def test_adds(cart):
    assert cart.add(1) == 1
`, `import pytest

def test_adds(cart):
    assert cart.add(1) == 1
`, 3},
	"setUp": {"test_setup_cart.py", `import unittest

class TestCart(unittest.TestCase):
    def setUp(self):
        self.cart = Cart()

    def test_adds(self):
        self.assertEqual(self.cart.add(1), 1)
`, `import unittest

class TestCart(unittest.TestCase):
    def test_adds(self):
        self.assertEqual(self.cart.add(1), 1)
`, 2},
	"@BeforeEach": {"CartTest.kt", `class CartTest {
    @BeforeEach
    fun setUp() {
        cart = Cart()
    }

    @Test
    fun adds() {
        assertEquals(1, cart.add(1))
    }
}
`, `class CartTest {
    @Test
    fun adds() {
        assertEquals(1, cart.add(1))
    }
}
`, 4},
}

// blockIsSetup is whether the file's call to name, a block such as
// Dart's setUp(() { … }) from its line to the first line at its indent
// closing it, counts as setup: the file's setup lines are the block's,
// and its examples measure as they do without it.
func (d *scrapState) blockIsSetup(name string) error {
	text, err := d.w.readIn(d.w.dir, d.file)
	if err != nil {
		return err
	}
	lines := strings.SplitAfter(string(text), "\n")
	start, end := -1, -1
	for i, line := range lines {
		switch {
		case start < 0 && strings.HasPrefix(strings.TrimSpace(line), name+"("):
			start = i
		case start >= 0 && leading(line) == leading(lines[start]) && strings.HasPrefix(strings.TrimSpace(line), "}"):
			end = i
		}
		if end >= 0 {
			break
		}
	}
	if start < 0 || end < 0 {
		return fmt.Errorf("%s has no %s block, so the step cannot fail", d.file, name)
	}
	without := strings.Join(slices.Concat(lines[:start], lines[end+1:]), "")
	d.setups = append(d.setups, setupFixture{file: d.file, with: string(text), without: without, setupLines: end - start + 1})
	return d.setupNotSize()
}

// leading is the indent a line starts with.
func leading(line string) string {
	return line[:len(line)-len(strings.TrimLeft(line, " \t"))]
}

// testFileWithSetup writes a test file with each kind of setup the list
// names.
func (d *scrapState) testFileWithSetup(list string) error {
	for _, kind := range listItems(list) {
		f, ok := setupFixtures[kind]
		if !ok {
			return fmt.Errorf("no test file is written with %s", kind)
		}
		d.setups = append(d.setups, f)
		if err := d.writeFile(f.file, f.with); err != nil {
			return err
		}
	}
	return nil
}

// setupNotSize is whether each file's setup lines are its setup's, and its
// examples measure as they do in the same file without the setup, which
// has none.
func (d *scrapState) setupNotSize() error {
	without := filepath.Join(d.w.dir, "without")
	for _, f := range d.setups {
		if err := d.w.writeIn(without, f.file, f.without); err != nil {
			return err
		}
	}
	bare, err := d.scrapIn(without, "--verbose")
	if err != nil {
		return err
	}
	for _, f := range d.setups {
		r, err := d.measuredReport(f.file)
		if err != nil {
			return err
		}
		if r.SetupLines != f.setupLines {
			return fmt.Errorf("%s counts %d setup lines, not %d", f.file, r.SetupLines, f.setupLines)
		}
		b := bare[f.file]
		if b.SetupLines != 0 {
			return fmt.Errorf("%s without its setup counts %d setup lines", f.file, b.SetupLines)
		}
		if len(r.Details) == 0 || len(r.Details) != len(b.Details) {
			return fmt.Errorf("%s has examples %+v, and without its setup %+v", f.file, r.Details, b.Details)
		}
		for i, e := range r.Details {
			if e.Lines != b.Details[i].Lines || e.Score != b.Details[i].Score {
				return fmt.Errorf("%q of %s is %d lines scoring %v, and without the setup %d scoring %v",
					e.Name, f.file, e.Lines, e.Score, b.Details[i].Lines, b.Details[i].Score)
			}
		}
	}
	return nil
}

// kotlinMethod writes the Kotlin class of the scenario, annotated as it
// says, with a @Test method making count assertEquals.
func (d *scrapState) kotlinMethod(method, count string) error {
	if d.kotlinClass == "" {
		return errors.New("no Kotlin class to put the method in")
	}
	n, err := countOf(count)
	if err != nil {
		return err
	}
	var source strings.Builder
	source.WriteString("package demo\n\n")
	for _, a := range d.annotations {
		source.WriteString(a + "\n")
	}
	source.WriteString("class " + d.kotlinClass + " {\n    @Test\n    fun " + method + "() {\n")
	for i := range n {
		fmt.Fprintf(&source, "        assertEquals(%d, list().size)\n", i)
	}
	source.WriteString("    }\n}\n")
	return d.writeFile(d.file, source.String())
}
