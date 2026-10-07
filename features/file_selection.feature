Feature: Choosing which files a command looks at
  Every command takes paths, path fragments, or --changed, and splits what it
  finds into production code and test code by each language's conventions.

  Background:
    Given a project with TypeScript, Python, Kotlin, and Go sources

  Scenario: Without paths the working directory is analyzed
    When I run a command with no paths
    Then every supported file under the working directory is selected

  Scenario: Paths may be files or directories
    When I run "itos-cc crap src/billing src/cart/total.ts"
    Then the files under src/billing and the file src/cart/total.ts are selected

  Scenario: A path that does not exist is a fragment
    Given there is no file or directory called "billing"
    When I run "itos-cc crap billing"
    Then every source whose path contains "billing" is selected, such as src/billing/invoice.ts

  Scenario: Selecting what git reports as changed
    Given git reports src/a.ts as modified, src/b.py as staged, and src/c.go as untracked
    And src/d.kt was deleted
    When I run "itos-cc crap --changed"
    Then src/a.ts, src/b.py, and src/c.go are selected
    And src/d.kt is not selected

  Scenario: --changed from a subdirectory selects the changes under it
    Given git reports src/a.ts and lib/b.ts as modified
    And src/año nuevo.ts as untracked
    When I run "itos-cc crap --changed" in src
    Then a.ts and año nuevo.ts are selected
    And lib/b.ts is not selected

  Scenario: --changed works before the first commit
    Given a git repository with staged files and no commits
    When I run a command with --changed
    Then the staged files are selected

  Scenario: --changed outside a git repository is a usage error
    Given the working directory is not a git repository
    When I run "itos-cc crap --changed"
    Then stderr says "--changed needs a git repository"
    And the exit code is 1

  Scenario: --changed with nothing changed selects nothing
    Given git reports no changes
    When I run "itos-cc crap --changed"
    Then stderr says "itos-cc: no source files to score"
    And the exit code is 0

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

  Scenario Outline: Build output is skipped, a package with its name is not
    Given a directory "<directory>" holding source files
    When the project's files are discovered
    Then its files are <selected>
    # build, dist, target, out, and coverage are build output names, and
    # also ordinary package names; mutate copies the same directories

    Examples:
      | directory                                   | selected     |
      | web/dist, beside web/package.json           | not selected |
      | target, beside pom.xml or Cargo.toml        | not selected |
      | app/build, beside app/build.gradle.kts      | not selected |
      | gen/out, which git ignores                  | not selected |
      | internal/out, a Go package                  | selected     |
      | out, beside go.mod only                     | selected     |

  Scenario: A file named directly is taken even inside a skipped directory
    When I run "itos-cc units testdata/board.go"
    Then testdata/board.go is analyzed

  Scenario: JavaScript files are TypeScript to every tool
    Given the files src/a.js, src/b.jsx, src/c.mjs, and src/d.cjs
    When the project's files are discovered
    Then each is selected as TypeScript and parsed with the JavaScript grammar

  Scenario: TypeScript declaration files and minified bundles are not source
    Given the files src/types.d.ts and public/vendor.min.js
    When the project's files are discovered
    Then neither is selected

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

  Scenario: Reports name files relative to the working directory
    When a command reports on a file inside the working directory
    Then the file is named by its path relative to the working directory
    But a file outside the working directory keeps its absolute path
