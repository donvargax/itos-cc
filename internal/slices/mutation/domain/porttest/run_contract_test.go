package porttest

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/donvargax/itos-cc/internal/slices/mutation/domain"
)

func TestMemoryListingContract(t *testing.T) {
	ListingContract(t, func(t *testing.T, fails bool) (mutants.TestLister, mutants.Command, *bytes.Buffer) {
		root := t.TempDir()
		command := mutants.Command{Root: root, Dir: root, Shell: "independent list"}
		log := &bytes.Buffer{}
		fact := Listing{Stdout: " first \t tests/a.py \r\nfirst\twrong.py\nsecond\r\n", Stderr: "independent diagnostic\n"}
		if fails {
			fact.ExitCode = 7
			fact.Reason = "exit status 7"
		}
		return &Lister{Listings: map[string]map[string]Listing{root: {command.String(): fact}}, Log: log}, command, log
	})
}
func TestMemorySourceEditsContract(t *testing.T) {
	SourceEditsContract(t, func(t *testing.T) (mutants.SourceEdits, string, func() string, func(string)) {
		path := filepath.Join(t.TempDir(), "current.go")
		files := map[string]string{path: "original planning bytes"}
		return &SourceEdits{Files: files}, path, func() string { return files[path] }, func(text string) { files[path] = text }
	})
	path := filepath.Join(t.TempDir(), "same.go")
	files := map[string]string{path: "body\n\n// itos-cc mutation:\n// actual summary\n// end itos-cc mutation\n"}
	writer := &SourceEdits{Files: files}
	if failure := writer.Annotate(t.Context(), path, "//", "// itos-cc mutation:\n// actual summary\n// end itos-cc mutation\n"); failure != nil || len(writer.Writes) != 0 {
		t.Fatalf("identical bytes rewritten: %v %v", failure, writer.Writes)
	}
}
