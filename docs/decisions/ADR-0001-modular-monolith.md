# ADR-0001: Modular Monolith

Status: Accepted

## Context

CommerceOps is initially being built by a very small team with AI-assisted development. It needs clear module boundaries without the operational complexity of distributed services.

## Decision

CommerceOps will begin as a modular monolith.

The Go backend will run as one primary application while business domains remain internally separated.

## Consequences

Benefits:
- simpler development
- simpler deployment
- easier debugging
- lower infrastructure cost
- easier AI comprehension

Tradeoff:
Individual modules cannot initially scale independently.

Modules may later be extracted if measured requirements justify it.