package metricsdir

import (
	"encoding/json"
	"errors"
	"maps"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/donvargax/itos-cc/internal/snapshot/port"
)

// shape is a snapshot of every kind a reader decodes into.
type shape struct {
	Name  string            `json:"name"`
	Count int               `json:"count"`
	Size  uint              `json:"size"`
	Share float32           `json:"share"`
	On    bool              `json:"on"`
	At    *time.Time        `json:"at"`
	Tests map[string]string `json:"tests"`
	Files []struct {
		Lines []int `json:"lines"`
	} `json:"files"`
}

func TestUndecodableWordsTheSnapshotsOwnTerms(t *testing.T) {
	cases := []struct{ json, reason string }{
		{`{"name": 7}`, "name should be a string, not a number"},
		{`{"count": "7"}`, "count should be a number, not a string"},
		{`{"count": 1.5}`, "count should be a whole number, not 1.5"},
		{`{"size": -1}`, "size should be a whole number, 0 or more, not -1"},
		{`{"share": 1e300}`, "share should be a number in range, not 1e300"},
		{`{"on": "yes"}`, "on should be a boolean, not a string"},
		{`{"name": true}`, "name should be a string, not a boolean"},
		{`{"at": 5}`, "at should be a string, not a number"},
		{`{"tests": []}`, "tests should be an object, not an array"},
		{`{"tests": {"a_test.go": {}}}`, "tests.a_test.go should be a string, not an object"},
		{`{"files": [{}, {"lines": [1, "2"]}]}`, "files[1].lines[1] should be a number, not a string"},
		{`"shape"`, "the snapshot should be an object, not a string"},
		{`{"name": "a"`, "unexpected end of JSON input"},
		{`{"name": "a"} x`, "invalid character 'x' after top-level value"},
	}
	for _, c := range cases {
		var v shape
		err := json.Unmarshal([]byte(c.json), &v)
		if err == nil {
			t.Fatalf("%s decoded", c.json)
		}
		got := Undecodable("s.json", err)
		if got.Name != "s.json" || got.Reason != c.reason {
			t.Errorf("Undecodable(%s) = %q, %q; want s.json, %q", c.json, got.Name, got.Reason, c.reason)
		}
	}
}

func TestUndecodableSaysNoGoTypeOfAMisusedDecoder(t *testing.T) {
	var v *shape
	err := json.Unmarshal([]byte(`{}`), v)
	if err == nil {
		t.Fatal("decoded into a nil pointer")
	}
	reason := Undecodable("s.json", err).Reason
	if reason != "it does not decode as this snapshot" || strings.Contains(reason, "shape") {
		t.Errorf("Reason = %q; want it said without the decoder's detail", reason)
	}
}

// Read words a snapshot that does not decode as Undecodable does, so a
// reader of .metrics/ hears the snapshot's key and kinds, never the Go
// types it decodes into; one that could not be read keeps the library's
// text (TestStoreTranslatesTheLibrarysFailures).
func TestReadWordsAnUndecodableSnapshotInItsOwnTerms(t *testing.T) {
	reasons := map[string]string{
		"bad.json":           "unexpected end of JSON input",
		"mutate/bad.go.json": "version should be a number, not a string",
		"files.json":         "files[1] should be a string, not a number",
		"list.json":          "the snapshot should be an object, not an array",
	}
	store, _ := open(t, map[string]string{
		"bad.json":           "{",
		"mutate/bad.go.json": `{"version": "one"}`,
		"files.json":         `{"version": 1, "files": ["a.go", 7]}`,
		"list.json":          `[1]`,
	})
	for _, name := range slices.Sorted(maps.Keys(reasons)) {
		var got sample
		found, failure := store.Read(t.Context(), name, &got)
		var unreadable *port.Unreadable
		if !found || !errors.As(failure, &unreadable) || unreadable.Name != name || unreadable.Reason != reasons[name] {
			t.Errorf("Read(%s) = %t, %v; want there and Unreadable because %q", name, found, failure, reasons[name])
		}
	}
}
