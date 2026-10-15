package mutants

import (
	"crypto/sha256"
	"encoding/hex"

	"path/filepath"

	"sort"
)

// FreshUnit identifies one admitted committed function independently of its
// mutation sites.
type FreshUnit struct {
	Path, Function, Identity string
	StartLine, EndLine       int
	Hash                     string
}

// FreshCandidate identifies a static mutation site without relying on cached
// outcomes or on the unit's display name alone.
type FreshCandidate struct {
	Identity     string
	Path         string
	Function     string
	UnitIdentity string
	Site         Site
	Rank         string
}

func FreshRank(seed, rel, identity string) string {
	sum := sha256.Sum256([]byte(FreshPlanVersion + "\x00" + seed + "\x00" + filepath.ToSlash(rel) + "\x00" + identity))
	return hex.EncodeToString(sum[:])
}

func SortFreshCandidates(candidates []FreshCandidate) {
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].Rank != candidates[j].Rank {
			return candidates[i].Rank < candidates[j].Rank
		}
		return candidates[i].Identity < candidates[j].Identity
	})
}

const FreshPlanVersion = "fresh-plan-v2"
