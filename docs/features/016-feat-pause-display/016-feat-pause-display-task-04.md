---
status: planned                    # planned -> in_progress -> done
depends_on: ["03"]                 # ID задач-зависимостей (строки: ["01", "02"])
wave: 3                            # волна параллельного выполнения
skills: [code-writing]             # МАССИВ скиллов для загрузки
verify: bash — `go test ./top/...` # инструмент верификации (опционально: curl, bash, user)
reviewers: [dev-code-reviewer, dev-security-auditor, dev-test-reviewer]  # явно указать. Пусто = fallback на defaults
teammate_name:                     # имя агента-исполнителя (опционально; если не задано — генерируется по описанию задачи)
---

# Task 04: Frozen header clock

## Required Skills

Перед выполнением задачи загрузи:
- `/skill:code-writing` — [skills/code-writing/SKILL.md](~/.claude/skills/code-writing/SKILL.md)

## Description

Line 1 of the sysstat panel is built in `renderSysstat` (`top/stat.go:269`), which calls
`time.Now()` inline at `top/stat.go:277`:

```
pgcenter: 2026-08-03 18:36:01, refresh: 1s, load average: ...
```

Task 03 makes that render core reachable from a second path: the repaint of a stored frame. Once
that exists, an inline `time.Now()` means a repainted frame prints the **current** clock over
**frozen** statistics — a header saying "now" above numbers that are three minutes old. That
incoherence is precisely what this feature exists to avoid, so tech-spec Decision 11 removes the
inline call: the render path captures the stamp once and `renderSysstat` receives it as a parameter.

Where the stamp comes from is settled and must not be re-litigated here:

- `stat.Stat` carries no collection time, and the user-spec confines this feature to the `top/`
  package, so the stamp is taken in the **render path** and stored with the frame. Task 03 owns the
  store (`frameStore.at`, `top/pause.go`) — this task threads that value into the renderer, it does
  not invent a second home for it.
- The gap between "collected" and "rendered" is bounded by one refresh interval and is invisible on
  screen. This is a deliberate, accepted approximation, not an oversight — do not try to close it by
  widening the change into `internal/stat`.
- The stamp is captured **only on the live path**. A repaint reuses the stored one; that reuse is
  exactly what freezes the clock. Re-stamping inside the repaint path would defeat this task
  entirely.

The change is small in surface (one parameter, one wrapper, one production call site) and mechanical
in the test file (five existing call sites), but it is the only thing that makes the frozen frame
self-consistent to the operator.

## What to do

1. Give `renderSysstat` a render-timestamp parameter and use it for line 1 instead of `time.Now()`,
   keeping the existing `"2006-01-02 15:04:05"` layout and the rest of the format string byte-identical.
2. Pass the parameter through the `printSysstat` wrapper (`top/stat.go:258`) unchanged — it stays a
   thin delegation.
3. Wire the production call site (`top/stat.go:175`, which after task 03 lives inside the shared
   render core) so that:
   - the **live** path captures the stamp exactly once per frame and uses that single value both for
     the header and for the store publication task 03 introduced — one `time.Now()`, two consumers,
     never two separate captures that can disagree;
   - the **repaint** path passes the stored stamp through unchanged and calls `time.Now()` nowhere.
   Read what task 03 actually landed (the render core's signature, where `frameStore` is published
   and read) and adapt to it. Do not rename or reshape task 03's store fields.
4. Update the five existing `renderSysstat` call sites in `top/stat_test.go` (lines 60, 97, 192, 440,
   441 — `:192` is the shared `verboseSysstatLines` helper covering eight tests) by passing a fixed
   `time.Time`. These are mechanical edits: no assertion may be weakened, loosened or deleted to
   accommodate the new signature.
5. Add the two tests in TDD Anchor below.
6. Do **not** modify `top/pause.go` — task 05 owns that file in this same wave. Read it for the store
   shape; leave it untouched. The same applies to `top/help.go` (task 06).

## TDD Anchor

Тесты, которые нужно написать ДО реализации. Пишем → запускаем → убеждаемся что падают → пишем код → убеждаемся что проходят.

- `top/stat_test.go::Test_renderSysstat_timestampFromParameter` — `renderSysstat` given a fixed
  `time.Time` far from the present (e.g. `2020-01-02 03:04:05`) prints exactly that instant on line 1
  in the existing layout, and line 1 contains no trace of the current wall clock (assert the exact
  expected `pgcenter: 2020-01-02 03:04:05, refresh: ...` prefix — an equality assertion, not a
  regexp, so a reintroduced `time.Now()` cannot pass).
- `top/stat_test.go::Test_renderSysstat_repaintClockDoesNotAdvance` — rendering the **stored** frame
  twice, with the wall clock demonstrably having moved between the two renders, produces the same
  line-1 timestamp both times. Drive this at the highest level reachable **without a live
  `*gocui.Gui`**: if task 03's render core is callable against an `io.Writer`/buffer, assert through
  it; otherwise assert through `renderSysstat` fed from a `frameStore` value constructed in the test
  (same package, no edit to `top/pause.go`). The test must fail if the repaint path re-stamps.

Both tests live in `top/stat_test.go`, follow the existing writer-based golden style of
`Test_renderSysstat_compact` (`bytes.Buffer` + `assert`), and need neither Postgres nor a terminal.

## Acceptance Criteria

- [ ] `renderSysstat` takes the render timestamp as a parameter and contains no `time.Now()` call.
- [ ] `printSysstat` forwards the parameter; its wrapper role is otherwise unchanged.
- [ ] Line 1's rendered text is byte-identical to today's for a given stamp (same layout, same
      `refresh: %ds`, same load-average formatting).
- [ ] The live path captures the stamp once per frame and shares that one value with the store; the
      repaint path reuses the stored stamp and calls `time.Now()` nowhere.
- [ ] Both TDD Anchor tests exist and pass; the repaint test fails if a `time.Now()` is put back on
      the repaint path (verify by trying it locally before finishing).
- [ ] All five existing call sites in `top/stat_test.go` compile against the new signature with no
      assertion weakened or removed; the eight tests behind `verboseSysstatLines` still pass.
- [ ] `go test ./top/...` is green; `make lint` reports nothing new.
- [ ] `top/pause.go`, `top/help.go` and every file outside `top/stat.go` + `top/stat_test.go` are
      untouched by this task.

## Context Files

**Feature artifacts:**
- [016-feat-pause-display.md](docs/features/016-feat-pause-display/016-feat-pause-display.md) — user-spec
- [016-feat-pause-display-tech-spec.md](docs/features/016-feat-pause-display/016-feat-pause-display-tech-spec.md) — tech-spec (Decision 11 is this task; Decisions 2 and 6 constrain it)
- [016-feat-pause-display-decisions.md](docs/features/016-feat-pause-display/016-feat-pause-display-decisions.md) — decisions log (read what tasks 01–03 recorded before starting)
- [016-feat-pause-display-code-research.md](docs/features/016-feat-pause-display/016-feat-pause-display-code-research.md) — code research

**Project knowledge:**
- [overview.md](.claude/skills/project-knowledge/overview.md)
- [architecture.md](.claude/skills/project-knowledge/architecture.md) — Data Flow (top command), Testing
- [patterns.md](.claude/skills/project-knowledge/patterns.md) — "Testable TUI Rendering — pure window function + io.Writer printers", "Naming Conventions", "Linting"

**Code files:**
- [top/stat.go](top/stat.go) — modify: `renderSysstat`, `printSysstat`, the production call site
- [top/stat_test.go](top/stat_test.go) — modify: five existing call sites + two new tests
- [top/pause.go](top/pause.go) — **read only** (owned by task 05 in this wave): `frameStore.at` is the stamp the repaint path reuses

## Verification Steps

- `go test ./top/...` — full package green, including the eight tests behind `verboseSysstatLines`.
- `go test ./top/... -run 'Sysstat'` — the two new tests present and passing (use for the fast TDD loop).
- `grep -n 'time.Now()' top/stat.go` — no occurrence inside `renderSysstat`; the render path has
  exactly one capture, on the live branch.
- Temporarily re-insert a `time.Now()` on the repaint path and confirm
  `Test_renderSysstat_repaintClockDoesNotAdvance` goes red; revert. (A test that cannot fail is not
  a test.)
- `git status --short` — only `top/stat.go` and `top/stat_test.go` modified.
- `make lint` — clean.

## Details

**Files:**

- `top/stat.go:269` — `renderSysstat(w io.Writer, s stat.Stat, verbose bool, local bool, dataDir string, refresh time.Duration) error`. Lines 276–278 build line 1 with `time.Now().Format("2006-01-02 15:04:05")`. Add the timestamp parameter and format it instead. Update the doc comment (262–268) to state that the caller supplies the frame's render time and why (a repaint must not advance the clock).
- `top/stat.go:258` — `printSysstat`, the thin `*gocui.View` wrapper over `renderSysstat`. Add the same parameter and forward it; nothing else changes here.
- `top/stat.go:175` — today the sole production call, inside `printStat`'s `g.Update` closure. Task 03 moves this into the shared render core with two callers (live and repaint). Supply the stamp from whatever task 03 threads through; if the core does not yet carry it, adding that parameter is part of this task.
- `top/stat_test.go` — five call sites to update: `:60` (`Test_renderSysstat_compact`), `:97` (`Test_renderSysstat_refreshFormat`, inside a table loop), `:192` (`verboseSysstatLines`, the shared helper behind eight verbose tests), `:440` and `:441` (`Test_renderSysstat_compactUnchanged`, which compares compact vs verbose output — both calls must receive the **same** stamp or its byte-identity assertion breaks for the wrong reason).

**Dependencies:** task 03 (the render core split and `frameStore`) must be merged first — this task's repaint half has nothing to attach to otherwise. No new Go packages; `time` is already imported in both files.

**Edge cases:**

- **Zero `time.Time`.** Do not special-case it and do not add a "zero means now" fallback: a zero value renders as `0001-01-01 00:00:01` on screen, which is a visible bug worth seeing, not something to paper over. Production callers always have a real stamp (`valid == false` means the repaint draws nothing at all — task 03's concern, not this one).
- **Time zone.** `time.Now()` is local and `Format` does not convert. Pass the value through as-is; do not add `.UTC()` — it would silently change what operators see today.
- **`Test_renderSysstat_compact` line 1** is asserted with a regexp (`stat_test.go:65-68`) that matches any `\d{4}-\d{2}-\d{2} ...` stamp, so a fixed time keeps it green with no edit to the pattern. Keep the regexp as it is — do not loosen it, and do not tighten it either (the exact-stamp assertion belongs in the new dedicated test).
- **`refresh`** stays a separate parameter and keeps its whole-seconds conversion; it is unrelated to this change.
- If task 03 named the store field or the core's parameter differently than the tech-spec sketch, follow what landed in code — consistency with the merged code beats consistency with the spec's illustrative snippet, and note the divergence in the decisions log.

**Implementation hints:**

- Parameter naming and placement: match `frameStore.at` (e.g. `at time.Time`) and keep the order identical in `printSysstat` and `renderSysstat`. Placing it next to `refresh` groups the two time-typed values; either position is fine as long as both functions agree.
- One capture per live frame is the invariant that matters. After the change, grep the render path (`top/stat.go`, `top/ui.go`) for `time.Now()` and be able to point at each remaining occurrence and say which path it serves.
- Write the tests first, watch them fail against the current signature (they will not compile — that is a legitimate red), then change the signature.
- The verbose tests reached through `verboseSysstatLines` assert rows 5–7 and the compact prefix; a fixed stamp in the helper leaves all of them untouched.

## Reviewers

- **dev-code-reviewer** → `docs/features/016-feat-pause-display/016-feat-pause-display-task-04-dev-code-reviewer-review.json`
- **dev-security-auditor** → `docs/features/016-feat-pause-display/016-feat-pause-display-task-04-dev-security-auditor-review.json`
- **dev-test-reviewer** → `docs/features/016-feat-pause-display/016-feat-pause-display-task-04-dev-test-reviewer-review.json`

## Post-completion

- [ ] Записать краткий отчёт в [016-feat-pause-display-decisions.md](docs/features/016-feat-pause-display/016-feat-pause-display-decisions.md) (Summary: 1-3 предложения, ревью со ссылками на JSON, без таблиц файндингов и дампов)
- [ ] Если отклонились от спека — описать отклонение и причину
- [ ] Обновить user-spec/tech-spec если что-то изменилось
