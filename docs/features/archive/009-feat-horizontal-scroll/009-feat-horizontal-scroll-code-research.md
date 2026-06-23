# Code Research — 009-feat-horizontal-scroll

Feature: horizontal scrolling of the statistics table in `pgcenter top` by columns, with the
first column frozen. Hotkeys `[` (scroll left) / `]` (scroll right). Manual visible-column
window built in `printStatHeader`/`printStatData`, NOT via gocui `origin.x`. Scope: `dbstat` view only.

Issue: #14 (open since 2015). Feature size: M.

---

## 1. Entry Points

### `top/stat.go` — rendering pipeline (the main change site)
The render path is: `printStat()` → `app.ui.Update(...)` → `printDbstat()` → `printStatHeader()` + `printStatData()`.

- **`printStat(app *app, s stat.Stat, props stat.PostgresProperties)`** — `top/stat.go:122`.
  Wraps everything in `app.ui.Update(func(g *gocui.Gui) error {...})`. For `dbstat`: gets the view
  (`g.View("dbstat")` at `:144`), calls `v.Clear()` (`:148`), then `printDbstat(v, app.config, s)`
  (`:150`). Note it passes `app.config` (so the handler has access to the whole config, not just the view).

- **`printDbstat(v *gocui.View, config *config, s stat.Stat)`** — `top/stat.go:320`.
  On `s.Error != nil` prints the error and returns (`:322-329`). Otherwise:
  `alignViewToResult(config, s.Result)` (`:332`) → `printStatHeader(v, s, config)` (`:335`) →
  `printStatData(v, s, config, isFilterRequired(config.view.Filters))` (`:341`).

- **`printStatHeader(v *gocui.View, s stat.Stat, config *config) error`** — `top/stat.go:364`.
  Current full body:
  ```go
  func printStatHeader(v *gocui.View, s stat.Stat, config *config) error {
      var pname string
      for i := 0; i < s.Result.Ncols; i++ {
          name := s.Result.Cols[i]
          // mark filtered column
          if config.view.Filters[i] != nil && config.view.Filters[i].String() != "" {
              pname = "*" + name
          } else {
              pname = name
          }
          // mark ordered column with foreground color
          if i != config.view.OrderKey {
              fmt.Fprintf(v, "\033[%d;%dm%-*s\033[0m", 30, 47, config.view.ColsWidth[i]+2, pname)
          } else {
              fmt.Fprintf(v, "\033[%d;%dm%-*s\033[0m", 47, 1, config.view.ColsWidth[i]+2, pname)
          }
      }
      fmt.Fprintf(v, "\n")
      return nil
  }
  ```
  Iterates **all** columns `0..Ncols-1`. The sort column (`OrderKey`) gets escape `\033[47;1m` (bold);
  every other column gets `\033[30;47m` (black-on-white). Filtered columns are prefixed `*`. Each cell is
  padded to `ColsWidth[i]+2` via `%-*s`. The whole header is one line terminated by `\n`.
  **This is the primary insertion point for the visible-window loop, `‹`/`›` markers, and bold-name frozen-column highlight.**

- **`printStatData(v *gocui.View, s stat.Stat, config *config, filter bool) error`** — `top/stat.go:398`.
  Outer loop over rows (`rownum < s.Result.Nrows`, `:400`). Per row: filter check (`:405-415`, ANDs all
  set filters, OR-breaks on first match), then inner loop `for i := range s.Result.Cols` (`:418`) that:
  truncates values longer than `ColsWidth[i]` to `value[:width-1]+"~"` (`:421-430`), returns error
  `"zero or negative width, skip"` when `width <= 0` (`:424-426`), prints `fmt.Fprintf(v, "%-*s", ColsWidth[i]+2, value)` (`:433`), increments a separate `colnum`. Terminates the row with `\n` (`:441`).
  **Note the dual counter `colnum`/`i`**: `i` ranges over `Cols`, `colnum` is bumped only when `doPrint`.
  Both are reset per row in the loop post-statement (`:400`). **Second primary insertion point for the visible window.**

### `top/keybindings.go` — hotkey registration
- **`keybindings(app *app) error`** — `top/keybindings.go:17`. Flat `[]key` slice; each `{viewname, key, handler}`
  registered via `app.ui.SetKeybinding(k.viewname, k.key, gocui.ModNone, k.handler)` (`:82`).
  Navigation hotkeys live on view `"sysstat"`. Existing nav: `KeyArrowLeft`→`orderKeyLeft` (`:22`),
  `KeyArrowRight`→`orderKeyRight` (`:23`), `KeyArrowUp`→`increaseWidth` (`:24`), `KeyArrowDown`→`decreaseWidth` (`:25`),
  `'<'`→`switchSortOrder` (`:26`), `'/'`→`dialogOpen(dialogFilter)` (`:57`).
  **`[` and `]` are FREE** — confirmed by full scan of the `keys` slice (`:18-77`) and `grep` for
  `'['`/`']'`/`0x5B`/`0x5D` across `top/*.go` (no matches). New bindings go on view `"sysstat"`,
  `gocui.ModNone`, with `'['` and `']'` as plain rune keys.

### `top/config_view.go` — handlers + view switching
- New scroll handlers belong here, alongside `orderKeyLeft`/`orderKeyRight` (`:21`/`:34`), which are the
  closest structural precedent (mutate `config.view`, clamp at boundary, then `config.viewCh <- config.view`).
- **`switchViewTo(app, c)`** — `:102`; **`viewSwitchHandler(config, c)`** — `:208` (see §2 for the offset-reset point).

### `top/help.go` — help screen
- **`helpTemplate`** const — `top/help.go:10`. The line `Left,Right,<,/` (`:21`) documents column nav.
  `[`/`]` must be added to the template. No `]`/`[` mention currently.

---

## 2. Data Layer (state: where to store the scroll offset)

No DB/schema changes — pure UI feature, operates on the already-collected `stat.PGresult` and `ColsWidth`.

### `internal/view/view.go` — `View` struct (`:10-31`)
Fields relevant to scroll: `Cols []string`, `Ncols int` (`:17`, right border for `OrderKey`),
`OrderKey int`, `OrderDesc bool`, `ColsWidth map[int]int` (`:21`), `Aligned bool` (`:22`),
`Filters map[int]*regexp.Regexp` (`:24`). **No existing field for column hiding or scroll offset.**

There is precedent for adding per-view UI/runtime state directly to `View`: ADR [001] added
`CollectExtra int`, `IOAvailable`, `DelayAcctAvailable`; ADR [001]/[003] added/removed `NotRecordable`.
A scroll offset field (e.g. `ColsScrollOffset int`) would follow that precedent and be per-view.

### Two candidate storage locations
1. **On `view.View`** (e.g. `ColsScrollOffset int`). Pros: per-view, flows through the existing
   `config.viewCh` transport like `OrderKey`; handler pattern identical to `orderKeyLeft`. Cons: see the
   **view-switch reload caveat** below — `viewSwitchHandler` reloads the view from the static `views` map,
   which has zero-value offset, so offset is *reset by construction* on switch (this is actually the desired
   behavior per acceptance criteria, but means the "saved" view in the map carries the offset only if it is
   written back; see `top/config_view.go:209` `config.views[config.view.Name] = config.view`).
2. **On `config`** (`top/config.go:10`, a separate `colsScrollOffset int`). Pros: clearly view-orthogonal;
   easy explicit reset in `switchViewTo`. Cons: not carried over `viewCh` — the print loop reads
   `app.config` (it gets the whole `*config`, see `printStat` → `printDbstat(v, app.config, s)` at
   `top/stat.go:150`), so this works for rendering, but the stats goroutine (`collectStat`) which receives
   only `view.View` over `viewCh` cannot see it (it does not need to — offset is render-only).

   **(tech-spec decides; both are viable. The render path has access to the full `*config`.)**

### `config` struct — `top/config.go:10`
```go
type config struct {
    view         view.View      // Current active view.
    views        view.Views     // List of all available views.
    queryOptions query.Options
    viewCh       chan view.View // Channel: config → stats goroutine.
    logtail      stat.Logfile
    dialog       dialogType
    menu         menuStyle
    procMask     int
}
```
`config.view` is the single source of truth for the **current** view. `config.views` holds all view
definitions (a `map[string]View`, `view.Views`). The render loop reads `config.view` (via `app.config`);
handlers mutate `config.view` and push it on `config.viewCh`.

### How the offset survives auto-refresh vs. view switch
- **Auto-refresh:** `collectStat()` (`top/stat.go:21-119`) re-runs `c.Update()` on a ticker and sends a
  fresh `stat.Stat` over `statCh`. It does **not** touch `config.view` between refreshes. The render loop
  reads `config.view`/`config` each tick, so any offset stored there persists across refreshes for free —
  satisfies "offset survives auto-refresh" with no extra work.
- **View switch:** `viewSwitchHandler(config, c)` — `top/config_view.go:208`:
  ```go
  func viewSwitchHandler(config *config, c string) {
      config.views[config.view.Name] = config.view  // save current view back to map
      config.view = config.views[c]                 // load target view from static map
      config.viewCh <- config.view
  }
  ```
  Line `:209` writes the **current** view (with its mutated state) back into `config.views`; line `:210`
  overwrites `config.view` with the target from the map. **The reset-to-0 point for the offset is here**:
  if offset lives on `view.View`, the target loaded at `:210` carries whatever offset was last saved for
  that view in the map — to force reset-on-switch you must either zero it at load, or never write it back at
  `:209`. If offset lives on `config`, reset it explicitly in `switchViewTo`/`viewSwitchHandler`.
  Note `switchViewToProcPidStat` (`:220`) deliberately bypasses `viewSwitchHandler` and patches the view
  manually (`:243-249`) — a second switch path to keep consistent if offset lives on `view.View`.

---

## 3. Similar Features

- **Column sort nav (`orderKeyLeft`/`orderKeyRight`)** — `top/config_view.go:21`/`:34`. The exact handler
  shape to mirror: read `config.view.OrderKey`, inc/dec, clamp against `0`/`Ncols-1` (wrap-around there;
  scroll should *clamp*, not wrap), then `config.viewCh <- config.view`. Tests: `Test_orderKeyLeft`/`Right`
  in `top/config_view_test.go:11`/`:43`.
- **Width nav (`increaseWidth`/`decreaseWidth`)** — `top/config_view.go:47`/`:60`. Mutate `ColsWidth[OrderKey]`,
  clamp with `math.Min`/`math.Max` against `colsWidthMax`=256 / `len(Cols[idx])`. Demonstrates per-column
  width manipulation and the `math` helper package usage.
- **ADR [006] pg_stat_io split** — `docs/decisions-log.md:349`. The whole `stat_io`/`stat_io_time` split
  (and the 7-way `pg_stat_statements` split) exists *because* there is no horizontal scroll today. ADR [006]
  also documents that **column hiding is impossible** without new code: `internal/align.SetAlign` floors
  every column at width 8 and `ColsWidth` is a runtime cache, not a preset. This feature implements the
  scroll those splits were a workaround for, but per interview scope it does **not** un-split them.

---

## 4. Integration Points

- **Render input:** `printStatHeader`/`printStatData` consume `s.Result` (a `stat.PGresult`: `.Cols []string`,
  `.Ncols int`, `.Nrows int`, `.Values [][]sql.NullString`) and `config.view.ColsWidth map[int]int`. The
  visible-window logic intersects these with the offset + frozen-first-column rule.
- **Width source:** `alignViewToResult(config, r)` — `top/stat.go:309` — guarantees `len(ColsWidth) == Ncols`
  before render (issue #99 fix). The scroll logic can rely on `ColsWidth[i]` being present and `> 0` for all
  `i in [0, Ncols)` after this call.
- **View transport:** `config.viewCh chan view.View` (`top/config.go:14`) — handlers push the mutated view;
  `collectStat()` receives it (`top/stat.go:77`). Offset only matters to the render path (which reads
  `config`/`config.view` directly), not to the collector — but if the offset is a `view.View` field it will
  ride this channel harmlessly.
- **Terminal width:** `gocui.View.Size()` gives the printable width. `dbstat` is created at
  `top/ui.go:155`: `SetView("dbstat", -1, 4, maxX, maxY-1)` with `v.Frame = false` and **no `Wrap`/`Autoscroll`**
  set (both default false). So `dbstat` width ≈ `maxX+1` columns; lines longer than that are clipped (see §5).
  `readLogfileRecent` (`top/stat.go:534`) shows the `v.Size()` idiom for reading view dimensions if the
  window calc needs the live width.
- **Filters:** `config.view.Filters` and `isFilterRequired` (`top/stat.go:578`) — the visible-window slice
  must apply *after* / orthogonally to filtering; filtering removes rows, scrolling hides columns. The `*`
  filter-marker prefix in the header (`top/stat.go:370-374`) widens that header cell's *string* but the cell
  is still padded to `ColsWidth[i]+2`, so it does not change the visible width budget.

---

## 5. gocui line clipping (confirmed)

Module: `github.com/jroimartin/gocui@v0.5.0` (`go.mod`), backend `nsf/termbox-go v1.1.1`,
`OutputNormal` mode (`top/ui.go:19` `gocui.NewGui(gocui.OutputNormal)`).

- **`View.draw()`** — `gocui/view.go:288`. With `Wrap=false` (dbstat's case), each `\n`-terminated line
  becomes one `viewLine` (`:315-317`). The render loop (`:341-358`) iterates runes and **`break`s when
  `x >= maxX`** (`:340`) — i.e. **anything past the right edge is simply not drawn; the overflow is silently
  clipped, exactly as described.** Line breaks are explicit `\n` (`Write`, `:202-230`). `v.ox` (origin.x)
  is 0 by default and the feature deliberately does **not** use it.
- **ANSI escapes do not consume cells:** `View.Write` (`:202`) → `parseInput` (`:236`) → `ei.parseOne`.
  While inside an ESC sequence `parseInput` returns `nil` (`:255-257`) so **no cell is appended**; only
  visible runes become `cell`s carrying fg/bg color. Therefore the `\033[...m` color codes emitted by
  `printStatHeader` do **not** count toward `maxX` clipping. **Implication:** when computing how many
  columns fit, count only visible characters = `Σ(ColsWidth[i]+2)` over chosen columns; escape codes are free.
  The `‹`/`›` markers, however, ARE visible runes and DO consume cells — they must be budgeted into the
  header width (potential off-by-one with the frozen column / last visible column).

---

## 6. Existing Tests (top package)

Framework: `testify` (`assert`), standard `go test`. Pattern for handler tests: spin a goroutine that
reads one `view.View` off `config.viewCh`, run the handler, assert on the received view (see
`top/config_view_test.go`). `make test` runs with `-race` + coverage.

- **`top/config_view_test.go`** — handler tests. Most relevant invariants the new handlers must not break,
  and the template to copy:
  - `Test_orderKeyLeft` (`:11`) / `Test_orderKeyRight` (`:43`) — boundary wrap-around for sort key.
    Hardcode `views["activity"].Ncols == 13` (comment at `:16` says 13; the activity view def at
    `internal/view/view.go:39` lists `Ncols: 14` but is overwritten at runtime by
    `query.SelectStatActivityQuery` in `Configure` — the test uses the static map value, so a scroll test
    on `activity` should be aware `Ncols` is version-dependent at runtime).
  - `Test_increaseWidth` (`:75`) / `Test_decreaseWidth` (`:108`) — width clamp.
  - `Test_switchViewTo` (`:195`) — the big table of all view transitions (`:205-231`); reads `v.Name` off
    `viewCh`. **If a scroll-offset field is added to `view.View`, this test still passes** (it only asserts
    `.Name`), but any reset-on-switch logic added to `viewSwitchHandler`/`switchViewTo` should get its own
    assertion here. Closes `viewCh` at `:251`.
  - `Test_switchSortOrder` (`:142`), `Test_setFilter` (`:175`), `Test_toggleSysTables` (`:331`),
    `Test_databasesNextView`/`statioNextView`/`statementsNextView`/`progressNextView` (`:262`–`:329`).
- **`top/stat_test.go`** — `Test_alignViewToResult` (`:63`) constructs synthetic `stat.PGresult` via a
  `makeResult(ncols)` helper (`:64-73`) — directly reusable for unit-testing a pure window-calc function with
  N columns. `Test_formatInfoString` (`:15`), `Test_formatError` (`:35`).
- **No existing tests on `printStatHeader`/`printStatData`/`printDbstat`** — confirmed by grep. These print
  to a `*gocui.View` (hard to unit-test directly). **The interview mandates extracting the visible-window
  calculation into a pure, testable function** (`interview.yml` `integration_points`, `testing_strategy`) —
  there is no precedent test to extend, this is greenfield unit-test territory. Recommended assertions per
  interview: visible range for given offset+widths, frozen-first-column always included, offset clamped to
  `[0, maxOffset]`, "everything fits" → no markers / no-op.

---

## 7. Shared Utilities

- **`internal/align/align.go`** — `SetAlign(r stat.PGresult, truncLimit int, dynamic bool) (map[int]int, []string)`
  (`:14`). Called from `alignViewToResult` with `(r, 1000, false)` (`top/stat.go:313`). **Width-flooring
  facts relevant to "how many columns fit":**
  - Every column is floored at **8** chars: `colnamelen = math.Max(len(colname), 8)` (`:36`).
  - Non-last columns longer than the col name: width = value length, but if `valuelen > colnamelen*2` it is
    capped at 32 (`:49-53`, fixed/`!dynamic` branch used by `top`).
  - The **last** column uses `truncLimit` when set (`:63-65`); with `truncLimit=1000` from `top` it is
    effectively the full value width.
  So a printed column occupies `ColsWidth[i]+2` cells (the `+2` inter-column gap is added in the print fns,
  not in `SetAlign`). The window calc must sum `ColsWidth[i]+2`, with `ColsWidth[i] >= 8` always.
- **`internal/math`** — `math.Min`/`math.Max` (int helpers), used by width/offset clamping
  (`top/config_view.go:6`, `:52`, `:65`). Use for clamping the new offset to `[0, maxOffset]`.
- **`isFilterRequired(f map[int]*regexp.Regexp) bool`** — `top/stat.go:578`.

---

## 8. Potential Problems

- **Shared print path affects every stat screen.** `printStatHeader`/`printStatData` render *all*
  `dbstat` views (activity, tables, pg_stat_io, all pgss sub-screens, progress, procpidstat, etc.). A bug
  in the window logic regresses every screen. Mitigation (per interview): pure unit-tested window function +
  manual TUI pass across screens.
- **Frozen highlight vs. sort highlight collision (col 0).** When `OrderKey == 0` the first column is both
  *frozen* (bold name, `\033[47;1m` already used for the sort column) and *sorted*. The current code already
  paints `OrderKey` bold (`top/stat.go:382-387`); the frozen highlight is *also* bold. Need an explicit
  precedence/combination rule so the two do not conflict (interview `risks`, `edge_cases`). This is a
  header-only concern — data rows are not colored for either.
- **Marker width accounting.** `‹`/`›` are visible runes and consume cells (§5). Inserting them into the
  header line changes the visible width by 1–2 chars and can push the last visible column's last char past
  `maxX` (clipped). Decide whether markers occupy the frozen column's `+2` gap, overwrite a trailing pad
  char, or sit on a reserved cell. Off-by-one risk with `ColsWidth[i]+2`.
- **`colnum`/`i` dual-counter in `printStatData`** (`top/stat.go:418-438`). The existing loop already has a
  subtle separation between `i` (over `Cols`) and `colnum` (over printed cells, bumped only on `doPrint`).
  Introducing a visible-column subset adds a third index concept (visible vs. absolute column). Care needed
  so value lookup uses the **absolute** column index into `s.Result.Values[rownum][absCol]` while padding
  uses `ColsWidth[absCol]`.
- **In-place mutation of `s.Result.Values` during truncation** (`top/stat.go:429`). The current code mutates
  the result string in place when truncating. If the window logic re-reads the same `PGresult` (it is a fresh
  batch each render, so generally safe), be aware values are already `~`-truncated to `ColsWidth[i]`.
- **`Ncols` is runtime/version-dependent.** Static `view.New()` `Ncols` values are overwritten by
  `Views.Configure()` (`internal/view/view.go:366-417`) per PG version (e.g. activity, bgwriter, stat_io,
  statements_jit). The window/`maxOffset` calc must use the **runtime** `Ncols`/`len(Cols)` from the live
  result, never the static literal. `alignViewToResult` already keys off `r.Ncols`, so reading
  `s.Result.Ncols` at render time is the safe source.
- **Terminal resize.** No resize handler recomputes offset today; gocui re-`draw()`s on resize and the next
  render recomputes the window from `v.Size()`. Offset must be **clamped to the new `maxOffset`** each render
  (not just on key press) so a shrink does not leave it out of range (interview `edge_cases`).
- **Empty result (0 rows).** `printStatData`'s outer loop is skipped when `Nrows == 0`; the header (with
  scroll + markers) still renders. The window calc must not assume `Nrows > 0`.

### Decisions-log (ADRs) directly affecting approach — settled, do not re-litigate
- **ADR [006-feat-pg-stat-io]** (`docs/decisions-log.md:365`): column hiding is NOT available because
  `align.SetAlign` floors width at 8 and `ColsWidth` is a runtime cache. **This is exactly why this feature
  builds a manual visible-window in the print fns rather than zeroing `ColsWidth[i]` to hide columns.** The
  scroll mechanism is the sanctioned alternative — it does not touch `SetAlign` or the width cache, it slices
  which columns are printed.
- **ADR [006]/[007]** also establish that pg_stat_io and pg_stat_statements are intentionally split into
  sub-screens *as the current workaround*. Interview scope explicitly keeps those splits (does not un-split).

### Tech-debt (Active) touching this area
- No Active Debt items in `top/stat.go`, `top/config_view.go`, `top/keybindings.go`, `internal/align`, or
  `internal/view` (reviewed `docs/tech-debt.md`). The Active items ([005] `top/reload_test.go`, [008]
  `record/record_test.go`, [006] replslots standby, [009] tar size, [003] self-reviews) are all outside the
  files this feature changes. **No debt blocks or directly affects this work.**

---

## 9. Constraints & Infrastructure

- **TUI stack:** `jroimartin/gocui v0.5.0` + `nsf/termbox-go v1.1.1`, `OutputNormal`. ANSI color via raw
  `\033[..m` escapes embedded in `Fprintf` (gocui parses them, §5).
- **Key-modifier constraint (drives the `[`/`]` choice):** termbox v1.1.1 cannot distinguish `Shift+arrow`
  or `Ctrl+arrow` from plain arrows; gocui only exposes `gocui.ModNone`/`gocui.ModAlt`. Hence plain-rune
  hotkeys `[`/`]` (interview `constraints`, `technical_decisions`). Bindings registered on view `"sysstat"`.
- **Width flooring:** `internal/align.SetAlign` floors each column at 8 chars (`align.go:36`); `ColsWidth`
  is a runtime cache (re-derived each render via `alignViewToResult`), not a user preset.
- **Go 1.25+**, cobra/pgx-v5/gocui/testify stack (`.claude/CLAUDE.md`). Build/test/lint:
  `make build` / `make test` (race+coverage) / `make lint` (golangci-lint+gosec) / `make vuln` (govulncheck).
- **No record/report impact (confirmed).** `record/` and `report/` are a separate stdout/tar pipeline, not
  the TUI `*gocui.View` path. `printStatHeader`/`printStatData` are TUI-only (operate on `*gocui.View`); the
  report pipeline has its own formatters (e.g. `report/report.go`) and uses `align.SetAlign` with
  `dynamic=true`. Scroll is a render-time view-window concept with no recorded/serialized state, so neither
  package is touched. Interview `constraints`/`migration_needs` confirm: no data/format/config migration.

---

## Summary of change sites

| File | Symbol / line | Change |
|------|---------------|--------|
| `top/stat.go` | `printStatHeader` `:364` | visible-column loop; `‹`/`›` markers; frozen-name bold |
| `top/stat.go` | `printStatData` `:398` | visible-column loop (absolute-index value lookup) |
| `top/keybindings.go` | `keys` slice `:18` | add `{"sysstat", '[', scrollLeft(...)}`, `{"sysstat", ']', scrollRight(...)}` |
| `top/config_view.go` | near `:34` | `scrollLeft`/`scrollRight` handlers (mirror `orderKeyRight`, clamp not wrap) |
| `top/config_view.go` | `viewSwitchHandler` `:208` (+`switchViewToProcPidStat` `:220`) | reset offset to 0 on switch |
| `view.View` *or* `config` | `internal/view/view.go:10` / `top/config.go:10` | new offset field (tech-spec decides) |
| `top/help.go` | `helpTemplate` `:21` | document `[`/`]` |
| new pure fn + test | (greenfield) | visible-window calc + unit tests (reuse `makeResult` helper from `stat_test.go:64`) |
