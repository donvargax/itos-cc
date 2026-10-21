package domain

import (
	"strconv"
	"strings"
)

type LineRange struct {
	Start, End int
	Deletion   bool
}

func HunkRanges(patch string) []LineRange {
	var ranges []LineRange
	for _, line := range strings.Split(patch, "\n") {
		if !strings.HasPrefix(line, "@@ ") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 3 || !strings.HasPrefix(fields[2], "+") {
			continue
		}
		startText, countText, hasCount := strings.Cut(fields[2][1:], ",")
		start, err := strconv.Atoi(startText)
		if err != nil {
			continue
		}
		count := 1
		if hasCount {
			count, err = strconv.Atoi(countText)
			if err != nil {
				continue
			}
		}
		if count == 0 {
			ranges = append(ranges, LineRange{start, start + 1, true})
		} else {
			ranges = append(ranges, LineRange{start, start + count - 1, false})
		}
	}
	return ranges
}

func OverlapsRanges(ranges []LineRange, start, end int) bool {
	for _, r := range ranges {
		if r.Start <= end && r.End >= start {
			return true
		}
	}
	return false
}
