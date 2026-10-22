# Working rules

Shared design, testing and dependency rules are in `docs/engineering.md`. They apply to every
stack and interface; stack and feature guides add only what is specific to them.
Go's packages, boundary lists, tests and quality checks are in `docs/go.md`.
Rules for any configuration file a project owns are in `docs/CONFIG.md`.
The command line's contract and rules are in `docs/CLI.md`.
Who owns each path, slice by slice, and which paths are shared is in `docs/ownership.md`.

The person's session coordinates with `itos go`. An implementing agent runs `itos guide work`,
takes its item with `itos work take`, does the work itself and starts no agents. Read the registry
with `itos work`; never edit owners or statuses by hand.

## Changes and commits

- Specify behavior before building it. The command line's features and fixes follow its
  black-box scenarios in `features/`, with red steps committed first. Never add unspecified
  behavior.
- Other work names a ledger task.
- No commit body line starts with a word and a colon, or with `with #,`.
- Read `git status --short` before staging; never stage a whole folder, `.`, or with
  `git add -A` or `-u`.
- Never bypass hooks, force-push or rewrite published history. Push with `itos push` in the
  background, reading its output and watching CI without polling. Never pipe a commit or push.
- Gates run through their configured hooks and CI, not manually before committing. Fix a valid
  check's failure in the work, not by weakening the check; stop on a wrong gate.

## Code boundaries

- Keep thin entry points over domain-owned behavior through ports. The domain knows no UI,
  filesystem implementation, encoding or exit codes; struct tags used only for serialization
  are allowed (decision 50). Use our fakes in its tests, never mocks.
- Look for a library before building what one likely solves; settle the choice in the spec.
- Code differing by OS takes the OS as a value. Build tags hold only a call unavailable on one
  OS, with no mutation site; never branch on runtime.GOOS inside logic.
- No credentials in fixtures, logs or committed files. Go vulnerability checks are not
  credential scans; the pinned gitleaks is, `tools/bin/pinned gitleaks dir . --redact`.
- Preserve third-party notices, `CONTRIBUTORS.md` and the comments naming where harvested code
  came from. Owner permission for copied infrastructure does not relicense dependencies or the
  original source projects.

<!-- itos:begin -->

## itos

Start with `itos go` unless your brief names another guide.

Stage only your own files, by explicit path: untracked files you did not make are someone else's work in progress.

<!-- itos:end -->

## Notes for agents

- Do not run mutation testing on this repository's own code while you work:
  no `itos-cc mutation run` over its sources, locally or in a push's CI. The
  nightly workflow is where it runs, committing `.metrics/mutate/` back. The Go tests of the mutation packages are fine:
  they mutate fixtures, not this code.

## Lessons

- Red-first tests must compile against the pre-change product and fail at
  the behavior assertion, not at missing new APIs. Use CLI fixtures and raw
  snapshot JSON when the implementation will add types. Verify every new
  behavioral failure before product changes; label checks of an older
  revision performed later as retrospective, not evidence obtained earlier.
  Guardrails may already pass and should be reported separately.
  Recorded 2026-10-08; last seen 2026-10-09; exit: permanent (judgment).
