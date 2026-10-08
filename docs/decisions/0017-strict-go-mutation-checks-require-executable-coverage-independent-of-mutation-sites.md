---
status: accepted
date: 2026-10-08
---

# Strict Go mutation checks require executable coverage independent of mutation sites

## Context and Problem Statement

Issue #23 makes --fail-uncovered prove execution independently of mutation sites. Recommended first slice: Go only, every judged executable function including functions with zero mutation sites, using uncovered executable blocks from measured statement coverage rather than syntax-tree statement enumeration. Persist independent coverage evidence so mutation check gives the same verdict without running tests, and reuse it only while its source and test/support inputs remain fresh. Report uncovered blocks as mutation.uncovered-statement with file, function and line, separate from mutant counts. Strict mode must not pass when coverage evidence is unavailable or missing; --fail-uncovered with --no-coverage is a usage conflict. Non-strict mutation behavior remains unchanged. Do not add coverage exceptions or the new mutators from #24 in this slice. Choose (a) this Go-first strict policy, (b) limit statement checks to functions that already have sites, leaving zero-site functions outside the gate, or (c) design all-language coverage requirements before implementation.

Asked as q-25, about uncovered-statements.

## Considered Options

- Go-first strict statement coverage for every judged executable function
- Only check statements in functions that already have mutation sites
- Design coverage requirements for every language before implementation

## Decision Outcome

Choose (a), Go-first strict coverage. With --fail-uncovered, every judged executable Go function needs fresh measured coverage evidence, even if it has no mutation sites. Report uncovered executable coverage blocks as mutation.uncovered-statement, separate from mutant counts, with file, function and line. Persist evidence for cached check/run parity and reuse only with fresh source and test/support inputs. Missing or unavailable coverage cannot pass strict mode; --no-coverage conflicts with --fail-uncovered. Non-strict behavior and other languages remain unchanged. Do not add coverage exceptions or new mutators in this slice.

### Consequences

Strict Go runs and cached checks judge zero-site functions too. Independent coverage evidence distinguishes measured-complete, measured-uncovered and unavailable or missing evidence. Old snapshots may still serve non-strict checks, but strict checks require fresh coverage evidence and may require a new run. Statement findings do not alter mutant counts or invent mutation replacements. Uncovered executable blocks are judged at the coverage format precision, not as a promise of syntax-tree statement enumeration. --no-coverage cannot be used with --fail-uncovered. Non-strict commands and other languages keep their existing behavior. Coverage exceptions, test-support exclusions, other-OS exclusions and additional mutation operators remain separate work.
