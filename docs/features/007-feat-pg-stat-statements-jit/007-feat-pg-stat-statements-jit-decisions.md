# Decisions Log: pg_stat_statements JIT screen

Отчёты агентов о выполнении задач. Каждая запись создаётся агентом, выполнившим задачу.

---

## Task 01: JIT query consts + version selector — internal/query/statements.go

**Status:** Done
**Commit:** 79514ea
**Agent:** jit-query-dev (general-purpose)
**Summary:** Добавлены две SQL-константы по образцу `PgStatStatementsTimingPG13`/`PgStatStatementsIoDefault` — `PgStatStatementsJITPG15` (PG15/16, 13 колонок) и `PgStatStatementsJITDefault` (PG17+, 15 колонок, +`deform_total`/`deform,ms` через `jit_deform_time`); `*_total` через `date_trunc('seconds', round(p.jit_*_time)/1000 * '1 second'::interval)::text`, `*_ms` через `round(p.jit_*_time)`, `functions` = `p.jit_functions`, md5-`queryid`, оба заканчиваются `WHERE p.jit_functions > 0` (Decision 3). Добавлен селектор `SelectStatStatementsJITQuery(version int) (string, int, [2]int, int)` (4-tuple по модели `SelectStatIOQuery` + `UniqueKey`): `>= PostgresV17` → `(Default, 15, {7,12}, 13)`, иначе → `(PG15, 13, {6,10}, 11)`. Покрыто unit-тестом обеих веток + JIT exec-подтест gated PG15+ (`t.Skipf` без PG).
**Deviations:** Нет. Колоночные алиасы, индексы, токены и стиль строки точно соответствуют tech-spec (Decision 2) и образцовым timing/io-константам.
**Tech debt:** Нет. Двойная константа (отличаются только deform-колонками) — намеренная по позиционному align (ADR [006]), флаг code-reviewer'а только чтобы будущий рефактор случайно не слил их в одну строку. Live exec-пути PG15–18 локально не исполнялись (нет тестового PG-кластера) — гейтятся CI-матрицей; подтверждено `--- SKIP` на всех четырёх версиях.

**Reviews:**

*Round 1:*
- dev-code-reviewer: approved, 0 critical/major, 1 minor (намеренное дублирование консты — не менять) → [task-01-dev-code-reviewer-review.json]
- dev-security-auditor: approved, 0 findings (статические шаблоны над server-controlled токенами, нет injection-поверхности) → [task-01-dev-security-auditor-review.json]
- dev-test-reviewer: passed, 0 critical/major, 1 minor (нет проверки границы PG<15 — by design, ветка отсутствует) → [task-01-dev-test-reviewer-review.json]

**Verification:**
- `go test ./internal/query/...` → ok (JIT exec-подтесты `t.Skipf` — PG-кластер недоступен локально; `TestSelectStatStatementsJITQuery` зелёный)
- `go build ./...` → clean; `gofmt -l` → clean; `go vet ./internal/query/...` → clean
- Commit 79514ea: 2 файла, +81 строка

---

## Task 02: Register statements_jit view + Configure + count-test fixes — internal/view/view.go

**Status:** Done
**Commit:** f7defde (view + Configure + count-tests), 304ee62 (review round 1 fix), 5028374 (review reports)
**Agent:** jit-view-dev (general-purpose)
**Summary:** В `view.New()` добавлена запись `statements_jit` по образцу `statements_io` с прецедентом `stat_io` для `MinRequiredVersion: query.PostgresV15` + `NotRecordable: true`: PG15-дефолты `QueryTmpl: query.PgStatStatementsJITPG15`, `Ncols: 13`, `DiffIntvl: {6,10}`, `UniqueKey: 11`, `OrderKey: 2` (gen_total), `OrderDesc: true`, `Msg` с подсказкой про `jit=off` (Decision 4), пустые `ColsWidth`/`Filters`. В `Configure()` добавлен `case "statements_jit":`, патчащий все 4 поля (`QueryTmpl`/`Ncols`/`DiffIntvl`/`UniqueKey`) из `query.SelectStatStatementsJITQuery(opts.Version)` — расширение модели `stat_io` (3 поля) до 4 с `UniqueKey`, т.к. md5-ключ JIT стоит в конце и сдвигается с `Ncols`. Поправлены три count-теста (TDD-якорь): `TestNew` 26→27, `TestView_VersionOK` строка 160000 26→27 (строки ≤140000 без изменений — `VersionOK(<150000)==false`), `Test_filterViews` `wantN +1` на всех 6 строках (`NotRecordable` отбрасывает view ДО версионного гейта, `wantV` неизменно). Добавлен guard-тест `TestNew_StatementsJITView` по образцу `TestNew_StatIOView`. `report/report.go` не тронут (NotRecordable → нет записи описания).
**Deviations:** Нет. Все Acceptance Criteria выполнены точно по task-02-спеку; `report.go` подтверждённо не изменён vs wave-1 база.
**Tech debt:** Нет.

**Reviews:**

*Round 1:*
- dev-code-reviewer: approved, 0 critical/major/minor (cross-file consistency: сигнатура 4-tuple селектора, PG15-дефолты, порядок NotRecordable→версионный гейт, три count-теста — всё подтверждено) → [task-02-dev-code-reviewer-review.json]
- dev-security-auditor: approved, 0 findings (нет injection-поверхности — только присваивание возврата чистой integer-switch функции; md5 — синтетический ключ отображения, не security-control) → [task-02-dev-security-auditor-review.json]
- dev-test-reviewer: approved, 0 critical/major, 1 minor (усилить проверку `Msg` до `assert.Contains(..., "jit=off")` по прецеденту `TestNew_StatIOTimeView`) → [task-02-dev-test-reviewer-review.json]

Minor от dev-test-reviewer применён в 304ee62 (`assert.Contains(t, jit.Msg, "jit=off")` пинит load-bearing роль `Msg` из Decision 4).

**Verification:**
- `go test ./internal/view/...` → ok; `go test ./record/...` (filterViews-тесты) → ok
- `go build ./...` → clean; `go vet ./internal/view/... ./record/...` → clean
- Три count-теста зелёные: `TestNew`=27, `Test_filterViews` (+1 на 6 строках), `TestView_VersionOK` (160000→27)
- `report/report.go` не изменён vs wave-1 база 8631ffd (git diff пуст)
- Примечание: live-DB тесты (`Test_app_record` и пр.) падают с connection-refused/nil-pointer — это pre-existing, требуют живого PG, воспроизводятся на HEAD~1; вне scope Task 02

---

## Task 03: TUI menu item + x-cycle wiring — top/menu.go, top/config_view.go

**Status:** Done
**Commit:** 1d8d678 (menu item + handler + x-cycle), 217b69d (review round 1 fix — обновление существующих regression-тестов)
**Agent:** jit-tui-dev (general-purpose)
**Summary:** Чистая TUI-обвязка нового view `statements_jit` (зарегистрирован в Task 02) через два штатных входа pgss. В `top/menu.go` `selectMenuStyle` `case menuPgss` добавлен 7-й пункт (индекс 6) `" pg_stat_statements JIT compilation"` сразу после `" pg_stat_statements WAL usage"` (leading-space-формат как у соседей; высота меню авто-размер от `len(s.items)` — геометрия не тронута). В `menuSelect` `case menuPgss` добавлен `case 6: viewSwitchHandler(app.config, "statements_jit")` строго между `case 5` и `default`. В `top/config_view.go` `statementsNextView` x-цикл расширен: `case "statements_wal"` теперь `next = "statements_jit"`, добавлен `case "statements_jit": next = "statements_timings"` — порядок цикла стал `… local → wal → jit → timings …`. `top/keybindings.go` не тронут (`x`/`X` уже подключены и уже гейтят пустой `ExtPGSSSchema`).
**Deviations:** TDD-якорь task-спека («у этого TUI-слоя нет существующего unit-покрытия, тесты не писать») оказался ФАКТИЧЕСКИ НЕВЕРНЫМ — найдено dev-test-reviewer (round 1, critical, подтверждено эмпирически `go test ./top/`). Слой покрыт table-driven тестами: `Test_selectMenuStyle` (счётчик пунктов `menuPgss`), `Test_statementsNextView` и `Test_switchViewTo` (x-цикл, включая переход `statements_wal`). Мои изменения сломали 3 существующих теста. Отклонение от плана: вместо «no tests» обновил три устаревших assertion'а под новое поведение (НЕ спекулятивные новые тесты — правка существующих regression-тестов): `menuPgss` count 6→7; `statements_wal → statements_jit` + новый `statements_jit → statements_timings` в обоих cycle-тестах. Это минимальная правка, восстанавливающая зелёный `make test` (CI-гейт проекта), и она же закрывает покрытие нового JIT-перехода. Причина расхождения в спеке зафиксирована для lead'а (TDD Anchor task-03 нуждается в исправлении).
**Tech debt:** Нет. Pre-existing: `Test_doReload` падает nil-pointer на `top/reload.go:18` — требует живого PostgreSQL, воспроизводится идентично на HEAD~2 (чистый worktree), вне scope Task 03.

**Reviews:**

*Round 1:*
- dev-code-reviewer: approved, 0 critical/major/minor (cross-file consistency: индекс пункта 6 совпадает с `case 6`, view `statements_jit` зарегистрирован в Task 02 → `viewSwitchHandler` резолвит реальный view, не zero-value; геометрия меню не тронута) → [task-03-dev-code-reviewer-review.json]
- dev-test-reviewer: **failed**, 2 critical + 1 major — устаревший TDD-якорь, сломаны 3 существующих теста (`Test_selectMenuStyle`, `Test_statementsNextView`, `Test_switchViewTo`) → [task-03-dev-test-reviewer-review.json]

Critical-замечания dev-test-reviewer применены в 217b69d (обновлены три assertion'а + добавлен новый JIT-переход в оба cycle-теста).

*Round 2:*
- dev-test-reviewer: **passed**, 0 findings (все 3 регрессии устранены, litmus-целостность сохранена — тесты проверяют реальное имя view/счётчик, а не моки; новый JIT-переход покрыт) → [task-03-dev-test-reviewer-review-round2.json]

**Verification:**
- `go build ./...` → clean; `go vet ./top/...` → clean
- `go test ./top/ -run 'Test_selectMenuStyle|Test_statementsNextView|Test_switchViewTo'` → все PASS (включая `Test_switchViewTo/14`)
- `make build` → собирает `bin/pgcenter` без ошибок
- Manual (local PG17, верифицирует user): `X` → 7-й пункт «pg_stat_statements JIT compilation», выбор открывает JIT-экран; `x` циклит `… wal → jit → timings …`

---

## Task 04: Pre-deploy QA

**Status:** Done
**Commit:** (wave-4 chore)
**Agent:** основной агент (team lead)
**Summary:** Прогон pre-deploy QA на ветке feature/pg-stat-statements-jit. Фича-поверхность зелёная: `make build` → bin/pgcenter; `go vet ./...` clean; `gofmt -l` clean; целевые тесты `internal/query` (TestSelectStatStatementsJITQuery), `internal/view` (TestNew=27, TestView_VersionOK 160000→27, TestNew_StatementsJITView), `record` (Test_filterViews), `top` (Test_selectMenuStyle/Test_statementsNextView/Test_switchViewTo) — все PASS. Manual TUI-проверка на локальном PG17 пользователем — OK (меню X 7-й пункт «pg_stat_statements JIT compilation», цикл x wal→jit→timings).
**Deviations:** golangci-lint/gosec не установлены локально — полный lint-гейт делегирован CI. Полный `make test` локально не зелёный из-за пре-существующих live-DB тестов (`Test_app_record`, `Test_doReload`, exec-подтесты PG14-18), которые падают/скипаются без PG-кластера и идентично воспроизводятся на develop — гейтятся CI-матрицей PG14-18 на push. Не регрессия (record.go/reload.go фичей не тронуты).
**Tech debt:** Нет (в рамках задачи). Для постмортема: code-research §5 не выявил тест-покрытие TUI-слоя (Test_selectMenuStyle/Test_statementsNextView/Test_switchViewTo) — из-за чего task-03 ошибочно заявил «нет тест-слоя»; пойман dev-test-reviewer (round 1 failed → round 2 passed).

**Reviews:** Нет — QA/верификационная задача (reviewers: []).

**Verification:**
- `make build` → ok; `go vet ./...` → clean; `gofmt -l` → clean
- targeted feature tests (query/view/record/top) → все PASS
- full lint + PG14-18 exec matrix → CI (on push)
- Manual TUI (PG17) → OK (user-confirmed)

---
