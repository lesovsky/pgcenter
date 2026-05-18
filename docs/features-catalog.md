# Features Catalog

User-facing capabilities of the product. Updated after each feature is finalized.
Used by spec-writer to understand existing functionality and avoid duplication or conflicts.

---

### [001-feat-per-process-system-stats] Per-process System Stats Screen

**What it does:** Opens a new TUI screen (`Shift+S`) showing all PostgreSQL backends with their real-time CPU utilization and IO activity read from Linux procfs, alongside the pg_stat_activity columns. Lets DBA immediately see whether a specific backend is CPU-bound or IO-bound without leaving pgcenter.

**Key scenarios:**
- Press `Shift+S` to switch to the per-process stats screen showing 17 columns: activity info (pid, datname, usename, state, wait_etype, wait_event) + accumulated CPU time (`all_total,s`, `us_total,s`, `sy_total,s`) + accumulated IO (`read_total,KiB`, `write_total,KiB`) + CPU rates (`%all`, `%us`, `%sy`) + IO rates (`read,KiB/s`, `write,KiB/s`) + query
- Press `I` to hide idle backends, press `A` to filter by minimum age threshold — same controls as the activity screen
- Run pgcenter without root or postgres-user privileges: IO columns show empty, CPU columns work normally, warning message explains the required permissions

**Limitations:**
- Local mode only — procfs is not available over remote PostgreSQL connections
- Auxiliary PostgreSQL processes (checkpointer, bgwriter, WAL writer, archiver) are not shown — they are absent from `pg_stat_activity`
- IO metrics are syscall-level bytes (includes page cache), not actual disk IO
- Per-process iowait (`wa%`) is not available without Netlink taskstats API — deferred to a future issue
- `pgcenter record` / `pgcenter report` do not record this screen's data (deferred to a future version)

**Touches:** Activity screen (shares `I` and `A` filter hotkeys and their guard logic in `top/config_view.go` and `top/dialog.go`).
