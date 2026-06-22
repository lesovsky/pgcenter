---
status: done                       # planned -> in_progress -> done
depends_on: []                     # ID задач-зависимостей (строки: ["01", "02"])
wave: 1                            # волна параллельного выполнения
skills: [code-writing]             # МАССИВ скиллов для загрузки
verify: bash                       # go test ./report/... -run Test_describeReport
reviewers: [dev-code-reviewer, dev-test-reviewer]  # явно указать. Пусто = fallback на defaults
teammate_name:                     # имя агента-исполнителя (опционально; если не задано — генерируется по описанию задачи)
---

# Task 02: Report describe wiring for the 5 new types

## Required Skills

Перед выполнением задачи загрузи:
- `/skill:code-writing` — [skills/code-writing/SKILL.md](~/.claude/skills/code-writing/SKILL.md)

## Description

`pgcenter report -d <reporttype>` prints a human-readable column description for the requested
stats report. The four 0.11.0 screens (5 report types: `bgwriter`, `replslots`, `stat_io`,
`stat_io_time`, `statements_jit`) are becoming recordable/reportable in this feature, so they now
need describe entries — without them `report -d` returns the literal `"unknown description
requested"` fallback for these types.

This task adds 5 column-description constants to `report/describe.go` (following the existing
multi-line `column / origin / description` table convention, like `pgStatWALDescription`),
registers them in the `describeReport` map in `report/report.go`, and adds the matching
`Test_describeReport` cases in `report/report_test.go`. This is a pure-wiring task: no recorder,
report-pipeline, or CLI-flag changes (CLI flags are Task 03; enabling recording is Task 01).

## What to do

1. Add 5 new description constants to the `const (...)` block in `report/describe.go`, named per
   the existing convention:
   - `pgStatBgwriterDescription` — pg_stat_bgwriter / pg_stat_checkpointer columns
   - `pgStatReplicationSlotsDescription` — pg_replication_slots / pg_stat_replication_slots columns
   - `pgStatIODescription` — pg_stat_io operations (count) columns
   - `pgStatIOTimeDescription` — pg_stat_io timings columns
   - `pgStatStatementsJITDescription` — pg_stat_statements JIT-compilation columns
   Each follows the multi-line backtick-string table format (header line, then
   `column / origin / description` rows, then a trailing `Details: <pg docs URL>` line) used by
   `pgStatWALDescription` and the other multi-column constants. Column meanings mirror the TUI
   screen columns of each view (the same columns the recorder stores and the report replays).

2. Register the 5 constants in the `describeReport` map in `report/report.go` (the
   `map[string]string` at report.go:606-629) under the report-type keys: `bgwriter`,
   `replslots`, `stat_io`, `stat_io_time`, `statements_jit`.

3. Add 5 new test cases to `Test_describeReport` in `report/report_test.go` (the testcases slice
   at report_test.go:1038-1059), each mapping the report-type key to its new constant. Keep the
   existing `{report: "invalid", want: "unknown description requested"}` case last.

4. Do NOT touch the procpidstat column-index const block in `report/report.go` (report.go:342-346)
   — that dedup is Task 09.

## TDD Anchor

Tests written/extended before the wiring (red → green):

- `report/report_test.go::Test_describeReport` — add `{report: "bgwriter", want: pgStatBgwriterDescription}`
  and assert `describeReport(&buf, "bgwriter")` writes exactly `pgStatBgwriterDescription`.
- `report/report_test.go::Test_describeReport` — `{report: "replslots", want: pgStatReplicationSlotsDescription}`.
- `report/report_test.go::Test_describeReport` — `{report: "stat_io", want: pgStatIODescription}`.
- `report/report_test.go::Test_describeReport` — `{report: "stat_io_time", want: pgStatIOTimeDescription}`.
- `report/report_test.go::Test_describeReport` — `{report: "statements_jit", want: pgStatStatementsJITDescription}`.

The test references the new constants, so the package will not compile (red) until the constants
exist and are wired into the map; once the constants are added and the map is updated, all cases
pass (green).

## Acceptance Criteria

- [ ] 5 new description constants exist in `report/describe.go`, named `pgStatBgwriterDescription`,
      `pgStatReplicationSlotsDescription`, `pgStatIODescription`, `pgStatIOTimeDescription`,
      `pgStatStatementsJITDescription`, each in the multi-line table convention with a `Details:` URL.
- [ ] The `describeReport` map has 5 new entries mapping `bgwriter` / `replslots` / `stat_io` /
      `stat_io_time` / `statements_jit` to the corresponding constants.
- [ ] `Test_describeReport` has 5 new passing cases; the `invalid` fallback case still passes.
- [ ] `go test ./report/... -run Test_describeReport` is green.
- [ ] The procpidstat col-index const block (report.go:342-346) is untouched.

## Context Files

**Feature artifacts:**
- [008-feat-record-report-0-11-views.md](008-feat-record-report-0-11-views.md) — user-spec
- [008-feat-record-report-0-11-views-tech-spec.md](008-feat-record-report-0-11-views-tech-spec.md) — tech-spec (Task 2; §9 of code-research)
- [008-feat-record-report-0-11-views-decisions.md](008-feat-record-report-0-11-views-decisions.md) — decisions log (created at completion)
- [008-feat-record-report-0-11-views-code-research.md](008-feat-record-report-0-11-views-code-research.md) — §9: constant names + locations

**Project knowledge:**
- [overview.md](.claude/skills/project-knowledge/overview.md) — features / supported stats (project doc; no project.md in this repo)
- [architecture.md](.claude/skills/project-knowledge/architecture.md) — package layout, report data flow
- [patterns.md](.claude/skills/project-knowledge/patterns.md) — code + testing conventions, version branching

**Code files:**
- [report/describe.go](report/describe.go) — add 5 description constants (follow `pgStatWALDescription` style)
- [report/report.go](report/report.go) — add 5 entries to `describeReport` map (606-629); do NOT touch 342-346
- [report/report_test.go](report/report_test.go) — add 5 `Test_describeReport` cases (1038-1059)
- [internal/view/view.go](internal/view/view.go) — view defs for the 5 types (140-243): columns/Ncols to mirror in descriptions

## Verification Steps

- Run `go test ./report/... -run Test_describeReport` — all describe cases (existing + 5 new + invalid fallback) pass.
- Run `go build ./...` — package compiles (new constants referenced by test and map).
- Confirm `report.go:342-346` (procPidStatCol* const block) is unchanged.

## Details

**Files:**
- `report/describe.go` — single top-level `const (...)` block (describe.go:3-441). Append the 5
  new `pgStat<Thing>Description` constants. Mirror the multi-line backtick table format of
  `pgStatWALDescription` (describe.go:140-157): a header sentence naming the source view(s), a
  `column / origin / description` table, and a closing `Details: <postgresql.org docs URL>` line.
  `procPidStatDescription` (describe.go:422-423) is the one-line exception — do NOT copy that
  terse style; use the full table style.
- `report/report.go` — add 5 map entries in `describeReport` (report.go:606-629), keys
  `bgwriter`, `replslots`, `stat_io`, `stat_io_time`, `statements_jit`. Keep gofmt map-value
  alignment tidy. Leave the `procPidStatCol*` const block (report.go:342-346) for Task 09.
- `report/report_test.go` — add 5 cases to the `Test_describeReport` testcases slice
  (report_test.go:1038-1059), each `{report: "<key>", want: <constant>}`. The `invalid`
  fallback case stays last. The slice currently also omits `statements_wal` — do NOT "fix"
  unrelated gaps (out of scope).

**Column meanings (mirror the view layouts in internal/view/view.go and the TUI screens):**
- `bgwriter` (view.go:140-152, Ncols 12, PG14 baseline; version-patched at PG17/PG18) —
  pg_stat_bgwriter + pg_stat_checkpointer counters (checkpoints, buffers written by
  checkpointer/bgwriter/backends, maxwritten, fsync, etc.). The recorded layout is version-aware
  (see `query.SelectStatBgwriterQuery`); describe the PG14 baseline columns.
- `replslots` (view.go:153-165, Ncols 15) — pg_replication_slots + pg_stat_replication_slots:
  slot_name, slot_type, active, wal_status, retained,KiB, and the cumulative spill/stream counters.
- `stat_io` (view.go:166-179, Ncols 16) — pg_stat_io operations: backend_type / object / context
  identity columns plus reads/writes/extends/hits and bytes-derived counters.
- `stat_io_time` (view.go:180-193, Ncols 10) — pg_stat_io timing columns (read_time/write_time/
  etc.); requires track_io_timing=on.
- `statements_jit` (view.go:230-243, Ncols 13, PG15 baseline) — pg_stat_statements JIT columns:
  user, database, jit_functions, generation/inlining/optimization/emission counts and times,
  plus the fake queryid + query. NB: the column count shifts at PG17 (Ncols 15) — describe the
  PG15 baseline; report-time `Configure` handles the version layout (not this task).

**Dependencies:** none (depends_on empty, wave 1). Disjoint files from Task 01 (view/record) and
Task 03 (cmd/report). Task 09 also edits `report/report.go` but a different region (const block at
342-346) — this task only adds map entries at 606-629; Task 09 is sequenced into a later wave to
avoid overlap.

**Edge cases:**
- `report -d` is description-only and does not read the archive, so there are no data/empty-archive
  edge cases here — just the map lookup.
- `statements_jit` version layout difference (PG15 13 cols vs PG17 15 cols): the description is
  static text, so describe the PG15 baseline column set; the report pipeline selects the version
  layout at replay time.

**Implementation hints:**
- Copy the `pgStatWALDescription` block as the structural template for column descriptions.
- Constant names must match code-research §9 exactly (the map keys and downstream golden tests in
  Tasks 04-07 assume these names).
- Use real PostgreSQL doc URLs in the `Details:` line (pg_stat_bgwriter / pg_stat_checkpointer,
  pg_replication_slots + pg_stat_replication_slots, pg_stat_io, pg_stat_statements pages).

## Reviewers

- **dev-code-reviewer** → `008-feat-record-report-0-11-views-task-02-dev-code-reviewer-review.json`
- **dev-test-reviewer** → `008-feat-record-report-0-11-views-task-02-dev-test-reviewer-review.json`

## Post-completion

- [ ] Записать краткий отчёт в [008-feat-record-report-0-11-views-decisions.md](008-feat-record-report-0-11-views-decisions.md) (Summary: 1-3 предложения, ревью со ссылками на JSON, без таблиц файндингов и дампов)
- [ ] Если отклонились от спека — описать отклонение и причину
- [ ] Обновить user-spec/tech-spec если что-то изменилось
