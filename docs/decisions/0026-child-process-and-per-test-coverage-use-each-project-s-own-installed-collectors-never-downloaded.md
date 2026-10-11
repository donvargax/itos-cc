---
status: accepted
date: 2026-10-11
---

# Child-process and per-test coverage use each project's own installed collectors, never downloaded

## Context and Problem Statement

Language parity, coverage from tests that start another process (issue #14, Go only today): collectors are NODE_V8_COVERAGE plus c8, coverage.py subprocess patching plus coverage combine, and the JaCoCo agent plus jacococli. itos-cc never downloads tools. Recommend: use the project's own installed c8, coverage.py and JaCoCo (the Gradle or Maven cache), and report a missing collector as coverage.tool-missing only under --fail-uncovered, otherwise as today's silent no-op with a log line; per-test listed coverage then sets the same variables to <dir>/<id> through one generic harness variable ITOS_CC_TEST_COVERDIR, as Go programs already use. Alternative: refuse mutation.tests and integration coverage for non-Go projects until each language is done.

Asked as q-41.

## Considered Options

- Use the project's own c8, coverage.py and JaCoCo; a missing collector fails only under --fail-uncovered
- Refuse mutation.tests and integration coverage for non-Go projects until each language is done

## Decision Outcome

As recommended, 2026-10-10: use the project's own installed c8, coverage.py and JaCoCo (never downloaded); a missing collector is coverage.tool-missing only under --fail-uncovered, otherwise a log line; per-test listed coverage sets the same variables to <dir>/<id> through ITOS_CC_TEST_COVERDIR.

### Consequences

A missing collector is coverage.tool-missing under --fail-uncovered and a log line otherwise. Per-test listed coverage goes through ITOS_CC_TEST_COVERDIR in every language.
