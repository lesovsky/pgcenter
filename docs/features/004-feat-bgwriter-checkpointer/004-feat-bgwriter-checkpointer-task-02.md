---
status: planned                    # planned -> in_progress -> done
depends_on: []                     # ID задач-зависимостей (строки: ["01", "02"])
wave: 1                            # волна параллельного выполнения
skills: [documentation-writing]    # МАССИВ скиллов для загрузки
verify: bash                       # инструмент верификации (опционально: curl, bash, user)
reviewers: [dev-code-reviewer]     # явно указать. Пусто = fallback на defaults
teammate_name:                     # имя агента-исполнителя (опционально; если не задано — генерируется по описанию задачи)
---

# Task 02: Correct overview.md

## Required Skills

Перед выполнением задачи загрузи:
- `/skill:documentation-writing` — [skills/documentation-writing/SKILL.md](~/.claude/skills/documentation-writing/SKILL.md)

## Description

Документация проекта (`.claude/skills/project-knowledge/overview.md`) годами ошибочно
заявляла, что pgcenter поддерживает `pg_stat_bgwriter` — в разделе «Supported PostgreSQL
Statistics» (строка ~21) стоит запись `pg_stat_bgwriter — background writer stats`, хотя
такого экрана в pgcenter нет.

Фича 004 добавляет этот экран впервые. В рамках данной задачи нужно привести `overview.md` в
соответствие с реальностью: убрать ложное «уже поддерживается» и заменить его на корректную
запись о НОВОМ экране bgwriter/checkpointer, который добавляется этой фичей.

Задача документационная, только для `overview.md`. Кода не касается. Она независима от
кодового пути (Wave 1) — корректность записи проверяется текстом user-spec, а не работающим
экраном.

## What to do

- Найти в `overview.md` раздел «Supported PostgreSQL Statistics» и строку про
  `pg_stat_bgwriter`, ошибочно перечисляющую его среди уже поддерживаемых статистик.
- Заменить эту запись точным описанием нового экрана bgwriter/checkpointer, отражающим
  текущую реальность фичи 004:
  - экран читает `pg_stat_bgwriter` и (на PG 17+) `pg_stat_checkpointer`;
  - однострочный экран в `pgcenter top`, горячая клавиша `b`;
  - поддерживаемые версии PostgreSQL 14–18, версионно-зависимый набор колонок;
  - в релизе 0.11.0 экран доступен только в интерактивном режиме `top` (помечен
    `NotRecordable`) — не записывается `record` и не отображается в `report`.
- Сохранить стиль и формат раздела (список `- \`name\` — описание`, тон, длина строк) — не
  переписывать соседние записи, не «улучшать» остальной файл.
- Убедиться, что после правки в файле не остаётся утверждения, будто `pg_stat_bgwriter`
  уже поддерживается «как есть» без оговорок про новый экран.

## Acceptance Criteria

- [ ] Из `overview.md` убрана ложная запись о уже существующей поддержке `pg_stat_bgwriter`.
- [ ] В `overview.md` есть точная запись про новый экран bgwriter/checkpointer:
      `pg_stat_bgwriter` + `pg_stat_checkpointer`, клавиша `b`, PG 14–18, только `top` /
      `NotRecordable` в 0.11.0.
- [ ] Стиль и формат раздела сохранены; соседние записи не тронуты.
- [ ] Никаких изменений в коде или других файлах PK.

## Context Files

**Feature artifacts:**
- [004-feat-bgwriter-checkpointer.md](docs/features/004-feat-bgwriter-checkpointer/004-feat-bgwriter-checkpointer.md) — user-spec
- [004-feat-bgwriter-checkpointer-tech-spec.md](docs/features/004-feat-bgwriter-checkpointer/004-feat-bgwriter-checkpointer-tech-spec.md) — tech-spec
- [004-feat-bgwriter-checkpointer-decisions.md](docs/features/004-feat-bgwriter-checkpointer/004-feat-bgwriter-checkpointer-decisions.md) — decisions log

**Project knowledge:**
- [overview.md](.claude/skills/project-knowledge/overview.md) — файл для правки (раздел «Supported PostgreSQL Statistics»)
- [architecture.md](.claude/skills/project-knowledge/architecture.md) — package layout, version handling (контекст)

**Code files:**
- [overview.md](.claude/skills/project-knowledge/overview.md) — раздел «Supported PostgreSQL Statistics», строка ~21 — заменить запись про `pg_stat_bgwriter`

## Verification Steps

- Шаг 1 (stale claim убран): убедиться, что в `overview.md` не осталось старой формулировки,
  заявляющей `pg_stat_bgwriter` как уже поддерживаемую статистику без контекста нового экрана.
  Пример: `grep -nE 'pg_stat_bgwriter[^+]*— background writer stats' .claude/skills/project-knowledge/overview.md`
  — старая строка `pg_stat_bgwriter — background writer stats` находиться не должна.
- Шаг 2 (новый экран упомянут): `grep -niE 'pg_stat_checkpointer|bgwriter' .claude/skills/project-knowledge/overview.md`
  — должна найтись новая запись про экран bgwriter/checkpointer (с `pg_stat_checkpointer` и/или
  упоминанием клавиши `b` / только-`top`).
- Ожидаемый результат: stale-формулировка отсутствует, новая корректная запись присутствует.

## Details

**Files:**
- `.claude/skills/project-knowledge/overview.md` — текущее состояние: в разделе «Supported
  PostgreSQL Statistics» (маркированный список, начинается строкой 17) на строке 21 стоит
  `- \`pg_stat_bgwriter\` — background writer stats`, что ложно заявляет уже существующую
  поддержку. Соседние строки (`pg_stat_wal` на строке 22, `pg_stat_statements` и т.д.)
  корректны — менять их не нужно. Заменить ТОЛЬКО строку про `pg_stat_bgwriter` на запись о
  новом экране bgwriter/checkpointer.

**Dependencies:** нет (Wave 1, depends_on: []). Независима от кодовых задач 1 и 3.

**Edge cases:**
- Не превращать запись в дублирующее описание всей фичи — overview.md высокоуровневый,
  достаточно одной точной строки/записи в стиле остальных пунктов раздела.
- Не заявлять поддержку `record`/`report`: в 0.11.0 экран `NotRecordable`, только `top`.
- `pg_stat_checkpointer` относится только к PG 17+; не утверждать, что он читается на всех
  версиях.

**Implementation hints:**
- Раздел оформлен маркированным списком (`- \`name\` — описание`). Сохранить этот формат,
  как у соседней записи `pg_stat_wal` (которая уже корректно несёт версионную оговорку
  `(PG 14+; reduced schema in PG 18)`).
- Источники правды по составу записи — user-spec (разделы «Что делаем», «Ограничения») и
  tech-spec (Solution): клавиша `b`, PG 14–18, `pg_stat_bgwriter` + `pg_stat_checkpointer`
  (PG 17+), `NotRecordable` / только `top` в 0.11.0.
- Принцип documentation-writing: без блоков кода, кратко, операционно по сути — одна точная
  запись в существующем стиле.

## Reviewers

- **dev-code-reviewer** → `docs/features/004-feat-bgwriter-checkpointer/004-feat-bgwriter-checkpointer-task-02-dev-code-reviewer-review.json`

## Post-completion

- [ ] Записать краткий отчёт в [004-feat-bgwriter-checkpointer-decisions.md](docs/features/004-feat-bgwriter-checkpointer/004-feat-bgwriter-checkpointer-decisions.md) (Summary: 1-3 предложения, ревью со ссылками на JSON, без таблиц файндингов и дампов)
- [ ] Если отклонились от спека — описать отклонение и причину
- [ ] Обновить user-spec/tech-spec если что-то изменилось
