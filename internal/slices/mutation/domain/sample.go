package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"slices"
)

// Sampled is a sample of cached mutants run again.
type Sampled struct {
	// Cached is how many mutants the sample was drawn from: those the fresh
	// entries of the files' snapshots record as killed, timeout,
	// over-limit, or survived.
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
	BaselineOutput Output
	// BaselineCopy is that baseline's tree a worker's copy holds when its
	// command passes in the project's own folder (FileResult), and
	// BaselineLeftOut what git ignores, and other repositories'
	// checkouts, that the copy left out.
	BaselineCopy    string
	BaselineLeftOut []string
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
// and the outcomes it has now.
type SampledMutant struct {
	Site
	Function string // namespace#name
	Recorded string
	// Outcome is the outcome of its first run now, and Outcomes every
	// outcome its runs gave, in run order: one when it agrees with
	// Recorded, more when it ran again (Classify).
	Outcome  string
	Outcomes []string
	// Scope is the scope of the tests it ran with now: the one its
	// snapshot records, unless the sample was given one.
	Scope string
	// Repeat is how many times it ran at least, --repeat; 0 counts as 1.
	Repeat int
}

// Agrees reports whether the mutant's outcome now is the one recorded.
func (m SampledMutant) Agrees() bool {
	return Agrees(m.Recorded, m.Outcome)
}

// Verdict is what the mutant's outcomes now say of the one recorded
// (Repeated).
func (m SampledMutant) Verdict() string {
	return Repeated(m.Recorded, m.Outcomes, m.Repeat)
}

// Agrees reports whether outcome, a run's, is recorded, the one a snapshot
// records. Killed, timeout and over-limit agree: each means the tests
// noticed, and a slower machine turns one into another.
func Agrees(recorded, outcome string) bool {
	return recorded == outcome || Noticed(recorded) && Noticed(outcome)
}

// The verdicts of Classify.
const (
	// Agreed is a mutant whose first run agrees with its recording.
	Agreed = "agreed"
	// Flaky is a mutant whose first run disagrees with its recording and
	// a later one agrees: its tests decide it differently with nothing
	// changed, so neither outcome is trusted.
	Flaky = "flaky"
	// Mismatched is a mutant all of whose runs disagree with its
	// recording.
	Mismatched = "mismatched"
)

// Reruns is how many times a sample runs again a mutant whose outcome
// disagrees with its recording, to tell a flaky one from a mismatch.
const Reruns = 2

// Classify is the verdict of a sampled mutant's outcomes, in run order,
// against recorded, the outcome its snapshot records: Agreed when the
// first agrees, Flaky when a later one does, Mismatched when the first and
// its Reruns all disagree, and "" while it needs another run.
func Classify(recorded string, outcomes []string) string {
	switch {
	case len(outcomes) == 0:
		return ""
	case Agrees(recorded, outcomes[0]):
		return Agreed
	case slices.ContainsFunc(outcomes[1:], func(o string) bool { return Agrees(recorded, o) }):
		return Flaky
	case len(outcomes) <= Reruns:
		return ""
	}
	return Mismatched
}

// Repeated is the verdict of the outcomes, in run order, of a mutant that
// runs at least repeat times (--repeat) against recorded, the outcome its
// snapshot records: "" while it has run fewer times, Flaky when those
// first repeat outcomes are not all alike (Alike), whatever recorded
// says, and otherwise Classify's, so outcomes alike that disagree with
// recorded are confirmed as Classify confirms them. A repeat of 0 or 1 is
// Classify alone; repeat is never negative.
func Repeated(recorded string, outcomes []string, repeat int) string {
	switch {
	case len(outcomes) < repeat:
		return ""
	case !Alike(outcomes[:repeat]):
		return Flaky
	}
	return Classify(recorded, outcomes)
}

// Alike reports whether outcomes, a mutant's, all agree with each other,
// as Agrees has them: killed, timeout and over-limit agree. None, or one,
// are alike.
func Alike(outcomes []string) bool {
	return !slices.ContainsFunc(outcomes, func(o string) bool { return !Agrees(outcomes[0], o) })
}

// Settled is the verdict of the outcomes, in run order, of a mutant its
// snapshot records flaky, which no outcome recorded can be judged
// against: Agreed once its first run and its Reruns all agree with each
// other, Flaky as soon as one disagrees with the first, and "" while it
// needs another run.
func Settled(outcomes []string) string {
	switch {
	case len(outcomes) == 0:
		return ""
	case !Alike(outcomes):
		return Flaky
	case len(outcomes) <= Reruns:
		return ""
	}
	return Agreed
}

// rank orders a candidate in the draw of seed: the SHA-256 of the seed, the
// file's slash-separated path from the project root, the function, and the
// site's key, which stay the same while the function is unchanged, on every
// platform and from every directory.
func Rank(seed, rel, function, Key string) string {
	sum := sha256.Sum256([]byte(seed + "\x00" + filepath.ToSlash(rel) + "\x00" + function + "\x00" + Key))
	return hex.EncodeToString(sum[:])
}
