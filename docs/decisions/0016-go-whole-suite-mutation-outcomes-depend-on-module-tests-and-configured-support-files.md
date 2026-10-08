---
status: accepted
date: 2026-10-08
---

# Go whole-suite mutation outcomes depend on module tests and configured support files

## Context and Problem Statement

Issue #20 closes the whole-suite freshness gap that ADR-0003 accepted. Recommended first slice: for Go, every outcome recorded with all-tests or test-command depends on all _test.go files beneath the nearest Go module root, excluding nested modules, plus mutation.tests.support globs at the project root. Include build-tagged tests conservatively. Feature files and other custom command inputs must match support globs; arbitrary inputs cannot be inferred. Apply evidence to all recorded broad-scope outcomes, not just kills, retain original scope on reuse, and never refresh evidence for unjudged outcomes in a partial run. Broad-scope snapshots without this evidence are stale and require rerunning. Own and listed scopes retain their existing dependencies; other languages remain separate work. This makes any suite-test or support edit stale the broad-scope results, costing more reruns. Choose (a) this Go-first scope, (b) a wider project-root/all-language policy specified before implementation, or (c) keep ADR-0003 and defer #20.

Asked as q-24, about whole-suite-freshness.

## Considered Options

- Go-first recorded-scope freshness using module tests and configured support files
- Design project-root or all-language dependencies before implementation
- Keep importer-only freshness and defer issue #20

## Decision Outcome

Choose (a), the Go-first scope. For each Go outcome recorded with all-tests or test-command, freshness also depends on every _test.go file beneath the nearest Go module root, excluding nested modules, including build-tagged tests conservatively, and on project-root mutation.tests.support matches. Feature files and other custom-command inputs require explicit support globs. Record dependencies for all broad-scope outcomes, retain recorded scopes on reuse, and do not refresh unjudged outcomes in partial runs. Missing broad-scope evidence makes legacy results stale. Own and listed scopes keep their existing rules; other languages remain unchanged in this slice.

### Consequences

Importer hashes remain the baseline dependency for every outcome. Go all-tests and test-command outcomes additionally require module-test and configured-support evidence; any added, changed or deleted dependency makes those outcomes stale. Broad-scope legacy outcomes must rerun once to record that evidence. All outcomes, including survivors and excepted survivors, keep the dependencies of their recorded scope. A plain or partial run cannot bless results it did not rerun. Listed outcomes retain ADR-0013 support and covering-test rules; own outcomes and other languages retain importer-only baseline freshness. Conservative whole-suite evidence costs more reruns after suite edits. Commands with inputs outside the discovered module tests must name those inputs through support globs; external inputs are not inferred.

## More Information

Supersedes ADR-0003.
