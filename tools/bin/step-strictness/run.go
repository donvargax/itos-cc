package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func hash(b []byte) string { sum := sha256.Sum256(b); return hex.EncodeToString(sum[:]) }

type attempt struct {
	Directory string         `json:"directory"`
	Exit      int            `json:"exit"`
	Duration  time.Duration  `json:"duration_ns"`
	Error     string         `json:"error,omitempty"`
	Cases     []caseEvidence `json:"cases,omitempty"`
}
type probeResult struct {
	Name             string      `json:"name"`
	ScenarioID       string      `json:"scenario_id"`
	Classification   string      `json:"classification"`
	Reason           string      `json:"reason"`
	IntendedFailures int         `json:"intended_failures"`
	OriginalHash     string      `json:"original_sha256"`
	EditedHash       string      `json:"edited_sha256"`
	RestoredHash     string      `json:"restored_sha256,omitempty"`
	RestoredMode     os.FileMode `json:"restored_mode,omitempty"`
	Attempt          attempt     `json:"attempt"`
}
type report struct {
	Version          int           `json:"version"`
	Control          attempt       `json:"control"`
	Probes           []probeResult `json:"probes"`
	Detected         int           `json:"detected"`
	Undetected       int           `json:"undetected"`
	Invalid          int           `json:"invalid"`
	ControlCases     int           `json:"control_cases"`
	ControlSteps     int           `json:"control_steps"`
	IntendedFailures int           `json:"intended_failures"`
	Error            string        `json:"error,omitempty"`
	Duration         time.Duration `json:"duration_ns"`
}
type execution struct {
	exit int
	err  error
}
type executor func(context.Context, string, string, string) execution

func writeDurable(path string, b []byte, mode os.FileMode) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	_, writeErr := f.Write(b)
	return errors.Join(writeErr, f.Sync(), f.Close())
}
func writeJSON(path string, value any) error {
	b, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	// Reports are replaced atomically. Recovery originals are never replaced.
	tmp, err := os.CreateTemp(filepath.Dir(path), ".report-")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	_, writeErr := tmp.Write(append(b, '\n'))
	if err = errors.Join(writeErr, tmp.Sync(), tmp.Close()); err != nil {
		return err
	}
	return os.Rename(name, path)
}

func begin(q *prepared) error {
	if q.output == "" {
		base, err := filepath.EvalSymlinks(os.TempDir())
		if err != nil {
			return err
		}
		if within(q.root, base) {
			return errors.New("temporary recovery directory must be outside checkout")
		}
		if strings.Contains(base, ",") {
			return errors.New("temporary directory contains a Godog formatter separator")
		}
		var err2 error
		q.output, err2 = os.MkdirTemp(base, "step-strictness-")
		if err2 != nil {
			return err2
		}
	} else if err := os.Mkdir(q.output, 0o700); err != nil {
		return err
	}
	paths := make([]string, 0, len(q.sources))
	for p := range q.sources {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	originals := make([]*source, 0, len(paths))
	for i, path := range paths {
		s := q.sources[path]
		s.Backup = fmt.Sprintf("original-%03d", i+1)
		if err := writeDurable(filepath.Join(q.output, s.Backup), s.bytes, 0o600); err != nil {
			return err
		}
		originals = append(originals, s)
	}
	if err := writeJSON(filepath.Join(q.output, "recovery.json"), originals); err != nil {
		return err
	}
	return writeJSON(filepath.Join(q.output, "plan.json"), q.plan)
}

func sameFile(path string, want []byte, mode os.FileMode) error {
	if err := noLinks(path, false); err != nil {
		return err
	}
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if err = sourceMode(info.Mode()); err != nil {
		return fmt.Errorf("concurrent mode change at %s: %w", path, err)
	}
	actual, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if !bytes.Equal(actual, want) || info.Mode().Perm() != mode {
		return fmt.Errorf("concurrent change at %s; preserving it and original recovery data", path)
	}
	return nil
}

func replaceOwned(path string, from, to []byte, mode os.FileMode) error {
	if err := sameFile(path, from, mode); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".step-strictness-")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	_, writeErr := f.Write(to)
	if err = errors.Join(writeErr, f.Chmod(mode), f.Sync(), f.Close()); err != nil {
		return err
	}
	if err = sameFile(path, from, mode); err != nil {
		return err
	}
	if err = os.Rename(name, path); err != nil {
		return err
	}
	return sameFile(path, to, mode)
}

// May be called after an installation error: a successful rename can precede a
// failed verification. Both known states are safe; anything else is a conflict.
func restoreSource(s *source, edited []byte) error {
	if sameFile(s.Path, s.bytes, s.Mode) == nil {
		return nil
	}
	return replaceOwned(s.Path, edited, s.bytes, s.Mode)
}

func runAttempt(ctx context.Context, q prepared, dir, selection string, execute executor) attempt {
	a := attempt{Directory: dir, Exit: -1}
	if err := os.Mkdir(dir, 0o700); err != nil {
		a.Error = err.Error()
		return a
	}
	started := time.Now()
	runCtx, cancel := context.WithTimeout(ctx, time.Duration(q.plan.DeadlineSeconds)*time.Second)
	defer cancel()
	outcome := execute(runCtx, q.root, dir, selection) // Must join its owned runner before returning.
	a.Exit = outcome.exit
	a.Duration = time.Since(started)
	if runCtx.Err() != nil {
		a.Error = runCtx.Err().Error()
		return a
	}
	if outcome.err != nil {
		a.Error = outcome.err.Error()
		return a
	}
	cases, err := readCases(filepath.Join(dir, "cucumber.json"))
	if err != nil {
		a.Error = err.Error()
		return a
	}
	a.Cases = cases
	return a
}

func runPlan(ctx context.Context, q *prepared, execute executor) (r report, err error) {
	started := time.Now()
	r.Version = 1
	if err = begin(q); err != nil {
		return r, err
	}
	defer func() {
		r.Duration = time.Since(started)
		if err != nil {
			r.Error = err.Error()
		}
		err = errors.Join(err, writeJSON(filepath.Join(q.output, "report.json"), r))
	}()
	ids := map[string]bool{}
	var selection []string
	for _, p := range q.probes {
		if !ids[p.ScenarioID] {
			selection = append(selection, p.ScenarioID)
		}
		ids[p.ScenarioID] = true
	}
	sort.Strings(selection)
	r.Control = runAttempt(ctx, *q, filepath.Join(q.output, "000-control"), "^@("+strings.Join(selection, "|")+")$", execute)
	if r.Control.Error != "" {
		return r, fmt.Errorf("green control invalid: %s", r.Control.Error)
	}
	if r.Control.Exit != 0 {
		return r, fmt.Errorf("green control exited %d", r.Control.Exit)
	}
	if err = controlCases(r.Control.Cases, ids); err != nil {
		return r, err
	}
	r.ControlCases = len(r.Control.Cases)
	for _, c := range r.Control.Cases {
		r.ControlSteps += len(c.Steps)
	}
	// Validate every intended assertion against the real inventory before edits.
	for _, p := range q.probes {
		for _, c := range r.Control.Cases {
			if c.ScenarioID == p.ScenarioID {
				if _, err = intendedStep(c, p.Assertion, false); err != nil {
					return r, err
				}
			}
		}
	}
	for i, p := range q.probes {
		if err = ctx.Err(); err != nil {
			return r, err
		}
		s := q.sources[p.path]
		result := probeResult{Name: p.Name, ScenarioID: p.ScenarioID, OriginalHash: s.OriginalHash, EditedHash: hash(p.edited)}
		// The defer covers installation as well as every handled execution outcome,
		// including panic. An unexpected external edit is retained, never clobbered.
		func() {
			defer func() {
				if value := recover(); value != nil {
					err = fmt.Errorf("runner panic: %v", value)
				}
				restoreErr := restoreSource(s, p.edited)
				if restoreErr != nil {
					err = errors.Join(err, restoreErr)
					return
				}
				result.RestoredHash = s.OriginalHash
				result.RestoredMode = s.Mode
			}()
			if err = replaceOwned(p.path, s.bytes, p.edited, s.Mode); err != nil {
				return
			}
			result.Attempt = runAttempt(ctx, *q, filepath.Join(q.output, fmt.Sprintf("%03d-%s", i+1, p.Name)), "^@"+p.ScenarioID+"$", execute)
			if result.Attempt.Error != "" {
				result.Classification = "invalid"
				result.Reason = result.Attempt.Error
				return
			}
			result.Classification, result.Reason, result.IntendedFailures = classify(result.Attempt.Cases, r.Control.Cases, p.probe, result.Attempt.Exit)
		}()
		if err != nil {
			result.Classification = "invalid"
			result.Reason = err.Error()
		}
		switch result.Classification {
		case "detected":
			r.Detected++
		case "undetected":
			r.Undetected++
		default:
			r.Invalid++
		}
		r.Probes = append(r.Probes, result)
		r.IntendedFailures += result.IntendedFailures
		if writeErr := writeJSON(filepath.Join(q.output, "report.json"), r); writeErr != nil {
			return r, errors.Join(err, writeErr)
		}
		if err != nil {
			return r, err
		}
		if ctx.Err() != nil {
			return r, ctx.Err()
		}
	}
	return r, nil
}
