Feature: Choosing which files a command looks at
  Every command takes paths, path fragments, or --changed, and splits what it
  finds into production code and test code by each language's conventions.

  Background:
    Given a project with TypeScript, Python, Kotlin, and Go sources

  @ID-SEL-01
  Scenario: Without paths the working directory is analyzed
    When I run a command with no paths
    Then every supported file under the working directory is selected

  @ID-SEL-02
  Scenario: Paths may be files or directories
    When I run "itos-cc crap src/billing src/cart/total.ts"
    Then the files under src/billing and the file src/cart/total.ts are selected

  @ID-SEL-03
  Scenario: A path that does not exist is a fragment
    Given there is no file or directory called "billing"
    When I run "itos-cc crap billing"
    Then every source whose path contains "billing" is selected, such as src/billing/invoice.ts

  @ID-SEL-04
  Scenario: Selecting what git reports as changed
    Given git reports src/a.ts as modified, src/b.py as staged, and src/c.go as untracked
    And src/d.kt was deleted
    When I run "itos-cc crap --changed"
    Then src/a.ts, src/b.py, and src/c.go are selected
    And src/d.kt is not selected

  @ID-SEL-05
  Scenario: --changed from a subdirectory selects the changes under it
    Given git reports src/a.ts and lib/b.ts as modified
    And src/año nuevo.ts as untracked
    When I run "itos-cc crap --changed" in src
    Then a.ts and año nuevo.ts are selected
    And lib/b.ts is not selected

  @ID-SEL-06
  Scenario: --changed works before the first commit
    Given a git repository with staged files and no commits
    When I run a command with --changed
    Then the staged files are selected

  @ID-SEL-07
  Scenario: --changed outside a git repository is a missing environment
    Given the working directory is not a git repository
    When I run "itos-cc crap --changed"
    Then stderr says "itos-cc: --changed needs a git repository: …"
    And the problem is "changed.no-git"
    And the exit code is 82

  @ID-SEL-08
  Scenario: An argument that names nothing is a usage error
    Given no file's path under the working directory contains "nosuch"
    When I run "itos-cc crap nosuch"
    Then the problem is "paths.unmatched", with argument "nosuch"
    And the exit code is 80
    # a fragment that matches only test files still selects them

  @ID-SEL-09
  Scenario: --changed with nothing changed selects nothing
    Given git reports no changes
    When I run "itos-cc crap --changed"
    Then stderr says "itos-cc: no source files to score"
    And the exit code is 0

  @ID-SEL-10
  Scenario: Dependencies, caches, and fixtures are never walked
    Given the project contains these directories, each holding source files:
      | directory     |
      | node_modules  |
      | vendor        |
      | .venv         |
      | venv          |
      | __pycache__   |
      | .metrics      |
      | testdata      |
      | .git          |
      | .any-hidden   |
    When the project's files are discovered
    Then nothing inside those directories is selected

  # Binary-only for good: what git ignores is the disk adapter's question to
  # the real git (git check-ignore), which no fake answers.
  @ID-SEL-11 @binary-only
  Scenario Outline: Build output is skipped, a package with its name is not
    Given a directory "<directory>" holding source files
    When the project's files are discovered
    Then its files are <selected>
    # build, dist, target, out, and coverage are build output names, and
    # also ordinary package names; mutation run copies the same directories

    Examples:
      | directory                                   | selected     |
      | web/dist, beside web/package.json           | not selected |
      | target, beside pom.xml or Cargo.toml        | not selected |
      | app/build, beside app/build.gradle.kts      | not selected |
      | gen/out, which git ignores                  | not selected |
      | internal/out, a Go package                  | selected     |
      | out, beside go.mod only                     | selected     |

    # slice-30: pubspec.yaml is a build manifest, so a Dart package's build
    # and coverage folders are build output too.
    @slice-30
    Examples:
      | directory                                   | selected     |
      | app/build, beside app/pubspec.yaml          | not selected |
      | app/coverage, beside app/pubspec.yaml       | not selected |

  @ID-SEL-12
  Scenario: A file named directly is taken even inside a skipped directory
    When I run "itos-cc units testdata/board.go"
    Then testdata/board.go is analyzed

  @ID-SEL-13
  Scenario: JavaScript files are TypeScript to every tool
    Given the files src/a.js, src/b.jsx, src/c.mjs, and src/d.cjs
    When the project's files are discovered
    Then each is selected as TypeScript and parsed with the JavaScript grammar

  @ID-SEL-14
  Scenario: TypeScript declaration files and minified bundles are not source
    Given the files src/types.d.ts and public/vendor.min.js
    When the project's files are discovered
    Then neither is selected

  @ID-SEL-15
  Scenario Outline: Telling test code from production code
    When the file "<path>" is discovered
    Then it is classified as <kind>

    Examples:
      | path                                 | kind       |
      | src/board.ts                         | production |
      | src/board.test.ts                    | test       |
      | src/board.spec.tsx                   | test       |
      | src/__tests__/board.ts               | test       |
      | src/board.js                         | production |
      | src/board.test.mjs                   | test       |
      | src/board.spec.cjs                   | test       |
      | pkg/board.py                         | production |
      | pkg/test_board.py                    | test       |
      | pkg/board_test.py                    | test       |
      | tests/helpers.py                     | test       |
      | conftest.py                          | test       |
      | board.go                             | production |
      | board_test.go                        | test       |
      | src/main/kotlin/demo/Board.kt        | production |
      | src/test/kotlin/demo/BoardTest.kt    | test       |
      | src/test/kotlin/demo/Fixtures.kt     | test       |
      | app/tests/Fixtures.kt                | test       |
      | src/main/kotlin/demo/BoardTests.kt   | test       |
      | src/main/kotlin/demo/BoardSpec.kt    | test       |

    # slice-28: a Dart test is a _test.dart file anywhere, or any file under
    # test/ (helpers and flutter_test_config.dart too), integration_test/ or
    # test_driver/.
    @slice-28
    Examples:
      | path                                 | kind       |
      | lib/board.dart                       | production |
      | lib/src/board_test.dart              | test       |
      | test/board_test.dart                 | test       |
      | test/helpers/fake_board.dart         | test       |
      | test/flutter_test_config.dart        | test       |
      | integration_test/app_test.dart       | test       |
      | test_driver/integration_test.dart    | test       |

  # slice-30, q-75 (a): a pubspec.yaml whose dependencies name the flutter
  # SDK makes the folders beside it that flutter create writes for each
  # platform part of the Flutter app. android holds MainActivity.kt and a
  # Gradle build, which would read as a Kotlin project that crap and
  # mutation plan Gradle for; none of it is the app's Dart. Beside a pure
  # Dart pubspec, or with no pubspec, a folder of those names is ordinary.
  @slice-30 @ID-SEL-22
  Scenario: A Flutter app's platform folders are not walked
    Given a Flutter app whose pubspec.yaml depends on the flutter SDK
    And its android, ios, macos, linux, windows, and web folders each hold source files
    When the project's files are discovered
    Then nothing inside those folders is selected
    And its lib folder's files are selected
    But a linux folder beside a pubspec.yaml that does not depend on the flutter SDK is selected

  # slice-30, q-77 (a): generated Dart is not source, as declaration files
  # and minified bundles are not (ID-SEL-14), for every command and for
  # test files too: build_runner's and protoc's suffixes, the plugin
  # registrant, a file whose leading comments carry source_gen's marker,
  # and what flutter gen-l10n writes beside an l10n.yaml (its
  # output-localization-file, default app_localizations.dart, and the
  # per-locale files beside it, in output-dir, default lib/l10n).
  @slice-30 @ID-SEL-23
  Scenario Outline: Generated Dart is not source
    Given a Flutter app holding <file><written>
    When the project's files are discovered
    Then <file> is <selected>

    Examples:
      | file                                  | written                                              | selected     |
      | lib/user.g.dart                       |                                                      | not selected |
      | lib/user.freezed.dart                 |                                                      | not selected |
      | test/user_test.mocks.dart             |                                                      | not selected |
      | lib/router.gr.dart                    |                                                      | not selected |
      | lib/injection.config.dart             |                                                      | not selected |
      | lib/api/user.pb.dart                  |                                                      | not selected |
      | lib/api/user.pbenum.dart              |                                                      | not selected |
      | lib/generated_plugin_registrant.dart  |                                                      | not selected |
      | lib/model.dart                        | , whose first line is "// GENERATED CODE - DO NOT MODIFY BY HAND" | not selected |
      | lib/l10n/app_localizations.dart       | , beside an l10n.yaml that names no output           | not selected |
      | lib/l10n/app_localizations_en.dart    | , beside an l10n.yaml that names no output           | not selected |
      | lib/i18n/strings_es.dart              | , beside an l10n.yaml with output-dir lib/i18n and output-localization-file strings.dart | not selected |
      | lib/l10n/app_localizations.dart       | , with no l10n.yaml                                  | selected     |
      | lib/model.dart                        | , whose first line is "// Generated by hand"         | selected     |
      | lib/user.dart                         |                                                      | selected     |
      | lib/g.dart                            |                                                      | selected     |

  @ID-SEL-16
  Scenario: Reports name files relative to the working directory
    When a command reports on a file inside the working directory
    Then the file is named by its path relative to the working directory
    But a file outside the working directory keeps its absolute path

  # slice-6, issue #16 and decision 45: itos-cc.yaml's exclude lists
  # doublestar globs, from the project root, of the sources (never the
  # tests) that crap, dry, units and mutation leave out. Exclusion always
  # wins: a path, a fragment or --changed naming an excluded source selects
  # nothing, and a fragment that matches only excluded sources is no
  # paths.unmatched. A count of the sources left out goes to stderr when
  # there is any, plain output and no contract. A glob that is no pattern
  # is config.invalid, naming its key.
  @slice-6 @ID-SEL-17
  Scenario: Sources that itos-cc.yaml excludes are left out
    Given itos-cc.yaml holds 'exclude: ["**/porttest/**"]'
    And the sources src/board.go and src/port/porttest/fake.go
    When I run "itos-cc crap --no-coverage --json"
    Then src/board.go is scored, and src/port/porttest/fake.go is not
    And stderr says "itos-cc: 1 sources left out by exclude in itos-cc.yaml"

  @slice-6 @ID-SEL-18
  Scenario: A path or --changed never selects an excluded source
    Given itos-cc.yaml holds 'exclude: ["**/porttest/**"]'
    And git reports src/port/porttest/fake.go as modified
    When I run "itos-cc crap --changed", "itos-cc crap porttest" or "itos-cc crap src/port/porttest/fake.go"
    Then stderr says "itos-cc: no source files to score"
    And the exit code is 0

  @slice-6 @ID-SEL-19
  Scenario: An exclude glob that is no pattern is a config error
    Given itos-cc.yaml holds 'exclude: ["src/**", "src/[x"]'
    When I run "itos-cc crap"
    Then the problem is "config.invalid", with key "exclude[1]"
    And the exit code is 80

  # bug-4: config.Read's error reached the classifier unwrapped, so a file
  # that could not be read was "a bug in itos-cc", exit 100, though docs/CLI.md
  # (row 38) and slice-6's Upgrading say it is config.invalid, exit 80, naming
  # the file. A directory in the file's place is what cannot be read here,
  # the same on every system, where a file's permissions are not. Each
  # command reads the file before it selects anything, so none needs a path.
  # scrap reads only tests and never reads the file.
  @bug-4 @ID-SEL-20
  Scenario Outline: An itos-cc.yaml that cannot be read is a config error
    Given itos-cc.yaml is a directory, so it cannot be read
    When I run "itos-cc <command>"
    Then the problem is "config.invalid", with file "itos-cc.yaml"
    And the exit code is 80

    Examples:
      | command            |
      | crap --no-coverage |
      | dry                |
      | units              |
      | mutation list      |
      | mutation run       |
      | mutation check     |
      | mutation sample    |

  @bug-4 @ID-SEL-21
  Scenario: scrap never reads itos-cc.yaml
    Given itos-cc.yaml is a directory, so it cannot be read
    When I run "itos-cc scrap"
    Then the exit code is 0

  # bug-8, q-90 and docs/CONFIG.md rule 1: itos-cc.yaml is read in the
  # subset of YAML that JSON can express, so an anchor, an alias or a merge
  # key (&x, *x, <<) is config.invalid, exit 80, naming the key at fault for
  # the first of them in document order: the key whose value carries the
  # anchor, or the merge key itself. An alias always follows its anchor, so
  # a file holding an alias is refused at its anchor's key. An alias can
  # expand without bound (CVE-2019-11253), and mutation except, --renew and
  # --prune spliced nothing into an aliased mutation.exceptions yet said
  # they had written it.
  # slice-49 refuses the rest of rule 1 (below).
  @bug-8 @ID-SEL-24
  Scenario Outline: An anchor, an alias or a merge key in itos-cc.yaml is a config error
    Given itos-cc.yaml holds '<yaml>'
    When I run "itos-cc crap --no-coverage"
    Then the problem is "config.invalid", with key "<key>"
    And the exit code is 80

    Examples:
      | yaml                                     | key           |
      | exclude: &e ["**/porttest/**"]           | exclude       |
      | exclude: [&g "**/porttest/**", *g]       | exclude[0]    |
      | {x: &e ["**/porttest/**"], exclude: *e}  | x             |
      | mutation: {<<: {exceptions: []}}         | mutation.<<   |

  # Every command reads the file before anything else, the writers of
  # mutation.exceptions included, so none writes it.
  @bug-8 @ID-SEL-25
  Scenario Outline: An aliased mutation.exceptions is refused by every command, and nothing writes the file
    Given itos-cc.yaml holds '{x: &e [], mutation: {exceptions: *e}}'
    When I run "itos-cc <command>"
    Then the problem is "config.invalid", with key "x"
    And the exit code is 80
    And itos-cc.yaml is byte for byte as it was

    Examples:
      | command                 |
      | crap --no-coverage      |
      | dry                     |
      | units                   |
      | mutation list           |
      | mutation run            |
      | mutation check          |
      | mutation sample         |
      | mutation except --renew |
      | mutation except --prune |

  # slice-49, itos-cc-yaml-version's first item (q-94, the person
  # 2026-10-24; docs/CONFIG.md rule 1). bug-8 refuses anchors, aliases and
  # merge keys; the rest of what JSON cannot express is refused here, for
  # every command, as config.invalid, exit 80, before every other check, in
  # document order with bug-8's:
  # 1. A tag written on a value, other than the non-specific ! and the tags
  #    JSON has (!!str, !!int, !!float, !!bool, !!null, !!seq, !!map), keyed
  #    by the key whose value carries it, its fix to write the value without
  #    the tag. Only a tag written in the file counts: a plain 2026-10-24,
  #    which YAML resolves as a timestamp, is read as its key's string.
  # 2. A key that is not a string (a list or a mapping as a key), keyed by
  #    the mapping holding it, the top level naming the file alone.
  # 3. A second document after ---, naming the file alone, its fix to keep
  #    one document; a --- before the first document is one document. The
  #    reader decodes a second time and expects the end of input.
  # The writers read the file through settings.Parse first, so they refuse
  # it unwritten. The feat is a BREAKING-CHANGE with an Upgrading footer
  # (remove each !tag and any --- after the first document), in a minor
  # release. Unknown keys (itos-cc-yaml-unknown-keys) and version: 1
  # (itos-cc-yaml-version-key) are the next items. Before this item SEL-26,
  # SEL-27 and SEL-28 fail at the problem step; SEL-29 is a guardrail that
  # passes today.
  @slice-49 @ID-SEL-26
  Scenario Outline: A tag or a key that is not a string in itos-cc.yaml is a config error
    Given itos-cc.yaml holds '<yaml>'
    When I run "itos-cc crap --no-coverage"
    Then the problem is "config.invalid", with key "<key>"
    And the exit code is 80

    Examples:
      | yaml                                             | key        |
      | exclude: [!foo "**/porttest/**"]                 | exclude[0] |
      | {x: !!python/object:os.system echo, exclude: []} | x          |
      | mutation: {[a]: b}                               | mutation   |

  @slice-49 @ID-SEL-27
  Scenario: A second document in itos-cc.yaml is a config error
    Given itos-cc.yaml holds:
      """
      exclude: ["**/porttest/**"]
      ---
      exclude: []
      """
    When I run "itos-cc crap --no-coverage"
    Then the problem is "config.invalid", with file "itos-cc.yaml"
    And its message says "a second document"
    And its fix says "one document"
    And the exit code is 80

  @slice-49 @ID-SEL-28
  Scenario Outline: A tagged mutation.exceptions is refused by its writers, and nothing writes the file
    Given itos-cc.yaml holds 'mutation: {exceptions: !foo []}'
    When I run "itos-cc <command>"
    Then the problem is "config.invalid", with key "mutation.exceptions"
    And the exit code is 80
    And itos-cc.yaml is byte for byte as it was

    Examples:
      | command                 |
      | mutation except --renew |
      | mutation except --prune |

  @slice-49 @ID-SEL-29
  Scenario Outline: The non-specific tag, JSON's tags and a leading --- are plain data
    Given itos-cc.yaml holds '<yaml>'
    And the sources src/board.go and src/port/porttest/fake.go
    When I run "itos-cc crap --no-coverage --json"
    Then src/board.go is scored, and src/port/porttest/fake.go is not

    Examples:
      | yaml                                    |
      | exclude: [! "**/porttest/**"]           |
      | exclude: !!seq [!!str "**/porttest/**"] |
      | --- {exclude: ["**/porttest/**"]}       |
