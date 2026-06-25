# Decisions Log: Overview Dashboard (verbose mode)

Отчёты агентов о выполнении задач. Каждая запись создаётся агентом, выполнившим задачу.

---

## Task 01: Net-new formatting helpers

**Status:** Done
**Commit:** 43c7229
**Agent:** fmt-dev
**Summary:** Добавлены три чистые функции в `internal/pretty/pretty.go` — `Ceil` (округление вверх через `math.Ceil`), `ReserveWidth` (фиксированная ширина `%*d`, никогда не усекает) и `RateUnit` (динамический суффикс с одношаговым промоушеном единицы при переполнении резерва). Делители: диск `MB/s→GB/s` = 1024 (бинарный, консистентно с `Size`); сеть `Mbps→Gbps` = **1000** — десятичный SI, так как Mbps/Gbps по сетевой конвенции десятичные, а сама величина Mbps в панели nicstat считается из `Rbytes/1024/128` (`top/stat.go:741`), и переход между Mbps↔Gbps — чисто единичный масштаб порядка. `pretty.Size` не тронут.
**Deviations:** Нет. (Шорткаты/отложенные находки: нет; `make lint` не прогнан — golangci-lint/gosec не установлены в окружении, `go vet` и `gofmt` чисты, lint остаётся на CI.)
**Tech debt:** Нет.

**Reviews:**

*Round 1:*
- dev-code-reviewer: approved, 3 minor (опциональные косметики, не применялись) → [010-feat-overview-dashboard-task-01-dev-code-reviewer-round1.json](010-feat-overview-dashboard-task-01-dev-code-reviewer-round1.json)
- dev-test-reviewer: needs_improvement, 2 major + 3 minor → [010-feat-overview-dashboard-task-01-dev-test-reviewer-round1.json](010-feat-overview-dashboard-task-01-dev-test-reviewer-round1.json)

*Round 2 (после исправлений, commit 7b1b72c):*
- dev-test-reviewer: passed, 0 findings → [010-feat-overview-dashboard-task-01-dev-test-reviewer-round2.json](010-feat-overview-dashboard-task-01-dev-test-reviewer-round2.json)

**Verification:**
- `go test ./internal/pretty/...` → ok (TestSize, TestCeil, TestReserveWidth, TestRateUnit, TestRateUnit_boundary, TestRateUnit_property)
- `go build ./...` → OK
- `go vet ./internal/pretty/...` + `gofmt -l` → чисто

---

## Task 03: io.Writer refactor of printSysstat/printPgstat

**Status:** Done
**Commit:** 7762a7e
**Agent:** render-dev
**Summary:** Поведенчески-сохраняющий enabling-рефакторинг: `printSysstat`/`printPgstat` (`top/stat.go`) стали тонкими обёртками над `*gocui.View`, делегирующими в новые `renderSysstat(w io.Writer, …)`/`renderPgstat(w io.Writer, …)` — по образцу `printDbstat → renderDbstat`. Все `fmt.Fprintf`/`Fprintln` перенесены дословно (изменён только получатель `v → w`), форматные строки/порядок аргументов/ANSI-коды/переводы строк не тронуты. Compact-вывод байт-в-байт идентичен, что зафиксировано writer-based golden-тестами (`Test_renderSysstat_compact`/`Test_renderPgstat_compact`) на `bytes.Buffer`. Verbose-строки не добавлялись — это Task 8.
**Deviations:** Нет. (`make lint` не прогнан — golangci-lint в окружении несовместим с конфигом проекта (migration v2); `go vet` и `gofmt -l` чисты, lint остаётся на CI.)
**Tech debt:** Нет.

**Reviews:**

*Round 1:*
- dev-code-reviewer: approved, 1 minor (стилистическая асимметрия `var err error`, унаследована из исходника — оставлена как есть для byte-identical рефакторинга) → [010-feat-overview-dashboard-task-03-dev-code-reviewer-round1.json](010-feat-overview-dashboard-task-03-dev-code-reviewer-round1.json)
- dev-test-reviewer: passed, 1 minor (pgstat line1 — tie-test на `formatInfoString`, намеренно по TDD-anchor; литеральный pinning в `Test_formatInfoString`) → [010-feat-overview-dashboard-task-03-dev-test-reviewer-round1.json](010-feat-overview-dashboard-task-03-dev-test-reviewer-round1.json)

**Verification:**
- `go test ./top/...` → ok (включая новые golden-тесты и существующие)
- `go build ./...` → OK
- `go vet ./top/...` + `gofmt -l top/stat.go top/stat_test.go` → чисто

---

## Task 04: GUC + data_directory reads

**Status:** Done
**Commit:** 8ca1188
**Agent:** guc-dev
**Summary:** `SelectCommonProperties` (`internal/query/common.go`) расширен пятью net-new чтениями в конце SELECT-списка: `max_worker_processes`/`max_logical_replication_workers`/`max_parallel_workers` (`::int`), `wal_segment_size` обёрнут в `pg_size_bytes(current_setting('wal_segment_size'))::int8` для получения int64-байт (приём из `wal.go:6`, а не pretty-строка), и `data_directory` (text). Соответствующие пять полей добавлены в `PostgresProperties` и пять scan-целей — в `.Scan(...)` функции `GetPostgresProperties` (`internal/stat/postgres.go`) строго в lockstep-порядке с SELECT (8 старых + 5 новых = 13). `TestGetPostgresProperties` расширен 4 ассертами; `GucMaxLogicalReplicationWorkers` намеренно не ассертится на значение (дефолт 4, но 0 допустим — важен только успех скана). `GucMaxPrepXacts` (declared-but-never-scanned placeholder) не тронут. `data_directory` нигде не логируется — только пробрасывается в поле структуры.
**Deviations:** `gofmt -w` при выравнивании struct-комментариев также нормализовал две предсуществующие неровные строки комментариев в `Test_parseDuration` (postgres_test.go) — out-of-scope, но это чистая gofmt-нормализация (никакого изменения поведения), оставлена ради gofmt-чистоты файла. `make lint` не прогнан — golangci-lint в окружении — битый симлинк; вместо него прогнаны `gosec` (0 issues), `go vet` и `gofmt -l` (чисто), полный lint остаётся на CI.
**Tech debt:** Нет.

**Reviews:**

*Round 1:*
- dev-code-reviewer: approved, 0 critical/major, 2 minor (информативные, действий не требуют) → [010-feat-overview-dashboard-task-04-dev-code-reviewer-round1.json](010-feat-overview-dashboard-task-04-dev-code-reviewer-round1.json)
- dev-security-auditor: approved, 0 findings (A03 injection — статический const, ввод не интерполируется; data_directory не утекает в логи) → [010-feat-overview-dashboard-task-04-dev-security-auditor-round1.json](010-feat-overview-dashboard-task-04-dev-security-auditor-round1.json)
- dev-test-reviewer: passed, 0 findings (litmus 4/4) → [010-feat-overview-dashboard-task-04-dev-test-reviewer-round1.json](010-feat-overview-dashboard-task-04-dev-test-reviewer-round1.json)

Исправлений не потребовалось — round 2 не запускался.

**Verification:**
- `go test ./internal/query/... ./internal/stat/...` → ok (live-PG кластер доступен)
- `go build ./...` → OK
- `gosec ./internal/query/... ./internal/stat/...` → 0 issues; `go vet` + `gofmt -l` → чисто

---

## Task 02: Verbose toggle plumbing

**Status:** Done
**Commit:** 2af9dd7 (impl) + 389ea24 (round 1 fixes)
**Agent:** toggle-dev
**Summary:** Реализована control-plane обвязка verbose-режима (Decision 2): добавлены `View.Verbose bool` (`internal/view/view.go`) и `config.verbose bool` (`top/config.go`). Новый `top/verbose.go::toggleVerbose(app)` зеркалит флаг по паттерну `showExtra` (write-into-all-views: каждая запись `config.views` + `config.view` + `config.verbose`), пушит вью в `viewCh` и печатает статус через `printCmdline` (`Verbose mode: on/off`). Хоткей `v` навешен на `sysstat` (`top/keybindings.go`), добавлена строка help (`top/help.go`). Ключевая, рисковая часть — в `collectStat()` (`top/stat.go`): seed `prevVerbose := v.Verbose` рядом с `prevCollectExtra`, и ранний `if prevVerbose != v.Verbose { … continue }` в ветке `case v = <-viewCh:` — размещён **до обоих** Reset-путей (условный CollectExtra-Reset и безусловный Reset), так что verbose-only toggle не вайпит prev-снапшот и не блэнкит дельты CPU/mem/load на один кадр. Персистентность через переключение экранов — бесплатно: флаг просто никогда не зануляется в `viewSwitchHandler` (в отличие от `scrollOffset`).
**Deviations:** `make lint` не прогнан — golangci-lint не установлен в окружении (`command not found`); вместо него `go vet ./top/... ./internal/view/...` и `gofmt -l` чисты, полный lint остаётся на CI. Шорткатов и отложенных находок нет.
**Tech debt:** Нет.

**Reviews:**

*Round 1:*
- dev-code-reviewer: approved, 0 critical/major, 2 minor (опциональные) → [010-feat-overview-dashboard-task-02-dev-code-reviewer-round1.json](010-feat-overview-dashboard-task-02-dev-code-reviewer-round1.json)
- dev-test-reviewer: passed, 3 minor (litmus 2/2, ничего блокирующего) → [010-feat-overview-dashboard-task-02-dev-test-reviewer-round1.json](010-feat-overview-dashboard-task-02-dev-test-reviewer-round1.json)

Применены два сходящихся minor-найдинга (commit 389ea24): grouping-комментарий о порядке веток в `collectStat()` (защита load-bearing инварианта от будущих перестановок) и явный ассерт персистентности через реальный `viewSwitchHandler` в `Test_toggleVerbose`. Найдинг про ассерт cmdline-сообщения и про отсутствие изолированного теста ветки `collectStat` — отклонены как scoped (прецеденты `showExtra`/`toggleIdleConns` cmdline не ассертят; у `collectStat` нет изолированного харнесса по спеку, проверка — в ручном QA Final Wave). Round 2 не запускался: оба ревьюера уже approved, правки — некостыльное hardening без изменения поведения.

**Verification:**
- `go test ./top/...` → ok (`Test_toggleVerbose` зелёный, регрессий по keybindings/view-count нет); `go test ./internal/view/...` → ok
- `go build ./...` → OK
- `go vet ./top/... ./internal/view/...` + `gofmt -l` → чисто

---

## Task 05: New aggregate SQL queries + verbose collection

**Status:** Done
**Commit:** ca65a37 (impl) + 70e0256 (round 1 fixes)
**Agent:** agg-dev
**Summary:** Добавлен слой данных для пяти verbose-строк правой панели `pgstat`. Новый `internal/query/overview.go` держит версионно-корректные агрегаты: `workload`/`databases`/`databases-size`/`workers`/`wal-size`/`send-recv` — статический SQL; `replication-lag` и `replication-slots` — рекавери-aware шаблоны (`{{.WalFunction1/2}}`), прогоняются через `query.Format` с `opts` из `NewOptions`. `bgwr/ckpt` переиспользует готовый `SelectStatBgwriterQuery(version)` без новой SQL. Плоская структура `PgstatOverview` + `collectOverviewStat(db, props, itv, prev)` (`internal/stat/postgres.go`) повторяют цепочку `collectActivityStat`: каждый независимый агрегат — собственный `QueryRow`, Go-side rate `(curr-prev)/itv`. tps = `(Δcommit+Δrollback)/itv`; `others` = интервальная дельта `deadlocks+conflicts+checksum_failures` (без `/s`); cache hit ratio — per-interval `Δhit/Δ(hit+read)` через чистый хелпер `cacheHitRatio()` (деление на ноль → `n/a`, не `NaN`). `bgwr/ckpt` сканируется by-name (раскладка колонок различается PG14-16/17/18). Недоступные сигналы (нет репликации, нет слотов, `archive_mode=off`, нехватка прав, первый тик) помечаются sentinel-флагами (`*Valid`/`sql.NullInt64`), отличимыми от реального `0`. Сбор проведён в `Collector.Update` под гейтом `if view.Verbose` — prev читается из `c.currPgStat.Overview` ДО сдвига `prevPgStat = currPgStat`; compact-путь не тронут.

**SECURITY:** дорогие/привилегированные агрегаты — каждый своим `QueryRow`: `sum(pg_database_size)` (`OverviewDatabasesSize`) и archiving backlog (`pg_ls_dir('pg_wal/archive_status')`, требует `pg_monitor`). Ошибка одного (42501 / `archive_mode=off`) деградирует только своё поле в `n/a`, не роняя сэмпл; `collectOverviewStat` вообще не возвращает error, сырой текст ошибки PG (с путями) нигде не пробрасывается и не логируется. SQL — статические `const`-литералы, единственная динамика — подстановка имён WAL-функций по `version`/`recovery` (не пользовательский ввод).

**Deviations:**
1. **`databases` разнесён на два запроса.** User-spec показывает строку `databases` как единое `XGB per Y databases, growth/s, cache hit ratio`, но AC/Decision 6/10 требуют, чтобы дорогой `sum(pg_database_size)` шёл отдельным `QueryRow`. Поэтому добавлены ДВА запроса: `OverviewDatabases` (count + cache-counters, дёшево, всегда заполняется) и `OverviewDatabasesSize` (размер, отдельный `QueryRow`, только он управляет `TotalSizeValid`). Падение size-агрегата теперь не гасит count/cache. Изначально (commit ca65a37) они были склеены — исправлено по critical-находке code-review в round 1.
2. **recovery-`t` тестируется substring-only.** Шаблонные WAL-fn запросы на standby-ветке (`pg_last_wal_receive_lsn`) проверяются только подстановкой через `Format`, без живого исполнения: fixture-кластеры 21914-21918 — primaries, и запуск standby-функции на primary вернёт ошибку. Исполнение — на recovery `f`.
3. **`make lint` не прогнан** — golangci-lint/gosec не установлены в окружении (как в Task 01/02); вместо них `go vet ./internal/query/... ./internal/stat/...` и `gofmt -l` чисты, полный lint остаётся на CI.

**Tech debt:** Нет.

**Reviews:**

*Round 1:*
- dev-code-reviewer: changes_required — 1 critical (`pg_database_size` не вынесен в отдельный `QueryRow`), 2 major, 2 minor → [010-feat-overview-dashboard-task-05-dev-code-reviewer-round1.json](010-feat-overview-dashboard-task-05-dev-code-reviewer-round1.json)
- dev-security-auditor: approved — 0 critical/major, 1 minor (осознанное тотальное глушение ошибок ради требования «не светить пути») → [010-feat-overview-dashboard-task-05-dev-security-auditor-round1.json](010-feat-overview-dashboard-task-05-dev-security-auditor-round1.json)
- dev-test-reviewer: needs_improvement — 4 major (Query без Scan; tps/others тавтология; backlog-деградация не проверена через collect; division-by-zero под флакающим `if`), 4 minor → [010-feat-overview-dashboard-task-05-dev-test-reviewer-round1.json](010-feat-overview-dashboard-task-05-dev-test-reviewer-round1.json)

Все critical/major приняты и исправлены (commit 70e0256): split `databases` на два `QueryRow`, чистый хелпер `cacheHitRatio()` + табличный тест на деление на ноль, query-тесты теперь `QueryRow().Scan(...)` в точные приёмники collect-функции, exact-formula ассерт tps/others против синтетического prev (`itv=2`), degradation-тест ассертит backlog/size, `Test_OverviewBgwriterColumns` проверяет by-name колонки на PG14-18, verbose-off проверяет нетронутость compact-пути. Отклонены как scoped: security-minor (осознанный компромисс по спеку), recovery-`t` live-исполнение (fixture-primaries).

*Round 2:*
- dev-code-reviewer: approved — все находки round 1 устранены, регрессий нет → [010-feat-overview-dashboard-task-05-dev-code-reviewer-round2.json](010-feat-overview-dashboard-task-05-dev-code-reviewer-round2.json)
- dev-test-reviewer: passed — все 4 major закрыты, ассерты осмысленные; 2 оставшихся minor (division-by-zero блок под `if` в live-тесте — дублируется безусловным `Test_cacheHitRatio`; recovery-`t` substring-only) не блокируют → [010-feat-overview-dashboard-task-05-dev-test-reviewer-round2.json](010-feat-overview-dashboard-task-05-dev-test-reviewer-round2.json)

Round 3 не запускался: оба ревьюера approved/passed.

**Verification:**
- `go test ./internal/...` → ok (live-PG кластер доступен; новые тесты зелёные на PG 14-18, пути деградации покрыты)
- `go build ./...` → OK
- `go vet ./internal/query/... ./internal/stat/...` + `gofmt -l` → чисто

---

## Task 06: Verbose-aware layout() geometry

**Status:** Done
**Commit:** 82853f6 (impl) + 3aef6b3 (round 1 fixes)
**Agent:** layout-dev
**Summary:** Извлечена чистая функция `topBandLayout(verbose, maxY) -> (sysstatY1, pgstatY1, cmdlineY0, cmdlineY1, dbstatY0 int, expanded bool)` в новый `top/layout.go` (Decision 3) — целочисленная арифметика без gocui/app, по прецеденту `visibleColumns`. Compact-ветка (`verbose=false` ИЛИ height-guard) воспроизводит исторические литералы байт-идентично (`4/4/3/5/4`). Verbose растит панели асимметрично: `sysstatY1 = 4+3`, `pgstatY1 = 4+5`; cmdline/dbstat расчищаются под более высокую (`pgstat`) панель: `bandTop = max-1` → `cmdline 8/10`, `dbstatY0 = max+1 = 10` — так dbstat теряет строки сверху, а не снизу. Height-guard: verbose раскрывается только если `dbstatY0 + header(1) + >=1 data row <= maxY-1` (порог `maxY>=13` при `dbstatY0=10`); иначе возврат compact-координат с `expanded=false`. `layout(app)` в `top/ui.go` теперь вызывает функцию один раз и кормит результат в четыре `SetView` вместо хардкод-литералов; блок `extra` не тронут. Когда verbose запрошен, но guard сработал, печатается одноразовая подсказка `terminal too short for verbose mode` через `printCmdline` — анти-спам через closure-captured флаг `verboseTooShortShown`, эмиссия только на флипе состояния, не на каждом кадре. `config.verbose` читается в gocui-handler goroutine (как `view.ShowExtra`) — гонки нет. Табличный тест `Test_topBandLayout` (`top/layout_test.go`) покрывает compact / verbose / height-guard / verbose-zero-maxY / boundary с обеих сторон порога, gocui-free.

**Deviations:**
1. **`make lint` не прогнан** — golangci-lint установлен в окружении, но его версия несовместима с конфигом репозитория (`unsupported version of the configuration`), как в Task 01/02/05. Вместо него `go vet ./top/...`, `gofmt -l` и `gosec ./top/...` чисты; полный golangci-lint остаётся на CI.
2. **Подсказка height-guard транзиентна (осознанный компромисс).** `printCmdline` авто-очищает cmdline через 2с, после чего verbose молча неактивен без индикации (отмечено code-reviewer как optional minor). Оставлено как есть: спек явно требует «не спамить каждый кадр», а `printCmdline` — указанный спеком канал. Постоянный индикатор — вне скоупа задачи.
3. **+3/+5 высоты и порог `maxY>=13` подтверждены, но не верифицированы визуально** на живых строках рендеринга — фактическая печать verbose-строк sysstat (+3) / pgstat (+5) добавляется в Task 08; высота band должна совпасть с числом реально печатаемых строк. Это проверяется в ручном QA (verify: user) и при интеграции Task 08.

**Tech debt:** Нет.

**Reviews:**

*Round 1:*
- dev-code-reviewer: approved — 0 critical/major, 2 minor (оба optional) → [010-feat-overview-dashboard-task-06-dev-code-reviewer-round1.json](010-feat-overview-dashboard-task-06-dev-code-reviewer-round1.json)
- dev-test-reviewer: passed — 0 critical/major, 2 minor (litmus 5/5), pyramid healthy → [010-feat-overview-dashboard-task-06-dev-test-reviewer-round1.json](010-feat-overview-dashboard-task-06-dev-test-reviewer-round1.json)

Применены три из четырёх находок (commit 3aef6b3): именованная константа `minDbstatRows = 2` на месте сравнения (само-документирующий off-by-one), репурпозинг избыточной строки `height-guard` (была `maxY=12`, дубль `boundary-fallback`) на отдельное `maxY=5`, и новый кейс `verbose-zero-maxY` (`maxY=0` → compact, защита инварианта «никаких сломанных координат»). Отклонён один optional-minor: транзиентность подсказки height-guard — осознанный компромисс по спеку (см. Deviation 2). Round 2 не запускался: оба ревьюера approved/passed в round 1, правки — некостыльное hardening без изменения поведения.

**Verification:**
- `go test ./top/...` → ok (`Test_topBandLayout` зелёный, 6 подкейсов; регрессий нет)
- `go build ./...` → OK; полный `go test ./...` → все пакеты ok
- `go vet ./top/...` + `gofmt -l` + `gosec ./top/...` → чисто

---

## Task 07: All-three verbose system collection branch

**Status:** Done
**Commit:** aec74e9 (impl) + 8716d3c (round 1 fixes)
**Agent:** collect-dev
**Summary:** В `Collector.Update` (`internal/stat/stat.go`) добавлена verbose-ветка СТРОГО ПОСЛЕ нетронутого мьютекс-`switch c.config.collectExtra` (R1): под гейтом `if view.Verbose` собираются все три системных источника (`Diskstats`/`Netdevs`/`Fsstats`) каждый тик через переиспользование `collectDiskstats`/`collectNetdevs`/`collectFsstats` (та же `%util`-математика, что и полные боковые панели — Decision 5). Каждый источник под `== nil` guard'ом — уже наполненный активной боковой панелью не пересобирается. Per-source ошибка НЕ прерывает сэмпл (паттерн `if x, err := c.collect*(db); err == nil`, без `return s, err`): одна сбойная подсистема оставляет источник `nil` (Task 8 отрисует `n/a`), остальные собираются. Добавлены два приватных forward-compatible поля на `Collector`: `verboseFirstTick bool` (сигнал композеру Task 8 рисовать `n/a` на тике без валидного prev) и `prevVerboseActive bool` (был ли verbose активен на прошлом тике). Логика re-arm: при входе в ветку `c.verboseFirstTick = !c.prevVerboseActive` (покрывает и самый первый тик, и каждый OFF→ON re-enable БЕЗ смены view), в конце ветки `c.prevVerboseActive = true`, в `else`-ветке `c.prevVerboseActive = false`. Механизм НЕ зависит от `c.Reset()` (Decision 2: `toggleVerbose` его не зовёт) и НЕ опирается на `s.Diskstats == nil` (на первом тике срез уже наполнен нулевой дельтой — `collect*` делают `prev=curr` при несовпадении длин). Compact-путь и боковые панели не тронуты.
**Deviations:** `make lint` не прогнан — golangci-lint в окружении отсутствует/несовместим с конфигом (как в Task 01/02/05/06); вместо него `go vet ./internal/stat/...` и `gofmt -l` чисты, полный lint остаётся на CI. Шорткатов и отложенных находок нет.
**Tech debt:** Нет.

**Reviews:**

*Round 1:*
- dev-code-reviewer: approved — 0 critical/major, 2 minor (оба опциональные, на стыке с будущими задачами: молчаливый drop ошибки источника — availability-трекинг отложен на Task 9 `verboseCollectState`; `== nil` vs `len()==0` — рендер `n/a` Task 8 должен опираться на `len()==0`) → [010-feat-overview-dashboard-task-07-dev-code-reviewer-round1.json](010-feat-overview-dashboard-task-07-dev-code-reviewer-round1.json)
- dev-test-reviewer: passed — litmus 9/9, pyramid healthy, 3 minor → [010-feat-overview-dashboard-task-07-dev-test-reviewer-round1.json](010-feat-overview-dashboard-task-07-dev-test-reviewer-round1.json)

Применены три test-minor (commit 8716d3c): добавлен под-кейс сосуществования с боковой панелью (`collectExtra=CollectDiskstats` + `Verbose=true` → Diskstats через switch под `== nil` guard'ом, Netdevs/Fsstats через verbose-ветку), прямой ассерт `prevVerboseActive == false` на OFF-тике (пин else-ветки re-arm), и per-source диагностические сообщения через `assert.NotEmpty`. Отклонены два code-minor — оба осознанно отложены на Task 8/9 (availability-маркер и `len()==0`-рендер — вне скоупа этой задачи, реализация корректна). Round 2 не запускался: оба ревьюера approved/passed в round 1, правки — некостыльное hardening теста без изменения поведения.

**Verification:**
- `go test ./internal/stat/...` → ok (live-PG кластер доступен; все три источника наполняются, флаг set/clear, OFF→ON re-arm без Reset, coexistence-кейс зелёные; существующие `TestCollector_Update`/`TestCollector_collectDiskstats` не падают)
- `go build ./...` → OK
- `go vet ./internal/stat/...` + `gofmt -l` → чисто

---

## Task 08: Verbose row composers (both panels)

**Status:** Done
**Commit:** 9268de9 (impl) + d0696f7 (round 1 fixes) + 54695df (review reports)
**Agent:** rows-dev
**Summary:** Финальный рендер-слой verbose-режима. Внутри writer-ядер `renderSysstat`/`renderPgstat` (`top/stat.go`, Task 3), под флагом verbose, добавлены 3 системные строки (`iostat`/`nicstat`/`filesyst`) и 5 pgstat-строк (`workload`/`databases`/`workers`/`replication`/`bgwr/ckpt`). Сигнатуры обёрток/ядер расширены: `printSysstat`/`renderSysstat(w, s, verbose, local, dataDir)`, `printPgstat`/`renderPgstat(w, s, props, db, verbose)`; verbose приходит из `app.config.verbose`, `local` из `app.db.Local`, `dataDir` из `props.DataDirectory`. `iostat`/`nicstat` выбирают устройство с макс. `%util`/`Utilization` среди активных (фильтры `Completed != 0` / `Packets != 0`, как в `printIostat`/`printNetdev`), читая `Util`/`Utilization` AS-IS из `count*Usage` — НЕ пересчитывая (Decision 5). `nicstat` rMbps/wMbps реплицируют print-time конверсию `printNetdev`: `Rbytes/1024/128`; `err/coll` = `(Rerrs+Terrs)`/`Tcolls`. `filesyst` — ФС каталога данных через новый чистый матчер `MatchDataDirFs(dataDir, fss, local)` (`internal/stat/fsstat.go`): longest-mount-prefix по границе компонента пути (`/var` не съедает `/variable`), при `local` сначала `filepath.EvalSymlinks(filepath.Clean(...))`, удалённо — нерезолвленный путь; любой сбой (broken symlink, EACCES, no-match, пустой dataDir) → `ok=false` → строка `n/a`, без паники и без логирования сырого пути; `mounted` усечён до 10 рун. pgstat-строки — из `PgstatOverview` (Task 5) с форматтерами Task 1 (`pretty.Ceil`/`ReserveWidth`/`Size`); недоступные сигналы (sentinel-флаги `*Valid` / `HasPrev=false`) → литерал `n/a`, отличимый от `0`; падение одного источника не гасит остальные строки. First-tick `n/a` для системных строк — по флагу коллектора, НЕ по `len(slice)==0`. Бюджеты разрядов системных строк по user-spec (devices=2, util=3, MB/s&Mbps=4, r/s&w/s=5, err&coll=4). compact-вывод обеих панелей при verbose=false байт-в-байт неизменён (golden-тесты).

**Deviations:**
1. **Затронут `internal/stat/stat.go` сверх заявленного списка файлов задачи** (задача разрешала только `top/stat.go`, `top/stat_test.go`, `internal/stat/fsstat.go`+тест). Причина — необходимая и неустранимая: флаг first-tick `verboseFirstTick` Task 7 — приватное поле `Collector`, а collector и renderer общаются ИСКЛЮЧИТЕЛЬНО через `stat.Stat` по каналу `statCh` (разные горутины). Без моста приватный флаг не доедет до `renderSysstat`. Добавлено публичное поле `System.VerboseFirstTick bool` (+ одна строка присваивания `s.VerboseFirstTick = c.verboseFirstTick` в verbose-ветке `Update`) — минимальный мост, точно соответствующий замыслу спека («n/a по флагу коллектора, не по `len(slice)==0`»). Подтверждено всеми тремя ревьюерами как корректное, минимальное, обоснованное отклонение.
2. **Матчер экспортирован: `matchDataDirFs` → `MatchDataDirFs`** (TDD-якорь спека называл его в lowercase, подразумевая unexported). Причина — композер живёт в пакете `top` и зовёт матчер через границу пакета, поэтому функция и тесты `Test_MatchDataDirFs_*` следуют экспортированному имени. Покрытие идентично; задокументировано комментарием в тесте.
3. **`rateField` в `top/stat.go` дублирует overflow/divisor-логику `pretty.RateUnit`** (отличие — r/w-префикс ставится МЕЖДУ цифрами и единицей, как в user-spec раскладке `1135 rMB/s`). Рефактор `pretty.RateUnit` под общий хелпер потребовал бы правки `internal/pretty/pretty.go` (вне разрешённых файлов) — отложено; дублирование маленькое и задокументированное (code-reviewer minor, optional).
4. **`make lint` не прогнан** — `golangci-lint` в окружении отсутствует/несовместим с конфигом (как в Task 01/02/05/06/07); вместо него `go vet ./top/... ./internal/stat/...`, `gofmt -l` и `gosec` чисты, полный lint остаётся на CI.

**Tech debt:** Нет (Deviation 3 — кандидат на консолидацию при будущем касании `internal/pretty`).

**Reviews:**

*Round 1:*
- dev-code-reviewer: approved_with_suggestions — 0 critical/major, 3 minor (все optional) → [010-feat-overview-dashboard-task-08-dev-code-reviewer-round1.json](010-feat-overview-dashboard-task-08-dev-code-reviewer-round1.json)
- dev-security-auditor: approved — 0 находок (EvalSymlinks/path: ошибка отбрасывается, raw-путь не логируется/не выводится, failure→n/a, без паники; новых SQL/shell-вызовов нет) → [010-feat-overview-dashboard-task-08-dev-security-auditor-round1.json](010-feat-overview-dashboard-task-08-dev-security-auditor-round1.json)
- dev-test-reviewer: passed — litmus 11/13, pyramid healthy, 4 minor (все 7 TDD-якорей закрыты + extra) → [010-feat-overview-dashboard-task-08-dev-test-reviewer-round1.json](010-feat-overview-dashboard-task-08-dev-test-reviewer-round1.json)

Применены сходящиеся code+test находки (commit d0696f7): full-line golden-ассерты для всех verbose-строк обеих панелей (фиксируют reserve-width / порядок полей — раскладка spec-load-bearing), вместо `Contains`-подстрок; `*_compactUnchanged` усилены — теперь сравнивают первые compact-строки verbose-вывода с чистым compact (verbose только дописывает, не возмущает), а не `verbose=false` сам с собой (тавтология); в pgstat-available кейсе грубый глобальный `NotContains("n/a")` заменён на позитивные пер-полевые golden (включая bgwr write/sync ms дельты и значение `maxwritten=4`); задокументирован rename матчера. Отклонены: рефактор `rateField`↔`pretty.RateUnit` (вне разрешённых файлов — Deviation 3) и косметика раскладки pgstat (бюджеты разрядов pgstat в спеке жёстко не заданы — проверяется на user-верификации). Round 2 не запускался: security approved 0 находок, code/test — только optional minor, actionable из них применены.

**Verification:**
- `go test ./top/... ./internal/stat/...` → ok (live-PG кластер доступен; все writer-based и fsstat-тесты зелёные; compact байт-в-байт неизменён)
- `go build ./...` → OK; полный `go test ./...` → все пакеты ok
- `go vet ./top/... ./internal/stat/...` + `gofmt -l` + `gosec ./top/... ./internal/stat/...` → чисто

---

## Task 09: Tiering + latency guard + first-tick handling

**Status:** Done
**Commit:** e5d5c71 (impl) + c6c5938 (round 1 fixes) + b9c5b3a (round 2 fixes) + 4540f80 (review reports)
**Agent:** tier-dev
**Summary:** Финальный штрих verbose-коллектора. Verbose-специфичные поля сгруппированы в именованный sub-struct `verboseCollectState` на `Collector` (`internal/stat/stat.go`, Decision 9): ОБА first-tick-поля Task 7 (`verboseFirstTick` + `prevVerboseActive`) переехали внутрь него с сохранением ре-арм-семантики (`verboseFirstTick = !prevVerboseActive` на каждом OFF→ON, БЕЗ опоры на `c.Reset()` — `toggleVerbose` Reset не зовёт, Decision 2); мост `System.VerboseFirstTick` обновлён. Добавлен per-source latency guard ТОЛЬКО для дорогого no-twin агрегата (db sizes / growth): при превышении порога источник троттлится и отдаёт кешированное **stale** значение (не `n/a`), system-строки и дешёвые агрегаты собираются каждый тик (консистентность с полными панелями). Гард — НЕ односторонняя защёлка: к latency-порогу добавлена per-source cadence (`dbSizeLastRun`), троттлинг снимается через бюджет в один refresh-интервал, после чего источник принудительно пересобирается для ре-замера latency — авто-resume достижим на боевом пути `Update`. Решение «троттлить/отдать stale» вынесено в чистую функцию `dbSizeThrottled(threshold, budget, sinceLastRun)` (тестируема без live PG). Latency дорогого запроса замеряется узко вокруг `OverviewDatabasesSize` `QueryRow` (возвращается вторым значением из `collectOverviewStat`), а не вокруг всей сборки. `Reset()` чистит `verboseCollectState` в lockstep с prev/curr (не «застрявший» троттл после view-switch), но ре-арм НЕ полагается на Reset. В `top/stat.go` подсказка `collecting...` в cmdline завязана на тот же флаг (через `Stat.System.VerboseFirstTick`, единый источник истины — без дубль-флага в `top/`), показывается пока флаг выставлен, само-очищается через 2с-таймер `printCmdline`, ре-аппиртся на каждом OFF→ON re-enable; mutual-exclusion `printCmdline` соблюдён (один вызов на путь). Всё за единственным `z` — нового user-knob нет.

**Финализированная константа порога latency guard (Decision 9 отложил точное значение сюда):** `latencyGuardThreshold(refresh) = max(refresh/4, 500ms)`. Именованные константы: `verboseGuardFloor = 500 * time.Millisecond` (абсолютный пол), `verboseGuardFraction = 4` (относительная доля = 25% интервала). Cadence-бюджет троттлинга = один refresh-интервал (`budget := refresh`). Семантика: при refresh=1s активен 500ms-floor (25% = 250ms < floor); при refresh=4s активны 25% = 1s. Порог сравнивается строго (`lastLatency > threshold` троттлит; at-threshold — нет), граница бюджета строгая (`sinceLastRun < budget` троттлит; at-budget — ре-проба).

**Deviations:**
1. **Затронут `internal/stat/postgres.go` (+`postgres_test.go`) сверх заявленного списка файлов задачи** (задача разрешала `internal/stat/stat.go`+тест и `top/stat.go`+тест). Причина — необходимая и неустранимая: дорогой запрос `sum(pg_database_size)` живёт внутри `collectOverviewStat` (`postgres.go`), и чтобы РЕАЛЬНО не платить за него при троттлинге (а не просто перезаписать поля после выполнения), функции нужен параметр пропуска. Добавлены: параметр `skipDatabasesSize bool` (гейтит только size-`QueryRow`; дешёвые агрегаты и все прочие строки собираются всегда) и второй возврат `time.Duration` (узкий замер latency size-запроса). Изменение в том же пакете, минимальное; 8 call-site в `postgres_test.go` обновлены механически (`x, _ :=` + `false`). Прецедент — Task 08 (вынужденное касание `stat.go` ради моста флага).
2. **`make lint`/`govulncheck` не прогнаны** — `golangci-lint`/`gosec`/`govulncheck` не установлены/несовместимы с конфигом в окружении (как в Task 01-08); вместо них `go vet ./internal/stat/... ./top/...` и `gofmt -l` чисты, полный lint/vuln остаётся на CI.

**Tech debt:** Нет.

**Reviews:**

*Round 1:*
- dev-code-reviewer: changes_required — 1 critical (latency guard был односторонней защёлкой: `skipSize` зависел только от `dbSizeLastLatency`, который обновляется лишь на реальной сборке → медленный источник троттлится навсегда, авто-resume не происходит в проде; unit-тест маскировал баг ручной мутацией поля), 2 minor → [010-feat-overview-dashboard-task-09-dev-code-reviewer-round1.json](010-feat-overview-dashboard-task-09-dev-code-reviewer-round1.json)
- dev-test-reviewer: passed — 0 critical/major, 3 minor → [010-feat-overview-dashboard-task-09-dev-test-reviewer-round1.json](010-feat-overview-dashboard-task-09-dev-test-reviewer-round1.json)

Critical устранён (commit c6c5938): добавлена per-source cadence `dbSizeLastRun`, `dbSizeThrottled` стал чистым 3-арг решением (no-cache / at-threshold / within-budget), на каждой реальной сборке стампятся ОБА поля → троттл снимается через бюджет, источник пере-пробируется (genuine throttle, auto-resume достижим на боевом пути). Minor'ы: узкий замер latency size-запроса (второй возврат `collectOverviewStat`); документирование 2с-self-clear cmdline на месте вызова. Test-minor'ы: новый чистый `Test_verboseCollectState_firstTickNotThrottled` (гард genuine-first-tick) + end-to-end auto-resume через реальный `Update` (бэкдейт `dbSizeLastRun` за бюджет).

*Round 2:*
- dev-code-reviewer: approved — 0 critical/major, 1 minor (опциональный doc-nit про zero-time first tick) → [010-feat-overview-dashboard-task-09-dev-code-reviewer-round2.json](010-feat-overview-dashboard-task-09-dev-code-reviewer-round2.json)
- dev-test-reviewer: passed — litmus 8/8, pyramid healthy, 2 minor (s3-assertion слегка timing-dependent; carried-over redundant_testing) → [010-feat-overview-dashboard-task-09-dev-test-reviewer-round2.json](010-feat-overview-dashboard-task-09-dev-test-reviewer-round2.json)

Применены оба опциональных minor'а (commit b9c5b3a): note на месте вызова про harmless zero-time first tick; детерминированный ассерт `dbSizeLastRun.After(backdated)` в end-to-end auto-resume (доказывает реальную пересборку независимо от тайминга хоста), latency-ассерт оставлен как coarse smoke-check с пометкой о запасе. Отклонён carried-over redundant_testing (ре-арм в длинном live-тесте `TestCollector_Update_Verbose`) — осознанно принят как есть ещё в round 1 (поведение покрыто end-to-end), вынос в чистый хелпер — вне скоупа. Round 3 не запускался: оба ревьюера approved/passed.

**Verification:**
- `go test ./internal/stat/... ./top/...` → ok (live-PG кластер доступен; пороговый table-тест, чистые throttle/auto-resume/first-tick тесты, end-to-end троттлинг со stale-кешем и system-rows-every-tick, cmdline-hint тест — зелёные; существующие тесты не падают)
- `go build ./...` → OK; полный `go test ./...` → все пакеты ok
- `go vet ./internal/stat/... ./top/...` + `gofmt -l` → чисто

---

## Task 10: Pre-deploy QA

**Status:** Done
**Commit:** (this commit)
**Agent:** qa
**Summary:** Приёмочный прогон на свежем билде. Все автоматические гейты зелёные: `make build` (бинарь собран), `make test` (race+coverage, 69.7%, 0 FAIL / 0 SKIP — интеграционные тесты прошли вживую против PG 14-18 на 127.0.0.1:21914-21918), `gosec` (73 файла, 0 issues), `go vet` чисто, `gofmt -l` чисто по всем 21 изменённым файлам фичи. 6 автоматически-проверяемых критериев приёмки — PASS с привязкой к тестам (byte-identical compact, n/a-деградация, GUC scan-lockstep, first-tick+re-arm без Reset, no view-count/keybinding регрессий, version-correct агрегаты PG 14-18). 6 визуальных TUI-критериев (геометрия verbose+height-guard, консистентность строк с полными панелями B/N/F/d/r/b, first-tick collecting.../stale-on-throttle) — DEFERRED-TO-USER (нет TUI E2E-харнесса), оформлены чек-листом ручной верификации в JSON-отчёте. Полный отчёт: [010-feat-overview-dashboard-qa-report.json](010-feat-overview-dashboard-qa-report.json).
**Deviations:** `golangci-lint` недоступен локально (v1-конфиг vs v2-инструмент) — заменён прокси `go vet` + `gofmt -l`, полный golangci-lint оставлен на CI (gosec прогнан локально, чисто).
**Tech debt:** Нет нового. Зафиксированы 2 minor-находки вне скоупа фичи: (1) `govulncheck` advisory GO-2026-5037 в stdlib crypto/x509 (тулчейн go1.25.10 → фикс в go1.25.11; не код проекта; резолв = бамп Go в CI); (2) pre-existing gofmt-дрейф в `internal/query/wal_test.go` и `internal/stat/procpidstat_test.go` (выравнивание полей, коммиты 1ebf907/99c8413, вне набора файлов фичи).

**Reviews:** N/A — приёмочная QA-задача без ревьюеров; результат самой проверки и есть отчёт.

**Verification:**
- `make build` → exit 0, ./bin/pgcenter
- `make test` → все пакеты ok, TEST_EXIT=0, coverage 69.7%, без SKIP/FAIL
- `gosec -quiet ./...` → 0 issues; `go vet ./...` + `gofmt -l .` (файлы фичи) → чисто
- `govulncheck ./...` → 1 stdlib-advisory (тулчейн, не код) — задокументирован как minor
- Визуальные TUI-критерии → DEFERRED-TO-USER (чек-лист в qa-report.json `deferredToPostDeploy`)

---

## Task 08: Visual-review fixes (live-TUI polish)

**Status:** Done
**Commit:** (this commit)
**Agent:** rows-dev
**Summary:** Четыре косметические правки в verbose-композерах `top/stat.go` по итогам ручного прогона живого TUI. (1) nicstat `err/coll`: post-slash значение (`Tcolls`) больше не паддится резервной шириной — `strconv.Itoa(pretty.Ceil(...))` вместо `pretty.ReserveWidth`, рендер стал тесным `N/0`. (2) bgwr/ckpt `write/sync` и `timed/req`: то же правило тесной A/B-пары — post-slash значения (`CkptSyncMsDelta`, `CkptReq`) теперь без паддинга; leading-колонка (pre-slash) сохраняет выравнивание. slots/retain и workers/max уже были тесными (post-slash = `pretty.Size`/`%d`) — не трогались. (3) filesyst `use%`: устранён рассинхрон с полной панелью fsstat (75% vs 74%) — строка рендерит `fs.Pused` форматом `%3.0f` (как `printFsstats` через `%8.0f`), **без** `Ceil`; правило ceil остаётся только для rate-полей. (4) replication label `send/recv` → `senders/receivers` (только подпись, значения те же — чтобы убрать сетевую коннотацию send/recv). Тесты-golden в `top/stat_test.go` обновлены под новый тесный формат, паритет use% (Pused 74.3 → 74, не 75) и новую подпись.
**Deviations:** Нет. `internal/stat/fsstat.go` не тронут — паритет use% решён в композере. `bin/pgcenter` (модифицированный трекнутый бинарь) не трогался.
**Tech debt:** Нет.

**Reviews:** dev-code-reviewer round2 — [010-feat-overview-dashboard-task-08-dev-code-reviewer-round2.json](010-feat-overview-dashboard-task-08-dev-code-reviewer-round2.json).

**Verification:**
- `go test ./top/...` → ok (golden-строки nicstat/bgwr/replication/filesyst зелёные)
- `go build ./...` → OK

---

## Task 08: Visual-review fix round 3 — резерв ширины n/a (статичные хвостовые подписи)

**Status:** Done
**Commit:** (this commit)
**Agent:** rows-dev
**Summary:** Правка по итогам ручного прогона живого TUI: в verbose-строке `databases` подпись `cache hit ratio` прыгала по горизонтали при переключении значения между `n/a` (3 симв.) и реальным `100.00%` (7 симв.), т.к. деградировавший `n/a` рендерился БЕЗ паддинга до зарезервированной ширины поля. Общий принцип фикса: sentinel `n/a` должен занимать ту же зарезервированную ширину, что и значение, которое он заменяет, — тогда он drop-in и не сдвигает хвостовую подпись. Введён хелпер `naReserve(width)` — `fmt.Sprintf("%*s", ...)` с right-align (зеркало `pretty.ReserveWidth` `%*d`) и защитой min-width = `len(naLiteral)` от усечения. Исправлены два поля с фиксированным резервом: (1) **cache hit ratio** (флагнутое) — значение теперь `%6.2f%%` (ширина 7: `100.00%` / ` 99.99%`), `n/a` → `naReserve(7)` = `    n/a`; подпись статична. (2) **workload rates** (`naInt`: tps/ins/upd/del/ret/tmp ширина 4, others ширина 3) — `naInt` теперь возвращает `naReserve(width)` вместо голого `naLiteral`, поэтому `n/a` → ` n/a` (ширина 4) попадает в слот значения.
**Deviations:** Аудит остальных verbose-полей: НЕ трогались (1) **bgwr/ckpt maxwritten** — его `n/a` сцеплено с `HasPrev=false`, что одновременно n/a'ит намеренно-тесный post-slash `syncMs` (правило A/B-композита из 95656e8, защищено), поэтому смещение подписи `maxwritten` определяется тесным композитом, а не fixed-reserve n/a — резерв ширины тут не сделал бы подпись статичной; (2) **databases size/growth, replication lag/retain/backlog** — используют `pretty.Size`, ширина которого изначально переменная (нет фиксированного резерва), так что сопоставление n/a↔значение там ill-defined (значение и так прыгает). Значения и правило тесного композита не менялись — только ширина n/a. `bin/pgcenter` не трогался.
**Tech debt:** Нет.

**Reviews:** dev-code-reviewer round3 — approved, 0 critical / 0 major, 2 optional minor. Применено: расширил byte-offset-проверку статичности подписи на workload-поле `tps` (путь naInt). Отклонено: документировать min-width-инвариант `naReserve` в коде — уже в doc-комментарии, все вызовы width≥3. Отчёт: [010-feat-overview-dashboard-task-08-dev-code-reviewer-round3.json](010-feat-overview-dashboard-task-08-dev-code-reviewer-round3.json).

**Verification:**
- `go test ./top/...` → ok (обновлённые golden + новый `Test_renderPgstat_verboseNAWidthStatic`: byte-offset подписи cache hit ratio и tps идентичны в состояниях n/a и значение)
- `go build ./...` → OK; `go vet ./top/...` → чисто
- Рендер cache hit ratio (databases-строка), все 3 состояния выровнены по одной колонке:
  - `... n/a growth/s,     n/a cache hit ratio`
  - `... n/a growth/s, 100.00% cache hit ratio`
  - `... n/a growth/s,  99.99% cache hit ratio`
