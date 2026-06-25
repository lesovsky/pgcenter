---
status: planned                    # planned -> in_progress -> done
depends_on: ["02"]                 # ID задач-зависимостей (строки: ["01", "02"])
wave: 2                            # волна параллельного выполнения
skills: [code-writing]             # МАССИВ скиллов для загрузки
verify: user                       # инструмент верификации (опционально: curl, bash, user)
reviewers: [dev-code-reviewer, dev-test-reviewer]  # явно указать. Пусто = fallback на defaults
teammate_name:                     # имя агента-исполнителя (опционально; если не задано — генерируется по описанию задачи)
---

# Task 06: Verbose-aware layout() geometry

## Required Skills

Перед выполнением задачи загрузи:
- `/skill:code-writing` — [skills/code-writing/SKILL.md](~/.claude/skills/code-writing/SKILL.md)

## Description

The top band of the `top` TUI is two side-by-side panels — `sysstat` (left) and `pgstat` (right) — each
4 printed rows tall, with the `cmdline` and the main `dbstat` table pinned immediately below at fixed
y-coordinates. Verbose mode (hotkey `v`, plumbed in Task 2) grows these panels with extra `label:value`
rows: `sysstat` gains +3 rows, `pgstat` gains +5 rows (asymmetric). When verbose is on, the band must
genuinely grow from the top and push `cmdline` and `dbstat` **down** (so `dbstat` loses rows at the top,
not the bottom — unlike the `extra` panel, which is a visual overlay).

Today this geometry is five hard-coded `SetView(name, x0, y0, x1, y1)` literals inside the `layout(app)`
closure in `top/ui.go` (the `4` in both panel calls, the `cmdline` `3`/`5`, and `dbstat`'s `4`). These
literals are the coupling that must become verbose-aware. Per **Decision 3**, the geometry arithmetic
(compact vs verbose, asymmetric heights, height-guard) must be extracted into a **pure**
`topBandLayout(verbose, maxY)` function — table-testable without gocui — leaving `layout()` to do only the
`SetView` plumbing. This mirrors the [009] `visibleColumns` precedent (a pure, unit-tested geometry function
pulled out of the render path).

A **height-guard** protects short terminals: if `maxY` cannot fit the expanded band plus `cmdline` (2 rows)
plus the `dbstat` header (1 row) plus at least 1 data row, the function returns the compact coordinates and
signals (via an `expanded`/fallback flag) that the band did not expand — `layout()` then emits a hint on
`cmdline` telling the user the terminal is too short for verbose.

The `config.verbose` read happens inside the `layout(app)` closure, which runs in the gocui handler
goroutine (the same goroutine that already reads `app.config.view.ShowExtra` at `ui.go:166`), so there is
**no data race** with the collector goroutine.

## What to do

- Create `top/layout.go` and add a pure function `topBandLayout(verbose bool, maxY int)` that returns the
  per-view y-coordinates needed by `layout()` plus a boolean indicating whether the band actually expanded.
  Suggested signature (from code-research §1-new): returns `sysstatY1, pgstatY1, cmdlineY0, cmdlineY1,
  dbstatY0 int, expanded bool`. The function does pure integer arithmetic — no gocui, no `app`.
  - Compact (verbose=false OR height-guard tripped): reproduce today's literals exactly — `sysstatY1 = 4`,
    `pgstatY1 = 4`, `cmdlineY0 = 3`, `cmdlineY1 = 5`, `dbstatY0 = 4`.
  - Verbose (expanded): `sysstatY1 = 4 + 3` (sysstat +3), `pgstatY1 = 4 + 5` (pgstat +5). `cmdline` and
    `dbstat` must clear the **taller** of the two panels: `bandTop = max(sysstatY1, pgstatY1) - 1`,
    `cmdlineY0 = bandTop`, `cmdlineY1 = bandTop + 2`, `dbstatY0 = max(sysstatY1, pgstatY1) + 1`.
  - Height-guard: when verbose is requested but `maxY` cannot fit
    `expanded-band + cmdline(2) + dbstat header(1) + ≥1 data row`, return the compact coordinates and
    `expanded = false`. This is a pure comparison on `maxY` — derive the minimum-`maxY` threshold from the
    verbose `dbstatY0` (need `dbstatY0 + 1 (header) + 1 (≥1 data row) ≤ maxY - 1`, matching the existing
    `dbstat` `y1 = maxY-1`). Confirm the exact off-by-one against the current literals so compact output is
    byte-identical.
- Rewire `layout(app)` in `top/ui.go` to call `topBandLayout(app.config.verbose, maxY)` once, then feed the
  returned coordinates into the existing four `SetView` calls (`sysstat`, `pgstat`, `cmdline`, `dbstat`) in
  place of the hard-coded literals. Keep the `(maxX-1)/2`, `maxX/2`, `maxX`, `-1` x-coordinates and the
  `Frame = false` / `SetCurrentView` / error-handling blocks unchanged.
- When verbose is requested but the height-guard tripped (`expanded == false` while `app.config.verbose ==
  true`), emit a short hint on `cmdline` (e.g. via `printCmdline`) telling the user the terminal is too short
  for verbose mode. Keep it to a single concise line and avoid spamming it on every redraw frame (see
  Implementation hints).
- Leave the conditional `extra` view block (`ui.go:166-180`) untouched — it is an independent overlay and out
  of scope for this task.
- Add `top/layout_test.go` with a table test of `topBandLayout` (see TDD Anchor).

## TDD Anchor

Тесты, которые нужно написать ДО реализации. Пишем → запускаем → убеждаемся что падают → пишем код → убеждаемся что проходят.

- `top/layout_test.go::Test_topBandLayout` — table test of the pure geometry function, with sub-cases:
  - `compact` — `verbose=false`, normal `maxY` → returns today's exact literals
    (`sysstatY1=4, pgstatY1=4, cmdlineY0=3, cmdlineY1=5, dbstatY0=4`), `expanded=false`.
  - `verbose` — `verbose=true`, tall enough `maxY` → asymmetric heights (`sysstatY1=7`, `pgstatY1=9`),
    `cmdline`/`dbstat` cleared below the taller (`pgstat`) panel, `expanded=true`.
  - `height-guard` — `verbose=true`, `maxY` too short to fit `band+cmdline+header+≥1 row` → returns the
    compact coordinates and `expanded=false` (graceful fallback, no broken/negative coords).
  - `boundary` — the smallest `maxY` that still expands and the largest that still falls back (assert the
    threshold is exactly where the height-guard flips).

## Acceptance Criteria

- [ ] `topBandLayout` is a pure function (no gocui, no `app`) in `top/layout.go`, table-tested in `top/layout_test.go`.
- [ ] Compact path is byte-identical to current behavior: the four `SetView` calls receive the same
      coordinates as the present hard-coded literals when `verbose=false`.
- [ ] Verbose path grows `sysstat` by +3 and `pgstat` by +5 rows; `cmdline` and `dbstat` shift **down** to
      clear the taller (`pgstat`) panel.
- [ ] Height-guard: on a terminal too short to fit the expanded band + cmdline + header + ≥1 data row,
      `layout()` falls back to compact and emits a cmdline hint (no broken/overlapping/negative-height views).
- [ ] `layout()` no longer contains hard-coded band-height literals — they come from `topBandLayout`.
- [ ] `go test ./top/...` passes; `make lint` (golangci-lint + gosec) clean.

## Context Files

**Feature artifacts:**
- [010-feat-overview-dashboard.md](010-feat-overview-dashboard.md) — user-spec (see "Состав и источники строк" + "Вертикальное место")
- [010-feat-overview-dashboard-tech-spec.md](010-feat-overview-dashboard-tech-spec.md) — tech-spec (Task 6, Decision 3)
- [010-feat-overview-dashboard-code-research.md](010-feat-overview-dashboard-code-research.md) — §1-new "layout() geometry — THE invasive core"
- [010-feat-overview-dashboard-decisions.md](010-feat-overview-dashboard-decisions.md) — decisions log

**Project knowledge:**
- [overview.md](.claude/skills/project-knowledge/overview.md) — project context (features, audience)
- [architecture.md](.claude/skills/project-knowledge/architecture.md) — package layout, data flow, `top` UI loop
- [patterns.md](.claude/skills/project-knowledge/patterns.md) — code patterns + testing conventions (table tests)

**Code files:**
- [top/ui.go](top/ui.go) — rewire `layout(app)` (`ui.go:103-184`); the five `SetView` literals at lines 114/128/139/155/167
- [top/layout.go](top/layout.go) — NEW: pure `topBandLayout(verbose, maxY)`
- [top/layout_test.go](top/layout_test.go) — NEW: table test of `topBandLayout`
- [top/stat.go](top/stat.go) — READ: `renderDbstat` writer-split precedent + the sysstat/pgstat row counts (to confirm +3/+5)

## Verification Steps

- `go test ./top/...` — `Test_topBandLayout` passes for compact / verbose / height-guard / boundary cases.
- `make build && make lint` — builds clean, no new lint/gosec findings.
- **Manual (verify: user):** launch `pgcenter top`, press `v`:
  - On a normal-height terminal: the top band grows (sysstat +3, pgstat +5), `cmdline` and the `dbstat`
    table move down, `dbstat` loses rows at the top, the table still renders without overlap.
  - On a deliberately short terminal (shrink the window): pressing `v` does NOT break the layout — it stays
    compact and shows a "terminal too short for verbose" hint on `cmdline`.
  - Press `v` again to toggle back: compact layout is visually identical to before.
  - Try several terminal sizes around the height-guard boundary to confirm the fallback flips cleanly.

## Details

<!-- All details for task execution — technical, organizational, any other. -->

**Files:**
- `top/layout.go` — NEW. Pure `topBandLayout(verbose bool, maxY int) (sysstatY1, pgstatY1, cmdlineY0,
  cmdlineY1, dbstatY0 int, expanded bool)`. Package `top`. Stdlib only (no gocui import).
- `top/ui.go` — rewire `layout(app)`. Current literals to replace (all inside the closure at `ui.go:103-184`):
  - `ui.go:114` `SetView("sysstat", -1, -1, (maxX-1)/2, 4)` → `…, sysstatY1)`
  - `ui.go:128` `SetView("pgstat", maxX/2, -1, maxX, 4)` → `…, pgstatY1)`
  - `ui.go:139` `SetView("cmdline", -1, 3, maxX, 5)` → `…, cmdlineY0, maxX, cmdlineY1)`
  - `ui.go:155` `SetView("dbstat", -1, 4, maxX, maxY-1)` → `…, dbstatY0, maxX, maxY-1)`
  - `ui.go:167` `extra` view — LEAVE UNCHANGED (out of scope).
- `top/layout_test.go` — NEW table test (see TDD Anchor).

**Dependencies:**
- Depends on **Task 2** (verbose plumbing): `app.config.verbose` must exist on `top.config`. Task 2 is in
  Wave 1; this task is Wave 2. If `config.verbose` is not yet present at start, re-read `top/config.go` —
  Task 2 adds it.
- This is the only Wave-2 task touching `top/ui.go`, `top/layout.go`, `top/layout_test.go` (file-disjoint from
  Task 5).

**Edge cases:**
- Very short terminal (`maxY` small): height-guard must fall back to compact — never return coordinates that
  overlap, invert (`y0 > y1`), or push `dbstat` off-screen. `layout()` already returns an empty error and
  re-draws when `maxX == 0 || maxY == 0` (`ui.go:109`); the height-guard is a separate, softer fallback for
  small-but-nonzero `maxY`.
- gocui coords are **inclusive** and `-1` means one row off-screen — preserve that convention; the band's
  top stays at `-1`.
- `cmdline` deliberately overlaps the bottom of the panels and is drawn after them (z-order). Keep that
  relationship: `cmdlineY0 = bandTop` where `bandTop = max(sysstatY1, pgstatY1) - 1`.
- Compact output must be byte-identical (refactor is behavior-preserving) — verify the compact branch
  reproduces `4 / 4 / 3 / 5 / 4` exactly.

**Implementation hints (НЕ псевдокод):**
- Asymmetric heights from code-research §1-new: `sysExtra = 3`, `pgExtra = 5`; `sysstatY1 = 4 + sysExtra`,
  `pgstatY1 = 4 + pgExtra`. The two panels keep independent `y1`; `cmdline`/`dbstat` clear the taller one
  via `max(sysstatY1, pgstatY1)`. Confirm the +3/+5 against the row counts that Task 8 will add (sysstat: 3
  verbose rows; pgstat: 5 verbose rows) — the band height must match the rows actually printed.
- Height-guard threshold: derive from the verbose `dbstatY0`. The minimum viable terminal needs
  `dbstatY0 + 1 (header) + 1 (≥1 data row) ≤ maxY - 1` (since `dbstat`'s `y1` is `maxY-1`). Compute this
  with the verbose coordinates and, if it fails, return the compact set with `expanded = false`. Keep the
  comparison purely on `maxY` so it is table-testable.
- For the cmdline hint, `printCmdline(app.ui, "...")` already exists (`ui.go:187`) and auto-clears after 2s;
  it is the natural channel. Guard against re-emitting it on every redraw frame if that proves noisy (e.g.
  only when verbose is requested but `expanded == false`) — a concise single-line hint is sufficient.
- Reuse the existing `SetView` error-handling / `Frame = false` / `SetCurrentView("sysstat")` blocks verbatim;
  only the coordinate arguments change.
- Mirror the [009] `visibleColumns` precedent (`top/stat.go:429`): a small pure geometry function with an
  exhaustive table test, no terminal needed.

## Reviewers

- **dev-code-reviewer** → `010-feat-overview-dashboard-task-06-dev-code-reviewer-review.json`
- **dev-test-reviewer** → `010-feat-overview-dashboard-task-06-dev-test-reviewer-review.json`

## Post-completion

- [ ] Записать краткий отчёт в [010-feat-overview-dashboard-decisions.md](010-feat-overview-dashboard-decisions.md) (Summary: 1-3 предложения, ревью со ссылками на JSON, без таблиц файндингов и дампов)
- [ ] Если отклонились от спека — описать отклонение и причину (особенно если +3/+5 высоты или порог height-guard уточнились при реализации)
- [ ] Обновить user-spec/tech-spec если что-то изменилось
