package counting_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/donvargax/itos-cc/internal/counting"
	"github.com/donvargax/itos-cc/internal/counting/port"
	"github.com/donvargax/itos-cc/internal/counting/port/porttest"
	"pgregory.net/rapid"
)

func TestCountCountsLogicalLinesAndUnicodeWhitespaceWords(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  counting.Result
	}{
		{name: "empty", input: "", want: counting.Result{}},
		{name: "one-byte unterminated line", input: "x", want: counting.Result{Lines: 1, Words: 1}},
		{name: "one unterminated line", input: "one", want: counting.Result{Lines: 1, Words: 1}},
		{name: "terminated line", input: "one\n", want: counting.Result{Lines: 1, Words: 1}},
		{name: "empty lines", input: "\n\n", want: counting.Result{Lines: 2}},
		{name: "mixed line endings", input: "one\r\ntwo\nthree", want: counting.Result{Lines: 3, Words: 3}},
		{name: "unicode whitespace and punctuation", input: "a\u2003b,\tc!", want: counting.Result{Lines: 1, Words: 3}},
		{name: "invalid UTF-8 is not whitespace", input: string([]byte{'a', 0xff, 'b'}), want: counting.Result{Lines: 1, Words: 1}},
		{name: "long line", input: strings.Repeat("x", 128*1024), want: counting.Result{Lines: 1, Words: 1}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := counting.Count([]byte(test.input)); got != test.want {
				t.Errorf("Count(%q) = %+v, want %+v", test.input, got, test.want)
			}
		})
	}
}

func TestFileReadsThroughFilesAndReturnsContentsCount(t *testing.T) {
	files := porttest.Files{Data: map[string][]byte{"notes": []byte("one\r\ntwo words\n")}}
	got, err := counting.File(t.Context(), files, "notes")
	if err != nil || got != (counting.Result{Lines: 2, Words: 3}) {
		t.Fatalf("File() = %+v, %v; want 2 lines and 3 words", got, err)
	}
}

func TestFileReturnsTypedMissingAndUnreadableFailures(t *testing.T) {
	for _, test := range []struct {
		name        string
		files       port.Files
		wantMissing bool
	}{
		{name: "missing", files: porttest.Files{}, wantMissing: true},
		{name: "unreadable", files: porttest.Files{Failures: map[string]port.ReadFailure{"notes": porttest.Unreadable("notes")}}, wantMissing: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := counting.File(t.Context(), test.files, "notes")
			if err == nil {
				t.Fatal("File() returned no failure")
			}
			if got != (counting.Result{}) {
				t.Errorf("File() result on failure = %+v, want zero result", got)
			}
			var missing *port.Missing
			var unreadable *port.Unreadable
			if test.wantMissing && !errors.As(err, &missing) {
				t.Errorf("failure %T = %v, want typed Missing", err, err)
			}
			if test.wantMissing && missing != nil && missing.Error() != "missing file: notes" {
				t.Errorf("missing error = %q, want path-specific message", missing.Error())
			}
			if !test.wantMissing && !errors.As(err, &unreadable) {
				t.Errorf("failure %T = %v, want typed Unreadable", err, err)
			}
			if !test.wantMissing && unreadable != nil && unreadable.Error() != "unreadable file: notes" {
				t.Errorf("unreadable error = %q, want path-specific message", unreadable.Error())
			}
		})
	}
}

func TestCountAddsAcrossNewlineTerminatedPieces(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		pieces := rapid.SliceOfN(rapid.StringMatching(`[a-z\p{Zs}\t]{0,30}`), 0, 12).Draw(t, "pieces")
		var joined string
		var want counting.Result
		for _, piece := range pieces {
			terminated := piece + "\n"
			part := counting.Count([]byte(terminated))
			want.Lines += part.Lines
			want.Words += part.Words
			joined += terminated
		}
		if got := counting.Count([]byte(joined)); got != want {
			t.Fatalf("Count(joined pieces) = %+v, sum of pieces = %+v", got, want)
		}
	})
}
