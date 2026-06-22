---
status: done                       # planned -> in_progress -> done
depends_on: []                     # ID задач-зависимостей (строки: ["01", "02"])
wave: 3                            # волна параллельного выполнения
skills: [code-writing]             # МАССИВ скиллов для загрузки
verify: bash — `go test ./internal/stat/... -run Diff`   # инструмент верификации
reviewers: [dev-code-reviewer, dev-test-reviewer]  # явно указать. Пусто = fallback на defaults
teammate_name:                     # имя агента-исполнителя (опционально; если не задано — генерируется по описанию задачи)
---

# Task 08: Tech-debt [007] — behavioral zero-cell diff test

## Required Skills

Перед выполнением задачи загрузи:
- `/skill:code-writing` — [skills/code-writing/SKILL.md](~/.claude/skills/code-writing/SKILL.md)

## Description

Закрываем техдолг [007]. Защита от NULL-ячеек в `pg_stat_io` (и `pg_replication_slots`)
реализована в SQL через `coalesce(...,0)` — записанные ячейки в записи всегда `"0"`, никогда
не `""`. Сейчас эта защита проверена **только структурально**: тест в `internal/query`
утверждает, что строка SQL содержит `coalesce` на каждой диффящейся колонке. Поведенческая
половина контракта — что `diff()` корректно вычитает coalesce-нулевые ячейки и НЕ обрывает
выборку (не «гасит» экран) — нигде не проверена. Тест в `internal/query` написать нельзя:
там нельзя импортировать `internal/stat` (циклическая зависимость), поэтому `diff()`
недостижим.

Эта проверка теперь **напрямую на пути фичи**: report при replay прогоняет записанные
coalesce-нулевые ячейки через `countDiff → Compare → diff` (report.go:452 → postgres.go:273
→ postgres.go:303). Если бы `diff()` спотыкался на `"0"`-ячейках или не сопоставлял строки по
UniqueKey, replay новых записываемых экранов (pg_stat_io, replslots) был бы сломан. Этот тест —
поведенческая страховка этого контракта.

Задача — добавить поведенческий тест рядом с `Test_diff` (postgres_test.go:283), который
строит синтетический `stat.PGresult` с нулевыми (coalesce-`"0"`) ячейками в диапазоне диффа,
прогоняет его через `diff()` (и/или публичную обёртку `Compare`) и проверяет: (1) чистые
`"0"`-дельты без ошибок, (2) строки сопоставляются между сэмплами по синтетическому UniqueKey
в стиле `io_key` (колонка 0).

## What to do

- Добавить новый тест в `internal/stat/postgres_test.go` рядом с `Test_diff`
  (postgres_test.go:283) и `Test_diff_pg18_wal_stats_age` (postgres_test.go:341). Имя теста
  должно матчиться маской `-run Diff` (verify-команда фильтрует по `Diff`) — например
  `Test_diff_zero_filled_cells`.
- Построить два `stat.PGresult` (prev и curr), имитирующих записанные coalesce-нулевые
  кумулятивные сэмплы pg_stat_io/replslots: несколько строк, где диффящиеся ячейки в обоих
  сэмплах равны `"0"` (значение, которое recorder кладёт для NULL-после-coalesce). Колонка 0 —
  синтетический UniqueKey в стиле `io_key` (стабильный идентификатор строки между сэмплами).
- Прогнать через `diff(curr, prev, itv, interval, ukey=0)` (как в существующих тестах,
  postgres_test.go:317/334) — указать `interval`, накрывающий нулевые колонки, и `itv=1`.
- Проверить, что ошибки нет и дельта для нулевых ячеек — чистый `"0"` (а не `""`, не паника,
  не обрыв всей выборки).
- Проверить сопоставление строк по UniqueKey: строки prev/curr с одинаковым ключом колонки 0
  сходятся в одну дифф-строку; ненулевая колонка вне `interval` (текстовая, как `io_key`)
  копируется as-is.
- Добавить кейс/ассерт, фиксирующий смысл [007]: смесь нулевой диффящейся ячейки и нормального
  кумулятивного счётчика в одной строке — нулевая даёт `"0"`, счётчик даёт корректную дельту.
  Это и есть «coalesce-ячейка не гасит экран».
- Запустить `go test ./internal/stat/... -run Diff` — убедиться, что новый тест и существующие
  `Test_diff*` зелёные.

## TDD Anchor

Тест пишем первым, потом проверяем, что он проходит на уже существующей (корректной) реализации
`diff()` — это закрывающий-долг тест, а не TDD под новый код. Сначала запускаем падающий
вариант с `""` (чтобы убедиться, что наивная NULL-ячейка действительно обрывала бы дифф —
демонстрация причины долга), затем `"0"`-вариант, который должен проходить.

- `internal/stat/postgres_test.go::Test_diff_zero_filled_cells` — `diff()` с диффящимися
  ячейками `"0"` (coalesce-значение) возвращает чистые `"0"`-дельты без ошибки.
- `internal/stat/postgres_test.go::Test_diff_zero_filled_cells` (подкейс UniqueKey) — строки
  сопоставляются между prev/curr по `io_key`-стиль колонке 0; смешанная строка (нулевая
  ячейка + ненулевой счётчик) диффится корректно.

## Acceptance Criteria

- [ ] Новый поведенческий тест добавлен в `internal/stat/postgres_test.go` рядом с `Test_diff`.
- [ ] Тест прогоняет coalesce-нулевые (`"0"`) ячейки через `diff()` и утверждает чистые
      `"0"`-дельты без ошибки и без обрыва выборки.
- [ ] Тест утверждает сопоставление строк между сэмплами по синтетическому `io_key`-стиль
      UniqueKey (колонка 0).
- [ ] Тест покрывает смешанную строку (нулевая диффящаяся ячейка + нормальный кумулятивный
      счётчик): нулевая → `"0"`, счётчик → корректная дельта.
- [ ] Имя теста матчится `-run Diff`; `go test ./internal/stat/... -run Diff` зелёный, включая
      существующие `Test_diff` и `Test_diff_pg18_wal_stats_age`.
- [ ] Никаких изменений в продакшн-коде `postgres.go` — только тест.

## Context Files

**Feature artifacts:**
- [008-feat-record-report-0-11-views.md](008-feat-record-report-0-11-views.md) — user-spec
- [008-feat-record-report-0-11-views-tech-spec.md](008-feat-record-report-0-11-views-tech-spec.md) — tech-spec (Task 8, §Testing Strategy [007])
- [008-feat-record-report-0-11-views-decisions.md](008-feat-record-report-0-11-views-decisions.md) — decisions log
- [008-feat-record-report-0-11-views-code-research.md](008-feat-record-report-0-11-views-code-research.md) — §10 [007], §11 п.3

**Project knowledge:**
- [overview.md](../../../.claude/skills/project-knowledge/overview.md) — features, supported stats
- [architecture.md](../../../.claude/skills/project-knowledge/architecture.md) — package layout, data flow
- [patterns.md](../../../.claude/skills/project-knowledge/patterns.md) — testing conventions

**Tech-debt:**
- [docs/tech-debt.md](../../tech-debt.md) — запись [007]

**Code files:**
- [internal/stat/postgres_test.go](../../../internal/stat/postgres_test.go) — добавить новый тест рядом с `Test_diff` (283)
- [internal/stat/postgres.go](../../../internal/stat/postgres.go) — функция под тестом `diff()` (303-358), обёртка `Compare` (273)

## Verification Steps

- Запустить `go test ./internal/stat/... -run Diff` — новый тест и все `Test_diff*` зелёные.
- Убедиться, что вариант с пустой ячейкой `""` действительно даёт ошибку (демонстрация причины
  долга), а вариант с `"0"` — чистую `"0"`-дельту без ошибки.
- `make test` / `make lint` остаются зелёными (изменён только тест-файл).

## Details

**Files:**
- `internal/stat/postgres_test.go` — добавить функцию-тест рядом с `Test_diff` (postgres_test.go:283).
  Существующие тесты уже строят `PGresult{}`-литералы и вызывают неэкспортируемый
  `diff(curr, prev, itv, interval, ukey)` напрямую (postgres_test.go:317, 334) — повторить
  этот стиль. Файл в пакете `stat`, так что `diff` достижим напрямую.

**Функция под тестом:**
- `diff()` — postgres.go:303-358. Уязвимая строка — postgres.go:336:
  `diffPair(curr.Values[i][l].String, prev.Values[j][l].String, itv)`. Пустая in-interval
  ячейка `""` доходит до `diffPair → parsePairInt → ParseInt("")` (postgres.go:477-487) →
  ошибка → `return diff, err` (postgres.go:337-339) → обрывает весь сэмпл. Существующий
  `Test_diff` уже проверяет error-путь на `"invalid"` (postgres_test.go:327-335).
- coalesce-значение `"0"` парсится нормально → `diffPair` возвращает `"0"` (целочисленная
  ветка, postgres.go:455-459: `(0-0)/1 = 0`). Это и есть контракт, который нужно зафиксировать
  поведенчески.
- UniqueKey-сопоставление: `diff` матчит строки по `cv[ukey].String != pv[ukey].String`
  (postgres.go:322). Поэтому колонка 0 в обоих сэмплах должна нести стабильный `io_key`-стиль
  идентификатор (например `"a1"`, `"b2"` — md5/строковый ключ в реале; для теста достаточно
  любых стабильных строк). UniqueKey-колонка обычно вне `interval`, поэтому копируется as-is
  (postgres.go:332).
- `Compare` — публичная обёртка над `calculateDelta` (postgres.go:273), которую report зовёт
  через `countDiff`. Достаточно тестировать `diff()` напрямую (как существующие тесты); при
  желании можно добавить параллельный ассерт через `Compare(...)` для покрытия пути report,
  но это опционально — не раздувать.

**Dependencies:** нет зависимостей от других задач (`depends_on: []`). Wave 3 — после Wave 1,
но файл `internal/stat/postgres_test.go` не пересекается с задачами Wave 1/2; конфликтов нет.
Пакеты: только `testify` (`assert`) и `database/sql` (`sql.NullString`), уже импортированы в
тест-файле.

**Edge cases:**
- Все диффящиеся ячейки строки = `"0"` → дельта `"0"`, не `""`, не ошибка.
- Смешанная строка: одна диффящаяся ячейка `"0"`, другая — нормальный кумулятивный счётчик
  (например prev `"100"`, curr `"150"`) → `"0"` и `"50"` соответственно.
- Несколько строк с разными `io_key`-ключами → каждая сопоставляется со своей парой; порядок
  колонки-ключа вне `interval`.
- (Демонстрационный, опционально внутри теста) пустая ячейка `""` в interval → `diff` возвращает
  ошибку — фиксирует, ПОЧЕМУ нужен coalesce. Не дублировать существующий `"invalid"`-кейс
  один-в-один: акцент на пустой строке как на NULL-после-сериализации.

**Implementation hints:**
- Маска `-run Diff` ловит `Test_diff`, `Test_diff_pg18_wal_stats_age` и новый тест — имя
  должно содержать `Diff`/`diff`. Не переименовывать существующие.
- `itv=1` упрощает дельту (деление на 1), как в `Test_diff`.
- `interval` подобрать так, чтобы UniqueKey (col 0) и любые текстовые колонки были вне
  диапазона, а нулевые/счётчиковые — внутри. Сверяться с реальными `DiffIntvl`/`UniqueKey`
  для stat_io (DiffIntvl `[4,14]`, UniqueKey 0) из code-research §2, но в тесте можно
  использовать упрощённую раскладку — главное, чтобы нулевые ячейки попадали в `interval`.
- НЕ менять `postgres.go` — долг закрывается тестом; реализация уже корректна (coalesce в SQL +
  целочисленный дифф `"0"`).

## Reviewers

- **dev-code-reviewer** → `008-feat-record-report-0-11-views-task-08-dev-code-reviewer-review.json`
- **dev-test-reviewer** → `008-feat-record-report-0-11-views-task-08-dev-test-reviewer-review.json`

## Post-completion

- [ ] Записать краткий отчёт в [008-feat-record-report-0-11-views-decisions.md](008-feat-record-report-0-11-views-decisions.md) (Summary: 1-3 предложения, ревью со ссылками на JSON, без таблиц файндингов и дампов)
- [ ] Отметить в записи техдолга [007] (`docs/tech-debt.md`), что поведенческая половина контракта закрыта (или оставить пометку для /done-финализации)
- [ ] Если отклонились от спека — описать отклонение и причину
