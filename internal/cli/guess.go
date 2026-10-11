package cli

import "strings"

// Closest is the candidate nearest to word, or "" when none is near enough
// to be what was meant: fewer than three edits away, the first of the
// nearest in candidates' order. A word that only ends differently, such as
// mutate for mutation, is near too: all but its last letter, four letters
// at least, begin exactly one candidate. An unknown command and an unknown
// flag name the one meant by it.
func Closest(word string, candidates []string) string {
	best, bestDist := "", 3
	for _, c := range candidates {
		if d := distance(word, c); d < bestDist {
			best, bestDist = c, d
		}
	}
	if best != "" || len(word) < 5 {
		return best
	}
	stem := word[:len(word)-1]
	for _, c := range candidates {
		if strings.HasPrefix(c, stem) {
			if best != "" {
				return ""
			}
			best = c
		}
	}
	return best
}

// distance is the Levenshtein distance between a and b.
func distance(a, b string) int {
	prev := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur := make([]int, len(b)+1)
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev = cur
	}
	return prev[len(b)]
}
