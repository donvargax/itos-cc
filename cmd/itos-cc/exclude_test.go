package main

import (
	"maps"
	"os"
	"testing"
)

// The scenarios of slice-6, itos-cc.yaml's exclude, run through godog:
// file_selection.feature's ID-SEL-17 to 19 (T-62) and mutate.feature's
// ID-MUT-243 to 246 (T-70). What one of them names and no step of it
// reads is held here: ID-MUT-245's title says no exception of an excluded
// source can be added, and its steps read the refusal, not the file. The
// test uses the module of mutate_since_test.go with a fake beside it in a
// package of its own, src/port/porttest.

// fakeFile is the fake, a source of test support, by its path from the
// module's root.
const fakeFile = "src/port/porttest/fake.go"

// fakeText is the fake. Its line 7 has three mutation sites, the first the
// 0 at column 9.
const fakeText = `package porttest

// Fake stands in for a board in tests.
type Fake struct{ n int }

func (f *Fake) Over(i int) bool {
	return 0 < i-f.n
}
`

// excludePorttest is an itos-cc.yaml that excludes every porttest package.
const excludePorttest = "exclude: [\"**/porttest/**\"]\n"

// fakeException is an entry of itos-cc.yaml excepting the survivor at
// line 7, column 9 of the fake, written as a person or mutation except
// writes one.
const fakeException = `mutation:
  exceptions:
    - file: src/port/porttest/fake.go
      function: example.com/m/src/port/porttest.Fake#Over
      hash: "0000000000000000"
      line_in_function: 2
      column: 9
      original: "0"
      replacement: "1"
      reason: the fake's bound is never read
`

// withFake is the files of every map in extra, the fake added.
func withFake(extra ...map[string]string) map[string]string {
	files := map[string]string{fakeFile: fakeText}
	for _, m := range extra {
		maps.Copy(files, m)
	}
	return files
}

func TestAnExceptionRefusedForAnExcludedSourceLeavesItosCcYamlAsItWas(t *testing.T) {
	t.Parallel()
	boardRepo(t, withFake(killedTests, map[string]string{"itos-cc.yaml": excludePorttest + fakeException}))

	site := fakeFile + ":7:9"
	if o := itosCc(t, "mutation", "except", site, "--reason", "x"); o.code != 2 {
		t.Errorf("mutation except %s: exit %d, want 2\n%s%s", site, o.code, o.stdout, o.stderr)
	}
	data, err := os.ReadFile(inWD(t, "itos-cc.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if got := string(data); got != excludePorttest+fakeException {
		t.Errorf("mutation except rewrote itos-cc.yaml:\n%s", got)
	}
}
