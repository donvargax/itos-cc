---
status: accepted
date: 2026-10-25
---

# Every Did-you-mean follows kong's rule from the fork, which the settings adapter may import

## Context and Problem Statement

shared-edit-distance: whose "Did you mean" rule, and may the settings adapter use kong?

Asked as q-99, about slice-60.

## Considered Options

- kong's rule from the fork, everywhere (chosen)
- one leaf package of itos-cc's own rule, kong's copy left alone

## Decision Outcome

Person 2026-10-25: kong's rule, which is friendlier: every candidate that starts with what was typed or is within two edits, several named as "Did you mean one of A, B?". The fork exports it (a public function over the candidates, kong's own closestCandidates and levenshtein), tagged as a new -itos version; itos-cc uses it for flags, commands, serve's hand parser and itos-cc.yaml keys, deleting internal/cli's distance and the settings adapter's edits. The architecture rules let the settings adapter import the fork for it (kong otherwise still stays in cmd and internal/cli).

### Consequences

The fork donvargax/kong exports the rule as a public function, tagged v1.16.2-itos.2; per q-100 it adds Closest's stem test after kong's rule finds nothing, kong's own closestCandidates untouched. itos-cc deletes internal/cli's distance and the settings adapter's edits. Decision 29's kong boundary gains one exception: the settings adapter (internal/slices/mutation/adapters/settings) may import the fork for this function; kong otherwise stays in cmd and internal/cli, and depguard's kong rule names the exception. Built in slice-60.
