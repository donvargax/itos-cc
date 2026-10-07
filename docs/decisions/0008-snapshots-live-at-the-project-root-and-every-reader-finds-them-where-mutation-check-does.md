---
status: accepted
date: 2026-10-07
---

# Snapshots live at the project root, and every reader finds them where mutation check does

## Context and Problem Statement

How should the graph find a file's mutation snapshot, given .metrics is relative to the working directory and the graph matches snapshots by path suffix (ID-GRAPH-26)? (1) The graph looks a file's snapshot up exactly as mutation check does, dropping the suffix match for mutation results. (2) Keep the leniency and document it. (3) Anchor .metrics to the project root for every command, snapshots naming files from the root, so the suffix match has no reason to exist.

Asked as q-16, about graph-snapshot-lookup.

## Considered Options

The options are those the question names.

## Decision Outcome

(1) and (3) together: anchor .metrics at the project root for every command (snapshots-at-root, first), and the graph looks snapshots up as mutation check does (graph-snapshot-lookup).

### Consequences

Every command keeps .metrics/ and itos-cc.yaml at the git top level (the working directory outside git), and every path a snapshot records is relative to that root; paths on the command line and in output stay relative to the working directory. mutation check, run, sample, except and the graph share one lookup, so a snapshot at another path or naming another file counts as none. A .metrics/ left in a subdirectory by an earlier version is ignored.
