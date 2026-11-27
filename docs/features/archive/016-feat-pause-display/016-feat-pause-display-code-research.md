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

---

## Implementation-level research (tech-spec phase)

**Updated: 2026-08-03.** Answers to the nine implementation questions raised while drafting the
tech-spec. Everything above stays valid, including the correction block. Where this section
contradicts §13 (open questions), this section settles it.
**Sources re-read for this pass:** `top/ui.go`, `top/stat.go`, `top/config.go`, `top/config_view.go`,
`top/dialog.go`, `top/keybindings.go`, `top/help.go`, `top/top.go`, `top/stat_test.go`,
`top/ui_test.go`, `top/dialog_test.go`, `top/config_view_test.go`, `internal/stat/stat.go`,
`internal/stat/log.go`, `internal/stat/postgres.go`, `gocui@v0.5.0/gui.go`.

---

### 14. Frame store discipline — settled: publish inside the render closure (option b)

#### 14.1 The deciding fact

The logtail buffer is born on the **gocui goroutine** and the frame arrives on the **worker
goroutine**:

- `readLogfileRecent` / `printLogtail` are called at `top/stat.go:227` and `top/stat.go:245`, both
  inside `printStat`'s `g.Update` closure (`top/stat.go:169-252`) → gocui goroutine.
- `stat.Stat` is received at `top/ui.go:128` in `doWork`'s select → worker goroutine.

So the two pieces of state that the spec requires to be stored **together** ("Показанные строки лога
хранятся вместе с замороженным кадром") are produced on two different goroutines.

- **Option (a), mutex on `top.config`:** the frame is written in the gate (worker goroutine, under
  the mutex), the logtail buffer is written in the render closure (gocui goroutine, also under the
  mutex). Two writers, two goroutines, one lock — the state is one object but it is genuinely shared,
  and `top/` gains its first mutex.
- **Option (b), publish inside the `g.Update` closure:** *both* writes happen at the bottom of the
  same closure, on the gocui goroutine. The store is never touched by any other goroutine, so it
  needs no primitive at all. **This is the only discipline that puts both halves in one place
  without a lock.**

Option (b) also makes the spec's own rule structural rather than asserted: §"Как должно работать"
step 3 says *"пока пауза выключена, сохранённым становится каждый отрисованный кадр"*. Under (b) the
store is written **by the renderer, at the moment it renders** — the stored frame is by construction
the frame that was on screen. Under (a) the store is written by the gate *before* rendering, so a
render that fails halfway (e.g. `g.View("dbstat")` error, `top/stat.go:191`) leaves the store
claiming something the screen never showed.

#### 14.2 Concrete shape

```go
// top/pause.go — owned by the gocui goroutine ONLY. Same discipline as cmdlineCfg (ui.go:18-21)
// and app.uiError (written/read in layout(), ui.go:188-191).
type frameStore struct {
    valid   bool
    s       stat.Stat
    at      time.Time   // frozen clock, see §20
    ltBuf   []byte      // last NON-EMPTY logtail buffer, see §17
    ltPath  string
}
```

Placement: `app.frame frameStore` next to `app.uiError` (`top/top.go:38-44`) — `app` survives the UI
rebuild (`mainLoop` replaces only `app.ui`, `top/ui.go:59`), which §2.3 requires. The pause **flag**
goes on `config` instead (`top/config.go:17-41`), because `cmdlineTokens(config *config)`
(`top/ui.go:406`) must read it. Splitting them is deliberate: the flag is cross-goroutine
(`atomic.Bool`), the store is gocui-only — they have different disciplines and should not share a
home or a comment.

The gate (`top/ui.go:128-129`) becomes discard-only:

```go
case s := <-statCh:
    if app.config.paused.Load() {
        repaintStored(app)   // §16, option B; the frame s is dropped, never stored
        continue
    }
    printStat(app, s, app.postgresProps)
```

The render closure publishes what it just rendered (`top/stat.go:169-252`, at the end, before
`return nil`):

```go
app.ui.Update(func(g *gocui.Gui) error {
    now := ...            // live: time.Now(); repaint: app.frame.at
    ... existing sysstat/pgstat/dbstat/extra rendering ...
    // publish: unconditional, because a repaint re-publishes the identical values
    app.frame.s, app.frame.at, app.frame.valid = s, now, true
    if len(buf) > 0 {     // mirrors printLogtail's own predicate, stat.go:1230
        app.frame.ltBuf, app.frame.ltPath = buf, app.config.logtail.Path
    }
    return nil
})
```

Note the publish is **unconditional** and needs no `if !paused` guard: while paused the only frames
that reach this closure are repaints of the stored frame itself, so the write is an identity. That
removes the failure mode the correction block §1 names (a discarded frame overwriting the store) by
construction rather than by a check.

#### 14.3 The happens-before edge that makes it race-free under `-race`

Two edges, both from the standard Go memory model, both already relied on by the existing code:

1. **Collector → worker:** `statCh <- stats` (`top/stat.go:73`) happens-before the corresponding
   receive `s := <-statCh` (`top/ui.go:128`). Everything the collector wrote into
   `Pgstat.Result.Values` is visible to the worker.
2. **Worker → gocui:** `g.Update(f)` (`gocui@v0.5.0/gui.go:311-313`) does
   `go func() { g.userEvents <- userEvent{f: f} }()`. The `go` statement happens-before the new
   goroutine's first instruction, and `g.userEvents <-` happens-before MainLoop's receive at
   `gui.go:376` (or `gui.go:398` in `consumeevents`), which then calls `ev.f(g)`. So every write the
   worker made before calling `Update` — including the capture of `s` — is visible inside the closure
   body running on the gocui goroutine.

Consequence: `app.frame` is written and read **exclusively** by the gocui goroutine (render closure,
`layout()`, key handlers) and never appears in any other goroutine's instruction stream. `-race`
sees no shared access to report. `writeCmdline` already documents exactly this pattern —
"Format the message in the caller's goroutine … No config field may be touched here; everything
below runs in the gocui MainLoop goroutine instead" (`top/ui.go:441-443`).

The **only** remaining cross-goroutine object is the pause flag: written by the `Space` handler
(gocui) and read by the gate (worker), hence `atomic.Bool`. Confirmed safe to embed in `config`:
`grep` finds no place in `top/` where a `config` is copied by value (all uses are `*config`;
`newConfig` returns a pointer, `top/config.go:44`; tests use `&config{...}` composite literals,
`top/stat_test.go:822, 830, 839, 851, 1102`), so `go vet`'s `copylocks` has nothing to flag.

**What the gate does in each discipline, side by side:**

| | Option (a) mutex | Option (b) publish-in-closure (**chosen**) |
|---|---|---|
| Gate body | `if paused { continue }` — and the *unpaused* branch must additionally take the lock to store the frame before calling `printStat` | `if paused { repaintStored(app); continue }` — nothing else, no store write at all |
| Frame written by | worker goroutine, under mutex | gocui goroutine, inside the render closure |
| Logtail written by | gocui goroutine, under the same mutex | gocui goroutine, same closure, same statement group |
| Primitives added | `sync.Mutex` + `atomic.Bool` | `atomic.Bool` only |
| "Stored == what was on screen" | asserted by convention | structural |

---

### 15. Resize detector — design

#### 15.1 gocui really does swallow the resize event

`handleEvent` (`gocui@v0.5.0/gui.go:410-419`) dispatches only `EventKey`/`EventMouse` to `onKey` and
`EventError` to the caller; `termbox.EventResize` falls into `default: return nil`. No keybinding,
no manager call, nothing reaches the application. The new size becomes visible **only** through
`flush()` (`gui.go:422-432`), which reads `termbox.Size()`, marks every view tainted when it differs,
assigns `g.maxX, g.maxY`, and *then* calls the managers' `Layout` (`gui.go:434-438`). Since
`Gui.Size()` returns `g.maxX, g.maxY` (`gui.go:100-102`), the `maxX, maxY := app.ui.Size()` already
present at `top/ui.go:144` is guaranteed to be the **new** size. `layout()` is therefore the only
possible detector site, exactly as Risk 6 of the user-spec states.

#### 15.2 Where it goes and what it stores

Inside the closure returned by `layout(app)` (`top/ui.go:143-238`), **at the end, after the "extra"
block, immediately before `return nil` (`top/ui.go:237`)**. Two reasons for the position:

- All four/five views (`sysstat` `:157`, `pgstat` `:171`, `cmdline` `:182`, `dbstat` `:198`, `extra`
  `:222`) have been created by then, so a repaint queued from here cannot hit `ErrUnknownView`.
- The degenerate-geometry guard at `top/ui.go:148-150` (`maxX == 0 || maxY == 0` → return an error)
  sits *before* it, so a post-pager zero size is never recorded as "seen" and never triggers a
  repaint into an invalid geometry.

State: two closure-local ints, in the same register as the existing `verboseTooShortShown`
(`top/ui.go:141`) — that variable is the precedent for per-Gui closure state, including the fact that
`layout(app)` is re-invoked on every rebuild (`top/ui.go:62`) and therefore starts fresh.

```go
return func(_ *gocui.Gui) error {
    maxX, maxY := app.ui.Size()
    if maxX == 0 || maxY == 0 { return fmt.Errorf("") }
    ... existing SetView plumbing, unchanged ...

    // Resize detector. gocui delivers no resize event (gui.go:410-419); the new size is visible
    // only here. lastX/lastY start at 0 on every new Gui, so the first layout of a rebuilt UI also
    // fires — which is what repaints the frozen frame after a pager/editor return.
    if maxX != lastX || maxY != lastY {
        lastX, lastY = maxX, maxY          // recorded BEFORE the request: see 15.4
        if app.config.paused.Load() {
            repaintStored(app)
        }
    }
    return nil
}
```

#### 15.3 Requesting the repaint without recursing into flush — `g.Update` from inside `Layout` is safe

**Verified in the v0.5.0 source, not assumed.** `Gui.Update` (`gui.go:311-313`) is three lines:

```go
func (g *Gui) Update(f func(*Gui) error) {
    go func() { g.userEvents <- userEvent{f: f} }()
}
```

It touches no `Gui` field, takes no lock (v0.5.0's `Gui` has no mutex at all), and hands the send to
a **new goroutine**. `g.userEvents` is buffered with capacity 20 (`gui.go:83`). The `Layout` callback
runs inside `flush()` on the MainLoop goroutine (`gui.go:435`, reached from `gui.go:367` or
`gui.go:384`). Therefore:

- No reentrancy: `Update` never calls `Layout`, `flush`, or `draw`.
- No deadlock even if `userEvents` is full — the *spawned* goroutine parks on the send, not the
  MainLoop goroutine, and MainLoop drains it at `gui.go:376`/`gui.go:398` on its very next iteration.
- The closure runs one event-loop iteration later, so the repaint lands in the *next* `flush()`.

(The alternative — rendering synchronously at the end of `Layout`, which `flush` would draw in the
same pass since managers run at `gui.go:434-438` before the view draw at `gui.go:439-465` — is
rejected: an error returned from `Layout` aborts `flush`, propagates out of `MainLoop`, and rebuilds
the UI. Deferring through `Update` does not remove that hazard but it does keep the render out of the
layout contract; see 15.4 for the rule that actually removes it.)

#### 15.4 Why it cannot produce an infinite rebuild loop

Three independent arguments, all needed:

1. **The repaint chain terminates after exactly one extra iteration.** `lastX/lastY` are assigned
   *before* the repaint is requested, and a repaint does not change the terminal size. The `flush()`
   that draws the repaint calls `Layout` again (`gui.go:435`), which now compares equal and requests
   nothing. One resize → one repaint → done. Note the *first* `flush()` of a new Gui always fires the
   detector (`lastX/lastY` are 0), which is the desired post-rebuild repaint, and it too fires exactly
   once.
2. **The repaint closure must never return a non-nil error.** This is the load-bearing rule. An error
   returned from a `g.Update` closure propagates out of `MainLoop` (`gui.go:377-379`) → `mainLoop`
   stores `app.uiError` and rebuilds the Gui (`top/ui.go:80-103`, `:48-50`) → `layout(app)` is
   re-created with `lastX/lastY == 0` → the detector fires again → if the repaint errors again, the
   cycle repeats. It is not literally infinite — `errorRate.check(1s, 5)` (`top/ui.go:92-95`) aborts
   the program after 5 errors in a second — but "pgcenter exits with *too many UI errors*" is not an
   acceptable outcome. **The repaint closure must swallow its render errors and `return nil`**,
   exactly as the cmdline clear timer already does for a missing view (`top/ui.go:482-485`:
   *"A missing view means the line is no longer relevant — there is nobody to report the error to, so
   return quietly"*). This is also the mitigation the user-spec's Risk 3 asks the tech-spec to record.
3. **The two error sources named in Risk 3 are removed at the source, not caught.** (i) Missing view:
   the detector sits after all `SetView` calls (15.2), and the `extra` branch of the repaint is gated
   on the same `ShowExtra > CollectNone` condition (`top/stat.go:201`) that creates the view
   (`top/ui.go:221`), so they cannot disagree. (ii) Logfile read: the paused repaint does not open,
   stat or read the file at all (§17), so `readLogfileRecent`'s error return
   (`top/stat.go:1208-1211, 1219-1222`) is unreachable while paused.

**Interaction with the verbose height-guard hint.** If `config.verbose && !expanded`
(`top/ui.go:211`) in the same `Layout` pass, `printCmdline` is called there too. Two `g.Update`
closures in one pass have unspecified relative order (`gui.go:308-310` documents this) — the
[028] failure mode. It is harmless for the marker: both closures compose the prefix through
`cmdlineTokens(cmdlineCfg)` (`top/ui.go:453`), so `[PAUSED]` is rendered whichever one wins; only
the *message* is at stake, and that is pre-existing debt, not something this feature adds.

---

### 16. Repaint mechanism — option B works, with one addition and one caveat

#### 16.1 Does a `viewCh` push actually produce a frame promptly? — Yes. Trace

`collectStat`'s post-send select is `top/stat.go:84-140`. On `case v = <-viewCh` (`:85`) every path
ends in a fresh collection:

| Branch | Line | What happens |
|---|---|---|
| refresh changed | `:93-96` | `continue` → loop top → `c.Update` (`:66`) → `statCh <- stats` (`:73`) |
| ShowExtra changed | `:99-103` | `ToggleCollectExtra`, `continue` → same |
| Verbose-only toggle | `:109-112` | `continue` → same |
| CollectExtra changed | `:119-122` | `c.Reset()`, falls through ↓ |
| **fall-through (all five render-only keys)** | `:124-133` | `ticker.Stop()`; `c.Reset()`; `c.Update(...)` **whose result is discarded** (only `err` is used, `:128-131`); `continue` → loop top → `c.Update` (`:66`) → `statCh <- stats` (`:73`) |

So **yes: every `viewCh` push causes an immediate collect-and-send, not a wait for the ticker.**
`[`, `]`, `↑`, `↓`, `\` (`top/config_view.go:63, 81, 94, 107, 200`) all take the fall-through row —
two collections and one send per keypress. Latency is one-to-two SQL round trips, independent of the
refresh interval. Option B is therefore mechanically sound: while paused, that emitted frame reaches
the gate (`top/ui.go:128`), is discarded, and the discard is what repaints the store.

Nor can the push block the UI while paused: the gate keeps receiving, so the collector is never
parked on `statCh <- stats` for longer than one cycle, and the handler's unbuffered `viewCh` send
completes within one collection — exactly as today.

#### 16.2 Option B vs option A

**Recommended: option B for the five keys + one explicit repaint for the filter dialog.**

- Option B costs **one rule in the gate** and leaves `scrollLeft`/`scrollRight`/`increaseWidth`/
  `decreaseWidth`/`clearFilters` (`top/config_view.go:59, 73, 87, 100, 193`) **completely
  unchanged** — five handlers not touched, which matters because `top/config_view_test.go` pins each
  of them (`:79, :113, :308, :341, :486, :519, :563`) through the fake-consumer idiom.
- Option A means editing all five handlers plus the dialog branch, and every edit must be careful not
  to double-write the cmdline ([027]).

**But `setFilter` must NOT be converted into a `viewCh` push.** `setFilter` (`top/config_view.go:124-145`)
is called from `dialogFinish` (`top/dialog.go:217`) and pushes nothing today — deliberately, because
filtering is render-time (`top/stat.go:743, 1051-1061`). Adding a push would send it down the
fall-through row above, which calls `c.Reset()` (`top/stat.go:127`). After a reset the next frame's
delta is computed by `calculateDelta` against a snapshot taken milliseconds earlier while the divisor
is the configured interval — `itv := int(refresh / time.Second)` (`internal/stat/stat.go:294`) is
derived arithmetically, **never measured** — so the frame shows near-zero rates for one tick. That is
a visible one-frame **dip** imported into the live filter path, i.e. a regression outside the
feature. (The same dip already happens on `[`/`]`/`↑`/`↓`/`\` today; that is pre-existing behaviour
worth recording as debt, not worth spreading.)

So the filter path gets an explicit repaint instead — one call, on the gocui goroutine, in
`dialogFinish`'s `dialogFilter` branch (`top/dialog.go:216-217`) or right after the switch, guarded
by `paused && app.frame.valid`. It reads the store on the goroutine that owns it (§14.3), so it needs
no synchronisation, and it adds no `printCmdline` call — `dialogFinish` already emits its message at
`top/dialog.go:242`.

#### 16.3 What option B implies that is worth stating

- **While paused, the store is repainted once per refresh interval** (every discarded tick), not only
  on demand. Idempotent — the render is a pure function of the store after §19 makes `printDataCell`
  non-mutating — but it means a resize *is* corrected within one refresh interval even without the
  §15 detector. The detector still earns its place because `z` allows intervals up to 300 s
  (`top/config_view.go:528`), where "within one interval" means five minutes.
- **The repaint must skip two things `printStat` does today:** the first-tick hint
  (`top/stat.go:165-167`, which would emit a second cmdline write) and the logfile-reading branch
  (`top/stat.go:226-249`). This is the correction block's point 3, and it is why the repaint is a
  *variant* of the render closure rather than `printStat` verbatim. Cleanest shape: extract the
  closure body into `renderFrame(g *gocui.Gui, app *app, f frameStore, live bool) error`, with
  `printStat` calling it with `live=true` and `repaintStored` with `live=false`.

---

### 17. Logtail buffer storage

#### 17.1 Where the state is today: nowhere in the application

This is the finding that shapes everything else. `readLogfileRecent` (`top/stat.go:1202-1226`)
returns `(info.Size(), nil, nil)` when the file has not changed or is empty (`:1213-1216`).
`printLogtail` (`top/stat.go:1229-1245`) wraps its entire body in `if len(string(buf)) > 0` — so on
a nil buffer it prints nothing **and does not even call `v.Clear()`** (the `Clear` is *inside* the
guard, `:1231-1232`).

Therefore the lines the user sees on a quiet log are held by **the gocui view's own line buffer**,
nothing else. There is no application-side copy of the displayed logtail today. Two consequences:

- The correction block's point 4 is confirmed: storing "the buffer of the frame on which `Space` was
  pressed" yields `nil` on a quiet log, and the panel would repaint empty.
- It also explains the pre-existing behaviour that the panel goes blank after any UI rebuild until
  the log next changes — the view buffer is destroyed and only a *changed* file refills it.

The only related application state is `app.config.logtail` — `stat.Logfile{Path string; File *os.File;
Size int64}` (`internal/stat/log.go:15-19`), read and written at `top/stat.go:227, 233, 235, 243, 245`,
all inside the render closure (gocui goroutine).

#### 17.2 What must be captured

The two values `printLogtail` consumes, and only those:

- `ltBuf []byte` — the exact `buf` returned by `readLogfileRecent` (`top/stat.go:227`), captured
  **only when non-empty**, using the same predicate `printLogtail` itself uses (`len(...) > 0`,
  `top/stat.go:1230`), so store and screen can never disagree about what "shown" means.
- `ltPath string` — `app.config.logtail.Path` at that moment, because it is printed as the panel's
  header line (`top/stat.go:1234`) and `Reopen` can change it (`internal/stat/log.go:48`).

`Size` is **not** captured: it is change-detection bookkeeping, never rendered.

Capture point: `top/stat.go:245`, right at the `printLogtail` call, inside the render closure — the
same statement group as the frame publish (§14.2), which is precisely what puts "the shown log lines"
and "the frame" in one place with no lock.

#### 17.3 How the repaint renders it without touching the file

The repaint's `case stat.CollectLogtail:` replaces the whole of `top/stat.go:227-248` with a single
call:

```go
case stat.CollectLogtail:
    // Frozen: no os.Stat, no Logfile.Read, no Reopen, no logtail.Size write. This is what removes
    // the second infinite-rebuild source named in the user-spec's Risk 3.
    if err := printLogtail(v, app.frame.ltPath, app.frame.ltBuf); err != nil { ... }
```

Note the repaint deliberately does **not** call `v.Clear()` itself: `printLogtail` clears only when
it has content (`top/stat.go:1231-1232`), so an empty store leaves whatever is on the view. After a
rebuild that is an empty view — the spec's "показывать нечего" case — and after an overlay close it
is the intact previous content. Both correct.

One cosmetic limitation to record: `readLogfileRecent` sizes its read from `v.Size()`
(`top/stat.go:1204-1206`), so the stored buffer was cut for the geometry in force when it was read.
Repainting it into a *taller* terminal shows fewer lines than would fit. Not a defect; the file is
re-read on resume.

#### 17.4 `config.logtail.Size` while paused, and rotation on resume

`logtail.Size` is written at exactly one place, `top/stat.go:243`, inside the branch the paused
repaint skips. So **it freezes at the value of the last live frame** and is never advanced during the
pause. Behaviour on resume, per case:

| Case at resume | `info.Size()` vs frozen `logtail.Size` | Result |
|---|---|---|
| Log grew during the pause | `>` | Normal read (`top/stat.go:1219`); the panel catches up in one frame with everything written during the pause. Correct. |
| Log rotated, new file still smaller than the frozen size | `<` | The existing rotation detector fires (`top/stat.go:233-240`): `v.Clear()` + `logtail.Reopen(app.db, VersionNum)` re-resolves the path via `GetPostgresCurrentLogfile` (`internal/stat/log.go:38-51`) and reopens. **Correct, and it works precisely because the frozen size is the pre-rotation one.** |
| Log rotated, new file already grown past the frozen size | `>=` | No `Reopen`. `logfile.Read` keeps reading the **old descriptor** (`l.File` still points at the rotated-away inode, `internal/stat/log.go:61`), so the panel shows stale lines until the next rotation. |

The third row is the miss window the correction block flags. It **exists today** — it is one refresh
interval wide — and a long pause widens it to the length of the pause. The correct behaviour on
resume is "let the existing size-based detector run against the frozen size", which is what happens
with zero extra code. Closing the widened window properly needs an identity check (inode via
`os.SameFile`, or mtime) rather than a size comparison; that is a separate change and, on the
evidence here, out of this feature's scope. It should be named in the tech-spec as an accepted
consequence, not silently inherited.

---

### 18. `[PAUSED]` restoration after a UI rebuild

#### 18.1 The exact insertion point

`top/ui.go:182-192`. The `cmdline` branch's `if err != nil` block is entered **only when `SetView`
returned `ErrUnknownView`**, i.e. only when the view was just *created* — once per Gui
(`gocui@v0.5.0/gui.go:138-151`: an existing view returns `nil`). That makes it the natural
"the cmdline is blank, put the marker back" hook, and it is already used for exactly that purpose
by the saved-UI-error write (`top/ui.go:188-191`).

```go
v, err = app.ui.SetView("cmdline", -1, cmdlineY0, maxX, cmdlineY1)
if err != nil {
    if err != gocui.ErrUnknownView {
        return fmt.Errorf("set cmdline view on layout failed: %w", err)
    }
    // show saved error to user if any
    if app.uiError != nil {
        printCmdline(app.ui, "%s", app.uiError)
        app.uiError = nil
    } else if app.config.paused.Load() {
        // Prefix-only line: composeCmdline renders the tokens and nothing else. Independent of the
        // frame store on purpose — the marker must survive a rebuild even when Space was pressed
        // before the first frame and there is nothing to repaint (user-spec, Ключевые компоненты).
        printCmdline(app.ui, "")
    }
}
```

#### 18.2 Why it satisfies "even when there is NO stored frame"

The trigger is the *view's creation*, not the store's contents. It fires on every rebuild path —
pager `l` (`top/pglog.go:33`), config viewer `C` (`top/pgconfig.go:55`), editor `E`
(`top/pgconfig.go:98`), `psql` `~` (`top/psql.go:21`), query report `G` (`top/report.go:148`) and the
UI-error restart — regardless of `app.frame.valid`. This is the failure the correction block's point 5
names ("хранилище пусто → выходим" loses the marker) and it is structurally avoided.

#### 18.3 Interaction with the [015] one-write-per-path rule

- The two writes are in an `if/else if`, so this code path still emits **exactly one** `printCmdline`.
  No new violation of the patterns.md rule or debt [027].
- When `app.uiError != nil` the marker still appears anyway: `writeCmdline` composes
  `composeCmdline(cmdlineTokens(cmdlineCfg), msg, width)` (`top/ui.go:453`), so the token prefix
  precedes any message. The `else` branch exists only for the *no-error* rebuild, which is the common
  case (pager return).
- The concurrent verbose height-guard hint (`top/ui.go:211-215`) can still fire in the same pass; as
  argued in §15.4 that costs at most the message, never the marker.

#### 18.4 Interaction with the generation-gated clear timer (`top/ui.go:465-475`)

- **This write arms no timer.** `writeCmdline` arms only when `arm && msg != ""` (`top/ui.go:461`),
  and here `msg == ""`. There is no new timer to gate.
- **A stale timer cannot clobber it.** Timers armed against the previous Gui captured the old
  `uiGeneration` (`top/ui.go:465`); `mainLoop` bumps the counter at `top/ui.go:57`, *before*
  `SetManagerFunc` (`:62`) and long before the new Gui's first `Layout` runs — so any pre-rebuild
  timer that fires later returns at `top/ui.go:473-475` without writing. Ordering is load-bearing and
  holds.
- **A timer armed after the restoration is harmless and in fact helpful:** when it fires it re-renders
  the prefix-only line (`top/ui.go:489`), i.e. `[PAUSED]` again — §6.3's conclusion, unchanged.

Precedent that `printCmdline(g, "")` is a supported call: `top/dialog.go:205` uses it, and
`top/ui_test.go:365` asserts it does not panic.

---

### 19. Non-mutating `printDataCell`

#### 19.1 Current code — `top/stat.go:1103-1119`, verbatim

```go
func printDataCell(w io.Writer, s stat.Stat, config *config, rownum, i int) error {
	// truncate values that are longer than column width
	valuelen := len(s.Result.Values[rownum][i].String)
	if valuelen > config.view.ColsWidth[i] {
		width := config.view.ColsWidth[i]
		if width <= 0 {
			return fmt.Errorf("zero or negative width, skip")
		}

		// truncate value up to column width and replace last character with '~' symbol
		s.Result.Values[rownum][i].String = s.Result.Values[rownum][i].String[:width-1] + "~"
	}

	// print value
	_, err := fmt.Fprintf(w, "%-*s", config.view.ColsWidth[i]+2, s.Result.Values[rownum][i].String)
	return err
}
```

#### 19.2 Minimal non-mutating rewrite

```go
func printDataCell(w io.Writer, s stat.Stat, config *config, rownum, i int) error {
	// Read once into a local: the value is FORMATTED here, never edited in place. Editing the
	// snapshot would make a frozen frame lose its original text on the first render (a widened
	// column could then never show it again) and, for DiffIntvl==[0,0] views, would write into the
	// array the collector still holds as prevPgStat.
	value := s.Result.Values[rownum][i].String

	// truncate values that are longer than column width
	if len(value) > config.view.ColsWidth[i] {
		width := config.view.ColsWidth[i]
		if width <= 0 {
			return fmt.Errorf("zero or negative width, skip")
		}

		// truncate value up to column width and replace last character with '~' symbol
		value = value[:width-1] + "~"
	}

	// print value
	_, err := fmt.Fprintf(w, "%-*s", config.view.ColsWidth[i]+2, value)
	return err
}
```

Deliberately byte-based, exactly as today (`len`, byte slicing). Converting to runes would change the
output for multi-byte values and belongs to a different change; the [029] sanitisation debt likewise
stays untouched.

#### 19.3 Which tests pin this, and do any need updating

- **`Test_printStatData_truncation`** — `top/stat_test.go:1269-1284` (doc comment `:1265-1268`). It
  seeds a 10-char value into a width-5 column (`:1273`), renders through `printStatData` (`:1277`),
  and asserts on the **buffer**: `Contains "abcd~"` (`:1282`), `NotContains "abcde"` (`:1283`). Both
  are statements about the writer output, not about the struct. **Passes unchanged; no update
  needed.**
- No other test asserts on `s.Result.Values` after a render. The tests that render the same `s` more
  than once — the alignment invariant test (`top/stat_test.go:1247-1262`, `makeRenderResult(7, 3)`
  with width 10) and the windowed tests (`:1141, :1169, :1183, :1226`) — use `rR-cC` values 5 bytes
  long against widths of 10, so truncation never triggers and the mutation was invisible there.
- **New test to add:** render at width 5, then render the *same* `stat.Stat` at width 10 and assert
  the full value reappears (this is the user-spec AC "Расширение колонки во время паузы не приводит к
  навсегда обрезанному значению"), or simply assert `s.Result.Values[0][0].String` is unchanged after
  a render.

#### 19.4 Does the change alone remove the aliasing write?

**Yes.** `grep` over `top/` (excluding tests) for any assignment into the result values finds exactly
one hit: `top/stat.go:1113` — the line being removed. Nothing else in `top/` writes into
`s.Result.Values`, `.Cols` or the `sql.NullString` cells. (`alignViewToResult`, `top/stat.go:670-678`,
*replaces* `config.view.Cols`/`ColsWidth`; it does not write through the result's slices.
`printDbstat` sets `s.Error = nil` at `top/stat.go:687`, but `s` is a by-value parameter and `Error`
is a scalar, so that write is confined to the callee's copy.)

So after this one-line change the render path performs **zero writes** into the collector-owned
backing array, which is what §2.2 identified as the hazard for `DiffIntvl == [0,0]` views
(`internal/view/view.go:43, 305, 317, 329, 352` — including `activity`, the default screen). Chain
re-verified: `calculateDelta` returns `delta = curr` for `[0,0]` (`internal/stat/postgres.go:596`),
`delta.sort` sorts it in place (`:599`), and `c.currPgStat` becomes `c.prevPgStat` on the next tick
(`internal/stat/stat.go:435-436`) — so the sent frame's `Values` array is the collector's `prev`
array. The collector reads only `prevPgStat.Activity.Calls` from it afterwards
(`internal/stat/stat.go:308`), never the `Values`, which is why no race is observable today; removing
the write means the aliasing is now merely a shared read, and **no deep copy of the frame is needed**
at store time. That settles §13.3 in favour of the non-mutating rewrite (smaller, and it removes the
cause instead of copying around it — matching the project's structural-fix preference).

---

### 20. Frozen clock

#### 20.1 Is there a collection time in `stat.Stat`? — No, one must be introduced

`Stat` / `System` / `Pgstat` (`internal/stat/stat.go:35-54`) carry no `time.Time`. The only
`time.Time` in the package is `verboseCollectState.dbSizeLastRun` (`internal/stat/stat.go:99`),
which is the verbose latency-guard clock and unrelated. `Collector.Update` never needs one: the delta
interval is arithmetic, `itv := int(refresh / time.Second)` (`internal/stat/stat.go:294`).

Two options, and the spec picks one for us:

- **(a) Add a field to `stat.Stat`.** Safe format-wise — `stat.Stat` is referenced only from `top/`
  (`grep` for `stat.Stat` outside tests hits only `top/stat.go` and `top/ui.go:108`; `record`/`report`
  do not serialise it). But the user-spec's Ограничения say *"Все изменения — внутри пакета `top/`"*,
  so a cross-package field is gratuitous.
- **(b) Stamp it in `top/` at render time. Recommended.** `now := time.Now()` once at the top of
  `printStat` (`top/stat.go:157`), passed down to the sysstat renderer and stored in `app.frame.at`
  in the same closure (§14.2). The divergence from true collection time is one render latency
  (sub-millisecond), far inside the spec's stated tolerance of one refresh interval — and because the
  same value is both rendered and stored, the frozen clock shows **exactly** the time that was on
  screen when `Space` was pressed.

#### 20.2 Exact signature change

```go
// top/stat.go:258 — thin wrapper
func printSysstat(v *gocui.View, s stat.Stat, verbose bool, local bool, dataDir string, refresh time.Duration, now time.Time) error

// top/stat.go:269 — writer-based core
func renderSysstat(w io.Writer, s stat.Stat, verbose bool, local bool, dataDir string, refresh time.Duration, now time.Time) error
```

Body change, `top/stat.go:276-278`:

```go
_, err = fmt.Fprintf(w, "pgcenter: %s, refresh: %ds, load average: %.2f, %.2f, %.2f\n",
    now.Format("2006-01-02 15:04:05"), int(refresh/time.Second),   // was: time.Now().Format(...)
    s.LoadAvg.One, s.LoadAvg.Five, s.LoadAvg.Fifteen)
```

`renderSysstatVerbose` (`top/stat.go:343`) prints no time and is unchanged.

#### 20.3 Production call sites

Two, both trivially threaded:

- `top/stat.go:259` — `printSysstat` → `renderSysstat`: forward `now`.
- `top/stat.go:175` — inside `printStat`'s render closure: pass the frame's timestamp
  (`time.Now()` for a live frame, `app.frame.at` for a repaint). If the render closure is extracted
  as `renderFrame(g, app, f frameStore, live bool)` (§16.3), the timestamp travels inside `f` and
  this call site reads `f.at`.

An alternative worth naming in the tech-spec: rather than a seventh parameter, pass the whole
`frameStore` (frame + timestamp) into the renderers. That is a larger refactor touching every render
signature; the extra parameter is the minimal change and matches how `refresh` was threaded in a
previous feature.

#### 20.4 Test call sites that need the new argument — five, not four

| File:line | Context |
|---|---|
| `top/stat_test.go:60` | `Test_renderSysstat_compact` (`:44`) |
| `top/stat_test.go:97` | `Test_renderSysstat_refreshFormat` (`:85`), inside the table loop |
| `top/stat_test.go:192` | helper `verboseSysstatLines` (`:190-195`) — **one edit covers eight tests**: `:199, :223, :247, :292, :316, :352, :392, :413` |
| `top/stat_test.go:440` | `Test_renderSysstat_compactUnchanged` (`:431`), compact buffer |
| `top/stat_test.go:441` | same test, verbose buffer |

Only one of them looks at the timestamp at all: `Test_renderSysstat_compact` matches line 1 with a
regexp (`top/stat_test.go:66-69`, `^pgcenter: \d{4}-\d{2}-\d{2} …$`), so it stays green with any
valid time — and it is the natural place to add the new deterministic assertion the user-spec asks
for ("отрисовка с сохранённой отметкой времени"): pass a fixed `time.Date(...)` and assert the exact
literal, which is impossible today.

---

### 21. Testability of the gate

#### 21.1 Concrete signature

`doWork` (`top/ui.go:106-135`) cannot be driven as-is: it creates `statCh` internally (`:108`),
spawns the real `collectStat` (`:111-114`) and seeds `viewCh` (`:117-118`). Extract only the select
loop, leaving the wiring in `doWork`:

```go
// statLoopExit tells doWork WHY the loop returned. The distinction is load-bearing: see 21.4.
type statLoopExit int

const (
    exitUI statLoopExit = iota // app.uiExit — pager/editor path
    exitCtx                    // ctx.Done()
)

// statLoop is doWork's select loop, with the channels and the render step injected so it can be
// driven without a terminal or a Postgres connection.
func statLoop(
    ctx context.Context,
    uiExit <-chan int,
    statCh <-chan stat.Stat,
    paused *atomic.Bool,
    render func(stat.Stat),   // real: printStat(app, s, app.postgresProps)
    repaint func(),           // real: repaintStored(app)
) statLoopExit {
    for {
        select {
        case <-uiExit:
            return exitUI
        case s := <-statCh:
            if paused.Load() {
                repaint()     // the frame is DROPPED, never stored (correction block §1)
                continue
            }
            render(s)
        case <-ctx.Done():
            return exitCtx
        }
    }
}
```

and `doWork` becomes:

```go
func doWork(ctx context.Context, app *app) {
    var wg sync.WaitGroup
    statCh := make(chan stat.Stat)
    wg.Add(1)
    go func() { collectStat(ctx, app.db, statCh, app.config.viewCh); wg.Done() }()

    app.config.view.Refresh = defaultRefresh
    app.config.viewCh <- app.config.view
    app.config.view.Refresh = 0

    if statLoop(ctx, app.uiExit, statCh, &app.config.paused,
        func(s stat.Stat) { printStat(app, s, app.postgresProps) },
        func() { repaintStored(app) },
    ) == exitCtx {
        wg.Wait()
    }
}
```

#### 21.2 What the test drives

No `*gocui.Gui` anywhere: `statCh` is a plain channel and `render`/`repaint` are function values, so
`app.ui.Update` is never reached. (This seam is mandatory, not stylistic: `writeCmdline` tolerates a
nil Gui (`top/ui.go:437-439`) but `printStat` does not — `app.ui.Update` on a nil `*gocui.Gui` panics
at `gui.go:312`.) The test supplies:

- an **unbuffered** `chan stat.Stat` — the buffering matters, see 21.3;
- a fake collector goroutine shaped like the real one (`top/stat.go:72-79`): send, and only *after*
  the send returns, record the completion;
- `paused` pre-set to `true`;
- `render`/`repaint` closures that count calls (guarded by a mutex or by counting on the loop
  goroutine and reading after it returns, so the test itself is `-race` clean);
- a `ctx` and a `uiExit` it controls.

The fake-consumer/producer idiom already exists in the package: `top/config_view_test.go:33-40`
(goroutine reads `config.viewCh`, asserts, then closes), reused at `:500`, `:549` and throughout.

#### 21.3 How it proves drain-and-discard

The user-spec's AC is *"сборщик успешно завершает не менее пяти отправок подряд, пока активна пауза"*.
On an **unbuffered** channel a completed send is proof that a receive happened — that is the whole
argument, and it is exactly the property §7.1 says the naive implementation destroys.

```
sent := 0
go func() {
    for i := 0; i < 5; i++ {
        statCh <- stat.Stat{}   // blocks until statLoop receives
        sent++                  // only reachable if the send completed
    }
    close(done)
}()
// ... run statLoop in another goroutine ...
<-done                          // fails by timeout if the gate ever stopped receiving
assert.Equal(t, 5, sent)
assert.Equal(t, 0, renderCalls) // discarded, not rendered
assert.Equal(t, 5, repaintCalls)// option B: each discard repaints the store
```

Then flip `paused` to `false`, send one more frame, and assert `renderCalls == 1` — the resume half.

#### 21.4 The trap the extraction must not introduce

Today `wg.Wait()` is called **only** on the `ctx.Done()` branch (`top/ui.go:130-132`) and **not** on
the `uiExit` branch (`top/ui.go:125-127`). That asymmetry is deliberate: on the pager path ctx is not
cancelled, so `collectStat` stays parked on `statCh <- stats` (`top/stat.go:73`) with nobody
receiving — an unconditional `wg.Wait()` after the extraction would hang the pager path forever.
Hence `statLoop` returns a reason and `doWork` waits only for `exitCtx`. A second cheap test pins
this: send `uiExit` while a producer send is still pending and assert `statLoop` returns.

Worth carrying into the tech-spec as a note: `top/stat.go:130` (`statCh <- stat.Stat{Error: err}`) is
an **unguarded** send on the view-switch re-init path — the one send in `collectStat` not wrapped in a
`select` with `ctx.Done()`. The gate keeps receiving, so this feature does not make it worse, but any
future "optimisation" that stops receiving turns it into a second deadlock site (§7.2).

---

### 22. Test inventory this change touches

| # | Test / helper | File:line | Effect of this feature |
|---|---|---|---|
| 1 | `Test_renderSysstat_compact` | `top/stat_test.go:44`, call `:60` | **Needs the new `now` arg.** Its line-1 regexp (`:66-69`) still passes; best place to add the deterministic frozen-clock assertion |
| 2 | `Test_renderSysstat_refreshFormat` | `top/stat_test.go:85`, call `:97` | **Needs the new arg.** No timestamp assertion |
| 3 | helper `verboseSysstatLines` | `top/stat_test.go:190-195`, call `:192` | **Needs the new arg — one edit.** Covers `:199, :223, :247, :292, :316, :352, :392, :413` |
| 4 | `Test_renderSysstat_compactUnchanged` | `top/stat_test.go:431`, calls `:440, :441` | **Needs the new arg** on both calls |
| 5 | `Test_printStatData_truncation` | `top/stat_test.go:1269-1284` | **Passes unchanged** — asserts on the writer output (`:1282-1283`), not on the struct. Add a companion non-mutation case |
| 6 | Windowed/alignment render tests | `top/stat_test.go:1141, 1169, 1183, 1226, 1247` | **Unaffected** — 5-byte values against width-10 columns never hit the truncation branch |
| 7 | `alignViewToResult` tests (`&config{...}` literals) | `top/stat_test.go:822, 830, 839, 851` | **Compile-safe** with an `atomic.Bool` field on `config` (composite literals, no value copies) |
| 8 | `makeRenderConfig` / `makeRenderResult` | `top/stat_test.go:1095, 1114` | **Unchanged**, and directly reusable to build stored frames for the new pause tests |
| 9 | `Test_cmdlineTokens` | `top/ui_test.go:308`; "bare config" `:314`; "with filters" `:319-329` | **Stays green** (a bare config is not paused) but is now under-specified — **add a paused subtest** and a paused+filter ordering subtest |
| 10 | `Test_setCmdlineConfig` | `top/ui_test.go:334`, `assert.Len(..., 1)` at `:350` | **Stays green** — the config it builds is not paused. Would break only if the pause token were emitted unconditionally |
| 11 | `Test_composeCmdline` | `top/ui_test.go:25`; pause token at `:29`; two-token cases `:92-99` | **Passes unchanged** — already written against a literal `[PAUSED]` |
| 12 | `Test_composeCmdlineTwoTokens` | `top/ui_test.go:134-153` | **Passes unchanged.** Should be upgraded to build the token via the real producer instead of the literal at `:135` |
| 13 | `Test_printCmdlineNilGui` | `top/ui_test.go:359-368`, incl. `printCmdline(nil, "")` at `:365` | **Unaffected**, and it is the precedent that the empty-message write used in §18 is supported |
| 14 | Dialog geometry tests | `top/dialog_test.go:45, 90, 105, 118, 152, 179, 213, 231, 263`; prefixes built via `tokenOf` `:30-39`, maps at `:119, :196, :293` | **All unaffected** — they build token slices literally and never read `cmdlineCfg`. **Add one case** with a `[PAUSED]`+filter prefix at 80 columns (the ~8-column shortening the user-spec accepts) |
| 15 | `viewCh`-pushing handler tests | `top/config_view_test.go:79 (scrollLeft), :113 (scrollRight), :308 (increaseWidth), :341 (decreaseWidth), :486/:519/:563 (clearFilters), :15/:47 (orderKey*), :375 (switchSortOrder)` | **All unaffected under option B** — those handlers are not edited. They would all need review under option A |
| 16 | `Test_setFilter` | `top/config_view_test.go:410-437` | **Unaffected** — `setFilter` itself is not edited; the repaint is added in `dialogFinish` (`top/dialog.go:216-217`) |
| 17 | Other `newConfig()` users | `top/verbose_test.go:20`, `top/config_test.go:9`, `top/top_test.go:13, 22`, `top/signal_test.go:63, 146, 180`, `top/config_view_test.go` (24 sites) | **Compile-safe**; nothing asserts on the config's field set |
| 18 | Keybinding table | `top/keybindings.go:17-92` | **No test exists at all** (`grep` for `keybindings` in `*_test.go` → nothing). Adding the `Space` row breaks nothing — and nothing will catch a wrong key constant except the stand run. §5's `gocui.KeySpace`-not-`' '` finding therefore has no automated guard |
| 19 | `doWork` / `collectStat` / `mainLoop` / `printStat` | — | **No test touches any of them** (`grep -rn "printStat(\|doWork\|collectStat\|mainLoop" --include=*_test.go top/` → one comment at `top/dialog_test.go:89`). The `statLoop` extraction of §21 creates the first |
| 20 | `top/help.go:10-48` (`helpTemplate`) | — | **No test.** The new `Space` line and the lifting-set line are verified by reading, not by assertion |

**Net breakage: five mechanical call-site edits (rows 1-4) and nothing else.** Everything else is
either unaffected or an addition. That is the smallest possible blast radius for the frozen clock,
and it is the argument for threading a timestamp rather than storing a pre-rendered sysstat string.
