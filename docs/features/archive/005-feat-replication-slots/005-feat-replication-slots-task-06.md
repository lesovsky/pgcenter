---
status: planned                    # planned -> in_progress -> done
depends_on: ["02", "03", "05"]     # ID задач-зависимостей (строки)
wave: 4                            # волна параллельного выполнения
skills: [pre-deploy-qa]            # МАССИВ скиллов для загрузки
verify: bash — full test/lint/vuln/build suite green; acceptance criteria met
reviewers: []                      # QA-gate — без ревьюеров
teammate_name:                     # имя агента-исполнителя (опционально)
---

# Task 06: Pre-deploy QA

## Required Skills

Перед выполнением задачи загрузи:
- `/skill:pre-deploy-qa` — [skills/pre-deploy-qa/SKILL.md](~/.claude/skills/pre-deploy-qa/SKILL.md)

## Description

Финальный приёмочный гейт фичи `replslots` (экран статистики слотов репликации,
горячая клавиша `o`). Это **QA-задача, а не написание кода** — здесь не пишется и не правится
никакой production-код. Цель: убедиться, что вся реализация (Wave 1–3: запрос+селектор, view+TUI,
интеграционные тесты, bump `Test_filterViews`, bump тестового образа) собирается, проходит весь
тестовый/линт/vuln/build-набор и удовлетворяет всем критериям приёмки из user-spec и tech-spec.

Задача стоит в финальной волне и зависит от завершения Task 02 (view wired в TUI),
Task 03 (`Test_filterViews` обновлён) и Task 05 (физический/логический интеграционные тесты).
Task 01 и Task 04 — транзитивные зависимости (через 02/05), отдельно перечислять не нужно.

Если на гейте обнаружится критическое нарушение (упавший тест, не собирается бинарь, не выполнен
критерий приёмки) — задача фиксирует finding с конкретным воспроизведением и эскалирует
к team lead; чинить production-код в рамках этой задачи нельзя, кроме явных fix-коммитов согласно
скиллу pre-deploy-qa (Phase 1, max 3 попытки на запуск окружения).

## What to do

1. Прочитать user-spec, tech-spec и (если появился) decisions-лог — собрать полный список
   критериев приёмки (пользовательских + технических).
2. Прогнать полный набор проверок проекта на актуальном закоммиченном коде:
   - `make test` (race detector + coverage; unit + tier-1/2 integration; tier-3 на образе `0.0.10`)
   - `make lint` (golangci-lint + gosec)
   - `make vuln` (govulncheck)
   - `make build` (бинарь в `./bin/pgcenter`)
3. Зафиксировать по каждому критерию приёмки статус (`passed` / `failed` / `not_verifiable`)
   с конкретным доказательством — имя теста, путь в коде, вывод лога.
4. Проверить покрытие фичи тестами: для каждого файла из «Files to modify» tech-spec убедиться,
   что есть соответствующий тест; что `passed`-критерии подкреплены тестом, реально исполняющим
   поведение (а не import-only).
5. Сформировать QA-отчёт (JSON) и краткую запись в decisions-лог со ссылкой на отчёт.
6. Если есть `not_verifiable` критерии (требующие живого TUI/глаз пользователя) — отметить их
   как deferred-to-post-deploy с условием и шагами проверки.

## Acceptance Criteria

Критерии из user-spec и tech-spec, проверяемые на этом гейте:

- [ ] `make test` зелёный: selector unit-тест + tier-1/2 integration на доступных PG-версиях;
      tier-3 зелёный на образе `0.0.10`, иначе корректно skipped.
- [ ] `make lint`, `make vuln`, `make build` — без замечаний; бинарь собирается.
- [ ] `SelectStatReplicationSlotsQuery` возвращает `(query, 15, [2]int{6,13})` для PG 14–18
      (единый запрос, без версионного ветвления) — unit-тест зелёный.
- [ ] Живой запрос возвращает ровно 15 колонок на PG 14–18 (`FieldDescriptions` gate).
- [ ] Физический слот: строка присутствует, `retained,KiB` не NULL, 8 дифф-счётчиков = `0`,
      `safe,KiB`/`stats_age` пусто.
- [ ] Логический слот: строка присутствует, spill/stream-колонки на месте (или skipped при
      `wal_level != logical`).
- [ ] `retained,KiB` recovery-aware (primary → `pg_current_wal_lsn`, standby →
      `pg_last_wal_receive_lsn`). Не проверяется автопрогоном — требует живого standby;
      **deferred-to-post-deploy / not-verifiable** на этом гейте, фиксируется явно, чтобы не
      потеряться.
- [ ] Клавиша `o` открывает `replslots`; view зарегистрирован `NotRecordable: true`;
      `Test_filterViews` зелёный с увеличенными счётчиками.
- [ ] Клавиша `o` присутствует в экране справки (`?`).
- [ ] Дефолтная сортировка по `retained,KiB` убыванием (`OrderKey=4`, `OrderDesc=true`);
      сортировка/фильтр работают как у других многострочных вьюх.
- [ ] Регрессий нет: весь набор `make test/lint/vuln/build` чистый.
- [ ] QA-отчёт (JSON) и запись в decisions-лог сформированы.

## Context Files

**Feature artifacts:**
- [005-feat-replication-slots.md](005-feat-replication-slots.md) — user-spec (критерии приёмки)
- [005-feat-replication-slots-tech-spec.md](005-feat-replication-slots-tech-spec.md) — tech-spec (технические критерии, Files to modify)
- [005-feat-replication-slots-decisions.md](005-feat-replication-slots-decisions.md) — decisions-лог (отклонения от плана, если есть)

**Project knowledge:**
- [overview.md](.claude/skills/project-knowledge/overview.md) — фичи, поддерживаемая статистика, аудитория
- [architecture.md](.claude/skills/project-knowledge/architecture.md) — раскладка пакетов, поток данных, обработка версий PG
- [patterns.md](.claude/skills/project-knowledge/patterns.md) — паттерны кода, тестовые соглашения, Git workflow

**Code files (под проверкой, не под правкой):**
- [internal/query/replication_slots.go](internal/query/replication_slots.go) — запрос + селектор (Task 01)
- [internal/query/replication_slots_test.go](internal/query/replication_slots_test.go) — unit + integration tiers 1/2/3 (Task 01, 05)
- [internal/view/view.go](internal/view/view.go) — регистрация view + Configure (Task 02)
- [top/keybindings.go](top/keybindings.go) — биндинг `o` (Task 02)
- [top/help.go](top/help.go) — `o` в справке (Task 02)
- [record/record_test.go](record/record_test.go) — `Test_filterViews` (Task 03)
- [testing/Dockerfile](testing/Dockerfile), [testing/prepare-test-environment.sh](testing/prepare-test-environment.sh) — тестовый образ `0.0.10` (Task 04)

## Verification Steps

- Запустить `make build` — бинарь `./bin/pgcenter` собирается без ошибок.
- Запустить `make test` — весь набор зелёный; зафиксировать total/passed/failed/skipped.
  Убедиться, что tier-3 логического слота либо зелёный (на образе `0.0.10`), либо корректно
  пропущен с `t.Skipf` (на образе с `wal_level=replica`) — пропуск не является провалом.
- Запустить `make lint` — без замечаний golangci-lint/gosec.
- Запустить `make vuln` — govulncheck без находок.
- Для каждого критерия приёмки сопоставить доказательство (имя теста / путь / лог).
- Критерии, требующие живого TUI и глаз пользователя (визуальный рендер экрана `o`, ручная
  проверка сортировки/справки) — пометить `not_verifiable` и вынести в deferred-to-post-deploy.

## Details

**Тип задачи:** pre-deploy QA gate. Production-код НЕ пишется. Без TDD Anchor (проверка —
прогон набора + сверка критериев приёмки).

**Окружение:** проект использует Makefile как основной интерфейс (`make build/test/lint/vuln`).
Тесты гоняются с race detector и coverage. Интеграционные тесты требуют живых контейнеров
PostgreSQL 14–18; tier-3 (логический слот) требует образа `lesovsky/pgcenter-testing:0.0.10`
с `wal_level=logical` и плагином `test_decoding`. Если образ `0.0.10` ещё не запушен мейнтейнером
(ручной шаг из Task 04), tier-3 корректно пропускается через `t.Skipf` — это ожидаемо и не
является провалом гейта (Decision 5 tech-spec).

**Files (под проверкой):**
- `internal/query/replication_slots.go` — гибридный запрос + `SelectStatReplicationSlotsQuery`;
  проверить контракт `(query, 15, [2]int{6,13})`, `coalesce(...,0)` на 8 счётчиках.
- `internal/query/replication_slots_test.go` — unit (Ncols/DiffIntvl на 14–18) + tier-1
  (`FieldDescriptions == 15`) + tier-2 (физический слот) + tier-3 (логический слот).
- `internal/view/view.go`, `top/keybindings.go`, `top/help.go` — регистрация view, биндинг `o`,
  справка.
- `record/record_test.go` — `Test_filterViews` (`wantN` +1 на каждой строке).
- `testing/Dockerfile`, `testing/prepare-test-environment.sh`, `.github/workflows/default.yml`,
  `.github/workflows/release.yml` — bump образа `0.0.9 → 0.0.10`.

**Dependencies:** Task 02 (view+TUI), Task 03 (`Test_filterViews`), Task 05 (integration tiers).
Транзитивно — Task 01 (запрос/селектор), Task 04 (тестовый образ).

**Edge cases при проверке:**
- tier-3 пропущен (нет образа `0.0.10` / `wal_level=replica`) — это `not_verifiable`/ожидаемый
  skip, НЕ proval. Зафиксировать как deferred с условием «после пуша образа `0.0.10`».
- Контейнеры PG недоступны на момент QA — интеграционные tier-1/2/3 нельзя выполнить; пометить
  соответствующие критерии `not_verifiable` и вынести в deferred (нужен живой кластер).
- Визуальные критерии (рендер экрана, цвет/раскладка колонок, реакция стрелок/`/`) —
  `not_verifiable` автоматическим прогоном; вынести в deferred-to-post-deploy для проверки
  пользователем в `pgcenter top`.

**Implementation hints:**
- Следовать фазам скилла pre-deploy-qa: Environment Rebuild (если применимо к Docker-окружению
  тестов) → Test Suite → Acceptance Criteria → Coverage Verification.
- Status отчёта: `passed`, только если ноль critical и окружение здорово; иначе `failed`.
- При критическом нарушении — НЕ имитировать «зелёный» результат; зафиксировать finding с
  конкретным воспроизведением (шаги, expected vs actual) и эскалировать к team lead.
- Не считать `t.Skipf`-пропуски за провал тестов.

## Reviewers

Нет — это QA-гейт без ревью.

## Post-completion

- [ ] Записать краткий отчёт в [005-feat-replication-slots-decisions.md](005-feat-replication-slots-decisions.md) (Summary: 1-3 предложения; ссылка на JSON QA-отчёт; перечислить deferred-to-post-deploy критерии, если есть; без таблиц файндингов и дампов)
- [ ] Если обнаружены отклонения от спека — описать отклонение и причину
- [ ] Обновить user-spec/tech-spec, если что-то фактически изменилось в ходе QA
