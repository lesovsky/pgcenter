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

---

## Implementation research — verbose-panel mode (post-pivot)

**Updated: 2026-06-25**

The design pivoted: this is NO LONGER a separate `overview` full-screen view. It is a **persistent
verbose toggle (hotkey `v`)** of the two existing top panels — `sysstat` (left, +3 rows) and `pgstat`
(right, +5 rows). Consequences that invalidate parts of §1-§8 above:

- **No new view is registered.** §2 (view registration), §5-tests (view-count assertions), the
  `Configure`/`NotRecordable`/`filterViews` discussion, and the "branch `printStat` on `view.Name`" idea
  in §3/§8 are all moot. The view-count tests (`view_test.go:11` `assert.Equal(t, 27, …)`,
  `TestView_VersionOK`, `record_test.go:Test_filterViews`) do **NOT** change — confirmed below (§7-new).
- **The render path is the easy part** (extend `printSysstat`/`printPgstat`); **the invasive core is
  `layout()` geometry** (§1-new) and **the collector all-three-at-once branch** (§5-new).
- §3's "free-form Fprintf is the right idiom" conclusion stands and is now the whole render story.

---

### 1-new. `layout()` geometry — THE invasive core

**File:** `/home/lesovsky/Git/github.com/lesovsky/pgcenter/top/ui.go`, `layout(app)` (`ui.go:103-184`).

**Current geometry math (all `SetView(name, x0, y0, x1, y1)`; gocui coords are inclusive, `-1` = one off-screen):**

| view | call | cite | spans |
|---|---|---|---|
| `sysstat` | `SetView("sysstat", -1, -1, (maxX-1)/2, 4)` | `ui.go:114` | left half, rows `y=0..4` (the `4` is the **hard-coded top-band height**) |
| `pgstat` | `SetView("pgstat", maxX/2, -1, maxX, 4)` | `ui.go:128` | right half, rows `y=0..4` |
| `cmdline` | `SetView("cmdline", -1, 3, maxX, 5)` | `ui.go:139` | full width, rows `y=4..5` — **deliberately overlaps** the bottom of the panels (`y=3..4`); drawn after them so it wins the z-order on its rows |
| `dbstat` | `SetView("dbstat", -1, 4, maxX, maxY-1)` | `ui.go:155` | full width, `y=5..maxY-1` (top is **`4`**, immediately below the band) |
| `extra` | `SetView("extra", -1, 3*maxY/5-1, maxX, maxY-1)` | `ui.go:167` (only if `ShowExtra > CollectNone`) | overlays bottom `2/5` of `dbstat` |

**Why `y1=4` is "fixed":** the literal `4` in the `sysstat`/`pgstat` `SetView` calls fixes the band at
**5 printed rows** (`y=0..4`); only 4 are written today (the 5th overlaps `cmdline`). `cmdline` is pinned
at `y=3..5` and `dbstat`'s top is pinned at `4`. These three literals (`4`, the `cmdline` `3`/`5`, and
`dbstat`'s `4`) are the coupling that must become verbose-aware.

**Precedent for verbose-dependent geometry — `extra` (`ui.go:166`):** the `extra` panel is the existing
example of conditional geometry, but note it does **NOT resize `dbstat`'s `SetView` coords** — it is created
**on top of** the lower portion of `dbstat` (gocui overlays later-created views; the same overlay trick is
how `cmdline` sits over the panels). So today's "extra shrinks dbstat" is visual overlay, not coordinate
recomputation. **The verbose spec wants the opposite**: the band must genuinely grow from the top and push
`cmdline` + `dbstat` **down** (so `dbstat` loses rows at the top, not the bottom). That is a true coordinate
recomputation, which `extra` does not demonstrate — verbose is more invasive than any existing toggle.

**Exactly what must change (all literals become a function of one band-height value):**
- compute `bandH := topBandHeight(verbose, maxY)` — compact → `5` (`y1=4`); verbose → `5 + extra` where
  `extra = 3` for `sysstat`, `5` for `pgstat` (asymmetric: the two panels have different verbose heights, so
  the band height is `max(4+3, 4+5)+1` = `pgstat`-driven, with `sysstat` either shrinking short or the band
  sized to the taller side and `sysstat` left-padding). The two panels can keep independent `y1`
  (`sysstat` `y1 = 4 + sysExtra`, `pgstat` `y1 = 4 + pgExtra`) but `cmdline`/`dbstat` must clear the **taller**
  of the two.
- `cmdline` `SetView("cmdline", -1, bandTop, maxX, bandTop+2)` where `bandTop = max(sysstatY1, pgstatY1) - 1`.
- `dbstat` `SetView("dbstat", -1, max(sysstatY1, pgstatY1)+1, maxX, maxY-1)` (top shifts down by the verbose growth).
- **Height-guard** (acceptance criterion + §"Вертикальное место"): if `maxY` cannot fit
  `bandH + cmdline(2) + dbstat header(1) + ≥1 data row`, do NOT expand — fall back to compact and emit a
  cmdline hint. This is a pure comparison on `maxY`.

**Where the geometry math should live (unit-testable without gocui):** extract a pure function, e.g.

```go
// topBandLayout returns the SetView y-coordinates for the top band given verbose state and terminal height.
// Pure arithmetic — no gocui — so the geometry (compact vs verbose, height-guard) is table-tested.
func topBandLayout(verbose bool, maxY int) (sysstatY1, pgstatY1, cmdlineY0, cmdlineY1, dbstatY0 int, expanded bool)
```

placed in `top/ui.go` (or a new `top/layout.go`) and called from inside the `layout(app)` closure, which
then only does the `SetView` plumbing. This mirrors the [009] precedent that pulled `visibleColumns` out of
the render path into a pure, table-tested function (`top/stat.go:429`, ADR [009-feat-horizontal-scroll]
"Manual column window … pure function … unit-testable without a live terminal"). The `verbose` flag reaches
`layout()` via `app.config` (the closure already captures `app`; it reads `app.config.view.ShowExtra` at
`ui.go:166` today — read `app.config.verbose` the same way).

---

### 2-new. Verbose toggle plumbing — end-to-end (reference: `ShowExtra` via `showExtra`)

**Reference flow for `B`/`N`/`F`/`L` (the extra-panel toggles):**
1. **Keybinding** (`top/keybindings.go:53-56`): `{"sysstat", 'B', showExtra(app, stat.CollectDiskstats)}`,
   `'N'`→`CollectNetdev`, `'F'`→`CollectFsstats`, `'L'`→`CollectLogtail`.
2. **Handler** `showExtra(app, extra)` (`top/extra.go:11-81`): toggles off if already shown
   (`extra.go:14`); else opens the `extra` gocui view (`openExtraView`, `extra.go:84`) and — the key part —
   **writes the mode onto EVERY view in the map** so it persists across screen switches:
   ```go
   for k, v := range app.config.views {       // extra.go:70-73
       v.ShowExtra = extra
       app.config.views[k] = v
   }
   app.config.view.ShowExtra = extra           // extra.go:74
   app.config.viewCh <- app.config.view        // extra.go:75 — push to collector + redraw
   ```
3. **Field** `ShowExtra int` lives on **`view.View`** (so it rides `viewCh` to the collector), but is
   mirrored into all map entries so a later view-switch (which loads from `config.views`) keeps it.
4. **Collector gate**: `collectStat` reads `v.ShowExtra` off `viewCh` and calls
   `c.ToggleCollectExtra(extra)` (`top/stat.go:36, 89-91`); `Collector.Update` switches on
   `c.config.collectExtra` (`internal/stat/stat.go:156-175`).
5. **Renderer**: `printStat` reads `app.config.view.ShowExtra` (`top/stat.go:159, 165`).

**The boolean-toggle precedent — `I` (`toggleIdleConns`, `config_view.go:411-435`):** flips
`config.queryOptions.ShowNoIdle = !config.queryOptions.ShowNoIdle` (`config_view.go:417`), re-`Format`s the
query, pushes on `viewCh`. `ShowNoIdle` lives on **`config.queryOptions`** (survives switches because
`queryOptions` is a single struct on `config`, not per-view), but it is **scoped** to `activity`/`procpidstat`
(`config_view.go:413`). This is the cleanest "boolean toggle on config" precedent.

**Where a `verbose bool` should live — two coupled homes (this is the crux):**
- The **renderer** (`printStat` → `printSysstat`/`printPgstat`) and **`layout()`** both read `app.config`
  directly, so they want the flag on **`top.config`** (new field `verbose bool`, `top/config.go:10-20`).
  This is the [009] persistence-vs-ephemerality axis (ADR [009] "Scroll offset on top.config"): unlike
  `scrollOffset`, verbose **must NOT be reset** in `viewSwitchHandler` (spec: persists across screens) — so
  it is config-level state that simply is never zeroed on switch (like `queryOptions.ShowNoIdle`).
- BUT the **collector** reads `view.View` off `viewCh`, **not** `config`. So the flag must ALSO ride the
  view. Two viable options:
  - **(A, recommended) mirror the `showExtra` pattern**: add `Verbose bool` to `view.View`, and in the
    `v` handler write it onto every map entry + `config.view` + `config.verbose`, then push `viewCh`. The
    collector reads `view.Verbose`; the renderer/`layout` read `config.verbose`. Symmetric with `ShowExtra`.
  - **(B) reuse `CollectExtra` (ADR [001])**: add a `CollectVerboseSystem` constant set on the view, read in
    `Collector.Update` to trigger the all-three branch (§5-new). But `CollectExtra` is a single `int`
    (mutually exclusive with `CollectProcPidStat`), and verbose must coexist with the normal active view's
    enrichment — so a **separate bool** (`view.Verbose`) is cleaner than overloading the `CollectExtra` int.
    Recommend (A): `view.Verbose bool` + `config.verbose bool`, kept in sync by the handler.

**Redraw / push pattern** (any handler): mutate state, then `config.viewCh <- config.view` (e.g.
`scrollLeft` `config_view.go:55`, `toggleIdleConns` `config_view.go:425`, `showExtra` `extra.go:75`). The
collector's `select` on `viewCh` (`top/stat.go:80-114`) applies the new view and triggers a fresh `Update`.

**`v` is FREE** — confirmed against the full `sysstat` binding list (`keybindings.go:21-68`): bound lowercase
are `q a b d f i j k l m n o p r s t w x z` and `[ ] < , ~ / - _`; bound uppercase `A B C D E F G I J K L N P Q R S X`.
`v` (and `V`) appear nowhere. Help text (`top/help.go`, `helpTemplate`) needs a one-line `v` entry.

---

### 3-new. `printSysstat` / `printPgstat` extension + io.Writer refactor

**Current `printSysstat`** (`top/stat.go:214-250`) — 4 `fmt.Fprintf` lines into the `sysstat` `*gocui.View`
(`Frame=false`): line1 time+loadavg (`:218`), line2 `%cpu` (`:226`), line3 MiB mem (`:234`), line4 MiB
swap+dirty/writeback (`:242`). Pure text + ANSI (`\033[37;1m…\033[0m`). Takes `v *gocui.View`.

**Current `printPgstat`** (`top/stat.go:253-285`) — 4 `fmt.Fprintf` lines: line1 `formatInfoString`
(`:255`), line2 `activity:` conns (`:261`), line3 `autovacuum:` (`:270`), line4 `statements:` (`:278`).
Takes `v *gocui.View, s stat.Stat, props stat.PostgresProperties, db *postgres.DB`.

**Both write plainly into a `Frame=false` view** (no `PGresult`, no `ColsWidth`, no `align`) — confirmed.
They are dispatched from `printStat` (`top/stat.go:128-146`): `g.View("sysstat")` → `v.Clear()` →
`printSysstat(v, s)`; same for `pgstat`.

**Adding verbose rows:** after the 4 compact lines, gate on the flag:
```go
if verbose {
    // sysstat: +3 rows
    _ = printIostatVerbose(w, s.Diskstats)   // "iostat: N devices, X% max util, …"
    _ = printNicstatVerbose(w, s.Netdevs)    // "nicstat: …"
    _ = printFilesystVerbose(w, s.Fsstats, dataDir)  // "filesyst: …"
}
```
(and the 5 pgstat rows analogously). The flag is `config.verbose` — thread it as a parameter.

**io.Writer refactor (009 pattern) for unit-testability:** today `printSysstat`/`printPgstat` are NOT
unit-tested (they take `*gocui.View`; `top/stat_test.go` only tests the writer-based `printStatHeader`/
`printStatData` with a `bytes.Buffer` — `stat_test.go:394-594`). Mirror `renderDbstat`'s split
(`top/stat.go:349`, which takes `io.Writer`): keep `printSysstat(v *gocui.View, …)` as a thin wrapper that
calls `renderSysstat(w io.Writer, s, verbose, …)`; the new verbose row composers take `io.Writer` and are
tested against `bytes.Buffer` (matching the testing strategy in the user-spec). `*gocui.View` satisfies
`io.Writer`, so the wrapper is a one-line pass-through.

---

### 3a-new. Formatting helpers — what exists vs net-new

**`internal/pretty/pretty.go` — only `pretty.Size(v float64) string`** (`pretty.go:8`):
```go
case v < 1024:          return fmt.Sprintf("%.0fB", v)
case v < 1048576:       return fmt.Sprintf("%.1fK", v/1024)
case v < 1073741824:    return fmt.Sprintf("%.1fM", v/1048576)
case v < 1099511627776: return fmt.Sprintf("%.1fG", v/1073741824)
default:                return fmt.Sprintf("%.1fT", v/1099511627776)
```
It switches the **byte** unit by magnitude (B/K/M/G/T) but appends a bare single letter and a fixed `%.1f` —
it is NOT a rate ("MB/s") suffix switch, has **no digit reservation / fixed width**, and rounds to one
decimal (not integer, not ceil). Used by `printFsstats` (`top/stat.go:765`) for the full fs panel.

**`internal/math/math.go` — only `Min(a,b int)` (`:4`) and `Max(a,b int)` (`:12`).** No float helpers, no
ceil. Repo-wide grep: **zero** uses of `math.Ceil` / `math.Round` / `math.Floor` anywhere in `internal/`
or `top/` (non-test).

**Verdict:**
- **Reusable**: `pretty.Size` for the `filesyst` size/used columns (consistent with the full fs panel, which
  uses it). That's the only reuse.
- **NET-NEW** (all three, as the spec states): (a) **integer ceil rounding** (`math.Ceil` wrapper — trivial,
  but absent); (b) **reserved-digit fixed-width** columns (static `%Nd` layout where only digits change — the
  spec's "резерв N цифр"); (c) **dynamic rate-unit suffix switch** (`MB/s→GB/s`, `Mbps→Gbps` on
  digit-overflow). These belong in a new pure formatter (e.g. `internal/pretty` additions or a `top`-local
  helper) so the overflow boundaries are property/table-tested (user-spec testing section).

---

### 4-new. System-stat aggregates + `%util` consistency (highest-risk)

The verbose one-row aggregates must reuse the **identical** math the full side panels use, so the device a
DBA sees in the verbose row matches the full `B`/`N`/`F` panel exactly. The data comes from the SAME
`s.Diskstats`/`s.Netdevs`/`s.Fsstats` already computed each tick (when collected — see §5-new).

**iostat (`%util`)** — struct `Diskstat` (`internal/stat/diskstats.go:20-61`); the panel printer is
`printIostat` (`top/stat.go:697-722`), which prints `s[i].Util` as the `%util` column. **`%util` is computed
in `countDiskstatsUsage` (`diskstats.go:190-249`):**
```go
itv := curr[i].Uptime - prev[i].Uptime                    // diskstats.go:207
stat[i].Util = sValue(prev[i].Tspent, curr[i].Tspent, itv, ticks) / 10   // diskstats.go:211
```
where `sValue(prev,curr,itv,ticks) = (curr-prev)/itv*ticks` (`stat.go:389-394`). `Tspent` is field 13
(`/proc/diskstats` "time spent doing I/Os, ms"). The verbose aggregate selects
`argmax_i s[i].Util` and reads that device's `Util`, `Rsectors` (→ rMB/s, already divided by `2048` in
`diskstats.go:243`), `Wsectors` (wMB/s, `:244`), `Rcompleted` (r/s, `:241`), `Wcompleted` (w/s, `:242`),
plus `len(s)` for the device count. **Reuse `s.Diskstats` as-is — do NOT recompute** — that is the
consistency guarantee. Note `printIostat` skips devices with `Completed == 0` (`stat.go:706`); the
max-util selection should skip the same inactive devices for an identical device set.

**nicstat (`%util`)** — struct `Netdev` (`internal/stat/netdev.go:24-55`); panel printer `printNetdev`
(`top/stat.go:725-751`). **`Utilization` is computed in `countNetdevsUsage` (`netdev.go:179-239`):**
```go
if curr[i].Speed > 0 {
    stat[i].Rutil = math.Min(stat[i].Rbytes*800/float64(curr[i].Speed), 100)   // netdev.go:223
    stat[i].Tutil = math.Min(stat[i].Tbytes*800/float64(curr[i].Speed), 100)   // :224
    switch curr[i].Duplex {
    case duplexFull: stat[i].Utilization = math.Max(stat[i].Rutil, stat[i].Tutil)            // :228
    case duplexHalf: stat[i].Utilization = math.Min((Rbytes+Tbytes)*800/Speed, 100)          // :230
    }
}
```
(`800` = `100` for percent × `8` bytes→bits; `Speed`/`Duplex` from ethtool, `duplexFull=1`/`duplexHalf=0`,
`ethtool.go:17-19`.) Select `argmax_i s[i].Utilization`. The verbose `rMbps`/`wMbps` must match the panel's
conversion — **`printNetdev` converts bytes→Mbps inline at print time**: `s[i].Rbytes/1024/128`
(`stat.go:741`), NOT in `countNetdevsUsage`. So the verbose row must apply the same `/1024/128` to
`Rbytes`/`Tbytes`. `err/coll` = `Rerrs+Terrs` (the panel prints `Rerrs`,`Terrs` separately, `stat.go:743`)
and `Tcolls` — compose as the spec's `IErr+Oerr / Coll`. `Saturation` field is also available.

**filesyst** — struct `Fsstat` (`internal/stat/fsstat.go:62-76`) with embedded `Mount{Device, Mountpoint,
Fstype, Options}` (`fsstat.go:19-24`); panel printer `printFsstats` (`top/stat.go:754-774`) prints
`Mount.Device`, `Size`/`Used`/`Avail` (via `pretty.Size`), `Pused` (`%use`), `Mount.Fstype`,
`Mount.Mountpoint`. The verbose row shows the **data_directory's** filesystem only (one row), fields:
`Mount.Device` (physical dev), `Mount.Mountpoint` (truncate to 10), `Mount.Fstype`, `Size`/`Used`/`Pused`.

**Identifying the data_directory's filesystem (the hard part):**
- `data_directory` IS a GUC but is **NOT** in `PostgresProperties` today — it is read ad hoc:
  `query.GetSetting, "data_directory"` (`internal/stat/log.go:129`; const `gucDataDir = "data_directory"`
  `top/pgconfig.go:25`). So a new read is needed (either add to `SelectCommonProperties` per §6-new, or a
  one-off `current_setting('data_directory')`).
- **No existing code matches a path to a mount/device.** `Fsstats` is a flat list of mounts; matching is
  net-new: pick the mount whose `Mountpoint` is the **longest prefix** of the (symlink-resolved)
  `data_directory` path — standard "longest mount-prefix wins" (e.g. `/var/lib/pgsql` over `/`).
- **Local-only symlink resolution constraint** (spec edge case): locally, resolve `data_directory` via
  `filepath.EvalSymlinks` before prefix-matching; **remotely** (PL/Perl path) the symlink cannot be
  resolved over the wire, so match the **unresolved** path (documented limitation). `db.Local`
  (`postgres.go:101`) gates which branch. Note `parseProcMounts` (`fsstat.go:27-60`) and the remote query
  (`fsstat.go:188`) both filter to `ext3|ext4|xfs|btrfs` — the data-dir mount must be one of these to appear.

**Risk note:** because `%util` (disk) and `Utilization` (net) are derived in `count*Usage` against a
`prev`/`curr` pair, the **first verbose tick has no prev** → these read 0; per spec, the first tick shows
`n/a` + `collecting...`, so the aggregate must detect "no prev yet" (e.g. empty `Diskstats`/`Netdevs` slice,
since `count*Usage` returns `nil` when `len(curr) != len(prev)`, `diskstats.go:191`/`netdev.go:180`) and
emit `n/a`, not a real `0`.

---

### 5-new. `collectExtra` mutual-exclusion → all-three-at-once verbose branch (R1, invasive)

**Current switch in `Collector.Update`** (`internal/stat/stat.go:156-175`) collects **exactly one** of
disk/net/fs per tick:
```go
switch c.config.collectExtra {
case CollectDiskstats: diskstats, err = c.collectDiskstats(db); … s.Diskstats = diskstats
case CollectNetdev:    netdevs,   err = c.collectNetdevs(db);   … s.Netdevs = netdevs
case CollectFsstats:   fsstats,   err = c.collectFsstats(db);   … s.Fsstats = fsstats
}
```
`c.config.collectExtra` is set by `ToggleCollectExtra` (`stat.go:292`). The three are mutually exclusive
because the side panel shows one at a time. The per-source snapshots
(`prev/currDiskstats`, `prev/currNetdevs`, `currFsstats`) already live on `Collector` (`stat.go:57-64`) and
each `collect*` keeps its own prev/curr (`stat.go:297-346`) — so collecting all three does **not** interfere.

**Verbose-gated all-three branch:** add, after the existing switch (leaving it untouched so side panels are
unchanged — R1 mitigation), e.g.:
```go
if view.Verbose {                       // new flag, read off the view (§2-new option A)
    if s.Diskstats == nil { s.Diskstats, err = c.collectDiskstats(db); … }
    if s.Netdevs == nil   { s.Netdevs,   err = c.collectNetdevs(db);   … }
    if s.Fsstats == nil   { s.Fsstats,   err = c.collectFsstats(db);   … }
}
```
The `== nil` guards avoid double-collecting when the active side panel already populated one (e.g. user is on
`B` iostat AND has verbose on). Each `collect*` is the same function the side panel uses → **identical
structs → consistency** (§4-new). Errors per source should be swallowed to `n/a` (spec: one source failing
must not blank the others) — so this branch should NOT `return s, err` like the side-panel switch does; it
should record per-source availability instead.

**How the verbose flag reaches the collector:** off `viewCh` as `view.Verbose` (§2-new). `collectStat`
(`top/stat.go:60-122`) already threads the received view into `c.Update(db, v, refresh)` (`stat.go:62`), so
`Update` sees `view.Verbose` with no new transport. (If option B / `CollectExtra` constant were used instead,
the existing `prevCollectExtra` change-detection + `c.Reset()` at `top/stat.go:100-103` would fire — another
reason to prefer the separate bool, which needs no `Reset`.)

---

### 6-new. New aggregate SQL + GUC reads (pgstat verbose rows)

**Template to follow:** the `Activity` chain — `collectActivityStat` (`internal/stat/postgres.go:56-104`)
dispatches version-specific single-row queries, scans into the flat `Activity` struct (`postgres.go:33-53`),
and computes Go-side rates vs a `prev` value and `itv`, e.g. `s.CallsRate = (s.Calls - prevCalls) / itv`
(`postgres.go`). Dispatch helpers: `SelectActivityActivityQuery(version)` (`common.go:127`). New aggregates
add: a query const (+ version variants), a dispatch fn, a struct (extend `Activity` or a new `Pgstat`
sub-struct), a collect call in `Update`, Go-side rates against `c.prevPgStat`.

**GUC reads** — `SelectCommonProperties` (`common.go:43-50`) currently reads `server_version`,
`server_version_num`, `track_commit_timestamp`, `max_connections`, `autovacuum_max_workers`,
`shared_preload_libraries`, `pg_is_in_recovery()`, `pg_postmaster_start_time()` → scanned in
`GetPostgresProperties` (`postgres.go:125-134`) into `PostgresProperties` (`postgres.go:107-120`).
**NOT read today** (all net-new, confirmed): `max_worker_processes`, `max_logical_replication_workers`,
`max_parallel_workers`, `wal_segment_size`, `data_directory`. Add-pattern (3 edits each): append
`current_setting('X')::type AS x` to `SelectCommonProperties`, add field to `PostgresProperties`, add
`&props.GucX` to the `.Scan(...)` in `GetPostgresProperties`. (`GucMaxPrepXacts` `postgres.go:115` is a
declared-but-never-scanned placeholder — same gap.)

| verbose pgstat row | source / status | cite |
|---|---|---|
| **workload** (tps=Σcommit+rollback, ins/upd/del/ret, tmp, others=Σdeadlocks+conflicts+csum_fail) | **NET-NEW aggregate**. No `SELECT sum(...) FROM pg_stat_database` exists; per-row cols are in `PgStatDatabaseGeneralDefault`. Go-side `/itv` rate vs prev. | `databases.go:5-16` (col names to sum) |
| **databases** (Σ`pg_database_size`+count, growth/s, per-interval cache hit `Δhit/Δ(hit+read)`) | **NET-NEW**. `pg_database_size` appears nowhere in the repo. count+sum trivial; growth = Go-side delta of total size vs prev; cache-hit per-interval = Go-side delta ratio. | none |
| **workers** (`max_worker_processes` umbrella; logical/parallel active from `backend_type` vs GUC) | **NET-NEW aggregate + 3 NET-NEW GUCs**. Active counts = `count(*) FILTER (WHERE backend_type …)` over `pg_stat_activity` (the `backend_type` filter idiom already in `SelectActivityDefault` `common.go:61`). | `common.go:61`; GUCs net-new |
| **replication** | mixed (see below) | |
|  · wal size (`pg_wal` dir) | **EXISTS-expr-liftable** — lift the waldir subselect | `wal.go:6,16` |
|  · archiving backlog = `count(.ready)*wal_segment_size` | **NET-NEW**, but adapts the EXACT existing precedent `count(1) * pg_size_bytes(current_setting('wal_segment_size'))` (over `pg_ls_waldir()`) to `pg_ls_dir('pg_wal/archive_status')` filtered to `.ready`; pure SQL, works remote; `n/a` on `archive_mode=off`/EACCES | precedent `wal.go:6` |
|  · lag bytes (worst-case) | **EXISTS-expr-liftable** — `{{.WalFunction1}}({{.WalFunction2}}(), replay_lsn)` diff, take `max`/sum over `pg_stat_replication` | `replication.go:5-15` |
|  · slots/retained WAL | **EXISTS-expr-liftable** — `{{.WalFunction1}}({{.WalFunction2}}(), s.restart_lsn)` (`retained,KiB`); `count(*)`+`max` | `replication_slots.go:14-31` |
|  · send/recv | **NET-NEW** — `count(*) FILTER (WHERE backend_type LIKE 'walsender%' / = 'walreceiver')` over `pg_stat_activity` | `common.go:61` idiom |
| **bgwr/ckpt** (timed/req absolute; write/sync ms delta; maxwritten) | **EXISTS-reuse the version-split** — `SelectStatBgwriterQuery(version)` returns the right query for PG14-16 (`pg_stat_bgwriter`) / PG17 / PG18 (`+pg_stat_checkpointer`); read `ckpt_timed`/`ckpt_req` (absolute — ADR [004] "Absolute event counters"), `ckpt_write,ms`/`ckpt_sync,ms` (delta), `maxwritten`. Drop buffers (spec: avoid dup of bgwriter screen). | `bgwriter.go:7-52` |

**Helpers the new queries reuse** (`query.go`): `selectWalFunctions(version, recovery)` (recovery-aware
WAL fn names — `pg_wal_lsn_diff`/`pg_current_wal_lsn` vs `pg_last_wal_receive_lsn`), `Options`/`NewOptions`,
`Format(tmpl, opts)`. Replication/WAL templates use `{{.WalFunction1/2}}` and MUST go through `Format`.

**Versioning gotchas:** `bgwr/ckpt` already handled by `SelectStatBgwriterQuery`. `pg_stat_database`
`checksum_failures`/`temp_files`/`conflicts`/`deadlocks` exist across PG 14-18; the `workload` `others`
composite is safe on supported versions. Recovery-aware WAL/lag on standby via `selectWalFunctions`.

---

### 7-new. Tiering/guard seam + which tests change

**Per-source rhythm/latency-guard state** belongs on the `Collector` struct (`internal/stat/stat.go:52-74`),
which already holds all prev/curr snapshots and is the long-lived per-session object reused every tick. Add
fields like `lastRunSizes time.Time` / per-source skip-divisors there; the verbose collect functions read/
update them. The spec's "throttle only the dear aggregates without a live panel twin (db sizes, growth);
system rows every tick" maps to: system collect (§5-new) runs unconditionally under verbose; the
db-size/growth aggregate gates on `time.Since(c.lastRunSizes) >= budget` and otherwise reuses a cached last
value (stale, not `n/a`). All hidden behind the single `z` interval — no second user knob. This mirrors the
extensible-seam mitigation (a registry of sources with per-source cadence on the Collector).

**Is registering a view required? NO.** Verbose is a **config flag + collector gate + renderer change** —
it adds **no** entry to `view.New()`. Therefore the brittle view-count assertions are **UNCHANGED**:
- `internal/view/view_test.go:11` `assert.Equal(t, 27, len(v))` — unchanged.
- `internal/view/view_test.go:224-248` `TestView_VersionOK` per-version totals (27/24/19/16/14/14) — unchanged.
- `record/record_test.go:101-137` `Test_filterViews` `wantN`/`wantV` — unchanged.
- `record/record_test.go:32` `Test_app_record` — unchanged (derives count dynamically via `countRecordable`).

This is the major win of the pivot: **zero view-count test churn.**

**No keybinding-count test exists** — there is no `top/keybindings_test.go` at all, so adding `v` breaks
nothing there.

**Tests that touch the files we'll modify (and whether they break):**
- `top/stat_test.go` — tests `formatInfoString`, `formatError`, `alignViewToResult`, `visibleColumns`,
  `printStatHeader`/`printStatData` (writer-based, `bytes.Buffer`). **`printSysstat`/`printPgstat` are NOT
  tested today.** Our changes there don't break existing tests; we ADD writer-based tests for the new rows.
- `internal/stat/stat_test.go` — `TestCollector_Update` (`:27-69`) sets
  `c.config.collectExtra = CollectDiskstats` then `c.Update(conn, views["activity"], time.Second)` (live PG,
  no skip guard). Adding the verbose branch (gated on `view.Verbose`, default false) leaves this green;
  a new test sets `view.Verbose=true` and asserts all three of `Diskstats`/`Netdevs`/`Fsstats` populate.
- `internal/query/common_test.go` — `Test_CommonQueries` (`:63-108`) execs `SelectCommonProperties` across
  PG versions with `t.Skipf` guards. **Adding GUCs to `SelectCommonProperties` keeps it valid** (just more
  columns); the scan in `GetPostgresProperties` must add matching targets or `Test_collectActivityStat`/
  `TestGetPostgresProperties` (`postgres_test.go:66-100`) will fail at scan — update the `.Scan(...)` in
  lockstep with the SELECT.
- `internal/stat/postgres_test.go` — `Test_collectPostgresStat`, `Test_collectActivityStat`,
  `TestGetPostgresProperties` (live PG). New aggregate collect fns get sibling live-PG tests with `t.Skipf`.
- `top/keybindings.go` — no test file; nothing breaks.
- `top/ui.go` — **no `ui_test.go` exists**; the new pure `topBandLayout` function should get one
  (`top/ui_test.go` or `layout_test.go`), table-testing compact/verbose/height-guard without gocui.
- `top/config_view_test.go` — only `Test_switchViewTo`/`Test_*NextView` hardcode view names; verbose adds no
  view, so **untouched**. (A new `Test_toggleVerbose`, like `Test_toggleIdleConns` `:617`, is additive.)

**Net:** no existing assertion breaks from the pivot; all test work is additive (geometry pure-fn,
writer-based row composition, all-three collect branch, new aggregate queries, GUC scan lockstep).

### ADRs / tech-debt bearing on this feature
- **[001] `CollectExtra` mechanism** — relevant but the verbose flag should be a **separate `view.Verbose
  bool`**, not an overload of the mutually-exclusive `CollectExtra` int (§2-new, §5-new).
- **[009] state on `top.config`, persistence-vs-ephemerality** — `verbose` lives on `config` like
  `scrollOffset`, but is the **non-reset** kind (NOT zeroed in `viewSwitchHandler`); persistence like
  `queryOptions.ShowNoIdle`.
- **[004] absolute checkpoint counters** — `bgwr/ckpt` timed/req are absolute; reuse `SelectStatBgwriterQuery`
  and read the pre-`DiffIntvl` counter columns directly.
- **Tech-debt [005]/[008]** (`top/reload_test.go`, `record/record_test.go` panic instead of `t.Skipf`
  without live PG) — Low; only relevant in that new live-PG tests must use the `t.Skipf` guard pattern, not
  panic. No view-count path is touched (pivot avoids it), so [008]'s `countRecordable` concern does not apply.
