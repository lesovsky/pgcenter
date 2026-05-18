package query

// PgStatActivityProcPidStat selects 7 columns from pg_stat_activity for the
// per-process system stats screen ("procpidstat").
//
// Column order (positional — Tasks 3–6 index by position):
//
//	0: pid
//	1: datname
//	2: usename
//	3: state
//	4: wait_etype
//	5: wait_event
//	6: query
//
// Template conventions mirror PgStatActivityDefault in activity.go:
//   - QueryAgeThresh is always embedded in the WHERE clause (no {{ if }} guard).
//     The default value "00:00:00.0" set by NewOptions means all rows pass.
//   - ShowNoIdle is conditional via {{ if .ShowNoIdle }}AND state != 'idle'{{ end }}.
//
// Nullable columns are wrapped with coalesce(col, empty-string) to avoid NULL
// handling downstream. The query column is additionally normalized via
// regexp_replace to collapse whitespace (matches behavior of PgStatActivityDefault).
// WHERE pid != pg_backend_pid() excludes the pgcenter backend itself.
const PgStatActivityProcPidStat = `SELECT pid,
    coalesce(datname, '') AS datname,
    coalesce(usename, '') AS usename,
    coalesce(state, '') AS state,
    coalesce(wait_event_type, '') AS wait_etype,
    coalesce(wait_event, '') AS wait_event,
    regexp_replace(coalesce(query, ''), E'\\s+', ' ', 'g') AS query
FROM pg_stat_activity
WHERE pid != pg_backend_pid()
AND ((clock_timestamp() - query_start) > '{{.QueryAgeThresh}}'::interval)
{{ if .ShowNoIdle }}AND state != 'idle'{{ end }}
ORDER BY pid`
