---
status: accepted
date: 2026-10-18
---

# A domain may carry struct tags used only for serialization, and holds no encoding logic

## Context and Problem Statement

Decision 47's review (A47-3): five domains (lang, duplication, teststructure, changerisk, coverage) carry json struct tags their slices encode directly, while docs/architecture.md says the domains know no JSON and docs/go.md holds only internal/mutants to no representation tags, so mutants alone pays with mirror structs and field-by-field copies in metricsdir, the mutation slice and config. Recommended: a domain may carry struct tags used only for serialization (json, yaml) on its own types, so adapters and slices encode its values with no mapper; it still imports no encoding package and holds no encoding or decoding logic, so legacy-key handling stays in its adapter. Alternatives: let mutants alone stay tag-free; or strip every tag and give each slice a mapper, which decision 47 rejects.

Asked as q-70, about adr-47-code-review.

## Considered Options

The options are those the question names.

## Decision Outcome

Allow them, mutants included (person, 2026-10-18): JSON tags in the domain are fine when they are only used for serialization. A domain may carry json and yaml struct tags on its own types; it imports no encoding package and holds no encoding or decoding logic, so a format's legacy keys and other wire rules stay in the adapter. Coverage's Gap keeps its one-line MarshalText, which returns the Gap's name for serialization only and imports nothing (the coordinator's reading).

### Consequences

None recorded.
