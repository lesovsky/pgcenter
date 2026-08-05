---
status: planned                    # planned -> in_progress -> done
depends_on: ["01", "02", "03", "04", "05", "06", "07", "08", "09"]
wave: 4                            # волна параллельного выполнения
skills: [pre-deploy-qa]            # МАССИВ скиллов для загрузки
verify: bash, user                 # full make test in the CI image, lint + vuln on the host, then the manual stand run
reviewers: []                      # QA-задача: ревьюеров нет, отчёт сам является результатом
teammate_name:                     # имя агента-исполнителя (опционально; если не задано — генерируется по описанию задачи)
---

# Task 10: Pre-deploy QA

## Required Skills

Перед выполнением задачи загрузи:
- `/skill:pre-deploy-qa` — [skills/pre-deploy-qa/SKILL.md](~/.claude/skills/pre-deploy-qa/SKILL.md)

## Description

The acceptance gate for feature 017 before the merge. Tasks 1–9 built four independent pieces inside
the WAL/archiving area: the archiver query and its selector, the PG 19 `fpi,KiB` column, the verbose
panel's move to `pg_ls_archive_statusdir()`, the `-W` string flag, the view registration, the `w`/`W`
navigation, the describe text, the golden replays, and the 0.12.0 release notes. This task walks
**every** acceptance criterion by name — 23 in the user-spec, 11 in the tech-spec — and returns a
verdict with evidence for each. Sampling is not acceptance: a criterion that is not walked is not
verified.

The run has two halves, and neither substitutes for the other.

**Automated half.** The full suite (`go test -race -p 1 -timeout 300s ./...`) inside the project CI
image `lesovsky/pgcenter-testing:0.0.11` with the PG 14–19 fixtures up, plus `make lint`
(golangci-lint + gosec) and `make vuln` (govulncheck) on the host. This covers everything a test can
pin: the 9-column archiver query on every version, the PG 19 wal layout, the `SET ROLE` privilege
checks in both directions, the view registration, the count-based tests, the CLI mapping, the
describe text and the golden replays.

**Manual half.** The stand run from the user-spec's «Как проверить» → «Пользователь проверяет»
section, driven over ssh in tmux per the `patterns.md` regimen. Everything that only exists against a
live archiver or a live terminal lives here, and the test fixtures **cannot** substitute: they run
`archive_mode=off` with no `archive_command` (verified live — `testing/prepare-test-environment.sh`
writes no archiving GUCs), so every integration test sees an all-zero, all-NULL archiver row. Working
archiving, broken archiving, the navigation cycle, the once-per-path subtitle, the 60-column frozen
column and the cost measurement have no other evidence source.

pgcenter is a CLI utility: there is no deploy step, no services to bring up, no images to rebuild.
Phase 1 of the `pre-deploy-qa` skill (Environment Rebuild) collapses to a fresh `make build` plus
bringing the CI image's fixtures up. "Pre-deploy" here means "ready to merge into develop", nothing
more.

**This task fixes nothing and changes no code.** A criterion that does not hold is recorded as FAIL
with evidence, and the feature does not pass the gate; the repair goes back to the task that owns the
file, through its own review cycle. The one exception is the pre-agreed outcome of the cost
measurement (Decision 9), which this task *decides* but does not implement.

## What to do

- Build fresh (`make build`) at the start of the session, before anything else. A stale binary
  silently invalidates every check that follows — cherry-picks and mid-session changes do not update
  `./bin/pgcenter`.
- Run the full suite inside the CI image with the PG 14–19 fixtures up, using the invocation in
  Details. Record the exit code, the pass/fail/skip counts and the race-detector result. **Zero
  races.**
- Run `make lint` and `make vuln` on the host, with `golangci-lint` on PATH (see Details — without it
  the target exits 127 and looks like a broken Makefile rather than a missing tool).
- Verify that no existing test's expectation was weakened to make the suite pass. Read the diff of
  every count-based test named in the tech-spec — `TestNew`, `TestView_VersionOK`, `Test_filterViews`,
  `Test_selectMenuStyle`, `Test_switchViewTo` — and confirm each carries a new **correct** number, not
  a deleted assertion, a loosened comparison or a removed row.
- Confirm the stand address with the project owner at the start of the run. The recorded address
  (`pgpro@10.128.31.96`) has a 24h TTL from 2026-08-05 and is very likely stale. **If the stand has
  expired, say so and ask for a new one — do not silently skip the manual gate.** There is no
  fallback: the archiving, navigation, narrow-terminal and cost criteria cannot be satisfied any other
  way, and marking them PASS without a capture is the failure mode this task exists to prevent.
- Take an inventory of the stand before planning the run: `tmux` present?, PostgreSQL flavour and
  version (`pg_ls_archive_statusdir()` needs PG 12+), `shared_preload_libraries`, where `$PGDATA` and
  `pg_wal/archive_status` live, and whether the login user can write there or needs sudo. Decide
  *before* writing the plan which steps the stand can support, not mid-run.
- Build a second binary from `master`, ship both under distinct names, and invoke each by absolute
  path. The stand carries a `master`-built pgcenter; running that one and reporting the result is the
  remote equivalent of testing a stale binary. The A/B binary is what separates a regression
  introduced by this feature from behaviour that was always broken.
- Drive everything in tmux at fixed geometry (`tmux new-session -d -s cap -x 190 -y 52`), with a
  second session at `-x 60` for the narrow pass. Capture with `capture-pane -p` for layout and
  content, `capture-pane -p -e` wherever colour or an attribute is the thing being checked (the frozen
  column is bold — without `-e` a missing attribute is invisible).
- Run the stand scenarios, all of them:
  - **Working archiving.** `archive_mode=on`, `archive_command='/bin/true'`, restart, then three
    `pg_switch_wal()` calls. Check `archived` grew by ≥ 3, `archived_age` ≤ 00:00:30, `last_archived`
    holds the last segment name, `ready` returned to 0, `failed` unchanged.
  - **Broken archiving.** `archive_command='/bin/false'`, reload, three more switches. Check `failed`
    grows tick to tick (two captures one interval apart), `ready` ≥ 3 and not falling, `last_failed`
    holds a segment name, `failed_age` ≤ 00:00:30, `archived` unchanged.
  - **Archiving off.** `archive_mode=off`, restart. Check the four age/name cells are **blank, not
    zeros and not dashes**, and that the screen subtitle is on the cmdline.
  - **Navigation.** The `w` cycle in both directions (`wal → archiver`, `archiver → wal`, and `w` from
    a third screen landing on `wal`), the two-item `W` menu with both items, and the help screen's
    `w,W` line plus `archiver` in the `Q` line.
  - **The subtitle, once per path, on each path separately.** Enter `archiver` by the `w` hotkey,
    capture, count the occurrences of `Show archiver statistics (requires archive_mode=on)`. Then
    leave, enter again through the `W` menu, capture, count again. These are two different call sites
    (`switchViewTo` vs `menuSelect` → `viewSwitchHandler`) and this is exactly where a duplicate would
    appear — checking one path proves nothing about the other.
  - **Narrow terminal.** A 60-column session: the `source` column stays frozen and the remaining
    columns are reachable by horizontal scroll (`[` / `]`).
  - **Privilege degradation on a terminal.** With the `pg_monitor`-less role of the cost run: the
    `archiver` screen shows the PostgreSQL error text instead of a table, pgcenter does not crash and
    retries on the next tick; the `wal` screen behaves as it did before the feature (A/B against the
    `master` binary). `pgcenter record` under the same role stops with an error — record it as the
    existing recorder behaviour, not as a defect of this feature.
- Run the **cost measurement** under the conditions Decision 9 fixes — a measurement taken any other
  way measures the wrong baseline and is worthless:
  - a role holding **only `pg_monitor`**, not a superuser (for a superuser the panel already pays one
    walk, so the change reads as 1× → 2×; for `pg_monitor` today's aggregate fails instantly with
    42501 and costs nothing, so the real change is zero → full);
  - verbose mode on (`v`), on a screen that is *not* `archiver` — the panel now pays the walk on every
    screen;
  - a concurrent `pgcenter record` running against the same cluster (the recorder runs every
    registered view's query every tick regardless of which screen is displayed);
  - `archive_mode=off`, so the archiver does not drain the directory mid-run;
  - ~200 000 **empty dummy files** named `0000000100000000000000NN.ready` in `archive_status/`. Real
    WAL segments are neither needed nor allowed here — 200 000 segments is ~3 TiB.
  - Measure three things: (1) the wall time of `SELECT count(*) FILTER (WHERE name LIKE '%.ready')
    FROM pg_ls_archive_statusdir()`; (2) the view-switch latency — with `archiver` open, press `w` and
    time the screen change (the unbuffered `viewCh` blocks the switch behind the collector query);
    (3) the cost of the now-heavier verbose panel, A/B against the `master`-built binary on the same
    unrelated screen.
  - No comparison against `pg_ls_waldir()` on a directory of comparable size — reproducing such a
    `pg_wal` is not feasible and the equivalence is structural (both are `SETOF record` with an lstat
    per file, verified).
  - **Then apply Decision 9's pre-agreed outcome, so the measurement cannot end in a shrug.** If the
    query time is comparable to the refresh interval (1 s by default) or the view switch becomes
    visibly sticky → recommend the throttle built on the machinery that already exists for exactly
    this (`verboseCollectState` + `latencyGuardThreshold`, today used only for the DB-size aggregate),
    as its own decision with the numbers attached. If the numbers are acceptable → the remainder goes
    into the tech-debt register as an explicit item (recorded here; `docs/tech-debt.md` is edited by
    `/done`, not by this task).
- Verify the report/CLI criteria against real data rather than by inference from unit tests: make a
  short recording with `pgcenter record` on a fixture cluster, then replay it — `report -W a`,
  `report -W w`, `report -d -W a`, `report -d -W w`, `report -W x`, `report -W -f dump.tar`, and
  `report -W` as the last token. Check the exact messages and exit codes. Use the `master`-built
  binary to produce a pre-0.12 `wal` archive on the PG 18 fixture and replay it with the new binary —
  the column set must be unchanged.
- Walk all 23 user-spec acceptance criteria and all 11 tech-spec acceptance criteria by name, marking
  each PASS / FAIL / NOT VERIFIABLE with concrete evidence: a test name, a command exit code, a stand
  capture, or a line of code. Read `…-decisions.md` first — a documented, accepted deviation is not a
  FAIL.
- Leave the stand as you found it: **delete the ~200 000 dummy `.ready` files before restoring
  `archive_mode`** (otherwise a live archiver will try to archive segments that do not exist), reset
  every GUC changed for an experiment and verify each with `SHOW`, drop or leave documented any test
  role created, restart the cluster if an experiment stopped it, and `tmux kill-session` at the end.
- Produce one explicit verdict: GO (ready to merge) or NEEDS WORK with the owning task listed for each
  FAIL.

## Acceptance Criteria

- [ ] `make build` succeeds at the start of the session; the binaries used on the stand are the ones
      just built, shipped explicitly and invoked by absolute path.
- [ ] Full `go test -race -p 1 -timeout 300s ./...` is green inside `lesovsky/pgcenter-testing:0.0.11`
      with the PG 14–19 fixtures up; zero race reports; counts (pass / fail / skip) recorded.
- [ ] Skipped tests are reported as skipped, not counted as passed — in particular the version-loop
      skips of debt [019], which take out every remaining version once one cluster is unavailable.
- [ ] `make lint` (golangci-lint + gosec) clean on the host, with `golangci-lint` actually on PATH —
      an exit 127 is an environment problem to fix, never a pass and never a "lint unavailable".
- [ ] `make vuln` (govulncheck) clean on the host.
- [ ] No existing test's expectation was weakened: `TestNew`, `TestView_VersionOK`,
      `Test_filterViews`, `Test_selectMenuStyle` and `Test_switchViewTo` each carry a new **correct**
      number, verified by reading the diff — not deleted, not loosened, no row removed.
- [ ] All 23 user-spec acceptance criteria walked by name, each with a status and concrete evidence.
      Criteria that only exist against a live archiver or a live terminal are backed by a stand
      capture, not by inference from a unit test.
- [ ] All 11 tech-spec acceptance criteria walked by name, including the four stale privilege comments
      and the version-independent selector's `_` parameter.
- [ ] The stand address was re-confirmed with the project owner at the start of the run. If it had
      expired, a new one was requested and the manual gate was **not** skipped.
- [ ] The stand inventory (tmux, PostgreSQL flavour and version, preloaded libraries, `$PGDATA`
      access) is recorded, and any step the stand could not support is marked NOT VERIFIABLE with the
      reason — never PASS.
- [ ] Both archiving scenarios were run and captured: `/bin/true` (archived ≥ +3, `archived_age`
      ≤ 00:00:30, `last_archived` set, `ready` back to 0, `failed` unchanged) and `/bin/false`
      (`failed` growing tick to tick, `ready` ≥ 3 and not falling, `last_failed` set, `failed_age`
      ≤ 00:00:30, `archived` unchanged).
- [ ] `archive_mode=off` captured: the four name/age cells are blank — not `0`, not `-` — and the
      screen subtitle is present.
- [ ] The subtitle was counted on **both** entry paths separately — the `w` hotkey and the `W` menu
      item — with one capture each, and appears exactly once per path.
- [ ] Navigation captured: `w` cycles `wal → archiver → wal`, `w` from a third screen lands on `wal`,
      the `W` menu has two working items, the help screen shows the `w,W` line and lists `archiver` in
      the `Q` line.
- [ ] The 60-column pass was actually run: the `source` column stays frozen (checked with
      `capture-pane -e`, since the freeze is an attribute) and the remaining columns are reachable by
      horizontal scroll.
- [ ] The cost measurement was taken under Decision 9's conditions — `pg_monitor`-only role, verbose
      on, concurrent `pgcenter record`, `archive_mode=off`, ~200 000 dummy `.ready` files — and all
      three numbers recorded: query time, view-switch latency, verbose-panel cost A/B against the
      `master` binary. A measurement taken as a superuser is invalid and must be re-run.
- [ ] Decision 9's pre-agreed outcome is applied and stated: either "numbers bad → throttle via
      `verboseCollectState` + `latencyGuardThreshold`, reopened as its own decision" or "numbers
      acceptable → remainder recorded for the tech-debt register". Not left as a shrug.
- [ ] The report/CLI criteria are backed by a real recording and replay, not only by unit tests:
      `-W a`, `-W w`, `-d -W a`, `-d -W w`, `-W x`, `-W -f dump.tar`, bare trailing `-W`, plus a
      pre-0.12 PG 18 `wal` archive made with the `master` binary and replayed unchanged.
- [ ] A second binary built from `master` was run on the same scenarios, and every difference is
      classified as feature-introduced or pre-existing.
- [ ] The stand was left as found: dummy `.ready` files deleted **before** `archive_mode` was
      restored, every changed GUC reset and verified with `SHOW`, cluster restarted if stopped, tmux
      session killed.
- [ ] Explicit verdict: GO or NEEDS WORK with the owning task named for each FAIL.
- [ ] No code was changed by this task (`git status` clean relative to the start of the run).

## Context Files

**Feature artifacts:**
- [017-feat-wal-archiver.md](017-feat-wal-archiver.md) — user-spec: «Критерии приёмки» (23 items — the
  authoritative user-facing list), «Как проверить» (the agent table and the stand-run bullets),
  «Ограничения», «Риски»
- [017-feat-wal-archiver-tech-spec.md](017-feat-wal-archiver-tech-spec.md) — tech-spec: «Acceptance
  Criteria» (11 items), Decisions 1–18 (**Decision 9** fixes the measurement conditions and its
  outcome; **Decisions 11, 12, 15** define behaviours that look like defects but are accepted),
  «Testing Strategy», «Agent Verification Plan», «Backward Compatibility», «Risks»
- [017-feat-wal-archiver-decisions.md](017-feat-wal-archiver-decisions.md) — decisions log: the
  reports of tasks 1–9, their review rounds and any recorded deviations. Read before judging any
  criterion — a documented, accepted deviation is not a FAIL
- [017-feat-wal-archiver-code-research.md](017-feat-wal-archiver-code-research.md) — §8 «Constraints &
  Infrastructure» carries the exact docker invocation and the golangci-lint PATH note; §7.4 the cost
  reasoning; §10.G the list of every test whose numbers change

**Project knowledge:**
- [overview.md](../../../.claude/skills/project-knowledge/overview.md) — what pgcenter is and which
  stats it serves (there is no `project.md` in this repo; `overview.md` is its equivalent)
- [architecture.md](../../../.claude/skills/project-knowledge/architecture.md) — package layout, the
  collector → `statCh` → render data flow, PG version handling
- [patterns.md](../../../.claude/skills/project-knowledge/patterns.md) — **«Manual Testing / QA
  Phase»** and **«Driving the TUI on a remote test stand»** (the regimen this task follows literally),
  «Adding a New View — test counts that must be updated», «Linting», «Report replay across a recorded
  version change»
- [deployment.md](../../../.claude/skills/project-knowledge/deployment.md) — release process (confirms
  a CLI has no deploy step) and the `lesovsky/pgcenter-testing` image the suite needs

**Code files (read only — change nothing):**
- [Makefile](../../../Makefile) — `build`, `test` (race, `-p 1`, timeout 300s), `lint`, `vuln`, `dep`
- [internal/query/archiver.go](../../../internal/query/archiver.go),
  [internal/query/wal.go](../../../internal/query/wal.go),
  [internal/query/overview.go](../../../internal/query/overview.go) — the three query-layer pieces
- [internal/view/view.go](../../../internal/view/view.go) — the archiver registration and its `Msg`
- [top/](../../../top/) — `config_view.go`, `menu.go`, `keybindings.go`, `help.go` and their tests
- [cmd/report/report.go](../../../cmd/report/report.go), [report/](../../../report/) — the `-W` flag,
  the describe text and the golden replays
- [doc/release-notes/v0.12.0.md](../../../doc/release-notes/v0.12.0.md) — task 9's output; both
  literal messages must be there
- [testing/prepare-test-environment.sh](../../../testing/prepare-test-environment.sh) — proof that the
  fixtures run `archive_mode=off`, which is why the stand run is mandatory
- [docs/tech-debt.md](../../../docs/tech-debt.md) — [019] version-loop skips, [029] unsanitised row
  values, [030] the flaky `profile.Test_profileLoop`, [017] the beta apt channel

## Verification Steps

- `make build` — builds `./bin/pgcenter` without errors. Note its mtime/size so a later "did I ship
  the right binary?" question has an answer. Build the `master` binary too, under a distinct name.
- Full suite inside the CI image (the fixtures live in the image; the host has no PG 14–19):

  ```bash
  docker run --rm -v "$PWD":/work -v "$HOME/go/pkg/mod":/gomod \
    -e GOROOT=/gomod/golang.org/toolchain@v0.0.1-go1.25.12.linux-amd64 \
    -e GOPATH=/gopath -e GOFLAGS=-mod=mod -e HOME=/root \
    -w /work lesovsky/pgcenter-testing:0.0.11 bash -c '
      export PATH=$GOROOT/bin:$PATH
      prepare-test-environment.sh > /tmp/prep.log 2>&1 || { tail -30 /tmp/prep.log; exit 1; }
      go test -race -p 1 -timeout 300s ./...'
  ```

  The image carries no Go — the toolchain is mounted from the host module cache, which is why `GOROOT`
  is set explicitly. Expected: `ok` for every package, no `WARNING: DATA RACE`, exit 0.
- `export PATH="$PATH:$(go env GOPATH)/bin" && make lint` — exit 0, empty findings list from both
  golangci-lint and gosec.
- `make vuln` — exit 0, no known vulnerabilities.
- `git diff` on the five count-based tests (`internal/view/view_test.go`, `record/record_test.go`,
  `top/config_view_test.go`, `top/menu_test.go`) — every changed number is a new correct number, and
  **read the test body**, not just the name: a test existing under the expected name is not evidence
  that it checks the claimed thing.
- `go test ./internal/query/ -run '<privilege test name>' -v` inside the image, pointwise, and read
  the body — the `SET ROLE` tests are the single most important assertion of the feature, because the
  fixture superuser hides exactly the defect piece 4 exists to fix.
- The stand run: for each scenario, the `send-keys` command, the resulting `capture-pane`, and the
  judgement. Attribute-sensitive checks (frozen column) use `-e`; layout and content use plain
  capture. Strip ANSI for diffing with `sed 's/\x1b\[[0-9;]*m//g'`.
- The subtitle count: two separate captures, one per entry path, each grepped for the message —
  `grep -c 'Show archiver statistics'` on the captured pane.
- CLI replay: exit codes captured with `echo $?` after each `report` invocation, and the exact stderr
  text compared against the literals (`flag needs an argument: 'W' in -W`, `report type is not
  specified, quit`).
- `SHOW archive_mode`, `SHOW archive_command`, and a directory listing of `archive_status/` at the end
  of the stand run — proof that the stand was restored.
- `git status` at the end — the QA task changed no code.

## Details

**Files:** no code file is modified. The deliverables are the QA report
(`logs/working/qa-report.json` per the skill — `logs/` is gitignored; if the feature's convention is a
per-feature copy, mirror it as `017-feat-wal-archiver-qa-report.json` in the feature directory, as
016 did), the stand captures (suggested: `logs/working/qa-017-stand/<check-name>.txt`), the entry in
`017-feat-wal-archiver-decisions.md`, and the verdict in the agent's final message.

**Dependencies:**
- Tasks 1–9 must be `done` with their review cycles closed. Tasks 5 and 6 are the direct dependency
  for the whole stand run — without the registered view and the `w`/`W` machinery there is no screen
  to drive.
- Local toolchain: Go 1.25+, docker, golangci-lint, gosec, govulncheck (`make dep`). `golangci-lint`
  lives in `$(go env GOPATH)/bin` and is **not** on the default PATH — without adding it `make lint`
  exits 127 and looks like a broken target rather than a missing tool.
- **A live PostgreSQL is required for `make test`.** Packages outside this feature open connections,
  and the feature's own integration tests need PG 14–19 on ports 21914–21919. They exist only inside
  the CI image — a missing cluster is an environment blocker, and calling it a criterion FAIL would be
  wrong.
- A stand is required and its address must be re-confirmed with the project owner. The recorded
  `pgpro@10.128.31.96` has a 24h TTL from 2026-08-05.

**Stand regimen (from `patterns.md` → «Driving the TUI on a remote test stand»):**
1. Confirm the address with the owner at the start of the run; do not record it afterwards.
2. Inventory first — `tmux` (installing it is fine, these stands carry passwordless sudo), the
   PostgreSQL flavour and version, `shared_preload_libraries`, `$PGDATA` and write access to
   `pg_wal/archive_status`. Observed variety: Ubuntu 24.04 with tmux preinstalled; Debian 12 with no
   tmux, no Go and a Postgres Pro build without `pg_stat_statements`.
3. `make build` locally, then `scp` both binaries (feature and `master`) under distinct names and
   invoke each by absolute path, so a capture can never be attributed to the wrong build.
4. `tmux new-session -d -s cap -x 190 -y 52` for the main pass; a second session at `-x 60` for the
   narrow pass. Fixed geometry is the entire reason for using tmux.
5. `send-keys` a hotkey, wait at least one refresh interval, then `capture-pane`. Plain capture for
   layout, content and row counts; `-e` when colour or an attribute is the thing being checked.
6. Leave the stand as found — see the cleanup order in Edge cases.

**Edge cases:**
- **The stand has expired.** Its TTL is 24h from 2026-08-05, and manual QA comes last. Ask for a new
  one. Do **not** mark the manual criteria PASS by inference and do **not** quietly drop them — the
  tech-spec Risks table already names this as the scenario in which the pipeline must not silently
  skip the gate. NOT VERIFIABLE with the reason is the honest outcome if no stand can be had.
- **`archive_mode` needs a restart, `archive_command` only a reload.** Plan the scenario order around
  that: `on` + `/bin/true` (restart), then swap to `/bin/false` (reload), then `off` for the cost run
  (restart). Fewer restarts, and the switch between the two archiving scenarios stays fast enough that
  `archived_age`/`failed_age` are still under 30 s when captured.
- **The cost measurement as a superuser is not a weak measurement, it is the wrong one.** For a
  superuser the verbose panel already walks `pg_ls_waldir()` every tick, so the change looks like
  1× → 2×; for a `pg_monitor`-only role today's aggregate fails instantly with 42501 and costs
  nothing, so the real change is zero → full. If the run was made as `postgres`, re-run it.
- **The dummy files must go before `archive_mode` comes back.** ~200 000 `.ready` files name segments
  that do not exist; with archiving re-enabled the archiver would spin on them. Delete first, restore
  the GUC second, then verify with `SHOW` and a listing.
- **Creating 200 000 files may need the postgres OS user.** `archive_status/` is owned by the cluster
  user; the login user may need sudo. Establish this during the inventory, not mid-run — and remember
  that `pg_ls_archive_statusdir()` is `missing_ok=true` (Decision 11), so a typo in the directory path
  yields a confident `0` instead of an error.
- **`profile.Test_profileLoop` fails.** Known flaky — tech debt [030], fails with `canceling statement
  due to user request`, passes on an immediate re-run, in a package this feature never touches. Re-run
  before filing it as a regression, and report it as "flaky, re-run green" — do not silently drop it
  and do not call the suite red because of it.
- **A version-loop test skips everything after the first unavailable cluster** (debt [019]). A skip is
  not a pass. If PG 19 (or any version) is unavailable in the image, say which criteria lost their
  evidence rather than reporting a green run.
- **PG 19 is beta2 in the image** and `wal_fpi_bytes` may move at beta3/RC (Risks, debt [017]). If the
  PG 19 fixture rejects the column, that is an environment/upstream finding with a named mitigation
  (the FPI piece is detachable), not a silent feature FAIL — report it as such with the observed
  catalog contents.
- **Three accepted behaviours must not be filed as defects.** Decision 11: on a cluster whose
  `archive_status` directory is missing, the verbose panel now shows a bare `0` instead of `n/a`.
  Decision 12: `report -W a` over an N-tick recording prints N−1 rows — the first sample is dropped by
  the shared report path, as it is for every `DiffIntvl{0,0}` screen. Decision 15: `report -W a` over
  an archive with no archiver entries prints no rows **and no header**, exiting 0. All three are
  written into the specs; verify them as specified behaviour.
- **`report -W w` on a PG 19 archive recorded by a pre-0.12 pgcenter fails with `diff failed`.** Known
  and accepted (Backward Compatibility); it belongs to the release notes, not to the defect list. The
  PG 18 pre-0.12 archive, by contrast, **must** replay unchanged — that one is a real criterion.
- **`pgcenter record` under a role without `pg_monitor` aborts the whole recording.** Existing
  recorder behaviour, fixed as expected by user-spec criterion 13 — record it, do not file it.
- **A difference appears on the feature binary but also on the `master` binary.** Pre-existing, not a
  regression. In feature 015 this reclassified three of five findings; without the second binary they
  would have been filed against the feature.
- **`make lint` complains about code the feature never touched.** Note it separately: not a blocker
  for the feature, but not grounds for declaring lint clean either.
- **A capture is missing for a terminal-only criterion.** Status is NOT VERIFIED, not PASS. This is
  the single most likely way a bad gate passes.

**Implementation hints:**
- Count criteria from the documents, not from memory: 23 checkboxes in the user-spec «Критерии
  приёмки», 11 in the tech-spec «Acceptance Criteria». Walk them in document order so nothing is
  skipped by association ("covered by the previous one" is how a criterion disappears).
- Several user-spec criteria have **two** evidence sources and need both cited: the privilege pair
  (criteria 16–17) is a unit test *and* the terminal behaviour of criterion 12; the verbose-panel
  criterion 21 is a unit test *and* the A/B stand observation; the help-screen criterion 8 is a unit
  test *and* a capture of the live help page.
- The subtitle criterion is the one most likely to pass by accident. It is not "the message appears" —
  it is "exactly once, on each of two independent call sites". Grep the capture and report the count,
  per path.
- The narrow-terminal check needs `-e`: the frozen column is rendered with an attribute, and a plain
  capture makes a lost freeze indistinguishable from a working one.
- For the report criteria, a recording made on a fixture cluster inside the CI image is enough — the
  archiver row will be all zeros and NULLs there, which is fine: these criteria are about the report
  pipeline (row counts, headers, exit codes, describe text), not about archiving values.
- Keep the report compact: a "criterion → status → evidence" table plus the verdict. Command output
  dumps do not belong in the decisions log; an exit code and a one-line summary are enough. Raw
  captures go to files under `logs/working/`.

## Reviewers

None — a QA task carries no reviewers by the catalogue; the report *is* the deliverable. The result is
accepted by the project owner at feature acceptance.

## Post-completion

- [ ] Записать краткий отчёт в [017-feat-wal-archiver-decisions.md](017-feat-wal-archiver-decisions.md)
      (Summary: 1-3 предложения, таблица критериев со статусами, вердикт GO/NEEDS WORK, ссылка на
      `logs/working/qa-report.json`; без дампов вывода команд)
- [ ] Record the cost-measurement numbers and the Decision 9 outcome in the decisions log — including,
      if the numbers are acceptable, the tech-debt item to be registered (throttling of the `.ready`
      listing and of the verbose panel's second directory walk). **Record only.** Editing
      `docs/tech-debt.md` belongs to `/done`, which owns that register.
- [ ] Если отклонились от спека — описать отклонение и причину
- [ ] Обновить user-spec/tech-spec если что-то изменилось


## Correction applied during task validation (2026-08-06)

**The verbose backlog renders `0`, not `0 B`.** Both specs and an earlier draft of this task quoted
`0 B` as the value shown when the `archive_status` directory is missing. The panel formats that field
through the project's size formatter, whose zero case returns a bare `0`. A QA gate checking for the
string `0 B` would report a false FAIL on correct behaviour.

**The stand's TTL has lapsed.** The stand named in the specs (`pgpro@10.128.31.96`) was issued on
2026-08-05 with a 24-hour TTL, so it is gone by the time this task runs. Do NOT quietly skip the manual
half: ask the project owner for a fresh stand at the start of this task, and record in the QA report
which scenarios were executed and which were blocked waiting for one. The automated half — the full
suite inside the CI image, lint and vuln — does not depend on the stand and runs regardless.
