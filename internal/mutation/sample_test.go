package mutation

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/donvargax/itos-cc/internal/mutants"
	mutantstest "github.com/donvargax/itos-cc/internal/mutants/porttest"
)

// No public scenario reaches these: a missing or wrong History binding, or a
// context cancelled between the selection and the seed, cannot be arranged
// through the binary or the memory driver, whose bindings are always actual.
// The seed's History is the bound one, never the disk's.
func TestSampleSeedsFromTheBoundHistoryOnlyWhenOmitted(t *testing.T) {
	repository := mutantstest.NewHistoryRepository()
	repository.Index = map[string]string{"main.go": "package board\n"}
	head := repository.Commit("independent base")
	h := Handler{History: mutantstest.History{Repository: repository}}
	if seed, err := h.seed(t.Context(), nil); err != nil || seed != head {
		t.Fatalf("default seed not the bound HEAD: %q %v", seed, err)
	}
	other := mutantstest.NewHistoryRepository()
	other.Commit("different independent base")
	if seed, err := (Handler{History: mutantstest.History{Repository: other}}).seed(t.Context(), nil); err != nil || seed == head {
		t.Fatalf("wrong HEAD binding retained the real seed: %q %v", seed, err)
	}
	unbound := Handler{}
	if _, err := unbound.seed(t.Context(), nil); err == nil || !strings.Contains(err.Error(), "history binding is missing") {
		t.Fatalf("default seed without a History binding: %v", err)
	}
	for _, explicit := range []string{"explicit independent seed", ""} {
		if seed, err := unbound.seed(t.Context(), &explicit); err != nil || seed != explicit {
			t.Fatalf("explicit seed %q demanded the unused HEAD binding: %q %v", explicit, seed, err)
		}
	}
	cause := errors.New("actual history consumer cancellation")
	ctx, cancel := context.WithCancelCause(t.Context())
	cancel(cause)
	_, err := h.seed(ctx, nil)
	var unavailable SeedUnavailable
	if !errors.As(err, &unavailable) || !errors.Is(sampleError(err, func(path string) string { return path }), cause) {
		t.Fatalf("seed lost the caller's cause: %v", err)
	}
	if _, ok := unavailable.Failure.(mutants.HistoryCancelled); !ok {
		t.Fatalf("cancellation not History's: %#v", unavailable.Failure)
	}
}

func TestAMismatchsFixRerunsInItsScope(t *testing.T) {
	t.Parallel()
	for scope, want := range map[string]string{
		"own":          "itos-cc mutation run --mutate-all src/b.ts",
		"listed":       "itos-cc mutation run --mutate-all src/b.ts",
		"all-tests":    "itos-cc mutation run --mutate-all --all-tests src/b.ts",
		"make test":    "itos-cc mutation run --mutate-all --test-command 'make test' src/b.ts",
		"echo 'a b' x": `itos-cc mutation run --mutate-all --test-command 'echo '\''a b'\'' x' src/b.ts`,
	} {
		if got := rerunIn(scope, "src/b.ts"); got != want {
			t.Errorf("scope %q: %q, want %q", scope, got, want)
		}
	}
}
