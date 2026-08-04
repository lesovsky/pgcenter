---
status: done                       # planned -> in_progress -> done
depends_on: []                     # ID задач-зависимостей (строки: ["01", "02"])
wave: 1                            # волна параллельного выполнения
skills: [code-writing]             # МАССИВ скиллов для загрузки
verify: bash — `go test ./top/ -run 'printDataCell|printStatData'`   # targeted: -run is CASE-SENSITIVE; the full ./top/... run needs live fixture clusters
reviewers: [dev-code-reviewer, dev-security-auditor, dev-test-reviewer]  # явно указать. Пусто = fallback на defaults
teammate_name:                     # имя агента-исполнителя (опционально; если не задано — генерируется по описанию задачи)
---

# Task 01: Non-mutating data cell rendering

## Required Skills

Перед выполнением задачи загрузи:
- `/skill:code-writing` — [skills/code-writing/SKILL.md](~/.claude/skills/code-writing/SKILL.md)

## Description

`printDataCell` (`top/stat.go:1113`) truncates a too-long cell value by writing the truncated string
**back into the result set** — `s.Result.Values[rownum][i].String = …[:width-1] + "~"`. Rendering is
therefore destructive: the printer edits the data it is printing. This task makes it truncate into a
local variable, so the render path only reads.

Two independent reasons make this a prerequisite for the pause feature, and both belong in the
change:

1. **A stored frame would be destroyed by its own rendering.** The feature freezes a frame and
   repaints it on resize, on width changes (`↑`/`↓`), on filter dialogs and after a pager return.
   With the in-place write, the first render replaces the stored value with its truncated form —
   widen the column afterwards and the original text is already gone, permanently, because there is
   no fresh frame to restore it. Live mode never notices, because every tick brings a new frame.
   With the write removed, a repaint of the store is a pure function of the store, and no deep copy
   of the frame is needed at store time (this is what Decision 10 buys the rest of the feature).

2. **It is the only write into `s.Result.Values` in all of `top/`, and it aliases the collector's
   own snapshot.** For views with `DiffIntvl == [0,0]`, `calculateDelta` returns `delta = curr`
   (`internal/stat/postgres.go:596`) instead of allocating a fresh `Values`, so the frame sent on
   `statCh` shares its backing array with `c.currPgStat.Result`, which becomes `c.prevPgStat.Result`
   on the next tick (`internal/stat/stat.go:435-436`). That set of views includes **`activity` — the
   default startup screen**. No race is observable today only because the collector never reads
   those strings back; removing the write turns the aliasing into a shared read and makes that
   accident irrelevant.

The safety property of this task: **rendered bytes must be byte-identical to today's output.** This
is a structural fix with zero visible behaviour change.

## What to do

- Rewrite `printDataCell` so the cell value is read once into a local variable, truncated in that
  local when it exceeds the column width, and printed from the local. No assignment into
  `s.Result.Values[rownum][i]` (or into any field of the result) remains anywhere in `top/`.
- Keep every observable aspect of the current behaviour: the same `width <= 0` error (and the same
  error text) when the column width is zero or negative, the same `width-1` cut plus `'~'` suffix,
  the same `%-*s` padding to `ColsWidth[i]+2`, and the same **byte**-based length and slicing.
  Converting to runes is a different change and is out of scope here; the [029] sanitisation debt
  likewise stays untouched.
- Add a regression test proving the source value survives rendering — the existing truncation test
  asserts on the rendered buffer only and cannot catch the mutation.
- Verify by grep that no write into the result set is left in the package.

## TDD Anchor

Write these first, watch the first one fail against the current implementation, then change
`printDataCell`.

- `top/stat_test.go::Test_printDataCell_doesNotMutateSource` — after rendering a row whose column-0
  value is longer than the column width, `s.Result.Values[0][0].String` still holds the original,
  untruncated text. **Fails on the current code** (the value comes back as `abcd~`).
- `top/stat_test.go::Test_printDataCell_widenAfterTruncation` — render the same `stat.Stat` twice:
  first with a narrow config (value truncated to `abcd~`), then with a config wide enough to hold
  it; the second render prints the full original value. This is the user-spec criterion "расширение
  колонки во время паузы не приводит к навсегда обрезанному значению", expressed at the printer
  level. **Fails on the current code.**
- `top/stat_test.go::Test_printStatData_truncation` (existing, `top/stat_test.go:1269-1284`) — must
  stay green untouched: it asserts `Contains "abcd~"` / `NotContains "abcde"` on `buf.String()`,
  i.e. rendered output rather than structure. Do **not** edit it; if it needs editing, the rewrite
  changed rendered output and is wrong.

## Acceptance Criteria

- [x] `printDataCell` performs no assignment into `s.Result.Values` (or any other field of the passed
      `stat.Stat`); `grep -rn "Result.Values\[" top/ --include='*.go'` shows reads only outside tests.
- [x] Rendered output is byte-identical to before the change for both the truncating and the
      non-truncating path (existing render tests pass unmodified).
- [x] The zero/negative-width branch still returns an error and prints nothing.
- [x] New test proves the source value is unchanged after a render that truncates it.
- [x] New test proves a second render at a larger column width shows the full original value.
- [x] `go test ./top/ -run 'printDataCell|printStatData'` passes; `make lint` is clean.
      (The full `./top/...` run additionally needs the fixture clusters on ports 21914-21919 —
      without them `top/report_test.go` panics on a nil connection. That is an environment
      condition, not a failure of this task.)

## Context Files

**Feature artifacts:**
- [016-feat-pause-display.md](016-feat-pause-display.md) — user-spec
- [016-feat-pause-display-tech-spec.md](016-feat-pause-display-tech-spec.md) — tech-spec (Decision 10,
  Implementation Tasks → Wave 1 → Task 1)
- [016-feat-pause-display-decisions.md](016-feat-pause-display-decisions.md) — decisions log
- [016-feat-pause-display-code-research.md](016-feat-pause-display-code-research.md) — §19
  "Non-mutating `printDataCell`" (current code, rewrite, which tests pin it, aliasing analysis);
  §2.2 and §7.4 for the aliasing chain

**Project knowledge:**
- [overview.md](../../../.claude/skills/project-knowledge/overview.md) — what pgcenter is, supported
  stats (there is no `project.md` in this repo; `overview.md` is its equivalent)
- [architecture.md](../../../.claude/skills/project-knowledge/architecture.md) — package layout,
  data flow collector → `statCh` → render
- [patterns.md](../../../.claude/skills/project-knowledge/patterns.md) — "Testable TUI Rendering"
  (printers take `io.Writer`, tests assert against `bytes.Buffer`), testing conventions

**Code files:**
- [top/stat.go](../../../top/stat.go) — `printDataCell` at `:1103-1119`, the truncation write at
  `:1113`; callers in `printStatData` (frozen column 0 and the windowed loop)
- [top/stat_test.go](../../../top/stat_test.go) — `Test_printStatData_truncation` at `:1269-1284`,
  helpers `makeRenderConfig` at `:1095` and `makeRenderResult` at `:1114`

## Verification Steps

- Run `go test ./top/ -run 'printDataCell|printStatData' -v` — the two new tests and
  `Test_printStatData_truncation` all appear in the output and pass. Note `-run` matches function
  names **case-sensitively**: a capitalised pattern such as `PrintDataCell` matches nothing and still
  exits 0 with "no tests to run", so a green run proves nothing unless the test names appear.
- The full `go test ./top/...` run additionally needs the fixture clusters on ports 21914-21919;
  without them `top/report_test.go` panics on a nil connection, which is an environment condition.
- Run `git diff top/stat_test.go` — the diff only **adds** tests; no existing assertion was relaxed
  or deleted.
- Run `grep -rn "s.Result.Values\[" top/*.go` (non-test files) — every hit is a read; no assignment
  remains.
- Run `make lint` — clean.
- Optional sanity check on the byte-identity property: run the render tests before and after the
  change and confirm the same buffers (the existing alignment-invariant and windowed tests already
  assert exact widths and content).

## Details

**Files:**
- `top/stat.go` — `printDataCell` (`:1103-1119`). Today: reads `len(s.Result.Values[rownum][i].String)`,
  and on overflow assigns the truncated string back into the same field before `fmt.Fprintf` prints
  that field. Change: bind the value to a local at the top, truncate the local, print the local.
  Update the doc comment to say the printer formats and never edits the result set, and say why
  (a stored frame must survive its own rendering; the array may be the collector's snapshot for
  `DiffIntvl == [0,0]` views). Nothing else in the file changes.
- `top/stat_test.go` — add the two tests from the TDD Anchor near the existing truncation test, in
  the same style: build with `makeRenderConfig(ncols, width)` / `makeRenderResult(ncols, nrows)`,
  overwrite a cell with a long `sql.NullString`, render through `printStatData` into a
  `bytes.Buffer` with a `win` from `visibleColumns(...)`. Note `makeRenderResult` values are `rR-cC`
  (5 bytes), so the long value must be seeded explicitly, as the existing test does. Do not modify
  existing tests or helpers.

**Dependencies:** none — this is Wave 1 and depends on no other task. Task 1 owns `top/stat.go` in
Wave 1; task 2 does not touch it. Tasks 3, 4 and 9 later modify `top/stat.go` and build on this
change (task 3 stores the frame and repaints it — the store is only safe to render repeatedly
because of this task).

**Edge cases:**
- `ColsWidth[i] <= 0` — must still return `fmt.Errorf("zero or negative width, skip")` before any
  slicing. Preserve the order: the width check comes after the overflow check, exactly as today, so
  a short value in a zero-width column behaves as it does now.
- `valuelen == ColsWidth[i]` — no truncation (strictly greater today; keep it strictly greater).
- Multi-byte values — byte slicing can cut a UTF-8 sequence mid-rune. That is today's behaviour and
  must be preserved verbatim; fixing it would change rendered bytes and is a separate change.
- `sql.NullString` with `Valid: false` — `.String` is `""`, handled by the same path; no new
  nil-handling is needed.
- Column 0 (frozen) goes through the same function via a separate call site; the change covers it
  automatically.

**Implementation hints:**
- The minimal rewrite is in the code-research file, §19.2 — a local `value := …`, the truncation
  applied to `value`, and `fmt.Fprintf(..., value)`. Follow it.
- `s stat.Stat` is passed **by value**, but `Result.Values` is a slice of slices, so the write goes
  through to the caller's data — that is exactly why the by-value signature does not protect
  anything today. No signature change is needed or wanted: the fix is removing the write, not
  copying the struct.
- **Do not** deep-copy `Result.Values` anywhere. That alternative was explicitly rejected in
  Decision 10 in favour of the causal fix.
- **Do not** cite or follow the superseded recommendations in the code-research file: it carries a
  correction block at the top overriding three of its own suggestions. §19 itself is not among them
  and is safe to follow.
- Keep the change surgical — no reformatting of neighbouring functions, no touching `printStatData`,
  `printStatHeader`, or `visibleColumns`.

## Reviewers

- **dev-code-reviewer** → `docs/features/016-feat-pause-display/016-feat-pause-display-task-01-dev-code-reviewer-review.json`
- **dev-security-auditor** → `docs/features/016-feat-pause-display/016-feat-pause-display-task-01-dev-security-auditor-review.json`
- **dev-test-reviewer** → `docs/features/016-feat-pause-display/016-feat-pause-display-task-01-dev-test-reviewer-review.json`

## Post-completion

- [x] Записать краткий отчёт в [016-feat-pause-display-decisions.md](016-feat-pause-display-decisions.md) (Summary: 1-3 предложения, ревью со ссылками на JSON, без таблиц файндингов и дампов)
- [ ] Если отклонились от спека — описать отклонение и причину
- [ ] Обновить user-spec/tech-spec если что-то изменилось
