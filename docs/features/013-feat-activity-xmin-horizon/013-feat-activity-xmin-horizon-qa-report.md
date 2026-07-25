# QA Report — 013-feat-activity-xmin-horizon

**Date:** 2026-07-26
**Branch:** `feature/activity-xmin-horizon`
**Commits under test:** `1dbe2eb` (wave 1), `a1db017` (wave 2)
**Environment:** `lesovsky/pgcenter-testing:0.0.11`, PG 14–19 on ports 21914–21919, live
**Binary:** rebuilt from the wave-2 tree before every manual check

## Automated

| Gate | Result |
|---|---|
| `make test` | pass, total coverage 70.1% |
| `make lint` | 0 issues; gosec clean |
| `go test ./report/...` with the sort fix present | pass, goldens unchanged, none regenerated |
| Live column names + order, PG 14–19 | 6/6 **PASS** (not skip) |
| PG 9.5–13 subtests | skip, as expected — no such clusters in the image |

`make vuln` reports two findings (`x/text` v0.36.0 and `crypto/tls` in go1.25.11). Both are
dependency- and toolchain-level, untouched by this branch, and of the same family as the resolved
debt [015]. Not a gate failure for this feature.

## Manual — user-spec acceptance

| # | Check | Result |
|---|---|---|
| 1 | 17 columns in the fixed order, PG 17 | **pass** — verified in `report -A` output |
| 2 | `leader` groups a parallel query | **pass** — see below |
| 3 | `backend_xid` empty when nothing written, set after a write | **pass** — see below |
| 4 | Blank is never rendered as `0` | **pass** — one row showed `horizon_xacts = 0` beside a blank `backend_xid` |
| 5 | Sorting a sparse numeric column | **pass** — covered by seven unit tests, each demonstrated red first |
| 6 | Replay of a pre-0.12 archive | **pass** — existing goldens pass unmodified |
| 7 | `report -d -A` shows three columns, PG 13+ note, four caveats | **pass** — rendered and read by eye |
| 8 | Tech-debt register updated | **pass** — [021] resolved, [020] corrected, [022]–[026] added |

### Parallel worker grouping — the decisive evidence

Live catalog during a 4-worker parallel scan:

```
pid    leader  raw leader_pid   backend_type
2206   2206    NULL             client backend    <- the leader
2208   2206    2206             parallel worker
2209   2206    2206             parallel worker
2210   2206    2206             parallel worker
2211   2206    2206             parallel worker
2212   2212    NULL             client backend    <- unrelated backend
```

The leader's raw `leader_pid` is NULL. This confirms empirically that the roadmap's original
scope — expose `leader_pid` so workers sort next to their leader — would not have achieved its own
stated goal: the leader would have sorted into the NULL block together with the unrelated backend
2212, away from its own workers. The derived column gives the leader and all four workers the same
value, and leaves unrelated backends on their own PIDs.

### backend_xid — the argument for keeping it, demonstrated

A psql session inside a transaction that had run an `INSERT`, then `pg_sleep`:

```
pid    leader  wait_event  state   backend_xid  horizon_xacts   query
2134   2134                active               2               (pgcenter's own, read-only)
2126   2126    PgSleep     active  784          2               pg_sleep(...)
```

Session 2126 shows an innocuous `pg_sleep` in `query` while `backend_xid = 784` reports that the
transaction has written. This is exactly the case argued during the interview: for an
idle-in-transaction session `query` holds the *last* statement and therefore misleads the
kill-safety decision, which is the question `backend_xid` exists to answer.

## Manual — checks added by the tech-spec beyond the user-spec

| Check | Result |
|---|---|
| String column sorted ascending | **pass** — `wait_event` ascending puts the blank row last; under the old comparator `""` sorted first |
| `replslots` default sort | **pass** — `retained,KiB` 8 above blank; Go now agrees with the query's own `DESC NULLS LAST` |
| Load: query cost with many sessions | **pass with a stated limit** — see below |

### Load

At 96 concurrent sessions under pgbench, the activity query completed in **0.06 s**, measured
including `psql` process startup, so the query itself is faster. Against the default 1 s refresh
interval that is at most 6% of the budget. The roadmap's "do not make an incident worse" rule
holds.

**Stated limit:** the fixture's `max_connections` is 100, so "a few hundred sessions" as the
tech-spec words it could not be reached — the measurement is at 96. Scaling beyond that is
inference, not measurement.

## Honest limits of this QA

- **The PG 12→13 branch boundary was not verified live and cannot be.** The image carries PG 14–19
  only. The boundary rests entirely on the table test, which pins it from both sides. A `140000`
  written where `130000` belongs would pass every live check performed here.
- **`replslots` ascending sort** was verified by the replay test
  `Test_app_doReport_ReplSlots_EmptyRetained` rather than by hand; the by-hand check covered the
  default descending direction only.
- **Load ceiling** as above.
- **One acceptance item is deliberately left to the user:** whether the screen has become harder to
  read now that `query` sits 38 characters further right. That is a judgement about the most-used
  screen in the tool and cannot be delegated to a test or to an agent.

## Verdict

Ready for review. Every automated gate passes, every user-spec acceptance criterion is met, and the
three checks the tech-spec added beyond the user-spec pass with one measurement limit stated
explicitly rather than glossed.
