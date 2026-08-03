---
created: 2026-08-03
status: draft
branch: feature/016-feat-pause-display
size: M
---

# Tech Spec: Пауза обновления экрана по `Space`

## Solution

The pause is a **display gate plus a stored frame**, with exactly one cross-goroutine value: an
`atomic.Bool` on `top.config`. Everything else — the frame, its timestamp, the logtail buffer — is
touched **only on the gocui goroutine**, both for reading and for writing.

The collector is untouched. `doWork` keeps receiving from `statCh` unconditionally: `collectStat`
only reaches its `viewCh` receive *after* a successful `statCh` send (`top/stat.go:72-84`), and ~17
key handlers push on the unbuffered `viewCh`, so a gate that stopped receiving would park the
collector and the first keypress would hang `MainLoop` forever, with `Space` itself unable to
recover it.

Three mechanisms carry the feature:

1. **The gate** (`statLoop`, extracted from `doWork` so it is testable): while paused, receive the
   frame, discard it, and ask for a repaint of the store.
2. **The store**, published from inside the render closure — the only place where the frame and the
   logtail buffer, which is produced there (`top/stat.go:227,245`), can be written together on one
   goroutine without a lock.
3. **Lifting**, called explicitly by every handler that needs fresh data — after its early returns,
   so an action that changed nothing does not lift the pause.

Repainting mostly rides mechanisms that already exist. Five render-only keys (`[`, `]`, `↑`, `↓`,
`\`) push `viewCh` today "solely to trigger an immediate redraw" (`top/config_view.go:57-58`), which
makes the collector emit a frame promptly (`top/stat.go:124-133`); the gate's rule — *a discarded
frame repaints the store* — makes all five work unchanged. Only the filter dialog and terminal resize
need wiring of their own. The `[PAUSED]` marker after a UI rebuild needs **no** code at all: the
rebuild path always carries a non-nil `app.uiError`, whose cmdline write goes through the [015]
composer and redraws the token prefix (Decision 8).

No SQL, no new views, no `view.View` field, no recorded-format change. `record`, `report` and
`profile` are untouched.

## Architecture

### What we're building/modifying

- **`config.paused atomic.Bool` (`top/config.go`)** — the only cross-goroutine value: written by the
  `Space` handler and by every lifting handler (gocui goroutine), read by `statLoop` (worker).
- **`frameStore` (new, `top/pause.go`)** — the frozen frame, its render timestamp, the last non-empty
  logtail buffer and path, and a validity flag. **Owned exclusively by the gocui goroutine.**
- **`statLoop` (extracted from `doWork`, `top/ui.go`)** — the receive loop with the gate, lifted out
  so drain behaviour is unit-testable without a terminal or Postgres.
- **`renderFrame` (extracted from `printStat`, `top/stat.go`)** — the render core shared by the live
  path and the repaint path. The two differ only in their *error policy* and in whether the two
  frame-external side effects (the log-file read and the first-tick hint) run.
- **`togglePause`, `liftPause`, `repaintStored`, `pauseToken` (new, `top/pause.go`)** — the handler,
  the lifting helper, the repaint entry point and the cmdline token.
- **`printDataCell` (`top/stat.go`)** — becomes non-mutating.
- **`renderSysstat` (`top/stat.go`)** — takes the frame's timestamp instead of calling `time.Now()`.
- **`layout` (`top/ui.go`)** — gains a size-change detector.
- **`dialogFinish` (`top/dialog.go`)** — the filter branch repaints; the backend-kill dialogs show
  the frame's age while paused.
- **Lifting call sites** — `top/config_view.go`, `top/extra.go`, `top/verbose.go`.
- **`keybindings` / `helpTemplate`** — one row and one two-line entry.

### How it works

**Live (pause off).** Unchanged, plus: inside the render closure, after the panels are drawn, the
frame, the render timestamp and the current logtail buffer are stored. The store therefore always
holds exactly what is on screen.

**Pausing.** `Space` sets the flag and performs exactly one silent cmdline re-render, so `[PAUSED]`
appears immediately.

**Paused.** `statLoop` receives every frame, discards it, and requests a repaint of the store. The
collector never blocks and nothing overwrites the store, so the freeze is real rather than cosmetic.

**Interacting with the frozen frame.** `[`, `]`, `↑`, `↓`, `\` push `viewCh` as today; the collector
answers with a frame; the gate discards it and repaints the store, so the new scroll offset, column
width or filter set is applied to the frozen data. The filter dialog pushes nothing even today, so
its branch repaints directly.

**Lifting.** Handlers that need fresh data call `liftPause` after their early returns. `viewSwitchHandler`
covers every screen switch including the four screen menus; `menuConf` (`E`) does not go through it,
which is exactly right — it opens the editor, a UI-rebuild path where the pause survives.

**Resize.** `layout` compares the terminal size with the previous one it saw and, on a change while
paused, requests a repaint. After a UI rebuild its state is zero, so the first layout of the new
`Gui` counts as a change — which is also what repaints the frame on return from the pager.

**Pager / editor return.** The flag and the store live on `config`/`app` and survive the rebuild. The
marker comes back through the `uiError` cmdline write; the frame comes back through the resize
detector's zero state.

**Resuming.** `Space` clears the flag and re-renders the cmdline once. The next collector frame
renders normally; since the collector never stopped, deltas remain per-interval.

## Decisions

### Decision 1: The frame store is a gocui-goroutine-only object — for reads as well as writes

**Decision:** `frameStore` is written **and read** only on the gocui goroutine. It is written inside
`printStat`'s render closure; it is read inside the repaint closure. `statLoop` never touches it:
`repaintStored` takes no frame data out of the store before entering `g.Update`, it only *asks* for
a repaint.

**Rationale:** the logtail buffer is produced inside the render closure (`top/stat.go:227,245`) while
`stat.Stat` arrives on the worker goroutine (`top/ui.go:128`), so publishing from the closure is the
only arrangement that puts both halves in one place without a lock. The read half matters just as
much and was missed in the first draft: a gate that called `printStat(app, app.frame.stats, props)`
would read the frame on the worker goroutine while the closure publishes it. That window opens on
every `Space` press — the last live frame is still being published when the flag flips — and a torn
read (`Nrows` from one frame, `Values` from another) is a slice-bounds panic inside a `g.Update`
closure, which gocui does not recover. `-race` would not catch it either, because `statLoop` is
tested with stub render/repaint functions.

**Alternatives considered:** a `sync.Mutex` on `top.config` (rejected — the package's first lock, and
it still would not answer where the logtail buffer is captured); the store on `view.View` (rejected
by ADR [009]/[010]: it would ride `viewCh` and disturb `collectStat`'s load-bearing change-detection
ladder, `top/stat.go:86-122`).

### Decision 2: The gate discards and asks for a repaint; publication stays unconditional

**Decision:** `case s := <-statCh: if paused { repaintStored(app); continue }; printStat(...)`. The
publication inside the render closure is not guarded by `if !paused`.

**Rationale:** while paused, the only thing reaching the render closure is a repaint of the stored
frame, so the publication is an identity write **performed on the same goroutine that owns the
store** — which is what makes it safe, not the fact that the value is equal. A guard would add a
second place where "is it paused" must be answered consistently.

**Alternatives considered:** publishing from the gate (rejected — a write on the worker goroutine,
see Decision 1); conditional publication (rejected as above).

### Decision 3: Render-only keys ride their existing `viewCh` push

**Decision:** no repaint calls added to `scrollLeft`/`scrollRight`/`increaseWidth`/`decreaseWidth`/
`clearFilters`. The gate's rule covers all five. An anchor comment is added at
`top/config_view.go:57-58`, where the "push solely to force a redraw" trick is documented, recording
that the pause gate now depends on it.

**Rationale:** verified in code — a `viewCh` push makes `collectStat` fall through
(`top/stat.go:124-133`) into `c.Update` and a send, so a frame arrives promptly rather than at the
next tick. Five explicit repaint calls would be five places to keep in sync with the gate. The anchor
comment exists because this coupling is otherwise invisible to a future refactor and only the stand
run would catch its loss.

**Known property, not introduced here:** that push also runs `c.Reset()`, so the frame it produces
carries near-zero deltas, and the send on the unbuffered `viewCh` blocks `MainLoop` until the
collector receives it. Both are today's behaviour for these five keys in live mode; the pause makes
them more frequent but not different. On a dead connection this means a frozen screen can stop
responding until the collector's query times out — an existing property of every `viewCh`-pushing
key, recorded here so it is not mistaken for a pause defect.

### Decision 4: The filter dialog repaints explicitly instead of gaining a `viewCh` push

**Decision:** the `dialogFilter` branch of `dialogFinish` (`top/dialog.go:216-217`) calls
`repaintStored` when paused. `setFilter` keeps pushing nothing.

**Rationale:** the symmetric-looking fix — make `setFilter` push like its neighbours — would run the
collector-reset path (Decision 3) and produce a near-zero frame in **live** mode, where the filter is
used far more often than under pause. Note honestly that `clearFilters` (`\`) already behaves that
way today; that is an argument for not spreading the behaviour, not for matching it. A direct repaint
is also strictly cheaper: no collector round-trip at all.

**Alternatives considered:** the `viewCh` push (rejected above); repainting unconditionally rather
than only when paused (rejected — in live mode it would race the natural frame).

### Decision 5: The resize detector lives at the end of `layout`, swallows errors, and reports failure once

**Decision:** `layout` keeps the last seen `maxX/maxY` in its enclosing closure (the
`verboseTooShortShown` precedent, `top/ui.go:141`), compares at the end of the function — after every
`SetView` and after the zero-geometry guard — and, when the size changed and the pause is on,
requests a repaint via `app.ui.Update` (the layout closure discards its `*gocui.Gui` parameter,
`top/ui.go:143`). The repaint closure returns `nil` unconditionally. A repaint failure sets a latch
and emits **one** cmdline message on the transition, mirroring `verboseTooShortShown`.

**Rationale:** gocui delivers no resize event (`handleEvent`, `gui.go:410-419`, has no `EventResize`
case); `layout` is the only place that sees the new size. `Update` from inside `Layout` is safe in
`jroimartin/gocui@v0.5.0`: it touches no `Gui` state, takes no locks, and hands the send to its own
goroutine. The loop cannot run away because the detector's state is updated *before* the repaint is
requested, and a repaint does not change the terminal size. Error-swallowing is load-bearing — an
error out of a `g.Update` closure tears down `MainLoop` (`gui.go:377-379`) → rebuild → fresh closure
with a zeroed detector → repaint again, bounded only by the `errorRate` guard that kills the process.
The latch exists because under pause the repaint is the *only* drawing path: a silent failure leaves
empty panels under a live `[PAUSED]` with no explanation, and it does not self-heal (`v.Clear()` runs
before the failing print).

**Alternatives considered:** not repainting on resize (rejected by the user-spec — after widening the
terminal the operator expects more columns); a polling goroutine (rejected — a second timer for
something `layout` already observes).

### Decision 6: The repaint path is a shared render core, not `printStat` reused verbatim

**Decision:** extract the panel/table rendering out of `printStat` into `renderFrame`, called by both
paths. The live path keeps today's error propagation; the repaint path wraps the same core in a
closure that returns `nil` and drives the Decision 5 latch. The repaint path also skips the log-file
read (Decision 7) and the first-tick hint.

**Rationale:** the first draft said "reuse `printStat` verbatim" and separately "skip the two side
effects", which is a contradiction; worse, `printStat`'s closure returns errors at seven points
(`top/stat.go:172-204`), so a repaint through it re-opens the infinite-rebuild loop that Decision 5
closes at the outer level. One core with two error policies removes both problems.

**Alternatives considered:** a boolean parameter on `printStat` (rejected — the two paths differ in
error policy, which a flag inside one function expresses badly).

### Decision 7: The stored logtail buffer is the last NON-EMPTY read, and the file is not touched while paused

**Decision:** store `buf` and `path` whenever `printLogtail` actually has content; repaint renders
from the stored buffer and performs no file access, no `Reopen`, no size bookkeeping.

**Rationale:** `readLogfileRecent` returns `nil` when the file has not changed and `printLogtail`
then prints nothing (`top/stat.go:1230-1232`), so storing "the buffer of the frame on which `Space`
was pressed" would leave the panel empty on a quiet log — the defect the user-spec closed. Not
touching the file at all is what keeps the rotation machinery out of the paused state: `Logfile.Reopen`
closes the current file *before* querying the database for the new path, so a rotation detected
during a pause with a broken connection would leave a closed handle and a stale size, and the
resulting error, raised on every interval, would tear down `MainLoop` below the `errorRate`
threshold. Skipping the branch entirely avoids that class.

**Bookkeeping note (corrected):** `config.logtail.Size` is written in two places — `top/stat.go:243`
(the skipped branch) and `top/extra.go:46` (`= 0` when the panel is opened with `L`). Only the first
is affected by the pause; the second cannot run while paused because `L` lifts it.

**Residual, pre-existing and merely widened:** the descriptor is held for the duration of the pause,
so a rotated-away file keeps its inode pinned, and if the new file outgrows the frozen size before
resume, the rotation detector does not fire. Recorded in the risk table; not introduced by this
feature.

### Decision 8: `[PAUSED]` after a UI rebuild needs no code — but it needs a test

**Decision:** do not add a marker-restoration branch. Add a test that pins the behaviour instead.

**Rationale:** the first draft proposed an `else if` to the `uiError` write in `layout`'s
cmdline-creation branch, on the belief that nothing writes the cmdline on the pager path. That branch
would be **dead code**: `showPgLog`/`runPsql`/`printQueryReport` call `g.Close()` after signalling
`uiExit` (`top/pglog.go:32-33`, `psql.go:20-21`, `report.go:147-148`); the next `layout` sees a 0×0
terminal and returns an error (`top/ui.go:148-150`); `MainLoop` returns it and `app.uiError` is set
unconditionally (`top/ui.go:89`). The UI is only ever rebuilt through a non-nil `uiError`, so the
`uiError` branch always wins — and since it writes through `printCmdline`, the [015] composer redraws
the token prefix, marker included, even though the message itself is empty.

**Alternatives considered:** the `else if` (rejected as unreachable); an unconditional write in
`layout` (rejected — a second cmdline write on that path, defect class [027]).

### Decision 9: Lifting is explicit, placed after early returns, and refreshes the cmdline

**Decision:** a `liftPause(g, config)` helper clears the flag and performs exactly one cmdline
re-render; it is called by every handler that needs fresh data, **after** that handler's early
returns. Call sites, from the code-research classification:

- `viewSwitchHandler` (`top/config_view.go:315`) — covers every screen switch, whether reached by a
  letter key or by the `D`/`X`/`P`/`J` menus.
- `switchViewToProcPidStat` (`:329`) — after its remote-connection early return (`:341`).
- `orderKeyLeft`/`orderKeyRight`/`switchSortOrder`, `toggleSysTables`, `toggleIdleConns`,
  `changeQueryAge`.
- `toggleVerbose` (`top/verbose.go:14`), `showExtra` (`top/extra.go:11`) — after the four logtail
  early returns (`:37-58`).

Handlers that already write the cmdline themselves must not call the refreshing variant — the
"exactly one write per path" rule (`patterns.md`, defect class [027]) applies. Task decomposition
produces the precise map of which call site writes and which does not.

**Rationale:** three requirements meet here. Lifting must happen where the *decision to need fresh
data* is made, not in a wrapper, because `menuConf` (`E`) must be excluded — it opens the editor, a
UI-rebuild path where the pause survives, and it is the one menu branch that does not go through
`viewSwitchHandler` (`top/menu.go:204-209`). Lifting must be after early returns, because the
user-spec requires that an action which changed nothing (no `pg_stat_statements`, remote `S`,
unreadable log) does not lift the pause. And it must refresh the cmdline, because most of these
handlers write nothing there, so `[PAUSED]` would otherwise stay on screen over live data — Risk 5 of
the user-spec, realised.

**Alternatives considered:** lifting inside the `viewCh` send (rejected — the five render-only keys
push there too and must *not* lift); lifting in `statLoop` when a frame is rendered (rejected — the
flag would clear itself asynchronously, and the operator's intent would not be what cleared it).

### Decision 10: `printDataCell` becomes non-mutating instead of deep-copying the frame

**Decision:** truncate into a local instead of writing back into `s.Result.Values[rownum][i].String`
(`top/stat.go:1113`).

**Rationale:** that line is the only write into `s.Result.Values` in all of `top/`, so removing it
protects the stored frame from its own rendering *and* eliminates the aliasing write into the
collector's snapshot for `DiffIntvl=[0,0]` views (`activity`, the default screen), and removes the
need to deep-copy the frame at all. `Test_printStatData_truncation` (`top/stat_test.go:1269-1284`)
asserts rendered output, not structure, so it stays green.

**Alternatives considered:** deep-copying `Result.Values` at store time (rejected — larger, and it
leaves the underlying defect in place; the owner chose the causal fix explicitly).

### Decision 11: The frame timestamp is taken in `top/`, not added to `stat.Stat`

**Decision:** the render path captures `time.Now()` alongside the frame; `renderSysstat` takes it as
a parameter instead of calling `time.Now()` itself (`top/stat.go:277`).

**Rationale:** `stat.Stat` carries no collection time and the user-spec confines the change to
`top/`. The gap between "collected" and "rendered" is bounded by one refresh interval and is
invisible on screen. Five test call sites need the new argument (`top/stat_test.go:60, 97, 192, 440,
441`; `:192` is a shared helper covering eight tests).

**Alternatives considered:** adding a timestamp to `stat.Stat` in `internal/stat` (rejected — it
widens the change beyond `top/` for a value only the renderer needs).

### Decision 12: `statLoop` is extracted for testability; the guarded-send fix goes with it

**Decision:** lift the select loop out of `doWork` into `statLoop(ctx, uiExit, statCh, paused,
render, repaint)`, returning its exit reason. In the same task, guard the currently bare send at
`top/stat.go:130` with a `select` on `ctx.Done()`. Do **not** add `wg.Wait()` to the `uiExit` branch.

**Rationale:** the loop is the only concurrency-sensitive code in the feature and is untestable
inside `doWork`. The bare send is a pre-existing hang: `wg.Wait()` on the `ctx.Done()` branch
(`top/ui.go:130-132`) waits for a collector that may be parked on that send forever, and this path
runs on **every** return from the pager (`mainLoop` cancels, then waits). The pause raises its
reachability, because the five render-only keys make the collector's re-initialisation branch a
routine event while paused. Five lines in a file this task edits anyway.
The `uiExit` branch needs no `wg.Wait()` — `mainLoop` already cancels and waits (`top/ui.go:99-102`).
(The first draft justified this by claiming the wait would hang forever; that was wrong, since
`cancel()` wakes the collector. The conclusion stands for the simpler reason that the wait is
redundant.)

**Alternatives considered:** testing `doWork` as-is (rejected — needs a live `Gui`); leaving the bare
send (rejected — the feature makes an existing hang materially more reachable).

### Decision 13: `Space` binds `gocui.KeySpace`, scoped to `"sysstat"`

**Decision:** one row: `{"sysstat", gocui.KeySpace, togglePause(app)}`.

**Rationale:** termbox classifies bytes `<= 0x20` as functional keys and emits `Ch = 0, Key =
KeySpace` (`termbox.go:575-577`), while gocui matches on `key && ch && mod` — a rune binding (`' '`)
would never fire. Scoping to `"sysstat"` keeps a space typed into a dialog a plain space. The
keybinding table has **no test coverage at all**, so a wrong constant is caught only on the stand.

**Alternatives considered:** a global binding (rejected — it would make typing a space into any
dialog impossible).

### Decision 14: The kill dialogs show the frame's age while paused

**Decision:** while the pause is active, the backend-cancel and backend-terminate dialogs (`-`, `_`,
`k`, `K`) include the age of the frozen frame in their prompt, e.g. `(frame is 4m12s old)`.

**Rationale:** the user-spec deliberately keeps the pause across these actions, and their argument is
a PID the operator reads **off a deliberately stale screen**; the stand scenario itself pauses for
three minutes before acting. The frozen clock gives an absolute time but not an age, and the value is
already stored (`frameStore.at`), so this is one string on data the feature already has. This is the
one place where a stale read has an irreversible consequence.

**Autopilot decision:** the user-spec does not ask for this. It is added under the autopilot mandate
as a safety consequence of a decision the spec *did* make (keeping the pause for DB actions), and is
recorded here so it is reconciled into the spec's post-implementation section rather than looking
like scope creep.

**Alternatives considered:** lifting the pause for kill dialogs (rejected — contradicts an approved
user-spec decision); showing nothing (rejected — the risk is real and the fix is one line).

## Data Models

```go
// top/pause.go — owned by the gocui goroutine, read and written only there.
type frameStore struct {
    stats   stat.Stat  // the frozen frame, as received
    at      time.Time  // when it was rendered — freezes the header clock, feeds the kill-dialog age
    logBuf  []byte     // last NON-EMPTY logtail read (nil when the panel was closed)
    logPath string     // the path that buffer came from
    valid   bool       // false until the first frame is rendered
}
```

`config` gains one field:

```go
paused atomic.Bool  // written by Space and by every lifting handler (gocui), read by statLoop (worker)
```

**Deliberately unbounded, do not add a limit or a copy.** The store holds exactly one frame and one
logtail buffer. The buffer is bounded by terminal geometry. The frame is bounded by the query result,
which on a `pg_stat_statements` screen with thousands of rows can reach tens of megabytes — held for
the duration of the pause instead of one tick. That is a constant, not a leak, and after Decision 10
it is a *retention*, not a duplication: no deep copy is taken.

No database objects, no serialized format, no `view.View` change.

## Dependencies

### New packages

None. `sync/atomic` is already used in `top/` (`uiGeneration`, `top/ui.go:29`).

### Using existing (from project)

- The [015] cmdline composer — `[PAUSED]` is added as data; `composeCmdline`, `renderCmdlineTokens`
  and the degradation ladder are untouched. The composer's doc comment (`top/ui.go:241-244`) already
  names `[PAUSED]` as its motivating example.
- `topBandLayout` (`top/layout.go:41`) and `visibleColumns` ([009]) — untouched; the repaint
  recomputes the window for the current width.

## Testing Strategy

**Feature size:** M

### Unit tests

- `statLoop` drains while paused: a fake `statCh` and a counter prove the sender completes **≥ 5**
  sends with the flag set, that `render` is never called and `repaint` always is; plus exit-reason
  cases for `uiExit` and `ctx.Done()`.
- `pauseToken` / `cmdlineTokens`: `[PAUSED]` present only when paused, left of the filter token,
  single variant, never degraded — asserted through `composeCmdline` against a `bytes.Buffer`.
- Marker after a UI rebuild: a `uiError`-shaped cmdline write reproduces the prefix with the marker
  (pins Decision 8, which ships no code).
- `printDataCell` is non-mutating: the source value is unchanged after rendering a truncated cell and
  the output bytes are identical to today's.
- `renderSysstat` prints the passed timestamp, not the wall clock.
- Store discipline: publish → repaint renders identical bytes; a second, different frame arriving
  while paused does not change what a repaint renders.
- Logtail: an empty read does not overwrite the stored buffer; a repaint renders the stored buffer
  and performs no file access.
- Lifting: table-driven over the classified handlers — each lifts, each no-op early return does not,
  `menuConf` does not.
- `sizeChanged(lastX, lastY, x, y)` — a pure helper, table-tested, so the arithmetic is not buried in
  `layout`.
- Kill-dialog prompt includes the frame age while paused and is unchanged when live.
- Errors while paused: a frame carrying an error is discarded like any other and surfaces on the
  first frame after resume.

### Integration tests

None. The feature adds no query and touches no database surface — confirmed in the user-spec.

### E2E tests

None. There is no automated end-to-end harness for the TUI; its role is played by the stand run in
the Final Wave.

## Agent Verification Plan

**Source:** user-spec "Как проверить" — 15 numbered steps, reproduced there in full.

### Verification approach

`make test` (with `-race`), `make lint`, `make vuln` for the automated half; a tmux stand run per
`patterns.md` for everything that only exists on a live terminal — fresh `make build`, the binary
shipped and invoked by explicit path, fixed geometry (`-x 190 -y 52` plus a narrow `-x 60` pass), and
a second binary built from `master` to separate regressions from pre-existing behaviour. The stand
address is requested from the owner at the start of the run; it is deliberately not stored in the
repository.

### Per-task verification

| Task | verify: | What to check |
|------|---------|--------------|
| 1 | bash | `go test ./top/...` — truncation output unchanged, source value unmutated |
| 2 | bash | `go test ./top/...` — token present/absent, position, no degradation |
| 3 | bash | `go test ./top/... -race` — ≥5 collector sends complete while paused; render never called while paused |
| 4 | bash | `go test ./top/...` — clock renders the stored stamp |
| 5 | bash | `go test ./top/...` — every classified handler lifts; no-op paths and `menuConf` do not |
| 6 | bash | `go test ./top/...` — help text contains the `Space` entry and the lifting set |
| 7 | bash | `go test ./top/...` — resize helper table; filter repaint invoked when paused; kill prompt carries the frame age |
| 8 | bash | `go test ./top/...` — empty read keeps the buffer; repaint touches no file |
| 9 | bash | full stand run: all 15 user-spec steps |

### Tools required

`bash`, `make`, `tmux` over ssh on the stand. No MCP tooling, no browser automation.

## Backward Compatibility

**Breaking changes:** no.

`renderSysstat`, `printDataCell` and `printStat` are unexported functions inside `top/`; their
signature changes are contained in the package. No exported API, no CLI flag, no config file, no
recorded-format field changes, so archives written by earlier versions replay identically and
`record`/`report`/`profile` are unaffected.

**Migration strategy:** N/A — nothing persisted changes.

**DB migration compatibility:** N/A — no database objects.

**Consumer impact:** none found — `grep` shows `renderSysstat`, `printDataCell` and `printStat` are
called only from `top/` and its tests.

## Risks

| Risk | Mitigation |
|------|-----------|
| A gate that stops receiving hangs the UI unrecoverably | Drain-and-discard is the specified shape; `statLoop` is extracted so a test proves the collector keeps completing sends |
| A torn read of the store panics inside a `g.Update` closure | Decision 1 — the store is gocui-goroutine-only for reads as well as writes; `repaintStored` extracts nothing before entering the closure |
| An error escaping a repaint closure tears down `MainLoop` → rebuild → repaint → repeat | Decisions 5 and 6 — the repaint path has its own error policy, returns `nil`, and reports failure once through a latch |
| A render-only key silently does nothing while paused | Decision 3 covers five keys via the existing push, Decision 4 the sixth; the stand run exercises each by name |
| The marker stays on screen after the pause is lifted by a handler that writes no cmdline | Decision 9 — `liftPause` performs the refresh itself |
| A no-op handler lifts the pause anyway | Decision 9 — lifting is placed after early returns; a table test covers the three known no-op paths |
| `wg.Wait()` on shutdown waits for a collector parked on a bare send | Decision 12 — the send is guarded in the same task |
| Wrong key constant is invisible to tests | The keybinding table has no coverage; the stand run is the only guard and it is a named acceptance step |
| The store is silently replaced, making the freeze cosmetic | Publication happens only on the render path; while paused the only render is the identity repaint, and stand step 3a checks values after ≥3 minutes |
| A held log descriptor pins a rotated-away inode; a grown new file defeats the rotation detector | Pre-existing behaviour, widened by the pause; documented in Decision 7, not introduced here |
| A crafted row value now persists on a frozen screen and across a pager return (debt [029]) | Out of scope by the user-spec; the persistence consequence is recorded into debt [029] as part of Task 9 |

## Acceptance Criteria

- [ ] `make test` passes with `-race`; no new races reported.
- [ ] `make lint` and `make vuln` clean.
- [ ] The `statLoop` test proves ≥5 completed collector sends while paused.
- [ ] No production code outside `top/` is modified.
- [ ] `view.View` gains no field; `record`/`report`/`profile` untouched.
- [ ] Every existing `top/` test still passes; the five `renderSysstat` call sites updated
      mechanically, no assertion weakened.
- [ ] Tech debt [029] is updated with the two new persistence consequences (frozen frame, logtail
      buffer) — not fixed, recorded.
- [ ] All user-spec acceptance criteria verified: automated ones by tests, terminal ones by the stand
      run.

## Implementation Tasks

**Wave rule (carried over from [015]):** no two tasks in the same wave modify the same file. The
partition below is checked against that rule.

| Wave | Tasks | Files |
|------|-------|-------|
| 1 | 1 | `top/stat.go`, `top/stat_test.go` |
| 1 | 2 | `top/config.go`, `top/keybindings.go`, `top/pause.go`, `top/pause_test.go`, `top/ui.go`, `top/ui_test.go` |
| 2 | 3 | `top/ui.go`, `top/stat.go`, `top/pause.go`, `top/ui_test.go`, `top/pause_test.go` |
| 3 | 4 | `top/stat.go`, `top/stat_test.go` |
| 3 | 5 | `top/config_view.go`, `top/extra.go`, `top/verbose.go`, `top/config_view_test.go` |
| 3 | 6 | `top/help.go`, `top/help_test.go` |
| 4 | 7 | `top/ui.go`, `top/dialog.go`, `top/pause.go`, `top/ui_test.go` |
| 5 | 8 | `top/stat.go`, `top/pause.go`, `top/stat_test.go` |

Wave 1: task 1 touches `stat.go`, task 2 does not — no overlap. Wave 3: `stat.go` (4), the handler
files (5) and `help.go` (6) are disjoint. Tasks 7 and 8 are serialised into their own waves because
both need `pause.go` and one of `ui.go`/`stat.go`.

### Wave 1 (независимые)

#### Task 1: Non-mutating data cell rendering
- **Description:** Make the data-cell printer truncate into a local instead of writing back into the
  result set. This protects any stored frame from being destroyed by its own rendering and removes an
  aliasing write into the collector's snapshot that exists today on screens without delta computation.
- **Skill:** code-writing
- **Reviewers:** dev-code-reviewer, dev-security-auditor, dev-test-reviewer
- **Verify:** bash — `go test ./top/...`
- **Files to modify:** `top/stat.go`, `top/stat_test.go`
- **Files to read:** `docs/features/016-feat-pause-display/016-feat-pause-display-code-research.md`

#### Task 2: Pause flag, `Space` binding and the `[PAUSED]` token
- **Description:** Add the pause flag to the display config, bind `Space` in the stats context, and
  add the marker to the cmdline composer as a single-variant token positioned left of the filter
  indicator. The marker must appear the moment the pause is engaged.
- **Skill:** code-writing
- **Reviewers:** dev-code-reviewer, dev-security-auditor, dev-test-reviewer
- **Verify:** bash — `go test ./top/...`
- **Files to modify:** `top/config.go`, `top/keybindings.go`, `top/pause.go`, `top/pause_test.go`,
  `top/ui.go`, `top/ui_test.go`
- **Files to read:** `top/verbose.go`, `docs/decisions-log.md`

### Wave 2 (зависит от Wave 1)

#### Task 3: The gate, the frame store and the shared render core
- **Description:** Extract the receive loop into a testable function, add the discard-and-repaint
  gate, split the render core so the live and repaint paths can differ in error policy, and publish
  the frame into a store owned solely by the UI goroutine. Includes guarding the collector's one
  unguarded send, which this feature makes materially more reachable.
- **Skill:** code-writing
- **Reviewers:** dev-code-reviewer, dev-security-auditor, dev-test-reviewer
- **Verify:** bash — `go test ./top/... -race`
- **Files to modify:** `top/ui.go`, `top/stat.go`, `top/pause.go`, `top/ui_test.go`,
  `top/pause_test.go`
- **Files to read:** `internal/stat/stat.go`,
  `docs/features/016-feat-pause-display/016-feat-pause-display-code-research.md`

### Wave 3 (зависит от Wave 2)

#### Task 4: Frozen header clock
- **Description:** Thread the frame's render timestamp into the summary-panel renderer so a repainted
  frame shows the time of the data on screen rather than the current wall clock. Without it the
  frozen frame carries a live clock — the incoherence the feature exists to avoid.
- **Skill:** code-writing
- **Reviewers:** dev-code-reviewer, dev-security-auditor, dev-test-reviewer
- **Verify:** bash — `go test ./top/...`
- **Files to modify:** `top/stat.go`, `top/stat_test.go`
- **Files to read:** `top/pause.go`

#### Task 5: Lifting the pause in handlers that need fresh data
- **Description:** Every action whose effect is produced by the collector must lift the pause, and it
  must do so after its own early returns so an action that changed nothing leaves the freeze intact.
  The config-menu path is deliberately excluded because it opens an editor, where the pause survives.
- **Skill:** code-writing
- **Reviewers:** dev-code-reviewer, dev-security-auditor, dev-test-reviewer
- **Verify:** bash — `go test ./top/...`
- **Files to modify:** `top/config_view.go`, `top/extra.go`, `top/verbose.go`,
  `top/config_view_test.go`
- **Files to read:** `top/menu.go`, `top/pause.go`,
  `docs/features/016-feat-pause-display/016-feat-pause-display-code-research.md`

#### Task 6: Help screen entry
- **Description:** Document the key and the set of actions that lift the pause, which the user-spec
  makes a user-visible requirement — an unexplained lift reads as a defect. The built-in help is the
  only user-facing hotkey documentation in the project.
- **Skill:** code-writing
- **Reviewers:** dev-code-reviewer, dev-test-reviewer
- **Verify:** bash — `go test ./top/...`
- **Files to modify:** `top/help.go`, `top/help_test.go`
- **Files to read:** `docs/features/016-feat-pause-display/016-feat-pause-display.md`

*Reviewer note: Task 6 edits a display-only string constant with no input handling, so
`dev-security-auditor` is omitted from the catalog default. Do not restore it during decomposition.*

### Wave 4 (зависит от Wave 3)

#### Task 7: Resize repaint, filter-dialog repaint and the frame-age prompt
- **Description:** Detect terminal size changes in the layout callback so a paused frame is re-laid
  out for the new width, repaint the stored frame when a filter is applied through its dialog (that
  path triggers no redraw of its own even today), and show the frozen frame's age in the
  backend-kill prompts, where acting on stale data is irreversible.
- **Skill:** code-writing
- **Reviewers:** dev-code-reviewer, dev-security-auditor, dev-test-reviewer
- **Verify:** bash — `go test ./top/...`
- **Files to modify:** `top/ui.go`, `top/dialog.go`, `top/pause.go`, `top/ui_test.go`
- **Files to read:** `top/config_view.go`, `top/layout.go`, `top/signal.go`

### Wave 5 (зависит от Wave 4)

#### Task 8: Logtail panel freeze and restore
- **Description:** Stop reading the log file while paused and render the panel from the last
  non-empty buffer stored with the frame. Without storing that buffer the panel comes back empty
  after a UI rebuild, since nothing else in the application holds those lines.
- **Skill:** code-writing
- **Reviewers:** dev-code-reviewer, dev-security-auditor, dev-test-reviewer
- **Verify:** bash — `go test ./top/...`
- **Files to modify:** `top/stat.go`, `top/pause.go`, `top/stat_test.go`
- **Files to read:** `top/extra.go`,
  `docs/features/016-feat-pause-display/016-feat-pause-display-code-research.md`

### Final Wave

#### Task 9: Pre-deploy QA
- **Description:** Acceptance testing: full suite with the race detector, lint, vulnerability scan,
  and the 15-step stand run from the user-spec including the narrow-terminal pass and the comparison
  binary built from `master`. Also records the two new persistence consequences into tech debt [029].
- **Skill:** pre-deploy-qa
- **Reviewers:** none
- **Verify:** bash — `make test`, `make lint`, `make vuln`, plus the stand run
- **Files to read:** `docs/features/016-feat-pause-display/016-feat-pause-display.md`,
  `.claude/skills/project-knowledge/patterns.md`, `docs/tech-debt.md`
