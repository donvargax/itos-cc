---
status: accepted
date: 2026-10-24
---

# Every package but cmd lives under internal/, the gate naming the few that may cross

## Context and Problem Statement

How do config, coverage, metrics and project move under internal/, and which packages outside internal/ may still import it once architecture-check refuses the rest?

Asked as q-89, about top-level-libraries-internal.

## Considered Options

The options are those the question names.

## Decision Outcome

The person, 2026-10-24, each as recommended: project becomes internal/project, an adapter-side engine; metrics folds into internal/metricsdir, the snapshot adapter that wraps it (ADR-0041), coveragetools calling metricsdir.Dir(); coverage's runner becomes the engine internal/covrun, never under internal/coverage/; config becomes internal/slices/mutation/adapters/settings, the adapter of mutation's settings port (ADR-0047), ending the shared config importing a slice's domain, ADR-0051's settings engine amended and its ownership mutation's. architecture-check refuses any package outside internal/ but cmd/ importing internal/, the gate landing first with today's offenders named and each move deleting its row; permanent named rows: features may import internal/project alone (the harness asks discovery's own rules, never a copy), tools/bin/release-notes and release-version internal/release alone (the template's layout); graph and server stay named until graph-snapshot (ADR-0060). A move whose pushes change qualifiers may draw a fresh mutation site and kills what it meets (ADR-0059's survivor rule); ADR-0040's pause does not apply. Each move git-mvs its .metrics/mutate snapshots with its files; one refactor-proof.yml run at the end.

### Consequences

Six tasks: the gate with named exemptions first, then project, metrics, coverage and config, each removing its exemption, and one refactor-proof.yml run. Permanent rows: features to internal/project, tools/bin/release-* to internal/release; graph and server until graph-snapshot. Amends ADR-0051's list of engines: settings is mutation's adapter now.
