Feature: CRAP scores
  "Which functions are complex and under-tested?" Each function scores
  CRAP = CC² × (1 − coverage)³ + CC. 1–5 is low risk, 5–30 is worth a look,
  and above 30 is complex and under-tested.

  Scenario Outline: The CRAP formula
    Given a function with complexity <cc> and <coverage>% coverage
    When it is scored
    Then its CRAP score is <crap>

    Examples:
      | cc | coverage | crap  |
      | 1  | 0        | 2.0   |
      | 5  | 100      | 5.0   |
      | 5  | 0        | 30.0  |
      | 10 | 50       | 22.5  |
      | 10 | 0        | 110.0 |

  Scenario: Scoring a project runs its tests with coverage first
    Given a project with tests
    When I run "itos-cc crap"
    Then each language's coverage command runs, with its output on stderr
    And stdout is a table with the columns CRAP, CC, and COV%
    And each row names the function as "namespace#name  file:line"
    And rows are sorted from the highest CRAP score down

  Scenario: Functions without coverage are listed last
    Given a function that no coverage report applies to
    When I run "itos-cc crap"
    Then its CRAP and COV% are shown as "N/A"
    And it is listed after every scored function, by complexity

  Scenario: A file the tests never load counts as uncovered
    Given coverage measured other Python files in the project
    And the report never mentions src/unused.py
    When I run "itos-cc crap"
    Then the functions of src/unused.py have 0% coverage

  Scenario: A language the tests did not measure is unknown, not uncovered
    Given the coverage report mentions no Kotlin file
    When I run "itos-cc crap"
    Then the Kotlin functions have no CRAP score

  Scenario: Only the worst functions
    When I run "itos-cc crap --top 20"
    Then only the 20 highest-scoring functions are printed
    But .metrics/crap.json still records every function

  Scenario: JSON output
    When I run "itos-cc crap --json"
    Then stdout is one object with "schema": 1, "ok", "entries", and "unmeasured"
    And each entry has namespace, name, language, file, start_line, end_line, complexity, coverage, and crap
    And each unmeasured build root has dir, language, cause, and reason

  Scenario: Every run writes a snapshot
    When I run "itos-cc crap"
    Then .metrics/crap.json holds every scored function

  Scenario: Failing a build on a threshold
    Given a function scores 42.0
    When I run "itos-cc crap --threshold 30"
    Then stderr says "itos-cc: board#place scores 42.0, above the threshold 30.0. Cover it with tests or split it."
    And it says so for every function above the threshold, each a "crap.threshold" problem with file, line, function, crap, and threshold
    And the exit code is 1

  Scenario: Staying under the threshold
    Given every function scores 30 or less
    When I run "itos-cc crap --threshold 30"
    Then the exit code is 0

  Scenario Outline: A threshold is not passed by code coverage could not measure
    Given a project whose <problem>
    When I run "itos-cc crap --threshold 30"
    Then its functions show "N/A"
    And the problem is "<rule>", with dir and language
    And the exit code is <code>

    Examples:
      | problem                                         | rule                       | code |
      | tests do not compile                            | coverage.measured-nothing  | 1    |
      | coverage tool is not installed                  | coverage.tool-missing      | 3    |
      | report is missing, with --use-existing-coverage | coverage.no-report         | 3    |
      | --coverage-report cannot be read                | coverage.report-unreadable | 2    |
    # N/A is not 0%: a broken test setup is not untested code, but it must
    # not pass the gate

  Scenario: Without a threshold, coverage that could not be measured is a warning
    Given a Go project whose tests do not compile
    When I run "itos-cc crap"
    Then stderr says "itos-cc: no coverage for <dir> (go): ..."
    And the exit code is 0

  Scenario: Nothing to score
    Given the selection holds no production source files
    When I run "itos-cc crap"
    Then stderr says "itos-cc: no source files to score"
    And the exit code is 0
