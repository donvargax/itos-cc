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

Stage only your own files, by explicit path: untracked files you did not make are someone else's work in progress.

<!-- itos:end -->

## Notes for agents

- Do not run mutation testing on this repository's own code on your machine:
  no `itos-cc mutation run` or `mutation sample` over its sources while you
  work, `--site` included: a `--site` run measures every listed scenario's
  reach first (about 470 scenarios, ~55 s on CI, some starting real
  language tools), the whole-suite load left to CI. A push's CI runs the code proof's one fresh site
  (`itos.yaml`'s `proof.code`) and `cd.yml`'s sample of ten recorded
  results; the nightly judges the rest, committing `.metrics/mutate/` back.
  The Go tests of the mutation packages are fine: they mutate fixtures, not
  this code.
- A survivor the code proof finds in a function your push changed, even one
  older than the push, is yours: kill it with a test in that push, and
  prove the kill by applying the mutant by hand in your worktree, running
  its package's tests (they must fail), and putting the line back. A repair
  push
  usually re-judges the same site, as `--seed {base}` seeds the draw by the
  range's start, but a site another push adds can outrank it and be drawn
  instead (2026-10-26, bug-11 and T-219), so a green repair run is not
  proof the first site died: the kill by hand is. When you judge the
  mutant truly equivalent, stop and report it with your reasoning: the
  coordinator approves it, or sends it back for a test, before its
  exception lands in `itos-cc.yaml` with its reason; the person reviews the
  exceptions weekly.
  About 22% of the mutants a test reaches survive (nightly 39930716718: 1871
  of 8317), so about one Go push in five may meet one.

## Lessons

- Red-first tests must compile against the pre-change product and fail at
  the behavior assertion, not at missing new APIs. Use CLI fixtures and raw
  snapshot JSON when the implementation will add types. Verify every new
  behavioral failure before product changes; label checks of an older
  revision performed later as retrospective, not evidence obtained earlier.
  Guardrails may already pass and should be reported separately.
  Recorded 2026-10-08; last seen 2026-10-09; exit: permanent (judgment).
- A commit that names a task in its Task: footer makes CI run that task's
  checks, so a docs commit editing an unfinished task's ledger entry names
  no task (docs need none): naming one whose checks cannot pass yet turns
  main red, and a revert, which must name it too, reruns them. Give the task
  back to clear it. Recorded 2026-10-24 (T-208, ci 40152891755; T-187 the
  same day); last seen 2026-10-24; exit: permanent (judgment).
