---
created: 2026-08-03
status: draft
branch: feature/016-feat-pause-display
size: M
---

# Tech Spec: Пауза обновления экрана по `Space`

## Solution

The pause is a **display gate plus a stored frame**, both living entirely in the gocui goroutine,
with exactly one cross-goroutine value: an `atomic.Bool` on `top.config`.

The collector is not touched. `doWork` keeps receiving from `statCh` unconditionally — that is not an
optimisation but the only non-hanging shape: `collectStat` only reaches its `viewCh` receive *after*
a successful `statCh` send (`top/stat.go:72-84`), and ~17 key handlers push on the unbuffered
`viewCh`, so a gate that stops receiving would park the collector and the first keypress would hang
`MainLoop` forever, with `Space` itself unable to recover it.

The frame is published into a gocui-goroutine-only store **from inside the render closure**
(`printStat`'s `g.Update`, `top/stat.go:169-252`). That placement is what makes the whole feature
mutex-free: the logtail buffer is produced inside that same closure (`top/stat.go:227,245`), so the
frame and the log lines are written in one place, on one goroutine, in one statement group. The gate
in `doWork` therefore only ever *discards and repaints*; it never writes shared state.

Repainting is driven by the mechanism the codebase already has. Five render-only keys (`[`, `]`,
`↑`, `↓`, `\`) push on `viewCh` today "solely to trigger an immediate redraw"
(`top/config_view.go:57-58`); that push causes the collector to emit a frame promptly
(`top/stat.go:124-133` falls through to `c.Update` and the send), so the gate's rule — *a discarded
frame repaints the store* — makes all five work unchanged. Only two paths need explicit wiring: the
filter dialog (which pushes nothing at all, even today) and terminal resize (for which gocui delivers
no event).

Everything else is small and local: a non-mutating `printDataCell`, a stored timestamp so the header
clock freezes with its frame, a skipped log-file read while paused, and a `[PAUSED]` token added to
the [015] cmdline composer as data.

No SQL, no new views, no `view.View` field, no recorded-format change. `record`, `report` and
`profile` are untouched.

## Architecture

### What we're building/modifying

- **`config.paused atomic.Bool` (`top/config.go`)** — the only cross-goroutine value: written by the
  `Space` handler on the gocui goroutine, read by `statLoop` on the worker goroutine.
- **`frameStore` (new, `top/pause.go`)** — the frozen frame: `stat.Stat`, its capture timestamp, the
  last non-empty logtail buffer and its path, plus a `valid` flag. Owned by the gocui goroutine
  exclusively; written only from inside `printStat`'s render closure.
- **`statLoop` (extracted from `doWork`, `top/ui.go`)** — the receive loop, lifted out so the drain
  behaviour is unit-testable without a terminal or Postgres. Contains the gate.
- **`togglePause` + `pauseToken` (new, `top/pause.go`)** — the key handler and the cmdline token,
  mirroring `top/verbose.go` in size and shape.
- **`repaintStored` (new, `top/pause.go`)** — renders the stored frame through the existing
  `printStat` render path, skipping the two frame-external side effects (the log-file read and the
  first-tick hint).
- **`printDataCell` (`top/stat.go`)** — becomes non-mutating.
- **`renderSysstat` (`top/stat.go`)** — takes the frame's timestamp instead of calling `time.Now()`.
- **`layout` (`top/ui.go`)** — gains a size-change detector and the `[PAUSED]` re-emission on
  cmdline creation.
- **`dialogFinish` (`top/dialog.go`)** — the `dialogFilter` branch gains an explicit repaint.
- **`keybindings` / `helpTemplate`** — one row and one two-line entry.

### How it works

**Live (pause off).** Unchanged: collector → `statCh` → `doWork`/`statLoop` → `printStat`. Inside
`printStat`'s render closure, after the panels are drawn, the frame, `time.Now()` and the current
logtail buffer are stored. So the store always holds exactly what is on screen.

**Pausing.** `Space` sets `config.paused` and performs exactly one silent cmdline re-render, so
`[PAUSED]` appears immediately.

**Paused.** `statLoop` receives every frame and discards it, then calls `repaintStored`. Frames keep
flowing, the collector never blocks, and nothing overwrites the store — the freeze is real, not
cosmetic.

**Interacting with the frozen frame.** `[`, `]`, `↑`, `↓`, `\` push `viewCh` as they do today; the
collector answers with a frame; the gate discards it and repaints the store, so the new scroll
offset / column width / filter set is applied to the frozen data. The filter dialog does not push,
so its branch calls `repaintStored` directly.

**Resize.** `layout` compares the current terminal size with the previous one it saw and, on a
change while paused, requests a repaint of the store.

**Pager / editor return.** `mainLoop` rebuilds the `Gui` and starts a new `doWork`. The flag and the
store live on `config`/`app`, so both survive. The new `layout` re-emits `[PAUSED]` when it creates
the cmdline view — independently of any frame repaint, because there may be no stored frame at all
(pause engaged before the first frame). The frame repaint itself is driven by the same size detector,
whose state is fresh after the rebuild.

**Resuming.** `Space` clears the flag and re-renders the cmdline once. The next collector frame
renders normally; because the collector never stopped, its deltas are per-interval as usual.

## Decisions

### Decision 1: The frame store is published from the render closure, not guarded by a mutex

**Decision:** the store lives on `app` and is written **only** inside `printStat`'s `g.Update`
closure, i.e. only on the gocui goroutine. `statLoop` never writes it.

**Rationale:** the logtail buffer is produced inside that closure (`top/stat.go:227,245`) while
`stat.Stat` arrives on the worker goroutine (`top/ui.go:128`). Publishing from the closure is the
only arrangement that puts both halves in one place without synchronisation. The happens-before edge
is already there: `statCh <- stats` → receive → `g.Update` (which spawns a goroutine and sends on the
buffered `userEvents`, `gocui/gui.go:311-313`) → consumption in `MainLoop` (`gui.go:376`). This also
matches the package rule from `patterns.md` — read state only on the gocui goroutine — and ADR [015],
which established the ambient-config discipline for exactly this reason.

**Alternatives considered:** a `sync.Mutex` on `top.config` (rejected: it would be the package's
first lock, and it would still not solve where the logtail buffer is captured, since that only exists
on the gocui side); storing on `view.View` (rejected by ADR [009]/[010] reasoning — it would ride
`viewCh` and disturb `collectStat`'s load-bearing change-detection ladder, `top/stat.go:86-122`).

**Autopilot note:** the user-spec deliberately left this open and validation leaned this way; the
research settled it on the logtail evidence above.

### Decision 2: The gate is discard-and-repaint, and the publication needs no `if !paused`

**Decision:** `if paused { <-statCh; repaintStored(app); continue }`. The store publication inside the
render closure stays unconditional.

**Rationale:** while paused, the only thing that reaches the render closure is a repaint *of the
stored frame itself*, so the publication is an identity write. Adding a condition would guard against
nothing while creating a second place where "is it paused" must be answered consistently.

**Alternatives considered:** publishing from the gate (rejected — puts a write on the worker
goroutine and reintroduces the need for a lock); conditional publication (rejected as above).

### Decision 3: The repaint of render-only keys rides the existing `viewCh` push

**Decision:** no new repaint calls in `scrollLeft`/`scrollRight`/`increaseWidth`/`decreaseWidth`/
`clearFilters`. The gate's "discarded frame repaints the store" rule covers all five.

**Rationale:** verified in code — a `viewCh` push while paused makes `collectStat` fall through
(`top/stat.go:124-133`) into `c.Update` and a send, so a frame arrives promptly rather than at the
next tick. Five explicit repaint calls would be five places to keep in sync with the gate.

**Alternatives considered:** explicit repaints in every handler (rejected: more code, same effect).

### Decision 4: `setFilter` gets an explicit repaint, NOT a `viewCh` push

**Decision:** the `dialogFilter` branch of `dialogFinish` (`top/dialog.go:216-217`) calls
`repaintStored` when paused. `setFilter` itself is left alone.

**Rationale:** making `setFilter` push on `viewCh` would be the symmetric-looking fix and is wrong:
that path runs `c.Reset()` and derives `itv` arithmetically from the refresh interval
(`internal/stat/stat.go:294`), so the next frame would show near-zero values. That is a visible
regression **in live mode**, outside this feature's perimeter, in exchange for internal symmetry.

**Alternatives considered:** the `viewCh` push (rejected on the regression above); repainting
unconditionally rather than only when paused (rejected — a redundant repaint in live mode competes
with the natural frame).

### Decision 5: The resize detector lives at the end of `layout`, and swallows its own errors

**Decision:** `layout` keeps the last seen `maxX/maxY` in its enclosing closure (the
`verboseTooShortShown` precedent, `top/ui.go:141`), compares at the end of the function — after every
`SetView` and after the zero-geometry guard — and, when the size changed and the pause is on,
requests a repaint. The repaint closure **returns `nil` unconditionally**, logging nothing.

**Rationale:** gocui delivers no resize event; `layout` is the only place that sees the new size.
`g.Update` from inside `Layout` is safe in `jroimartin/gocui@v0.5.0` — it touches no `Gui` state,
takes no locks, and hands the send to its own goroutine. The loop cannot run away because the
detector's state is updated *before* the repaint is requested and a repaint does not change the
terminal size. The error-swallowing is load-bearing: an error out of a `g.Update` closure tears down
`MainLoop` (`gui.go:377-379`), which rebuilds the UI, which recreates the closure with a zeroed
detector, which repaints again — bounded only by the `errorRate` guard (`top/ui.go:92-95`) that
terminates the program.

**Alternatives considered:** lowering the requirement so resize does not repaint (rejected by the
user-spec — after widening the terminal the operator expects to see more columns); a polling
goroutine (rejected — a second timer for something `layout` already observes).

### Decision 6: The stored logtail buffer is the last NON-EMPTY read

**Decision:** store `buf` and `path` when `printLogtail` actually has content; on repaint, render
from the stored buffer without touching the file.

**Rationale:** `readLogfileRecent` returns `nil` when the file has not changed, and `printLogtail`
then prints nothing (`top/stat.go:1230-1232`). Storing "the buffer of the frame on which `Space` was
pressed" would therefore leave the panel empty on a quiet log — precisely the defect the user-spec
closed. `config.logtail.Size` is written only inside the skipped branch (`top/stat.go:243`), so it
freezes with everything else, and the rotation detector (`size < logtail.Size`, `:233-240`) still
fires correctly on resume. Residual gap, pre-existing and merely widened by a long pause: if the new
file outgrows the frozen size before resume, the read continues against the old descriptor.

**Alternatives considered:** re-reading the file on repaint (rejected — the log would run ahead of
frozen statistics, and a read error would propagate out of the render closure).

### Decision 7: `[PAUSED]` is re-emitted where the cmdline view is created

**Decision:** in `layout`'s cmdline-creation branch (`top/ui.go:182-192`), as an `else if` to the
existing `uiError` write.

**Rationale:** the cmdline view is recreated empty on every UI rebuild and nothing else writes to it
on the pager path. Tying the marker to the frame repaint would lose it exactly when there is no
stored frame. One write per path is preserved (it is an `else if`), and the clear timer is not armed
because the message is empty (`top/ui.go:461`); stale timers from the previous `Gui` are already
fenced by `uiGeneration` (`ui.go:57`).

### Decision 8: `printDataCell` becomes non-mutating instead of deep-copying the frame

**Decision:** truncate into a local variable rather than writing back into
`s.Result.Values[rownum][i].String` (`top/stat.go:1113`).

**Rationale:** this is the cause, not a symptom. That line is the only write into `s.Result.Values`
in all of `top/`, so removing it both protects the stored frame from its own rendering and eliminates
the aliasing write into the collector's snapshot for `DiffIntvl=[0,0]` views (`activity`, the default
screen). It also removes the need to deep-copy the frame at all. `Test_printStatData_truncation`
(`top/stat_test.go:1269-1284`) asserts rendered output, not structure, so it stays green.

**Alternatives considered:** deep-copying `Result.Values` at store time (rejected — larger and leaves
the underlying defect in place; the owner chose the causal fix explicitly).

### Decision 9: The frame timestamp is taken in `top/`, not added to `stat.Stat`

**Decision:** `printStat` captures `time.Now()` alongside the frame and stores it; `renderSysstat`
takes it as a parameter instead of calling `time.Now()` itself (`top/stat.go:277`).

**Rationale:** `stat.Stat` carries no collection time and the user-spec confines the change to
`top/`. The difference between "collected at" and "rendered at" is bounded by one refresh interval
and is invisible on screen. Five test call sites need the new argument (`top/stat_test.go:60, 97,
192, 440, 441`; `:192` is a shared helper covering eight tests).

**Autopilot assumption:** the spec says the clock shows "the time of the frame on the screen"; taking
the stamp at render time satisfies that reading and keeps `internal/stat` untouched.

### Decision 10: `statLoop` is extracted so the drain can be proven, and `wg.Wait()` stays where it is

**Decision:** lift the select loop out of `doWork` into `statLoop(ctx, uiExit, statCh, paused,
render, repaint)`, returning why it exited. **Do not** add a `wg.Wait()` to the `uiExit` branch.

**Rationale:** the loop is the feature's only concurrency-sensitive code and is untestable inside
`doWork`. The `wg.Wait()` asymmetry is deliberate in the existing code — it runs only on the
`ctx.Done()` branch (`top/ui.go:130-132`); adding it to the `uiExit` branch would hang the pager path
forever, because the collector is parked on a send that nobody will receive until the new `doWork`
starts.

### Decision 11: Bind `gocui.KeySpace`, scoped to `"sysstat"`

**Decision:** one row in the keybinding table, `{"sysstat", gocui.KeySpace, togglePause(app)}`.

**Rationale:** termbox classifies bytes `<= 0x20` as functional keys and emits `Ch = 0, Key =
KeySpace`, while gocui matches on `key && ch && mod` — so a rune binding (`' '`) would never fire.
Scoping to `"sysstat"` is what keeps a space typed into a dialog a plain space. Note the keybinding
table has **no test coverage at all**, so a wrong constant here is caught only on the stand.

## Data Models

```go
// top/pause.go
type frameStore struct {
    stats     stat.Stat  // the frozen frame, as received
    at        time.Time  // when it was rendered — freezes the header clock
    logBuf    []byte     // last NON-EMPTY logtail read (nil if the panel was closed)
    logPath   string     // path that buffer came from
    valid     bool       // false until the first frame is rendered
}
```

`config` gains one field:

```go
paused atomic.Bool  // written by the Space handler (gocui), read by statLoop (worker)
```

No database, no serialized format, no `view.View` change.

## Dependencies

### New packages

None. `sync/atomic` is already used in `top/` (`uiGeneration`, `top/ui.go:29`).

### Using existing (from project)

- `top/ui.go` cmdline composer ([015]) — the `[PAUSED]` token is added as data; `composeCmdline`,
  `renderCmdlineTokens` and the degradation ladder are untouched.
- `printStat` (`top/stat.go:157`) — reused verbatim as the repaint entry point, with the two
  frame-external side effects skipped.
- `topBandLayout` (`top/layout.go:41`) — untouched pure geometry.
- `visibleColumns` ([009]) — untouched; the repaint recomputes the window for the current width.

## Testing Strategy

**Feature size:** M

### Unit tests

- `statLoop` drains while paused: a fake `statCh` and a counter prove the sender completes **≥ 5**
  sends with the flag set, and that `render` is never called while `repaint` is.
- `statLoop` renders normally with the flag clear, and exits on `uiExit` and on `ctx.Done()` with the
  right reason (guarding the `wg.Wait()` asymmetry from Decision 10).
- `pauseToken` / `cmdlineTokens`: `[PAUSED]` present only when paused, positioned left of the filter
  token, single variant, never degraded — asserted against a `bytes.Buffer` via `composeCmdline`.
- `printDataCell` is non-mutating: the input value is unchanged after rendering a truncated cell, and
  the rendered output is byte-identical to today's.
- `renderSysstat` prints the passed timestamp, not the wall clock.
- Frame-store discipline: publishing a frame then repainting yields the same rendered bytes; a
  second, different frame arriving while paused does not change what a repaint renders.
- Logtail store: an empty read does not overwrite the stored buffer; a repaint renders the stored
  buffer and performs no file access.
- Resize detector: a pure helper (`sizeChanged(lastX, lastY, x, y)`) table-tested, so the arithmetic
  is not buried inside `layout`.

### Integration tests

None. The feature adds no query and touches no database surface — confirmed in the user-spec.

### E2E tests

None. There is no automated end-to-end harness for the TUI; its role is played by the stand run
below, which is part of the Final Wave.

## Agent Verification Plan

**Source:** user-spec "Как проверить" — 15 numbered steps, reproduced there in full.

### Verification approach

`make test` / `make lint` / `make vuln` for the automated half; a tmux stand run per `patterns.md`
for everything that only exists on a live terminal. The stand run must build fresh, ship the binary
explicitly, use fixed geometry (`-x 190 -y 52` plus a narrow `-x 60` pass), and carry a second binary
built from `master` so a regression is separated from pre-existing behaviour. The stand address is
requested from the owner at the start of the run — it is deliberately not stored in the repository.

### Per-task verification

| Task | verify: | What to check |
|------|---------|--------------|
| 1 | bash | `go test ./top/...` — truncation output unchanged, source value unmutated |
| 2 | bash | `go test ./top/...` — token present/absent, position, no degradation |
| 3 | bash | `go test ./top/...` — ≥5 collector sends complete while paused; no render while paused |
| 4 | bash | `go test ./top/...` — clock renders the stored stamp |
| 5 | bash | `go test ./top/...` — empty read keeps the buffer; repaint touches no file |
| 6 | bash | `go test ./top/...` — resize helper table; filter repaint invoked when paused |
| 7 | bash | `go test ./top/...` — marker re-emitted on cmdline creation |
| 8 | bash | `go build ./...` + help text contains the `Space` entry and the lifting set |
| 9 | bash | full stand run: all 15 user-spec steps |

### Tools required

`bash`, `make`, `tmux` over ssh on the stand. No MCP tooling, no browser automation.

## Backward Compatibility

**Breaking changes:** no.

`renderSysstat` and `printDataCell` are unexported functions inside `top/`; their signature and
behaviour changes are contained in the package. No exported API, no CLI flag, no config file, no
recorded-format field is added or altered, so archives written by any previous version replay
identically and `pgcenter record`/`report`/`profile` are unaffected.

**Migration strategy:** N/A — nothing persisted changes.

**DB migration compatibility:** N/A — no database objects.

**Consumer impact:** none found. `grep` over the repository shows `renderSysstat` and `printDataCell`
are called only from `top/` and its tests.

## Risks

| Risk | Mitigation |
|------|-----------|
| A gate that stops receiving hangs the whole UI, unrecoverably | Drain-and-discard is the specified shape; `statLoop` is extracted precisely so a test proves the collector keeps completing sends |
| An error escaping a repaint closure tears down `MainLoop` → rebuild → repaint → repeat | Every repaint closure returns `nil` unconditionally (Decision 5); the detector updates its state before requesting the repaint |
| A render-only key silently does nothing while paused | Decision 3 covers five keys through the existing push; Decision 4 covers the sixth; the stand run exercises each by name |
| `setFilter` "symmetry fix" via `viewCh` introduces a near-zero frame in live mode | Recorded as Decision 4 with the reason, so a later reviewer does not re-introduce it |
| Wrong key constant (`' '` instead of `gocui.KeySpace`) is invisible to tests | The keybinding table has no test coverage — the stand run is the only guard, and it is a named acceptance step |
| The stored frame is silently replaced by later frames, making the freeze cosmetic | Publication happens only in the render path; while paused the only render is the identity repaint (Decision 2), and stand step 3a checks values after ≥3 minutes |
| Long pause widens the logtail rotation-detection gap | Pre-existing behaviour, documented in Decision 6; not introduced here |

## Acceptance Criteria

- [ ] `make test` passes with `-race`; no new races reported.
- [ ] `make lint` and `make vuln` clean.
- [ ] `statLoop` test proves ≥5 completed collector sends while paused.
- [ ] No production code outside `top/` is modified.
- [ ] `view.View` gains no field; `record`/`report`/`profile` untouched.
- [ ] Every existing test in `top/` still passes; the five `renderSysstat` call sites updated
      mechanically, no assertion weakened.
- [ ] All user-spec acceptance criteria verified — automated ones by tests, terminal ones by the
      stand run.

## Implementation Tasks

### Wave 1 (независимые)

#### Task 1: Non-mutating data cell rendering
- **Description:** Make `printDataCell` truncate into a local instead of writing back into the
  result set. This protects any stored frame from being destroyed by its own rendering and removes
  the aliasing write into the collector's snapshot that exists today for `DiffIntvl=[0,0]` screens.
- **Skill:** code-writing
- **Reviewers:** dev-code-reviewer, dev-test-reviewer
- **Verify:** bash — `go test ./top/...`
- **Files to modify:** `top/stat.go`, `top/stat_test.go`
- **Files to read:** `docs/features/016-feat-pause-display/016-feat-pause-display-code-research.md`

#### Task 2: Pause flag, `Space` binding and the `[PAUSED]` token
- **Description:** Add the pause flag to `config`, bind `Space` in the stats context, and add the
  marker to the cmdline composer as a single-variant token positioned left of the filter indicator.
  The handler performs exactly one silent cmdline re-render so the marker appears immediately.
- **Skill:** code-writing
- **Reviewers:** dev-code-reviewer, dev-test-reviewer
- **Verify:** bash — `go test ./top/...`
- **Files to modify:** `top/config.go`, `top/keybindings.go`, `top/pause.go`, `top/ui.go`,
  `top/ui_test.go`
- **Files to read:** `top/verbose.go`, `docs/decisions-log.md`

### Wave 2 (зависит от Wave 1)

#### Task 3: The gate, the frame store and the repaint rule
- **Description:** Extract the receive loop from `doWork` into a testable `statLoop`, add the
  discard-and-repaint gate, and publish the frame into a gocui-goroutine-only store from inside the
  render closure. This is the feature's core and the only concurrency-sensitive part.
- **Skill:** code-writing
- **Reviewers:** dev-code-reviewer, dev-security-auditor, dev-test-reviewer
- **Verify:** bash — `go test ./top/... -race`
- **Files to modify:** `top/ui.go`, `top/pause.go`, `top/stat.go`, `top/ui_test.go`
- **Files to read:** `top/stat.go`, `internal/stat/stat.go`,
  `docs/features/016-feat-pause-display/016-feat-pause-display-code-research.md`

### Wave 3 (зависит от Wave 2)

#### Task 4: Frozen header clock
- **Description:** Thread the frame's capture timestamp into the summary-panel renderer so a
  repainted frame shows the time of the data on screen rather than the current wall clock. Without
  this the frozen frame carries a live clock, which is the incoherence the feature exists to avoid.
- **Skill:** code-writing
- **Reviewers:** dev-code-reviewer, dev-test-reviewer
- **Verify:** bash — `go test ./top/...`
- **Files to modify:** `top/stat.go`, `top/stat_test.go`
- **Files to read:** `top/pause.go`

#### Task 5: Logtail panel freeze and restore
- **Description:** Skip the log-file read while paused and render the panel from the last non-empty
  buffer stored with the frame. Without storing the buffer the panel comes back empty after a UI
  rebuild, since nothing else in the application holds those lines.
- **Skill:** code-writing
- **Reviewers:** dev-code-reviewer, dev-security-auditor, dev-test-reviewer
- **Verify:** bash — `go test ./top/...`
- **Files to modify:** `top/stat.go`, `top/pause.go`, `top/stat_test.go`
- **Files to read:** `docs/features/016-feat-pause-display/016-feat-pause-display-code-research.md`

#### Task 6: Filter-dialog repaint and the resize detector
- **Description:** Repaint the stored frame when a filter is applied through the dialog (that path
  triggers no redraw of its own even today), and detect terminal size changes in the layout callback
  so a paused frame is re-laid out for the new width. Both repaint closures must swallow their errors.
- **Skill:** code-writing
- **Reviewers:** dev-code-reviewer, dev-test-reviewer
- **Verify:** bash — `go test ./top/...`
- **Files to modify:** `top/dialog.go`, `top/ui.go`, `top/pause.go`, `top/ui_test.go`
- **Files to read:** `top/config_view.go`, `top/layout.go`

#### Task 7: Marker restoration after a UI rebuild
- **Description:** Re-emit `[PAUSED]` where the cmdline view is created, so the marker survives a
  return from the pager or editor even when no frame was ever stored. Tying it to the frame repaint
  would lose it in exactly that case.
- **Skill:** code-writing
- **Reviewers:** dev-code-reviewer, dev-test-reviewer
- **Verify:** bash — `go test ./top/...`
- **Files to modify:** `top/ui.go`, `top/ui_test.go`
- **Files to read:** `top/pause.go`

### Wave 4 (зависит от Wave 3)

#### Task 8: Help screen entry
- **Description:** Document the key and, as the user-spec requires, the set of actions that lift the
  pause — an unexplained lift reads as a defect. The built-in help is the only user-facing hotkey
  documentation in the project.
- **Skill:** code-writing
- **Reviewers:** dev-code-reviewer
- **Verify:** bash — `go build ./... && ./bin/pgcenter --help`
- **Files to modify:** `top/help.go`
- **Files to read:** `docs/features/016-feat-pause-display/016-feat-pause-display.md`

### Final Wave

#### Task 9: Pre-deploy QA
- **Description:** Acceptance testing: full test suite, lint, vulnerability scan, and the 15-step
  stand run from the user-spec, including the narrow-terminal pass and the comparison binary built
  from `master`.
- **Skill:** pre-deploy-qa
- **Reviewers:** none
