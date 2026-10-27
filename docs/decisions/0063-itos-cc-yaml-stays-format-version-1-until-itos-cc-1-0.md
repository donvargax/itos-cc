---
status: accepted
date: 2026-10-26
---

# itos-cc.yaml stays format version 1 until itos-cc 1.0

## Context and Problem Statement

Does itos-cc.yaml's format version move while itos-cc is still 0.x? docs/CONFIG.md says every key added comes with a new version (q-98's slice-53), so q-101's hash version key would make files version 2, which an older pinned itos-cc refuses.

Asked as q-104, about except-rehash.

## Considered Options

- Keep version 1 while itos-cc is 0.x, adding keys within it; versions move after 1.0 (chosen)
- Every new key brings a new format version now, as CONFIG.md's rule 6 said

## Decision Outcome

Person 2026-10-27: no. A file format version must not move while itos-cc is still 0.x; versioning the format starts after itos-cc 1.0. Until then itos-cc.yaml stays version 1 and keys are added within it (an older itos-cc meeting a key it does not know refuses it as an unknown key, as today), so q-101's hash version key and any other new key land in version 1.

### Consequences

docs/CONFIG.md's rules 5-7 apply from itos-cc 1.0: before it, a new key is added to version 1, and an older itos-cc meets it as an unknown key (config.invalid, exit 80, with its Did you mean). q-101's hash version key lands in version 1. slice-62's spec gave 'a key needs a new format version' as the reason for no opt-out setting; that reason no longer holds, though the call itself stands until a project asks.
