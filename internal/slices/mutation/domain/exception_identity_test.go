package domain_test

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/donvargax/itos-cc/internal/lang"
	selected "github.com/donvargax/itos-cc/internal/selection"
	seltest "github.com/donvargax/itos-cc/internal/selection/port/porttest"
	mutants "github.com/donvargax/itos-cc/internal/slices/mutation/domain"
)

// No public scenario reaches an original or a replacement holding > or :,
// a file holding :, or the malformed identities beside a site.
func TestAnIdentityKeepsItsSiteKeyWhole(t *testing.T) {
	t.Parallel()
	for arg, want := range map[string]mutants.Identity{
		"board/board.go:example.com/fx/board#Count@4:6:b1a8f41702cd975f:35:<><=": {File: "board/board.go", Namespace: "example.com/fx/board", Name: "Count", StartLine: 4, EndLine: 6, Hash: "b1a8f41702cd975f", Key: "35:<><="},
		"a.go:p#F@1:3:ab12:7:>>>=":        {File: "a.go", Namespace: "p", Name: "F", StartLine: 1, EndLine: 3, Hash: "ab12", Key: "7:>>>="},
		"a.ts:m#f@2:2:ff:3:a:b>c":         {File: "a.ts", Namespace: "m", Name: "f", StartLine: 2, EndLine: 2, Hash: "ff", Key: "3:a:b>c"},
		"dir:x/a.go:p.q#T.M@9:12:0:0:1>0": {File: "dir:x/a.go", Namespace: "p.q", Name: "T.M", StartLine: 9, EndLine: 12, Hash: "0", Key: "0:1>0"},
		"a.py:#f@1:2:ab:3:<><=":           {File: "a.py", Namespace: "", Name: "f", StartLine: 1, EndLine: 2, Hash: "ab", Key: "3:<><="},
	} {
		got, ok := mutants.ParseIdentity(arg)
		if !ok || got != want || got.String() != arg || !mutants.IsIdentity(arg) {
			t.Errorf("%q: %+v %v, want %+v; string %q", arg, got, ok, want, got.String())
		}
	}
	for _, arg := range []string{
		"a.go:F@1:2:ab:3:<><=",      // no function's #
		":p#F@1:2:ab:3:<><=",        // no file
		"p#F@1:2:ab:3:<><=",         // no file's :
		"a.go:p#@1:2:ab:3:<><=",     // no name
		"a.go:p#F@1:2:ab:3:",        // no change
		"../a.go:p#F@1:2:ab:3:<><=", // outside the project root
		"/a.go:p#F@1:2:ab:3:<><=",
		"a.go:p#F@99999999999999999999:2:ab:3:<><=", // no line
		"src/board.go:7:19",
	} {
		if _, ok := mutants.ParseIdentity(arg); ok || !mutants.IsIdentity(arg) && arg != "src/board.go:7:19" {
			t.Errorf("%q read as an identity", arg)
		}
	}
	for _, site := range []string{"src/board.go:7:19", `C:\src\board.go:7:19`, "a.go:p#F@1:2:3:<><="} {
		if mutants.IsIdentity(site) {
			t.Errorf("%q is written as an identity", site)
		}
	}
}

// identityOf is the identity a counted run names site of the fixture's
// board.go by.
func identityOf(t *testing.T, opt mutants.CheckOptions, path string, site mutants.Site) mutants.Identity {
	t.Helper()
	parsed, err := selected.Parse(t.Context(), opt.Tree, opt.Surroundings, path)
	if err != nil {
		t.Fatal(err)
	}
	defer parsed.Close()
	u := parsed.Units[site.Unit]
	text := fmt.Sprintf("board.go:%s@%d:%d:%s:%s", mutants.UnitID(u.Namespace, u.Name), u.StartLine, u.EndLine, mutants.UnitHash(parsed, u), site.Key())
	id, ok := mutants.ParseIdentity(text)
	if !ok {
		t.Fatalf("%q is no identity", text)
	}
	return id
}

// The scenarios' fixture has one function and one site: these hold the
// unit chosen among several and the refusals the scenarios name by rule
// alone, against the fakes, a fresh snapshot recording every site.
func TestDomainIdentifiedSurvivorIsTheSiteOfTheSourceAsItIsNow(t *testing.T) {
	f, site := survivingFixture(t)
	opt := exceptionOptions(f)
	id := identityOf(t, opt, f.path, site)
	want, err := mutants.Survivor(t.Context(), f.path, site.Line, site.Column, opt, f.edits)
	if err != nil {
		t.Fatal(err)
	}
	got, line, err := mutants.IdentifiedSurvivor(t.Context(), id, opt, f.edits)
	if err != nil || got != want || line != site.Line {
		t.Fatalf("fresh survivor: %+v %d %v, want %+v at %d", got, line, err, want, site.Line)
	}
	with := func(source string) mutants.CheckOptions {
		o := opt
		o.Tree = seltest.TreeOf(f.root, map[string]string{"board.go": source, "go.mod": "module example.test/board\n", "board_test.go": "package board\n"}, nil)
		return o
	}
	moved := strings.Replace(runSource, "func", "\n\nfunc", 1)
	if got, line, err = mutants.IdentifiedSurvivor(t.Context(), id, with(moved), f.edits); err != nil || got != want || line != site.Line+2 {
		t.Fatalf("moved: %+v %d %v", got, line, err)
	}
	// Two of the name and hash, on lines 2 and 6: the one at the
	// identity's lines, else the nearest by its first line, the first of two
	// as near.
	twice := "package board\nfunc Value(x int) bool { return x > 1 }\n\n\n\nfunc Value(x int) bool { return x > 1 }\n"
	for lines, want := range map[[2]int]int{{2, 2}: 2, {6, 6}: 6, {5, 5}: 6, {1, 1}: 2, {4, 4}: 2, {5, 2}: 6, {2, 6}: 2} {
		at := id
		at.StartLine, at.EndLine = lines[0], lines[1]
		if _, line, err = mutants.IdentifiedSurvivor(t.Context(), at, with(twice), f.edits); err != nil || line != want {
			t.Errorf("lines %v: %d %v, want %d", lines, line, err, want)
		}
	}
	var stale *mutants.IdentityStaleError
	changed := strings.Replace(runSource, "x > 1", "x > 2", 1)
	if _, _, err = mutants.IdentifiedSurvivor(t.Context(), id, with(changed), f.edits); !errors.As(err, &stale) || stale.Why != mutants.ExceptionChanged || stale.Hash == "" || stale.Hash == id.Hash {
		t.Fatalf("changed: %v", err)
	}
	for name, gone := range map[string]mutants.Identity{"key": withKey(id, "99:>>="), "name": withName(id, "Other"), "namespace": withNamespace(id, "other"), "file": withFile(id, "gone.go"), "language": withFile(id, "board.txt")} {
		if _, _, err = mutants.IdentifiedSurvivor(t.Context(), gone, opt, f.edits); !errors.As(err, &stale) || stale.Why != mutants.ExceptionGone {
			t.Errorf("gone %s: %v", name, err)
		}
	}
	snap := f.snapshots.Values["board.go"]
	for outcome, uncovered := range map[string]bool{mutants.Killed: false, mutants.Timeout: false, mutants.Uncovered: true} {
		// The identity's mutant alone is recorded so, the others survived.
		for i, m := range snap.Units[0].Mutants {
			snap.Units[0].Mutants[i].Outcome = mutants.Survived
			if m.Key() == site.Key() {
				snap.Units[0].Mutants[i].Outcome = outcome
			}
		}
		var none *mutants.NoSurvivorError
		_, _, err = mutants.IdentifiedSurvivor(t.Context(), id, opt, f.edits)
		if !errors.As(err, &none) || none.Uncovered != uncovered || none.Line != site.Line || none.Column != site.Column || !strings.Contains(none.Reason, "recorded "+outcome) {
			t.Errorf("recorded %s: %+v", outcome, err)
		}
	}
	// Another mutant recorded killed is no refusal of this one.
	for i, m := range snap.Units[0].Mutants {
		snap.Units[0].Mutants[i].Outcome = mutants.Killed
		if m.Key() == site.Key() {
			snap.Units[0].Mutants[i].Outcome = mutants.Survived
		}
	}
	if len(snap.Units[0].Mutants) < 2 {
		t.Fatalf("the fixture's function has one mutant: %+v", snap.Units[0].Mutants)
	}
	if _, _, err = mutants.IdentifiedSurvivor(t.Context(), id, opt, f.edits); err != nil {
		t.Fatalf("another mutant killed: %v", err)
	}
	// A stale snapshot, its tests changed, is no refusal, whatever it
	// records.
	for i := range snap.Units[0].Mutants {
		snap.Units[0].Mutants[i].Outcome = mutants.Killed
	}
	snap.Tests = map[string]string{"board_test.go": "another"}
	f.snapshots.Values["board.go"] = snap
	if _, _, err = mutants.IdentifiedSurvivor(t.Context(), id, opt, f.edits); err != nil {
		t.Fatalf("stale snapshot: %v", err)
	}
}

// Python's and TypeScript's namespace, in a counted run's identity, is the
// module the committed tree names (stableFunction), not the unit's own: the
// name alone is compared there.
func TestDomainIdentifiedSurvivorComparesTheNameAloneWhereTheNamespaceIsTheTrees(t *testing.T) {
	f := newRunFixture(t)
	opt := exceptionOptions(f)
	source := "def value(x):\n    return x > 1\n"
	path := filepath.Join(f.root, "mod.py")
	opt.Tree = seltest.TreeOf(f.root, map[string]string{"mod.py": source}, nil)
	f.edits.Files[path] = source
	parsed, err := selected.Parse(t.Context(), opt.Tree, opt.Surroundings, path)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Spec != lang.Detect(path) || len(parsed.Units) != 1 {
		t.Fatalf("python fixture: %+v", parsed.Units)
	}
	u := parsed.Units[0]
	site := mutants.Sites(parsed)[0]
	text := fmt.Sprintf("mod.py:src.elsewhere#%s@%d:%d:%s:%s", u.Name, u.StartLine, u.EndLine, mutants.UnitHash(parsed, u), site.Key())
	parsed.Close()
	id, _ := mutants.ParseIdentity(text)
	got, line, err := mutants.IdentifiedSurvivor(t.Context(), id, opt, f.edits)
	if err != nil || line != site.Line || got.Function != mutants.UnitID(u.Namespace, u.Name) {
		t.Fatalf("python identity: %+v %d %v", got, line, err)
	}
}

func withKey(id mutants.Identity, key string) mutants.Identity { id.Key = key; return id }
func withName(id mutants.Identity, name string) mutants.Identity {
	id.Name = name
	return id
}
func withNamespace(id mutants.Identity, namespace string) mutants.Identity {
	id.Namespace = namespace
	return id
}
func withFile(id mutants.Identity, file string) mutants.Identity { id.File = file; return id }
