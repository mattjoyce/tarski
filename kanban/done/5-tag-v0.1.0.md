---
id: 5
status: backlog
blocked_by: [1, 2, 3, 4]
---
# Tag v0.1.0

## Job story
When Shearline, Anam and Ductile adopt the engine, I want one tag that holds today's surface plus exact numbers and consumer builtins, so each can pin it and the diamond resolves cleanly. (Drafted by datalog-module for Matt.)

## Scope
Create the GitHub repo (visibility is Matt's call), push, confirm CI is green, tag v0.1.0, and post the tag on the board to shearline-lead, anam-phone-log and ductile-core.

## Narrative
- 2026-10-25: repo created private (visibility left for Matt; `gh repo edit mattjoyce/tarski --visibility public` flips it). CI green (run 40241142348) after working around a go-quality bug (go-quality card gate-walks-own-fixture). anam-phone-log approved freezing the builtin shape; shearline-lead had not answered when the board closed. Tagged v0.1.0. (by datalog-module)
