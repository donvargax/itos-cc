package domain

import (
	"strings"
	"testing"

	"github.com/donvargax/itos-cc/internal/lang"
	"github.com/donvargax/itos-cc/internal/lang/port/porttest"
)

// parse is src parsed in memory as the file at path, nothing around it,
// closed when the test ends.
func parse(t *testing.T, path, src string) *lang.File {
	t.Helper()
	f, err := lang.Parse(t.Context(), porttest.Surroundings{}, lang.Detect(path), path, []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(f.Close)
	return f
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

// forms is the forms of each source, parsed as the file at its path and
// named as names gives it.
func forms(t *testing.T, sources map[string]string, names map[string]string, opt Options) []Form {
	t.Helper()
	var all []Form
	for path, src := range sources {
		all = append(all, Forms(parse(t, path, src), names[path], opt)...)
	}
	return all
}

func TestComparePairsAndFocus(t *testing.T) {
	sources := map[string]string{"/work/invoice.py": invoice, "/work/receipt.py": receipt, "/work/parse.py": unrelated}
	names := map[string]string{"/work/invoice.py": "invoice.py", "/work/receipt.py": "receipt.py", "/work/parse.py": "parse.py"}
	all := forms(t, sources, names, Defaults)
	got := Compare(all, nil, Defaults)
	if len(got) != 1 {
		t.Fatalf("candidates %+v, want the invoice/receipt pair", got)
	}
	c := got[0]
	if c.Left.File != "invoice.py" || c.Right.File != "receipt.py" || c.Language != "python" {
		t.Errorf("pair %s / %s in %s, want invoice.py / receipt.py in python", c.Left.File, c.Right.File, c.Language)
	}
	if c.Score < Defaults.Threshold || c.Score >= 1 {
		t.Errorf("score %v: one extra line should be close but not identical", c.Score)
	}
	if focused := Compare(all, map[string]bool{"/work/parse.py": true}, Defaults); len(focused) != 0 {
		t.Errorf("focus on parse.py reported %+v", focused)
	}
	if focused := Compare(all, map[string]bool{"/work/receipt.py": true}, Defaults); len(focused) != 1 {
		t.Errorf("focus on receipt.py reported %+v, want its pair", focused)
	}
}

// The names the caller gives are what a pair reads in and is ordered by,
// whatever paths the files were parsed as.
func TestTheCallersNamesOrderAPair(t *testing.T) {
	sources := map[string]string{"/work/invoice.py": invoice, "/work/receipt.py": receipt}
	names := map[string]string{"/work/invoice.py": "z/invoice.py", "/work/receipt.py": "a/receipt.py"}
	got := Compare(forms(t, sources, names, Defaults), nil, Defaults)
	if len(got) != 1 || got[0].Left.File != "a/receipt.py" || got[0].Right.File != "z/invoice.py" {
		t.Fatalf("candidates %+v, want a/receipt.py then z/invoice.py", got)
	}
	if s := got[0].Left; s.StartLine != 1 || s.EndLine != 8 || s.Name != "render" || s.Nodes == 0 {
		t.Errorf("left side %+v, want render at lines 1-8 with its nodes", s)
	}
}

func TestSmallFunctionsAreNotCandidates(t *testing.T) {
	short := Forms(parse(t, "/work/a.py", "def a(x):\n    return g(x)\n"), "a.py", Defaults)
	if len(short) != 0 {
		t.Errorf("a two-line function became a candidate: %+v", short)
	}
	few := Forms(parse(t, "/work/b.py", invoice), "b.py", Options{Threshold: 0.82, MinLines: 1, MinNodes: 1000})
	if len(few) != 0 {
		t.Errorf("a function under MinNodes became a candidate: %+v", few)
	}
}

func TestCopiesOfOneFunctionAreOneGroup(t *testing.T) {
	side := func(file string) Side { return Side{File: file, StartLine: 1, EndLine: 9, Name: "f"} }
	pair := func(a, b string, score float64) Candidate {
		return Candidate{Score: score, Language: "go", Left: side(a), Right: side(b)}
	}
	groups := Groups([]Candidate{
		pair("a.go", "b.go", 1), pair("a.go", "c.go", 0.9), pair("a.go", "d.go", 0.95),
		pair("b.go", "c.go", 0.9), pair("b.go", "d.go", 0.95), pair("c.go", "d.go", 0.92),
		pair("x.go", "y.go", 0.85),
	})
	if len(groups) != 2 {
		t.Fatalf("groups %+v, want the four copies and the pair", groups)
	}
	var files []string
	for _, m := range groups[0].Members {
		files = append(files, m.File)
	}
	if strings.Join(files, " ") != "a.go b.go c.go d.go" || groups[0].MinScore != 0.9 || groups[0].MaxScore != 1 {
		t.Errorf("first group %v %v–%v, want a b c d at 0.9–1", files, groups[0].MinScore, groups[0].MaxScore)
	}
	if len(groups[1].Members) != 2 || groups[1].MinScore != 0.85 {
		t.Errorf("second group %+v, want the pair at 0.85", groups[1])
	}
}
