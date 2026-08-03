---
status: planned                    # planned -> in_progress -> done
depends_on: ["05"]                 # ID задач-зависимостей (строки: ["01", "02"])
wave: 4                            # волна параллельного выполнения
skills: [code-writing]             # МАССИВ скиллов для загрузки
verify: bash — `go test ./top/ -run 'Test_resizeDetector' -race`  # targeted: the full ./top/... run needs a live PG fixture cluster
reviewers: [dev-code-reviewer, dev-security-auditor, dev-test-reviewer]  # явно указать. Пусто = fallback на defaults
teammate_name:                     # имя агента-исполнителя (опционально; если не задано — генерируется по описанию задачи)
---

# Task 07: Repaint on terminal resize

## Required Skills

Перед выполнением задачи загрузи:
- `/skill:code-writing` — [skills/code-writing/SKILL.md](~/.claude/skills/code-writing/SKILL.md)

## Description

While the display is paused nothing renders except the repaint of the stored frame. If the terminal
is resized, the frozen text stays on screen but the visible-column window was computed for the old
width: after widening, columns that now fit are still hidden; after narrowing, the `‹`/`›` markers
lie. The user-spec makes this an acceptance criterion — after widening the operator must see more
columns, after narrowing fewer, and the edge arrows must disappear on the side where nothing is
hidden any more.

**The detector has exactly one possible home, and it is `layout`.** gocui delivers no resize event
to the application: `handleEvent` (`gocui@v0.5.0/gui.go:410-419`) dispatches only `EventKey`,
`EventMouse` and `EventError`; `termbox.EventResize` falls into `default: return nil`. The new size
becomes visible only through `flush()` (`gui.go:422-432`), which reads `termbox.Size()`, assigns
`g.maxX/g.maxY` and *then* calls the managers' `Layout`. So the `maxX, maxY := app.ui.Size()`
already present at `top/ui.go:144` is guaranteed to be the new size, and `layout` is the only place
that ever sees it. This is Risk 6 of the user-spec and Decision 5 of the tech-spec.

**The same detector doubles as the post-rebuild repaint.** `layout(app)` is re-invoked by `mainLoop`
on every UI rebuild (`top/ui.go:62`), so the closure's remembered size starts at zero and the first
layout of a new `Gui` counts as a change. That is what puts the frozen frame back on screen after a
return from the pager, the editor or `psql` — a separate user-spec criterion this task satisfies for
free, without a line of code of its own.

This task adds the detector and its wiring only. The repaint itself (`repaintStored`), the frame
store **and the failure latch of Decision 5** belong to Task 3 and already exist by Wave 4 —
Decision 6 assigns the latch to Task 3 explicitly and leaves this task only the resize trigger and
its test. This task **calls into** `top/pause.go` and must not modify it (Task 8 runs in the same
wave and the wave rule forbids overlapping file ownership; `pause.go` is owned by nobody in Wave 4).

## What to do

- Add **one** stateful detector to `top/ui.go` — a `resizeDetector` struct holding `lastX/lastY`
  with an `observe(paused bool, x, y int) bool` method, or an equivalent shape — that owns the
  whole "remember, compare, answer" decision **without touching gocui**. It must record the new
  size *before* answering `true`, and it must record the new size even when the pause is off. This
  is the seam that makes "a repaint is requested on a size change while paused, and not otherwise"
  testable without a terminal, in the same spirit as the `topBandLayout` precedent from [009]/[010]:
  the decision lives in a unit that a table test can drive, not buried inside `layout`.
- **One layer, not two.** An earlier draft of this task also asked for a separate pure
  `sizeChanged(lastX, lastY, x, y int) bool`. Do not add it: the comparison is one expression and
  `observe` is already fully testable, so a second named layer would be a wrapper with no caller of
  its own. The tech-spec's Testing Strategy bullet that names `sizeChanged` is satisfied by the
  merged unit's table test — record the naming deviation in the decisions log.
- Hold one detector instance in `layout`'s enclosing closure, in the same register as the existing
  `verboseTooShortShown` (`top/ui.go:141`) — per-`Gui` state that is deliberately reset on rebuild.
- Call it at the **end** of the layout closure, after the `extra` block and immediately before
  `return nil` (`top/ui.go:237`); when it answers `true`, call `repaintStored(app)`.
- Read `top/pause.go` first and use whatever entry point Task 3 actually published. Do not
  reimplement the repaint, do not read the frame store from `layout`, and do not extract anything
  out of the store before entering the update closure — Decision 1 makes the store
  gocui-goroutine-only for reads as well as writes.
- Add the covering test for the Decision 5 failure latch (one cmdline message per failure
  transition) in `top/ui_test.go`, driving the latch unit that Task 3 shipped in `top/pause.go`. A
  test file may exercise a function declared in another file of the same package, so this needs no
  edit to `top/pause.go`. See **Details → the failure latch** for what to do if Task 3 did not ship
  it as a separately callable unit.
- Extend `top/ui_test.go` (it exists, 365 lines, 9 top-level tests) — never overwrite it, never
  weaken an existing assertion.

## TDD Anchor

Write these first, watch them fail, then add the detector and the wiring.

- `top/ui_test.go::Test_resizeDetector_observe` — driven as a **sequence**, because the point is the
  state machine, not one call. Because this is now the only unit under test, the table must cover
  the plain arithmetic cases too — they no longer have a separate helper test of their own:
  - paused, the zero start state `(0, 0) → (190, 52)` → `true`. This case *is* the post-rebuild
    repaint, so it must be asserted explicitly, not arrive as an accident of the other rows;
  - the immediately following identical observation → `false` (the termination argument of
    Decision 5: one resize → one repaint → done);
  - paused, **width only** changed `(190, 52)` → `(60, 52)` → `true`, then `(60, 52)` again →
    `false`;
  - paused, **height only** changed `(190, 52)` → `(190, 24)` → `true`. Do not omit this row — the
    detector compares both dimensions and a height-only resize re-lays out the panel bands;
  - paused, **both** changed → `true`; and both a growth `(60, 24) → (190, 52)` and a shrink
    `(190, 52) → (60, 24)` → `true`;
  - **not** paused, `(190, 52)` → `(60, 52)` → `false` — no repaint is requested in live mode;
  - not paused `(190, 52)` → `(60, 52)`, then paused with the same `(60, 52)` → `false`: the size
    was recorded while live, so resuming the pause alone does not manufacture a repaint.
**The failure latch is NOT tested here.** Task 3 both ships the latch and owns its test
(`top/pause_test.go::Test_repaintFailureLatch_reportsOncePerTransition`, same fail/fail/fail/success/
fail sequence). Duplicating it in this task would be worse than redundant: a `-run` pattern matching
`Test_repaintFailureLatch` also matches Task 3's test by prefix, so this task's verification would go
green without a line being written. Rely on Task 3's coverage and keep this task's tests on the
resize seam only.

Not testable in this package and deliberately out of the anchor: `layout` itself. It calls
`app.ui.Size()`, and a `*gocui.Gui` cannot be constructed in a unit test — which is precisely why
the decision is extracted into the detector above. The wiring inside `layout` is covered by the
stand run in Task 10 (step 5 of the user-spec table: `tmux resize-window` 190 → 60 → 190).

## Acceptance Criteria

- [ ] The detector is a single unit in `top/ui.go` that touches no `app` and no `gocui` — it takes
      the pause flag and the two ints and answers a bool — and is table-tested, including the
      height-only change and the zero start state.
- [ ] No second comparison layer: `grep -n "func sizeChanged" top/ui.go` finds nothing.
- [ ] The detector's state lives in `layout`'s enclosing closure, so a UI rebuild resets it to zero
      and the first layout of a new `Gui` requests a repaint when paused.
- [ ] The size is recorded **before** the repaint is requested, and recorded on every observed
      change regardless of the pause flag.
- [ ] The detector call sits after all `SetView` calls and after the `extra` block, immediately
      before `layout`'s `return nil`; the `maxX == 0 || maxY == 0` guard still returns before it, so
      a post-pager zero size is never recorded and never triggers a repaint.
- [ ] While the pause is off, no repaint is ever requested from `layout`.
- [ ] `top/pause.go` is not modified by this task (`git diff --stat` shows only `top/ui.go` and
      `top/ui_test.go`).
- [ ] `layout` still returns `nil` on the success path; the detector adds no new error return.
- [ ] The repaint failure latch of Task 3 is covered by a test proving exactly one cmdline message
      per failure transition. No latch state is declared in `top/ui.go`.
- [ ] The targeted run `go test ./top/ -run 'Test_resizeDetector' -race`
      passes, as does a targeted re-run of the pre-existing `top/ui_test.go` tests, all unmodified;
      `make lint` is clean.

## Context Files

**Feature artifacts:**
- [016-feat-pause-display.md](016-feat-pause-display.md) — user-spec: Risk 6 (`:315-317`), the resize
  edge case (`:210`), the acceptance criterion (`:263`), stand step 5 (`:407`)
- [016-feat-pause-display-tech-spec.md](016-feat-pause-display-tech-spec.md) — **Decision 5** (the
  detector, the error-swallowing rule, the latch), **Decision 6** (the repaint path's error policy
  and the sentence assigning the failure latch to Task 3, leaving this task only the resize
  trigger), Decision 1 (store ownership), Decision 3 (why the five render-only keys need nothing
  here), Implementation Tasks → Wave 4 → Task 7
- [016-feat-pause-display-decisions.md](016-feat-pause-display-decisions.md) — decisions log; read
  the Task 3 and Task 5 entries for what `top/pause.go` actually ended up exporting, in particular
  the name and shape of the failure latch unit
- [016-feat-pause-display-code-research.md](016-feat-pause-display-code-research.md) — **§15**
  "Resize detector — design": §15.1 gocui swallows the event, §15.2 where it goes and what it
  stores, §15.3 why `Update` from inside `Layout` is safe, §15.4 why it cannot loop; §16.3 for why
  the detector still earns its place even though a paused frame is repainted once per interval;
  §7.6 for what survives a resize without a repaint

**Project knowledge:**
- [overview.md](../../../.claude/skills/project-knowledge/overview.md) — what pgcenter is (there is
  no `project.md` in this repo; `overview.md` is its equivalent)
- [architecture.md](../../../.claude/skills/project-knowledge/architecture.md) — package layout,
  data flow, "Horizontal Column Scroll" (the visible-column window a resize invalidates)
- [patterns.md](../../../.claude/skills/project-knowledge/patterns.md) — "Testable TUI Rendering —
  pure window function + io.Writer printers", "The cmdline: one composer, transient messages,
  persistent state", testing conventions

**Code files:**
- [top/ui.go](../../../top/ui.go) — `layout(app)` at `:138-239`: the closure at `:143` discards its
  `*gocui.Gui` parameter, `maxX, maxY := app.ui.Size()` at `:144`, the zero-geometry guard at
  `:148-150`, the `verboseTooShortShown` closure state at `:141` and its flip logic at `:211-218`,
  the `extra` block at `:221-235`, `return nil` at `:237`. Also `mainLoop`'s
  `SetManagerFunc(layout(app))` at `:62` (why the closure is recreated per `Gui`) and the
  error-swallowing precedent in the cmdline clear timer at `:477-495`
- [top/ui_test.go](../../../top/ui_test.go) — 365 lines, 9 existing top-level tests
  (`Test_composeCmdline`, `Test_composeCmdlineTwoTokens`, `Test_composeCmdlineRunes`,
  `Test_filterToken`, `Test_filterTokenDeterministicOrder`, `Test_filterTokenStripsControlRunes`,
  `Test_cmdlineTokens`, `Test_setCmdlineConfig`, `Test_printCmdlineNilGui`), testify `assert`,
  table-driven subtests via `t.Run`; add the new tests at the end, after `Test_printCmdlineNilGui`
- [top/layout.go](../../../top/layout.go) — `topBandLayout`: the precedent for a decision extracted
  out of `layout` into a unit a table test can drive, with its test in `top/layout_test.go`
- [top/pause.go](../../../top/pause.go) — **read only, do not modify**: `frameStore`,
  `repaintStored`, `togglePause`/`liftPause`, and the Decision 5 failure latch that Task 3 shipped
  here
- [top/config.go](../../../top/config.go) — `config.paused atomic.Bool` added by Task 2; read it
  with `app.config.paused.Load()`

## Verification Steps

- Run the targeted suite, with the race detector, since the two new tests are the deliverable:
  `go test ./top/ -run 'Test_resizeDetector' -race` — both pass, no races
  (the detector is closure-local and touches no shared state beyond the `atomic.Bool` load).
- Re-run the pre-existing `top/ui_test.go` tests to prove nothing regressed:
  `go test ./top/ -run 'Test_composeCmdline|Test_cmdlineTokens|Test_filterToken|Test_setCmdlineConfig|Test_printCmdlineNilGui'`
  — all pass, all unmodified.
- **Environment-dependent, not a gate for this task:** the full `go test ./top/... -race` needs the
  PostgreSQL fixture cluster on `127.0.0.1:21917` (`internal/postgres/testing.go`), which several
  `top/` tests connect to. Run it if the cluster is up and report the result; if it is not
  available, say so plainly rather than reporting a skipped run as a pass. The full suite is
  covered by the stand run in Task 10.
- Run `git diff --stat` — only `top/ui.go` and `top/ui_test.go` are changed.
- Run `git diff top/ui_test.go` — the diff only **adds** tests; no existing assertion relaxed or
  deleted.
- Run `grep -n "repaintStored" top/ui.go` — exactly one call, inside `layout`, after the `extra`
  block.
- Run `grep -n "func sizeChanged\|Latch\|latch" top/ui.go` — no separate comparison helper, no latch
  state declared in this file.
- Run `make lint` — clean.
- Read the final `layout` body and confirm by eye: the guard at `:148-150` is still the first thing
  after `Size()`, and the detector is the last thing before `return nil`.

## Details

**Files:**

- `top/ui.go` — two changes, both additive:
  1. The stateful detector. Suggested shape, adapt if you find something cleaner that keeps the same
     properties:
     - state: `lastX, lastY int`;
     - `observe(paused bool, x, y int) bool`: if the size did not change → `false`; otherwise record
       the new size **first**, then return `paused`.
     Doc comment must record the three facts a reader cannot recover from the code: gocui delivers
     no resize event so this is the only observation point; the state is per-`Gui` and its zero
     value is what repaints the frame after a rebuild; the record-before-answer order is what makes
     the repaint chain terminate.
  2. In `layout`: one `var` next to `verboseTooShortShown` (`:141`), and one `if` before
     `return nil` (`:237`) calling `repaintStored(app)`.
- `top/ui_test.go` — the two tests from the TDD Anchor, appended in the file's existing style:
  `t.Run` subtests, testify `assert`, a doc comment above each test explaining what property it
  pins (every existing test in that file has one).

**Dependencies:**
- Depends on Task 5 (Wave 3) only for wave ordering; the code it actually needs comes from Task 2
  (`config.paused`) and Task 3 (`repaintStored`, the frame store, the shared render core). Both are
  merged before this task starts.
- Wave partition: Task 8 owns `top/dialog.go` this wave. `top/pause.go` is owned by **nobody** in
  Wave 4 — read it, call into it, do not edit it. If you conclude an edit there is unavoidable,
  stop and report rather than edit.

**The failure latch (Decision 5) — read this before writing code.** Under pause the repaint is the
*only* drawing path, so a silent repaint failure leaves empty panels under a live `[PAUSED]` and
does **not** self-heal, because `v.Clear()` runs before the failing print. Hence: one cmdline
message on the failure transition, latched, exactly like `verboseTooShortShown`.

**The latch is not yours to build.** The failure is observable only *inside* the repaint closure,
which lives in `repaintStored` (`top/pause.go`), and Decision 6 assigns that closure, its error
policy and the latch to Task 3 — which also owns its test. Your job here is exactly one thing: wire
the resize trigger into the existing repaint path (the `repaintStored(app)` call). Do not add a latch
test of your own: Task 3's `Test_repaintFailureLatch_reportsOncePerTransition` already covers it, and
a same-prefix test here would make this task's `-run` pattern pass without any work being done.

**If Task 3 did not ship the latch as a callable unit — stop and report.** Do not edit
`top/pause.go`, and do not implement a latch of your own in `top/ui.go`: a latch declared here would
have no caller, since the only code that can observe a repaint failure lives in `pause.go`. That is
unreachable-except-from-its-own-test code, which is worse than the gap it papers over. Record the
gap in the decisions log and in your final report so the orchestrator can assign it back to the
owner of `pause.go`, and finish the rest of this task.

**Edge cases:**
- **Zero geometry after a pager/editor return.** `layout` returns `fmt.Errorf("")` at `:148-150`
  before the detector, so `(0, 0)` is never recorded as "seen" and never triggers a repaint into an
  invalid geometry. Do not move the detector above that guard, and do not add a zero check of your
  own — it would be dead code that hides the real guard.
- **No stored frame yet** (`Space` pressed before the first frame, or a rebuild in that state).
  `valid == false` is a normal state, not an error: the repaint draws nothing and returns without
  touching the latch. The `valid` check belongs **inside** the repaint closure (Decision 1) — do not
  test it from `layout`, that would read a gocui-goroutine field from the layout pass in a way the
  decision explicitly forbids replicating.
- **Live mode.** Size changes must still be recorded but must request nothing: gocui already redraws
  live frames every tick, and a repaint there would race the natural frame (the same reasoning that
  makes Decision 4 guard the dialog repaint on `paused`).
- **A resize while paused with the pause lifted between the layout pass and the repaint closure.**
  Harmless: the repaint renders the stored frame, the next collector frame overwrites it. No guard
  needed, and adding one would be a second read of `paused` on a different goroutine.
- **The verbose height-guard hint fires in the same layout pass.** Then two `g.Update` closures are
  queued in one pass and gocui does not specify their relative order (`gui.go:308-310`, defect class
  [028]). Harmless here: both compose the prefix through `cmdlineTokens(cmdlineCfg)`, so `[PAUSED]`
  renders whichever wins; only the transient message is at stake, and that is pre-existing debt.
  Do not try to order them.
- **Rapid resize (dragging a terminal edge).** Each `flush` produces one detector fire and one
  repaint request; `userEvents` is buffered at 20 and MainLoop drains it every iteration, so the
  queue cannot grow without bound. No debouncing — it would add a timer for a problem that does not
  exist.

**Implementation hints:**
- Use `app.ui.Update`, **not** `g.Update`, if you ever need the raw call: the layout closure discards
  its `*gocui.Gui` parameter (`return func(_ *gocui.Gui) error` at `:143`), so `g` is not in scope.
  In the expected shape you never need it — `repaintStored(app)` enters the update itself.
- `Update` from inside `Layout` is safe in `jroimartin/gocui@v0.5.0` — verified in the source, not
  assumed: it is three lines (`gui.go:311-313`), touches no `Gui` field, takes no lock (v0.5.0's
  `Gui` has no mutex) and hands the channel send to a new goroutine. No reentrancy into `flush`, no
  deadlock even on a full channel — the *spawned* goroutine parks, not MainLoop. The closure runs
  one event-loop iteration later, so the repaint lands in the next `flush`.
- **The repaint closure must return `nil` unconditionally.** An error escaping a `g.Update` closure
  propagates out of `MainLoop` (`gui.go:377-379`) → `mainLoop` stores `app.uiError` and rebuilds the
  `Gui` → `layout(app)` is recreated with a zeroed detector → the detector fires again → repeat,
  bounded only by `errorRate.check(1s, 5)` (`top/ui.go:92-95`), which kills the process with "too
  many UI errors". If Task 3 left an error path escaping that closure, stop and report it to the
  orchestrator as a Task 3 defect — do not paper over it in `layout` and do not fix it in
  `top/pause.go`, which this wave does not let you edit.
- **Do not render synchronously at the end of `Layout`.** It would draw in the same pass, but an
  error returned from `Layout` aborts `flush` and rebuilds the UI — the hazard the deferred
  `Update` plus the `nil` return exists to avoid.
- **Do not add a repaint call to `scrollLeft`/`scrollRight`/`increaseWidth`/`decreaseWidth`/
  `clearFilters`.** Decision 3: their existing `viewCh` push already produces a frame that the gate
  discards and repaints. Nothing about resize changes that.
- Keep the change surgical: no reformatting of `layout`'s existing blocks, no touching
  `topBandLayout`, `composeCmdline` or the cmdline writers.
- Naming: unexported, lowerCamelCase, consistent with `topBandLayout`/`visibleColumns`. Match the
  file's comment style — the existing comments explain *why*, not *what*.

## Reviewers

- **dev-code-reviewer** → `docs/features/016-feat-pause-display/016-feat-pause-display-task-07-dev-code-reviewer-review.json`
- **dev-security-auditor** → `docs/features/016-feat-pause-display/016-feat-pause-display-task-07-dev-security-auditor-review.json`
- **dev-test-reviewer** → `docs/features/016-feat-pause-display/016-feat-pause-display-task-07-dev-test-reviewer-review.json`

## Post-completion

- [ ] Записать краткий отчёт в [016-feat-pause-display-decisions.md](016-feat-pause-display-decisions.md) (Summary: 1-3 предложения, ревью со ссылками на JSON, без таблиц файндингов и дампов)
- [ ] Если отклонились от спека — описать отклонение и причину
- [ ] Обновить user-spec/tech-spec если что-то изменилось
