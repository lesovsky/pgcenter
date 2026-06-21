---
status: done                       # planned -> in_progress -> done
depends_on: ["01"]                 # ID задач-зависимостей
wave: 2                            # волна параллельного выполнения
skills: [code-writing]             # МАССИВ скиллов для загрузки
verify: bash                       # инструмент верификации (опционально: curl, bash, user)
reviewers: [dev-code-reviewer, dev-test-reviewer]  # явно указать. Пусто = fallback на defaults
teammate_name:                     # имя агента-исполнителя (опционально; если не задано — генерируется по описанию задачи)
---

# Task 03: TUI navigation, menu & help — top/

## Required Skills

Перед выполнением задачи загрузи:
- `/skill:code-writing` — [skills/code-writing/SKILL.md](~/.claude/skills/code-writing/SKILL.md)

## Description

Wire the TUI navigation for the new `pg_stat_io` screen into `pgcenter top`. The screen is split into
two sub-views — **count** (`stat_io`, operations + KiB) and **time** (`stat_io_time`, timings) —
registered as static views in Task 02. This task adds the user-facing controls that let a DBA reach
and toggle those sub-views, exactly mirroring the established `databases` family (lowercase switch +
uppercase menu).

Concretely:
- `j` enters the count screen from any other view and toggles count↔time on subsequent presses
  (a 2-way `statioNextView` helper, copy of `databasesNextView`).
- `J` opens a 2-item menu (`pg_stat_io operations`, `pg_stat_io timings`) via a new `menuStatIO` type.
- Help text gains a `j`/`J` line and a note clarifying that `Q` does NOT reset `pg_stat_io` (it is
  shared/cluster-wide stats, reset only via `pg_stat_reset_shared('io')` — same behaviour as
  bgwriter/wal/replslots).

This is the last wiring layer: keybinding → `switchViewTo`/`menuOpen` → `viewSwitchHandler` loads the
view named `stat_io` / `stat_io_time` from the static map. No new mechanism is introduced — every
piece copies an existing precedent in the same files.

**Naming contract (fixed — do NOT rename):** view names `stat_io` / `stat_io_time`, `switchViewTo`
token `"statio"`, menu type `menuStatIO`. This is the same three-way naming the `databases` family
uses (`databases` / `databases_general` / `menuDatabases`). The view names `stat_io` /
`stat_io_time` are the registration contract from Task 02 — switch to them verbatim.

**Dependency note:** depends on Task 01 for the broader feature (version constants / query layer)
but is **file-independent** of Task 02 (which touches `internal/view/view.go`). At runtime the `j`/`J`
controls will only render data once the views from Task 02 exist; the unit tests in this task
(`menuStatIO` item-count) and `make build` do not require Task 02. Per Decision 7 (navigation +
naming contract), Decision 9 (`track_io_timing` hint surfaced via the time-view `Msg` — owned by
Task 02, this task only switches to it), and Decision 10 (`NotRecordable`, no record/report wiring).

## What to do

1. **`top/keybindings.go`** — add two bindings to the `sysstat` view, alongside the existing
   lowercase-switch / uppercase-menu pairs (`d`/`D`, `x`/`X`, `p`/`P`):
   - `{"sysstat", 'j', switchViewTo(app, "statio")}` — lowercase: enter + toggle count↔time.
   - `{"sysstat", 'J', menuOpen(menuStatIO, app.config, "")}` — uppercase: open the 2-item menu
     (the `pgssSchema` arg is `""` — the pgss-availability guard in `menuOpen` only fires for
     `menuPgss`).
   The generic `"menu"` bindings (Esc/Up/Down/Enter) already drive any menu including `menuStatIO`;
   do not touch them.

2. **`top/menu.go`** — three additions, all copying the existing `menuPgss` / `menuDatabases` shape:
   - Add `menuStatIO` to the `menuType` iota block (after `menuConf`).
   - Add a `case menuStatIO` to `selectMenuStyle` returning a `menuStyle` with exactly **2 items**:
     `" pg_stat_io operations"` and `" pg_stat_io timings"`, with a sensible title
     (`" Choose pg_stat_io mode (Enter to choose, Esc to exit): "`).
   - Add a `case menuStatIO` to `menuSelect` mapping `cy 0 → viewSwitchHandler(app.config, "stat_io")`
     and `cy 1 → viewSwitchHandler(app.config, "stat_io_time")`, with `default` → `"stat_io"`, then
     `printCmdline(app.ui, "%s", app.config.view.Msg)` (mirror the `menuPgss` branch).

3. **`top/config_view.go`** — two additions:
   - Add a `statioNextView(current string) string` 2-way toggle helper (copy `databasesNextView`):
     `"stat_io" → "stat_io_time"`, `"stat_io_time" → "stat_io"`, `default → "stat_io"`.
   - Add a `case "statio": viewSwitchHandler(app.config, statioNextView(app.config.view.Name))` to the
     `switch c` in `switchViewTo` (place it among the family cases, before `default`).

4. **`top/help.go`** — extend `helpTemplate`:
   - Add a `j,J` line in the `general actions:` block, in the same style as the `d,D` / `x,X` / `p,P`
     lines: `'j' pg_stat_io switch (operations/timings), 'J' pg_stat_io menu.`
   - Add a short note that `Q` does NOT reset `pg_stat_io` (shared/cluster-wide stats). Keep it
     concise; place it near the existing `Q` description in `other actions:` (e.g. clarify that
     `'Q' reset postgresql statistics counters` excludes shared stats such as pg_stat_io / bgwriter).

5. **`top/menu_test.go`** — add `{menu: menuStatIO, want: 2}` to the `Test_selectMenuStyle`
   testcases table.

## TDD Anchor

Write/extend the menu item-count test BEFORE implementing the `menuStatIO` style, confirm it fails
(`menuStatIO` undefined / wrong count), then implement until green.

- `top/menu_test.go::Test_selectMenuStyle` — extend the testcases table with `{menu: menuStatIO, want: 2}`;
  asserts `selectMenuStyle(menuStatIO)` returns a style with exactly 2 items
  (`pg_stat_io operations`, `pg_stat_io timings`). Mirrors the existing `{menuPgss, 6}` / `{menuDatabases, 2}` rows.

Note: `switchViewTo`, `statioNextView`, and the keybinding wiring have no existing unit-test harness
in `top/` (they require a live gocui UI + view map). Their verification is `make build` + the manual
US walk in Task 04 — do not invent a fake test for them. `statioNextView` is a pure function and may
optionally get a small table-driven test if cheap, but it is not required.

## Acceptance Criteria

- [ ] `j` bound to `switchViewTo(app, "statio")` and `J` bound to `menuOpen(menuStatIO, app.config, "")`
      in `top/keybindings.go` (sysstat view).
- [ ] `menuStatIO` added to the `menuType` iota; `selectMenuStyle(menuStatIO)` returns exactly 2 items
      (`pg_stat_io operations`, `pg_stat_io timings`).
- [ ] `menuSelect` `case menuStatIO` maps `cy` 0/1 to `viewSwitchHandler` `"stat_io"` / `"stat_io_time"`
      (default `"stat_io"`), followed by `printCmdline(... app.config.view.Msg)`.
- [ ] `statioNextView` 2-way toggle added (`stat_io`↔`stat_io_time`, default `stat_io`) and a
      `case "statio"` added to `switchViewTo`.
- [ ] `helpTemplate` has a `j`/`J` line and a note that `Q` does not reset `pg_stat_io` (shared stats).
- [ ] `Test_selectMenuStyle` includes `{menuStatIO, 2}` and passes.
- [ ] `go test ./top/...` passes (modulo the pre-existing `Test_doReload` fixture issue — see Edge cases).
- [ ] `make build` succeeds.
- [ ] No regression in existing menu/keybinding/help behaviour; no rename of existing view names or menu types.

## Context Files

**Feature artifacts:**
- [006-feat-pg-stat-io.md](006-feat-pg-stat-io.md) — user-spec
- [006-feat-pg-stat-io-tech-spec.md](006-feat-pg-stat-io-tech-spec.md) — tech-spec (Decision 7 navigation + naming contract, Decision 9, Decision 10)
- [006-feat-pg-stat-io-code-research.md](006-feat-pg-stat-io-code-research.md) — section 3 (menu pattern + `j`/`J` bindings), section 5 (version gating UX), section 8 item 7 (Q / shared stats)
- [006-feat-pg-stat-io-decisions.md](006-feat-pg-stat-io-decisions.md) — decisions log (append on completion)

**Project knowledge:**
- [overview.md](.claude/skills/project-knowledge/overview.md) — features, supported stats, target audience
- [architecture.md](.claude/skills/project-knowledge/architecture.md) — package layout, data flow, PG version handling
- [patterns.md](.claude/skills/project-knowledge/patterns.md) — code patterns, testing conventions, version branching

**Code files:**
- [top/keybindings.go](top/keybindings.go) — add `j`/`J` bindings (modify)
- [top/menu.go](top/menu.go) — `menuStatIO` iota + `selectMenuStyle` case + `menuSelect` case (modify)
- [top/config_view.go](top/config_view.go) — `statioNextView` helper + `case "statio"` in `switchViewTo` (modify)
- [top/help.go](top/help.go) — `j`/`J` help line + `Q` note (modify)
- [top/menu_test.go](top/menu_test.go) — `{menuStatIO, 2}` testcase (modify)

## Verification Steps

- Run `go test ./top/...` — `Test_selectMenuStyle` passes with the new `{menuStatIO, 2}` row.
  (If `Test_doReload` fails, confirm it is the pre-existing PG-fixture panic — tech-debt [005] — and
  not caused by this task; the menu/help/keybinding tests must be green.)
- Run `make build` — binary builds cleanly (`./bin/pgcenter`), confirming `menuStatIO`, `statioNextView`,
  the `"statio"` case, and the new bindings compile and reference valid identifiers.
- Optional sanity: `grep -n statioNextView top/config_view.go` and `grep -n menuStatIO top/menu.go`
  to confirm all three menu additions are present.

## Details

**Files:**
- `top/keybindings.go` — currently has the lowercase-switch / uppercase-menu pairs at lines 29–45
  (`'d'`→`switchViewTo(app,"databases")` / `'D'`→`menuOpen(menuDatabases,...)`; `'x'`/`'X'`;
  `'p'`/`'P'`). `j` and `J` are verified-free (no existing binding uses them). Add the two new entries
  in the same block, following the `app.config` / `app.config, ""` argument style of the neighbours.
- `top/menu.go` — `menuType` iota at lines 14–25 (`menuNone, menuDatabases, menuPgss, menuProgress,
  menuConf`): add `menuStatIO` after `menuConf`. `selectMenuStyle` switch at lines 35–92: add a
  `case menuStatIO` (the `menuPgss` case at 48–60 is the multi-item template; the `menuDatabases`
  case at 39–47 is the 2-item template — copy the latter's shape). `menuSelect` switch at lines
  134–202: add a `case menuStatIO` with two `cy` branches + default + the trailing
  `printCmdline(app.ui, "%s", app.config.view.Msg)` (mirror `menuPgss` at 145–162). `menuOpen`
  (lines 95–126) needs no change — its `pgssSchema == ""` guard only triggers for `menuPgss`.
- `top/config_view.go` — `databasesNextView` (lines 127–140) is the exact template for
  `statioNextView` (2-way). `switchViewTo` (lines 102–125) holds the `switch c` with family cases
  `"databases"`/`"statements"`/`"progress"` and a `default` that calls `viewSwitchHandler(app.config, c)`;
  add `case "statio"` before `default`. `viewSwitchHandler` (lines 189–193) is the correct low-level
  switch here — pg_stat_io has NO runtime patches (unlike `switchViewToProcPidStat`), so plain
  `viewSwitchHandler` is right; do NOT route through `switchViewToProcPidStat`.
- `top/help.go` — `helpTemplate` const at lines 10–43. The `general actions:` block (lines 12–24)
  has the `d,D` / `x,X` / `p,P` lines (16–18) — add the `j,J` line in the same column-aligned style.
  The `Q` description is in `other actions:` (line 38: `',' show system tables on/off, 'Q' reset
  postgresql statistics counters.`) — add the shared-stats caveat there (keep ASCII alignment of the
  template intact).
- `top/menu_test.go` — `Test_selectMenuStyle` table at lines 9–18: add `{menu: menuStatIO, want: 2}`.

**Dependencies:**
- Task 01 (broader feature: `PostgresV16` constant + query layer) — runtime data depends on it.
- Task 02 registers the `stat_io` / `stat_io_time` views; this task switches to those names. File-wise
  independent — these tasks can run in parallel within Wave 2; build/tests here do not import Task 02 code.

**Edge cases:**
- `j` pressed from an unrelated view → `statioNextView` `default` returns `"stat_io"` (enters count
  screen). Pressing `j` again toggles to `"stat_io_time"`, then back. This satisfies "j enters and toggles".
- On PG14/15 there is NO UI-level version guard at switch time (verified — `switchViewTo` does not
  check `VersionOK`). Pressing `j`/`J` switches the view; the collector's `VersionOK` gate
  (`MinRequiredVersion`, set in Task 02) returns the standard "not supported" error into the main pane.
  This task adds NO version check — do not add one (Decision 7 / code-research §5).
- `Q` reset: `top/reset.go` deliberately does NOT reset shared stats (bgwriter/archiver/io). The help
  note documents this so a non-resetting `stats_age` is not read as a bug. No code change to `reset.go`.
- Pre-existing tech-debt [005]: `Test_doReload` may panic locally when PG fixtures are absent. It is
  unrelated to this task — verify the menu/help tests are green and rely on `make build` + CI.

**Implementation hints:**
- Keep `printCmdline(app.ui, "%s", app.config.view.Msg)` after the `menuStatIO` selection so the
  time-view `Msg` (the `track_io_timing` hint from Decision 9, set in Task 02) shows on switch.
- Maintain the ASCII alignment of `helpTemplate` — it is a raw string literal rendered verbatim; a
  misaligned line is a visible defect.
- Do not add a menu/title for a non-existent third mode — exactly 2 items.

## Reviewers

- **dev-code-reviewer** → `006-feat-pg-stat-io-task-03-dev-code-reviewer-review.json`
- **dev-test-reviewer** → `006-feat-pg-stat-io-task-03-dev-test-reviewer-review.json`

## Post-completion

- [ ] Записать краткий отчёт в [006-feat-pg-stat-io-decisions.md](006-feat-pg-stat-io-decisions.md) (Summary: 1-3 предложения, ревью со ссылками на JSON, без таблиц файндингов и дампов)
- [ ] Если отклонились от спека — описать отклонение и причину
- [ ] Обновить user-spec/tech-spec если что-то изменилось
