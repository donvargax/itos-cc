---
status: accepted
date: 2026-10-11
---

# The pure lang asks a port for what surrounds a file, with the context first

## Context and Problem Statement

Splitting lang (decision 42): lang names a namespace by looking around the file on disk (Go's nearest go.mod with a module line, TypeScript's package.json/tsconfig.json, Python's pyproject.toml/setup.py/setup.cfg, the working directory). How does the pure lang get those answers, where does the disk code go, and does Parse take a context? Recommend: lang keeps every language's rules and asks a port declared in internal/lang/port (exists, read, working directory, absolute path; each method context first; a porttest fake); Parse(ctx, …) and UnitsOf(ctx, …) take it; the OS implementation, ParseFile and FindUp live in project, and internal/workdir implements the port for slices and domains; packages not yet refactored call project directly and pass context.TODO() until S4 threads a context. Alternatives: the caller supplies the module path (every caller learns each language's markers); a Surroundings interface with no context, the slice capturing ctx in a closure (a context held in a value, against decision 30); the disk part in a lang/disk subpackage or a new internal adapter.

Asked as q-59.

## Considered Options

The options are those the question names.

## Decision Outcome

As recommended (the person, 2026-10-11): lang is a core part of the domain and follows its rules whole, a real port with the context first, never an exception to save work.

### Consequences

About 40 call sites gain a context; packages not yet refactored pass context.TODO(), which S4 replaces. project holds the OS calls lang made, in the same order, so every namespace and byte stays the same. internal/workdir, the selection adapter, also implements lang's port. Domain tests parse in memory through the porttest fake.
