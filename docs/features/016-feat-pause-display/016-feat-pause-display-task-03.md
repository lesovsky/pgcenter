---
status: planned                    # planned -> in_progress -> done
depends_on: ["01", "02"]           # ID задач-зависимостей (строки: ["01", "02"])
wave: 2                            # волна параллельного выполнения
skills: [code-writing]             # МАССИВ скиллов для загрузки
verify: bash — `go test ./top/ -run 'statLoop|frameStore|repaint|renderFrame|Latch' -race -count=5` # инструмент верификации (опционально: curl, bash, user)
reviewers: [dev-code-reviewer, dev-security-auditor, dev-test-reviewer]  # явно указать. Пусто = fallback на defaults
teammate_name:                     # имя агента-исполнителя (опционально; если не задано — генерируется по описанию задачи)
---

# Task 03: The gate, the frame store and the shared render core

## Required Skills

Перед выполнением задачи загрузи:
- `/skill:code-writing` — [skills/code-writing/SKILL.md](~/.claude/skills/code-writing/SKILL.md)

## Description

This is the core of the pause feature and its **only concurrency-sensitive part**. Everything else in
the feature (marker, help entry, lifting helpers, resize detector, dialog repaint) is single-goroutine
plumbing built on top of what this task establishes. Four things ship together, because they are one
design:

1. **The gate.** Today `doWork` (`top/ui.go:106-135`) receives a frame and renders it. Under pause the
   frame must be **received, discarded, and answered with a repaint of the store**. The receive is not
   optional: `collectStat` reaches its `viewCh` receive only *after* a successful `statCh` send
   (`top/stat.go:72-84`), and ~17 key handlers push on the unbuffered `viewCh`. A gate that stopped
   receiving would park the collector, the first keypress would block `MainLoop` forever, and `Space`
   itself could not recover it. Drain-and-discard is the only non-hanging shape.

2. **A testable loop.** The select loop is lifted out of `doWork` into `statLoop`, because that loop is
   the only code in the feature where a mistake is a deadlock rather than a cosmetic defect, and inside
   `doWork` it is untestable (it creates `statCh` itself, spawns the real collector, and needs a live
   `*gocui.Gui` to render).

3. **The frame store.** `frameStore` holds the frozen frame, the timestamp it was rendered at, the
   logtail buffer fields and a `valid` flag. It is owned by the gocui goroutine **for reads as well as
   writes** — that is the whole of Decision 1. The logtail buffer is born inside the render closure
   (`top/stat.go:227,245`, gocui goroutine) while `stat.Stat` arrives on the worker goroutine
   (`top/ui.go:128`), so publishing from inside the closure is the only arrangement that puts both
   halves in one place without a lock. The read half matters just as much: a gate that called
   `printStat(app, app.frame.stats, props)` would read the store on the worker goroutine while the
   closure publishes it — a torn read (`Nrows` from one frame, `Values` from another) is a slice-bounds
   panic inside a `g.Update` closure, which gocui does not recover.

4. **A shared render core.** The repaint cannot go through `printStat` unchanged: its closure returns
   errors at **thirteen** points (`top/stat.go:172-247`), and an error out of a `g.Update` closure tears
   down `MainLoop` → UI rebuild → fresh repaint → the same error, bounded only by the `errorRate` guard
   that kills the process. So the panel/table rendering is extracted into `renderFrame`, used by both
   paths, which differ in **four** dimensions (Decision 6): error policy (propagate on live,
   swallow-and-latch on repaint), the **render timestamp** (a freshly captured `time.Now()` live, the
   stored `at` on repaint), the source of the logtail content (file vs stored buffer), and whether the
   first-tick hint runs. Two of the four — the timestamp and the logtail source — are *consumed* only by
   later tasks (04 threads the stamp into the header, 09 fills the stored buffer), but both must be
   **parameters of the core from the start**: task 04 runs in wave 3 and task 09 in wave 5, and neither
   may edit `top/pause.go`, which task 05 owns in wave 3. A seam cut for only one of them forces a later
   wave to re-open a file it does not own.

The feature's **one structural guarantee** lives in this task: `statLoop` receives neither `*app` nor
the store, and its `repaint` parameter is a bare `func()`. From inside the gate there is physically
nothing to pass, so the racy shape cannot be written by inattention. Nothing else here is structural —
`frameStore`, `statLoop` and the closures all live in package `top`, where unexported means nothing.
**Do not widen that signature**, and do not "helpfully" pass `app` into it.

The task also carries one pre-existing fix that belongs here: the bare send at `top/stat.go:130`
(`statCh <- stat.Stat{Error: err}`) is the one send in `collectStat` not guarded by `ctx.Done()`. On
the UI-error rebuild path `mainLoop` does wait for the collector (`top/ui.go:99-102`), so a collector
parked there makes that wait eternal. The pause makes this materially more reachable, because the five
render-only keys turn the collector's re-initialisation branch into a routine event.

## What to do

**1. Extract `statLoop` from `doWork` (`top/ui.go`).**

- New function with exactly this signature (Decision 12) —
  `statLoop(ctx, uiExit, statCh, paused, render, repaint)`, returning an exit-reason value that
  distinguishes `uiExit` from `ctx.Done()`. `paused` is `*atomic.Bool` (the field task 02 added to
  `config`), `render` is `func(stat.Stat)`, `repaint` is a bare `func()`.
- `doWork` keeps all the wiring it has today — creating `statCh`, spawning `collectStat`, seeding
  `viewCh` with the default refresh — and calls `statLoop` with closures over `app`.
- `doWork` calls `wg.Wait()` **only** on the ctx exit reason, exactly as today (`top/ui.go:130-132`).
  **Do not add `wg.Wait()` to the `uiExit` branch** — on the pager path ctx is not cancelled, and
  `mainLoop` already cancels and waits on every path that needs it.

**2. The gate.** Inside `statLoop`'s `statCh` branch: when paused, call `repaint()` and `continue` —
the frame is dropped, never stored, never rendered. When not paused, `render(s)`. The receive stays
unconditional.

**3. The frame store (`top/pause.go`, extend the file task 02 created).**

- Define `frameStore` with the fields from the tech-spec's Data Models section: the frame, the render
  timestamp, the last non-empty logtail buffer and its path, and `valid`. Unexported fields, no getters
  in production code — the only way to draw it is its own repaint entry point.
- Add the store to the `app` struct (`top/top.go`, next to `uiError`): `app` survives the UI rebuild,
  which the frozen frame must too.
- Add the publish helper used by the live render path, and `repaintStored(app)` — the repaint entry
  point. `repaintStored` **extracts nothing from the store before entering `g.Update`**; it only asks
  for a repaint.
- The `valid` check lives **inside** the repaint closure, never before entering `g.Update` — testing it
  in the gate would read a gocui-owned field from the worker goroutine, reintroducing the race in
  miniature. `valid == false` is a **normal state** (pause engaged before the first frame): draw
  nothing, return `nil`, touch no failure latch.
- Add the repaint failure latch. The tech-spec assigns it to **this** task: Decision 5 specifies the
  behaviour, Decision 6 states that "the failure latch of Decision 5 belongs to Task 3, since it lives
  with the repaint closure's error policy" — task 7 only wires the resize trigger into it. A repaint
  error is swallowed, the closure returns `nil` unconditionally, and exactly **one** cmdline message is
  emitted on the no-failure → failure transition; a subsequent successful repaint re-arms it. The latch
  lives next to the store, gocui-goroutine-only, and is shared by every `repaintStored` caller (this
  task's gate, task 7's resize detector, task 8's filter dialog).
- **The latch must be a separately callable unit of state, not a bare `if` buried in the closure.**
  This is a testability requirement, not a style preference: the repaint closure can only fail at
  `g.View(...)`, and a `*gocui.Gui` / `*gocui.View` cannot be constructed outside the gocui package —
  the repo already records exactly this in the comment on `Test_printCmdlineNilGui`
  (`top/ui_test.go:354-356`). So a test can never *make* the closure fail. Give the latch its own
  entry point — a method on the store (or a small type beside it) that takes the repaint outcome and
  returns whether a message must be emitted, updating the state as a side effect — and have the
  closure call it and, only when it returns `true`, call `printCmdline`. Then the transition logic is
  driven directly by a test, without a Gui, and the closure keeps just the call.

**4. Split `printStat` into a shared core (`top/stat.go`).**

- Extract the panel/table rendering out of `printStat`'s `g.Update` closure into `renderFrame`.
- `printStat` keeps: the first-tick hint (`top/stat.go:165-167`), today's error propagation out of the
  closure, the live logtail source (read the file), and the publish into the store.
- The repaint path wraps the same core in a closure with the opposite error policy and the stored
  logtail source, and it does **not** run the first-tick hint.
- **Design the signature to already accept BOTH the render timestamp and the logtail source**
  (Decision 6), even though task 04 is what consumes the stamp and task 09 what fills the stored
  buffer. Cutting the seam for only one of them is what the tech-spec explicitly forbids: task 04
  would otherwise have to add the timestamp parameter itself, and its only way to hand a *stored*
  stamp to the core would be editing `top/pause.go` — which task 05 owns in the same wave.
  - Live path: `renderFrame` is called with the timestamp captured for this frame and the file-backed
    logtail source.
  - Repaint path: `renderFrame` is called with `frameStore.at` and the stored buffer/path.
- **Capture the stamp exactly once per live frame, and feed both consumers from that one capture.**
  Call `time.Now()` in a single statement at the top of `printStat`'s `g.Update` closure (that is
  "when it was rendered", which is what the store's `at` field documents), pass that value into
  `renderFrame`, and let `renderFrame` write **that same value** into the store when it publishes.
  One stamp, two consumers — the store and (after task 04) the header clock. Add no second
  `time.Now()` call of your own on either path; the one already inside `renderSysstat` stays where it
  is and is task 04's to remove.
- **Do not change `renderSysstat`'s signature** — it keeps its own `time.Now()` at `top/stat.go:277`
  for now. Threading the stamp into the header is task 04, which owns `top/stat_test.go` and its five
  call sites (`top/stat_test.go:60, 97, 192, 440, 441`). This task stops at the `renderFrame`
  boundary: the parameter exists and reaches the store, and task 04 pushes it one level further down.
- **Publish only on the live path** (Decision 2). A repaint must not republish: republishing would
  restamp the frame with the repaint's own clock and the frozen header clock would tick, defeating
  task 04; the logtail buffer would be re-stored from itself. While paused the store must be immutable
  by construction.
- **Make the publish decision a callable unit too**, for the same reason as the latch: `renderFrame`
  itself cannot be driven from a test (it does `g.View` lookups). Express "which path am I" as data —
  a small params/source value built by two constructors, one per path, carrying the stamp, the logtail
  source and whether publishing happens — and put the publish step behind a helper that takes that
  value. Then the invariant "the repaint path never publishes" is checkable by a test that calls those
  two constructors and the helper, instead of resting on a reviewer's reading of the closure.

**5. Guard the bare send (`top/stat.go:130`).** Wrap `statCh <- stat.Stat{Error: err}` in a `select`
with `ctx.Done()`, mirroring the guarded send at `top/stat.go:72-79` (on cancel: close the channel and
return, like the other two exits in the function).

**6. Tests** — see TDD Anchor. At least one must drive the **real** repaint closure rather than a stub:
`-race` alone cannot catch a violation of the ownership rule, because the `statLoop` tests use stub
render/repaint functions and therefore never touch the store from two goroutines even if the production
code would.

## TDD Anchor

Тесты, которые нужно написать ДО реализации. Пишем → запускаем → убеждаемся что падают → пишем код →
убеждаемся что проходят.

Gate and loop (`top/ui_test.go`):

- `top/ui_test.go::Test_statLoop_drainsWhilePaused` — an **unbuffered** `chan stat.Stat`, a producer
  goroutine shaped like the real collector (send, and only *after* the send returns record the
  completion), `paused` pre-set to `true`. All **≥ 5** sends complete, `render` is called **0** times,
  `repaint` is called once per discarded frame. On an unbuffered channel a completed send *is* the
  proof that a receive happened — that is the whole argument. Must fail by timeout (not hang the suite)
  if the gate ever stops receiving.
- `top/ui_test.go::Test_statLoop_rendersAfterResume` — with `paused` flipped back to `false`, the next
  frame reaches `render` exactly once and `repaint` is not called for it.
- `top/ui_test.go::Test_statLoop_exitOnUIExit` — a send on `uiExit` returns the UI exit reason, even
  with a producer send still pending; the test asserts the reason value, since `doWork` keys
  `wg.Wait()` off it.
- `top/ui_test.go::Test_statLoop_exitOnContextCancel` — a cancelled context returns the ctx exit
  reason.
- Counters must be read after the loop goroutine has returned, or guarded, so the tests themselves are
  `-race` clean.

Store and repaint (`top/pause_test.go`):

- `top/pause_test.go::Test_repaintStored_invalidStoreIsNoop` — drives the **real** repaint closure with
  `valid == false`: it returns `nil`, draws nothing, does not dereference the `*gocui.Gui` (so it is
  callable with a nil Gui in the test, which is what proves the `valid` check runs before any view
  lookup), and leaves the failure latch untouched. This requires the closure body to be a **named
  function** taking `*gocui.Gui`, with `repaintStored` doing nothing but handing it to `g.Update` —
  otherwise there is no way to call it from a test at all.
- `top/pause_test.go::Test_frameStore_publishOnlyOnLivePath` — the immutability check, aimed at the
  step that can actually regress. Publish frame A with stamp `t1` through the real publish helper (not
  by assigning fields by hand). Then build the **repaint path's** render params from the store and run
  the publish step for them with a later, **different** frame B and stamp `t2` in hand: the store must
  still hold A's values **and `t1`**. Then run the same publish step with the **live path's** params
  for B/`t2`: the store must now hold B and `t2`. Both halves must be present — the first fails if the
  repaint path ever republishes, the second proves the test is not asserting a helper that never
  writes at all.
  *Why this shape:* driving `statLoop` with `paused == true` and a stubbed repaint proves nothing here
  — the gate cannot reach the store by construction (that is Decision 1's whole point) and the stub
  does not run the publish step, so such a test cannot fail for the reason it claims to check.
- `top/pause_test.go::Test_frameStore_repaintRendersIdenticalBytes` — the stored frame rendered twice
  through the **clock-free** writer-based cores (`renderDbstat` / `printStatData`) into a
  `bytes.Buffer` produces identical bytes, before and after a discarded frame passes through the
  paused gate.
  *Do not include `renderSysstat` in this assertion in this task.* It still calls `time.Now()` itself
  (`top/stat.go:277`, inside `renderSysstat` declared at `:269`) and formats to second precision, so
  two renders straddling a second boundary would differ — a flaky test asserting the opposite of what
  the feature wants. Extending this assertion to the sysstat panel belongs to task 04, once the core
  takes the stamp all the way down; leave a comment in the test saying so.
- `top/pause_test.go::Test_repaintFailureLatch_reportsOncePerTransition` — drives the latch unit
  directly (see "What to do" §3): feed it fail → fail → fail → success → fail and assert the
  "emit a message?" answers are `true, false, false, false, true`, i.e. exactly one report per
  no-failure → failure transition, silence on repeats, and re-arming after a success. No `*gocui.Gui`
  is involved, which is the point: a test cannot make the real closure fail, because its only failure
  site is `g.View(...)` and a `*gocui.Gui`/`*gocui.View` cannot be constructed outside the gocui
  package (`top/ui_test.go:354-356` records this property). The "the closure returns `nil` every time"
  half is not unit-testable and is verified by inspection plus the `grep` step below.

Collector (`top/stat_test.go` is **not** in this task's file list — assert the guarded send through the
loop-level tests above, or add the check to `top/pause_test.go`; do not edit `top/stat_test.go`, task 4
owns it in wave 3):

- The guarded send is verified by inspection plus `go test ./top/... -race`; if a test is written for
  it, put it in a file this task owns.

## Acceptance Criteria

- [ ] `statLoop` exists with exactly the Decision 12 signature; `repaint` is a bare `func()` and
      neither `*app` nor the store is reachable from inside the loop.
- [ ] `doWork` is a thin wrapper: same wiring as today, `wg.Wait()` only on the ctx exit reason, no
      `wg.Wait()` on the `uiExit` branch.
- [ ] While paused, every collector send completes: the drain test proves **≥ 5** consecutive completed
      sends with `render` never called and `repaint` always called.
- [ ] Both exit reasons are covered by tests and are distinguishable by the caller.
- [ ] `frameStore` lives in `top/pause.go`, is written and read only on the gocui goroutine, and is
      never touched by `statLoop` or by any worker-goroutine code.
- [ ] `repaintStored` reads nothing out of the store before entering `g.Update`; the `valid` check is
      inside the closure.
- [ ] `valid == false` repaints as a silent no-op: no draw, no error, no latch, no cmdline write.
- [ ] The store is published **only** on the live render path; a repaint changes neither the stored
      frame nor its timestamp, and this is proved by a test that calls the publish step for both
      paths' params — not by a `statLoop` test with a stubbed repaint.
- [ ] `renderFrame` is shared by both paths and already takes **both** the render timestamp and the
      logtail source as parameters; the repaint path passes `frameStore.at` and the stored buffer,
      performs no file access, and does not run the first-tick hint.
- [ ] Exactly one `time.Now()` is added on the live render path, and its value reaches both the store
      and `renderFrame`. `renderSysstat`'s signature is unchanged (task 04 owns it).
- [ ] The repaint closure returns `nil` unconditionally; the failure latch is a separately callable
      unit whose transition behaviour is covered by a test that needs no `*gocui.Gui`.
- [ ] `top/stat.go:130` is guarded by a `select` on `ctx.Done()`; no bare channel send remains in
      `collectStat`.
- [ ] At least one test drives the real repaint closure rather than a stub.
- [ ] The task's targeted test run (see Verification Steps) passes with `-race`, no new races;
      `make build`, `make lint` and `make vuln` clean.
- [ ] No production code outside `top/` is modified; `view.View` gains no field.

## Context Files

**Feature artifacts:**
- [016-feat-pause-display.md](016-feat-pause-display.md) — user-spec (the "≥5 sends" acceptance
  criterion, the "pause before the first frame" case, the frozen-frame rules)
- [016-feat-pause-display-tech-spec.md](016-feat-pause-display-tech-spec.md) — tech-spec: **Decisions
  1, 2, 6, 12** are this task, and Decision 6 also assigns Decision 5's **failure latch** to it (task 7
  only wires the resize trigger in); Data Models for `frameStore`; Testing Strategy; Implementation
  Tasks → Wave 2 → Task 3
- [016-feat-pause-display-decisions.md](016-feat-pause-display-decisions.md) — decisions log
- [016-feat-pause-display-code-research.md](016-feat-pause-display-code-research.md) — §1.2 (where the
  gate goes), §1.3 (what the closure captures), §7.1 (why not receiving is a total deadlock), §7.2 (the
  unguarded send), §7.3 (`uiExit` and the pager path), §14 (frame store discipline and the
  happens-before argument), §16 (repaint mechanism, `renderFrame` shape), §17 (logtail buffer),
  §21 (concrete `statLoop` signature and the test shape). **Caveat:** §14.2 says the publish is
  "unconditional" — that was **superseded by Decision 2** in the tech-spec (publish only on the live
  path). The tech-spec wins.

**Project knowledge:**
- [overview.md](../../../.claude/skills/project-knowledge/overview.md) — what pgcenter is (this repo
  has no `project.md`; `overview.md` is its equivalent)
- [architecture.md](../../../.claude/skills/project-knowledge/architecture.md) — package layout,
  goroutine model, collector → `statCh` → render data flow
- [patterns.md](../../../.claude/skills/project-knowledge/patterns.md) — "Testable TUI Rendering"
  (pure functions + `io.Writer` printers, assertions against `bytes.Buffer`), testing conventions

**Code files:**
- [top/ui.go](../../../top/ui.go) — `doWork` at `:106-135` (select loop `:123-134`), `mainLoop`'s
  cancel-and-wait at `:78-102`, the `cmdlineCfg` gocui-ownership comment at `:18-40` (the discipline to
  copy for the store), `layout` at `:137` (`verboseTooShortShown` latch precedent at `:141`),
  `writeCmdline`'s nil-Gui tolerance at `:435-439`
- [top/stat.go](../../../top/stat.go) — `collectStat` at `:25-141` (guarded send `:72-79`, **bare send
  `:130`**), `firstTickHint` `:149-154`, `printStat` `:157-253` (the `g.Update` closure `:169-252`,
  thirteen error returns, logtail branch `:226-249`), `printLogtail` `:1229-1245`,
  `readLogfileRecent` `:1202-1226`
- [top/pause.go](../../../top/pause.go) — created by task 02; **extend, never overwrite**. Add
  `frameStore`, the publish helper, `repaintStored` and the failure latch here
- [top/top.go](../../../top/top.go) — `app` struct at `:39-46`; add the store field next to `uiError`
- [top/config.go](../../../top/config.go) — `config` struct at `:17-41`; task 02 adds `paused
  atomic.Bool` here. Read only — **do not modify** (task 05 touches `pause.go` in wave 3, and
  `config.go` is task 02's)
- [top/ui_test.go](../../../top/ui_test.go) — exists (9 tests, all cmdline-composer); extend with the
  `statLoop` tests
- [top/pause_test.go](../../../top/pause_test.go) — created by task 02; extend with the store tests
- [top/config_view_test.go](../../../top/config_view_test.go) — the fake-consumer/producer idiom to
  copy (`:33-40`, reused at `:500`, `:549`)
- [internal/stat/stat.go](../../../internal/stat/stat.go) — `Stat` at `:35-54`; read-only context for
  what a stored frame contains

## Verification Steps

- `go test ./top/ -run 'statLoop|frameStore|repaint|renderFrame|Latch' -race -count=5` — the task's
  own tests pass and are stable under repetition, not accidentally green once. The drain test
  completes rather than timing out, and asserts ≥5 completed sends, 0 renders, 5 repaints.
- `go vet ./top/` and `go build ./...` — the whole package still compiles, including the tests that
  the targeted run filters out.
- **Do not use a bare `go test ./top/...` as the pass/fail gate for this task.** The package cannot
  run to completion without live PostgreSQL: `top/report_test.go:14` calls
  `postgres.NewTestConnect()` and then dereferences the connection, which panics inside
  `internal/postgres` when no fixture cluster is listening. That is an environment prerequisite (the
  project's fixture clusters, see `patterns.md`), not a broken build — if you have the clusters, the
  full run is a welcome extra signal; if you do not, a panic there says nothing about this task.
  Run the targeted subset instead, and never "fix" it by editing `top/report_test.go`.
- `git diff top/` — `top/stat_test.go`, `top/config.go`, `top/keybindings.go`, `top/config_view.go`,
  `top/dialog.go`, `top/help.go` are untouched (they belong to other tasks/waves).
- `grep -n "statCh <-" top/stat.go` — every send is inside a `select` with `ctx.Done()`.
- `grep -n "app\.frame\|frameStore" top/*.go` — every hit is in `top/pause.go`, `top/top.go`, the live
  publish site in `top/stat.go`, or a test. **No hit inside `statLoop` or any function it calls.**
- `make lint` and `make vuln` — clean.
- `make build` still succeeds; running the binary against a live Postgres shows unchanged live-mode
  behaviour (the gate is a no-op while `paused` is false).

## Details

**Files:**
- `top/ui.go` — extract `statLoop` (+ its exit-reason type) out of `doWork`'s select loop; `doWork`
  becomes wiring plus one call. Nothing else in this file changes (the resize detector is task 07).
- `top/stat.go` — extract `renderFrame` from `printStat`'s `g.Update` closure; `printStat` becomes the
  live wrapper (first-tick hint, live error policy, file logtail source, publish). Guard the bare send
  at `:130`.
- `top/pause.go` — add `frameStore`, the publish helper, `repaintStored`, the repaint closure and the
  failure latch. Task 02 created this file for the flag/token/handler; extend it.
- `top/top.go` — **one line**: the store field on `app`. Listed for this task in the tech-spec's
  Implementation Tasks table; wave 2 contains only this task and no other task in any wave touches
  `top/top.go`, so there is no wave conflict.
- `top/ui_test.go` — add the `statLoop` tests (the file currently holds only cmdline-composer tests).
- `top/pause_test.go` — add the store/repaint tests.

**Dependencies:**
- Task 02 must be merged first: it creates `top/pause.go` / `top/pause_test.go` and adds
  `config.paused atomic.Bool`. `statLoop` takes `*atomic.Bool`, so it does not depend on where the flag
  lives, but `doWork` passes `&app.config.paused`.
- Task 01 (non-mutating `printDataCell`) is what makes a repaint of the store a pure function of the
  store. Do not add a deep copy at store time — Decision 10 removed the need for one, and the store is
  deliberately a retention, not a duplication.
- Downstream: task 04 threads the stored timestamp into `renderSysstat`; task 07 and task 08 call
  `repaintStored`; task 09 fills the logtail source. Leave those seams in place, implement none of them.
- No new packages. `sync/atomic` is already imported in `top/ui.go`.

**Edge cases:**
- **Pause engaged before the first frame** — `valid == false`. Repaint draws nothing, returns `nil`,
  touches no latch. The screen stays empty under a live `[PAUSED]`; the first frame after resume renders
  normally. This is a normal state, not an error, and must not be logged or reported.
- **A frame carrying `Stat.Error`** — discarded like any other while paused (an approved user-spec
  decision); it surfaces on the first frame after resume. Do not special-case it in the gate.
- **`statCh` closed by the collector on cancel** — the receive then yields zero values in a tight loop
  until the `ctx.Done()` case is selected. That is today's behaviour; do not add a closed-channel
  branch.
- **A repaint that fails repeatedly** — one message, then silence, until a repaint succeeds again.
  Never propagate the error: an error out of a `g.Update` closure tears down `MainLoop`
  (`gocui/gui.go:377-379`) → rebuild → fresh repaint → same error, bounded only by `errorRate` killing
  the process.
- **Two cmdline writes on one path** is defect class [027]. The repaint path's only cmdline write is the
  latch message on the failure transition; it must not also run the first-tick hint.
- **Logtail during waves 2-4** — with the stored buffer not yet captured (task 09), a repaint renders an
  empty logtail source. `printLogtail` prints nothing and does not even call `v.Clear()` when the buffer
  is empty (`top/stat.go:1230-1232`), so the panel keeps whatever it had. That is the correct interim
  state; do not "fix" it by making the repaint read the file.

**Implementation hints:**
- The `statLoop` seam is mandatory, not stylistic: `printStat` calls `app.ui.Update`, which panics on a
  nil `*gocui.Gui` (`gocui/gui.go:312`). Function-value `render`/`repaint` parameters are what keep a
  `*gocui.Gui` out of the tests entirely.
- The store needs no synchronisation primitive at all, and adding one is a design error, not extra
  safety. The happens-before chain that makes it race-free: collector's `statCh <-` → worker's receive;
  worker's `g.Update(f)` → `MainLoop`'s receive → `f(g)`. So everything the worker captured is visible
  inside the closure, and `app.frame` appears in exactly one goroutine's instruction stream. Copy the
  ownership-comment style of `cmdlineCfg` (`top/ui.go:18-40`) and `writeCmdline` (`top/ui.go:441-443`)
  — an explicit comment naming the owning goroutine is the project's convention for this.
- `printCmdline` from inside a `g.Update` closure is an established pattern here — the logtail branch
  already does it (`top/stat.go:229`). The latch message may use it.
- Timestamp: `renderFrame` takes it as a parameter from the start (Decision 6) and the live path
  captures it once and stores it — but **do not change `renderSysstat`'s signature**. `renderSysstat`
  (`top/stat.go:269`) keeps its inline `time.Now()` at `:277` until task 04; threading the stamp into
  the header there would scoop that task and break its five test call sites out of turn. Inside this
  task the parameter's only consumer is the store, and that is sufficient: the seam exists, so task 04
  changes one signature in `top/stat.go` and touches nothing in `top/pause.go`.
- Two shapes satisfy "publish only on the live path": the publish statement inside `renderFrame` under
  its live/repaint discriminator, or `renderFrame` returning what it rendered and the live caller
  publishing. Prefer whichever leaves **no publish statement reachable from the repaint path at all** —
  the guarantee is worth more than the shorter diff.
- `printDataCell` is non-mutating after task 01, so the render of a stored frame is idempotent. Do not
  re-introduce a write into `Result.Values` anywhere on the repaint path.
- While paused, the store is repainted once per refresh interval (every discarded tick), not only on
  demand. Keep the repaint cheap and allocation-free beyond what rendering already costs.
- The five render-only keys (`[`, `]`, `↑`, `↓`, `\`) are **not** touched by this task: their existing
  `viewCh` push makes the collector emit a frame promptly (`top/stat.go:124-133`), the gate discards it,
  and the discard is what repaints the store (Decision 3). If a test of those handlers changes
  behaviour, something in the gate is wrong.
- Do not touch `collectStat`'s branch-ordering comments at `top/stat.go:85-122` — that ladder is
  load-bearing and unrelated to the guarded send.

## Reviewers

- **dev-code-reviewer** → `016-feat-pause-display-task-03-dev-code-reviewer-review.json`
- **dev-security-auditor** → `016-feat-pause-display-task-03-dev-security-auditor-review.json`
- **dev-test-reviewer** → `016-feat-pause-display-task-03-dev-test-reviewer-review.json`

## Post-completion

- [ ] Записать краткий отчёт в [016-feat-pause-display-decisions.md](016-feat-pause-display-decisions.md) (Summary: 1-3 предложения, ревью со ссылками на JSON, без таблиц файндингов и дампов)
- [ ] Если отклонились от спека — описать отклонение и причину
- [ ] Обновить user-spec/tech-spec если что-то изменилось
