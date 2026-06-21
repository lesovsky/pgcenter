---
# Creation date (YYYY-MM-DD)
created: 2026-06-21

# Status: draft | approved
status: approved

# Work type: feature | bug | refactoring
type: feature

# Feature size: S (1-3 files, local fix) | M (several components) | L (new architecture)
size: M
---

# User Spec: pg_stat_io screen (unified IO breakdown)

## Что делаем

Добавляем в `pgcenter top` новый многострочный экран для `pg_stat_io` (PostgreSQL 16+),
показывающий ввод-вывод в разрезе **источник × тип объекта × контекст**
(`backend_type × object × context`) — по одной строке на комбинацию. Источник — системная
вьюха `pg_stat_io`; кумулятивные счётчики показываются дельтой за интервал (рейты), как и в
остальных экранах pgcenter.

Поскольку у вьюхи очень много колонок, а в pgcenter **нет горизонтального скролла**, экран
разбит на **два под-экрана** (как `pg_stat_statements`):

- **count-экран** — счётчики операций (`reads`, `writes`, `extends`, `writebacks`, `hits`,
  `evictions`, `reuses`, `fsyncs`) плюс объём throughput в KiB для data-moving операций
  (`reads`/`writes`/`extends`);
- **time-экран** — задержки операций (`read_time`, `write_time`, `writeback_time`,
  `extend_time`, `fsync_time`).

Навигация: `j` открывает экран и переключает count↔time; `J` открывает меню выбора под-экрана.
В релизе 0.11.0 экран доступен только в интерактивном режиме (`top`) — запись
(`record`/`report`) выносится в **отдельную фичу** того же релиза (сначала реализация в `top`,
затем слой записи/отчётов), поэтому здесь вьюхи помечаются `NotRecordable: true`.

## Зачем

Сейчас pgcenter не показывает разбивку IO по источникам **вообще**. Во время инцидента
«всплеск дисковой нагрузки» DBA не может средствами pgcenter отличить, кто генерирует IO:
автовакуум, клиентские бэкенды, checkpointer или background writer, — и через какой контекст
(`normal` / `vacuum` / `bulkread` / `bulkwrite`). Это приходится выяснять руками в `psql`.

Дополнительно `pg_stat_io` — единственный источник данных, которые **уехали из других вьюх**:

- на PG 17+ `buffers_backend` / `buffers_backend_fsync` убрали из `pg_stat_bgwriter` — теперь
  они только в `pg_stat_io` (экран bgwriter в pgcenter уже вынужденно их не показывает на 17+);
- на PG 18 тайминги WAL-IO убрали из `pg_stat_wal` — они тоже переехали сюда (`object='wal'`).

Без этого экрана обе области — слепые пятна.

## Пользовательские истории

- Как DBA при всплеске IO, я хочу видеть разбивку `reads`/`writes` по `backend_type × context`,
  чтобы отличить vacuum-driven нагрузку от client-driven.
- Как DBA на PG 18, я хочу видеть WAL-IO (`object='wal'`), который ушёл из `pg_stat_wal`,
  чтобы контролировать давление на запись/fsync WAL.
- Как DBA, я хочу замечать высокие `evictions`/`reuses`, чтобы судить о нехватке
  `shared_buffers` и о churn кольцевых буферов.
- Как DBA с включённым `track_io_timing`, я хочу видеть латентность `read`/`write`/`fsync` по
  источникам, чтобы находить медленные IO-пути.

### Пользовательские сценарии

**Сценарий 1: триаж всплеска IO (count-экран)**
1. Пользователь нажимает `j` — открывается count-экран `pg_stat_io`.
2. Система показывает строки `backend_type / object / context` с рейтами операций, по умолчанию
   отсортированные по `reads` (по убыванию).
3. Пользователь нажимает `/` и фильтрует по колонке `backend_type` (например, `autovacuum`),
   чтобы изолировать вклад автовакуума.
4. Результат: видно, что основной поток `reads` идёт от `autovacuum worker` в контексте
   `vacuum`, а не от клиентов — направление расследования определено.

**Сценарий 2: давление WAL на PG 18 (count-экран)**
1. Пользователь на count-экране нажимает `/` и фильтрует `object = wal`.
2. Система оставляет только WAL-строки.
3. Пользователь смотрит `write,KiB` и `fsyncs`.
4. Результат: видно интенсивность записи и синхронизации WAL — данные, которых на PG 18 больше
   нет в `pg_stat_wal`.

**Сценарий 3: латентность IO (time-экран)**
1. Пользователь нажимает `j` ещё раз — переключение на time-экран.
2. Если `track_io_timing = off`, система показывает в командной строке подсказку
   `track_io_timing is off — timings unavailable`; иначе — рейты таймингов.
3. Пользователь сортирует по `fsync_time`.
4. Результат: видно, какой источник создаёт наибольшую задержку на fsync.

## Дизайн и интерфейс

### Страницы / экраны

- **count-экран `pg_stat_io` (по умолчанию при входе):** многострочная таблица. Колонки:
  `io_key` (короткий md5-ключ для сопоставления строк, как `queryid` в `pg_stat_statements`),
  `backend_type`, `object`, `context` (идентификация, абсолютные), затем рейты —
  `reads`, `read,KiB`, `writes`, `write,KiB`, `extends`, `ext,KiB`, `hits`, `evictions`,
  `writebacks`, `reuses`, `fsyncs`, и в конце `stats_age` (абсолютная). Объём в KiB считается
  гибридно: на PG 16/17 — производно из `op_bytes` (`reads*op_bytes/1024` и т.д.), на PG 18 —
  из нативных `read_bytes`/`write_bytes`/`extend_bytes`.
- **time-экран `pg_stat_io`:** та же идентификация (`backend_type`, `object`, `context`), затем
  рейты `read_time`, `write_time`, `writeback_time`, `extend_time`, `fsync_time` и `stats_age`.

Набор строк на обоих под-экранах одинаков.

### Ключевые компоненты

- **Меню выбора под-экрана (`J`):** всплывающее меню из 2 пунктов —
  `pg_stat_io operations` (= count-экран) / `pg_stat_io timings` (= time-экран),
  по образцу меню `pg_stat_statements`.
- **Тоггл (`j`):** заход на экран и переключение count↔time без меню.
- **Сортировка / фильтр:** штатные средства pgcenter — стрелки/`<` для сортировки по любой
  колонке, `/` для regex-фильтра по выбранной колонке.

### Формы и ввод данных

Форм ввода нет (это экран наблюдения). Единственный ввод — существующий диалог фильтра по `/`.

### UX-поведение

- Вход на экран — по умолчанию count-экран, сортировка по `reads` (убыв.).
- Строки, где все count-счётчики `NULL`/0 (неприменимые комбинации), **не показываются**.
- При `track_io_timing = off` time-экран показывает нули (строки не скрываются) и подсказку в
  командной строке.
- На PostgreSQL 14/15 (где `pg_stat_io` нет) — штатное сообщение
  `ERROR: selected statistics is not supported by current version of Postgres` в основной
  области, без паники и без пустого экрана.
- **Приоритет колонок:** идентификация (`backend_type`, `object`, `context`) и ключевые рейты
  (`reads`, `writes`) идут первыми и остаются видимыми на типичных терминалах 120–160 колонок;
  менее критичные (`writebacks`, `reuses`, `fsyncs`, `stats_age`) уходят в хвост и обрезаются
  первыми.
- Идентичность строки сопоставляется между сэмплами по технической колонке `io_key` (короткий
  md5), которая отображается как первая колонка — по образцу `queryid` в `pg_stat_statements`.
  Читаемые `backend_type/object/context` показываются отдельными колонками.
- В help-экране (`h`/`F1`) отражено, что `Q` **не сбрасывает** `pg_stat_io` (это shared-статистика).

## Как должно работать

### Основной сценарий

1. Пользователь нажимает `j` (или `J` → выбор пункта меню).
2. pgcenter переключается на соответствующий под-экран `pg_stat_io` и при каждом обновлении
   опрашивает вьюху, считает дельты счётчиков относительно прошлого сэмпла и показывает рейты.
3. Пользователь сортирует/фильтрует строки и находит источник нагрузки.

### Граничные случаи

- `pg_stat_io` отсутствует (PG < 16) → штатное сообщение «not supported».
- Неприменимые комбинации дают `NULL` в счётчиках (например, `fsyncs` для `temp relation`,
  `reads` для `background writer`) → такие значения показываются как 0, строка и экран остаются
  корректными.
- Standby: `pg_stat_io` заполнен (`backend_type='startup'`) — специальной обработки не требуется.
- PG 18 добавляет `context='init'` (инициализация WAL-сегментов) — отображается как обычные
  строки, без спец-кейса.
- Узкий терминал — хвостовые колонки обрезаются (штатное поведение pgcenter).

## Критерии приёмки

- [ ] `j` открывает count-экран; повторное `j` переключает count↔time; `J` открывает меню из 2 пунктов.
- [ ] Оба под-экрана показывают корректные per-interval рейты по всем непустым строкам; счётчики — дельтой, идентификация и `stats_age` — абсолютные.
- [ ] Строки, где все count-счётчики `NULL`/0, скрыты (SQL-`WHERE`); набор строк одинаков на обоих под-экранах.
- [ ] PG 16/17 и PG 18 показывают корректный объём IO в KiB; на PG 18 присутствуют строки `object='wal'`; данные согласованы на всех поддерживаемых версиях.
- [ ] PG 14/15: штатное сообщение «not supported», без паники и пустого экрана.
- [ ] `track_io_timing = off`: time-экран показывает нули (строки не скрыты) и подсказку в командной строке.
- [ ] Дельты корректны, когда несколько строк делят один `backend_type` (строки сопоставляются между сэмплами по полной идентичности `backend_type × object × context`).
- [ ] help-экран документирует, что `Q` не сбрасывает `pg_stat_io`.

## Ограничения

- Только PostgreSQL **16+** (на 14/15 экран недоступен; pgcenter поддерживает 14–18).
- Только интерактивный режим `top`; обе вьюхи помечены `NotRecordable: true`. Поддержка
  `record`/`report` — **отдельная фича** релиза 0.11.0 (top-first), не входит в объём.
- Ширина count-экрана ориентировочно ~165 колонок (оценочно, зависит от данных); на более узких
  терминалах хвостовые колонки обрезаются — приоритет колонок описан в разделе «UX-поведение».
- Совместимость: чтение системной вьюхи, без новых внешних зависимостей; работает через существующее
  соединение `pgx/v5` и пайплайн query→format→diff.

## Риски

- **Риск 1 (высокий):** `NULL` внутри диапазона дельт обрушивает весь сэмпл (пустой экран).
  **Митигация:** `coalesce(...,0)` на каждой diff-колонке + unit-тест со строкой, содержащей `NULL`.
- **Риск 2 (средний):** ширина count-экрана ориентировочно ~165 колонок — на узких терминалах обрезается хвост.
  **Митигация:** осознанно принято; полный набор как у `statements`; колонки сортируемы и
  фильтруемы для выбора релевантных строк.
- **Риск 3 (низкий):** `numeric`-поля `*_bytes` на PG 18 рендерятся с десятичной точкой.
  **Митигация:** целочисленное `/1024` в SQL сохраняет integer-тип.
- **Риск 4 (низкий):** забыть синтетический `io_key` → тихий баг неверных дельт (строки матчатся
  по одному `backend_type`). **Митигация:** явный критерий приёмки + тест сопоставления строк.

## Технические решения

- Делаем **два под-экрана** (count/time), а не одну таблицу, потому что у pgcenter нет
  горизонтального скролла, а колонок слишком много (как и у `pg_stat_statements`). Потеря
  «увидеть `reads` и `read_time` в одной строке» — осознанный размен; урезанный однокранный
  вариант убил бы истории про `evictions`/`reuses` и WAL-fsync.
- Показываем **только рейты** (дельты), без абсолютных тоталов: это live-инструмент, сигнал — в
  скорости; `stats_age` — единственная абсолютная метрика (как в `wal`/`bgwriter`).
- На count-экране показываем **и операции, и объём (bytes)**; источник bytes версионно-зависим
  (`op_bytes` до 18, нативные `*_bytes` с 18).
- Делаем **три отдельные сортируемые колонки** `backend_type/object/context` (а не одну
  склейку), чтобы per-column фильтр `/` оставался полезным на ~30 строках.
- Скрываем строки с нулевыми операциями на стороне **SQL-запроса** — так набор строк одинаков на
  обоих под-экранах.
- Не делаем `record`/`report` в этой фиче — иначе размер становится XL; выносим в отдельную
  фичу 0.11.0.

## Тестирование

**Unit-тесты:** делаются всегда, не обсуждаются. Селектор запроса (`io_test.go`) на версиях
{14,15,16,17,18} — проверка `(Ncols, DiffIntvl)` per-branch; отдельный тест diff-безопасности на
строке с `NULL`.

**Интеграционные тесты:** делаем — live-запрос к `pg_stat_io` на CI-матрице PG 14–18; прогон на
PG 18 — реальный гейт нативных `*_bytes` и строк `object='wal'`. Локально поднят только PG 17,
остальные версии — `t.Skipf`.

**E2E тесты:** не делаем — у проекта нет E2E-уровня для TUI; функциональное покрытие даёт
интеграция + ручной QA.

## Как проверить

### Агент проверяет

| Шаг | Инструмент | Ожидаемый результат |
|-----|-----------|-------------------|
| `make build` | bash | Бинарь `./bin/pgcenter` собран без ошибок |
| `make test` | bash | Unit + integration зелёные (локально PG 17; недоступные версии — Skipf) |
| Запрос на каждой версии | go test | Корректная форма и данные для PG 16/17 и PG 18 |
| Diff со строкой, содержащей `NULL` | go test | Экран не обрушается, пустые значения как 0 |
| Тест меню под-экранов | go test | Меню `pg_stat_io` содержит ровно 2 пункта |

### Пользователь проверяет

- На живом PG 17: нажать `j`, пройти count↔time, проверить сортировку по `reads`, фильтр `/` по
  `backend_type`/`object` — убедиться, что строки и рейты осмысленны (US1–US4).
- При наличии PG 18: проверить присутствие строк `object='wal'` и нативных bytes; сверить порядок
  величин bytes с PG 17 (расхождение из-за `io_combine_limit` ожидаемо, не баг).
- На PG 14/15: убедиться, что `j` даёт сообщение «not supported», без паники.

## Post-implementation

Updated: 2026-06-21

### Divergences from original spec

- **io_key visibility:** original spec assumed the synthetic row-matching key could be a hidden
  service column → it is **displayed** as the first column (like `pg_stat_statements`' `queryid`).
  Reason: pgcenter has no column-hide mechanism — `internal/align.SetAlign` floors every column at
  width 8 and `ColsWidth` is a runtime cache, not a preset. Discovered during tech-spec code
  verification; the spec body was updated before implementation.
- **track_io_timing hint:** AC8 described "when all timing columns are zero, show a hint"; implemented
  (Decision 9) as a **static cmdline `Msg`** on the time view ("requires track_io_timing=on"), shown
  on switch rather than computed by scanning each sample. Same user intent, simpler.
- **Column order:** count-screen columns were reordered during validation so lower-priority
  `writebacks`/`reuses`/`fsyncs` trail `hits`/`evictions` (priority-based clipping on narrow terminals).

### Added during implementation

- Nothing beyond the spec. `PostgresV15/16/17/18` constants and the test files are enabling code.
- A CI-only regression fix: adding two `NotRecordable` views required updating `record`'s
  `Test_filterViews` expected counts (caught by CI, not the sandbox). Lesson recorded in `patterns.md`.

### Descoped / Deferred

- record/report support — deliberately a separate 0.11.0 feature (TUI-first), not a regression.
- Behavioral `diff()` NULL-safety test — logged as tech-debt [007] (import-cycle constraint).
