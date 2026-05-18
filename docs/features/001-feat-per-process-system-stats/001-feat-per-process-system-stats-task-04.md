---
status: planned
depends_on: ["02"]
wave: 2
skills: [code-writing]
verify: "bash — go test ./record/... && make build"
reviewers: [dev-code-reviewer, dev-security-auditor, dev-test-reviewer]
teammate_name:
---

# Task 04: View registration, new View fields, and record skip

## Required Skills

Before starting, load:
- `/skill:code-writing` — [SKILL.md](~/.claude/skills/code-writing/SKILL.md)

## Description

This task extends the `view.View` struct with three new fields required by the per-process system stats screen, registers the `"procpidstat"` view in `view.New()`, adds the `CollectProcPidStat` constant to `internal/stat/stat.go`, and makes the record subsystem skip the new view.

The `View` struct gets three fields with Go zero values that preserve all existing behavior unchanged:
- `CollectExtra int` — signals non-SQL enrichment in `Collector.Update()`; `0` means no enrichment (existing views unaffected)
- `IOAvailable bool` — carries the `/proc/[pid]/io` capability flag from the screen-open handler to the Collector via `viewCh`; `false` for all existing views
- `NotRecordable bool` — when `true`, instructs `record/record.go:filterViews()` to skip this view; existing views keep the default `false`

The `"procpidstat"` view is a static entry in `view.New()` with 17 columns, `DiffIntvl: [2]int{0,0}` (no SQL-level diff), `Filters: map[int]*regexp.Regexp{}` (initialized map, required — a nil map panics on `/` filter input), and `NotRecordable: true`. `CollectExtra` and `IOAvailable` are NOT set here — they are patched at runtime in `switchViewToProcPidStat()` after loading from the map.

The `CollectProcPidStat = 5` constant goes into the existing iota block in `internal/stat/stat.go` as the next value after `CollectLogtail = 4`. This constant is used in Task 5 by `Collector.Update()` to branch into procfs enrichment.

`record/record.go:filterViews()` currently filters views by version and pg_stat_statements availability. A one-line guard is added to skip views where `NotRecordable` is true. The record subsystem must not attempt to collect the procpidstat view because its SQL query returns only 7 columns while the view declares `Ncols: 17` — the procfs enrichment that produces the remaining 10 columns never runs in the record context.

## What to do

1. In `internal/view/view.go`, add three fields to the `View` struct: `CollectExtra int`, `IOAvailable bool`, `NotRecordable bool`. Place them after the existing `ShowExtra int` field to keep related fields together.

2. In `internal/view/view.go`, add the `"procpidstat"` entry to the map returned by `view.New()`. Set `Name`, `QueryTmpl: query.PgStatActivityProcPidStat`, `DiffIntvl: [2]int{0, 0}`, `Ncols: 17`, `OrderKey: 0`, `OrderDesc: false`, `ColsWidth: map[int]int{}`, `Filters: map[int]*regexp.Regexp{}`, `Msg: "Show per-process system stats"`, `NotRecordable: true`. Leave `CollectExtra` and `IOAvailable` at zero values.

3. In `internal/stat/stat.go`, add `CollectProcPidStat = 5` to the existing `const` block after `CollectLogtail`. The block currently uses `iota` starting at 0; `CollectProcPidStat` must be explicitly set to `5` (or added as the next `iota` entry after `CollectLogtail`).

4. In `record/record.go`, inside `filterViews()`, add a check at the top of the loop body (before the version check) that deletes the view and continues if `v.NotRecordable` is true.

5. Write tests for the `filterViews()` change: verify that a view with `NotRecordable: true` is excluded from the returned views map, and that a view with `NotRecordable: false` (default) is retained.

6. Run `go test ./record/... && make build` and confirm both pass.

## TDD Anchor

Write these tests before implementing the production changes. Run them — confirm they fail. Implement — confirm they pass.

- `record/record_test.go::TestFilterViews_NotRecordable` — a view with `NotRecordable: true` is removed from the map returned by `filterViews()`; the count of filtered views increases by one
- `record/record_test.go::TestFilterViews_Recordable` — a view with `NotRecordable: false` (default zero value) passes through `filterViews()` unchanged when version is satisfied

## Acceptance Criteria

- [ ] `view.View` struct has three new fields: `CollectExtra int`, `IOAvailable bool`, `NotRecordable bool`
- [ ] All existing views compile and behave identically (zero values for new fields = no behavior change)
- [ ] `view.New()` returns a map that includes `"procpidstat"` with `Ncols: 17`, `DiffIntvl: [2]int{0, 0}`, `NotRecordable: true`, and an initialized (non-nil) `Filters` map
- [ ] `internal/stat/stat.go` defines `CollectProcPidStat = 5` (or next iota after `CollectLogtail`)
- [ ] `record/record.go:filterViews()` skips views where `NotRecordable == true`
- [ ] `go test ./record/...` passes with the new `filterViews` tests covering the `NotRecordable` path
- [ ] `make build` produces a binary without errors

## Context Files

**Feature artifacts:**
- [001-feat-per-process-system-stats.md](docs/features/001-feat-per-process-system-stats/001-feat-per-process-system-stats.md) — user-spec
- [001-feat-per-process-system-stats-tech-spec.md](docs/features/001-feat-per-process-system-stats/001-feat-per-process-system-stats-tech-spec.md) — tech-spec
- [001-feat-per-process-system-stats-decisions.md](docs/features/001-feat-per-process-system-stats/001-feat-per-process-system-stats-decisions.md) — decisions log

**Project knowledge:**
- [architecture.md](.claude/skills/project-knowledge/architecture.md)
- [patterns.md](.claude/skills/project-knowledge/patterns.md)

**Code files:**
- [internal/view/view.go](internal/view/view.go) — add fields to `View` struct and register `"procpidstat"` in `view.New()`
- [internal/stat/stat.go](internal/stat/stat.go) — add `CollectProcPidStat` constant
- [record/record.go](record/record.go) — add `NotRecordable` skip in `filterViews()`
- [internal/query/procpidstat.go](internal/query/procpidstat.go) — read only; provides `query.PgStatActivityProcPidStat` referenced in the view entry

## Verification Steps

- Run `go test ./record/...` — all tests pass including the new `TestFilterViews_NotRecordable` test
- Run `make build` — binary builds without errors or warnings
- Confirm no regressions: `go test ./internal/view/... ./internal/stat/...` all pass

## Details

**Files:**

`internal/view/view.go` — Current state: `View` struct has 16 fields ending with `ShowExtra int`. The `New()` function returns a map with 20 existing views. What to change:
- Add `CollectExtra int`, `IOAvailable bool`, `NotRecordable bool` after `ShowExtra int` in the struct definition.
- Add the `"procpidstat"` key to the map in `New()` with the full view config from the tech-spec Data Models section. The `QueryTmpl` must reference `query.PgStatActivityProcPidStat` — this constant is created in Task 2 and is available when this task runs (wave 2 depends on wave 1). Ensure the `Filters` field is `map[int]*regexp.Regexp{}` (non-nil initialized map), not omitted.

`internal/stat/stat.go` — Current state: `const` block uses `iota` with values `CollectNone=0`, `CollectDiskstats=1`, `CollectNetdev=2`, `CollectFsstats=3`, `CollectLogtail=4`. The `switch c.config.collectExtra` in `Update()` handles cases 1–4. What to change:
- Add `CollectProcPidStat` as the next entry (`= 5`, or continue the iota). Do NOT add a case to the switch yet — that is Task 5.

`record/record.go` — Current state: `filterViews()` iterates over views, deletes those that fail `VersionOK()`, and deletes `statements_*` views when `pgssSchema` is empty. What to change:
- At the top of the loop body (before the `VersionOK` check), add: if `v.NotRecordable` is true, delete the view from the map, increment `filtered`, and `continue`. This mirrors the pattern of the existing version check immediately below.

**Dependencies:**
- Task 2 must be complete (wave 1) — `internal/query/procpidstat.go` must exist and export `PgStatActivityProcPidStat` for the view entry to compile.
- Tasks 5 and 6 (wave 3) depend on the `CollectExtra`, `IOAvailable`, and `NotRecordable` fields added here.

**Edge cases:**
- `Filters` must be an initialized map literal `map[int]*regexp.Regexp{}`, not `nil`. A nil `Filters` map causes a panic when the user presses `/` to set a filter (map write on nil map). All 20 existing views in `New()` explicitly initialize it — follow the same pattern.
- `CollectExtra` and `IOAvailable` intentionally start at zero values in the static map. They are patched at runtime in `switchViewToProcPidStat()` after loading the view from the map. Do not set them here.
- `filterViews()` modifies the map passed to it in place (deletes keys). The increment of `filtered` counter must happen for every deleted view including `NotRecordable` ones, so the INFO message about skipped stats fires correctly if needed.

**Implementation hints:**
- The `record` package has no existing tests file — check with `ls record/` first. If there is no `record_test.go`, create it. Look at `internal/stat/*_test.go` for testify usage patterns (`assert.Equal`, `assert.NotContains`).
- The `filterViews` function signature is `func filterViews(version int, pgssSchema string, views view.Views) (int, view.Views)`. Tests can call it with a minimal `view.Views` map containing one view with `NotRecordable: true` and an empty `pgssSchema`, `version: 0` to isolate the new behavior.
- `view.New()` is called in tests indirectly — verify the procpidstat entry compiles and has the right fields by adding a simple assertion in `TestFilterViews_NotRecordable` that calls `view.New()` and checks the procpidstat entry has `NotRecordable: true` and `Ncols: 17`.

## Reviewers

- **dev-code-reviewer** → `docs/features/001-feat-per-process-system-stats/001-feat-per-process-system-stats-task-04-dev-code-reviewer-review.json`
- **dev-security-auditor** → `docs/features/001-feat-per-process-system-stats/001-feat-per-process-system-stats-task-04-dev-security-auditor-review.json`
- **dev-test-reviewer** → `docs/features/001-feat-per-process-system-stats/001-feat-per-process-system-stats-task-04-dev-test-reviewer-review.json`

## Post-completion

- [ ] Write a brief report to [001-feat-per-process-system-stats-decisions.md](docs/features/001-feat-per-process-system-stats/001-feat-per-process-system-stats-decisions.md) (summary: 1-3 sentences, review links, no finding tables or dumps)
- [ ] If deviated from spec — describe the deviation and reason
- [ ] Update user-spec/tech-spec if anything changed
