---
status: accepted
date: 2026-10-11
---

# Exit codes follow square/exit, keeping 75 for retry

## Context and Problem Statement

Should itos-cc and go-template-itos adopt square/exit's semantic exit codes?

Asked as q-56.

## Considered Options

The options are those the question names.

## Decision Outcome

Adopt them, keeping 75 for a failure that may pass when run again (answered 2026-10-11): 0, 1 for a check's no, 80 usage, 81 unknown command, 82 a requirement not met (was 3), 100 internal (was 70), 75 retry, 130 interrupted. Precedence 130, 80 and 81, 100, 82, 75, 1. A feat in the same minor release as exit 130 (q-51), proposed to go-template-itos alongside.

### Consequences

Scripts reading 2, 3 or 70 break: a minor release with exit 130, called out in Upgrading, and proposed to go-template-itos alongside.
