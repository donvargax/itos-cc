package porttest

import (
	"reflect"
	"testing"

	mutants "github.com/donvargax/itos-cc/internal/slices/mutation/domain"
)

// FreshNamedContract holds the admission of named sites (mutation run
// --site, slice-63) shared by the physical adapter and memory
// implementation: the named mutants alone, a mutant named twice once,
// their functions alone, nothing ranked; and every argument that names no
// mutant refused, in order, with why.
func FreshNamedContract(t *testing.T, open func(*testing.T) FreshFixture) {
	t.Helper()
	source := "package value\n\nfunc Value(x int) bool {\n\treturn x < 5\n}\n\nfunc Other(x int) bool {\n\treturn x > 9\n}\n"
	t.Run("named sites are judged alone, each mutant once", func(t *testing.T) {
		b := open(t)
		historyCommit(t, b.HistoryFixture, map[string]string{"go.mod": "module example/named\n", "value.go": source, "value_test.go": "package value\n"})
		at := mutants.FreshSite{Argument: "value.go:4:11", File: "value.go", Line: 4, Column: 11}
		p, failed := b.Fresh.Plan(t.Context(), mutants.FreshRequest{Root: b.Root, Sites: []mutants.FreshSite{at}})
		if failed != nil {
			t.Fatal(failed)
		}
		defer b.Fresh.Release(t.Context(), p)
		if len(p.Selected) != 1 || p.Selected[0].Site.Original != "<" || len(p.Units) != 1 || p.Units[0].Function != "example/named#Value" {
			t.Fatalf("plan %+v, want Value's < alone", p)
		}
		id, ok := mutants.ParseIdentity(p.Selected[0].Identity)
		if !ok {
			t.Fatalf("identity %q", p.Selected[0].Identity)
		}
		again, failed := b.Fresh.Plan(t.Context(), mutants.FreshRequest{Root: b.Root, Sites: []mutants.FreshSite{at, {Argument: p.Selected[0].Identity, File: id.File, Identity: &id}}})
		if failed != nil {
			t.Fatal(failed)
		}
		defer b.Fresh.Release(t.Context(), again)
		want := []mutants.FreshNamed{{Argument: at.Argument, Identities: []string{id.String()}}, {Argument: id.String(), Identities: []string{id.String()}}}
		if len(again.Selected) != 1 || len(again.Eligible) != 1 || len(again.Omitted) != 0 || again.Algorithm != mutants.FreshNamedAlgorithm || again.Seed != "" ||
			!reflect.DeepEqual(again.Named, want) {
			t.Fatalf("plan %+v, want the one mutant named twice, %+v", again, want)
		}
	})
	t.Run("every argument that names no mutant is refused, with why", func(t *testing.T) {
		b := open(t)
		historyCommit(t, b.HistoryFixture, map[string]string{"go.mod": "module example/named\n", "value.go": source, "value_test.go": "package value\n"})
		p, failed := b.Fresh.Plan(t.Context(), mutants.FreshRequest{Root: b.Root, Sites: []mutants.FreshSite{{Argument: "value.go:4:11", File: "value.go", Line: 4, Column: 11}}})
		if failed != nil {
			t.Fatal(failed)
		}
		id, _ := mutants.ParseIdentity(p.Selected[0].Identity)
		b.Fresh.Release(t.Context(), p)
		changed, missing, siteless := id, id, id
		changed.Hash, missing.Name, siteless.Key = "0123456789abcdef", "Missing", "99:<><="
		sites := []mutants.FreshSite{
			{Argument: "value.go:4:12", File: "value.go", Line: 4, Column: 12},
			{Argument: "value_test.go:1:1", File: "value_test.go", Line: 1, Column: 1},
			{Argument: "gone.go:1:1", File: "gone.go", Line: 1, Column: 1},
			{Argument: "changed", File: id.File, Identity: &changed},
			{Argument: "missing", File: id.File, Identity: &missing},
			{Argument: "siteless", File: id.File, Identity: &siteless},
			{Argument: "test", File: "value_test.go", Identity: &id},
		}
		_, failed = b.Fresh.Plan(t.Context(), mutants.FreshRequest{Root: b.Root, Sites: sites})
		unmatched, ok := failed.(mutants.FreshUnmatched)
		if !ok {
			t.Fatalf("failure %T %v, want FreshUnmatched", failed, failed)
		}
		want := []mutants.UnmatchedSite{{Argument: "value.go:4:12", Why: mutants.UnmatchedNoSite}, {Argument: "value_test.go:1:1", Why: mutants.UnmatchedNoSite},
			{Argument: "gone.go:1:1", Why: mutants.UnmatchedGone}, {Argument: "changed", Why: mutants.UnmatchedChanged, Hash: id.Hash},
			{Argument: "missing", Why: mutants.UnmatchedGone}, {Argument: "siteless", Why: mutants.UnmatchedGone}, {Argument: "test", Why: mutants.UnmatchedGone}}
		if !reflect.DeepEqual(unmatched.Sites, want) {
			t.Errorf("unmatched %+v, want %+v", unmatched.Sites, want)
		}
		if unmatched.Error() == "" {
			t.Error("an unmatched failure says nothing")
		}
	})
}
