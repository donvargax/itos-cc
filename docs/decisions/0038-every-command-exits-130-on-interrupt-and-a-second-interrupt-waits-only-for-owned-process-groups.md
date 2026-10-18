---
status: accepted
date: 2026-10-11
---

# Every command exits 130 on interrupt, and a second interrupt waits only for owned process groups

## Context and Problem Statement

Exit 130 on Ctrl-C changes serve (0 → 130) and mutation run --count (75, count.interrupted → 130, interrupted): adopt it fully, keep --count's 75, or defer?

Asked as q-51.

## Considered Options

The options are those the question names.

## Decision Outcome

Adopt it fully, in a minor release (answered 2026-10-11): every command exits 130 with rule interrupted, serve and mutation run --count included, called out in Upgrading. A second Ctrl-C ends the process at once, except while the process groups itos-cc owns are being killed (ADR-0021). q-37 settles as mutation.cleanup-failed, exit 70.

### Consequences

Keeps ADR-0031's exit 130, single report line and internal/cli machinery, with itos-cc's lower-case rule interrupted. serve (was 0) and mutation run --count (was 75, count.interrupted) change: a minor release, called out in Upgrading. A second Ctrl-C ends the process at once except while the process groups itos-cc owns are being killed (ADR-0021), which finish first so none outlives it. A cleanup that fails is mutation.cleanup-failed with the internal exit (q-37).

## More Information

Supersedes ADR-0031.

The internal exit is 100 since decision 36 (square/exit, answered the same day), so q-37's
mutation.cleanup-failed exits 100, not 70, as slice-7 and slice-8 ship it (ID-MUT-250) and
docs/CLI.md says.
