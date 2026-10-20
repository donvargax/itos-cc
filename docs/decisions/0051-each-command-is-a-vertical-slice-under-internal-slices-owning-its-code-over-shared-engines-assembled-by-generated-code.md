---
status: accepted
date: 2026-10-20
---

# Each command is a vertical slice under internal/slices owning its code, over shared engines, assembled by generated code

## Context and Problem Statement

T-86 (feature-owned vertical slices, so an ordinary feature change stays in one folder and agents can work on different features at once) needs ADR-0029's layout revised. Where do feature folders live, where do scenarios and steps live, how are commands assembled without central hand edits, and how is path ownership recorded? Recommended: internal/slices/<name>/ with engines left where they are; .feature files and steps stay in features/ with their hand lists generated; a stdlib generator writing a committed cmd/itos-cc/commands_gen.go, checked current in CI; ownership a docs convention first.

Asked as q-72, about T-86.

## Considered Options

- Slices under internal/slices/<name>/
- Slices kept at internal/<name>/
- Slices under internal/features/<name>/
- Scenarios and steps stay in features/, their lists generated
- Scenarios and steps move beside each slice
- A stdlib generator writes the assembly, checked in CI
- One hand-kept list in cmd
- Ownership a docs convention first
- Ownership an itos per-task paths field now

## Decision Outcome

The person's calls on 2026-10-20, each the recommendation. Feature folders live under internal/slices/<name>/: units, dry, scrap, crap, mutation (run, check, list, sample and except as one) and version, each owning its CLI and application code, its own pure domain, consumer-owned ports, adapters, fakes and tests in subpackages that keep the domain, port, porttest and adapter separation, so a slice's domain still imports no adapter, UI, kong or I/O. Engines stay where they are behind stable APIs: lang, selection, coverage, snapshot, project and git history, settings, process ownership, the UI in internal/cli and an import-resolution engine lifted out of graph. A slice imports no other slice, checked by tools/bin/architecture-check. The .feature files and their step code stay in features/, one harness; the hand lists in features/steps.go and the in-process driver's adapters are generated. Commands are assembled by a stdlib go/parser generator under tools/ that reads each slice's declaration and writes a committed cmd/itos-cc/commands_gen.go, registering slices through the kong fork's DynamicCommand in name order and binding ports through providers so itos-cc.yaml is still read only when a command runs; a CI static step checks the file is current. Which paths each slice owns is a docs convention first, engines, generated files and assembly needing a task of their own; an itos feature only if the parallel-agents trial after T-86 shows agents straying. The coordinator's calls: slices take their ports as methods on cmd's adapters; duplication's Fingerprint and Jaccard move into lang, used by dry and scrap; graph keeps a copy of changerisk's four-line Score; serve and graph stay legacy outside the slices; cmd's mutation integration tests stay in cmd as assembly tests that mutation owns by path.

### Consequences

Supersedes ADR-0029's layout; its arrows stay (a domain imports no adapter, UI, kong or I/O package; an adapter imports only ports; kong only in cmd and internal/cli; errors sealed sets the UI turns into exit codes) and gain a sibling rule: no package under internal/slices/X imports internal/slices/Y. ADR-0037's moves continue, a package moving in the task that makes it part of a slice, its .metrics/mutate snapshots and exceptions with it. ADR-0047 and decision 50 stand. T-86 lands as small tasks, each green alone: generic dispatch, the sibling rule, units first, then the generator, dry, scrap, the exclude setting out of mutants.Config, crap, version, mutation in pieces, the import-resolution engine, generated scenario registration, and one refactor-proof at the end. Help, errors, exit codes, lazy config for help and a fresh parser per run are unchanged; no DI framework, init registry or new dependency. docs/go.md, docs/architecture.md and README's layout describe it as each task lands.

## More Information

Supersedes ADR-0029.
