---
status: superseded by ADR-0051
date: 2026-10-10
---

# Go code is a shared domain behind ports, separate adapters and thin slices named for their commands

## Context and Problem Statement

How should the Go code be laid out so a project made from the template can add a second command without restructuring first? The person set the goal for v0.2.0 on 2026-10-10: itos-template's decision 17, a shared domain behind ports, separate adapters and thin slices, with code that differs by OS taking the OS as a value (its decision 26), as a refactor that changes no behaviour. Two calls were open. The domain and the command are both named count. Recommended: the slice is named for its command, internal/count, and the domain for what it is, internal/counting, as itos-template pairs update with updating and adopt with adoption; or the slice takes a suffix, internal/countcmd; or slices go under internal/commands. And the slice's application layer. Recommended: an explicit Query carrying domain values and a Handle that only wires it to the domain, Run assembling the Query from kong's flags, so a made project copies that shape for its next command; or Run calls the domain directly as now.

Asked as q-32.

## Considered Options

- Domain internal/counting, slice internal/count, explicit Query and Handle
- Domain internal/count, slice internal/countcmd
- Slices under internal/commands
- Run calls the domain directly, no Query or Handle

## Decision Outcome

Decision 17's layout, both recommendations taken, the person's call on 2026-10-10. On stack/go the domain is internal/counting, its port internal/counting/port keeping Files, Missing and Unreadable, and port/porttest the fake; the disk adapter is internal/disk, importing only the port, code that differs by OS taking the OS as a value with no runtime.GOOS branch in logic. On go/cli the command is the slice internal/count, named for the command: its kong struct and Run (UI), a Query carrying domain values and a Handle that only wires it to the domain (application), no counting and no options struct copying domain values. The stdin adapter is internal/stdin, importing only the port; cmd/itos-cc assembles everything and injects the disk fallback, so no adapter imports another. internal/cli stays the shared UI: errors are sealed sets of domain types, the UI alone turns them into exit codes, held by gochecksumtype, and only an unclassified error exits 70. depguard holds the arrows on both branches: the domain imports no adapter, slice, UI, kong or I/O package; an adapter imports only port packages; a slice imports no other slice; kong only in cmd and internal/cli. The domain is shared so a second command reuses counting without reaching into another slice.

### Consequences

Applies itos-template decisions 17 and 26 to this template. stack/go moves internal/count/domain to internal/counting and internal/count/disk to internal/disk; go/cli then turns internal/count/command into the slice internal/count with its Query and Handle and internal/count/input into internal/stdin. Every scenario passes unchanged; the moves are refactor commits. Each depguard arrow gets fixtures in tools/testdata/architecture it must accept and refuse, so tools/bin/architecture-check proves it, and tools/bin/domain-coverage reads internal/counting. docs/go.md and docs/architecture.md describe the layout. A made project adds a command as a new slice named for it, reusing the domain through its ports.
