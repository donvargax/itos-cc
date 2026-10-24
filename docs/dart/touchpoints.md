# Dart (Flutter) in itos-cc: touch points, ownership after the slices refactor, and a split

> **Status (2026-10-21, T-182):** D0-D11 are done but D8 (the idea dart-cd-other-os: `cd.yml`'s `dart` job runs on Linux alone). The decisions taken are q-74..q-83's Dart ones (q-74 to q-78 and q-83; q-80, how a project brings back what Dart leaves out, is still open) and the slices' scenarios, slice-28 to slice-38, which are the spec. This file is the research as found (2026-10-20/21): a line the work settled otherwise is ~~struck~~ or carries a **[now]** note saying what the code does, and its line numbers are those of `23adc19`, read as places, not addresses.

Read-only survey of this repository at `3efc0d9` (2026-10-20). T-168 moved its paths to where the
slices refactor (ADR-0051) put the files and read every line number it names in a moved or changed
file again at `23adc19` (2026-10-21); the history, counts and estimates in section 0 are the
survey's. When it was made the slices refactor was still open, its `slices-version`,
`slices-mutation`, `slices-import-engine`, `slices-scenario-registration` and `slices-close` ideas
`todo`; they became T-155 to T-172, and `docs/vertical-slices-inventory.md` ends with where each
move put each package. T-165 rewrote the coverage runner's markers rows (2.3): the in-process
driver's copy is gone. The ledger's own entry is `dart-support`
(`tasks/work-items.yaml`, kind idea, `depends_on: [slice-7]`). Per that entry the person's order
is: the refactor first, then the open GitHub issues, then Dart. It also says the spec must decide which
commands come first, the coverage tool, and the parsing library.

Scope note from the person (via the coordinator): Dart support means Flutter apps almost exclusively.
Lines marked **[Flutter]** say where a Flutter app differs. **[guess]** marks anything not checked against
code or a primary source. **[verified]** marks Dart-grammar facts checked against the grammar's
`grammar.js` / `src/node-types.json`, downloaded for the survey.

---

## 0. How Kotlin got here (the model), size, and surprises

Kotlin was never added on its own. The PoC spike `98c1bc6` (2026-10-02, "extract functions and methods from
TypeScript, Python, Kotlin, and Go with tree-sitter") brought all four languages in at once, along with
`lang/kotlin.go`. `go.mod` gained `tree-sitter-kotlin` in that same commit (`git log -S tree-sitter-kotlin -- go.mod`
returns only 98c1bc6, although the module path later changed to fwcd's). Everything after that was per-language
**parity** work, done in TDD pairs (a `test:` spec commit, then a `feat:` commit) under ledger items:

| Kotlin-specific work | commits | files / lines |
|---|---|---|
| Grammar switch to fwcd's (multi-annotated classes) | `01b43e0` | 10 f, +742/-108 (HTTP finding too) |
| Same-package refs for the graph | `1375e7e` | 18 f, +439 |
| Own tests = test classes that reach a file (own-tests-kotlin, decision 23) | `dda29ef` + `bd67e7d` | 19 f, ~+927 |
| Strict line coverage (strict-lines-kotlin, decision 25) | `f79e869` + `3368676` | 18 f, ~+950 |
| JVMs a test starts as integration coverage (integration-coverage-kotlin, decision 27) | `6d56598` + `451e0e4` | 15 f, ~+1,073 |
| Real tools smoke + CI (T-13) | `aa794f6`, `5a9ba19`, `b161df7` (revert), `26ee7fc` | 17 f +1,613; cd.yml |
| Counted runs offline (counted-languages) | `3ceec51` + `af7b260` | 7 f, +381 |
| Multi-language parity slices that included Kotlin (transitive reach, support reach, whole-suite freshness, fail-closed, strict-scope refusals, `--fail-uncovered=lines`, unreached baseline) | ~20 commits | most of the rest |

Totals: `git log -i --grep=kotlin` gives 123 commits: 65 `docs:` (ledger take/queue/close, q-38..q-44,
decisions 23-27), 24 `test:`, 15 `feat:`, 5 `refactor:`, 5 `ci:`, 4 `fix:` and 5 PoC-era. The 58 non-docs
commits touch 469 file-changes, +29.5k/-4.2k lines. That count includes multi-language commits and the
godog conversions, so it overstates Kotlin alone. The Kotlin-only feature pairs above come to about 52
file-changes and ~3k lines. Kotlin-specific product code today is ~1.3k lines [estimate]:
`internal/lang/kotlin.go` 327, `internal/covrun/kotlin.go` 104, `internal/covrun/jacoco.go` 306, `kotlinPlan`
(`internal/covrun/run.go:532-576`) ~90, `testinputs` Kotlin inputs/scope ~160, `mutants` Gradle/Maven choice
~70, the producers and body line ~70, the scrap Kotlin extractor ~125 (`internal/slices/scrap/domain/extract.go:280-402`),
and graph ~40. Kotlin-specific tests and fixtures: cmd tests 624 lines (`mutate_kotlin_*_test.go`,
`strict_lines_kotlin_test.go`), `internal/covrun/jacoco_test.go` 228, `features/mutate_language_kotlin_fixture.go`
67, and a smoke project `cmd/itos-cc/testdata/smoke/kotlin` (4 files).

**Size estimate for Dart [guess]:** Kotlin-level parity (units, crap, dry, scrap, mutation incl. own tests,
strict lines, counted runs, real-tool CI) would take about 40-60 non-docs commits and 300-450 file-changes, plus
~50 docs/ledger commits. Dropping integration (child-process) coverage and per-test collectors, as
Flutter apps rarely need them, takes out ~15%.

**What surprised in Kotlin (and what the Dart analogue will be):**
1. **Grammar quality.** The tree-sitter-grammars Kotlin grammar lost multi-annotated classes. fwcd's is
   pinned to an untagged commit as a pseudo-version (`go.mod:13`, `IDEAS.md:148-150`), and it names few fields,
   so names are found by child kind (`internal/lang/kotlin.go:15-18`). Regression scenarios:
   `@ID-UNIT-11`, `@ID-SCRAP-04`. Dart: the only Go-bound grammar found is
   `github.com/UserNobody14/tree-sitter-dart`, also untagged (proxy latest
   `v0.0.0-20261018035034-0038280b774b`, MIT, cgo with `src/scanner.c`). Its `go.mod` requires the old
   `github.com/smacker/go-tree-sitter`, though only its `binding_test.go` uses it, so expect an extra
   `go.sum`/`go.mod` line at most [guess]. Its `Language()` returns `unsafe.Pointer`, the same shape as
   fwcd's binding, so `sitter.NewLanguage` takes it [verified from bindings/go/binding.go]. ABI
   compatibility with go-tree-sitter v0.25.0 is unchecked [guess: fine].
2. **cgo.** Grammars are C. `release.yml:11-12,196` builds each platform natively with `CGO_ENABLED=1`
   (GoReleaser's `CGO_ENABLED=0` at `.goreleaser.yaml:21` cannot cross-build). A Dart grammar adds a C
   compile on Linux, macOS and Windows CD runners [guess: the scanner compiles under MSVC/MinGW as fwcd's does].
3. **Build-tool variety.** Kotlin needed three coverage runners (`KotlinRunner`, `internal/covrun/kotlin.go:89`:
   Gradle JaCoCo, Kover, Maven), an XML parser (`ParseJaCoCo`, `internal/coverage/formats.go:173`), test
   *class* discovery (`lang.TestClasses`, `kotlin.go:205`), because Gradle and Maven select classes, not files,
   reactor `-pl -am` selection (`internal/slices/mutation/domain/command_policy.go:103-135`), multi-module closure
   (a test in another module measures nothing), `--rerun` against Gradle's up-to-date check, a
   `KotlinBodyLine` because JaCoCo puts default-argument bridges on declaration lines
   (`internal/slices/mutation/domain/coverage.go:216-243`), JaCoCo agent/cli jars found in the Gradle/Maven caches
   (`internal/covrun/jacoco.go:56-140`), and CI shards because cold Gradle took 7.6 min (`cd.yml:235-246`).
   Dart/Flutter is closer to **Python's** model (tests are files, run by path, reach-based own tests) and
   to **TypeScript's** LCOV. One runner (`flutter test`), one format (LCOV, already parsed).
4. **Python-only extras Kotlin lacks**, which Dart may need: an inventory command for files no test loads
   (`internal/covrun/inventory.go:26-29`, Python and TypeScript only; Kotlin uses `classInventories`,
   `:156`); supervised probes through the executor (`PlansSupervised`, `internal/covrun/run.go:139-143`;
   `freshPythonCommand` in freshenv); `PathEnv`/`PathDirs` (`command_policy.go:51`); bytecode and pytest
   cache hygiene (`project.NoBytecodeEnv`, MUT-214); conftest-style implicit support files
   (`internal/imports/python.go:100-135`, its `support`). TypeScript extras: `Spec.Inline` units (Express callbacks), `Mutations.Other`
   for `?.`/`??`, a `stableModuleNamespace` for counted runs (`internal/slices/mutation/domain/fresh_admission.go:100-110`),
   and an unattested runner (a project's own coverage script).

---

## 1. Flutter facts that shape the design

- Build root: `pubspec.yaml`. A Flutter app's pubspec has `dependencies: flutter: sdk: flutter` and
  `dev_dependencies: flutter_test: sdk: flutter`. Runner: `flutter test`; a pure Dart package would use
  `dart test` [guess: out of first scope per the person's note]. **[now]** q-74: Flutter apps and packages only; a
  pure Dart package is left unmeasured, its coverage asking `--coverage-command` (idea dart-pure-packages).
- `flutter test --coverage [--coverage-path <file>]` writes LCOV, by default `coverage/lcov.info`, with
  `SF:` paths relative to the package (`lib/...`). The LCOV parser exists (`internal/coverage/formats.go:36`
  `ParseLCOV`; `Parse` at `:16` sniffs Go/XML and otherwise reads LCOV). Relative `SF:` paths go
  through the existing matcher (`internal/coverage/coverage.go:513-575`). [guess] The LCOV lists only libraries a
  test loaded. It writes no BRDA unless `--branch-coverage` is passed [guess]. So strict lines need either an
  inventory (a generated test importing every `lib/` file and running no test [guess], as Vitest's
  "run no test" inventory does) or fail closed as Jest/c8 do (README.md:535-540). **[now]** itos-cc runs
  without `--branch-coverage`, by line; strict Dart fails closed (slice-36): a library no test loads
  is `mutation.coverage-missing`, and the inventory is the idea dart-strict-inventory.
- `flutter test` runs `pub get` implicitly unless `--no-pub`. Worker copies and frozen exports must run
  with `--no-pub` (offline, as counted runs require: MUT-209).
- `.dart_tool/package_config.json` (gitignored) is what resolves `package:` imports. A worker copy copies it today,
  since `.dart_tool` is not in worker `skipDirs` (`internal/slices/mutation/adapters/worktrees/worker.go:27-30`), and copies
  `.dart_tool/flutter_build` caches with it. A frozen export from git has none of it (see freshenv).
  **[now]** A worker's copy neither copies nor links `.dart_tool`: it runs `flutter pub get --offline` once
  before its first run (slice-35); a counted run readies its export with `flutter pub get --offline --enforce-lockfile` (slice-37). The pub cache
  (`~/.pub-cache`, `PUB_CACHE`) is outside the project, like Gradle's cache.
- **[Flutter]** App folders `android/`, `ios/`, `macos/`, `linux/`, `windows/`, `web/`:
  - `android/app/src/main/kotlin/.../MainActivity.kt` plus `android/app/build.gradle(.kts)` makes discovery see
    **a Kotlin build root inside every Flutter app** (`internal/covrun/run.go:104` markers). crap and mutation would
    plan Gradle/JaCoCo for it (needs the Android SDK), and strict runs "fail closed when another language
    measured nothing" (MUT-204, `internal/slices/mutation/run.go:367-376`). ~~This needs a decision: an automatic
    exclusion of a Flutter app's platform folders, or documented `exclude:` globs (decision 45).~~ **[now]** q-75
    (slice-30): discovery never walks them beside a pubspec.yaml that depends on the flutter SDK, and no
    key brings them back while q-80 is open.
  - ~~`ios/Pods`, `ios/.symlinks` (links into the pub cache) and `macos/Pods` are copied into every worker
    (symlinks as links, `worker.go:73-78`).~~ **[now]** A worker's copy leaves out the platform folders, these
    with them (slice-35, `worktrees/dart.go`). `android/.gradle` is already skipped, and `android/app/build` is already
    build output (beside `build.gradle`).
  - ~~`build/` beside `pubspec.yaml` is skipped only through `git check-ignore` today
    (`internal/project/project.go:60-69`), because `pubspec.yaml` is not in `buildManifests` (`:33-36`).~~
    **[now]** `pubspec.yaml` is a build manifest (slice-30), so `build/` and `coverage/` beside it are build output.
- Generated Dart (`*.g.dart`, `*.freezed.dart`, `*.mocks.dart`, `*.gr.dart`, `*.config.dart`,
  `lib/generated_plugin_registrant.dart`, `firebase_options.dart`, l10n output) would be measured and mutated
  unless excluded. `lang.Detect` already hard-codes `.d.ts`/`.min.js` (`internal/lang/lang.go:275-281`).
  **[now]** q-77 (slice-30): discovery takes none of build_runner's and protoc's suffixes, the plugin registrant,
  a file carrying source_gen's marker or gen-l10n's output (`internal/project/flutter.go`); `firebase_options.dart`
  is not among them. Worker copies keep generated Dart, and no key brings it back while q-80 is open.
- Tests: `test/**/*_test.dart` (widget and unit tests), `integration_test/` (needs a device or desktop
  embedder: never a mutation's own tests, [guess]), `test_driver/`. **[now]** q-76: neither ever runs
  (slice-35), nor counts among an outcome's tests (slice-38); running them is the idea dart-integration-tests. `flutter_test_config.dart` in a test
  directory wraps every test file below it, like `conftest.py`, so it is an implicit support file for reach and
  fingerprints. `dart_test.yaml` configures tags and timeouts. Golden tests (`matchesGoldenFile`) depend on platform fonts:
  a baseline may fail off its recording OS [guess: note for CI fixtures].
- `flutter test` start-up is slow (compiles a test bundle; seconds per invocation [guess: 5-15 s cold])
  and dominates mutant trials, as Gradle did for Kotlin.
- Toolchain location: `flutter` on PATH, or FVM (`.fvmrc`, `.fvm/flutter_sdk/bin/flutter`) [guess: as
  common in Flutter apps as `gradlew` is in Gradle builds; prefer it as `gradlew` is preferred,
  `testtools/toolchain.go:87-89`]. **[now]** `covrun.FlutterFor` prefers the SDK FVM links beside the
  nearest `.fvmrc`, else the PATH's (slice-32).
- **Grammar shape [verified]:**
  - A Dart function or method is **two sibling nodes**: `function_signature` / `method_signature` /
    `getter_signature` / `setter_signature`, followed by `function_body` (`grammar.js:217-249` top level,
    `:2141-2147` class members). No single node spans a unit. `lang.Unit.Node` is one node, and
    `walker.unit` takes StartLine/EndLine from it (`lang.go:435-468`). Its users: complexity (`lang.go:470`),
    `mutants.Sites`/`UnitHash` byte ranges (`internal/slices/mutation/domain/sites.go:51-150`), coverage evidence
    (`internal/slices/mutation/domain/coverage.go:184-214`), fingerprints, and scrap. **This is an engine change in `internal/lang`**
    (e.g. a Spec hook for a unit's body or end node, or a span), not just a new `dart.go`. **[now]** `Spec.Body`
    (slice-28).
  - There is **no call node**. Calls are `selector`/`argument_part` chains after a primary (`selector`,
    `argument_part`, `unconditional_assignable_selector`, `conditional_assignable_selector`, `cascade_section`). `Syntax.Calls`/`Members`
    with `IsCallee`/`Callee` (`lang.go:181-230`) assume a call node holding its callee. dry fingerprints (`fingerprint.go:118`),
    scrap (`measure.go:184`) and http (`http.go:83..248`) depend on them. Expect an engine-level adaptation, like
    Kotlin's `navigation_suffix` look-through at `lang.go:178-195`. **[now]** `Syntax.Links` (slice-29).
  - Operators are named leaf nodes (`additive_operator`, `relational_operator`, `equality_operator`,
    `logical_and_operator`, `logical_or_operator`, `negation_operator`, `minus_operator`), and `true`/`false` are
    named leaves, so `Mutations` can likely be declared with the existing tables [guess: the leaf text is the token].
    `if_null_expression` (`??`) and `conditional_assignable_selector` (`?.`) suit `Mutations.Other` and `Decision`.
    **[now]** `a ?? b` becomes `b` through `LeftDeletions`; `?.` is a decision but never a mutation site, nor are
    `*` and `/` (slice-29).
  - Decisions: `if_statement`, `conditional_expression`, `for_statement`, `while_statement`, `do_statement`
    [guess name], `catch_clause`, `switch_statement_case`, `switch_expression_case`, `logical_and/or`, `??`, `?.`,
    and, **[Flutter]**, collection `if_element` and `for_element`, used throughout widget trees.
  - Imports: `library_import`, `library_export`, `part_directive`, `part_of_directive` with `uri`. There is no package
    declaration, so the namespace comes from the path (`modulePath` with the `pubspec.yaml` marker, `lang.go:582`,
    which already strips a leading `lib/`). **[now]** `dartLibrary`: the pubspec.yaml's name, then the path under
    `lib/`, dotted.

---

## 2. Touch points, by area

Columns: **where** (path:line, symbol), **what Dart needs**, **owner after ADR-0051**. "Engine" means it stays where it
is behind a stable API (ADR-0051: lang, selection, coverage, snapshot, project/git history, settings,
processes, UI, import engine). "slices/mutation" means the mutation slice, `internal/slices/mutation`: its
domain and the domain's fakes in `domain/` and `domain/porttest/`, its adapters in `adapters/<name>/`
(T-157 to T-161).

**[now]** Slices 28-38 built every row below; a note on a row says where the work went otherwise.

### 2.1 Grammar, language detection, units, complexity (lang engine)

| where | what | owner after |
|---|---|---|
| `go.mod:7-22`, `go.sum` | add `github.com/UserNobody14/tree-sitter-dart` (pseudo-version). The person selects it in the spec (`docs/engineering.md:50-55`). `IDEAS.md:148` notes the Kotlin pin; add a Dart line | shared root file |
| `internal/lang/dart.go` (new) | `init()` → `register(&Spec{...})` (pattern `kotlin.go:23-63`): Name `"dart"`, Extensions `.dart`, Grammar, Namespace (`modulePath(..., "pubspec.yaml")`), Unit, Container (`class_definition`, `mixin_declaration`, `extension_declaration`, `extension_type_declaration`, `enum_declaration`), Private (name starts with `_`), Decision, IsTest, Separator, Syntax, Mutations, Comment `//`, Imports. Optional BodyLine/Inline | lang engine |
| `internal/lang/lang.go:20-41` `Unit`, `:44-101` `Spec`, `:435-468` `walker.unit` | engine change for signature+body siblings (§1). Possibly also `Inline` for Flutter callbacks (decision, default no) | lang engine |
| `internal/lang/lang.go:181-230` `Syntax.IsCallee/Callee` | selector-chain calls (§1) | lang engine |
| `internal/lang/lang.go:275-281` `Detect` | generated-file suffixes (`.g.dart`, `.freezed.dart`, `.mocks.dart`, ...) if decided here rather than in `exclude` | lang engine |
| `internal/lang/http.go:30,71-75,322,388` | HTTP endpoints/calls per language. **serve/graph only, optional** (Dio/http clients, shelf routes) | lang engine (serve is legacy) |
| `internal/lang/lang_test.go`, `namespace_test.go`, `fingerprint_test.go` | unit tests of the rules (decision 49: scenarios are the spec, unit tests only for domain rules) | lang engine |
| `internal/lang/kotlin.go:144-188` `TopLevelNames`/`ReferencedNames`, `:205` `TestClasses` | Kotlin-only exports. Dart needs no test-class finder (tests run by file) and no same-package refs, but may need `part of` → library mapping (see 2.6) | lang engine |

Scenarios (spec, red first): `features/units.feature` new scenario like `@ID-UNIT-10` (`:158-197`),
covering top-level functions, methods, getters/setters, constructors (named, factory), extension methods, mixins, `_private`,
and a **[Flutter]** `StatelessWidget.build`/`State.build` class; `features/complexity.feature` `@ID-CPLX-01` Examples row
(`:14-18`) plus a "Dart decisions" scenario like `@ID-CPLX-06` (`:111-125`) incl. collection-if/for and `??`/`?.`;
`@ID-CPLX-03` (optional chaining) row/And. Steps: `features/complexity.go:28-33` (straight-line source map),
`:46,:50` regex `(TypeScript|Python|Go|Kotlin)`; `features/units.go` (new step only if a Given needs one).

### 2.2 File selection and build roots (selection engine, project library)

| where | what | owner after |
|---|---|---|
| `internal/project/project.go:19-24` `skipDirs` | `.dart_tool` is already skipped by the dot rule in discovery (`:108`). Decide on `android`/`ios`/... (§1). **[now]** q-75: `internal/project/flutter.go`'s platform rule, not `skipDirs` | project engine |
| `internal/project/project.go:33-36` `buildManifests` | add `pubspec.yaml`, so `build/` and `coverage/` beside it are build output | project engine |
| `internal/selection/port/porttest/tree.go:75-90` | the fake's copy of the same lists, which `TreeContract` keeps honest. **It must change in the same commit** as project.go. **[now]** the Flutter rules' copy is `porttest/flutter.go` | selection engine |
| `IsTest` in `dart.go` | `test/`, `integration_test/`, `test_driver/` dirs and the `_test.dart` suffix | lang engine |

Scenarios: `features/file_selection.feature:139-147` `@ID-SEL-15` Examples rows (`test/widget_test.dart`
test, `lib/main.dart` production, `integration_test/app_test.dart` test). **Do not add Dart to the
Background project** (`features/selection.go:73-90,102`, "TypeScript, Python, Kotlin and Go sources"): its counts
feed most SEL/CRAP scenarios, so a new language there ripples [guess: avoid].

### 2.3 crap and coverage (coverage engine)

| where | what | owner after |
|---|---|---|
| `internal/covrun/run.go` `markers` | `"dart": {"pubspec.yaml"}`; the one table, which the in-process driver reads too through `Markers` (T-165), so nothing in cmd changes | coverage engine (top-level `coverage`, ADR-0037 may move it under internal later) |
| `internal/covrun/run.go:149-227` `buildPlans` | `case "dart"` → `dartPlan`; with `narrow` (OwnTests+reach) group by reaching test files as Python does (`:166-167`, `PythonTests`) | coverage engine |
| `internal/covrun/dart.go` (new) | `dartPlan(dir,out,tests)`: `flutter test --no-pub --coverage --coverage-path <out>/lcov.info [tests]`, `Reports`, `Existing` (`coverage/lcov.info`), `Unsupported` when no `flutter` (never downloaded, decision 26), FVM preference; `DartTests` (test files among reach, excluding `integration_test/`); `DartRunner` if `dart test` is ever supported. **[now]** the command is `flutter --suppress-analytics --no-version-check test --no-pub --no-dds --coverage --coverage-path=…`; `Unsupported` also for a pure Dart package and one pub has not resolved; `DartTests` keeps only `test/`, so `test_driver/` is out too; no `DartRunner` (idea dart-pure-packages) | coverage engine |
| `internal/covrun/inventory.go:26-29` `inventories` | optional Dart inventory for unreached files (§1), or none, leaving strict to fail closed. **[now]** none: strict fails closed (slice-36; idea dart-strict-inventory) | coverage engine |
| `internal/covrun/pertest.go:73-130` `testCollectors` | listed tests' per-test collectors. Dart: none (fallback one run per test, decision 10) [decision]. **[now]** none (q-76), so no listed test reaches a Dart line; per-test-name coverage is the idea dart-per-test-coverage | coverage engine |
| `internal/covrun/jacoco.go`/`collector.go` integration coverage | processes a test starts: recommend "Dart does not measure child processes", recorded beside decisions 26/27 [decision]. **[now]** q-76 phase 1: none, recorded there, not beside decisions 26/27 | coverage engine |
| `internal/coverage/formats.go:16-75` | LCOV already parsed; DA only unless branch coverage; nothing to add [guess] | coverage engine |
| `internal/coverage/porttest/measurer.go:64,175,208` | the fake's per-language reach/report rules (`group.Language != "python" && != "kotlin"` at :208) | coverage engine (fake) |
| `cmd/itos-cc/scenarios_coverage_trace_test.go:273-275` | Kotlin selector special case; Dart likely none | cmd |
| `features/binary.go:108-176` `Coverage` switch | `case "dart"`: check `flutter` (and `dart`) on PATH, skip or fail under `ITOS_CC_LANGUAGE_TOOLS`; `languageJob` (`:206-211`) already skips non-Go in CI jobs without the tools. **[now]** `flutter` alone; the fixture is resolved with `flutter pub get` and FVM's link made; a Dart scenario skips in every CI job but `cd.yml`'s `dart` job, which sets `ITOS_CC_FLUTTER_TOOLS` | features harness |
| `features/steps.go:127-130,259-262` | doc comments listing `Tests.Language`/`Coverage.Language` values | features harness |
| `internal/slices/crap/crap.go:79,315` | help text "Go and TypeScript coverage runs only the tests that load..."; changes if Dart narrows. **[now]** Dart does not narrow: the help says Python, Kotlin and Dart run their whole suite (T-181) | slices/crap |
| `internal/slices/crap/domain/crap.go` | language-agnostic (uses `measured[u.Language]`, `:89`) | slices/crap |

Scenarios: `features/coverage.feature` `@ID-COV-04` (whole suite), `@ID-COV-05` Examples (`:56-60`, choosing the
command: a Dart row with `pubspec.yaml`, `flutter test --coverage`), `@ID-COV-08` (`:88-92`, a tool not
installed is not fetched); `features/coverage.go:284` regex `(Go|TypeScript|Python|Kotlin)`;
`features/coverage_projects.go` (per-language project builders, 19 Kotlin mentions); `features/crap.feature`
`@ID-CRAP-05` (`:66-70`) optional row.

### 2.4 dry (slices/dry)

`internal/slices/dry/domain/dry.go` is language-agnostic: it groups by `Language` (`:160`) and uses `lang.Fingerprint`.
It depends only on Dart's `Syntax` (2.1). Scenario: `features/dry.feature` `@ID-DRY-02` Examples row (`:22-27`);
steps `features/dry.go:65,68` regexes. **Owner: slices/dry, with zero product edits expected.**

### 2.5 scrap (slices/scrap)

| where | what | owner after |
|---|---|---|
| `internal/slices/scrap/domain/extract.go:42-55` `Extract` switch + new `(x *extractor) dart` (Kotlin's is `:280-402`) | `test(...)`, `testWidgets(...)`, `group(...)` titles and nesting, `setUp`/`setUpAll`/`tearDown` setup lines, data tables | slices/scrap |
| `measure.go:88-105` `loopCases` | `for (final c in cases)` in Dart's `for_statement` | slices/scrap |
| `measure.go:222-249` `isAssertion` | `expect`, `expectLater`, `verify`/`verifyNever` (mockito, mocktail), `expectAsync*` | slices/scrap |
| `measure.go:251-280` mocks map + `isMock` | `Mock` subclasses, `when(...)`, `mocktail`'s `when(() => ...)`, `registerFallbackValue` | slices/scrap |
| `analyze.go:287-292` `tableIdiom` | Dart idiom string (no built-in parametrize: "a loop over cases calling test", or package-specific) [decision] | slices/scrap |

Scenarios: `features/scrap.feature` `@ID-SCRAP-01` Examples (`:15-20`, `flutter_test`/`package:test`, groups),
`@ID-SCRAP-07` (`:95-103`, assertions), `@ID-SCRAP-10` (`:130-137`), `@ID-SCRAP-15` (`:204-209`), plus
**[Flutter]** a widget test (`testWidgets`, `pumpWidget`, `tester.tap`, `find.byType`) scenario. Steps:
`features/scrap.go:86,123,142` regexes, `features/scrap_examples.go`, `scrap_grouping.go`.

### 2.6 Import resolution and test reach (import engine, `internal/imports`)

| where | what | owner after |
|---|---|---|
| `internal/imports/languages.go` table line, new `internal/imports/dart.go` resolver (`index`, `resolve`, `module`; T-163) | Dart: `package:<own name>/x.dart` → `lib/x.dart` (needs the pubspec `name:`), relative URIs, `dart:`/other packages external, conditional imports (both targets), **`export`** (barrels re-export; transitive reach must follow them), **`part`/`part of`** (a part file is its library's module: dart.go's `module`; `part of <name>` may need it to see the index, which `Resolution.Module` absorbs). **[now]** each file is its own module, and a library depends on its parts, as on what it exports | **import engine** (`internal/imports`, T-162) |
| `internal/imports/dart.go` `facts` (what `Of` keeps; Kotlin's are kotlin.go's) | per-language extras (Kotlin's top-level and refs); Dart's part-of library. **[now]** none: a part's `part of` names its library back and is no import | import engine |
| `internal/imports/dart.go` `support` (Python's conftest.py is python.go's); `reach.go` names no language | transitive closure already applies to every language not `oneHop`. Add `flutter_test_config.dart` (applies to test files at and below its directory, up to the package root) as `support`, as python.go applies conftest.py | import engine |
| `internal/slices/mutation/adapters/importgraph/reach.go` | adapter; no language code | slices/mutation adapter |
| `internal/slices/mutation/domain/porttest/reach.go:39,66,86` | the Reach fake's per-language rules (Go one hop, Python conftest) | slices/mutation porttest |
| `features/imports.go:12` `sourceExtensions` | add `.dart` (the harness's declared adjacency) | features harness |

Scenarios: `features/mutate.feature` `@ID-MUT-215` Examples (`:2076-2081`, transitive reach),
`@ID-MUT-219` Examples (`:2160-2168`, support files: "a flutter_test_config.dart above it"); steps
`features/mutate_language_reach.go:14,19` regexes, `mutate_language_reach_fixture.go` (18 Kotlin mentions).
`architecture_graph.feature` has no steps (legacy, not converted), so Dart rows there are optional.

### 2.7 mutation: sites, commands, toolchain (slices/mutation)

| where | what | owner after |
|---|---|---|
| `dart.go` `Mutations` (2.1) | swaps, deletions (`!` prefix only, never the postfix null-assert `!`), literals, `Other` for `??`/`?.` | lang engine |
| `internal/slices/mutation/domain/sites.go:51-150` | language-agnostic over `Unit.Node`; affected by the sibling-body engine change | slices/mutation domain |
| `internal/slices/mutation/domain/inputs.go:16-24` `ToolFacts` | new fields (Flutter binary path, test files), as Python's `PythonFiles` | slices/mutation domain |
| `internal/slices/mutation/domain/command_policy.go:22-70` `ChooseCommand` | `case "dart"`: `flutter test --no-pub [--fail-fast? guess] <reaching test files>`, `request.All` → whole `test/`. **[now]** `DartArgs`: flutter test has no fail-fast flag, so none; `Ready` is `flutter pub get --offline` | slices/mutation domain |
| `internal/slices/mutation/domain/fresh_commands.go:31` `AddFreshBaseline` | add `dart` to the reach-based "runs nothing" languages (Python/Kotlin) | slices/mutation domain |
| `internal/slices/mutation/domain/fresh_admission.go:100-110` `stableFunction` | `case "dart"`: `stableModuleNamespace(rel, committed, "pubspec.yaml")` | slices/mutation domain |
| `internal/slices/mutation/domain/load_snapshot.go:26` | TypeScript-only scope probe; none for Dart [guess] | slices/mutation domain |
| `internal/slices/mutation/wording.go:21-32` `LanguageName` | the generic capitaliser already gives "Dart"; no edit | slices/mutation |
| `internal/slices/mutation/mutation.go:70,87`, `run_contract.go` (13-408), `check_contract.go:31,108-120` | help and contract text listing languages ("Python, TypeScript or Kotlin line"). **[now]** done in T-181 | slices/mutation |
| `cmd/itos-cc/testdata/mutation-help.json` | help snapshot (`mutation_parser_test.go`); every help-text change rewrites it | cmd (owned by mutation by path) |
| `internal/slices/mutation/adapters/testtools/toolchain.go:26-99` `Observe`, `:139-156` `CoverageRunner` | `case "dart"`: Root = nearest `pubspec.yaml`, Flutter vs Dart from pubspec, FVM, test files from `coverage.DartTests` | slices/mutation adapter |
| `internal/slices/mutation/domain/porttest/tools.go:121-259` | the Toolchain fake's per-language cases (`:164-246`) | slices/mutation porttest |
| `internal/slices/mutation/domain/porttest/workers.go:394-399` | fake's list of known runner basenames (`python`, `gradle`, `mvn`, `go`); add `flutter`, `dart`. **[now]** left as it was: neither name was added | slices/mutation porttest |
| `internal/slices/mutation/adapters/freshenv/preparation.go:522-531` | own baselines; `:524` `hasReachedUnit` (python, kotlin) gains dart | slices/mutation adapter |
| `internal/slices/mutation/adapters/worktrees/worker.go:21-30` `linkDirs`/`skipDirs` | `.dart_tool`: copy, skip and re-create `package_config.json`, or link (**not** link: `flutter test` writes into it, so parallel workers would race) [decision]; **[Flutter]** skip `ios/Pods`, `macos/Pods`, platform folders. **[now]** slice-35: `.dart_tool` neither copied nor linked (`worker.go`'s `skipDirs`), `flutter pub get --offline` making the copy's own once per worker; in `worktrees/dart.go`, `build/` and `coverage/` beside a pubspec.yaml and the platform folders left out; a workspace member's copy holds the workspace root and its members | slices/mutation adapter |

Scenarios: `features/mutate.feature` "Dart sites" like `@ID-MUT-05` (`:111-130`); `@ID-MUT-07`
Examples (`:170-176`, narrowest test command); an own-tests outline like `@ID-MUT-217` (`:2120-2140`, "A Dart
file's own tests are the test files that reach it"); `@ID-MUT-20`-style "no coverage for the language".
Steps: `features/mutate.go:158` regex, `features/mutate_commands.go`, `features/mutate_language_own.go`,
and **the hand list `features/mutate_language.go:23` `languageMutationSteps`** if a new step group is added.

### 2.8 mutation: strict line coverage evidence, fingerprints, freshness (slices/mutation)

| where | what | owner after |
|---|---|---|
| `internal/slices/mutation/domain/coverage.go:24` `StrictLanguages` | add `"dart"` (only if decided, since it needs proven executable lines, see inventory). **[now]** added (slice-36), failing closed on a library no test loads | slices/mutation domain |
| `coverage.go:44-56` `CoverageProducer`, `:65-121` per-runner producers, `:125-145` `ValidCoverageProducer` | Dart producer strings ("flutter test --no-pub --coverage <reaching tests>; scope=own" / all-tests). **[now]** `flutter test --no-pub --no-dds --coverage --coverage-path=<report> <reaching test files>; scope=own`, and `…; scope=all-tests` with no files | slices/mutation domain |
| `coverage.go:184-214` `LineCoverageEvidence` | ~~Dart first-line rule like `KotlinBodyLine` (the signature line, field initialisers, constructor initialiser lists) [guess: needs measurement on real LCOV]~~ **[now]** no body-line hook: a Dart function's lines run from its signature line, which the VM counts at each call, and a line any of whose code ran is covered | slices/mutation domain |
| `internal/slices/mutation/domain/run_coverage.go:124-154` `CoverageInputsFor` | runner-specific producer branch (TypeScript/Kotlin), if Dart has more than one runner | slices/mutation domain |
| `internal/slices/mutation/adapters/testinputs/inputs.go:59-78` `Coverage` switch | `case "dart"` → `DartCoverageInputs` | slices/mutation adapter |
| `internal/slices/mutation/adapters/testinputs/line_coverage_inputs.go` (`KotlinCoverageInputs` `:257-315` as the model) | fingerprint the package root: every `.dart` under it (lib, test, integration_test), `pubspec.yaml`, `pubspec.lock`, `dart_test.yaml`, `build.yaml`, `l10n.yaml`, **[Flutter]** declared assets and fonts (golden tests read them), `flutter_test_config.dart`, Flutter/Dart SDK version (`flutter --version --machine`), `FLUTTER_ROOT`, FVM's `.fvmrc`; leave out `.dart_tool`, `build`, `coverage`, platform folders. **[now]** `DartCoverageInputs` (slice-36): every `.dart` of the package, or of a member's whole pub workspace, generated Dart included, each `pubspec.yaml`, `pubspec_overrides.yaml` and `dart_test.yaml`, the root's `pubspec.lock`, the nearest `.fvmrc`, the `flutter` used, `FLUTTER_ROOT` and `PUB_CACHE`; not `build.yaml`, `l10n.yaml`, assets, fonts or `flutter --version`; `integration_test/` and `test_driver/` are left out too | slices/mutation adapter |
| `internal/slices/mutation/adapters/testinputs/line_coverage_scope.go:39-58` `LineCoverageUnsupported` + new `(s scopeReader) dart` (Kotlin's at `:335-379`) | refuse scopes the fingerprints cannot prove: `path:` dependencies outside the root, ~~pub workspaces (`workspace:` / `resolution: workspace`)~~, `dependency_overrides` with paths. **[now]** a pub workspace is admitted, its whole workspace the root for a member; refused is a path dependency or override, in a `pubspec.yaml` or `pubspec_overrides.yaml`, outside that root; git and hosted dependencies are admitted | slices/mutation adapter |
| `internal/slices/mutation/adapters/testinputs/suite.go:56-69` `suiteRoot` | `case "dart"`: nearest `pubspec.yaml` (workspace root if a pub workspace is ever supported). **[now]** the nearest `pubspec.yaml`, a member's own too; an `--all-tests` outcome rests on `test/` alone (slice-38) | slices/mutation adapter |
| `internal/slices/mutation/domain/porttest/fingerprints.go:57-327` (switches at `:59-65,209-226,275-284,309-315`), `scope.go:405-416`, `input_contract.go:105-170` | the Fingerprints fake and its contract rows per language (`{"kotlin","src/main/kotlin/A.kt","build.gradle.kts","@jvm-env"}` at `:107`) | slices/mutation porttest |
| `internal/slices/mutation/run.go:118,367-376,402,423`, `counted_ui.go:299-305` | Go-specific strict paths; Dart follows the line-precision path with no edit [guess] | slices/mutation |

Scenarios (all in `features/mutate.feature`, mostly Examples rows): `@ID-MUT-204` (`:1845-1850`, fail
closed), `@ID-MUT-205` (`:1862-1867`), `@ID-MUT-206`, `@ID-MUT-207`, `@ID-MUT-223`-style "A strict Dart run
proves every executable line" (`:2225-2232`), `@ID-MUT-224` Examples (`:2245-2253`, refused scopes),
`@ID-MUT-247` (`:1920-1925`), `@ID-MUT-258` (comment-free hash, `features/mutate_comment_free.go:76`
regex). Steps: `features/mutate_strict_lines.go:33`, `mutate_strict_refusal.go:18`,
`mutate_strict_lines_fixture.go`, `mutate_unjudged_evidence.go:32`, `mutate_language_suite.go:39`.

### 2.9 mutation: counted runs, frozen exports, installed dependencies (slices/mutation)

| where | what | owner after |
|---|---|---|
| `internal/slices/mutation/adapters/freshenv/dependencies.go:26-87` `linkInstalledDependencies` | today it links `node_modules` and the venv into the frozen export. Dart: the export lacks the gitignored `.dart_tool/package_config.json`, so copy it in (or run `flutter pub get --offline` there); the pub cache needs no link (like Gradle's). **[now]** slice-37: it links FVM's `.fvm/flutter_sdk`, and `freshenv/dart.go` readies each package with `flutter pub get --offline --enforce-lockfile` | slices/mutation adapter |
| `internal/slices/mutation/adapters/freshenv/preparation.go:533-541` `CommandEnvironment` | offline env for Flutter (`PUB_OFFLINE`? `--no-pub` in argv) [guess]. **[now]** no variable: `--no-pub` and `--offline` in argv | slices/mutation adapter |
| `internal/slices/mutation/adapters/freshenv/baseline.go:54,160` | Python-only baseline details; none for Dart [guess] | slices/mutation adapter |

Scenario: `@ID-MUT-209` Examples (`:1972-1976`, "the pub cache, run with --no-pub"); steps
`features/mutate_language_tools.go:28` regex, `mutate_language_provenance.go`.

### 2.10 Settings / config

`config/` and `itos-cc.yaml` hold no language tables (grep finds none). `exclude` (decision 45) is the existing
tool for generated files and platform folders. If the spec wants defaults, they go either in `lang.Detect`
(generated suffixes) or in discovery (platform folders), not in config [decision]. `docs/CONFIG.md` changes only
if a Dart-specific setting is added.

### 2.11 Docs

| where | what |
|---|---|
| `README.md:3-4` | language list (one line every item would want to touch: give it to the closing docs item) |
| `README.md:154-164` crap, `:234-294` integration coverage, `:334-354` own tests, `:504-549` strict, `:709-764` evidence and suite roots, `:840-855` reach, `:885-894` scrap, `:911-916` graph modules | per-feature paragraphs naming each language |
| `docs/CLI.md:32` exit table, `:158-168`, `:213-239`, `:263-330`, `:389`, `:505` (rules 34-35 variables) | contract prose naming languages |
| `docs/go.md:908` | Kotlin runner wrapper note. No "adding a language" section exists (gap: worth one). **[now]** `docs/architecture.md` has it |
| `features/README.md:81-90` | tools each language's binary-driver scenarios need |
| `cmd/itos-cc/main.go:1` | package doc "measures code quality in TypeScript, Python, Kotlin, and Go" |
| `IDEAS.md:148-150` | grammar pin note |
| ~~new decisions~~ | ~~amend or extend decisions 23 (own tests by import), 24 (transitive reach), 25 (strict lines), 26/27 (collectors, child processes) to name Dart; q-numbers through `itos decision`~~ **[now]** no decision was amended: q-74..q-83 and the slices' scenarios hold Dart's |
| **[now]** | T-181 named Dart in the help and `main.go:1`, and T-182 in every other row above |

### 2.12 Real tools, CI, pinned installs

**[now]** `cd.yml` gained a `dart` job of its own on ubuntu-latest, not a languages shard:
`kuhnroyal/flutter-fvm-config-action` reads the root `.fvmrc` (Flutter 3.47.7) into
`subosito/flutter-action` with `cache: true`, both pinned by SHA, and it runs the slices' Dart
scenarios by their tags with `ITOS_CC_FLUTTER_TOOLS`. No Dart case was added to
`language_tools_test.go` and no Dart smoke test or `testdata/smoke/dart/` exists: the scenarios run
the real tool. macOS and Windows are the idea dart-cd-other-os.

| where | what |
|---|---|
| `.github/workflows/cd.yml:212-345` `languages` job | a `dart` matrix shard (or `flutter: true` flag as `kotlin:` at `:242-246`) with `subosito/flutter-action` pinned by digest and `flutter-version` pinned (Renovate manages only `gomod`, `github-actions`, `renovate-config`: `.github/renovate.json5:8`, so the Flutter version is a manual pin like `gradle-version: "9.8.1"` at `:257`); `flutter pub get` warming the pub cache for fixtures; `go test` selectors for the Dart scenario IDs and cmd tests |
| `cmd/itos-cc/language_tools_test.go:39-100` `languageTools` | `case "dart"`/`"flutter"`: `flutter` on PATH |
| `cmd/itos-cc/language_smoke_test.go:111-116` | `TestLanguageToolSmokeDart` |
| `cmd/itos-cc/testdata/smoke/dart/` (new) | a minimal Flutter app: `pubspec.yaml`, `pubspec.lock`, `lib/`, `test/`. No `android/`/`ios/` unless testing the exclusion |
| `features/binary.go:85-211` | see 2.3 |
| `testdata/tools/` | node and python pins live here; Flutter needs nothing here (the SDK comes from the action) [guess] |
| `@binary-only` scenarios | the real-tool ones (counted offline run, cache hygiene like MUT-214, parallel data like MUT-233) carry `@binary-only` with a reason in `features/README.md` (`:42-43`, `:1257`) |

No `.feature` file is per-language: Dart joins existing files as Examples rows or new scenarios, so
the scenario registration (`features/steps_gen.go`, which `tools/bin/assemble` writes from each
step file's `featureFile` declaration since T-164, replacing `steps.go`'s `Converted`/`InProcess`
hand lists) and `features/smoke.yaml` need **no** edit; only a new `.feature` file would add a
declaration beside its steps and rerun assemble.
Scenario IDs continue each file's sequence (next free IDs are the coordinator's to hand out to avoid
collisions).

---

## 3. Shared files: where parallel agents would collide

| file | why shared | who should own it |
|---|---|---|
| `go.mod`, `go.sum` | grammar dependency | the lang item only |
| `internal/lang/lang.go` | engine changes (unit span, Syntax callee, Detect) | the lang item only |
| `internal/lang/dart.go` | the whole Spec: units, decisions, syntax, mutations, imports | the lang item (later items read it; a follow-up edit waits) |
| `internal/project/project.go:19-36` + `internal/selection/port/porttest/tree.go:75-90` | twin tables under one contract | the selection item, one commit |
| `internal/covrun/run.go` (markers + `buildPlans` switch) | every coverage change | the coverage item only; it also exports `DartTests` for mutation |
| `internal/slices/mutation/domain/coverage.go`, `command_policy.go`, `inputs.go` | mutation domain tables | split: commands/toolchain item gets `command_policy.go`/`inputs.go`; strict item gets `coverage.go`/`run_coverage.go` |
| `internal/slices/mutation/domain/porttest/*.go` (tools, fingerprints, scope, reach, workers, input_contract) | fakes mirroring every adapter | per file: tools/workers → commands item; fingerprints/scope/input_contract → strict item; reach → reach item |
| `features/mutate.feature` | most rows | many items add rows to **different** Examples tables, so git merges them when hunks are not adjacent; coordinator rebases in order [guess: low risk] |
| `features/mutate_language.go:23` | hand list of step groups | one line per new group: small conflicts |
| `features/binary.go` `Coverage` switch | tool checks | the coverage item |
| `cmd/itos-cc/testdata/mutation-help.json`, `internal/slices/mutation/mutation.go:70,87`, `run_contract.go`, `check_contract.go` | help text naming languages | one item (the strict item, or a closing docs item) |
| `README.md`, `docs/CLI.md`, `features/README.md` | prose naming languages | each item edits only its own section; `README.md:3` and `main.go:1` go to the closing item |
| `.github/workflows/cd.yml` | language shards and selectors | the CI item; later items append their scenario IDs to its selector (conflict-prone single lines `:314,320`) — better: a Dart shard with its own selector lines. **[now]** a `dart` job selecting by slice tags |
| `tasks/*.yaml`, `docs/decisions/` | ledger and ADRs | coordinator only (`itos work`, `itos decision`) |
| generated `cmd/itos-cc/commands_gen.go` | assembly | untouched: no new command |

---

## 4. Proposed split and order

**[now]** The split held: D0 became q-74..q-78 and q-83, D1 to D10 the slices slice-28 to slice-38 (slice-38
closing a freshness gap slice-36 left), and D11 T-181 (the help) and T-182 (the docs). All are done
but D8, whose Linux job runs and whose macOS and Windows runs are the idea dart-cd-other-os.

All items land after `slices-close` (ADR-0051 says the parallel-agents trial comes after T-86), on the
refactored paths. Each item puts its scenarios red first (AGENTS.md; Lessons: red tests compile against
the old product and fail at the behaviour).

**D0 — decisions (person, coordinator records; no code).** Grammar selection
(UserNobody14/tree-sitter-dart pin) and the unit model for signature+body siblings; namespace naming
(`pubspec` name prefix or not); Flutter-only first (no `dart test`); generated files and platform folders
(Detect, discovery default, or `exclude` docs); `integration_test/` never own tests; `.dart_tool` in
workers and exports; no child-process or per-test collectors (decisions 26/27 analogue); strict lines for
Dart now or later (inventory or fail closed); which commands first (the ledger asks: units, dry, scrap,
crap, mutation). ~~New ADRs or amendments for decisions 23-27.~~ **[now]** no ADR was written or amended; the
q-numbers and scenarios hold the decisions.

**Wave 1 (one agent):**
- **D1 lang**: `go.mod`/`go.sum`, `internal/lang/dart.go`, `lang.go` engine changes, lang tests;
  scenarios UNIT (new), CPLX-01 row + "Dart decisions", MUT "Dart sites" (sites need only lang + mutants
  `Sites`, which runs without tools), and the `features/complexity.go` regexes. Unblocks everything.

**Wave 2 (parallel after D1; disjoint files):**
- **D2 selection**: `internal/project/project.go` + selection porttest twin, `IsTest` rows (SEL-15), the platform-folder
  rule if decided in discovery. (Touches `dart.go` only if IsTest is adjusted: keep IsTest in D1.)
- **D3 dry**: DRY-02 row + `features/dry.go` regexes (likely no product change; it can fold into D1 if small).
- **D4 scrap**: `internal/slices/scrap/domain/{extract,measure,analyze}.go`, scrap.feature rows and widget-test scenario,
  `features/scrap*.go`.
- **D5 coverage + crap**: `internal/covrun/run.go`, `internal/covrun/dart.go`, `internal/covrun/inventory.go` (if decided),
  `internal/coverage/porttest/measurer.go`, `cmd/itos-cc/scenarios_fakes_test.go`, `features/binary.go`,
  `features/coverage*.go`, COV-04/05/08 rows, crap help text. Exports `DartTests`.
- **D6 reach** (needed `slices-import-engine`, now T-162 and T-163, done): Dart resolution in the import engine (package:,
  relative, export, part/part of, conditional imports, `flutter_test_config.dart`), `internal/slices/mutation/domain/porttest/reach.go`,
  `features/imports.go`, MUT-215/219 rows, `features/mutate_language_reach*.go`.

**Wave 3:**
- **D7 mutation commands** (needs D5, D6): `internal/slices/mutation/domain/command_policy.go`, `inputs.go`,
  `fresh_commands.go`, `adapters/freshenv/preparation.go:524`, `adapters/testtools/toolchain.go`,
  `domain/porttest/{tools,workers}.go`, `adapters/worktrees/worker.go` (`.dart_tool`, Pods), MUT-07 row, a Dart own-tests outline, `features/mutate_language.go:23`.
- **D8 CI real tools** (needs D5; extend after D7): `cd.yml` Dart shard, `language_tools_test.go`,
  `language_smoke_test.go`, `testdata/smoke/dart/`. Run it early, because Flutter's start-up cost sets shard sizes.
  **[now]** a `dart` job on Linux alone, with no smoke test (2.12); macOS and Windows are the idea dart-cd-other-os.

**Wave 4 (parallel after D7):**
- **D9 strict evidence**: `internal/slices/mutation/domain/coverage.go`, `run_coverage.go`, `adapters/testinputs/*` (inputs,
  scope, suite), `domain/porttest/{fingerprints,scope,input_contract}.go`, MUT-204/205/206/207/223/224/247/258 rows, mutation help
  text and `mutation-help.json`.
- **D10 counted runs**: `internal/slices/mutation/adapters/freshenv/dependencies.go`, `CommandEnvironment`,
  `domain/fresh_admission.go:100-110`,
  MUT-209 row (binary-only real tools).

**Wave 5:** **D11 docs close**: `README.md:3`, `cmd/itos-cc/main.go:1`, any cross-cutting README/CLI prose
the items left, `IDEAS.md:148`, and a new "adding a language" section in `docs/go.md` (a gap this survey hit).
**[now]** The section is `docs/architecture.md`'s "Adding a language".

Dependency graph:

```
D0 ──> D1 ──┬──> D2
            ├──> D3
            ├──> D4
            ├──> D5 ──┬──> D7 ──┬──> D9 ──┐
            │         └──> D8   └──> D10 ─┼──> D11
            └──> D6 ──────> D7            │
 (slices-import-engine) ──> D6            │
 D8 also follows D7 for the mutation smoke ┘
```

Collision risks that remain inside the split: `features/mutate.feature` (D1, D6, D7, D9, D10 add rows),
`features/mutate_language.go:23` (D6, D7, D9, D10), and `cd.yml` selectors once D8 exists (D9, D10 add IDs).
Run D9 and D10 one after the other if the coordinator wants zero rebase work.

---

## 5. Things to note for the coordinator

- The memory note "one implementing agent in this repo at a time; no worktrees unless asked" is the
  current rule. This split assumes the parallel-worktree trial that `slices-close` sets up. **[now]** q-78 allowed
  it after T-86.
- `internal/covrun/run.go` imported `mutants.MavenTests`/`GradleTests` when this was surveyed; T-156 gave the
  runner its own copies, `gradleTests` and `mavenTests` (`internal/covrun/run.go:582-606`), so it imports no
  mutation package. D5 must add no coverage→slice import.
- Kotlin-in-Flutter (the `android/` Gradle root) is the most likely first-run surprise. Settle it in D0 before
  any crap scenario, or every strict Flutter run fails closed on an unmeasured Kotlin root. **[now]** Settled by
  q-75 (slice-30).
- Grammar maturity: the Dart grammar was revised as recently as 2026-10-18 (record-return members, primary
  constructors per its comments). Pin a commit and keep regression scenarios, as UNIT-11 does for Kotlin.
- ~~Gap worth an idea in this repo's registry (feedback rule, own tool): docs/go.md has no "Adding a language" checklist.
  This file is a first draft of one.~~ **[now]** `docs/architecture.md` has the checklist.
