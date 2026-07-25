---
status: planned
depends_on: ["01"]
wave: 2
skills: [code-writing]
verify: "bash — go test ./internal/postgres/..."
reviewers: [dev-code-reviewer, dev-security-auditor, dev-test-reviewer]
teammate_name:
---

# Task 02: Version constant, port mapping, and test-connection hardening

## Required Skills

Перед выполнением задачи загрузи:
- `/skill:code-writing` — [skills/code-writing/SKILL.md](~/.claude/skills/code-writing/SKILL.md)

## Description

This is the foundation task for PG 19 support: every later task in the feature (the version-aware progress
selectors in Task 3, the test-suite sweep in Task 4) references the PG 19 version constant and connects to
the PG 19 test cluster that Task 1 added to the testing image. Nothing in the feature compiles or runs
against PG 19 until this task lands, so it is deliberately small and sits alone in its wave.

Three changes, in one logical unit — "teach the codebase that PostgreSQL 19 exists":

1. **Version constant.** `internal/query/query.go` holds the single block of `PostgresVNN` constants that
   every version gate in the project compares against (`if version >= PostgresV18 { … }` in the selectors —
   see `internal/query/io.go:88`). The block currently ends at `PostgresV18 = 180000`; PG 19 needs its entry
   there before any selector can branch on it.

2. **Test cluster port.** `internal/postgres/testing.go` maps a numeric server version to the port its
   cluster listens on inside the `pgcenter-testing` image. Task 1 created the PG 19 cluster; this map is how
   the Go test suite reaches it.

3. **Test-connection hardening (Decision 5).** `NewTestConnectVersion` today falls back to `ports[140000]`
   for any version it has no mapping for. That fallback is actively dangerous for this feature: a forgotten
   map entry would make every "PG 19" subtest connect to PG 14, pass, and report green coverage for a server
   that was never exercised. The function's own doc comment already promises the opposite behaviour
   ("Returns an error if the requested version is not available in the test environment"), so the code
   contradicts its documented contract. Replace the fallback with an error. The user explicitly rejected the
   alternative of keeping the fallback and adding an acceptance check on top of it — fix the cause, not the
   symptom.

The hardening is a behaviour change, not a refactor, which is why this task carries a TDD anchor and a
dedicated unit test in a package that currently has no test for this function.

## What to do

1. Add the PG 19 constant to the version block in `internal/query/query.go`, following the existing naming
   and value convention of the block (the block currently ends at `PostgresV18 = 180000`; derive the new
   name and value from the established pattern rather than inventing a new one). No other edit to that file.

2. Add the PG 19 entry to the `ports` map in `internal/postgres/testing.go`, in the "active versions" group
   at the top of the map, above the existing entries. The port value follows the convention used by
   `testing/prepare-test-environment.sh`, which derives every cluster's port from its major version
   (`port="219${v}"`) — read that script and derive the PG 19 port from it rather than guessing. Confirm the
   value matches the cluster Task 1 actually started.

3. Replace the unmapped-version fallback in `NewTestConnectVersion` with an error return that names the
   version and states that no test cluster port mapping exists for it. Plain `fmt.Errorf`, no sentinel error
   type, no wrapping — matching the error style already used in `internal/postgres/postgres.go`. The `fmt`
   import has to be added; the file currently imports nothing.

4. Write the unit test for the new behaviour first (see TDD Anchor). The `internal/postgres` package has no
   test file for `testing.go` today — create one following the naming convention of the package's existing
   tests (`postgres_test.go`, `connopts_test.go`).

5. Re-derive the blast radius before and after the change rather than trusting this document: grep for every
   `NewTestConnectVersion` call site and for the version lists those call sites loop over, and confirm the
   union of versions actually passed is a subset of the port map. Then confirm the whole suite still
   compiles (`go build ./... && go vet ./...`), because the function is called from many packages.

**Explicitly out of scope:** do not repoint `NewTestConnect` or `NewTestConfig` at PG 19. Both pin the
default fixture cluster (PG 17, port 21917) that the record/report/top tests run against; moving them would
silently change the PostgreSQL version the entire non-`internal/query` suite executes under. Adding PG 19 to
the per-version test lists is Task 4's job, not this task's.

## TDD Anchor

<!-- Fill if task includes writing code. For non-code tasks (user instructions, deploy, config) — delete this section. -->

Тесты, которые нужно написать ДО реализации. Пишем → запускаем → убеждаемся что падают → пишем код → убеждаемся что проходят.

The behaviour change in `NewTestConnectVersion` is the part that needs tests. Both cases go in a new
`internal/postgres/testing_test.go`; use table-driven subtests if that reads better than two functions.

- `internal/postgres/testing_test.go::TestNewTestConnectVersion_UnmappedVersion` — a version absent from the
  ports map (e.g. an implausible future/never-shipped numeric version such as `999000`) returns a non-nil
  error and a nil `*DB`. The error message mentions the requested version. Must fail before the fix, because
  today the call silently connects to the PG 14 cluster and returns a usable connection.
- `internal/postgres/testing_test.go::TestNewTestConnectVersion_MappedVersion` — a mapped version still
  returns a working connection with no error, proving the hardening did not break the happy path. Cover the
  newly added PG 19 mapping here. Follow the package's existing convention for tests that need a live
  server: if the cluster is unavailable, `t.Skipf` rather than fail (see the `t.Skipf` handling described in
  `patterns.md` "Adding a New PostgreSQL Version").

Note the asymmetry deliberately: the unmapped-version case must NOT require a live database — it has to fail
before any connection attempt, which is the whole point of the change. If the test only passes when a
cluster is running, the implementation is wrong.

## Acceptance Criteria

- [ ] The PG 19 constant exists in the `internal/query/query.go` version block, named and valued
      consistently with the constants above it.
- [ ] The `ports` map in `internal/postgres/testing.go` has the PG 19 entry, in the active-versions group,
      with the port derived from `testing/prepare-test-environment.sh`'s convention and matching the running
      cluster from Task 1.
- [ ] `NewTestConnectVersion` returns an error for a version with no port mapping — no fallback to
      `ports[140000]`, no connection attempt, nil `*DB`.
- [ ] The error is a plain `fmt.Errorf` naming the version and the missing mapping; no sentinel error type
      is introduced.
- [ ] The function's doc comment and its behaviour now agree (the comment needs no change — the code was the
      thing that was wrong).
- [ ] Both TDD anchor tests exist and pass; the unmapped-version test passes with no PostgreSQL running.
- [ ] Every existing `NewTestConnectVersion` call site still behaves identically — re-verified by grep, not
      assumed.
- [ ] `go build ./... && go vet ./...` clean; `go test ./internal/postgres/...` green.
- [ ] `make lint` clean.

## Context Files

**Feature artifacts:**
- [012-feat-pg19-compatibility-baseline.md](012-feat-pg19-compatibility-baseline.md) — user-spec
- [012-feat-pg19-compatibility-baseline-tech-spec.md](012-feat-pg19-compatibility-baseline-tech-spec.md) —
  tech-spec; this task is "Task 2" in Implementation Tasks, and **Decision 5** is the source of truth for
  the hardening
- [012-feat-pg19-compatibility-baseline-decisions.md](012-feat-pg19-compatibility-baseline-decisions.md) —
  decisions log (append the task report here)
- [012-feat-pg19-compatibility-baseline-code-research.md](012-feat-pg19-compatibility-baseline-code-research.md)
  — codebase research; contains the call-site inventory for `NewTestConnectVersion`

**Project knowledge:**
- [overview.md](../../../.claude/skills/project-knowledge/overview.md) — what pgcenter is (this project has
  no `project.md`; `overview.md` is its equivalent)
- [architecture.md](../../../.claude/skills/project-knowledge/architecture.md) — package layout and PG
  version handling
- [patterns.md](../../../.claude/skills/project-knowledge/patterns.md) — "Adding a New PostgreSQL Version"
  (step 1 is literally this task's port map entry), "Version-Specific Query Pattern", "Error Wrapping",
  "Naming Conventions"
- [deployment.md](../../../.claude/skills/project-knowledge/deployment.md) — the pgcenter-testing image the
  port map points at

**Code files:**
- [internal/query/query.go](../../../internal/query/query.go) — modify: add the PG 19 version constant
- [internal/postgres/testing.go](../../../internal/postgres/testing.go) — modify: add the PG 19 port, remove
  the fallback
- [internal/postgres/postgres.go](../../../internal/postgres/postgres.go) — read: `Connect` and the
  `fmt.Errorf` style to match
- [internal/query/io.go](../../../internal/query/io.go) — read: how a version constant is consumed by a
  selector (`SelectStatIOQuery`, the `version >= PostgresV18` gate) — the shape Task 3 will copy
- [testing/prepare-test-environment.sh](../../../testing/prepare-test-environment.sh) — read: source of the
  port numbering convention
- [internal/postgres/postgres_test.go](../../../internal/postgres/postgres_test.go) — read: test style of
  the package the new test file joins

## Verification Steps

- `go test ./internal/postgres/...` — green, including both new tests.
- Run the unmapped-version test with no PostgreSQL reachable (stop the local clusters or run it in isolation
  via `go test -run TestNewTestConnectVersion_UnmappedVersion ./internal/postgres/`) — it must still pass,
  proving the error short-circuits before the connection attempt.
- `git stash` the `testing.go` change and confirm the unmapped-version test fails — the TDD red step,
  evidence that the test actually pins the new behaviour.
- `go build ./... && go vet ./...` — the whole tree still compiles; no call site broke.
- `go test ./internal/query/...` against the local test image from Task 1 — no regression in the packages
  that call `NewTestConnectVersion` most heavily.
- `make lint` — clean.

## Details

<!-- All details for task execution — technical, organizational, any other. -->

**Files:**

- `internal/query/query.go` — current state: a single `const (...)` block at the top of the file listing
  `PostgresV94` through `PostgresV18`, one constant per major version, numeric server-version values. The
  rest of the file is the `Options` struct and `NewOptions`. Change: one line appended to that block for
  PG 19. Nothing else in this file belongs to this task.

- `internal/postgres/testing.go` — current state: 45 lines, no imports at all. Three functions:
  `NewTestConfig()` (pins port 21917), `NewTestConnect()` (delegates to `NewTestConnectVersion(170000)`),
  and `NewTestConnectVersion(version int)` which holds the `ports` map (twelve entries, split by comment
  into "active versions" 180000..140000 and "EOL versions kept for reference" 130000..90400), looks the
  version up, and on a miss assigns `port = ports[140000]` before building the config and calling
  `Connect`. Changes: add the PG 19 entry to the active group; replace the `if !ok { port = ports[140000] }`
  fallback with an error return; add the `fmt` import. Leave `NewTestConfig` and `NewTestConnect` alone.

- `internal/postgres/testing_test.go` — new file, package `postgres` (the existing tests in this package are
  internal, not `postgres_test`).

**Dependencies:**

- Task 01 (wave 1) — builds the testing image with the PG 19 cluster and leaves it running. The
  mapped-version test needs that cluster to actually verify the new port; without it the test skips rather
  than fails, but then the port value is unverified, so run against Task 1's local image.
- No new Go packages. `fmt` is stdlib and already used throughout the package.
- Downstream: Task 3 consumes the version constant in its selectors; Task 4 adds the PG 19 literal to the
  per-version test lists that feed `NewTestConnectVersion`. Neither can start until this lands.

**Edge cases:**

- **Unmapped version → no connection attempt.** The error must be returned before `NewConfig`/`Connect`, so
  the failure is instant and works with no server running. Returning an error only after a failed dial would
  technically satisfy "returns an error" but not the intent.
- **EOL versions stay in the map.** They are mapped but their clusters are not running, so they still fail
  at connect time with a connection error, not the new mapping error. That distinction is intentional:
  "version I don't know about" and "version whose cluster isn't up" are different failures, and callers'
  `t.Skipf` handling covers the second. Do not remove or reorganise the EOL entries.
- **Existing callers.** Re-derive the count by grep rather than trusting any document — an independent check
  found 37 call sites taking a loop variable plus one internal call passing a literal from `NewTestConnect`,
  where the tech-spec had said 41. What matters is the invariant, not the number: every version reaching the
  map is present in it. Note also what the hardening does and does not buy: a forgotten map entry now makes
  the affected subtests **skip** instead of silently exercising the oldest cluster. Skipping is honest but
  still green in CI, so the one-off stopped-cluster check in the QA task remains the thing that proves the
  new version is actually reached.
- All `NewTestConnectVersion(version)` call sites pass a loop variable rather than a
  literal; the versions come from `versions := []int{...}` lists in the test files. The union of those
  literals today is `{90500, 90600, 100000, 110000, 120000, 130000, 140000, 150000, 160000, 170000, 180000}`
  — a strict subset of the port map, so no existing caller changes behaviour. **Re-derive this with a grep
  instead of trusting the numbers above**; if a version outside the map turns up, stop and report rather
  than papering over it by widening the map.
- **Port collision.** Verify the PG 19 port is not already taken by another entry in the map — the EOL 9.x
  rows use a different numbering shape (`21996`/`21995`/`21994`) than the modern `219NN` rows.

**Implementation hints:**

- Keep the diff minimal — this task should be roughly three edited lines plus a new test file. Resist
  tidying the surrounding map, comments, or the two sibling helpers.
- The `ports` map is a local variable rebuilt on every call. That is pre-existing and not this task's
  problem; do not hoist it to a package-level var.
- Follow `patterns.md` "Adding a New PostgreSQL Version" — its step 1 is exactly the port map entry, and its
  step 2 (the test version lists) is deliberately Task 4's, not yours.
- Error text: name the version and say there is no test cluster port mapping for it. Lowercase, no trailing
  punctuation, per Go convention and the existing `fmt.Errorf("failed connection establishing: %w", err)`
  style in `postgres.go`.
- `NewTestConnectVersion` is test-only scaffolding, not production code — but it is compiled into the
  non-test package `postgres`, so `make lint`/gosec see it like any other function.

## Reviewers

- **dev-code-reviewer** → `docs/features/012-feat-pg19-compatibility-baseline/012-feat-pg19-compatibility-baseline-task-02-dev-code-reviewer-review.json`
- **dev-security-auditor** → `docs/features/012-feat-pg19-compatibility-baseline/012-feat-pg19-compatibility-baseline-task-02-dev-security-auditor-review.json`
- **dev-test-reviewer** → `docs/features/012-feat-pg19-compatibility-baseline/012-feat-pg19-compatibility-baseline-task-02-dev-test-reviewer-review.json`

## Post-completion

- [ ] Записать краткий отчёт в [012-feat-pg19-compatibility-baseline-decisions.md](012-feat-pg19-compatibility-baseline-decisions.md) (Summary: 1-3 предложения, ревью со ссылками на JSON, без таблиц файндингов и дампов)
- [ ] Записать фактическое число call sites `NewTestConnectVersion` и подтверждённое множество версий (результат grep из шага 5) — Decision 5 явно требует переподтверждения вместо доверия tech-spec
- [ ] Если отклонились от спека — описать отклонение и причину
- [ ] Обновить user-spec/tech-spec если что-то изменилось
