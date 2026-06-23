# Code Research — 010-feat-overview-dashboard

A new full-screen, **card-oriented** TUI screen in `pgcenter top`, opened by hotkey `v`, aggregating
overall-instance metrics from many sources with per-metric `n/a` degradation. TUI-only in v1
(`NotRecordable`). Card layout differs from the row-oriented stats tables.

This document answers the 8 research questions that gate the central architectural decision:
**reuse the existing free-form-text render path, or build a new card render path.**

All paths absolute. Everything under `.claude/worktrees/` is stale and was ignored.

---

## 0. TL;DR — answers to the gating questions

1. **Hotkey `v` is FREE.** Full bound-key inventory below; `v`, `V`, `c`, `e`, `g`, `M`, `O`, `T`, `W`, `Y`, `Z` are unbound on `sysstat`.
2. **A new view is registered by adding an entry to `view.New()`** (`internal/view/view.go:37`) and a keybinding. Adding it breaks 2-3 view-count test assertions (listed in §2 / §5-tests).
3. **A free-form-text render path ALREADY EXISTS and is the dashboard's natural home.** The top summary block (`pgstat`/`sysstat` gocui views) and the full-screen `help` view are both plain `fmt.Fprintf`-into-a-`Frame=false`-gocui-view blocks — exactly "cards of label:value text." The dashboard should be a **new full-screen gocui view + a new render function in the same idiom** (it cannot reuse the `dbstat` table renderer, which is column/`PGresult`-bound). Detail in §3.
4. **System stats (CPU/mem/load) are already collected EVERY tick** by `Collector.Update`, with a local(/proc) vs remote(PL/Perl `pgcenter` schema) branch already wired (§4). The dashboard reuses this verbatim.
5. **The collection seam** is `Collector.Update` (`internal/stat/stat.go:122`), driven by `collectStat`'s single ticker (`top/stat.go:79`). A secondary/slower rhythm + latency backoff can live entirely inside an overview-specific collect function gated by tick-count/elapsed-time — no second user knob (§5).
6. **Roughly half the dashboard metrics need NEW aggregate SQL**; the other half reuse existing single-row queries or liftable column expressions. Per-metric table in §6.
7. **Probe→n/a pattern**: `CheckIOAvailable`/`CheckDelayAcctAvailable` + the empty-string-`NullString` → blank-cell passthrough in `procpidstat.go` (§7).
8. **Constraints**: `internal/align` floors width at 8 — but it only governs the **table** path, so a free-form card render is unaffected; the render loop's single-table assumption and gocui geometry are the real constraints (§8).

---

## 1. Hotkey `v` availability — Entry Points

**File:** `/home/lesovsky/Git/github.com/lesovsky/pgcenter/top/keybindings.go`
Keys are bound in `keybindings(app *app)` (`keybindings.go:17`) as a `[]key` slice, then registered via
`app.ui.SetKeybinding(viewname, key, gocui.ModNone, handler)` (`keybindings.go:84`).

**All keys bound on the `sysstat` view** (the main screen — this is where `v` would live):

| lowercase | bound to | | uppercase / sym | bound to |
|---|---|---|---|---|
| `a` | activity | | `A` | change age dialog |
| `b` | bgwriter | | `B` | diskstat extra |
| `d` | databases switch | | `C` | show config |
| `f` | functions | | `D` | databases menu |
| `h` | help | | `E` | edit configs menu |
| `i` | indexes | | `F` | fsstats extra |
| `j` | statio switch | | `G` | query report dialog |
| `k` | cancel group dialog | | `I` | idle conns toggle |
| `l` | show pg log | | `J` | statio menu |
| `m` | show proc mask | | `K` | terminate group dialog |
| `n` | set mask dialog | | `L` | logtail extra |
| `o` | replslots | | `N` | nicstat extra |
| `p` | progress switch | | `P` | progress menu |
| `q` | quit (on sysstat) | | `Q` | reset stat |
| `r` | replication | | `R` | reload dialog |
| `s` | sizes | | `S` | procpidstat (Shift+S) |
| `t` | tables | | `X` | pgss menu |
| `w` | wal | | | |
| `x` | statements switch | | | |
| `z` | refresh interval dialog | | | |

Symbols bound on `sysstat`: `[` `]` (scroll), `<` (sort dir), `,` (sys tables), `/` `-` `_` `~`.
Global: `Ctrl+C`, `Ctrl+Q` (quit). Other view-scopes: `dialog`, `menu`, `help`.

**Conclusion: `v` (lowercase) is FREE** on `sysstat`. Also free: `c e g u y` lowercase; `M O T U V W Y Z` uppercase; `.` `;` `:` `!` `?` etc. `v` for "**v**iew/o**v**erview" is unambiguous. Bind as
`{"sysstat", 'v', switchViewTo(app, "overview")}` (or a dedicated handler — see §3/§5 on why it may need
a procpidstat-style dedicated handler if it patches runtime fields).

Help text lives in `top/help.go:10` (`helpTemplate` const) and needs a one-line entry for `v`.

---

## 2. Screen/view registration & switching

**`view.View` struct** — `/home/lesovsky/Git/github.com/lesovsky/pgcenter/internal/view/view.go:10-31`.
Key fields a dashboard view would set: `Name`, `MinRequiredVersion`, `QueryTmpl`/`Query` (could be empty
if the dashboard does not flow through the table render — see §3), `Ncols`, `ColsWidth`, `Msg`,
`CollectExtra int` (the enrichment hook, ADR [001]), `NotRecordable bool` (set **true** for v1, ADR [004]/[008]).

**`view.New() Views`** — `view.go:37-360` — a `map[string]View` of 27 predefined views. **Add an `"overview"`
entry here.**

**`Configure(opts query.Options)`** — `view.go:366-417` — version-dispatches `QueryTmpl`/`Ncols`/`DiffIntvl`
per view (a `switch` over view names, `view.go:372-403`), then `Format`s every view's `Query` from its
template (`view.go:407-414`). If the dashboard has its own SQL, add a `case "overview"` here; if it
collects via a bespoke `Collector` path (likely), it needs no `Configure` case but **must still have a
non-empty `Query` OR be excluded from the recordable loop** — see the `Test_app_record` caveat in §5-tests.

**Default view** is set in `app.setup()`: `app.config.view = app.config.views["activity"]`
(`top/top.go:69`). Per the interview, **leave this as `activity`** — overview is not the landing screen.

**View-switch flow:**
- `switchViewTo(app, c string)` — `top/config_view.go:134` — the generic handler bound to most keys; for a
  plain screen it calls `viewSwitchHandler(app.config, c)` (the `default` branch, `config_view.go:152`).
- `viewSwitchHandler(config, c)` — `config_view.go:240-245` — persists current view into `config.views`,
  loads the target, **resets `config.scrollOffset = 0`** (ADR [009]), and pushes onto `config.viewCh`.
- `switchViewToProcPidStat(app)` — `config_view.go:253-303` — the **precedent for a screen that patches
  runtime-only fields** (`CollectExtra`, `IOAvailable`, `DelayAcctAvailable`) onto the view copy and sends
  it directly, **deliberately bypassing** `viewSwitchHandler` (which would reload from the static map and
  discard patches — comment at `config_view.go:251-252`). **If the dashboard needs runtime probe flags
  (likely, for per-metric n/a), copy this dedicated-handler pattern, not the generic `switchViewTo`.**

**How `viewCh` drives rendering:** `top/ui.go:77` starts `collectStat(ctx, db, statCh, config.viewCh)`.
The collector blocks on `viewCh` for the active view (`top/stat.go:33,81`), collects, sends `Stat` on
`statCh`; `doWork` receives it and calls `printStat(app, s, props)` (`top/ui.go:94`), which fans the one
`Stat` out into the gocui views (`top/stat.go:126`). A handler triggers a redraw simply by sending the
(possibly unchanged) view on `viewCh` (e.g. `scrollLeft`, `config_view.go:55`).

**Test coupling to view count** — see §5-tests for exact assertions and line numbers.

---

## 3. Rendering paths — THE KEY QUESTION

### gocui layout setup
**File:** `/home/lesovsky/Git/github.com/lesovsky/pgcenter/top/ui.go`, function `layout(app)` (`ui.go:103-184`),
installed via `app.ui.SetManagerFunc(layout(app))` (`ui.go:27`). It creates 4 always-present gocui views by
geometry, each with `v.Frame = false`:

| gocui view | geometry (`ui.go`) | content |
|---|---|---|
| `sysstat` | `SetView("sysstat", -1, -1, (maxX-1)/2, 4)` `:114` | system summary (load/cpu/mem/swap), 4 lines, left half of top band |
| `pgstat` | `SetView("pgstat", maxX/2, -1, maxX, 4)` `:128` | postgres summary (conn info, activity, autovac, statements), 4 lines, right half |
| `cmdline` | `SetView("cmdline", -1, 3, maxX, 5)` `:139` | command line / messages |
| `dbstat` | `SetView("dbstat", -1, 4, maxX, maxY-1)` `:155` | **the main stats TABLE**, full width, rest of screen |
| `extra` | `SetView("extra", -1, 3*maxY/5-1, maxX, maxY-1)` `:167` (conditional) | side panel (iostat/nicstat/fsstats/logtail), only when `ShowExtra > CollectNone` |

### The main stats TABLE render (NOT reusable for cards)
`printDbstat` (`top/stat.go:324`) → `renderDbstat(w io.Writer, config, s, termWidth)` (`top/stat.go:349`) →
`printStatHeader` / `printStatData` (`stat.go:537` / `:606`). This path is **fundamentally column-oriented**:
it consumes `s.Result` (a `PGresult` with `Cols`/`Values`), aligns via `config.view.ColsWidth`, and renders
a windowed grid with frozen-column horizontal scroll. **A card layout (free-form label:value groups) cannot
use this path** — there is no `PGresult` grid to render. Note the writer-based split (`renderDbstat` takes
`io.Writer`) is the 009 testing pattern the dashboard should mirror for its own renderer.

### The top SUMMARY/HEADER block — the closest existing "card"
**This is the existing free-form-text render path.** It is the strongest evidence that the dashboard needs
**no new render machinery beyond a new gocui view + a new printer function in the same idiom.**

- `printSysstat(v *gocui.View, s stat.Stat)` — `top/stat.go:214-250`. Four `fmt.Fprintf` lines into the
  `sysstat` gocui view: line1 time+loadavg, line2 `%cpu` breakdown, line3 MiB mem, line4 MiB swap. Pure
  text with ANSI escapes (`\033[37;1m...\033[0m`) for emphasis. **No `PGresult`, no `ColsWidth`, no align.**
- `printPgstat(v *gocui.View, s stat.Stat, props, db)` — `top/stat.go:253-285`. Four `fmt.Fprintf` lines:
  line1 conninfo string (`formatInfoString`, `stat.go:288`: `state [..]: host:port user@db (ver.., up.., recovery..)`),
  line2 `activity: N/M conns, ... idle, ... active, ... waiting`, line3 autovacuum, line4 statements.
  Reads scalars from `s.Activity.*` and `props.*`. **This is literally a "card" of label:value cells
  rendered as free text.**
- Both are dispatched from `printStat` (`top/stat.go:126-211`): `g.View("sysstat")` → `v.Clear()` →
  `printSysstat(v, s)`; same for `pgstat`. The pattern is: **fetch the named gocui view, clear, Fprintf lines.**

### The side panels (extra views) — also free-form / tabular text
`printIostat` (`stat.go:697`), `printNetdev` (`stat.go:725`), `printFsstats` (`stat.go:754`),
`printLogtail` (`stat.go:804`). Each is a header line + `fmt.Fprintf` rows into the `extra` gocui view
(`top/stat.go:159-208` dispatch). They are tabular-by-hand (fixed `%10.2f`-style column formats), **not**
the `ColsWidth`/`PGresult` table machinery — i.e. another free-form-Fprintf path. The full-screen `help`
view (`top/help.go:50` `showHelp` → `g.SetView("help", ...)` + `Frame=false` + `fmt.Fprintf(v, helpTemplate, ...)`)
is the precedent for a **full-screen** free-form overlay created on demand.

### Conclusion (the architectural decision input)
**A free-form-text render path already exists and is the right basis.** The dashboard render function should:
1. Create a new full-screen gocui view (e.g. `"overview"`) in `layout()` — geometry like `dbstat`
   (`-1, 4, maxX, maxY-1`) or full-screen like `help` — with `v.Frame = false`. (Decision point: a
   persistent layout view like `dbstat` vs an on-demand overlay like `help`. The `dbstat`-style persistent
   view fits the per-tick `printStat` fan-out better; the dashboard would then be shown/hidden by clearing
   it or by gating which view is "current".)
2. Add a `printOverview(w io.Writer, s stat.Stat, props ...)` printer in the `printSysstat`/`printPgstat`
   idiom (plain `Fprintf` of label:value cells, ANSI emphasis), taking an `io.Writer` so it is unit-testable
   without a live terminal (009 pattern). Card placement by terminal width/height becomes a **pure layout
   function** (interview testing strategy), separate from the `Fprintf`.
3. Wire it into `printStat` (`top/stat.go:126`) — but note: `printStat` currently renders **all** of
   sysstat+pgstat+dbstat **every tick** regardless of active view (it always writes the table). The
   dashboard either coexists as another always-rendered region or the fan-out gains a branch on
   `app.config.view.Name == "overview"`. **This is the single-table assumption to break — see §8.**

**It must NOT reuse `renderDbstat`/`printStatHeader`/`printStatData`** — those are `PGresult`-bound.

---

## 4. System stats collection (Data / Integration)

**The dashboard's system cards reuse an existing, already-per-tick collection path verbatim.**

**`stat.Stat`** (`internal/stat/stat.go:35-49`) embeds `System` (`LoadAvg, Meminfo, CPUStat, Diskstats,
Netdevs, Fsstats`) and `Pgstat` (`{Activity, Result}`), plus `Error`. This is the channel payload.

**`Collector.Update(db, view, refresh) (Stat, error)`** — `internal/stat/stat.go:122-289`. Always-every-tick
collection (feeds the summary block, and would feed the dashboard's system cards):
1. `readLoadAverage` → `s.LoadAvg` (`stat.go:126`)
2. `readMeminfo` → `s.Meminfo` (`stat.go:134`)
3. `readCPUStat` + `countCPUUsage(prev,curr,ticks)` → `s.CPUStat` (`stat.go:142-149`)
4. one of disk/net/fs **only when `collectExtra` set** (`stat.go:156-175`)
5. `collectActivityStat` → `s.Pgstat.Activity` (`stat.go:190`)
6. per-view SQL `collectPostgresStat(db, view.Query)` → `s.Pgstat.Result` (`stat.go:204`)
7. optional procpidstat enrichment (`stat.go:215`) and `calculateDelta` (`stat.go:281`).

**Struct fields available to the dashboard:**
- `CPUStat` (`cpu.go:16`): `User Nice Sys Idle Iowait Irq Softirq Steal Guest GstNice Total` (float64).
- `Meminfo` (`memstat.go:15`): `MemTotal MemFree MemUsed SwapTotal SwapFree SwapUsed MemCached MemBuffers
  MemDirty MemWriteback MemSlab` (uint64; local values are MB).
- `LoadAvg` (`loadavg.go:15`): `One Five Fifteen` (float64).
- `Diskstats=[]Diskstat`, `Netdevs=[]Netdev`, `Fsstats=[]Fsstat` (only when extra collection enabled).

**Local vs remote decision** — branch is `if db.Local { …Local } else if schemaAvail { …Remote } else {
empty }` in each reader's dispatcher: `readCPUStat` (`cpu.go:32`), `readMeminfo` (`memstat.go:30`),
`readLoadAverage` (`loadavg.go:22`), `readDiskstats` (`diskstats.go:67`), `readNetdevs` (`netdev.go:61`),
`readFsstats` (`fsstat.go:82`). `db.Local` is computed once at connect: `Local: isLocalhost(host)`
(`internal/postgres/postgres.go:101`; `isLocalhost` at `:147` — unix-socket/localhost/loopback/local-iface).
`config.SchemaPgcenterAvail` is set true only when `!db.Local && pgcenter schema exists`
(`internal/stat/postgres.go:145`). **This is exactly the "remote system stats via PL/Perl" differentiator
the feature wants — it already works.**

**Remote PL/Perl source** (`testing/fixtures.sql`): all `sys_proc_*` views over the core
`pgcenter.get_proc_stats(...)` PL/Perl reader (`fixtures.sql:31`); plus `pgcenter.get_sys_clk_ticks()`
(`:23`), `pgcenter.get_netdev_link_settings()` (`:10`), `pgcenter.get_filesystem_stats()` (`:59`).

**What the summary block already shows from system stats:** the `sysstat` block (`printSysstat`,
`stat.go:214`) already renders loadavg, full `%cpu` breakdown, MiB mem, MiB swap+dirty/writeback. The
dashboard can show the same numbers (and the missing IO/disk metrics require enabling
`Diskstats`/`Fsstats` collection, which today is gated behind the mutually-exclusive `collectExtra` switch —
see §8 constraint).

---

## 5. Refresh interval `z` and the collection loop

**`z` hotkey** → `dialogOpen(app, dialogChangeRefresh)` (`keybindings.go:68`) → `changeRefresh(answer, config)`
(`top/config_view.go:438-459`): parses 1-300s, sets `config.view.Refresh`, pushes on `viewCh`, then resets
`Refresh=0` (so it is **not** persisted per-view; it is a collector setting). One single user knob.

**The collect→render tick loop** — `collectStat` (`top/stat.go:25-123`):
- prefills a `prev` snapshot (`Update` at `stat.go:42`), sleeps 100ms (`stat.go:50`).
- main loop: `c.Update(db, v, refresh)` (`stat.go:62`) → send on `statCh` → arm `time.NewTicker(refresh)`
  (`stat.go:79`) → `select` on `{viewCh (apply new view/refresh/extra), ctx.Done, ticker.C}`.
- **The single ticker is the only rhythm.** `Update` collects every source synchronously each tick.

**Introducing a secondary/slower rhythm + latency backoff WITHOUT a second user knob — the seam:**
The clean seam is **inside an overview-specific collect function** (analogous to `collectActivityStat`,
`internal/stat/postgres.go:56`) invoked from `Collector.Update` only when the active view is the dashboard.
That function owns its own cadence state on the `Collector` struct (which already holds prev/curr snapshots,
`stat.go:52-74`):
- **Three rhythms** = gate expensive sub-queries by an internal tick counter / `time.Since(lastRun)`:
  cheap-fresh scalars every tick; WAL-size dear-but-fresh every N ticks; data/db-sizes slow-cached every M
  ticks (or on first open). All driven off the existing `refresh` value passed to `Update` — **no new user
  knob**, just internal divisors and cached last values stored on the `Collector`.
- **Latency backoff** = time each source's `db.QueryRow`; if a source exceeds a budget, increase its
  internal divisor (skip more ticks). Purely internal collector state.

This keeps the user-facing model (`z` sets one interval) intact while the dashboard self-throttles its
expensive sources. The mechanism mirrors how `CallsRate` already computes a rate against `prevCalls` and
`itv` in Go (`postgres.go:94`) and how the procpidstat path keeps prev/curr maps on the Collector.

### 5-tests. Test coupling when adding a new view
When `"overview"` is registered in `view.New()` (27 → 28 views), these break:
- `internal/view/view_test.go:11` — `assert.Equal(t, 27, len(v))` → **bump to 28** (always).
- `internal/view/view_test.go:224-247` (`TestView_VersionOK`) — per-version totals (rows at `:229-234`,
  e.g. `{160000, 27}`, `{140000, 24}`…). **Each row whose `version >= overview.MinRequiredVersion`
  increments by 1.** If overview has no version gate, all six +1.
- `record/record_test.go` `Test_filterViews` (`:101`, testcases `:124-129`, asserts `:134-135`) — shifts
  `wantV` (+1, recordable & version-OK rows) or `wantN` (+1, dropped rows). **Since v1 sets
  `NotRecordable: true`, overview is dropped by `filterViews`** — so the `wantN` (filtered-out count) rows
  increment, not `wantV`. Confirm against the actual `MinRequiredVersion` chosen.
- `record/record_test.go` `Test_app_record` (`:32`) — **NOT hardcoded**; derives count via
  `countRecordable(view.New()) + 2` (`:37`). Auto-adjusts. BUT the `app.setup` loop (`record_test.go:26-27`)
  asserts `v.Query != ""` for **every** view — **an overview view with an empty `Query` would fail this**.
  Either give it a benign non-empty `Query`, or ensure it is excluded before this loop. (`NotRecordable`
  alone does not exempt it from the non-empty-Query assertion if that loop iterates all of `view.New()`.)
- `top/config_view_test.go` `Test_switchViewTo` (`:353`) — only if overview joins a `switchViewTo`/next-view
  cycle. A standalone hotkey screen needs **no** change here.
- `top/menu_test.go` `Test_selectMenuStyle` (`:8`, counts at `:13-19`) — only if overview is added to a menu.
  A standalone hotkey screen needs **no** change.

---

## 6. Metric source SQL — what exists vs new (Data Layer)

The repo splits queries into **per-row table queries** (multi-row, for the scrolling table) and
**single-value aggregate queries** (`internal/query/common.go`, feeding the `Activity` summary). **The
`Activity`/`collectActivityStat`/`SelectActivityDefault` chain is the template to follow**: version-dispatched
single-row scalar queries scanned into a flat struct, rates computed in Go vs a `prev` value and `itv`
(`internal/stat/postgres.go:33-104`, `internal/query/common.go:54-138`).

| Dashboard metric | Status | Source / cite |
|---|---|---|
| sessions by state (total/idle/active/idle_xact/waiting/others/prepared) | **EXISTS aggregate, reuse as-is** | `SelectActivityDefault` `common.go:54` (+ version variants `:65-93`, dispatch `SelectActivityActivityQuery` `:127`); already in `Activity.Conn*` |
| longest running xact / query | **EXISTS aggregate, reuse** | `SelectActivityTimes` `common.go:112` → `Activity.XactMaxTime`/`PrepMaxTime` (text interval; conflates xact+query "max") |
| checkpoint info (timed/req, write/sync ms, buffers) | **EXISTS single-row, reuse cols** | `bgwriter.go:7/19/30` (version split: pre-17 `pg_stat_bgwriter`, 17+ `pg_stat_checkpointer`) |
| tps (commits+rollbacks), tuples ins/upd/del/ret/fetch, cache-hit | **NEW aggregate** (only per-row exists) | per-row `PgStatDatabaseGeneralDefault` `databases.go:5`; need `SELECT sum(xact_commit), sum(...) FROM pg_stat_database` + Go-side rate + hit-ratio `blks_hit/(blks_hit+blks_read)` |
| database time = sum(total_exec_time); time-in-IO = sum(total_io_time) | **NEW aggregate** (only avg + report-CTE exist) | `SelectActivityStatementsLatest` `common.go:120` gives avg only; raw sums only inside report CTE `statements.go:101/164/243` (formatted text, single-stmt). New `SELECT sum(...)` needed; mind PG13/17 column splits |
| replication lag sec + bytes | **NEW aggregate** (only per-standby row exists) | per-row `PgStatReplicationDefault` `replication.go:5`; need `max(...)`/single-value, reuse `{{.WalFunction1/2}}` diff exprs |
| replication slots retained WAL | **NEW aggregate** (per-slot only); expr liftable | `PgStatReplicationSlots` `replication_slots.go:14` (`retained,KiB` expr reusable); need `max`/count |
| dead tuples / vacuum debt | **NEW aggregate** (per-table only) | per-row `n_dead_tup` in `tables.go:5`/`:10`; need `SELECT sum(n_dead_tup) FROM pg_stat_all_tables` |
| WAL growth (current_wal_lsn delta) | **Partial; WAL fns + waldir liftable** | `pg_stat_wal` `wal.go:5` gives `wal,KiB` + `waldir_size`; `selectWalFunctions()` `query.go:68` yields recovery-aware `pg_wal_lsn_diff`/`pg_current_wal_lsn`; trivial new diff query |
| xid age / wraparound (max age(datfrozenxid), autovacuum_freeze_max_age) | **NEW (nothing exists)** | no `datfrozenxid`/`age(`/`autovacuum_freeze_max_age` anywhere; GUC also absent from `SelectCommonProperties` |
| database sizes / data-dir + wal-dir size on disk | **Mostly NEW** | no `pg_database_size` anywhere; per-table only (`sizes.go:5`). waldir size reusable (`wal.go:6` `pg_ls_waldir()` sub-select); data-dir FS size via `pgcenter.get_filesystem_stats` (schema-only) |

**Template helpers** the new queries reuse: `query.Options` (`query.go:25`), `NewOptions` (`:41`),
`selectWalFunctions` (`:68`, recovery+version-aware WAL fn names), `Format(tmpl, opts)` (`:90`).
**Caveat:** `props.GucMaxPrepXacts` (`postgres.go:115`) is declared but never populated by
`SelectCommonProperties` (`common.go:43`); if the dashboard wants `max_prepared_transactions` it's a new GUC read.

---

## 7. Probe → n/a pattern

**Probes** (both in `internal/stat/procpidstat.go`):
- `CheckIOAvailable(pid int) error` — `procpidstat.go:194`. Opens `/proc/[pid]/io`; `nil` on success, OS
  error (e.g. EACCES) on failure. Caller must pass a PID owned by a different user (a PG backend), since
  `/proc/self/io` is always readable (ADR [001]).
- `CheckDelayAcctAvailable() bool` — `procpidstat.go:207`. Reads `/proc/sys/kernel/task_delayacct`; `true`
  only when content is `"1"`.

**The n/a render** is the **empty-string-`NullString` → blank-cell passthrough**, NOT a literal "n/a":
`buildProcPidResultRaw` writes `nullString("")` for unavailable columns (IO totals `procpidstat.go:344-352`,
iodelay `:358-365`, IO rate `:384-387`, %iodelay `:402-404`). The format stage
(`formatProcPidResultForDisplay` `:458`, helpers `formatBytesCell` `:534`, `formatIODelayCell` `:549`)
returns early on `cell.String == ""`, so the column renders **blank**. Contract documented at
`procpidstat.go:231,237-240`: *"missing data is rendered as `0` (CPU/rate) or `""` (IO/iodelay)."*

**For the dashboard**, mirror this: collect each metric independently; on probe-fail / query-fail store a
sentinel and **substitute `n/a` at the format stage** (the interview requires literal `n/a`, not blank — so
substitute `"n/a"` where procpidstat passes `""` through). Per the interview/success-criteria (c), unavailable
metrics must render `n/a`, never `0` or blank — so the format stage should emit the string `n/a`, distinct
from a real `0`.

**Flag flow** probe→render: `view.IOAvailable`/`DelayAcctAvailable` (`view.go:28-29`) are patched onto the
view copy in `switchViewToProcPidStat` (`config_view.go:280-286`) and read inside `Collector.Update`
(`stat.go:252`, `:267-268`) → `BuildProcPidResult`. The dashboard would add analogous capability flags (or
collect availability per-source inside its own collect fn) and pass them to its printer.

---

## 8. Potential Problems & Constraints

- **`internal/align` width-floor of 8 — does NOT affect a free-form card view.** `align.SetAlign` floors
  every column at width 8 (`align.go:36`, and ADR [006]/[009] cite this as why column hiding is impossible).
  This governs **only the `PGresult` table path** (`renderDbstat`). A card renderer that does its own
  `Fprintf` (like `printSysstat`/`printPgstat`) never calls `SetAlign`, so the floor is irrelevant. **This
  is a positive finding** — it removes a constraint people might assume applies.

- **Single-table assumption in the render loop (the real structural constraint).** `printStat`
  (`top/stat.go:126-211`) unconditionally renders `sysstat` + `pgstat` + `dbstat` **every tick**, and
  `dbstat` always expects a `PGresult`. To show a card screen the fan-out must branch on the active view
  (e.g. render `overview` instead of `dbstat` when `config.view.Name == "overview"`), and `layout()`
  (`ui.go:103`) must create the overview gocui view. Today only the `extra` view is conditionally created
  (`ui.go:166`) — the overview view needs the same conditional-create + conditional-render treatment, or it
  coexists. This is the only invasive change to the shared render loop.

- **gocui geometry constraints.** Views are positioned by absolute `SetView(name, x0,y0,x1,y1)` math against
  `ui.Size()`; geometry can be 0 right after a pager/editor (`ui.go:109` guards `maxX==0`). The dashboard's
  pure layout function must clamp card placement to `maxX,maxY` and degrade gracefully on an 80x24 terminal
  (success-criterion a: all cards in one frame, no truncation). gocui truncates overlong lines at the view
  edge (the table path relies on this) — the card printer must wrap/place cards itself rather than rely on it.

- **Mutually-exclusive `collectExtra` switch blocks always-on disk/IO system cards.** Disk/net/fs are only
  collected when one `collectExtra` mode is active (`stat.go:156-175`), and they are mutually exclusive. If
  the dashboard wants IO/disk system metrics **always** (alongside CPU/mem/load), it cannot use the existing
  `collectExtra` gate as-is — it needs its own always-on collection of the disk/fs sources for the dashboard
  view (a new branch in `Update` keyed on the dashboard view), respecting the local/remote dispatch in §4.

- **Collection cost on a struggling instance (interview constraint).** The dashboard pulls from 6+ sources;
  the §5 three-rhythm + latency-backoff seam mitigates this, but every new aggregate query (§6) runs against
  the monitored instance each (gated) tick. Keep the cheap-fresh set minimal; cache sizes/xid-age.

### ADRs that directly affect this feature (settled — do not re-litigate)
- **[004]/[008] `NotRecordable`** — v1 ships `NotRecordable: true` (interview-confirmed). ADR [008] notes no
  production view currently sets it; the dashboard would re-introduce a live user of the field. Recording a
  composite-aggregate screen is deferred (interview).
- **[001] `CollectExtra int` on `view.View`** — the established hook to trigger a bespoke (non-SQL / mixed)
  collection in `Collector.Update` without creating a side panel. The dashboard's bespoke collect path
  should use this hook (a new `CollectOverview` constant) rather than string-coupling on the view name.
- **[009] `scrollOffset` reset on view switch / manual column window** — horizontal scroll is table-only;
  irrelevant to a card view, but `viewSwitchHandler` still resets it harmlessly on entering overview.
- **[006]/[009] `align` floor & no column hiding** — table-only; does not constrain the card renderer (above).
- **[001]/[009] dedicated handler bypassing `viewSwitchHandler`** — copy `switchViewToProcPidStat`'s pattern
  if the dashboard patches runtime probe flags onto the view.

### Tech-debt items in touched areas (from `docs/tech-debt.md`)
- **[005] `top/reload_test.go` panics without live PG** (Low) — environmental; if the feature adds tests in
  `top/` that need PG, follow the rest of the suite's `t.Skipf` guard, not a panic.
- **[008] `record/record_test.go` `Test_app_record` panics without live PG** (Low) — relevant only because
  registering a new view touches the recordable-view count path (§5-tests); the auto-`countRecordable`
  adjustment means no count edit, but the non-empty-`Query` assertion still applies.
- **[009] tar entry size trusted in `NewPGresultFile`** (Low) — only relevant if v1 ever adds record/report
  (deferred), so out of scope.

---

## Key files (quick index)

| Concern | File |
|---|---|
| Hotkey binding | `/home/lesovsky/Git/github.com/lesovsky/pgcenter/top/keybindings.go` |
| View registration / struct / Configure | `/home/lesovsky/Git/github.com/lesovsky/pgcenter/internal/view/view.go` |
| View switch handlers | `/home/lesovsky/Git/github.com/lesovsky/pgcenter/top/config_view.go` |
| gocui layout + collect/render loop | `/home/lesovsky/Git/github.com/lesovsky/pgcenter/top/ui.go` |
| Summary/card render + table render | `/home/lesovsky/Git/github.com/lesovsky/pgcenter/top/stat.go` |
| Full-screen overlay precedent (help) | `/home/lesovsky/Git/github.com/lesovsky/pgcenter/top/help.go` |
| Collector + Stat struct | `/home/lesovsky/Git/github.com/lesovsky/pgcenter/internal/stat/stat.go` |
| Activity aggregate collector + props | `/home/lesovsky/Git/github.com/lesovsky/pgcenter/internal/stat/postgres.go` |
| Single-value aggregate queries | `/home/lesovsky/Git/github.com/lesovsky/pgcenter/internal/query/common.go` |
| Per-row table queries (lift exprs) | `internal/query/{databases,replication,replication_slots,bgwriter,wal,tables,sizes}.go` |
| Probe→n/a pattern | `/home/lesovsky/Git/github.com/lesovsky/pgcenter/internal/stat/procpidstat.go` |
| local/remote + isLocalhost | `/home/lesovsky/Git/github.com/lesovsky/pgcenter/internal/postgres/postgres.go` |
| Remote PL/Perl system stats | `/home/lesovsky/Git/github.com/lesovsky/pgcenter/testing/fixtures.sql` |
| System readers (local/remote) | `internal/stat/{cpu,memstat,loadavg,diskstats,netdev,fsstat}.go` |
| align width-floor (table-only) | `/home/lesovsky/Git/github.com/lesovsky/pgcenter/internal/align/align.go` |
