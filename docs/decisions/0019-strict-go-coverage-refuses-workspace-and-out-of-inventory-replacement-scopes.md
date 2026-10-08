---
status: accepted
date: 2026-10-08
---

# Strict Go coverage refuses workspace and out-of-inventory replacement scopes

## Context and Problem Statement

Strict Go coverage fingerprints currently stop at the nearest module. Active go.work files and local replace targets outside that module can introduce source inputs beyond that inventory. Recommended bounded first slice: refuse to admit or reuse strict statement-coverage evidence when an active Go workspace is used or a local replacement target is outside the selected source module; strict run/check must report that unsupported scope and cannot pass from old cached evidence. GOWORK=off disables workspace use and remains supported. Local replacements whose targets are inside the same inventoried module stay supported; do not follow arbitrary external directories. Non-strict behavior remains unchanged. Add regression coverage and record multi-module provenance support as separate future work. Choose (a) this explicit refusal, or (b) expand #23 now to fingerprint active workspace files, their modules and out-of-module local replacement dependency trees.

Asked as q-27, about strict-go-coverage.

## Considered Options

- Refuse unsupported strict scopes without changing non-strict behavior
- Track workspace modules and out-of-module local dependency trees now

## Decision Outcome

Choose (a): refuse strict evidence for an active Go workspace or a local replacement target outside the inventoried source-module boundary, including excluded nested modules. Both strict run and strict check must report unsupported scope, and existing cached evidence cannot bypass the refusal. GOWORK=off remains supported; replacement inputs already inside the actual inventoried module remain supported. Do not follow arbitrary external dependency trees. Preserve non-strict behavior and record multi-module provenance as separate future work.

### Consequences

An active workspace or an out-of-inventory local replacement cannot be proved by nearest-module fingerprints and therefore prevents strict run/check success, even when an older cache looks fresh. GOWORK=off avoids workspace use. A lexically nested replacement with its own excluded go.mod is outside the inventory too. Replacement inputs already inside the inventoried module remain supported. Detection does not authorize following or reading arbitrary external source trees. Supporting these projects in strict mode requires a separate multi-module provenance design; non-strict commands continue to work as before.
