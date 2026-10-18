package features

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// What scrap prints for each file: the order of the files, their
// recommendations and the hint.

// scrapSample is the project the scenarios that only say When I run run in:
// a clean Python file, a coverage matrix in TypeScript, a Python example
// that asserts nothing, and Go subtests with mocks and logic.
var scrapSample = map[string]string{
	"test_ok.py":    cleanPy,
	"parse.test.ts": "import { it, expect } from \"vitest\";\n\n" + dataOnly["TypeScript"][1],
	"test_x.py":     "def test_runs():\n    run()\n",
	"cart_test.go": `package cart

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
`,
}

const cleanPy = "def test_adds():\n    assert add(1, 2) == 3\n\ndef test_parses():\n    assert parse('x') == {'x': None}\n"

// actions are the words a file's action is printed as.
const actions = "LEAVE_ALONE|AUTO_TABLE_DRIVE|AUTO_REFACTOR|MANUAL_SPLIT|REVIEW_FIRST"

// eachFilePrintedAs is whether the run prints one line of each file it
// reports, in the form the sentence gives with file the path, ACTION the
// action and … a number, and nothing else on the line.
func (d *scrapState) eachFilePrintedAs(form string) error {
	reports, err := d.reported()
	if err != nil {
		return err
	}
	pattern := regexp.QuoteMeta(form)
	pattern = strings.Replace(pattern, "file", `(\S+)`, 1)
	pattern = strings.Replace(pattern, "ACTION", "(?:"+actions+")", 1)
	pattern = strings.ReplaceAll(pattern, "…", `\d+(?:\.\d+)?`)
	line := regexp.MustCompile("^" + pattern + "$")
	seen := map[string]int{}
	for _, text := range outputLines(filepath.ToSlash(d.w.stdout)) {
		if m := line.FindStringSubmatch(strings.TrimSuffix(text, "\r")); m != nil {
			seen[m[1]]++
		}
	}
	if len(reports) < 2 {
		return fmt.Errorf("the run reports %d files, too few to show the form", len(reports))
	}
	for file := range reports {
		if seen[file] != 1 {
			return fmt.Errorf("%s is printed %d times as %q\n%s", file, seen[file], form, d.w.report())
		}
	}
	return nil
}

// highestPressureFirst is whether the files' lines come from the highest
// pressure down, and the run printed two pressures.
func (d *scrapState) highestPressureFirst() error {
	if _, err := d.reported(); err != nil {
		return err
	}
	files, err := printed(d.w.stdout)
	if err != nil {
		return err
	}
	var pressures []float64
	for _, f := range files {
		p, _ := strconv.ParseFloat(f.fields[3], 64)
		pressures = append(pressures, p)
	}
	if len(slices.Compact(slices.Clone(pressures))) < 2 {
		return fmt.Errorf("the files' pressures are %v, too few to order", pressures)
	}
	if !slices.IsSortedFunc(pressures, func(a, b float64) int { return cmp.Compare(b, a) }) {
		return fmt.Errorf("the files are printed with the pressures %v", pressures)
	}
	return nil
}

// recommendationsFollow is whether each file the run does not leave alone
// is followed by its recommendations, indented, each with its confidence,
// one of them printed at least.
func (d *scrapState) recommendationsFollow() error {
	reports, err := d.reported()
	if err != nil {
		return err
	}
	files, err := printed(d.w.stdout)
	if err != nil {
		return err
	}
	shown := 0
	for _, f := range files {
		r := reports[f.fields[1]]
		want := r.Recommendations
		if r.Action == "LEAVE_ALONE" {
			want = nil
		}
		if !slices.Equal(f.recommendations, want) {
			return fmt.Errorf("under %s are printed %+v, not %+v", r.File, f.recommendations, want)
		}
		shown += len(want)
	}
	if shown == 0 {
		return errors.New("the run prints no recommendation, so the step cannot fail")
	}
	return nil
}

// quietFile writes two examples sharing most of their structure, which
// scrap leaves alone with a LOW hint, and checks it does.
func (d *scrapState) quietFile(action, confidence string) error {
	if err := d.writeFile("quiet.test.ts", quietTs); err != nil {
		return err
	}
	reports, err := d.scrapIn(d.w.dir, d.file)
	if err != nil {
		return err
	}
	r := reports[d.file]
	if r.Action != action || !slices.ContainsFunc(r.Recommendations, func(rec scrapRecommendation) bool { return rec.Confidence == confidence }) {
		return fmt.Errorf("scrap gives %s %s and %+v, not %s with a %s hint", d.file, r.Action, r.Recommendations, action, confidence)
	}
	return nil
}

// quietTs is two examples, each asserting once, that share most of their
// structure: 0.8 alike, below what scrap asks a helper for.
const quietTs = `it("charges a card", () => {
  const cart = new Cart();
  cart.add({ sku: "a", price: 10 });
  cart.add({ sku: "b", price: 20 });
  const customer = new Customer("ada");
  const payment = new Payment(customer, "card");
  payment.authorize(cart.total());
  const order = checkout(cart, customer, payment);
  order.confirm();
  const receipt = order.receipt();
  expect(receipt.total).toBe(30);
});

it("charges a voucher", () => {
  const cart = new Cart();
  cart.add({ sku: "a", price: 10 });
  cart.add({ sku: "b", price: 20 });
  const customer = new Customer("ada");
  const payment = new Payment(customer, "card");
  payment.authorize(cart.total());
  const order = checkout(cart, customer, payment);
  order.confirm();
  extra1(order, receipt.lines);
  extra2(order, receipt.lines);
  expect(receipt.total).toBe(30);
});
`

// hint is the recommendation of the file its action leaves alone.
func (d *scrapState) hint() (string, error) {
	r, err := d.its()
	if err != nil {
		return "", err
	}
	for _, rec := range r.Recommendations {
		return rec.Text, nil
	}
	return "", fmt.Errorf("%s has no hint", r.File)
}

// printedWith is whether the run, with the flag added, prints the hint
// under the file, with its confidence.
func (d *scrapState) printedWith(flag string) error {
	hint, err := d.hint()
	if err != nil {
		return err
	}
	r, _ := d.its()
	code, stdout, stderr, err := d.w.capture(d.w.last.dir, d.w.env(), append(slices.Clone(d.w.last.args), flag)...)
	if err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("with %s itos-cc exited %d\n%s%s", flag, code, stdout, stderr)
	}
	files, err := printed(stdout)
	if err != nil {
		return err
	}
	for _, f := range files {
		if f.fields[1] == r.File && slices.Contains(f.recommendations, scrapRecommendation{r.Recommendations[0].Confidence, hint}) {
			return nil
		}
	}
	return fmt.Errorf("with %s the hint %q is not printed under %s\n%s", flag, hint, r.File, stdout)
}

// everyExamplePrinted is whether, under each file, every example is
// printed with the measures the list names, as the run recorded them, and
// one has a smell.
func (d *scrapState) everyExamplePrinted(list string) error {
	reports, err := d.reported()
	if err != nil {
		return err
	}
	files, err := printed(d.w.stdout)
	if err != nil {
		return err
	}
	columns := map[string]func(m []string, e scrapExample) bool{
		"score":      func(m []string, e scrapExample) bool { return strings.TrimSpace(m[1]) == fmt.Sprintf("%.1f", e.Score) },
		"lines":      func(m []string, e scrapExample) bool { return m[6] == strconv.Itoa(e.Lines) },
		"assertions": func(m []string, e scrapExample) bool { return m[7] == strconv.Itoa(e.Assertions) },
		"decisions":  func(m []string, e scrapExample) bool { return m[8] == strconv.Itoa(e.Decisions) },
		"mocks":      func(m []string, e scrapExample) bool { return m[9] == strconv.Itoa(e.Mocks) },
		"smells":     func(m []string, e scrapExample) bool { return m[10] == strings.Join(e.Smells, " ") },
	}
	keys := listItems(list)
	for _, key := range keys {
		if columns[key] == nil {
			return fmt.Errorf("no example is printed with its %s", key)
		}
	}
	smelly := false
	for _, f := range files {
		r := reports[f.fields[1]]
		if len(f.examples) != len(r.Details) {
			return fmt.Errorf("%d examples of %s are printed, of %d", len(f.examples), r.File, len(r.Details))
		}
		for i, e := range r.Details {
			m := f.examples[i]
			if m[2] != r.File || m[3] != strconv.Itoa(e.StartLine) || m[4] != strconv.Itoa(e.EndLine) || m[5] != strconv.Quote(e.Name) {
				return fmt.Errorf("example %d of %s is printed as %q, not %q at %d-%d", i+1, r.File, m[0], e.Name, e.StartLine, e.EndLine)
			}
			for _, key := range keys {
				if !columns[key](m, e) {
					return fmt.Errorf("%q is printed with another %s than %+v", m[0], key, e)
				}
			}
			smelly = smelly || len(e.Smells) > 0
		}
	}
	if !smelly {
		return errors.New("no example has a smell, so the step cannot fail")
	}
	return nil
}

// eachReportHas is whether every file of the object has each key the list
// names.
func (d *scrapState) eachReportHas(list string) error {
	var object struct {
		Files []map[string]any `json:"files"`
	}
	if err := json.Unmarshal([]byte(d.w.stdout), &object); err != nil || len(object.Files) == 0 {
		return fmt.Errorf("the object lists no file: %v\n%s", err, d.w.report())
	}
	for _, f := range object.Files {
		for _, key := range listItems(list) {
			if _, ok := f[key]; !ok {
				return fmt.Errorf("the report of %v has no %q", f["file"], key)
			}
		}
	}
	return nil
}
