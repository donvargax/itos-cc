---
status: accepted
date: 2026-11-09
---

# The graph snapshot is one project per file, its repository named by its git remote

## Context and Problem Statement

graph-snapshot (q-86/ADR-0060: itos-cc writes the architecture graph as .metrics/graph.json, the viewer only reads files). Proposed items, cheapest first: 1 move graph/ into internal/slices/graph (refactor, no behaviour change; ends the graph/server architecture-check exemptions); 2 itos-cc graph writes .metrics/graph.json (about 1.9 MB here, sorted, deterministic), viewer and serve unchanged, reviving the @wip architecture_graph scenarios against the command; 3 the nightly commits it after crap and dry; 4 the viewer reads the file and serve serves files only (breaks serve's HTTP API, drops --interval); 5 serve's end state. Confirm the recommended set or change any: D1 mutation freshness reaches graph through a port graph owns, which cmd binds to a function the mutation slice exports (check's rule stays mutation's, no exemption). D2 command name graph. D3 whole project, no path flags, ignores exclude. D4 one project per file; several repos merged later. D5 --json prints a summary (counts), not the graph. D6 "version" is the format version 1, not a rebuild counter. D7 the repo node is named by its folder, as today (a local clone in code-quality rewrites ids; alternative: the git remote's repository name). D8 serve keeps serving the source files graph.json lists for the viewer's source panel. D9 the viewer polls the file every few seconds by Last-Modified. D10 same nightly job. D11 serve ends as a static file server on kong (alternative: removed in favour of the separate web app).

Asked as q-95, about T-208.

## Considered Options

The options are those the question names.

## Decision Outcome

Person 2026-10-24: the recommended set (D1 port graph owns, bound by cmd to a mutation-slice export; D2 graph; D3 whole project, ignores exclude; D4 one project per file; D5 --json summary; D6 format version 1; D8 serve serves the listed source files; D9 viewer polls by Last-Modified; D10 same nightly job) except D7: the repository node is named by the git remote's repository name, stable between a local clone and CI. D11: the end state is a static file server on kong, but that work (item 5) is parked as the lowest priority: done only when nothing else remains and the web app (the person's and a friend's, its own project) has not yet moved out. Items 1 to 4 proceed in queue order.

### Consequences

Builds on ADR-0060. The command is itos-cc graph, whole project, ignoring exclude. The node's name is stable between a local clone and CI. serve ends as a static file server, that last step parked as the lowest priority.
