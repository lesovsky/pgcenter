---
status: planned
depends_on: ["05"]
wave: 3
skills: [code-writing]
verify: "bash — `go test -race ./top/...` in the CI image (the whole package needs live clusters); on the host `go test ./top/ -run 'Test_walNextView|Test_switchViewTo|Test_selectMenuStyle|Test_menuSelectWAL|Test_keybindingsWAL|Test_helpTemplate'`"
reviewers: [dev-code-reviewer, dev-security-auditor, dev-test-reviewer]
teammate_name:
---

# Task 06: TUI navigation — `w` cycle, `W` menu, help

## Required Skills

Before starting, load:
- `/skill:code-writing` — [SKILL.md](~/.claude/skills/code-writing/SKILL.md)

## Description

Task 05 registered the `archiver` view in `view.New()`. Nothing in the TUI can reach it yet: `w` is
still a plain one-way switch to `wal` (`top/keybindings.go:38` → `switchViewTo`'s `default` arm), there
is no `W` menu, and the help screen documents neither.

This task adds the navigation layer, copying the `pg_stat_io` (`j`/`J`) precedent end to end. That
precedent is five pieces and this task reproduces all five:

1. `walNextView(current string) string` in `top/config_view.go` — the cycle function, sibling of
   `statioNextView` (`:274-287`).
2. A `case "wal":` in `switchViewTo`'s dispatch switch (`:241-252`), sending the key through
   `walNextView`.
3. `menuWAL` in `top/menu.go`'s iota block, its `selectMenuStyle` branch with two items, and its
   `menuSelect` branch calling `viewSwitchHandler` directly.
4. `{"sysstat", 'W', menuOpen(menuWAL, app.config, "")}` in `top/keybindings.go`. `'W'` is free —
   a full read of `keybindings.go:18-85` shows no `'W'` binding (`profile`'s `-W` at
   `cmd/profile/profile.go:52` is an unrelated CLI flag on a different sub-command).
5. Three changed help-screen lines in `top/help.go`.

One deliberate deviation from the precedent needs an explicit code comment (Decision 6): every other
cycle passes a *group* name that is not a view name (`statio`, `databases`, `progress`, `statements`),
but here the string `"wal"` serves as **both** the view name and the cycle name. The view cannot be
renamed — it is the `report -W w` report type and the tar-entry prefix in recorded archives. Only one
dispatch site is affected and the menu path bypasses `switchViewTo` entirely, so the collision is
contained; the comment exists so a future reader does not "fix" it into a separate group name.

The help screen is the project's only user-facing hotkey documentation, and in this project it is
pinned **by test, not by review** — `top/help_test.go` exists for exactly that reason (feature 016).
Two acceptance criteria of the user-spec ride on the help text, so the new `w,W` entry and the
`archiver` addition to the `Q` caveat are pinned there too, in the style of the existing entry tests.

**All five pieces are unit-testable, including the two TUI ones.** A **zero-value `&gocui.Gui{}`** is
enough for both, and this was verified empirically against the existing `menuStatIO` branch before this
task was written:

- `gocui.Gui.SetView` builds a real `*gocui.View` from the passed coordinates without touching a
  terminal (it returns `gocui.ErrUnknownView` as its *created* signal — that is what `menu.go:117`
  keys off), `View.SetCursor` works on it, and `DeleteView`/`SetCurrentView` merely scan a slice,
  returning `gocui.ErrUnknownView` instead of panicking. Driving `menuSelect(app)(&gocui.Gui{}, mv)`
  over the `menuStatIO` branch with cursor 0 and 1 produced `stat_io` and `stat_io_time` on `viewCh`,
  returned a nil error (once a `sysstat` view exists for `menuClose` to focus), and left the menu reset
  to `menuNone`.
- `keybindings(app)` with `app.ui = &gocui.Gui{}` returns nil: `gocui.SetKeybinding` only appends to a
  slice. Uniqueness of a key is then assertable through the exported `DeleteKeybinding` — today `'J'`
  deletes once and reports `keybinding not found` on the second call, while `'W'` reports not-found
  immediately.

So the `W` path gets a real test on both halves — the branch that resolves the cursor to a view, and
the binding that reaches it — and the "`'W'` is free" claim becomes a regression test rather than a
grep-and-trust note. The *nil* Gui that `top/pause_test.go:552-577` warns about is a different thing
from a zero-value one; `top/ui_test.go:406-425` already uses the zero-value idiom.

## What to do

1. **Add `walNextView`** to `top/config_view.go`, next to `statioNextView`, with the same shape and a
   doc comment in the same form: `wal` → `archiver`, `archiver` → `wal`, anything else → `wal`.

2. **Add the `case "wal":` arm** to `switchViewTo`'s `switch c`, alongside `case "statio":`, routing
   through `walNextView(app.config.view.Name)` into `viewSwitchHandler`. Carry a code comment
   recording Decision 6: `"wal"` is simultaneously the view name and the `w`-group name, this is the
   group's single dispatch point, and `walNextView`'s default returns `"wal"` so `w` pressed on any
   other screen still lands on the wal screen exactly as before. Nothing else in `switchViewTo` moves:
   the pg_stat_statements guard stays above, and the single trailing
   `printCmdline(g, "%s", app.config.view.Msg)` stays the one write per path.

3. **Add `menuWAL`** to the iota block in `top/menu.go`, immediately after `menuStatIO` and **above the
   blank line** — inside the menu group, never in the directions group below it.

4. **Add the `case menuWAL:` branch to `selectMenuStyle`**, copied from `case menuStatIO:`: two items,
   each with the established leading space, and a title in the existing
   `" Choose … (Enter to choose, Esc to exit): "` form.

5. **Add the `case menuWAL:` branch to `menuSelect`**, copied from `case menuStatIO:`: cursor index 0 →
   `wal`, 1 → `archiver`, default → `wal`, each via `viewSwitchHandler` directly (not `switchViewTo`),
   followed by exactly one `printCmdline(app.ui, "%s", app.config.view.Msg)`.

6. **Add the `W` keybinding** to `top/keybindings.go`, directly after the `'J'` line. The existing
   `{"sysstat", 'w', switchViewTo(app, "wal")}` line stays byte-identical — its meaning changes only
   because `switchViewTo` gained the new case.

7. **Edit three lines of `helpTemplate` in `top/help.go`:**
   - the `r,w` line loses `w` and its `'w' WAL,` clause, keeping only `'r' replication,`;
   - a new `w,W` line is added directly after the `j,J` line, in that line's format:
     `'w' pg_stat_wal / pg_stat_archiver switch, 'W' WAL statistics menu.`;
   - the `Q`-does-not-reset caveat gains `archiver` at the end of its list:
     `pg_stat_io, bgwriter, wal, archiver`.

   Description columns of the block stay aligned; the leftover `r` keeps its own line (the tech-spec
   settles this cosmetic question — do not fold it into the `a,b,f,o` line above).

8. **Amend the stale prohibition in `top/pause_test.go`** (`:561-568`): the sentences claiming that
   *`menuSelect` itself is unreachable from a unit test* and that a Gui *"comes only from
   `gocui.NewGui`"* are wrong — a zero-value `&gocui.Gui{}` reaches every branch (see Description).
   Correct exactly those sentences to say that the blocker is specific to the `menuConf` branch, whose
   terminal call `editPgConfig` needs a live `*postgres.DB`. Nothing else in that comment block moves,
   and the deleted `Test_menuConfPathDoesNotLift` stays deleted — its problem was that it asserted on a
   config the callee never receives, which is untouched by any of this.

9. **Write the tests first** (see TDD Anchor): the new `Test_walNextView`, `Test_menuSelectWAL`,
   `Test_keybindingsWAL`, the three help tests, and the new rows in `Test_switchViewTo` and
   `Test_selectMenuStyle`.

## TDD Anchor

Write these BEFORE the production code. Run them, see them fail for the right reason, then implement,
then see them pass. Every test named here runs on the host under the `-run` filter from the frontmatter
(the *package as a whole* needs live clusters — see Verification Steps — but none of these tests do),
so there is no excuse for an unverified red-to-green transition.

- `top/config_view_test.go::Test_walNextView` — three cases, modelled exactly on
  `Test_statioNextView` (`:670-683`): `wal` → `archiver`, `archiver` → `wal`, `unknown` → `wal`.
- `top/config_view_test.go::Test_switchViewTo` — extend the existing table (`:592-624`) with three
  cycle rows: `{current: "wal", to: "wal", want: "archiver"}`, `{current: "archiver", to: "wal", want:
  "wal"}`, `{current: "activity", to: "wal", want: "wal"}`. The existing row
  `{current: "sizes", to: "wal", want: "wal"}` (`:604`) **survives unchanged** — the cycle's default arm
  returns `"wal"` — do not "fix" it. `{current: "wal", to: "replication", want: "replication"}` (`:605`)
  is likewise unaffected.
- `top/menu_test.go::Test_selectMenuStyle` — add `{menu: menuWAL, want: 2}` to the table (`:8-25`,
  currently `menuNone:0, menuDatabases:2, menuPgss:7, menuProgress:6, menuConf:4, menuStatIO:2`).
- `top/help_test.go::Test_helpTemplate_walEntry` — using the existing `helpEntryLine` / `descColumn`
  helpers: the `'w' pg_stat_wal` marker appears on exactly one line; that line starts with
  `    w,W`; its description equals the exact string
  `'w' pg_stat_wal / pg_stat_archiver switch, 'W' WAL statistics menu.`; it sits immediately after the
  `j,J` line; and its `descColumn` equals the `j,J` line's `descColumn`.
- `top/help_test.go::Test_helpTemplate_resetCaveat` — the `'Q' does not reset shared stats` line lists
  `archiver` (assert on that single line, never on the whole template — a bare `archiver` substring
  would match anywhere).

## Acceptance Criteria

Per `patterns.md` ("Extract the decision out of the unreachable closure"), a test guarding an invariant
counts only after it has been seen red. Each mutation below must be applied, the suite run, the named
test observed failing, and the mutation reverted.

- [ ] `walNextView` exists with the three-arm shape; `Test_walNextView` passes
- [ ] **Mutation:** making `walNextView` return `"wal"` from its `case "wal":` arm turns
      `Test_walNextView` red **and** turns the `wal → archiver` row of `Test_switchViewTo` red
- [ ] **Mutation:** deleting the `case "wal":` arm from `switchViewTo` turns the
      `{current: "wal", want: "archiver"}` row of `Test_switchViewTo` red (the `archiver → wal` and
      `sizes → wal` rows stay green under this mutation — that is expected, and is why the first row
      is the one that proves the dispatch)
- [ ] `Test_switchViewTo` passes with the three new rows and with `:604`/`:605` unmodified
- [ ] `menuWAL` is declared inside the menu group of the iota block (above the blank line), so
      `moveUp`/`moveDown` remain in the directions group
- [ ] `selectMenuStyle(menuWAL)` returns a two-item menu; `Test_selectMenuStyle` passes
- [ ] **Mutation:** removing one item from the `menuWAL` style turns `Test_selectMenuStyle` red
- [ ] `menuSelect` has a `case menuWAL:` mapping cursor 0 → `wal`, 1 → `archiver`, default → `wal`,
      each through `viewSwitchHandler`, with exactly one `printCmdline` for the branch
- [ ] `{"sysstat", 'W', menuOpen(menuWAL, app.config, "")}` is registered and no other binding claims
      `'W'`; the existing `'w'` binding line is byte-identical to before
- [ ] The help screen carries the `w,W` line in the `j,J` format, directly after the `j,J` line; the
      `r,w` line has become `r` with only the `'r' replication,` clause
- [ ] **Mutation:** reverting the help text to the old `r,w  'r' replication, 'w' WAL,` line (or
      rewording the new entry's description) turns `Test_helpTemplate_walEntry` red
- [ ] The `Q`-does-not-reset line lists `pg_stat_io, bgwriter, wal, archiver`
- [ ] **Mutation:** dropping `archiver` from that line turns `Test_helpTemplate_resetCaveat` red
- [ ] `Test_helpTemplate_pauseEntry`, `Test_helpTemplate_pauseLiftingActions` and
      `Test_helpTemplate_formatVerbs` still pass unchanged (the pause tests key off *relative* line
      offsets, which an inserted line shifts uniformly; the format test counts `%`, which the new text
      must not disturb)
- [ ] `go test ./top/...` passes; `go build ./cmd/pgcenter` and `go vet ./top/...` are clean
- [ ] `make lint` reports no new findings in `top/`

## Context Files

**Feature artifacts:**
- [017-feat-wal-archiver.md](017-feat-wal-archiver.md) — user-spec (navigation section, lines 165-184;
  acceptance criteria for `w`, `W`, the `Msg` and the help screen, lines 266-274)
- [017-feat-wal-archiver-tech-spec.md](017-feat-wal-archiver-tech-spec.md) — tech-spec; Task 6 in
  Wave 3, Decision 5 (static `Msg`) and Decision 6 (the `"wal"` group name)
- [017-feat-wal-archiver-code-research.md](017-feat-wal-archiver-code-research.md) — §3.1 (the five-piece
  `j`/`J` precedent), §5.2 (every current test value), §10.D (the exact five edits)
- [017-feat-wal-archiver-decisions.md](017-feat-wal-archiver-decisions.md) — decisions log

**Project knowledge:**
- [overview.md](../../../.claude/skills/project-knowledge/overview.md) — what pgcenter is; this
  project has no `project.md`
- [architecture.md](../../../.claude/skills/project-knowledge/architecture.md) — package layout, the
  `top` package and view/collector data flow
- [patterns.md](../../../.claude/skills/project-knowledge/patterns.md) — "Extract the decision out of
  the unreachable closure" (the mutation rule above), "Adding a New View — test counts that must be
  updated" (the `top` tests that break on a new menu/cycle entry), "The cmdline: one composer,
  transient messages, persistent state" (one `printCmdline` per path)

**Code files to modify:**
- [top/config_view.go](../../../top/config_view.go) — `switchViewTo` (`:232-257`) gets the `case "wal":`
  arm with the Decision 6 comment; `walNextView` is added next to `statioNextView` (`:274-287`)
- [top/menu.go](../../../top/menu.go) — `menuWAL` in the iota block (`:14-26`), the `selectMenuStyle`
  branch after `case menuStatIO:` (`:87-95`), the `menuSelect` branch after `case menuStatIO:`
  (`:194-203`)
- [top/keybindings.go](../../../top/keybindings.go) — one new row after `:49` (`'J'`)
- [top/help.go](../../../top/help.go) — `helpTemplate`: line 14 (`r,w`), a new line after line 19
  (`j,J`), line 45 (the `Q` caveat)
- [top/config_view_test.go](../../../top/config_view_test.go) — `Test_switchViewTo` table (`:592-624`),
  new `Test_walNextView` next to `Test_statioNextView` (`:670-683`)
- [top/menu_test.go](../../../top/menu_test.go) — `Test_selectMenuStyle` table (`:8-25`)
- [top/help_test.go](../../../top/help_test.go) — two new tests reusing `helpEntryLine` (`:14-30`) and
  `descColumn` (`:35-48`)

**Code files to read for context:**
- [internal/view/view.go](../../../internal/view/view.go) — the `wal` entry (`:129-139`), the
  `stat_io`/`stat_io_time` pair (`:165-190`) and the `archiver` entry registered by Task 05; the
  archiver `Msg` is `Show archiver statistics (requires archive_mode=on)`
- [top/pause_test.go](../../../top/pause_test.go) — the comment block at `:552-577` explains why
  `menuSelect` itself is unreachable from a unit test; read it before attempting to test the new menu
  branch

## Verification Steps

1. Run `go test ./top/...` — passes, including `Test_walNextView`, `Test_switchViewTo`,
   `Test_selectMenuStyle`, `Test_helpTemplate_walEntry`, `Test_helpTemplate_resetCaveat` and the three
   pre-existing help tests. No PostgreSQL fixture is needed for this package.
2. Run the mutation checks listed in the Acceptance Criteria one at a time: apply the mutation, run
   `go test ./top/...`, confirm the *named* test fails, revert. Record in the decisions log that they
   were run and observed red — not that they "would" fail.
3. Run `go build ./cmd/pgcenter` — exits 0.
4. Run `go vet ./top/...` and `make lint` — no new findings.
5. Run `go test ./internal/view/... ./record/...` — unchanged by this task, confirming no accidental
   coupling was introduced.
6. Inspect the rendered help block by eye (or by printing `helpTemplate`): the `general actions:` block
   reads `a,b,f,o` / `r` / `s,t,i` / `d,D` / `x,X` / `p,P` / `j,J` / `w,W` / `S` …, with all
   descriptions in one column.

## Details

### Files

**`top/config_view.go`** — `switchViewTo` (`:232-257`) currently dispatches `databases`, `statements`,
`progress`, `statio`, and treats everything else as a direct view name via `default`. That default is
how `'w' → "wal"` works today. Add `case "wal":` alongside `case "statio":`; nothing above or below it
moves. `walNextView` goes next to `statioNextView` (`:274-287`) and follows the same
`var next string` / `switch` / `return next` shape and doc-comment form used by all four existing
cycle functions.

**`top/menu.go`** — the iota block (`:14-26`) holds six menu constants, then a blank line, then
`moveUp`/`moveDown` which share the same `iota` counter. Inserting `menuWAL` after `menuStatIO` shifts
`moveUp`/`moveDown` from 6/7 to 7/8; that is harmless because `direction` is a distinct type whose
values are only ever compared against themselves (`moveCursor`, `:268-312`). Inserting `menuWAL`
*below* the blank line is the mistake to avoid. `selectMenuStyle` (`:36-103`) and `menuSelect`
(`:140-…`) each get one branch copied from their `menuStatIO` sibling.

**`top/keybindings.go`** — the `keys` table (`:18-85`). Add the `'W'` row immediately after the `'J'`
row at `:49`, keeping the menu bindings grouped together.

**`top/help.go`** — `helpTemplate` is a raw string constant (`:10-50`) with exactly one `%s` verb,
consumed by `showHelp` via `fmt.Fprintf`. Three lines change; the current text is at `:14`, `:19` (the
insertion point is after it) and `:45`.

**`top/help_test.go`** — already carries the helper pair `helpEntryLine` (fails on an absent or
ambiguous marker) and `descColumn` (alignment is compared *between entries*, never against a magic
number). Both new tests must use them; do not introduce hard-coded column numbers.

### Dependencies

- **Task 05** must be done first: `Test_switchViewTo` builds its config with `newConfig()`
  (`top/config.go:52-59`), which calls `view.New()`. Without the registered `archiver` view, the
  `archiver` rows resolve to a zero-value `view.View` whose `Name` is `""` and the test fails for a
  reason that has nothing to do with this task.
- No new Go packages, no new imports in any of the four production files.

### Edge cases

- **`w` from an unrelated screen.** `walNextView`'s default arm returns `"wal"`, which preserves
  today's behaviour exactly and is what keeps the existing `sizes → wal` expectation green. Removing
  the default (or making it return `"archiver"`) is a user-visible regression.
- **The `Msg` must print exactly once per path.** The hotkey path prints it once, at
  `switchViewTo`'s single trailing `printCmdline`; the menu path prints it once, at the end of its
  `menuSelect` branch. Do not add a `printCmdline` inside the new dispatch case — two writes in one
  path leave only the last visible (`patterns.md`, the cmdline section), and this is precisely where
  the user-spec expects doubling to appear. Both paths are verified separately on the stand (Task 10).
- **`menuSelect` cannot be unit-tested.** Every branch ends in an unconditional `menuClose(g, v)`,
  which calls `g.DeleteView`/`g.SetCurrentView` on a `*gocui.Gui` that only `gocui.NewGui` can
  produce. Do not invent a seam or a test that "looks like proof" — `top/pause_test.go:552-577`
  documents an earlier attempt of exactly that class and forbids restoring it. The branch is covered
  by diff review plus the stand run.
- **`keybindings()` cannot be unit-tested either**, for the same reason (it needs a live
  `*gocui.Gui`). The `'W'`-is-free claim is a static one: grep the table for `'W'` before and after.
- **The new help line is 88 characters.** Its `j,J` neighbour is already 85, so it is consistent with
  the block. Only the `Space` entry has an enforced ≤80 rule (`top/help_test.go:78-79`); do not extend
  that rule to the new line, and do not shorten the `j,J` line to match.

### Implementation hints

- Copy, do not improvise: `statioNextView`, the `menuStatIO` style branch, the `menuStatIO` select
  branch and the `'J'` keybinding row are the four literal templates. Matching their shape is what
  makes this task reviewable at a glance.
- The menu window is sized `g.SetView("menu", 0, 5, 72, 6+len(s.items))` (`menu.go:116`), so two items
  give geometry identical to `menuStatIO` and the title fits inside 72 columns.
- Menu item strings carry a leading space by convention — every existing item has one.
- When writing `Test_helpTemplate_walEntry`, assert on the *description* returned by
  `entry[descColumn(entry):]`, as `Test_helpTemplate_pauseEntry` does, so padding is checked by the
  alignment assertion rather than baked into the string comparison.
- Before assuming anything in `top/` is untested, grep `top/*_test.go` for the function you are
  changing — `patterns.md` records that a previous feature's research wrongly assumed the TUI layer
  had no tests.

## Reviewers

- **dev-code-reviewer** → `docs/features/017-feat-wal-archiver/017-feat-wal-archiver-task-06-dev-code-reviewer-review.json`
- **dev-security-auditor** → `docs/features/017-feat-wal-archiver/017-feat-wal-archiver-task-06-dev-security-auditor-review.json`
- **dev-test-reviewer** → `docs/features/017-feat-wal-archiver/017-feat-wal-archiver-task-06-dev-test-reviewer-review.json`

## Post-completion

- [ ] Write a brief report to [017-feat-wal-archiver-decisions.md](017-feat-wal-archiver-decisions.md)
      (summary: 1-3 sentences, review rounds with links to the JSON reports, no findings tables or code
      dumps) — include which mutations were run and that each was observed red
- [ ] If deviated from spec — describe the deviation and reason
- [ ] Update user-spec/tech-spec if anything changed
