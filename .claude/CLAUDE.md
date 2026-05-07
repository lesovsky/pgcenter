# pgcenter — Project Constitution

## Project

pgcenter is a command-line admin tool for observing and troubleshooting PostgreSQL.
It provides a top-like interface over PostgreSQL statistics views.

Repository: https://github.com/lesovsky/pgcenter

## Tech Stack

- **Language**: Go 1.22+
- **CLI framework**: cobra
- **PostgreSQL driver**: pgx/v4, pgconn
- **TUI**: gocui
- **Testing**: testify

## Branch Strategy (Git Flow)

- `develop` — active development, dirty work goes here
- `master` — stable code, squash-merge from develop
- `release` — release branch, MR from master when ready

All work starts in `develop`. MR to `master` with squash. MR to `release` when a release is warranted.

## Development Priorities

1. Update dependencies, improve test coverage
2. Add support for newer PostgreSQL versions (statistics view changes)
3. New features (TBD)

## Build & Test

```bash
make build     # build binary to ./bin/pgcenter
make test      # run tests with race detector + coverage
make lint      # golangci-lint + gosec
```

## Project Structure

```
cmd/           # CLI entry points (top, record, report, profile)
internal/
  align/       # column alignment logic
  math/        # math utilities for stats
  postgres/    # PostgreSQL connection and queries
  pretty/      # pretty-printing
  query/       # SQL queries for stats views
  stat/        # statistics collection and processing
  version/     # version info (injected via ldflags)
  view/        # TUI views
```

## Key Conventions

- PostgreSQL statistics are queried via `internal/query` — one file per stats view
- New PG version support means updating queries in `internal/query` to handle schema changes
- TUI rendering lives in `internal/view`
- Tests require a running PostgreSQL instance (integration tests)

## Release

Built and released via goreleaser (`.goreleaser.yml`). Docker image pushed to DockerHub as `lesovsky/pgcenter`.
