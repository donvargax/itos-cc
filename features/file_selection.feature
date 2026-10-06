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

  Scenario: Dependencies, build output, caches, and fixtures are never walked
    Given the project contains these directories, each holding source files:
      | directory     |
      | node_modules  |
      | vendor        |
      | build         |
      | dist          |
      | target        |
      | out           |
      | coverage      |
      | .venv         |
      | venv          |
      | __pycache__   |
      | .metrics      |
      | testdata      |
      | .git          |
      | .any-hidden   |
    When the project's files are discovered
    Then nothing inside those directories is selected

  Scenario: A file named directly is taken even inside a skipped directory
    When I run "itos-cc units testdata/board.go"
    Then testdata/board.go is analyzed

  Scenario: TypeScript declaration files are not source
    Given a file src/types.d.ts
    When the project's files are discovered
    Then src/types.d.ts is not selected

  Scenario Outline: Telling test code from production code
    When the file "<path>" is discovered
    Then it is classified as <kind>

    Examples:
      | path                                 | kind       |
      | src/board.ts                         | production |
      | src/board.test.ts                    | test       |
      | src/board.spec.tsx                   | test       |
      | src/__tests__/board.ts               | test       |
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
