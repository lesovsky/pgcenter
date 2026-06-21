---
created: 2026-06-21
status: approved
branch: feature/replication-slots
size: M
---

# Tech Spec: replication slots screen (pg_replication_slots + pg_stat_replication_slots)

## Solution

Add a new multi-row TUI screen `replslots` (hotkey `o`) to `pgcenter top`, showing one row
per replication slot. Data comes from a single hybrid query —
`pg_replication_slots LEFT JOIN pg_stat_replication_slots ON slot_name` — covering both
physical and logical slots. State columns (slot_name, slot_type, active, wal_status, retained
WAL, safe_wal_size) render absolute; the eight logical-decoding cumulative counters
(spill/stream/total) render as per-interval deltas. The screen reuses the established
version-aware-selector + view-registration pattern shipped in feature 004 (bgwriter) and the
multi-row diff/sort/filter machinery used by the `replication`/`tables` screens. It is
`NotRecordable` (TUI-only in 0.11.0). The chosen column subset of both views is stable across
PG 14–18, so the query needs no per-version branching.

## Architecture

### What we're building/modifying

- **`internal/query/replication_slots.go`** (new) — the hybrid query string (with WAL-function
  template placeholders + `coalesce(...,0)` on the eight diffed counters + KiB conversions) and
  the selector `SelectStatReplicationSlotsQuery(_ int) (string, int, [2]int)`.
- **`internal/query/replication_slots_test.go`** (new) — unit test on the selector
  (Ncols/DiffIntvl) and integration tests (execute-only + physical slot + logical slot).
- **`internal/view/view.go`** — register the `replslots` view in `New()` and add a `case` in
  `Configure()` that calls the selector.
- **`top/keybindings.go`** — bind hotkey `o` to `switchViewTo(app, "replslots")`.
- **`top/help.go`** — add `o` to the help mode line.
- **`record/record_test.go`** — bump the `Test_filterViews` `wantN` assertion (+1 per row); the
  view is `NotRecordable`, so the existing `filterViews()` drops it with no production change.
- **`testing/prepare-test-environment.sh`, `testing/Dockerfile`** — enable `wal_level=logical`
  and bump the image label to `0.0.10` (needed for the logical-slot integration test).
- **`.github/workflows/default.yml`, `.github/workflows/release.yml`** — bump the test-container
  tag `0.0.9 → 0.0.10`.

### How it works

1. User presses `o` → `keybindings.go` calls `switchViewTo(app, "replslots")` →
   `viewSwitchHandler` puts the `replslots` view on `viewCh` (direct view, no NextView family).
2. At connect time, `view.Configure(opts)` calls `SelectStatReplicationSlotsQuery(opts.Version)`,
   assigns `QueryTmpl`/`Ncols`/`DiffIntvl`, then `query.Format` substitutes the
   `{{.WalFunction1}}`/`{{.WalFunction2}}` placeholders with the recovery-correct LSN functions
   (primary → `pg_current_wal_lsn`, standby → `pg_last_wal_receive_lsn`) chosen by
   `selectWalFunctions(version, recovery)`.
3. Each tick: `stat` collects the multi-row result, `diff()` matches rows by `UniqueKey`
   (`slot_name`, col 0), subtracts the contiguous diffed block `[6,13]`, copies the rest as-is;
   `sort()` orders by `OrderKey` (retained, col 4) descending.
4. `view` renders header + one row per slot; arrows re-sort, `/` filters.

## Decisions

### Decision 1: Hybrid query, single version-independent string
**Decision:** One query string for all PG 14–18:

```
SELECT s.slot_name AS slot_name,
       s.slot_type AS slot_type,
       s.active::text AS active,
       s.wal_status AS wal_status,
       ({{.WalFunction1}}({{.WalFunction2}}(), s.restart_lsn) / 1024)::bigint AS "retained,KiB",
       (s.safe_wal_size / 1024)::bigint AS "safe,KiB",
       coalesce(ss.spill_txns, 0)  AS spill_txns,
       coalesce(ss.spill_count, 0) AS spill_count,
       (coalesce(ss.spill_bytes, 0) / 1024)::bigint  AS "spill,KiB",
       coalesce(ss.stream_txns, 0)  AS stream_txns,
       coalesce(ss.stream_count, 0) AS stream_count,
       (coalesce(ss.stream_bytes, 0) / 1024)::bigint AS "stream,KiB",
       coalesce(ss.total_txns, 0)   AS total_txns,
       (coalesce(ss.total_bytes, 0) / 1024)::bigint  AS "total,KiB",
       date_trunc('seconds', now() - ss.stats_reset)::text AS stats_age
FROM pg_replication_slots s
LEFT JOIN pg_stat_replication_slots ss ON s.slot_name = ss.slot_name
ORDER BY "retained,KiB" DESC NULLS LAST
```

Column indices (0-based): 0 slot_name, 1 slot_type, 2 active, 3 wal_status, 4 retained,KiB,
5 safe,KiB, 6–13 the eight diffed counters, 14 stats_age. → `Ncols=15`, `DiffIntvl=[2]int{6,13}`,
`OrderKey=4`, `UniqueKey=0`.
**Rationale:** The chosen `pg_replication_slots` subset (slot_name, slot_type, active, wal_status,
restart_lsn, safe_wal_size) and the whole of `pg_stat_replication_slots` are schema-stable on
PG 14–18, so no per-version branching is needed. retained WAL via the existing WAL-function
template is recovery-aware for free.
**Alternatives considered:** Pure `pg_stat_replication_slots` (no retained WAL, no physical
slots) — rejected in user-spec. Adding `conflicting` (PG16) / `invalidation_reason` (PG18) —
rejected: would force per-version query strings (ADR-004 situation) for a niche cause-attribution
signal already covered by `wal_status`.

### Decision 2: coalesce the eight diffed counters to 0
**Decision:** Wrap the eight logical-decoding counters in `coalesce(..., 0)` in SQL.
**Rationale:** Physical slots are absent from `pg_stat_replication_slots`, so the LEFT JOIN
yields NULL for these columns. A physical slot matches itself across samples (same `slot_name`)
and enters the diff branch; with empty-string NULLs, `diffPair("","")` →
`strconv.ParseInt("")` returns an error and aborts the sample
(verified in `internal/stat/postgres.go:303-358`; `diffPair` :444). Coalescing to 0 makes
physical rows diff cleanly to `0`. retained/safe/stats_age stay nullable (absolute columns,
outside `DiffIntvl`) — NULL renders empty with no crash (physical slots show empty
`safe,KiB`/`stats_age`).
**Alternatives considered:** Render `-`/empty for physical slots — rejected: needs per-cell
view-specific rendering pgcenter does not have (scope creep). The adjacent `slot_type=physical`
column disambiguates the `0`.

### Decision 3: Default sort by retained,KiB descending
**Decision:** `OrderKey=4` (retained), `OrderDesc=true`; SQL `ORDER BY "retained,KiB" DESC NULLS LAST`.
**Rationale:** The feature exists for disk-fill triage — the greediest slot must be on top. This
deviates from the col-0 default of every other multi-row view; documented intentionally so it is
not read as a bug. The Go-side `sort()` governs the displayed order each tick; the SQL ORDER BY
sets a sensible first-frame order (matches the `replication` convention of an explicit ORDER BY).
**Alternatives considered:** `OrderKey=0` (slot_name) like every other multi-row view — rejected:
buries the disk-fill offender, forcing the DBA to re-sort on every open.

### Decision 4: Selector keeps an (unused) version parameter
**Decision:** `func SelectStatReplicationSlotsQuery(_ int) (string, int, [2]int)` — returns the
single query, `15`, `[2]int{6,13}` regardless of version. The parameter is named `_` to satisfy
revive's unused-parameter rule.
**Rationale:** Signature symmetry with `SelectStatBgwriterQuery`/`SelectStatWALQuery` and a ready
extension point if `conflicting`/`invalidation_reason` are added later (they would reintroduce
version branching). The unit test still iterates PG 14–18 and asserts identical output, pinning
the "no divergence" invariant.

### Decision 5: Logical-slot test is defensive (skips unless wal_level=logical)
**Decision:** The tier-3 logical-slot integration test runs `SHOW wal_level` and `t.Skipf`s if it
is not `logical`. Slot creation/cleanup is wrapped so a missing `test_decoding` plugin also skips.
Both tiers create slots idempotently (drop-if-exists before create) and drop them in `defer`, so a
SIGKILL'd prior run cannot block a re-run on a duplicate slot name.
**Rationale:** Decouples the manual test-image push from the code merge — CI stays green on the
old image (`wal_level=replica`), and the test starts exercising logical slots once
`pgcenter-testing:0.0.10` is live and the workflow tag is bumped. No fragile ordering dependency.
**Alternatives considered:** A hard ordering (push image, then merge code that assumes
`wal_level=logical`) — rejected: any failure between the two leaves CI red on a transient
infrastructure state.

### Decision 6: TUI-only (NotRecordable), no record/report
**Decision:** Register with `NotRecordable: true`; only `Test_filterViews` needs updating.
**Rationale:** Roadmap TUI-first for all 0.11.0 views (ADR-004). Record/report for the new
views is the planned next phase. Documented limitation in user-spec (retrospective analysis
needs it most for this feature).
**Alternatives considered:** Ship record/report in this feature — rejected: doubles scope
(storage format + report pipeline), contradicts the roadmap's TUI-first sequencing.

## Data Models

No Go structs or DB schema changes. The feature reads two existing PostgreSQL views and produces
a 15-column `PGresult` consumed by the existing `stat`/`view` pipeline (`sql.NullString` scanning
in `postgres.go` already handles the LEFT JOIN NULLs).

Selector contract: `SelectStatReplicationSlotsQuery(_ int) (string, int, [2]int)` →
`(PgStatReplicationSlots, 15, [2]int{6,13})`.

View registration (`internal/view/view.go` `New()`), mirroring the `bgwriter` entry:
`MinRequiredVersion: query.PostgresV14`, `QueryTmpl: query.PgStatReplicationSlots`,
`DiffIntvl: [2]int{6,13}`, `Ncols: 15`, `OrderKey: 4`, `OrderDesc: true`, `UniqueKey: 0`
(default), `ColsWidth: map[int]int{}`, `Filters: map[int]*regexp.Regexp{}`,
`NotRecordable: true`, `Msg: "Show replication slots statistics"`.

## Dependencies

### New packages
- None.

### Using existing (from project)
- `internal/query` — `Options.WalFunction1/2`, `selectWalFunctions`, `Format`, `NewOptions`,
  `PostgresV14`; selector pattern from `bgwriter.go`/`wal.go`.
- `internal/stat` — multi-row `diff()` (UniqueKey row matching) and `sort()`.
- `internal/view` — `New()`/`Configure()`; multi-row sort/filter (OrderKey/Filters) from
  `replication`/`tables`.
- `internal/postgres` — `NewTestConnectVersion` for integration tests.
- `record` — generic `filterViews()` already drops `NotRecordable` views.

## Testing Strategy

**Feature size:** M

### Unit tests
- `Test_SelectStatReplicationSlotsQuery`: assert `(_, Ncols, DiffIntvl)` == `(15, [6,13])` for
  PG 14, 15, 16, 17, 18 (mirrors `Test_SelectStatBgwriterQuery`). Pins the no-version-divergence
  invariant.

### Integration tests
- **Tier 1 (execute / schema gate):** for PG 14–18, `Format` the query, run `conn.Query`, assert
  `len(FieldDescriptions()) == 15`. Runs on the current image (empty slot set tolerated).
- **Tier 2 (physical slot):** create `pg_create_physical_replication_slot('pgcenter_test_phys', true)`,
  assert the row exists, `retained,KiB` is non-NULL, the eight counters render `0`; drop the slot
  in `defer`. Runs on the current image (`wal_level=replica`).
- **Tier 3 (logical slot):** `t.Skipf` unless `wal_level=logical`; create
  `pg_create_logical_replication_slot('pgcenter_test_logical', 'test_decoding')`, assert the row
  exists and the spill/stream columns are present; drop in `defer`. Requires image `0.0.10`.

### E2E tests
- None — no user flow beyond opening the screen; covered by unit + integration.

## Agent Verification Plan

**Source:** user-spec "Как проверить".

### Verification approach
Automated `make test` (unit + tier-1/2 integration on whatever PG versions are running; tier-3
runs when the `0.0.10` image is live), `make build`, `make lint`, `make vuln`. The live
`len(FieldDescriptions()) == Ncols` assertion is the schema-divergence gate across PG 14–18.

### Per-task verification
| Task | verify: | What to check |
|------|---------|--------------|
| 1 | bash | `make test` — selector unit test + tier-1 integration green on PG 14–18 |
| 2 | bash + user | `make build`; user presses `o`, sees the screen, `o` in help (`?`) |
| 3 | bash | `make test` — `Test_filterViews` green with bumped counts |
| 4 | bash | `make build`; workflow/image files parse; (maintainer) image `0.0.10` pushed |
| 5 | bash | `make test` — tier-2 physical green; tier-3 logical green on `0.0.10` (else skipped) |

### Tools required
bash only (make). No MCP/Playwright. No deploy.

## Backward Compatibility

N/A — adding new code only. New view, new query file, new hotkey `o` (was free), test-only edit
to `record_test.go`, additive test-infra/workflow changes.

**Breaking changes:** no.
**Migration strategy:** none needed (additive). Test-image bump `0.0.9 → 0.0.10` is CI-only and
must be pushed by the maintainer before the workflow tag bump takes effect; the defensive
`t.Skipf` (Decision 5) removes any hard ordering requirement.
**DB migration compatibility:** N/A — read-only SELECT.
**Consumer impact:** none found — no existing view, query, or exported function is modified.

## Risks

| Risk | Mitigation |
|------|-----------|
| Physical-slot NULL counters break diff (`diffPair("")` → ParseInt error) | `coalesce(...,0)` on the eight diffed counters (Decision 2); verified in code |
| `Test_filterViews` CI assertion fails if not bumped (same miss as bgwriter) | Task 3 bumps `wantN` +1 on all six rows in the same change |
| SQL drafted from PG docs + code research, not executed against live PG (containers down at planning time) | Tier-1/2/3 integration tests are the gate; `dev-skeptic` validates column/function names |
| Test-image push (`0.0.10`) and workflow tag bump must be coordinated | Defensive `t.Skipf` decouples them (Decision 5); maintainer pushes the image |
| 15 columns exceed narrow terminals | Incident-critical columns ordered left so edge-clipping drops least-important first (documented in user-spec) |
| Tier-2 physical slot retains WAL on the shared fixture | Drop the slot in `defer` teardown |

## Acceptance Criteria

Технические критерии (дополняют пользовательские из user-spec):

- [ ] `SelectStatReplicationSlotsQuery` returns `(query, 15, [2]int{6,13})` for PG 14–18; unit
      test green.
- [ ] Live query returns exactly 15 columns on PG 14–18 (`FieldDescriptions` gate).
- [ ] Physical-slot integration test: row present, `retained,KiB` non-NULL, counters `0`.
- [ ] Logical-slot integration test: row present, spill/stream columns present (or skipped when
      `wal_level != logical`).
- [ ] `o` opens `replslots`; view registered `NotRecordable: true`; `Test_filterViews` green.
- [ ] `o` present in help (`?`).
- [ ] No regressions: `make test`, `make lint`, `make vuln`, `make build` all clean.

## Implementation Tasks

### Wave 1 (независимые)

#### Task 1: Hybrid query + selector + unit/tier-1 tests
- **Description:** Add `internal/query/replication_slots.go` with the hybrid
  `pg_replication_slots LEFT JOIN pg_stat_replication_slots` query (WAL-function placeholders,
  `coalesce(...,0)` on the eight counters, KiB conversions) and
  `SelectStatReplicationSlotsQuery(_ int) (string, int, [2]int)`. Add `replication_slots_test.go`
  with the selector unit test and the tier-1 execute/`FieldDescriptions==15` integration test for
  PG 14–18. This is the data core, testable in isolation.
- **Skill:** code-writing
- **Reviewers:** dev-code-reviewer, dev-test-reviewer
- **Verify:** bash — `make test` (selector + tier-1 green on available PG versions)
- **Files to modify:** `internal/query/replication_slots.go` (new), `internal/query/replication_slots_test.go` (new)
- **Files to read:** `internal/query/bgwriter.go`, `internal/query/bgwriter_test.go`, `internal/query/replication.go`, `internal/query/query.go`

#### Task 4: Test-image bump for logical-slot support
- **Description:** Enable `wal_level=logical` in `testing/prepare-test-environment.sh` (with an
  inline comment marking it test-only so it does not leak into user config examples), bump BOTH
  `0.0.9` version literals in `testing/Dockerfile` (the `LABEL version` and the `CMD` echo string)
  to `0.0.10`, fix the stale `PostgreSQL 14-17` header comment to `14-18`, and update the test
  container tag `0.0.9 → 0.0.10` in `default.yml` and `release.yml`. The actual
  `docker build && docker push lesovsky/pgcenter-testing:0.0.10` is a manual maintainer step
  (DockerHub credentials) — document it in the task; CI has no image-build job.
- **Skill:** deploy-pipeline
- **Reviewers:** dev-deploy-reviewer, dev-code-reviewer
- **Verify:** bash — files parse / `make build`; maintainer confirms image `0.0.10` pushed
- **Files to modify:** `testing/prepare-test-environment.sh`, `testing/Dockerfile`, `.github/workflows/default.yml`, `.github/workflows/release.yml`
- **Files to read:** `.claude/skills/project-knowledge/deployment.md`

### Wave 2 (зависит от Wave 1)

#### Task 2: Wire the replslots view into the TUI
- **Description:** Register the `replslots` view in `internal/view/view.go` `New()` (OrderKey=4,
  DiffIntvl=[6,13], Ncols=15, NotRecordable, modeled on the `bgwriter` entry) and add the
  `Configure()` case calling `SelectStatReplicationSlotsQuery`. Bind hotkey `o` in
  `top/keybindings.go` and add `o` to the `top/help.go` mode line. Depends on the selector
  (Task 1).
- **Skill:** code-writing
- **Reviewers:** dev-code-reviewer
- **Verify:** bash + user — `make build`; user presses `o`, screen renders, `o` shown in help
- **Files to modify:** `internal/view/view.go`, `top/keybindings.go`, `top/help.go`
- **Files to read:** `internal/view/view.go` (bgwriter/replication entries, Configure), `top/config_view.go`

#### Task 5: Physical + logical slot integration tests
- **Description:** Extend `replication_slots_test.go` with tier-2 (create a physical slot, assert
  row/retained/counters, drop in `defer`) and tier-3 (skip unless `wal_level=logical`, create a
  `test_decoding` logical slot, assert spill/stream columns, drop) integration tests across the
  live PG 14–18 matrix. Create slots idempotently (drop-if-exists before create) so an interrupted
  run cannot block re-runs. Depends on the query (Task 1) and the image bump (Task 4).
- **Skill:** code-writing
- **Reviewers:** dev-test-reviewer, dev-code-reviewer
- **Verify:** bash — `make test` (tier-2 green; tier-3 green on `0.0.10`, else skipped)
- **Files to modify:** `internal/query/replication_slots_test.go`
- **Files to read:** `internal/query/replication_test.go`, `internal/query/bgwriter_test.go`, `internal/postgres/testing.go`

### Wave 3 (зависит от Wave 2)

#### Task 3: Bump Test_filterViews for the new NotRecordable view
- **Description:** Update `record/record_test.go` `Test_filterViews` — increment `wantN` by 1 on
  every test row (the `replslots` view is `NotRecordable`, dropped on every version before the
  version branch), and refresh the adjacent `wantN`-logic comment block to document the replslots
  contribution alongside bgwriter. No production change to `record.go`. Depends on the view being
  registered (Task 2).
- **Skill:** code-writing
- **Reviewers:** dev-test-reviewer
- **Verify:** bash — `make test` (`Test_filterViews` green)
- **Files to modify:** `record/record_test.go`
- **Files to read:** `record/record.go`, `internal/view/view.go`

### Final Wave

#### Task 6: Pre-deploy QA
- **Description:** Acceptance testing: run `make test`/`make lint`/`make vuln`/`make build` and
  verify the acceptance criteria from user-spec and tech-spec (hotkey, 15 columns PG 14–18,
  physical-shows-0, retained sort, NotRecordable, help entry, both integration tiers).
- **Skill:** pre-deploy-qa
- **Reviewers:** none
