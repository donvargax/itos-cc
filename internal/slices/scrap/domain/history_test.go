package teststructure

import (
	"slices"
	"testing"
)

// files are the File of each report.
func files(reports []FileReport) []string {
	var out []string
	for _, r := range reports {
		out = append(out, r.File)
	}
	return out
}

// A run records the files it measured, compared with their last reports,
// and keeps every file it did not measure (ID-SCRAP-27); what a snapshot
// writes is sorted by key, and carries no comparison.
func TestHistoryKeepsTheFilesARunDidNotMeasure(t *testing.T) {
	stale := &Comparison{Verdict: "worse"}
	h := HistoryOf([]FileReport{
		{File: "src/b.test.ts", Pressure: 9, Compare: stale},
		{File: "src/a.test.ts", Pressure: 1},
		{File: "src/a.test.ts", Pressure: 4},
	})
	got := h.Record("src/a.test.ts", FileReport{File: "a.test.ts", Pressure: 2})
	if got.File != "a.test.ts" || got.Compare == nil || got.Compare.Verdict != "improved" || got.Compare.PreviousPressure != 4 || got.Compare.Delta != -2 {
		t.Errorf("Record(a) = %+v, compare %+v; want a.test.ts improved from its last report, 4", got, got.Compare)
	}
	fresh := h.Record("src/c.test.ts", FileReport{File: "c.test.ts", Pressure: 3})
	if fresh.Compare != nil {
		t.Errorf("Record(c), a file never recorded, compared with %+v", fresh.Compare)
	}
	written := h.Files()
	if !slices.Equal(files(written), []string{"src/a.test.ts", "src/b.test.ts", "src/c.test.ts"}) {
		t.Fatalf("Files() = %q, want every file by key, sorted", files(written))
	}
	for _, r := range written {
		if r.Compare != nil {
			t.Errorf("%s is written with a comparison, %+v", r.File, r.Compare)
		}
	}
	if written[0].Pressure != 2 || written[1].Pressure != 9 {
		t.Errorf("Files() = %+v, want a's new report and b's kept", written)
	}
}

// A history of nothing writes an empty list, never none.
func TestAnEmptyHistoryWritesAnEmptyList(t *testing.T) {
	if got := HistoryOf(nil).Files(); got == nil || len(got) != 0 {
		t.Errorf("Files() = %#v, want an empty list", got)
	}
}
