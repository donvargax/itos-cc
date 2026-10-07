package scrap

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/donvargax/itos-cc/lang"
)

func analyze(t *testing.T, path, src string) FileReport {
	t.Helper()
	f, err := lang.Parse(lang.Detect(path), path, []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(f.Close)
	return Analyze(f, path)
}

// summary renders each example as "group/name a=assertions d=decisions m=mocks [table]".
func summary(r FileReport) []string {
	var out []string
	for _, e := range r.Details {
		s := fmt.Sprintf("%s/%s a=%d d=%d m=%d", e.Group, e.Name, e.Assertions, e.Decisions, e.Mocks)
		if e.Table {
			s += " table"
		}
		out = append(out, s)
	}
	return out
}

func assertSummary(t *testing.T, r FileReport, want []string) {
	t.Helper()
	if got := summary(r); !slices.Equal(got, want) {
		t.Errorf("examples:\n got  %q\n want %q", got, want)
	}
}

func TestTypeScriptExamples(t *testing.T) {
	r := analyze(t, "cart.test.ts", `import { describe, it, expect, vi, beforeEach } from "vitest";

describe("cart", () => {
  beforeEach(() => { reset(); });
  it("adds", () => { expect(add(1)).toBe(1); expect(size()).toBe(1); });
  it.each([[1, 2], [2, 3]])("increments %i", (a, b) => { expect(inc(a)).toBe(b); });
  describe("checkout", () => {
    it("charges", () => {
      const pay = vi.fn();
      vi.spyOn(api, "post");
      vi.mock("./bank");
      for (const item of items) { if (item.price > 0) { pay(item); } }
    });
  });
  it.todo("refunds");
});
`)
	assertSummary(t, r, []string{
		"cart/adds a=2 d=0 m=0",
		"cart/increments %i a=1 d=0 m=0 table",
		"cart › checkout/charges a=0 d=2 m=3",
	})
	if r.SetupLines != 1 {
		t.Errorf("setup lines %d, want 1", r.SetupLines)
	}
}

func TestPythonExamples(t *testing.T) {
	r := analyze(t, "test_cart.py", `import pytest
from unittest.mock import patch

@pytest.fixture
def cart():
    return Cart()

def test_adds(cart):
    assert cart.add(1) == 1

@pytest.mark.parametrize("a,b", [(1, 2), (2, 3)])
def test_inc(a, b):
    assert inc(a) == b

class TestCheckout:
    @patch("shop.bank.charge")
    def test_charges(self, charge):
        with pytest.raises(ValueError):
            checkout(None)
        self.assertEqual(charge.call_count, 0)

    def helper(self):
        pass
`)
	assertSummary(t, r, []string{
		"/test_adds a=1 d=0 m=0",
		"/test_inc a=1 d=0 m=0 table",
		"TestCheckout/test_charges a=2 d=0 m=1",
	})
	if r.SetupLines != 3 {
		t.Errorf("setup lines %d, want the 3-line fixture", r.SetupLines)
	}
}

func TestGoExamples(t *testing.T) {
	r := analyze(t, "cart_test.go", `package cart

func TestAdds(t *testing.T) {
	if got := Add(1); got != 1 {
		t.Errorf("Add(1) = %d", got)
	}
}

func TestInc(t *testing.T) {
	for _, c := range []struct{ a, b int }{{1, 2}, {2, 3}} {
		t.Run(fmt.Sprint(c.a), func(t *testing.T) {
			if Inc(c.a) != c.b {
				t.Fatal("wrong")
			}
		})
	}
}

func TestCheckout(t *testing.T) {
	t.Run("charges", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		bank := NewMockBank(ctrl)
		bank.EXPECT().Charge(1)
		Checkout(bank)
	})
	t.Run("refunds", func(t *testing.T) {
		if Refund() {
			log(1)
		} else {
			t.Error("no refund")
		}
	})
}

func TestMain(m *testing.M) { os.Exit(m.Run()) }
`)
	assertSummary(t, r, []string{
		"/TestAdds a=1 d=0 m=0",
		"/TestInc a=1 d=0 m=0 table",
		"TestCheckout/charges a=0 d=0 m=3",
		// An if with an else is logic, even when one side reports a failure.
		"TestCheckout/refunds a=1 d=1 m=0",
	})
}

func TestKotlinExamples(t *testing.T) {
	r := analyze(t, "CartTest.kt", `class CartTest {
    @BeforeEach
    fun setUp() { cart = Cart() }

    @Test
    fun adds() {
        cart.add(1) shouldBe 1
        assertEquals(1, cart.size)
    }

    @ParameterizedTest
    @ValueSource(ints = [1, 2])
    fun increments(a: Int) { assertTrue(inc(a) > a) }

    @Test
    fun charges() {
        val bank = mockk<Bank>()
        every { bank.charge(any()) } returns true
        checkout(bank)
        verify { bank.charge(1) }
    }
}

class CartSpec : FunSpec({
    context("cart") {
        test("adds") { add(1) shouldBe 1 }
    }
})
`)
	assertSummary(t, r, []string{
		"CartTest/adds a=2 d=0 m=0",
		"CartTest/increments a=1 d=0 m=0 table",
		"CartTest/charges a=1 d=0 m=2",
		"CartSpec › cart/adds a=1 d=0 m=0",
	})
	if r.SetupLines != 2 {
		t.Errorf("setup lines %d, want 2", r.SetupLines)
	}
}

func TestAssertionHelpersAndCaseLoops(t *testing.T) {
	r := analyze(t, "x_test.go", `package x

func assertParsed(t *testing.T, in string, want int) {
	t.Helper()
	if got := Parse(in); got != want {
		t.Errorf("Parse(%q) = %d", in, want)
	}
}

func TestParse(t *testing.T) {
	assertParsed(t, "1", 1)
}

func TestCases(t *testing.T) {
	for in, want := range map[string]int{"1": 1, "2": 2} {
		if got := Parse(in); got != want || got < 0 {
			t.Errorf("Parse(%q) = %d", in, got)
		}
	}
}

func TestCheckout(t *testing.T) {
	checkout(cart)
}
`)
	assertSummary(t, r, []string{
		"/TestParse a=1 d=0 m=0",
		"/TestCases a=1 d=0 m=0 table",
		// checkout is not an assertion helper.
		"/TestCheckout a=0 d=0 m=0",
	})
}

func TestFixtureTextIsNotTestSize(t *testing.T) {
	r := analyze(t, "test_x.py", "def test_parses():\n    src = \"\"\"\n"+strings.Repeat("line\n", 40)+"\"\"\"\n    assert parse(src)\n")
	e := r.Details[0]
	if e.RawLines != 44 || e.Lines != 3 {
		t.Errorf("lines %d raw %d, want 3 code lines of 44", e.Lines, e.RawLines)
	}
}

// rows repeats row n times, one case per line.
func rows(row string, n int) string {
	return strings.Repeat(row+"\n", n)
}

func TestCaseTablesAreNotTestSize(t *testing.T) {
	for name, tc := range map[string]struct{ path, src string }{
		"go inline": {"a_test.go", "package a\n\nfunc TestInc(t *testing.T) {\n\tfor _, c := range []struct{ in, want int }{\n" +
			rows("\t\t{1, 2},", 44) + "\t} {\n\t\tif got := inc(c.in); got != c.want {\n\t\t\tt.Errorf(\"inc(%d) = %d\", c.in, got)\n\t\t}\n\t}\n}\n"},
		"go named, t.Run": {"a_test.go", "package a\n\nfunc TestInc(t *testing.T) {\n\ttests := []struct {\n\t\tin, want int\n\t}{\n" +
			rows("\t\t{1, 2},", 44) + "\t}\n\tfor _, tt := range tests {\n\t\tt.Run(\"\", func(t *testing.T) {\n\t\t\tif inc(tt.in) != tt.want {\n\t\t\t\tt.Fail()\n\t\t\t}\n\t\t})\n\t}\n}\n"},
		"python loop":        {"test_a.py", "def test_inc():\n    cases = [\n" + rows("        (1, 2),", 44) + "    ]\n    for a, b in cases:\n        assert inc(a) == b\n"},
		"python parametrize": {"test_a.py", "@pytest.mark.parametrize(\"a,b\", [\n" + rows("    (1, 2),", 44) + "])\ndef test_inc(a, b):\n    assert inc(a) == b\n"},
		"typescript loop":    {"a.test.ts", "it(\"incs\", () => {\n  const cases = [\n" + rows("    [1, 2],", 44) + "  ];\n  for (const [a, b] of cases) {\n    expect(inc(a)).toBe(b);\n  }\n});\n"},
		"typescript each":    {"a.test.ts", "it.each([\n" + rows("  [1, 2],", 44) + "])(\"incs %i\", (a, b) => {\n  expect(inc(a)).toBe(b);\n});\n"},
		"kotlin loop":        {"ATest.kt", "class ATest {\n  @Test fun inc() {\n    val cases = listOf(\n" + rows("      1 to 2,", 44) + "    )\n    for ((a, b) in cases) { assertEquals(b, inc(a)) }\n  }\n}\n"},
		"kotlin csv":         {"ATest.kt", "class ATest {\n  @ParameterizedTest\n  @CsvSource(\n" + rows("    \"1, 2\",", 44) + "  )\n  fun inc(a: Int, b: Int) { assertEquals(b, inc(a)) }\n}\n"},
	} {
		t.Run(name, func(t *testing.T) {
			r := analyze(t, tc.path, tc.src)
			if len(r.Details) != 1 {
				t.Fatalf("examples %q, want one", summary(r))
			}
			e := r.Details[0]
			if !e.Table || e.Lines > 12 || e.RawLines < 44 || slices.Contains(e.Smells, "large") {
				t.Errorf("table %v, %d code lines of %d, smells %v: want a table of a few code lines", e.Table, e.Lines, e.RawLines, e.Smells)
			}
			if r.Action != LeaveAlone {
				t.Errorf("action %s, recommendations %+v: a table-driven test is the shape scrap asks for", r.Action, r.Recommendations)
			}
		})
	}
}

func TestScore(t *testing.T) {
	if s := Score(Metrics{Lines: 8, Assertions: 2}); s != 1 {
		t.Errorf("a small asserted example scores %v, want 1", s)
	}
	if s := Score(Metrics{Lines: 8}); s != 7 {
		t.Errorf("an example without assertions scores %v, want 7", s)
	}
	if s := Score(Metrics{Lines: 35, Decisions: 2, Mocks: 4, Assertions: 1}); s != 13 {
		t.Errorf("large, branching, mocked example scores %v, want 1+4+5+3=13", s)
	}
}

// coverageMatrix is many small examples that differ only in their data.
const coverageMatrix = `import { it, expect } from "vitest";

it("parses one", () => { expect(parse("1")).toBe(1); });
it("parses two", () => { expect(parse("2")).toBe(2); });
it("parses ten", () => { expect(parse("10")).toBe(10); });
it("parses neg", () => { expect(parse("-4")).toBe(-4); });
`

func TestCoverageMatrixAsksForATable(t *testing.T) {
	r := analyze(t, "parse.test.ts", coverageMatrix)
	if r.Action != AutoTableDrive {
		t.Fatalf("action %s, want %s; clusters %+v", r.Action, AutoTableDrive, r.Clusters)
	}
	if len(r.Clusters) != 1 || r.Clusters[0].Kind != "table" || len(r.Clusters[0].Examples) != 4 {
		t.Fatalf("clusters %+v, want one table of 4", r.Clusters)
	}
	if !strings.Contains(r.Recommendations[0].Text, "it.each") {
		t.Errorf("recommendation %q should name the Vitest idiom", r.Recommendations[0].Text)
	}
}

func TestCleanFileIsLeftAlone(t *testing.T) {
	r := analyze(t, "test_ok.py", "def test_adds():\n    assert add(1, 2) == 3\n\ndef test_parses():\n    assert parse('x') == {'x': None}\n")
	if r.Action != LeaveAlone || r.Pressure != 0 || len(r.Recommendations) != 0 {
		t.Errorf("action %s pressure %v recommendations %v", r.Action, r.Pressure, r.Recommendations)
	}
}

func TestMissingAssertionsAskForRefactoring(t *testing.T) {
	r := analyze(t, "test_x.py", "def test_runs():\n    run()\n")
	if r.Action != AutoRefactor || r.Recommendations[0].Confidence != "HIGH" {
		t.Errorf("action %s recommendations %+v", r.Action, r.Recommendations)
	}
}

func TestCompare(t *testing.T) {
	r := FileReport{Pressure: 3}
	r.CompareTo(FileReport{Pressure: 10})
	if r.Compare.Verdict != "improved" || r.Compare.Delta != -7 {
		t.Errorf("compare %+v", r.Compare)
	}
	r.CompareTo(FileReport{Pressure: 3.2})
	if r.Compare.Verdict != "unchanged" {
		t.Errorf("compare %+v", r.Compare)
	}
}

func TestKotlinMultiAnnotatedTestClass(t *testing.T) {
	r := analyze(t, "ApiTest.kt", "package demo\n\n@SpringBootTest\n@AutoConfigureMockMvc\nclass ApiTest {\n    @Test\n    fun lists() { assertEquals(1, list().size) }\n}\n")
	assertSummary(t, r, []string{"ApiTest/lists a=1 d=0 m=0"})
}
