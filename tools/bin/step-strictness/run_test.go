package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRestoresExactDirtyBaselineAfterEachOutcome(t *testing.T) {
	for _, outcome := range []string{"detected", "undetected", "compile", "setup", "missing report", "deadline", "interrupt", "panic"} {
		t.Run(outcome, func(t *testing.T) {
			path, source, p := fixturePlan(t)
			savePlan(t, path, p)
			q, err := prepare(path)
			if err != nil {
				t.Fatal(err)
			}
			original, err := os.ReadFile(source)
			if err != nil {
				t.Fatal(err)
			}
			info, err := os.Stat(source)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			joined := false
			execute := func(ctx context.Context, root, dir, selection string) execution {
				b, err := os.ReadFile(source)
				if err != nil {
					return execution{-1, err}
				}
				if strings.Contains(selection, "(") {
					saveCases(t, filepath.Join(dir, "cucumber.json"), []caseEvidence{example(2)})
					return execution{0, nil}
				}
				if !bytes.Contains(b, []byte("want := 2")) {
					t.Fatal("probe did not edit actual source")
				}
				defer func() { joined = true }()
				c := failingCase(2)
				exit := 1
				switch outcome {
				case "undetected":
					c = example(2)
					exit = 0
				case "compile", "missing report":
					return execution{1, nil}
				case "setup":
					c.Steps[0].Result.Status = "failed"
					c.Steps[0].Result.Error = "setup failure"
				case "deadline":
					return execution{-1, context.DeadlineExceeded}
				case "interrupt":
					cancel()
					<-ctx.Done()
					return execution{-1, ctx.Err()}
				case "panic":
					panic("test runner panic")
				}
				saveCases(t, filepath.Join(dir, "cucumber.json"), []caseEvidence{c})
				return execution{exit, nil}
			}
			r, err := runPlan(ctx, &q, execute)
			t.Cleanup(func() {
				if err := os.RemoveAll(q.output); err != nil {
					t.Error(err)
				}
			})
			if (outcome == "interrupt" || outcome == "panic") && err == nil {
				t.Fatal("handled cancellation/panic lost")
			}
			if len(r.Probes) != 1 || !joined {
				t.Fatalf("runner was not joined: %+v", r)
			}
			want := "invalid"
			if outcome == "detected" || outcome == "undetected" {
				want = outcome
			}
			if r.Probes[0].Classification != want {
				t.Fatalf("got %+v, want %s", r.Probes[0], want)
			}
			after, err := os.ReadFile(source)
			if err != nil {
				t.Fatal(err)
			}
			afterInfo, err := os.Stat(source)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(after, original) || info.Mode().Perm() != afterInfo.Mode().Perm() {
				t.Fatal("dirty bytes or mode not restored")
			}
			if r.Probes[0].RestoredHash != hash(original) {
				t.Fatal("missing verified restoration hash")
			}
			backup, err := os.ReadFile(filepath.Join(q.output, "original-001"))
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(backup, original) {
				t.Fatal("recovery bytes differ")
			}
			if _, err = os.Stat(filepath.Join(q.output, "report.json")); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestConcurrentChangesArePreservedWithRecovery(t *testing.T) {
	path, source, p := fixturePlan(t)
	savePlan(t, path, p)
	q, err := prepare(path)
	if err != nil {
		t.Fatal(err)
	}
	execute := func(_ context.Context, _, dir, selection string) execution {
		if !strings.Contains(selection, "(") {
			if err := os.WriteFile(source, []byte("someone else's new work"), 0o700); err != nil {
				t.Fatal(err)
			}
		}
		saveCases(t, filepath.Join(dir, "cucumber.json"), []caseEvidence{example(2)})
		return execution{0, nil}
	}
	r, err := runPlan(context.Background(), &q, execute)
	t.Cleanup(func() { _ = os.RemoveAll(q.output) })
	if err == nil || !strings.Contains(err.Error(), "concurrent change") {
		t.Fatalf("missing conflict: %+v %v", r, err)
	}
	b, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "someone else's new work" {
		t.Fatal("clobbered concurrent edit")
	}
	if _, err = os.Stat(filepath.Join(q.output, "recovery.json")); err != nil {
		t.Fatal("lost recovery", err)
	}
	if r.Probes[0].RestoredHash != "" {
		t.Fatal("claimed restoration after conflict")
	}
}

func TestInvalidGreenControlNeverEdits(t *testing.T) {
	path, source, p := fixturePlan(t)
	savePlan(t, path, p)
	q, err := prepare(path)
	if err != nil {
		t.Fatal(err)
	}
	r, err := runPlan(context.Background(), &q, func(context.Context, string, string, string) execution {
		return execution{1, errors.New("compile failure")}
	})
	t.Cleanup(func() { _ = os.RemoveAll(q.output) })
	if err == nil || len(r.Probes) != 0 {
		t.Fatalf("accepted invalid control: %+v %v", r, err)
	}
	b, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "dirty WIP\nwant := 1\n" {
		t.Fatal("control failure edited source")
	}
}

func TestAttemptsNeverOverwritePreviousEvidence(t *testing.T) {
	path, _, p := fixturePlan(t)
	second := p.Probes[0]
	second.Name = "second"
	p.Probes = append(p.Probes, second)
	savePlan(t, path, p)
	q, err := prepare(path)
	if err != nil {
		t.Fatal(err)
	}
	r, err := runPlan(context.Background(), &q, func(_ context.Context, _, dir, selection string) execution {
		c := example(2)
		exit := 0
		if !strings.Contains(selection, "(") {
			c = failingCase(2)
			exit = 1
		}
		saveCases(t, filepath.Join(dir, "cucumber.json"), []caseEvidence{c})
		return execution{exit, nil}
	})
	t.Cleanup(func() { _ = os.RemoveAll(q.output) })
	if err != nil || r.Detected != 2 {
		t.Fatalf("%+v %v", r, err)
	}
	if r.Control.Directory == r.Probes[0].Attempt.Directory || r.Probes[0].Attempt.Directory == r.Probes[1].Attempt.Directory {
		t.Fatal("attempt evidence paths overlap")
	}
	for _, dir := range []string{r.Control.Directory, r.Probes[0].Attempt.Directory, r.Probes[1].Attempt.Directory} {
		if _, err = os.ReadFile(filepath.Join(dir, "cucumber.json")); err != nil {
			t.Fatal(err)
		}
	}
}

func TestRestorationAfterInstalledButUnverifiedEdit(t *testing.T) {
	path, source, p := fixturePlan(t)
	savePlan(t, path, p)
	q, err := prepare(path)
	if err != nil {
		t.Fatal(err)
	}
	s := q.sources[q.probes[0].path]
	edited := q.probes[0].edited
	// Model the observable state when rename succeeded and the final verification
	// returned an I/O error. Recovery must accept the known installed bytes.
	if err = os.WriteFile(source, edited, s.Mode); err != nil {
		t.Fatal(err)
	}
	if err = restoreSource(s, edited); err != nil {
		t.Fatal(err)
	}
	if err = sameFile(source, s.bytes, s.Mode); err != nil {
		t.Fatal(err)
	}
	// An installation failure before rename leaves the known original state.
	if err = restoreSource(s, edited); err != nil {
		t.Fatal(err)
	}
}

func TestConcurrentSpecialModeIsNotOverwritten(t *testing.T) {
	path, file, p := fixturePlan(t)
	savePlan(t, path, p)
	q, err := prepare(path)
	if err != nil {
		t.Fatal(err)
	}
	s := q.sources[q.probes[0].path]
	edited := q.probes[0].edited
	if err = replaceOwned(file, s.bytes, edited, s.Mode); err != nil {
		t.Fatal(err)
	}
	if err = os.Chmod(file, s.Mode|os.ModeSetuid); err != nil {
		t.Skipf("special mode unavailable: %v", err)
	}
	info, err := os.Stat(file)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSetuid == 0 {
		t.Skip("filesystem does not retain setuid")
	}
	if err = restoreSource(s, edited); err == nil {
		t.Fatal("concurrent special mode was overwritten")
	}
	after, err := os.Stat(file)
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if after.Mode()&os.ModeSetuid == 0 || !bytes.Equal(b, edited) {
		t.Fatal("concurrent state was changed")
	}
}
