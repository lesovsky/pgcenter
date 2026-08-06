# pgcenter — Code Patterns

## Adding a New PostgreSQL Version

**Beta versions come from a separate apt channel with a per-major component.** `apt.postgresql.org`
publishes each major version in its own component, so the source line must name it —
`jammy-pgdg-testing 19`, not `jammy-pgdg-testing main`. With `main` the package is simply invisible and
`apt-cache policy` reports candidate `(none)`, which reads exactly like "the packages do not exist" (this
cost a false "feature blocked" conclusion in 012). Install with an explicit `-t <suite>`: the stable
channel outranks the beta one for the shared `libpq5`, so without it the install fails on a dependency.
Listing only the version component — not `main` — is what keeps the beta channel from offering anything
else; an apt-preferences pin adds nothing, because the beta suite already ships `NotAutomatic` (priority
100). `libpq5` does move to the beta version and that is expected: one client library serves all clusters.

1. Add port to `internal/postgres/testing.go` ports map
2. Add version to all `versions := []int{...}` lists in `internal/query/*_test.go`
3. Run tests — `t.Skipf` handles unavailable versions gracefully
4. If a stats view changed: add a new query constant and selector function in `internal/query/`
5. Wire selector into `internal/view/view.go: Configure()` if Ncols also changes
6. Update pgcenter-testing Docker image (see deployment.md)

`NewTestConnectVersion` returns an error for a version with no port mapping — it used to fall back to the
oldest cluster, which made a forgotten entry invisible: subtests named after the new version passed while
exercising a completely different server. Note what the error does and does not buy: a missing entry now
makes those subtests **skip**, which is honest but still green in CI, so proving a new version is actually
reached still needs a deliberate check (stop the cluster, confirm skip rather than pass).

## Version-Specific Query Pattern

When a PG version changes columns in a stats view:
- Add `PgStatXxxPGNN` constant in the relevant `internal/query/*.go` file
- Add `SelectStatXxxQuery(version int) (string, int)` returning template + ncols
- Call it in `view.Configure()` under the correct view name
- Add version-specific test cases in `*_test.go`

When versions differ by **column count** (not just names), the selector must also carry the
layout: return `(string, int, [2]int)` (`+DiffIntvl`, see `io.go`) or `(string, int, [2]int, int)`
(`+UniqueKey`, see the `statements_jit` selector in `internal/query/statements.go` — UniqueKey
points at the trailing md5 `queryid`, whose index shifts with ncols) and patch all returned
fields in `Configure()`. When only names change but the count is constant (e.g. `statements_timings`),
`Configure()` swaps `QueryTmpl` alone and the static `Ncols`/`DiffIntvl`/`UniqueKey` stay valid.

Reference implementations of the single-row version-aware view: `internal/query/wal.go` and `internal/query/bgwriter.go`. The bgwriter screen is notable for placing absolute event-counter columns (`ckpt_*`, `rstpt_*`) **outside** the contiguous `DiffIntvl` range so they render cumulative, while the work/time/buffer columns inside the range render as per-interval deltas.

For a **multi-row hybrid view** that LEFT JOINs two stats views, see `internal/query/replication_slots.go` (the `replslots` screen). Two patterns it establishes:
- **`coalesce(...,0)` on diffed columns fed by a LEFT JOIN.** A row present in both samples enters `diff()`/`diffPair()`; if an outer-joined diffed column is SQL NULL it scans as an empty string and `strconv.ParseInt("")` aborts the whole sample. Coalescing NULL→0 in SQL keeps such rows diff-safe (physical slots, absent from `pg_stat_replication_slots`, render `0`). Only diffed columns need this — absolute columns outside `DiffIntvl` pass NULLs through as empty.
- **Recovery-aware WAL distance for free** via the `{{.WalFunction1}}({{.WalFunction2}}(), lsn)` template (`selectWalFunctions` in `query.go` picks `pg_current_wal_lsn` on a primary, `pg_last_wal_receive_lsn` on a standby) — no recovery branch in the query.
A multi-row view sets `UniqueKey` to the stable row identity (slot_name, col 0) for cross-sample row matching, and may set a non-default `OrderKey` (replslots: 4 = retained,KiB desc) for a domain-appropriate default sort.

When the row identity is **composite** (more than one column), emit a synthetic key column and point `UniqueKey` at it — `internal/query/io.go` (the `pg_stat_io` screen) does `left(md5(backend_type||object||context),10) AS io_key` at column 0, following `statements_io`'s `queryid`. Column hiding is still not available (`internal/align` floors width at 8), so the key column is shown, not hidden. `io.go` is also the reference for splitting one wide stats view into two registered sub-views (`stat_io` count / `stat_io_time` time) navigated by a lowercase toggle (`statioNextView`) plus an uppercase menu (`menuStatIO`) — the pattern to copy when a view's columns are better presented as logically grouped screens. (Since 009-feat-horizontal-scroll the main table also scrolls horizontally; the sub-screen split is kept as a deliberate grouping choice, not because columns would otherwise be unreachable — see `architecture.md`.)

## Testable TUI Rendering — pure window function + io.Writer printers (009-feat-horizontal-scroll)

`gocui.View` cannot be constructed in a unit test, which historically left the `dbstat` print path untested. Two patterns make TUI rendering testable without a live terminal — copy them for any non-trivial render logic:

- **Pure layout function as the single source of truth.** Extract the only non-trivial arithmetic (the visible-column window) into a pure function `visibleColumns(...)` (`top/stat.go`) that takes plain inputs (counts, widths, terminal width, offset) and returns a value (`columnWindow` + clamped offset + flags). It is exhaustively unit-tested, including a property test that walks the parameter space (`ncols × widths × termWidth`) to prove an invariant — here, "the last column is reachable at `maxOffset`". The function re-clamps on every call, so render and key handlers never hold an authoritative copy of derived state.
- **Write back the clamped value at render time.** The render path (`renderDbstat`) calls the pure function once, renders from its result, and writes the clamped offset back to `config.scrollOffset`. Key handlers only nudge the raw offset (and guard against int overflow); the upper bound is enforced solely by the render-time clamp. This keeps the "what fits" logic in one place rather than duplicated in handlers.
- **Printers take `io.Writer`, not `gocui.View`.** `printStatHeader`/`printStatData` accept an `io.Writer` and the precomputed `columnWindow` instead of reading `v.Size()` internally, so tests assert rendered output against a `bytes.Buffer`. The width-and-window decision is hoisted to the caller, which is the only piece that needs the live view.

**Caveat learned in manual QA:** a window function that admits a scrollable column only when it fits *whole* silently drops a deliberately wide trailing column (e.g. `query`). Allow the last column to render *partially* (start-in-budget), and reserve marker-glyph width in **both** the forward and backward walk, or the last column becomes unreachable at the right edge. This class of bug is invisible to unit tests written against the original (whole-column) semantics — a litmus test that fails on the wrong semantics is the guard.

## Extract the decision out of the unreachable closure (016-feat-pause-display)

The `io.Writer` pattern above covers *rendering*. This one covers *decisions* made inside a `g.Update`
closure — code a test can never enter, because its failure site is `g.View(...)` and neither
`*gocui.Gui` nor `*gocui.View` can be built outside the gocui package (the repo already records this
on `Test_printCmdlineNilGui`, `top/ui_test.go`).

Give each such decision its own callable unit and have the closure do nothing but call it:

- **The repaint failure latch** is a method on the store taking the repaint outcome and returning
  whether to emit a message. Its fail → fail → success → fail transition behaviour is driven directly
  by a test; the closure keeps one call.
- **"Which path am I"** is a value (`renderParams`, built by one constructor per path) rather than a
  branch inside the closure, so "the repaint path never publishes" is checkable by calling the two
  constructors and the publish helper.
- **The logtail source choice** is `selectLogtail(params, read)`, where the live read is passed as a
  closure. A test asserts non-invocation with a `read` that calls `t.Fatal`.

**Why this is a rule and not a preference.** Two review rounds in 016 produced the same defect class
twice: a test that looks like proof and cannot fail. One asserted on a local `config` that the
handler under test never receives; the other drove a function whose signature could not reach the file
it claimed to prove untouched. Both passed. Both were caught only by **mutating the production code**
and observing that the suite stayed green — inverting `if !p.fromFile`, deleting a bounds guard,
hoisting a lift above its early return. Make that the acceptance criterion when a test guards an
invariant: name the mutation the test must fail on, run it, and see red before believing the green.

When the property genuinely cannot be reached without a forbidden seam, say so in the test comment
and defer it to the stand run — do not dress inspection up as a red test.

## Running the mutation, and reading its result honestly (017-feat-wal-archiver)

The rule above ("name the mutation, see it red") is necessary but not sufficient. Three ways it was
observed to lie in 017, all found by doing it rather than by reasoning about it:

- **A mutation that reddens for the wrong reason proves nothing.** Name the mutation *and* check
  which assertion goes red. Dropping `pg_monitor` from the positive privilege test reddened on the
  test's own membership guard — before the query ran — so the SQLSTATE 42501 the criterion asked
  about was never reached, and the claim ended up resting on a permanently-present negative test
  instead. Same class, recorded as a deliberate negative control: deleting the `archiver` case from
  `Views.Configure()` leaves the package green, because `New()` already seeds the same
  `QueryTmpl`/`Ncols`/`DiffIntvl` — so those `TestViews_Configure` asserts guard drift between the
  selector and the static entry, not the wiring they look like they guard. Write that boundary next
  to the assertion; the next reader will otherwise assume the stronger claim.
- **"No FAIL lines" is not "the test passed".** A mutation that breaks compilation produces
  neither — one in 017 left a variable unused, the package did not build, and grepping the output
  for `FAIL` read as green. Mutate through a declaration that stays used (or otherwise keep the
  package compiling), and confirm the run by counting PASS/RUN for the named subtest rather than by
  the absence of FAIL. The sibling trap: `go test -run` is case-sensitive, so a filter that silently
  matches nothing looks identical to a clean pass.
- **"This cannot be unit-tested" was false twice in one feature.** A zero-value `&gocui.Gui{}` is
  enough to drive `menuSelect`'s branches to completion and to exercise keybinding registration —
  the gocui constraint recorded above applies to `g.Update` closures and `*gocui.View`, not to
  everything that mentions gocui. What gocui does not give you is the registered binding: the
  handlers are unexported and cannot be fetched or invoked, so `keybindings()` was split into a
  `keybindingsList(app) []key` table plus the registration loop, which makes *which handler a key
  carries* assertable. Before that split, binding `W` to the wrong menu left the whole suite green.

## Verbose display-mode toggle (010-feat-overview-dashboard)

When adding an on/off *display mode* that layers extra rows over the current screen (not a new screen),
mirror the verbose top-panel mode rather than registering a view:

- **Dual-home the flag like `showExtra`.** A mode that both the collector and the renderer must see needs
  two homes: `view.View.Verbose bool` (rides `viewCh` to `Collector.Update`) and `top.config.verbose bool`
  (read by the renderer/layout in the gocui handler goroutine). The toggle handler (`top/verbose.go:toggleVerbose`)
  writes the flag into **every** view in `config.views` (the `showExtra` write-into-all-views idiom) so the
  mode **persists across screen switches** — persistence is free because `viewSwitchHandler` simply never
  zeroes it (unlike `scrollOffset`, which it deliberately resets). Prefer a dedicated boolean over
  overloading `CollectExtra` (a mutually-exclusive `int` whose toggle path fires `Reset()` — see ADR [010]).
- **Skip the `collectStat()` Reset on a mode-only toggle.** `collectStat()` calls `c.Reset()` on the
  `viewCh` push, which blanks the `prev*` snapshots (one frame of empty CPU/mem/load deltas). Add an early
  `if prevVerbose != v.Verbose { … continue }` branch (mirroring the existing `ShowExtra` branch, placed
  **before** both Reset paths) so toggling the mode does not wipe the snapshots.

## Panel/screen consistency — reuse the struct math (010-feat-overview-dashboard)

When a summary row must show the *same* number a detail panel/screen shows, read the **same struct** the
full panel renders and replicate any print-time conversion, do not recompute from scratch:

- The verbose iostat/nicstat rows select the max-`%util` device and read `Util`/`Utilization` AS-IS from
  the existing `count*Usage` structs (the full `B`/`N`/`F` panels' math), filtering active devices the same
  way `printIostat`/`printNetdev` do. nicstat's rMbps/wMbps is **computed at print time** in `printNetdev`
  (`Rbytes/1024/128`), so the verbose row replicates that exact conversion — recomputing independently is a
  divergence bug.
- The verbose filesyst `use%` uses panel parity (`fs.Pused` via `%3.0f`, **not** `Ceil`) so it matches the
  full fsstat panel (a `Ceil` would read 75% where the panel reads 74%). The ceil rule applies only to rate
  fields, not to percentages already computed by the struct.

## Reserved-width `n/a` for static trailing labels (010-feat-overview-dashboard)

A degraded field that renders `n/a` (3 chars) where a value (e.g. ` 99.99%`, 7 chars) would otherwise sit
makes the **trailing label jump horizontally** as the signal appears/disappears. Reserve the `n/a` to the
value's reserved width so it is a drop-in: `naReserve(width)` = `fmt.Sprintf("%*s", …)` right-aligned (the
mirror of `pretty.ReserveWidth`'s `%*d`), with a `len("n/a")` min-width guard. Apply it only to
**fixed-width** fields (cache-hit ratio `%6.2f%%`, the `%d` workload rates). Variable-width `pretty.Size`
fields are made drop-in the same way via `pretty.SizeWidth(v, width)` (see the rate-formatter section) —
since 011-refactor-tech-debt-paydown the verbose Size fields use it under a single `sizeFieldWidth = 8`
const with `naReserve(sizeFieldWidth)` fallbacks, so value and `n/a` share the reserve. A row's first
field (e.g. `wal size`) pushes no trailing label and stays a bare `Size`.

## Dynamic unit-suffix rate formatter (010-feat-overview-dashboard)

For a fixed-digit-budget rate column that must not break layout on top-end hardware (NVMe arrays >9.7 GB/s,
25/40/100GbE), `internal/pretty` has three net-new pure formatters: `Ceil` (round up via `math.Ceil` —
`internal/math` had no ceil), `ReserveWidth` (`%*d` fixed width, never truncates), and `RateUnit` (promotes
the unit one step on reserved-digit overflow — MB/s→GB/s with binary 1024, Mbps→Gbps with **decimal 1000**
per network convention). Pure functions → property/table tests at the overflow boundary.

Since 011-refactor-tech-debt-paydown the overflow/divisor/ceil computation lives in one unexported core
`rateUnitParts(v, family, width) (field, unit)`; `RateUnit` (no separator, `9999MB/s`) and
`RateUnitPrefixed(v, family, prefix, width)` (a `" "+r/w` marker between digits and unit, `1135 rMB/s`,
used by the verbose disk/net rows) both delegate to it — add a new assembly form there, never a second
copy of the overflow logic. Also added: `pretty.SizeWidth(v, width)` = `fmt.Sprintf("%*s", width, Size(v))`,
the fixed-width drop-in for `Size` (right-align, never truncate, digits/units unchanged) — `Size` itself
stays variable-width for its other callers.

## Adding a New View — test counts that must be updated

Registering a view in `view.New()` couples to count-based tests that fail in CI (not always locally) if missed:
- `internal/view/view_test.go: TestNew` pins the total view count. `TestView_VersionOK` pins per-version availability — its row at a version **≥ the new view's `MinRequiredVersion`** also increases by one (feature 007's PG15+ view bumped only the `160000` row, not the `≤140000` rows).
- `record/record_test.go: Test_filterViews` pins, per version, how many views `filterViews` drops vs keeps. A `NotRecordable: true` view is always dropped, so every `wantN` row increases by the number of new `NotRecordable` views (feature 006 added 2 → `+2` each row; feature 007 added 1 → `+1`; `wantV` unchanged). This test runs without Postgres, so a stale count is a real failure even though the rest of the `record` package skips/fails on a missing PG fixture — do not assume a red `record` package is only the connection-refused tests.

Counts alone are a weak guard, and 017 measured it: with `Test_filterViews` pinning only numbers, a
view that fell out of the kept set could be masked by an arithmetic coincidence, so the table gained
a per-row "is *this* view kept" field. The same feature also found that nothing pins the registry's
own invariants for any other view — `key == v.Name`, and non-nil `ColsWidth`/`Filters`. The maps
matter beyond tidiness: three writers in `top/config_view.go` write into them **in place**, so a nil
map is a panic on the first column-width change or filter, not a wrong number. Pin them for any view
you add.

Adding a `pg_stat_statements` **sub-screen** (or any `menuPgss`/cycle entry) additionally breaks `top` tests — `Test_selectMenuStyle` (pins each menu's item count), `Test_statementsNextView`, and `Test_switchViewTo` (pin the `x`-cycle transitions). These `top` tests DO run locally without Postgres, so they catch the miss in `make test` — but feature 007's code-research overlooked them (the task wrongly assumed the TUI layer had no tests). When touching `top/menu.go` or `top/config_view.go`, grep `top/*_test.go` for the function you changed before assuming it is untested. A **new** hotkey group (017's `w`/`W`) breaks the same three plus the help-screen tests, and needs one thing more: the binding itself, which only `keybindingsList` makes assertable (see the mutation section above).

## Error Wrapping

Use `fmt.Errorf("context: %w", err)` for all error wrapping in production code.
Use `errors.Is(err, target)` for error comparison (not `==`).
Exception: `printCmdline()` and `fmt.Sprintf()` use `%s` (not error wrapping functions).

## Sorting

Use `sort.SliceStable` (not `sort.Slice`) in `internal/stat/postgres.go` to ensure deterministic ordering of rows with equal sort keys across Go versions. Stability is load-bearing, not cosmetic: the activity screen's parallel-group view relies on rows with an equal `leader` staying in the `ORDER BY pid DESC` order the query gave them.

`PGresult.sort` (013-feat-activity-xmin-horizon) carries three rules that a sparse column made necessary:

- **The comparator mode is chosen from the first non-empty cell**, not from row 0. A blank first cell fails both `ParseFloat` and `parseDuration`, so a numeric column would silently fall into the string comparator, where `"9"` outranks `"1000000"`.
- **An empty cell orders last in both directions and in all three modes.** Emptiness is decided by the rendered string, deliberately not by `sql.NullString.Valid`: every render path prints `.String` alone, so a SQL NULL and a genuine empty string look identical on screen and must not be ordered differently. `Valid` is also unreliable for this — `diff()` sets it true unconditionally inside the `DiffIntvl` range, so it would mean different things on diffed and non-diffed screens. Before this rule, a blank parsed as `0` and was indistinguishable from a genuine zero.
- **The sort key is bounds-checked.** It never comes from the data being sorted — it is a screen seed or an index resolved against an earlier sample — so a replayed archive can hand it a key the current layout does not have.

**Testing a sort change is where this is easy to get wrong.** Two obvious constructions do not discriminate: a lone blank under descending sort lands last under both old and new behaviour, and putting the blank first only diverges if the remaining values order differently lexicographically than numerically (`2048` before `1024` does not; `512` vs `1024` does). Any test here must be **shown failing against the unfixed comparator**, not reasoned about — three successive specification attempts at this were each too weak, and the values that trip it are the ones already sitting in the neighbouring test.

## Report replay across a recorded version change

`report/report.go:processData` reconfigures the view when a replayed sample's recorded PG version
changes. `Configure` rewrites the query and column count and nothing else, so anything derived from
the previous layout survives unless it is reset explicitly. Three things must be
(013-feat-activity-xmin-horizon, closing debt [021]):

- the alignment flag, or widths from the old layout are reused;
- the header-repeat counter, or the previous header stays on screen for another 20 rows;
- the resolved sort column — and **restoring the view's seed key is required, not just re-arming the
  latch**: if the requested `-o` column is absent from the new layout, the latch simply stays down and
  the index resolved against the old layout survives. With columns inserted mid-layout that index now
  denotes a different column, so the report sorts by something the operator did not ask for — a wrong
  answer with no symptom.

Two traps when testing this. The replay path consumes the first sample after a version change, so the
archive needs at least two samples on each side. And `processData` runs in a goroutine, so a panic
there takes down the whole test binary instead of reddening one test — drive the formatting function
directly.

**Error paths in that pipeline must leave the reader unblocked.** `readTar` sends on an unbuffered
channel and signals completion from a defer; a `processData` error that abandons both leaves the
command hanging rather than exiting, which for a CLI in a pipe is worse than a crash. `doReport`
drains until the reader finishes.

## Manual Testing / QA Phase

Always run `make build` as the first step of any manual TUI verification, even if a previous
build completed earlier in the same session. Cherry-picks, rebases, and mid-session code
changes do not automatically update `./bin/pgcenter`. A stale binary silently invalidates every
visual check that follows. The rule: one manual verification session = one fresh build at the
start.

### Driving the TUI on a remote test stand (agent-run verification)

An agent can verify interactive TUI behaviour end-to-end — not only unit-test the render cores —
by driving pgcenter inside tmux on a dedicated stand over ssh. The regimen below is carried over
from the sibling book project (`../postgresql-destruction-recovery-guide-book`, "Операционные
правила стенда"), where it is already proven for capturing pgcenter screens.

**Stand.** A dedicated VM reachable over ssh. The address and credentials are **not stored in this
repository, and not worth recording anywhere else either** — stands are ephemeral, with a TTL
measured in hours, so a saved address is stale by the next session. Ask the author at the start of
every run, and never assume the previous run's state survived.

**Take an inventory before planning the run — the stand is not a fixed image.** Observed so far:
Ubuntu 24.04 with tmux preinstalled, and Debian 12 with no tmux, no Go, and a Postgres Pro build
whose `shared_preload_libraries` lacks `pg_stat_statements` (so the statements screens are simply
unavailable). Check for `tmux`, the PostgreSQL flavour and the loaded libraries first; installing
tmux is fine with the passwordless sudo these stands carry, but a missing extension may mean a
screen cannot be exercised at all — decide that before writing the plan, not mid-run.

**Bring a second binary built from `master`.** Running the same scenario on both is what separates a
regression introduced by the feature from behaviour that was always broken. In 015 this reclassified
three of five findings as pre-existing; without it they would have been filed against the feature.

**The binary under test must be shipped explicitly.** The stand carries a pgcenter built from
`master`; a feature branch's behaviour is not there until you copy the freshly built
`./bin/pgcenter` over and invoke it by an explicit path. Running the system-wide `pgcenter` and
reporting the result is the remote-stand equivalent of testing a stale binary.

**Deterministic terminal size is the point of using tmux.** `tmux new-session -d -s cap -x 190
-y 52` fixes the geometry, so width-dependent behaviour becomes reproducible rather than a
function of whatever window the operator had open. Narrow sizes (`-x 60`) are how the horizontal
column window, the verbose height-guard fallback and any width-clamped indicator get exercised
deliberately — these paths are otherwise nearly untestable by hand.

**Keys go in with `send-keys`, screens come out with `capture-pane`.** Send a hotkey, allow at
least one refresh interval to pass, then capture. Two capture modes, and picking the wrong one
silently invalidates the check:

- `tmux capture-pane -t cap -p` — plain text, escape sequences stripped. Use for layout, column
  content, row counts, indicator text.
- `tmux capture-pane -t cap -p -e` — **keeps** the escape sequences. This is the only way to
  verify colour and attribute rendering (bold values in the header panels, the reverse-video sort
  column, the frozen-column bold). Without `-e` a missing `\033[37;1m` is indistinguishable from a
  present one, so a colour regression passes.

Stripping ANSI from an `-e` capture for diffing: `sed 's/\x1b\[[0-9;]*m//g'`.

**Leave the stand as you found it.** Server GUCs changed for an experiment are reset afterwards
and the reset is verified with `SHOW`; the tmux session is killed at the end of the run.

## The cmdline: one composer, transient messages, persistent state (015-feat-tui-papercuts)

Everything written to the cmdline goes through **one composition point**. `printCmdline` keeps its
historical signature and its 2-second clear timer; `printCmdlinePersist` is the same writer without
the timer, for text that must survive until dismissed (the dialog prompt). Both delegate to a shared
core that builds the line as *reserved state prefix* + *transient message* and clamps it to the
terminal width in **runes**.

- **Persistent state is a token, not a message.** `cmdlineToken` carries renderings ordered
  longest-to-shortest; the composer degrades the rightmost token through its variants only when the
  line does not fit, then drops tokens from the right. A token with exactly one variant never
  shrinks — that is how a "must always be visible" indicator is expressed in data rather than in a
  special case. Adding a second token requires no change to the composer.
- **Read state only on the gocui goroutine.** The composer reads its state from a package-level
  ambient `*config`, written once by a named setter from `RunMain` — deliberately not from `newApp`,
  which unit tests call repeatedly and would leave a stale pointer behind. Dereference only inside
  `g.Update` closures or key handlers. This is the package's only package-level mutable var; it
  exists so the 44 existing call sites keep their signature.
- **The clear timer renders, it does not erase.** It re-renders the prefix-only line inside its own
  `g.Update` (removing an older race where a bare goroutine touched the view buffer) and is gated on
  an atomic UI-generation counter **captured into a local before the goroutine is spawned** — read
  inside, it would compare a value to itself and do nothing. `mainLoop` rebuilds the `Gui` without
  closing it on every pager/editor return, so an ungated timer pins the abandoned one.
- **Sanitise anything that came from the server.** Column names reach the indicator, and
  `gocui.View.Clear()` resets the line buffer but *not* the escape-interpreter state — an
  unterminated sequence on this low-frequency surface survives the whole session, unlike the stats
  table, which repaints every tick amid correct sequences and self-heals.

**Still true, and now the sharper hazard:** two writes in one code path still leave only the last
one visible, because `g.Update` enqueues each from its own goroutine and the order is not
guaranteed. Keep exactly one call per path — an `if/else`, or the 4-branch `switch` that
`switchViewToProcPidStat` uses for its two independent probes. Two known paths violate this and
predate the composer: `dialogFinish` writes an empty line before its result, so **no message shown
after a dialog closes is ever visible** (the action still happens), and the verbose height-guard
hint loses to `collecting...` on the same keypress. Both are registered tech debt — do not treat a
"missing" cmdline message as a new bug before checking whether it is one of these.

## Adding a Hybrid View (SQL + procfs enrichment)

When a view combines SQL and local system data (e.g., procpidstat = pg_stat_activity + /proc):

1. Define a `CollectExtra` constant in `internal/stat/stat.go` iota block. The iota is offset by 1 (`pgProcUptimeQuery` string constant precedes the group): existing values `CollectNone=1, ..., CollectLogtail=5`; next is 6.
2. Register the view in `view.New()` with `NotRecordable: true`, `DiffIntvl: [2]int{0,0}`, `Filters: map[int]*regexp.Regexp{}`. Leave `CollectExtra`/`IOAvailable` at zero — set at runtime by the switch handler.
3. The switch handler (`top/config_view.go`) must save/load/patch/send the view manually — NOT via `viewSwitchHandler`, which reloads from the static map and discards runtime patches.
4. In `Collector.Update()`, add a `view.CollectExtra == CollectXxx` branch after `collectPostgresStat` to enrich and replace the SQL result.
5. In `top/stat.go:collectStat()`, add `prevCollectExtra` change-detection alongside `ShowExtra` to call `c.Reset()` on view switches.
6. If the view should NOT be recordable: set `NotRecordable: true` in view definition; `filterViews()` skips it automatically.
   If the view SHOULD be recordable with procfs enrichment: leave `NotRecordable` at default `false` and follow the tarRecorder stateful pattern (step 7 below).
7. Reference implementation: `internal/stat/procpidstat.go`, `top/config_view.go:switchViewToProcPidStat`.

## Recording a Hybrid View (SQL + procfs, with pgcenter record)

When a hybrid view needs record/report support (reference: 003-feat-procpidstat-record-report):

1. Leave `NotRecordable: false` (default) on the view. Add local/remote gate in `record.app.setup()`: if `!db.Local`, delete the view from `views` and print INFO — procfs is not available over remote connections.
2. Add `isLocal`, `ticks`, `cpuCount`, availability flags, and `prev`/`curr` procfs maps to `tarRecorder` struct. Initialize in `app.setup()` via `GetSysticksLocal()`, `runtime.NumCPU()`, and `stat.CheckIOAvailable()` / `stat.CheckDelayAcctAvailable()` probes.
3. In `tarRecorder.collect()`, add an enrichment branch **after** the main views loop. Mirror the map-rotation protocol from `Collector.Update()`: build `newPrev` from current map filtered to PIDs in the SQL result, rotate maps, then read procfs for each PID (`stat.ReadProcPidStat`, `stat.ReadProcPidIO`). Compute `itv` via `time.Since(lastCollect)`.
4. In `tarRecorder.write()`, hoist `now := time.Now()` to the function top so all entries share the same timestamp. Append a `sysinfo.TIMESTAMP.json` entry (`stat.SysInfo{Ticks, CPUCount}`) for each tick — needed by the report pipeline to document recording environment.
5. In `report/report.go`: extend `isFilenameOK` to accept the new entry prefix; handle the entry in `readTar`; extend `metadata` struct if report-side metadata is needed. Use `DiffIntvl=[0,0]` if rates are pre-computed by the recorder (same as `activity` pattern).
6. Detect "no data" in `processData` via `anyDataPrinted bool` (not `linesPrinted` — initialized to `repeatHeaderAfter = 20`). Detect unavailable columns via empty-string sentinel on first result.

## Git Workflow

- Work in `develop`, open PRs to `master` with squash merge
- After squash: `git reset --hard master && git push --force-with-lease` to sync develop
- Release: tag on master → push to `release` branch → triggers release workflow

### Commit trailers — single `Co-Authored-By`

A commit message carries **at most one** `Co-Authored-By:` trailer, on the last line. When
collapsing several commits into one, **deduplicate the trailer to a single line** — do not let the
concatenated commit bodies stack N copies. The per-commit trailer on the feature branch is fine;
the pile-up only appears when bodies are joined, so the fix lives at the squash/merge step:

- **GitHub PR squash merge** (the usual path here — see the `(#NNN)` merge commits on `develop`):
  GitHub's default squash body concatenates every source commit message, so each per-commit
  trailer stacks (plus GitHub appends its own `Co-authored-by` lines). **Override the squash
  commit body** — `gh pr merge --squash --body "…"` (or edit it in the merge UI) so the final
  message has exactly one `Co-Authored-By:` at the end.
- **Local squash** — `git merge --squash {branch}` then `git commit` with a hand-written body
  (one trailer), not the auto-generated `.git/SQUASH_MSG` that lists every commit.

## Linting

`.golangci.yml` enables: errcheck, gocritic, gosimple, govet, ineffassign, revive, staticcheck, unused.
Run locally: `make lint` (golangci-lint + gosec) and `make vuln` (govulncheck).
Known suppressions: `// #nosec G204,G702` on `exec.Command` calls (pager/editor from env vars).

## Naming Conventions

Go acronyms: `CPUStat` not `CpuStat`, `PGresult` not `PgResult`.
Unused function parameters in callbacks: rename to `_`.
