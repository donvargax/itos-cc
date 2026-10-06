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
    Then it is written to a temporary file and renamed into place

  Scenario: Raw coverage is not committed
    When coverage is run
    Then raw reports go to .metrics/coverage/, which holds a .gitignore of "*"
