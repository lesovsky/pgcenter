# QA report — [012] PostgreSQL 19 compatibility baseline

**Date:** 2026-07-25
**Environment:** `lesovsky/pgcenter-testing:0.0.11` (published), clusters PG 14–19 on 21914–21919,
PostgreSQL 19beta2 (`19~beta2-1.pgdg22.04+1`).

## Automated

| Check | Result |
|---|---|
| `make test` on the feature branch, PG 14–19 | all packages pass |
| `make lint` | 0 issues |
| `make build` / `make install` | pass |
| `testing/e2e.sh` including port 21919 | pass |
| Same suite on **unmodified** `develop` with the new image | all packages pass, e2e pass — the image rebuild introduced no regression of its own |
| PG 19 subtests of the three progress screens | **PASS**, not SKIP — the queries executed against the live cluster and returned the promised column counts |
| Replay test, PG 18 vs PG 19 archives | pass; sabotaging the PG 19 diff interval reddens only the PG 19 case |
| Existing progress goldens | byte-identical, as expected |

`govulncheck` was launched but its output was not captured in the run that reported lint; it is the one
automated item to re-confirm.

## Manual, on live PG 19

**Screen walk.** 25 screens captured via tmux (`activity`, `databases`, `tables`, `indexes`, `sizes`,
`functions`, `replication`, `replslots`, `wal`, `bgwriter`, both IO screens, all seven statements
sub-screens, all six progress screens). No panics, no render errors; the header reports `ver: 19beta2`.

**New columns against psql, same moment — the feature's core criterion:**

```
psql:      5318 | manual | aggressive | scanning heap
pgcenter:  5318 ... bigt  manual  aggressive  active  Timeout.VacuumDelay  scanning heap
```

`started_by` and `mode` render exactly what the server reports. `mode = aggressive` was reproduced with
`VACUUM FREEZE` — cheaper than the spec assumed, which allowed for skipping it.

`backup_type` was confirmed in the catalog during a live `pg_basebackup` (`5380 | full | waiting for
checkpoint to finish`); the value did not land in a screen capture before the backup moved on.

**Lock counter** (the hardcoded `wait_event_type = 'Lock'` literal): with two sessions deliberately
blocked, psql counts 2 and the summary panel shows `2 waiting`. Match.

**Version reachability:** a connection request for a version absent from the port map returns an error
instead of silently connecting elsewhere — covered by a unit test that passes with no server running.

**`pg_basebackup`** works at all only because of this feature's `pg_hba.conf` fix; verified by taking an
actual base backup (`PG_VERSION: 19`).

## Not completed

- **REPACK on the cluster progress screen.** The command itself works on PG 19 beta2 (returns `REPACK`),
  and the structural basis of the compatibility claim was verified directly: `pg_stat_progress_cluster`
  still exposes all 12 columns including `command`, so the nine pgcenter reads are intact. But after the
  earlier `VACUUM FREEZE` the table rebuilt too quickly for the progress row to be caught on screen. The
  screen itself renders fine (captured in the walk). Worth one more attempt on a dirtier table.
- **`backup_type` on screen** — same shape of problem: confirmed in the catalog, not captured in a frame.
- **`mode = failsafe` and `started_by = autovacuum_wraparound`** — deliberately not reproduced, per the
  user-spec: same code path, disproportionate setup cost.

## Findings

None. No breakage was found on PG 19, so the routing rule for defects belonging to later features of the
release was not exercised.
