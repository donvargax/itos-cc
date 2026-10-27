package domain

import "testing"

// A site of an inline unit, a TypeScript route, is the route's: the
// function around it never takes it at its place, as Locate does not.
func TestRehashableTakesOnlyTheFunctionsOwnSite(t *testing.T) {
	f := parse(t, "x.ts", "export function mount(app) {\n  // routes\n  app.get(\"/x\", (req, res) => res.json(req.q > 0));\n}\n")
	sites := Sites(f)
	at := -1
	for i, s := range sites {
		if s.Original == ">" {
			at = i
		}
	}
	if at < 0 || sites[at].Unit != 1 {
		t.Fatalf("no site of the route: %+v", sites)
	}
	s := sites[at]
	for i, want := range []bool{false, true} {
		u := f.Units[i]
		e := Exception{Function: UnitID(u.Namespace, u.Name), Hash: PreviousUnitHash(f, u), LineInFunction: s.Line - u.StartLine + 1,
			Column: s.Column, Original: s.Original, Replacement: s.Replacement}
		if _, _, ok := Rehashable(e, f, sites); ok != want {
			t.Errorf("%s: rehashable %v, want %v", u.Name, ok, want)
		}
	}
}
