package mutants

import (
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
)

// Sampled is a sample of cached mutants run again.
type Sampled struct {
	// Cached is how many mutants the sample was drawn from: those the fresh
	// entries of the files' snapshots record as killed, timeout, or
	// survived.
	Cached int
	// Files holds each file with a sampled mutant, in the order given.
	Files []SampledFile
}

// SampledFile is one file's sampled mutants.
type SampledFile struct {
	Rel string
	// BaselineFailed is true when the baseline of a scope its sampled
	// mutants run in failed, with that baseline's output.
	BaselineFailed bool
	BaselineOutput string
	// FailedSelections is each selection of listed tests a sampled mutant
	// of the file needed that failed without any mutant: that mutant is
	// not decided, so none of the file's is reported, as when a baseline
	// failed.
	FailedSelections []FailedSelection
	// Mutants is each sampled mutant, in site order, with what its snapshot
	// records and what it did now; empty when a baseline or a selection
	// failed.
	Mutants []SampledMutant
}

// SampledMutant is a mutant run again: the outcome its snapshot records
// and the outcome it has now.
type SampledMutant struct {
	Site
	Function string // namespace#name
	Recorded string
	Outcome  string
	// Scope is the scope of the tests it ran with now: the one its
	// snapshot records, unless the sample was given one.
	Scope string
}

// Agrees reports whether the mutant's outcome now is the one recorded.
// Killed and timeout agree: both mean the tests noticed, and a slower
// machine turns one into the other.
func (m SampledMutant) Agrees() bool {
	noticed := func(o string) bool { return o == Killed || o == Timeout }
	return m.Recorded == m.Outcome || noticed(m.Recorded) && noticed(m.Outcome)
}

// rank orders a candidate in the draw of seed: the SHA-256 of the seed, the
// file's slash-separated path from the project root, the function, and the
// site's key, which stay the same while the function is unchanged, on every
// platform and from every directory.
func Rank(seed, rel, function, Key string) string {
	sum := sha256.Sum256([]byte(seed + "\x00" + filepath.ToSlash(rel) + "\x00" + function + "\x00" + Key))
	return hex.EncodeToString(sum[:])
}
