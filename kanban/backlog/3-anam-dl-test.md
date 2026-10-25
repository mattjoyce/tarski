---
id: 3
status: backlog
blocks: [5]
---
# Anam's rules compile and evaluate here

## Job story
When the engine changes, I want Anam's unmodified `rules/anam.dl` compiled and evaluated in this repo's tests, so a change that would break Anam's query fails here first. (Drafted by datalog-module for Matt; asked by shearline-lead and anam-phone-log.)

## Scope
Copy `anam.dl` from anam origin/master into `testdata/`, with its source commit noted. Compile it against a schema of `entry/7`, `attr/3`, `closes/1` and evaluate a small hand-made base, checking `current/2` on a known case.

## Acceptance
- `anam_test.go` passes; README and AGENTS.md name it.
