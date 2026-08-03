---
status: planned                    # planned -> in_progress -> done
depends_on: ["09"]                 # ID задач-зависимостей (строки: ["01", "02"])
wave: 6                            # волна параллельного выполнения
skills: [pre-deploy-qa]            # МАССИВ скиллов для загрузки
verify: bash                       # make test, make lint, make vuln, plus the stand run
reviewers: []                      # QA-задача: ревьюеров нет, отчёт сам является результатом
teammate_name:                     # имя агента-исполнителя (опционально; если не задано — генерируется по описанию задачи)
---

# Task 10: Pre-deploy QA

## Required Skills

Перед выполнением задачи загрузи:
- `/skill:pre-deploy-qa` — [skills/pre-deploy-qa/SKILL.md](~/.claude/skills/pre-deploy-qa/SKILL.md)

## Description

The acceptance gate for feature 016 before the merge. Tasks 1–9 built the pause: the non-mutating
cell printer, the flag and the `[PAUSED]` token, the gate plus the frame store and the shared render
core, the frozen header clock, lifting in the handlers that need fresh data, the help entry, repaint
on resize, the filter dialog, and the logtail freeze. This task walks every acceptance criterion by
name and reports a verdict with evidence for each.

The run has two halves, and neither substitutes for the other.

**Automated half** — `make test` (race detector), `make lint` (golangci-lint + gosec), `make vuln`
(govulncheck). This covers the criteria a unit test can pin: the collector completing ≥ 5 sends while
paused, the token's presence and position, the non-mutating cell, the stored timestamp, store
immutability, lifting per handler, the size-change helper, the logtail buffer.

**Manual half** — the stand run from the user-spec's «Как проверить» table, driven over ssh in tmux
per the `patterns.md` regimen. Everything that only exists on a live terminal lives here: that the
frame is genuinely frozen after minutes, that `[PAUSED]` is where it should be on a 60-column
terminal, that resize repaints under the new width, that no key on the main layout hangs the UI, and
that every lifting key lifts. There is no automated end-to-end harness for the TUI in this project —
the stand run *is* the E2E layer, and skipping it leaves those criteria unverified rather than
verified-by-inference.

pgcenter is a CLI utility: there is no deploy step, no services to bring up, no images to rebuild.
Phase 1 of the `pre-deploy-qa` skill (Environment Rebuild) collapses to a fresh `make build`.
"Pre-deploy" here means "ready to merge into develop", nothing more.

**This task fixes nothing.** A criterion that does not hold is recorded as FAIL with evidence, and
the feature does not pass the gate; the repair goes back to the task that owns the file, through its
own review cycle. That rule held for feature 015 and is not relaxed here.

## What to do

- Build fresh (`make build`) at the start of the session, before anything else. A stale binary
  silently invalidates every visual check that follows.
- Run `make test`, `make lint`, `make vuln`. Record the exit code and the summary of each. Confirm
  `-race` reports no race — the gate is the one place in the feature where a data race would be
  plausible, since the store is read from `g.Update` closures.
- Ask the project owner for the stand address at the start of the run. It is deliberately not stored
  in this repository: stands are ephemeral, with a TTL measured in hours, so a saved address is stale
  by the next session. Never assume the previous run's state survived.
- Take an inventory of the stand before planning the run — it is not a fixed image. Check for `tmux`,
  the PostgreSQL flavour, and which libraries are preloaded. Decide *before* writing the plan which
  steps the stand can actually support, not mid-run.
- Build a second binary from `master` and ship both. Run the same scenario on both and classify each
  difference as feature-introduced or pre-existing.
- Ship the binary under test explicitly and invoke it by absolute path. The stand carries a pgcenter
  built from `master`; running that one and reporting the result is the remote equivalent of testing
  a stale binary.
- Drive the run in tmux at fixed geometry (`-x 190 -y 52`), with a separate narrow pass (`-x 60`) for
  the token ladder. Capture with `capture-pane -p` for layout and content, and with
  `capture-pane -p -e` where attributes matter.
- Walk the stand-run table in the user-spec top to bottom — steps 1–11 including the sub-steps 3a and
  7a–7d. For each: what was sent, what was captured, PASS / FAIL / NOT VERIFIABLE, and why.
- Walk all 21 user-spec acceptance criteria and all 8 tech-spec acceptance criteria by name, marking
  each PASS / FAIL / NOT VERIFIABLE HERE with concrete evidence — a test name, a command exit code, a
  capture from the stand, or a line of code.
- Verify the containment criteria by inspection, not by trust: no production code changed outside
  `top/`, `view.View` gained no field, `record`/`report`/`profile` untouched, and the five
  `renderSysstat` call sites in `top/stat_test.go` updated mechanically with no assertion weakened.
- Leave the stand as found: reset any GUC changed for an experiment and verify with `SHOW`, restart
  the cluster if step 7b stopped it, and kill the tmux session at the end.
- Produce one explicit verdict: GO (ready to merge) or NEEDS WORK with the owning task listed for
  each FAIL.

## Acceptance Criteria

- [ ] `make build` succeeds; the binary used on the stand is the one just built, shipped explicitly
      and invoked by absolute path.
- [ ] `make test` green with the race detector; no race reported. Output recorded.
- [ ] `make lint` clean (golangci-lint + gosec).
- [ ] `make vuln` clean (govulncheck).
- [ ] All 21 user-spec acceptance criteria walked by name, each with a status and concrete evidence.
      Criteria that only exist on a terminal are backed by a stand capture, not by inference from a
      unit test.
- [ ] All 8 tech-spec acceptance criteria walked by name, including: no production code modified
      outside `top/`; `view.View` gains no field; `record`/`report`/`profile` untouched; every
      existing `top/` test still passes; the five `renderSysstat` call sites updated mechanically
      with no assertion weakened; the `statLoop` test proves ≥ 5 completed collector sends while
      paused; `↓` before the first frame does not crash the process.
- [ ] The stand-run table walked in full — steps 1–11 with sub-steps 3a and 7a–7d — each with what
      was sent, what was captured, and a status.
- [ ] The narrow pass (`-x 60`) was actually run, and the token ladder result recorded: `[PAUSED]`
      still visible, the filter indicator yielding space first.
- [ ] A second binary built from `master` was run on the same scenario, and every difference is
      classified as feature-introduced or pre-existing.
- [ ] The stand inventory (tmux, PostgreSQL flavour, preloaded libraries) is recorded, and any step
      the stand could not support is marked NOT VERIFIABLE with the reason — never PASS.
- [ ] The stand was left as found: GUCs reset and verified with `SHOW`, cluster restarted if stopped,
      tmux session killed.
- [ ] Explicit verdict: GO or NEEDS WORK with the owning task named for each FAIL.
- [ ] No code was changed by this task (`git status` on `top/` clean relative to the start of the
      run).

## Context Files

**Feature artifacts:**
- [016-feat-pause-display.md](016-feat-pause-display.md) — user-spec: «Критерии приёмки» (21 items),
  «Как проверить» (the stand-run table — the authoritative list of manual steps), «Ограничения»,
  «Риски»
- [016-feat-pause-display-tech-spec.md](016-feat-pause-display-tech-spec.md) — tech-spec:
  «Acceptance Criteria» (8 items), «Testing Strategy», «Agent Verification Plan» (per-task verify
  table), «Risks», «Backward Compatibility», Decisions 1–14
- [016-feat-pause-display-decisions.md](016-feat-pause-display-decisions.md) — decisions log: the
  reports of tasks 1–9, their review rounds and any recorded deviations. Read it before judging a
  criterion — a documented, accepted deviation is not a FAIL
- [016-feat-pause-display-code-research.md](016-feat-pause-display-code-research.md) — background on
  the render path, the collector loop and the aliasing chain; useful when a criterion needs to be
  judged against code rather than output

**Project knowledge:**
- [overview.md](../../../.claude/skills/project-knowledge/overview.md) — what pgcenter is and which
  stats it serves (there is no `project.md` in this repo; `overview.md` is its equivalent)
- [architecture.md](../../../.claude/skills/project-knowledge/architecture.md) — package layout, the
  collector → `statCh` → render data flow, PG version handling
- [patterns.md](../../../.claude/skills/project-knowledge/patterns.md) — **"Driving the TUI on a
  remote test stand"** (the regimen this task follows literally), "Testable TUI Rendering", "The
  cmdline: one composer, transient messages, persistent state", testing conventions
- [deployment.md](../../../.claude/skills/project-knowledge/deployment.md) — release process (confirms
  there is no deploy step for a CLI) and the `lesovsky/pgcenter-testing` container the test suite
  needs

**Code files (read only — change nothing):**
- [top/](../../../top/) — everything the feature touched: `stat.go`, `ui.go`, `pause.go`,
  `config.go`, `config_view.go`, `keybindings.go`, `dialog.go`, `extra.go`, `verbose.go`, `help.go`
  and their tests
- [top/stat_test.go](../../../top/stat_test.go) — the five `renderSysstat` call sites at `:60`,
  `:97`, `:192`, `:440`, `:441` (the sixth occurrence, `top/stat.go:259`, is the production caller)
- [Makefile](../../../Makefile) — targets `build`, `test`, `lint`, `vuln`, `dep`
- [docs/tech-debt.md](../../../docs/tech-debt.md) — [029] unsanitised row values, [030] the flaky
  `profile.Test_profileLoop`

## Verification Steps

- `make build` — builds `./bin/pgcenter` without errors. This is also the artifact shipped to the
  stand; note its mtime/size so a later "did I ship the right binary?" question has an answer.
- `make test` — green, `-race` silent. If `profile.Test_profileLoop` fails, re-run before filing it:
  it is registered tech debt [030], fails intermittently with `canceling statement due to user
  request`, and passes on an immediate re-run. Note the re-run in the report rather than quietly
  hiding the first failure.
- `make lint` — exit code 0, empty findings list.
- `make vuln` — exit code 0, no known vulnerabilities.
- `go test ./top/ -run '<name>' -v` pointwise for criteria that hinge on one test — and **read the
  test body**. A test existing under the expected name is not evidence that it checks the claimed
  thing; this matters most for the `statLoop` drain test, store immutability, the frozen clock, the
  lifting table and the logtail buffer.
- Containment checks: `git diff --stat` against the feature's base — every changed production file
  under `top/`; `git diff internal/stat/ record/ report/ profile/` empty; `grep -n "type View struct"
  -A 40 internal/view/view.go` compared against the base to confirm no field was added.
- `git diff top/stat_test.go` around the five `renderSysstat` call sites — the change is the added
  argument and nothing else; no assertion loosened, no golden value edited.
- The stand run: for each table row, the `send-keys` command, the resulting `capture-pane`, and the
  judgement. Attribute-sensitive rows use `-e`; layout rows use plain capture.
- `git status` at the end — the QA task changed no code.

## Details

**Files:** no code file is modified. The deliverables are the QA report
(`logs/working/qa-report.json`, per the skill; `logs/` is gitignored), the entry in
`016-feat-pause-display-decisions.md`, and the verdict in the agent's final message.

**Dependencies:**
- Tasks 1–9 must be `done` with their review cycles closed. Task 9 (logtail) is the direct
  dependency: stand step 7d cannot run before it lands.
- Local toolchain: Go 1.25+, golangci-lint, gosec, govulncheck (`make dep`).
- **A live PostgreSQL is required for `make test`.** None of the feature's own unit tests need one,
  but `make test` runs `go test ./...` and packages outside the feature open connections
  (`top/top_test.go` via `postgres.NewTestConnect()`, `top/report_test.go` panics on a nil
  connection). Clusters come from `lesovsky/pgcenter-testing:0.0.11`, ports 21914–21919 (see
  `deployment.md`). Check availability **before** the run — a missing cluster is an environment
  blocker, and calling it a criterion FAIL would be wrong.
- A stand is required and its address must be requested from the project owner. There is no fallback:
  the terminal-only criteria cannot be satisfied any other way, and marking them PASS without a
  capture is the failure mode this task exists to prevent.

**Stand regimen (from `patterns.md` → "Driving the TUI on a remote test stand"):**
1. Ask the owner for the address at the start of the run. Do not record it anywhere afterwards.
2. Inventory first: is `tmux` installed (installing it is fine — these stands carry passwordless
   sudo); which PostgreSQL flavour; which `shared_preload_libraries`. Observed variety so far:
   Ubuntu 24.04 with tmux preinstalled, and Debian 12 with no tmux, no Go and a Postgres Pro build
   without `pg_stat_statements`.
3. `make build` locally, then `scp` both binaries (feature and `master`) and invoke each by absolute
   path. Give them distinct names so a capture can never be attributed to the wrong build.
4. `tmux new-session -d -s cap -x 190 -y 52` for the main pass; a second session at `-x 60` for the
   narrow pass. Fixed geometry is the entire reason for using tmux.
5. `send-keys` a hotkey, wait at least one refresh interval, then `capture-pane`. Plain capture for
   layout/content/row counts; `-e` when colour or attributes are the thing being checked. Stripping
   ANSI for a diff: `sed 's/\x1b\[[0-9;]*m//g'`.
6. Leave the stand as found: reset any changed GUC and verify with `SHOW`; restart the cluster if
   step 7b stopped it; `tmux kill-session` at the end.

**Edge cases:**
- **`profile.Test_profileLoop` fails.** Known flaky, tech debt [030], in a package this feature never
  touches. Re-run before filing it as a regression. Report it as "flaky, re-run green" — do not
  silently drop it and do not call the suite red because of it.
- **Step 7 exceptions that are not defects.** `S` on a remote connection and `L` with an unreachable
  log file return early and therefore do **not** lift the pause. This is the specified behaviour
  (lifting is placed after the early returns, Decision 9), and on most stands both will trigger.
  Record them as expected, not as FAIL.
- **`X` needs `pg_stat_statements`.** If the stand's `shared_preload_libraries` lacks it, the
  statements menu is unavailable and that lifting key cannot be exercised. NOT VERIFIABLE with the
  reason recorded — never PASS by analogy with `D`/`P`/`J`.
- **Menu `E` is deliberately excluded from the lifting set.** Selecting a config in the config menu
  goes to the editor and the pause survives. Do not file it as a missing lift.
- **The blackholed-connection limitation is not reproducible in acceptance.** The tech-spec Risks
  table records it explicitly: while paused, a `viewCh`-pushing key blocks until the collector
  answers, and on a silently dropped connection the frozen screen stops responding to every key
  including `Space`. A stopped cluster (step 7b) closes the socket and returns an error immediately,
  so the UI stays responsive — that step does **not** reproduce it. Reproducing it needs a blackholed
  connection and the only exit is killing the process. Report it as a known, unobserved limitation.
  Neither claim it covered nor file it as a new defect.
- **A capture is missing for a terminal-only criterion.** Status is NOT VERIFIED, not PASS. This is
  the single most likely way a bad gate passes.
- **A difference appears on the feature binary but also on the `master` binary.** Pre-existing, not a
  regression. In feature 015 this reclassified three of five findings; without the second binary they
  would have been filed against the feature.
- **`make lint` complains about code the feature never touched.** Note it separately: not a blocker
  for the feature, but not grounds for declaring lint clean either.
- **The prompt in dialogs is ~8 columns shorter while paused**, so it truncates earlier on a narrow
  terminal. This is an accepted trade-off written into the user-spec, not a defect.
- **The `Space`-before-first-frame case leaves the screen blank.** That is the documented exception:
  there is nothing to repaint, so the screen stays empty with the marker on. PASS means the marker is
  there and the screen is empty — not that a frame appears.

**Implementation hints:**
- Count criteria from the documents, not from memory: 21 checkboxes in the user-spec «Критерии
  приёмки», 8 in the tech-spec «Acceptance Criteria». The tech-spec calls the manual run "the 15-step
  stand run"; the table in the user-spec actually has steps 1–11 plus sub-steps 3a and 7a–7d. Walk
  the rows of the table — do not trust either number.
- The five `renderSysstat` call sites are in `top/stat_test.go` (`:60`, `:97`, `:192`, `:440`,
  `:441`); `top/stat.go:259` is the production caller. Verify by grep against the working tree rather
  than by trusting the spec — a count mismatch with a compiling build is a note against the spec text,
  not a FAIL.
- Step 3a is the criterion the whole feature rests on and it needs real waiting: pause on a screen
  with fast-moving numbers, wait ≥ 3 minutes, force a repaint with `[`, and compare against the
  capture taken at pause time. Do not shorten it — a store that is silently republished on repaint
  looks perfect at 5 seconds.
- Step 11 (every key of the main layout, one at a time, with a capture between presses) is the guard
  against the feature's worst failure mode: a UI that hangs and can only be escaped by killing the
  process. Include `Space` itself, and the keys that push through `viewCh`.
- The keybinding table has no unit-test coverage — the stand run is the only thing that proves
  `Space` is bound at all, and it is bound scoped to the `sysstat` view. Step 7c (a space typed into
  the filter input) is the other half of that check.
- Keep the report compact: a "criterion → status → evidence" table plus the verdict. Command output
  dumps do not belong in the decisions log; the exit code and a one-line summary are enough.

## Reviewers

None — a QA task carries no reviewers by the catalogue; the report *is* the deliverable. The result
is accepted by the project owner at feature acceptance.

## Post-completion

- [ ] Записать краткий отчёт в [016-feat-pause-display-decisions.md](016-feat-pause-display-decisions.md) (Summary: 1-3 предложения, таблица критериев со статусами, вердикт GO/NEEDS WORK, ссылка на `logs/working/qa-report.json`; без дампов вывода команд)
- [ ] Record the finding for tech debt **[029]** (unsanitised row values): the feature adds two new
      persistence consequences — a hostile row value now survives on a frozen screen, and across a
      pager return; the same applies to the stored logtail buffer. **Record only.** Editing
      `docs/tech-debt.md` belongs to `/done`, which owns that register; this task neither edits
      documents nor carries reviewers.
- [ ] Если отклонились от спека — описать отклонение и причину
- [ ] Обновить user-spec/tech-spec если что-то изменилось
