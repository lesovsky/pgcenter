---
status: planned
depends_on: []
wave: 1
skills: [code-writing]
verify: "bash — go test ./internal/query/... ./internal/view/... && go test ./top/ -run 'Test_orderKey'"
reviewers: [dev-code-reviewer, dev-security-auditor, dev-test-reviewer]
teammate_name:
---

# Task 01: PG 13+ activity query branch

## Required Skills

Перед выполнением задачи загрузи:
- `/skill:code-writing` — [skills/code-writing/SKILL.md](~/.claude/skills/code-writing/SKILL.md)

## Description

Экран `activity` получает три новые колонки на PostgreSQL 13 и новее: `leader` (идентификатор
группы параллельного запроса), `backend_xid` (писала ли транзакция) и `horizon_xacts` (на сколько
транзакций назад сессия держит горизонт xmin). Это ядро фичи — всё остальное (сортировка разреженных
колонок, описания в `report -d -A`) навешивается на результат этой задачи.

Механика простая: в `SelectStatActivityQuery` добавляется **ранний возврат** новой константы
`PgStatActivityPG13` с `Ncols = 17` **над** существующим `switch`. Сам `switch` не трогается,
ничего не переименовывается, seed в `view.New()` (`Ncols: 14`) остаётся как есть — обоснование
целиком в Decision 3 tech-spec'а. Сигнатура селектора остаётся 2-кортежем `(string, int)`:
`DiffIntvl` у `activity` — `{0, 0}`, `UniqueKey` — 0, и ни то, ни другое не меняется, потому что
`pid` остаётся колонкой 0.

Вторая половина задачи — **тесты, которые реально проверяют, а не подтверждают**. Их две штуки, и
обе закрывают конкретные дыры:

1. **Граница ветки не проверяется живьём.** `leader_pid` появился в PG 13, но в тестовом образе есть
   только PG 14–19. Если написать `version >= 140000` вместо `130000`, **все живые проверки пройдут**
   — ошибка станет видна только у пользователя на PG 13. Единственный страж — табличный тест, и
   поэтому он обязан пиннить границу **с обеих сторон**: PG 12 → старая ветка / 14 колонок, PG 13 →
   новая ветка / 17 колонок (Decision 2).
2. **Живой тест сегодня ничего не утверждает о раскладке.** `Test_StatActivityQueries` выбрасывает
   `Ncols` в `_` и проверяет только «запрос не упал». Заявленная раскладка из 17 колонок так и
   останется заявленной. Тест переводится на `conn.Query` + `rows.FieldDescriptions()` и проверяет
   **имена колонок и их порядок** — по образцу `bgwriter_test.go` / `progress_vacuum_test.go`, только
   строже (те сверяют лишь количество).

Третья точка — `TestViews_Configure`: табличный тест доказывает, что *селектор* возвращает нужную
константу, но не то, что `Configure` донесла её до view. Блоки `case 130000:` / `case 120000:` в
`internal/view/view_test.go` уже существуют, но говорят только про `replication` — в них добавляются
утверждения про `activity`. Так же пиннила свою границу фича 012.

## What to do

1. **`internal/query/query.go`** — убедиться, что константа `PostgresV13 = 130000` есть.
   **Она уже присутствует** (строка 16, рядом с `PostgresV12`/`PostgresV14`) — так же говорит и
   tech-spec («No new version constant is needed: `PostgresV13` already exists in `query.go`»).
   Добавлять ничего не нужно, второе объявление просто не скомпилируется. Файл, скорее всего,
   останется неизменённым; если так — сказать об этом в отчёте, а не искать, что бы в нём поправить.

2. **`internal/query/activity.go`** — добавить константу `PgStatActivityPG13` с раскладкой из 17
   колонок (таблица ниже, порядок обязателен дословно). Комментарий над константой — в стиле файла:
   что за запрос, с какой версии, что дают три новые колонки.

3. **`internal/query/activity.go`** — в `SelectStatActivityQuery` добавить ранний возврат
   `if version >= PostgresV13 { return PgStatActivityPG13, 17 }` **выше** существующего `switch`.
   `switch` не редактировать, `PgStatActivityDefault` не переименовывать. Уместен короткий
   комментарий, что после появления ранней ветки `default:` покрывает PG 10–12.

4. **`internal/query/activity_test.go`, `TestSelectStatActivityQuery`** — расширить таблицу так,
   чтобы граница была зажата с обеих сторон: PG 12 остаётся на старом запросе с 14 колонками,
   PG 13 переходит на новый с 17. Существующие кейсы (9.5 / 9.6 / 10) сохранить. Добавить и
   современную версию (например 19), чтобы было видно: выше границы ветка одна.

5. **`internal/query/activity_test.go`, `Test_StatActivityQueries`** — заменить проверку «запрос
   выполняется» на проверку **имён колонок и их порядка**: выполнять через `conn.Query`, сверять
   `rows.FieldDescriptions()` с ожидаемым списком имён для этой версии. Список версий в тесте уже
   полный — **не расширять его**. Недоступные версии по-прежнему скипаются существующим
   `t.Skipf`. Проверка количества колонок вытекает из проверки списка — отдельного `assert.Len`
   не требуется, но сверка `Ncols` из селектора с длиной списка лишней не будет.

6. **`internal/view/view_test.go`, `TestViews_Configure`** — в существующий `case 130000:` добавить
   утверждения, что `views["activity"]` получил `PgStatActivityPG13` и `Ncols == 17`; в `case
   120000:` — что он получил `PgStatActivityDefault` и `Ncols == 14`. Существующие утверждения про
   `replication`/`statements_timings` в этих блоках не трогать. Матрицу версий не расширять — кейсы
   на 120000 и 130000 в ней уже есть.

7. **Не трогать** `internal/view/view.go` — ни seed `activity` (`Ncols: 14`), ни проводку в
   `Configure` (`case "activity"` уже вызывает селектор). Не трогать `internal/stat/`, `report/`,
   `top/`. Описания колонок для `report -d -A` — задача 4, не эта.

## TDD Anchor

Тесты пишем ДО реализации, убеждаемся что падают, потом пишем код.

- `internal/query/activity_test.go::TestSelectStatActivityQuery` — граница ветки зажата с обеих
  сторон: `120000` → `PgStatActivityDefault` / 14 колонок, `130000` → `PgStatActivityPG13` / 17
  колонок; старые кейсы 90500/90600/100000 продолжают возвращать своё. **Это единственная защита от
  `>= 140000`**, живьём граница не проверяема.
- `internal/query/activity_test.go::Test_StatActivityQueries` — на каждой доступной живой версии
  (PG 14–19) `rows.FieldDescriptions()` возвращает ровно ожидаемые имена колонок в ожидаемом
  порядке; `leader` — индекс 1, `backend_xid` — 11, `horizon_xacts` — 12. Падает до реализации,
  потому что сейчас запрос отдаёт 14 колонок без новых имён.
- `internal/view/view_test.go::TestViews_Configure` — после `Configure` на версии `130000`
  `views["activity"].QueryTmpl == query.PgStatActivityPG13` и `Ncols == 17`; на `120000` —
  `PgStatActivityDefault` и `Ncols == 14`. Проверяет, что новая ветка доезжает до view, а не только
  возвращается селектором.

## Acceptance Criteria

- [ ] `SelectStatActivityQuery(130000)` возвращает `PgStatActivityPG13` и 17; `SelectStatActivityQuery(120000)`
      возвращает `PgStatActivityDefault` и 14 — обе стороны границы зафиксированы табличным тестом
- [ ] Раскладка PG 13+ соответствует таблице Data Models дословно: `leader` на позиции 1,
      `backend_xid` на 11, `horizon_xacts` на 12, `query` остаётся последней
- [ ] `leader` = `coalesce(leader_pid, pid)`, `backend_xid` = `backend_xid::text`,
      `horizon_xacts` = `age(backend_xmin)`; `coalesce` на xid-колонках **отсутствует** — иначе
      пустое значение отрендерится как `0` и сломает требование «пусто, никогда не 0»
- [ ] Живые серверы PG 14–19 возвращают ровно указанные имена колонок в указанном порядке
      (`rows.FieldDescriptions()`), а не просто «запрос не упал»
- [ ] Существующий `switch` в `SelectStatActivityQuery` не изменён; `PgStatActivityDefault`,
      `PgStatActivity96`, `PgStatActivity95` не переименованы
- [ ] Сигнатура `SelectStatActivityQuery` осталась `(string, int)`; `DiffIntvl` и `UniqueKey`
      у `activity` не менялись
- [ ] `view.New()` не изменён — seed `activity` по-прежнему `PgStatActivityDefault` с `Ncols: 14`
- [ ] `TestViews_Configure` проверяет `activity` по обе стороны границы
- [ ] Список версий в `Test_StatActivityQueries` не расширялся
- [ ] `go test ./top/ -run 'Test_orderKey'` зелёный — страж seed'а `Ncols == 14` не сдвинут
- [ ] `go test ./internal/query/... ./internal/view/...` зелёный; `make lint` без замечаний

## Context Files

**Feature artifacts:**
- [013-feat-activity-xmin-horizon.md](013-feat-activity-xmin-horizon.md) — user-spec: семантика трёх
  колонок, требование «пусто, никогда не 0», сценарии 1–3
- [013-feat-activity-xmin-horizon-tech-spec.md](013-feat-activity-xmin-horizon-tech-spec.md) —
  Task 1, **Data Models** (раскладка колонок — источник истины), Decision 1 (почему в середину, а не
  в хвост), Decision 2 (граница PG 13 и её непроверяемость), Decision 3 (ранний возврат, ничего не
  переименовывается, seed не трогается), Decision 7 (какой каст нужен, а какой нет)
- [013-feat-activity-xmin-horizon-decisions.md](013-feat-activity-xmin-horizon-decisions.md) —
  decisions log (файла ещё нет, создаётся в Post-completion)

**Project knowledge:**
- [overview.md](../../../.claude/skills/project-knowledge/overview.md) — что за проект, какие
  статистики поддерживаются
- [architecture.md](../../../.claude/skills/project-knowledge/architecture.md) — раскладка пакетов,
  поток данных, работа с версиями PG; раздел про version-aware селекторы (строка про
  `SelectStatActivityQuery` устареет после этой задачи — правит её задача 5, не эта)
- [patterns.md](../../../.claude/skills/project-knowledge/patterns.md) — идиома version branching в
  `internal/query`, конвенции тестов (табличные тесты, живые кластеры, `t.Skipf` при недоступности)

**Code files (изменяем):**
- [internal/query/activity.go](../../../internal/query/activity.go) — три константы и селектор из
  трёх веток. **Добавить** `PgStatActivityPG13` и ранний возврат над `switch`.
- [internal/query/activity_test.go](../../../internal/query/activity_test.go) — `TestSelectStatActivityQuery`
  (таблица на 3 кейса) и `Test_StatActivityQueries` (живой прогон через `conn.Exec`, `Ncols`
  выброшен в `_`). **Изменить оба.**
- [internal/query/query.go](../../../internal/query/query.go) — константы версий. `PostgresV13`
  **уже есть** (строка 16); только проверить.
- [internal/view/view_test.go](../../../internal/view/view_test.go) — `TestViews_Configure`, блоки
  `case 130000:` (строка 203) и `case 120000:` (строка 210). **Добавить** утверждения про `activity`.

**Code files (только читаем):**
- [internal/query/replication.go](../../../internal/query/replication.go) — прецедент работы с
  `backend_xmin`: `backend_xmin::text::bigint` ради арифметики (мотив другой, техника та же);
  там же живёт одноимённая колонка `horizon_xacts` с другой формулой
- [internal/query/wal.go](../../../internal/query/wal.go) — форма селектора с ранним `if version >= ...`
- [internal/query/progress_vacuum.go](../../../internal/query/progress_vacuum.go) — ближайший
  прецедент (фича 012): ранний возврат + константа с суффиксом версии, вставка колонок в середину
- [internal/query/progress_vacuum_test.go](../../../internal/query/progress_vacuum_test.go) —
  форма живого теста и отдельного табличного теста на селектор
- [internal/query/bgwriter_test.go](../../../internal/query/bgwriter_test.go) — та же форма с
  `defer conn.Close()` и `rows.Err()`
- [internal/view/view.go](../../../internal/view/view.go) — seed `activity` (строки 39–49) и
  `Configure` (строка 375). **Не изменяем.**
- [top/config_view_test.go](../../../top/config_view_test.go) — `Test_orderKeyLeft` (строка 13) и
  `Test_orderKeyRight` (строка 45): два утверждения (`0 → 13`, `13 → 0`) считают последний индекс от
  seed'а `activity`. Живой страж Decision 3. **Не изменяем**, но прогоняем.

## Verification Steps

- Поднят тестовый контейнер `lesovsky/pgcenter-testing:0.0.11` с кластерами PG 14–19 на портах
  21914–21919 (иначе живая половина проверки молча скипнется).
- `go test ./internal/query/... ./internal/view/...` — зелёный.
- `go test -v -run 'Test_StatActivityQueries' ./internal/query/...` — сабтесты
  `pg_stat_activity/140000` … `/190000` действительно **выполнились**, а не `SKIP`. Если все шесть
  скипнулись, живая проверка раскладки не состоялась — поднять кластеры и прогнать заново.
- `go test -v -run 'TestSelectStatActivityQuery' ./internal/query/...` — в таблице видны кейсы
  `120000` и `130000`.
- `go test ./top/ -run 'Test_orderKey'` — зелёный. Это прямой страж Decision 3: `Test_orderKeyLeft`
  (`0 → 13`) и `Test_orderKeyRight` (`13 → 0`) считают последний индекс от seed'а `activity`
  (`Ncols: 14`). Если seed случайно подняли до 17, оба падают — и падают только здесь, ни
  `internal/query`, ни `internal/view` этого не заметят. Эти два теста БД не требуют и обязаны
  выполниться, а не скипнуться.
- Ручная перекрёстная сверка: имена и порядок колонок в тесте совпадают с таблицей Data Models
  tech-spec'а. Это единственный источник истины для PG 12/13, где живой проверки нет.
- `make lint` — чисто. Длина строки самой константы не проверяется: `lll` в `.golangci.yml`
  не включён (там поверх дефолтов v2 только `gocritic` и `revive`), а `gofmt` длину не трогает.
  Соседние константы в этом файле уже длинные — ориентируйся на них.
- `git diff --stat` — затронуты только четыре файла из «изменяем»; `internal/view/view.go`,
  `internal/stat/`, `report/`, `top/` в диффе отсутствуют.

## Details

**Раскладка PG 13+ (17 колонок, порядок обязателен дословно):**

| idx | имя колонки | выражение в SQL |
|-----|-------------|-----------------|
| 0 | `pid` | `pid` |
| 1 | `leader` | `coalesce(leader_pid, pid) AS leader` |
| 2 | `cl_addr` | `host(client_addr) AS cl_addr` |
| 3 | `cl_port` | `client_port AS cl_port` |
| 4 | `datname` | `datname` |
| 5 | `usename` | `usename` |
| 6 | `appname` | `application_name AS appname` |
| 7 | `backend_type` | `backend_type` |
| 8 | `wait_etype` | `wait_event_type AS wait_etype` |
| 9 | `wait_event` | `wait_event` |
| 10 | `state` | `state` |
| 11 | `backend_xid` | `backend_xid::text` |
| 12 | `horizon_xacts` | `age(backend_xmin) AS horizon_xacts` |
| 13 | `xact_age` | `date_trunc('seconds', clock_timestamp() - xact_start)::text AS xact_age` |
| 14 | `query_age` | `date_trunc('seconds', clock_timestamp() - query_start)::text AS query_age` |
| 15 | `change_age` | `date_trunc('seconds', clock_timestamp() - state_change)::text AS change_age` |
| 16 | `query` | `regexp_replace(query, E'\\s+', ' ', 'g') AS query` |

Всё, что вне трёх новых позиций, копируется из `PgStatActivityDefault` без изменений — включая
`FROM pg_stat_activity`, фильтр по `{{.QueryAgeThresh}}`, блок `{{ if .ShowNoIdle }}` и
`ORDER BY pid DESC`. Шаблонные плейсхолдеры обязаны сохраниться: без них `Format()` вернёт запрос
с пустым фильтром, и тест этого не поймает.

**Ожидаемые списки имён для живого теста** (выбираются той же границей, что и в селекторе):

- PG 13+ (17) — таблица выше.
- PG 10–12 (14, `PgStatActivityDefault`): `pid, cl_addr, cl_port, datname, usename, appname,
  backend_type, wait_etype, wait_event, state, xact_age, query_age, change_age, query`.
- PG 9.6 (13, `PgStatActivity96`): то же, но без `backend_type`.
- PG 9.5 (12, `PgStatActivity95`): `pid, cl_addr, cl_port, datname, usename, appname, waiting,
  state, xact_age, query_age, change_age, query`.

Списки ниже PG 13 в тестовом окружении всегда скипаются — но они должны быть в тесте, иначе
`t.Skipf` придётся ставить раньше выбора списка, и тест станет неполным на случай появления старых
кластеров.

**Files:**
- `internal/query/activity.go` — сейчас: три константы (`PgStatActivityDefault` 14 колонок,
  `PgStatActivity96` 13, `PgStatActivity95` 12) и `SelectStatActivityQuery` со `switch` из трёх
  веток, где `default:` отдаёт `PgStatActivityDefault, 14`. **Сделать:** добавить четвёртую
  константу `PgStatActivityPG13` и ранний `if version >= PostgresV13` над `switch`.
- `internal/query/activity_test.go` — сейчас: `TestSelectStatActivityQuery` (три кейса, без
  `t.Run`) и `Test_StatActivityQueries` (`tmpl, _ := SelectStatActivityQuery(version)`, затем
  `conn.Exec(q)` и `assert.NoError`). **Сделать:** расширить таблицу; переписать живой тест на
  `conn.Query` + сверку `rows.FieldDescriptions()`.
- `internal/query/query.go` — `PostgresV13 = 130000` уже объявлена. **Ничего не добавлять.**
- `internal/view/view_test.go` — `TestViews_Configure`, матрица версий 9.4–19 × recovery ×
  trackCommit × querylen, затем `switch tc.version`. **Сделать:** дописать утверждения про
  `activity` в блоки 130000 и 120000.

**Dependencies:** зависимостей от других задач нет (`depends_on: []`, wave 1). Внешне: запущенный
тестовый контейнер с кластерами PG 14–19 для живой части. Задачи 2 (сортировка) и 3 (report) идут
параллельно и этот код не трогают; задача 4 (`describe.go`) читает результат этой задачи.

**Edge cases:**
- **`>= 140000` вместо `>= 130000`** — самый вероятный тихий провал. Все живые проверки пройдут,
  потому что PG 13 в образе нет. Ловится только кейсом `120000`/`130000` в табличном тесте;
  проверить его глазами после написания.
- **Имя колонки у `backend_xid::text`.** PostgreSQL сохраняет имя исходной колонки при касте
  простой ссылки, поэтому имя ожидается `backend_xid`. Если живой тест покажет `?column?` или
  `text` — добавить явный `AS backend_xid` и отметить это в отчёте. Живая проверка имён здесь и
  нужна именно за этим.
- **Каст на `age(backend_xmin)` не нужен** (Decision 7, измерено): `age(xid)` возвращает `integer`
  и сканируется в строковую матрицу. Лишний каст читается как несущий смысл и стоит следующему
  читателю похода в исходники.
- **Никакого `coalesce` на `backend_xid` и `horizon_xacts`.** NULL должен доехать до рендерера
  пустой строкой — на этом держится требование user-spec «пусто, никогда не 0». `coalesce(..., 0)`
  выглядит аккуратнее и ломает главный сценарий фичи.
- **`leader` наоборот требует `coalesce`**: сырой `leader_pid` пуст у самого лидера и непуст только
  у воркеров, поэтому сортировка по сырой колонке не собирает группу. `coalesce(leader_pid, pid)`
  даёт лидеру собственный `pid` — именно этим группа и склеивается.
- **`leader_pid` появился в PG 13** — на PG 12 и ниже колонки в каталоге нет, запрос упал бы с
  ошибкой. Это и есть причина границы, а не стилистика.
- **Права доступа:** непривилегированный пользователь видит NULL в `backend_xid`/`backend_xmin`
  чужих сессий — `pg_stat_activity` возвращает NULL, а не ошибку. В коде ловить нечего; оговорка
  для оператора добавляется в задаче 4.
- **`Ncols = 17` и seed 14.** Расхождение намеренное: `Configure` перезаписывает seed при коннекте.
  Поднятие seed до 17 ломает два утверждения в `top/config_view_test.go` (`Test_orderKeyLeft` `0 → 13`
  и `Test_orderKeyRight` `13 → 0` — оба считают последний индекс как `Ncols-1`) и ничего не меняет в
  рантайме (Decision 3). Поэтому эти два теста прогоняются явно: `go test ./top/ -run 'Test_orderKey'`.
  Побочно: комментарий в тесте говорит `Ncols == 13`, хотя seed — 14; комментарий врёт, поведение
  верное — **не трогать**, это не наша задача.
- **Тесты `top`/`record` паникуют без живых кластеров независимо от этой задачи**
  (`Test_getQueryReport`, `Test_app_setup`, `Test_tarRecorder`). Причина — nil-pointer в
  `postgres.DB.Close` (`internal/postgres/postgres.go:135` разыменовывает `db.Conn` у `db == nil`)
  после неудачного коннекта: тест идёт дальше вместо `t.Skipf`. Это то же семейство, что уже
  закрытые долги [005] и [008], а **не** долг [019] (тот про девять тестов, где `t.Skipf` внутри
  цикла версий скипает весь оставшийся цикл). Ни то, ни другое не является регрессией от этой
  правки и здесь не «чинится» — важно лишь не принять эту панику за поломку своего кода.

**Implementation hints:**
- Форма селектора — как в `wal.go` и `progress_vacuum.go`: ранний `if` сверху, исторический
  «лестничный» `switch` под ним нетронутым.
- Константа склеивается из строковых литералов по стилю файла; строка с `query` использует
  backtick-литерал из-за `E'\\s+'` — скопировать её как есть.
- В живом тесте сравнивать удобно так: собрать `[]string` из `string(d.Name)` по
  `rows.FieldDescriptions()` (в pgx/v5 `Name` — `string`; если сборка ругается на тип, привести
  явно) и сверить одним `assert.Equal` со срезом ожидаемых имён — тогда при расхождении в диффе
  видно и порядок, и лишние/недостающие колонки. Поэлементные `assert` дают худшую диагностику.
- Не забыть `rows.Close()` и `assert.NoError(t, rows.Err())` — как в `bgwriter_test.go`; без
  `Close()` соединение останется занятым и следующая версия в цикле упадёт непонятной ошибкой.
- Скип недоступной версии остаётся ровно там же, где сейчас — после `postgres.NewTestConnectVersion`.
- Безопасность: запрос — статическая константа-шаблон, пользовательский ввод в неё не подставляется
  (`Format()` подставляет только `Options`, заполняемые самим pgcenter). Новых точек инъекции
  задача не создаёт; новых прав у pgcenter не требует — обе колонки читаются из `pg_stat_activity`
  в пределах уже имеющихся привилегий.

## Reviewers

- **dev-code-reviewer** → `docs/features/013-feat-activity-xmin-horizon/013-feat-activity-xmin-horizon-task-01-dev-code-reviewer-review.json`
- **dev-security-auditor** → `docs/features/013-feat-activity-xmin-horizon/013-feat-activity-xmin-horizon-task-01-dev-security-auditor-review.json`
- **dev-test-reviewer** → `docs/features/013-feat-activity-xmin-horizon/013-feat-activity-xmin-horizon-task-01-dev-test-reviewer-review.json`

## Post-completion

- [ ] Записать краткий отчёт в [013-feat-activity-xmin-horizon-decisions.md](013-feat-activity-xmin-horizon-decisions.md) (Summary: 1-3 предложения, ревью со ссылками на JSON, без таблиц файндингов и дампов)
- [ ] Отдельной строкой отметить, что `PostgresV13` в `query.go` уже существовала и файл не менялся
      (как и предупреждает tech-spec)
- [ ] Зафиксировать фактический результат живой проверки имён колонок: на каких версиях реально
      выполнилось (не скипнулось) и совпал ли порядок с Data Models
- [ ] Если отклонились от спека — описать отклонение и причину
- [ ] Обновить user-spec/tech-spec если что-то изменилось
