---
status: accepted
date: 2026-10-23
---

# A mutant that does not compile counts as killed in every language

## Context and Problem Statement

How is a Dart mutant that does not compile reported? Today a mutant's outcome is killed, survived, timeout or uncovered, and a Go or Kotlin mutant that fails to build counts as killed (its tests fail). flutter test also exits 1 for a compile error, but its JSON reporter tells one apart (an error on the 'loading' test). slice-29 kept Dart's sites to swaps that compile whatever the types, so such mutants should be rare (+ on a String, say). (a, recommended) As Go and Kotlin today: killed, the same rule for every compiled language, nothing new in the snapshot; an idea records a not-compiled status for Go, Kotlin and Dart together, decided later with numbers from real Flutter runs. (b) A new outcome, not-compiled, for Dart now, left out of the score: truthful for Dart, but Go and Kotlin keep counting theirs as killed until they follow, and the snapshot, check, counts and reports all learn it. (c) A new outcome for Go, Kotlin and Dart at once, a cross-language item before Dart's mutation commands.

Asked as q-83, about dart-support.

## Considered Options

The options are those the question names.

## Decision Outcome

(a), the person on 2026-10-21: a Dart mutant that does not compile counts as killed, as Go's and Kotlin's do today; nothing new in the snapshot. A not-compiled outcome for Go, Kotlin, Dart and any later language whose mutants can fail to build is discussed and built later, together (idea mutant-not-compiled).

### Consequences

Dart follows Go and Kotlin; nothing new in the snapshot. A not-compiled outcome for every language whose mutants can fail to build is built later, together (idea mutant-not-compiled).
