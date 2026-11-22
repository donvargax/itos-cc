package cli

import (
	"path/filepath"
	"testing"
)

func TestShowNamesAFileSlashSeparatedWhateverTheSystem(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name       string
		ui         *UI
		path, want string
	}{
		{"Windows' relative file", &UI{Separator: '\\'}, `src\board.go`, "src/board.go"},
		{"a file outside the directory", &UI{Dir: "/work", Separator: filepath.Separator}, "/other/board.go", "/other/board.go"},
		{"a slash file", &UI{Dir: "/work", Separator: filepath.Separator}, "/work/src/board.go", "src/board.go"},
		{"this system's separator when none is given", &UI{}, filepath.Join("src", "board.go"), "src/board.go"},
	} {
		if got := tc.ui.Show(tc.path).String(); got != tc.want {
			t.Errorf("%s: Show(%q) = %q, want %q", tc.name, tc.path, got, tc.want)
		}
	}
}
