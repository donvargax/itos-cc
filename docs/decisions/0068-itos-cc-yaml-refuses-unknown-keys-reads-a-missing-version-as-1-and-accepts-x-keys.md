---
status: accepted
date: 2026-11-09
---

# itos-cc.yaml refuses unknown keys, reads a missing version as 1 and accepts x- keys

## Context and Problem Statement

itos-cc.yaml strictness (raised 2026-10-24 after q-94, from a side agent's note and the robustness principle): should unknown keys be refused, should a missing version be refused, and is there an escape hatch?

Asked as q-98, about slice-53.

## Considered Options

The options are those the question names.

## Decision Outcome

Person 2026-10-24: unknown keys stay refused at every level (slice-52). A missing version reads as version 1, permanently; only a version newer than itos-cc reads is refused, with a fix to upgrade (slice-53 changes from refusing it). Keys starting with x- are extension keys, accepted and ignored at any mapping level, as Docker Compose and OpenAPI do, their values still held to rule 1 (no anchors, aliases, merge keys or tags); a new slice before slice-52 is released.

### Consequences

Unknown keys are refused at every level. A version newer than itos-cc reads is refused with a fix to upgrade. x- keys are accepted and ignored at any mapping level, their values still held to rule 1. ADR-0063 later keeps the file at version 1 until itos-cc 1.0, so a new key does not move the version before then.
