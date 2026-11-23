# State machines

The owned command's cancel, exit, reap and cleanup machine in `internal/processes`
(`runOwnedCommand`, `cancelGroup`, `ownedGroupGone`, `waitOwnedProcessGroup`). It has no
explicit state in the code (T-246 documents it as it is; an explicit one is a later task), so the
states below are the ones its values and its order of calls imply. `TestOwnedCommandMachine`
(`owned_command_machine_unix_test.go`, rapid) names the same states, actions and invariants and
checks every one of them. `TestGroupEndsAreJudgedInEveryOrder` (T-239, T-242) keeps its rows as
the named regressions of the orders that failed on a real system.

## The system as a value

A real `/bin/sh` is the group's leader, so its own exit status, `os/exec`'s `Cancel` and `Wait`
are the system's. The group's other members, kill(2)'s answers and the clock are a model played
through `groupCalls` (`signal`, `wait`, `runnable`, `now`, `sleep`), so one Linux run plays
darwin's answers as well. kill(2) toward the group answers, in this order:

| the group holds | linux | darwin |
|---|---|---|
| nothing | ESRCH | ESRCH |
| the leader or a killable member running | nil | nil |
| zombies, nothing running that can be signalled | nil | EPERM |
| only a member that cannot be signalled | EPERM | EPERM |

SIGKILL ends the leader and the killable member (they become zombies); the unkillable member
ignores it. Linux's probe also asks `/proc` (`runnable`) whether a member still runs.

## States

5 components, 16 states.

- **Leader** (3): `running`; `zombie` (exited, or killed, and not yet reaped); `reaped`.
- **Killable member** and **unkillable member** (4 each, so 8 with both): `absent`; `running`;
  `zombie`; `gone` (init reaped it).
- **Context** (3): `live`; `ended by its parent`; `ended by its timeout`
  (`ErrCommandTimeout` as the cause).
- **Cleanup deadline** (2): `before`; `past`.
- **Run** (3, not counted above, since it follows the leader): `waiting` (Wait pending),
  `cleaning` (Wait reaped the leader), `returned`.

## Actions

6 actions and the invariant check, each a rapid action of the test of the same name.

| action | from | to | in the code |
|---|---|---|---|
| `ExitLeader(code)` | leader running | leader zombie, its exit status code | the command's own exit |
| `MemberExits(m)` | member running | member zombie | a member's own exit |
| `InitReaps` | a member zombie | members gone | init reaping; also at the cleanup's tick `initReapsAt` |
| `Cancel(cause)` | context live | context ended; `cancelGroup` kills `-pgid`; a running leader becomes a zombie; killable member zombie | `killGroup`'s `cmd.Cancel` |
| `PastDeadline` | deadline before | deadline past | the clock moving on |
| `Reap` | run waiting | run cleaning, then returned | `calls.wait` returning; `killOwnedProcessGroup`, then `waitOwnedProcessGroup` polls `ownedGroupGone` every 5 ms |
| (`""`) | any | the same | the invariants below, between any two actions |

`Reap` ends the run, so it ends the machine; a leader still running exits on its own with code 0
first. An action that does not apply to the state is skipped.

## The code's transitions

`owned_machine.go` holds this table as `ownedTransitions` and one function, `next`, from a state
and an event to the next state; an event with no row for the state it meets is no transition.
`TestOwnedCommandTransitionsMatchTheDocument` fails when this table and the code's differ in an
event, a state or a target. A run starts at `run=waiting, context=live, deadline=before,
group=unknown`. A row's `when` names what it needs of the state, its `then` what it sets.

<!-- transitions -->
| event | when | then |
|---|---|---|
| Wait reaps the leader | run=waiting | run=cleaning |
| the cleanup fails | run=cleaning | run=returned |
| the cleanup succeeds | run=cleaning, group=gone | run=returned |
| the context ends by its parent | context=live | context=ended by its parent |
| the context ends by its timeout | context=live | context=ended by its timeout |
| the deadline passes | run=cleaning, deadline=before | deadline=past |
| the probe finds the group gone | run=cleaning | group=gone |
| the probe finds the group live | run=cleaning | group=live |
<!-- /transitions -->

## Invariants

10 invariants. The first 2 hold between any two actions, the other 8 when the run returned; each
is a check of `TestOwnedCommandMachine`, in the method named.

| | invariant | checked in |
|---|---|---|
| I1 | The run returns only once Wait reaped the leader. | `invariants` |
| I2 | The group is not killed before its context ended or its leader was reaped. | `signal`, `invariants` |
| I3 | Every signal names the owned group, as `-pgid`, and is SIGKILL or the probe. | `signal`, `judgment` |
| I4 | A command that exited on its own is judged by its exit status, whatever cancel or timeout landed on its zombie (T-242, bug-5). | `judgment` |
| I5 | A command the cancel killed is judged by its cause: `TimedOut` for the timeout, `Cancelled` for its parent. | `judgment` |
| I6 | The only error is a `CleanupFailed`, never a cancel's or a kill's own. | `judgment` |
| I7 | The cleanup succeeds exactly when the group is gone: on Linux no member can run, on darwin nothing is in it. Nothing still running passes; nothing gone fails. | `judgment` |
| I8 | The cleanup fails only at or after its deadline (6ec9027c, f779e4c8). | `judgment` |
| I9 | The cleanup never sleeps once its deadline passed, so it ends within it. | `sleep`, `judgment` |
| I10 | The command's output is handed over whole, after its reader is done (T-243; `-race` holds the ordering). | `judgment` |

## Findings

None. 3000 runs (`-rapid.checks=3000`), on Linux with both systems' answers, found no reachable
state that breaks an invariant. The test fails on the T-242 regression (`cancelGroup` returning
the kill's answer): I6, with `ExitLeader` then `Cancel` as its shrunk actions.

What the model leaves out, so no invariant speaks for it: more than two members; a member that
exits while the cleanup polls (the test's actions never run during cleanup); a cancel after
`Reap`; a shared `CleanupBudget`; the memory and output caps; Windows' unowned path.

## Runs

`TestOwnedCommandMachine` runs 40 cases by default, at most (`ownedMachineChecks`); an explicit
`-rapid.checks=N` or a smaller `RAPID_CHECKS` decides otherwise.
