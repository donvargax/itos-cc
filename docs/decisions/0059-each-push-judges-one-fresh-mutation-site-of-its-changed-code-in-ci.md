---
status: accepted
date: 2026-10-23
---

# Each push judges one fresh mutation site of its changed code in CI

## Context and Problem Statement

How does p1-itos-proof-code's code proof judge a push, given no one records mutation results for a push's changed functions (AGENTS.md bars agents from mutating locally and the nightly records once a day), so a check proof reading recorded results (mutation check --since {base}) would fail every Go push?

Asked as q-85, about T-187.

## Considered Options

The options are those the question names.

## Decision Outcome

A fresh site in CI (the person, 2026-10-23), as itos's own proof does: proof.code.check runs itos-cc mutation run --count 1 --since {base} --seed {base} --fail-uncovered=lines --json, judging one fresh site of the range's changed code in every push's ci run, and cd.yml runs itos-cc mutation sample --count 10 on every source push. AGENTS.md's note is amended: mutation runs in a push's CI this way, still never locally. Amends the 2026-10-18 plan, whose check proof read recorded results.

### Consequences

proof.code runs mutation run --count 1 --since {base} --seed {base} --fail-uncovered=lines --json in ci.yml's and cd.yml's ci jobs, and cd.yml samples ten recorded results on every source push. No mutation run on an agent's machine. A survivor the proof finds in a function the push changed is killed in that push; an equivalent one is excepted with its reason, the coordinator approving and the person reviewing the list weekly (2026-10-23). mutation.tests.list names only the scenarios the in-process run executes, since a counted run fails closed on a listed test with no data (2026-10-24).
