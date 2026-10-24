---
status: accepted
date: 2026-10-23
---

# The Linux release links statically against musl

## Context and Problem Statement

The release links Linux and Windows statically (release.yml: -extldflags=-static), so each binary carries, beyond the Go modules THIRD_PARTY_NOTICES now names, the Go runtime and standard library (BSD-3, a notice suffices), MinGW-w64's runtime and winpthreads on Windows (permissive notices), and glibc's static libc on Linux (LGPL-2.1: a static link asks more than a notice, the means for a user to relink, such as the object files). This is not legal advice; what should the release do? (a, recommended) Add Go's LICENSE and MinGW-w64's notices to THIRD_PARTY_NOTICES, and build the Linux binary static against musl (MIT) instead of glibc, keeping a binary that runs on any Linux with nothing more to ship. (b) Same notices, and link Linux dynamically against glibc, built on the oldest supported runner so it runs on older systems; no relink duty, but the binary needs a compatible glibc. (c) Same notices, keep static glibc, and publish what LGPL's relink terms ask (object files and link instructions) with each release.

Asked as q-81, about T-176.

## Considered Options

The options are those the question names.

## Decision Outcome

(a), the person on 2026-10-21: build Linux statically against musl instead of glibc, keeping the run-anywhere binary (Alpine and scratch images included), and add the Go runtime's, MinGW-w64's and musl's notices to THIRD_PARTY_NOTICES. Dynamic glibc was weighed and passed over: it cannot run on Alpine or scratch images.

### Consequences

The binary runs anywhere, Alpine and scratch images included; THIRD_PARTY_NOTICES carries the Go runtime's, MinGW-w64's and musl's notices. Dynamic glibc was passed over: it cannot run on Alpine or scratch.
