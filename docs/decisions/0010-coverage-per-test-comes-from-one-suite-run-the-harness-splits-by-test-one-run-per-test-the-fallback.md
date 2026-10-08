---
status: accepted
date: 2026-10-07
---

# Coverage per test comes from one suite run the harness splits by test, one run per test the fallback

## Context and Problem Statement

test-attribution: how is coverage measured per test (itos has about 505 scenarios)? (a, recommended) One suite run: itos-cc sets a per-test coverage base and the harness gives each test's child processes GOCOVERDIR=<base>/<test-id>; itos-cc falls back to one run per test when the harness writes nothing there. (b) One run per test, with no harness cooperation.

Asked as q-18, about test-attribution.

## Considered Options

The options are those the question names.

## Decision Outcome

(a) One run, split by the harness into <base>/<test-id>; one run per test as the fallback.

### Consequences

itos-cc sets ITOS_CC_TEST_COVERDIR; a harness that gives each test's processes GOCOVERDIR=<that directory>/<test ID> has its coverage split in one run, and otherwise each test runs alone with a GOCOVERDIR of its own. Only Go programs report coverage per test so far.
