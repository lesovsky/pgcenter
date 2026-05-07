# pgcenter — Project Overview

pgcenter is a command-line admin tool for observing and troubleshooting PostgreSQL in real time.
It reads PostgreSQL internal statistics views and presents them in a top-like interactive TUI.

## Commands

| Command   | Purpose |
|-----------|---------|
| `top`     | Real-time monitoring (main feature) — live stats updates |
| `record`  | Collect stats to files ("poor man's monitoring") |
| `report`  | Build reports from recorded files |
| `profile` | Wait events profiler — shows what queries are waiting on |

## Supported Statistics

- `pg_stat_activity` — active connections and their state
- `pg_stat_database` — per-database metrics (commits, rollbacks, tuples, deadlocks, temp files)
- `pg_stat_replication` — connected standbys and replication lag
- `pg_stat_user_tables`, `pg_stat_user_indexes` — table/index access stats
- `pg_stat_bgwriter` — background writer stats
- `pg_stat_statements` — top queries by various metrics (requires extension)
- System stats — CPU, memory, disk, network (read from /proc)

## Target Users

PostgreSQL DBAs who need to monitor and troubleshoot Postgres in production without GUI tools.

## Current Status (May 2026)

Active but slow-moving. Community contributes bug fixes and small improvements.
Owner focus: dependency updates → test improvements → PG version compatibility → new features.
