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
