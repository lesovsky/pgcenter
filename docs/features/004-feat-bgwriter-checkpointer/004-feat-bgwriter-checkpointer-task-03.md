---
status: done                       # planned -> in_progress -> done
depends_on: ["01"]                 # ID задач-зависимостей (строки: ["01", "02"])
wave: 2                            # волна параллельного выполнения
skills: [code-writing]             # МАССИВ скиллов для загрузки
verify: user                       # инструмент верификации (опционально: curl, bash, user)
reviewers: [dev-code-reviewer, dev-test-reviewer]  # явно указать. Пусто = fallback на defaults
teammate_name:                     # имя агента-исполнителя (опционально; если не задано — генерируется по описанию задачи)
---

# Task 03: Register view + TUI wiring

## Required Skills

Перед выполнением задачи загрузи:
- `/skill:code-writing` — [skills/code-writing/SKILL.md](~/.claude/skills/code-writing/SKILL.md)

## Description

Wire the new single-row `bgwriter` screen (which combines `pg_stat_bgwriter` + `pg_stat_checkpointer`)
into the `pgcenter top` TUI. Task 01 already provides the query layer
(`internal/query/bgwriter.go`: the `PgStatBgwriter*` constants and the
`SelectStatBgwriterQuery(version) (string, int, [2]int)` selector). This task does the wiring so a
user can press `b` and actually see the screen.

There are four edits, all mirroring the existing `pg_stat_wal` screen:
1. Register a `"bgwriter"` entry in the views map (`internal/view/view.go`) with `NotRecordable: true`.
2. Add a `case "bgwriter"` in `Configure()` calling `query.SelectStatBgwriterQuery(opts.Version)` so
   the version-specific `QueryTmpl/Ncols/DiffIntvl` are applied at connection time.
3. Bind hotkey `b` in `top/keybindings.go` to switch to the view.
4. Add `b` to the mode-key help row in `top/help.go`.

Plus a documentation-hygiene edit: refresh the now-stale `NotRecordable` example comment in
`record/record.go`. The comment currently cites `procpidstat` as the example user of the flag, but
`procpidstat` no longer sets `NotRecordable` (recording support was added in feature 003 —
see commit `83f95cd`). The `bgwriter` view becomes the **only / first** live user of the flag, so the
comment should reference it instead.

The data path is pure SQL — no enrichment, no `CollectExtra`. The existing collector → `diff()` →
render pipeline handles the new view unchanged; no other files need touching. In particular
`top/reset.go` needs NO change (shared bgwriter/checkpointer counters are already excluded from the
`Q` reset), and there is no report-side wiring (`NotRecordable: true` keeps the view out of recording
and structurally out of the report path).

## What to do

1. **`internal/view/view.go` — add the `"bgwriter"` view entry.** Mirror the `"wal"` entry
   (`view.go:128-139`). Place it adjacent to `"wal"` for readability. Set:
   - `Name: "bgwriter"`
   - `MinRequiredVersion: query.PostgresV14`
   - `QueryTmpl: query.PgStatBgwriterPG14` (PG14 default; overridden per version in `Configure()`)
   - `DiffIntvl: [2]int{3, 10}` (PG14 default)
   - `Ncols: 12` (PG14 default)
   - `OrderKey: 0`, `OrderDesc: true`
   - `ColsWidth: map[int]int{}`
   - `Msg: "Show bgwriter / checkpointer statistics"`
   - `Filters: map[int]*regexp.Regexp{}`
   - `NotRecordable: true`

   > Use the exact `QueryTmpl`/`Ncols`/`DiffIntvl` PG14-branch values produced by Task 01's
   > `SelectStatBgwriterQuery`. If Task 01 named the PG14 constant differently than `PgStatBgwriterPG14`,
   > use the actual name — verify by reading `internal/query/bgwriter.go`.

2. **`internal/view/view.go` — add the `Configure()` case.** In the first `switch k` loop
   (`view.go:307-325`), after the `case "wal"`, add:
   ```go
   case "bgwriter":
       view.QueryTmpl, view.Ncols, view.DiffIntvl = query.SelectStatBgwriterQuery(opts.Version)
       v[k] = view
   ```
   The second loop (`view.go:328-335`) calls `query.Format()` for all views — no change there.

3. **`top/keybindings.go` — bind hotkey `b`.** After the `wal` line (`keybindings.go:35`), add:
   ```go
   {"sysstat", 'b', switchViewTo(app, "bgwriter")},
   ```
   `b` (lowercase) is confirmed free; `B` (uppercase, diskstats) is unrelated and stays untouched.

4. **`top/help.go` — add `b` to the mode-key help row.** The general-actions row
   (`help.go:13`) currently reads `a,f,r,w` with per-key descriptions. Add `b` so the row reads
   `a,b,f,r,w` (alphabetical) and include a short description for `b` (bgwriter/checkpointer).
   Keep the surrounding lines and column alignment consistent with the existing block.

5. **`record/record.go` — refresh the stale `NotRecordable` example comment** (`record.go:205-207`).
   Replace the `procpidstat`-based example (procpidstat no longer sets the flag) with a `bgwriter`-based
   one: e.g. that the bgwriter/checkpointer screen is TUI-only and intentionally not recorded. Keep it a
   comment-only edit — do not change any logic in `filterViews()`.

## TDD Anchor

Wiring is config/map/keybinding/help/comment changes; there is no new behavioral unit under test that
isn't already covered by Task 01's selector tests and the existing view/diff suite. Manual TUI
verification (the `verify: user` step) is the acceptance gate for this task. Before relying on `make
build`, run the existing package tests to confirm no regression:

- `go build ./...` — compiles after the view/keybinding/help/comment edits.
- `go test ./internal/view/... ./top/...` — existing view and top tests still pass (no regression
  from the new map entry / `Configure()` case / keybinding).

## Acceptance Criteria

- [ ] `"bgwriter"` view entry exists in `internal/view/view.go` with `NotRecordable: true`,
      `MinRequiredVersion: query.PostgresV14`, `Msg: "Show bgwriter / checkpointer statistics"`, and
      PG14-default `QueryTmpl`/`Ncols`/`DiffIntvl`.
- [ ] `case "bgwriter"` in `Configure()` calls `query.SelectStatBgwriterQuery(opts.Version)` and
      reassigns `QueryTmpl/Ncols/DiffIntvl`.
- [ ] `{"sysstat", 'b', switchViewTo(app, "bgwriter")}` keybinding registered after the `wal` line.
- [ ] `top/help.go` mode-key row lists `b` (sorted `a,b,f,r,w`) with a description.
- [ ] `record/record.go` `NotRecordable` example comment refers to bgwriter, not procpidstat.
- [ ] No change to `top/reset.go`, `cmd/report/report.go`, or `filterViews()` logic.
- [ ] `make build` and `make lint` pass; `make test` shows no regressions.
- [ ] Manual: pressing `b` on PG17/PG18 opens the screen with correct columns (event counters
      absolute, work/time/buffer columns delta), and `b` appears in the `?` help.

## Context Files

**Feature artifacts:**
- [004-feat-bgwriter-checkpointer.md](004-feat-bgwriter-checkpointer.md) — user-spec
- [004-feat-bgwriter-checkpointer-tech-spec.md](004-feat-bgwriter-checkpointer-tech-spec.md) — tech-spec (Task 3, Architecture "How it works", Decision 4 stats_age, Decision 5 reviewers)
- [004-feat-bgwriter-checkpointer-code-research.md](004-feat-bgwriter-checkpointer-code-research.md) — code research (§2 view.go, §3 keybindings/help, §4 reset, §5 record.go, §8 integration points)
- [004-feat-bgwriter-checkpointer-decisions.md](004-feat-bgwriter-checkpointer-decisions.md) — decisions log

**Project knowledge:**
- [project.md](.claude/skills/project-knowledge/overview.md) — project overview (features, supported stats)
- [architecture.md](.claude/skills/project-knowledge/architecture.md) — package layout, data flow, PG version handling
- [patterns.md](.claude/skills/project-knowledge/patterns.md) — code patterns, version branching, testing conventions

**Code files:**
- [internal/view/view.go](../../../internal/view/view.go) — add `"bgwriter"` map entry (~line 128, mirror `"wal"`) + `case "bgwriter"` in `Configure()` (~line 321, after `case "wal"`)
- [top/keybindings.go](../../../top/keybindings.go) — add `b` keybinding after line 35 (`wal` line)
- [top/help.go](../../../top/help.go) — add `b` to mode-key help row (line 13)
- [record/record.go](../../../record/record.go) — refresh stale `NotRecordable` example comment (lines 205-207)
- [internal/query/bgwriter.go](../../../internal/query/bgwriter.go) — READ: Task 01 output; confirm exact const name + selector signature
- [top/config_view.go](../../../top/config_view.go) — READ: `switchViewTo` / `viewSwitchHandler` (bgwriter falls into the `default` branch, like `wal`)

## Verification Steps

- `make build` — binary compiles with the new view/keybinding/help/comment edits.
- `make lint` — no new golangci-lint / gosec findings.
- `make test` — existing tests pass; no regression from the new map entry / `Configure()` case.
- Manual (`verify: user`): run `./bin/pgcenter top` against a live PG17 and PG18 cluster, press `b`:
  - The screen opens (title / columns present).
  - Event counters (`ckpt_timed`, `ckpt_req`, and on PG17+ `rstpt_*`) render as absolute cumulative
    values (they do not flicker to 0 between refreshes).
  - Work/time/buffer columns (`ckpt_write,ms`, `ckpt_sync,ms`, `buf_*`, `maxwritten`) render as
    per-interval deltas.
  - `stats_age` renders as pass-through text.
  - Press `?` (or `h`): the help shows `b` in the general-actions mode row.

## Details

**Files:**
- `internal/view/view.go` — currently has the `"wal"` entry at lines 128-139 and the `Configure()`
  `switch k` loop at lines 307-325 (with `case "wal"` at 321-323). Add the parallel `"bgwriter"` map
  entry and `case "bgwriter"`. `NotRecordable bool` is the field at `view.go:30`. No other view
  currently sets `NotRecordable: true`; bgwriter will be the sole user.
- `top/keybindings.go` — `"sysstat"` switch hotkeys are one-liners at lines 29-38; `wal` at line 35.
  Add the `b` line directly after it.
- `top/help.go` — `helpTemplate` const; general-actions block starts at line 13 (`a,f,r,w     mode: ...`).
  Insert `b` in the key list and add a short `'b' bgwriter/checkpointer` description; preserve the
  multi-line alignment of the block.
- `record/record.go` — `filterViews()` at lines 200-218; the `NotRecordable` skip + stale comment at
  lines 205-208. Comment-only change. Do NOT alter the `delete(views, k)` logic.

**Dependencies:**
- Task 01 (`internal/query/bgwriter.go`) MUST be complete — `SelectStatBgwriterQuery` and the
  `PgStatBgwriter*` query constants must exist. Read that file first to get the exact PG14 const name
  and confirm the selector returns `(string, int, [2]int)`.

**Edge cases:**
- `MinRequiredVersion: query.PostgresV14` means the view is available on all pgcenter-supported
  versions (PG14+); `VersionOK` gates it (`view.go:341-343`).
- Hotkey collision: lowercase `b` is free; uppercase `B` (diskstats, line 47) is a distinct gocui key
  and must not be touched.
- `Configure()` overrides the static map `QueryTmpl/Ncols/DiffIntvl` at runtime — the literal map
  values are only the PG14 defaults, same convention as `wal`.

**Implementation hints:**
- Mirror `wal` exactly for the map entry and the `Configure()` case — the structure is identical
  (single-row, version-aware, diffed range + excluded `stats_age`).
- `switchViewTo(app, "bgwriter")` needs no special-casing: it falls into the `default` branch of
  `switchViewTo` (`config_view.go`), like `wal`/`activity`. No `*NextView` helper required.
- Keep the help row alphabetically sorted (`a,b,f,r,w`) to match the existing convention.
- No security reviewer on this task (Decision 5): static map/keybinding/help/comment wiring with no
  input surface, no injection or auth concern.

## Reviewers

- **dev-code-reviewer** → `004-feat-bgwriter-checkpointer-task-03-dev-code-reviewer-review.json`
- **dev-test-reviewer** → `004-feat-bgwriter-checkpointer-task-03-dev-test-reviewer-review.json`

## Post-completion

- [ ] Записать краткий отчёт в [004-feat-bgwriter-checkpointer-decisions.md](004-feat-bgwriter-checkpointer-decisions.md) (Summary: 1-3 предложения, ревью со ссылками на JSON, без таблиц файндингов и дампов)
- [ ] Если отклонились от спека — описать отклонение и причину
- [ ] Обновить user-spec/tech-spec если что-то изменилось
