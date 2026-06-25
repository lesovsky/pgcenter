---
status: planned                    # planned -> in_progress -> done
depends_on: []                     # ID задач-зависимостей (строки: ["01", "02"])
wave: 1                            # волна параллельного выполнения
skills: [code-writing]             # МАССИВ скиллов для загрузки
verify: bash                       # инструмент верификации (опционально: curl, bash, user)
reviewers: [dev-code-reviewer, dev-test-reviewer]  # явно указать. Пусто = fallback на defaults
teammate_name:                     # имя агента-исполнителя (опционально; если не задано — генерируется по описанию задачи)
---

# Task 02: Verbose toggle plumbing

## Required Skills

Перед выполнением задачи загрузи:
- `/skill:code-writing` — [skills/code-writing/SKILL.md](~/.claude/skills/code-writing/SKILL.md)

## Description

Wire up the on/off plumbing for the new verbose display mode. This is the control-plane task for the
whole feature: it introduces the single boolean that turns the verbose top panels on and off, the `v`
hotkey that flips it, and the wiring that carries the flag to both consumers (the renderer/layout read it
off `top.config`; the collector reads it off `view.View`). No rendering and no new queries here — later
tasks (3, 6, 8, 9) consume this flag.

The mode follows the existing **`showExtra` mirror-into-all-views pattern** (`top/extra.go:70-75`): the
handler writes the flag onto every entry in `config.views` plus `config.view`, then pushes on `viewCh`.
Writing it onto every map entry is what makes the flag survive a screen switch (`viewSwitchHandler`
reloads the active view from `config.views`, `config_view.go:241-242`). Because `verbose` is simply never
zeroed on switch (unlike `scrollOffset`, which IS reset at `config_view.go:243`), persistence comes for
free — no change to `viewSwitchHandler` is required. This is Decision 2 in the tech-spec (a dedicated bool
on both `view.View` and `top.config`, not an overload of the mutually-exclusive `CollectExtra` int).

The critical, non-obvious part is the `collectStat()` change-detection in `top/stat.go`. The `viewCh`
receive branch contains **two** `c.Reset()` calls: a **conditional** one in the `CollectExtra`-change path
(~`top/stat.go:101`, inside `if prevCollectExtra != v.CollectExtra`) and an **unconditional** one further
down (~`top/stat.go:108`) that always wipes the collector's `prev*` snapshots and re-runs `Update`. If a
bare `v` press falls through, the unconditional Reset (and, depending on ordering, the conditional one)
would blank the CPU/mem/load deltas for one frame (they are computed against the now-wiped previous
snapshot). The fix (Decision 2 / Risks row) is to add a `prev.Verbose != v.Verbose → continue` early-out
branch that precedes **both** Reset paths — i.e. placed early enough in the `viewCh` case (alongside the
existing `ShowExtra` branch at ~`top/stat.go:89-93`, before the `CollectExtra` check). When only the
verbose flag changed, update the tracked value and `continue` so the loop skips every Reset path and keeps
emitting correct deltas.

(All `top/stat.go` line numbers in this task are approximate and will drift as the file changes — locate
the branches by their surrounding code, not by exact line.)

## What to do

1. Add `Verbose bool` field to `view.View` (`internal/view/view.go`), documented like the existing
   `ShowExtra` field. Zero value `false` = current compact behavior (no behavior change for existing code).
2. Add `verbose bool` field to `top.config` (`top/config.go`), documented as persistent state that — unlike
   `scrollOffset` — is NOT reset on screen switch.
3. Create `top/verbose.go` with a `toggleVerbose(app *app)` handler returning the gocui
   `func(g *gocui.Gui, v *gocui.View) error` signature. It must:
   - flip the mode: compute the new boolean (`!app.config.verbose`);
   - mirror it into every entry of `app.config.views`, into `app.config.view.Verbose`, and into
     `app.config.verbose` (the `showExtra` write-into-all-views pattern, `top/extra.go:70-75`);
   - push `app.config.view` on `app.config.viewCh`;
   - print a cmdline status message (e.g. `Verbose mode: on` / `Verbose mode: off`) via `printCmdline`.
4. Register the `v` keybinding in `top/keybindings.go` on the `"sysstat"` view → `toggleVerbose(app)`,
   alongside the other `sysstat` bindings. (`v` and `V` are confirmed free — code-research §2-new.)
5. Add a one-line help entry for `v` to `helpTemplate` in `top/help.go` (a short verbose-mode line; the
   "extra stats actions" block near the `B,N,F,L` entry is the natural home, or a new line in the same
   block).
6. In `top/stat.go`, inside the `case v = <-viewCh:` block of `collectStat()`, add a verbose
   change-detection branch placed early enough to precede **both** Reset paths — the conditional
   `c.Reset()` in the `CollectExtra`-change check (~`top/stat.go:100-103`) AND the unconditional
   `c.Reset()` further down (~`top/stat.go:108`). Put it alongside the `ShowExtra` branch
   (~`top/stat.go:89-93`), before the `CollectExtra` check: track the previous verbose value (a local like
   `prevVerbose := v.Verbose`, initialized next to `prevCollectExtra` at ~`top/stat.go:57`), and when
   `prevVerbose != v.Verbose`, update it and `continue` — so a verbose-only toggle does NOT fall through to
   either `Reset()`. Do not change the refresh/extra/CollectExtra branches. (Cited line numbers are
   approximate and will drift.)

## TDD Anchor

Tests written BEFORE implementation, in `top/verbose_test.go` (new), modeled on `Test_toggleIdleConns`
(`top/config_view_test.go:617`):

- `top/verbose_test.go::Test_toggleVerbose` — given a `newConfig()` with an active view, calling the
  `toggleVerbose` handler flips `config.verbose` from false→true→false, sets `config.view.Verbose` to the
  same value, mirrors `Verbose` onto every entry in `config.views` (so a later view switch preserves it),
  and pushes the updated `view.View` on `config.viewCh` (drain it in a goroutine, asserting `v.Verbose`
  matches the new flag — the `Test_toggleIdleConns` goroutine-drain shape).

(The `collectStat()` no-Reset behavior is verified by the existing `top/...` test run plus manual QA;
`collectStat` itself has no isolated unit test harness — assert the toggle semantics via the handler test
above and confirm `go test ./top/...` stays green.)

## Acceptance Criteria

- [ ] `view.View` has a `Verbose bool` field; `top.config` has a `verbose bool` field; both default false.
- [ ] `toggleVerbose` flips `config.verbose`, mirrors `Verbose` into all `config.views` entries +
      `config.view`, pushes on `viewCh`, and prints a cmdline status message.
- [ ] `v` is bound on the `sysstat` view to `toggleVerbose`; help text documents `v`.
- [ ] The verbose flag persists across screen switches (mirrored into the views map; not reset in
      `viewSwitchHandler`).
- [ ] `collectStat()` has a `prev.Verbose != v.Verbose → continue` branch placed before BOTH Reset paths
      (the conditional `CollectExtra` Reset and the unconditional Reset), so a verbose-only `viewCh` push
      does NOT trigger any `c.Reset()` (CPU/mem/load deltas are not blanked on toggle).
- [ ] `Test_toggleVerbose` passes; `go test ./top/...` is green; no existing keybinding/view-count test
      regressions.
- [ ] `make lint` clean for the touched files.

## Context Files

**Feature artifacts:**
- [010-feat-overview-dashboard.md](docs/features/010-feat-overview-dashboard/010-feat-overview-dashboard.md) — user-spec
- [010-feat-overview-dashboard-tech-spec.md](docs/features/010-feat-overview-dashboard/010-feat-overview-dashboard-tech-spec.md) — tech-spec (Task 2, Decision 2)
- [010-feat-overview-dashboard-decisions.md](docs/features/010-feat-overview-dashboard/010-feat-overview-dashboard-decisions.md) — decisions log
- [010-feat-overview-dashboard-code-research.md](docs/features/010-feat-overview-dashboard/010-feat-overview-dashboard-code-research.md) — §2-new (verbose toggle plumbing end-to-end)

**Project knowledge:**
- [overview.md](.claude/skills/project-knowledge/overview.md) — project context (no project.md in this repo)
- [architecture.md](.claude/skills/project-knowledge/architecture.md) — package layout, data flow, viewCh seam
- [patterns.md](.claude/skills/project-knowledge/patterns.md) — testing conventions, error wrapping, manual QA phase

**Code files:**
- [internal/view/view.go](internal/view/view.go) — add `Verbose bool` field to `View` struct
- [top/config.go](top/config.go) — add `verbose bool` field to `config` struct
- [top/verbose.go](top/verbose.go) — NEW: `toggleVerbose` handler
- [top/keybindings.go](top/keybindings.go) — register `v` → `toggleVerbose(app)` on `sysstat`
- [top/help.go](top/help.go) — add `v` help line to `helpTemplate`
- [top/extra.go](top/extra.go) — reference: `showExtra` mirror-into-all-views pattern (lines 70-75)
- [top/config_view.go](top/config_view.go) — reference: `toggleIdleConns` (411-435) boolean-toggle precedent; `viewSwitchHandler` (240-245) `scrollOffset` reset (verbose must NOT be reset here)
- [top/stat.go](top/stat.go) — `collectStat()` viewCh handler: add the no-Reset verbose branch (mirror ShowExtra branch at ~89-93), placed before BOTH the conditional CollectExtra Reset (~100-103) and the unconditional Reset (~108); line numbers approximate

## Verification Steps

- Run `go test ./top/...` — `Test_toggleVerbose` passes; all existing `top` tests green (keybindings,
  view counts, toggles unaffected).
- Inspect: `collectStat()` has the verbose change-detection `continue` branch ahead of BOTH the conditional
  `CollectExtra` Reset and the unconditional `c.Reset()`; the verbose-toggle path does not reach any Reset.
- `make lint` clean on touched files.
- (Manual, deferred to Final Wave QA) `v` flips the mode, status line shows it, and CPU/mem deltas in the
  sysstat panel do not blink on toggle.

## Details

**Files:**
- `internal/view/view.go` — `View` struct (lines 10-31). Add `Verbose bool` with a doc comment next to
  `ShowExtra`/`CollectExtra`. The flag rides `viewCh` to the collector.
- `top/config.go` — `config` struct (lines 10-20). Add `verbose bool`. Document it as persistent (NOT
  ephemeral like `scrollOffset` on line 19). `newConfig()` needs no change (zero value false is correct).
- `top/verbose.go` — NEW file, package `top`. Single `toggleVerbose(app *app) func(g *gocui.Gui, v
  *gocui.View) error`. Imports `github.com/jroimartin/gocui`. Mirror the `showExtra` write loop
  (`top/extra.go:70-75`) exactly: `for k, v := range app.config.views { v.Verbose = newVal;
  app.config.views[k] = v }`, then `app.config.view.Verbose = newVal`, `app.config.verbose = newVal`,
  `app.config.viewCh <- app.config.view`, then `printCmdline(g, ...)`. Unlike `showExtra` there is no
  separate gocui view to open/close — verbose is rendered into the existing `sysstat`/`pgstat` panels
  (handled in later tasks), so do NOT call `openExtraView`/`SetView`.
- `top/keybindings.go` — add `{"sysstat", 'v', toggleVerbose(app)}` to the `keys` slice (alongside the
  other lowercase `sysstat` bindings, e.g. near the `B/N/F/L` showExtra bindings at lines 53-56).
- `top/help.go` — `helpTemplate` const (lines 10-46). Add a one-line `v` entry (e.g. in the "extra stats
  actions" block) describing the verbose toggle. Keep the existing alignment style.
- `top/stat.go` — `collectStat()` (~lines 25-123; numbers approximate, will drift). Two edits: (1) near
  line 57 add `prevVerbose := v.Verbose` next to `prevCollectExtra := v.CollectExtra`; (2) in the
  `case v = <-viewCh:` block, add — in the same style as the `ShowExtra` branch (~89-93) and positioned to
  precede BOTH Reset paths (i.e. before the `CollectExtra` check at ~100-103 with its conditional
  `c.Reset()`, and thus also before the unconditional `c.Reset()` at ~108) —
  `if prevVerbose != v.Verbose { prevVerbose = v.Verbose; continue }`. A verbose-only toggle then returns
  to the top of the loop without hitting either Reset, so the next `Update` still has valid `prev*` data.

**Dependencies:** None — Wave 1, `depends_on: []`. Tasks 3, 6, 8, 9 consume the flag this task introduces.
stdlib + existing imports only.

**Edge cases:**
- Verbose toggled on, then user switches screens repeatedly → flag stays on (mirrored into all views map
  entries; never zeroed in `viewSwitchHandler`). Add/keep this guarantee by writing into every map entry.
- Toggling verbose must NOT blank CPU/mem/load deltas — guaranteed only by the no-Reset `continue` branch
  in `collectStat()`, which must sit ahead of BOTH the conditional `CollectExtra` Reset and the
  unconditional Reset. This is the single highest-risk spot in the task (tech-spec Risks row).
- `viewSwitchHandler` resets `scrollOffset` (line 243) — do NOT add a verbose reset there; verbose is
  persistent by design.
- The `collectStat` first-update block (lines 41-57) already reads the initial view; `prevVerbose` must be
  seeded from that initial `v` so the first real toggle is detected.

**Implementation hints:**
- Use `printCmdline` (already used across handlers, e.g. `top/extra.go:77`, `top/config_view.go:428`) for
  the status message; it owns cmdline mutual-exclusion (patterns.md "printCmdline — Mutual Exclusion").
- `toggleVerbose` does not need to be scoped to a specific view (unlike `toggleIdleConns`, which guards on
  `activity`/`procpidstat`): verbose applies to the top panels on every screen, so no `config.view.Name`
  guard.
- Follow error-wrapping conventions only where an error can occur; `toggleVerbose` mirrors `showExtra`'s
  no-error-on-the-mirror-loop shape and returns `nil` on the happy path.
- Keep the `collectStat` edit minimal and self-explanatory with a short comment (mirror the existing
  `ShowExtra` branch comment style); do not reorder or touch the refresh/extra/CollectExtra branches.

## Reviewers

- **dev-code-reviewer** → `docs/features/010-feat-overview-dashboard/010-feat-overview-dashboard-task-02-dev-code-reviewer-review.json`
- **dev-test-reviewer** → `docs/features/010-feat-overview-dashboard/010-feat-overview-dashboard-task-02-dev-test-reviewer-review.json`

## Post-completion

- [ ] Записать краткий отчёт в [010-feat-overview-dashboard-decisions.md](docs/features/010-feat-overview-dashboard/010-feat-overview-dashboard-decisions.md) (Summary: 1-3 предложения, ревью со ссылками на JSON, без таблиц файндингов и дампов)
- [ ] Если отклонились от спека — описать отклонение и причину
- [ ] Обновить user-spec/tech-spec если что-то изменилось
