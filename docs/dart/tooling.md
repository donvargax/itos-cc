# Dart and Flutter support in itos-cc: tooling research

> Research for the idea dart-support, kept as found (2026-10-20/21), not a spec: the items and their scenarios are the spec once written. The person's answers to q-74..q-78 settle scope; paths here predate the end of the slices refactor and are rechecked before Dart starts.

Date: 2026-10-20. Scope weighting from the person: **Flutter apps are the use case to support; pure
Dart packages come second.** Flutter material leads each section, and the pure Dart notes are kept short.

Evidence tags:
- **[verified]**: run on this machine (Dart SDK 3.13.5 stable, `dart --version`; Linux x64; Go 1.27.3).
- **[source]**: read in the tool's source code (Flutter `master` at commit `6597889`, 2026-10-21,
  sparse clone; the stable release at that time is Flutter 3.47.7 / Dart 3.13.5).
- **[doc]**: from the primary documentation page linked beside it.
- **[3rd-party]**: from another project's README or a measurement nobody here repeated.
- **[unverified]**: memory or inference that this research did not check.

Flutter is not installed here, and nothing was downloaded to install it (see the user rule on
toolchains). So every `flutter test` claim rests on the flutter_tools source, Flutter's docs, or
third-party measurements. None of it was run.

---

## 1. Parsing (tree-sitter grammar)

**Grammar.** Use [UserNobody14/tree-sitter-dart](https://github.com/UserNobody14/tree-sitter-dart).
It is the only maintained Dart grammar on GitHub. Active fork checks:
`tree-sitter-grammars/tree-sitter-dart` and `nvim-treesitter/tree-sitter-dart` both return 404. A
Codeberg fork exists for Rust crates ([issue #81](https://github.com/UserNobody14/tree-sitter-dart/issues/81)).

- **License.** `LICENSE` is MIT, "Copyright (c) 2020-2023 UserNobody14 and others", and GitHub
  detects it as MIT. `package.json` says `"license": "ISC"`, which is a metadata mismatch. Treat
  the grammar as MIT and keep its notice. [verified, from a clone]
- **Activity.** 199 commits, 107 stars, last push 2026-10-18, not archived. Recent commits:
  - `0038280` (2026-10-18), "require single-record type commas (#109)".
  - `6b86540` (2026-10-13), "support Dart 3.12/3.13 syntax (primary constructors, concise constructors)".
  - Dot shorthands (Dart 3.10) in 2026-03.
  - Extension types in 2025-10.
  
  [verified, `git log`]
- **No tags and no releases.** `gh release list` is empty and `git tag` is empty. The Go module
  resolves only to a pseudo-version: `github.com/UserNobody14/tree-sitter-dart v0.0.0-20261018035034-0038280b774b`.
  This works the same way as the repo's current `fwcd/tree-sitter-kotlin v0.0.0-2026…` pin. [verified]
- **ABI.** `src/parser.c` has `#define LANGUAGE_VERSION 15`, which go-tree-sitter v0.25 accepts.
  The grammar's `package.json` has a peer dependency on `tree-sitter ^0.25.0`. [verified]
- **Go bindings.** `bindings/go/binding.go` has the same shape as tree-sitter-go v0.25's binding:
  cgo includes `../../src/parser.c` and `scanner.c`, and `func Language() unsafe.Pointer`.
  - Its `go.mod` (go 1.22) requires `github.com/smacker/go-tree-sitter`, but only its
    `binding_test.go` imports smacker. In a consumer, `go get` + `go mod tidy` leave smacker out of
    `go.mod`. They do add smacker (and through it about a dozen tree-sitter-* grammars) to
    `go.sum` only. None of it is compiled. [verified]
  - **It works with `github.com/tree-sitter/go-tree-sitter v0.25.0`.** A probe built with
    `sitter.NewLanguage(dart.Language())` parsed every file below. [verified]
    - Cold build with cgo (core plus grammar): 10.8 s wall, 29 s CPU on 8 cores. [verified]
    - Probe binary: 4.4 MB. [verified]
    - `src/parser.c` is 7.1 MB of C source. [verified]
- **Dart 3 coverage.** Every check below was parsed by the probe with `HasError() == false`
  [verified]:
  - class modifiers: `sealed`, `final`, `base`, `interface`, `abstract mixin class`,
    `mixin class`, `abstract final`, `abstract base`
  - records (types, literals and named fields)
  - patterns: object, record, list with rest, map, cast, null-check `?`, null-assert, logical
    `||` / `&&`, relational, and `when` guards
  - switch expressions, `if (x case …)`, pattern assignment `(a, b) = (b, a)`, destructuring
    declarations
  - extension types; enhanced enums; dot shorthands (`.green`); primary constructors (`class P(final int x, final int y);`)
  - generic named constructor calls (`G<int, String>.named()`), `sync*` / `async*` / `yield*`,
    labels and `continue outer`, `late`, `external`, null-aware cascades (`?..`), `[...?l]`,
    `?[`, `{?a: 1}`, tear-offs
  
  Corpus checks, both with **0 files with parse errors** [verified]:
  - All 484 `.dart` files of [flutter/samples](https://github.com/flutter/samples) (2026-10-20).
  - 1,821 `.dart` files of `flutter/flutter` `packages/flutter/lib` + `packages/flutter/test` +
    parts of `flutter_tools` (53 MB).
  
  Throughput is about 1–3.4 MB/s single-threaded (12.4 MB in 3.7 s on `packages/flutter/lib`). [verified]
- **Macros and augmentations.** Dart stopped work on macros in January 2025
  ([dart.dev blog, "An update on Dart macros & data serialization"](https://dart.dev/blog/an-update-on-dart-macros-data-serialization)),
  so nothing needs parsing there. The grammar has no `augment` rule (`grep` gives 0). Augmentations
  are not a stable feature in Dart 3.13 [unverified]. Recommendation: treat them as out of scope.
- **Known gaps and quirks** that matter to itos-cc [verified unless noted]:
  - **A function is not one node.** A top-level function is `function_signature` followed by a
    sibling `function_body` under `program`. A method is `method_signature` + `function_body`
    siblings in `class_body`, `extension_body`, `enum_body`, or the `class_body` of a mixin or
    extension type. Abstract or bodiless members and initializer-only constructors
    (`C(this.a) : assert(a > 0);`) are a `declaration`. A local function is
    `local_function_declaration` (wrapping a `lambda_expression`). The unit node must therefore
    pair a signature with the next `function_body` sibling. `lang.walker.unit()` takes the span
    from one node, so the Dart spec either:
    - uses `function_body` as the unit node, takes the name from its previous sibling, and gives
      a start-line hook so the signature line counts; or
    - extends `Spec`.
    
    Note that constructor initializer lists (`: a = x ?? 0`) and default values sit in the
    signature, not the body.
  - **Constructor calls and plain calls look the same** without `new`/`const`: `Text('x')` parses
    as `identifier` + `selector(argument_part)`, the same as a function call
    ([issue #72](https://github.com/UserNobody14/tree-sitter-dart/issues/72)). Only `const X()`
    shows up as `const_object_expression`. Call and member syntax for dry fingerprints is
    therefore `selector`/`argument_part`, not `call_expression`.
  - **Patterns are partly flat.** In `case int n || double n when n > 0:`, the
    `logical_or_operator` and the guard sit directly in `switch_statement_case`. There is no
    `logical_or_pattern` node and no named guard node: `when` is an anonymous token. The
    `logical_or_operator` / `logical_and_operator` named nodes are shared by expressions and
    patterns, which suits counting.
  - **`on T { }` without `catch` has no node.** Inside `try_statement` it is an anonymous `on`
    token + `type_identifier` + `block`. `catch_clause` exists only when there is `catch (…)`.
  - **Null-aware forms are anonymous tokens:**
    - `?.` is `conditional_assignable_selector` with a `"?."` token.
    - `?[` is `conditional_assignable_selector` with a `"?"` token + `index_selector`.
    - `?..` is a `"?.."` token in `cascade_section`.
    - `...?` is `"..."` `"?"` tokens in `spread_element`.
    - `{?k: v}` is a `"?"` token in `pair`.
    - `??` is an `if_null_expression`.
    - `??=` is a token in `assignment_expression`.
  - Open issues, for example [#107](https://github.com/UserNobody14/tree-sitter-dart/issues/107)
    (a generic named constructor split at a type-argument comma, in a case the probe did not
    reproduce) and #73 (loop labels, which parsed fine here). Old segfault reports (#29, #56) come
    from Neovim and Dart FFI wrappers. Not reproduced here.
  - The grammar does not do semantic checking: a Dart program with type errors parses cleanly.
- **Alternatives if the module dependency is unwanted.**
  - **(a) Vendor the grammar.** Copy `src/parser.c`, `scanner.c` and `tree_sitter/*.h` into
    `internal/…/dartgrammar` with a three-line cgo binding, and keep the MIT notice (AGENTS.md:
    preserve third-party notices). That gives a pinned copy without smacker in `go.sum`, at the
    cost of 7 MB in the repo.
  - **(b) Run the Dart analyzer as a subprocess.** `package:analyzer` gives the exact AST and
    resolved types, which is what `dart_mutant` uses to pre-check mutants. It needs a Dart program
    shipped or run with `dart run`, with Dart startup per call. It is a poor fit for parsing (every
    other language uses tree-sitter in-process), but it is worth considering later as a mutant
    compile pre-check (section 6).

  **Recommendation:** the module pin, like Kotlin. Vendoring is the fallback.

## 2. Coverage

### Flutter apps (`flutter test`)
These flags are in the `flutter test` source (`packages/flutter_tools/lib/src/commands/test.dart`) [source]:
- `--coverage` collects coverage into `--coverage-path` (default `coverage/lcov.info`). The output
  is **LCOV only**: the formatter is `hitmap.formatLcov(resolver, reportOn:…, basePath: <cwd>)`,
  so `SF:` paths are **relative to the package directory** (`SF:lib/foo.dart`).
- `--branch-coverage` implies coverage and adds VM branch data (`BRDA`).
- `--merge-coverage` merges with `coverage/lcov.base.info`. It needs the external `lcov` tool.
- `--coverage-package <regexp>` (repeatable). When unset, coverage is limited to the current
  package **plus its pub-workspace member projects** (`_getCoveragePackages` → `project.workspaceProjects`).
- There is no `--function-coverage` flag in flutter test, and no raw JSON output.
- Ignore pragmas are applied automatically: `HitMap.parseJsonSync(…, checkIgnoredLines: true, …)`
  in `test/coverage_collector.dart`.
- Coverage from all test files is merged into one global hitmap, so one run gives **no
  per-test-file attribution**.
- Coverage turns on the VM service in each `flutter_tester` process.

### Pure Dart (`dart test`)
Package:test 1.33.0 / test_core 0.6.21 / coverage 1.16.0 [verified]:
- `dart test --coverage-path=<file>` writes **LCOV directly**, and `--branch-coverage` adds
  `BRDA`. Both arrived in package:test **1.27.0**, and `--coverage-package` in 1.29.0
  ([CHANGELOG](https://pub.dev/packages/test/changelog)). The project resolves `test` itself, so
  old lockfiles lack these flags.
  - `SF:` paths are **absolute**, unlike Flutter.
  - Ignore pragmas are applied: a line with `// coverage:ignore-line` disappeared from `DA`. [verified]
- `dart test --coverage=<dir>` writes **one VM JSON file per test suite**, for example
  `covdir/test/calc_test.dart.vm.json` with `{"type":"CodeCoverage","coverage":[{"source":…,"hits":[line,count,…]}]}`.
  That gives per-test-file coverage in one run. Convert it with
  `dart pub global run coverage:format_coverage --lcov --check-ignore -i <dir> --report-on=lib`. [verified, doc]
- `dart pub global run coverage:test_with_coverage [--branch-coverage] [--function-coverage]`
  writes `coverage/coverage.json` + `coverage/lcov.info`. In a workspace, pass the member test
  directories after `--`. ([pub.dev/packages/coverage](https://pub.dev/packages/coverage)) [doc]

### Shared behavior
- **Branch coverage exists but is not decision coverage.** Each `BRDA:<line>,0,0,<hits>` is a VM
  "block entry" with a hit count. Branches are not paired true/false.
  - Example: `if (x < 0) … else if (x == 0) …` gave `BRDA:1,0,0,1`, `BRDA:2,0,0,1` and `BRDA:4,0,0,2`. [verified]
  - Recommendation: line coverage only, as for the other languages.
- **Coverable lines follow VM tokens, not source lines.** A `return 0;` line inside an `else if`
  was never listed in `DA` even though it ran. [verified, observed and not explained]
- **Libraries that are never loaded are missing from the report.**
  - A `lib/unused.dart` that no test imported was absent from `lcov.info`.
  - A loaded library with uncalled functions appeared with `DA:n,0`. [verified]
  - The same holds for Flutter: it filters the same VM hitmap [source; long-standing Flutter
    complaint, issue number unverified].
  - itos-cc must therefore treat a `lib/` file absent from the LCOV as 0 % covered, not as
    unknown.
- **Ignore pragmas:** `// coverage:ignore-line`, `// coverage:ignore-start` … `// coverage:ignore-end`
  (inclusive), and `// coverage:ignore-file` ([coverage README](https://pub.dev/packages/coverage)) [doc].
  `flutter test` and `dart test --coverage-path` apply them. `format_coverage` applies them only
  with `--check-ignore`.

### Per-test coverage
- **Selecting one test:**
  - `dart test -N/--plain-name <substr>`, `-n/--name <regexp>`, `path?line=10&col=2` and
    `path?name=…` ([test README](https://pub.dev/packages/test)) [verified/doc].
  - `flutter test --plain-name`, `--name`, and `test/foo_test.dart?name=a&line=1` [source].
- **Per-test coverage** is one run per test with its own coverage path. That costs a full test
  start per test (seconds for Flutter, see section 3). Per *file* is cheaper:
  - Pure Dart: `--coverage=<dir>` gives it in one run.
  - Flutter: one `flutter test <file> --coverage --coverage-path=…` per file.
  
  Recommendation: per-file attribution, the same granularity the mutation runner uses.

### Subprocess / integration coverage
- There is **no `GOCOVERDIR` equivalent**: no environment variable makes child Dart processes
  write coverage [unverified absence; nothing like it is in the coverage README].
- Dart coverage comes from the VM service. Start the child with
  `dart --pause-isolates-on-exit --disable-service-auth-codes --enable-vm-service=N [--branch-coverage] script.dart`,
  then run `coverage:collect_coverage --uri=… --wait-paused --resume-isolates` (README) [doc].
- AOT-compiled executables (`dart compile exe`) have no VM service, so they produce no coverage
  [unverified].
- Flutter `integration_test` coverage on a device: the tool passes the same coverage watcher, but
  that path was not checked [unverified]. Out of scope anyway (section 3).

## 3. Running tests (for mutation and coverage)

### Flutter app
- **Commands** [source]:
  - All tests: `flutter test` from the package root. It recursively finds `test/**/*_test.dart`
    and only scans `test/`, "so that files with names like hit_test.dart don't get run".
  - One file: `flutter test test/foo_test.dart`.
  - One test: `--plain-name`, or `?name=`/`?line=`.
  - It requires `pubspec.yaml` in the working directory and a `flutter_test` or `test` dependency.
- **No device needed for unit and widget tests.**
  - They run in the host `flutter_tester` binary (`flutterTesterBinPath`), and `requiredArtifacts`
    is empty unless the run is an integration test. [source]
  - flutter/samples runs `flutter test` on plain `ubuntu-latest` and `macos-latest` without xvfb
    ([main.yml](https://github.com/flutter/samples/blob/main/.github/workflows/main.yml)). [verified, read]
- **integration_test/ needs a device** (an emulator, a real device, or a desktop target `-d linux`
  under xvfb on CI) ([docs](https://docs.flutter.dev/testing/integration-tests)) [doc].
  - `flutter test` refuses to mix unit and integration files in one call:
    "Integration tests and unit tests cannot be run in a single invocation". [source]
  - **Recommendation:** out of scope; never pass `integration_test/` paths.
- **Exit codes.** The runner result is passed through (`throwToolExit(null, exitCode: result)`) [source].
  These are package:test's codes, measured with `dart test` [verified]:

  | Code | Meaning |
  | --- | --- |
  | 0 | passed |
  | 1 | test failure **or a compile error** |
  | 79 | no tests ran / name matched nothing |
  | 64 | usage error |
  | 65 / 66 / 70 | data / no input / software error |

  A compile error shows up in `-r json` as an `error` event on the hidden test
  `"loading test/x_test.dart"` with text `Failed to load "…": lib/x.dart:4:12: Error: …` and
  `testDone result:"error"`. This separates *compile error* from *killed*. [verified]
- **Reporters** [source]:
  - `-r/--reporter compact|expanded|failures-only|github|json|silent`. The JSON protocol is at
    [json_reporter.md](https://github.com/dart-lang/test/blob/master/pkgs/test/doc/json_reporter.md).
  - `--file-reporter json:<path>`.
  - `github` is the default reporter on GitHub Actions.
- **Timeouts.**
  - `--timeout 60s|2x|none` and `--ignore-timeouts` [source].
  - `testWidgets` default under `flutter test`: **10 minutes**
    (`defaultTestTimeout = Timeout(Duration(minutes: 10))` in `flutter_test/lib/src/binding.dart`).
    Widget tests run on a fake clock. [source]
  - Plain `test()` default: 30 s of inactivity (`invoker.dart`) [verified, source].
  - A mutant with an endless loop that never yields is caught by neither, so itos-cc must kill the
    process tree on its own deadline.
- **Concurrency and caching.**
  - Each test file gets its own `flutter_tester` process. `-j/--concurrency` sets how many run in
    parallel (forced to 1 for integration and web).
  - All test files of one invocation are compiled by one incremental frontend server into
    `build/test_cache/…` [source].
  - The verbose-only `--experimental-faster-testing` runs files in lightweight engines [source].
  - `--no-pub` skips the implicit `pub get` [source].
  - `--no-test-assets` skips building `build/unit_test_assets` [source].
- **Startup cost.** [3rd-party: [dart_mutant README](https://github.com/t09tanaka/dart_mutant), Flutter 3.47 on a loaded 12-thread Mac]
  - About **5 s** from `flutter test` start until the first test file has run.
  - One `flutter test` start per mutant took **1.5–3 s**.
  - A mutant run as an extra test file inside one long start took about **0.5 s**.
  - `dart_mutant` therefore runs Flutter mutants as one test file each, in one `flutter test --no-pub --concurrency=1 --reporter json --file-reporter json:<f>` start. It falls back to one start per mutant for anything other than *Killed*.
  
  The comparable pure Dart number here: `dart test` on a one-file package took about 2 s wall, with
  about 1.8 s in suite load/compile [verified].
- **Worker copy layout** [verified for pure Dart; Flutter from source/docs]:
  - **Copy:** `lib/`, `test/`, `pubspec.yaml`, `pubspec.lock` (apps commit it; the
    `flutter create` `.gitignore` excludes `.dart_tool/`, `/build/`, `/coverage/` and
    `.flutter-plugins-dependencies`, not the lockfile
    ([template](https://github.com/flutter/flutter/blob/master/packages/flutter_tools/templates/app/.gitignore.tmpl))),
    `analysis_options.yaml`, `l10n.yaml`, `dart_test.yaml`, `test/flutter_test_config.dart`, the
    asset and font directories named in `pubspec.yaml` (`flutter: assets:`/`fonts:`, needed by
    `build/unit_test_assets`), golden files next to tests, and generated sources (`*.g.dart`,
    `*.freezed.dart`, l10n output) because tests compile against them.
  - **Leave out:** `android/`, `ios/`, `macos/`, `linux/`, `windows/`, `web/`, `build/`,
    `coverage/`, `integration_test/`, `test_driver/`, `.idea/`. flutter_tester does not use the
    platform folders for widget tests. A plugin's platform channels are not implemented in
    flutter_tester anyway [unverified as a blanket rule; native-assets hooks (`hook/build.dart`)
    of dependencies still build for the host].
  - **`.dart_tool/package_config.json`:**
    - Hosted packages are **absolute** `file:///home/…/.pub-cache/hosted/pub.dev/<pkg>-<ver>`.
    - The package itself is `"rootUri": "../"`.
    - Workspace members are relative (`"../pkgs/a"`).
    - Path dependencies are relative when declared relative (`cache.getDirectory(id, relativeFrom: …)`).
    - Pub also writes `pubCache`, plus `flutterRoot` and `flutterVersion` when Flutter is
      available ([pub entrypoint.dart](https://github.com/dart-lang/pub/blob/master/lib/src/entrypoint.dart)). [verified, source]
    - Pub treats the config as stale when Flutter has "moved" or been updated, or when the pub
      cache moved. [source]
  
  A copy into another directory on the same machine is therefore usable. A copy with fresh mtimes
  (`pubspec.yaml` newer than `package_config.json`) makes `dart test` run an implicit `pub get`.
  That rewrote `package_config.json` from the cache offline in about 2 s. A copy without
  `.dart_tool/` also resolves implicitly. [verified]

  **Recommendation:**
  - Copy, run `flutter pub get --offline` once per worker (or preserve mtimes), and then run
    every test with `--no-pub`.
  - Keep the worker's `build/test_cache` between mutants so the incremental compiler is reused.
  - Path dependencies outside the package (`path: ../shared`) break a copy that is not at the same
    relative depth. Copy the dependency, or rewrite to the absolute path on re-resolution.
- **Pub cache.** Default `~/.pub-cache`, or `%LOCALAPPDATA%\Pub\Cache` on Windows. It can be
  overridden with `PUB_CACHE` ([dart.dev env vars](https://dart.dev/tools/pub/environment-variables))
  [doc; Windows path unverified]. `dart pub get --offline` resolves from the cache only
  ([pub get](https://dart.dev/tools/pub/cmd/pub-get)) [doc, verified in practice].
- **Pub workspaces.** Introduced in **Dart 3.6** (not 3.5); `workspace:` globs arrived in 3.11
  ([dart.dev/tools/pub/workspaces](https://dart.dev/tools/pub/workspaces)) [doc]. A local workspace
  (`workspace: [pkgs/a, pkgs/b]`, members with `resolution: workspace`) showed [verified]:
  - The only `pubspec.lock` and `.dart_tool/package_config.json` are at the **root**.
  - Each member gets only `.dart_tool/pub/workspace_ref.json`, containing `{"workspaceRoot": "../../../.."}`.
  - `dart test` run inside a member works.
  
  Copying one member alone loses its resolution. A worker must copy the workspace root
  `pubspec.yaml`, `pubspec.lock`, root `.dart_tool/package_config.json`, and every member the
  package depends on, at the same relative paths. Alternatively it can copy the whole workspace
  minus platform folders.

  Flutter runs `flutter test` inside a member and includes workspace projects in the default
  coverage packages [source]. The Flutter minimum for workspaces is 3.27, the release that shipped
  Dart 3.6 [unverified].

### Pure Dart (brief)
`dart test`, `dart test test/x_test.dart`, `-N`, `-r json`, `--file-reporter json:<f>`,
`--timeout`, `--suite-load-timeout`, `-j`. It has the same discovery rule: `test/` and
`*_test.dart` (test README line 141). Exit codes are in the table above. [verified]

## 4. Project detection

- **Markers:**
  - `pubspec.yaml` is required.
  - `pubspec.lock` means the package is resolved.
  - `analysis_options.yaml` is common but optional.
  - `.dart_tool/package_config.json` means resolved locally.
  - `.metadata` is written by `flutter create` [unverified].
- **Flutter or pure Dart:** `dependencies: flutter: sdk: flutter` in `pubspec.yaml`.
  - A Flutter test project also has `dev_dependencies: flutter_test: sdk: flutter`.
  - Once resolved, `package_config.json` carries `flutterRoot`/`flutterVersion`.
  - **App or package:** an app has platform folders (`android/`, `ios/`, …), `lib/main.dart` and
    `publish_to: none` [template, partly unverified].
- **Test runner choice.** Use `flutter test` iff the package depends on the `flutter` SDK
  (`flutter_test` needs the Flutter engine); otherwise use `dart test`.
- **Monorepos:**
  - Pub workspaces: a root `pubspec.yaml` with `workspace:`, members with `resolution: workspace`.
  - **Melos 8.x** keeps its config under `melos:` in the root `pubspec.yaml`, has no
    `melos.yaml`, builds on pub workspaces, and needs Dart ≥ 3.9 ([pub.dev/packages/melos](https://pub.dev/packages/melos)) [doc].
  - Older Melos (≤ 6) used `melos.yaml` + `pubspec_overrides.yaml` [doc, inferred].
  
  Detection rule: each `pubspec.yaml` with `lib/` or `test/` is a package, and a root with
  `workspace:` is a workspace.

## 5. Complexity counting

**DCM** ([dcm.dev cyclomatic-complexity](https://dcm.dev/docs/metrics/function/cyclomatic-complexity/)) [doc]:
- Base 1. Adds for `if` (statement **and expression**, i.e. collection-if), `catch`, `do`,
  `while`, `for` (statement and expression, i.e. collection-for), and `? :`.
- Adds for each `case` in a switch statement **or expression**; "the `default` case and the last
  case don't count".
- Adds for the null-aware `?[]`, `?.`, `?..`, `...?`, `{?key: value}`, `{key: ?value}`.
- Adds for `&&` and `||` "in binary expressions or patterns", and for `??` and `??=`.
- Default threshold 15.

The other itos-cc specs for comparison (`internal/lang/typescript.go`, `kotlin.go`; read only) count:
- TypeScript: if, for, for-in, while, do, catch, ternary, `switch_case` (not default), `&&`,
  `||`, `??`, `?.`.
- Kotlin: when entries except `else`, `?.`, and elvis.
- Closures stay inside their enclosing unit, except TypeScript route callbacks.

**Proposed Dart decision set**, as tree-sitter-dart node kinds [verified kinds]:

| Counts +1 | Node / token |
| --- | --- |
| if statement (incl. `if (x case …)`) | `if_statement` |
| collection if | `if_element` |
| loops | `for_statement` (both C-style and for-in), `while_statement`, `do_statement` |
| collection for | `for_element` |
| catch handlers | each anonymous `on` token in `try_statement` + each `catch_clause` not preceded by `on` (or simply: each handler) |
| ternary | `conditional_expression` |
| switch statement case | `switch_statement_case` (not `switch_statement_default`) |
| switch expression arm | `switch_expression_case`, except an arm whose pattern is the bare wildcard `_` (`constant_pattern` → `identifier "_"`), the analogue of Kotlin `else` |
| `&&` / `||` (expressions and patterns) | `logical_and_operator`, `logical_or_operator` |
| `??`, `??=` | `if_null_expression`, `"??="` token |
| null-aware access | `conditional_assignable_selector` (`?.` and `?[`), `"?.."` in `cascade_section`, `"?"` in `spread_element` and `pair` |
| `when` guard | optional; DCM does not count it. Recommendation: do not count it (the case already counts) |

DCM excludes the *last* case of every switch, but itos-cc's TypeScript and Kotlin specs exclude
only default/else. Recommendation: exclude only `default` / `_`, for consistency across
languages, and record the difference from DCM.

**Flutter specifics:**
- **Build methods.** A `build` method is mostly one returned expression tree.
  `if_element`/`for_element` in `children: [...]`, `?:` and `??` are its branches, and callbacks
  (`onPressed: () => setState(…)`) are closures. With closures kept inside the enclosing unit (as
  for TypeScript and Kotlin), `build` absorbs all its callbacks' branches and often exceeds 15.
- **Coverage of build methods is usually high:** any widget test that pumps the widget runs
  `build`, so the CRAP of `build` tracks complexity more than coverage.
- **Recommendations:**
  - Count collection-if/for (as DCM does).
  - Keep closures inside the unit for consistency.
  - Consider reporting `build`, `createState`, `toString`, `==`/`hashCode` as ordinary units (no
    special-casing in v1).
- `?.` on nullable widget fields (`widget.title?.length`) is frequent in Flutter. Counting it
  matches TypeScript and Kotlin in itos-cc and matches DCM.

## 6. Mutation precedent

- **[mutation_test](https://pub.dev/packages/mutation_test)** 1.8.1 (BSD-3-Clause,
  [domohuhn/mutation-test](https://github.com/domohuhn/mutation-test), last push 2026-09-12) [doc, verified].
  - Text and regex rules from an XML file (`lib/src/configuration/builtin_rules.dart`, 41 rules):
    - `&&`↔`||`; `+=`/`-=`/`*=`/`/=`/`&=`/`^=` → `=`; `==`↔`!=`; `<=`→`==`/`<`; `>=`→`==`/`>`
    - `if (c)` → `if (!(c))`, plus negating chain operands
    - number → negative, `+`↔`-`
    - swapping 2–4 function arguments
  - Each mutant runs shell commands with an `expected-return`. Any other exit code means
    *detected*. There is a per-command `timeout`.
  - `--coverage lcov.info` skips unhit lines.
  - Reports in HTML, JUnit and xUnit.
  - Being regex-based, it is not AST-aware.
- **[dart_mutant](https://github.com/t09tanaka/dart_mutant)** 0.2.1 (MIT, 2026-10-13; tags
  "Dart and Flutter") [3rd-party README]. It is the strongest precedent.
  - Syntax mutators, contract-compatible with `test_gates` 0.1.0:
    - `ArithmeticOperator` (`+`↔`-`, `*`↔`/`, `%`→`*`, `~/`→`*`)
    - `EqualityOperator` (`<`→`<=`/`>=`, …, `==`↔`!=`)
    - `LogicalOperator` (`&&`↔`||`, `a ?? b`→`b`)
    - `ConditionalExpression`: the condition of `if`, `?:` and collection `if` → `true`/`false`;
      loops → `false`
    - `BooleanLiteral`, `UnaryOperator` (`-a`→`a`), `UpdateOperator`
    - `AssignmentOperator` (incl. `??=`→`=`)
    - `BlockStatement` (`{…}`→`{}`)
    - `RemoveCall` (unused-result call → `;`)
    - `StringLiteral`, `ArrayDeclaration` (`[a, b]`→`[]`)
  - Type-aware mutators (`MethodExpression` on num/String/Iterable/Map/DateTime/Duration,
    `NumberLiteral`, guard mutators, `ReturnValue`) need the analyzer.
  - Practices worth adopting:
    - Pre-check each mutant for compile errors with `package:analyzer` in memory. Over 10 % of
      syntax mutants did not compile, because of null safety and type promotion after `if (x == null) return`.
    - Per-test timeout 10 s; run deadline 3× baseline + 20 s.
    - A memory cap: 2× baseline + 1 GB.
    - Tests mapped by path mirror (`lib/a/b.dart` → `test/a/b_test.dart`).
    - The Flutter one-start-many-mutants batching in section 3.
    - Stryker JSON output.
- **[radioactive_dart](https://github.com/ricardoboss/radioactive_dart)** (2026-08-21) [3rd-party README].
  - Pure Dart only.
  - Routes mutants to the test files that cover their line, using coverage it collects itself.
  - Treats timeouts as inconclusive.
  - Requires package:test ≥ 1.24.6.
- **Mapping for itos-cc's tree-sitter `Mutations` spec** [verified kinds]:
  - `SwapParents`: the operator tokens are named nodes: `additive_operator`,
    `multiplicative_operator`, `relational_operator`, `equality_operator`, `logical_and_operator`,
    `logical_or_operator`.
  - `??` is an anonymous token in `if_null_expression`.
  - Deletions: `negation_operator` (`!`), `minus_operator`.
  - Literals: `decimal_integer_literal`, `decimal_floating_point_literal`, `true`, `false`.
  - Optional chaining: `?.` → `.` mostly does **not compile** under null safety (a receiver of
    nullable type). Expect compile errors here, so either skip it or rely on the compile-error
    classification.

## 7. CI install (GitHub Actions)

- **Flutter:** [subosito/flutter-action](https://github.com/subosito/flutter-action) `@v2` (latest
  `v2.23.0`, 2026-03-25) [doc, verified via API].
  - Pin with `flutter-version: 3.47.7` (an exact version, or `3.x`) + `channel: stable`, or with
    `flutter-version-file: pubspec.yaml`. The file option needs an exact version in the pubspec
    and installs `yq`, which is missing on Windows runners.
  - Caching: `cache: true` for the SDK and `pub-cache` for dependencies, via `actions/cache@v5`.
  - Pin the action by commit SHA, as flutter/samples does.
  - **Archive sizes** for stable 3.47.7 (HEAD requests to `storage.googleapis.com/flutter_infra_release`) [verified]:
    - Linux `.tar.xz`: **1.58 GB**
    - macOS x64/arm64 `.zip`: **2.26 GB**
    - Windows `.zip`: **1.93 GB**
    
    A cached SDK uses that much of the 10 GB per-repo Actions cache, once per OS.
  - **Install time**, from flutter-action's own CI
    ([run 25140539065](https://github.com/subosito/flutter-action/actions/runs/25140539065), 2026-04-30;
    whole job durations for an uncached stable install from github.com/flutter/flutter) [verified via API]:
    - ubuntu-latest: about **67 s**
    - macos-latest: about **140 s**
    - windows-latest: about **153 s**
    - The `test_cache` jobs: 46 s / 90 s / 97 s.
- **Dart only:** [dart-lang/setup-dart](https://github.com/dart-lang/setup-dart) `@v1` (latest
  `v1.8.1`, 2026-08-28) [doc, verified].
  - `sdk: 3.13.5 | 3.13 | stable | beta | dev | main`; `flavor`; `architecture`.
  - No caching is documented, and it does not install Flutter.
  - SDK zip sizes are about 220–239 MB.
  - Its install step took **5–14 s** on ubuntu, macos and windows in its CI
    ([run 39083659082](https://github.com/dart-lang/setup-dart/actions/runs/39083659082)) [verified via API].
- **For itos-cc's own CI:** the test fixtures need Flutter only for `flutter test` scenarios.
  - Gate them behind an opt-in like the other binary drivers (T-153), on ubuntu.
  - Keep pure Dart and parse-only scenarios on setup-dart, which is about 10× faster.
  - Windows and macOS Flutter jobs add about 2.5 min of install each before caching.

## 8. Generated code

- **Common generated files:**
  - build_runner / source_gen: `*.g.dart` (json_serializable, riverpod_generator, …),
    `*.freezed.dart`, `*.mocks.dart` (mockito), `*.gr.dart` (auto_route), `*.config.dart`
    (injectable) [unverified list beyond freezed and json].
  - protoc: `*.pb.dart`, `*.pbenum.dart`, `*.pbjson.dart`, `*.pbserver.dart`.
  - Flutter: `lib/generated_plugin_registrant.dart`, and l10n output.
- **freezed** writes `// GENERATED CODE - DO NOT MODIFY BY HAND` + `// coverage:ignore-file` into
  every `.freezed.dart` header ([freezed builder.dart](https://github.com/rrousselGit/freezed/blob/master/packages/freezed/lib/builder.dart)) [verified].
- **json_serializable** does not add it by default. Its README tells users to add it with a
  `build.yaml` `source_gen:combining_builder` `preamble: // coverage:ignore-file`
  ([README](https://github.com/google/json_serializable.dart/tree/master/json_serializable)) [verified].
- **source_gen** headers start with `// GENERATED CODE - DO NOT MODIFY BY HAND` [verified for freezed; general claim unverified].
- **l10n:** on Flutter master, `synthetic-package` (the `package:flutter_gen` output) has been
  **removed**: setting it true is an error, and false gives a warning
  (`localizations_utils.dart`) [source]. Generated `app_localizations*.dart` therefore now lands
  in the source tree (default `lib/l10n`, or `output-dir` in `l10n.yaml`). The docs page still
  describes the synthetic package ([i18n docs](https://docs.flutter.dev/ui/internationalization)) [doc, stale].
- **How coverage tools exclude:**
  - Only through the pragmas (`coverage:ignore-file`).
  - Or by post-filtering the LCOV (`lcov --remove` and similar).
  - There is no built-in glob exclusion in `flutter test` or `format_coverage`.
  - `--coverage-package` filters only by package.
  - DCM and the analyzer use `analysis_options.yaml` `analyzer: exclude:` globs.
- **Recommendation for itos-cc:**
  - Exclude by default:
    - files whose first lines contain `GENERATED CODE - DO NOT MODIFY BY HAND`, `Generated by`,
      or `coverage:ignore-file`
    - the globs `**/*.g.dart`, `**/*.freezed.dart`, `**/*.mocks.dart`, `**/*.gr.dart`, `**/*.pb*.dart`
    - `lib/generated_plugin_registrant.dart`
  - Honor `coverage:ignore-*` lines as not coverable when computing CRAP (Flutter's LCOV already
    drops them).
  - Consider reading `analyzer: exclude:`.
  - Keep generated files in worker copies, since tests compile against them, but never mutate them.

---

## Decisions the spec has to make (with recommendation)

1. **Grammar source.** Recommendation: pin `github.com/UserNobody14/tree-sitter-dart` by
   pseudo-version (currently `v0.0.0-20261018035034-0038280b774b`), as for Kotlin. Record the MIT
   notice, and accept smacker entries in `go.sum` only. Fallback: vendor `src/` with the notice.
2. **Unit node shape.** The grammar splits signature and body. Recommendation: the unit is the
   `function_body` named from its preceding `function_signature`/`method_signature` sibling, with a
   hook so the unit's start line is the signature's. Also count `local_function_declaration` and
   closures inside the enclosing unit. Abstract members are not units.
3. **Unit naming and namespace.** Recommendation: namespace `package:<pubspec name>/<path under lib>`
   (or the relative path outside `lib/`), with class, mixin, extension, enum and extension type as
   containers. `Separator: "."`. Name constructors `C.named` and unnamed ones `C`; getters and
   setters by name (setter `s=`?), operators `operator +`. Private means a leading `_`.
4. **Decision set.** Recommendation: the table in section 5. Count collection-if/for, `??`, `??=`
   and the null-aware forms. Skip `default` and the bare `_` arm. Do not count `when`.
5. **Which runner.** Recommendation: `flutter test` when `pubspec.yaml` depends on the `flutter`
   SDK, else `dart test`. Flutter apps first. Exclude `integration_test/` (it needs a device).
6. **Coverage format and path base.** Recommendation:
   - Flutter: `flutter test --coverage --coverage-path=<tmp>/lcov.info --no-pub`. `SF` paths are
     relative to the package dir.
   - Dart: `dart test --coverage-path=<tmp>`, with absolute `SF` paths; requires test ≥ 1.27.
     Otherwise `--coverage=<dir>` + `format_coverage`.
   - Normalize both.
   - Line coverage only; ignore BRDA.
   - Treat `lib/` files missing from LCOV as 0 %.
7. **Coverage attribution granularity.** Recommendation: per test file (one `flutter test <file>`
   run each when needed). Do not do per-test-name coverage: about 2–5 s of Flutter startup per
   test is too costly.
8. **Integration/subprocess coverage.** Recommendation: unsupported in v1. There is no `GOCOVERDIR`
   analogue, and VM-service collection is out of scope.
9. **Mutation run strategy for Flutter.** Recommendation:
   - v1 runs one `flutter test --no-pub --fail-fast -r json --file-reporter json:<f> --timeout 10s <mapped test files>`
     per mutant in a worker copy that keeps `build/test_cache`.
   - Classify compile errors from the JSON `error` on the `loading …` test, not from the exit code
     (a compile error gives 1, the same as a failure).
   - Enforce an itos-side deadline: the widget-test default is 10 min.
   - Consider dart_mutant's many-mutants-per-start batching later.
10. **Test selection per mutant.** Recommendation: coverage-routed test files (from decision 7).
    Fall back to the path mirror (`lib/a/b.dart` → `test/a/b_test.dart`), then the whole `test/`.
11. **Worker copy contents.**
    - Recommendation: copy the package minus `android ios macos linux windows web build coverage integration_test test_driver .idea`.
    - Keep `.dart_tool/` or run `flutter pub get --offline` once per worker, then use `--no-pub`.
    - For a pub-workspace member, copy the workspace root files and the members it depends on at
      the same relative paths.
    - Out-of-tree `path:` dependencies: copy them or re-resolve.
12. **Mutation operators.** Recommendation: start with the AST-safe subset that maps to existing
    `Mutations` fields:
    - arithmetic, equality and relational swaps
    - `&&`↔`||`, `??` → right operand
    - `!` and unary `-` deletion
    - boolean and integer literals
    - collection-if condition → true/false
    
    Leave out `?.`→`.` (mostly non-compiling). Expect more than 10 % compile errors, and report
    them as their own status. An analyzer pre-check is a later option.
13. **Generated code.** Recommendation:
    - Default-exclude files by header marker or glob from all metrics and mutation.
    - Honor `// coverage:ignore-line/start/end/file` as not coverable.
    - Keep them in worker copies.
14. **Project detection and monorepos.** Recommendation:
    - A package is any `pubspec.yaml` with `lib/` or `test/`.
    - A workspace root is a pubspec with `workspace:`.
    - Melos 8 needs nothing extra, since it is pub workspaces.
    - Ignore `.dart_tool/` and `build/` when walking.
15. **Test-file detection (`IsTest`).** Recommendation: `*_test.dart` anywhere, and everything under
    `test/`, `integration_test/` and `test_driver/`. Treat `test/flutter_test_config.dart` and
    `test/**/helpers` as test support, not product.
16. **Minimum versions.** Recommendation: Dart ≥ 3.6 (workspaces) and Flutter ≥ 3.27 [unverified
    pairing] for workspace support. The `dart test --coverage-path` path needs package:test ≥ 1.27,
    else falls back to `--coverage` + format_coverage.
17. **CI for itos-cc's own Dart scenarios.** Recommendation: setup-dart for parse and pure-Dart
    scenarios (about 10 s). subosito/flutter-action pinned by SHA with `flutter-version: 3.47.7` and
    `cache: true`, on ubuntu only and behind the binary-driver opt-in (T-153). Note the 1.6–2.3 GB
    SDK archives against the 10 GB cache.

