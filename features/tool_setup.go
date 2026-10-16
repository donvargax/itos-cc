package features

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"

	"github.com/donvargax/itos-cc/project"
)

// Tools verifies independent fixture facts with the same caller cwd and
// environment that the product's ordinary interpreter probe inherits.
func (binary) Tools(ctx context.Context, dir string, setup ToolSetup) error {
	if cause := context.Cause(ctx); cause != nil {
		return cause
	}
	for _, name := range setup.NodeBins {
		if filepath.Base(name) != name {
			return fmt.Errorf("invalid fixture NodeBin %q", name)
		}
		if project.NodeBin(dir, name) == "" {
			return fmt.Errorf("fixture declares installed NodeBin %s, but none is observed from %s", name, dir)
		}
	}
	if setup.Python == nil {
		return nil
	}
	probe := setup.Python
	if probe.Interpreter == "" {
		return errors.New("Python setup probe needs its actual interpreter identity")
	}
	cmd := exec.CommandContext(ctx, probe.Interpreter, "-c", "import pytest")
	cmd.Dir, cmd.Env = dir, environment(setup.Env)
	err := cmd.Run()
	if cause := context.Cause(ctx); cause != nil {
		return cause
	}
	var exit *exec.ExitError
	if err != nil && !errors.As(err, &exit) {
		return fmt.Errorf("fixture interpreter probe could not execute: %w", err)
	}
	if (err == nil) != probe.Importable {
		return fmt.Errorf("fixture pytest import from %s using %s: importable=%t, declared %t", dir, probe.Interpreter, err == nil, probe.Importable)
	}
	return nil
}
