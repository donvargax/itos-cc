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
      And it lists the commands crap, dry, mutation, scrap, serve, units, and version
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
        | args                     |
        | crap -h                  |
        | crap --help              |
        | help crap                |
        | mutation run src/a.go -h |
        | units --changed --help   |

    # Binary-only for good: each row builds itos-cc as a release, go install
    # or a person's go build makes it, and what a binary reports comes from
    # that build's stamp and the module version Go records in it.
    @ID-CLI-03 @binary-only
    Scenario Outline: Reporting the version
      Given the binary was built <how>
      When I run "itos-cc <command>"
      Then the first line of stdout is "itos-cc <reported>"

      Examples:
        | how                                                  | command   | reported                 |
        | by a release with -X main.version=v1.2.3             | version   | 1.2.3                    |
        | by "go install ...@v1.2.3"                           | version   | 1.2.3                    |
        | from a local checkout with "go build"                | --version | <next>-0.<time>-<commit> |
        | with "go build" from a tree with no git information  | --version | dev                      |
      # a checkout's build says the pseudo-version Go records from git: the
      # release after its last tag, the commit's time and its commit, every
      # version without its leading v (#19, ID-CLI-18)

    @ID-CLI-04
    Scenario Outline: An unknown command is a usage error
      When I run "itos-cc <args>"
      Then stderr says "itos-cc: there is no command \"<name>\"<guess> Run 'itos-cc --help' for the commands."
      And the exit code is 81

      Examples:
        | args        | name   | guess                                           |
        | lint        | lint   | .                                               |
        | crapp       | crapp  | . Did you mean one of 'crap', 'graph', 'scrap'? |
        | help nosuch | nosuch | .                                               |

    # slice-60, q-99 (the person 2026-10-25): every "Did you mean" follows
    # kong's rule, which the person finds friendlier: each candidate that
    # starts with what was typed or is within two edits of it, in the order
    # the candidates are declared, one named as "Did you mean X?" and
    # several as "Did you mean one of X, Y?". The kong fork exports the rule
    # (a new -itos tag) and itos-cc uses it for commands, flags, serve's hand
    # parser and itos-cc.yaml keys (SEL-41), deleting its own two copies; the
    # architecture rules let the settings adapter import the fork for it
    # (decision 62). q-100 (the person 2026-10-25): when kong's rule finds
    # nothing, the fork's function falls back to today's stem test (all but
    # the typed word's last letter, four letters at least, begins exactly
    # one candidate), so CLI-15's mutate still suggests 'mutation'; kong's
    # own closestCandidates stays untouched. Plain output and the fix's
    # wording change for the inputs the rule now matches more of (crapp
    # names 'crap', 'graph' and 'scrap' in CLI-04 and CLI-10); the problem's
    # rule and fields do not, so no breaking note. Before this item "s"
    # names no command, failing at the stderr step.
    @slice-60 @ID-CLI-30
    Scenario: A prefix of several commands names them all
      When I run "itos-cc s"
      Then stderr says "itos-cc: there is no command \"s\". Did you mean one of 'scrap', 'serve'? Run 'itos-cc --help' for the commands."
      And the exit code is 81

    # Binary-only for good, as ID-CLI-03: each row builds itos-cc as a
    # release or go install makes it. Issue #19: the release archive said
    # 1.2.3 and go install v1.2.3. Every version printed loses a leading v,
    # a stamp given with one and a checkout's pseudo-version included, so
    # ID-CLI-03's rows change with this slice.
    @binary-only @slice-9 @ID-CLI-18
    Scenario Outline: A release and a go install of one version say it alike, with no v
      Given the binary was built <how>
      When I run "itos-cc version"
      Then the first line of stdout is "itos-cc 1.2.3"
      When I run "itos-cc version --json"
      Then stdout is:
        """
        {
          "ok": true,
          "schema": 1,
          "version": "1.2.3"
        }
        """

      Examples:
        | how                                     |
        | by a release with -X main.version=1.2.3 |
        | by "go install ...@v1.2.3"              |

    # Decisions 36 and 38: square/exit's codes, none of 2, 3 and 70, and 130
    # in every command's, since every command stops on an interrupt.
    @slice-7 @slice-8 @ID-CLI-19
    Scenario Outline: Each help lists the exit codes its command ends with
      When I run "itos-cc <args>"
      Then its help lists the exit codes <codes>, and no other
      And it lists 130 as "interrupted"

      Examples:
        | args                   | codes                             |
        | --help                 | 0, 1, 75, 80, 81, 82, 100 and 130 |
        | crap --help            | 0, 1, 80, 82, 100 and 130         |
        | dry --help             | 0, 80, 82, 100 and 130            |
        | scrap --help           | 0, 80, 82, 100 and 130            |
        | units --help           | 0, 80, 82, 100 and 130            |
        | help serve             | 75, 80, 100 and 130               |
        | version --help         | 0, 80, 100 and 130                |
        | mutation               | 0, 80, 81 and 130                 |
        | mutation run --help    | 0, 1, 80, 82, 100 and 130         |
        | mutation list --help   | 0, 80, 82, 100 and 130            |
        | mutation check --help  | 0, 1, 80, 82, 100 and 130         |
        | mutation sample --help | 0, 1, 80, 82, 100 and 130         |
        | mutation except --help | 0, 1, 80, 100 and 130             |
      # serve ends only when stopped, so 0 is not among its codes

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
      And the exit code is 80

      Examples:
        | args                    | message                                                       |
        | crap --no-such-flag     | crap takes no flag --no-such-flag. Run 'itos-cc crap --help' for its flags. |
        | units -changed          | units takes no flag -changed. Did you mean --changed? …       |
        | crap --threshold        | --threshold needs a value. Give it one, such as --threshold N. |
        | crap --top --changed    | --top needs a value. …                                         |
        | crap --threshold=abc    | --threshold needs a number, and was given "abc". Give it a number. |
        | units --tests=yes       | --tests takes no value, and was given "yes". Write --tests alone. |
        | crap --top 3 --top 5    | --top is given more than once. Give it once.                   |
        | version extra           | version takes no arguments, and was given "extra". …           |
      # a flag is never read as a value: --top --changed is a --top with no value

  Rule: One contract with scripts

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
              "exit": 81,
              "fix": "Did you mean one of 'crap', 'graph', 'scrap'? Run 'itos-cc --help' for the commands.",
              "message": "there is no command \"crapp\"",
              "rule": "command.unknown"
            }
          ],
          "schema": 1
        }
        """
      And the exit code is 81
      # each problem carries a stable rule id and its subject as keys, such
      # as file, line, and function; scripts never read message or fix

    @ID-CLI-11
    Scenario: Plain output prints each problem on stderr
      When a run has a problem
      Then stderr says "itos-cc: <message>. <fix>"

    # Decision 36 renumbers ID-CLI-08's kinds as square/exit does, keeping
    # 75 for a failure that may pass when run again; this replaces it.
    @slice-7 @ID-CLI-20
    Scenario Outline: Each kind of ending has its own exit code
      Given <situation>
      When the command ends
      Then the exit code is <code>

      Examples:
        | situation                                                           | code |
        | it succeeded                                                        | 0    |
        | a check said no: a mutant survived, a function is over a threshold  | 1    |
        | a usage or config error: a bad flag, path, or report                | 80   |
        | an unknown command                                                  | 81   |
        | the environment lacks something: a tool, a report, a git repository | 82   |
        | an error itos-cc could not classify                                 | 100  |

      # The binary driver's alone until T-112 (features.Deferred), as
      # ID-CLI-08's row was.
      @T-112
      Examples:
        | situation                                              | code |
        | a temporary failure, such as serve's port being in use | 75   |
      # 130, an interrupt, is the next Rule's

    @slice-7 @ID-CLI-21
    Scenario Outline: An unknown command exits 81, any other usage error 80
      When I run "itos-cc <args>"
      Then the problem is "<rule>"
      And the exit code is <code>

      Examples:
        | args                   | rule                | code |
        | lint                   | command.unknown     | 81   |
        | help nosuch            | command.unknown     | 81   |
        | mutation nosuch        | command.unknown     | 81   |
        | crap --no-such-flag    | flags.unknown       | 80   |
        | units --tests=yes      | flags.switch-value  | 80   |
        | mutation run --count 0 | flags.value-invalid | 80   |
        | version extra          | args.unexpected     | 80   |
        | crap nosuch            | paths.unmatched     | 80   |

    @slice-7 @ID-CLI-22
    Scenario: A run with problems of several kinds exits as the first kind, and --json gives each its own
      Given a Go function scoring 20 beside TypeScript whose coverage tool is not installed
      When I run "itos-cc crap --threshold 5 --json"
      Then its problems in --json are:
        | rule                  | exit |
        | coverage.tool-missing | 82   |
        | crap.threshold        | 1    |
      And the exit code is 82
      # the first of 130, 80 and 81, 100, 82, 75, and 1, not the highest

  # Decision 38 (q-51, q-54): every command, serve and mutation run --count
  # included, stops on SIGINT and on SIGTERM, says once that it was
  # interrupted, reports the rule interrupted, exit 130 in --json, and exits
  # 130. On Linux and macOS it then raises the signal that came again, with
  # its default action, so a shell reads 130 for SIGINT and 143 for SIGTERM
  # and stops its loop; Windows exits 130. Each scenario signals a run once
  # it shows it is under way, never after a guessed delay. A second
  # interrupt ending a command at once is held by unit tests on fake signals.
  Rule: An interrupt stops every command

    # Binary-only for good, as the rest of this Rule: the signal reaches
    # itos-cc as a process of its own.
    @binary-only @slice-8 @ID-CLI-23
    Scenario: An interrupted serve says so once and exits 130
      Given "itos-cc serve" is serving the scratch folder
      When it is interrupted
      Then stderr says "itos-cc: interrupted" once
      And it ends as interrupted, which a shell reads as 130
      # serve exited 0 when interrupted until decision 38

    @binary-only @slice-7 @slice-8 @ID-CLI-24
    Scenario: An interrupted serve with --json keeps its address beside the problem
      Given "itos-cc serve --json" is serving the scratch folder
      When it is interrupted
      Then stdout is one object with "schema": 1, "ok", "address", and "problems"
      And its problems in --json are:
        | rule        | exit |
        | interrupted | 130  |
      And it ends as interrupted, which a shell reads as 130

    @binary-only @slice-7 @slice-8 @ID-CLI-25
    Scenario: SIGTERM stops a command as an interrupt does, and a shell reads 143
      Given "itos-cc serve --json" is serving the scratch folder
      When it is sent SIGTERM
      Then its problems in --json are:
        | rule        | exit |
        | interrupted | 130  |
      And it ends as terminated, which a shell reads as 143
      # SIGTERM is what CI sends to cancel a job; --json says 130 for either

    @binary-only @slice-8 @ID-CLI-26
    Scenario: An interrupt stops crap promptly while its test hangs, leaving no process its tests started
      Given a Go project whose test starts a process of its own and then hangs
      When "itos-cc crap" is interrupted once that process is running
      Then stderr says "itos-cc: interrupted" once
      And it ends as interrupted, which a shell reads as 130
      And no process its tests started is still running

    @binary-only @slice-8 @ID-CLI-27
    Scenario: A shell reads 130 from an interrupted command
      Given a shell runs:
        """
        itos-cc serve --port 0
        echo "status $?"
        """
      When itos-cc alone is interrupted once it is serving
      Then the shell prints "status 130"

    @binary-only @slice-8 @ID-CLI-28
    Scenario: Ctrl-C stops a shell's loop over a command
      Given a shell runs:
        """
        for run in first second; do
          itos-cc serve --port 0
          echo "$run ended with status $?"
        done
        """
      When Ctrl-C reaches the shell and itos-cc once itos-cc is serving
      Then the shell ends interrupted, before the second itos-cc starts
      # a command that catches the signal and exits, even with 130, lets the
      # loop go on: a shell stops only when the command ends by the signal

  # serve-embedded-viewer (ADR-0060): until the graph is a .metrics snapshot
  # its own web app reads, serve serves the viewer its build embedded, so a
  # release's binary shows the graph with no clone, Node or --ui. A build
  # embeds what server/ui holds, where the release copies viewer/dist; a
  # checkout's or go install's build holds none and answers / as before.
  # The bundle's elkjs, EPL-2.0 OR GPL-3.0-or-later in an AGPL-3.0 binary,
  # is taken under GPL-3.0-or-later (the person, q-87, 2026-10-24), which
  # GPLv3's section 13 lets combine with AGPLv3. The coordinator's calls
  # (2026-10-24): server/ui holds only a committed .gitignore and is
  # embedded with all:ui, the release copying viewer/dist there before each
  # build, so every Go job builds unchanged and a checkout's build answers /
  # as before (no build tag: AGENTS.md keeps those for OS calls; no
  # committed dist); release.yml gains a viewer job building it once and a
  # step serving the archive's binary; THIRD_PARTY_NOTICES names the
  # bundle's 18 npm packages, tools/bin/notices -check keeping it true;
  # binary +1.9 MB.
  Rule: serve serves the viewer its build carries

    # Binary-only for good, as ID-CLI-03: each row builds itos-cc.
    @binary-only @slice-42 @ID-CLI-29
    Scenario Outline: serve answers / with the viewer it was built with, unless --ui names another
      Given the binary was built <how>
      And the scratch folder holds a viewer in "other" whose index.html says "other viewer"
      And "itos-cc serve <args>" is serving the scratch folder
      When I request "GET /"
      Then the response is 200 and its body says "<says>"
      When I request "GET /api/graph"
      Then the response is JSON with the version, nodes, and edges

      Examples:
        | how                                                                                  | args       | says                                                                                         |
        | with "go build" from a tree whose server/ui holds a viewer saying "built-in viewer" |            | built-in viewer                                                                              |
        | with "go build" from a tree whose server/ui holds a viewer saying "built-in viewer" | --ui other | other viewer                                                                                 |
        | with "go build" from a tree with no git information                                  |            | itos-cc API: /api/graph, /api/events, /api/source. Start the viewer, or pass --ui viewer/dist. |

  # Decided with the person on 2026-10-06 (q-1, q-3): mutation testing is one
  # group, mutation, following itos's CLI rules 3, 4, 7 and 13. mutate and
  # mutate --scan go with no alias: itos-cc is before 1.0, so a breaking
  # change needs only a minor release. The problem rules move with the
  # command (mutate.survived becomes mutation.survived, and so on); the
  # cache folder .metrics/mutate/ and the "itos-cc mutate:" marker of the
  # summary comment stay, since renaming them would rewrite every committed
  # cache and duplicate every annotation. mutation check, the check mode,
  # is its own item. (annotate-marker later renamed the marker to
  # "itos-cc mutation:", a run still replacing a comment with the old one.)
  Rule: The mutation group

    @mutation-group @ID-CLI-12
    Scenario: The group alone names its subcommands
      When I run "itos-cc mutation"
      Then stdout names the subcommands run and list
      And the exit code is 0

    @mutation-group @ID-CLI-13
    Scenario: mutation run mutates
      When I run "itos-cc mutation run src/board.ts"
      Then stdout says "14 killed, 1 survived, 2 uncovered in src/board.ts (ran 7, reused 8)"
      And each survivor is a "mutation.survived" problem

    @mutation-group @ID-CLI-14
    Scenario: mutation list lists the sites without running tests
      When I run "itos-cc mutation list x.ts"
      Then stdout lists each mutation site of x.ts with its line, column, function, original, and replacement
      And no test command runs

    @mutation-group @ID-CLI-15
    Scenario: mutate is no longer a command
      When I run "itos-cc mutate src/board.ts"
      Then stderr says "itos-cc: there is no command \"mutate\". Did you mean 'mutation'? Run 'itos-cc --help' for the commands."
      And the exit code is 81

    @mutation-group @ID-CLI-16
    Scenario: An unknown subcommand of the group is a usage error
      When I run "itos-cc mutation nosuch"
      Then the problem is "command.unknown"
      And stderr names the subcommands run and list
      And the exit code is 81

    @mutation-group @ID-CLI-17
    Scenario: --scan is not a flag of mutation run
      When I run "itos-cc mutation run --scan x.ts"
      Then the problem is "flags.unknown", with flag "--scan"
      And the exit code is 80
