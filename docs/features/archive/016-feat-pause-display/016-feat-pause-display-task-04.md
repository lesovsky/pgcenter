---
status: done                    # planned -> in_progress -> done
depends_on: ["03"]                 # ID задач-зависимостей (строки: ["01", "02"])
wave: 3                            # волна параллельного выполнения
skills: [code-writing]             # МАССИВ скиллов для загрузки
verify: bash — `go test ./top/ -run 'Sysstat'`  # инструмент верификации (опционально: curl, bash, user)
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
- **The plumbing already exists when this task starts.** Tech-spec Decision 6 requires task 03 to
  design `renderFrame`'s signature to *already* carry the render timestamp, precisely so this task
  does not have to re-open the seam Wave 2 cut — and does not have to open it inside `top/pause.go`,
  which task 05 owns in this very wave. So this task's job is narrow: change `renderSysstat`'s own
  signature and pass `renderFrame`'s timestamp value down into it.
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
3. Wire the sole production call site (`top/stat.go:175`, which after task 03 lives inside
   `renderFrame`) to pass `renderFrame`'s own render-timestamp parameter into `renderSysstat`. That
   parameter **already exists** when this task starts — tech-spec Decision 6 puts it in task 03's
   scope — so this is one argument threaded one level down, nothing more. Read what task 03 actually
   landed and match its parameter name and position; do not rename or reshape anything of task 03's.
4. **If that parameter is not there, stop.** A missing timestamp on `renderFrame` is a task-03
   defect, not this task's work item. Do **not** add it yourself, and above all do **not** edit
   `top/pause.go` or the repaint call site to work around it — task 05 owns `top/pause.go` in this
   same wave, and touching it breaks the wave rule. Escalate to the orchestrator and wait: the fix
   belongs in task 03.
5. Update the five existing `renderSysstat` call sites in `top/stat_test.go` (lines 60, 97, 192, 440,
   441 — `:192` is the shared `verboseSysstatLines` helper covering eight tests) by passing a fixed
   `time.Time`. These are mechanical edits: no assertion may be weakened, loosened or deleted to
   accommodate the new signature.
6. Add the two tests in TDD Anchor below.
7. Do **not** modify `top/pause.go` — task 05 owns that file in this same wave. Read it for the store
   shape; leave it untouched. The same applies to `top/help.go` (task 06). This task's diff is
   `top/stat.go` and `top/stat_test.go`, nothing else.

## TDD Anchor

Тесты, которые нужно написать ДО реализации. Пишем → запускаем → убеждаемся что падают → пишем код → убеждаемся что проходят.

- `top/stat_test.go::Test_renderSysstat_timestampFromParameter` — `renderSysstat` given a fixed
  `time.Time` far from the present (e.g. `2020-01-02 03:04:05`) prints exactly that instant on line 1
  in the existing layout. Assert the **exact** expected line — `pgcenter: 2020-01-02 03:04:05,
  refresh: 1s, load average: …` — with `assert.Equal`. It must be an equality assertion, never a
  regexp: a `\d{4}-\d{2}-\d{2}` pattern matches the wall clock just as happily as the parameter, so a
  reinstated `time.Now()` would sail straight through it. Equality against a stamp far from today is
  what makes this test able to fail.
- `top/stat_test.go::Test_renderSysstat_timestampIsTheOnlySource` — table over two distinct fixed
  stamps rendered from the same `stat.Stat`: each rendering's line 1 carries its own stamp, and the
  two line-1 strings differ **only** in the timestamp field (rows 2–4 stay byte-identical). This pins
  that the parameter is genuinely the source of line 1 rather than being accepted and ignored — a
  failure mode the single-stamp test above cannot distinguish.

Both tests live in `top/stat_test.go`, follow the existing writer-based golden style of
`Test_renderSysstat_compact` (`bytes.Buffer` + `assert`), and need neither Postgres nor a terminal.

**What these tests deliberately do *not* cover, and why.** The end-to-end invariant "a repaint does
not advance the clock" is **not observable from this task**. `renderFrame` is extracted from a
closure that obtains its views through `g.View(...)` (`top/stat.go:169-252`), so it writes to a live
`*gocui.View`, not to a buffer — there is no way to call it from a test without a `*gocui.Gui`. A
test that fell back to "call `renderSysstat` twice with the same `at`" would reduce to a tautology:
it would stay green even with `time.Now()` put back on the repaint path, which is precisely the
regression it claims to guard. Do not write it. That invariant is covered where it *is* observable —
by task 03's store-immutability test (tech-spec Testing Strategy: "a repaint driven by a later frame
renders the original values and the original timestamp"), because task 03 owns the store and its
tests. This task's contribution to that invariant is the removal of `time.Now()` from
`renderSysstat`, which the two tests above do pin.

## Acceptance Criteria

- [ ] `renderSysstat` takes the render timestamp as a parameter and contains no `time.Now()` call.
- [ ] `printSysstat` forwards the parameter; its wrapper role is otherwise unchanged.
- [ ] Line 1's rendered text is byte-identical to today's for a given stamp (same layout, same
      `refresh: %ds`, same load-average formatting).
- [ ] `renderSysstat` receives its timestamp from `renderFrame`'s existing render-timestamp
      parameter — no new capture, no new parameter added to `renderFrame`, no edit to `top/pause.go`.
- [ ] Both TDD Anchor tests exist and pass, and the first one fails if `time.Now()` is put back into
      `renderSysstat` (verify by trying it locally before finishing).
- [ ] All five existing call sites in `top/stat_test.go` compile against the new signature with no
      assertion weakened or removed; the eight tests behind `verboseSysstatLines` still pass.
- [ ] `go test ./top/ -run 'Sysstat'` is green and the whole `top` package still compiles
      (`go vet ./top/`); `make lint` reports nothing new.
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
- [top/pause.go](top/pause.go) — **read only** (owned by task 05 in this wave): `frameStore.at` is the stamp the repaint path reuses. Context for understanding where the value originates; nothing in this task requires editing it, and editing it is a wave-rule violation

## Verification Steps

- `go test ./top/ -run 'Sysstat'` — green. This is the verification command for this task: it covers
  the two new tests, both golden tests, the refresh-format table and all eight tests reached through
  `verboseSysstatLines` (every affected test name contains `Sysstat`), and it is the fast TDD loop.
- `go vet ./top/` — proves the whole package, tests included, still compiles after the signature
  change even though not every test in it is run.
- **Environment condition:** a full `go test ./top/...` requires a **live PostgreSQL** with
  `pg_stat_statements` — `top/report_test.go:14` queries the database and panics without it. Do not
  treat a failure there as a regression from this task, and do not use the full-package run as this
  task's gate. Run it only if a database is available (and then it is the pre-deploy QA task's job,
  not this one's).
- `grep -n 'time.Now()' top/stat.go` — no occurrence inside `renderSysstat`. For every occurrence
  that remains in the file, be able to say which path it serves.
- Temporarily put `time.Now()` back into `renderSysstat` and confirm
  `Test_renderSysstat_timestampFromParameter` goes red; revert. (A test that cannot fail is not a
  test — and this is the falsifiability check for the change this task actually makes.)
- `git status --short` — only `top/stat.go` and `top/stat_test.go` modified.
- `make lint` — clean.

## Details

**Files:**

- `top/stat.go:269` — `renderSysstat(w io.Writer, s stat.Stat, verbose bool, local bool, dataDir string, refresh time.Duration) error`. Lines 276–278 build line 1 with `time.Now().Format("2006-01-02 15:04:05")`. Add the timestamp parameter and format it instead. Update the doc comment (262–268) to state that the caller supplies the frame's render time and why (a repaint must not advance the clock).
- `top/stat.go:258` — `printSysstat`, the thin `*gocui.View` wrapper over `renderSysstat`. Add the same parameter and forward it; nothing else changes here.
- `top/stat.go:175` — today the sole production call (`printSysstat(v, s, app.config.verbose, app.db.Local, props.DataDirectory, app.config.refresh)`), inside `printStat`'s `g.Update` closure. Task 03 moves this into `renderFrame`, which by then already takes the render timestamp as a parameter (Decision 6). Pass that parameter through — one argument, one level down. If it is absent, stop and escalate (see step 4 of "What to do"); `top/pause.go` and the repaint call site are out of bounds for this task.
- `top/stat_test.go` — five call sites to update: `:60` (`Test_renderSysstat_compact`), `:97` (`Test_renderSysstat_refreshFormat`, inside a table loop), `:192` (`verboseSysstatLines`, the shared helper behind eight verbose tests), `:440` and `:441` (`Test_renderSysstat_compactUnchanged`, which compares compact vs verbose output — both calls must receive the **same** stamp or its byte-identity assertion breaks for the wrong reason).

**Dependencies:** task 03 (the render core split, its timestamp parameter and `frameStore`) must be merged first — there is nothing to thread the value from otherwise. No new Go packages; `time` is already imported in both files.

**Edge cases:**

- **Zero `time.Time`.** Do not special-case it and do not add a "zero means now" fallback: a zero value renders as `0001-01-01 00:00:00` on screen, which is a visible bug worth seeing, not something to paper over. Production callers always have a real stamp (`valid == false` means the repaint draws nothing at all — task 03's concern, not this one).
- **Time zone.** `time.Now()` is local and `Format` does not convert. Pass the value through as-is; do not add `.UTC()` — it would silently change what operators see today.
- **`Test_renderSysstat_compact` line 1** is asserted with a regexp (`stat_test.go:65-68`) that matches any `\d{4}-\d{2}-\d{2} ...` stamp, so a fixed time keeps it green with no edit to the pattern. Keep the regexp as it is — do not loosen it, and do not tighten it either (the exact-stamp assertion belongs in the new dedicated test).
- **`refresh`** stays a separate parameter and keeps its whole-seconds conversion; it is unrelated to this change.
- If task 03 named the store field or the core's parameter differently than the tech-spec sketch, follow what landed in code — consistency with the merged code beats consistency with the spec's illustrative snippet, and note the divergence in the decisions log.

**Implementation hints:**

- Parameter naming and placement: match `frameStore.at` (e.g. `at time.Time`) and keep the order identical in `printSysstat` and `renderSysstat`. Placing it next to `refresh` groups the two time-typed values; either position is fine as long as both functions agree.
- The capture itself belongs to task 03 (one `time.Now()` on the live path, feeding both the header and the store); this task only *consumes* it. After the change, grep `top/stat.go` and `top/ui.go` for `time.Now()` and be able to point at each remaining occurrence and say which path it serves — if you find two captures on the live path, that is a task-03 finding to report, not a thing to fix here.
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
