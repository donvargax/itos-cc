package lang

import "testing"

// These tests hold how a unit's structure is compared (Fingerprint,
// Jaccard): what normalization drops and what it keeps.

// score fingerprints the only unit in each source, parsed as a file with
// extension ext, and returns their similarity.
func score(t *testing.T, ext, a, b string) float64 {
	t.Helper()
	return Jaccard(prints(t, ext, a), prints(t, ext, b))
}

// prints is the fingerprints of the only unit in src, parsed as a file
// with extension ext.
func prints(t *testing.T, ext, src string) []uint64 {
	t.Helper()
	f := parse(t, "/work/x"+ext, src)
	if len(f.Units) != 1 {
		t.Fatalf("want one unit in %q, got %d", src, len(f.Units))
	}
	p, _ := Fingerprint(f, f.Units[0].Node)
	return p
}

// Routes are units of their own, so two functions that mount different
// routes the same way are duplicates, whatever the routes do.
func TestRoutesAreNotPartOfTheFunctionAroundThem(t *testing.T) {
	a := `export function mount(app, db) {
  const prefix = config.prefix;
  app.get("/a", (req, res) => { if (req.q) { res.json(db.all()); } else { res.sendStatus(404); } });
  app.post("/a", (req, res) => res.json(db.add(req.body)));
  return app;
}`
	b := `export function mount(app, db) {
  const prefix = config.prefix;
  app.get("/b", (req, res) => res.send(1));
  app.post("/b", (req, res) => { for (const x of req.body) { db.put(x); } res.sendStatus(201); });
  return app;
}`
	got := [2][]uint64{}
	for i, src := range []string{a, b} {
		f := parse(t, "/work/x.ts", src)
		got[i], _ = Fingerprint(f, f.Units[0].Node, f.InnerNodes(f.Units[0])...)
	}
	if s := Jaccard(got[0], got[1]); s != 1 {
		t.Errorf("score = %v, want 1", s)
	}
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
