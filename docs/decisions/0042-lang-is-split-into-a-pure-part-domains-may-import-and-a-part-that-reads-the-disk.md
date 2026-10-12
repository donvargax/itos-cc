---
status: accepted
date: 2026-10-11
---

# lang is split into a pure part domains may import and a part that reads the disk

## Context and Problem Statement

The duplication and test-structure domains (S3) work on parsed source, lang.File and its tree-sitter nodes, but depguard's domain rule lets domains import only domains, and lang is not pure: parsing a Go file reads the nearest go.mod (lang/golang.go). Allow lang in domains with parsing kept in handlers by a task check, keep dry/ and scrap/ top-level beside lang, or split lang first into a pure part domains may import and a parsing part that reads the disk?

Asked as q-58.

## Considered Options

The options are those the question names.

## Decision Outcome

Split lang first (the person, 2026-10-11): a pure part (File, Unit, Spec, the syntax nodes) that domains may import, and the part that reads the disk kept out of it; a task of its own before dry and scrap move.

### Consequences

A task before S3 splits it; depguard's domain rule allows only the pure part; dry/ and scrap/ then move under internal/ as domains (ADR-0037).
