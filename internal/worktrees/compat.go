// Package worktrees runs mutation commands in private copies, through the
// process-ownership engine, internal/processes, which owns their processes
// until all output waiters and ordinary descendants have joined.
// Its inward boundary is mutants.Workers, beside the mutation domain values.
package worktrees

import "github.com/donvargax/itos-cc/internal/slices/mutation/domain"

type Command = mutants.Command
