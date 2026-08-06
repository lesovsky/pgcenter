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
   `cmd/profile/profile.go:52` is an unrelated CLI flag on a different sub-command) — and this task
   turns that reading into `Test_keybindingsWAL`.
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
   Correct exactly those sentences to say that the blocker is narrower than stated — and do **not**
   replace one false claim with another: `editPgConfig` is *not* out of reach for want of a live
   `*postgres.DB`, because `top/pgconfig.go` returns early on `!db.Local`, so the `menuConf` branch is
   drivable with `&gocui.Gui{}` plus `&postgres.DB{Local: false}` — an idiom this package already uses
   in `top/config_view_test.go`. What genuinely needs a real environment is only the local-DB editor
   path beyond that early return. Nothing else in that comment block moves,
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
- `top/menu_test.go::Test_menuSelectWAL` — **the new one**: drives the real `menuSelect(app)` closure
  over a zero-value Gui, one sub-test per cursor position, asserting cursor 0 → `wal`, 1 → `archiver`,
  5 → `wal` (the default arm). Shape, all of it verified to work:

  ```go
  g := &gocui.Gui{}
  _, _ = g.SetView("sysstat", 0, 0, 20, 20)   // menuClose focuses it; without it menuSelect returns ErrUnknownView
  mv, err := g.SetView("menu", 0, 5, 72, 20)  // taller than production so SetCursor can reach the default arm
  // err == gocui.ErrUnknownView means "created" — treat any other error as fatal (menu.go:117 does the same)
  _ = mv.SetCursor(0, cy)                     // out-of-view rows are rejected with "invalid point", hence the height

  app := &app{config: newConfig(), ui: g}
  app.config.view = app.config.views["activity"]
  app.config.menu = selectMenuStyle(menuWAL)
  ```

  `viewCh` is unbuffered, so read it from a goroutine (with a timeout, as `Test_switchViewTo` and
  `pause_test.go` do) before calling the handler. Assert three things per case: the name arriving on
  `viewCh`, `assert.NoError` on the handler's return, and `menuNone` in `app.config.menu.menuType`
  afterwards — the reset at the end of `menuSelect` is what makes the next `Space`/menu press sane.
  Note the leak: `printCmdline` calls `g.Update`, which spawns a goroutine that parks forever on the
  zero Gui's nil `userEvents` channel — one per case, the same intentional class as
  `Test_showExtraCloseLifts` (`top/pause_test.go:589-591`); say so in a comment.
- `top/keybindings_test.go::Test_keybindingsWAL` — **the new file**: `app := &app{config: newConfig(),
  ui: &gocui.Gui{}}`, `require.NoError(t, keybindings(app))`, then use the exported
  `app.ui.DeleteKeybinding` as the probe: `("sysstat", 'W', gocui.ModNone)` succeeds once and returns
  `keybinding not found` on the second call (exactly one binding), and `'W'` on each of the other
  registered view names — `""`, `"menu"`, `"dialog"`, `"help"` — returns not-found (nobody else claims
  it). Delete-probing is destructive, so build a fresh `app` per assertion group or order the calls
  deliberately.
- `top/help_test.go::Test_helpTemplate_walEntry` — using the existing `helpEntryLine` / `descColumn`
  helpers. Use **`"'w' "`** (with the trailing space) as the marker, not `'w' pg_stat_wal`:
  `helpEntryLine` fails on an *ambiguous* marker, so this single call is what makes restoring the old
  `'w' WAL,` clause to the `r` line turn the test red. Then: the line starts with `    w,W`; its
  description equals the exact string
  `'w' pg_stat_wal / pg_stat_archiver switch, 'W' WAL statistics menu.`; it sits immediately after the
  `j,J` line; and its `descColumn` equals the `j,J` line's `descColumn`.
- `top/help_test.go::Test_helpTemplate_replicationEntry` — **the new one**, guarding the other half of
  the edit: the line found by marker `'r' replication,` starts with `    r ` (a space after the `r`,
  which the old `    r,w` prefix fails), contains no `'w'` at all, has exactly `'r' replication,` as its
  description, and its `descColumn` equals that of the `s,t,i` line (`'s' tables sizes` marker).
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
      each through `viewSwitchHandler`, with exactly one `printCmdline` for the branch;
      `Test_menuSelectWAL` passes on all three cursor positions
- [ ] **Mutation:** swapping the `case 0:` and `case 1:` targets of the `menuWAL` branch turns
      `Test_menuSelectWAL` red on both positions
- [ ] **Mutation:** deleting the `app.config.menu = selectMenuStyle(menuNone)` reset turns
      `Test_menuSelectWAL`'s `menuNone` assertion red
- [ ] `{"sysstat", 'W', menuOpen(menuWAL, app.config, "")}` is registered and no other binding claims
      `'W'`; `Test_keybindingsWAL` passes; the existing `'w'` binding line is byte-identical to before
- [ ] **Mutation:** adding a second `'W'` row to the `keys` table (any handler) turns
      `Test_keybindingsWAL` red on the "deletes exactly once" assertion; removing the `'W'` row
      turns it red on the first delete
- [ ] The help screen carries the `w,W` line in the `j,J` format, directly after the `j,J` line; the
      `r,w` line has become `r` with only the `'r' replication,` clause
- [ ] **Mutation:** restoring the `'w' WAL,` clause to the `r` line turns `Test_helpTemplate_walEntry`
      red — the `"'w' "` marker becomes ambiguous — **and** turns `Test_helpTemplate_replicationEntry`
      red on the no-`'w'` assertion
- [ ] **Mutation:** restoring the key token `r,w` (leaving the description alone) turns
      `Test_helpTemplate_replicationEntry` red on the `    r ` prefix assertion
- [ ] **Mutation:** rewording the new entry's description turns `Test_helpTemplate_walEntry` red
- [ ] The `Q`-does-not-reset line lists `pg_stat_io, bgwriter, wal, archiver`
- [ ] **Mutation:** dropping `archiver` from that line turns `Test_helpTemplate_resetCaveat` red
- [ ] `Test_helpTemplate_pauseEntry`, `Test_helpTemplate_pauseLiftingActions` and
      `Test_helpTemplate_formatVerbs` still pass unchanged (the pause tests key off *relative* line
      offsets, which an inserted line shifts uniformly; the format test counts `%`, which the new text
      must not disturb)
- [ ] `top/pause_test.go`'s comment block no longer claims that `menuSelect` as a whole is untestable;
      the deleted `Test_menuConfPathDoesNotLift` has *not* been restored
- [ ] `go test -race ./top/...` passes **inside the CI image** (on the host the package cannot be run
      whole: `Test_getQueryReport`, `top/report_test.go:12-14`, fails on a refused connection and then
      panics on a nil pointer, killing the test binary — environment, not this task's code)
- [ ] The host-side filtered run
      `go test ./top/ -run 'Test_walNextView|Test_switchViewTo|Test_selectMenuStyle|Test_menuSelectWAL|Test_keybindingsWAL|Test_helpTemplate'`
      passes
- [ ] `make build` (note: `go build ./cmd` fails — Go refuses to write an executable named `cmd` next to the `cmd/` directory; use `make build` or `go build -o /dev/null ./cmd` as a compile check) (that is the main package — `cmd/pgcenter.go`; the Makefile builds it as
      `go build … -o bin/pgcenter ./cmd`) and `go vet ./top/...` are clean
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
  (`:194-203`). Two seams the new test leans on: `menuOpen` (`:116-122`) treats
  `gocui.ErrUnknownView` from `SetView` as the "view was created" signal, and `menuClose` (`:234-243`)
  focuses `sysstat`, which is why the test registers such a view
- [top/keybindings.go](../../../top/keybindings.go) — one new row after `:49` (`'J'`)
- [top/help.go](../../../top/help.go) — `helpTemplate`: line 14 (`r,w`), a new line after line 19
  (`j,J`), line 45 (the `Q` caveat)
- [top/config_view_test.go](../../../top/config_view_test.go) — `Test_switchViewTo` table (`:592-624`),
  new `Test_walNextView` next to `Test_statioNextView` (`:670-683`)
- [top/menu_test.go](../../../top/menu_test.go) — `Test_selectMenuStyle` table (`:8-25`), plus the new
  `Test_menuSelectWAL`
- [top/keybindings_test.go](../../../top/keybindings_test.go) — **new file**, `Test_keybindingsWAL`
- [top/help_test.go](../../../top/help_test.go) — three new tests reusing `helpEntryLine` (`:14-30`) and
  `descColumn` (`:35-48`)
- [top/pause_test.go](../../../top/pause_test.go) — the two over-general sentences at `:561-568` are
  corrected (step 8); the rest of the block, and the deletion it records, stand

**Code files to read for context:**
- [internal/view/view.go](../../../internal/view/view.go) — the `wal` entry (`:129-139`), the
  `stat_io`/`stat_io_time` pair (`:165-190`) and the `archiver` entry registered by Task 05; the
  archiver `Msg` is `Show archiver statistics (requires archive_mode=on)`
- [top/ui_test.go](../../../top/ui_test.go) — `Test_cmdlineMarkerAfterUIRebuild` (`:406-425`), the
  project's existing zero-value `&gocui.Gui{}` idiom, and its comment on why a bare Gui is safe there

## Verification Steps

**On the environment, first.** `go test ./top/...` **cannot be run whole on the host**, before or
after this task: `Test_getQueryReport` (`top/report_test.go:12-14`) opens a test connection, fails on
connection refused, and then dereferences the nil connection — the panic takes the whole test binary
down. That is the documented state of this repo (clusters live on ports 21914-21919 inside the CI
image only), not something this task introduces or fixes. So the loop below is: filtered runs on the
host, one full run in the image.

1. **Host, inner loop.** Run
   `go test ./top/ -run 'Test_walNextView|Test_switchViewTo|Test_selectMenuStyle|Test_menuSelectWAL|Test_keybindingsWAL|Test_helpTemplate'`
   — passes. Every test this task adds or touches is in that set, and none of them needs a cluster:
   `Test_walNextView`, the extended `Test_switchViewTo`, `Test_selectMenuStyle`, `Test_menuSelectWAL`,
   `Test_keybindingsWAL`, the three new help tests and the three pre-existing ones.
2. Run the mutation checks listed in the Acceptance Criteria one at a time: apply the mutation, run the
   filtered command from step 1, confirm the *named* test fails, revert. Record in the decisions log
   that they were run and observed red — not that they "would" fail.
3. **CI image, full package.** Run the whole `top` package with the race detector inside the project's
   test image, which brings the clusters up:

   ```bash
   docker run --rm -v "$PWD":/work -v "$HOME/go/pkg/mod":/gomod \
     -e GOROOT=/gomod/golang.org/toolchain@v0.0.1-go1.25.12.linux-amd64 \
     -e GOPATH=/gopath -e GOFLAGS=-mod=mod -e HOME=/root \
     -w /work lesovsky/pgcenter-testing:0.0.11 bash -c '
   export PATH=$GOROOT/bin:$PATH
   prepare-test-environment.sh > /tmp/prep.log 2>&1 || { tail -30 /tmp/prep.log; exit 1; }
   go test -race -p 1 -timeout 300s ./top/... ./internal/view/... ./record/...
   '
   ```

   `./internal/view/...` and `./record/...` ride along unchanged by this task — a failure there means
   accidental coupling was introduced. (The `GOROOT` path tracks the local toolchain; check
   `go env GOROOT` if the image errors out on it.)
4. Run `make build` (note: `go build ./cmd` fails — Go refuses to write an executable named `cmd` next to the `cmd/` directory; use `make build` or `go build -o /dev/null ./cmd` as a compile check) — exits 0. (`./cmd` is the main package, `cmd/pgcenter.go`; there is no
   `./cmd/pgcenter` package.)
5. Run `go vet ./top/...` and `make lint` — no new findings. `golangci-lint` lives in
   `$(go env GOPATH)/bin`, which is not on the default PATH; without it `make lint` exits 127.
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
number). All three new help tests must use them; do not introduce hard-coded column numbers. The
`helpEntryLine` ambiguity check is not just hygiene here — it is the assertion that makes the
`'w' WAL,` clause unrestorable, so the marker must be `"'w' "` and not the longer `'w' pg_stat_wal`.

**`top/keybindings_test.go`** — does not exist yet; this task creates it with `Test_keybindingsWAL`.
It is the first test in the package to call `keybindings()`, so keep it small and self-explaining:
what makes it work is that `gocui.SetKeybinding` (`gui.go:249-259`) only appends to a slice and
`DeleteKeybinding` (`gui.go:261-275`) scans it, removes the first match and otherwise returns
`keybinding not found`. Neither touches a terminal.

### Dependencies

- **Task 05** must be done first: `Test_switchViewTo` and `Test_menuSelectWAL` build their config with
  `newConfig()` (`top/config.go:52-59`), which calls `view.New()`. Without the registered `archiver`
  view, the `archiver` cases resolve to a zero-value `view.View` whose `Name` is `""` and the tests
  fail for a reason that has nothing to do with this task.
- No new Go packages, and no new imports in any of the four production files. The two new/extended test
  files import `github.com/jroimartin/gocui` and testify, both already direct dependencies.

### Edge cases

- **`w` from an unrelated screen.** `walNextView`'s default arm returns `"wal"`, which preserves
  today's behaviour exactly and is what keeps the existing `sizes → wal` expectation green. Removing
  the default (or making it return `"archiver"`) is a user-visible regression.
- **The `Msg` must print exactly once per path.** The hotkey path prints it once, at
  `switchViewTo`'s single trailing `printCmdline`; the menu path prints it once, at the end of its
  `menuSelect` branch. Do not add a `printCmdline` inside the new dispatch case — two writes in one
  path leave only the last visible (`patterns.md`, the cmdline section), and this is precisely where
  the user-spec expects doubling to appear. `Test_menuSelectWAL` does **not** observe this: on a
  zero-value Gui the write inside `g.Update` never runs. The write count is a diff-review item, and
  what the user actually sees is verified on the stand (Task 10).
- **The zero-value Gui is the seam — do not invent another one.** `menuSelect` and `keybindings` are
  both driven directly, as described in the Description and the TDD Anchor. What remains genuinely
  out of reach is narrower and unrelated to this task: only the local-DB editor path inside
  `editPgConfig`, past its `!db.Local` early return — the branch itself is drivable with
  `&gocui.Gui{}` and `&postgres.DB{Local: false}`. Nothing here justifies threading new parameters through
  production signatures to make something observable, and nothing here justifies restoring the
  deleted `Test_menuConfPathDoesNotLift`, which asserted on a config its callee never received.
- **`menuOpen` is still not exercised.** The test drives `menuSelect` with a hand-built menu view and
  `app.config.menu` set from `selectMenuStyle(menuWAL)` — the same state `menuOpen` would leave — so
  the `'W'` → menu-window step itself (title, geometry, `menuDraw`) is covered by `Test_selectMenuStyle`
  plus the stand run in Task 10, not by `Test_menuSelectWAL`. Do not overstate what the new test proves
  in the decisions log.
- **The new help line is 89 characters.** Its `j,J` neighbour is already 86, so it is consistent with
  the block. Only the `Space` entry has an enforced ≤80 rule (`top/help_test.go:77-79`); do not extend
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
- The same rule applies to "untest*able*": before writing that a TUI handler cannot be covered, spend
  the two minutes on a throwaway test that calls it with a zero-value `&gocui.Gui{}` and see what
  actually happens. That is how both TUI tests in this task came to exist — the earlier revision of
  this file asserted the opposite, from reading rather than running.

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
