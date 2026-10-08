---
status: accepted
date: 2026-10-07
---

# An exception whose file is renamed follows it and fails once as moved

## Context and Problem Statement

exceptions-deleted-file: what happens to an exception whose file is renamed, and do --since runs judge such entries? (a, recommended) It follows the rename where exactly one selected source holds its function's name and hash, still excepting the mutant there, and fails as mutation.exception-stale with why moved, naming the new file; a --since run whose range deleted or renamed the file judges it too. (b) It fails as gone and the mutant at the new path is a survivor. (c) It follows silently. For --since: (a) keep, (b) whole-project runs only.

Asked as q-22, about exceptions-deleted-file.

## Considered Options

The options are those the question names.

## Decision Outcome

(a) Follow the rename and fail as moved, naming the new file; --since runs judge entries whose file the range deleted or renamed.

### Consequences

A run or check of the whole project, and a --since run whose range deleted or renamed the file, judge an entry whose file is no longer a selected source. Where exactly one selected source holds a function of the entry's name and hash, the entry still excepts its mutant there and fails as mutation.exception-stale with why moved and the new file; otherwise it fails as gone. A run never writes itos-cc.yaml, so the fix is a one-path edit there.
