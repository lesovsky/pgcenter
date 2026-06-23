---
created: 2026-06-23
status: draft
branch: dev
size: M
---

# Tech Spec: Горизонтальный скролл таблицы статистики

## Solution

Add by-column horizontal scrolling to the main stats table (`dbstat` view) in `pgcenter top`. The first column is frozen (always rendered); the remaining columns form a sliding window whose left edge is controlled by a scroll offset. The window is computed and rendered manually inside the existing print functions (`printStatHeader`/`printStatData`) — gocui's built-in viewport scroll is not used, because it would move the whole row and make freezing the first column impossible.

Scroll offset is held as ephemeral UI state on the `top.config` struct (not on `view.View`), driven by two new hotkeys `[` / `]`, reset to 0 on every view switch, and preserved across auto-refresh ticks within a view. Edge markers `‹` / `›` on the header row signal hidden columns to the left/right. The whole feature is confined to the `dbstat` area; extra side panels, the record/report pipeline, and the existing sub-screen splits (`pg_stat_io`, `pg_stat_statements`) are untouched.

## Architecture

### What we're building/modifying

- **`top.config.scrollOffset int`** (new field, `top/config.go`) — ephemeral horizontal scroll position of the active screen. Index into the *scrollable* columns (those after the frozen first column).
- **Pure window function** (new, `top/stat.go`) — given column count, per-column widths, terminal width, and current offset, returns the visible scrollable column range, the clamped offset, and `hasHiddenLeft`/`hasHiddenRight` flags. Unit-tested in isolation.
- **`printStatHeader` / `printStatData`** (modified, `top/stat.go`) — render the frozen first column plus the visible window instead of all columns; bold the frozen column name; draw `‹`/`›` edge markers in the header.
- **Scroll handlers + hotkeys** (new, `top/config_view.go` + `top/keybindings.go`) — `]` increments and `[` decrements `config.scrollOffset` (clamped), then notify via `viewCh`.
- **Offset reset** (modified, `top/config_view.go`) — zero `config.scrollOffset` in `viewSwitchHandler` (covers all `switchViewTo` branches) and in `switchViewToProcPidStat` (the second switch path that bypasses `viewSwitchHandler`).
- **Help screen** (modified, `top/help.go`) — document `[` / `]`.

### How it works

1. User presses `]` → handler clamps and increments `config.scrollOffset`, sends `config.view` on `viewCh`; the stats goroutine re-renders.
2. `printDbstat(v, config, s)` calls `printStatHeader` then `printStatData`. Both query terminal width via `v.Size()`, call the pure window function with `s.Result.Ncols`, `config.view.ColsWidth`, width, and `config.scrollOffset`, and obtain the visible scrollable range + marker flags + clamped offset.
3. Each renders column index 0 first (frozen), then iterates only the columns inside the visible window. The header bolds the frozen column name and prints `‹`/`›` where the flags indicate hidden columns.
4. On a view switch (`switchViewTo` → `viewSwitchHandler`, or `Shift+S` → `switchViewToProcPidStat`), `config.scrollOffset` is reset to 0 so the new screen starts unscrolled. Auto-refresh ticks do not touch the offset, so scrolling persists within a screen.

## Decisions

### Decision 1: Scroll offset lives on `top.config`, not on `view.View`
**Decision:** Store the scroll position as a new `scrollOffset int` field on the `top.config` struct, not as a field on `view.View`.
**Rationale:** `view.View` values are stored in `config.views` and `viewSwitchHandler` deliberately persists per-view state (`OrderKey`, `Filters`, `ColsWidth`) into that map so it is restored when the user returns to a screen. The user-spec requires the opposite for scroll — it must reset on every switch. Putting the offset on `view.View` would make it inherit that persistence and survive switches. `config` is the natural home for ephemeral "current screen" UI state; it is already threaded into both the render path (`printDbstat(v, app.config, s)`) and the key handlers.
**Alternatives considered:** Field on `view.View` with explicit zeroing on load — works but fights the intentional per-view persistence mechanism and needs zeroing in every load path anyway. Rejected as conceptually wrong (offset is not part of a view's definition).

### Decision 2: Manual column window in the print functions, not gocui viewport scroll
**Decision:** Compute the visible column window in a pure function and render the frozen column + window manually inside `printStatHeader`/`printStatData`.
**Rationale:** Freezing the first column is the core requirement. gocui's `SetOrigin`/viewport-x shift moves the entire buffered row, which cannot keep the first column fixed. The print functions already iterate columns, so manual windowing fits the existing structure. Extracting the window math into a pure function makes the only non-trivial logic unit-testable without a live terminal.
**Alternatives considered:** gocui viewport `origin.x` (cannot freeze first column — rejected). Hiding columns via `ColsWidth[i]=0` (impossible — `internal/align.SetAlign` floors width at 8, `ColsWidth` is a runtime cache, per ADR [006-feat-pg-stat-io]).

### Decision 3: Reset offset on both view-switch paths
**Decision:** Zero `config.scrollOffset` in `viewSwitchHandler` and, separately, in `switchViewToProcPidStat`.
**Rationale:** `switchViewToProcPidStat` does not delegate to `viewSwitchHandler` (it patches runtime fields onto the view manually and must not reload from the static map). A single reset in `viewSwitchHandler` would miss the `Shift+S` path, leaving a stale offset when entering the per-process screen.
**Alternatives considered:** Reset only in `viewSwitchHandler` — misses `procpidstat`. Reset inside the render path on a detected view-name change — adds hidden state tracking to the render loop. Both rejected.

### Decision 4: Sort-column highlight takes priority over the frozen-column highlight
**Decision:** The frozen first column's name is rendered bold. When the first column is also the active sort column (`OrderKey == 0`), the existing sort highlight (`\033[47;1m`) wins and the frozen-bold adds nothing.
**Rationale:** `printStatHeader` already renders the sort column with a distinct escape sequence; the first column can simultaneously be the sort column. Layering two bold/background sequences risks a garbled cell. Sort state is more informative to act on, so it takes precedence; both states being "emphasized" reads consistently.
**Alternatives considered:** Combine both into a third unique style — extra escape-sequence handling for a rare overlap, rejected as over-engineering.

### Decision 5: Edge markers `‹`/`›` are budgeted into header width
**Decision:** Draw `‹` near the left edge and `›` near the right edge of the header row only when columns are hidden in that direction; account for their cells when computing how many columns fit, so data-row column alignment is not shifted.
**Rationale:** Markers are visible runes (unlike ANSI escapes, which consume no cells), so naive printing risks an off-by-one in width accounting. Confining markers to the header row keeps data rows clean; reserving their width prevents header/data misalignment.
**Alternatives considered:** Markers in the cmdline area (user chose edge markers); markers overlaid on column-name characters (loses a name character). Exact glyph placement is an implementation detail handled in decomposition; the width-budget rule is the constraint.

## Data Models

No DB schema or serialized format changes. One new in-memory field:

```go
// top/config.go — config struct
scrollOffset int // horizontal scroll position over scrollable columns (index 1..Ncols-1); 0 = no scroll
```

The pure window function signature is illustrative (finalized in decomposition), e.g.:

```go
// returns visible scrollable column range [first,last], clamped offset, and hidden-side flags
func visibleColumns(ncols int, colsWidth map[int]int, termWidth, offset int) (first, last, clamped int, hiddenLeft, hiddenRight bool)
```

## Dependencies

### New packages
- None.

### Using existing (from project)
- `internal/align` — `ColsWidth` (per-column widths) feed the window width math; unchanged.
- `internal/math` — `Min`/`Max` for offset clamping (already used in `config_view.go`).
- `github.com/jroimartin/gocui` — `View.Size()` for terminal width; `SetKeybinding` for `[`/`]`.

## Testing Strategy

**Feature size:** M

### Unit tests
- `visibleColumns`: all columns fit → offset clamps to 0, both hidden flags false, full range returned.
- `visibleColumns`: narrow width, offset 0 → frozen col + leading scrollable cols, `hiddenRight=true`, `hiddenLeft=false`.
- `visibleColumns`: offset in the middle → `hiddenLeft` and `hiddenRight` both true, correct range.
- `visibleColumns`: offset past the end → clamped to last valid offset, `hiddenRight=false`.
- `visibleColumns`: very narrow terminal (only frozen column fits) → graceful range (no panic, no negative width).
- Offset clamping in scroll handlers: `[` at offset 0 stays 0; `]` at max stays max.

### Integration tests
- None. The scroll logic is pure UI math over an already-collected `PGresult`; it does not depend on SQL, PostgreSQL version, or a DB connection. Consistent with the user-spec testing decision.

### E2E tests
- None. TUI visual behavior is verified manually (no external automation harness exists for the gocui UI).

## Agent Verification Plan

**Source:** user-spec "Как проверить" section.

### Verification approach
Automated: run unit tests for the window function and scroll handlers; build and static-analysis gates. Visual scroll/freeze/marker behavior is verified by the user in a narrow terminal (cannot be asserted programmatically).

### Per-task verification
| Task | verify: | What to check |
|------|---------|--------------|
| 1 | bash | `go test ./top/...` — window-function unit tests pass |
| 2 | user | narrow terminal: `]`/`[` scroll columns, first column stays, `‹`/`›` appear correctly, frozen name bold |
| 3 | bash | `go test ./top/...` — handler/reset tests pass; `make build` |
| 4 | bash | `make test && make lint && make vuln` — full suite green |

### Tools required
- `bash` (go test, make).
- No MCP tools; no Playwright (terminal UI).

## Backward Compatibility

**Breaking changes:** no

**Migration strategy:** N/A — adds a new internal struct field and changes in-memory render behavior only. No public API, no DB schema, no on-disk/config/record format change. New hotkeys `[`/`]` are additive; existing keybindings are unchanged.

**DB migration compatibility:** N/A — no DB migration.

**Consumer impact:** none — `top.config` is an internal struct; `record`/`report` packages do not use the TUI print path (confirmed in code research).

## Risks

| Risk | Mitigation |
|------|-----------|
| `printStatHeader`/`printStatData` are shared by all stat screens; a windowing bug could break rendering everywhere. | Pure, unit-tested window function; manual verification across a set of screens (activity, tables, pg_stat_io). |
| `printStatData` uses a dual `colnum`/`i` counter and mutates `s.Result.Values` in place on truncation; windowing must index values by absolute column index correctly. | Explicit in tech-spec; covered by code review and the data-render manual check; keep value lookup on the absolute index. |
| Visible-rune markers `‹`/`›` mis-counted against width → off-by-one / header-data misalignment. | Budget marker cells into the width math (Decision 5); boundary unit test on marker flags. |
| Bold frozen-column highlight collides with sort-column highlight on column 0. | Sort highlight takes priority (Decision 4). |

## Acceptance Criteria

Технические критерии приёмки (дополняют пользовательские из user-spec):

- [ ] `visibleColumns` (pure function) has unit tests covering: all-fit, scroll-right, mid-scroll, past-end clamp, very-narrow, both marker flags.
- [ ] `config.scrollOffset` is reset to 0 in both `viewSwitchHandler` and `switchViewToProcPidStat`.
- [ ] `[` / `]` registered on the `sysstat` view; clamped at both ends.
- [ ] First column always rendered regardless of offset; offset indexes only columns `1..Ncols-1`.
- [ ] `‹`/`›` markers rendered only when columns are hidden in that direction.
- [ ] All existing `top` package tests pass with no regressions (`Test_switchViewTo`, `Test_orderKeyLeft/Right`, `Test_alignViewToResult`, etc.).
- [ ] `make test`, `make lint`, `make vuln` are green.
- [ ] Help screen documents `[` / `]`.

## Implementation Tasks

### Wave 1 (независимые)

#### Task 1: Pure column-window function + scroll-offset state
- **Description:** Add the `scrollOffset` field to `top.config` and implement the pure function that computes the visible scrollable column window (frozen first column + sliding range), the clamped offset, and the hidden-left/hidden-right flags from column widths, terminal width, and offset. This is the testable core of the feature, independent of rendering and key handling.
- **Skill:** code-writing
- **Reviewers:** dev-code-reviewer, dev-test-reviewer
- **Verify:** bash — `go test ./top/...`
- **Files to modify:** `top/config.go`, `top/stat.go`, `top/stat_test.go`
- **Files to read:** `top/config_view.go`, `internal/align/align.go`, `internal/view/view.go`

### Wave 2 (зависит от Wave 1)

#### Task 2: Render frozen column + visible window in header and data
- **Description:** Modify `printStatHeader` and `printStatData` to render the frozen first column plus the visible window returned by the Task 1 function instead of all columns, bold the frozen column name (sort highlight takes priority on column 0), and draw `‹`/`›` edge markers when columns are hidden. Value lookups must use absolute column indices and preserve the existing truncation behavior.
- **Skill:** code-writing
- **Reviewers:** dev-code-reviewer, dev-test-reviewer
- **Verify:** user — narrow terminal: scroll with `]`/`[`, first column stays fixed, markers appear correctly, frozen name bold
- **Files to modify:** `top/stat.go`
- **Files to read:** `top/config.go`, `internal/view/view.go`

#### Task 3: Scroll hotkeys, offset reset, and help text
- **Description:** Add `[` / `]` handlers that decrement/increment `config.scrollOffset` with clamping, register them on the `sysstat` view, reset the offset to 0 on both view-switch paths (`viewSwitchHandler` and `switchViewToProcPidStat`), and document the keys on the help screen. Depends on the offset field and clamp logic from Task 1.
- **Skill:** code-writing
- **Reviewers:** dev-code-reviewer, dev-test-reviewer
- **Verify:** bash — `go test ./top/...` (handler/reset tests) and `make build`
- **Files to modify:** `top/config_view.go`, `top/keybindings.go`, `top/help.go`, `top/config_view_test.go`
- **Files to read:** `top/config.go`, `top/stat.go`

### Final Wave

#### Task 4: Pre-deploy QA
- **Description:** Acceptance testing: run the full suite (`make test`, `make lint`, `make vuln`) and verify all acceptance criteria from user-spec and tech-spec, including manual narrow-terminal checks across a set of stat screens.
- **Skill:** pre-deploy-qa
- **Reviewers:** none
