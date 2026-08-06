# pgcenter — Project Overview

pgcenter is a command-line admin tool for observing and troubleshooting PostgreSQL in real time.
It reads PostgreSQL internal statistics views and presents them in a top-like interactive TUI.

## Commands

| Command   | Purpose |
|-----------|---------|
| `top`     | Real-time monitoring (main feature) — live stats with refresh; the main stats table scrolls horizontally by column (`[`/`]`) with a frozen first column for narrow terminals (009-feat-horizontal-scroll); hotkey `v` expands the top `sysstat`/`pgstat` summary panels into a verbose instance-health overview (+3/+5 rows), persistent across screens (010-feat-overview-dashboard) |
| `record`  | Collect stats to tar files ("poor man's monitoring") |
| `report`  | Build reports from recorded files. **Breaking change in 0.12.0:** `-W` is no longer a boolean — it takes `w` (pg_stat_wal) or `a` (pg_stat_archiver), like `-J c\|t`. A legacy `report -W -f dump.tar` fails with `report type is not specified, quit` because pflag eats `-f` as the flag's value; documented in `doc/release-notes/v0.12.0.md` |
| `profile` | Wait events profiler — shows what queries are waiting on |

## Supported PostgreSQL Statistics

- `pg_stat_activity` — active connections and their state; on PG 13+ also `leader` (which parallel group a backend belongs to), `backend_xid` (has the transaction written) and `horizon_xacts` (how far back it holds the xmin horizon), so "who is blocking vacuum and is killing them cheap" is answerable without leaving pgcenter
- `pg_stat_database` — per-database metrics (commits, rollbacks, tuples, deadlocks, temp files)
- `pg_stat_replication` — connected standbys and replication lag
- `pg_stat_user_tables`, `pg_stat_user_indexes` — table/index access stats
- `pg_stat_bgwriter` (+ `pg_stat_checkpointer` on PG 17+) — background writer / checkpointer screen (hotkey `b`; PG 14–19; recordable via `record`/`report -B`)
- `pg_replication_slots` (+ `pg_stat_replication_slots`) — replication slots screen (hotkey `o`; PG 14–19; multi-row, all slots; retained WAL + wal_status + spill/stream; recordable via `record`/`report -L`)
- `pg_stat_io` — unified IO breakdown by backend_type × object × context (hotkey `j` toggles count↔time sub-screens, `J` opens the mode menu; PG 16+; multi-row; this is where `buffers_backend`/`buffers_backend_fsync` went on PG 17+ and WAL IO timings on PG 18; recordable via `record`/`report -J c|t`)
- `pg_stat_wal` — WAL generation stats (hotkey `w`; PG 14+; reduced schema in PG 18 — WAL IO timings moved to `pg_stat_io`; on PG 19 a `fpi,KiB` column from `wal_fpi_bytes` sits next to the `fpi` counter, so how much WAL full-page images actually cost is readable beside how many there were; recordable via `record`/`report -W w`)
- `pg_stat_archiver` — archiver screen (hotkey `w` cycles `wal` ↔ `archiver`, `W` opens the two-item menu; PG 14+; single row: the `.ready` backlog count plus the archiver's own success/failure counters, last WAL name and ages on each side; recordable via `record`/`report -W a`). Answers "when did archiving stop, on which segment, and how much has piled up"; the first signal that it stopped comes earlier, from the verbose panel's backlog in bytes
- `pg_stat_statements` — top queries by various metrics (requires extension); 7 sub-screens under the `X` menu / `x` cycle: timings, general, IO, temp files, local (temp tables), WAL, and **JIT** (compilation cost per query — generation/inlining/optimization/emission phase times + functions, `+deform` on PG 17+; PG 15+; rows filtered to `jit_functions > 0`; recordable via `record`/`report -X j`)
- `pg_stat_progress_*` — progress of vacuum, analyze, cluster, create index, basebackup and copy (hotkey `p` cycles, `P` opens the menu); on PG 19 the vacuum screen also shows `started_by` and `mode`, analyze shows `started_by`, and basebackup shows `backup_type`
- System stats — CPU, memory, disk, network (read from /proc or via PL/Perl schema)

## Target Audience

PostgreSQL DBAs who need to monitor and troubleshoot Postgres in production without GUI tools.

## PostgreSQL Version Support

Active support: PG 14, 15, 16, 17, 18, 19.
EOL versions (9.5–13) are no longer tested but code paths remain for reference.

## Current Status (May 2026)

Active development resumed with v0.10.0 after 5 years.
Priorities: stability, PG version compatibility, community contributions.
