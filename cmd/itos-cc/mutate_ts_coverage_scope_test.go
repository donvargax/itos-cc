package main

import (
	"strings"
)

// A helper the counted tests share. ID-MUT-208 holds the ts-coverage-scope
// slice's old test whole (T-101).

// coverageLines are the coverage commands stderr says ran.
func coverageLines(stderr string) []string {
	var out []string
	for _, line := range strings.Split(stderr, "\n") {
		if strings.HasPrefix(line, "itos-cc: coverage ") && strings.Contains(line, "$ ") {
			out = append(out, line)
		}
	}
	return out
}
