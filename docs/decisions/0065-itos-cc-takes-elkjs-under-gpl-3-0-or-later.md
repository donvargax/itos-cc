---
status: accepted
date: 2026-11-09
---

# itos-cc takes elkjs under GPL-3.0-or-later

## Context and Problem Statement

Embedding the viewer puts elkjs 0.12.0 (the graph layout library viewer/src/layout.ts imports) into every release binary, the first non-permissive dependency it carries: elkjs is licensed EPL-2.0 OR GPL-3.0-or-later, and itos-cc is AGPL-3.0. Under which does itos-cc take it? (a, recommended) GPL-3.0-or-later: GPLv3's section 13 lets a GPLv3 work be combined with an AGPLv3 one, so the binary stays AGPL-3.0 and THIRD_PARTY_NOTICES names elkjs under GPL-3.0-or-later with its text. (b) EPL-2.0: its weak copyleft is not GPL-compatible unless the distributor names GPL as a Secondary License, which elkjs's choice already offers, so (a) is the simpler reading. (c) Do not bundle elkjs: replace it with a permissive layout library first, a larger change. The coordinator is not a lawyer; the person decides.

Asked as q-87, about slice-42.

## Considered Options

The options are those the question names.

## Decision Outcome

GPL-3.0-or-later (the person, 2026-10-24): itos-cc takes elkjs under GPL-3.0-or-later, which GPLv3's section 13 lets combine with AGPLv3, so the binary stays AGPL-3.0 and THIRD_PARTY_NOTICES names elkjs under GPL-3.0-or-later with its licence text.

### Consequences

The release binary stays AGPL-3.0, which GPLv3's section 13 lets combine with it, and THIRD_PARTY_NOTICES names elkjs under GPL-3.0-or-later with its licence text. A later non-permissive dependency is asked the same way.
