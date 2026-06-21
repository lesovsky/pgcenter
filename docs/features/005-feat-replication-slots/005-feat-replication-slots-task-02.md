---
status: planned                    # planned -> in_progress -> done
depends_on: ["01"]                 # ID задач-зависимостей (строки: ["01", "02"])
wave: 2                            # волна параллельного выполнения
skills: [code-writing]             # МАССИВ скиллов для загрузки
verify: bash + user               # инструмент верификации (опционально: curl, bash, user)
reviewers: [dev-code-reviewer]     # явно указать. Пусто = fallback на defaults
teammate_name:                     # имя агента-исполнителя (опционально; если не задано — генерируется по описанию задачи)
---

# Task 02: Wire the replslots view into the TUI

## Required Skills

Перед выполнением задачи загрузи:
- `/skill:code-writing` — [skills/code-writing/SKILL.md](~/.claude/skills/code-writing/SKILL.md)

## Description

Feature 005 adds a new multi-row TUI screen `replslots` (hotkey `o`) to `pgcenter top`,
showing one row per PostgreSQL replication slot for disk-fill triage.

Task 01 (the dependency) creates the query layer: the `query.PgStatReplicationSlots` template
constant and the version-aware selector `query.SelectStatReplicationSlotsQuery(_ int) (string, int, [2]int)`.

This task wires that query into the TUI so the user can actually open the screen. Three things:
1. Register the `replslots` view in `internal/view/view.go` `New()` (the static view map) and
   add a `case "replslots":` in `Configure()` that calls the selector — mirroring the existing
   `bgwriter` entry shipped in feature 004.
2. Bind hotkey `o` in `top/keybindings.go` to switch to the `replslots` view. It is a direct
   view (no NextView family / `config_view.go` case is needed — see notes below).
3. Add `o` to the help mode line in `top/help.go` so `?`/`h` documents the new hotkey.

After this task the screen is reachable and renders; recording stays disabled (`NotRecordable`).

## What to do

1. In `internal/view/view.go`, inside `New()`, add a `"replslots"` entry to the returned map,
   placed next to the `"bgwriter"` entry and modeled on it. Field values (from tech-spec
   Decisions 1/3 and the Data Models section):
   - `Name: "replslots"`
   - `MinRequiredVersion: query.PostgresV14`
   - `QueryTmpl: query.PgStatReplicationSlots`
   - `DiffIntvl: [2]int{6, 13}`
   - `Ncols: 15`
   - `OrderKey: 4` — non-default; sort by `retained,KiB` so the greediest slot is on top
     (Decision 3 — intentional deviation from the col-0 default of every other multi-row view)
   - `OrderDesc: true`
   - `UniqueKey: 0` — default (slot_name); omit the field, matching bgwriter
   - `ColsWidth: map[int]int{}`
   - `Filters: map[int]*regexp.Regexp{}`
   - `NotRecordable: true`
   - `Msg: "Show replication slots statistics"`
2. In `internal/view/view.go` `Configure()`, add `case "replslots":` to the version switch,
   immediately after the `case "bgwriter":`, mirroring it:
   `view.QueryTmpl, view.Ncols, view.DiffIntvl = query.SelectStatReplicationSlotsQuery(opts.Version)`
   then `v[k] = view`.
3. In `top/keybindings.go`, add a keybinding row to the `keys` slice:
   `{"sysstat", 'o', switchViewTo(app, "replslots")}` — place it next to the other direct view
   bindings (e.g. after the `'b'` bgwriter binding). No change to `top/config_view.go` is needed:
   `replslots` is a direct view, so `switchViewTo`'s `default:` branch handles it via
   `viewSwitchHandler(app.config, "replslots")`.
4. In `top/help.go`, add `'o'` to the help `general actions:` block. Extend the existing
   `a,b,f,r,w` mode line (or its continuation) with an entry like `'o' replication slots`, keeping
   the existing alignment/format of the `helpTemplate` string.
5. Update the view-count assertion and add a guard test (see TDD Anchor) so registration is pinned.

## TDD Anchor

Tests to write BEFORE implementation (write → run → see fail → implement → see pass):

- `internal/view/view_test.go::TestNew` — bump the expected view count from `23` to `24`
  (adding `replslots` adds one entry to `New()`'s map). This currently passes at `23` and must
  fail after you write the new expectation, then pass once the entry is added.
- `internal/view/view_test.go::TestNew_ReplslotsView` — new test mirroring the existing
  `TestNew_BgwriterView`: assert the `"replslots"` key is present; `NotRecordable == true`;
  `MinRequiredVersion == query.PostgresV14`; `Ncols == 15`; `DiffIntvl == [2]int{6, 13}`;
  `OrderKey == 4`; `OrderDesc == true`; `Msg == "Show replication slots statistics"`.

Note: the keybinding (`top/keybindings.go`) and help line (`top/help.go`) are gocui-rendered and
not unit-tested in this codebase (there is no `top/help_test.go` and the help template is a plain
string) — they are covered by the `user` verification step (open the screen, check `?`).

## Acceptance Criteria

- [ ] `replslots` view registered in `view.New()` with the exact field values above.
- [ ] `Configure()` has a `case "replslots":` calling `query.SelectStatReplicationSlotsQuery(opts.Version)`.
- [ ] Hotkey `o` bound in `top/keybindings.go` (`{"sysstat", 'o', switchViewTo(app, "replslots")}`).
- [ ] `o` documented in the `top/help.go` help mode line.
- [ ] `TestNew` asserts 24 views; `TestNew_ReplslotsView` passes.
- [ ] `make build` succeeds; user presses `o` and the screen renders; `o` appears in help (`?`).
- [ ] No regressions: `make test`, `make lint`, `make build` clean.

## Context Files

**Feature artifacts:**
- [005-feat-replication-slots.md](005-feat-replication-slots.md) — user-spec
- [005-feat-replication-slots-tech-spec.md](005-feat-replication-slots-tech-spec.md) — tech-spec (Task 2; Decisions 1/3/6; Data Models)
- [005-feat-replication-slots-decisions.md](005-feat-replication-slots-decisions.md) — decisions log

**Project knowledge:**
- [overview.md](.claude/skills/project-knowledge/overview.md) — features, supported stats
- [architecture.md](.claude/skills/project-knowledge/architecture.md) — package layout, data flow, view-registration & version-handling pattern
- [patterns.md](.claude/skills/project-knowledge/patterns.md) — code/testing conventions, version branching

**Code files:**
- [internal/view/view.go](../../../internal/view/view.go) — MODIFY: add `replslots` entry in `New()` (~lines 140–152, after `bgwriter`) and `case "replslots":` in `Configure()` (~lines 337–339)
- [internal/view/view_test.go](../../../internal/view/view_test.go) — MODIFY: bump `TestNew` count, add `TestNew_ReplslotsView`
- [top/keybindings.go](../../../top/keybindings.go) — MODIFY: add `o` binding to `keys` slice (~line 36)
- [top/help.go](../../../top/help.go) — MODIFY: add `o` to `helpTemplate` general actions block (~lines 13–14)
- [top/config_view.go](../../../top/config_view.go) — READ only: confirm `switchViewTo` `default:` branch handles direct views (no edit needed)

## Verification Steps

- `make build` — compiles cleanly with the new view/keybinding/help.
- `make test` — `TestNew` (now 24) and `TestNew_ReplslotsView` green; full suite green.
- `make lint` — clean (no unused-var / formatting issues).
- User (manual TUI check): run `./bin/pgcenter top` against a PG 14+ instance, press `o` —
  the replication-slots screen renders (header + rows, or empty header if no slots). Press `?`
  (or `h`) and confirm `o` is listed in the general actions block. Press `b` then `o` to confirm
  view switching back and forth works without panic.

## Details

**Files:**
- `internal/view/view.go` — currently has 23 views in `New()` (asserted by `TestNew`). The
  `bgwriter` entry (lines ~140–152) is the exact template to copy: it is the only other
  `NotRecordable` PG14+ view. Copy its shape, change `Name`, `QueryTmpl`, `DiffIntvl`, `Ncols`,
  `OrderKey` (4, not 0), `Msg`. In `Configure()` the `bgwriter` case (lines ~337–339) is the
  template for the new `case "replslots":` — same three-assignment + `v[k] = view` shape.
- `internal/view/view_test.go` — `TestNew` asserts `len(v) == 23`; `TestNew_BgwriterView`
  (lines ~16–26) is the template for `TestNew_ReplslotsView`.
- `top/keybindings.go` — the `keys` slice (lines ~18–74) lists direct view bindings like
  `{"sysstat", 'b', switchViewTo(app, "bgwriter")}` (line 36). Add the `o` binding alongside.
  The hotkey `o` is currently free (verified — not present in the slice).
- `top/help.go` — `helpTemplate` (lines ~10–42); the `general actions:` block (lines ~13–18)
  documents mode hotkeys. The `a,b,f,r,w` line lists letter modes. Add `o` for replication slots,
  preserving column alignment of the template literal.

**Dependencies:**
- Task 01 must be done first: `query.PgStatReplicationSlots` (template const) and
  `query.SelectStatReplicationSlotsQuery` (selector) must exist, else this task will not compile.
  Both are currently absent (verified) — they are Task 01's deliverable.

**Edge cases:**
- A PG instance with zero replication slots renders an empty screen (header only) — expected,
  not an error. Confirm no panic on the empty result.
- Physical slots show `0` for the eight diffed counters and empty `safe,KiB`/`stats_age`
  (handled by the query in Task 01) — out of scope here but good to eyeball during the manual check.
- Servers below PG 14: `MinRequiredVersion: query.PostgresV14` gates the view; pressing `o` on an
  older server follows the same path as other PG14+ views (e.g. `bgwriter`/`wal`) — no special
  handling added here.

**Implementation hints:**
- Mirror `bgwriter` precisely; the only semantic difference is `OrderKey: 4` (Decision 3) — keep
  the inline rationale brief if you add a comment, do not over-document.
- `UniqueKey: 0` is the struct zero value — omit the field (bgwriter omits it too).
- Direct view ⇒ no `config_view.go` `*NextView` function and no `switchViewTo` `case` needed; the
  `default:` branch (config_view.go line ~118) already routes it.
- Run the `view` package test first (`go test ./internal/view/...`) for the fast TDD loop before
  `make test`.

## Reviewers

- **dev-code-reviewer** → `005-feat-replication-slots-task-02-dev-code-reviewer-review.json`

## Post-completion

- [ ] Записать краткий отчёт в [005-feat-replication-slots-decisions.md](005-feat-replication-slots-decisions.md) (Summary: 1-3 предложения, ревью со ссылками на JSON, без таблиц файндингов и дампов)
- [ ] Если отклонились от спека — описать отклонение и причину
- [ ] Обновить user-spec/tech-spec если что-то изменилось
