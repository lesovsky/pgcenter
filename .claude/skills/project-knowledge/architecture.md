# pgcenter — Architecture

## Package Layout

```
cmd/
  pgcenter.go     # root cobra command
  top/            # top command — real-time TUI
  record/         # record command — stats collection to files
  report/         # report command — report generation
  profile/        # profile command — wait events profiler

internal/
  postgres/       # PG connection management, query execution
  query/          # SQL queries for each stats view (one file per view)
  stat/           # stats collection, processing, diff computation
  view/           # TUI view definitions (column layout, sort, filter)
  align/          # column width alignment
  pretty/         # human-readable formatting (bytes, intervals)
  math/           # numeric utilities for rate calculations
  version/        # version string (injected via ldflags at build time)
```

## Data Flow (top command)

```
postgres connection
  → query/         SQL query per stats view
  → stat/          collect → compute diff vs previous sample
  → view/          apply column layout, sort, filter
  → gocui TUI      render to terminal
```

## PostgreSQL Version Handling

Stats views change between PG versions (new columns, renamed columns, new views).
Each stats view query in `internal/query/` may have version-specific variants.
Version detection happens at connect time via `SELECT version()`.

## Configuration

Connection params: CLI flags (host, port, user, dbname, password).
pgpass and pg_service files are supported via pgconn.
Custom queries: YAML config files in `config/`.

## Testing

Integration tests require a live PostgreSQL instance.
Test helpers in `testing/`.
Run with: `make test` (uses `-race -p 1 -timeout 300s`).
