# pgcenter — Deployment & Release

## Release Process

1. All work merged to `master` via squash PRs from `develop`
2. Create and push tag: `git tag vX.Y.Z && git push origin vX.Y.Z`
3. Push master to release branch: `git push origin master:release`
4. `release.yml` workflow triggers automatically

## Release Workflow (.github/workflows/release.yml)

Triggered by push to `release` branch. Two jobs:

**test** — runs full test suite in `lesovsky/pgcenter-testing:0.0.10` container:
- Go 1.25.10 (cached), module cache, lint tools cache
- `make lint` + `govulncheck ./...` + `make test` + `make build` + e2e tests

**release** — runs after test passes:
- `docker/login-action@v3` with `DOCKER_USERNAME` / `DOCKER_PASSWORD` secrets
- `goreleaser/goreleaser-action@v6` creates GitHub Release + pushes Docker image

## GoReleaser (.goreleaser.yml)

Builds for `linux/amd64` and `linux/arm64`.
Produces: `.tar.gz` archives, `.deb`, `.rpm`, `.apk` packages, `checksums.txt`.
Docker image pushed to `lesovsky/pgcenter:vX.Y.Z` and `:latest`.

## CI (default.yml)

Triggered on every push. Same container and tooling as release test job.
Container: `lesovsky/pgcenter-testing:0.0.10`

## Test Container (lesovsky/pgcenter-testing)

Source: `testing/Dockerfile`. Current version: `0.0.10`.
Contains: Ubuntu 22.04, PostgreSQL 14–18 with `plperlu` + CPAN modules.
Clusters created via `pg_createcluster` (Ubuntu 22.04 Docker doesn't auto-init).
Ports: PG14=21914, PG15=21915, PG16=21916, PG17=21917, PG18=21918.

To rebuild and push:
```
docker build -t lesovsky/pgcenter-testing:X.Y.Z testing/
docker push lesovsky/pgcenter-testing:X.Y.Z
```
Then update container tag in both `default.yml` and `release.yml`.

## GitHub Secrets Required

| Secret | Purpose |
|--------|---------|
| `DOCKER_USERNAME` | DockerHub login |
| `DOCKER_PASSWORD` | DockerHub password |
| `GITHUB_TOKEN` | Auto-provided by GitHub Actions for release creation |

## Docker Image (lesovsky/pgcenter)

Built from `Dockerfile` (two-stage: `golang:1.25-alpine` → `alpine:3.21`).
Published on release to DockerHub as `lesovsky/pgcenter:vX.Y.Z` and `:latest`.

Usage:
```
docker pull lesovsky/pgcenter:latest
docker run -it --rm lesovsky/pgcenter:latest pgcenter top -h <host> -U <user> -d <db>
```
