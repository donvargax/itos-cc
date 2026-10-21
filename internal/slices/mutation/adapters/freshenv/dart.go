package freshenv

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"slices"
	"strings"

	covrun "github.com/donvargax/itos-cc/coverage"
	"github.com/donvargax/itos-cc/internal/lang"
	mutants "github.com/donvargax/itos-cc/internal/slices/mutation/domain"
)

// readyDart readies each Dart build root of sources, files of the frozen
// export, once: git ignores .dart_tool, so the export holds no package
// config, and coverage plans none, until pub resolves it. What runs is the
// Ready of the command the toolchain chooses for the file, flutter pub get
// --offline, in that command's Dir, so a pub workspace's member resolves
// its workspace at the root the export holds. It resolves the commit's
// pubspec.lock from the pub cache alone, and in the export alone
// --enforce-lockfile makes a lock that does not match its pubspec fail
// rather than resolve afresh, as offlineArgs makes Gradle run --offline.
// Each runs with CommandEnvironment through execute and is returned for
// the preparation's record; a failure names the command and its output.
func (e Environment) readyDart(ctx context.Context, sources []string, log io.Writer, execute covrun.CommandExecutor) ([]covrun.CommandExecution, error) {
	var calls []covrun.CommandExecution
	readied := map[string]bool{}
	for _, path := range sources {
		if spec := lang.Detect(path); spec == nil || spec.Name != "dart" {
			continue
		}
		if err := ctx.Err(); err != nil {
			return calls, err
		}
		c, failure := mutants.ObserveCommand(ctx, e.Toolchain, mutants.CommandRequest{Path: path})
		if failure != nil {
			return calls, fmt.Errorf("choose the command of %s: %w", path, failure)
		}
		if len(c.Ready) == 0 || readied[c.Dir] {
			continue
		}
		readied[c.Dir] = true
		args := slices.Clone(c.Ready)
		if !slices.Contains(args, "--enforce-lockfile") {
			args = append(args, "--enforce-lockfile")
		}
		cmd := exec.CommandContext(ctx, args[0], args[1:]...) //nolint:gosec // the selected package's own toolchain resolving the frozen export from the pub cache
		cmd.Dir = c.Dir
		CommandEnvironment(cmd)
		var output bytes.Buffer
		cmd.Stdout, cmd.Stderr = &output, &output
		_, _ = fmt.Fprintf(log, "itos-cc: ready %s$ %s\n", c.Dir, strings.Join(args, " "))
		err := execute(ctx, cmd)
		calls = append(calls, covrun.CommandExecution{Args: args, Dir: c.Dir, Err: err})
		if err != nil {
			return calls, fmt.Errorf("ready the frozen Dart package with %s: %w\n%s", strings.Join(args, " "), err, strings.TrimSpace(output.String()))
		}
	}
	return calls, nil
}
