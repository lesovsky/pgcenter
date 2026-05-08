# pgcenter — Project Constitution

## Project

pgcenter is a command-line admin tool for observing and troubleshooting PostgreSQL.
It provides a top-like interactive TUI over PostgreSQL statistics views.

Repository: https://github.com/lesovsky/pgcenter
Latest release: v0.10.0 (May 2026)

## Tech Stack

- **Language**: Go 1.25+
- **CLI framework**: cobra
- **PostgreSQL driver**: pgx/v5 (pgx/v5/pgconn for error types)
- **TUI**: gocui
- **Testing**: testify
- **Linting**: golangci-lint, gosec, govulncheck

## Branch Strategy (Git Flow)

- `develop` — active development
- `master` — stable, squash-merge from develop
- `release` — triggers release workflow on push

## Build & Test

```bash
make build   # build to ./bin/pgcenter
make test    # race detector + coverage
make lint    # golangci-lint + gosec
make vuln    # govulncheck
```

## Project Knowledge

See `.claude/skills/project-knowledge/` for:
- `overview.md` — features, supported stats, target audience
- `architecture.md` — package layout, data flow, PG version handling
- `patterns.md` — code patterns, testing conventions, version branching
- `deployment.md` — release process, CI/CD, Docker image
