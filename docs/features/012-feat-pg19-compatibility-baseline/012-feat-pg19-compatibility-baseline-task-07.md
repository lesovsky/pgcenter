---
status: planned
depends_on: ["03"]
wave: 4
skills: [documentation-writing]
verify: bash — no stale 14–18 version ranges remain in the three files
reviewers: [dev-code-reviewer]
teammate_name:
---

# Task 07: Documentation update

## Required Skills

Перед выполнением задачи загрузи:
- `/skill:documentation-writing` — [skills/documentation-writing/SKILL.md](~/.claude/skills/documentation-writing/SKILL.md)

## Description

Зафиксировать поддержку PostgreSQL 19 в базе знаний проекта (`.claude/skills/project-knowledge/`).
Меняются три документа, каждый по своей причине:

- **overview.md** — утверждение о поддерживаемых версиях («Active support: PG 14, 15, 16, 17, 18») и
  диапазоны версий у отдельных экранов в разделе Supported PostgreSQL Statistics, где сегодня стоит
  «PG 14–18».
- **deployment.md** — состав тестового образа (`PostgreSQL 14–18`) и список портов кластеров
  (`PG14=21914 … PG18=21918`), которому нужен `PG19=21919`.
- **architecture.md** — инвентарь версионных селекторов в разделе «PostgreSQL Version Handling», который
  пополняется тремя progress-селекторами из Task 3, и карта портов в разделе Testing.

Инвентарь селекторов в architecture.md — это то, что следующая фича читает первым, когда ей нужно понять,
какие экраны уже версионно-зависимы и по какому идиому. Поэтому три новых селектора и новые колонки —
содержательная часть задачи, а не косметика.

Диапазоны вида «PG 14–18» рассыпаны по нескольким местам, а не лежат в одной строке, поэтому приёмка —
grep по устаревшим диапазонам, а не проверка одной очевидной строки.

**README намеренно не трогаем.** Списка поддерживаемых версий в нём нет, и user-spec явно убрал его из
критериев приёмки: заводить такой список — значит заводить ещё одно место, которое устаревает каждый релиз
(user-spec, «Что решили и почему»).

## What to do

1. Прочитать три файла целиком и найти все места, где встречается верхняя граница поддержки (строки вида
   «14–18», «14-18», «PG 18», «21918») — не только те, что перечислены в Details.
2. **overview.md:** обновить раздел «PostgreSQL Version Support» (активная поддержка включает PG 19) и
   диапазоны версий у экранов bgwriter и replication slots, где стоит «PG 14–18». Проверить формулировки
   про PG 18 у `pg_stat_io` и `pg_stat_wal` — они описывают, что произошло *в* PG 18, и переписывать их не
   нужно, если утверждение остаётся верным на PG 19.
3. **overview.md:** отразить, что три progress-экрана (vacuum, analyze, basebackup) на PG 19 показывают
   новые колонки `started_by` / `mode` / `backup_type` — так же кратко, как описаны остальные экраны, без
   раздувания.
4. **deployment.md:** состав тестового образа — PostgreSQL 14–19; в список портов добавить `PG19=21919`.
   Не менять тег образа и не описывать процесс публикации — это зона Task 8 и Task 9.
5. **architecture.md:** в инвентарь версионных селекторов добавить три новых селектора с их точками
   ветвления, возвращаемыми `(query, Ncols, DiffIntvl)` и константой `PostgresV19`; в разделе Testing
   добавить порт PG 19 в карту портов и отразить, что `NewTestConnectVersion` возвращает ошибку для версии
   без порта (это уже описано — проверить, что формулировка совпадает с реализацией из Task 2).
6. Прогнать grep по устаревшим диапазонам (см. Verification Steps) и убедиться, что остались только те
   упоминания PG 18, которые описывают исторические изменения схемы, а не границу поддержки.

## Acceptance Criteria

- [ ] `overview.md` заявляет активную поддержку PG 14–19; ни один экран не описан диапазоном «PG 14–18»
      там, где на самом деле работает и на PG 19.
- [ ] `overview.md` упоминает новые колонки progress-экранов на PG 19 (`started_by`, `mode`, `backup_type`).
- [ ] `deployment.md` описывает тестовый образ как содержащий PostgreSQL 14–19 и перечисляет порт
      `PG19=21919`.
- [ ] `architecture.md` перечисляет `SelectStatProgressVacuumQuery`, `SelectStatProgressAnalyzeQuery`,
      `SelectStatProgressBasebackupQuery` в инвентаре версионных селекторов — с точкой ветвления PG 19 и
      возвращаемыми значениями.
- [ ] `architecture.md` содержит `PG19=21919` в карте портов раздела Testing.
- [ ] grep по устаревшим диапазонам («14–18», «14-18») по трём файлам не находит утверждений о границе
      поддержки.
- [ ] README не изменён.
- [ ] В документации нет блоков кода с реализацией и дублирования того, что уже сказано в соседнем файле
      (принципы documentation-writing).

## Context Files

**Feature artifacts:**
- [012-feat-pg19-compatibility-baseline.md](012-feat-pg19-compatibility-baseline.md) — user-spec
  (раскладки экранов, критерий про документацию, решение про README)
- [012-feat-pg19-compatibility-baseline-tech-spec.md](012-feat-pg19-compatibility-baseline-tech-spec.md) —
  tech-spec (Task 7, Decision 1 и 2 — позиции колонок и арность селекторов)
- [012-feat-pg19-compatibility-baseline-decisions.md](012-feat-pg19-compatibility-baseline-decisions.md) —
  decisions log (отчёт Task 3 — что фактически реализовано в селекторах)

**Project knowledge:**
- [overview.md](../../../.claude/skills/project-knowledge/overview.md) — файл проекта (аналог project.md);
  изменяется
- [architecture.md](../../../.claude/skills/project-knowledge/architecture.md) — изменяется
- [deployment.md](../../../.claude/skills/project-knowledge/deployment.md) — изменяется
- [patterns.md](../../../.claude/skills/project-knowledge/patterns.md) — конвенции версионного ветвления;
  читать для сверки формулировок про селекторы (не изменяется)

**Code files (только чтение — источник фактов для документации):**
- [internal/query/query.go](../../../internal/query/query.go) — константа `PostgresV19`
- [internal/query/progress_vacuum.go](../../../internal/query/progress_vacuum.go),
  [progress_analyze.go](../../../internal/query/progress_analyze.go),
  [progress_basebackup.go](../../../internal/query/progress_basebackup.go) — три новых селектора
- [internal/postgres/testing.go](../../../internal/postgres/testing.go) — карта портов и поведение
  `NewTestConnectVersion`
- [testing/Dockerfile](../../../testing/Dockerfile) — фактический состав кластеров в образе

## Verification Steps

- Grep по устаревшим диапазонам — не должен находить утверждений о границе поддержки:
  `grep -rn "14–18\|14-18" .claude/skills/project-knowledge/overview.md .claude/skills/project-knowledge/deployment.md .claude/skills/project-knowledge/architecture.md`
- Grep по PG 19 — должен находить его во всех трёх файлах:
  `grep -rln "PG 19\|PostgresV19\|21919" .claude/skills/project-knowledge/`
- `git diff --name-only` показывает ровно три изменённых файла; `README.md` среди них нет.
- Сверить каждое новое утверждение с кодом: имена селекторов и `Ncols`/`DiffIntvl` в architecture.md
  совпадают с реализацией из Task 3, порт 21919 — с картой в `internal/postgres/testing.go`.

## Details

**Files:**

- `.claude/skills/project-knowledge/overview.md` — сейчас: раздел «PostgreSQL Version Support» говорит
  «Active support: PG 14, 15, 16, 17, 18»; в разделе Supported PostgreSQL Statistics у экранов bgwriter и
  replication slots стоит «PG 14–18», у `pg_stat_io` — «PG 16+», у `pg_stat_wal` — «PG 14+». Изменить:
  верхнюю границу активной поддержки и диапазоны экранов; добавить упоминание новых progress-колонок PG 19.
  Строки про `pg_stat_io`/`pg_stat_wal`, описывающие *что произошло в PG 18*, — исторические факты, их
  переписывать не нужно.
- `.claude/skills/project-knowledge/deployment.md` — сейчас раздел «Test Container»: «Contains: Ubuntu
  22.04, PostgreSQL 14–18 with `plperlu` + CPAN modules» и «Ports: PG14=21914 … PG18=21918». Изменить обе
  строки. Тег образа (`0.0.10`) и инструкцию по сборке/публикации не трогать — их обновляют Task 8/Task 9.
- `.claude/skills/project-knowledge/architecture.md` — сейчас раздел «PostgreSQL Version Handling»
  перечисляет селекторы `SelectStatActivityQuery`, `SelectStatReplicationQuery`,
  `SelectStatDatabaseGeneralQuery`, `SelectStatStatementsTimingQuery`, `SelectStatWALQuery`,
  `SelectStatBgwriterQuery`, `SelectStatReplicationSlotsQuery`, `SelectStatIOQuery`/`SelectStatIOTimeQuery`;
  строка про `SelectStatReplicationSlotsQuery` содержит «version-independent on PG 14–18». Добавить три
  progress-селектора в том же стиле (одна строка на селектор: точка ветвления + что возвращает + чем
  отличается PG 19-ветка). Раздел Testing: карта портов и описание `NewTestConnectVersion`.

**Факты для architecture.md (из Decision 1 и 2 tech-spec; сверить с кодом Task 3):**

| Селектор | Ветвление | PG 19: колонки | Ncols (pre-19 → 19) | DiffIntvl (pre-19 → 19) |
|---|---|---|---|---|
| `SelectStatProgressVacuumQuery(version)` | PG 19 | `started_by`, `mode` | 13 → 15 | `{10,11}` → `{12,13}` |
| `SelectStatProgressAnalyzeQuery(version)` | PG 19 | `started_by` | 12 → 13 | `{0,0}` → `{0,0}` |
| `SelectStatProgressBasebackupQuery(version)` | PG 19 | `backup_type` | 11 → 12 | `{9,9}` → `{10,10}` |

Все три возвращают 3-кортеж `(string, int, [2]int)` (Decision 2: `UniqueKey` не двигается — `pid` остаётся
колонкой 0, поэтому 4-кортеж по ADR [007] не требуется). Новые колонки вставлены непосредственно перед
`state` — после `relation` на vacuum/analyze, после `duration` на basebackup (Decision 1). Значения
pre-19 `Ncols`/`DiffIntvl` в таблице выше — ожидаемые; **обязательно сверить с фактическим кодом**
`internal/query/progress_*.go` и `internal/view/view.go` после Task 3 и писать в документацию то, что
реально в коде.

**Dependencies:** Task 3 (три селектора должны существовать — документируем реализованное, а не
запланированное). Task 2 даёт `PostgresV19` и порт 21919. Task 1 даёт PG 19 в образе. Пакетов не требует.

**Edge cases:**
- Упоминание «PG 18» бывает двух типов: граница поддержки (обновляем) и исторический факт вида «WAL IO
  timings moved to `pg_stat_io` in PG 18» (оставляем). Различать по смыслу, а не заменять слепо.
- Тире в диапазонах — длинное (`–`), не дефис. Grep должен ловить оба варианта.
- `SelectStatReplicationSlotsQuery` описан как «version-independent on PG 14–18» — на PG 19 набор колонок
  не менялся, диапазон нужно расширить, а не превращать строку в версионно-зависимую.
- Экран `progress_cluster` на PG 19 продолжает работать (`pg_stat_progress_cluster` сохранена ради
  обратной совместимости с `REPACK`) — если в документации появится соблазн это отметить, держать одной
  фразой; две новые progress-вью PG 19 (`pg_stat_progress_repack`, `pg_stat_progress_data_checksums`) —
  вне скоупа фичи, в документацию их не заносить.
- Число зарегистрированных экранов (27) не меняется — не править счётчики, если они где-то упомянуты.

**Implementation hints:**
- Documentation-writing: без блоков кода с реализацией, без дублирования между файлами — каждый факт живёт
  в одном месте (состав образа и порты — deployment.md; поведение тест-хелперов и инвентарь селекторов —
  architecture.md; пользовательский взгляд на экраны — overview.md). Карта портов дублируется в
  deployment.md и architecture.md уже сегодня — это существующее состояние, синхронизировать оба, новых
  дублей не заводить.
- Стиль правок — минимальный diff: правим существующие строки, новые разделы не создаём.
- Не описывать процесс релиза, теги образа и CI — это Task 8/Task 9 и другие разделы deployment.md.

## Reviewers

- **dev-code-reviewer** → `012-feat-pg19-compatibility-baseline-task-07-dev-code-reviewer-review.json`

## Post-completion

- [ ] Записать краткий отчёт в [012-feat-pg19-compatibility-baseline-decisions.md](012-feat-pg19-compatibility-baseline-decisions.md) (Summary: 1-3 предложения, ревью со ссылками на JSON, без таблиц файндингов и дампов)
- [ ] Если отклонились от спека — описать отклонение и причину
- [ ] Обновить user-spec/tech-spec если что-то изменилось
