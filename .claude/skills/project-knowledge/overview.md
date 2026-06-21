# pgcenter — Project Overview

pgcenter is a command-line admin tool for observing and troubleshooting PostgreSQL in real time.
It reads PostgreSQL internal statistics views and presents them in a top-like interactive TUI.

## Commands

| Command   | Purpose |
|-----------|---------|
| `top`     | Real-time monitoring (main feature) — live stats with refresh |
| `record`  | Collect stats to tar files ("poor man's monitoring") |
| `report`  | Build reports from recorded files |
| `profile` | Wait events profiler — shows what queries are waiting on |

## Supported PostgreSQL Statistics

- `pg_stat_activity` — active connections and their state
- `pg_stat_database` — per-database metrics (commits, rollbacks, tuples, deadlocks, temp files)
- `pg_stat_replication` — connected standbys and replication lag
- `pg_stat_user_tables`, `pg_stat_user_indexes` — table/index access stats
- `pg_stat_bgwriter` + `pg_stat_checkpointer` — background writer / checkpointer screen (single-row, hotkey `b`; PG 14–18, `pg_stat_checkpointer` columns on PG 17+; TUI-only / not recordable in 0.11.0)
- `pg_stat_wal` — WAL generation stats (PG 14+; reduced schema in PG 18)
- `pg_stat_statements` — top queries by various metrics (requires extension)
- System stats — CPU, memory, disk, network (read from /proc or via PL/Perl schema)

## Target Audience

PostgreSQL DBAs who need to monitor and troubleshoot Postgres in production without GUI tools.

## PostgreSQL Version Support

Active support: PG 14, 15, 16, 17, 18.
EOL versions (9.5–13) are no longer tested but code paths remain for reference.

## Current Status (May 2026)

Active development resumed with v0.10.0 after 5 years.
Priorities: stability, PG version compatibility, community contributions.
