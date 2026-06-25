---
status: done                    # planned -> in_progress -> done
depends_on: ["02"]                 # ID задач-зависимостей (строки: ["01", "02"])
wave: 2                            # волна параллельного выполнения
skills: [code-writing]             # МАССИВ скиллов для загрузки
verify: bash + user               # инструмент верификации (опционально: curl, bash, user)
reviewers: [dev-code-reviewer, dev-test-reviewer]
teammate_name:                     # имя агента-исполнителя (опционально; если не задано — генерируется по описанию задачи)
---

# Task 03: [012] Fixed-width verbose Size fields

## Required Skills

Перед выполнением задачи загрузи:
- `/skill:code-writing` — [skills/code-writing/SKILL.md](~/.claude/skills/code-writing/SKILL.md)

## Description

Tech-debt item **[012]**: пять Size-полей в verbose-режиме pgstat-панели «дышат» — при смене значения
между тиками их ширина меняется, и следующая за полем подпись (label) визуально сдвигается. Это видно,
когда DBA нажимает `v` в `pgcenter top`: колонки и подписи прыгают от тика к тику.

Причина: поля рендерятся через `pretty.Size(...)` (переменная ширина: `"0"`, `"512B"`, `"1.0M"`,
`"1023.9G"`), а их `n/a`-fallback — через голый `naLiteral` (`"n/a"`, тоже не зарезервирован под
ширину значения). Поэтому позиция trailing-label нестабильна и между двумя значениями, и между
значением и `n/a`.

Решение: добавить экспортируемый fixed-width вариант `pretty.SizeWidth(v, width)` (правое выравнивание
`Size(v)` в `width` колонок, по модели `ReserveWidth` — никогда не обрезает, при переполнении
детерминированно расширяется). Применить его с единой шириной `sizeFieldWidth = 8` к пяти verbose
Size-полям, а их `n/a`-fallback заменить на `naReserve(sizeFieldWidth)`. Цифры и единицы измерения
остаются идентичны `pretty.Size` — добавляется только ведущий padding.

Поле **wal size** (`top/stat.go:601`, `pretty.Size(o.WalSize)`) **исключено** из scope (Decision 5):
оно первое в своей строке и не толкает trailing-label, поэтому визуально не «дышит».

Задача в Wave 2, потому что делит `internal/pretty/pretty.go` И `top/stat.go` с задачей 02 [011];
запускается **после** мёрджа задачи 02.

## What to do

1. В `internal/pretty/pretty.go` добавить экспортируемую функцию `SizeWidth(v float64, width int) string`,
   которая правым выравниванием помещает `Size(v)` в `width` колонок (`fmt.Sprintf("%*s", width, Size(v))`).
   Зеркалит `ReserveWidth`: никогда не обрезает, при переполнении детерминированно расширяется. Цифры и
   единицы, выдаваемые `Size`, должны остаться неизменными. Снабдить doc-comment в стиле соседних
   функций (`ReserveWidth`/`RateUnit`).
2. В `top/stat.go` рядом с `cacheHitWidth` (строка 319) добавить именованную константу
   `sizeFieldWidth = 8` с поясняющим комментарием (самая широкая реалистичная Size-строка — 7 символов
   `"1023.9M"`/`"1023.9G"`/`"1023.9T"`; резерв 8 даёт колонку запаса и чистое правое выравнивание).
3. В `renderPgstatVerbose` (`top/stat.go`) обернуть пять Size-значений в `pretty.SizeWidth(v, sizeFieldWidth)`:
   - строка 561 — `size` (`TotalSize`)
   - строка 563 — `growth` (`GrowthPerSec`)
   - строка 590 — `lag` (`LagBytes`)
   - строка 594 — `retain` (`RetainedBytes`)
   - строка 598 — `backlog` (`ArchivingBacklog`)
4. Заменить их `n/a`-fallback c голого `naLiteral` на `naReserve(sizeFieldWidth)`:
   - строка 559 — инициализация `size, growth := naLiteral, naLiteral` (оба → `naReserve(sizeFieldWidth)`)
   - строка 588 — `lag := naLiteral`
   - строка 592 — `retain := naLiteral`
   - строка 596 — `backlog := naLiteral`
   Так позиция trailing-label идентична и в состоянии «значение», и в состоянии `n/a`.
5. **Не трогать** wal size (`top/stat.go:601`, `pretty.Size(o.WalSize)`) — остаётся голым `Size`
   (первое поле строки, label не толкает — Decision 5).
6. Написать/обновить тесты (см. TDD Anchor) до изменения кода.
7. Прогнать `make test`, `make lint`, `make vuln` — всё зелёное.

## TDD Anchor

Тесты пишем ДО реализации. Сначала падают (n/a-vs-value offset-ассерт для Size-полей падает сегодня —
это и есть регрессия, которую фиксируем), затем код, затем проходят.

- `internal/pretty/pretty_test.go::TestSizeWidth` — новый table-тест на `pretty.SizeWidth`:
  значение уже резерва выравнивается правым padding-ом до точной ширины (`"    1.0M"` при
  width 8); значение шире резерва расширяется без обрезки (`%*s` semantics); цифры/единицы идентичны
  `pretty.Size(v)` для тех же входов (нулевое значение `"0"`, `"512B"`, `"1023.9G"`).
- `top/stat_test.go::Test_renderPgstat_verboseNAWidthStatic` — расширить (сейчас ~406-443): для пяти
  Size-полей (databases-row: `size` перед `per`, `growth` перед `growth/s`; replication-row: `lag`
  перед `lag`, `retain` перед `slots/retain`, `backlog` перед `archiving backlog`) добавить две группы
  ассертов:
  (а) trailing-label стоит на ИДЕНТИЧНОМ байтовом offset между двумя сэмплами значений разной ширины
  (например `1.0M` vs `1023.9G`);
  (б) trailing-label стоит на идентичном offset между состоянием «значение» и состоянием `n/a`
  (ассерт (б) падает на текущем коде — голый `naLiteral` уже резерва значения).
- `top/stat_test.go::Test_renderPgstat_verboseAvailable` — обновить существующие goldens (строки ~385
  databases-row и ~391 replication-row) на padded-форму: значения/единицы идентичны, добавлен только
  ведущий padding (`"1.0T"` → `"    1.0T"` при width 8 и т.п.). `assert.NotContains(buf, "n/a")`
  должен остаться зелёным.

## Acceptance Criteria

- [ ] `pretty.SizeWidth(v, width)` добавлена, правым выравниванием помещает `Size(v)` в `width`, никогда
      не обрезает, при переполнении расширяется; цифры/единицы идентичны `pretty.Size`.
- [ ] `sizeFieldWidth = 8` — именованная константа в `top/stat.go` рядом с `cacheHitWidth`.
- [ ] Пять verbose Size-полей (TotalSize, GrowthPerSec, LagBytes, RetainedBytes, ArchivingBacklog)
      рендерятся через `pretty.SizeWidth(v, sizeFieldWidth)`.
- [ ] Их `n/a`-fallback рендерится через `naReserve(sizeFieldWidth)` (не голый `naLiteral`).
- [ ] wal size (`top/stat.go:601`) НЕ изменён — остаётся голым `pretty.Size` (Decision 5).
- [ ] Позиция колонок/labels стабильна между сэмплами значений разной ширины И между значением и `n/a`
      (offset-ассерты зелёные).
- [ ] `make test`, `make lint` (golangci-lint v2 + gosec, без G115), `make vuln` — зелёные.
- [ ] Нет регрессий в существующих `internal/pretty`, `top` тестах.
- [ ] Ручная проверка: пользователь нажал `v` в `pgcenter top` и подтвердил, что Size-колонки/labels не
      сдвигаются между тиками (MANDATORY).
- [ ] Задача закоммичена независимо.

## Context Files

**Feature artifacts:**
- [011-refactor-tech-debt-paydown.md](011-refactor-tech-debt-paydown.md) — user-spec
- [011-refactor-tech-debt-paydown-tech-spec.md](011-refactor-tech-debt-paydown-tech-spec.md) — tech-spec (Decision 3, Decision 5)
- [011-refactor-tech-debt-paydown-decisions.md](011-refactor-tech-debt-paydown-decisions.md) — decisions log
- [011-refactor-tech-debt-paydown-code-research.md](011-refactor-tech-debt-paydown-code-research.md) — code research (sections [012])

**Project knowledge:**
- [overview.md](.claude/skills/project-knowledge/overview.md) — features, supported stats, target audience
- [architecture.md](.claude/skills/project-knowledge/architecture.md) — package layout, data flow, PG version handling
- [patterns.md](.claude/skills/project-knowledge/patterns.md) — code patterns, testing conventions, verbose-panel formatting helpers

**Code files:**
- [internal/pretty/pretty.go](internal/pretty/pretty.go) — добавить экспортируемую `SizeWidth`
- [top/stat.go](top/stat.go) — добавить `sizeFieldWidth`, обернуть 5 Size-полей + заменить их n/a-fallback
- [internal/pretty/pretty_test.go](internal/pretty/pretty_test.go) — новый table-тест `TestSizeWidth`
- [top/stat_test.go](top/stat_test.go) — расширить `Test_renderPgstat_verboseNAWidthStatic`, обновить goldens в `Test_renderPgstat_verboseAvailable`

## Verification Steps

- Запустить `go test ./internal/pretty/... ./top/...` — все тесты, включая новый `TestSizeWidth` и
  расширенный `Test_renderPgstat_verboseNAWidthStatic`, проходят; обновлённые goldens зелёные.
- Запустить `make test && make lint && make vuln` — всё зелёное (race+coverage; golangci-lint v2 + gosec
  без G115; govulncheck).
- MANDATORY ручная проверка: собрать `make build`, запустить `pgcenter top`, нажать `v`, убедиться, что
  в строках `databases:` и `replication:` Size-значения и следующие за ними подписи не сдвигаются между
  тиками. Пользователь подтверждает.

## Details

**Files:**
- `internal/pretty/pretty.go` — сейчас содержит `Size` (строки 8-24), `ReserveWidth` (45-47),
  `RateUnit` (59-78). Добавить `SizeWidth(v float64, width int) string` = `fmt.Sprintf("%*s", width, Size(v))`
  рядом с `ReserveWidth`/`Size`. Важно: задача 02 [011] уже изменила этот файл (добавила `rateUnitParts`/
  `RateUnitPrefixed`) — работать поверх мёрджнутой версии задачи 02.
- `top/stat.go` — `cacheHitWidth = 7` на строке 319; добавить `sizeFieldWidth = 8` рядом. В
  `renderPgstatVerbose` (546-625) обернуть 5 Size-значений (561/563/590/594/598) и заменить 4 места
  инициализации n/a (559/588/592/596). Помнить: задача 02 [011] уже переправила вызовы `rateField` →
  `pretty.RateUnitPrefixed` в `renderSysstatVerbose` этого же файла — абсолютные номера строк могли
  немного сдвинуться; ориентироваться на имена полей (`size`/`growth`/`lag`/`retain`/`backlog`), а не на
  номера.
- `internal/pretty/pretty_test.go` — добавить `TestSizeWidth` в стиле соседних table/property тестов
  (`TestSize`, `TestReserveWidth`).
- `top/stat_test.go` — `Test_renderPgstat_verboseAvailable` (356-398) goldens на строках 385/391;
  `Test_renderPgstat_verboseNAWidthStatic` (406-443) — образцовый byte-offset-паттерн через
  `strings.Index(row, label)`, расширить его на 5 Size-полей.

**Dependencies:**
- Зависит от задачи 02 [011] (общие файлы `internal/pretty/pretty.go` и `top/stat.go`). Запускать только
  после мёрджа задачи 02.
- Внешних пакетов нет. `pretty` использует только `fmt` и stdlib `math`. Импорты `pretty` в `top/stat.go`
  уже на месте (`pretty.Size`, `pretty.ReserveWidth`).

**Edge cases:**
- Значение шире резерва (`width` 8): `%*s` НЕ обрезает — поле детерминированно расширяется (модель
  `ReserveWidth`). Реалистичный максимум Size-строки — 7 символов, поэтому при width 8 переполнение на
  реальных данных не достигается, но семантика «никогда не обрезать» обязана быть покрыта тестом.
- Нулевое значение: `Size(0)` = `"0"`, `SizeWidth(0, 8)` = `"       0"` (7 пробелов + `0`).
- `n/a`-состояние: `naReserve(sizeFieldWidth)` правым выравниванием помещает `"n/a"` в 8 колонок
  (`"     n/a"`), floor по `len(naLiteral)`=3 не активируется (8 > 3) — позиция label совпадает с
  состоянием значения.
- wal size исключён — НЕ оборачивать (Decision 5); это первое поле replication-строки.
- `databases`-строка: `growth` уходит в n/a и при `!TotalSizeValid` (внешняя ветка — `growth` остаётся
  `naReserve`), и при `TotalSizeValid && !hp` (`growth` остаётся `naReserve`, `size` — значение). Обе
  ветки должны держать одинаковую ширину growth.

**Implementation hints:**
- `SizeWidth` — тонкая обёртка над `Size`, ровно как `ReserveWidth` над `%*d`; не дублировать switch из
  `Size`.
- lint: revive `redefines-builtin-id` (severity error) — не использовать имена параметров, шейдящие
  builtins (см. недавний коммит 8f1d588). Безопасные имена: `v float64, width int`.
- Перед правкой зафиксировать текущие goldens `Test_renderPgstat_verboseAvailable`, чтобы убедиться, что
  меняется ТОЛЬКО ведущий padding (цифры/единицы байт-в-байт те же).
- В `Test_renderPgstat_verboseNAWidthStatic` уже есть готовый паттерн offset-сравнения через
  `strings.Index` — переиспользовать его, добавив сэмпл значения второй ширины и сэмпл n/a для 5 полей.

## Reviewers

- **dev-code-reviewer** → `011-refactor-tech-debt-paydown-task-03-dev-code-reviewer-review.json`
- **dev-test-reviewer** → `011-refactor-tech-debt-paydown-task-03-dev-test-reviewer-review.json`

## Post-completion

- [ ] Записать краткий отчёт в [011-refactor-tech-debt-paydown-decisions.md](011-refactor-tech-debt-paydown-decisions.md) (Summary: 1-3 предложения, ревью со ссылками на JSON, без таблиц файндингов и дампов)
- [ ] Если отклонились от спека — описать отклонение и причину
- [ ] Обновить user-spec/tech-spec если что-то изменилось
