---
status: accepted
date: 2026-10-10
---

# context.Context carries cancellation from cmd through Handle to the ports, never a value

## Context and Problem Statement

Where does context.Context belong in decision 29's layout? A Query and its Handle carry no UI concern, as a web API's handler carries nothing of its HTTP request; Go's context.Context is not the request but the cancellation and deadline of the work, as .NET's CancellationToken is beside a MediatR query. Recommended: cmd makes the context (signal.NotifyContext in a later slice), Run passes it to Handle(ctx, q) as its first parameter, never a field of the Query, and it flows to the domain function that calls a port and to the port's methods (Files.Read(ctx, path)) and their adapters; pure domain logic never takes one, and no value rides in it, only cancellation and deadlines. Threaded now, as a refactor before T-52, so the shape a made project copies has it; Ctrl-C handling, a behaviour change, is a later slice with scenarios. Or: add it later to every port, handler and adapter at once. Or: not at all until a command needs it.

Asked as q-33, about T-52.

## Considered Options

- Thread it now through ports, domain calls of ports, Handle and adapters
- Add it later to every port, handler and adapter at once
- Not at all until a command needs it

## Decision Outcome

The recommendation, the person's call on 2026-10-10. context.Context carries only cancellation and deadlines, never a value, and is always a first parameter, never a struct field. cmd/itos-cc makes it; Run passes it to Handle(ctx, q); it reaches the domain function that calls a port, the port's methods and their adapters, which honour it; pure domain logic takes none. A stack/go refactor threads it through the Files port, the domain and the disk adapter before T-52, and T-52 gives Handle and the stdin adapter theirs. Ctrl-C handling is a later slice with scenarios.

### Consequences

Adds to decision 29. Files.Read takes a context, as does the domain function that calls it, internal/disk and internal/stdin honour it, and the slice's Handle takes one before its Query. No behaviour changes until a later slice makes cmd cancel it on Ctrl-C, with the exit code that slice settles. A made project's next command copies the shape, so cancelling long I/O and cleaning up needs no change to its ports or handlers.
