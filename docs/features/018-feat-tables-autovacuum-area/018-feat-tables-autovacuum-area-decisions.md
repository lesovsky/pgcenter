# Decisions Log: 018-feat-tables-autovacuum-area

One entry per task or significant phase. Record outcomes and deviations, not chatter.

---

## Phase: Feature execution — wave 1 started

**Summary:** Approved standalone execution resumed under the current inherited-model policy. Tasks 01 and 02 are in progress in isolated worktrees; requested tiers remain `strong` and `standard` respectively, but no runtime model alias is exposed.
**Deviations:** The earlier pause pending a secondary model pool is superseded by the current feature-execution contract, which explicitly permits inherited selection. Scripted feature acceptance other than continuous SC-4 remains owned by freeze-qa after wave 4.
**Tech debt:** None added.
**Verification:** Clean feature branch, ready freeze `44c953532a043de5de847f679df27b65c019a408`, valid manifest and approved specifications/decomposition confirmed before task dispatch.

---

## Phase: Acceptance harness freeze

**Summary:** Froze the five-scenario black-box harness at SHA `44c953532a043de5de847f679df27b65c019a408`. Final `base` is clean and manifest-valid: SC-2/SC-4 pass; SC-1/SC-3/SC-10 emit exactly their approved feature-missing checkpoints. Independent semantic freeze review approved with zero findings.
**Deviations:** No recorded baselines or external providers apply. The first run labelled `base` was preserved as `base-attempt-3-dirty` after Python bytecode made its tree dirty; bytecode generation was disabled and the final `base` rerun passed cleanly.
**Tech debt:** None added by the harness. Existing report exit-code debt [034] is handled by output assertions rather than misclassified as feature absence.
**Verification:** `acceptance/runs/base/result.json`, `acceptance/runs/base/readiness.json`, `acceptance/freeze.json`, manifest digest `94bf65bf1fb2a1a92de6d1722a1feb15b4af9a57a2e8c26588921544b08d3e6d`, and `018-feat-tables-autovacuum-area-acceptance-freeze-review.json`. Earlier attempts and sanitized repair evidence are preserved under `acceptance/runs/`.

---

## Phase: Decomposition

**Summary:** Approved six implementation/documentation tasks in four waves. Tasks 01/02 build independent query layers; Task 03 registers views and truthful recording exclusions; Tasks 04/05 independently cover replay and TUI; Task 06 reconciles documentation. The Acceptance Execution Map covers SC-1–SC-4 and SC-10, with SC-5–SC-9 explicitly manual. This decomposition phase owns harness creation and freeze; freeze-qa owns the post-implementation `feature-r1` execution.
**Deviations:** No separate implementation task owns acceptance harness production or final QA; those belong to the decomposition and freeze-qa phases respectively. Task 01 has a concrete disposable-PG19 supervisor contract, and Decision 12 sanitization applies to scripted and manual evidence.
**Tech debt:** Implementation retains the approved [035]/[029] updates, leaves [020] unchanged, and registers the version-blind x/p cycles in Task 06.
**Verification:** Independent decomposition review round 2 approved with zero findings; independent skeptic review approved after checking 124 factual claims with zero mismatches. DAG is acyclic and same-wave product file ownership is disjoint.

---

## Phase: Feature execution — paused before wave 1

**Summary:** Execution planning completed and validated: branch `feature/018-feat-tables-autovacuum-area` clean at `e4d54e9`, base `2dbd626`, freeze `ready` at SHA `44c9535`, six tasks all `planned`, DAG acyclic, waves and reviewers confirmed. No task was started.
**Deviations:** Execution is paused by owner decision because the runtime Agent tool exposes no `model` parameter and no `[secondary_model]` pool is configured, so the required strong/standard tier routing cannot be performed or honestly recorded. The owner will configure the pool in a separate session, then rerun `/skill:feature-execution docs/features/018-feat-tables-autovacuum-area/018-feat-tables-autovacuum-area`.
**Tech debt:** None added.
**Verification:** `git status` clean; freeze.json verdict `ready`; telemetry event `paused_pending_model_pool`.

---

## Phase: Claude-to-Kimi process migration

**Summary:** Migrated active feature artifacts to the six-phase Kimi SDLC without changing the approved product scope. Added stable US/AC/SC identifiers, explicit scriptable/manual acceptance ownership, current Project Knowledge paths, and the required technical coverage maps. The pre-existing task/wave draft is retained only as an unapproved appendix for the next decomposition session.
**Deviations:** CLI record/report and regression checks now have external acceptance scenarios; interactive TUI verification remains manual. Independent review also exposed a valid same-PG19 pgcenter-upgrade replay gap, so AC-41/SC-10 and Decision 9 were added as required compatibility work rather than leaving the original 'unaffected' claim false. Decisions 10–12 bind destructive-fixture isolation, named recording INFO, and sanitized evidence. No new screen, column set, or other product scope beyond the approved tables/autovacuum area and its required replay safety was added.
**Tech debt:** Scope updates for existing [029] and [035] are required by AC-40. Existing feature decisions about [020] and the version-blind `x`/`p` cycles are unchanged.
**Proposed follow-ups:** In a fresh session on `feature/018-feat-tables-autovacuum-area`, run `/skill:decompose-tech-spec docs/features/018-feat-tables-autovacuum-area/018-feat-tables-autovacuum-area`; then complete independent plan/harness review and acceptance freeze before implementation.
**Verification:** Project `project-knowledge` skill loaded from `.kimi-code`; `git diff --check` and JSON/JSONL parsing passed. Final independent reports are approved: user-spec round 4, tech-spec round 3, security round 3, and skeptic round 3 (74/52/56 factual claims checked across skeptic rounds, zero mismatches). Earlier reports are preserved. Runtime implementation, mutation, acceptance, and stand evidence remain future execution gates and are not claimed here.
