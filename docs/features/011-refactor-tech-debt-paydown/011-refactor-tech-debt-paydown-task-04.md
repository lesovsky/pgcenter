---
status: planned                    # planned -> in_progress -> done
depends_on: ["01", "02", "03"]     # ID задач-зависимостей (строки)
wave: 3                            # волна параллельного выполнения
skills: [pre-deploy-qa]            # МАССИВ скиллов для загрузки
verify: bash — make test && make lint && make vuln
reviewers: []                      # QA-задача: ревьюеров нет
teammate_name:                     # имя агента-исполнителя (опционально)
---

# Task 04: Pre-deploy QA

## Required Skills

Перед выполнением задачи загрузи:
- `/skill:pre-deploy-qa` — [skills/pre-deploy-qa/SKILL.md](~/.claude/skills/pre-deploy-qa/SKILL.md)

## Description

Финальная приёмочная проверка фичи 011 (tech-debt paydown) перед выкаткой. Три предыдущие задачи закрыли
независимые пункты техдолга: [009] defensive allocation cap на tar-записи, [011] консолидация
rate-хелпера, [012] fixed-width verbose Size-поля. Каждая закоммичена отдельно.

Эта задача — НЕ кодовая: ничего не реализуется и не правится. Дело QA — прогнать полный gate
(`make test` / `make lint` / `make vuln`), пройтись по каждому критерию приёмки из user-spec и tech-spec
по всем трём пунктам, и подтвердить ОБЯЗАТЕЛЬНУЮ ручную визуальную проверку [012] (косметический фикс
верифицируется только глазами). Результат проверки — и есть deliverable.

Особое внимание:
- [009] — over-limit запись отклоняется во всех трёх ветках `report.readTar` (`meta.*`, `sysinfo.*`,
  stat) без аллокации, а легитимный архив по-прежнему воспроизводится.
- [011] — вывод verbose disk/net строк байт-в-байт совпадает с дорефакторным (boundary-таблица +
  существующие goldens зелёные).
- [012] — пять verbose Size-полей фиксированной ширины; колонки и хвостовые подписи стабильны между
  сэмплами и между состояниями value/`n/a`.
- gosec не должен давать G115 (тип `MaxResultFileSize` — `int64`, без `int()`-конверсий в guard).

## What to do

1. Собрать бинарь (`make build`) — убедиться, что сборка проходит.
2. Прогнать полный gate последовательно: `make test` (race + coverage), `make lint` (golangci-lint v2 +
   gosec), `make vuln` (govulncheck). Все три — зелёные.
3. По gosec явно подтвердить: предупреждения **G115 отсутствуют** (это была одна из целей [009]).
4. Пройтись по каждому критерию приёмки из user-spec и tech-spec по трём пунктам ([009]/[011]/[012]) и
   зафиксировать pass/fail с привязкой к конкретным тестам/наблюдениям.
5. Проверить отсутствие регрессий в существующих тестах `internal/pretty`, `top`, `internal/stat`,
   `report`.
6. Подтвердить независимую покоммитную выкатку трёх задач (три отдельных коммита).
7. Запросить и зафиксировать ОБЯЗАТЕЛЬНУЮ ручную проверку [012]: пользователь нажимает `v` в
   `pgcenter top` на инстансе с меняющимися размерами/лагом и подтверждает, что Size-колонки и подписи
   не «дышат» (не сдвигаются по горизонтали) между сэмплами.
8. Свести итог: GO / NO-GO, со списком проверенных критериев и статусом ручной проверки.

## Acceptance Criteria

- [ ] `make test` зелёный (race + coverage), включая новые table/golden/tar-тесты.
- [ ] `make lint` чистый — golangci-lint v2 + gosec без замечаний; **нет G115**.
- [ ] `make vuln` чистый (govulncheck).
- [ ] [009]: `stat.MaxResultFileSize int64 = 256 MiB`; `NewPGresultFile` отклоняет `bufsz < 0` и
      `bufsz > limit` без аллокации; cap применён во всех трёх ветках `report.readTar`; over-limit запись
      отклоняется на каждой ветке, легитимный архив воспроизводится.
- [ ] [011]: `rateField` удалён; `RateUnit` + `RateUnitPrefixed` делегируют общему core; verbose
      disk/net вывод байт-в-байт совпадает с дорефакторным (boundary-таблица + goldens зелёные).
- [ ] [012]: пять verbose Size-полей и их `n/a`-фоллбэки фиксированной ширины; позиции колонок/подписей
      стабильны между сэмплами и между value/`n/a`; цифры/единицы идентичны `pretty.Size`.
- [ ] Нет регрессий в `internal/pretty`, `top`, `internal/stat`, `report`.
- [ ] Три задачи закоммичены независимо.
- [ ] ОБЯЗАТЕЛЬНАЯ ручная проверка [012] (`v` в `pgcenter top`) подтверждена пользователем.

## Context Files

**Feature artifacts:**
- [011-refactor-tech-debt-paydown.md](docs/features/011-refactor-tech-debt-paydown/011-refactor-tech-debt-paydown.md) — user-spec (раздел «Критерии приёмки» и «Как проверить»)
- [011-refactor-tech-debt-paydown-tech-spec.md](docs/features/011-refactor-tech-debt-paydown/011-refactor-tech-debt-paydown-tech-spec.md) — tech-spec (Acceptance Criteria, Agent Verification Plan)
- [011-refactor-tech-debt-paydown-decisions.md](docs/features/011-refactor-tech-debt-paydown/011-refactor-tech-debt-paydown-decisions.md) — decisions log (отчёты задач 01–03)

**Project knowledge:**
- [overview.md](.claude/skills/project-knowledge/overview.md) — команды (`top`/`record`/`report`), supported stats
- [architecture.md](.claude/skills/project-knowledge/architecture.md) — package layout, verbose top-panel mode (010)
- [patterns.md](.claude/skills/project-knowledge/patterns.md) — Manual Testing / QA Phase, Reserved-width `n/a`, Linting

## Verification Steps

- `make build` → бинарь `./bin/pgcenter` собирается без ошибок.
- `make test` → все пакеты зелёные; новые тесты [009]/[011]/[012] присутствуют и проходят.
- `make lint` → golangci-lint v2 + gosec чисто; вручную просмотреть вывод на предмет G115 (должен
  отсутствовать).
- `make vuln` → govulncheck без находок.
- Сверить каждый критерий приёмки с конкретным тестом/наблюдением; зафиксировать pass/fail.
- Получить от пользователя подтверждение ручной verbose-проверки [012].
- Итог: GO только если все автоматические gate зелёные И ручная проверка [012] подтверждена.

## Details

**Files:** проверочная задача — файлы кода не модифицируются. Источники истины — user-spec, tech-spec,
decisions log и вывод `make test/lint/vuln`.

**Dependencies:** зависит от задач 01 ([009]), 02 ([011]), 03 ([012]) — все три должны быть завершены и
закоммичены. Запускать после слияния всех трёх.

**Edge cases для проверки:**
- [009] границы: `hdr.Size == limit` разрешён, `== limit+1` отклонён, `== 0` разрешён (пустая запись),
  `< 0` отклонён; over-limit покрыт на всех трёх ветках (`meta.*`, `sysinfo.*`, stat).
- [011] граница промоушена: `maxFit` vs `maxFit+1`; сохранён пробел и префикс `r`/`w` (формат вида
  `1135 rMB/s`).
- [012] значение шире резерва (TB-диапазон, строка ≥ 7 символов) расширяет поле детерминированно, не
  ломая верстку; `n/a` занимает зарезервированную ширину 8.
- Поле `wal size` (`top/stat.go:601`) намеренно ВНЕ scope — это первое поле строки, его подпись не
  сдвигается; не считать его отсутствие фиксированной ширины дефектом.

**Implementation hints:**
- gosec встроен в `make lint`; G115 — целевое предупреждение, проверить его отсутствие явно.
- Точечный прогон, если нужно сузить: `go test ./internal/stat/... ./report/...` ([009]),
  `go test ./internal/pretty/... ./top/...` ([011]/[012]).
- Ручная проверка [012] — единственная часть, требующая живого `pgcenter top`; всё остальное
  покрывается unit/table/golden-тестами без живого PostgreSQL.
- Следовать конвенциям шаблона pre-deploy-qa (см. SKILL.md): итоговый verdict GO / NO-GO с явным
  статусом ручной проверки.

## Post-completion

- [ ] Записать краткий отчёт в [011-refactor-tech-debt-paydown-decisions.md](docs/features/011-refactor-tech-debt-paydown/011-refactor-tech-debt-paydown-decisions.md) (Summary: результат gate + статус ручной проверки [012] + GO/NO-GO, 1-3 предложения)
- [ ] Если найдены отклонения от спека — описать отклонение и причину
- [ ] Обновить user-spec/tech-spec если что-то изменилось
