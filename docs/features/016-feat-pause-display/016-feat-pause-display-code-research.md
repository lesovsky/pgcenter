# Code Research — 016-feat-pause-display

> **ПОПРАВКИ ПОСЛЕ УТВЕРЖДЁННЫХ РЕШЕНИЙ USER-SPEC (2026-08-03).** Первые три места ниже описывают
> формы, которые спека прямо запрещает; ещё два пункта — уточнения, всплывшие при валидации.
> Техническое задание пишется по спеке, а не по исходному тексту исследования:
>
> 1. **§1.2, фрагмент `if paused { store(s); continue }` — сломанная форма.** Хранилище нельзя
>    обновлять отброшенными кадрами: замороженным считается кадр, который был на экране в момент
>    нажатия `Space`. Записывать в хранилище следует только пока пауза выключена.
> 2. **§7.7, «логтейл никогда не перерисовывается из хранилища, потому что `L` снимает паузу» —
>    неверно.** `L`, нажатая *во время* паузы, действительно её снимает, но панель, открытая *до*
>    паузы, остаётся на экране и попадает в каждую перерисовку. По решению владельца она
>    замораживается вместе с кадром: файл лога на паузе не читается.
> 3. **§10, «reuse `printStat` verbatim» — недостаточно.** `printStat` начинается с подсказки
>    первого тика и содержит ветку чтения файла лога; при перерисовке из хранилища обе должны быть
>    пропущены.
> 4. **Уточнения по хранению строк лога (решение Q24) и по §13.1.** «Показанные строки лога» — это
>    последний **непустой** прочитанный буфер: `readLogfileRecent` возвращает `nil`, когда файл не
>    изменился, и тогда `printLogtail` ничего не печатает; прямолинейная реализация «сохранить
>    буфер того кадра, на котором нажали `Space`» на спокойном логе даст пустую панель — ровно тот
>    дефект, который решением Q24 и закрывался. Детектор ротации (`size < logtail.Size`) работает
>    покадрово, поэтому длинная пауза расширяет окно промаха: после снятия паузы панель может
>    какое-то время читать старый дескриптор. Кроме того, буфер логтейла рождается внутри
>    `g.Update`-замыкания `printStat`, то есть на gocui-горутине, тогда как кадр приходит на
>    worker-горутине — это фактически закрывает открытый вопрос §13.1 в пользу публикации через
>    `g.Update`: гейт в `doWork` должен только отбрасывать кадры, ничего не записывая.
> 5. **`[PAUSED]` после пересборки интерфейса восстанавливается отдельной записью в командную
>    строку**, независимой от перерисовки кадра: вьюха `cmdline` создаётся пустой, а сохранённого
>    кадра может не быть вовсе (пауза до первого кадра). Реализация вида «хранилище пусто → выходим»
>    потеряет маркер.
>
> Отдельно: подсказки `collecting...` в обычном режиме не существует — она печатается только по
> флагу первого тика verbose-режима (`top/stat.go:150`, флаг выставляется исключительно внутри
> verbose-ветки, `internal/stat/stat.go:290`). Спека это учитывает: при паузе до первого кадра
> экран просто пуст.

**Date:** 2026-08-03
**Scope:** `pgcenter top` render pipeline, `doWork` gate placement, frame storage, repaint paths,
cross-goroutine state, hotkey classification, the [015] cmdline composer, tests, risks.
**Sources read:** `top/*.go` (all), `internal/stat/stat.go`, `internal/stat/postgres.go`,
`internal/view/view.go`, `github.com/jroimartin/gocui@v0.5.0`, `github.com/nsf/termbox-go@v1.1.1`,
`docs/decisions-log.md`, `docs/tech-debt.md`, `.claude/skills/project-knowledge/patterns.md`.

---

## 0. Verdict on the pre-settled constraints

Checked one by one. Two are wrong in their citation, one is wrong in its consequence (understated),
one needs a correction of substance, and two hold exactly.

| # | Claim | Verdict |
|---|-------|---------|
| 1 | `statCh` unbuffered "at top/ui.go:73" | **Substance HOLDS, citation WRONG.** `statCh := make(chan stat.Stat)` is `top/ui.go:108`. Line 73 is `wg.Add(1)` in `mainLoop`. **Consequence understated — see §7.1: not selecting on `statCh` deadlocks the whole UI, it does not merely show a stale frame.** |
| 2 | Sorting runs in the collector goroutine | **HOLDS.** `internal/stat/stat.go:440` → `calculateDelta` (`internal/stat/postgres.go:580`) → `delta.sort` (`postgres.go:599`, impl `postgres.go:678`), all inside `Collector.Update`, called from `collectStat` (`top/stat.go:66`). |
| 3 | Filtering is a render-time op at `top/stat.go:743` | **HOLDS, line-exact.** `top/stat.go:743`: `return printStatData(w, s, config, isFilterRequired(config.view.Filters), win)`; the row predicate is `top/stat.go:1051-1061`. |
| 4 | Verbose + side panels need collector participation via `viewCh` | **HOLDS.** `view.Verbose` is read in `Collector.Update` (`internal/stat/stat.go:262, 401`), `ShowExtra` in `ToggleCollectExtra` (`top/stat.go:99-102`), `CollectExtra` in `Update` (`internal/stat/stat.go:333`). A stored frame carries no `Overview` and no `Diskstats/Netdevs/Fsstats` when those were off. |
| 5 | `mainLoop` rebuilds the Gui, destroying view buffers | **HOLDS, with a nuance.** `top/ui.go:48-50` creates a fresh `gocui.NewGui` per loop iteration. On the pager path the OLD Gui *is* closed — by the handler itself (`top/pglog.go:33`, `pgconfig.go:55, 98`, `psql.go:21`, `report.go:148`) — but it is abandoned as an object, and the new Gui's views start with empty buffers. The "without closing it" phrasing in `top/ui.go:24-28` describes the *UI-error restart* path, not the pager path. Either way the conclusion stands: **after a rebuild nothing repaints the screen until the next frame from the collector.** |
| 6 | The composer takes a second token with no change to itself | **HOLDS, and is already tested.** `composeCmdline` (`top/ui.go:257`) is token-count agnostic; `top/ui_test.go:29, 92-99, 134-147` already exercise a literal `[PAUSED]` token to the left of the filter token. Only `cmdlineTokens` (`top/ui.go:406-417`) must change. |

Additional correction of substance, not in the constraint list:

- **The header clock is not frozen by "not repainting" — it is re-derived on every render.**
  `renderSysstat` calls `time.Now()` inline (`top/stat.go:277`). Any repaint of a stored frame
  (resize, filter, pager return) re-prints the *current* time next to *frozen* stats — exactly the
  incoherent screen interview Q3 ruled out. Freezing the clock requires either storing the frame's
  timestamp and threading it into `renderSysstat` (signature change, 1 call site + 4 tests), or
  skipping the sysstat repaint entirely (rejected: after a rebuild the panel would stay blank).

---

## 1. Entry Points — the render pipeline of `pgcenter top`

### 1.1 Call chain and goroutine ownership

```
RunMain (top/top.go:12)                                  [main goroutine]
  postgres.Connect -> newApp -> setCmdlineConfig(app.config) (top.go:26) -> app.setup() (top.go:29)
  mainLoop(ctx, app) (top.go:35)
    for { (ui.go:48)
      gocui.NewGui (ui.go:50)                            [main goroutine]
      uiGeneration.Add(1) (ui.go:57)
      app.ui.SetManagerFunc(layout(app)) (ui.go:62)
      keybindings(app) (ui.go:65)
      go doWork(ctx, app) (ui.go:74)                     [WORKER goroutine]
      app.ui.MainLoop() (ui.go:80)                       [becomes the GOCUI goroutine]
      ... on error: cancel(); wg.Wait(); loop -> rebuild
    }
```

```
doWork (ui.go:106)                                       [worker goroutine]
  statCh := make(chan stat.Stat)                         (ui.go:108)  <-- UNBUFFERED
  go collectStat(ctx, app.db, statCh, app.config.viewCh) (ui.go:111)  [COLLECTOR goroutine]
  app.config.view.Refresh = defaultRefresh; viewCh <- view (ui.go:117-118)
  for { select {
    case <-app.uiExit:            return                 (ui.go:125)
    case s := <-statCh:           printStat(app, s, props) (ui.go:128-129)   <-- THE GATE GOES HERE
    case <-ctx.Done():            wg.Wait(); return      (ui.go:130-132)
  }}
```

```
collectStat (stat.go:25)                                 [collector goroutine]
  NewCollector; v := <-viewCh (stat.go:33); prefill Update (stat.go:42)
  for {
    stats := c.Update(db, v, refresh)                    (stat.go:66)   <-- SQL + procfs + diff + SORT
    select { case statCh <- stats:  | case <-ctx.Done(): close(statCh); return }  (stat.go:72-79)
    ticker := time.NewTicker(refresh)                    (stat.go:83)
    select { case v = <-viewCh: ...  | case <-ctx.Done() | case <-ticker.C }      (stat.go:84-140)
  }
```

```
printStat (stat.go:157)                                  [called on WORKER goroutine]
  firstTickHint -> printCmdline (stat.go:165-167)        -> its own g.Update
  app.ui.Update(func(g) { ... })                         (stat.go:169)  [body runs on GOCUI goroutine]
      g.View("sysstat"); v.Clear(); printSysstat (stat.go:170-178)  -> renderSysstat (stat.go:269)
      g.View("pgstat");  v.Clear(); printPgstat  (stat.go:180-188)  -> renderPgstat  (stat.go:481)
      g.View("dbstat");  v.Clear(); printDbstat  (stat.go:190-199)  -> renderDbstat  (stat.go:706)
      if ShowExtra > CollectNone: printIostat / printNetdev / printFsstats / logtail (stat.go:201-250)
```

`layout(app)` (`ui.go:138`) runs on the **gocui goroutine**, called by `gocui.flush()` on *every*
event-loop iteration (`gocui@v0.5.0/gui.go:434-438`) — it only does `SetView` plumbing plus the
verbose height-guard hint (`ui.go:211-218`); it never prints stats.

### 1.2 Where the gate goes

The only correct place is the `statCh` branch of `doWork`'s select (`ui.go:128-129`): the receive
must keep happening, and only the `printStat` call is skipped. Concretely:

```go
case s := <-statCh:            // the receive is NOT conditional — see §7.1
    if paused { store(s); continue }   // drain and discard (store for repaint)
    printStat(app, s, app.postgresProps)
```

### 1.3 What the `g.Update` closure captures today

`printStat`'s closure (`stat.go:169-252`) captures `app` (hence `app.config`, `app.db`,
`app.postgresProps`), the frame `s` **by value**, and `props`. Inside the closure it reads/writes:

| Touched | Where | Note |
|---|---|---|
| `app.config.verbose` | `stat.go:175, 185` | read |
| `app.config.refresh` | `stat.go:175` | read |
| `app.config` (whole, for `printDbstat`) | `stat.go:196` | `renderDbstat` **writes** `config.scrollOffset` (`stat.go:734`) and `config.autoScrollToOrderKey` (`stat.go:716`), and `alignViewToResult` writes `config.view.Cols/ColsWidth/Aligned` (`stat.go:675-677`) |
| `app.config.view.ShowExtra` | `stat.go:201, 207` | read |
| `app.config.logtail.Size/Path` | `stat.go:227-245` | read+write |

So a repaint-from-stored-frame is *literally the same call*: `printStat(app, storedFrame, props)`.
No new render entry point is needed — only a frame to hand it.

---

## 2. Data Layer — what a frame is

### 2.1 The type on the wire

`chan stat.Stat`, unbuffered (`ui.go:108`). `stat.Stat` (`internal/stat/stat.go:35-54`):

```go
type Stat struct {
    System        // LoadAvg, Meminfo, CPUStat, Diskstats, Netdevs, Fsstats, VerboseFirstTick
    Pgstat        // Activity (all scalars), Result PGresult, Overview PgstatOverview
    Error error
}
```

`PGresult` (`internal/stat/postgres.go:444-450`) is `Values [][]sql.NullString`, `Cols []string`,
`Ncols`, `Nrows`, `Valid`. `Diskstats`/`Netdevs`/`Fsstats` are slices. Everything else
(`Activity` `internal/stat/postgres.go:113-133`, `PgstatOverview` `internal/stat/stat.go:30+`,
`CPUStat`, `Meminfo`, `LoadAvg`) is flat scalars.

`stat.Stat` is passed **by value** end to end (`ui.go:129` → `stat.go:157` → closure capture), so a
plain assignment `last = s` copies all scalars and shares the slice headers. Shallow copy is the
cheap option; the question is whether the collector still touches those backing arrays.

### 2.2 Aliasing hazard — real, and it bites the default screen

`Collector.Update` (`internal/stat/stat.go:435-445`):

```go
c.prevPgStat = c.currPgStat
c.currPgStat = Pgstat{Activity: activity, Result: res, Overview: overview}
diff, _ := calculateDelta(c.currPgStat.Result, c.prevPgStat.Result, ...)
s.Pgstat.Result = diff
```

`calculateDelta` (`internal/stat/postgres.go:580-601`):
- `DiffIntvl != [0,0]` → `diff()` allocates a **fresh** `Values` (`postgres.go:606`), so the sent
  frame does not alias the collector's snapshots. Values are then sorted in that fresh slice.
- `DiffIntvl == [0,0]` → `delta = curr` (`postgres.go:596`), then `delta.sort(...)`
  (`postgres.go:599`) sorts **in place**. The frame sent on `statCh` therefore shares its
  `Values` backing array with `c.currPgStat.Result`, which becomes `c.prevPgStat.Result` on the
  next tick.

Views with `DiffIntvl == [0,0]` (`internal/view/view.go:43, 305, 317, 329, 352`) include
**`activity` — the default startup screen** — plus `procpidstat` and three others.

Today the collector never *reads* those strings again for `[0,0]` views (`diff()` is not called for
them; only `prevPgStat.Activity.Calls` is read, `internal/stat/stat.go:308`), so no race is
observable. But the render side **writes** into that array: `printDataCell` truncates in place
(`top/stat.go:1113`):

```go
s.Result.Values[rownum][i].String = s.Result.Values[rownum][i].String[:width-1] + "~"
```

Two consequences for this feature:

1. **A stored frame is destructively edited by its own rendering.** Truncation is idempotent at a
   fixed width, but if the user widens the column (`↑`, `increaseWidth`, `config_view.go:87`) or
   the terminal is resized wider while paused, the repaint shows the *already truncated* `…~`
   text forever — the original value is gone. Live mode never notices because every tick brings a
   fresh frame.
2. **The write reaches the collector's `prevPgStat` for `[0,0]` views**, and pausing extends the
   window in which the render goroutine holds that array from ~1 refresh to unbounded.

**Recommendation for the frame store:** store the whole `stat.Stat` (all panels need it — Q3 froze
the whole frame), but **deep-copy `Pgstat.Result.Values`** at store time (`Nrows × Ncols`
`sql.NullString`, a few thousand small strings at worst — a screenful). `Cols` may be shared
(never mutated on the render path — `alignViewToResult` *replaces* the slice on `config.view`,
`stat.go:675`, it does not write through it). `Diskstats/Netdevs/Fsstats` are read-only on the
render path (`printIostat` `stat.go:1122`, `printNetdev` `stat.go:1150`, `printFsstats`
`stat.go:1179`) — shallow is fine, and those toggles lift the pause anyway.

Plus, per §0: store the **frame timestamp** if the clock is to freeze.

### 2.3 Where the store must live

It cannot be a `doWork` local: `doWork` **returns** on the pager/editor path (`ui.go:125`) and a
brand-new `doWork` is started after the UI rebuild (`ui.go:74`). The store must outlive it →
`app` or `config`. That makes it the **second** cross-goroutine object (see §4), not just the
`atomic.Bool` the roadmap anticipated.

Two workable disciplines:

- **(a) Mutex on `config`** — `sync.Mutex` + `last stat.Stat` + `lastValid bool`. Written by the
  worker goroutine in the gate, read by the gocui goroutine on repaint. Explicit, but it is a new
  synchronisation primitive in a package whose only one today is `uiGeneration` (`ui.go:29`).
- **(b) Publish through `g.Update`** — the gate hands the frame over inside a `g.Update` closure
  that just assigns it, so the store is only ever touched on the gocui goroutine, exactly like
  `cmdlineCfg` (ADR [015], `ui.go:18-21`). The channel send inside `Update` provides the
  happens-before edge. This matches the package's existing "read state only on the gocui goroutine"
  rule (patterns.md §"The cmdline"). Cost: the store is written asynchronously, so a repaint
  triggered in the same keypress could theoretically read a frame one tick old — harmless here.

Whichever is chosen must be stated in the tech-spec, because option (b) contradicts the interview's
"the flag is the only cross-goroutine race" as literally written.

---

## 3. Similar Features — patterns to copy

| Feature | What it establishes | Where |
|---|---|---|
| **[009] horizontal scroll** | Ephemeral render-only state on `top.config`, not on `view.View`; render-time clamp with write-back; handlers only nudge. Precedent for "a key changes rendering without touching the collector" — but note both handlers still push `viewCh` **purely to force a redraw** (`config_view.go:63, 81`) | `config.scrollOffset` (`config.go:26`), `renderDbstat` (`stat.go:734`) |
| **[009] auto-scroll one-shot** | A `bool` on `config` set by a handler and consumed by the next render (`config.autoScrollToOrderKey`, `config.go:32`, consumed `stat.go:715-721`). The nearest existing shape to a pause flag | `config_view.go:32, 48` |
| **[010] verbose** | Dual-home flag: `view.View.Verbose` rides `viewCh`, `config.verbose` is read by the renderer/layout. Write-into-all-views idiom | `verbose.go:14-36`, ADR [010] |
| **[015] cmdline composer** | Token data model, ambient config, generation-gated timer | `ui.go:241-501` — see §6 |
| **[015] `uiGeneration`** | The **only** `sync/atomic` primitive in `top/`, with an explicit comment describing why (`ui.go:24-29`). A pause `atomic.Bool` should sit next to the same class of documentation | `ui.go:29` |

`showExtra`/`toggleVerbose` are the reference for "a toggle that must reach the collector";
`scrollLeft`/`scrollRight` for "a toggle that must not".

---

## 4. Integration Points & cross-goroutine state (question D)

### 4.1 What crosses goroutines today

| Object | Written by | Read by | Sync |
|---|---|---|---|
| `config.viewCh` (unbuffered, `config.go:49`) | ~14 key handlers (gocui goroutine) + `doWork` seed (`ui.go:118`, worker) | `collectStat` (`stat.go:85`) | channel |
| `statCh` (unbuffered, `ui.go:108`) | `collectStat` (`stat.go:73, 130`) | `doWork` (`ui.go:128`) | channel |
| `app.uiExit` (unbuffered, `top.go:82`) | pager/editor/psql handlers (gocui) | `doWork` (`ui.go:125`); closed by `quit` (`top.go:90`) | channel |
| `view.View` **contents** sent on `viewCh` | handlers | collector | value copy — **but `Filters`, `ColsWidth` maps and `Cols` slice are shared by reference**; the collector reads none of them |
| `cmdlineCfg` (`ui.go:22`) | `setCmdlineConfig` once, pre-goroutine (`top.go:26`) | gocui goroutine only | discipline, documented `ui.go:18-21` |
| `uiGeneration` (`ui.go:29`) | `mainLoop` (`ui.go:57`) | timer goroutines (`ui.go:465, 473`) | `atomic.Uint64` |

**Everything else on `config` is single-goroutine (gocui) by construction**: `scrollOffset`,
`verbose`, `autoScrollToOrderKey`, `refresh`, `dialog`, `menu`, `procMask`, `queryOptions`,
`view.*` are written by handlers and read either by `layout()` or inside `printStat`'s `g.Update`
closure — all on the gocui goroutine. The comments say so explicitly at `ui.go:154`
(`config.verbose` "read in the gocui handler goroutine … no race") and `config.go:38-39`
(`config.refresh` "Written on the gocui goroutine … and read there too").

**This is why the pause flag is different**: it is the first field that must be *read on the worker
goroutine* (`doWork`'s select) and *written on the gocui goroutine* (the Space handler). Hence
`atomic.Bool`. Matching convention: put it on `config` next to the other display-mode flags
(`config.go:26-40`), with a comment in the same register as `ui.go:24-29`.

`config` is only ever handled as `*config` (`newConfig` returns a pointer, `config.go:44`;
tests build `&config{...}`, e.g. `stat_test.go:822, 1102`), so an embedded `atomic.Bool` will not
trip `go vet` copylocks.

### 4.2 Files this feature touches

- `top/config.go` — pause flag (+ frame store, if it lives here).
- `top/ui.go` — `doWork` gate (`ui.go:128`), repaint-after-rebuild, `cmdlineTokens` (`ui.go:406`).
- `top/keybindings.go` — one row in the table (`keybindings.go:18-81`), scoped `"sysstat"`.
- `top/pause.go` (new) — handler + token helper, mirroring `top/verbose.go`'s size and shape.
- `top/help.go` — `helpTemplate` (`help.go:10-48`); the interview made the lifting set a
  **user-visible requirement**, so it needs its own line, not just a `Space` mention.
- `top/stat.go` — only if the frozen clock is implemented via a timestamp parameter to
  `renderSysstat` (`stat.go:269`).

---

## 5. Hotkey classification — verified against the code (question E)

Key: **C** = effect produced in the collector goroutine (needs a new frame) → must LIFT the pause.
**R** = effect produced at render time from the frame in hand → pause can be KEPT.

| Key | Handler | `viewCh` push? | Where the effect is produced | Class | Interview said | Match |
|---|---|---|---|---|---|---|
| `←` / `→` | `orderKeyLeft/Right` `config_view.go:22, 40` | yes (`:34, :50`) | `OrderKey` → `Collector.Update` → `calculateDelta` → `delta.sort` (`internal/stat/stat.go:440`) | **C** | LIFTS | ✔ |
| `<` | `switchSortOrder` `config_view.go:113` | yes (`:118`) | same sort path, `OrderDesc` | **C** | LIFTS | ✔ |
| `,` | `toggleSysTables` `config_view.go:386` | yes (`:418`) | rewrites `view.Query` (`:403-415`) | **C** | LIFTS | ✔ |
| `I` | `toggleIdleConns` `config_view.go:491` | yes (`:505`) | rewrites `view.Query` (`:499`) | **C** | LIFTS | ✔ |
| `A` | `dialogChangeAge` → `changeQueryAge` `config_view.go:426` | yes (`:451`) | rewrites `view.Query` (`:443-450`) | **C** | LIFTS | ✔ |
| letters `a d r o t i s f w b p x j` | `switchViewTo` → `viewSwitchHandler` `config_view.go:209, 315` | yes (`:320`) | new query + `c.Reset()` (`top/stat.go:127`) | **C** | LIFTS | ✔ |
| `S` | `switchViewToProcPidStat` `config_view.go:329` | yes (`:366`) | `CollectExtra` enrichment (`internal/stat/stat.go:333`) | **C** | LIFTS | ✔ |
| `D X P J E` menus | `menuOpen`/`menuSelect` `menu.go:106, 140` | yes via `viewSwitchHandler` | as view switch (`menuConf` opens `$EDITOR` instead) | **C** | LIFTS | ✔ |
| `v` | `toggleVerbose` `verbose.go:14` | yes (`:26`) | `view.Verbose` gates collection of `Overview` + all three system sources (`internal/stat/stat.go:262, 401`) | **C** | LIFTS | ✔ |
| `B` `N` `F` `L` | `showExtra` `extra.go:11` | yes (`:75`, `:104`) | `ToggleCollectExtra` (`top/stat.go:101`), and `L` re-reads the logfile each frame (`top/stat.go:227-245`) | **C** | LIFTS | ✔ |
| `/` | `dialogFilter` → `setFilter` `config_view.go:124` | **no** | `printStatData` predicate, `stat.go:743, 1051-1061` | **R** | KEEPS | ✔ |
| `\` | `clearFilters` `config_view.go:193` | **yes, `:200`, but only when `n > 0`** | render-time predicate | **R**, *with a caveat* | KEEPS | ⚠ see note |
| `[` `]` | `scrollLeft/Right` `config_view.go:59, 73` | **yes, `:63, :81` — "solely to trigger an immediate redraw"** | `visibleColumns` at render (`stat.go:727`) | **R**, *with a caveat* | KEEPS | ⚠ see note |
| `↑` `↓` | `increaseWidth/decreaseWidth` `config_view.go:87, 100` | yes (`:94, :107`) | `ColsWidth` read at render (`stat.go:1106, 1117`) | **R**, *with a caveat* | KEEPS | ⚠ see note |
| `z` | `changeRefresh` `config_view.go:518` | yes (`:535`) | collector's ticker (`top/stat.go:93-96`); header value is `config.refresh`, render-time (`stat.go:175`) | **R** (effect visible on resume) | KEEPS | ✔ |
| `Q` | `resetStat` `reset.go:13` | no | server-side `SELECT pg_stat_reset()`; shows up in the next frame | **R** | KEEPS | ✔ |
| `-` `_` `k` `K` `n` `R` `G` | `dialogOpen` variants, `signal.go`, `reload.go`, `report.go` | no | server-side / pager | **R** | KEEPS | ✔ |
| `m` | `showProcMask` `signal.go:141` | no | cmdline only | **R** | KEEPS | ✔ |
| `h` / `F1` | `showHelp` `help.go:52` | no | overlay view | **R** | KEEPS | ✔ |
| `l` `C` `~` | pager/editor/psql `pglog.go:32`, `pgconfig.go:54, 97`, `psql.go:20` | no (they push `uiExit`) | external program + UI rebuild | **R** (pause survives — needs repaint) | KEEPS | ✔ |
| `q` `Ctrl+C` `Ctrl+Q` | `app.quit` `top.go:88` | — | exit | n/a | — | — |

**The classification is correct. Three caveats about the "KEEPS" rows, which are the interesting part:**

1. **`[`, `]`, `↑`, `↓` and `\` push on `viewCh` today purely to force an immediate repaint** —
   `config_view.go:57-58` says it in so many words: *"sends the view on viewCh solely to trigger an
   immediate redraw — the view itself is not mutated"*. That mechanism **stops working while
   paused**: the collector will produce a frame, `doWork` will discard it, and the screen will not
   move until the user unpauses. So every render-only key that "keeps" the pause needs an explicit
   **repaint-from-stored-frame** path, or it silently does nothing. This is the single largest
   piece of work the roadmap does not name.
   Note `changeRefresh` (`z`) also pushes `viewCh` and its push *is* meaningful (the collector
   changes its ticker, `top/stat.go:93`) — leave it alone.
2. `\ clearFilters` pushes `viewCh` only when it actually removed something (`config_view.go:199`);
   a no-op clear already produces no redraw today.
3. `A` (`changeQueryAge`) and `z` are reached through the **dialog**, whose current view is
   `"dialog"` — a `"sysstat"`-scoped Space binding cannot fire there, which is what makes typing a
   space into a regexp filter safe. Confirmed at `gocui@v0.5.0/keybinding.go:36-41` (`matchView`)
   and `gui.go:593`.

**Key-constant finding (load-bearing):** bind `gocui.KeySpace`, **not** the rune `' '`.
termbox classifies any byte `<= 0x20` as a functional key and sets `Ch = 0, Key = Key(b)`
(`termbox-go@v1.1.1/termbox.go:575-580`; `KeySpace Key = 0x20` at `api_common.go:122`).
gocui matches on `key && ch && mod` (`keybinding.go:31-33`), so a binding registered as `' '`
(`ch=' ', key=0`) would **never** fire against an event carrying `key=KeySpace, ch=0`.

---

## 6. The [015] cmdline composer (question F)

### 6.1 Token model

```go
type cmdlineToken struct{ variants []string }   // top/ui.go:245
```

`variants` is ordered **longest → shortest**. A token with exactly one variant never shrinks; it is
dropped whole instead (`ui.go:241-244` says this in the doc comment, naming `[PAUSED]` as the
motivating example).

`composeCmdline(tokens, msg, width) string` (`ui.go:257-300`) — pure, no gocui, no config. Ladder:

1. `width <= 0` → `""` (`ui.go:260-262`) — the post-pager degenerate geometry.
2. render `tokens[:kept]` at their current variant index; if the prefix fits, stop (`ui.go:269-273`).
3. otherwise degrade the **rightmost** token one step (`ui.go:277-281`);
4. when it has no shorter variant, **drop** it (`ui.go:282`) and repeat.
5. Then: no prefix → `truncateRunes(msg, width)`; no msg → prefix; else
   `prefix + " " + truncateRunes(msg, rest-1)`, and if fewer than 2 columns remain the message is
   dropped rather than leaving a dangling separator (`ui.go:285-299`).

All arithmetic is in **runes** (`utf8.RuneCountInString`), Decision 12.

### 6.2 Where tokens come from

```go
func cmdlineTokens(config *config) []cmdlineToken {   // top/ui.go:406
    if config == nil { return nil }
    var tokens []cmdlineToken
    if t, ok := filterToken(config.view.Filters, config.view.Cols); ok {
        tokens = append(tokens, t)
    }
    return tokens
}
```

**This is the whole change needed for a left-of-filter `[PAUSED]`:** prepend the pause token before
the `filterToken` append. Order in the slice *is* left-to-right screen order
(`renderCmdlineTokens`, `ui.go:303-312`), and the degradation ladder always eats from the right, so
a leftmost single-variant `[PAUSED]` survives until the filter token is fully gone — which
`ui.go_test`'s cases at `ui_test.go:92-99` already assert.

Contract for the reader: `cmdlineTokens` **MUST be called from the gocui goroutine only**
(`ui.go:400-405`). A pause token read from `config` there is consistent with that *if the flag is
atomic* — the `atomic.Bool` load is safe from any goroutine, so no discipline is broken.

Three call sites consume the tokens, and all three get `[PAUSED]` for free:
`writeCmdline` (`ui.go:453`), the clear timer (`ui.go:489`), and `dialogOpen` →
`dialogPromptFit`/`dialogInputX0` (`dialog.go:162-165`).

**The dialog geometry is width-sensitive to a new token.** `dialogPromptFit` (`dialog.go:70-89`)
computes the room for the prompt as `maxX - minDialogInputWidth - 1 - len(prefix) - 1`. Adding 8
runes of `[PAUSED]` shortens every dialog prompt by 8 columns while paused. It degrades correctly
(the prompt truncates with `…`, and the input field's 10 reserved columns are untouched) — but the
`Set state mask` prompt is already 93 characters and already truncates at 80 columns, so on a
narrow terminal a paused dialog prompt will be visibly shorter. Worth one acceptance line, not a
design change.

### 6.3 The 2-second clear timer vs a persistent token

`printCmdline` → `writeCmdline(g, arm=true, ...)` (`ui.go:421, 435`). The timer does **not erase** —
it re-renders the *prefix-only* line (`ui.go:477-495`), i.e. `composeCmdline(cmdlineTokens(cmdlineCfg), "", width)`.
So a persistent token is **strengthened**, not threatened, by the timer: two seconds after any
message the line becomes exactly `[PAUSED][F:...]`. Nothing to do.

The timer is armed only when `msg != ""` (`ui.go:461`), and gated on the UI generation captured
into a local *before* the goroutine starts (`ui.go:465, 473`) — `uiGeneration` bumps at
`ui.go:57`. If pause is toggled while a timer is in flight, the timer redraws with whatever tokens
are current at fire time, which is the desired behaviour.

**Trap inherited from debt [027]/[028] and patterns.md:** *exactly one `printCmdline` per code
path.* The Space handler must therefore emit **one** message (or none) — and note that the token
itself is the marker, so the handler arguably should print nothing at all and let the next
`g.Update`-driven repaint carry the new prefix. If it prints nothing, the token will not appear
until *something* rewrites the cmdline — so the handler must trigger a cmdline write explicitly
(e.g. one `printCmdline(g, "...")`, or a direct prefix-only re-render). This is a real behavioural
decision, not a detail: without it, pressing Space shows no `[PAUSED]` until the next unrelated
message.

---

## 7. Potential Problems / Risks (question H)

### 7.1 The `doWork` gate: not receiving is a total UI deadlock, not a stale frame

The interview says a naive "skip rendering" pause "blocks the collector on send and renders a STALE
frame on resume". The real failure is worse. Trace it:

- `collectStat` sends at `top/stat.go:72-79` and only *after* a successful send does it enter the
  select that receives `viewCh` (`stat.go:84-140`).
- If `doWork` stops receiving `statCh`, the collector parks on the send forever (until ctx cancel).
- `viewCh` is unbuffered (`config.go:49`) and **~14 key handlers push on it from the gocui
  goroutine** (`config_view.go:34, 50, 63, 81, 94, 107, 118, 200, 320, 366, 418, 451, 505, 535`,
  `extra.go:75, 104`, `verbose.go:26`).
- Therefore the first key the user presses after such a pause blocks the **gocui MainLoop
  goroutine** permanently: no redraw, no further keys, **Space itself cannot un-pause**. The
  process must be killed.

So "drain and discard" is not an optimisation, it is the only non-hanging shape. Write the test as
"the collector completes N sends while paused" (see §8).

### 7.2 `collectStat`'s unguarded error send

`top/stat.go:130`:

```go
statCh <- stat.Stat{Error: err}     // NOT inside a select with ctx.Done()
```

Every other send in that function is guarded (`stat.go:72-79`). This one is on the view-switch
re-initialisation path. It is a pre-existing hang risk on shutdown; the pause gate does not make it
worse (the gate keeps receiving), but any implementation that ever stops receiving turns this into
a second deadlock site. Note it in the spec so a later "optimisation" does not reintroduce it.

### 7.3 `uiExit` and the pager path

`doWork` returns on `<-app.uiExit` (`ui.go:125`) **without** cancelling ctx and **without**
`wg.Wait()` on its inner collector goroutine — the collector keeps running and parks on the send
until `mainLoop` cancels ctx after `MainLoop()` returns (`ui.go:99`). Consequences for pause:

- The pause **flag** survives (it lives on `config`, which survives the rebuild). ✔
- The **frame store must not be a `doWork` local** — see §2.3.
- After the rebuild the new `doWork` starts a **new `Collector`** (`stat.go:26`) with a fresh
  prefill (`stat.go:42`), so the first post-rebuild frame is a normal one; nothing about pause
  changes that.
- The explicit repaint must run **after** the new Gui's first `flush()` has created the views,
  otherwise `g.View("sysstat")` fails inside the closure and the returned error propagates out of
  `MainLoop` (`gui.go:377-379`) → yet another UI rebuild → an infinite rebuild loop. In practice
  `gocui.MainLoop` flushes once before entering the event loop (`gui.go:367`) and `Update`'s event
  is only consumed inside the loop (`gui.go:376`), so calling `printStat` at the top of `doWork` is
  safe by construction — but it is safe *by that ordering*, so the tech-spec should say so.

### 7.4 In-place truncation destroys the stored frame

`printDataCell` (`top/stat.go:1113`) rewrites the value with a `~` suffix. Repainting a stored
frame after a width increase or a wider resize shows permanently truncated text. Also debt **[029]**
(row values reach the terminal unsanitised) lives in the same function. Fix options: deep-copy the
frame at store time (§2.2), or make `printDataCell` non-mutating (truncate into a local). The second
is a one-line change and also removes the aliasing write into the collector's snapshot for
`DiffIntvl=[0,0]` views — but it edits a hot path that four tests pin (`stat_test.go:1269`).

### 7.5 The clock (repeat of §0)

`renderSysstat` prints `time.Now()` (`top/stat.go:277`). A repainted stored frame shows a live clock
over frozen stats unless a timestamp is stored and threaded through. Four existing tests call
`renderSysstat` directly (`stat_test.go:44, 85, 199, 431`) and would need the new argument.

### 7.6 gocui redraw semantics — what survives without a repaint

`flush()` (`gui.go:422-470`) redraws every view from its internal line buffer on every loop
iteration, and marks all views tainted on a size change (`gui.go:427-431`). Therefore:

- **Terminal resize while paused**: the frozen text stays on screen (no blank frame), but the
  dbstat column window was computed for the *old* width (`renderDbstat` → `visibleColumns`,
  `stat.go:727`) — a widened terminal shows a narrow table with stale edge markers until a repaint.
  So resize *should* repaint from the store, as the interview decided.
- **help/menu/dialog overlays**: they are separate views (`help.go:54`, `menu.go:116`,
  `dialog.go:172`) drawn on top; deleting them (`closeHelp` `help.go:76`, `menuClose`
  `menu.go:234`, `dialogClose` `dialog.go:258`) reveals the underlying views' *buffers*, which are
  intact. **No repaint needed for overlays.** This is worth stating: it removes three paths from
  the "must repaint" list.
- **UI rebuild (pager/editor/psql/UI error)**: new views, empty buffers → **blank screen** until
  something prints. This is the only path that *requires* the repaint.

### 7.7 Frames-every-tick assumptions in `top/`

Searched; three places assume periodicity, none breaks:

- `alignViewToResult` (`stat.go:670`) is idempotent once `Aligned` is set.
- The logtail branch tracks `config.logtail.Size` across frames (`stat.go:233-243`) — but `L` lifts
  the pause, so it is never repainted from a store.
- `firstTickHint` (`stat.go:149`) fires off the collector's flag; while paused the frame is
  discarded before `printStat`, so `collecting...` is simply not shown. Since `v` lifts the pause,
  this cannot be reached in a paused state.

### 7.8 Registered tech debt intersecting this feature

| Debt | Relevance | Suggested handling |
|---|---|---|
| **[027]** post-dialog cmdline messages never visible (`dialogFinish` writes twice) | If the Space handler is ever reached from a dialog-adjacent path, or if the pause handler prints two messages, the marker write is lost | **Do not add a second `printCmdline` in the pause path.** Do not attempt to fix [027] here |
| **[028]** verbose height-guard hint loses the `g.Update` race | Same root cause; a reminder that "one cmdline write per path" is the rule | Note only |
| **[029]** row values unsanitised in `printDataCell` | Same function that mutates the stored frame (§7.4); a paused screen holds a hostile value on screen indefinitely rather than for one tick | Mention in the spec's security note; sanitising is out of scope, but the *persistence* changes the severity argument slightly |
| **[016]** collectors swallow errors | Cited by the interview as the reason not to bypass errors — confirmed: a swallowed error arrives as a frame with empty fields, not `Error != nil` | Already settled: no bypass |
| **[030]** `profile.Test_profileLoop` flaky | Will show up in `make test` runs; not caused by this feature | Re-run before filing |

### 7.9 ADRs that constrain the implementation

- **[015] One cmdline composer fed by an ambient config** — the pause token must be read via
  `cmdlineTokens(cmdlineCfg)`; do not thread a second ambient or add a parameter to the writers.
- **[015] The clear timer renders, it does not erase** — do not add a pause-specific timer.
- **[009] Scroll offset on top.config, not on view.View** — the pause flag belongs on `top.config`
  for the same reason (it is a display mode, not a collector setting). Do **not** add
  `view.View.Paused`: it would ride `viewCh` and confuse `collectStat`'s change-detection ladder
  (`top/stat.go:86-122`), whose branch order is explicitly load-bearing.
- **[010] `view.Verbose` + `config.verbose` dual boolean** — a *counter-example* here: pause needs
  no collector home, so it must stay single-homed.

---

## 8. Existing Tests (question G)

### 8.1 What exists

| Area | File | Notes |
|---|---|---|
| cmdline composer | `top/ui_test.go:25-165` | `Test_composeCmdline` table (18 cases) **already contains a `[PAUSED]` single-variant token** (`ui_test.go:29`) and two-token cases (`:92-99`); `Test_composeCmdlineTwoTokens` (`:134`) is an explicit acceptance test for exactly this feature's token, asserting `"[PAUSED][F:datname] сообщение"` |
| token construction | `top/ui_test.go:168-304` | `filterToken` ladder, determinism, control-rune stripping |
| ambient config | `top/ui_test.go:308-365` | `cmdlineTokens` nil-safety, `setCmdlineConfig` with cleanup |
| dialog geometry | `top/dialog_test.go:45-290` | `dialogPromptFit`/`dialogInputX0` — these compose real prefixes, so they are sensitive to a wider prefix |
| render cores | `top/stat_test.go:864-1490` | `visibleColumns`, `printStatHeader/Data` against `bytes.Buffer`, `renderDbstat` auto-scroll one-shot; helpers `makeRenderConfig` (`:1094`) / `makeRenderResult` (`:1110`) |
| panels | `top/stat_test.go:44-808` | `renderSysstat`/`renderPgstat` compact + verbose, golden-ish substring assertions |
| handlers pushing on `viewCh` | `top/config_view_test.go` (all) | **The precedent for the goroutine-gate test**: each test spawns a goroutine that reads `config.viewCh`, asserts on the received view, then `close(config.viewCh)` (`config_view_test.go:33-40`) |
| layout geometry | `top/layout_test.go` | pure `topBandLayout` |

### 8.2 What does NOT exist

- **No test touches `doWork`, `mainLoop`, `collectStat`, `printStat` or `keybindings`.**
  (`grep -rn "doWork\|collectStat\|mainLoop" --include=*_test.go` → the only hit is a comment in
  `dialog_test.go:89`.) The keybinding table is entirely untested — adding a row breaks nothing,
  and conversely nothing will catch a wrong key constant (§5) except the stand run.
- No test constructs a `gocui.Gui` (it cannot be constructed headless), so `printStat` and the
  repaint path are unit-untestable as written.

### 8.3 What a pause gate would break

**Nothing, if the gate is added inside `doWork` and `cmdlineTokens` gains a token.** Specifically:

- No count-based test pins the number of `cmdlineTokens` entries; `Test_cmdlineTokens`
  (`ui_test.go:308-331`) asserts `assert.Empty` for a *bare* `newConfig()` and `[]cmdlineToken{want}`
  for a config with filters. **Both break the moment the pause token is unconditional.** They must
  stay green by construction — the token is emitted only when the flag is set — but the tests should
  gain a paused case.
- `Test_composeCmdline` / `Test_composeCmdlineTwoTokens` were written *for* this feature; they will
  pass unchanged and should be extended to assert the real `cmdlineTokens` output rather than a
  literal.
- `top/dialog_test.go` cases compose prefixes explicitly from `tokenOf(...)` values and do not read
  `cmdlineCfg`, so they are unaffected — but a **new** case ("prompt fits with `[PAUSED]` present at
  80 columns") is the cheap guard for §6.2.
- If the frozen-clock timestamp is threaded into `renderSysstat`, four tests need the new argument
  (`stat_test.go:44, 85, 199, 431`) — mechanical, but it is the largest test churn in the feature.

### 8.4 Testing the gate itself — the shape that works

`doWork` creates `statCh` internally (`ui.go:108`) and needs a live `*gocui.Gui` to render, so it
cannot be driven as-is. Two options:

- **(a) Extract the gate as a pure predicate/step**, e.g. `func (c *config) paused() bool` plus a
  small `gateFrame(app, s) bool`, and unit-test the predicate. Cheap, but proves nothing about the
  drain.
- **(b) Extract the select loop** into `runLoop(ctx, app, statCh <-chan stat.Stat, uiExit <-chan int)`
  with `doWork` as a thin wrapper that wires the real channels. Then the test spawns a fake
  "collector" that sends N frames on an unbuffered channel and asserts all N sends complete while
  paused — the direct expression of §7.1, and it mirrors the fake-consumer idiom already used
  throughout `config_view_test.go`. The render call must be behind a seam (a func field or a nil
  `app.ui` short-circuit — `writeCmdline` already tolerates a nil Gui, `ui.go:437-439`, but
  `printStat` does not: `app.ui.Update` on a nil `*gocui.Gui` panics).

**(b) is the recommended shape** and is what the interview's "unit test the gate on fake channels"
implies. It requires a small, contained refactor of `ui.go:106-135`.

---

## 9. Repaint paths — the complete enumeration (question C)

| Path | Trigger | Data source today | Blank if frames are discarded? | Needs explicit repaint? |
|---|---|---|---|---|
| Periodic frame | collector tick | `statCh` → `printStat` | — | — (this is what is gated) |
| Forced redraw via `viewCh` | `[`, `]`, `↑`, `↓`, `\` | collector produces an extra frame → `printStat` | **Yes — the key silently does nothing** | **Yes** |
| `layout()` | every `flush()` (`gui.go:434`) | none — only `SetView` + the verbose hint | No (views keep their buffers) | No |
| Terminal resize | termbox → `flush()` marks tainted (`gui.go:427`) | view line buffers | No, but the column window is stale for the new width | **Yes** (quality, not correctness) |
| Overlay open/close (`h`, menus, dialogs) | `SetView`/`DeleteView` | underlying buffers intact | No | No |
| `printCmdline` / clear timer | any message | `composeCmdline` only | cmdline only | No |
| **UI rebuild** (pager `l`/`C`/`~`/`G`, editor `E`, UI error) | `mainLoop` loop (`ui.go:48-50`) | brand-new empty views | **Yes — fully blank screen** | **Yes** |
| First frame after resume | Space | normal path | — | No |

Two paths must repaint from the store; the rest are free. That is a smaller surface than the
interview assumed for overlays, and a larger one than it assumed for `[`/`]`/`↑`/`↓`/`\`.

---

## 10. Shared Utilities worth reusing

| Utility | Location | Use here |
|---|---|---|
| `composeCmdline`, `renderCmdlineTokens`, `truncateRunes`, `appendCmdlineVariant` | `top/ui.go:257-382` | unchanged; the token is data |
| `cmdlineTokens` | `top/ui.go:406` | the single edit point for `[PAUSED]` |
| `printCmdline` / `printCmdlinePersist` | `top/ui.go:421, 428` | one call max per path (patterns.md) |
| `printStat` | `top/stat.go:157` | reuse verbatim as the repaint function |
| `topBandLayout` | `top/layout.go:41` | pure geometry, untouched |
| `makeRenderConfig` / `makeRenderResult` | `top/stat_test.go:1094, 1110` | build stored frames in tests |
| `uiGeneration` pattern | `top/ui.go:24-29, 465-475` | the documentation register for a new atomic |

---

## 11. Constraints & Infrastructure

- Go 1.25+, `gocui v0.5.0` (unmaintained fork-of-record for this project), `termbox-go v1.1.1`,
  `pgx/v5`, testify.
- `make test` runs with `-race`. A pause flag read from the worker goroutine and written from the
  gocui goroutine **will be reported by the race detector** unless it is `atomic.Bool` (or
  mutex-guarded) — this is not optional even though the value is a single bit.
- `make lint` = golangci-lint (errcheck, gocritic, gosimple, govet, ineffassign, revive,
  staticcheck, unused) + gosec; `govet` includes `copylocks` — see §4.1 for why `config` is safe.
- No env vars, no CI/CD, no migration, no recorded-format impact: the feature is TUI-only and adds
  no query. `record`/`report` are untouched (`view.View` gains no field).
- Manual verification per patterns.md §"Driving the TUI on a remote test stand": fresh `make build`,
  explicit binary path, `tmux new-session -d -s cap -x 190 -y 52`, plus a narrow run (`-x 60`) to
  exercise the token ladder, `capture-pane -e` only where attributes matter, and a second binary
  from `master` for regression separation.
- Help text is user-visible output: the interview made "document the lifting set" a requirement, so
  `help.go:10-48` changes are part of the deliverable, not an afterthought.

---

## 12. External Libraries — the APIs this feature depends on

**gocui v0.5.0** (`~/go/pkg/mod/github.com/jroimartin/gocui@v0.5.0`), no Context7 entry; read from
source:

- `Gui.Update(f func(*Gui) error)` (`gui.go:311`) — **spawns a goroutine per call** that pushes onto
  `userEvents`. An error returned by `f` propagates out of `MainLoop` (`gui.go:377-379`) and tears
  down the UI. Every repaint therefore carries UI-restart risk if it references a missing view.
- `Gui.MainLoop()` (`gui.go:351`) — flushes once *before* the event loop, then per iteration.
- `Gui.flush()` (`gui.go:422`) — calls every manager's `Layout`, then redraws each view from its
  buffer; marks all views tainted on a size change (`gui.go:427-431`).
- `Gui.SetView` (`gui.go:130`) — on an existing view only updates coordinates and sets `tainted`;
  **content is preserved**. Rejects `x0 >= x1` with `"invalid dimensions"` (the ADR [015] hazard).
- `Gui.SetKeybinding` / `keybinding.matchKeypress` (`keybinding.go:31`) / `matchView`
  (`keybinding.go:36`) — view-scoped dispatch, exact `key && ch && mod` match.
- `Key` constants incl. `KeySpace = Key(termbox.KeySpace)` (`keybinding.go:124`).

**termbox-go v1.1.1** — `KeySpace Key = 0x20` (`api_common.go:122`); the parser treats
`inbuf[0] <= KeySpace` as a functional key, emitting `Ch = 0, Key = Key(inbuf[0])`
(`termbox.go:575-580`). This is the authority for "bind `gocui.KeySpace`, never `' '`".

**sync/atomic** — `atomic.Bool` (`Load`/`Store`), matching the existing `atomic.Uint64`
(`top/ui.go:29`).

---

## 13. Open questions for the tech-spec

1. **Frame-store discipline**: mutex on `config` vs publish-through-`g.Update` (§2.3). The
   interview's "the flag is the only cross-goroutine race" does not survive contact with the store.
2. **Frozen clock**: thread a timestamp into `renderSysstat` (4 test signatures + 1 call site) or
   accept a live clock over frozen stats (contradicts interview Q3).
3. **Deep copy vs non-mutating `printDataCell`** (§2.2, §7.4) — the second is smaller and fixes a
   latent aliasing write, but touches a hot path with existing tests.
4. **Does the Space handler print a cmdline message?** If not, the `[PAUSED]` token will not appear
   until an unrelated write occurs (§6.3). Exactly one write per path either way.
5. **Repaint wiring for `[`/`]`/`↑`/`↓`/`\`** — they currently force redraws through `viewCh`, which
   the gate swallows (§5, caveat 1). Either they call the repaint directly, or the gate treats a
   discarded frame as a repaint trigger for the *stored* frame (simpler: after discarding, if a
   repaint was requested, re-render the store).
6. **Testability refactor of `doWork`** — extract the select loop so the drain can be proven (§8.4).
