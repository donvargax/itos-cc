---
status: accepted
date: 2026-10-12
---

# Scenarios run in process against the ports' fakes and against the built binary

## Context and Problem Statement

Scenarios through two drivers: the godog harness runs each feature file against the built binary (decision 39), slow under mutation (every mutant rebuilds and reruns it). Recommend: the same scenarios also run in process, the steps calling cmd's run(ctx, args) with a UI writing to buffers and the ports' porttest fakes, Given steps filling the fakes; the binary driver stays and proves main's wiring, the real adapters, exit codes and signals; both run in CI; a step that can only hold in one driver is tagged, never copied; each port's porttest holds one contract suite run against both its fake and its adapter, so a fake cannot drift from what the adapter does; pilot on units, dry and scrap after S4, then each command's move adds its file. Alternatives: binary only, as now (slow mutation, every kill through a process); in process only (fast, but nothing proves the wiring or the real adapters).

Asked as q-60, about scenario-two-drivers.

## Considered Options

- Two drivers, in process with fakes and the binary, with contract suites for every port
- The binary only, as decision 39 has it
- In process only

## Decision Outcome

As recommended (the person, 2026-10-12): the same tests as fast tests with fakes and as real tests, the best of both worlds.

### Consequences

Every port's porttest exports a contract suite its fake and its adapter both pass (port-contract-suites). The godog harness has two drivers, in process with fakes and the built binary, both in CI, a scenario running under both unless tagged for one (scenario-two-drivers, after S4). Each later command move adds its feature file to the in-process driver. Mutation kills domain and slice mutants in process; adapters are killed by their own and the contract tests. Decision 39 stands: the binary driver is the one it names.

## More Information

Amended by ADR-0052: both drivers still run in CI and CD, but locally the scenarios run in process against the fakes alone; TestFeatures runs only where ITOS_CC_FEATURES_BINARY_DRIVER is 1 or ITOS_CC_FEATURES_BINARY names a binary, as every workflow sets it.
