---
status: accepted
date: 2026-10-11
---

# Strict line coverage for TypeScript, Python and Kotlin counts partly executed lines as covered and refuses out-of-root scopes

## Context and Problem Statement

Language parity, strict statement coverage (ADR-0017/0018/0019) for TypeScript, Python and Kotlin. Evidence comes at each format's line precision (LCOV DA lines from Vitest v8 and coverage.py, JaCoCo/Kover XML lines), which ADR-0017 already allows. Open: (1) is a JaCoCo line with both missed and covered instructions uncovered? Recommend covered, as Go judges whole blocks executed or not, and branches stay out of scope as Go has none; alternative: uncovered. (2) Which scopes to refuse, as ADR-0019 refuses Go workspaces: recommend refusing npm/pnpm/yarn workspaces and file:/link:/workspace: dependencies outside the root, Python path or editable dependencies outside the root, Gradle includeBuild and Maven parent relativePath or modules outside the root; alternative: follow them.

Asked as q-40.

## Considered Options

The options are those the question names.

## Decision Outcome

As recommended, 2026-10-10: (1) a JaCoCo line with both missed and covered instructions is covered, branches out of scope; (2) refuse npm/pnpm/yarn workspaces and file:/link:/workspace: dependencies outside the root, Python path or editable dependencies outside the root, Gradle includeBuild and Maven parent relativePath or modules outside the root.

### Consequences

Branches stay out of scope as in Go. Workspaces and dependencies outside the root (npm/pnpm/yarn workspaces, file:/link:/workspace: deps, Python path or editable deps, Gradle includeBuild, Maven parent relativePath or modules) are refused in strict mode, as ADR-0019 refuses Go workspaces.
