# pgcenter — Project Instructions

## Project

pgcenter is a command-line administration tool for observing and troubleshooting PostgreSQL.
Its main interface is a top-like interactive TUI over PostgreSQL and Linux system statistics.
It also records statistics for later analysis, replays recorded reports, and profiles wait events.

Repository: https://github.com/lesovsky/pgcenter

## Technology

- Go 1.26+
- Cobra and pflag for the CLI
- pgx/v5, including pgx/v5/pgconn for PostgreSQL errors
- gocui for the interactive TUI
- testify for tests
- golangci-lint, gosec, and govulncheck for quality checks

## Project Knowledge

Load the `project-knowledge` skill when planning, implementing, reviewing, testing, documenting,
or deploying project changes. Read the references relevant to the task rather than loading every
document indiscriminately:

- `.kimi-code/skills/project-knowledge/references/project.md` — capabilities, audience, and supported PostgreSQL versions
- `.kimi-code/skills/project-knowledge/references/architecture.md` — packages, data flow, view configuration, and architectural invariants
- `.kimi-code/skills/project-knowledge/references/patterns.md` — implementation and testing conventions, Git workflow, and manual TUI verification
- `.kimi-code/skills/project-knowledge/references/deployment.md` — CI, release process, containers, and operational requirements

Verify documentation against the code when they disagree. Update affected knowledge during feature
finalization, and update stale in-code comments when a change invalidates them.

## Build and Verification

```bash
make build   # build ./bin/pgcenter
make test    # race detector and coverage
make lint    # golangci-lint and gosec
make vuln    # govulncheck
```

The complete PostgreSQL-backed suite runs in `lesovsky/pgcenter-testing:0.0.11` with clusters for
PostgreSQL 14–19. A host run that skips unavailable integration tests is not the full verification
gate. CI also runs `testing/e2e.sh`.

For manual TUI verification, always start with a fresh `make build`. Use deterministic tmux geometry,
capture plain text or ANSI output as appropriate, and compare with a binary built from `master` when
classifying regressions. Follow the detailed regimen in
`.kimi-code/skills/project-knowledge/references/patterns.md`.

## Git and Feature Workflow

- `develop` is the active development branch.
- `master` is the stable branch; release changes reach it through reviewed squash merges.
- Pushing `master` to `release` triggers the release workflow after tagging.
- Feature development uses six phases: `spec-writer`, `tech-spec-planning`, `decompose-tech-spec`,
  `feature-execution`, `freeze-qa`, and `done`.
- Keep parallel task worktrees under the ignored `.worktrees/` directory.
- Keep orchestrator state in the ignored `.feature-orchestration.md` file.
- Preserve `docs/features/archive/` and `docs/metrics/` as historical evidence. Do not rewrite old
  artifacts merely to match a newer development process.

## Change Constraints

- Active PostgreSQL support is 14–19; older code paths remain unless their removal is explicitly approved.
- pgcenter targets Linux. Local procfs and remote PL/Perl collection have different availability rules.
- Make the minimum change that satisfies the approved contract and match existing package conventions.
- Do not add dependencies, configuration surfaces, or record/report compatibility changes speculatively.
- Treat recorded archives as a compatibility surface and replay them using the recorded PostgreSQL version.
