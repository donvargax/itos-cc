Feature: Command line
  itos-cc is one binary with a subcommand per tool. Every command shares the
  same way of choosing files and the same exit code for misuse, so people and
  coding agents can drive any tool the same way.

  Scenario: Running without a command prints usage and fails
    When I run "itos-cc"
    Then the usage text is printed to stderr
    And the exit code is 1

  Scenario Outline: Asking for help prints usage and succeeds
    When I run "itos-cc <flag>"
    Then the usage text is printed to stdout
    And it lists the commands crap, dry, mutate, scrap, serve, units, and version
    And the exit code is 0

    Examples:
      | flag   |
      | -h     |
      | --help |
      | help   |

  Scenario: An unknown command is rejected
    When I run "itos-cc lint"
    Then stderr says "itos-cc: unknown command \"lint\""
    And the usage text follows it
    And the exit code is 1

  Scenario Outline: Each command describes its own options
    When I run "itos-cc <command> -h"
    Then the command's usage and options are printed

    Examples:
      | command |
      | crap    |
      | dry     |
      | mutate  |
      | scrap   |
      | serve   |
      | units   |

  Scenario Outline: Reporting the version
    Given the binary was built <how>
    When I run "itos-cc <command>"
    Then stdout is "itos-cc <reported>"

    Examples:
      | how                                       | command   | reported |
      | by a release with -X main.version=v1.2.3  | version   | v1.2.3   |
      | by "go install ...@v1.2.3"                | version   | v1.2.3   |
      | from a local checkout with "go build"     | --version | dev      |

  Scenario: Flags may come before or after paths
    When I run "itos-cc crap src --json"
    Then it behaves the same as "itos-cc crap --json src"

  Scenario: An unknown flag is a usage error
    When I run "itos-cc crap --no-such-flag"
    Then the exit code is 1
