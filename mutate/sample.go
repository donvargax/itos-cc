package mutate

import (
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"sort"

	"github.com/donvargax/itos-cc/lang"
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
	Rel            string
	BaselineFailed bool
	BaselineOutput string
	// Mutants is each sampled mutant, in site order, with what its snapshot
	// records and what it did now; empty when the baseline failed, since
	// none ran.
	Mutants []SampledMutant
}

// SampledMutant is a mutant run again: the outcome its snapshot records
// and the outcome it has now.
type SampledMutant struct {
	Site
	Function string // namespace#name
	Recorded string
	Outcome  string
}

// Agrees reports whether the mutant's outcome now is the one recorded.
// Killed and timeout agree: both mean the tests noticed, and a slower
// machine turns one into the other.
func (m SampledMutant) Agrees() bool {
	noticed := func(o string) bool { return o == Killed || o == Timeout }
	return m.Recorded == m.Outcome || noticed(m.Recorded) && noticed(m.Outcome)
}

// Sample runs again count mutants drawn by seed from those the fresh
// entries of files' snapshots record as killed, timeout, or survived, all
// of them when there are fewer, after each test command's baseline, in a
// worker's copy as Run does. Fresh is as Check judges it. It runs no
// coverage and writes nothing. The draw depends only on the seed and on
// each candidate's file, function, and site, so one seed draws the same
// mutants from the same snapshots wherever the code moved. opt's Workers,
// TimeoutFactor, TestCommand, AllTests, Judge, Tests, and Log apply as in
// Run; the rest is not used.
func Sample(files []string, count int, seed string, opt Options) (Sampled, error) {
	var states []*fileState
	defer func() {
		for _, s := range states {
			s.file.Close()
		}
	}()
	type candidate struct {
		state    *fileState
		site     int
		function string
		recorded string
		rank     string
	}
	var candidates []candidate
	for _, path := range files {
		f, err := lang.ParseFile(path)
		if err != nil {
			return Sampled{}, err
		}
		s := &fileState{file: f, sites: Sites(f), command: TestCommand(path, opt.TestCommand, opt.AllTests)}
		states = append(states, s)
		c, err := checkParsed(f, path, opt.Judge, opt.Tests, nil)
		if err != nil {
			return Sampled{}, err
		}
		s.rel, s.result = c.Rel, &FileResult{Rel: c.Rel}
		// Every site is left alone but those drawn.
		s.outcomes = make([]string, len(s.sites))
		at := map[int]map[string]int{}
		for i, site := range s.sites {
			s.outcomes[i] = skipped
			if at[site.Unit] == nil {
				at[site.Unit] = map[string]int{}
			}
			at[site.Unit][site.Key()] = i
		}
		for _, fn := range c.Functions {
			if fn.State != Fresh {
				continue
			}
			for _, m := range fn.Mutants {
				i, ok := at[fn.unit][m.key()]
				if !ok || (m.Outcome != Killed && m.Outcome != Timeout && m.Outcome != Survived) {
					continue
				}
				candidates = append(candidates, candidate{state: s, site: i, function: fn.Function, recorded: m.Outcome,
					rank: rank(seed, s.rel, fn.Function, m.key())})
			}
		}
	}
	result := Sampled{Cached: len(candidates), Files: []SampledFile{}}
	sort.SliceStable(candidates, func(a, b int) bool { return candidates[a].rank < candidates[b].rank })
	candidates = candidates[:min(count, len(candidates))]
	if len(candidates) == 0 {
		return result, nil
	}
	drawn := map[*fileState]map[int]candidate{}
	for _, c := range candidates {
		if drawn[c.state] == nil {
			drawn[c.state] = map[int]candidate{}
		}
		drawn[c.state][c.site] = c
		c.state.outcomes[c.site] = ""
	}
	if err := execute(states, opt); err != nil {
		return result, err
	}
	for _, s := range states {
		if drawn[s] == nil {
			continue
		}
		f := SampledFile{Rel: s.rel, Mutants: []SampledMutant{}}
		if s.result.BaselineFailed {
			f.BaselineFailed, f.BaselineOutput = true, s.result.BaselineOutput
			result.Files = append(result.Files, f)
			continue
		}
		for i, c := range drawn[s] {
			f.Mutants = append(f.Mutants, SampledMutant{Site: s.sites[i], Function: c.function, Recorded: c.recorded, Outcome: s.outcomes[i]})
		}
		sort.Slice(f.Mutants, func(a, b int) bool {
			ma, mb := f.Mutants[a], f.Mutants[b]
			if ma.Line != mb.Line {
				return ma.Line < mb.Line
			}
			if ma.Column != mb.Column {
				return ma.Column < mb.Column
			}
			return ma.Replacement < mb.Replacement
		})
		result.Files = append(result.Files, f)
	}
	return result, nil
}

// rank orders a candidate in the draw of seed: the SHA-256 of the seed, the
// file's slash-separated path, the function, and the site's key, which stay
// the same while the function is unchanged, on every platform.
func rank(seed, rel, function, key string) string {
	sum := sha256.Sum256([]byte(seed + "\x00" + filepath.ToSlash(rel) + "\x00" + function + "\x00" + key))
	return hex.EncodeToString(sum[:])
}
