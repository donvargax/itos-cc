package mutate

import (
	"context"

	"github.com/donvargax/itos-cc/internal/testinputs"
	"github.com/donvargax/itos-cc/project"
)

// Direct legacy callers retain actual disk observations until T-92/T-95.
// Cmd uses its bound Fingerprints; these wrappers never construct fake facts.
func TestHashes(tests []string) (map[string]string, error) {
	return testinputs.TestHashes(project.Root(), tests)
}
func TestHashesUnder(root string, tests []string) (map[string]string, error) {
	return testinputs.TestHashes(root, tests)
}
func SupportHashes(root string, globs []string) (map[string]string, error) {
	return testinputs.SupportHashes(root, globs)
}
func SuiteTestHashes(ctx context.Context, source, root string) (map[string]string, error) {
	return testinputs.SuiteTestHashes(ctx, source, root)
}
func GoModuleTestHashes(source, root string) (map[string]string, error) {
	return testinputs.GoModuleTestHashes(source, root)
}
func GoCoverageInputs(source, root, producer string, support map[string]string) (map[string]string, error) {
	return testinputs.GoCoverageInputs(testinputs.CurrentRuntime(), source, root, producer, support)
}
func PythonCoverageInputs(ctx context.Context, source, root, producer string, support map[string]string) (map[string]string, error) {
	return testinputs.PythonCoverageInputs(ctx, testinputs.CurrentRuntime(), source, root, producer, support)
}
func TypeScriptCoverageInputs(ctx context.Context, source, root, producer string, support map[string]string) (map[string]string, error) {
	return testinputs.TypeScriptCoverageInputs(ctx, testinputs.CurrentRuntime(), source, root, producer, support)
}
func KotlinCoverageInputs(ctx context.Context, source, root, producer string, support map[string]string) (map[string]string, error) {
	return testinputs.KotlinCoverageInputs(ctx, testinputs.CurrentRuntime(), source, root, producer, support)
}
func KotlinBuildRoot(ctx context.Context, source string) string {
	return testinputs.KotlinBuildRoot(ctx, source)
}
func GoCoverageUnsupported(source string) (string, error) {
	return testinputs.GoCoverageUnsupported(testinputs.CurrentRuntime(), source)
}
func LineCoverageUnsupported(ctx context.Context, language, source, root string) (string, error) {
	return testinputs.LineCoverageUnsupported(ctx, language, source, root)
}
