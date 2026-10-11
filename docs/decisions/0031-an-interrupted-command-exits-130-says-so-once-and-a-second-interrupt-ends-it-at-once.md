---
status: accepted
date: 2026-10-10
---

# An interrupted command exits 130, says so once, and a second interrupt ends it at once

## Context and Problem Statement

What does an interrupted command do? No rule in docs/CLI.md, itos's or itos-template's says. Recommended: exit 130 (128 plus SIGINT, what shells report for Ctrl-C) on every system, Windows too; one stderr line saying it was interrupted, and with --json the usual failure object with rule INTERRUPTED; the first Ctrl-C cancels the context and the command stops promptly, a second ends the process at once, skipping cleanup (clig.dev #robustness). Or: exit 130 and print nothing. Or: reuse 75, a temporary failure.

Asked as q-34, about cli-ctrl-c.

## Considered Options

- Exit 130, one stderr line or the INTERRUPTED object, a second interrupt ends at once
- Exit 130 and print nothing
- Reuse 75, a temporary failure

## Decision Outcome

The recommendation, the person's call on 2026-10-10. Exit 130 on every system; one stderr line saying the command was interrupted, and with --json the failure object with rule INTERRUPTED; the first Ctrl-C cancels the context, a second ends the process at once with 130. The double Ctrl-C is shared machinery in internal/cli, a function taking the interrupts as a channel that cmd feeds from signal.Notify, unit-tested with fake interrupts on all three systems; no demo command is added to show cleanup.

### Consequences

docs/CLI.md gains exit code 130 in its table and rules for interrupts and for cleanup, a command writing a temporary file and renaming it so an interrupt never leaves half a file. internal/cli holds the interrupt handling every command shares, cmd/itos-cc feeding it signal.Notify, the one system-specific line; decision 30's context is what it cancels, and the adapters act on a done context. Windows receives the interrupt in scenarios through CTRL_BREAK_EVENT on a new process group, falling back to Linux and macOS if that proves unstable in CI.
