package domain

import "context"

// Exceptions updates the current document, rather than a stale selected Config.
// Except replaces the first SameSite entry; Renew matches complete Old values,
// consumes duplicates once and publishes only a complete batch; Prune
// removes entries matched the same way.
type Exceptions interface {
	Except(ctx context.Context, exception Exception) (bool, ExceptionFailure)
	Renew(ctx context.Context, renewals []Renewal) ExceptionFailure
	Prune(ctx context.Context, entries []Exception) ExceptionFailure
}

// ExceptionFailure preserves invalid-document facts separately from I/O/cancel.
//
//sumtype:decl
type ExceptionFailure interface {
	error
	exceptionFailure()
}

// ExceptionInvalid is a document that cannot be read, Reason saying why,
// Key naming the key at fault where one is, and Fix how to mend it where
// the refusal has its own mending.
type ExceptionInvalid struct{ Reason, Key, Fix string }

func (ExceptionInvalid) exceptionFailure() {}
func (f ExceptionInvalid) Error() string   { return f.Reason }

type ExceptionFailed struct{ Cause error }

func (ExceptionFailed) exceptionFailure() {}
func (f ExceptionFailed) Error() string   { return f.Cause.Error() }
func (f ExceptionFailed) Unwrap() error   { return f.Cause }

type ExceptionCancelled struct{ Cause error }

func (ExceptionCancelled) exceptionFailure() {}
func (f ExceptionCancelled) Error() string   { return f.Cause.Error() }
func (f ExceptionCancelled) Unwrap() error   { return f.Cause }

// SourceStatus observes following-Stat kind, not no-follow orphan presence.
type SourceStatus interface {
	Kind(ctx context.Context, path string) (SourceKind, InputFailure)
}
type SourceKind uint8

const (
	SourceMissing SourceKind = iota
	SourceDirectory
	SourceNonDirectory
)
