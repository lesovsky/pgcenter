---
created: 2026-05-19
status: approved
type: feature
size: M
---

# User Spec: Record/Report для per-process system stats

## Что делаем

Добавляем поддержку `pgcenter record` и `pgcenter report` для экрана per-process system stats
(`Shift+S`, вид `procpidstat`). Закрывает tech-debt [002]. После этой фичи DBA может записывать
per-process CPU/IO-статистику в tar-файл и строить по ней отчёты — так же, как уже работает
запись `pg_stat_activity`, `pg_stat_statements` и других видов.

## Зачем

DBA не может сделать post-mortem анализ per-process нагрузки: `pgcenter top → Shift+S` показывает
только текущий момент, а исторических данных нет. После инцидента невозможно восстановить, какой
backend грузил CPU или IO в момент X. Все остальные экраны pgcenter поддерживают запись —
только `procpidstat` исключён. Эта фича устраняет пробел.

## Пользовательские истории

- Как DBA, я хочу чтобы `pgcenter record` автоматически собирал per-process stats вместе
  с остальными видами, чтобы не думать о дополнительных флагах при запуске мониторинга.
- Как DBA, я хочу запустить `pgcenter report -N` после инцидента и увидеть какой backend
  грузил CPU или IO в момент X, чтобы делать post-mortem анализ без внешних инструментов.
- Как DBA, я хочу использовать `-s`, `-e`, `-o`, `-g`, `-l`, `-t` вместе с `-N` точно так же
  как с `-A`, чтобы фильтровать и сортировать записанные данные.
- Как DBA, я хочу чтобы `pgcenter report -N` на старом tar-файле (без procpidstat данных)
  не падал с ошибкой, а выводил понятное сообщение.

### Пользовательские сценарии

**Сценарий 1: Ночная запись + утренний post-mortem**
1. DBA запускает `pgcenter record -f /var/log/pgcenter.tar -i 10s` и оставляет на ночь.
2. Утром происходит инцидент — жалобы на деградацию производительности с 03:15 до 03:45.
3. DBA запускает `pgcenter report -f /var/log/pgcenter.tar -N -s 03:10 -e 03:50 -o "%all"`.
4. Report показывает построчный дамп снапшотов: для каждого момента — список backends с CPU%,
   IO KiB/s, iodelay% и текстом запроса.
5. DBA видит, что backend PID 7823 имел `%all=87.50` в 03:22 с запросом `SELECT * FROM orders...`
6. Результат: виновник инцидента найден без дополнительных инструментов.

**Сценарий 2: Фильтрация по нагруженному backend**
1. DBA хочет посмотреть только на конкретный backend, запрос которого он помнит.
2. Запускает `pgcenter report -f stats.tar -N -g "query:SELECT.*orders"`.
3. Report показывает только строки, где `query` совпадает с паттерном.
4. Результат: отфильтрованная история нагрузки конкретного запроса.

**Сценарий 3: Remote-подключение при записи**
1. DBA запускает `pgcenter record -h db.prod.example.com -f stats.tar`.
2. При старте выводится `INFO: procpidstat skipped (remote mode: /proc not available)`.
3. Все остальные виды записываются штатно.
4. `pgcenter report -N -f stats.tar` выводит: `INFO: no procpidstat data in this archive`.
5. Результат: нет ошибки, только информативные сообщения.

**Сценарий 4: IO недоступно при записи**
1. DBA запускает pgcenter record не от пользователя `postgres`.
2. При старте `INFO: procpidstat IO stats unavailable (permission denied on /proc/[pid]/io)`.
3. CPU-данные записываются нормально, IO-колонки (`read,KiB/s`, `write,KiB/s`,
   `read_total,KiB`, `write_total,KiB`) сохраняются как `""`.
4. `pgcenter report -N` выводит WARNING в шапке и показывает пустые IO-колонки.
5. Результат: частичные данные лучше чем ничего; DBA понимает причину.

**Сценарий 5: Старый tar-файл**
1. DBA запускает `pgcenter report -N -f old_stats.tar` на tar, записанном старой версией pgcenter.
2. Report выводит: `INFO: no procpidstat data in this archive`.
3. Все остальные типы отчётов (`-A`, `-X m` и др.) работают без изменений.
4. Результат: backward compat сохранён, нет паники.

## Дизайн и интерфейс

### pgcenter record — изменений в CLI нет

`pgcenter record` записывает procpidstat автоматически вместе со всеми другими видами.
Дополнительные флаги не нужны.

При local-подключении procpidstat записывается наравне с `activity`, `tables`, `statements` и др.
При remote-подключении (`-h db.prod.example.com`) procpidstat пропускается с INFO-сообщением.

```
INFO: recording to pgcenter.stat.tar
INFO: procpidstat skipped (remote mode: /proc not available)   ← только при remote
```

### pgcenter report -N — новый флаг

Новый boolean флаг `-N` / `--proc-stats`: "show per-process system stats report".
Не меняет существующий `-A` (activity) — backward compat сохранён.

Все форматирующие опции работают с `-N` так же как с `-A`:
- `-s` / `--start` — начало интервала
- `-e` / `--end` — конец интервала
- `-o` / `--order` — сортировка по колонке
- `-g` / `--grep` — фильтр по значению колонки (`colname:pattern`)
- `-l` / `--limit` — ограничение строк на снапшот
- `-t` / `--strlimit` — усечение длинных строк

Describe: `pgcenter report -d -N` выводит описание колонок procpidstat.

### Формат вывода

Построчный дамп снапшотов с временными метками — аналогично `pgcenter report -A`:

```
INFO: reading from pgcenter.stat.tar
INFO: report procpidstat
INFO: start from: 2026-05-19 03:10:00 MST, to: 2026-05-19 03:50:00 MST
WARNING: IO stats unavailable in recorded data (empty columns)   ← только если IO отсутствует

2026/05/19 03:22:01, rate: 10s
pid   datname  usename   state   ...  all_total,s  us_total,s  ...  %all   %us  read,KiB/s  ...  query
7823  orders   postgres  active  ...  00:01:23     00:01:15    ...  87.50  80.20  204.80    ...  SELECT * FROM orders WHERE ...
1042  orders   appuser   idle    ...  00:00:05     00:00:04    ...  0.00   0.00  0.00       ...
```

Колонки — те же 19 что в TUI (`Shift+S`):
`pid`, `datname`, `usename`, `state`, `wait_etype`, `wait_event`,
`all_total,s`, `us_total,s`, `sy_total,s`, `read_total,KiB`, `write_total,KiB`, `iodelay_total,s`,
`%all`, `%us`, `%sy`, `read,KiB/s`, `write,KiB/s`, `%iodelay`, `query`.

Значения rate-колонок (`%all`, `read,KiB/s`, `%iodelay`) отражают интервал записи.
Значения accumulated-колонок (`all_total,s`, `read_total,KiB`) — абсолютные с момента старта process.

## Как должно работать

### Основной сценарий (local mode, IO и delayacct доступны)

1. DBA запускает `pgcenter record -f stats.tar -i 10s`.
2. Recorder подключается к PostgreSQL, обнаруживает local-mode (через `db.Local`).
3. На каждом тике recorder:
   a. Выполняет SQL-запрос `pg_stat_activity` → 7-колоночный PGresult.
   b. Для каждого PID читает `/proc/[pid]/stat` и `/proc/[pid]/io`.
   c. Сравнивает с предыдущим тиком (хранится в struct) → вычисляет rate-колонки.
   d. Собирает 19-колоночный PGresult с display-значениями.
   e. Пишет в tar: `procpidstat.TIMESTAMP.json` + `sysinfo.TIMESTAMP.json`.
4. DBA останавливает запись (Ctrl+C).
5. DBA запускает `pgcenter report -N -f stats.tar`.
6. Report читает tar, отображает построчно: для каждого timestamp — список backends.

### Граничные случаи

- **Remote-подключение:** recorder пропускает procpidstat (db.Local == false), выводит INFO.
  `pgcenter report -N` на таком tar: INFO "no procpidstat data", пустой вывод.
- **IO недоступно (`EACCES` на `/proc/[pid]/io`):** IO-колонки записываются как `""`.
  Report выводит WARNING в шапке, показывает пустые IO-колонки.
- **delayacct недоступно (`kernel.task_delayacct=0`):** iodelay-колонки записываются как `""`.
  Поведение аналогично IO.
- **Backend завершился между тиками:** его строка отсутствует в следующем снапшоте (как в TUI).
- **Первый тик recorder:** нет prev-данных, rate-колонки = `"0"`. Этот snapshot записывается в tar,
  но processData в report пропускает первый item (`if !prevStat.Valid → continue`) — стандартное
  поведение для всех видов pgcenter. DBA нулевые rate-значения не видит.
- **Старый tar без procpidstat:** `pgcenter report -N` выводит INFO, не падает.
- **Tar без sysinfo entry:** report воспроизводит записанные данные без изменений — при Option B
  rate-колонки уже вычислены рекордером и хранятся как строки. sysinfo информационна и не влияет
  на вывод отчёта. Отсутствие sysinfo не является ошибкой.
- **Recorder прерван (SIGINT) на полтике:** незаписанный entry игнорируется при чтении.
- **CLK_TCK отличается на машине просмотра:** sysinfo содержит значение с машины записи —
  rates корректны при переносе tar на другую машину.

## Критерии приёмки

- [ ] `pgcenter record -f stats.tar` записывает procpidstat-данные при local-подключении
- [ ] `pgcenter record -h remote-host -f stats.tar` выводит INFO о пропуске procpidstat
- [ ] `pgcenter report -f stats.tar -N` выводит построчный дамп с временными метками и ненулевыми данными (≥ 1 строка)
- [ ] `pgcenter report -f stats.tar -d -N` выводит describe колонок procpidstat
- [ ] Колонки `%all`, `%us`, `%sy` содержат числовые значения (≥ 0, не пустые строки)
- [ ] Колонки `read,KiB/s`, `write,KiB/s` содержат числовые значения ≥ 0 (или `""` если IO недоступно)
- [ ] Колонка `%iodelay` содержит числовые значения ≥ 0 (может превышать 100 — это ок)
- [ ] `pgcenter report -N -f stats.tar -o "%all"` — строки в каждом снапшоте отсортированы по убыванию `%all`
- [ ] `pgcenter report -N -f stats.tar -g "state:active"` — вывод содержит только строки где `state` совпадает с паттерном
- [ ] `pgcenter report -N -f stats.tar -l 3` — не более 3 строк на снапшот в выводе
- [ ] `pgcenter report -N -f stats.tar -s HH:MM -e HH:MM` — снапшоты вне диапазона не выводятся
- [ ] `pgcenter report -A -f stats.tar` работает без изменений (backward compat)
- [ ] Старый tar без procpidstat: `pgcenter report -N` выводит INFO, не падает
- [ ] Tar с пустыми IO-колонками: report выводит WARNING в шапке, CPU-колонки показывают данные
- [ ] `make test` проходит без новых ошибок (включая обновлённые тесты record_test.go и новые procpidstat-тесты)
- [ ] `make lint` проходит без новых предупреждений
- [ ] `make build` успешен
- [ ] Тесты `TestBuildProcPidResult_*` проходят после MVC-рефактора (regression: TUI Shift+S не сломан)

## Ограничения

- **Только local mode.** procfs недоступен при remote-подключении PostgreSQL. При remote-записи
  procpidstat пропускается автоматически.
- **Rate-колонки отражают интервал записи.** `%all`, `read,KiB/s` вычислены в момент записи.
  Кастомный интервал просмотра (`-s`/`-e`) не пересчитывает rates.
- **Accumulated-колонки — абсолютные значения.** `all_total,s`, `read_total,KiB` показывают
  накопленное с момента старта процесса, а не дельту за интервал просмотра.
- **Требования к правам для IO-метрик.** `/proc/[pid]/io` требует совпадения UID или `CAP_SYS_PTRACE`.
  При отсутствии прав IO-колонки пустые.
- **Требует `CONFIG_TASK_DELAY_ACCT=y` и `kernel.task_delayacct=1`** для iodelay-колонок.
  При недоступности — колонки пустые.

## Риски

- **Риск: MVC-рефактор buildProcPidResult вызовет регрессию TUI.**
  Регрессия TUI (`Shift+S`) является блокирующим критерием приёмки — фича не принята, пока TUI не работает.
  Митигация: существующие тесты `TestBuildProcPidResult_*` покрывают 19-колоночный вывод; новые тесты
  `buildProcPidResultRaw` / `formatProcPidResultForDisplay` добавляются параллельно.

- **Риск: tarRecorder становится stateful — потенциальные ошибки при долгих сессиях записи.**
  Митигация: prev/curr maps — простые `map[int]ProcPidStat` / `map[int]ProcPidIO`; Go GC управляет
  памятью; race detector в `make test -race` покрывает concurrent access.

- **Риск: scope расширяется параллельными треками (MVC-рефактор + recorder + report + CLI).**
  Митигация: MVC-рефактор ограничен одной функцией (`buildProcPidResult`), остальные треки атомарны.
  TUI-регрессия является блокирующим критерием — любой scope creep обнаруживается на тестах.

- **Риск: golden tar в report/testdata/ не содержит procpidstat entries — тесты потребуют обновления.**
  Митигация: `report/report_test.go` имеет флаг `-update` для регенерации golden файлов;
  новые entries добавляются вместе с тестовым кодом.

- **Риск: record_test.go тесты инвертируются при NotRecordable=false.**
  Митигация: `TestFilterViews_NotRecordable` инвертируется явно; счётчики в `Test_filterViews` обновляются.

## Технические решения

- Мы выбрали Option B (хранить display-строки в tar, `DiffIntvl=[0,0]`), потому что это
  устоявшийся паттерн pgcenter (так работает `activity` view) и не требует изменений в report pipeline.
  Rate-колонки вычисляются recorder при записи, report делает pass-through без пересчёта.
  Альтернатива Option A (хранить raw jiffies, пересчитывать в report через `DiffIntvl=[6,11]`)
  отклонена: cols 6–11 содержат строки `HH:MM:SS` — `diffPair` не может их распарсить без
  дополнительного форматера в report pipeline.

- Мы делаем MVC-разрез `buildProcPidResult` на `buildProcPidResultRaw` + `formatProcPidResultForDisplay`
  для архитектурной чистоты TUI, потому что смешение модели/вида — выявленный tech-debt из 001/002.
  Публичная сигнатура `buildProcPidResult` сохраняется для единственного caller'а (`Collector.Update()`);
  разрез внутренний. Recorder вызывает `buildProcPidResult` целиком.

- Мы добавляем `sysinfo.*` tar entry (отдельный от `meta.*`) для хранения `ticks` и `cpuCount`,
  потому что системные свойства (`CLK_TCK`, число CPU) — другой bounded context от PostgreSQL-метаданных.
  sysinfo пишется на каждом тике для консистентности с паттерном `meta.*`; при Option B значения
  не изменяются и report не использует их для пересчёта — они информационны.

- Мы используем `db.Local` (уже реализован в `postgres.Connect()` через `isLocalhost()`) для
  определения local/remote режима в recorder, потому что переизобретать wheel не нужно.

- Мы добавляем `-N` как новый boolean флаг (не меняем `-A`), потому что `-A` — boolean,
  изменение его семантики сломало бы существующие скрипты.

- Мы не делаем integration тесты для recorder statefulness, потому что procfs-данные
  недетерминированы в CI (паттерн из 001/002).

## Тестирование

**Unit-тесты:** делаются всегда.
- `buildProcPidResultRaw`: числовые значения в cols 0–5 (labels) и 6–11 (jiffies/bytes/ticks)
- `formatProcPidResultForDisplay`: корректное форматирование (HH:MM:SS, %, KiB/s)
- sysinfo JSON write/read round-trip
- Обновить `TestFilterViews_NotRecordable` (инверсия после NotRecordable=false)
- Обновить счётчики в `Test_filterViews` (procpidstat теперь считается)
- Добавить procpidstat + sysinfo entries в golden tar для `Test_app_doReport`

**Интеграционные тесты:** не делаем — procfs-данные недетерминированы в CI (паттерн из 001/002).

**E2E:** агент запускает `pgcenter record -c 3 -i 1s -f /tmp/test.tar`, затем
`pgcenter report -N -f /tmp/test.tar` — проверяет непустой вывод.

## Как проверить

### Агент проверяет

| Шаг | Инструмент | Ожидаемый результат |
|-----|-----------|-------------------|
| 1. `make build` | bash | Сборка без ошибок |
| 2. `make test` | bash | Все тесты зелёные, включая новые procpidstat-record/report тесты |
| 3. `make lint` | bash | Нет новых предупреждений |
| 4. Record 3 снапшота | `./bin/pgcenter record -c 3 -i 1s -f /tmp/test.tar` | Файл создан, нет ошибок |
| 5. Report -N | `./bin/pgcenter report -N -f /tmp/test.tar` | Непустой вывод с временными метками |
| 6. Report -d -N | `./bin/pgcenter report -d -N -f /tmp/test.tar` | Describe показывает колонки procpidstat |
| 7. Backward compat | `./bin/pgcenter report -A -f /tmp/test.tar` | Activity report работает без изменений |
| 8. Report на старом tar | `./bin/pgcenter report -N -f report/testdata/pgcenter.stat.golden.tar` | INFO без паники |

### Пользователь проверяет

- Запустить `pgcenter record` на 30–60 секунд под реальной нагрузкой на PostgreSQL.
  Запустить `pgcenter report -N -f stats.tar` — вывод непустой, форматирование корректное
  (колонки выровнены, временные метки видны, `%all` содержит числа).
- Запустить `pgcenter report -N -f stats.tar -o "%all"` — строки отсортированы по CPU-нагрузке.
- Открыть `pgcenter top → Shift+S` — экран per-process stats работает как раньше (regression check).

## Post-implementation

<!-- This section is filled automatically by /done during feature finalization.
     It captures divergences between the original spec and the actual result.
     DO NOT fill manually — this is maintained by the reconciliation process. -->
