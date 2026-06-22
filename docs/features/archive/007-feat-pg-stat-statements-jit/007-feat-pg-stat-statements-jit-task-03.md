---
status: done                       # planned -> in_progress -> done
depends_on: ["02"]                 # ID задач-зависимостей (строки: ["01", "02"])
wave: 3                            # волна параллельного выполнения
skills: [code-writing]             # МАССИВ скиллов для загрузки
verify: user                       # local PG17: X → JIT opens; x cycles wal → jit → timings; make build clean
reviewers: [dev-code-reviewer, dev-test-reviewer]
teammate_name:
---

# Task 03: TUI menu item + x-cycle wiring

## Required Skills

Перед выполнением задачи загрузи:
- `/skill:code-writing` — [skills/code-writing/SKILL.md](~/.claude/skills/code-writing/SKILL.md)

## Description

Expose the new `statements_jit` view (registered in Task 02) through the two TUI entry points
that every other pgss sub-screen already uses:

1. The uppercase `X` menu — add a 7th `menuPgss` item ("pg_stat_statements JIT compilation")
   and its `menuSelect` handler so a DBA can pick the JIT screen explicitly.
2. The lowercase `x` cycle — insert `statements_jit` into `statementsNextView` between
   `statements_wal` and `statements_timings`, so repeatedly pressing `x` walks through the JIT
   screen in order.

This is pure TUI plumbing — no query logic, no view registration (Task 02 owns that), and no
keybinding changes. The `X` key (`menuOpen(menuPgss, …)`) and the `x` key
(`switchViewTo(app, "statements")`) are already wired in `top/keybindings.go` and already guard
the absence of `pg_stat_statements` (empty `ExtPGSSSchema`), so they are not touched here.

This task depends on Task 02: the `statements_jit` view must exist in the view map, otherwise
`viewSwitchHandler` would resolve a zero-value `View{}` when the menu/cycle targets it.

## What to do

1. In `top/menu.go`, `selectMenuStyle` → `case menuPgss` items slice (currently 6 entries,
   indices 0–5), add a 7th entry at index 6 immediately after `" pg_stat_statements WAL usage"`:
   `" pg_stat_statements JIT compilation"`. Keep the leading space to match the existing item
   formatting. The menu UI auto-sizes its height to `len(s.items)` (menu.go:115) — do NOT touch
   the `SetView` geometry.
2. In `top/menu.go`, `menuSelect` → `case menuPgss` inner `switch cy` (currently handles
   `case 0`–`case 5` then `default`), add `case 6: viewSwitchHandler(app.config, "statements_jit")`
   immediately after `case 5` (`statements_wal`) and before `default`. Leave the existing
   `default` (`statements_timings`) unchanged.
3. In `top/config_view.go`, `statementsNextView`, insert `statements_jit` into the cycle between
   `wal` and `timings`: change the `case "statements_wal":` body from
   `next = "statements_timings"` to `next = "statements_jit"`, and add a new
   `case "statements_jit": next = "statements_timings"`. Leave the trailing `default` unchanged.
4. Do NOT modify `top/keybindings.go` — `x` and `X` are already wired and already guard the
   missing-pgss case.
5. Run `make build` to confirm the package compiles cleanly.

## TDD Anchor

<!-- No automated test for this layer. -->

There is no automated test for this layer and none is added here. The TUI menu list and the
`x`-cycle next-view chain have no existing Go unit-test coverage — every prior pgss-menu wiring
(`statements_io`, `statements_wal`) and the `x`-cycle entries were added the same way and
verified only by `make build` plus a manual TUI check. `top/menu.go` and `top/config_view.go`
exercise gocui UI objects and the live view map that cannot be driven without a running terminal
+ PostgreSQL. Verification for this task is therefore: `make build` (compile) + manual TUI check
on local PG17 (see Verification Steps). Do not invent or add speculative tests for this layer.

## Acceptance Criteria

- [ ] `top/menu.go`: `menuPgss` items slice has a 7th entry (index 6)
      `" pg_stat_statements JIT compilation"` right after `" pg_stat_statements WAL usage"`.
- [ ] `top/menu.go`: `menuSelect` `case menuPgss` has `case 6:
      viewSwitchHandler(app.config, "statements_jit")` before `default`.
- [ ] `top/config_view.go`: `statementsNextView` cycle is `… wal → jit → timings …`
      (`case "statements_wal"` → `statements_jit`; new `case "statements_jit"` →
      `statements_timings`).
- [ ] `top/keybindings.go` is unchanged.
- [ ] `make build` is clean.
- [ ] Manual (local PG17): pressing `X` shows a 7th menu item; selecting it opens the JIT screen.
      Pressing `x` cycles `wal → jit → timings`.

## Context Files

**Feature artifacts:**
- [007-feat-pg-stat-statements-jit.md](007-feat-pg-stat-statements-jit.md) — user-spec
- [007-feat-pg-stat-statements-jit-tech-spec.md](007-feat-pg-stat-statements-jit-tech-spec.md) — tech-spec (Task 3; Solution §3; "How it works" steps 1–2; Backward Compatibility note on cycle order)
- [007-feat-pg-stat-statements-jit-code-research.md](007-feat-pg-stat-statements-jit-code-research.md) — code-research §5 (menu + cycle wiring, exact anchors)
- [007-feat-pg-stat-statements-jit-decisions.md](007-feat-pg-stat-statements-jit-decisions.md) — decisions log

**Project knowledge:**
- [overview.md](.claude/skills/project-knowledge/overview.md) — project context (no project.md in this repo; overview.md is the entry point)
- [architecture.md](.claude/skills/project-knowledge/architecture.md) — package layout, data flow, view/menu wiring
- [patterns.md](.claude/skills/project-knowledge/patterns.md) — code/testing conventions, version branching

**Code files:**
- [top/menu.go](top/menu.go) — modify: `selectMenuStyle` `case menuPgss` items (menu.go:53–60); `menuSelect` `case menuPgss` switch (menu.go:155–171)
- [top/config_view.go](top/config_view.go) — modify: `statementsNextView` (config_view.go:160–180)
- [top/keybindings.go](top/keybindings.go) — read only: `x` (keybindings.go:40), `X` (keybindings.go:45) already wired; NO change

## Verification Steps

- `make build` → builds `./bin/pgcenter` with no errors.
- Manual on local PG17 (`bin/pgcenter`):
  - Press `X` → menu lists 7 items, the last being "pg_stat_statements JIT compilation".
    Select it → the JIT screen opens (view `statements_jit`, its `Msg` shown on the cmdline).
  - From a pgss screen press `x` repeatedly → the cycle reaches `statements_wal`, then the JIT
    screen, then `statements_timings` (order: `… wal → jit → timings …`).
  - Confirm no regression: every other pgss screen is still reachable via `X` and `x`.

## Details

<!-- All details — based on reading the actual files. -->

**Files:**
- `top/menu.go` — current `case menuPgss` items slice has exactly 6 entries (menu.go:54–59):
  timings, general, input/output, temp files input/output, temp tables (local) input/output,
  WAL usage. Add the 7th. The `menuSelect` `case menuPgss` `switch cy` (menu.go:156–171) maps
  `0→statements_timings, 1→statements_general, 2→statements_io, 3→statements_temp,
  4→statements_local, 5→statements_wal, default→statements_timings`. Add `case 6` →
  `statements_jit` before `default`. The index must line up with the new item's slice position
  (6). Menu height auto-sizes from `len(s.items)` at menu.go:115 — no geometry edit.
- `top/config_view.go` — `statementsNextView` (config_view.go:160–180) currently chains
  `timings→general→io→temp→local→wal→timings`. Change `case "statements_wal"` target from
  `statements_timings` to `statements_jit`, and add `case "statements_jit"` →
  `statements_timings`. `default` stays `statements_timings`.

**Dependencies:** Task 02 (the `statements_jit` view must be registered in `view.New()` /
`config.views`, otherwise `viewSwitchHandler` resolves an empty `View{}`).

**Edge cases:**
- Missing `pg_stat_statements`: already handled — `menuOpen(menuPgss, …)` prints
  "NOTICE: pg_stat_statements not found" when `ExtPGSSSchema == ""` (menu.go:110–113), and
  `switchViewTo(app, "statements")` prints a NOTICE and keeps the current view (config_view.go:105).
  No change needed.
- PG<15: the JIT view is gated by `MinRequiredVersion: query.PostgresV15` (Task 02). The runtime
  collector guard (`view.VersionOK`) returns "selected statistics is not supported by current
  version of Postgres" — same graceful degrade as `statements_wal` on PG<13. The menu/cycle
  target it unconditionally; no extra guard is added here (consistent with existing behavior).
- Index drift: the `case 6` index and the slice position (index 6) must match; an off-by-one
  here silently maps the menu item to the wrong view.

**Implementation hints:**
- Match the existing leading-space + label formatting exactly (e.g. `" pg_stat_statements JIT
  compilation"`).
- Insert `case 6` strictly between `case 5` and `default` in `menuSelect`; do not reorder other
  cases.
- The cycle change is the one user-visible behavior shift (one extra stop inserted between `wal`
  and `timings`) — the tech-spec Backward Compatibility section explicitly approves it; no other
  transition is reordered.

## Reviewers

- **dev-code-reviewer** → `007-feat-pg-stat-statements-jit-task-03-dev-code-reviewer-review.json`
- **dev-test-reviewer** → `007-feat-pg-stat-statements-jit-task-03-dev-test-reviewer-review.json`

## Post-completion

- [ ] Записать краткий отчёт в [007-feat-pg-stat-statements-jit-decisions.md](007-feat-pg-stat-statements-jit-decisions.md) (Summary: 1-3 предложения, ревью со ссылками на JSON, без таблиц файндингов и дампов)
- [ ] Если отклонились от спека — описать отклонение и причину
- [ ] Обновить user-spec/tech-spec если что-то изменилось
