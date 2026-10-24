---
status: accepted
date: 2026-10-23
---

# itos-cc.yaml's include brings back only what itos-cc leaves out by default

## Context and Problem Statement

How does a project bring back a file itos-cc leaves out by default, a Flutter app's platform folder (q-75) or generated Dart (q-77)? (a, recommended) A new itos-cc.yaml key, include: globs checked as exclude's are, which bring back only what itos-cc leaves out by default for Dart (platform folders, generated files), never what exclude names, which still wins; the default-skipped dependency and cache folders (ID-SEL-10) stay out. (b) The same key for every default, node_modules and .d.ts included. (c) No key yet: a project that wants these measured is a later idea, and q-75/q-77's 'itos-cc.yaml can bring it back' waits.

Asked as q-80, about dart-support.

## Considered Options

The options are those the question names.

## Decision Outcome

(a) The person, 2026-10-21: a new itos-cc.yaml key, include:, globs checked as exclude's are, bringing back only what itos-cc leaves out by default for Dart (a Flutter app's platform folders, generated Dart); exclude still wins; the default-skipped dependency and cache folders (ID-SEL-10) stay out.

### Consequences

include: takes globs checked as exclude's are; exclude still wins; the default-skipped dependency and cache folders (ID-SEL-10) stay out.
