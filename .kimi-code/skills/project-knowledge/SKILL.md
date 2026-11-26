---
name: project-knowledge
description: Reads pgcenter's architecture, stack, conventions, statistics views, deployment, and operational rules when a task needs project-specific context.
whenToUse: When planning, implementing, reviewing, testing, documenting, or deploying changes in pgcenter.
---

# pgcenter Project Knowledge

Project documentation is the single source of truth for stable project knowledge. Read the files
relevant to the task rather than loading everything indiscriminately:

- **[project.md](references/project.md)** — overview, audience, capabilities, supported statistics, and PostgreSQL version scope.
- **[architecture.md](references/architecture.md)** — package structure, data flow, version-aware queries, TUI state, and architectural invariants.
- **[patterns.md](references/patterns.md)** — implementation and testing conventions, Git workflow, and manual TUI verification.
- **[deployment.md](references/deployment.md)** — CI, release process, test containers, packages, and Docker images.

Verify facts against code when documentation and implementation disagree, report the discrepancy,
and update stale documentation during feature finalization. Roadmaps, feature contracts, ADRs, and
tech debt live under `docs/`, not in these stable knowledge references.
