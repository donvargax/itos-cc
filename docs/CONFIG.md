# Configuration format guidelines

These rules cover every file this project reads or writes as its own configuration. They
apply whatever the project's language or interface. Formats that belong to other tools, such
as `go.mod`, a linter's settings, CI workflows or `itos.yaml`, follow those tools' rules
instead.

The project has no configuration file of its own yet, and applies these rules when it adds its
first one. Nothing here is implemented yet: there is no loader, no schema and no command that
prints defaults. Each rule's source is named. An interface's own guide covers how configuration
is found and how flags and environment variables override it. Where the project has a command
line, that guide is `docs/CLI.md`.

The rules are adapted from [itos-template](https://github.com/donvargax/itos-template)'s
`docs/CONFIG.md` with its owner's permission (`CONTRIBUTORS.md`).

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
