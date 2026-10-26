package covrun

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// coverFiles is the names of the files in dir, sorted.
func coverFiles(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

func TestAListedFolderKeepsOnlyTheDataOfBinariesOtherThanTheMeasuringOne(t *testing.T) {
	d := t.TempDir()
	writeFiles(t, d, map[string]string{
		"covmeta.H":                 "",
		"covcounters.H.4242.170000": "",
		"covmeta.O":                 "",
		"covcounters.O.7.170001":    "",
	})
	if !hasCoverData(d, "H") {
		t.Errorf("the other binary's data in %s reads as nothing written", d)
	}
	if got, want := coverFiles(t, d), []string{"covcounters.O.7.170001", "covmeta.O"}; !slices.Equal(got, want) {
		t.Errorf("the folder holds %q, want the other binary's files alone %q", got, want)
	}
}

func TestWithoutAHashOfItsOwnNothingIsRemoved(t *testing.T) {
	d := t.TempDir()
	files := map[string]string{"covmeta.H": "", "covcounters.H.4242.170000": ""}
	writeFiles(t, d, files)
	if !hasCoverData(d, "") {
		t.Errorf("the data in %s reads as nothing written", d)
	}
	if got := coverFiles(t, d); len(got) != len(files) {
		t.Errorf("the folder holds %q, want every file kept", got)
	}
}

func TestAFolderHoldingOnlyTheMeasuringBinarysDataCountsAsNotWritten(t *testing.T) {
	d := t.TempDir()
	writeFiles(t, d, map[string]string{"covmeta.H": "", "covcounters.H.4242.170000": ""})
	if hasCoverData(d, "H") {
		t.Errorf("the measuring binary's own data in %s reads as written", d)
	}
	if got := coverFiles(t, d); len(got) != 0 {
		t.Errorf("the folder holds %q, want none of the measuring binary's files", got)
	}
}

func TestTheHashIsTheNameOfTheMetaDataFileWrittenToAScratchFolder(t *testing.T) {
	var scratch string
	got := coverHash(func(dir string) error {
		scratch = dir
		return os.WriteFile(filepath.Join(dir, "covmeta.H"), nil, 0o600)
	})
	if got != "H" {
		t.Errorf("hash %q, want H", got)
	}
	if _, err := os.Stat(scratch); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the scratch folder %s is left (%v)", scratch, err)
	}
	if got := coverHash(func(string) error { return errors.New("not built with -cover") }); got != "" {
		t.Errorf("hash %q from a binary without meta-data, want none", got)
	}
	if got := coverHash(func(string) error { return nil }); got != "" {
		t.Errorf("hash %q with no meta-data file written, want none", got)
	}
	missing := filepath.Join(t.TempDir(), "missing")
	for _, name := range []string{"TMPDIR", "TMP", "TEMP"} {
		t.Setenv(name, missing)
	}
	if got := coverHash(func(dir string) error { t.Errorf("wrote to %s with no scratch folder", dir); return nil }); got != "" {
		t.Errorf("hash %q with no scratch folder, want none", got)
	}
}
