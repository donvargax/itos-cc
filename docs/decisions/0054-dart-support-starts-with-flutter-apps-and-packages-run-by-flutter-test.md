---
status: accepted
date: 2026-10-23
---

# Dart support starts with Flutter apps and packages run by flutter test

## Context and Problem Statement

Which Dart projects does the first version support? (a, recommended) Flutter apps and packages, run with flutter test, all five commands (units, dry, scrap, crap, mutation), Flutter 3.27 or newer with Dart 3.6 (pub workspaces); pure Dart packages (dart test) become a later idea. (b) Flutter and pure Dart together from the start: a second runner, coverage path base and CI install to build and keep. (c) Flutter with fewer commands first (units, dry, scrap, crap) and mutation later.

Asked as q-74, about dart-support.

## Considered Options

The options are those the question names.

## Decision Outcome

(a), the person on 2026-10-21: Flutter apps and packages, run with flutter test, all five commands, Flutter 3.27 or newer with Dart 3.6; pure Dart packages (dart test) become a later idea.

### Consequences

Flutter 3.27 or newer with Dart 3.6 (pub workspaces); all five commands. Pure Dart packages run by dart test are a later idea (dart-pure-packages).
