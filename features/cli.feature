Feature: Command line
  itos-cc is one binary with a subcommand per tool. Every command follows
  itos's CLI rules (docs/CLI.md): the same way of choosing files, the same
  flags parsed the same way, and one contract with scripts: the exit code,
  the --json object less "message" and "fix", and the files under .metrics/.
  Plain output is for people and may change in any release.

  Rule: Help and version

    @ID-CLI-01
    Scenario Outline: Asking for help prints it and succeeds
      When I run "itos-cc <args>"
      Then the usage text is printed to stdout
      And it lists the commands crap, dry, mutate, scrap, serve, units, and version
      And the exit code is 0

      Examples:
        | args   |
        |        |
        | -h     |
        | --help |
        | help   |

    @ID-CLI-02
    Scenario Outline: Each command's help gives its contract
      When I run "itos-cc <args>"
      Then the command's usage, options, --json shape, problem rules, and exit codes are printed to stdout
      And it ends with examples and the address for issue reports
      And the exit code is 0

      Examples:
        | args                  |
        | crap -h               |
        | crap --help           |
        | help crap             |
        | mutate src/a.go -h    |
        | units --changed --help |

    @ID-CLI-03
    Scenario Outline: Reporting the version
      Given the binary was built <how>
      When I run "itos-cc <command>"
      Then the first line of stdout is "itos-cc <reported>"

      Examples:
        | how                                       | command   | reported |
        | by a release with -X main.version=v1.2.3  | version   | v1.2.3   |
        | by "go install ...@v1.2.3"                | version   | v1.2.3   |
        | from a local checkout with "go build"     | --version | dev      |

    @ID-CLI-04
    Scenario Outline: An unknown command is a usage error
      When I run "itos-cc <args>"
      Then stderr says "itos-cc: there is no command \"<name>\"<guess> Run 'itos-cc --help' for the commands."
      And the exit code is 2

      Examples:
        | args        | name   | guess                     |
        | lint        | lint   | .                         |
        | crapp       | crapp  | . Did you mean 'crap'?    |
        | help nosuch | nosuch | .                         |

  Rule: Flags are read by one spec in every command

    @ID-CLI-05
    Scenario: Long flags with two dashes, in any position
      When I run "itos-cc crap src --top=5 --threshold 30 billing"
      Then it is the same as "itos-cc crap --top 5 --threshold 30 src billing"

    @ID-CLI-06
    Scenario: -- ends the options
      When I run "itos-cc units -- --tests"
      Then "--tests" is a path, not the flag

    @ID-CLI-07
    Scenario Outline: A bad flag is a usage error
      When I run "itos-cc <args>"
      Then stderr says "itos-cc: <message>"
      And the exit code is 2

      Examples:
        | args                    | message                                                       |
        | crap --no-such-flag     | crap takes no flag --no-such-flag. Run 'itos-cc crap --help' for its flags. |
        | units -changed          | units takes no flag -changed. Did you mean --changed? …       |
        | crap --threshold        | --threshold needs a value. Give it one, such as --threshold N. |
        | crap --top --json       | --top needs a value. …                                         |
        | crap --threshold=abc    | --threshold needs a number, and was given "abc". Give it a number. |
        | units --tests=yes       | --tests takes no value, and was given "yes". Write --tests alone. |
        | crap --top 3 --top 5    | --top is given more than once. Give it once.                   |
        | version extra           | version takes no arguments, and was given "extra". …           |
      # a value is never read as a flag: --top --json is a --top with no value

  Rule: One contract with scripts

    @ID-CLI-08
    Scenario Outline: Exit codes by kind
      Given <situation>
      When the command ends
      Then the exit code is <code>

      Examples:
        | situation                                                        | code |
        | it succeeded                                                     | 0    |
        | a check said no: a mutant survived, a function is over a threshold | 1    |
        | a usage or config error: a bad flag, path, or report             | 2    |
        | the environment lacks something: a tool, a report, a git repository | 3    |
        | an error itos-cc could not classify                              | 70   |
        | a temporary failure, such as serve's port being in use           | 75   |
      # with problems of several kinds: 2, then 70, 3, 75, and 1

    @ID-CLI-09
    Scenario: --json prints one object
      When I run "itos-cc crap --json"
      Then stdout is one object with "schema": 1, "ok": true, and the command's own keys
      And progress and test output go to stderr

    @ID-CLI-10
    Scenario: --json prints the object for a failure too
      When I run "itos-cc --json crapp"
      Then stdout is:
        """
        {
          "ok": false,
          "problems": [
            {
              "command": "crapp",
              "fix": "Did you mean 'crap'? Run 'itos-cc --help' for the commands.",
              "message": "there is no command \"crapp\"",
              "rule": "command.unknown"
            }
          ],
          "schema": 1
        }
        """
      And the exit code is 2
      # each problem carries a stable rule id and its subject as keys, such
      # as file, line, and function; scripts never read message or fix

    @ID-CLI-11
    Scenario: Plain output prints each problem on stderr
      When a run has a problem
      Then stderr says "itos-cc: <message>. <fix>"
