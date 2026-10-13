---
status: accepted
date: 2026-10-12
---

# Adapters may import the domain they serve; nothing outside the domain is imported into it

## Context and Problem Statement

Decision 29 lets an adapter import only port packages, never the domain it serves, and every adapter rule in .golangci.yml holds that. Decision 46's coverage port gives a Report, a domain type, so its adapter would import internal/coverage. Keep 29 whole (the port speaks raw results in port types and the domain parses and assembles them, an extra task reshaping the runner), or let an adapter import the domain it serves?

Asked as q-63, about template-adoption.

## Considered Options

- Adapters and ports may use the domain's types, the dependency pointing only inward
- Adapters import only port packages and the domain parses raw results (decision 29 as written)

## Decision Outcome

The person (2026-10-13): let the adapter import it. That rule was too strict: the domain leaking outward is fine, and preferred, since it avoids mappers, and parsing raw values in the domain would be worse. Nothing may leak into the domain.

### Consequences

Amends decision 29's adapter rule; the rest of 29 stands. An adapter, and a port package, may import the domain it serves and build or return its types, so no mapper translates between them; the domain still imports no adapter, slice, cmd, UI or I/O package, and the domain and domain-io rules are unchanged. Adapter depguard rules allow their own domain package as each is touched (internal/coveragetools first, S5); existing ports need not change. docs/go.md's boundary list says so.
