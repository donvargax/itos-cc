package porttest

import (
	"context"
	"errors"
	"reflect"
	"testing"

	mutants "github.com/donvargax/itos-cc/internal/slices/mutation/domain"
)

// ExceptionFixture exposes independently owned current entries for shared
// outcome checks. The concrete opener owns all document representation.
type ExceptionFixture struct {
	Store   mutants.Exceptions
	Entries func() []mutants.Exception
	Replace func([]mutants.Exception)
}

// ExceptionsContract covers current-state replacement and atomic complete-Old
// batch matching. YAML comments/permissions and malformed bytes stay concrete.
func ExceptionsContract(t *testing.T, open func(*testing.T, []mutants.Exception, bool) ExceptionFixture) {
	a := mutants.Exception{File: "a.go", Function: "p#F", Hash: "old", LineInFunction: 1, Column: 2, Original: ">", Replacement: ">=", Reason: "old reason"}
	b := a
	b.File = "b.go"
	t.Run("current append and SameSite replacement", func(t *testing.T) {
		f := open(t, []mutants.Exception{a}, false)
		// A concurrent unrelated entry after initial selection must survive.
		f.Replace([]mutants.Exception{a, b})
		updated := a
		updated.Reason = "new reason"
		updated.LineText = "actual line"
		replaced, failure := f.Store.Except(t.Context(), updated)
		if failure != nil || !replaced || !reflect.DeepEqual(f.Entries(), []mutants.Exception{updated, b}) {
			t.Fatalf("current replacement: %v %v %+v", replaced, failure, f.Entries())
		}
		c := a
		c.Column++
		replaced, failure = f.Store.Except(t.Context(), c)
		if failure != nil || replaced || !reflect.DeepEqual(f.Entries(), []mutants.Exception{updated, b, c}) {
			t.Fatalf("append: %v %v %+v", replaced, failure, f.Entries())
		}
	})
	t.Run("complete Old duplicate batch", func(t *testing.T) {
		f := open(t, []mutants.Exception{a, a, b}, false)
		next := a
		next.Hash = "new"
		other := next
		other.Column++
		failure := f.Store.Renew(t.Context(), []mutants.Renewal{{Old: a, New: next}, {Old: a, New: other}})
		if failure != nil || !reflect.DeepEqual(f.Entries(), []mutants.Exception{next, other, b}) {
			t.Fatalf("duplicates: %v %+v", failure, f.Entries())
		}
		before := f.Entries()
		missing := a
		missing.Reason = "not the complete old value"
		failure = f.Store.Renew(t.Context(), []mutants.Renewal{{Old: next, New: a}, {Old: missing, New: b}})
		var failed mutants.ExceptionFailed
		if !errors.As(failure, &failed) || !reflect.DeepEqual(f.Entries(), before) {
			t.Fatalf("partial batch: %v %+v", failure, f.Entries())
		}
	})
	t.Run("missing creation versus nonempty renewal", func(t *testing.T) {
		f := open(t, nil, true)
		if failure := f.Store.Renew(t.Context(), nil); failure != nil {
			t.Fatal(failure)
		}
		if failure := f.Store.Renew(t.Context(), []mutants.Renewal{{Old: a, New: b}}); failure == nil {
			t.Fatal("missing renewal succeeded")
		}
		replaced, failure := f.Store.Except(t.Context(), a)
		if failure != nil || replaced || !reflect.DeepEqual(f.Entries(), []mutants.Exception{a}) {
			t.Fatalf("creation: %v %v %+v", replaced, failure, f.Entries())
		}
	})
	t.Run("cancelled caller preserves cause and entries", func(t *testing.T) {
		f := open(t, []mutants.Exception{a}, false)
		ctx, cancel := context.WithCancelCause(t.Context())
		cause := errors.New("caller stopped")
		cancel(cause)
		_, failure := f.Store.Except(ctx, b)
		var stopped mutants.ExceptionCancelled
		if !errors.As(failure, &stopped) || !errors.Is(failure, cause) {
			t.Fatalf("except cancellation: %v", failure)
		}
		failure = f.Store.Renew(ctx, []mutants.Renewal{{Old: a, New: b}})
		if !errors.As(failure, &stopped) || !errors.Is(failure, cause) || !reflect.DeepEqual(f.Entries(), []mutants.Exception{a}) {
			t.Fatalf("renew cancellation: %v %+v", failure, f.Entries())
		}
	})
}

// SourceStatusContract holds following-Stat kind independently of Lstat policy.
func SourceStatusContract(t *testing.T, open func(*testing.T) (mutants.SourceStatus, string, string, string)) {
	status, file, dir, missing := open(t)
	for _, c := range []struct {
		path string
		kind mutants.SourceKind
	}{{file, mutants.SourceNonDirectory}, {dir, mutants.SourceDirectory}, {missing, mutants.SourceMissing}} {
		got, failure := status.Kind(t.Context(), c.path)
		if failure != nil || got != c.kind {
			t.Fatalf("%s: %v %v", c.path, got, failure)
		}
	}
	ctx, cancel := context.WithCancelCause(t.Context())
	cause := errors.New("kind stopped")
	cancel(cause)
	_, failure := status.Kind(ctx, file)
	var stopped mutants.InputCancelled
	if !errors.As(failure, &stopped) || !errors.Is(failure, cause) {
		t.Fatalf("kind cancellation: %v", failure)
	}
}
