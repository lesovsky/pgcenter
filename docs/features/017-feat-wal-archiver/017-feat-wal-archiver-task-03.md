---
status: planned                    # planned -> in_progress -> done
depends_on: ["01"]                 # ID задач-зависимостей (строки: ["01", "02"])
wave: 2                            # волна параллельного выполнения
skills: [code-writing]             # МАССИВ скиллов для загрузки
verify: "bash — go test ./internal/query/... ./internal/stat/...; the aggregate returns a number under a role holding only pg_monitor"
reviewers: [dev-code-reviewer, dev-security-auditor, dev-test-reviewer]  # явно указать. Пусто = fallback на defaults
teammate_name:                     # имя агента-исполнителя (опционально; если не задано — генерируется по описанию задачи)
---

# Task 03: Verbose panel backlog on a pg_monitor-accessible function

> **Moved from Wave 1 to Wave 2 (tech-spec Decision 19).** This task and Task 01 both need a
> role-creation / `SET ROLE` test helper, and both add test code to the **same Go package**
> (`internal/query`) — different files, one package namespace, so two package-level helpers would not
> compile and two differently-named copies would be duplicated logic. Task 01 now owns a single shared
> helper in `internal/postgres/testing.go`; this task reuses it and must not define its own.

## Required Skills

Перед выполнением задачи загрузи:
- `/skill:code-writing` — [skills/code-writing/SKILL.md](~/.claude/skills/code-writing/SKILL.md)

## Description

The verbose panel's WAL-archiving backlog (`OverviewArchivingBacklog`, `internal/query/overview.go:92-102`)
today reads `pg_ls_dir('pg_wal/archive_status')`. That function's ACL is `{postgres=X/postgres}` —
**superuser only**. `pg_ls_waldir` and `pg_ls_archive_statusdir` are `{postgres, pg_monitor}`. Measured
live on PG 14.23 / 18.4 / 19beta2 (code-research §10.A.2) — do not re-derive it.

The consequence is the whole reason this task exists: a role holding only `pg_monitor` — the most
common monitoring role, and the one the panel was written for — gets `ERROR: permission denied for
function pg_ls_dir` on every tick, the field degrades to `n/a`, and the operator never gets the first
signal that archiving has stopped. ADR [010] recorded `pg_monitor` as sufficient; it is not.
Tech-spec Decision 8 supersedes that ADR.

This task switches the `FROM` clause to `pg_ls_archive_statusdir()` and keeps the **output contract
byte-for-byte identical**: one `bigint` column, bytes, `count(.ready) × wal_segment_size`.

**Scope fence — read this twice.** The degrade-to-`n/a` mechanism in `collectOverviewStat`
(`internal/stat/postgres.go:288-295` — own `QueryRow`, swallowed error, `ArchivingBacklogValid` gate)
is **NOT changed**. The renderer (`top/stat.go:722-723`) is **NOT touched**. Only the SQL function and
four comments change. Anything else in this task is scope creep.

Four stale comments are corrected here, in the same task, because they are what a future reader would
trust — and because they are exactly how the wrong privilege assumption survived into ADR [010] and
went unnoticed by the test suite:

- `internal/query/overview.go:92-99` — asserts `pg_ls_dir` requires "pg_monitor/superuser", and
  justifies swallowing the error text because it contains a filesystem path. Both arguments stop
  applying once the function changes (`pg_ls_archive_statusdir()` takes no path argument).
- `internal/stat/postgres.go:288-290` — the same two claims, repeated at the consumer.
- `internal/query/overview_test.go:124-125` — claims the fixtures role "has access" via
  pg_monitor/superuser.
- `internal/stat/postgres_test.go:224` — claims "the fixtures role has pg_monitor". The fixtures role
  is `postgres`, a **superuser**. That is why the current broken query passes the suite today.

`docs/decisions-log.md` ADR [010] states the privilege requirement incorrectly too — **do not edit the
ADR log in this task.** Amending it happens at feature finalization.

## What to do

1. Write the failing tests first (see TDD Anchor) and observe them RED against the current
   `pg_ls_dir` query. A test that only runs as the fixture superuser proves nothing here — it is
   precisely the test that passes today with the broken query.
2. Change `OverviewArchivingBacklog`'s `FROM` clause to `pg_ls_archive_statusdir()`, dropping the
   `AS name` relation alias. The `SELECT` list stays character-identical.
3. Rewrite the doc comment above the constant: state the real privilege requirement
   (`pg_ls_archive_statusdir()` — superuser or `pg_monitor`), keep the own-`QueryRow` requirement and
   its real reason (the field must degrade to `n/a` without aborting the whole overview sample), and
   drop the filesystem-path argument that no longer applies.
4. Rewrite the consumer comment in `collectOverviewStat` the same way — the code below it does not
   change.
5. Correct the two test comments that misdescribe the fixtures role: it is `postgres`, a superuser,
   not a `pg_monitor` grantee.
6. Confirm the new tests go GREEN, and confirm the pre-existing
   `Test_ArchivingBacklogQuery_Degrades` and `Test_collectOverviewStat_Degradation` still pass
   unchanged in behaviour (their comments change, their assertions do not).
7. Run each mutation listed in Acceptance Criteria and see the named test go RED before believing the
   green.

## TDD Anchor

Тесты, которые нужно написать ДО реализации. Пишем → запускаем → убеждаемся что падают → пишем код → убеждаемся что проходят.

- `internal/query/overview_test.go::Test_ArchivingBacklogQuery_PgMonitorRole` — for each version in
  `overviewVersions` (PG 14–19, `t.Skipf` on unavailable clusters): create a `NOLOGIN`, non-superuser
  role, `GRANT pg_monitor` to it, `SET ROLE` to it, **assert the session is actually restricted**
  (`current_user` equals the role name and `rolsuper` is false for it), then scan
  `OverviewArchivingBacklog` into an `int64` — `assert.NoError` and `>= 0`. `RESET ROLE` in a
  `defer`. **RED today**: fails with `permission denied for function pg_ls_dir`.
- `internal/query/overview_test.go::Test_ArchivingBacklogQuery_NoPrivilegeRole` — same shape, but the
  role is granted nothing: the query must return a permission error (`42501`). Proves the new
  function is not a privilege downgrade — it is not readable by an unprivileged role.
- `internal/stat/postgres_test.go::Test_collectOverviewStat_PgMonitorRole` — under the same
  `pg_monitor`-only role on the default test cluster, `collectOverviewStat` returns
  `ArchivingBacklogValid == true` with `ArchivingBacklog >= 0`, and the rest of the sample stays
  populated (`Valid`, `DatabasesCount >= 1`). **RED today**: `ArchivingBacklogValid` is false because
  the aggregate 42501s. This is the end-to-end proof that the panel now shows a number for the role
  the feature exists to serve.
- Unchanged, must keep passing: `internal/query/overview_test.go::Test_ArchivingBacklogQuery_Degrades`
  and `internal/stat/postgres_test.go::Test_collectOverviewStat_Degradation` — assertions untouched,
  only their comments corrected.

## Acceptance Criteria

Each item below names a **mutation and the test it must turn RED**. Run the mutation, see red, revert.
Green alone is not evidence (patterns.md, "Extract the decision out of the unreachable closure").

- [ ] `OverviewArchivingBacklog` reads `FROM pg_ls_archive_statusdir()` with no `AS name` alias; the
      `SELECT` list is unchanged and the query still yields one non-NULL `bigint` in bytes.
- [ ] **Mutation:** revert the `FROM` clause to `pg_ls_dir('pg_wal/archive_status') AS name` →
      `Test_ArchivingBacklogQuery_PgMonitorRole` and `Test_collectOverviewStat_PgMonitorRole` must go
      RED. If they stay green, the tests are not running under the restricted role.
- [ ] **Mutation:** delete the `SET ROLE` statement from `Test_ArchivingBacklogQuery_PgMonitorRole`
      (leaving the session as the fixture superuser) → the test must go RED on its own
      `current_user` / `rolsuper` guard. A test that would still pass here is a vacuous gate.
- [ ] **Mutation:** remove the `GRANT pg_monitor` from the role setup →
      `Test_ArchivingBacklogQuery_PgMonitorRole` must go RED with a permission error.
- [ ] **Mutation:** add `GRANT pg_monitor` to the deny role in
      `Test_ArchivingBacklogQuery_NoPrivilegeRole` → that test must go RED (it asserts a permission
      error).
- [ ] Role creation is idempotent (safe to re-run against a cluster where the role already exists),
      the roles are `NOLOGIN` and non-superuser, and every test that calls `SET ROLE` issues
      `RESET ROLE` via `defer` so no assumed role leaks into later assertions (Decision 18).
- [ ] `Test_ArchivingBacklogQuery_Degrades` and `Test_collectOverviewStat_Degradation` pass with their
      assertions unmodified.
- [ ] All four stale comments are corrected: no comment in the tree still claims `pg_ls_dir` requires
      "pg_monitor/superuser", justifies the error swallow by a filesystem path in the message, or
      claims the fixtures role holds `pg_monitor`.
- [ ] `collectOverviewStat`'s error-swallow + `ArchivingBacklogValid` degradation path is
      byte-identical apart from its comment; `top/stat.go` is not touched.
- [ ] A cluster whose `archive_status` directory is absent now reports `0 B` instead of `n/a`
      (`pg_ls_archive_statusdir()` is `missing_ok=true`). This is accepted by Decision 11 — it must
      **not** be "fixed", worked around, or guarded against in this task.
- [ ] `go test ./internal/query/... ./internal/stat/...` passes inside the CI image; `make lint` and
      `make vuln` are clean on the host.

## Context Files

**Feature artifacts:**
- [017-feat-wal-archiver.md](docs/features/017-feat-wal-archiver/017-feat-wal-archiver.md) — user-spec
- [017-feat-wal-archiver-tech-spec.md](docs/features/017-feat-wal-archiver/017-feat-wal-archiver-tech-spec.md) — tech-spec (Task 3 in Wave 1; Decisions 8, 11, 18)
- [017-feat-wal-archiver-code-research.md](docs/features/017-feat-wal-archiver/017-feat-wal-archiver-code-research.md) — §10.A has the exact current SQL, the live ACL measurement, the consumer, and the affected tests; §8 has the CI-image command
- [017-feat-wal-archiver-decisions.md](docs/features/017-feat-wal-archiver/017-feat-wal-archiver-decisions.md) — decisions log

**Project knowledge:**
- [overview.md](.claude/skills/project-knowledge/overview.md) — what pgcenter is, supported stats
- [architecture.md](.claude/skills/project-knowledge/architecture.md) — package layout, `internal/query` → `internal/stat` data flow, PG version handling
- [patterns.md](.claude/skills/project-knowledge/patterns.md) — Testing conventions, live-PG test shape, and the "name the mutation the test must fail on" rule

**Code files:**
- [internal/query/overview.go](internal/query/overview.go) — `OverviewArchivingBacklog` at `:92-102`: change the `FROM` clause, rewrite the doc comment
- [internal/query/overview_test.go](internal/query/overview_test.go) — add the two role-scoped tests; correct the comment at `:124-125`
- [internal/stat/postgres.go](internal/stat/postgres.go) — `collectOverviewStat` at `:288-295`: comment only, code unchanged
- [internal/stat/postgres_test.go](internal/stat/postgres_test.go) — add the collect-level role test; correct the comment at `:224`
- [docs/decisions-log.md](docs/decisions-log.md) — ADR [010] "Archiving backlog via `count(.ready) × wal_segment_size`" at `:654-666`; read-only in this task
- [internal/postgres/testing.go](internal/postgres/testing.go) — `NewTestConnect()` (PG 17) and `NewTestConnectVersion()` port map

## Verification Steps

- Before implementing: run the three new tests and confirm they are RED for the right reason —
  `permission denied for function pg_ls_dir` (not a typo, not a connection failure).
- After implementing, in the CI image (PG 14–19 fixtures; the clusters do not exist on the host):

  ```bash
  docker run --rm -v "$PWD":/work -v "$HOME/go/pkg/mod":/gomod \
    -e GOROOT=/gomod/golang.org/toolchain@v0.0.1-go1.25.12.linux-amd64 \
    -e GOPATH=/gopath -e GOFLAGS=-mod=mod -e HOME=/root \
    -w /work lesovsky/pgcenter-testing:0.0.11 bash -c '
      export PATH=$GOROOT/bin:$PATH
      prepare-test-environment.sh > /tmp/prep.log 2>&1 || { tail -30 /tmp/prep.log; exit 1; }
      go test -race -p 1 -timeout 300s ./internal/query/... ./internal/stat/...'
  ```

  Expected: all green, including `Test_ArchivingBacklogQuery_Degrades` and
  `Test_collectOverviewStat_Degradation` with unmodified assertions.
- Re-run the same command a second time without recreating the containers: the role-creating tests
  must still pass (idempotency).
- Run each mutation from Acceptance Criteria, confirm the named test goes RED, revert.
- Grep the tree for the stale claims — no hit may remain:
  `grep -rn "pg_ls_dir" internal/` and `grep -rn "has pg_monitor" internal/`.
- `make lint` and `make vuln` on the host.

## Details

<!-- All details for task execution — technical, organizational, any other. -->

**Files:**

- `internal/query/overview.go` — current text at `:100-102` is verbatim:
  ```go
  OverviewArchivingBacklog = "SELECT " +
      "count(*) FILTER (WHERE name LIKE '%.ready') * pg_size_bytes(current_setting('wal_segment_size')) AS backlog " +
      "FROM pg_ls_dir('pg_wal/archive_status') AS name"
  ```
  Only the last line changes, to `"FROM pg_ls_archive_statusdir()"`. The doc comment at `:92-99` is
  rewritten (see below).
- `internal/query/overview_test.go` — `overviewVersions` at `:13` is
  `{140000, 150000, 160000, 170000, 180000, 190000}`; reuse it. `Test_ArchivingBacklogQuery_Degrades`
  at `:123-146` keeps its assertions; its comment at `:124-125` is corrected. Two new tests are added.
- `internal/stat/postgres.go` — `:288-295`, comment only:
  ```go
  // replication: archiving backlog. OWN QueryRow: pg_ls_dir requires pg_monitor/superuser; a 42501
  // privilege error or archive_mode=off degrades this field to n/a. The raw error (which contains a
  // filesystem path) is deliberately swallowed and never logged or surfaced.
  ```
  The four lines of code below it (`sql.NullInt64`, `err == nil && backlog.Valid`, the two field
  assignments) stay exactly as they are.
- `internal/stat/postgres_test.go` — `Test_collectOverviewStat_Degradation` at `:206-236` keeps its
  assertions; the comment at `:224` is corrected. One new test is added.

**Dependencies:** none — `depends_on: []`. No new Go packages. Wave 1, alongside Tasks 1, 2 and 4.

**Coordination risk (read before writing test helpers):** Task 1 creates
`internal/query/archiver_test.go` in the **same Go package** (`query`) and also needs `SET ROLE`
plumbing. Two package-level helpers with the same name will not compile. If a suitable helper already
exists in the package when you start, reuse it; otherwise give yours an overview-specific name. The
same applies to the SQL role names — creation is idempotent, so a shared name is functionally safe
(`make test` runs with `-p 1` and these tests do not call `t.Parallel()`), but a Go identifier clash
is not. `internal/stat` is a separate package and needs its own helper.

**Edge cases:**

- **`AS name` must be dropped.** `pg_ls_dir(text)` returns `SETOF text` with an unnamed column, so
  `AS name` was doing double duty: aliasing the relation *and* supplying the column name that
  `FILTER (WHERE name LIKE …)` resolves against. `pg_ls_archive_statusdir()` is `SETOF record` with
  OUT parameters `name text, size bigint, modification timestamptz` — it already provides `name`.
  Keeping `AS name` renames the whole relation and the query fails with
  `column "name" does not exist`.
- **Type and NULL-ness are unchanged.** `count(*)` is `bigint`, `pg_size_bytes()` is `bigint`, the
  product is `bigint`; over an empty set `count(*)` is `0`, never NULL. The consumer's
  `sql.NullInt64` scan needs no change.
- **`missing_ok` differs.** `pg_ls_dir('pg_wal/archive_status')` is `missing_ok=false`;
  `pg_ls_archive_statusdir()` is `missing_ok=true` (verified live on PG 18.4 by moving the directory
  aside: old query errors, new one returns `0`). A cluster with no `archive_status` directory flips
  from `n/a` to a confident `0 B`. Accepted by Decision 11 — do not add machinery to restore `n/a`.
- **The fixtures run `archive_mode=off`** with no `archive_command`, so the backlog is always `0` in
  tests. The tests prove *privilege and executability*, not archiving behaviour — the behavioural
  check is the stand run in Task 10. Do not try to make the fixtures archive.
- **Test roles are created at test time** (Decision 18) — the test image is deliberately frozen and
  the tree contains no `CREATE ROLE`/`GRANT` today, so this is a new pattern in this repo. Make
  creation tolerant of an existing role, make the roles `NOLOGIN` and non-superuser, grant nothing
  beyond `pg_monitor` (never `CREATEROLE`, never `SUPERUSER`), and always `RESET ROLE` in a `defer`.
- **PG version spread:** `pg_ls_archive_statusdir()` exists from PG 12; the matrix here starts at
  PG 14, so every fixture has it. No version branching in this query.

**Implementation hints:**

- The rewritten doc comment above `OverviewArchivingBacklog` should carry, concisely: what it returns
  (bytes, `count(.ready) × wal_segment_size`); that the function is `pg_ls_archive_statusdir()`, which
  superuser **and** `pg_monitor` can execute (unlike `pg_ls_dir`, which is superuser-only — worth one
  clause, since that is the bug being fixed and Decision 8 supersedes ADR [010]); that it MUST run as
  its own `QueryRow` so any error degrades only this field to `n/a` instead of aborting the sample;
  and that the function is `missing_ok=true`, so a missing `archive_status` yields `0`, not an error.
  Do not restate the filesystem-path argument.
- The consumer comment in `collectOverviewStat` says the same thing in one or two lines: own
  `QueryRow` so a privilege error or `archive_mode=off` degrades this field alone; the error is
  swallowed rather than surfaced. Keep it a comment change only.
- Idempotent role creation is easiest as a `DO $$ … $$` block guarded on `pg_roles`, executed through
  `db.Exec` (`internal/postgres.DB` exposes `Exec`, `Query`, `QueryRow`; pgx runs in simple-protocol
  mode, so multi-statement DDL strings are acceptable).
- The anti-vacuous guard is the load-bearing part of the `pg_monitor` test: assert `current_user` and
  the role's `rolsuper = false` **before** running the aggregate, so deleting the `SET ROLE` cannot
  leave the test silently passing as `postgres`.
- Assert the deny-role failure by SQLSTATE `42501` (pgx exposes it via `*pgconn.PgError`, already a
  project dependency) rather than by matching English message text.
- Follow the existing live-PG test shape in this file: `postgres.NewTestConnectVersion(version)` with
  `t.Skipf` when the cluster is unavailable, and `conn.Close()` per iteration.

## Reviewers

- **dev-code-reviewer** → `docs/features/017-feat-wal-archiver/017-feat-wal-archiver-task-03-dev-code-reviewer-review.json`
- **dev-security-auditor** → `docs/features/017-feat-wal-archiver/017-feat-wal-archiver-task-03-dev-security-auditor-review.json`
- **dev-test-reviewer** → `docs/features/017-feat-wal-archiver/017-feat-wal-archiver-task-03-dev-test-reviewer-review.json`

## Post-completion

- [ ] Записать краткий отчёт в [017-feat-wal-archiver-decisions.md](docs/features/017-feat-wal-archiver/017-feat-wal-archiver-decisions.md) (Summary: 1-3 предложения, ревью со ссылками на JSON, без таблиц файндингов и дампов)
- [ ] Если отклонились от спека — описать отклонение и причину
- [ ] Обновить user-spec/tech-spec если что-то изменилось
- [ ] ADR [010] в `docs/decisions-log.md` остаётся нетронутым в этой задаче — поправка на Decision 8 делается при финализации фичи
