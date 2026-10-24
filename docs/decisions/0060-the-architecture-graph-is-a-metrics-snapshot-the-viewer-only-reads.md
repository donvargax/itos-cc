---
status: accepted
date: 2026-10-23
---

# The architecture graph is a .metrics snapshot the viewer only reads

## Context and Problem Statement

How does serve's architecture graph reach its own web app: serve builds it in process and the app asks over HTTP, as today, or itos-cc writes it as a .metrics snapshot the app only reads, as Uncle Bob's uml-viewer reads the files his tools write?

Asked as q-86, about graph-snapshot.

## Considered Options

The options are those the question names.

## Decision Outcome

A snapshot the app only reads (the person, 2026-10-24), so the UI code stays simple: itos-cc writes the graph, its tree, import edges and metrics, as .metrics/graph.json beside crap, dry, scrap and mutation's snapshots, and the web app reads .metrics/ and needs none of itos-cc's code. serve's move (T-102 to T-113) is re-cut against this when it is specified; serve-embedded-viewer stays as the stopgap until then.

### Consequences

itos-cc gains a command writing .metrics/graph.json (idea graph-snapshot), committed by the nightly with the other snapshots (T-190). The web app leaves itos-cc reading files alone, as uml-viewer does, so it carries no graph code. The deferred serve tasks T-102 to T-113, built around serve's HTTP adapter and its move onto kong, are re-cut against the file contract when serve's move is specified; serve and its embedded viewer (serve-embedded-viewer) stay meanwhile.
