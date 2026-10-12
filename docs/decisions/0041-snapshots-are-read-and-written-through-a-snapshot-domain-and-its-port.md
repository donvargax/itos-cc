---
status: accepted
date: 2026-10-11
---

# Snapshots are read and written through a snapshot domain and its port

## Context and Problem Statement

Moving dry and scrap onto slices (S3): they read and write .metrics/*.json snapshots and name paths from the project root themselves, which a slice may not do. Where does snapshot I/O go? Recommend a new shared domain internal/snapshot with its own port (Root, Read, Write; sealed failures Unreadable, Unwritable, Cancelled carrying the library's text; a porttest fake), the pure part (format version, naming a path from a root) in the domain, and an adapter internal/metricsdir wrapping metrics and project, its classifier in cmd. Alternative: grow selection's Tree port with Recorded, Record and Root.

Asked as q-57.

## Considered Options

The options are those the question names.

## Decision Outcome

A new shared domain internal/snapshot with its own port and an adapter internal/metricsdir, as recommended (the person, 2026-10-11). crap, mutation and serve take it as they move.

### Consequences

dry and scrap move first; crap, mutation and serve keep the metrics library until each moves, then use the port. The JSON bytes and atomic writes stay the metrics library's, behind the adapter.
