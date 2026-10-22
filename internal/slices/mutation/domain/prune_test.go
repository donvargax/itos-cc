package domain

import (
	"slices"
	"testing"
)

func TestPrunedIsTheEntriesNothingCanMendInTheirOrder(t *testing.T) {
	entry := func(column int) Exception {
		return Exception{File: "a.go", Function: "p#F", Column: column, Reason: "r"}
	}
	moved := entry(6)
	moved.File = "b.go"
	stale := []StaleException{
		{Exception: entry(5), Why: ExceptionExcluded},
		{Exception: entry(2), Why: ExceptionChanged},
		{Exception: entry(1), Why: ExceptionKilled, Line: 7},
		{Exception: moved, Why: ExceptionKilled},
		{Exception: entry(3), Why: ExceptionGone},
		{Exception: entry(4), Why: ExceptionMoved, NewFile: "b.go"},
		{Exception: entry(3), Why: ExceptionGone},
	}
	got := Pruned([]Exception{entry(1), entry(2), entry(3), entry(4), entry(5), entry(3), entry(6)}, stale)
	want := []StaleException{stale[2], stale[4], stale[0], stale[6]}
	if !slices.Equal(got, want) {
		t.Errorf("pruned %+v, want %+v", got, want)
	}
}
