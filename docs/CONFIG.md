# Configuration format guidelines

These rules cover every file this project reads or writes as its own configuration. They
apply whatever the project's language or interface. Formats that belong to other tools, such
as `go.mod`, a linter's settings, CI workflows or `itos.yaml`, follow those tools' rules
instead.

The project's own configuration file is `itos-cc.yaml`, at the project root. These rules are
the policy for its format and for any file the project adds; they are not a description of
what the loader does. Where the two differ, the section below says so, and changing the loader
to follow a rule is behaviour work of its own, with its scenarios, never part of a docs or
refactor change. Each rule's source is named. An interface's own guide covers how configuration
is found and how flags and environment variables override it. Where the project has a command
line, that guide is `docs/CLI.md`, whose rule 38 lists `itos-cc.yaml`'s keys.

The rules are adapted from [itos-template](https://github.com/donvargax/itos-template)'s
`docs/CONFIG.md` with its owner's permission (`CONTRIBUTORS.md`).

## itos-cc.yaml as implemented

The mutation slice's settings adapter, `internal/slices/mutation/adapters/settings` (the
top-level `config` package until T-197), reads `itos-cc.yaml` (`settings.Parse`) for every
command, and the slice's exceptions adapter writes it (`mutation except`, `mutation except
--renew` and `mutation except --prune`), reading it with `settings.Parse` first. Its keys are `version`, `exclude`, `mutation.exceptions`,
`mutation.tests` and `mutation.copy` (`docs/CLI.md`, rule 38). Against the rules below, the loader as it stands:

| Rule | What the loader does now |
| ---- | ------------------------ |
| 1 | Reads YAML with `go.yaml.in/yaml/v3` into Go structs. A key given twice is refused, and so is an anchor, an alias or a merge key (`&x`, `*x`, `<<`) anywhere in the document, for every command, at the key whose value carries the first anchor in document order, or at the merge key itself (`mutation.<<`), its fix to write the value in place (bug-8). So are, in document order with those, a tag written in the file other than `!` and JSON's (`!!str`, `!!int`, `!!float`, `!!bool`, `!!null`, `!!seq`, `!!map`), such as `!foo` or `!!python/object`, at the key whose value carries it, a list or a mapping as a key, at the mapping holding it, and a second document after `---`, naming the file alone, each with a fix of its own (slice-49); a plain value YAML resolves as a timestamp, carrying no tag written, is read as its key's string. |
| 2 | A key the format does not list is refused, at every level, for every command, after rule 1's refusals and before any value is checked, at its path from the top (`mutation.exceptions[0].line_txt`); its fix asks "Did you mean `line_text`?", or "Did you mean one of `each`, `sep`?" for several in their order, naming each listed key at that place that starts with the key or is within two edits of it (Levenshtein), or, when none is, the one listed key all but the key's last letter begins, five letters at least, the command line's rule from the kong fork (decision 62, slice-60), and otherwise names the keys listed there in their order (`version, exclude, mutation` at the top level) (slice-52). The listed keys are one table held as data (`settings.format`), which the reader walks the document against and a JSON Schema can be generated from. A key starting with exactly `x-` is an extension key, as Docker Compose and OpenAPI allow, so other tools and people can keep their own data in the file: it is accepted and ignored at every level (`x-owner`, `mutation.x-note`, `mutation.exceptions[0].x-seen`), never decoded, its value anything JSON can express and still held to rule 1, and `mutation except`, `--renew` and `--prune` keep it byte for byte as every byte they do not write, and in an entry they rewrite after the keys they write (rule 17); `X-owner` and `x_owner` are keys like any other (slice-54). |
| 3 | Stops at the first problem. An `exclude` or `mutation.copy` glob's problem, an anchor's, a merge key's, a tag's, a key's that is not a string, a `version`'s and a key's the format does not list carry their key from the top (`exclude[1]`, `mutation.copy[0]`, `mutation.<<`); an exception's or `mutation.tests`' names it in the message (`mutation.exceptions[0] has no reason`). |
| 4 | Checks each `exclude` and `mutation.copy` glob (empty, absolute, `..`, a backslash, or no pattern), each exception's required keys, and `mutation.tests`' placeholders, before any command uses them. |
| 5–7 | `version` is a JSON integer, and 1 is the only version so far (slice-53). A file without it reads as version 1, for good, with no refusal and no note, so every file written before the key keeps working (q-98); this departs from rule 5's "begin every file with `version`", since refusing such a file would break every project on upgrade and gain nothing. A missing, empty or comment-only file is an empty configuration, as before. A version newer than this itos-cc reads is `config.invalid`, exit 80, keyed `version`, its message naming both versions and its fix to upgrade itos-cc; a value that is not a positive integer as JSON writes one (`"1"`, `1.5`, `0`) is `config.invalid`, keyed `version`. The check runs after rule 1's refusals and before rule 2's, so `version` is a listed key. From this release every key added comes with a new version, and a file of version 1 is read as written for good; itos-cc never rewrites a file to migrate it. `mutation except`, creating the file, starts it with `version: 1`; a file it finds without one is left without one. |
| 8–11 | Keys are snake case (`line_in_function`, `ids_pattern`); exceptions and tests are objects in lists; paths and globs are slash-separated, from the project root, the folder the file is in. |
| 12 | `mutation.tests`' commands are single strings a shell runs, not lists of words. |
| 13 | Holds no secrets. |
| 14 | Its keys are documented in `docs/CLI.md` and README, without a typed reference of every default. |
| 15–16 | No command prints the file with its defaults, and no JSON Schema is published. |
| 17 | `mutation except`, `--renew` and `--prune` change only the lines of the entries they write or remove (a removed entry with the comment lines directly above it), in block style with the codec's key order, keeping every other byte of the document, its other entries included, and its permissions; an entry they rewrite keeps its `x-` keys, with their values and the comment lines directly above each, after the keys they write (slice-55); a list written in flow style is rewritten whole in block style, and a flow-style `mutation` mapping, which they cannot splice into, has the whole document re-encoded. |

A file that is not there is an empty configuration. One whose text the loader refuses is
`config.invalid`, exit 80; one the disk cannot read is an internal failure, exit 100.

## Sources

| Key    | Source                                                                                                                                   |
| ------ | ---------------------------------------------------------------------------------------------------------------------------------------- |
| K8S    | Kubernetes API Conventions, <https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md> |
| AIP    | Google API Improvement Proposals, <https://google.aip.dev>; the number after the key names the proposal, for example AIP `126`           |
| CLIG   | Command Line Interface Guidelines, <https://clig.dev>, `#configuration`                                                                  |
| JSON   | RFC 8259, The JavaScript Object Notation (JSON) Data Interchange Format, <https://www.rfc-editor.org/rfc/rfc8259>                        |
| YAML   | YAML Ain't Markup Language, version 1.2.2, <https://yaml.org/spec/1.2.2/>                                                               |
| SCHEMA | JSON Schema, <https://json-schema.org>                                                                                                   |
| OWNER  | The principles of itos-template's owner, carried from itos-template                                                                      |

## Principles

- **Convention over configuration, made visible.** A key may have a default, so a file says only
  what differs. Every default is written down and can be printed, so a reader never has to guess
  one (OWNER). A reader cannot learn a convention from the file alone, so the tool that reads the
  file shows it.
- **Explicit is better than implicit.** A value that changes what a tool does is written in the
  file or shown as a default, never inferred from something else (OWNER).
- **JSON is the data model, YAML its syntax.** A file is plain JSON data: objects, arrays,
  strings, numbers, booleans and null (JSON). People write it as YAML, which also reads JSON
  (YAML), so a tool may write JSON into the same `.yaml` file and it reads the same (OWNER).
- **No language of our own.** A format that needs loops, functions or shared fragments is a job
  for a configuration language that outputs JSON, such as CUE, Pkl, Jsonnet or Dhall. That
  language runs on the author's side, and the project reads only its output (OWNER).

## Rules

### Reading a file

1. Read a file as YAML into JSON's data model, and refuse anything JSON cannot express. That
   means custom tags (`!!python/object`, `!foo`), more than one document in a file, anchors,
   aliases and merge keys (`&x`, `*x`, `<<`), keys that are not strings, and a key given twice.
   A tag can make a reader build an arbitrary object (the class of CVE-2022-1471), and an alias
   can expand without bound (the "billion laughs" of CVE-2019-11253). Treat every file as
   untrusted. The rule refuses only what JSON cannot express, not every YAML spelling: quotes,
   comments and block or flow style are accepted. So is the non-specific tag `!`, which builds
   nothing and is read as if it were not there (OWNER; YAML; JSON).
2. Refuse a key the format does not list, so a misspelt key never passes for an option (K8S;
   OWNER).
3. Name every problem at once, each by the path of its key from the top (`servers[1].port`), so
   an author fixes them all in one go (AIP `193`).
4. Check every value before using any. A reference to another part of the file names something
   the file holds, a pattern is valid, and a value is one its key accepts.

### Versions and change

5. Begin every file with `version`, an integer. A tool refuses a file whose version it does not
   know, so an earlier tool never misreads a file written for a later one (K8S).
6. A new key comes with a new version that names it. A tool keeps reading every earlier version
   as it was written (K8S; AIP `180`).
7. Removing a key, renaming it or changing what a value means is a breaking change. It needs a
   major release and an upgrade note telling authors what to change (AIP `180`).

### Shape

8. Name keys in snake case, the same style in every file (AIP `140`).
9. Write a list of named things as a list of objects, each with its `name`, never as an object
   keyed by name. A list keeps its order, and every entry stays the same shape (K8S, "Lists of
   named subobjects preferred over maps").
10. Prefer a string from a known set over a boolean for anything that may grow a third value. Prefer
    a list over a single value for anything that may hold more than one (`scans: [credentials]`)
    (K8S, "Primitive types"; AIP `126`).
11. Write paths relative to the file, with `/` as the separator on every system.
12. Keep a value whole. A command is a list of words (`[go, test, ./...]`), not a string a tool
    splits.
13. Hold no secrets. A credential belongs in the environment or a secret store, never in a file
    the repository keeps (`docs/security.md`).

### Defaults and documentation

14. Document every key where people read the format: its type, whether it is required and its
    default, with an example.
15. Let the tool print a file as it read it, with every default filled in, so convention stays
    visible (OWNER; CLIG `#configuration`).
16. Publish a JSON Schema for each format, versioned with it. An editor can then check and
    complete a file, and any tool that writes one has a contract to follow (SCHEMA).

### Writing a file

17. A tool writes a file in the format's own key order, one key per line, using no YAML-only
    feature. What it writes then reads back the same, and a diff shows only what changed.
