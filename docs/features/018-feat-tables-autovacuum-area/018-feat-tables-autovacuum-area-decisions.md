# Decisions Log: 018-feat-tables-autovacuum-area

One entry per task or significant phase. Record outcomes and deviations, not chatter.

---

## Phase: Feature execution — wave 1 started

**Summary:** Approved standalone execution resumed under the current inherited-model policy. Tasks 01 and 02 are in progress in isolated worktrees; requested tiers remain `strong` and `standard` respectively, but no runtime model alias is exposed.
**Deviations:** The earlier pause pending a secondary model pool is superseded by the current feature-execution contract, which explicitly permits inherited selection. Scripted feature acceptance other than continuous SC-4 remains owned by freeze-qa after wave 4.
**Tech debt:** None added.
**Verification:** Clean feature branch, ready freeze `44c953532a043de5de847f679df27b65c019a408`, valid manifest and approved specifications/decomposition confirmed before task dispatch.

---

## Phase: Feature execution — wave 1 integrated

**Summary:** Tasks 01 and 02 passed two review rounds each and are integrated at `0fe1b53`. Task 01 delivers the locked autovacuum query, real PG 19 privilege/order/catalog tests, and a fail-closed disposable wraparound supervisor. Task 02 delivers PG 19 trailing `stats_age` selectors with live PG 14–19 layouts and safe temporary NULL probes.
**Deviations:** Round 1 required outer-relation test coverage, fail-closed Docker/test evidence, network isolation, and removal of destructive persistent probes; all findings were fixed and both reviewers approved round 2. No incremental feature acceptance run was performed: the installed runner executes all five frozen scenarios, while the approved decomposition assigns feature acceptance, including the continuous SC-4 rerun, to freeze-qa after wave 4. No acceptance result is claimed here.
**Tech debt:** None added. The pre-existing `prepare-test-environment.sh: line 36: all: command not found` diagnostic remains outside this feature; all clusters and tests nevertheless became available and passed.
**Verification:** Integrated `go test ./internal/query/ -count=1` and `go test -race ./internal/query/ -count=1` passed in the PG 14–19 image using the host Go 1.25.13 toolchain mounted read-only. Integrated `testing/run-autovacuum-wraparound-test.sh` produced a test-level PASS and verified external destruction. Frozen acceptance files are unchanged. The first integration-container attempt could not start Go because the testing image does not bundle it; the corrected standard run supplied the toolchain explicitly.

---

## Task 01: Autovacuum scores query and isolated integration proof

**Summary:** Implemented the eleven-column PG 19 query, absolute dead tuples, outer-side schema filtering, wraparound exception and raw-score ordering. Added structural/live/privilege tests and the isolated supervisor with identity, network and cleanup controls.
**Review/fix rounds:** Two code and two security rounds; one fix commit. Final reports contain zero findings.
**Verification:** Real PG 19 tests, targeted wraparound PASS, timeout/SIGKILL/daemon-error controls, full query package and race runs passed. Outer-relation, rounded-alias and removed-wraparound mutations reddened their named assertions.

---

## Task 02: Version-aware stats_age queries

**Summary:** Added PG 19 tables/indexes selectors with trailing nullable `stats_age`, preserving legacy layouts and diff intervals. Live tests pin PG 14–19 field descriptions and NULL rendering using per-session temporary relations.
**Review/fix rounds:** Two code and two security rounds; one fix commit replaced unsafe persistent probes. Final reports contain zero findings.
**Verification:** Targeted and full query package tests passed on PG 14–19 without skipped live subtests; both source mutations reddened the named assertions. Integrated race testing passed.

---

## Phase: Feature execution — wave 2 integrated

**Summary:** Task 03 is integrated at `e3efd93`. The registry and load-bearing selectors are wired, deliberate recording exclusions now emit deterministic truthful INFO, the PG 17 recorder uses filtered views, and no-diff empty-cell behavior is mutation-checked.
**Deviations:** None from the task contract. Incremental acceptance remains deferred to the approved post-wave-4 freeze-qa run; no result is claimed.
**Review/fix rounds:** One code-review round, zero findings and no fix round.
**Verification:** Integrated `go test ./internal/view/ ./record/ ./internal/stat/ -count=1` and `go test ./internal/query/ -count=1` passed in the PG 14–19 image. The non-zero-interval mutation reddened both named empty-cell assertions. Frozen acceptance files remain unchanged.

---

## Phase: Feature execution — wave 3 integrated

**Summary:** Tasks 04 and 05 are integrated at `092912a`. Recorded relation layout changes now form safe replay boundaries with complete renderer/sort reset; PG 19 navigation, disabled-menu refusal, system toggle and help are wired.
**Deviations:** Task 04's existing error-path fixture now uses a malformed numeric counter because a same-version column-list change is intentionally valid. Security review reproduced the pre-existing malformed-`Ncols` panic and classified it as out-of-scope debt [020]; it remains open and is not claimed fixed. Scripted SC-2/SC-3/SC-4/SC-10 remain assigned to the post-wave-4 freeze-qa run.
**Review/fix rounds:** Tasks 04 and 05 each passed one code-review round with zero findings; Task 04 passed security review with no in-scope finding. No fix rounds.
**Verification:** Integrated report/record/top/view/query/stat tests passed in the PG 14–19 image. All 12 replay matrix variants passed without skips, and layout-comparison and seed-sort mutations produced named red assertions. Frozen acceptance files remain unchanged.

---

## Phase: Feature execution — wave 4 integrated

**Summary:** Task 06 is integrated at `539364a`. Feature catalog, Project Knowledge, release notes and debt registers match the implemented behavior. Debt [037] records the deliberately unfixed version-blind `x`/`p` cycles.
**Deviations:** None. Existing [020] is byte-unchanged; no shared renderer, parser or sanitizer fix is claimed.
**Review/fix rounds:** One code-review round with zero findings and no fix round. Manual SC-9 explicitly passed AC-37, AC-38, AC-39 and AC-40.
**Verification:** Documentation consistency checks and `git diff --check` passed. Only the five approved documentation/register files changed in the task commit. All scripted feature acceptance remains assigned to freeze-qa.

---

## Phase: Feature execution — stopped for owner review after SC-1 re-freeze

**Summary:** Whole-feature review round 1 found a destructive rounded-tie fixture, an invalid freeze-qa path and stale roadmap text. Separate product/test commit `b5af8f2` and docs commit `191cc07` address those findings; final re-review remains pending. The owner approved one SC-1 network-isolation re-freeze and, after the first triage, a narrow archive-ownership correction. Harness-only commits `fcea62e` and `f1660ea` preserve all assertions and are separate from product changes.
**Stop condition:** Clean `base-reFreeze-1` failed on root-owned archive readability; triage classified it as harness-only. After ownership repair, clean `base-reFreeze-2` reached report assertions but failed because container UTC archive filenames and host `+0500` report bounds exclude valid ticks. Triage round 2 classified this as a new harness timezone defect and a second comparable harness failure. The execution contract therefore requires an immediate feature stop before any third automatic fix or rerun.
**Current state:** All six implementation tasks remain done and integrated. No feature-r1 result or final QA is claimed. Both re-freeze base runs are preserved with `dirty=false`; the other four scenarios retained valid base outcomes in each run. `acceptance/freeze.json` still points to the original `44c9535` freeze and has not been relabelled ready for the modified harness. Whole-feature code/security re-review after the fixes is pending.
**Next decision:** The owner must either stop for manual handling or explicitly reauthorize one cohesive harness-only UTC correction, manifest refresh, clean full base re-freeze, feature validation and independent acceptance/code/security reviews. Product code must not change in that harness commit.
**Verification:** Read-only triage reports round 1/2 and sanitized `base-reFreeze-1`/`base-reFreeze-2` evidence are committed. No raw archive, transcript or credential evidence is included.

---

## Phase: Feature execution — complete and ready for freeze-qa

**Summary:** All four waves and six tasks are implemented, reviewed and integrated. Requirements coverage is complete at implementation/test level for US-1–US-5 and AC-1–AC-41; SC-9 passed. Scripted SC-1/SC-2/SC-3/SC-4/SC-10 passed the re-freeze feature validation, but fresh freeze-qa `feature-r1`, manual SC-5–SC-8 and the final AC-32 quality gate remain pending and are not claimed here.
**Whole-feature review:** Round 1 findings were fixed in separate product/test and docs commits. Round 2 code review approved with zero findings. Round 2 security review approved with zero in-scope findings; only existing out-of-scope [020] and [029] remain recorded.
**Acceptance:** One owner-approved SC-1 batch re-freeze completed after two preserved failed attempts and explicit reauthorization. Accepted freeze SHA is `affb927d2fd536172e31a3efb3e6146d0ba3b57d`; independent freeze review approved with zero findings. Feature revalidation `reFreeze-3-feature` passed all five scenarios and does not replace fresh `feature-r1`.
**Selection mode:** `inherited` throughout. Requested strong/standard tiers were planning metadata only; the runtime exposed no model aliases.
**Deviations/follow-ups:** No product dependencies or shared renderer/parser/sanitizer changes were added. Existing [020]/[029]/[035] and new [037] remain follow-ups, as does planned record/report support for autovacuum_scores. No push, integration-branch merge, deployment or feature-branch deletion occurred.
**Next phase:** Start a new session and run `/skill:freeze-qa docs/features/018-feat-tables-autovacuum-area/018-feat-tables-autovacuum-area`. Provide PG 19 and PG 18 stands for manual SC-5–SC-8 and A/B comparison.

---

## Phase: Acceptance harness — owner-approved batch re-freeze approved

**Summary:** The explicitly reauthorized cohesive UTC correction completed the single SC-1 batch re-freeze. Harness-only commits `fcea62e`, `f1660ea` and `e36af86` isolate PostgreSQL with network-none, retain restrictive host-owned archive access and align recorder/report timestamps in UTC without weakening assertions.
**Evidence:** Accepted `base-reFreeze-3` at `affb927` is clean and manifest-valid: SC-2/SC-4 pass and SC-1/SC-3/SC-10 emit exactly their approved feature-missing checkpoints. Feature validation `reFreeze-3-feature` at `e36af86` is clean with all five scenarios `0/pass/complete`. Failed `base-reFreeze-1` and `base-reFreeze-2` remain preserved.
**Review:** Independent `dev-acceptance-reviewer` approved the complete repair history and both final runs with zero findings. `freeze.json` now points to `base-reFreeze-3` readiness and manifest digest `f6a3ddadef225e12477ec9ac5fd4b5f67286bdec9fb5e741b9b5baadff1de195`.
**Boundary:** This is harness revalidation, not final QA. Freeze-qa must still run its fresh `feature-r1`; manual SC-5–SC-8 and final quality gates remain pending.

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

---

## Phase: Freeze QA — failed mandatory vulnerability gate

**Summary:** Fresh `feature-r1` passed all five frozen scenarios with valid markers, `dirty=false` and `manifestValid=true`. Manual SC-5–SC-8 also passed in owner-authorized local Docker/tmux runs with master A/B and sanitized minimized evidence; SC-9 remained passed. Build, race-enabled PG 14–19 tests, lint, E2E and the dedicated wraparound test passed. The mandatory `make vuln` gate failed on GO-2026-6809, GO-2026-6629 and GO-2026-6607, so QA is failed and Gate 3 was not opened.
**Evidence:** `018-feat-tables-autovacuum-area-qa-report-r1.json`; `acceptance/runs/feature-r1/result.json`; `logs/working/qa-018/quality-summary.txt`, `make-vuln.log`, `make-vuln-master.log`, and `manual-sc5.log` through `manual-sc8.log`.
**Triage/deferred:** No acceptance scenario failed, so no acceptance triage report was required. No criterion was deferred to post-deploy. The three vulnerabilities reproduce identically on master, but their baseline origin does not convert AC-32/TS-2 to a pass.
**Next action:** The owner must decide on separate dependency/toolchain remediation and then rerun fresh freeze-qa at a reviewed HEAD. No product, dependency or harness fix was made by this phase; `/skill:done`, push, merge and release were not invoked.
