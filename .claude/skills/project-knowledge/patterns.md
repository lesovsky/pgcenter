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
