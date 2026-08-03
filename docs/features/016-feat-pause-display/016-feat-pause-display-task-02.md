---
status: planned                    # planned -> in_progress -> done
depends_on: []                     # ID задач-зависимостей (строки: ["01", "02"])
wave: 1                            # волна параллельного выполнения
skills: [code-writing]             # МАССИВ скиллов для загрузки
verify: bash                       # go test ./top/...
reviewers: [dev-code-reviewer, dev-security-auditor, dev-test-reviewer]
teammate_name:
---

# Task 02: Pause flag, `Space` binding and the `[PAUSED]` token

## Required Skills

Перед выполнением задачи загрузи:
- `/skill:code-writing` — [skills/code-writing/SKILL.md](~/.claude/skills/code-writing/SKILL.md)

## Description

This is the first half of the pause feature: the **state**, the **key** and the **marker**. After
this task the operator can press `Space`, the flag flips and `[PAUSED]` appears in the cmdline
immediately — but nothing is frozen yet. The gate, the frame store and the repaint path are Task 3
and are explicitly **out of scope here**.

Three pieces:

1. **`config.paused atomic.Bool`** — the only cross-goroutine value in the whole feature: written by
   the `Space` handler and (later, Task 5) by the lifting handlers on the gocui goroutine, read by
   `statLoop` on the worker goroutine (Task 3). It is an atomic rather than a plain `bool` for that
   reason alone.
2. **`Space` bound to `togglePause`** in the `"sysstat"` context, using `gocui.KeySpace` — not the
   rune `' '` (Decision 13; see Details for why a rune binding would never fire).
3. **`[PAUSED]` as a cmdline token** — the [015] composer already accepts a second token; the marker
   is added as *data*, one single-variant token placed to the LEFT of the filter token. Neither
   `composeCmdline` nor `renderCmdlineTokens` is edited: the composer's own doc comment
   (`top/ui.go:241-244`) names `[PAUSED]` as its motivating example, and `top/ui_test.go` already
   contains literal `[PAUSED]` composer cases written by [015] in anticipation of this task.

The task also pins Decision 8 with a test that guards code which deliberately does not exist: after
a pager/editor return the UI is rebuilt only through a non-nil `app.uiError`, whose `printCmdline`
write in `layout` re-renders the token prefix and brings the marker back. No restoration branch is
written; the test is what keeps that property from silently regressing.

## What to do

1. Add the `paused atomic.Bool` field to `config` (`top/config.go`), placed with the other
   display-mode fields and carrying a doc comment that states its write/read goroutines — the same
   discipline `uiGeneration` documents at `top/ui.go:24-29`. `newConfig` needs no change: the zero
   value is "not paused".
2. Create `top/pause.go` with exactly two things at this stage:
   - `pauseToken` — returns the single-variant `[PAUSED]` token and an "active" flag, reading the
     config's pause state. Nil-safe on the config, mirroring `cmdlineTokens`' contract.
   - `togglePause(app)` — a gocui key handler that flips the flag and performs **exactly one**
     silent cmdline re-render so the marker appears (or disappears) on the same keypress. No text
     message: the user-spec forbids one, and two writes on one path is defect class [027].
   Give the file a package-level doc comment stating that everything in it runs on the gocui
   goroutine — Task 3 will add the frame store here and depends on that being written down.
3. Wire the token into `cmdlineTokens` (`top/ui.go`) **before** the filter token, so the rendered
   order is `[PAUSED][F:...]`. This is the only edit to `top/ui.go` in this task.
4. Add one keybinding row (`top/keybindings.go`): `Space` in the `"sysstat"` context. Keep it inside
   the existing `sysstat` block near the other display-mode toggles.
5. Write the tests listed under TDD Anchor first, watch them fail, then implement.

## TDD Anchor

Tests to write BEFORE the implementation.

New file `top/pause_test.go`:

- `top/pause_test.go::Test_pauseToken` — table: nil config → not active; fresh config → not active;
  paused config → active with exactly one variant, `"[PAUSED]"`. The single-variant assertion is the
  substance: it is what makes the marker non-degradable.
- `top/pause_test.go::Test_togglePause` — the handler flips the flag on the first call and back on
  the second, and does not panic with a nil `*gocui.Gui` (both cmdline writers are nil-Gui-safe, so
  the handler is reachable in a unit test without a terminal).

Extending `top/ui_test.go` (the file exists — extend it, never overwrite):

- `top/ui_test.go::Test_cmdlineTokens` — new subtests on the existing function: "paused config"
  yields exactly the pause token; "paused config with active filters" yields two tokens in the order
  pause-then-filter.
- `top/ui_test.go::Test_cmdlineTokensPauseNeverDegrades` — compose the real tokens of a paused,
  filtered config through `composeCmdline` at widths that force the ladder: the filter token steps
  down and is eventually dropped while `[PAUSED]` stays whole; at a width too small for the marker
  it is absent rather than cut.
- `top/ui_test.go::Test_cmdlineMarkerAfterUIRebuild` — pins Decision 8: publish a paused config as
  the ambient (restoring the previous value with `t.Cleanup`, as `Test_setCmdlineConfig` does), then
  reproduce the write `layout` performs on the rebuild path — the message formatted from a non-nil
  `app.uiError`, which on that path is `fmt.Errorf("")` and therefore empty — and assert the composed
  line still carries the `[PAUSED]` prefix.

## Acceptance Criteria

- [ ] `config` has a `paused atomic.Bool` field with a comment naming its writer and reader goroutines.
- [ ] `view.View` gains **no** field; nothing outside `top/` is modified.
- [ ] `Space` is bound via `gocui.KeySpace` in the `"sysstat"` context only — a space typed into a
      dialog or a menu is still a plain space.
- [ ] Pressing the bound key flips the flag and produces **exactly one** cmdline write, with no text
      message.
- [ ] `[PAUSED]` renders only while paused, always to the left of `[F:...]`, always with exactly one
      variant.
- [ ] `composeCmdline` and `renderCmdlineTokens` are unchanged.
- [ ] `top/pause.go` and `top/pause_test.go` are created; `top/ui_test.go` is extended and all its
      existing tests still pass unmodified in substance.
- [ ] `go test ./top/ -run 'Pause|Cmdline'` passes; `make lint` clean (in particular `go vet`
      copylocks — see Edge cases). The full `./top/...` run additionally needs the fixture clusters
      on ports 21914-21919 — without them `top/report_test.go` panics on a nil connection, which is
      an environment condition rather than a failure of this task.

## Context Files

**Feature artifacts:**
- [016-feat-pause-display.md](docs/features/016-feat-pause-display/016-feat-pause-display.md) — user-spec
- [016-feat-pause-display-tech-spec.md](docs/features/016-feat-pause-display/016-feat-pause-display-tech-spec.md) — tech-spec (Decisions 8 and 13, Data Models, Testing Strategy)
- [016-feat-pause-display-decisions.md](docs/features/016-feat-pause-display/016-feat-pause-display-decisions.md) — decisions log (created on completion)
- [016-feat-pause-display-code-research.md](docs/features/016-feat-pause-display/016-feat-pause-display-code-research.md) — code research

**Project knowledge:**
- [overview.md](.claude/skills/project-knowledge/overview.md) — what the tool is and who uses it
- [architecture.md](.claude/skills/project-knowledge/architecture.md) — package layout, goroutines, data flow
- [patterns.md](.claude/skills/project-knowledge/patterns.md) — "The cmdline: one composer, transient messages, persistent state", testing conventions

**Project docs:**
- [decisions-log.md](docs/decisions-log.md) — the [015] composer/ambient-config decisions
- [tech-debt.md](docs/tech-debt.md) — defect class [027], the "one cmdline write per path" rule

**Code files:**
- [top/config.go](top/config.go) — add the `paused atomic.Bool` field
- [top/keybindings.go](top/keybindings.go) — add the `Space` row
- [top/pause.go](top/pause.go) — **new file**: `pauseToken`, `togglePause`
- [top/pause_test.go](top/pause_test.go) — **new file**: tests for both
- [top/ui.go](top/ui.go) — `cmdlineTokens` only
- [top/ui_test.go](top/ui_test.go) — **exists**, extend
- [top/verbose.go](top/verbose.go) — read: the shape of a display-mode toggle handler in this package
- [top/top.go](top/top.go) — read: the `app` struct the handler closes over

## Verification Steps

- `go test ./top/...` — all green, including the pre-existing `Test_composeCmdline*` cases.
- `go test ./top/... -run 'Pause|Cmdline' -v` — the new tests are actually being run (a `_test.go`
  file with a typo'd name or a wrong package silently runs nothing).
- `go vet ./top/...` — no copylocks complaint about `config` (the new field embeds `noCopy`).
- `make lint` — clean.
- Manual reading check: `git diff top/ui.go` touches `cmdlineTokens` and nothing else.

## Details

**Files:**

- `top/config.go` — one new field on the `config` struct, plus the `sync/atomic` import. Put it with
  the other display-mode fields (`verbose`, `scrollOffset`, `autoScrollToOrderKey`), not at the end.
  The comment matters as much as the field: state that the gocui goroutine writes it (`Space`, and
  from Task 5 the lifting handlers) and the worker goroutine reads it (`statLoop`, Task 3), and that
  this is the feature's only cross-goroutine value. Precedent for that style: the `uiGeneration`
  comment, `top/ui.go:24-29`.
- `top/keybindings.go` — one row in the `keys` slice: the `"sysstat"` context, `gocui.KeySpace`, the
  handler returned by `togglePause(app)`. The table takes `any` for the key, so a wrong constant
  compiles fine; there is **no test coverage of this table at all**, which is why the constant is
  called out explicitly here and re-checked on the stand in Task 10.
- `top/pause.go` — new. Keep it to the two functions this task needs. Task 3 adds `frameStore`,
  `repaintStored`, and Task 5 adds `liftPause`/`liftPauseRefresh` to the same file, so leave the file
  doc comment general enough to cover them ("pause state and the objects owned by the gocui
  goroutine") without describing machinery that does not exist yet.
- `top/ui.go` — inside `cmdlineTokens`, append the pause token before the filter token. Do not move
  the nil-config guard, do not touch the composer or the ladder.
- `top/ui_test.go` — extend. Note `Test_composeCmdlineTwoTokens` carries an [015]-era comment saying
  "no pause code exists in this feature and none must appear"; that sentence is now stale. Updating
  the comment is fine, deleting or rewriting the test is not — it is a user-spec acceptance criterion
  of the previous feature and it must keep building its token as a literal, so it keeps testing the
  composer rather than this task's code.
- `top/pause_test.go` — new. Follow the package conventions: `package top`, testify `assert`,
  table-driven where there is more than one case, and a comment on each test naming the decision or
  property it pins.

**Dependencies:** none — `depends_on: []`, wave 1. `sync/atomic` is already used in `top/`
(`uiGeneration`). Task 1 shares the wave but owns `top/stat.go`, which this task must not touch.
Tasks 3, 5, 7, 8 consume what is built here; the field name, the token function and the handler name
are their contract, so do not rename them opportunistically later.

**Edge cases:**

- `atomic.Bool` embeds `noCopy`, so any copy of a `config` **value** would trip `go vet`'s copylocks
  check. Verified today: `config` is constructed once as `&config{...}` and passed as `*config`
  everywhere, including in tests — so nothing breaks. If a new copy appears, fix the copy, not the
  field type.
- `pauseToken` must tolerate a nil config: `cmdlineTokens` is called with the package ambient
  `cmdlineCfg`, which is deliberately left nil in unit tests (`Test_printCmdlineNilGui`).
- `togglePause` must tolerate a nil `*gocui.Gui` — that is what makes it unit-testable; both cmdline
  writers already return silently on nil, so simply do not dereference `g` in the handler.
- Pausing before the first frame is legal (user-spec): the marker appears over an empty screen. There
  is no "is there a frame?" precondition on the key.
- Width: the marker plus a filter indicator is ~18 columns. On a terminal too narrow for both, the
  filter token degrades and is dropped first because the composer works from the right — which is
  precisely why the pause token goes on the LEFT. Do not add a special case to enforce this.

**Implementation hints:**

- **Why `gocui.KeySpace` and not `' '` (Decision 13):** termbox classifies bytes `<= 0x20` as
  functional keys and delivers `Ch = 0, Key = KeySpace` (`termbox.go:575-577`); gocui matches a
  binding on `key && ch && mod`, so a rune binding for `' '` can never match an event whose `Ch` is
  zero. It would compile, register, and never fire.
- **Why `"sysstat"` and not the global `""` context:** a global binding would swallow the space bar
  everywhere, making it impossible to type a space into the filter dialog or any other input.
- **The "one silent re-render":** the shared writer skips arming its 2-second clear timer when the
  message is empty (`top/ui.go:461`), so an empty-message write re-renders the prefix-only line and
  leaves nothing to expire. That is the whole mechanism — no new writer, no new timer.
- **Do not add `view.View.Paused`.** A field there would ride `viewCh` into `collectStat`, whose
  change-detection ladder compares incoming views and would react to a flag that has nothing to do
  with collection.
- **Do not touch `doWork` / `statLoop` / `printStat` in this task.** A gate added here would be
  merged over by Task 3 and, worse, a gate without the store is a screen that stops updating and
  never repaints.
- The token is a `cmdlineToken{variants: []string{"[PAUSED]"}}` — a single element. Do not pass it
  through `appendCmdlineVariant`; there is no ladder to build.

## Reviewers

- **dev-code-reviewer** → `docs/features/016-feat-pause-display/016-feat-pause-display-task-02-dev-code-reviewer-review.json`
- **dev-security-auditor** → `docs/features/016-feat-pause-display/016-feat-pause-display-task-02-dev-security-auditor-review.json`
- **dev-test-reviewer** → `docs/features/016-feat-pause-display/016-feat-pause-display-task-02-dev-test-reviewer-review.json`

## Post-completion

- [ ] Записать краткий отчёт в [016-feat-pause-display-decisions.md](docs/features/016-feat-pause-display/016-feat-pause-display-decisions.md) (Summary: 1-3 предложения, ревью со ссылками на JSON, без таблиц файндингов и дампов)
- [ ] Если отклонились от спека — описать отклонение и причину
- [ ] Обновить user-spec/tech-spec если что-то изменилось
