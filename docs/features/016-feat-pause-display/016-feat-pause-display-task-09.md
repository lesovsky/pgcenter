---
status: planned                    # planned -> in_progress -> done
depends_on: ["07", "08"]           # ID задач-зависимостей (строки: ["01", "02"])
wave: 5                            # волна параллельного выполнения
skills: [code-writing]             # МАССИВ скиллов для загрузки
verify: bash — `go test ./top/ -run 'Logtail|logtail'` # точечный прогон: полный `./top/...` требует живых fixture-кластеров
reviewers: [dev-code-reviewer, dev-security-auditor, dev-test-reviewer]  # явно указать. Пусто = fallback на defaults
teammate_name:                     # имя агента-исполнителя (опционально; если не задано — генерируется по описанию задачи)
---

# Task 09: Logtail panel freeze and restore

## Required Skills

Перед выполнением задачи загрузи:
- `/skill:code-writing` — [skills/code-writing/SKILL.md](~/.claude/skills/code-writing/SKILL.md)

## Description

The `L` panel (last lines of the Postgres log) is the one panel whose content does not come from the
frame. It is produced inside the render closure by reading the log file from disk
(`top/stat.go:226-249`). This task freezes it together with everything else: **while paused the log
file is not touched at all** — no `os.Stat`, no `Logfile.Read`, no `Reopen`, no `logtail.Size`
write — and the panel is repainted from a buffer stored alongside the frame.

Three facts make the shape of this task non-obvious, and all three are load-bearing:

1. **The stored buffer must be the last NON-EMPTY read, not "the buffer of the frame on which
   `Space` was pressed".** `readLogfileRecent` returns `(size, nil, nil)` when the file has not
   changed or is empty (`top/stat.go:1213-1216`), and `printLogtail` wraps its whole body in
   `if len(string(buf)) > 0` (`top/stat.go:1230`) — so on a quiet log it prints nothing and does not
   even `Clear`. Storing the frame's own buffer would therefore leave the panel empty on a quiet log:
   exactly the defect the user-spec closed (Q24). The predicate for capturing must be the same
   predicate `printLogtail` uses, so the store and the screen can never disagree about what "shown"
   means.

2. **The panel must be restorable at all, because nothing else in the application holds those
   lines.** Today the displayed lines live only in the gocui view's own line buffer; `stat.Stat`
   does not carry them. On a UI rebuild (pager, editor, `psql`) the `extra` view is recreated empty
   and the only code that refills it is the file read this task disables while paused. Without the
   store, returning from the pager with the panel open would show an empty bottom third under a live
   `[PAUSED]`.

3. **"No file access at all" is stronger than "read but handle errors", and deliberately so.**
   `Logfile.Reopen` closes the current file *before* querying the database for the new path
   (`internal/stat/log.go:39,44`). A rotation detected during a pause on a broken connection would
   leave a closed handle and a stale size, and the resulting error — raised on every interval from
   inside the render closure — would tear down `MainLoop` below the `errorRate` threshold. Skipping
   the branch entirely removes that whole class instead of guarding it.

**Bookkeeping, verified — carry it in.** `config.logtail.Size` is written in exactly two places:
`top/stat.go:243` (the branch this task skips) and `top/extra.go:46` (zeroed when the panel is
opened with `L`; Task 5 reworked that into a local `stat.Logfile` value committed only on success).
Because the size freezes with everything else, the rotation detector on resume
(`size < logtail.Size`, `top/stat.go:233-240`) still fires correctly: the frozen size is the
pre-rotation one, which is precisely what the comparison needs.

This task also fills in the logtail source parameter that Task 3 designed into the shared render
core (`renderFrame`) — the seam exists already; do not redesign it, fill it.

## What to do

- **Capture on the live path only.** In the live logtail branch, after `readLogfileRecent` returns
  content, commit the buffer **and** `config.logtail.Path` into the frame store as a pair, using the
  same non-empty predicate `printLogtail` uses. `Size` is *not* stored — it is change-detection
  bookkeeping and is never rendered. Publication stays live-path-only per Decision 2; a repaint must
  never write the store. Hold the branch's `buf`/`Path` in two locals declared **before** the
  extra-panel block so the single sync call below can see them; the case body only assigns them.
- **Keep the store coherent with what the panel is showing.** When the live render path is not
  showing the logtail panel (`config.view.ShowExtra != stat.CollectLogtail`, which covers both a
  closed panel and a switch to `B`/`N`/`F`), drop the stored buffer and path. This implements the
  data model's `logBuf []byte // nil when the panel was closed` and prevents a repaint from ever
  drawing the previous file's lines under the previous file's header. Do this inside the live path
  in `top/stat.go`/`top/pause.go` — **do not touch `top/extra.go`**, which Task 5 owns.
- **Put the drop OUTSIDE the extra-panel block — this is the easy thing to get wrong.** The entire
  extra render section, `switch` included, is wrapped in
  `if app.config.view.ShowExtra > stat.CollectNone {` (`top/stat.go:200`). The single most important
  case for the drop is `ShowExtra == stat.CollectNone` — the panel is **closed** — and in that case
  control never enters the block at all. A drop written inside the block, or inside the `switch`'s
  `default:`, silently never runs for a closed panel, and a repaint after closing the panel would
  redraw the old log lines. Make the decision one **unconditional** call on the live path, placed
  after the block (or before it), fed the current `ShowExtra` value explicitly — e.g. a
  `frameStore` method `syncLogtail(show int, path string, buf []byte)` that captures when
  `show == stat.CollectLogtail && len(buf) > 0` and drops otherwise. One call site, reached for every
  `ShowExtra` value, is what makes the rule testable and keeps capture and drop from drifting apart.
- **Fill the repaint's logtail source.** The repaint's `case stat.CollectLogtail:` renders the stored
  pair and nothing else: no `os.Stat`, no `Logfile.Read`, no `Reopen`, no `logtail.Size` assignment,
  and no `printCmdline` error write. Whatever seam Task 3 left in `renderFrame` (a source parameter,
  or `live bool` plus a source) is the seam you fill — keep its shape.
- **Make the two halves testable without a terminal.** Extract a writer-based core out of
  `printLogtail` (`renderLogtail(w io.Writer, path string, buf []byte) error`), leaving `printLogtail`
  as the thin `*gocui.View` wrapper, exactly as `printSysstat`→`renderSysstat` and
  `printDbstat`→`renderDbstat` already do in this file. Rendered bytes must be identical to today's,
  including the `Clear`-only-when-there-is-content semantics. Make the repaint's logtail source
  callable without a live `Gui` **by typing its sink as `io.Writer`, so a test can hand it a
  `*bytes.Buffer`**. Do **not** design it so a test passes a nil `*gocui.View`: `*gocui.View`
  implements `io.Writer`, so a nil one becomes a non-nil interface holding a nil pointer and panics
  on the first write instead of being ignored. If Task 3's seam really does hand the source a
  `*gocui.View`, adapt at the wrapper boundary — the stored-source core still takes `io.Writer`.
- **Prove the freeze, do not assert it — and make the adversary READABLE, not broken.** The obvious
  version of this test (non-existent `Path`, nil `File`) proves nothing: a real read would fail at
  `os.Stat` before reaching `Read`, so there is no panic; and the error branch returns before
  `logtail.Size = size`, so the `Size` assertion holds for a reading implementation too. The only
  live assertion left would be "no error", which an implementer satisfies by reading the file every
  interval and swallowing the error. Instead point `config.logtail` at a **real, readable, opened**
  file in `t.TempDir()` whose content differs from the stored buffer and whose on-disk size differs
  from the sentinel `Size`. Then a repaint that touches the file succeeds and renders visibly
  different bytes under a different header path — and the byte-exact assertion fails, which is the
  point.
- **Record the residual, do not fix it.** The descriptor is held for the whole pause, so a
  rotated-away file keeps its inode pinned, and if the new file outgrows the frozen size before
  resume the size-based detector does not fire. This exists today (one refresh interval wide) and is
  merely widened by the pause. Name it in a code comment and in the decisions-log entry; closing it
  needs an identity check (`os.SameFile`/mtime) and is out of scope.

## TDD Anchor

Write these first, watch them fail, then implement.

- `top/pause_test.go::Test_frameStore_storeLogtail_emptyReadKeepsBuffer` — store a non-empty buffer
  with path A, then call the capture with a nil (and separately, an empty) buffer: the stored buffer
  and path are unchanged. This is the Q24 defect expressed at the store level.
- `top/pause_test.go::Test_frameStore_storeLogtail_commitsPathAndBufferTogether` — a non-empty
  capture with path B replaces **both** fields; the store never holds buffer A with path B.
- `top/pause_test.go::Test_frameStore_syncLogtail_dropsForEveryNonLogtailShowExtra` — **the
  placement test.** Seed the store with a non-empty pair, then drive the single sync entry point
  once per `ShowExtra` value — `stat.CollectNone`, `CollectDiskstats`, `CollectNetdev`,
  `CollectFsstats` — and assert both stored fields are nil/empty every time. `CollectNone` is the
  row that matters and the row a careless implementation misses, because the live render's whole
  extra section is wrapped in `if app.config.view.ShowExtra > stat.CollectNone` (`top/stat.go:200`)
  and a drop written inside that block never executes for a closed panel. Table this over the four
  values explicitly rather than testing one representative value.
- `top/pause_test.go::Test_frameStore_syncLogtail_capturesOnlyForLogtail` — the same entry point with
  `ShowExtra == stat.CollectLogtail` and a non-empty buffer captures the pair; with
  `stat.CollectLogtail` and an empty/nil buffer it leaves the previous pair intact (the quiet-log
  rule, at the same seam). Together with the test above this pins the whole truth table of the one
  call, so the only thing left for review by inspection is *where* the call sits.
- `top/stat_test.go::Test_renderLogtail_outputUnchanged` — for a non-empty buffer the output is
  byte-identical to today's format: the header line `\033[30;47m<path>:\033[0m\n` followed by the raw
  buffer, nothing else.
- `top/stat_test.go::Test_renderLogtail_emptyBufferPrintsNothing` — nil and empty buffers write zero
  bytes into a `*bytes.Buffer` and return no error. (The wrapper's "no `Clear`" half needs a real
  `*gocui.View` and stays a review-by-inspection item — do not fake it with a nil view.)
- `top/stat_test.go::Test_repaintLogtail_noFileAccess` — **the adversarial test; build the adversary
  usable, not broken.** Write a real file into `t.TempDir()`, e.g.
  `adversary.log` containing `"ADVERSARY LINE — MUST NOT APPEAR\n"`, and set
  `config.logtail = stat.Logfile{Path: <that path>, Size: 4242}` with `Open()` called on it, so the
  descriptor is live and the sentinel `Size` differs from the file's real size (making
  `readLogfileRecent`'s "unchanged file" early return *not* fire). Seed the store with a different
  pair: `("/var/log/postgresql/A.log", []byte("line1\nline2\n"))`. Render the repaint's logtail
  source into a `*bytes.Buffer` and assert **all four**:
  1. the output is byte-exactly the stored pair — header `\033[30;47m/var/log/postgresql/A.log:\033[0m\n`
     plus `line1\nline2\n`, nothing more;
  2. the output contains neither `"ADVERSARY"` nor the temp path;
  3. no error is returned and nothing panics;
  4. `config.logtail.Size` is still `4242` — a real read reaches `logtail.Size = size` and would
     overwrite it with the file's actual size.

  Every assertion now does work: because the file is readable and the descriptor is open, an
  implementation that touches the file *succeeds* and renders the adversary's bytes under the
  adversary's path. `if err == nil { … }` around a real read no longer passes this test — which is
  exactly what the broken-file version failed to guarantee.

## Acceptance Criteria

- [ ] The repaint path contains no call to `readLogfileRecent`, `Logfile.Read`, `Logfile.Reopen`,
      `os.Stat` and no assignment to `config.logtail.Size`; `grep -n "readLogfileRecent\|Reopen\|logtail.Size" top/*.go` shows those only on the live path (and in `top/extra.go`, untouched).
- [ ] The store captures the logtail buffer and path **only when the buffer is non-empty**, using the
      same predicate as `printLogtail`; an empty read leaves the previous pair intact.
- [ ] Buffer and path are committed together — the store can never pair one file's lines with another
      file's header.
- [ ] The stored logtail pair is dropped on the live path when the panel is not showing the log —
      **including `ShowExtra == stat.CollectNone`**, i.e. the decision is made outside the
      `if app.config.view.ShowExtra > stat.CollectNone` block at `top/stat.go:200`, not inside it.
- [ ] The capture/drop decision has exactly one call site on the live path, reached for every
      `ShowExtra` value, and its full truth table is covered by store-level tests.
- [ ] The store is written on the live path only; a repaint performs no store write (Decision 2).
- [ ] A repaint with an empty/unset store prints nothing and does not `Clear` the view — the panel
      keeps whatever it had, which after a rebuild is empty (the user-spec's "показывать нечего").
- [ ] `printLogtail`'s rendered bytes are unchanged; the writer-based core takes an `io.Writer` and
      is covered by tests that write into a `*bytes.Buffer`. No test passes a nil `*gocui.View`.
- [ ] The no-file-access test uses a real, readable, opened file as the adversary, so it fails if the
      repaint reads the file — including when the read succeeds.
- [ ] `top/extra.go` is not modified by this task.
- [ ] `go test ./top/ -run 'Logtail|logtail'` passes; `go test ./top/ -run 'Logtail|logtail' -race`
      is clean; `make lint` is clean. The full `go test ./top/...` is environment-dependent (see
      Verification Steps) — if the fixture cluster is unavailable, record that rather than claiming a
      full green run.

## Context Files

**Feature artifacts:**
- [016-feat-pause-display.md](016-feat-pause-display.md) — user-spec (decision Q24 — what "the shown
  log lines" means; the freeze/no-op requirements for the unreadable-log paths)
- [016-feat-pause-display-tech-spec.md](016-feat-pause-display-tech-spec.md) — **Decision 7** (this
  task's whole scope), Decision 6 (the shared render core and its logtail source parameter),
  Decision 2 (only the live path publishes), Decision 9 (why `top/extra.go` was reworked by Task 5),
  Data Models (`frameStore`), Testing Strategy → Unit tests → "Logtail", Implementation Tasks →
  Wave 5 → Task 9
- [016-feat-pause-display-decisions.md](016-feat-pause-display-decisions.md) — decisions log; read
  the entries for tasks 3 and 5 to learn the actual shape of `renderFrame` and of the reworked
  `showExtra` logtail branch before writing code
- [016-feat-pause-display-code-research.md](016-feat-pause-display-code-research.md) — **§17**
  (logtail buffer storage: where the state is today, what must be captured, how the repaint renders
  it, `Size` and rotation on resume — read all four subsections), §16.3 (why the repaint is a variant
  of the render closure), §14.2 (store shape — **but see the superseded note in Details**), §7.7 and
  the correction block at the top of the file

**Project knowledge:**
- [overview.md](../../../.claude/skills/project-knowledge/overview.md) — what pgcenter is and what
  the log panel is for (there is no `project.md` in this repo; `overview.md` is its equivalent)
- [architecture.md](../../../.claude/skills/project-knowledge/architecture.md) — package layout,
  collector → `statCh` → render data flow, goroutine ownership
- [patterns.md](../../../.claude/skills/project-knowledge/patterns.md) — "Testable TUI Rendering"
  (printers take `io.Writer`, tests assert against `bytes.Buffer`); "Read state only on the gocui
  goroutine"; testing conventions

**Code files:**
- [top/stat.go](../../../top/stat.go) — the live logtail branch (`:226-249`), `readLogfileRecent`
  (`:1202-1226`), `printLogtail` (`:1229-1245`), plus `renderFrame` as Task 3 left it
- [top/pause.go](../../../top/pause.go) — `frameStore`, `repaintStored`; add the logtail capture/drop
  methods here
- [top/stat_test.go](../../../top/stat_test.go) — render tests go here; style reference
  `Test_firstTickCollectingHint` (`:1492`), `Test_printStatData_truncation` (`:1269`)
- [top/extra.go](../../../top/extra.go) — **read only**: the `L` handler, `logtail.Path`/`Size`
  writes at `:41-59` as reworked by Task 5
- [internal/stat/log.go](../../../internal/stat/log.go) — `Logfile{Path, File, Size}` (`:15-19`),
  `Open` (`:22`), `Close` (`:32`), `Reopen` (`:36-51`, note the `Close` before the DB query),
  `Read` (`:53`)

## Verification Steps

- Run `go test ./top/ -run 'Logtail|logtail' -v` — the new tests are present and green. This is the
  primary `verify` command and it needs no database.
- Run `go test ./top/ -run 'Logtail|logtail' -race` — clean; the store is still touched only on the
  gocui goroutine.
- **Do not treat `go test ./top/...` as the gate.** Part of the package talks to live fixture
  clusters (`postgres.NewTestConnect`, container `lesovsky/pgcenter-testing`); without them
  `Test_getQueryReport` fails and then *panics* on a nil connection at `top/report_test.go:14`,
  killing the package run regardless of this task's changes. Run it only if the cluster is up;
  otherwise stay with the targeted `-run` and say so plainly in the report.
- Run `grep -n "readLogfileRecent\|\.Reopen(\|logtail\.Size" top/*.go` — every hit is on the live
  render path or in `top/extra.go`; none is reachable from the repaint path.
- **Read the drop's placement, do not infer it.** Open `top/stat.go` at the extra-panel block
  (`if app.config.view.ShowExtra > stat.CollectNone`, `:200`) and confirm the sync call sits
  *outside* its braces. A call inside the block passes every unit test in this task and still fails
  in production for a closed panel — the store-level table test pins the function's behaviour, not
  its call site.
- Run `git diff --stat top/extra.go` — empty (this task must not modify it).
- Run `git diff top/stat_test.go top/pause_test.go` — tests are added, none relaxed or deleted.
- Run `make lint` — clean.
- Manual sanity check for the stand run in Task 10 (not required here): open the log panel with `L`,
  press `Space`, wait past several refresh intervals, open and quit the pager (`p`), confirm the log
  panel comes back with the same lines and the same header path.

## Details

**Files:**

- `top/stat.go`
  - *Live logtail branch* (`:226-249`, as Task 3 restructured it). Today: `readLogfileRecent` →
    rotation check (`size < logtail.Size` → `v.Clear()` + `Reopen`) → `logtail.Size = size` →
    `printLogtail(v, logtail.Path, buf)`. Change: keep all of it, and add the capture right at the
    `printLogtail` call — the same statement group as the frame publish, which is what puts "the
    shown lines" and "the frame" in one place with no lock (code-research §17.2).
  - *Repaint logtail branch.* Renders `renderLogtail`/`printLogtail` from the stored pair only. Note
    the repaint deliberately does **not** call `v.Clear()` itself: `printLogtail` clears only when it
    has content, so an empty store leaves the view as it is — empty after a rebuild, intact previous
    content after an overlay close. Both are correct (§17.3).
  - *`printLogtail` (`:1229-1245`).* Split into the `*gocui.View` wrapper plus a writer-based
    `renderLogtail`. The wrapper keeps the guard that decides whether to `Clear` at all; the core
    writes the `\033[30;47m%s:\033[0m\n` header and the buffer. Byte-identical output is a hard
    requirement — the escape sequences and the trailing newline behaviour must not shift.
  - *Do not touch* `readLogfileRecent` — its signature, its `v.Size()`-derived limits and its
    "unchanged file → nil buffer" contract all stay exactly as they are.
- `top/pause.go` — add the capture and drop operations as methods on `frameStore` (they are the only
  things that write `logBuf`/`logPath`), so the non-empty predicate is one named, unit-testable
  place rather than an `if` buried in the render closure. Keep Decision 1 intact: no getter that
  hands the store to another goroutine, nothing read out before entering `g.Update`.
- `top/stat_test.go` — the `renderLogtail` tests and the no-file-access test.
- `top/pause_test.go` — the store-behaviour tests. **This file is not in the tech-spec's file list
  for Task 9**; adding tests to it is safe because Task 9 is alone in Wave 5, and store behaviour
  belongs next to the store. Note the deviation in the decisions-log entry.

**Dependencies:**
- Task 3 — `frameStore`, `repaintStored`, `renderFrame` and its logtail source parameter. This task
  fills that parameter; it must not change the core's signature. If Task 3's seam turned out to be
  `live bool`, keep it and add the source alongside rather than reshaping the call.
- Task 5 — the reworked `showExtra` logtail branch (local `stat.Logfile` committed only on success)
  and the lifting placement. Read what actually landed before assuming the `top/extra.go:45-46`
  mutation is gone.
- Tasks 7 and 8 — the repaint entry points (resize/rebuild, filter dialog) that make the restored
  panel observable. Wave-5 serialisation exists because this task needs `top/stat.go` and
  `top/pause.go` together.
- No new packages.

**Edge cases:**
- *Quiet log.* `readLogfileRecent` returns a nil buffer whenever the size is unchanged or the file is
  empty — the common case, not a rare one. The store must survive it untouched.
- *Pause before the panel was ever opened / before any non-empty read.* `logBuf` is nil; the repaint
  prints nothing, does not `Clear`, and does not touch the Decision 5 failure latch. Nothing to draw
  is not a failure.
- *Panel closed, or switched to `B`/`N`/`F`, while the pause is on.* Those paths lift the pause
  (Task 5), so live frames resume and the live path drops the stored pair. Verify the drop happens on
  the live path, not the repaint path.
- *Panel reopened on a different file.* `L` lifts the pause; on a quiet new log the freshly opened
  panel is blank until the log changes (pre-existing behaviour). The drop rule is what stops a
  subsequent repaint from filling that blank panel with the *previous* file's lines under the
  previous path header.
- *Geometry.* The stored buffer was sized from `v.Size()` at read time (`top/stat.go:1204-1206`), so
  repainting it into a taller terminal shows fewer lines than would fit. Not a defect — the file is
  re-read on resume. Do not try to re-read on resize.
- *Rotation on resume,* three cases (§17.4): log grew → normal read, the panel catches up in one
  frame; rotated and the new file is still smaller than the frozen size → the existing detector fires
  and `Reopen` re-resolves the path; rotated and the new file already grew past the frozen size → no
  `Reopen`, stale lines until the next rotation. The third is the accepted residual — document, do
  not fix.
- *Held descriptor.* The open `*os.File` pins a rotated-away inode for the duration of the pause.
  Same residual, same treatment.
- *No cmdline writes from the repaint.* The live branch prints `Tail Postgres log failed: %s` on
  error; the repaint has no errors of that kind to report and must add no `printCmdline` call — a
  second write on a path is defect class [027].

**Implementation hints:**
- The exact shape of the repaint branch is in code-research §17.3 — follow it.
- **Superseded material, do not follow blindly:** code-research §14.2 shows the frame publish as
  *unconditional* ("a repaint re-publishes identical values"). Tech-spec **Decision 2 overrides it**:
  a repaint would stamp the frozen frame with the current clock and re-store the buffer from itself,
  so publication is live-path-only. The same file carries a correction block at the top overriding
  §1.2, §7.7 and §10 — read it before quoting anything from the research.
- `printLogtail`'s predicate is written `len(string(buf)) > 0`. Keep the semantics exactly (nil and
  empty behave the same, and neither clears the view). Rewriting it as `len(buf) > 0` is behaviourally
  identical and acceptable; changing *when* the `Clear` happens is not.
- The log buffer is raw file content rendered with escape sequences already interpreted by the view —
  that is tech debt [029], pre-existing and explicitly out of scope. This task must not widen it: the
  stored buffer goes through the same `printLogtail` path as today. The new *persistence* consequence
  (a crafted log line now survives on a frozen screen and across a pager return) is handed to `/done`
  for the debt register — do not edit `docs/tech-debt.md` here.
- Keep the change surgical: no reformatting of `readLogfileRecent`, `printIostat`/`printNetdev`/
  `printFsstats`, or the surrounding `switch`.

## Reviewers

- **dev-code-reviewer** → `docs/features/016-feat-pause-display/016-feat-pause-display-task-09-dev-code-reviewer-review.json`
- **dev-security-auditor** → `docs/features/016-feat-pause-display/016-feat-pause-display-task-09-dev-security-auditor-review.json`
- **dev-test-reviewer** → `docs/features/016-feat-pause-display/016-feat-pause-display-task-09-dev-test-reviewer-review.json`

## Post-completion

- [ ] Записать краткий отчёт в [016-feat-pause-display-decisions.md](016-feat-pause-display-decisions.md) (Summary: 1-3 предложения, ревью со ссылками на JSON, без таблиц файндингов и дампов)
- [ ] Если отклонились от спека — описать отклонение и причину (как минимум: тесты в `top/pause_test.go`, которого нет в списке файлов задачи; сброс сохранённого буфера при закрытии панели)
- [ ] Обновить user-spec/tech-spec если что-то изменилось
