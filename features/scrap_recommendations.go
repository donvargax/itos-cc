package features

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// The recommendations scrap makes for a test file, and the order it ranks
// them in.

// dataOnly are, by language, four small examples that differ only in their
// data.
var dataOnly = map[string][2]string{
	"TypeScript": {"parse.test.ts", `it("parses one", () => { expect(parse("1")).toBe(1); });
it("parses two", () => { expect(parse("2")).toBe(2); });
it("parses ten", () => { expect(parse("10")).toBe(10); });
it("parses neg", () => { expect(parse("-4")).toBe(-4); });
`},
	"Python": {"test_parse.py", `def test_parses_one():
    assert parse("1") == 1

def test_parses_two():
    assert parse("2") == 2

def test_parses_ten():
    assert parse("10") == 10

def test_parses_neg():
    assert parse("-4") == -4
`},
	"Go": {"parse_test.go", `package parse

func TestParsesOne(t *testing.T) {
    if got := Parse("1"); got != 1 {
        t.Errorf("Parse = %d", got)
    }
}

func TestParsesTwo(t *testing.T) {
    if got := Parse("2"); got != 2 {
        t.Errorf("Parse = %d", got)
    }
}

func TestParsesTen(t *testing.T) {
    if got := Parse("10"); got != 10 {
        t.Errorf("Parse = %d", got)
    }
}

func TestParsesNeg(t *testing.T) {
    if got := Parse("-4"); got != -4 {
        t.Errorf("Parse = %d", got)
    }
}
`},
	"Kotlin": {"ParseTest.kt", `class ParseTest {
    @Test
    fun parsesOne() { assertEquals(1, parse("1")) }

    @Test
    fun parsesTwo() { assertEquals(2, parse("2")) }

    @Test
    fun parsesTen() { assertEquals(10, parse("10")) }

    @Test
    fun parsesNeg() { assertEquals(-4, parse("-4")) }
}
`},
}

func (d *scrapState) dataOnlyFile(language string) error {
	f, ok := dataOnly[language]
	if !ok {
		return fmt.Errorf("no %s test file is written", language)
	}
	return d.writeFile(f[0], f[1])
}

// recommendationNames is whether a recommendation of the file says to fold
// examples into the idiom.
func (d *scrapState) recommendationNames(idiom string) error {
	r, err := d.measuredReport(d.file)
	if err != nil {
		return err
	}
	for _, rec := range r.Recommendations {
		if strings.Contains(rec.Text, "fold them into "+idiom+":") {
			return nil
		}
	}
	return fmt.Errorf("no recommendation of %s folds its examples into %s: %+v", d.file, idiom, r.Recommendations)
}

// words are a text's words, lower-cased, without punctuation or articles.
func words(text string) []string {
	var out []string
	for _, word := range strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-' && r != '_' && r != '.' && r != '/'
	}) {
		word = strings.Trim(word, ".")
		if word != "" && !slices.Contains([]string{"a", "an", "the", "this"}, word) {
			out = append(out, word)
		}
	}
	return out
}

// saysInOrder is whether text holds every word of what, in its order.
func saysInOrder(text, what string) bool {
	rest := words(text)
	for _, word := range words(what) {
		i := slices.Index(rest, word)
		if i < 0 {
			return false
		}
		rest = rest[i+1:]
	}
	return true
}

// aRecommendationSays is whether a recommendation of the file, of the
// confidence when one is given, says what the sentence says.
func (d *scrapState) aRecommendationSays(confidence, says string) error {
	r, err := d.its()
	if err != nil {
		return err
	}
	for _, rec := range r.Recommendations {
		if (confidence == "" || rec.Confidence == confidence) && saysInOrder(rec.Text, says) {
			return nil
		}
	}
	return fmt.Errorf("no %s recommendation of %s says to %s: %+v", confidence, r.File, says, r.Recommendations)
}

// alikeExamples writes three examples of sixteen lines whose bodies differ
// in one call, 0.9 alike as scrap rounds it, and checks scrap clusters all
// three at least percent alike, each larger than a table's example.
func (d *scrapState) alikeExamples(percent int) error {
	var source strings.Builder
	for i, step := range []string{"order.ship(customer.address);", "notify(customer.address);", "archive(order);"} {
		fmt.Fprintf(&source, `it("checks out order %d", () => {
  const cart = new Cart();
  cart.add({ sku: "a", price: 10 });
  cart.add({ sku: "b", price: 10 });
  cart.add({ sku: "c", price: 10 });
  const customer = new Customer("ada");
  customer.address = new Address("1 Main St");
  const payment = new Payment(customer, "card");
  payment.authorize(cart.total());
  const order = checkout(cart, customer, payment);
  order.confirm();
  %s
  const receipt = order.receipt();
  receipt.print();
  expect(order.total).toBe(30);
});

`, i+1, step)
	}
	if err := d.writeFile("checkout.test.ts", source.String()); err != nil {
		return err
	}
	reports, err := d.scrapIn(d.w.dir, d.file)
	if err != nil {
		return err
	}
	r := reports[d.file]
	for _, c := range r.Clusters {
		if len(c.Examples) == len(r.Details) && c.Similarity >= float64(percent)/100 {
			for _, e := range r.Details {
				if e.Lines <= fewLines {
					return fmt.Errorf("%q is %d lines, no large example", e.Name, e.Lines)
				}
			}
			return nil
		}
	}
	return fmt.Errorf("scrap clusters no three examples of %s at least %d%% alike: %+v", d.file, percent, r.Clusters)
}

// mockedFile writes count examples, mocked of them setting up mocks mocks,
// and checks scrap measures them so.
func (d *scrapState) mockedFile(count, mocked, mocks int) error {
	var source strings.Builder
	for i := range count {
		m := scrapMetrics{Lines: mocks + 4, Assertions: 1}
		if i < mocked {
			m.Mocks = mocks
		}
		example, err := tsExample(fmt.Sprintf("example %d", i+1), m)
		if err != nil {
			return err
		}
		source.WriteString(example + "\n")
	}
	if err := d.writeFile("mocked.test.ts", source.String()); err != nil {
		return err
	}
	reports, err := d.scrapIn(d.w.dir, d.file)
	if err != nil {
		return err
	}
	r, heavy := reports[d.file], 0
	for _, e := range r.Details {
		if e.Mocks >= mocks {
			heavy++
		}
	}
	if r.Examples != count || heavy != mocked {
		return fmt.Errorf("scrap finds %d examples, %d of them with %d or more mocks", r.Examples, heavy, mocks)
	}
	return nil
}

// troubledFile writes the file of d.examples examples, hot of them scoring
// at least score, spread round-robin across groups describe blocks, the
// others asserting and plain, and checks scrap measures them so.
func (d *scrapState) troubledFile(hot int, score float64, groups int) error {
	if d.examples < hot {
		return fmt.Errorf("%d examples hold no %d scoring %v or more", d.examples, hot, score)
	}
	blocks := make([]strings.Builder, groups)
	for i := range d.examples {
		var example string
		if i < hot {
			// Two branches and an assertion score 6.
			example = fmt.Sprintf("  it(\"troubled %d\", () => {\n    if (a%d) { run(); }\n    if (b%d) { run(); }\n    expect(value).toBe(%d);\n  });\n", i+1, i, i, i)
		} else {
			example = fmt.Sprintf("  it(\"plain %d\", () => {\n    expect(value).toBe(%d);\n  });\n", i+1, i)
		}
		_, _ = blocks[i%groups].WriteString(example) // a strings.Builder's write never fails
	}
	var source strings.Builder
	for i := range blocks {
		fmt.Fprintf(&source, "describe(\"part %d\", () => {\n%s});\n\n", i+1, blocks[i].String())
	}
	if err := d.writeFile("troubled.test.ts", source.String()); err != nil {
		return err
	}
	reports, err := d.scrapIn(d.w.dir, d.file)
	if err != nil {
		return err
	}
	r := reports[d.file]
	var found []string
	spread := map[string]bool{}
	for _, e := range r.Details {
		if e.Score >= score {
			found = append(found, e.Name)
			spread[e.Group] = true
		}
	}
	if r.Examples != d.examples || len(found) != hot || len(spread) != groups {
		return fmt.Errorf("scrap finds %d examples, %v scoring %v or more, across %d groups", r.Examples, found, score, len(spread))
	}
	return nil
}

// manyProblems is a TypeScript test file with more problems than
// recommendations are listed: four examples asserting nothing, two too
// large, three with logic, two mock-heavy, one asserting too much.
func manyProblems() string {
	var source strings.Builder
	write := func(name string, m scrapMetrics) {
		example, _ := tsExample(name, m)
		source.WriteString(example + "\n")
	}
	for i := range 4 {
		write(fmt.Sprintf("asserts nothing %d", i+1), scrapMetrics{Lines: 3 + i})
	}
	for i := range 2 {
		write(fmt.Sprintf("too large %d", i+1), scrapMetrics{Lines: 32 + i, Assertions: 1})
	}
	for i := range 3 {
		write(fmt.Sprintf("branches %d", i+1), scrapMetrics{Lines: 4 + i, Decisions: 1, Assertions: 1})
	}
	for i := range 2 {
		write(fmt.Sprintf("mocks %d", i+1), scrapMetrics{Lines: 6 + i, Mocks: 3, Assertions: 1})
	}
	write("asserts too much", scrapMetrics{Lines: 14, Assertions: 11})
	return source.String()
}

// rankedRecommendations is whether the file's recommendations come in the
// sentence's order of confidence, and the list holds two of them.
func (d *scrapState) rankedRecommendations(first, second, third, fourth string) error {
	if second != third {
		return fmt.Errorf("%s before %s, and %s before %s, is no order", first, second, third, fourth)
	}
	rank := map[string]int{first: 0, second: 1, fourth: 2}
	r, err := d.its()
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	for i, rec := range r.Recommendations {
		if _, ok := rank[rec.Confidence]; !ok {
			return fmt.Errorf("recommendation %d is %s, no confidence the sentence orders", i+1, rec.Confidence)
		}
		seen[rec.Confidence] = true
		if i > 0 && rank[rec.Confidence] < rank[r.Recommendations[i-1].Confidence] {
			return fmt.Errorf("a %s recommendation follows a %s one: %+v", rec.Confidence, r.Recommendations[i-1].Confidence, r.Recommendations)
		}
	}
	if len(seen) < 2 {
		return fmt.Errorf("the recommendations are all %v, too few confidences to order", seen)
	}
	return nil
}

// problems are how many recommendations a report's measures call for: one
// for each smell of an example and each cluster.
func problems(r scrapReport) int {
	n := len(r.Clusters)
	for _, e := range r.Details {
		n += len(e.Smells)
	}
	return n
}

// atMostListed is whether the file lists most recommendations, its cap,
// printed and recorded, its examples calling for more.
func (d *scrapState) atMostListed(most int) error {
	r, err := d.its()
	if err != nil {
		return err
	}
	// The file calls for more, so the cap is what is listed: at most most,
	// and no fewer.
	if n := problems(r); n <= most {
		return fmt.Errorf("%s calls for %d recommendations, so the step cannot fail", r.File, n)
	}
	if len(r.Recommendations) != most {
		return fmt.Errorf("%s calls for %d recommendations and lists %d, not the %d it is capped at", r.File, problems(r), len(r.Recommendations), most)
	}
	files, err := printed(d.w.stdout)
	if err != nil {
		return err
	}
	for _, f := range files {
		if f.fields[1] == r.File && len(f.recommendations) != len(r.Recommendations) {
			return fmt.Errorf("%d recommendations are printed of %s, and %d recorded", len(f.recommendations), r.File, len(r.Recommendations))
		}
	}
	return nil
}

// eachNamesItsExample is whether every recommendation names an example of
// the file, quoted, with its line range.
func (d *scrapState) eachNamesItsExample() error {
	r, err := d.its()
	if err != nil {
		return err
	}
	if len(r.Recommendations) == 0 {
		return fmt.Errorf("%s has no recommendation", r.File)
	}
	for _, rec := range r.Recommendations {
		if !slices.ContainsFunc(r.Details, func(e scrapExample) bool {
			lines := fmt.Sprintf("%d-%d)", e.StartLine, e.EndLine)
			return strings.Contains(rec.Text, strconv.Quote(e.Name)+" (lines "+lines) || strings.Contains(rec.Text, strconv.Quote(e.Name)+" ("+lines)
		}) {
			return fmt.Errorf("%q names no example of %s with its line range", rec.Text, r.File)
		}
	}
	return nil
}
