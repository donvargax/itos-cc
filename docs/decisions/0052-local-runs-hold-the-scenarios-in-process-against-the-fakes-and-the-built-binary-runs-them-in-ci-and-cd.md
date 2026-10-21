---
status: accepted
date: 2026-10-20
---

# Local runs hold the scenarios in process against the fakes, and the built binary runs them in CI and CD

## Context and Problem Statement

Where do the scenarios run outside CI? (a, recommended) Locally the in-process driver alone, with the ports' fakes; the binary driver is opt-in by an environment variable that ci.yml and cd.yml set, so CI keeps its binary smoke set and range-named scenarios and CD the whole binary suite on every system, the @binary-only scenarios (real tools, real processes and signals, real builds and folders) running there only. (b) Locally the in-process driver plus the binary driver for the @binary-only and deferred scenarios alone, needing every real tool installed. (c) Locally both drivers in full, as today: itos.yaml's scenario lines, go test ./... and crap or mutation over this repository build and run the binary.

Asked as q-73.

## Considered Options

- Locally the in-process driver alone; the binary driver opt-in by an environment variable ci.yml and cd.yml set
- Locally the in-process driver plus the binary driver for the @binary-only and deferred scenarios
- Locally both drivers in full, as today

## Decision Outcome

(a), the person on 2026-10-21: local runs are in-memory end-to-end tests with fakes, as a FastAPI service's TestClient with dependency overrides; the real binary is CI's and CD's. Whether code only the @binary-only scenarios reach should gain contract or adapter tests that run locally is a separate item, measured before it is built.

### Consequences

Amends decision 44, whose two drivers both still run in CI and CD, and decision 48, whose tiers are unchanged: TestFeatures runs only where the opt-in variable is set, and logs that it ran nothing otherwise, so go test ./..., itos.yaml's scenario lines and crap or mutation over this repository run no binary locally. ci.yml and cd.yml set the variable on every job that runs TestFeatures, so nothing leaves CI or CD. The @binary-only scenarios run in CI (those the range names or the smoke set holds) and CD only. What only they reach is measured, and code nothing local exercises gains adapter or contract tests where that is cheaper than waiting for CD, never a fake standing in for the real tool a scenario is about.
