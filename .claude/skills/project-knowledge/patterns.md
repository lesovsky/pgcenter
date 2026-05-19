# pgcenter — Code Patterns

## Adding a New PostgreSQL Version

1. Add port to `internal/postgres/testing.go` ports map
2. Add version to all `versions := []int{...}` lists in `internal/query/*_test.go`
3. Run tests — `t.Skipf` handles unavailable versions gracefully
4. If a stats view changed: add a new query constant and selector function in `internal/query/`
5. Wire selector into `internal/view/view.go: Configure()` if Ncols also changes
6. Update pgcenter-testing Docker image (see deployment.md)

## Version-Specific Query Pattern

When a PG version changes columns in a stats view:
- Add `PgStatXxxPGNN` constant in the relevant `internal/query/*.go` file
- Add `SelectStatXxxQuery(version int) (string, int)` returning template + ncols
- Call it in `view.Configure()` under the correct view name
- Add version-specific test cases in `*_test.go`

## Error Wrapping

Use `fmt.Errorf("context: %w", err)` for all error wrapping in production code.
Use `errors.Is(err, target)` for error comparison (not `==`).
Exception: `printCmdline()` and `fmt.Sprintf()` use `%s` (not error wrapping functions).

## Sorting

Use `sort.SliceStable` (not `sort.Slice`) in `internal/stat/postgres.go` to ensure deterministic ordering of rows with equal sort keys across Go versions.

## Manual Testing / QA Phase

Always run `make build` as the first step of any manual TUI verification, even if a previous
build completed earlier in the same session. Cherry-picks, rebases, and mid-session code
changes do not automatically update `./bin/pgcenter`. A stale binary silently invalidates every
visual check that follows. The rule: one manual verification session = one fresh build at the
start.

## printCmdline() — Mutual Exclusion

`printCmdline(g, msg)` calls `g.Update` followed by `v.Clear`. If it is called twice in the
same view-switch handler the second call immediately overwrites the first render. When a
handler needs to show either a warning or a normal message, these two cases must be mutually
exclusive — use an `if/else` branch, not two sequential calls. Calling `printCmdline(warning)`
and then `printCmdline(v.Msg)` in the same code path will always discard the warning before
the user can read it.

When multiple independent availability probes can fail (e.g., IO + delay accounting in
`switchViewToProcPidStat`), use a 4-branch `switch` covering all combinations, with a combined
message for the case where both are unavailable — still exactly one `printCmdline` call per path.

## Adding a Hybrid View (SQL + procfs enrichment)

When a view combines SQL and local system data (e.g., procpidstat = pg_stat_activity + /proc):

1. Define a `CollectExtra` constant in `internal/stat/stat.go` iota block. The iota is offset by 1 (`pgProcUptimeQuery` string constant precedes the group): existing values `CollectNone=1, ..., CollectLogtail=5`; next is 6.
2. Register the view in `view.New()` with `NotRecordable: true`, `DiffIntvl: [2]int{0,0}`, `Filters: map[int]*regexp.Regexp{}`. Leave `CollectExtra`/`IOAvailable` at zero — set at runtime by the switch handler.
3. The switch handler (`top/config_view.go`) must save/load/patch/send the view manually — NOT via `viewSwitchHandler`, which reloads from the static map and discards runtime patches.
4. In `Collector.Update()`, add a `view.CollectExtra == CollectXxx` branch after `collectPostgresStat` to enrich and replace the SQL result.
5. In `top/stat.go:collectStat()`, add `prevCollectExtra` change-detection alongside `ShowExtra` to call `c.Reset()` on view switches.
6. Add `NotRecordable` skip in `record/record.go:filterViews()`.
7. Reference implementation: `internal/stat/procpidstat.go`, `top/config_view.go:switchViewToProcPidStat`.

## Git Workflow

- Work in `develop`, open PRs to `master` with squash merge
- After squash: `git reset --hard master && git push --force-with-lease` to sync develop
- Release: tag on master → push to `release` branch → triggers release workflow

## Linting

`.golangci.yml` enables: errcheck, gocritic, gosimple, govet, ineffassign, revive, staticcheck, unused.
Run locally: `make lint` (golangci-lint + gosec) and `make vuln` (govulncheck).
Known suppressions: `// #nosec G204,G702` on `exec.Command` calls (pager/editor from env vars).

## Naming Conventions

Go acronyms: `CPUStat` not `CpuStat`, `PGresult` not `PgResult`.
Unused function parameters in callbacks: rename to `_`.
