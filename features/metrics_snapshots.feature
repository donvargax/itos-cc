Feature: Snapshots under .metrics
  Every command writes a JSON snapshot under .metrics/ that projects commit,
  so a clone has the numbers without rerunning and nobody reruns mutants that
  are already killed.

  Scenario Outline: Each command writes its snapshot
    When I run "itos-cc <command>"
    Then <snapshot> is written

    Examples:
      | command | snapshot                         |
      | crap    | .metrics/crap.json               |
      | dry     | .metrics/dry.json                |
      | scrap   | .metrics/scrap.json              |
      | mutate  | .metrics/mutate/SOURCE_PATH.json |

  Scenario: An unchanged result is an unchanged file
    Given a command has written its snapshot
    When I run it again and nothing in the code has changed
    Then the snapshot file is byte-for-byte the same
    # snapshots carry no timestamps and are sorted

  Scenario: Every snapshot records its format version
    When any snapshot is written
    Then it has "version": 1

  Scenario: Readers never see half a snapshot
    When a snapshot is written
    Then it is written to a temporary file of its own and renamed into place
    And two runs writing the same snapshot at once leave one whole snapshot

  Scenario: Raw coverage is not committed
    When coverage is run
    Then raw reports go to .metrics/coverage/, which holds a .gitignore of "*"

  Scenario: Runs at the same time keep their coverage apart
    Given an agent runs "itos-cc mutate" while a commit hook runs "itos-cc crap"
    When both run coverage
    Then each writes its reports to a run-* directory of its own under .metrics/coverage/
    And each reads only its own
    And the directory is deleted when the run ends, after its reports replace the previous run's
    # so --use-existing-coverage reads the latest run, and nothing accumulates

  Scenario: Directories of killed runs are cleaned up
    Given a run-* directory left by a run that was killed a day or more ago
    When coverage is run
    Then that directory is deleted
