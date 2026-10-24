---
status: accepted
date: 2026-10-23
---

# Flutter tests count in phases, unit and widget tests under test/ first

## Context and Problem Statement

Which Flutter tests count, and how is coverage measured? (a, recommended) Unit and widget tests under test/ (headless, flutter_tester); integration_test/ and test_driver/ never count, as they need a device. Line coverage from flutter test's LCOV, a lib/ file the report never names counting as 0%; coverage attributed per test file, not per test name (Flutter's 2-5 s start per run makes per-name too slow); no coverage of processes tests start (no GOCOVERDIR analogue), as decisions 26 and 27 allow. (b) As (a) plus per-test-name coverage, at one flutter test start per test. (c) As (a) plus integration_test/ on a device or emulator where one is present.

Asked as q-76, about dart-support.

## Considered Options

The options are those the question names.

## Decision Outcome

In phases, the person on 2026-10-21: phase 1 is (a), unit and widget tests under test/, line coverage from flutter test's LCOV with a lib/ file it never names at 0%, attributed per test file, no coverage of processes tests start. Phase 2 adds per-test-name coverage (b), and phase 3 integration_test/ on a device or emulator where one is present (c); each later phase starts only after the person has used the one before and decides it is needed, as an idea of its own until then.

### Consequences

Phase 1: line coverage from flutter test's LCOV, a lib/ file it never names at 0%, attributed per test file. Phase 2 adds per-test-name coverage, phase 3 integration_test/ on a device or emulator; each starts only once the person agrees.
