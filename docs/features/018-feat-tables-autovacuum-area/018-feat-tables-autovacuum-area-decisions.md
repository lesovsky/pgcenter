# Decisions Log: 018-feat-tables-autovacuum-area

One entry per task or significant phase. Record outcomes and deviations, not chatter.

---

## Phase: Claude-to-Kimi process migration

**Summary:** Migrated active feature artifacts to the six-phase Kimi SDLC without changing the approved product scope. Added stable US/AC/SC identifiers, explicit scriptable/manual acceptance ownership, current Project Knowledge paths, and the required technical coverage maps. The pre-existing task/wave draft is retained only as an unapproved appendix for the next decomposition session.
**Deviations:** CLI record/report and regression checks now have external acceptance scenarios; interactive TUI verification remains manual. Independent review also exposed a valid same-PG19 pgcenter-upgrade replay gap, so AC-41/SC-10 and Decision 9 were added as required compatibility work rather than leaving the original 'unaffected' claim false. Decisions 10–12 bind destructive-fixture isolation, named recording INFO, and sanitized evidence. No new screen, column set, or other product scope beyond the approved tables/autovacuum area and its required replay safety was added.
**Tech debt:** Scope updates for existing [029] and [035] are required by AC-40. Existing feature decisions about [020] and the version-blind `x`/`p` cycles are unchanged.
**Proposed follow-ups:** In a fresh session on `feature/018-feat-tables-autovacuum-area`, run `/skill:decompose-tech-spec docs/features/018-feat-tables-autovacuum-area/018-feat-tables-autovacuum-area`; then complete independent plan/harness review and acceptance freeze before implementation.
**Verification:** Project `project-knowledge` skill loaded from `.kimi-code`; `git diff --check` and JSON/JSONL parsing passed. Final independent reports are approved: user-spec round 4, tech-spec round 3, security round 3, and skeptic round 3 (74/52/56 factual claims checked across skeptic rounds, zero mismatches). Earlier reports are preserved. Runtime implementation, mutation, acceptance, and stand evidence remain future execution gates and are not claimed here.
