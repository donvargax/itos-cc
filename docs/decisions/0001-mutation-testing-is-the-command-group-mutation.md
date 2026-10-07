---
status: accepted
date: 2026-10-07
---

# Mutation testing is the command group mutation

## Context and Problem Statement

Name the read-only command, and does it replace mutate --scan? (a, recommended) One command, itos-cc sites, that lists each selected function's mutation sites with their cached outcome, marks missing or stale ones, and exits 1 as q-2 says; mutate --scan goes, a breaking change made in the next major release with the others (rules 42, 43), which also clears the rule-18 row in docs/CLI.md. (b) Same, under another name you pick (not mutants: a letter from mutate breaks rule 5, as crap and scrap do). (c) A check-only command, keeping mutate --scan as a listed exception.

Asked as q-3, about mutate-check-sample.

## Considered Options

- A group, mutation, with the actions run, list and check (rules 3, 4, 7, 13)
- Subcommands under mutate (mutate sites): mutate alone would still run mutants, a new implicit default subcommand (rule 7), and a verb as a group name (rule 3)
- A read-only command of its own beside mutate, such as itos-cc sites: no rule broken, but mutation testing split across two unrelated names
- mutate --check, keeping mutate --scan: two flags that select another action (rule 18)

## Decision Outcome

A group, mutation, following rules 3, 4, 7 and 13: mutation run (today's mutate), mutation list (today's mutate --scan) and mutation check (the check mode). mutate and --scan go with no alias; itos-cc is pre-1.0 (v0.1.1), so a breaking change needs only a minor release.

### Consequences

itos-cc mutate is gone with no alias: mutation run replaces it, mutation list replaces mutate --scan, and mutation check is the check mode; the problem rules are mutation.*. The cache folder .metrics/mutate/ and the "itos-cc mutate:" summary marker stay, since renaming them would rewrite every committed cache and duplicate every annotation. A new mutation action, such as the sample mode, joins the group as a verb. Before 1.0 a breaking change like this one ships in a minor release; docs/CLI.md says so.
