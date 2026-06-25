---
status: planned                    # planned -> in_progress -> done
depends_on: ["01", "03", "05", "07"]   # ID задач-зависимостей (строки)
wave: 4                            # волна параллельного выполнения
skills: [code-writing]             # МАССИВ скиллов для загрузки
verify: user                       # верификация: user — verbose rows consistent with full B/N/F/d/r/b panels
reviewers: [dev-code-reviewer, dev-security-auditor, dev-test-reviewer]
teammate_name:
---

# Task 08: Verbose row composers (both panels)

## Required Skills

Перед выполнением задачи загрузи:
- `/skill:code-writing` — [skills/code-writing/SKILL.md](~/.claude/skills/code-writing/SKILL.md)

## Description

Финальный рендер-слой verbose-режима. Все данные к этому моменту уже собраны предыдущими волнами:
Task 1 дал форматтеры (ceil, reserved-digit fixed width, динамический суффикс единиц), Task 3 расщепил
`printSysstat`/`printPgstat` в тонкие `*gocui.View`-обёртки поверх writer-ядер `renderSysstat`/`renderPgstat(w io.Writer, …)`,
Task 5 добавил агрегатные SQL-запросы + структуры для pgstat-строк (workload/databases/workers/replication/bgwr-ckpt)
и GUC-чтения, Task 7 — verbose-ветку «все три источника сразу» в `Collector.Update`, наполняющую
`s.Diskstats`/`s.Netdevs`/`s.Fsstats` каждый тик при verbose.

Задача — внутри `renderSysstat`/`renderPgstat`, под флагом verbose, отрендерить 8 дополнительных
`label:value`-строк: **3 системные** (`iostat`/`nicstat`/`filesyst`) и **5 pgstat** (`workload`/`databases`/
`workers`/`replication`/`bgwr/ckpt`), используя форматтеры Task 1.

Ключевое требование — **консистентность с полными панелями `B`/`N`/`F` и экранами `d`/`r`/`b`**: verbose
читает те же самые структуры, что рендерят полные панели, и выбирает устройство с максимальной `%util`
(disk) / `Utilization` (net), повторяя точную панель-математику (в частности, `nicstat` rMbps/wMbps —
конверсия `Rbytes/1024/128`, выполняемая в `printNetdev` на этапе печати, а НЕ в `countNetdevsUsage`).
`filesyst` показывает ФС каталога данных (`data_directory`), найденную по самому длинному совпадению
mount-префикса; локально путь резолвится через `filepath.EvalSymlinks`, удалённо — по нерезолвленному пути.

Безопасность и устойчивость: первый тик (нет prev → пустой `Diskstats`/`Netdevs` слайс) → `n/a`, не `0`;
недоступный источник → `n/a`, остальные строки рендерятся; любой сбой `EvalSymlinks`/mount-match → `n/a`,
не panic; сырой текст ошибок PG/путей в вывод не попадает.

## What to do

- В `top/stat.go`, внутри `renderSysstat(w io.Writer, s stat.Stat, …)` (writer-ядро из Task 3), под флагом
  verbose дописать 3 системные строки после существующих 4 компактных:
  - **`iostat`**: выбрать устройство с максимальной `s.Diskstats[i].Util` среди устройств с `Completed != 0`
    (тот же фильтр, что `printIostat`). Поля строки: `devices` (= число активных устройств; резерв 2 цифры),
    `max util` (резерв 3 цифры), `rMB/s`/`wMB/s` (поля `Rsectors`/`Wsectors` выбранного устройства — они уже
    в MB/s, резерв 4 цифры, динамический суффикс), `r/s`/`w/s` (`Rcompleted`/`Wcompleted`, резерв 5 цифр).
    Если `s.Diskstats` пуст (первый тик / нет prev) → вся строка `n/a`.
  - **`nicstat`**: выбрать интерфейс с максимальной `s.Netdevs[i].Utilization` среди интерфейсов с
    `Packets != 0` (фильтр как в `printNetdev`). Поля: `devices` (резерв по числу), `max util` (резерв 3),
    `rMbps`/`wMbps` = `Rbytes/1024/128` и `Tbytes/1024/128` выбранного интерфейса (точная репликация
    print-time конверсии из `printNetdev`, резерв 4 цифры, динамический суффикс), `err/coll` — композит
    `(Rerrs+Terrs)` и `Tcolls`, формат `1111/2222 err/coll` (резерв 4 цифры на каждое число).
    Пустой `s.Netdevs` → строка `n/a`.
  - **`filesyst`**: найти `Fsstat`, чей `Mount.Mountpoint` — самый длинный префикс пути `data_directory`
    (longest-mount-prefix-wins). Поля: `device` (`Mount.Device`), `mounted` (`Mount.Mountpoint`, обрезка до
    10 символов), `filesystem` (`Mount.Fstype`), `size`/`used`/`use %` (`Size`/`Used` через `pretty.Size`,
    `Pused`). Источник `data_directory` — `props.DataDirectory` (GUC из Task 4). Если совпадение не найдено
    / `s.Fsstats` пуст → `n/a`.
- В `internal/stat/fsstat.go` добавить чистую функцию сопоставления каталога данных с ФС:
  принимает `data_directory`, `Fsstats`, флаг `local` (= `db.Local`); локально предварительно резолвит путь
  через `filepath.EvalSymlinks` (с `filepath.Clean`, как в существующем коде файла), удалённо использует путь
  как есть; возвращает `Fsstat` с самым длинным `Mountpoint`-префиксом и `ok bool`. Любая ошибка
  `EvalSymlinks` (broken symlink, EACCES) → `ok=false`, без паники и без логирования сырого пути.
- В `renderPgstat(w io.Writer, …)` под флагом verbose дописать 5 pgstat-строк из структур, наполненных
  Task 5 (round-вверх через ceil-форматтер Task 1):
  - **`workload`**: `tps` (=`xact_commit+xact_rollback`/s), `ins/s`/`upd/s`/`del/s`, `ret/s`, `tmp/s`,
    `others` (=`deadlocks+conflicts+checksum_failures` **за интервал**, без `/s`).
  - **`databases`**: `XGB per Y databases` (`sum(pg_database_size)` + количество), `growth/s`,
    `cache hit ratio` (per-interval `Δhit/Δ(hit+read)`).
  - **`workers`**: `workers/max` (зонтик `max_worker_processes`), `logical workers` (/`max_logical_replication_workers`),
    `parallel workers` (/`max_parallel_workers`).
  - **`replication`**: `wal size`, `lag`, `slots/retain`, `archiving backlog`, `send/recv`.
  - **`bgwr/ckpt`**: `timed/req` (абсолютные), `write/sync ms` (per-interval дельта), `maxwritten`.
  - Каждая метрика-дельта на первом тике (нет prev) → `n/a`; недоступный источник (`archive_mode=off`,
    нет репликации, нет прав, версия PG без view) → литерал `n/a`; падение одного источника не гасит
    остальные строки.
- Раскладка резервирования разрядов и динамический суффикс — статичны, меняются только цифры; точные
  бюджеты разрядов и форматы полей см. user-spec «Состав и источники строк».
- Написать writer-based тесты в `top/stat_test.go` (против `bytes.Buffer`) и тесты matcher'а в
  `internal/stat/fsstat_test.go` — см. TDD Anchor.

## TDD Anchor

Тесты пишем ДО реализации. Пишем → запускаем → убеждаемся что падают → пишем код → убеждаемся что проходят.

- `top/stat_test.go::Test_renderSysstat_verboseIostatMaxUtil` — при нескольких устройствах в `s.Diskstats`
  verbose-строка `iostat` показывает значения устройства с максимальной `Util`; устройства с `Completed==0`
  пропускаются (тот же набор, что `printIostat`).
- `top/stat_test.go::Test_renderSysstat_verboseNicstatConversion` — verbose `nicstat` показывает
  `Rbytes/1024/128` / `Tbytes/1024/128` выбранного интерфейса (паритет с print-time конверсией `printNetdev`);
  `err/coll` = `(Rerrs+Terrs)` и `Tcolls` в формате `1111/2222`.
- `top/stat_test.go::Test_renderSysstat_verboseFirstTickNA` — пустые `s.Diskstats`/`s.Netdevs` (нет prev,
  первый тик) → строки `iostat`/`nicstat` рендерят `n/a`, не `0`.
- `top/stat_test.go::Test_renderSysstat_verboseFilesystMounted10` — поле `mounted` обрезается до 10 символов.
- `internal/stat/fsstat_test.go::Test_matchDataDirFs_longestPrefix` — при mountpoints `/` и `/var/lib/pgsql`
  и `data_directory=/var/lib/pgsql/data` выбирается `/var/lib/pgsql` (самый длинный префикс по границе компонента).
- `internal/stat/fsstat_test.go::Test_matchDataDirFs_noMatch` — нет совпадающего mountpoint → `ok=false`
  (далее composer → `n/a`).
- `internal/stat/fsstat_test.go::Test_matchDataDirFs_evalSymlinksFailure` — сломанный/недоступный symlink
  при `local=true` → `ok=false`, без паники.
- `top/stat_test.go::Test_renderPgstat_verboseNA` — недоступный pgstat-источник (нулевая/sentinel-структура)
  рендерит `n/a`, остальные строки рендерятся.
- `top/stat_test.go::Test_renderSysstat_compactUnchanged` / `Test_renderPgstat_compactUnchanged` —
  при verbose=false вывод байт-в-байт совпадает с компактным (verbose-строки не добавляются).

## Acceptance Criteria

- [ ] При verbose 3 системные строки (`iostat`/`nicstat`/`filesyst`) рендерятся в панель `sysstat`.
- [ ] При verbose 5 pgstat-строк (`workload`/`databases`/`workers`/`replication`/`bgwr/ckpt`) рендерятся в `pgstat`.
- [ ] `iostat`/`nicstat` выбирают устройство с максимальной `%util`/`Utilization`, пропуская неактивные
      (`Completed==0`/`Packets==0`) — тот же набор устройств, что полные панели `B`/`N`.
- [ ] `nicstat` rMbps/wMbps = `Rbytes/1024/128` / `Tbytes/1024/128` (паритет с `printNetdev`).
- [ ] `filesyst` показывает ФС `data_directory` по самому длинному mount-префиксу; `mounted` обрезано до 10.
- [ ] Локально путь `data_directory` резолвится `filepath.EvalSymlinks`; удалённо — нерезолвленный путь.
- [ ] Первый тик (нет prev) → `n/a` (не `0`); недоступный источник → `n/a`, остальные строки целы.
- [ ] Любой сбой `EvalSymlinks`/mount-match → `n/a`, без паники; сырой текст ошибок PG/путей не выводится и не логируется.
- [ ] Значения округлены вверх (ceil), резерв разрядов статичен, динамический суффикс единиц подключён.
- [ ] При verbose=false вывод компактных панелей байт-в-байт не изменился.
- [ ] `go test ./top/... ./internal/stat/...` зелёные; `make lint` чистый.

## Context Files

**Feature artifacts:**
- [010-feat-overview-dashboard.md](010-feat-overview-dashboard.md) — user-spec (см. «Состав и источники строк»: точная раскладка полей, композит nicstat `IErr+Oerr / Coll` как `1111/2222`, обрезка mount до 10, бюджеты разрядов)
- [010-feat-overview-dashboard-tech-spec.md](010-feat-overview-dashboard-tech-spec.md) — tech-spec (Task 8, Decisions 5 и 7)
- [010-feat-overview-dashboard-code-research.md](010-feat-overview-dashboard-code-research.md) — code-research (раздел «4-new»: точная reuse-математика %util, nicstat `/1024/128`, longest mount-prefix, EvalSymlinks local-only, first-tick `n/a`)
- [010-feat-overview-dashboard-decisions.md](010-feat-overview-dashboard-decisions.md) — decisions log

**Project knowledge:**
- [overview.md](../../../.claude/skills/project-knowledge/overview.md) — обзор проекта, поддерживаемые статистики, версии PG
- [architecture.md](../../../.claude/skills/project-knowledge/architecture.md) — раскладка пакетов, data flow top, секция Testing
- [patterns.md](../../../.claude/skills/project-knowledge/patterns.md) — «Testable TUI Rendering — pure window function + io.Writer printers», Error Wrapping, Testing conventions

**Code files:**
- [top/stat.go](../../../top/stat.go) — добавить verbose-строки в `renderSysstat`/`renderPgstat`
- [top/stat_test.go](../../../top/stat_test.go) — writer-based тесты composer'ов
- [internal/stat/fsstat.go](../../../internal/stat/fsstat.go) — добавить функцию matching data_directory ↔ ФС (longest mount-prefix, EvalSymlinks local-only)
- [internal/stat/diskstats.go](../../../internal/stat/diskstats.go) — поля `Diskstat` (читать)
- [internal/stat/netdev.go](../../../internal/stat/netdev.go) — поля `Netdev` (читать)
- [internal/pretty/pretty.go](../../../internal/pretty/pretty.go) — `pretty.Size` + форматтеры Task 1 (читать/использовать)
- [internal/stat/postgres.go](../../../internal/stat/postgres.go) — `PostgresProperties` (`DataDirectory`, GUC из Task 4), pgstat-структуры из Task 5 (читать)

## Verification Steps

- Запустить `go test ./top/... ./internal/stat/...` — все writer-based и fsstat-тесты зелёные; компактный
  вывод байт-в-байт не изменился.
- Запустить `make lint` — golangci-lint + gosec чистые (особенно: нет логирования сырых путей/ошибок).
- **user (verify: user):** в живом TUI включить verbose (`v`) и сверить значения verbose-строк со значениями
  полных панелей/экранов на тех же данных:
  - `iostat`-строка ↔ панель `B` (то же устройство с max `%util`, те же rMB/s/wMB/s/r/s/w/s);
  - `nicstat`-строка ↔ панель `N` (тот же интерфейс, те же rMbps/wMbps);
  - `filesyst`-строка ↔ панель `F` (ФС каталога данных);
  - `databases`/`replication`/`bgwr-ckpt` строки ↔ экраны `d`/`r`/`b`.
  - Проверить деградацию: на standby / при `archive_mode=off` / при удалённом подключении соответствующие
    поля показывают `n/a`, остальные строки целы; первый тик показывает `n/a` (не `0`).

## Details

**Files:**
- `top/stat.go` — внутри writer-ядер `renderSysstat`/`renderPgstat` (Task 3) добавить verbose-блоки под
  флагом verbose (флаг приходит из `config.verbose`/`view.Verbose`, Task 2 — как параметр/часть сигнатуры,
  установленной Task 3). Системные строки строятся из `s.Diskstats`/`s.Netdevs`/`s.Fsstats`; pgstat-строки —
  из структур, наполненных Task 5 (расширенный `Activity` или новый `Pgstat` sub-struct). Round-вверх и
  reserved-digit/динамический суффикс — через форматтеры Task 1 из `internal/pretty`.
- `internal/stat/fsstat.go` — новая чистая функция (напр. `matchDataDirFs(dataDir string, fss Fsstats, local bool) (Fsstat, bool)`):
  при `local` сначала `resolved, err := filepath.EvalSymlinks(filepath.Clean(dataDir))`; при ошибке →
  вернуть `ok=false` (не паниковать, не логировать сырой путь); затем выбрать `Fsstat` с самым длинным
  `Mount.Mountpoint`, являющимся префиксом пути. Сравнение префикса — по границе компонента пути
  (`/var` не должен «съесть» `/variable`), как принято для mount-matching.
- `top/stat_test.go` — таблично-параметризованные writer-тесты против `bytes.Buffer` (прецедент —
  `printStatHeader`/`printStatData`/`renderDbstat` тесты в этом же файле).
- `internal/stat/fsstat_test.go` — табличные тесты matcher'а (longest-prefix, no-match, EvalSymlinks-failure).

**Dependencies:**
- Task 1 — форматтеры ceil / reserved-digit / dynamic unit suffix в `internal/pretty/pretty.go`.
- Task 3 — `renderSysstat`/`renderPgstat(w io.Writer, …)` writer-ядра, в которые встраиваются verbose-строки.
- Task 5 — pgstat-агрегатные структуры (workload/databases/workers/replication/bgwr-ckpt) + GUC из Task 4
  (`DataDirectory`, `GucMaxWorkerProcesses`, `GucMaxLogicalReplicationWorkers`, `GucMaxParallelWorkers`,
  `GucWalSegmentSize`) на `PostgresProperties`.
- Task 7 — verbose-ветка наполнения `s.Diskstats`/`s.Netdevs`/`s.Fsstats` каждый тик.
- Пакеты: только stdlib (`path/filepath.EvalSymlinks`, `filepath.Clean`); новых внешних зависимостей нет.

**Edge cases:**
- Первый verbose-тик: `countDiskstatsUsage`/`countNetdevsUsage` возвращают `nil` при `len(curr)!=len(prev)`,
  поэтому `s.Diskstats`/`s.Netdevs` пусты → строка `n/a`, НЕ `0` (Decision 5, risk-таблица tech-spec).
- Все устройства неактивны (`Completed==0` / `Packets==0`) → нет кандидата на max-util → `n/a`.
- `data_directory` не покрыт ни одним mount из allowlist (`ext3|ext4|xfs|btrfs`) → `n/a` (документированное
  ограничение: WAL/tablespaces на других ФС не покрываются).
- Сломанный symlink / EACCES при `EvalSymlinks` (local) → `n/a`, не panic, без логирования пути.
- Удалённое подключение (`db.Local==false`): matching по нерезолвленному пути; remote-фильтр ФС тот же
  allowlist.
- `archive_mode=off` / нет репликации / нет прав (`pg_monitor`) / версия PG без нужного view → конкретное
  поле/строка `n/a`; одна ошибка источника не гасит остальные строки (Decision 5, risk-таблица).
- Динамический суффикс: топовое железо (NVMe >9.7 GB/s, 25/40/100GbE) переполняет резерв разрядов →
  единица переключается (MB/s→GB/s, Mbps→Gbps), раскладка не ломается.
- verbose=false: ни одна verbose-строка не добавляется; компактный вывод байт-в-байт неизменен.

**Implementation hints:**
- `%util` (disk) и `Utilization` (net) уже вычислены в `countDiskstatsUsage` (`diskstats.go:211`) /
  `countNetdevsUsage` (`netdev.go:223-230`) — НЕ пересчитывать, читать из `s.Diskstats`/`s.Netdevs`
  (гарантия консистентности, Decision 5).
- `iostat`-поля для выбранного устройства: `Rsectors`/`Wsectors` (уже MB/s, `diskstats.go:243-244`),
  `Rcompleted`/`Wcompleted` (r/s, w/s), `Util`, `len(s)` для счётчика.
- `nicstat` rMbps/wMbps = `Rbytes/1024/128` / `Tbytes/1024/128` — конверсия делается в `printNetdev`
  на этапе печати (`stat.go:741`), а НЕ в `countNetdevsUsage`; реплицировать ТОЧНО. `err/coll`:
  `Rerrs+Terrs` и `Tcolls` (`printNetdev` печатает `Rerrs`/`Terrs`/`Tcolls` раздельно, `stat.go:743`).
- Фильтр активных устройств для max-util-выбора — тот же, что в полных панелях: disk `Completed==0`
  пропускать (`stat.go:706`), net `Packets==0` пропускать (`stat.go:734`) — иначе набор устройств
  разойдётся с панелью.
- `filesyst`: `data_directory` — это `props.DataDirectory` (GUC добавлен Task 4); сегодня в `PostgresProperties`
  его не было (читался ad hoc в `log.go:129`). Используй longest-prefix по компонентам пути.
- НЕ возвращать ошибку из composer'а наверх по first-scan-error-паттерну сайд-панелей; каждый источник
  деградирует в `n/a` независимо. Сырой текст ошибок PG/путей в вывод/лог не помещать (risk-таблица).
- Тесты: брать прецедент уже существующих writer-тестов в `top/stat_test.go` (`bytes.Buffer`).

## Reviewers

- **dev-code-reviewer** → `010-feat-overview-dashboard-task-08-dev-code-reviewer-review.json`
- **dev-security-auditor** → `010-feat-overview-dashboard-task-08-dev-security-auditor-review.json`
- **dev-test-reviewer** → `010-feat-overview-dashboard-task-08-dev-test-reviewer-review.json`

## Post-completion

- [ ] Записать краткий отчёт в [010-feat-overview-dashboard-decisions.md](010-feat-overview-dashboard-decisions.md) (Summary: 1-3 предложения, ревью со ссылками на JSON, без таблиц файндингов и дампов)
- [ ] Если отклонились от спека — описать отклонение и причину
- [ ] Обновить user-spec/tech-spec если что-то изменилось
