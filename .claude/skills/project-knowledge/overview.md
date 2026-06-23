# pgcenter — Project Overview

pgcenter is a command-line admin tool for observing and troubleshooting PostgreSQL in real time.
It reads PostgreSQL internal statistics views and presents them in a top-like interactive TUI.

## Commands

| Command   | Purpose |
|-----------|---------|
| `top`     | Real-time monitoring (main feature) — live stats with refresh; the main stats table scrolls horizontally by column (`[`/`]`) with a frozen first column for narrow terminals (009-feat-horizontal-scroll) |
| `record`  | Collect stats to tar files ("poor man's monitoring") |
| `report`  | Build reports from recorded files |
| `profile` | Wait events profiler — shows what queries are waiting on |

## Supported PostgreSQL Statistics

- `pg_stat_activity` — active connections and their state
- `pg_stat_database` — per-database metrics (commits, rollbacks, tuples, deadlocks, temp files)
- `pg_stat_replication` — connected standbys and replication lag
- `pg_stat_user_tables`, `pg_stat_user_indexes` — table/index access stats
- `pg_stat_bgwriter` (+ `pg_stat_checkpointer` on PG 17+) — background writer / checkpointer screen (hotkey `b`; PG 14–18; recordable via `record`/`report -B`)
- `pg_replication_slots` (+ `pg_stat_replication_slots`) — replication slots screen (hotkey `o`; PG 14–18; multi-row, all slots; retained WAL + wal_status + spill/stream; recordable via `record`/`report -L`)
- `pg_stat_io` — unified IO breakdown by backend_type × object × context (hotkey `j` toggles count↔time sub-screens, `J` opens the mode menu; PG 16+; multi-row; this is where `buffers_backend`/`buffers_backend_fsync` went on PG 17+ and WAL IO timings on PG 18; recordable via `record`/`report -J c|t`)
- `pg_stat_wal` — WAL generation stats (PG 14+; reduced schema in PG 18 — WAL IO timings moved to `pg_stat_io`)
- `pg_stat_statements` — top queries by various metrics (requires extension); 7 sub-screens under the `X` menu / `x` cycle: timings, general, IO, temp files, local (temp tables), WAL, and **JIT** (compilation cost per query — generation/inlining/optimization/emission phase times + functions, `+deform` on PG 17+; PG 15+; rows filtered to `jit_functions > 0`; recordable via `record`/`report -X j`)
- System stats — CPU, memory, disk, network (read from /proc or via PL/Perl schema)

## Target Audience

PostgreSQL DBAs who need to monitor and troubleshoot Postgres in production without GUI tools.

## PostgreSQL Version Support

Active support: PG 14, 15, 16, 17, 18.
EOL versions (9.5–13) are no longer tested but code paths remain for reference.

## Current Status (May 2026)

Active development resumed with v0.10.0 after 5 years.
Priorities: stability, PG version compatibility, community contributions.
