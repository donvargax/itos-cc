// Package testinputs fingerprints mutation test and independent coverage
// provenance. Its inventories preserve the legacy language/root boundaries.
package testinputs

import (
	"context"
	"os"
	"runtime"
	"strings"

	"github.com/donvargax/itos-cc/internal/lang"
	"github.com/donvargax/itos-cc/internal/slices/mutation/domain"
)

// CurrentRuntime is captured at the real process boundary, never by memory.
// The Go version is not itos-cc's own: Go evidence asks the project's
// toolchain (go env GOVERSION).
func CurrentRuntime() mutants.RuntimeInputs {
	env := map[string]string{}
	for _, v := range os.Environ() {
		k, value, _ := strings.Cut(v, "=")
		env[k] = value
	}
	return mutants.RuntimeInputs{Platform: runtime.GOOS, Architecture: runtime.GOARCH, Env: env}
}

type Fingerprints struct{ Runtime mutants.RuntimeInputs }

func inputFailure(ctx context.Context, err error) mutants.InputFailure {
	if cause := context.Cause(ctx); cause != nil {
		return mutants.InputCancelled{Cause: cause}
	}
	if err != nil {
		return mutants.InputFailed{Cause: err}
	}
	return nil
}
func (f Fingerprints) Tests(ctx context.Context, root string, tests []string) (map[string]string, mutants.InputFailure) {
	if failure := inputFailure(ctx, nil); failure != nil {
		return nil, failure
	}
	out, err := TestHashes(root, tests)
	return out, inputFailure(ctx, err)
}
func (f Fingerprints) Support(ctx context.Context, root string, globs []string) (map[string]string, mutants.InputFailure) {
	if failure := inputFailure(ctx, nil); failure != nil {
		return nil, failure
	}
	out, err := SupportHashes(root, globs)
	return out, inputFailure(ctx, err)
}
func (f Fingerprints) Suite(ctx context.Context, source, root string) (map[string]string, mutants.InputFailure) {
	if failure := inputFailure(ctx, nil); failure != nil {
		return nil, failure
	}
	out, err := SuiteTestHashes(ctx, source, root)
	return out, inputFailure(ctx, err)
}
func (f Fingerprints) Coverage(ctx context.Context, source, root, producer string, support map[string]string) (map[string]string, mutants.InputFailure) {
	if failure := inputFailure(ctx, nil); failure != nil {
		return nil, failure
	}
	var out map[string]string
	var err error
	if spec := lang.Detect(source); spec != nil {
		switch spec.Name {
		case "go":
			out, err = GoCoverageInputs(ctx, f.Runtime, source, root, producer, support)
		case "python":
			out, err = PythonCoverageInputs(ctx, f.Runtime, source, root, producer, support)
		case "typescript":
			out, err = TypeScriptCoverageInputs(ctx, f.Runtime, source, root, producer, support)
		case "kotlin":
			out, err = KotlinCoverageInputs(ctx, f.Runtime, source, root, producer, support)
		}
	}
	return out, inputFailure(ctx, err)
}
func (f Fingerprints) Unsupported(ctx context.Context, language, source, root string) (string, mutants.InputFailure) {
	if failure := inputFailure(ctx, nil); failure != nil {
		return "", failure
	}
	var reason string
	var err error
	if language == "go" {
		reason, err = GoCoverageUnsupported(f.Runtime, source)
	} else {
		reason, err = LineCoverageUnsupported(ctx, language, source, root)
	}
	return reason, inputFailure(ctx, err)
}
