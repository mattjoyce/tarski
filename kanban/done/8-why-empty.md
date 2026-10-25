---
id: 8
status: done
---
# Why is this answer empty?

## Job story
When a question over the record comes back empty, I want the engine to say why, so an empty answer is never mistaken for "there is nothing" (Anam card 182, "silent empties are the worst failure for an assistant"). (Drafted by datalog-module for Matt.)

## Narrative
- 2026-10-25: `Result.WhyEmpty(name, maxDepth)` is WhyNot with every goal position open (`_`, which the walk already treats as a fresh open position), sharing one `explain` with WhyNot. ok is false for a non-empty relation, an unknown one, or a run without proofs. An empty base relation reads "no such fact" (nil Rules). The text reads `high(_) is not derived ... stopped at V > 140 with At="2026-07-20", V=128`; JSON writes an open position as null, as WhyNot already did for open positions in Because. Tests: whyempty_test.go. (by datalog-module)
