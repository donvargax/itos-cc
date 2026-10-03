package dry

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/donvargax/itos-cc/lang"
)

// score fingerprints the only unit in each source and returns their
// similarity.
func score(t *testing.T, ext, a, b string) float64 {
	t.Helper()
	return Jaccard(prints(t, ext, a), prints(t, ext, b))
}

func prints(t *testing.T, ext, src string) []uint64 {
	t.Helper()
	path := "x" + ext
	f, err := lang.Parse(lang.Detect(path), path, []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if len(f.Units) != 1 {
		t.Fatalf("want one unit in %q, got %d", src, len(f.Units))
	}
	p, _ := Fingerprint(f, f.Units[0].Node)
	return p
}

// Names, locals, predicates, and literals differ; the shape and the called
// functions do not.
func TestRenamedCodeScoresOne(t *testing.T) {
	cases := []struct{ ext, a, b string }{
		{".ts",
			"function alpha(xs) { const ys = filter(xs, odd); return map(ys, 1); }",
			"function beta(items) { const kept = filter(items, even); return map(kept, 2); }"},
		{".py",
			"def alpha(xs):\n    ys = filter(xs, odd)\n    return map(ys, 'a')\n",
			"def beta(items):\n    kept = filter(items, even)\n    return map(kept, 'b')\n"},
		{".go",
			"package p\nfunc alpha(xs []int) []int { ys := filter(xs, odd); return mapf(ys, 1) }\n",
			"package p\nfunc beta(items []int) []int { kept := filter(items, even); return mapf(kept, 2) }\n"},
		{".kt",
			"fun alpha(xs: List<Int>): List<Int> { val ys = filter(xs, odd); return map(ys, 1) }\n",
			"fun beta(items: List<Int>): List<Int> { val kept = filter(items, even); return map(kept, 2) }\n"},
	}
	for _, c := range cases {
		if s := score(t, c.ext, c.a, c.b); s != 1 {
			t.Errorf("%s: score %v, want 1", c.ext, s)
		}
	}
}

func TestFieldAccessIsTheSameShape(t *testing.T) {
	a := "function f(order) { return order.total * 2; }"
	b := "function g(row) { return row.amount * 3; }"
	if s := score(t, ".ts", a, b); s != 1 {
		t.Errorf("order.total vs row.amount: %v, want 1", s)
	}
}

func TestCalledFunctionsAndOperatorsMatter(t *testing.T) {
	base := "function f(xs) { const ys = filter(xs, odd); return map(ys, inc); }"
	for name, other := range map[string]string{
		"different callee":   "function f(xs) { const ys = reject(xs, odd); return map(ys, inc); }",
		"different operator": "function f(xs) { const ys = filter(xs, odd); return map(ys, inc) - 1; }",
	} {
		if s := score(t, ".ts", base, other); s >= 1 || s <= 0 {
			t.Errorf("%s: score %v, want strictly between 0 and 1", name, s)
		}
	}
}

func TestCommentsDoNotCount(t *testing.T) {
	a := "def f(x):\n    return g(x)\n"
	b := "def f(x):\n    # explain\n    return g(x)  # why\n"
	if s := score(t, ".py", a, b); s != 1 {
		t.Errorf("score %v, want 1", s)
	}
}

func TestUnrelatedCodeScoresLow(t *testing.T) {
	a := "function f(xs) { for (const x of xs) { if (x > 0) { total += x; } } return total; }"
	b := "function g(s) { return fetch(url(s)).then(parse).catch(report); }"
	if s := score(t, ".ts", a, b); s > 0.3 {
		t.Errorf("score %v, want under 0.3", s)
	}
}

func write(t *testing.T, dir, name, src string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

const invoice = `def render(invoice):
    lines = []
    for item in invoice.items:
        lines.append(format_line(item.name, item.price))
    total = sum(item.price for item in invoice.items)
    lines.append(format_total(total))
    return "\n".join(lines)
`

const receipt = `def render(receipt):
    out = []
    for entry in receipt.entries:
        out.append(format_line(entry.label, entry.amount))
    paid = sum(entry.amount for entry in receipt.entries)
    out.append(format_total(paid))
    out.append(format_footer(receipt.store))
    return "\n".join(out)
`

const unrelated = `def parse(text):
    if not text:
        raise ValueError("empty")
    head, _, rest = text.partition(":")
    return {"head": head.strip(), "rest": rest.strip()}
`

func TestComparePairsAndFocus(t *testing.T) {
	dir := t.TempDir()
	inv := write(t, dir, "invoice.py", invoice)
	rec := write(t, dir, "receipt.py", receipt)
	other := write(t, dir, "parse.py", unrelated)
	forms, err := Forms([]string{inv, rec, other}, Defaults)
	if err != nil {
		t.Fatal(err)
	}
	got := Compare(forms, nil, Defaults)
	if len(got) != 1 {
		t.Fatalf("candidates %+v, want the invoice/receipt pair", got)
	}
	c := got[0]
	if filepath.Base(c.Left.File) != "invoice.py" || filepath.Base(c.Right.File) != "receipt.py" {
		t.Errorf("pair %s / %s, want invoice.py / receipt.py", c.Left.File, c.Right.File)
	}
	if c.Score < Defaults.Threshold || c.Score >= 1 {
		t.Errorf("score %v: one extra line should be close but not identical", c.Score)
	}
	if focused := Compare(forms, map[string]bool{other: true}, Defaults); len(focused) != 0 {
		t.Errorf("focus on parse.py reported %+v", focused)
	}
	if focused := Compare(forms, map[string]bool{rec: true}, Defaults); len(focused) != 1 {
		t.Errorf("focus on receipt.py reported %+v, want its pair", focused)
	}
}

func TestSmallFunctionsAreNotCandidates(t *testing.T) {
	dir := t.TempDir()
	a := write(t, dir, "a.py", "def a(x):\n    return g(x)\n")
	forms, err := Forms([]string{a}, Defaults)
	if err != nil {
		t.Fatal(err)
	}
	if len(forms) != 0 {
		t.Errorf("a two-line function became a candidate: %+v", forms)
	}
}
