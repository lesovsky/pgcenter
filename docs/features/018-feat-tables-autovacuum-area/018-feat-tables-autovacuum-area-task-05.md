---
status: in_progress
depends_on: [01, 03]
wave: 3
model_tier: standard
covers: [US-1, US-2, US-3, US-4, US-5, AC-10, AC-11, AC-13, AC-14, AC-15, AC-16, AC-17, AC-18, AC-19, AC-20, AC-35]
acceptance_scenarios: [SC-5, SC-6, SC-7, SC-8]
skills: [code-writing]
verify: go test ./top/ -run 'Tables|Menu|Keybindings|ToggleSysTables|help'; manual SC-5 through SC-8 remain final stand checks
reviewers: [dev-code-reviewer]
---

# Task 05: Version-aware tables navigation, system toggle, and help

## Outcome

PG 19 users can reach `autovacuum_scores` through `t` and `T`; earlier-version users retain legacy `t` behavior and receive an explicit disabled-menu explanation. The existing `,` toggle and built-in help cover the new screen.

## What to do

- Add `tablesNextView(current string, version int)` and the `case "tables"` dispatch in `switchViewTo`. On PG 19 cycle tables→autovacuum_scores→tables; below PG 19 stay on tables; from every other screen return tables. Preserve the group/view/report/tar name collision comment.
- Add `menuTables` and bind `T`; keep the existing lowercase `t` binding bytes unchanged. Pass the server version into menu construction, show two entries on every version, and append exactly `— requires PostgreSQL 19` to the second label below PG 19.
- Add menu selection for both entries. Below PG 19, selecting the marked entry sends no view change, leaves the current view unchanged, prints one NOTICE explaining the requirement, and still closes/resets the menu.
- Apply Decision 7: move the shared cmdline write into each relevant selection branch so every path has exactly one write. Do not add a counting seam or alter the shared cmdline composer.
- Wire all four `toggleSysTables` literals, including the slice length, and update the test table to use the new query's filter markers. Do not replace the existing ViewType mechanism.
- Update help: remove the ambiguous `'t' tables,` fragment from the `s,t,i` row and add the `t,T` row after `w,W`, preserving pinned adjacency and the single formatting placeholder.

## Behaviors to Test

- Cycle table across current view × PG 18/19/out-of-range versions → exact target; unrelated screens always lead to tables.
- Menu construction below/at/above PG 19 → two entries, suffix only below 19, stable cursor mapping.
- Disabled selection → no viewCh send, unchanged view, closed/reset menu and one refusal branch; enabled selections switch correctly.
- Bound `T`/`t` handlers → correct menu/cycle behavior and exactly-once registration only in the intended view.
- `,` on the new screen → formatted predicate changes and all four internal literals stay consistent; activity remains a no-op.
- Help → exactly one `t` marker, correct prefix/description and required adjacency.

## Acceptance Criteria

- [ ] AC-13–AC-20 and AC-35 are implemented and unit-tested at the available seams.
- [ ] AC-10/AC-11 are connected to the existing system-relation toggle; fixture truth remains Task 01's integration proof.
- [ ] AC-17 exactly-one-message behavior is established by branch review and remains mandatory in manual SC-5/SC-7; no unobservable unit claim is made.
- [ ] Other cycles, menus, shared rendering and cmdline code are unchanged.

## Context

**Feature artifacts:**
- `docs/features/018-feat-tables-autovacuum-area/018-feat-tables-autovacuum-area.md` — user-spec.
- `docs/features/018-feat-tables-autovacuum-area/018-feat-tables-autovacuum-area-tech-spec.md` — Decisions 6 and 7.
- `docs/features/018-feat-tables-autovacuum-area/018-feat-tables-autovacuum-area-decomposition.md` — waves and Acceptance Execution Map.
- `docs/features/018-feat-tables-autovacuum-area/018-feat-tables-autovacuum-area-decisions.md` — decisions and deviations.

**Project knowledge:**
- `.kimi-code/skills/project-knowledge/references/architecture.md`
- `.kimi-code/skills/project-knowledge/references/patterns.md`

**Starting points (verified, not restrictive):**
- `top/config_view.go` and `config_view_test.go` — switchViewTo, walNextView and four-literal toggle.
- `top/menu.go` and `menu_test.go` — menuWAL shape and refusal/reset path.
- `top/keybindings.go` and `keybindings_test.go` — free T key and assertable handler list.
- `top/help.go` and `help_test.go` — pinned help layout.
- `top/top.go` — VersionNum initialization.

## Verification Steps

- `go test ./top/ -run 'Tables|Menu|Keybindings|ToggleSysTables|help' -count=1` → named tests execute and pass.
- `go test ./top/ -count=1` → full package passes.
- During final stand QA, run SC-5–SC-8 with fresh `make build`, deterministic tmux geometry and a `master` binary → record manual evidence without treating unit tests as substitutes.
- Apply Decision 12 to every SC-5–SC-8 artifact → use a no-secret synthetic workload and no credential values; keep raw tmux/session transcripts outside Git under mode 0700 with trap-based deletion; publish only minimized redacted captures after `assert_sanitized` and secret-scan checks. The stand runner records this gate verdict and freeze-qa verifies it.

## Notes

Do not retrofit version-awareness into `x` or `p`; Task 06 registers that debt. Do not change shared sanitization or frozen-width rendering.

## Reviewers

- **dev-code-reviewer** → `docs/features/018-feat-tables-autovacuum-area/018-feat-tables-autovacuum-area-task-05-dev-code-reviewer-round{R}.json`
