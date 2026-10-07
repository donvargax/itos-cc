# CLI design

itos-cc follows the CLI rules of itos, its host, so a person or an agent meets
the same rules in both: [itos's docs/CLI.md](https://github.com/donvargax/itos/blob/main/docs/CLI.md).
This page names those rules by their numbers there, says whether itos-cc
follows each, and lists where it does not yet. Change this page in the same
commit as the command line.

## The contract with scripts

Scripts can rely on three things only, as with itos (its decision 35):

- The exit code.
- The `--json` output, less every key named `message` or `fix`. These keys
  hold the same sentences as the plain output.
- The files that itos-cc writes under `.metrics/`.

All plain output is for people and can change in any release. A change to the
contract is a breaking change, made in a major release with the others that
are ready (rules 42 and 43). Before 1.0 a breaking change is made in a minor
release instead: itos-cc is at 0.x.

### Exit codes

| Code | Meaning                                                                                |
| ---- | -------------------------------------------------------------------------------------- |
| 0    | Success.                                                                               |
| 1    | A check said no: a mutant survived, an uncovered mutant with `--fail-uncovered`, mutation results missing or stale, a function is over `--threshold`, tests that measure nothing. |
| 2    | A usage or config error: a bad flag, path, argument, or report.                        |
| 3    | The environment lacks something: a tool, a report, a git repository.                   |
| 70   | An internal error that itos-cc could not classify, a panic included. Report it.        |
| 75   | A temporary failure: the same command can pass when run again unchanged.               |

A run with problems of several kinds exits with the first of 2, 70, 3, 75,
and 1 that it has.

### `--json`

Every command takes `--json` and then prints one object on stdout:

```json
{"schema": 1, "ok": true, "...": "the command's own keys"}
```

A later release only adds keys. A run that did not succeed, a usage error
included, prints the object too, with `"ok": false` and its problems:

```json
{"schema": 1, "ok": false, "problems": [
  {"rule": "mutation.survived", "message": "…", "fix": "…",
   "file": "src/a.go", "line": 12, "column": 9, "function": "example.com/m/a#Pos",
   "original": ">", "replacement": ">="}
]}
```

`rule` is a stable id; the other keys besides `message` and `fix` are the
problem's subject. Progress and test output go to stderr, never stdout.

### Problem rules

| Rule                         | Exit | Command        | Subject keys                                                |
| ---------------------------- | ---- | -------------- | ----------------------------------------------------------- |
| `flags.unknown`              | 2    | every          | `flag`                                                      |
| `flags.value-missing`        | 2    | every          | `flag`                                                      |
| `flags.value-invalid`        | 2    | every          | `flag`, `value`                                             |
| `flags.switch-value`         | 2    | every          | `flag`                                                      |
| `flags.repeated`             | 2    | every          | `flag`                                                      |
| `flags.conflict`             | 2    | mutation run, mutation check | `flag`                                        |
| `command.unknown`            | 2    | itos-cc, mutation | `command`                                                |
| `command.missing`            | 2    | itos-cc --json, mutation --json | none                                       |
| `args.unexpected`            | 2    | version, help  | `argument`                                                  |
| `paths.unmatched`            | 2    | crap, dry, mutation run, mutation list, mutation check, scrap, units | `argument` |
| `changed.no-git`             | 3    | crap, dry, mutation run, mutation list, mutation check, scrap, units | none     |
| `since.bad-ref`              | 2    | mutation run, mutation check | `ref`                                         |
| `since.no-git`               | 3    | mutation run, mutation check | none                                          |
| `coverage.command-needs-report` | 2 | crap, mutation run | none                                                    |
| `coverage.measured-nothing`  | 1    | crap --threshold | `dir`, `language`, or `report`                            |
| `coverage.tool-missing`      | 3    | crap --threshold | `dir`, `language`                                         |
| `coverage.no-report`         | 3    | crap --threshold | `dir`, `language`                                         |
| `coverage.report-unreadable` | 2    | crap --threshold | `report`                                                  |
| `crap.threshold`             | 1    | crap           | `file`, `line`, `function`, `crap`, `threshold`             |
| `mutation.survived`          | 1    | mutation run, mutation check | `file`, `line`, `column`, `function`, `original`, `replacement` |
| `mutation.uncovered`         | 1    | mutation run --fail-uncovered, mutation check --fail-uncovered | `file`, `line`, `column`, `function`, `original`, `replacement` |
| `mutation.missing`           | 1    | mutation check | `file`, `line`, `function`                                  |
| `mutation.stale`             | 1    | mutation check | `file`, `line`, `function`                                  |
| `mutation.baseline-failed`   | 1    | mutation run   | `file`                                                      |
| `serve.repo-unreadable`      | 2    | serve          | none                                                        |
| `serve.port-in-use`          | 75   | serve          | `port`                                                      |
| `internal`                   | 70   | every          | none                                                        |

## The rules

Rules 1 to 43 of itos's docs/CLI.md, as they apply to itos-cc.

| Rule | Topic | itos-cc |
| ---- | ----- | ------- |
| 1 | Short lowercase program name | Follows. |
| 2 | Lowercase subcommands with dashes | Follows. |
| 3, 4 | Groups are singular nouns, actions imperative verbs | Follows. One group, `mutation`, a singular noun whose actions are verbs: `mutation run`, `mutation list`, `mutation check`. Every other command is named for what it measures (`crap`, `dry`, `scrap`, `units`) or does (`serve`). |
| 5 | No two commands with similar names | Does not follow: `crap` and `scrap`. |
| 6 | No everyday verb that points at another command | Follows. |
| 7, 8 | No implicit default subcommand, no abbreviations | Follows: `itos-cc` alone and `itos-cc mutation` alone print help; an unknown command names the one meant. |
| 9 | Help everywhere, on stdout, exit 0 | Follows: `itos-cc`, `--help`, `help <command>`, `<command> --help`, `-h` in any position; for the group, `mutation`, `mutation -h`, `help mutation run`, `mutation run --help`. |
| 10 | Help gives the `--json` shape and exit codes | Follows, with each command's problem rules. |
| 11 | `--version` and `version` print `itos-cc <version>` first | Follows. |
| 12 | Unknown command exits 2 and names a guess | Follows, for a group's subcommands too: `mutation nosuch` exits 2 and names `run`, `list`, and `check`. |
| 13 | A group with no subcommand names them | Follows: `itos-cc mutation` prints the group's help, naming `run`, `list`, and `check`, on stdout with exit 0, as `itos-cc` alone does; with `--json` it is `command.missing`, exit 2. |
| 14 | Help ends with examples and the issues address | Follows. |
| 15, 16 | Long flags, `-h` the only short one; standard names | Follows. |
| 17 | A flag means the same in every command | Does not follow: `--threshold`. |
| 18 | A flag changes an action, never selects another | Follows. |
| 19–22 | `--flag=value` and `--flag value`; bad, missing, repeated, or switch values exit 2; a value is never a flag; no optional values | Follows: every command reads its flags from one declared spec (`cmd/itos-cc/cli.go`). |
| 23 | `--` ends the options; `-` is stdin or stdout | Follows for `--`; no command reads stdin or writes a file. |
| 24 | Flags in any position | Follows. |
| 25 | Each argument checked; a bad one exits 2 | Follows: a path that is no file and no fragment of one exits 2. |
| 26 | Main output on stdout, the rest on stderr | Follows. |
| 27 | JSON only with `--json`, `"schema": 1`, keys only added | Follows. `serve`'s HTTP API is JSON by design and not stdout. |
| 28 | A stderr hint when plain output looks like data | No plain output looks like data. |
| 29 | `--json` prints the object for every failure | Follows, usage errors and panics included. |
| 30 | No colour; `NO_COLOR` passed on | Follows: itos-cc prints no colour, and the tests it runs inherit the environment. |
| 31 | The exit code comes from the kind; unclassified is 70 | Follows. |
| 32 | Error lines start `itos-cc:` and say what to do | Partly: an internal error prints Go's error text. |
| 33 | Help and code agree on exit codes | Follows; tests check every command's help. |
| 34, 35 | `ITOS_CC_` variables; flag, then environment, then config | itos-cc reads no variables or config of its own. |
| 36 | No network check in CI | Follows: itos-cc never touches the network, nor downloads a tool. |
| 37 | Questions only on a terminal, with a flag each | itos-cc asks nothing. |
| 38 | Project settings in a file under version control | None yet; the mutation exceptions of issue #10 would be one. |
| 39–41 | Entry points for other programs | None. |
| 42, 43 | Breaking changes together in a major release, no compatibility code | Follows; before 1.0, in a minor release: `mutate` became `mutation run` in one with no alias. |

## Where itos-cc does not follow these rules yet

| Rule | What itos-cc does now | Example |
| ---- | --------------------- | ------- |
| 5 | Two commands with names a letter apart. | `crap`, `scrap` |
| 17 | `--threshold` is a CRAP score in `crap` and a similarity in `dry`. | `crap --threshold 30`, `dry --threshold 0.9` |
| 32 | An internal error's message is Go's error text. | a source file that cannot be read |
