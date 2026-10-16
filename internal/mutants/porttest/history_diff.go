package porttest

import (
	"maps"
	"slices"
	"strings"

	"github.com/donvargax/itos-cc/internal/mutants"
)

// historyRanges compares actual fixture lines. LCS edits keep pure deletion
// admission separate from the new-side executable-line ranges, as -U0 does.
func historyRanges(before, after string) []mutants.LineRange {
	lines := func(text string) []string {
		if text == "" {
			return nil
		}
		return strings.Split(strings.TrimSuffix(text, "\n"), "\n")
	}
	a, b := lines(before), lines(after)
	common := make([][]int, len(a)+1)
	for i := range common {
		common[i] = make([]int, len(b)+1)
	}
	for i := len(a) - 1; i >= 0; i-- {
		for j := len(b) - 1; j >= 0; j-- {
			if a[i] == b[j] {
				common[i][j] = common[i+1][j+1] + 1
			} else {
				common[i][j] = max(common[i+1][j], common[i][j+1])
			}
		}
	}
	var ranges []mutants.LineRange
	i, j, start, editing := 0, 0, 0, false
	flush := func() {
		if !editing {
			return
		}
		if start == j {
			ranges = append(ranges, mutants.LineRange{Start: j, End: j + 1, Deletion: true})
		} else {
			ranges = append(ranges, mutants.LineRange{Start: start + 1, End: j})
		}
		editing = false
	}
	for i < len(a) || j < len(b) {
		if i < len(a) && j < len(b) && a[i] == b[j] {
			flush()
			i++
			j++
			continue
		}
		if !editing {
			start = j
			editing = true
		}
		if i < len(a) && (j == len(b) || common[i+1][j] >= common[i][j+1]) {
			i++
		} else {
			j++
		}
	}
	flush()
	return ranges
}

// historyRenames detects one-to-one fixture moves from immutable contents,
// including edited files retaining at least half their bytes in complete lines.
// Exact copies win before similarity matching; unrelated additions stay additions.
func historyRenames(before, after map[string]string) map[string]string {
	removed, added := map[string]string{}, map[string]string{}
	for p, text := range before {
		if _, ok := after[p]; !ok {
			removed[p] = text
		}
	}
	for p, text := range after {
		if _, ok := before[p]; !ok {
			added[p] = text
		}
	}
	out := map[string]string{}
	for _, old := range slices.Sorted(maps.Keys(removed)) {
		for _, next := range slices.Sorted(maps.Keys(added)) {
			if removed[old] == added[next] {
				out[old] = next
				delete(removed, old)
				delete(added, next)
				break
			}
		}
	}
	for len(removed) > 0 && len(added) > 0 {
		oldBest, newBest, best := "", "", 0
		for _, old := range slices.Sorted(maps.Keys(removed)) {
			for _, next := range slices.Sorted(maps.Keys(added)) {
				score := historySimilarity(removed[old], added[next])
				if score > best {
					oldBest, newBest, best = old, next, score
				}
			}
		}
		if best < 50 {
			break
		}
		out[oldBest] = newBest
		delete(removed, oldBest)
		delete(added, newBest)
	}
	return out
}
func historySimilarity(a, b string) int {
	size := max(len(a), len(b))
	if size == 0 {
		return 100
	}
	counts := map[string]int{}
	for _, line := range strings.SplitAfter(a, "\n") {
		counts[line]++
	}
	same := 0
	for _, line := range strings.SplitAfter(b, "\n") {
		if counts[line] > 0 {
			counts[line]--
			same += len(line)
		}
	}
	return 100 * same / size
}
