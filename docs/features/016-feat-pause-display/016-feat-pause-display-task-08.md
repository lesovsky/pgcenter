---
status: planned                    # planned -> in_progress -> done
depends_on: ["05"]                 # ID задач-зависимостей (строки: ["01", "02"])
wave: 4                            # волна параллельного выполнения
skills: [code-writing]             # МАССИВ скиллов для загрузки
verify: bash — `go test ./top/ -run 'Test_applyFilter|Test_dialog'` # точечный прогон: полный `./top/...` требует живых fixture-кластеров
reviewers: [dev-code-reviewer, dev-security-auditor, dev-test-reviewer]  # явно указать. Пусто = fallback на defaults
teammate_name:                     # имя агента-исполнителя (опционально; если не задано — генерируется по описанию задачи)
---

# Task 08: Filter dialog under pause

## Required Skills

Перед выполнением задачи загрузи:
- `/skill:code-writing` — [skills/code-writing/SKILL.md](~/.claude/skills/code-writing/SKILL.md)

## Description

Applying a filter through the `/` dialog is the **one** interactive action on a frozen frame that
needs code of its own. The other five render-only keys (`[`, `]`, `↑`, `↓`, `\`) push on `viewCh`
today "solely to trigger an immediate redraw", the collector answers with a frame, and the gate's
rule — *a discarded frame repaints the store* — makes them work under pause without a line of change
(Decision 3). `setFilter` (`top/config_view.go:124-145`) is different: it edits `view.Filters` and
returns a message, and pushes **nothing**, not even in live mode. Live that is fine — the predicate
is evaluated at render time (`top/stat.go:743, 1051-1061`), so the filter simply becomes visible on
the next collector tick. Under pause that tick renders nothing, so without an explicit repaint
pressing `Enter` on the filter dialog would appear to do nothing at all. This task adds that
repaint in the `dialogFilter` branch of `dialogFinish` (`top/dialog.go:216-217`), guarded by the
pause flag.

**Do not "fix the asymmetry" by making `setFilter` push on `viewCh`.** That path runs `c.Reset()`
(`top/stat.go:127`) and the next delta is divided by the *configured* interval rather than a
measured one (`itv := int(refresh / time.Second)`, `internal/stat/stat.go:294`), so the frame would
show near-zero rates for one tick — a visible dip imported into **live** mode, where filtering is
used far more often than under pause. `clearFilters` (`\`) already behaves that way today; that is
an argument against spreading the behaviour, not for matching it. The direct repaint is also
strictly cheaper: no collector round-trip at all (Decision 4).

The second half of the task is the dialog's geometry. The `[PAUSED]` marker sits in the same
reserved cmdline prefix the dialog prompt is composed against, so while the pause is on the prompt
loses roughly 8 columns — the user-spec records this as expected behaviour, not a defect: on a
narrow terminal a long prompt is truncated earlier than today. The [015] width budget
(`dialogPromptFit`, `dialogInputX0`) already truncates prompts and reserves `minDialogInputWidth`
usable columns for the input field, and it is token-agnostic by construction. This task **verifies**
that it still holds with the marker present — it does not reimplement or re-tune it. If a test here
goes red, the fix belongs in the budget, not in a special case for the marker.

## What to do

- Repaint the stored frame from the `dialogFilter` branch of `dialogFinish` when the pause is on,
  leaving live mode byte-for-byte as it is today. `setFilter` itself is not edited and gains no
  `viewCh` push.
- Make that branch unit-testable without a live `gocui.Gui`: put the "apply the filter, then ask for
  a repaint while paused" step in a small helper in `top/dialog.go` that takes the repaint as a bare
  `func()` parameter, the way `statLoop` takes its own (Decision 12). `dialogFinish` binds
  `repaintStored` into it. Passing a bare `func()` is also what keeps the store unreachable from
  this call site (Decision 1) — nothing here may read frame data.
- Do not touch the cmdline: `dialogFinish` already prints the message `setFilter` returns
  (`top/dialog.go:242`). A second write is defect class [027].
- Extend `top/dialog_test.go` with the repaint tests and with the prompt-geometry tests that put the
  `[PAUSED]` marker into the prefix. Build that prefix through `cmdlineTokens(config)` on a paused
  config rather than hand-writing the token, so the tests exercise the real ordering and do not pin
  Task 2's literal.
- **Every test that drives the filter path must build a real `view.View` first.** `newConfig()`
  (`top/config.go:44-51`) fills only `views` and `viewCh`; `config.view` stays the zero `view.View`,
  whose `Filters` map is **nil**. `setFilter`'s success branch does
  `view.Filters[view.OrderKey] = re`, which panics with "assignment to entry in nil map" on a bare
  `newConfig()`. Set `c.view = view.View{Cols: …, Filters: map[int]*regexp.Regexp{}, OrderKey: …}`
  before calling the helper — in the repaint tests, not only in the geometry ones.

## TDD Anchor

Write these first, run them against the current code (the helper does not exist yet — the repaint
tests fail to compile, which is the intended red), then implement.

**Fixture precondition for the two filter-path tests below.** `newConfig()` leaves `config.view` as
the zero `view.View` with a **nil** `Filters` map, and `setFilter` writes into that map — a bare
`newConfig()` panics before any assertion runs. Build the config as

```go
c := newConfig()
c.view = view.View{Cols: []string{"datname", "usename"}, Filters: map[int]*regexp.Regexp{}, OrderKey: 1}
```

(the idiom of `Test_cmdlineTokens`, `top/ui_test.go:317-330`), then set the pause flag the way Task 2
landed it. A nil-map panic here is a broken fixture, not a finding about the helper.

- `top/dialog_test.go::Test_applyFilter_repaintsWhenPaused` — table over `{paused, live}`: with the
  flag set the repaint callback is invoked exactly once; with it clear it is never invoked. In both
  rows the returned message and the resulting `view.Filters` are exactly what `setFilter` alone
  produces (a valid pattern lands in the map and returns `"Filters: ok"`). Each row needs its **own**
  freshly built `view.View` — the valid-pattern case mutates the map, so a shared fixture would let
  one row's write decide the other row's outcome.
- `top/dialog_test.go::Test_applyFilter_repaintsOnUnchangedFilter` — an invalid regexp
  (`"("`) and an empty answer on a column with no filter both still repaint while paused, and the
  message is `setFilter`'s own. Same fixture requirement: the empty-answer case reads
  `view.Filters[view.OrderKey]` and `delete`s from it — both are safe on a nil map, so this test
  would pass with a broken fixture while its sibling panics. Build the map here too, so the two
  tests exercise the same object.
- `top/dialog_test.go::Test_dialogPromptFitWithPauseMarker` — sweep over every entry of
  `allDialogTypes`, over prefixes `{paused}` and `{paused + active filter}`, and over terminal
  widths: the composed line never outgrows `maxX - minDialogInputWidth - 1`, the input field's first
  content column (`x0+1`) is not inside the printed text, the field keeps at least
  `minDialogInputWidth` usable columns, and the shown prompt is still a prefix of the original modulo
  the `…` marker. This is the user-spec criterion "подсказка обрезается раньше обычного — это
  ожидаемое поведение" expressed as an invariant.
- `top/dialog_test.go::Test_dialogPromptShorterUnderPause` — at a fixed `maxX` and on a prompt long
  enough to be truncated in **both** states (use `dialogPrompts(dialogSetMask)`, 93 runes), the
  prompt shown under pause is strictly shorter than the live one, and the difference equals the
  rendered marker plus its separating space — the user-spec's "примерно на 8 символов". Derive that
  width from `composeCmdline(pausedTokens, "", wide)`, never from a literal `9`.
- `top/dialog_test.go::Test_dialogMarkerSurvivesNarrowDialog` — at `maxX = 60` with an active filter
  and the pause on: the composed dialog line still starts with the marker, `x0 < maxX-1`, and the
  field keeps its `minDialogInputWidth` columns. The narrow-terminal case the stand run checks at
  `-x 60`.

## Acceptance Criteria

- [ ] Applying a filter through the dialog while paused repaints the stored frame; the frozen table
      is redrawn with the new predicate applied.
- [ ] In live mode the filter dialog behaves exactly as before: no repaint call, no `viewCh` push,
      no extra cmdline write.
- [ ] `setFilter` (`top/config_view.go`) is unmodified — `git diff` shows no change to that file.
- [ ] The repaint branch is covered by unit tests that need no `gocui.Gui` and no Postgres.
- [ ] The dialog prompt invariants (budget, no overlay, `minDialogInputWidth` preserved, prompt is a
      prefix of the original) hold for every prompt with the `[PAUSED]` marker in the prefix.
- [ ] Only `top/dialog.go` and `top/dialog_test.go` are modified; `top/ui.go`, `top/pause.go` and
      `top/config_view.go` are untouched.
- [ ] All nine pre-existing tests in `top/dialog_test.go` still pass, none weakened or deleted.
- [ ] `go test ./top/ -run 'Test_applyFilter|Test_dialog'` passes; `make lint` is clean. The full
      `go test ./top/...` is environment-dependent (see Verification Steps) — if the fixture cluster
      is unavailable, record that in the report instead of claiming a full green run.

## Context Files

**Feature artifacts:**
- [016-feat-pause-display.md](016-feat-pause-display.md) — user-spec: "Формы и ввод данных" (the
  marker takes part of the dialog line), "Диалоги — то же правило" (`/` keeps the pause), and the two
  acceptance criteria about the shorter prompt and the narrow terminal with an active filter
- [016-feat-pause-display-tech-spec.md](016-feat-pause-display-tech-spec.md) — **Decision 4** (this
  task's core), Decision 3 (why the other five keys need nothing), Decision 1 (the store is not
  readable from here), Decision 12 (the bare `func()` repaint parameter), Testing Strategy → "Dialog
  prompt geometry holds while the marker is present", Implementation Tasks → Wave 4 → Task 8
- [016-feat-pause-display-decisions.md](016-feat-pause-display-decisions.md) — decisions log
- [016-feat-pause-display-code-research.md](016-feat-pause-display-code-research.md) — §16.2 "Option
  B vs option A" (the full argument against a `viewCh` push in `setFilter`), §16.3, and the test
  impact table row 16 ("`Test_setFilter` unaffected — the repaint is added in `dialogFinish`").
  **Read its correction block first**: the file carries corrections that override several of its own
  suggestions. One more override applies to this task — see Implementation hints below

**Project knowledge:**
- [overview.md](../../../.claude/skills/project-knowledge/overview.md) — what pgcenter is (this repo
  has no `project.md`; `overview.md` is its equivalent)
- [architecture.md](../../../.claude/skills/project-knowledge/architecture.md) — package layout,
  data flow collector → `statCh` → render
- [patterns.md](../../../.claude/skills/project-knowledge/patterns.md) — "The cmdline: one composer,
  transient messages, persistent state (015)" (the token prefix and the dialog budget),
  "Testable TUI Rendering" (pure helpers + `io.Writer` printers, since `gocui.View` cannot be
  constructed in a unit test), "Linting"

**Code files:**
- [top/dialog.go](../../../top/dialog.go) — **modify**: the `dialogFilter` branch of `dialogFinish`
  (`:216-217`); `dialogPromptFit` (`:70`), `dialogInputX0` (`:104`) and `dialogOpen` (`:118`) are
  read-only context for the geometry half
- [top/dialog_test.go](../../../top/dialog_test.go) — **modify (extend, nine tests already exist)**:
  helpers `allDialogTypes` (`:14`) and `activeFilterTokens` (`:29`) are reused by the new tests
- [top/config_view.go](../../../top/config_view.go) — **read only**: `setFilter` (`:124-145`),
  `isFilterActive` (`:151`), `clearFilters` (`:193`, the neighbour that does push and must not be
  imitated), and the "push solely to trigger an immediate redraw" comment at `:56-58`
- [top/pause.go](../../../top/pause.go) — **read only** (created by Tasks 2/3/5): `repaintStored`,
  the pause flag accessor and `pauseToken`
- [top/ui.go](../../../top/ui.go) — **read only**: `composeCmdline` (`:257`), `cmdlineTokens`
  (`:406`), `truncateRunes` (`:315`), `printCmdline` / `printCmdlinePersist` (`:421`, `:428`)
- [top/config.go](../../../top/config.go) — **read only**: `config` and `newConfig`, plus the
  `paused` field Task 2 adds

## Verification Steps

- Run `go test ./top/ -run 'Test_applyFilter|Test_dialog'` — the nine pre-existing dialog tests in
  their original form plus the five new ones, all green. This is the primary `verify` command and it
  needs no database.
- **Do not treat `go test ./top/...` as the gate.** Part of the package talks to live fixture
  clusters (`postgres.NewTestConnect`, container `lesovsky/pgcenter-testing`); without them
  `Test_getQueryReport` fails and then *panics* on a nil connection in
  `top/report_test.go:14`, taking the whole package run down regardless of this task's changes. Run
  it only if the cluster is up; otherwise stick to the targeted `-run` above and say so plainly in
  the report rather than passing a partial run off as a full one.
- Run `git diff --name-only` — exactly two files: `top/dialog.go`, `top/dialog_test.go`. Any hit on
  `top/ui.go`, `top/pause.go` or `top/config_view.go` is a wave-partition violation (Task 7 owns
  `top/ui.go` this wave).
- Run `git diff top/dialog_test.go` — the diff only **adds**; no existing assertion relaxed, no
  existing test deleted or renamed.
- Run `grep -n "viewCh" top/dialog.go top/config_view.go` — `setFilter` still pushes nothing and
  `dialog.go` contains no push.
- Run `grep -n "printCmdline" top/dialog.go` — the count is unchanged from `master`; the filter
  branch added no write.
- Run `make lint` — clean.
- Deferred to the Task 10 stand run (not verifiable here): pause, press `/`, type an expression,
  press `Enter` — the frozen screen redraws with fewer rows and `[PAUSED]` stays on the line.

## Details

**Files:**
- `top/dialog.go` — add one small helper next to `dialogFinish` and call it from the `dialogFilter`
  branch. Shape:

  `applyFilter(answer string, config *config, repaint func()) string` — calls
  `setFilter(answer, config.view)`, then invokes `repaint()` when the pause flag is set, and returns
  `setFilter`'s message unchanged. The call site becomes
  `message = applyFilter(answer, app.config, func() { repaintStored(app) })`.

  Read the pause flag exactly the way Task 2 landed it (`config.paused.Load()` or the accessor it
  introduced) — do **not** add a second accessor of your own. Document in the helper's comment *why*
  this one branch needs a repaint when its five neighbours do not, and that the `func()` parameter is
  what keeps the store unreadable from here.
- `top/dialog_test.go` — extend. The file's existing style is table/sweep tests over
  `allDialogTypes` and widths, built through the same two calls `dialogOpen` makes
  (`dialogPromptFit` → `composeCmdline` → `dialogInputX0`); follow it.

  **Config fixture — required in every test that reaches `setFilter`, not just the geometry ones.**
  `newConfig()` (`top/config.go:44-51`) returns `&config{views: …, viewCh: …}` and nothing else, so
  `config.view` is the zero `view.View` and `view.Filters` is a **nil** map. `setFilter`'s success
  branch does `view.Filters[view.OrderKey] = re` — writing to a nil map panics. Build the view
  explicitly before each call:

  ```go
  c := newConfig()
  c.view = view.View{Cols: []string{"datname", "usename"}, Filters: map[int]*regexp.Regexp{}, OrderKey: 1}
  // then set the pause flag as Task 2 landed it
  ```

  (idiom: `Test_cmdlineTokens`, `top/ui_test.go:317-330`). A shared package-level fixture is wrong
  here: the valid-pattern row mutates `Filters`, so give each table row its own. For the paused
  prefix in the geometry tests, take `cmdlineTokens(c)` off the same kind of config — the `Cols` and
  `Filters` are what make `filterToken` produce the `[F:datname]` half of the prefix.

**Dependencies:**
- Depends on Task 5, which is the last Wave-3 task to touch `top/pause.go`; `repaintStored` itself
  comes from Task 3 and the pause flag plus `pauseToken` from Task 2. All three are done before this
  wave starts.
- **Wave partition:** Task 7 owns `top/ui.go` in Wave 4. This task modifies **only**
  `top/dialog.go` and `top/dialog_test.go` — it reads `top/ui.go` and `top/pause.go` and calls into
  them, never edits them. The helper goes in `top/dialog.go`, not in `top/config_view.go`.

**Edge cases:**
- **Nothing has ever been rendered** (`Space` before the first frame, then `/`): `repaintStored`
  draws nothing and returns — that is a normal state, not an error. `top/dialog.go` must **not**
  inspect the store's validity flag or any other field of it; the guard here is the pause flag alone.
- **Invalid regexp / empty answer on an unfiltered column:** the message is `setFilter`'s error or
  "no filter on this column", and while paused the repaint still runs. Deliberate — see the TDD
  anchor.
- **`Esc` on the dialog:** `dialogCancel` is a different handler, untouched; the pause and the frame
  survive, nothing is repainted.
- **`\` (clear filters):** not this task. It rides its existing `viewCh` push (Decision 3) — do not
  add a repaint call there, and do not remove its push.
- **A space typed into the filter expression** must stay a plain space: `Space` is bound in the
  `"sysstat"` context only (Decision 13). Nothing here should widen that binding.
- **Degenerate geometry** (`maxX <= 0` after returning from a pager): `dialogOpen` already refuses to
  open with a message; the marker does not change that path.
- **Terminal narrower than the reservation:** `dialogPromptFit` returns an empty prompt and the field
  keeps its columns. With the marker this regime simply starts at a wider terminal — see the sweep
  bound below.

**Implementation hints:**
- **One override of the code-research file.** §16.2 suggests guarding the repaint with
  `paused && app.frame.valid`. Use the pause flag **only**: the tech-spec's Data Models section makes
  `valid == false` a normal state handled inside the repaint, and Decision 1 keeps the store's fields
  unreachable from call sites. Reaching into `app.frame` from `dialog.go` would be exactly the
  coupling Decision 1 removes.
- **The sweep lower bound is not 24.** `Test_dialogPromptFitMarksTruncation/"marker never widens the
  line"` starts its width sweep at 24 because below that the budget
  (`maxX - minDialogInputWidth - 1`) no longer covers the 11-rune `[F:datname]` prefix, which
  `dialogPromptFit` cannot shrink. With `[PAUSED]` in front the prefix is ~19-20 runes and the same
  regime starts at ~30. Derive the bound in the new tests as
  `utf8.RuneCountInString(prefix) + minDialogInputWidth + 1` instead of copying the literal 24 —
  otherwise the new sweep fails legitimately at widths 24..29 and the temptation will be to weaken
  the assertion.
- **Choose a prompt that is actually truncated** for the "shorter under pause" test. `"Set filter: "`
  is 12 runes and fits at `maxX = 80` both with and without the marker, so both answers would be
  equal and the test would prove nothing. `dialogPrompts(dialogSetMask)` is 93 runes and is cut in
  both states — the existing `Test_dialogSetMaskOpensAt80Columns` already relies on that length.
- Prefer new sibling tests over editing the existing sweeps. If you do extend the `prefixes` map in
  `Test_dialogPromptNeverOverlappedByInput` or the map in the "marker never widens the line"
  subtest, add entries only, change no assertion, and mind the bound above.
- Do not hardcode `"[PAUSED]"` in assertions. Render it via
  `composeCmdline(cmdlineTokens(pausedConfig), "", wideWidth)` on a config with no filters and
  compare against that — the test then survives a rename of the marker in Task 2.
- Keep the change surgical: no reformatting of `dialogOpen`, no touching `dialogPrompts`,
  `dialogInputX0` or `dialogPromptFit`, no new constants.

## Reviewers

- **dev-code-reviewer** → `docs/features/016-feat-pause-display/016-feat-pause-display-task-08-dev-code-reviewer-review.json`
- **dev-security-auditor** → `docs/features/016-feat-pause-display/016-feat-pause-display-task-08-dev-security-auditor-review.json`
- **dev-test-reviewer** → `docs/features/016-feat-pause-display/016-feat-pause-display-task-08-dev-test-reviewer-review.json`

## Post-completion

- [ ] Записать краткий отчёт в [016-feat-pause-display-decisions.md](016-feat-pause-display-decisions.md) (Summary: 1-3 предложения, ревью со ссылками на JSON, без таблиц файндингов и дампов)
- [ ] Если отклонились от спека — описать отклонение и причину
- [ ] Обновить user-spec/tech-spec если что-то изменилось
