# Release 0.12.0 — PostgreSQL 19 + Troubleshooting

**Theme:** Two halves in one release. First, keep the promise pgcenter is built on — that it tracks
current PostgreSQL statistics — by supporting **PostgreSQL 19** (beta 1 released 2026-06-04, GA
expected September/October 2026). Second, close the gaps a DBA hits during an incident rather than
during a metrics review: who blocks whom, who holds the xmin horizon, what is on fire on this screen
right now, plus the interaction papercuts that make the TUI slower to drive than it should be.

**Audience lens:** practicing PostgreSQL DBA in the middle of an incident. Prioritize answers that
today require leaving pgcenter for `psql`.

## Already merged, ships with this release

Unreleased on `master`/`develop` since v0.11.0 (2026-06-22):

- **[009-feat-horizontal-scroll]** by-column horizontal scroll of the stats table (`[`/`]`, frozen
  first column) — closes issue #14, open since 2015
- **[010-feat-overview-dashboard]** verbose mode for the summary panels (`v`)
- **[011-refactor-tech-debt-paydown]** bounded report replay allocation + stable verbose panel widths

An early release of just these three was considered and **rejected**: it would only be insurance
against running out of time for further feature work, and that time is available. There are no
external release deadlines on this project.

## Organising principle: enter each code area exactly once

This release is ordered to **minimise rework**, not to be interruptible. Grouping features by
*theme* ("PG 19" vs "troubleshooting") was tried first and rejected — it split single code areas
across two features and produced six places where the same view, hotkey group or test fixture would
be touched twice. Features here are therefore grouped by the **code area they touch**, and the
concrete double-touches that grouping removes are recorded per feature under *Why these are
together*.

Two ordering rules fall out of the same principle:

1. **Column-set changes precede colorization.** Colorization rules address rows of a specific
   column layout; writing them before the layout is final means rewriting them. So the activity
   column work [013] comes before color [014].
2. **Colorization precedes every new screen.** If color came last, each new screen would need a
   return visit to add its rules — N extra passes. Placed early, each new screen defines its own
   rules inside its own feature — zero extra passes.

Each feature goes through the full SDD pipeline (user-spec → tech-spec → decompose → implement →
review) **one at a time**, in the order below. Numbering continues from the archive (last
archived: `011`).

## Cross-cutting policy: record/report per view, by design risk

0.11.0 shipped every new view TUI-only (`NotRecordable: true`) and retrofitted record/report later in
one sweep, [008]. Measured cost of that sweep, from `docs/metrics/`: **11 tasks, size M, 4 waves, ~40
agents, 18 commits for four views** — roughly 2.75 tasks per view, about what folding it in would
have added, *plus* an entire extra SDD cycle (user-spec, tech-spec, five validators, task validation,
finalization, archive, metrics). That extra cycle is pure duplication.

But a blanket "fold it in everywhere" is wrong too: **the recorded format is a compatibility
surface.** Freezing it before a view's column design has settled through real use is what produced
issue #122. TUI-first was never about saving effort — it was about not freezing a format too early.

**Policy for 0.12.0 — decide per view by design risk, not by rule:**

- **Fold record/report in** where design risk is zero: **`archiver`** (7 columns, stable since
  PG 9.0 — nothing to settle).
- **Keep `NotRecordable: true`** where the column set will only settle through use:
  `autovacuum_scores`, `recovery`, `contention`, `subscription_stats`.
- The retrofit for those four is an **explicitly planned 0.13.0 item** (see Out of scope), not a
  surprise.

## Scope & order

### [012] PostgreSQL 19 compatibility baseline

- **Status:** planned — **do this first**. Probe passed 2026-07-25 on the existing `ubuntu:22.04` base:
  PG 19 beta2 installs, the cluster starts on 21919, `plperlu` loads, and the three new columns are present
  in the live catalog. No base-image migration needed. The one non-obvious detail: the beta suite must be
  declared with the major-version **component** (`jammy-pgdg-testing 19`) — without it the packages are
  invisible — and installed with an explicit target release. See the feature's decisions log.
- **Value:** the release's central promise. Honest framing: **pgcenter does not break on PG 19.**
  Verified against the code — version selectors are written as
  `if version >= PostgresV18 { newest } else { older }` (`internal/query/wal.go:26`,
  `bgwriter.go:42`, `io.go:88`), so 190000 lands in the newest branch automatically; every query
  lists columns explicitly, so columns added in PG 19 break nothing; and the renamed wait event type
  (`BUFFERPIN` → `BUFFER`) is not hardcoded anywhere — pgcenter shows whatever the server reports. So
  this is **feature catch-up plus a tested guarantee**, not an emergency fix.
- **Scope, in order:**
  1. **CI matrix + test fixture for PG 19** — add `PostgresV19 = 190000` to
     `internal/query/query.go`, a PG 19 cluster to `internal/postgres/testing.go` (the port map ends
     at `180000: 21918`), and the version to the GitHub Actions matrix.
  2. **Verification pass over every existing screen** on PG 19 — this is what turns "should work"
     into "tested", and it is the deliverable users care about.
  3. **Only the additive columns whose areas are not re-entered later:**
     - `pg_stat_progress_vacuum`: `started_by` (autovacuum vs manual) + `mode` (aggressiveness);
       `pg_stat_progress_analyze`: `started_by`; `pg_stat_progress_basebackup`: `backup_type`
       (`full`/`incremental`). **Best value per line of code in the release** — the progress screens
       already exist, and "is this vacuum mine or autovacuum's, and is it aggressive?" is exactly
       what you ask while staring at a long vacuum.
     - `pg_stat_wal_receiver`: new `connecting` status value (display-only, no code change).
- **Deliberately NOT here** (this is the anti-rework boundary): the PG 19 WAL full-page-image column
  goes to [016] with the rest of the WAL area; the `pg_stat_replication_slots` columns go to [018]
  with the rest of the replication area; `stats_reset`/`stats_age` on tables/indexes/functions goes
  to [017] with the tables area. Adding them here would mean entering all three areas twice.
- **Risk to retire on day one:** PG 19 beta packages live in a **separate PGDG repository**
  (`pgdg-testing`), not the main one. If beta packages cannot be installed in CI, the whole PG 19
  block needs replanning — probe this **before writing any query**, not on the third feature.
- **Timing:** implemented against beta; a re-verification pass at RC/GA is needed but cheap (the CI
  matrix reports it) and does not block the release. Catalog details can still move in beta 2/3/RC;
  exposure is small given explicit column lists.
- **Open question for spec:** whether the additive columns need `PostgresV19` query branches per view
  or fit the existing newest-branch queries — decide per view, do not generalize.

### [013] Activity screen: xmin horizon + parallel worker grouping

- **Status:** planned
- **Value:** high, daily. Closes issue #148. Today the activity screen cannot answer "is this
  idle-in-transaction session holding the xmin horizon and blocking vacuum?" — that needs `psql`.
  Second scope item: on PG 14+ parallel workers clutter the screen with no way to attribute them to
  their leader.
- **Shape:** columns added to the existing `activity` view (no new screen):
  - **Horizon:** `backend_xid`, `backend_xmin`, and the derived **`xmin_age` in transactions**
    (`age(backend_xmin)`).
  - **Parallelism:** `leader_pid`, so parallel workers can be sorted/filtered next to their leader.
- **Why these are together:** both change the `activity` column set. Every such change ripples
  through the PG 14–19 version tests and the `report -A` layout; one feature pays that ripple once.
- **Decided — do not re-open in the spec:** `xmin_age` is **transactions only**, no wall-clock value.
  There is no honest conversion from xid distance to time (10 000 transactions is 3 seconds on a busy
  OLTP box and three days on a quiet one). The time dimension is read from the **`xact_age` column
  already on the screen**; side by side they give the complete answer ("holding the horizon 2.3M
  transactions back, transaction open 40 minutes") with nothing invented.
  `pg_xact_commit_timestamp(backend_xmin)` was considered and rejected — it needs
  `track_commit_timestamp=on`, which is almost never enabled.
- **Why now:** [009] shipped horizontal scroll, so widening the activity column set is no longer
  gated on terminal width — the previous hard constraint on this screen.
- **Product decisions for the spec:**
  - Expose `leader_pid` as a column **only**; no hide-parallel-workers toggle (it would duplicate the
    existing `I`/`A` filter machinery for a narrower case).
  - `backend_xid`/`backend_xmin` are NULL for most rows (no xid assigned, non-`client backend`
    types) — render blank, never `0`.
- **Plumbing:** `internal/query/activity.go` — column count on `PgStatActivityDefault` (14 → 14+N)
  and its version selector.
- **Open question for spec:** does `report -A` replay of pre-0.12 archives stay clean when the live
  column count grows (the [008] version-metadata path suggests yes — verify, do not assume).

### [014] Row colorization

- **Status:** planned
- **Value:** medium, but high *perceived* — this is what makes competing TUIs feel faster to read.
  Anomalies are currently invisible until you read every row.
- **Shape:** attributes on **data** rows. The mechanism already half-exists — the header row emits
  ANSI today (reverse video on the sorted column, bold on the frozen column, in
  `top/stat.go:printHeaderCell`) — so this extends it downward rather than introducing it. Initial
  rule set, activity screen: `idle in transaction` → yellow; waiting on a lock → red; query older
  than the age threshold → bold; auxiliary/autovacuum backends → dim.
- **Position is load-bearing (see the organising principle):** **after** [013], so the rules are
  written once against the final activity column layout; **before** every new screen, so each new
  screen defines its own colorization rules inside its own feature instead of earning a return visit.
- **Product decisions (locked):**
  - **Opt-out is mandatory:** honor the `NO_COLOR` environment variable and add a `--no-color` flag.
    Monochrome stays a first-class mode — some operators prefer it, and light-background terminals
    make naive color choices unreadable.
  - Rules must key off **column names, not indices**, so a future column-set change cannot silently
    mis-color a row.
  - Precedence against the existing header attributes and the [009] frozen-column bold must be
    specified explicitly, not discovered at render time.
  - Colorization is **per-row semantic**, not per-value thresholds on every numeric column — the
    latter is a rabbit hole and a configuration surface nobody asked for.
- **Plumbing:** `top/stat.go` render path (`printStatData`), freshly refactored by [009] — good
  timing. No query changes.

### [015] TUI papercuts batch

- **Status:** planned
- **Value:** medium in aggregate, each item small. These are the frictions that make the TUI slower
  to drive. Batched as one feature because each is a handful of lines in `top/`, and separate SDD
  cycles would cost more than the code.
- **Why not merged with [014]:** both touch `top/`, but they touch *different functions* — color
  lives in the cell renderers, these live in keybindings, the column-window logic and the cmdline.
  Merging would produce an incoherent spec without saving a pass. Sequenced adjacently instead, so
  the render-layer context is warm.
- **Scope (four items):**
  1. **Pause on `Space`.** Every top-like tool has it; pgcenter has no `Space` binding at all. Must
     show `PAUSED` in the cmdline so a frozen screen is never mistaken for a quiet cluster.
     **Decided — do not re-litigate:** pause gates the **render only**; the collector keeps running,
     untouched. Rationale: the collector's prev/curr snapshots keep stepping every tick, so deltas
     stay honest per-interval and the first frame after resume is a normal fresh frame. Stopping the
     collector would freeze `prev` at pause start, and the first `Update` after resume would show the
     delta **for the whole pause window** in columns labelled per-interval (10 minutes paused at
     `refresh=1s` → a number ~600× normal). That is an artifact, not a signal.
     **Implementation detail that must be in the spec:** `statCh` is **unbuffered**
     (`top/ui.go:73`), so merely skipping the render blocks the collector on send and renders a
     *stale* frame (collected at pause start) on resume. Pause must **drain and discard** frames, not
     just skip rendering.
  2. **Auto-scroll to the sorted column** — a limitation recorded by [009]: sorting by a column
     outside the visible window produces no visible effect. Bring it into view on sort change.
  3. **Persistent active-filter indicator in the cmdline** — today the only cue is a `*` on the
     column header, which after [009] can itself be scrolled off-screen. Result: reading filtered
     data believing it is the whole picture. The one genuine footgun in the batch.
  4. **Clock + current refresh interval in the header** — for correlating with the Postgres log, and
     so the value set by `z` is visible afterwards.
- **Plumbing:** `top/keybindings.go`, `top/layout.go`, `top/stat.go`, `top/config_view.go`. No query
  changes, no new views.

### [016] WAL and archiving area — one pass

- **Status:** planned
- **Value:** medium — low frequency, high severity (archiving failure → WAL accumulation → disk
  fill). Honest scoping: the *most* valuable archiving metric, the backlog of `.ready` files,
  **already ships** in the [010] verbose panel (`OverviewArchivingBacklog`). What
  `pg_stat_archiver` adds is the **identity and timeline of the failure**: `last_failed_wal`,
  `last_failed_time`, `failed_count` growth, `last_archived_wal`/`_time`, `stats_reset`.
- **Scope:**
  - New `archiver` view, reached by **cycling the existing `w` hotkey** (`wal → archiver`) with `W`
    opening the 2-item menu — the [006] `j`/`J` toggle precedent.
  - **PG 19:** WAL bytes written for full page images, added to the `wal` view (also exposed by
    `pg_stat_get_backend_wal()`).
  - **record/report folded in** for `archiver` per the cross-cutting policy — zero design risk.
- **Why these are together:** both touch the WAL area — the same `internal/query/wal.go`, the same
  `w` hotkey group, the same help-screen entry, the same menu. Split across two features (as the
  theme-based draft had it) this area would be entered twice.
- **Why `archiver` is a separate view, not extra columns on `wal`:** widening `wal` would change the
  recorded column layout for `report -W`; a separate view leaves existing archives untouched. The
  grouping under `w` gives "WAL lifecycle in one place" without the compatibility cost.
- **Plumbing:** new view `archiver`, `internal/query/archiver.go`. `pg_stat_archiver` is stable since
  PG 9.0 — no version branching for it; the FPI column needs a PG 19 branch in `wal.go`.

### [017] Tables and autovacuum area — one pass

- **Status:** planned
- **Value:** `pg_stat_autovacuum_scores` is **the most valuable new view in PG 19** for this
  audience — per-table autovacuum detail: why autovacuum is not visiting a table and what its
  priority is. DBAs currently reconstruct this by hand from `n_dead_tup`, `reltuples` and the
  threshold GUCs; the visibility simply did not exist before.
- **Scope:**
  - New `autovacuum_scores` view (PG 19+), column set settled in the spec against the **actual
    catalog**, not the release notes.
  - **PG 19:** `stats_reset` → a `stats_age` column on the `tables`, `indexes` and `functions`
    screens, reusing the established `stats_age` pattern (not diffed, excluded from `DiffIntvl`).
  - `NotRecordable: true` for `autovacuum_scores` per the cross-cutting policy.
- **Why these are together:** both touch the tables/indexes/functions screen family. If
  `autovacuum_scores` hangs off a `t`-group cycle (the option preferred below), it is literally the
  same hotkey group as the screens gaining `stats_age`.
- **Product decisions for the spec:**
  - Hotkey: prefer a **cycle off the existing `t`** (`tables → autovacuum_scores`) with `T` as the
    menu, by the [006] `j`/`J` precedent, rather than burning one of the few free letters (`e`, `g`,
    `u`, `y`).
  - `MinRequiredVersion PostgresV19`; older versions report "not supported" via the existing runtime
    version guard.
- **Plumbing:** new `internal/query/autovacuum_scores.go`; multi-row view model shared with `tables`.
  `stats_age` additions touch `tables.go`, `indexes.go`, `functions.go` and their report layouts.

### [018] Replication and recovery area — one pass

- **Status:** planned — the heaviest of the area passes (candidate for a tech-spec-time split)
- **Value:** three distinct gaps closed in one visit to the same area.
- **Scope:**
  - **PG 19 columns on the replslots screen:** `mem_exceeded_count` (how many times
    `logical_decoding_work_mem` was exceeded — a direct tuning signal complementing the existing
    spill columns), plus `slotsync_skip_count` / `slotsync_last_skip` / `slotsync_skip_reason` (also
    on `pg_replication_slots`).
  - **`pg_stat_subscription_stats`** — new view, one row per subscription, cycled off the existing
    `o` hotkey (`replslots → subscription`) with `O` as the menu: publisher and subscriber side under
    one key. Subscriber-side apply errors and conflicts are otherwise invisible in pgcenter.
    Deferred once already, from [005-feat-replication-slots]. **`MinRequiredVersion PostgresV15`.**
    **PG 19 branch from day one:** PG 19 renamed `sync_error_count` → `sync_table_error_count`
    (sequence errors are now tracked separately) and added `update_deleted` (rows where updates were
    ignored due to concurrent deletes) and `sync_seq_error_count`. Writing this without the PG 19
    branch means writing the query twice.
  - **`pg_stat_recovery`** (PG 19) — recovery status detail. Row shape (single- vs multi-row) settled
    in the spec against the actual catalog. `MinRequiredVersion PostgresV19`.
  - **Standby test fixture** for the harness, which the recovery view needs anyway.
  - `NotRecordable: true` for `subscription` and `recovery` per the cross-cutting policy.
- **Why these are together:** one area, one hotkey group (`o`/`O`), one help edit — and above all
  **one standby fixture**. The harness currently has no standby cluster, which is the sole reason
  tech-debt **[006]** (replslots `retained,KiB` standby path) and **[010]** (verbose recovery-`t`
  WAL standby path) are both open and "verified by string substitution only". Building the fixture
  here retires two debt items as a side effect; building `pg_stat_recovery` in isolation would
  build the fixture and capture neither.
- **Note on value:** `subscription_stats`' worth depends on whether **the user** runs logical
  replication — which is an argument for shipping it, not against it. pgcenter cannot know its
  reader's topology; a view that is empty for some deployments and essential for others belongs in
  the tool.
- **Plumbing:** `internal/query/replication_slots.go` (PG 19 branch), new
  `internal/query/subscription.go` and `internal/query/recovery.go`, `o`/`O` menu group,
  `internal/postgres/testing.go` (standby fixture) + CI.

### [019] Contention area: `pg_locks` + `pg_blocking_pids()` + `pg_stat_lock` — one pass

- **Status:** planned — last, biggest, highest risk
- **Value:** highest of the troubleshooting half, and pgcenter's biggest **functional** gap for
  incident work. Today the activity screen shows `wait_etype=Lock` / `wait_event=transactionid` and
  stops there — *who* holds the lock requires `psql`. A lock storm behind one long transaction is a
  weekly-frequency incident class.
- **Scope:**
  - **Live snapshot sub-screen** (`c`), one row per **waiting** session: `pid`, `wait_age`,
    `blocker_pids`, `locktype`, `mode`, `relation`, `usename`, `datname`, `query`. A snapshot view
    like `activity` — no diff/`DiffIntvl` logic, which makes it cheaper than the 0.11.0 screens
    despite being the flagship.
  - **`pg_stat_lock` aggregates sub-screen** (PG 19), cumulative per-lock-type counters, reached by
    cycling `c` with `C`… — note `C` is taken (`show config`); settle the second key in the spec.
  - `NotRecordable: true` per the cross-cutting policy.
- **Why these are together — this is the sixth double-touch the regrouping removes:** `pg_stat_lock`
  (cumulative per-lock-type counters) and the live "who blocks whom" snapshot are **complementary,
  not duplicates**, and they belong in one hotkey group. Building the snapshot screen first and
  adding `pg_stat_lock` later would mean retrofitting cycle machinery onto `c` after the fact and
  re-deciding the screen's information architecture. One design, one pass.
- **Key complexity — a performance risk, not a plumbing risk.** `pg_blocking_pids()` is documented as
  unsuitable for monitoring dashboards: it takes locks on lock-manager partitions and degrades with
  session count. The design **must**:
  - filter `pg_locks` to `granted = false` **first** (normally 0–5 rows on a healthy cluster) and
    call `pg_blocking_pids()` **only** for those pids;
  - be a genuine no-op when nothing waits — empty screen, no function call at all.
  A "call it for every backend" query is an outage amplifier on a cluster already in trouble and must
  be rejected in the spec.
- **Product decisions for the spec:**
  - **Hotkey `c`** ("contention"). The mnemonic is weak — `l`/`L`/`k`/`K`/`A` are taken. Alternatives
    if disliked: `g`, `W`. Settle before implementation; it is user-visible.
  - Root-blocker ordering: sort so the session at the root of the wait chain is identifiable. Full
    recursive chain (tree) rendering is **out of scope** — `blocker_pids` as a list is the 90%
    answer; a tree needs a recursive CTE and a display model pgcenter does not have.
  - Show all `locktype`s, not just relation-level — `transactionid`/`tuple` waits are exactly the
    interesting ones.
- **Why last:** by this point every pattern it needs is established — the PG 19 branch style [012],
  colorization rules [014], the cycle/menu machinery [016]/[017]/[018].
- **Plumbing:** new view(s) under a `locks`/`contention` group, new `internal/query/locks.go`,
  multi-row snapshot view model shared with `activity`. `pg_blocking_pids()` exists since 9.6 — no
  version branching for the base query.

## Cross-cutting principles

**Do not make an incident worse.** Any query added here must be cheap on a cluster that is *already*
in trouble — precisely when these screens get opened. Concretely: [019]'s `pg_blocking_pids()`
gating, and no new unconditional per-tick privileged calls.

**Monochrome stays first-class.** [014] adds color as an enhancement, never a requirement:
`NO_COLOR` and `--no-color` produce exactly today's output.

**New-view column sets are settled against the live catalog**, not the release notes — for every
PG 19 view in [017], [018] and [019].

**record/report is decided per view by design risk** — see the cross-cutting policy above.

## Out of scope / backlog (post-0.12.0 candidates)

- **record/report retrofit for the four `NotRecordable` views** of this release —
  `autovacuum_scores`, `recovery`, `contention`, `subscription`. An explicitly planned 0.13.0
  feature, the [008] pattern applied once with the column designs settled. Contention has the real
  forensic value.
- **`pg_stat_progress_repack`** (new in PG 19) — the new `REPACK` command replaces `VACUUM FULL` and
  `CLUSTER`, and gets its own progress view. Surfaced during the [012] user-spec work; this roadmap was
  written before the view reached the catalog. **Nothing breaks without it:** `pg_stat_progress_cluster`
  is kept in PG 19 for backwards compatibility and translates a `REPACK` into one of the two older
  commands, so pgcenter's existing `progress_cluster` screen keeps working — verified in [012]'s
  verification pass. A dedicated screen is its own feature (hotkey, menu entry, record/report policy, the
  view-count test ripple), not a column addition. Candidate for 0.13.0.
- **`pg_stat_progress_data_checksums`** (new in PG 19) — progress of online checksum enabling. Same
  origin and same reasoning as above, but a weaker fit: this is cluster maintenance rather than
  statistics monitoring, so its place in pgcenter deserves its own discussion. Candidate for 0.13.0.
- **Recursive lock-wait chain (tree) rendering** — [019] ships `blocker_pids` as a list.
- **Full "who holds the xmin horizon" aggregate** across all four sources (backends, replication
  slots, prepared transactions, standby feedback). [013] covers the backend source only — the one on
  the activity screen; the complete answer is arguably its own small screen or verbose row.
  Deliberately deferred to keep [013] from ballooning.
- **`EXPLAIN ANALYZE` AIO statistics** (new `IO` option in PG 19) — pgcenter does not run EXPLAIN;
  noted so it is not mistaken for an oversight.
- **`pg_dsm_registry_allocations`** (new in PG 19) — shared-memory internals, not an ops screen.
- **Logging facility** (`--log-file`) — tech debt [016]; still blocked on the same product decision,
  unchanged by this release.
- **Per-value threshold coloring / configurable color scheme** — explicitly rejected for [014].

## Finalization

- [ ] PG 19 re-verification pass at RC/GA (CI matrix) — does not block the release
- [ ] Update `overview.md` (PG 19 in supported versions, new screens, new activity columns, color mode)
- [ ] Update `features-catalog.md` per feature
- [ ] Update `docs/decisions-log.md`; close tech debt [006] and [010] if [018]'s standby fixture
      retires them
- [ ] Update the built-in help screen (`top/help.go`) — new hotkeys (`c`, `Space`) and the `w`/`W`,
      `t`/`T`, `o`/`O` cycles
- [ ] Version bump — git-tag-driven via ldflags; no CHANGELOG (GoReleaser generates release notes)
- [ ] Release per `deployment.md` (tag on master → push to `release`)
