---
status: planned                    # planned -> in_progress -> done
depends_on: ["05"]                 # ID задач-зависимостей (строки: ["01", "02"])
wave: 4                            # волна параллельного выполнения
skills: [code-writing]             # МАССИВ скиллов для загрузки
verify: bash — `go test ./top/...` # инструмент верификации (опционально: curl, bash, user)
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

This task adds the detector and its wiring only. The repaint itself (`repaintStored`) and the frame
store belong to Task 3 and already exist by Wave 4; this task **calls into** `top/pause.go` and must
not modify it (Task 8 runs in the same wave and the wave rule forbids overlapping file ownership;
`pause.go` is owned by nobody in Wave 4).

## What to do

- Add a pure comparison helper to `top/ui.go` — `sizeChanged(lastX, lastY, x, y int) bool` — so the
  arithmetic is table-testable instead of buried in `layout`. This follows the `topBandLayout`
  precedent from [009]/[010]: pure integer function in the file, table test next to it.
- Add a tiny stateful detector next to it (a `resizeDetector` struct with `lastX/lastY` and an
  `observe(paused bool, x, y int) bool` method, or an equivalent shape) that owns the "remember,
  compare, answer" decision **without touching gocui**. It must record the new size *before*
  answering `true`, and it must record the new size even when the pause is off. This is the seam
  that makes "a repaint is requested on a size change while paused, and not otherwise" testable
  without a terminal.
- Hold one detector instance in `layout`'s enclosing closure, in the same register as the existing
  `verboseTooShortShown` (`top/ui.go:141`) — per-`Gui` state that is deliberately reset on rebuild.
- Call it at the **end** of the layout closure, after the `extra` block and immediately before
  `return nil` (`top/ui.go:237`); when it answers `true`, call `repaintStored(app)`.
- Read `top/pause.go` first and use whatever entry point Task 3 actually published. Do not
  reimplement the repaint, do not read the frame store from `layout`, and do not extract anything
  out of the store before entering the update closure — Decision 1 makes the store
  gocui-goroutine-only for reads as well as writes.
- Verify that the repaint failure latch of Decision 5 (one cmdline message per failure transition)
  exists and is covered; add the covering table test in `top/ui_test.go`. See **Details → the
  failure latch** for what to do if Task 3 did not ship it.
- Extend `top/ui_test.go` (it exists, 365 lines, 10 tests) — never overwrite it, never weaken an
  existing assertion.

## TDD Anchor

Write these first, watch them fail, then add the helper, the detector and the wiring.

- `top/ui_test.go::Test_sizeChanged` — table over the pure helper: identical size → `false`; width
  only changed → `true`; height only changed → `true`; both changed → `true`; growth and shrink
  both → `true`; the zero start state `(0, 0) → (190, 52)` → `true` (this case *is* the post-rebuild
  repaint, so it must be asserted explicitly, not as an accident).
- `top/ui_test.go::Test_resizeDetector_observe` — driven as a **sequence**, because the point is the
  state machine, not one call:
  - paused, first observation `(190, 52)` → `true`; the immediately following identical observation
    → `false` (this is the termination argument of Decision 5: one resize → one repaint → done);
  - paused, `(190, 52)` → `(60, 52)` → `true`, then `(60, 52)` again → `false`;
  - **not** paused, `(190, 52)` → `(60, 52)` → `false` — no repaint is requested in live mode;
  - not paused `(190, 52)` → `(60, 52)`, then paused with the same `(60, 52)` → `false`: the size
    was recorded while live, so resuming the pause alone does not manufacture a repaint.
- `top/ui_test.go::Test_repaintFailureLatch` — the Decision 5 latch: a first failure reports (one
  message), a second consecutive failure does not, a success clears the latch, and a failure after
  that success reports again. Exactly one message per off→on transition, mirroring
  `verboseTooShortShown` (`top/ui.go:211-218`). Drive the latch's own state function; do not try to
  reach it through `printCmdline`, which needs a real `gocui.View`.

Not testable in this package and deliberately out of the anchor: `layout` itself. It calls
`app.ui.Size()`, and a `*gocui.Gui` cannot be constructed in a unit test — which is precisely why
the decision is extracted into the two helpers above. The wiring inside `layout` is covered by the
stand run in Task 10 (step 5 of the user-spec table: `tmux resize-window` 190 → 60 → 190).

## Acceptance Criteria

- [ ] `sizeChanged` is a pure function of four ints in `top/ui.go` — no `app`, no `gocui`, no
      package state — and is table-tested.
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
- [ ] The repaint failure latch emits exactly one cmdline message per failure transition, and this
      is covered by a test.
- [ ] `go test ./top/...` passes, including every pre-existing `top/ui_test.go` test unmodified;
      `make lint` is clean.

## Context Files

**Feature artifacts:**
- [016-feat-pause-display.md](016-feat-pause-display.md) — user-spec: Risk 6 (`:315-317`), the resize
  edge case (`:210`), the acceptance criterion (`:263`), stand step 5 (`:407`)
- [016-feat-pause-display-tech-spec.md](016-feat-pause-display-tech-spec.md) — **Decision 5** (the
  detector, the error-swallowing rule, the latch), Decision 1 (store ownership), Decision 3 (why the
  five render-only keys need nothing here), Decision 6 (the repaint path's error policy),
  Implementation Tasks → Wave 4 → Task 7
- [016-feat-pause-display-decisions.md](016-feat-pause-display-decisions.md) — decisions log; read
  the Task 3 and Task 5 entries for what `top/pause.go` actually ended up exporting
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
- [top/ui_test.go](../../../top/ui_test.go) — existing 10 tests, testify `assert`, table-driven
  subtests via `t.Run`; add the new tests at the end, next to `Test_printCmdlineNilGui`
- [top/layout.go](../../../top/layout.go) — `topBandLayout`: the precedent for a pure integer helper
  extracted out of `layout`, with its table test in `top/layout_test.go`
- [top/pause.go](../../../top/pause.go) — **read only, do not modify**: `frameStore`,
  `repaintStored`, `togglePause`/`liftPause`, and whether the failure latch lives there
- [top/config.go](../../../top/config.go) — `config.paused atomic.Bool` added by Task 2; read it
  with `app.config.paused.Load()`

## Verification Steps

- Run `go test ./top/...` — all tests pass, including the three new ones and every pre-existing
  `top/ui_test.go` test in its original form.
- Run `go test ./top/... -race` — no new races (the detector is closure-local and touches no shared
  state beyond the `atomic.Bool` load).
- Run `git diff --stat` — only `top/ui.go` and `top/ui_test.go` are changed.
- Run `git diff top/ui_test.go` — the diff only **adds** tests; no existing assertion relaxed or
  deleted.
- Run `grep -n "repaintStored" top/ui.go` — exactly one call, inside `layout`, after the `extra`
  block.
- Run `make lint` — clean.
- Read the final `layout` body and confirm by eye: the guard at `:148-150` is still the first thing
  after `Size()`, and the detector is the last thing before `return nil`.

## Details

**Files:**

- `top/ui.go` — three changes, all additive:
  1. `sizeChanged(lastX, lastY, x, y int) bool` — pure, one comparison, with a doc comment saying
     why it is a named function rather than an inline condition (table-testability; the
     `topBandLayout` precedent).
  2. The stateful detector next to it. Suggested shape, adapt if you find something cleaner that
     keeps the same properties:
     - state: `lastX, lastY int`;
     - `observe(paused bool, x, y int) bool`: if the size did not change → `false`; otherwise record
       the new size **first**, then return `paused`.
     Doc comment must record the three facts a reader cannot recover from the code: gocui delivers
     no resize event so this is the only observation point; the state is per-`Gui` and its zero
     value is what repaints the frame after a rebuild; the record-before-answer order is what makes
     the repaint chain terminate.
  3. In `layout`: one `var` next to `verboseTooShortShown` (`:141`), and one `if` before
     `return nil` (`:237`) calling `repaintStored(app)`.
- `top/ui_test.go` — the three tests from the TDD Anchor, appended in the file's existing style:
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

The failure is observable only *inside* the repaint closure, which lives in `repaintStored`
(`top/pause.go`) — Decision 6 assigns that closure and its error policy to Task 3. So:

- **Expected case** — Task 3 shipped the latch. Then this task adds no latch code. Its job is to
  confirm the behaviour is actually covered and, if it is not, add `Test_repaintFailureLatch` to
  `top/ui_test.go`. A test file may exercise a function declared in another file of the same
  package, so this needs no edit to `top/pause.go`.
- **Gap case** — Task 3 did not ship it. Do **not** edit `top/pause.go` to add it. Implement the
  latch as a pure state helper in `top/ui.go` (e.g. `failureLatch.report(failed bool) bool`
  returning whether to emit), cover it with `Test_repaintFailureLatch`, leave the wiring as a
  one-line change for the owner of `pause.go`, and record the gap explicitly in the decisions log
  and in your final report so the orchestrator can assign it. Do not silently drop the requirement.

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
  many UI errors". If Task 3 left an error path escaping that closure, treat it as the gap case
  above and report it — do not paper over it in `layout`.
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
