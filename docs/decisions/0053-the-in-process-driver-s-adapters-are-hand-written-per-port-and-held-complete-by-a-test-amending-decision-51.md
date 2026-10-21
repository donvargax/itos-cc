---
status: accepted
date: 2026-10-21
---

# The in-process driver's adapters are hand-written per port and held complete by a test, amending decision 51

## Context and Problem Statement

ADR-0051 says the in-process driver's adapters are generated. The plan for slices-scenario-registration found they change per port, not per slice: a slice declaring only ports cmd's adapters already answer adds nothing, and which fake answers each port is knowledge no slice declaration holds. Should they be generated? (a, recommended) No: they stay hand-written, moved into an inProcessAdapters provider beside diskAdapters, and a test fails when any adapters field is unbound on disk or in process, so a new port cannot be forgotten in either; recorded as an amendment to ADR-0051. (b) Yes, as ADR-0051 says: every port gets a named in-process constructor by convention and a generator writes adapters, diskAdapters and the in-process literal, rewriting 14 adapters literals across 5 files. (c) Neither now: leave memory.Run as it is and revisit when a slice adds a port.

Asked as q-79, about T-165.

## Considered Options

- Hand-written in an inProcessAdapters provider, with a test holding every port bound on disk and in process
- Generated from a naming convention for each port's in-process constructor
- Left as memory.Run builds them until a slice adds a port

## Decision Outcome

(a), the person on 2026-10-21: the in-process driver's adapters stay hand-written per port, moved into an inProcessAdapters provider beside diskAdapters, and TestEveryPortIsBoundOnDiskAndInProcess fails when any adapters field is unbound on disk or in process; ADR-0051 amended.

### Consequences

Amends decision 51, whose in-process adapters were to be generated: they change per port, not per slice, and which fake answers a port is knowledge no slice declaration holds. T-165 moves them into inProcessAdapters beside diskAdapters and adds TestEveryPortIsBoundOnDiskAndInProcess; a new port goes into adapters, diskAdapters and inProcessAdapters, the test failing until all three bind it. Everything else in decision 51 stands, the generated command assembly and step registration included.
