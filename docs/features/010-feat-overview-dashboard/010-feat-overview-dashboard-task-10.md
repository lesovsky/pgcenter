---
status: planned                    # planned -> in_progress -> done
depends_on: ["01", "02", "03", "04", "05", "06", "07", "08", "09"]
wave: 6
skills: [pre-deploy-qa]
verify: bash — make build && make test && make lint
reviewers: []
teammate_name:
---

# Task 10: Pre-deploy QA

## Required Skills

Перед выполнением задачи загрузи:
- `/skill:pre-deploy-qa` — [skills/pre-deploy-qa/SKILL.md](~/.claude/skills/pre-deploy-qa/SKILL.md)

## Description

Финальная волна фичи 010 (verbose-режим верхних панелей `sysstat`/`pgstat`). Все девять реализующих задач (формат-хелперы, toggle-плэмбинг, io.Writer-рефактор, GUC-чтения, агрегатные запросы, геометрия `layout()`, all-three system collection, row composers, tiering/guard) завершены и слиты. Задача — провести приёмочное тестирование на свежем билде: собрать бинарь с нуля, прогнать весь набор тестов (race + coverage), линтер и анализ уязвимостей, после чего пройтись по всем критериям приёмки из user-spec и tech-spec и убедиться, что фича делает то, что заявлено, и ничего не сломала.

Это QA-задача, а не написание кода: верификация — сам приёмочный прогон. Часть критериев (поведение TUI: разворот панелей, консистентность с полными панелями, height-guard, первый тик, remote/standby/`archive_mode=off`) проверяется вручную через запущенный `pgcenter top` против живого PostgreSQL, потому что в проекте нет TUI E2E-харнесса. Автоматическая часть (`make build`/`make test`/`make lint`/`govulncheck`) — обязательный гейт.

Результат — отчёт: для каждого критерия приёмки статус (pass / fail / blocked) с доказательством (вывод команды, наблюдение в TUI, либо причина невозможности проверки). Любой fail документируется как находка для исправляющей итерации.

## What to do

1. Свежий билд: выполнить `make build` с нуля (`./bin/pgcenter` собирается без ошибок).
2. Тесты: выполнить `make test` — все юнит- и интеграционные тесты зелёные (race detector + coverage). Интеграционные тесты против живого PG используют `t.Skipf`-guard; если кластера нет — зафиксировать, какие тесты пропущены, как ограничение проверки.
3. Линт: выполнить `make lint` (golangci-lint + gosec) — без замечаний.
4. Уязвимости: выполнить `make vuln` (govulncheck) — чисто.
5. Пройтись по критериям приёмки user-spec ("Критерии приёмки") и tech-spec ("Acceptance Criteria"). Для каждого зафиксировать pass/fail/blocked с доказательством. Покрыть в частности:
   - `v` переключает verbose обеих панелей; повторное `v` → compact; флаг персистентен между экранами (не сбрасывается при `viewSwitchHandler`).
   - В verbose `sysstat` показывает 3 доп. строки (iostat/nicstat/filesyst), `pgstat` — 5 (workload/databases/workers/replication/bgwr-ckpt); раскладка и суффиксы статичны, меняются только значения.
   - Verbose-агрегаты консистентны с полными панелями `B`/`N`/`F` и экранами `d`/`r`/`b` — одно устройство/число не расходится; verbose показывает целые с округлением вверх.
   - `iostat`/`nicstat` — устройство с максимальным `%util`; `filesyst` — ФС каталога данных.
   - Деградация в `n/a`: недоступный сигнал → литерал `n/a` (никогда `0`/пусто); падение одного источника не гасит остальные verbose-строки (per-source независимость).
   - Первый тик: дельты `n/a` + подсказка `collecting...` в cmdline, гаснет после первого рефреша.
   - Граничные пути: remote без PL/Perl-схемы (системные строки `n/a`), standby/без репликации (lag/slots/send-recv пусто/`n/a`), `archive_mode=off` (`archiving backlog` → `n/a`).
   - Height-guard: на низком терминале verbose не разворачивается, в cmdline подсказка; layout не ломается.
   - Compact-вывод байт-идентичен прежнему поведению (рефактор behavior-preserving).
   - Регрессии: нет изменений в количестве view, в keybindings и в существующих сайд-панелях/экранах; все тесты проходят.
6. Собрать отчёт по результатам прогона; найденные расхождения оформить как находки (severity / issue / fix) для исправляющей итерации.

## Acceptance Criteria

- [ ] `make build` собирает `./bin/pgcenter` без ошибок (свежий билд).
- [ ] `make test` зелёный (race + coverage); пропущенные `t.Skipf` интеграционные тесты задокументированы как ограничение.
- [ ] `make lint` (golangci-lint + gosec) без замечаний.
- [ ] `make vuln` (govulncheck) чисто.
- [ ] Все критерии приёмки user-spec ("Критерии приёмки") проверены, статус по каждому зафиксирован.
- [ ] Все технические критерии tech-spec ("Acceptance Criteria") проверены, статус по каждому зафиксирован.
- [ ] Проверены граничные пути: remote без схемы, standby/без репликации, `archive_mode=off`, первый тик, height-guard.
- [ ] Подтверждено отсутствие регрессий: view-count, keybindings, существующие панели/экраны.
- [ ] Compact-вывод подтверждён как байт-идентичный прежнему (writer-тесты + визуально).
- [ ] Итоговый отчёт сформирован; каждый fail оформлен как находка (severity/issue/fix).

## Context Files

**Feature artifacts:**
- [010-feat-overview-dashboard.md](docs/features/010-feat-overview-dashboard/010-feat-overview-dashboard.md) — user-spec (разделы "Критерии приёмки", "Как проверить")
- [010-feat-overview-dashboard-tech-spec.md](docs/features/010-feat-overview-dashboard/010-feat-overview-dashboard-tech-spec.md) — tech-spec (разделы "Acceptance Criteria", "Agent Verification Plan")
- [010-feat-overview-dashboard-decisions.md](docs/features/010-feat-overview-dashboard/010-feat-overview-dashboard-decisions.md) — decisions log (отчёты задач 01–09)

**Project knowledge:**
- [overview.md](.claude/skills/project-knowledge/overview.md) — фичи, поддерживаемые статистики, аудитория
- [architecture.md](.claude/skills/project-knowledge/architecture.md) — компоновка пакетов, поток данных, версии PG
- [patterns.md](.claude/skills/project-knowledge/patterns.md) — паттерны кода и тестирования, версионное ветвление
- [deployment.md](.claude/skills/project-knowledge/deployment.md) — процесс сборки/релиза, make-таргеты

## Verification Steps

- `make build` → бинарь `./bin/pgcenter` собран, exit 0.
- `make test` → весь набор зелёный (race + coverage); зафиксировать список `t.Skipf`-пропусков при отсутствии кластера.
- `make lint` → golangci-lint + gosec без замечаний, exit 0.
- `make vuln` → govulncheck без находок.
- Ручной TUI-прогон `./bin/pgcenter top` на широком терминале: `v` разворачивает обе панели; сверка verbose-строк с полными панелями `B`/`N`/`F` и экранами `d`/`r`/`b` — числа совпадают; переход между экранами (`a`/`t`/…) сохраняет verbose; повторное `v` → compact; первый тик показывает `collecting...`, который гаснет.
- Ручной TUI-прогон на низком терминале: verbose не разворачивается, в cmdline подсказка, раскладка цела.
- Ручной прогон против remote-инстанса без PL/Perl-схемы / standby / `archive_mode=off`: соответствующие строки `n/a`, панель не падает.
- Итог: каждый критерий приёмки помечен pass/fail/blocked с доказательством; список расхождений (если есть) оформлен как находки.

## Details

**Files:** код не правится — это приёмочная проверка результата задач 01–09. Источники истины для критериев: user-spec "Критерии приёмки" (toggle/персистентность; 3+5 строк; консистентность; max-util-устройство; workload/databases per-interval; workers/max; archiving backlog; `n/a`-деградация; динамический суффикс; tiering/guard; первый тик; height-guard) и tech-spec "Acceptance Criteria" (byte-identical compact; консистентность с `B`/`N`/`F`; `n/a` без блокировки остальных строк; height-guard; no view-count/keybinding/panel-регрессий; lint+vuln clean; версионная корректность PG 14–18).

**Build/test commands (из .claude/CLAUDE.md и deployment.md):**
- `make build` — сборка в `./bin/pgcenter`
- `make test` — race detector + coverage
- `make lint` — golangci-lint + gosec
- `make vuln` — govulncheck

**Dependencies:** зависит от задач 01–09 (волны 1–5 завершены и слиты). Реальных кодовых зависимостей нет — задача только запускает и наблюдает.

**Edge cases для проверки:**
- Нет живого PG-кластера → интеграционные тесты `t.Skipf`-пропускаются; это не fail тестового прогона, но зафиксировать как ограничение покрытия, и ручные DB-зависимые критерии помечать blocked при недоступности кластера.
- Remote без PL/Perl-схемы → системные verbose-строки `n/a`.
- Симлинк `data_directory` по сети не резолвится → `filesyst` показывает ФС нерезолвленного пути (документированное ограничение, не баг).
- standby / нет репликации → lag/slots/send-recv пусто/`n/a`; WAL/checkpoint recovery-aware.
- `archive_mode=off` → `archiving backlog` → `n/a`.
- Первый тик без prev → дельты `n/a` + `collecting...`.
- Низкий терминал → verbose не разворачивается, подсказка в cmdline.
- Переполнение разрядов → переключение суффикса (MB/s→GB/s, Mbps→Gbps).

**Implementation hints:**
- Verbose — TUI-only режим, в `record`/`report` не участвует; проверять только в `pgcenter top`.
- Консистентность сверять на одном и том же устройстве (max-`%util`) и в одно и то же время — verbose округляет вверх, полная панель показывает дроби; сравнивать с поправкой на ceil-округление.
- `n/a` должен быть литералом, отличимым от `0` и пустоты — отдельно проверить, что падение одного источника не гасит остальные строки.
- Регрессии view-count/keybindings: verbose реализован как mode-flag без нового view, поэтому `view_test.go`/`Test_filterViews` не должны были измениться — подтвердить отсутствием диффа в этих местах и зелёными тестами.
- При отсутствии кластера для части ручных критериев — корректно помечать blocked с причиной, не выдавать за pass.

## Reviewers

Нет ревьюеров — приёмочная QA-задача, результат самой проверки и есть отчёт.

## Post-completion

- [ ] Записать краткий отчёт в [010-feat-overview-dashboard-decisions.md](docs/features/010-feat-overview-dashboard/010-feat-overview-dashboard-decisions.md) (Summary: результат приёмки, статус по критериям, найденные расхождения — без дампов выводов команд)
- [ ] Если найдены fail-критерии — оформить находки (severity/issue/fix) для исправляющей итерации
- [ ] Если в ходе QA выявлено расхождение со спеком — описать его и причину; обновить user-spec/tech-spec при необходимости
