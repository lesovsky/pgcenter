# Code Research — 013 Activity: xmin horizon + parallel worker grouping

**Date:** 2026-07-25
**Feature base:** `docs/features/013-feat-activity-xmin-horizon/013-feat-activity-xmin-horizon`
**Scope:** add `backend_xid`, `backend_xmin`, `horizon_xacts`, `leader_pid` to the existing
`activity` view; new PG 13+ query branch becomes the default; `PgStatActivityDefault` becomes
the PG 10–12 branch.

Verification note: several claims below are backed by a **live experiment** run against the
working tree (a temporary 18-column PG 13+ branch was wired into `internal/query/activity.go`
and `internal/view/view.go`, the suite was run, then both files were reverted via
`git checkout`). Those claims are marked **[measured]**. The tree is clean.

---

## 1. Entry Points

### `internal/query/activity.go` (55 lines, whole file is in scope)

Three query constants + one selector.

| Const | Lines | Ncols | Notes |
|---|---|---|---|
| `PgStatActivityDefault` | 6–16 | 14 | PG 10+ today; becomes the **PG 10–12** branch |
| `PgStatActivity96` | 20–29 | 13 | no `backend_type` |
| `PgStatActivity95` | 33–42 | 12 | `waiting` instead of `wait_event_type`/`wait_event` |

Current column order of `PgStatActivityDefault` (activity.go:6–12):

```
0 pid          5 appname       10 xact_age
1 cl_addr      6 backend_type  11 query_age
2 cl_port      7 wait_etype    12 change_age
3 datname      8 wait_event    13 query
4 usename      9 state
```

Selector (activity.go:45–55):

```go
func SelectStatActivityQuery(version int) (string, int)
```

Returns `(template, ncols)`. **Only two callers exist:**

- `internal/view/view.go:375` — `view.QueryTmpl, view.Ncols = query.SelectStatActivityQuery(opts.Version)`
- `internal/query/activity_test.go:22,33`

**Does the signature have to stay `(string, int)`?** Yes, and it can. The 4 new columns are
appended/inserted into a non-diffed view (`DiffIntvl = {0,0}`), so there is no `DiffIntvl` to
return and no `UniqueKey` to shift. Compare with the richer selectors that had to grow their
signature because their diffed window moved: `SelectStatBgwriterQuery` /
`SelectStatWALQuery` / `SelectStatProgressVacuumQuery` return `(string, int, [2]int)`
(`view.go:390,393,405`) and `SelectStatStatementsJITQuery` returns `(string, int, [2]int, int)`
(`view.go:387`). `SelectStatReplicationQuery(version int, track bool) (string, int)`
(`replication.go:56`) is the closest analogue — same 2-value shape, extra input parameter.

Recommended shape (no signature change needed):

```go
func SelectStatActivityQuery(version int) (string, int) {
	switch {
	case version < PostgresV96:  return PgStatActivity95, 12
	case version < PostgresV10:  return PgStatActivity96, 13
	case version < PostgresV13:  return PgStatActivityDefault, 14
	default:                     return PgStatActivityPG13, 18
	}
}
```

Note the existing selector uses raw literals (`90600`, `100000`) rather than the
`query.PostgresV*` constants declared at `internal/query/query.go:9–23`. Neighbouring
selectors (e.g. `replication.go:58`) do the same. Matching local style means raw literals;
using the constants would be a (small, unrequested) style change.

### `internal/view/view.go`

`New()` registers `activity` at **view.go:40–50**:

```go
"activity": {
    Name:      "activity",
    QueryTmpl: query.PgStatActivityDefault,
    DiffIntvl: [2]int{0, 0},
    Ncols:     14,
    OrderKey:  0,
    OrderDesc: true,
    ColsWidth: map[int]int{},
    Msg:       "Show activity statistics",
    Filters:   map[int]*regexp.Regexp{},
},
```

`Configure()` case at **view.go:374–376**. Nothing else in `Configure` touches `activity`.

**Convention question for the map default — this is a real fork, and it decides whether any
test changes at all.**

The `New()` map is the *pre-`Configure`* seed. Every production path calls `Configure` before
anything renders (`top/top.go:63`, `record/record.go:129`, `report/report.go:258`), so the seed
values are never what a user sees. The prevailing convention (established by 012) is to leave
the seed at pre-branch values: `bgwriter` seeds `PgStatBgwriterPG14`/12 (view.go:144–146) while
its PG 18/19 branch is 14 columns; `progress_vacuum` seeds `Default`/13 (view.go:280–282) while
PG 19 is 15. 012's acceptance criteria stated this explicitly ("the static `New()` map still
holds pre-19 values").

**[measured] — the choice is worth exactly two test edits:**

| Option | `New()` entry | Test fallout |
|---|---|---|
| **A (recommended)** — follow 012 | leave `QueryTmpl: query.PgStatActivityDefault, Ncols: 14` untouched | **zero.** Full run of `internal/view`, `internal/query`, `report`, `top`, `record`, `internal/align` is green (only the 3 pre-existing no-PG failures). `internal/view/view.go` needs **no edit at all** — the `Configure` wiring at :375 already exists |
| B | `QueryTmpl: query.PgStatActivityPG13, Ncols: 18` | `top/config_view_test.go:18,48` fail (`want: 13` → 17; `{orderKey: 13}` → `{orderKey: 17}`) |

Option A is also the smaller diff: only `internal/query/activity.go` changes on the code side.

**Who else reads `activity`'s `Ncols`?** Exactly two places in the whole repo, both in `top`:

- `top/config_view.go:26` — `orderKeyLeft`: wraps `OrderKey` to `Ncols - 1`
- `top/config_view.go:38` — `orderKeyRight`: wraps `OrderKey` to `0` at `>= Ncols`

`report/` **never** reads `view.Ncols` (verified by grep: the only `Ncols` references in
`report/report.go` are `res.Ncols` at :360 and :395 — both on the *recorded* result).
`align.SetAlign` reads `r.Ncols` from the `PGresult`, never from the view (align.go:43,56).
`top/stat.go:676,949` also read `s.Result.Ncols`, not the view's.

---

## 2. Data Layer

### `stat.PGresult` (internal/stat/postgres.go:443–450)

```go
type PGresult struct {
	Values [][]sql.NullString
	Cols   []string
	Ncols  int
	Nrows  int
	Valid  bool
}
```

Built by `NewPGresultQuery(db, query)` (postgres.go:453–515): `ncols` and `Cols` come from
`rows.FieldDescriptions()` — i.e. **from the live result set**, not from the view. Every value
is scanned into `sql.NullString` (postgres.go:478–482).

### Diff path — activity never diffs

`Compare` → `calculateDelta` (postgres.go:575–602):

```go
if interval != [2]int{0, 0} {
    delta, err = diff(curr, prev, itv, interval, ukey)
} else {
    delta = curr          // <-- activity takes this branch
}
delta.sort(skey, desc)
```

Activity's `DiffIntvl` is `{0,0}` (view.go:43), so **`diff()` is never called for activity**.
Consequences:

- `coalesce(...,0)` is **not** needed for the new columns. The replication_slots lesson
  (`coalesce` on diffed cumulative columns) does not apply here — it exists so `diffPair`
  does not choke on an empty string. Activity's values are copied verbatim.
- NULL → `sql.NullString{String: "", Valid: false}` → prints as blank. Decision 5 ("render
  blank, never 0") is satisfied by *not* coalescing. Adding `coalesce(backend_xid, 0)` would
  actively violate Decision 5.

### Sorting on a mostly-NULL column (postgres.go:663–701)

`sort()` picks the comparator from `r.Values[0][key].String` — the **first row's** value:

```go
sample := r.Values[0][key].String
if _, err := strconv.ParseFloat(sample, 64); err == nil { /* numeric */ }
else if _, err := parseDuration(sample); err == nil { /* duration */ }
else { /* string fallback */ }
```

If the user sorts by `backend_xmin` and row 0 happens to be NULL (`""`), `ParseFloat("")`
fails, `parseDuration("")` fails → **string sort**, so `"9999"` sorts before `"10000"`. This
is pre-existing behaviour of the sampling heuristic (same issue exists today on any
sometimes-empty column), not a regression, but it is worth stating in the spec as known
behaviour for the three new numeric-ish columns.

---

## 3. Report / Replay Compatibility — the high-priority question

### Is `activity` recordable? — **Yes.**

`record/record.go:200–233` `filterViews()` drops a view only when:
1. `v.NotRecordable == true` (activity does not set it — view.go:40–50, zero value `false`), or
2. `!v.VersionOK(version)` — activity has no `MinRequiredVersion` (zero → always OK), or
3. the key has prefix `statements_` and pgss schema is missing.

Activity therefore always survives. Confirmed by the golden archive itself: it contains
`activity.20210614T115633.123.json` (`report/testdata/pgcenter.stat.golden.tar`).

### How replay handles a recorded column count ≠ live `Ncols`

The replay path never consults the view's column count. Trace:

1. `report/report.go:170,211` — `stat.NewPGresultFile(r, hdr.Size)` unmarshals the tar entry
   into a `PGresult` carrying its **own** `Cols`, `Ncols`, `Nrows` from the recording.
2. `report.go:250–268` — on the first sample (and on every metadata version change) the view
   is reconfigured from the archive:
   ```go
   if !prevStat.Valid || prevMeta.version != d.meta.version {
       ...
       views := view.Views{config.ReportType: v}
       err := views.Configure(query.Options{Version: d.meta.version})
       v = views[config.ReportType]
       continue
   }
   ```
   This is the feature-[008] version-metadata path. It **does** reconfigure from the archive's
   recorded PG version — but for activity it only rewrites `QueryTmpl` (dead in report) and
   `Ncols` (unread in report).
3. `report.go:305` — `countDiff(d.res, prevStat, itv, v)` uses only `v.DiffIntvl`, `v.OrderKey`,
   `v.OrderDesc`, `v.UniqueKey`. For activity `DiffIntvl == {0,0}` → `delta = curr` verbatim.
4. `report.go:311,476–486` — `formatStatSample` calls `align.SetAlign(*d, c.TruncLimit, true)`
   on the **diff result**, i.e. on the recorded shape, and writes `view.Cols` / `view.ColsWidth`
   from it.
5. `report.go:513` (header) and `report.go:564` (data) both loop `for i := range v.Cols` /
   `for i := range res.Cols` — **the recorded column list**, never `view.Ncols`.
6. Ordering: `report.go:296–302` resolves `OrderColName` against `d.res.Cols` (recorded names),
   so `report -A --order pid` still works on an old archive.

### Does a pre-0.12 (14-column) archive replay cleanly at live `Ncols = 18`? — **Yes. [measured]**

`report/testdata/pgcenter.stat.golden.tar` is a **PG 14 recording** (`meta.*` entry:
`"version_num":"140000"`) whose `activity.*` entry has `Ncols: 14` and exactly today's 14
column names. `Test_app_doReport` replays it against
`testdata/report_activity.golden` (report/report_test.go:33–36) plus four more activity
goldens (report_test.go:138–157).

Experiment: with the activity view registered at `Ncols: 18` and
`SelectStatActivityQuery(140000)` returning the new 18-column branch — i.e. exactly the
mismatch this feature creates —

```
go test -count=1 ./report/...   →  ok   github.com/lesovsky/pgcenter/report  0.860s
```

All five activity goldens matched byte-for-byte. **This is not "provable from code alone,
needs a test" — the repo already contains the test, it already covers the exact scenario, and
it already passes under the mismatch.** The spec should call `Test_app_doReport` out as the
standing regression guard for acceptance criterion (б), rather than commissioning a new
fixture archive.

Recommended addition anyway (cheap, matches feature 008/012 style): a
`report/report_record_activity_test.go` synthetic-replay test covering criterion (в) — *new*
archives show the new columns — which no existing test covers.

Harness to copy: `report/report_record_progress_vacuum_test.go`. Its shape (per case):
a hand-built 7-column `meta.*` `PGresult` whose `version_num` drives
`views.Configure` (:86–98); two stat snapshots (:100–118); a literal
`sysinfo.*` blob `{"ticks":100,"cpu_count":4}` (:120); an in-memory `archive/tar` with six
entries in tick order (:124–138); then `newApp(config)` → `app.writer = &buf` →
`app.doReport(tar.NewReader(&tarBuf))` (:140–152); assertions with explicit sentinels before
the golden compare (:154–170); goldens regenerated via the package-level
`update` flag (`report/report_test.go:24`) → `go test ./report/ -run … -update`.

Two hard constraints, both commented in the original: **the stat entry's basename must equal
`config.ReportType`** (otherwise `isFilenameOK`, report.go:408–424, skips it silently and the
test passes on an empty report), and **ticks exactly 1 s apart** so `itv == 1`.

What to drop for activity: everything diff-related. With `DiffIntvl {0,0}` there is no delta
sentinel like the progress test's `700`, and the same-`pid`-across-ticks requirement is moot.
Be honest in the doc comment: the test proves that an 18-column recorded result
renders/aligns/truncates correctly and that a 14-column archive still renders as before — it
**cannot** prove version-aware layout selection, because at report time neither `Ncols` (never
read) nor `DiffIntvl` (identical on both branches) differs.

### Tech debt [020] and [021] — reachability assessment

**[020] "Diff loop indexes the previous snapshot by the current snapshot's width"
(`internal/stat/postgres.go:632,638,651`) — NOT made reachable by this feature.**
`diff()` is only entered when `DiffIntvl != {0,0}` (postgres.go:590). Activity is `{0,0}`
(view.go:43) and stays `{0,0}` — none of the 4 new columns is cumulative. Activity cannot
reach the diff loop at all, before or after this feature.

**[021] "Column widths not recomputed after a mid-archive version change"
(`report/report.go:476–486`) — marginally widened, already reachable today, and it is a panic
not a cosmetic bug.**

- `formatStatSample` returns early on `view.Aligned` (report.go:477) and nothing ever resets
  `Aligned`; `Configure` (view.go:367–427) does not clear it either.
- So an archive recorded across a major upgrade that changes activity's column count keeps the
  first layout's `ColsWidth`. In `printStatSample` the missing map keys read `0`, and
  report.go:566–570 has **no zero-width guard**:
  ```go
  if valuelen > view.ColsWidth[i] {
      width := view.ColsWidth[i]                       // 0 for a column added after alignment
      res.Values[rownum][colnum].String = ...[:width-1] + "~"   // [:-1] → panic
  }
  ```
  (`top/stat.go:1003–1008` *does* have the guard — `printDataCell` returns
  `"zero or negative width, skip"`. `report` does not. This asymmetry is worth noting.)
- Reachability **today**: activity already changes width across the 9.6→10 boundary
  (13 → 14, activity.go:51,53), so an archive spanning a 9.6→10 upgrade already triggers it.
- Reachability **after this feature**: one more boundary, 12→13 (14 → 18).
- Verdict: the feature does not open a new class of bug; it adds one more (equally exotic)
  trigger to an already-reachable one. Severity stays Low. If the spec wants to close it
  cheaply, the minimal fix is `view.Aligned = false` next to the `views.Configure(...)` call at
  report.go:258–265 — three characters of behaviour, inside the version-change branch that is
  already skipping the sample.

---

## 4. Tests That Break or Need Updating

Baseline in this environment (no live PG clusters on ports 21910–21919): `record.Test_app_setup`,
`record.Test_tarRecorder`, `top.Test_getQueryReport` fail for lack of a database. Those are
noise. With that subtracted, the experiment gives an exact list. **[measured]**

### Under Option A (`New()` untouched) — **nothing breaks. [measured]**

```
go test -count=1 ./internal/view/... ./record/... ./report/... ./top/... \
                 ./internal/query/... ./internal/align/...
→ ok internal/view · ok report · ok internal/query · ok internal/align
→ record: Test_app_setup, Test_tarRecorder   (baseline, needs live PG)
→ top:    Test_getQueryReport                (baseline, needs live PG)
```

### Under Option B (`New()` bumped to 18) — 2 tests, both in `top/config_view_test.go`

| Test | Line | Current | After |
|---|---|---|---|
| `Test_orderKeyLeft` | config_view_test.go:18 | `{orderKey: 0, want: 13}` + comment "because of `views["activity"].Ncols == 13`" | `want: 17` |
| `Test_orderKeyRight` | config_view_test.go:48 | `{orderKey: 13, want: 0}` "13 is the index of last column" | `{orderKey: 17, want: 0}` |

The comment at :18 is already stale — it says `Ncols == 13` while `view.New()` sets 14; it
means `Ncols - 1`. Worth correcting whichever option is taken.

### NOT broken — verified, do not "fix" preemptively

- `internal/query/activity_test.go:10–26` `TestSelectStatActivityQuery` — its table only covers
  `90500/90600/100000`, and `100000` still maps to `PgStatActivityDefault`/14 under Decision 3
  (branch point is PG 13). **Passes unchanged.** It must be **extended** with a `130000+` case,
  not repaired.
- `internal/query/activity_test.go:28–50` `Test_StatActivityQueries` — version list at :29 is
  `{90500…190000}`, already includes 130000–190000, and executes the query live. It needs no
  edit to keep passing; it will exercise the new branch automatically once clusters are up.
  This is the test that will catch an `xid`/`age()` SQL mistake.
  **But it proves less than it looks:** line 33 discards the returned Ncols (`tmpl, _ := …`) and
  line 44 runs `conn.Exec(q)`, so the declared column count is never checked against a real
  server. Feature 012 upgraded the analogous progress test to `conn.Query(q)` +
  `assert.Len(t, rows.FieldDescriptions(), wantNcols)` — see
  `internal/query/bgwriter_test.go:41,56–59` and `progress_vacuum_test.go:15–33`. Copying that
  upgrade here is the single highest-value test change in this feature: it is what turns
  "18" from a claim into a verified fact on PG 13–19.
- `internal/view/view_test.go` `TestNew` (:10–12, count 27), `TestView_VersionOK` (:~270,
  per-version totals), `record/record_test.go` `Test_filterViews` (:109–150) — all count
  **views**, not columns. This feature adds no view. **Unaffected — confirmed by run.** The
  `patterns.md` note about count-based tests does not apply here.
- `internal/view/view_test.go` `TestViews_Configure` — asserts activity only in the `90600`
  (:236–237) and `90500` (:239–240) cases. **Passes unchanged**, but should gain a
  `case 130000:` / `case 140000:` assertion pinning `PgStatActivityPG13` and `Ncols: 18`
  (mirroring how 012 added the `case 190000:` block at view_test.go:186–192).
- `report/` — all activity goldens pass under the mismatch (see §3). `Test_describeReport`
  (report_test.go:1179) compares against the const itself, so editing
  `pgStatActivityDescription` cannot break it.
- `top/stat_test.go` — the `visibleColumns` tests (:595 onward, :813–1002) build their own
  synthetic column sets; none reads `views["activity"]`.
- `internal/stat/stat_test.go:36–48, 81, 149, 237, 415–431` — five hand-built activity
  `view.View` literals carrying `QueryTmpl: query.PgStatActivityDefault` and `Ncols: 14`. Each
  is immediately followed by `views.Configure(opts)` with the **live** version (e.g.
  stat_test.go:49–50, 433), which overwrites both fields — so the literals are inert seeds and
  do **not** break. Decision 3 keeps the `PgStatActivityDefault` identifier alive, so there is
  no rename fallout either; renaming it would break all five.
  ⚠️ **One live assertion to watch:** `internal/stat/stat_test.go:442` asserts
  `assert.NotEqual(t, 19, s.Pgstat.Result.Ncols)` — "result must NOT be the 19-col procpidstat
  shape". At 18 columns this still passes, but activity is now **one column away** from
  colliding with procpidstat's 19 and silently neutering that guard. Worth a comment, or
  re-anchoring the assertion on `Cols` names instead of the count.
- `top/config_view_test.go` filter/sort/verbose tests, `top/signal_test.go`,
  `top/verbose_test.go` — use `config.views["activity"]` but assert nothing about its width.

---

## 5. UI / Render Interaction

### Horizontal scroll ([009], `top/stat.go:751–850`)

`visibleColumns(ncols, colsWidth, termWidth, offset)` is width-driven and count-agnostic:
column 0 is frozen and always charged to the budget (:770), columns `1..ncols-1` form the
sliding window, `maxOffset` is derived from a backward walk (:811–826), and the offset is
re-clamped every frame and written back (`renderDbstat`, top/stat.go:676–683). Growing activity
from 14 to 18 columns just makes `maxOffset` larger — no structural interaction.

Two real, non-blocking consequences:

1. **The trailing `query` column moves further right.** `countFit` (:783–793) counts a column as
   visible once its *start* is inside the budget, deliberately so the very wide `query` column
   stays partially visible instead of vanishing (comment at :778–782). With 4 more columns ahead
   of it, `query` is reached at a higher offset — on an 80-column terminal the operator will have
   to scroll to see the query text that used to be on screen. Placement of the new columns
   therefore matters for daily UX (see below).
2. **`scrollOffset` is reset on view switch** (`top/config_view.go:243`) and re-clamped per
   render, so a stale offset from a narrower layout cannot leak.

**Column placement recommendation for the spec** (a decision the spec still owes — see §7,
pitfall #1): appending all four at the end would put them *after* `query`, effectively
unreachable. Feature 012 faced the identical question and settled on inserting mid-layout,
*before* `state`, "because the columns read as attributes of the row, not as metrics, and the
tail is where `query` lives — the column horizontal scroll can push it out of view"
(ADR [012], `docs/decisions-log.md:801`). Applying the same reasoning here: place the four new
columns immediately **after `state`, before `xact_age`** — the horizon signal then sits
adjacent to the `xact_age` column the roadmap names as its time counterpart, and `query` stays
last. That is the layout used in the measurement experiment.

### `alignViewToResult` (top/stat.go:629–643)

```go
if config.view.Aligned && len(config.view.ColsWidth) == r.Ncols { return }
```

This is the issue-#99 guard: it re-aligns whenever the map size disagrees with the result width.
It makes the `top` path robust to any transient view/result column-count mismatch — including a
`view.New()` map default of 14 followed by a live 18-column result. So registering either 14 or
18 in `New()` is safe for rendering; only `orderKeyLeft`/`orderKeyRight` (§1) read the raw
`Ncols`, and those run after `Configure`.

### `internal/align/align.go` and mostly-NULL columns

`SetAlign` (align.go:14–79) computes per-column width as `max(len(value), max(len(colname), 8))`.
For a column that is NULL in every row, `valuelen = max(0, 1) = 1` and
`aligningIsLessThanColname(1, colnamelen, 0)` is true → width = `colnamelen` (≥ 8). So an
all-NULL `backend_xid` column renders as a header-width column of blanks. **No misbehaviour, no
zero width, no panic.** (`horizon_xacts` is 13 characters, so its width is name-driven anyway.)

One consequence to note: `top` uses `dynamic = false` (top/stat.go:639) and clamps very wide
values to 32; `report` uses `dynamic = true` (report.go:482). Neither is affected by empty cells.

### Filters `I` / `A` and the `procpidstat` screen

- `A` (age threshold) → `top/config_view.go:359–365` sets `config.queryOptions.QueryAgeThresh`;
  `I` (show idle) → `top/config_view.go:410–430` toggles `config.queryOptions.ShowNoIdle`. Both
  are **template variables** (`internal/query/query.go:33,35`) substituted by `query.Format`.
  Any new activity branch must keep both placeholders verbatim:
  `'{{.QueryAgeThresh}}'::interval` in the WHERE clause and
  `{{ if .ShowNoIdle }} AND state != 'idle' {{ end }}`. Omitting either silently disables a
  documented keybinding.
- `procpidstat` uses a **separate 7-column query**, `query.PgStatActivityProcPidStat`
  (`internal/query/procpidstat.go:25–36`), registered independently at view.go:349–359 with
  `Ncols: 19` (the SQL 7 columns plus 12 procfs-derived ones — see
  `internal/stat/procpidstat.go:29` `procPidResultNcols = 19`). It has its own template copies of
  `QueryAgeThresh`/`ShowNoIdle` and is **not** produced by `SelectStatActivityQuery`.
  **Verified unaffected.** Its positional column contract is documented at procpidstat.go:6–14
  and locked by `TestProcPidColIndexConstants`.
- Regex filters (`top/config_view.go:116–131`) are keyed by `view.OrderKey` (the current sort
  column index) and live only in memory; there is no `pgcenterrc`-style persistence of
  `OrderKey`/filters anywhere in the repo (grep: no `pgcenterrc`). So a column-index shift cannot
  corrupt a saved user config.

---

## 6. Documentation / Help Surfaces

| File | Lines | What | Action |
|---|---|---|---|
| `report/describe.go` | 179–199 (`pgStatActivityDescription`) | `report -d -A` output; 14 column rows | **must** add 4 rows |
| `internal/stat/help.go` | 144–166 (`PgStatActivityDescription`) | same table, different copy | see note below |
| `top/help.go` | 1–86 | hotkey cheat-sheet only (`a,b,f,o` mode keys at :13; `I`/`A` at :32–37) | **no change** — no new hotkey |
| `README.md` / `doc/` | — | check for an activity column list | verify during implementation |

**`report/describe.go` — follow the 012 house style exactly:**

1. Rows go in **emitted column order**, not alphabetically or grouped by theme.
2. Add a trailing availability note in the shape 012 established
   (`report/describe.go:221` — `Note: started_by and mode are available since PG19.`), e.g.
   `Note: backend_xid, backend_xmin, horizon_xacts and leader_pid are available since PG13.`
3. Add an `activity` case to **`Test_describeProgressColumnOrder`**
   (`report/report_test.go:1216–1252`) or a sibling test. This matters: the main
   `Test_describeReport` (report_test.go:1167+) compares the block against the const **by
   identity**, so it is structurally blind to a row in the wrong slot. The order test asserts
   marker positions with `require.NotEqual(t, -1, pos)` *before* the ordering compare —
   because `strings.Index` returns `-1`, which sorts before everything and would make a
   *missing* row look correctly ordered (report_test.go:1247–1249).

**`internal/stat/help.go` is dead code.** Grep across the repo finds **no consumer** of
`PgStatActivityDescription` (or any other `PgStat*Description` in that file) — the only hits are
the declarations themselves. `top/help.go` renders its own keybinding help and does not import
these. It has already drifted: `internal/stat/help.go:59–60` still calls the replication horizon
columns `xact_age*` / `time_age*`, whereas the live query names them `horizon_xacts` /
`horizon_age` (`internal/query/replication.go:28–29`) and `report/describe.go:75–76` says
`horizon_xacts` / `horizon_age`.

→ **Contradiction with the brief:** the task statement cites `internal/stat/help.go:59` as
precedent for the `horizon_xacts` name. It is not — that line says `xact_age*`. The real
precedents are `internal/query/replication.go:28` and `report/describe.go:75`, which do use
`horizon_xacts`. Decision 2 (the name) still stands; only the citation is wrong.

The spec should decide explicitly whether to (a) update the dead block for consistency,
(b) leave it, or (c) delete the file as debt cleanup. Not deciding means an inconsistent
docstring lands either way.

---

## 7. Similar Prior Feature — model this on 012

**Closest match: `012-feat-pg19-compatibility-baseline`** (archived
`docs/features/archive/012-feat-pg19-compatibility-baseline/`), specifically its progress-screen
half: *add columns mid-layout to three existing recordable views behind a new version branch*.
That is structurally the same problem, minus the diff-window complication.

### Its task/commit shape

| Commit | Task | Files |
|---|---|---|
| `4df20d7` | 02 | `internal/query/query.go` (version const), `internal/postgres/testing.go` (port map) + its test |
| `4313da8` | 03 | the three `internal/query/progress_*.go` + their tests **and** `internal/view/view.go` + `view_test.go` — deliberately **one** task because they all touch `view.go` |
| `8ebbe26` | 04 | 21 `_test.go` version-literal sweep, explicitly **excluding** `view_test.go` (owned by task 03: "tests live with the code they prove") |
| `f2f8902` | 05 | `report/describe.go` + `report/report_test.go` |
| `7230124` | 06 | new `report/report_record_progress_vacuum_test.go` + 2 goldens |
| `180de26` | 07 | project-knowledge docs |

For 013 the equivalent is smaller: no new version constant, no new port-map entry (PG 13 is
already `21913`, `internal/postgres/testing.go:28`), and under Option A no `view.go` change —
so tasks 02 and 03 collapse into "edit `internal/query/activity.go` + `activity_test.go`".

### Conventions it locked

- **Old constant keeps its name and stays byte-identical**; the new one is suffixed with its
  version (`PgStatProgressVacuumPG19`, progress_vacuum.go:19) — → `PgStatActivityPG13`.
- **A doc comment on the new constant states why the columns sit where they do and what index
  shift results** (progress_vacuum.go:15–18). Do the same.
- **Selector arity is driven by what actually moves** — `patterns.md:30–42`: `(string, int)`
  when only Ncols moves; `+[2]int` when DiffIntvl moves; `+int` when UniqueKey moves. Activity
  moves only Ncols → **the existing 2-tuple is correct**, and widening it would be gratuitous
  (012's "uniform arity across the family" argument applied to three *sibling* selectors
  introduced together; `SelectStatActivityQuery` has no siblings).
- **Branch shape:** 012's spec asserted "every branch is `>=` on the newest arm". That is
  **false for `activity.go:46–54`**, which is an ascending `switch { case version < …; default }`.
  Do not copy the `>=` idiom — insert `case version < 130000: return PgStatActivityDefault, 14`
  and make the new constant the `default` arm.
- `TestViews_Configure` gained a `case 190000:` block **and** a mirrored `case 140000:` proving
  the old branch is byte-identical (view_test.go:186–202). Mirror with `case 130000:` /
  `case 120000:` for activity.

### Pitfalls 012 explicitly recorded

1. **Column position is a product decision, not arithmetic.** 012's first research pass placed
   the new columns after `datname`; the `(Ncols, DiffIntvl)` math is identical wherever they go,
   so **no test can catch a wrong position** — a layout nobody approved would have shipped.
   012 settled on *before* `state`, "because the columns read as attributes of the row, not as
   metrics, and the tail is where `query` lives — horizontal scroll can push it out of view"
   (ADR [012], decisions-log.md:801). **013 must make this an explicit spec decision.**
2. **`DiffIntvl` is the only field that fails silently** — a stale interval produces a plausible
   wrong number rather than a crash. **Does not apply to activity** (`{0,0}` on both branches,
   `calculateDelta` short-circuits). This materially weakens the case for a heavyweight replay
   test here.
3. **Goldens must not change.** A diff in `report_activity*.golden` is "a red flag, not expected
   churn". Verified green in this research (§3).
4. **Execution tests that silently prove nothing** — flagged twice in 012's risk table. Exactly
   the `Exec` vs `Query`+`FieldDescriptions` issue in `Test_StatActivityQueries` (§4).
5. **Test expectations must be *derived* from the per-view gates, not copied from a lower row**
   (the `Test_filterViews` lesson).

### Relevant ADRs in `docs/decisions-log.md`

| ADR | Line | Bearing on 013 |
|---|---|---|
| [004] Per-version column sets, not NULL-padded unified columns | :191 | **Governing precedent.** Each branch returns only the columns that exist there; shared columns keep identical headers and order |
| [004] Absolute event counters via DiffIntvl placement | :207 | Explains why `{0,0}` copies everything as-is |
| [005] `coalesce(...,0)` on diffed counters for NULL safety | :285 | **Scoped to columns inside `DiffIntvl`.** Explicitly does not apply here — 012 restated it: "NULL renders blank — the columns sit outside `DiffIntvl`, so no `coalesce` is needed and none should be added" |
| [005] Single query for PG 14–18, no version branching | :301 | The counter-case: don't branch for niche, version-fragmented signal. 013's spec owes a sentence on why 4 columns clear that bar |
| [006] Per-version branch PG 16/17 vs PG 18 | :383 | Reference for a two-way branch with stable logical shape |
| [007] JIT selector returns a 4-tuple | :397 | Establishes the arity ladder — **not triggered here** (`OrderKey`/`UniqueKey` stay 0) |
| [008] Lift `NotRecordable` only — pure-SQL views need no recorder change | :429 | "report-time `Configure(Options{Version})` already selects the version-correct layout; the rebuilt SQL is never executed in report" — the formal basis for §3 |
| [008] Replay tests: synthetic in-memory tar + goldens, not the legacy fixture | :462 | The harness pattern. Caveat: that ADR calls the fixture "~PG13"; it is actually **140000**, and unlike the 0.11 screens it *does* contain activity — so 013 has fixture coverage for free |
| [012] Progress screens: new columns mid-layout, version-aware DiffIntvl | :801 | The column-placement rationale quoted above |
| [012] Test connection refuses unmapped versions instead of falling back | :778 | Relevant if any test loop gains a version |

---

## 8. Constraints, SQL and Type-System Notes

### Scanning `xid` through the pgx simple-protocol path — **works, no cast strictly required**

`internal/postgres/postgres.go:53` sets `pgConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol`,
so every value arrives in **text format**. Chain for `backend_xid` (OID 28) into `*sql.NullString`
(pgx v5.9.2, go.mod:8):

1. `pgtype/pgtype_default.go:92` registers `xid` → `Uint32Codec{}`.
2. `Uint32Codec.PlanScan` (`pgtype/uint32.go:227–248`) has **no case for `*sql.NullString`** in
   `TextFormatCode` → returns `nil`.
3. `Map.planScan` (`pgtype/pgtype.go:1122–1127`) then sees `target.(sql.Scanner)` with a non-nil
   registered type → `scanPlanCodecSQLScanner`.
4. That calls `Uint32Codec.DecodeDatabaseSQLValue` (`pgtype/uint32.go:250–261`), which returns
   `int64(n)` — or **`nil, nil` for a SQL NULL** (:251–253).
5. `sql.NullString.Scan(int64)` → `convertAssign` int64 → decimal string;
   `sql.NullString.Scan(nil)` → `{String: "", Valid: false}`.

So a bare `backend_xid` scans to `"748291"` and NULL scans to blank, exactly as Decision 5
requires. **Not verified live** — the fixture clusters (ports 21910–21919,
`internal/postgres/testing.go:19–35`) are down in this environment. `Test_StatActivityQueries`
covers it once clusters are up.

Style note: the existing `replication.go:28` writes `backend_xmin::text::bigint`. An explicit
`backend_xid::text` / `backend_xmin::text` in the new branch costs nothing, matches local
precedent, and removes the dependency on pgx codec-fallback behaviour. Recommended.

`leader_pid` is `integer` and `age(xid)` returns `integer` — both scan trivially.

### `age(backend_xmin)` vs the replication formula — **use `age()`; and flag the naming clash**

| | activity (this feature) | replication (`replication.go:28`) |
|---|---|---|
| Formula | `age(backend_xmin)` | `(pg_last_committed_xact()).xid::text::bigint - backend_xmin::text::bigint` |
| Requires GUC | **no** | `track_commit_timestamp = on` |
| Reference point | next XID to be assigned | last **committed** xid |
| Available since | always | 9.5 (function), gated by GUC |

`pg_last_committed_xact()` errors out with *"could not get commit timestamp data"* when
`track_commit_timestamp` is off — which is why the replication screen has the whole
`SelectStatReplicationQuery(version, track bool)` / `PgStatReplicationExtended` machinery, with
`track` computed at `internal/view/view.go:368–371` from `opts.GucTrackCommitTS`. Decision 1
correctly declines to bring that machinery to activity, so **`age(backend_xmin)` is the only
available formulation** and is the correct one. Confirmed available on every supported version.

**Naming-consistency implication (flag for the spec):** after this feature two screens will
have a column literally named `horizon_xacts`, computed by two different expressions against
two different reference points. In practice the two values differ by the number of
transactions assigned-but-not-yet-committed at sample time — normally small, but not
identical, and the replication one can be *negative* on an idle cluster while `age()` cannot.
Options: (a) accept and document the difference in both describe blocks; (b) switch the
replication screen's non-GUC-dependent half to `age(backend_xmin)` too — out of scope here,
and it would change an existing recorded column's meaning; (c) rename one. Decision 2 locks
the *name*, not the reconciliation, so the spec should state which of (a)/(b)/(c) it takes.
Recommended: **(a)** — one line in each describe block.

### Tech-debt items that touch this feature (ADRs are in §7)

- **[020]** — not reachable via activity (`DiffIntvl {0,0}`). No action. §3.
- **[021]** — reachable, already reachable today, one extra trigger added. Optional one-line
  mitigation at `report/report.go:258–265`. §3.
- **[019]** (`t.Skipf` without a per-version `t.Run` wrapper) — `activity_test.go` is **not**
  affected: `Test_StatActivityQueries` already wraps each version in `t.Run`
  (activity_test.go:32) and skips inside the subtest (:40). Good template to copy.
- **[016]** (silent error swallowing) — `NewPGresultQuery` skips unscannable rows with a bare
  `continue` (postgres.go:486–488). If the `xid` scan ever failed, activity would silently show
  fewer rows rather than an error. Argues for the explicit `::text` cast above.

### Build / CI

Go 1.25 with `toolchain go1.25.11` (go.mod:3,5); `make build` / `make test` (race + coverage) /
`make lint` (golangci-lint v2 + gosec) / `make vuln`. Test clusters 21910–21919 map version →
port at `internal/postgres/testing.go:19–35`; PG 13 is **21913** and is in the map, so the new
branch point is directly testable in CI.

---

## 9. Files to Change (checklist)

Assumes **Option A** (`New()` map untouched — §1).

| File | Change | Required? |
|---|---|---|
| `internal/query/activity.go` | add `PgStatActivityPG13` const (with a placement/index-shift doc comment); insert `case version < 130000: return PgStatActivityDefault, 14`; new const becomes `default`, 18. Keep `(string, int)` | **yes** |
| `internal/query/activity_test.go:10–26` | **extend** `TestSelectStatActivityQuery`: add `{120000, PgStatActivityDefault, 14}` and `{130000, PgStatActivityPG13, 18}` | **yes** |
| `internal/query/activity_test.go:28–50` | upgrade `Exec` → `Query` + `assert.Len(rows.FieldDescriptions(), wantNcols)` (pattern: `bgwriter_test.go:41,56–59`) | strongly recommended |
| `internal/view/view.go` | **none** — `Configure` wiring at :375 already handles it | no |
| `internal/view/view_test.go` | add `case 130000:` / `case 120000:` to `TestViews_Configure` (mirror view_test.go:186–202) | **yes** |
| `report/describe.go:179–199` | 4 rows in emitted order + `Note: … available since PG13.` | **yes** |
| `report/report_test.go:1216+` | add an `activity` case to `Test_describeProgressColumnOrder` (or a sibling order test) | **yes** |
| `report/report_record_activity_test.go` | **new** — synthetic in-memory tar replay, two cases (`120000` → 14 cols, `130000` → 18 cols) + goldens; covers criterion (в). Note it can only prove *render/align* correctness, not layout selection (§3) | recommended |
| `internal/stat/help.go:144–166` | decide: update / leave / delete (dead code, already drifted) | decision needed |
| `internal/stat/stat_test.go:442` | consider re-anchoring `NotEqual(19, Ncols)` on column names (activity is now 1 away from 19) | nice-to-have |
| `report/report.go:258–265` | optional one-liner `v.Aligned = false` to close debt [021] | optional |
| `top/config_view_test.go:18` | fix the stale `Ncols == 13` comment | cosmetic |
| `docs/` | features-catalog, `docs/tech-debt.md` ([021] note), PK `patterns.md` if it enumerates activity's branches | **yes** |

## 10. Open Decisions the Spec Still Owes

1. **Column placement** — no test can catch a wrong choice (012's pitfall #1). Recommended:
   the four new columns immediately after `state`, before `xact_age`, keeping `query` last.
2. **Option A vs B** for the `New()` map seed (§1). Recommended: A.
3. **`horizon_xacts` formula divergence** between the `activity` and `replication` screens
   (§8) — accept + document, or reconcile.
4. **`internal/stat/help.go`** — update, leave, or delete.
5. Whether to spend the one-liner on tech debt [021] inside this feature.

---

# Updated: 2026-07-25 — Implementation Level

Research pass 2, run against the **approved** user-spec. Everything below is either a
file:line citation or a **[measured]** fact. Measurements were taken by patching the working
tree, running, and reverting — `git status` is clean (only the harness-owned `*-metrics.json`
is modified).

Two live PostgreSQL 16 servers were available in this environment (a project container on
:5433 and a throwaway `postgres:16` spun up and destroyed for the parallel-worker probe), so
several claims that pass 1 had to leave as "not verified live" are now measured.

## 0. What pass 1 says that the approved spec overrides

Pass 1 was written against a 4-column, 18-column-total draft with the new columns placed after
`state`. The approved spec fixes **3 columns / 17 total** in a different order. Superseded:

| Pass-1 statement | Status |
|---|---|
| 4 new columns incl. raw `backend_xmin`; `Ncols = 18` | **superseded** — 3 columns, `Ncols = 17` |
| "place them after `state`, before `xact_age`" (§5, §10.1) | **superseded** — spec puts `leader` after `pid`, and `backend_xid`/`horizon_xacts` between `state` and `xact_age` |
| §10 "Open Decisions the Spec Still Owes" (all 5) | **closed by the spec** — placement fixed; Option A confirmed; formula divergence → document; `help.go` → leave + debt entry; [021] → fix in-feature |
| `PgStatActivityPG13` name, `(string, int)` arity, `DiffIntvl {0,0}`, no `coalesce` for NULL-rendering, replay path never reads `view.Ncols`, `Test_app_doReport` as the standing golden guard | **still valid**, re-verified below |

Pass-1 §4's note that `internal/stat/stat_test.go:442` asserts `NotEqual(t, 19, Ncols)` becomes
*less* pressing: at 17 columns activity is two away from procpidstat's 19, not one.

## 1. The new query branch

### 1.1 Current shape — `internal/query/activity.go` (55 lines, whole file in scope)

- `PgStatActivityDefault` — :6–16, 14 cols, PG 10+ today.
- `PgStatActivity96` — :20–29, 13 cols.
- `PgStatActivity95` — :33–42, 12 cols.
- `SelectStatActivityQuery(version int) (string, int)` — :46–55, ascending
  `switch { case version < 90600; case version < 100000; default }`.

Note the file uses **raw version literals** (`90600`, `100000`), not the `PostgresV*`
constants at `internal/query/query.go:9–23`. `replication.go:58` does the same.
`progress_vacuum.go:33` uses `PostgresV19`. Both idioms exist; matching the file means a raw
`130000`.

### 1.2 Target selector

```go
func SelectStatActivityQuery(version int) (string, int) {
	switch {
	case version < 90600:
		return PgStatActivity95, 12
	case version < 100000:
		return PgStatActivity96, 13
	case version < 130000:
		return PgStatActivityDefault, 14
	default:
		return PgStatActivityPG13, 17
	}
}
```

Arity stays `(string, int)`. Confirmed against the arity ladder in `patterns.md:30–42` and
ADR [007]: `+[2]int` only when `DiffIntvl` moves, `+int` only when `UniqueKey` moves.
Activity's view literal (`internal/view/view.go:40–50`) sets `DiffIntvl: [2]int{0,0}`,
`OrderKey: 0`, and no `UniqueKey` (zero value). **None of the three changes** — only `Ncols`
moves. Widening the signature would be gratuitous.

### 1.3 The new constant — live-verified on PG 16 **[measured]**

Place it directly after `PgStatActivityDefault`, with a doc comment naming the placement
rationale and the index shift — the shape `progress_vacuum.go:15–18` established.

```go
	// PgStatActivityPG13 queries activity stats from pg_stat_activity for PG 13 and newer.
	// PG 13 adds leader_pid; leader is derived as coalesce(leader_pid, pid) so a leader and its
	// workers share one value and sort as a group (raw leader_pid is NULL on the leader itself).
	// leader sits next to pid as a second identity attribute; backend_xid and horizon_xacts sit
	// between state and xact_age so they read as "did it write -> how far back it holds the
	// horizon -> how long it has been open". query stays last (widest column, partially-visible
	// tail of the horizontal scroll). Ncols 14 -> 17; DiffIntvl stays {0,0}.
	// - regexp_replace() removes extra spaces, tabs and newlines from queries.
	PgStatActivityPG13 = "SELECT pid, coalesce(leader_pid, pid) AS leader, " +
		"host(client_addr) AS cl_addr, client_port AS cl_port, " +
		"datname, usename, application_name AS appname, backend_type, " +
		"wait_event_type AS wait_etype, wait_event, state, " +
		"backend_xid::text AS backend_xid, age(backend_xmin) AS horizon_xacts, " +
		"date_trunc('seconds', clock_timestamp() - xact_start)::text AS xact_age, " +
		"date_trunc('seconds', clock_timestamp() - query_start)::text AS query_age, " +
		"date_trunc('seconds', clock_timestamp() - state_change)::text AS change_age, " +
		`regexp_replace(query, E'\\s+', ' ', 'g') AS query ` +
		"FROM pg_stat_activity " +
		"WHERE ((clock_timestamp() - xact_start) > '{{.QueryAgeThresh}}'::interval " +
		"OR (clock_timestamp() - query_start) > '{{.QueryAgeThresh}}'::interval) " +
		"{{ if .ShowNoIdle }} AND state != 'idle' {{ end }} ORDER BY pid DESC"
```

Both template placeholders (`{{.QueryAgeThresh}}`, `{{ if .ShowNoIdle }}`) must be carried
verbatim — they are the `A` and `I` hotkeys (`top/config_view.go:359–365`, `:410–430`).
Dropping either silently disables a documented keybinding.

**Executed against a live PG 16.** Column names, order and types come back exactly as the spec
fixes them:

```
attnum |    attname    | format_type      attnum |    attname    | format_type
     1 | pid           | integer               10 | wait_event    | text
     2 | leader        | integer               11 | state         | text
     3 | cl_addr       | text                  12 | backend_xid   | text
     4 | cl_port       | integer               13 | horizon_xacts | integer
     5 | datname       | name                  14 | xact_age      | text
     6 | usename       | name                  15 | query_age     | text
     7 | appname       | text                  16 | change_age    | text
     8 | backend_type  | text                  17 | query         | text
     9 | wait_etype    | text
```

### 1.4 Correction to the brief: only `backend_xid` needs `::text` **[measured]**

The brief says "both xid columns need explicit `::text`". Measured: `age(backend_xmin)`
returns **`integer`**, not `xid` — `age(xid)` is a plain catalog function. No cast is needed
and none should be added; `cl_port` is already an `integer` scanned into `sql.NullString`
today, so the codec path is proven. `backend_xid` **is** type `xid` and does need `::text`
(the `replication.go:28` precedent), which also removes the dependency on pgx's
`Uint32Codec` → `DecodeDatabaseSQLValue` fallback documented in pass 1 §8.

`NULL::text` is still NULL, so the blank-not-zero rendering is preserved by the cast.

### 1.5 Column semantics — measured on a live PG 16 parallel query

A 4-worker parallel scan inside a writing transaction, sampled from a second session:

```
 pid | leader | raw_leader_pid |  backend_type   | backend_xid | horizon_xacts | state  | xact_age
 117 |    117 |                | client backend  | 733         |             1 | active | 00:00:01
 119 |    117 |            117 | parallel worker |             |             1 | active | 00:00:01
 120 |    117 |            117 | parallel worker |             |             1 | active | 00:00:01
 121 |    117 |            117 | parallel worker |             |             1 | active | 00:00:01
 122 |    117 |            117 | parallel worker |             |             1 | active | 00:00:01
 136 |    136 |                | client backend  |             |             1 | active | 00:00:00
```

Every column AC is now a measured fact rather than a claim:

- `leader` = own pid for the leader (117) and for an unrelated backend (136); = the leader's
  pid for all four workers. Sorting by `leader` groups them. **AC 3 satisfied.**
- `raw_leader_pid` is **NULL on the leader** — the empirical basis for the spec's decision to
  expose the derived column rather than the raw one.
- `backend_xid` = `733` on the leader only. **Worth documenting:** workers of a *writing*
  transaction still show an empty `backend_xid`, because the xid belongs to the leader's
  transaction. A DBA sorting by `backend_xid` sees the leader, not the group.
- `horizon_xacts` = `1` identically on leader and all workers — the spec's "workers inherit
  the leader's snapshot" edge case, measured.
- Idle sessions in the same sample showed `backend_xid`, `horizon_xacts` **and** `xact_age`
  all blank; a session with an open snapshot but no writes showed `horizon_xacts = 0` with
  `backend_xid` blank — i.e. the "0 is not the same as blank" distinction the spec's UX rule
  rests on occurs in practice, not just in theory.

### 1.6 `coalesce` here does not violate ADR [005]

ADR [005] (decisions-log.md:285) scopes `coalesce(...,0)` to columns **inside** `DiffIntvl`,
for `diffPair` NULL-safety. `coalesce(leader_pid, pid)` is a *semantic derivation*, not
NULL-padding for the diff engine, and it is the spec's explicit decision. `backend_xid` and
`horizon_xacts` are deliberately **not** coalesced — that is what makes them render blank
(pass 1 §2, still valid: activity's `DiffIntvl {0,0}` sends `calculateDelta` down the
`delta = curr` branch at `internal/stat/postgres.go:596`, so values are copied verbatim).

### 1.7 `view.New()` seed stays at `PgStatActivityDefault` / 14 — confirmed

`internal/view/view.go:40–50`. `Configure()` already wires activity at **view.go:374–376**;
the case needs **no edit** — `SelectStatActivityQuery` keeps its signature.

Raising the seed to 17 breaks exactly two assertions, both in `top/config_view_test.go`:

- `:18` — `{orderKey: 0, want: 13}` with the (already stale) comment
  `// why 13? because of views["activity"].Ncols == 13`; it means `Ncols - 1`, and `New()`
  currently sets 14. Would become `want: 16`.
- `:48` — `{orderKey: 13, want: 0}` `// 13 is the index of last column`. Would become
  `{orderKey: 16, want: 0}`.

Those two lines are the only consumers of activity's `Ncols` in the repo
(`top/config_view.go:26` `orderKeyLeft`, `:38` `orderKeyRight`). `report/` never reads
`view.Ncols`. Leaving the seed alone matches the convention 012 locked and keeps the code
diff to `internal/query/activity.go` alone.

The stale comment at `config_view_test.go:18` should be corrected regardless (`Ncols - 1`).

## 2. The sort fix

### 2.1 Current code — `internal/stat/postgres.go:663–701`

```go
sample := r.Values[0][key].String            // :668 — row 0 only
if _, err := strconv.ParseFloat(sample, 64); err == nil { /* numeric   :670 */ }
else if _, err := parseDuration(sample); err == nil     { /* duration  :681 */ }
else                                                     { /* string    :692 */ }
```

Single caller: `calculateDelta` at :599, reached from `stat.Compare` (:575, used by
`report/report.go:453`) and directly from `internal/stat/stat.go:440` (the `top` path).

**[measured]** `strconv.ParseFloat("")` and `parseDuration("")` both error, so a column whose
first row is empty falls through to the string comparator — where `"9" > "1000000"`.

### 2.2 Target shape — prototyped, compiled, and run **[measured]**

```go
func (r *PGresult) sort(key int, desc bool) {
	if r.Nrows == 0 {
		return /* nothing to sort */
	}

	// Pick the comparator from the first non-empty cell, not from row 0: a column that is
	// empty in the first row (backend_xid, horizon_xacts are empty for most rows) would
	// otherwise fall back to the string comparator, where "9" sorts above "1000000".
	var sample string
	for i := range r.Values {
		if r.Values[i][key].String != "" {
			sample = r.Values[i][key].String
			break
		}
	}

	// An empty cell is not a zero: "holds no horizon" and "holds the horizon at distance 0"
	// are different states. Empties go last in both directions so they never mix with a
	// genuine 0.
	withEmptyLast := func(cmp func(a, b string) bool) func(i, j int) bool {
		return func(i, j int) bool {
			a, b := r.Values[i][key].String, r.Values[j][key].String
			if a == "" || b == "" {
				return a != "" && b == ""
			}
			return cmp(a, b)
		}
	}

	switch {
	case isParsableFloat(sample):
		sort.SliceStable(r.Values, withEmptyLast(func(a, b string) bool {
			l, _ := strconv.ParseFloat(a, 64)
			m, _ := strconv.ParseFloat(b, 64)
			if desc { return l > m }
			return l < m
		}))
	case isParsableDuration(sample):
		sort.SliceStable(r.Values, withEmptyLast(func(a, b string) bool {
			l, _ := parseDuration(a)
			m, _ := parseDuration(b)
			if desc { return l > m }
			return l < m
		}))
	default:
		sort.SliceStable(r.Values, withEmptyLast(func(a, b string) bool {
			if desc { return a > b }
			return a < b
		}))
	}
}
```

Notes on the shape:

- The `if/else if/else` chain has to become a `switch` (or two named predicates) because the
  comparator is now built by a wrapper — `isParsableFloat` / `isParsableDuration` are
  two-line helpers next to `parseDuration` (`postgres.go:707`).
- `sort.SliceStable` is preserved in all three arms (determinism requirement, `patterns.md`).
- The empty-vs-empty case returns `false` (`a != ""` is false), so equal-empty rows keep their
  incoming order under `SliceStable`.
- Sampling walks `r.Values` **before** the sort, so the sample is order-independent — this is
  what makes the mode deterministic regardless of which row arrived first.
- All-empty column: `sample == ""` → string arm → the comparator returns `false` for every
  pair → `SliceStable` is a no-op. No panic, no reordering.

**[measured]** behaviour of the prototype on a sparse numeric column
`["", "9", "1000000", "", "0", "250"]`:

```
desc: ["1000000" "250" "9" "0" "" ""]
asc : ["0" "9" "250" "1000000" "" ""]
```

`0` stays adjacent to the real numbers and never mixes with the blanks — exactly the AC.
On a sparse duration column `["", "791:04:45", "79:18:40", ""]`:

```
dur desc: ["791:04:45" "79:18:40" "" ""]
dur asc : ["79:18:40" "791:04:45" "" ""]
```

### 2.3 Existing tests — none break **[measured]**

| Test | Location | What it covers | Under the fix |
|---|---|---|---|
| `Test_sort` | `internal/stat/postgres_test.go:704–777` | 4 subtests (numeric/string × asc/desc) on `newTestPGresult()`; plus an empty-`PGresult` no-op case at :773–776 | **passes** — no empty cells in the fixture |
| `Test_sort_duration` | `postgres_test.go:782–814` | issue #50 regression: `"791:04:45"` must outrank `"79:18:40"` | **passes** |
| `Test_calculateDelta` | `postgres_test.go:443–510` | exercises `sort` via `calculateDelta` at :494/:499/:504 | **passes** |
| `Test_Compare` | `postgres_test.go:627` | `Compare(... skey 0 ...)` | **passes** |
| `Test_diff*`, `Test_diffPair`, `Test_parseDuration` | `postgres_test.go` | adjacent | **pass** |
| all `report/` goldens (~30, incl. 5 activity) | `report/testdata/*.golden` | replay output | **pass, byte-identical** |

Verified by running the prototype: `go test ./internal/stat/ -run 'Test_sort|Test_calculateDelta|Test_Compare|Test_diff|Test_parseDuration'` → PASS; `go test ./report/... ./internal/align/...` → ok; `go test ./top/ -run 'Test_orderKey|Test_visibleColumns|...'` → ok.
(`internal/stat.Test_readCpuStat` fails in this environment **before and after** the patch — a
`/proc` parsing baseline failure, not related.)

The activity goldens are insensitive because every one of them sorts on `pid`
(`view.New()` `OrderKey: 0`; the two order-variant goldens are
`report_activity_order_pid_asc/desc.golden`), and `pid` is never empty.

### 2.4 New test to add

`internal/stat/postgres_test.go`, next to `Test_sort_duration` — `Test_sort_sparse`:
a sparse numeric column asserting (a) numeric order regardless of which row is first
(run the same fixture twice with the empty row moved to position 0 and to the middle),
(b) blanks last in **both** directions, (c) a real `"0"` above the blanks. Add a sparse
duration case and an all-empty column case in the same test.

### 2.5 Blast radius — which screens change behaviour

Two independent behaviour changes ship together. Both need to be in the tech-spec's
blast-radius section, because **the second one is larger than the spec's Risk 5 describes.**

**(a) Mode change** — a sparse ★ column whose *first row* happens to be empty flips from the
accidental string comparator to numeric/duration. Non-deterministic today (depends on which
row the server returned first).

**(b) Placement change — the bigger one.** In a ★ column whose first row is *non*-empty, the
mode is already numeric/duration today, and `l, _ := strconv.ParseFloat("")` swallows the
error, so **an empty cell is currently ordered as `0`**. Consequences:

- **ASC: empty cells are currently FIRST in every ★ column** on every screen. All of them
  move to last. This is the change users will actually notice.
- **DESC: empties land last only if every real value is ≥ 0.** `activity.cl_port` legitimately
  holds `-1` (unix-socket connections), so today an empty `cl_port` sorts *above* `-1`.
  That row moves.

The spec's Risk 5 names only the mode change ("возраст транзакции… начнёт сортироваться как
длительность, а не как строка"). Change (b) applies to columns that already sort numerically
and is what makes this a general-engine change.

#### The sparse-and-numeric/duration set (columns whose ordering changes)

| View | idx | Column | SQL source | New mode |
|---|---|---|---|---|
| activity | 2 | `cl_port` | `client_port` NULL for background procs (`activity.go:6`) | numeric |
| activity | 10,11,12 | `xact_age`, `query_age`, `change_age` | `clock_timestamp() - xact_start` etc. NULL when the timestamp is NULL (`activity.go:9–11`) | duration |
| activity (new) | 11,12 | `backend_xid`, `horizon_xacts` | the feature's own columns | numeric |
| replication | 7–11 | `pending/write/flush/replay/total,KiB` | `sent_lsn`/`write_lsn`/… NULL while a standby is in `startup`/`backup` (`replication.go:7–11`), outside `DiffIntvl {6,6}` | numeric |
| replication (ext) | 15,16 | `horizon_xacts`, `horizon_age` | NULL without `hot_standby_feedback` (`replication.go:28–29`) | numeric / duration |
| databases_general | 18 | `stats_age` | `stats_reset` NULL for never-reset DBs (`databases.go:15`), outside `DiffIntvl {2,17}` | duration |
| **replslots** | **4** | **`retained,KiB`** | `restart_lsn` NULL for a slot that reserved no WAL (`replication_slots.go:18`) | numeric |
| replslots | 5 | `safe,KiB` | `safe_wal_size` NULL whenever `max_slot_wal_keep_size = -1` — **the Postgres default**, so 100 % empty on a stock cluster (`replication_slots.go:19`) | numeric |
| replslots | 14 | `stats_age` | LEFT JOIN → NULL for every *physical* slot (`replication_slots.go:27–28`) | duration |
| progress_vacuum | 1,7,8,9 | `xact_age`, `size_total,KiB`, `scanned_total,%`, `vacuumed_total,%` | `RIGHT JOIN pg_stat_activity` (`progress_vacuum.go:12`) → all `v.*` NULL for a VACUUM with no progress row yet | duration / numeric |
| progress_copy | 1,8,11 | `xact_age`, `size_total,KiB`, `processed,%` | explicit `nullif(p.bytes_total, 0)` → NULL for every `COPY FROM STDIN` (`progress_copy.go:10`) | duration / numeric |
| progress_basebackup | 7 | `size_total,KiB` | `backup_total` NULL while `initializing` (`progress_basebackup.go:10`) | numeric |
| progress_cluster / _index / _analyze | 1 | `xact_age` | NULL `xact_start` | duration |
| wal, bgwriter, stat_io, stat_io_time | last | `stats_age` | `stats_reset` NULL (`wal.go:10,20`; `bgwriter.go:12,24,35`; `io.go:33,56,77`) | duration |
| procpidstat | 9,10,11,15,16,17 | IO / iodelay totals and rates | **not SQL** — the enrichment stage writes a `""` sentinel per row when `/proc/<pid>/io` is unreadable or the pid vanished (`internal/stat/procpidstat.go:350,351,360,386,387,404,411`); `validPID` is evaluated per row, so `""` and numbers genuinely mix in one snapshot | numeric / duration |

Screens with **no** sparse columns at all: tables, indexes, sizes, functions, and every
`statements_*` (all counters `coalesce(...,0)` or NOT NULL in the catalog).

#### Highest-risk single screen: `replslots`

`internal/view/view.go:159` sets `OrderKey: 4` → `retained,KiB`, which is **the only default
sort key in the repo that is both sparse and numeric**. Every other view's default `OrderKey`
points at a never-empty column (`pid`, `relation`, `io_key`, …) or at a sparse *text* column
(`databases_general`/`databases_sessions` `datname`, col 0 — the `pg_stat_database`
shared-objects row has `datname IS NULL`; mode is unchanged, and under the default DESC the
empty is already last).

Supporting argument for the tech-spec: the replslots SQL **already declares the intended
semantics** — `ORDER BY "retained,KiB" DESC NULLS LAST` (`replication_slots.go:31`). Today
`sort()` re-sorts in Go and, under ASC, hoists the NULL slots to the top, contradicting the
query's own `NULLS LAST`. The new rule makes Go agree with the SQL. This is the cleanest
framing of "исправление, а не регрессия".

#### Correction to a pass-1 / common assumption

Being **inside** `DiffIntvl` does *not* guarantee a non-empty cell:

- `internal/stat/postgres.go:582–584` — `if !prev.Valid { return curr, nil }`: on the first
  sample and after every view switch `diff()` is skipped entirely and raw SQL output is sorted.
- `internal/stat/postgres.go:650–655` — the `if !found` branch copies a row that exists in
  `curr` but not in `prev` (a new backend / new slot / new relation) **verbatim, including its
  in-interval columns**.

So "diffed ⇒ numeric string" holds only for rows matched by `UniqueKey` on a second-or-later
sample.

#### Edge case the tech-spec must specify explicitly

When a column is empty in **every** row, the "first non-empty cell" scan finds nothing and
`sample` stays `""` → the string arm → the comparator is a no-op. This is not hypothetical:
`replslots.safe,KiB` is NULL for all rows on a default cluster, and `activity.cl_addr` is
empty for all rows on a purely local cluster. Behaviour is correct (nothing to order) but it
must be a stated decision, not an accident.

#### Open decision: does empty-last apply to the string arm too?

The prototype applies `withEmptyLast` uniformly, including the string comparator. That changes
a common operation: sorting `activity` ascending by `wait_event` puts blanks last instead of
first. The spec's UX rule is written in the context of the sparse **numeric** column
("Сортировка по разреженной числовой колонке…"), so the string arm is not explicitly
mandated. Options: (a) uniform, one rule, blanks are never a value — recommended, and it is
what makes `databases_general --order datname` ASC behave sanely; (b) restrict `withEmptyLast`
to the numeric and duration arms, minimising the diff. **No golden is affected either way**
(measured, §2.3), so this is a product call, not a test-cost call.

## 3. Tech debt [021] — measured, not inferred

### 3.1 The panic is real **[measured]**

`report/report.go:564–571`:

```go
for i := range res.Cols {                       // recorded column count (17)
    valuelen := len(res.Values[rownum][colnum].String)
    if valuelen > view.ColsWidth[i] {           // missing key -> 0, so true for any non-empty value
        width := view.ColsWidth[i]              // 0
        res.Values[rownum][colnum].String = ...[:width-1] + "~"   // [:-1]
```

`top/printDataCell` (`top/stat.go:1000–1011`) **does** guard this — `if width <= 0 { return
fmt.Errorf("zero or negative width, skip") }` at `:1005–1007`. `report` has no such guard.
The asymmetry is confirmed.

Built a synthetic two-version archive (PG 12 → PG 13, 14 → 17 columns) and ran it through the
current, unpatched code:

```
panic: runtime error: slice bounds out of range [:-1]
	report.printStatSample  report/report.go:570
	report.processData      report/report.go:320
	report.(*app).doReport.func2  report/report.go:128
```

Two things the tech-spec must know:

1. It is a **panic, not an error return** — `doReport` cannot report it.
2. It happens on a **goroutine spawned at `report/report.go:127`**, so a test cannot recover
   it. The TDD red step is a hard process crash that takes the whole `go test` binary down,
   not a failed assertion.

### 3.2 The fix — two lines, and the second one is not optional

`Views` is `map[string]View` (`internal/view/view.go:35`) — a **value** map — and `Configure`
(`view.go:367–427`) never touches `Aligned`. So the round-trip at `report/report.go:255–265`
preserves `Aligned = true` from the previous layout. The fix goes right after
`v = views[config.ReportType]` (`report.go:265`), inside the branch that already `continue`s:

```go
				v = views[config.ReportType]
				v.Aligned = false
				linesPrinted = repeatHeaderAfter

				continue
```

`v.Aligned = false` alone fixes the widths but **leaves a stale header**: `printStatHeader`
(`report.go:508–511`) only prints when `printedNum >= repeatHeaderAfter`, and `linesPrinted`
is well under 20 mid-report, so the 17-column rows would print under the 14-column header for
up to 20 more lines. The AC says "нет ни устаревшей раскладки" — a header naming the wrong
columns is a stale layout. Resetting `linesPrinted` forces the header to reprint on the first
sample of the new layout.

**[measured]** with both lines, the same synthetic archive produces a clean report:

```
pid  cl_addr  cl_port  datname … xact_age  query_age  change_age  query      <- 14-col header
2026/05/19 10:00:01, rate: 1s
12345  127.0.0.1  5432  db  postgres  psql  client backend … 00:00:10 …
pid  leader  cl_addr  cl_port … state  backend_xid  horizon_xacts  xact_age …   <- 17-col header
2026/05/19 10:00:03, rate: 1s
12345  12345  127.0.0.1  5432  db  postgres  psql  client backend … 748291  2300000 …
```

**[measured]** `go test -count=1 ./report/...` → `ok` with the patch applied: **no golden
churn** across all ~30 goldens. (Both patches were reverted; the tree is clean.)

### 3.3 "The replay path skips the first sample of a new version" — confirmed

`report/report.go:250–268`. The condition is `if !prevStat.Valid || prevMeta.version !=
d.meta.version`; the body reassigns `prevStat`/`prevTs`, reconfigures the view, and
**`continue`s at :267** — the sample is never printed.

**[measured]**: the 4-tick archive above (2 ticks at PG 12, 2 at PG 13) printed exactly **2**
data rows — tick 0 consumed as the initial sample, tick 1 printed, tick 2 consumed by the
version change, tick 3 printed. A test archive therefore needs **≥ 2 samples after the version
change**, exactly as the spec's AC warns; with one, the report is empty and the test is
vacuously green.

### 3.4 Cost of the two-version synthetic archive — the AC's conditional is satisfied

The spec makes this AC conditional on the archive being cheap to build. **It is.** The probe
above is a working implementation: **~85 lines**, one file, no golden needed if sentinel
assertions are used (or one golden if the 012 style is followed). It is the
`report_record_progress_vacuum_test.go` harness (173 lines, 2 cases) with the diff machinery
removed and one extra tick-pair added:

- two `meta.*` payloads differing only in `version_num` (`120020` / `130016`);
- two `PGresult` payloads, 14 cols and 17 cols;
- 4 ticks × 3 entries (`meta.` / `activity.` / `sysinfo.`) = 12 tar entries, ticks exactly
  1 s apart;
- `newApp(config)` → `app.writer = &buf` → `app.doReport(tar.NewReader(&tarBuf))`.

Two hard constraints carried over from the original harness: the stat entry's basename **must**
equal `config.ReportType` (`isFilenameOK`, `report.go:408–424`, otherwise entries are skipped
silently and the test passes on an empty report), and ticks exactly 1 s apart so `itv == 1`.
A third, specific to this test: assert **two** timestamp lines / that the 17-column header
appears, so a one-sample-after-switch mistake cannot pass.

**Recommendation: keep the AC unconditional.** The cost is ~85 lines and the red step is a
reproducible panic.

## 4. Test work — exact patterns to copy

### 4.1 `internal/query/activity_test.go:10–26` — extend the table

Current table covers `90500 / 90600 / 100000` only. It **passes unchanged** (100000 still maps
to `PgStatActivityDefault`/14) and must be *extended*, not repaired. The spec's "граница
фиксируется с обеих сторон" needs both rows:

```go
		{version: 120000, wantQ: PgStatActivityDefault, wantN: 14},
		{version: 130000, wantQ: PgStatActivityPG13, wantN: 17},
```

Add `140000` / `190000` too if the 012 style is followed (`progress_vacuum_test.go:46–48`
pins one version on each side plus the newest).

### 4.2 `internal/query/activity_test.go:28–50` — upgrade to column-name+order verification

Today (`:33`) the returned Ncols is discarded (`tmpl, _ := …`) and (`:44`) the query is run via
`conn.Exec(q)`, which throws the result away — the test proves only "does not error".

The house pattern for asserting **count** is `bgwriter_test.go:53–61` and
`progress_vacuum_test.go:26–32`:

```go
		rows, err := conn.Query(q)
		assert.NoError(t, err)
		assert.Len(t, rows.FieldDescriptions(), wantNcols)
		rows.Close()
```

The spec asks for more than the count — **names and order**. There is no existing precedent for
that; it is a new (cheap) step on top:

```go
			tmpl, wantNcols := SelectStatActivityQuery(version)
			…
			rows, err := conn.Query(q)
			assert.NoError(t, err)

			fds := rows.FieldDescriptions()
			assert.Len(t, fds, wantNcols)
			got := make([]string, len(fds))
			for i, fd := range fds {
				got[i] = string(fd.Name)
			}
			assert.Equal(t, wantCols, got)   // per-version expected slice, in emitted order
			rows.Close()
			assert.NoError(t, rows.Err())
```

`wantCols` becomes a per-version field on the test-case struct; the existing loop already wraps
each version in `t.Run` (`activity_test.go:32`) and skips inside the subtest (`:40`), which is
the shape debt [019] wants — no change needed there.

`fd.Name` is `[]byte` on pgx v5 in some versions and `string` in others; `string(fd.Name)` is
safe either way.

**Availability caveat, load-bearing for the spec's honesty:** `internal/postgres/testing.go:19–35`
maps 130000→21913 and 120000→21912, but the `pgcenter-testing` image ships **PG 14–19 only**
(the map's own comment marks 90400–130000 as "EOL versions kept for reference"). So the live
name/order check runs on **PG 14–19**; on PG 12/13 `NewTestConnectVersion` returns an error and
the subtest **skips** — green, but proving nothing. That gap is exactly the third tech-debt
entry the spec's AC asks to register.

### 4.3 `report/report_test.go:1216–1255` — `Test_describeProgressColumnOrder`

Its shape (quoted, `:1242–1254`):

```go
	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			prev := -1
			for _, m := range tc.markers {
				pos := strings.Index(tc.text, m)
				// Presence first: strings.Index returns -1 for a missing marker, and -1 is less than
				// anything, so an ordering-only assertion would pass on a row that is not there at all.
				require.NotEqual(t, -1, pos, "description must contain a row for %q", strings.TrimPrefix(m, "\n- "))
				assert.Greater(t, pos, prev, "row %q is out of order", strings.TrimPrefix(m, "\n- "))
				prev = pos
			}
		})
	}
```

Cases are `{name, text, markers []string}` (`:1220–1240`). The activity analogue is one more
case — either appended to this table (the name would then be inaccurate) or in a sibling
`Test_describeActivityColumnOrder` with the identical body:

```go
	{
		name:    "activity",
		text:    pgStatActivityDescription,
		markers: []string{"\n- pid", "\n- leader", "\n- state", "\n- backend_xid", "\n- horizon_xacts", "\n- xact_age", "\n- query"},
	},
```

The `require.NotEqual(t, -1, pos)` presence check before the ordering compare is the
load-bearing part — without it a missing row reads as correctly ordered. `strings` and
`require` are already imported in the file.

Note why this test exists at all: `Test_describeReport` (`report_test.go:~1180–1214`) compares
each block **against the constant itself** (`assert.Equal(t, tc.want, buf.String())` at `:1211`,
where `tc.want` *is* `pgStatActivityDescription`), so it is structurally blind to a row in the
wrong slot and cannot break when the constant is edited.

### 4.4 Sort tests

- **New** `Test_sort_sparse` in `internal/stat/postgres_test.go`, next to `Test_sort_duration`
  (`:782`). Must cover: numeric order independent of which row is first (same data, empty row
  at index 0 and in the middle); blanks last in **both** directions; a genuine `"0"` ranked
  with the numbers, above the blanks; a sparse duration column; an all-empty column.
  ⚠️ `Test_sort` (`:704`) shares one `res := newTestPGresult()` across subtests and mutates it
  in place (`:705`, `:768`) — a new test must build its own fixture per case.
- **Recommended golden** in `report/report_record_replslots_test.go` (`:49`, `:146` already pin
  `retained,KiB` DESC via the view's own `OrderKey: 4`): add a slot row with empty
  `retained,KiB` (a slot that reserved no WAL) and an empty `stats_age` on the physical slot,
  in both directions. This is the only place where the changed behaviour meets a default sort
  key, and the existing golden corpus has **zero** coverage of it.

### 4.5 Existing golden corpus — no regeneration, and no coverage either

**[measured]** every `report/` golden passes unchanged under both patches. The reason is that
the only sparse sort key in the corpus is `databases_general`/`databases_sessions` `datname`
(the `pg_stat_database` shared-objects row, empty in 9 rows of each golden) sorted **DESC** —
the one direction where old and new agree. All four `report_activity*.golden` files order by
`pid`, never empty, even though they do contain empty `cl_addr`, `wait_etype`, `wait_event`,
`datname` and `xact_age` cells.

Corollary for the spec: "goldens unchanged" is **not** evidence that the sort change is safe —
it is evidence that the corpus never exercised it. New tests are required, not fixups.

### 4.6 A latent flaw in an existing test, worth not building on

`internal/stat/postgres_test.go:504–506`: `calculateDelta(curr, prev, 1, [2]int{0,0}, 1, true, 0)`
takes the `delta = curr` branch (`postgres.go:596`), which **aliases** `curr.Values`;
`delta.sort()` then reorders `curr` in place, so `assert.Equal(t, curr, got)` at `:506` is
vacuously true. Not caused by this feature and not in scope to fix, but no new assertion should
lean on it.

## 5. Documentation surfaces

### 5.1 `report/describe.go:179–199` — `pgStatActivityDescription`

Structure: a title line (`:180`), a blank line, a `  column\torigin\t\t\tdescription` header
(`:182`), 14 rows in **emitted column order** (`:183–196`), a blank line, then
`Details: https://…` (`:198`).

Insertion points, in emitted order:

- `- leader` immediately **after** `- pid` (`:183`), before `- cl_addr` (`:184`);
- `- backend_xid` and `- horizon_xacts` immediately **after** `- state` (`:192`), before
  `- xact_age` (`:193`).

Tab layout (tab stops of 8; origin column starts at 16, description at 40):

```
- leader<TAB>leader_pid,pid<TAB><TAB>...
- backend_xid<TAB>backend_xid<TAB><TAB>...
- horizon_xacts<TAB>backend_xmin<TAB><TAB>...
```

(`- leader` is 8 chars → one tab reaches 16; `- backend_xid` 13 and `- horizon_xacts` 15 →
one tab each. Origins `leader_pid,pid` 14, `backend_xid` 11, `backend_xmin` 12 → two tabs each
to reach 40. Compare the existing `- appname\tapplication_name\t` — a 16-char origin needs only
one tab.)

The **three caveats** required by the AC go after the table and before `Details:`, in the slot
where 012 put `Note: started_by and mode are available since PG19.` (`describe.go:221`). Four
notes in total — the version note plus the three caveats:

1. availability — the three columns exist since PG 13;
2. `leader` is `coalesce(leader_pid, pid)`, i.e. the leader's own pid rather than the raw
   `leader_pid` (which is empty on the leader itself);
3. the horizon is shown for backends only — replication slots, prepared transactions and
   standby feedback also hold it and are not in `pg_stat_activity`;
4. this `horizon_xacts` is `age(backend_xmin)`, computed differently from the identically
   named column on the `replication` screen (`describe.go:75`, backed by
   `replication.go:28` = `(pg_last_committed_xact()).xid - backend_xmin`). The two differ by
   the number of assigned-but-uncommitted transactions; the replication one can go negative,
   `age()` cannot.

Caveat 4 is also the content of the tech-debt entry the AC asks for. Consider a matching
one-liner in the `replication` block (`describe.go:75`) so the divergence is discoverable from
either screen.

`Test_describeReport` (`report_test.go:1211`) compares the block against the constant itself,
so editing the constant **cannot** break it — which is precisely why §4.3's order test is
required.

### 5.2 `internal/stat/help.go` is dead — re-confirmed, correctly excluded

`grep -rn "PgStatActivityDescription"` over the whole repo returns **one** hit: the declaration
at `internal/stat/help.go:145`. No consumer exists for it or for any other `PgStat*Description`
in that file; `top/help.go` renders its own keybinding cheat-sheet and does not import them.

The file has already drifted: `internal/stat/help.go:59–60` still calls the replication horizon
columns `xact_age*` / `time_age*`, while the live query (`internal/query/replication.go:28–29`)
and `report/describe.go:75–76` both say `horizon_xacts` / `horizon_age`.

→ Pass 1's correction stands: the brief that cited `internal/stat/help.go:59` as the precedent
for the name `horizon_xacts` cited the wrong line — that line says `xact_age*`. The real
precedents are `internal/query/replication.go:28` and `report/describe.go:75`. The spec's
decision on the *name* is unaffected.

The spec's choice — leave the file alone, register it as tech debt — is correct and needs no
code change.

### 5.3 `top/help.go`

No change. No new hotkey; the `I` / `A` filter keys are documented at `:32–37` and their
behaviour is unchanged (the new query carries both template placeholders verbatim).

## 6. Things that contradict, or are not covered by, the spec

Flagged rather than worked around.

1. **The brief's "both xid columns need explicit `::text`" is wrong for `horizon_xacts`.**
   `age(backend_xmin)` returns `integer`, measured (§1.4). Only `backend_xid` is of type `xid`
   and needs the cast. Not a spec statement — the spec does not mention casts — so nothing in
   the approved document changes.

2. **The spec's "три новые колонки добавляют 32 символа" undercounts.** 8 + 11 + 13 = 32 is the
   sum of the column *widths*; every column is printed with a two-character gap
   (`report.go:575` `%-*s` with `ColsWidth[i]+2`; `top/stat.go:1014` likewise), so the actual
   shift of `query` is **38** characters. This does not change any decision — the spec's
   conclusion ("no separate mitigation needed, horizontal scroll covers it") holds — but the
   number in the rationale is off by six.

3. **The [021] fix needs a second line the spec does not anticipate.** `v.Aligned = false`
   alone leaves the previous layout's *header* on screen for up to 20 lines (§3.2). Meeting
   the AC's "нет устаревшей раскладки" requires `linesPrinted = repeatHeaderAfter` as well.
   This is an addition to the debt register's one-line description, not a contradiction of the
   spec.

4. **Risk 5 understates the sort blast radius.** It describes only the mode flip
   (string → duration). The larger change is placement: empty cells are ordered *as zero*
   today in every ★ column whose first row is non-empty, so **ASC sorting moves blanks from
   first to last on every screen in the ★ table**, and DESC moves them past any negative value
   (`activity.cl_port = -1`). Still "исправление, а не регрессия" — `replication_slots.go:31`
   already writes `ORDER BY "retained,KiB" DESC NULLS LAST`, so the fix makes Go agree with the
   SQL — but the tech-spec's blast-radius section should say so in these terms.

5. **Undecided: does empty-last apply to the string comparator?** (§2.5). The spec's UX rule is
   phrased about the sparse *numeric* column. Uniform application changes ordinary text sorts
   (`wait_event` ASC). No test or golden distinguishes the two options, so it must be decided
   deliberately.

6. **Undecided: the all-empty column.** No non-empty sample exists → string arm → no-op.
   Correct, but it should be a stated decision; it is the normal state of `replslots.safe,KiB`
   on a default cluster and of `activity.cl_addr` on a local-only cluster.

7. **Not a contradiction, but worth recording in the describe block:** parallel workers of a
   *writing* transaction show an **empty `backend_xid`** — the xid belongs to the leader
   (§1.5, measured). A DBA sorting by `backend_xid` to find writers sees leaders only. The
   spec's Scenario 3 is unaffected (it starts from the horizon holder), but the "пусто =
   ничего не писала" reading is imprecise for a worker row.

8. **The live name/order check cannot cover the new branch boundary.** The port map has entries
   for PG 12 (21912) and PG 13 (21913), but `pgcenter-testing` ships PG 14–19, so those
   subtests skip. The spec already states this as a known limitation and asks for a tech-debt
   entry; §4.2 confirms it from the code.

## 7. Files to change — revised checklist

| File | Change | Required |
|---|---|---|
| `internal/query/activity.go` | add `PgStatActivityPG13` (17 cols, doc comment per §1.3); insert `case version < 130000: return PgStatActivityDefault, 14`; new const becomes `default`. Arity unchanged | **yes** |
| `internal/query/activity_test.go:10–26` | extend table with `120000`→Default/14 and `130000`→PG13/17 | **yes** |
| `internal/query/activity_test.go:28–50` | `Exec` → `Query` + assert `FieldDescriptions()` names **and** order per version (§4.2) | **yes** |
| `internal/stat/postgres.go:663–701` | rewrite `sort` per §2.2; add `isParsableFloat` / `isParsableDuration` helpers | **yes** |
| `internal/stat/postgres_test.go` | new `Test_sort_sparse` (§4.4) | **yes** |
| `report/report.go:265` | `v.Aligned = false` + `linesPrinted = repeatHeaderAfter` (§3.2) | **yes** |
| `report/report_record_activity_version_switch_test.go` | **new** — two-version synthetic archive, 4 ticks (§3.4), ~85 lines | **yes** |
| `report/describe.go:179–199` | 3 rows in emitted order + 4 notes (§5.1) | **yes** |
| `report/report_test.go:1216+` | activity case in the column-order test (§4.3) | **yes** |
| `report/report_record_replslots_test.go` | empty `retained,KiB` / `stats_age` rows, both directions (§4.4) | recommended |
| `internal/view/view.go` | **none** — `Configure` at `:374–376` already wires it; `New()` seed stays 14 (§1.7) | no |
| `internal/view/view_test.go` | add `case 130000:` / `case 120000:` to `TestViews_Configure`, mirroring the 012 block at `view_test.go:186–202` | **yes** |
| `top/config_view_test.go:18` | correct the stale `Ncols == 13` comment (means `Ncols - 1`) | cosmetic |
| `docs/tech-debt.md` | 3 new entries (dead `internal/stat/help.go`; `horizon_xacts` formula divergence across two screens; port map vs. test-image coverage below PG 14); update [021] to "resolved" | **yes** |
| `docs/decisions-log.md`, features catalog, PK `patterns.md` | per `/done` | **yes** |
