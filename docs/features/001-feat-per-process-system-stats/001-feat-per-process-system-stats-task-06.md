---
status: planned
depends_on: ["04"]
wave: 3
skills: [code-writing]
verify: "bash — make build && make lint"
reviewers: [dev-code-reviewer, dev-security-auditor, dev-test-reviewer]
---

# Task 06: Hotkey, local-mode guard, and filter guard extensions

## Required Skills

Before starting, load:
- `/skill:code-writing` — [SKILL.md](~/.claude/skills/code-writing/SKILL.md)

## Description

This task wires the `procpidstat` screen into the TUI interaction layer. It is the final code
task for the per-process system stats feature and makes the new screen accessible to the user.

Four files are touched:

- `top/keybindings.go` — registers `'S'` (Shift+S) as the hotkey for the new screen.
- `top/config_view.go` — adds `switchViewToProcPidStat()`, the handler invoked by that hotkey.
  The function enforces the local-mode guard (`db.Local`), probes `/proc/self/io` availability,
  patches `CollectExtra` and `IOAvailable` onto the view struct, and sends the view on `viewCh`.
  Additionally the `toggleIdleConns` guard is extended to also permit the `'I'` filter on the
  `procpidstat` screen (Decision 8).
- `top/dialog.go` — the compound guard at line 51 currently blocks `dialogChangeAge` for all
  non-activity views. Per Decision 7, the `dialogChangeAge` check is extracted into its own
  `if` block that allows both `"activity"` and `"procpidstat"`. The cancel/terminate/mask
  dialogs remain `"activity"`-only.
- `top/help.go` — adds the `'S'` entry to the help text so users can discover the new screen.

All four changes are presentation-layer only; no new business logic is introduced here — the
actual stats collection (Task 05) and view registration (Task 04) were completed in earlier waves.

## What to do

1. In `top/keybindings.go`, add a keybinding entry `{"sysstat", 'S', switchViewToProcPidStat(app)}` to
   the `keys` slice, adjacent to the other uppercase letter bindings (`'B'`, `'N'`, `'F'`, `'L'`).

2. In `top/config_view.go`, implement `switchViewToProcPidStat(app *app)` returning
   `func(g *gocui.Gui, _ *gocui.View) error`. The function must:
   - Return early with a cmdline warning if `app.db.Local` is false
     ("Per-process stats available in local mode only").
   - Call `stat.CheckIOAvailable()` (exported, lives in `internal/stat/procpidstat.go` from Task 01).
   - If IO is unavailable, print a cmdline warning once
     ("Cannot read /proc/self/io: permission denied. Run as postgres user or via sudo.").
   - Save the current view back to the views map (same as the first line of `viewSwitchHandler`).
   - Load the `"procpidstat"` view from `app.config.views`.
   - Patch `v.CollectExtra = stat.CollectProcPidStat` and `v.IOAvailable = (ioErr == nil)`.
   - Set `app.config.view = v` and send `v` on `app.config.viewCh`.
   - Print the view's `Msg` to cmdline on success.

3. In `top/config_view.go`, extend `toggleIdleConns` guard: change the early-return condition
   from `config.view.Name != "activity"` to
   `config.view.Name != "activity" && config.view.Name != "procpidstat"`.

4. In `top/dialog.go`, restructure `dialogOpen` to isolate the `dialogChangeAge` check.
   The current compound guard `(d > dialogFilter && d <= dialogChangeAge) && name != "activity"`
   must be replaced by two separate checks:
   - First check: if `d` is one of `dialogCancelQuery`, `dialogTerminateBackend`,
     `dialogCancelGroup`, `dialogTerminateGroup`, or `dialogSetMask`, and the current view is
     not `"activity"` — print the appropriate message and return.
   - Second check: if `d == dialogChangeAge` and the current view is neither `"activity"` nor
     `"procpidstat"` — print the age-threshold message and return.

5. In `top/help.go`, add `'S'` to the help text. Place it in the `general actions` section
   alongside the other view-switching letters, or add a dedicated `per-process stats` subsection
   immediately after `extra stats actions`. The entry should read:
   `S   per-process system stats (local mode only; Shift+S).`

## TDD Anchor

All changes in this task are in the `top/` package which is not covered by automated unit tests
(TUI callbacks require `gocui.Gui` instances that cannot be constructed without a terminal).
The verification path is `make build && make lint` (compilation + static analysis).

There are no new exported functions with testable pure logic — `switchViewToProcPidStat` closes
over `app` and calls gocui primitives. The guard logic (local-mode, IO check, dialog dispatch)
is covered indirectly by build correctness and by the existing `internal/stat` and `record`
tests from earlier tasks.

If the project adds `top/` package tests in the future, the following cases would apply:
- `switchViewToProcPidStat` with `db.Local = false` — verify no view switch occurs, warning printed.
- `toggleIdleConns` called from `"procpidstat"` view — verify query is re-formatted and sent on `viewCh`.
- `dialogOpen(dialogChangeAge)` from `"procpidstat"` — verify dialog opens (no early return).
- `dialogOpen(dialogTerminateBackend)` from `"procpidstat"` — verify early return with message.

## Acceptance Criteria

- [ ] `make build` succeeds after all changes — no compilation errors
- [ ] `make lint` reports no new warnings
- [ ] Pressing `Shift+S` (`'S'`) in local mode switches to the `procpidstat` view (Msg shown in cmdline)
- [ ] Pressing `Shift+S` in remote mode prints "Per-process stats available in local mode only" and stays on current view
- [ ] When `/proc/self/io` is unreadable (EACCES), a warning is printed once and the screen still opens
- [ ] `'I'` (toggleIdleConns) works on the `procpidstat` screen — query is reformatted and sent on `viewCh`
- [ ] `'A'` (dialogChangeAge) opens on the `procpidstat` screen without showing the "activity only" error
- [ ] Cancel/terminate/mask dialogs (`'-'`, `'_'`, `'n'`, `'k'`, `'K'`) on the `procpidstat` screen show "allowed in pg_stat_activity view only" and do NOT open a dialog
- [ ] `'S'` key appears in `top/help.go` help text with correct description
- [ ] `keybindings.go` registers `'S'` as a `"sysstat"` binding calling `switchViewToProcPidStat(app)`

## Context Files

**Feature artifacts:**
- [001-feat-per-process-system-stats.md](docs/features/001-feat-per-process-system-stats/001-feat-per-process-system-stats.md) — user-spec
- [001-feat-per-process-system-stats-tech-spec.md](docs/features/001-feat-per-process-system-stats/001-feat-per-process-system-stats-tech-spec.md) — tech-spec

**Project knowledge:**
- [architecture.md](.claude/skills/project-knowledge/architecture.md)
- [patterns.md](.claude/skills/project-knowledge/patterns.md)

**Code files (modify):**
- [top/keybindings.go](top/keybindings.go) — add `'S'` binding for `switchViewToProcPidStat`
- [top/config_view.go](top/config_view.go) — add `switchViewToProcPidStat`; extend `toggleIdleConns` guard
- [top/dialog.go](top/dialog.go) — restructure compound guard to isolate `dialogChangeAge`
- [top/help.go](top/help.go) — add `'S'` entry to help text

**Code files (read for context):**
- [top/pglog.go](top/pglog.go) — `db.Local` guard pattern (`if !db.Local { printCmdline(...); return nil }`)
- [top/ui.go](top/ui.go) — `printCmdline` signature and behavior

## Verification Steps

1. Run `make build` — must complete without errors. A compilation error means a missing import,
   wrong function signature, or reference to a not-yet-exported symbol from Tasks 01/04/05.
2. Run `make lint` — must report no new warnings. Pay attention to `errcheck` (all errors from
   `stat.CheckIOAvailable()` must be handled) and `revive` (unused parameters).
3. Manually confirm: grep for `'S'` in `top/keybindings.go` — entry must exist.
4. Manually confirm: grep for `switchViewToProcPidStat` in `top/config_view.go` — function must exist.
5. Manually confirm: `dialogChangeAge` guard in `top/dialog.go` allows `"procpidstat"`.
6. Manually confirm: `toggleIdleConns` guard in `top/config_view.go` allows `"procpidstat"`.

## Details

### Files

**`top/keybindings.go`** (current state: 83 lines, defines `keys []key` slice)

The `keys` slice currently includes uppercase bindings at lines 47–50:
`'B'` → `showExtra(CollectDiskstats)`, `'N'` → `showExtra(CollectNetdev)`, etc.
Add `{"sysstat", 'S', switchViewToProcPidStat(app)}` immediately after these entries.
No import changes needed — `switchViewToProcPidStat` will be in the same package.

**`top/config_view.go`** (current state: 349 lines)

Two changes:

1. New function `switchViewToProcPidStat(app *app) func(g *gocui.Gui, _ *gocui.View) error`.
   This follows the same closure pattern as `switchViewTo` and `showExtra`.
   - Guard: `if !app.db.Local` — use `printCmdline(g, "Per-process stats available in local mode only")`
     and `return nil`. Pattern is identical to `showPgLog` in `top/pglog.go`.
   - `ioErr := stat.CheckIOAvailable()` — note this must be the exported name; confirm the
     exact function name from `internal/stat/procpidstat.go` created in Task 01.
   - If `ioErr != nil` — `printCmdline(g, "Cannot read /proc/self/io: permission denied. Run as postgres user or via sudo.")` — do NOT return; the screen still opens, IO columns will be empty.
   - Save current view: `app.config.views[app.config.view.Name] = app.config.view`
   - Load view: `v := app.config.views["procpidstat"]`
   - Patch: `v.CollectExtra = stat.CollectProcPidStat` and `v.IOAvailable = (ioErr == nil)`
   - Apply: `app.config.view = v` and `app.config.viewCh <- v`
   - Print: `printCmdline(g, "%s", v.Msg)`
   - Required imports: `"github.com/lesovsky/pgcenter/internal/stat"` — check if already imported;
     if not, add it. The package is already imported in `top/stat.go` so no cycle is introduced.

2. `toggleIdleConns` guard (line 302): change `config.view.Name != "activity"` to
   `config.view.Name != "activity" && config.view.Name != "procpidstat"`.
   The rest of the function body is unchanged.

**`top/dialog.go`** (current state: 179 lines)

The compound guard at line 51 is:
```
if (d > dialogFilter && d <= dialogChangeAge) && app.config.view.Name != "activity" {
```
This must be split. The replacement logic:
- If `d` is in range `(dialogFilter, dialogCancelQuery..dialogSetMask]` and view is not
  `"activity"` → print the existing message for cancel/terminate/mask and return.
- If `d == dialogChangeAge` and view is neither `"activity"` nor `"procpidstat"` → print the
  existing age message and return.

Both checks must still call `printCmdline(g, "%s", msg)` and `return nil`, keeping the exact
message strings from the existing switch-case. No other changes to `dialog.go`.

**`top/help.go`** (current state: 81 lines, `helpTemplate` const)

The help text has sections: `general actions`, `extra stats actions`, `activity actions`,
`other actions`. Add `'S'` to the `general actions` section, on the line that lists the single-
letter view switches (`a,f,r,w` / `s,t,i`). The `s` key is already listed there for "sizes" —
`'S'` (uppercase) is a separate key. Add a distinct line or append it to the existing format.
The simplest approach: add a new line in `general actions`:
```
    S                 'S' per-process system stats (local mode only).
```

### Dependencies

- Depends on Task 04 (`"procpidstat"` view registered in `view.New()`, `CollectProcPidStat`
  constant defined in `internal/stat/stat.go`).
- Depends on Task 01 (`stat.CheckIOAvailable()` function exported from `internal/stat/procpidstat.go`).
- Task 05 (Collector integration) must also be complete for the screen to actually collect data,
  but this task's compilation only requires the view struct and constant from Task 04.

### Edge Cases

- `switchViewToProcPidStat` must NOT call `viewSwitchHandler` — that function overwrites
  `config.view` from the static map and discards the runtime patches (`CollectExtra`,
  `IOAvailable`). Implement the view save/load/patch inline as specified above.
- Do not print the IO-unavailable warning as a fatal error. The screen must still switch even
  when IO is unavailable; the warning is informational only.
- The `dialogChangeAge` restructuring must not accidentally allow termination dialogs on
  `"procpidstat"`. Only `dialogChangeAge` gets the extended allowlist. The `cancel`, `terminate`,
  `mask` checks retain their `"activity"`-only guard.
- `'S'` is Shift+S. In gocui, `'S'` (Go rune literal) correctly maps to the uppercase S key.
  No special modifier configuration is needed — verify against existing uppercase bindings like
  `'Q'`, `'E'`, `'D'` which follow the same pattern.

## Reviewers

- **dev-code-reviewer** → `docs/features/001-feat-per-process-system-stats/001-feat-per-process-system-stats-task-06-dev-code-reviewer-review.json`
- **dev-security-auditor** → `docs/features/001-feat-per-process-system-stats/001-feat-per-process-system-stats-task-06-dev-security-auditor-review.json`
- **dev-test-reviewer** → `docs/features/001-feat-per-process-system-stats/001-feat-per-process-system-stats-task-06-dev-test-reviewer-review.json`

## Post-completion

- [ ] Write a brief report to `docs/features/001-feat-per-process-system-stats/001-feat-per-process-system-stats-decisions.md` (summary 1-3 sentences, review links, no findings dumps)
- [ ] If deviated from spec — describe the deviation and reason
- [ ] Update user-spec/tech-spec if anything changed
