package covrun

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The listed tests' coverage says on the log when their run ends and when
// their conversions end, with how long each took (slice-64, ID-MUT-367 to
// ID-MUT-369). The scenarios hold the lines through the binary; these hold
// them here too, where the code proof measures, through a recording
// executor that runs no listed test.

// splitting is a recorder whose listed run of every test writes Go
// coverage data for test a under the folder the run's environment names,
// as a harness that splits the run does, and whose conversions each write
// an empty profile where their -o names.
func splitting() *recorder {
	return &recorder{also: func(cmd *exec.Cmd) {
		for _, kv := range cmd.Env {
			if dir, ok := strings.CutPrefix(kv, TestCoverDirEnv+"="); ok {
				_ = os.MkdirAll(filepath.Join(dir, "a"), 0o750)
				_ = os.WriteFile(filepath.Join(dir, "a", "covmeta.0"), nil, 0o600)
			}
		}
		profiled(cmd)
	}}
}

// alone is a recorder whose runs write Go coverage data only where a
// test's run alone gives the processes GOCOVERDIR, as a harness that does
// not split the run of every test does.
func alone() *recorder {
	return &recorder{also: func(cmd *exec.Cmd) {
		if len(cmd.Args) > 2 && cmd.Args[2] == "every test" {
			return
		}
		for _, kv := range cmd.Env {
			if dir, ok := strings.CutPrefix(kv, "GOCOVERDIR="); ok {
				_ = os.WriteFile(filepath.Join(dir, "covmeta.0"), nil, 0o600)
			}
		}
		profiled(cmd)
	}}
}

// profiled writes an empty Go profile where cmd, a conversion, names its
// output.
func profiled(cmd *exec.Cmd) {
	for _, arg := range cmd.Args {
		if out, ok := strings.CutPrefix(arg, "-o="); ok {
			_ = os.WriteFile(out, []byte("mode: set\n"), 0o600)
		}
	}
}

// inOrder fails t unless log has a line matching each of lines, in order.
func inOrder(t *testing.T, log string, lines ...string) {
	t.Helper()
	at := 0
	all := strings.Split(log, "\n")
	for _, line := range lines {
		pattern := regexp.MustCompile("^" + line + "$")
		for at < len(all) && !pattern.MatchString(all[at]) {
			at++
		}
		if at == len(all) {
			t.Fatalf("no line %q in order in the log:\n%s", line, log)
		}
		at++
	}
}

func TestTheSupervisedListedTestsSayWhenTheirRunAndTheirConversionsEnd(t *testing.T) {
	for _, c := range []struct {
		name string
		rec  *recorder
		ran  string
	}{
		{"split", splitting(), "ran"},
		{"each alone", alone(), "ran one at a time"},
	} {
		t.Run(c.name, func(t *testing.T) {
			root := t.TempDir()
			p, _ := listed(root)
			var log bytes.Buffer
			tc, _, err := MeasureTestsSupervised(context.Background(), p, filepath.Join(root, "data"), nil, &log, c.rec.execute)
			if err != nil || tc == nil {
				t.Fatalf("listed coverage: %v\n%s", err, log.String())
			}
			inOrder(t, log.String(),
				`itos-cc: coverage .*\$ every test`,
				`itos-cc: coverage: the listed tests `+c.ran+` in [0-9]+\.[0-9]s; converting the coverage of 1 of them`,
				`itos-cc: coverage: converted the coverage of 1 listed tests in [0-9]+\.[0-9]s`)
		})
	}
}

func TestSupervisedListedTestsWithoutDataSayNothingOfAConversion(t *testing.T) {
	root := t.TempDir()
	p, _ := listed(root)
	p.Tests = append(p.Tests, Test{ID: "b"})
	var log bytes.Buffer
	// a writes data in the run of every test and b none: the run fails
	// closed before any conversion.
	if _, _, err := MeasureTestsSupervised(context.Background(), p, filepath.Join(root, "data"), nil, &log, splitting().execute); err == nil || !strings.Contains(err.Error(), "listed test b produced no per-ID coverage data") {
		t.Fatalf("a listed test without data: %v", err)
	}
	if strings.Contains(log.String(), "converting") || strings.Contains(log.String(), "converted") {
		t.Errorf("a run that converts nothing says it converts:\n%s", log.String())
	}
}

func TestTheListedTestsSayWhenTheirRunAndTheirConversionsEnd(t *testing.T) {
	root := t.TempDir()
	p, rec := listed(root)
	var log bytes.Buffer
	// The conversion of what the recorder wrote, a real go tool covdata,
	// fails, which is logged; the conversions still end.
	if _, failure := MeasureTests(context.Background(), p, filepath.Join(root, "data"), nil, &log, rec.execute); failure != nil {
		t.Fatalf("listed coverage: %v\n%s", failure, log.String())
	}
	inOrder(t, log.String(),
		`itos-cc: coverage .*\$ every test`,
		`itos-cc: coverage: the listed tests ran in [0-9]+\.[0-9]s; converting the coverage of 1 of them`,
		`itos-cc: coverage: converted the coverage of 1 listed tests in [0-9]+\.[0-9]s`)
}
