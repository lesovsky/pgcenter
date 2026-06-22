---
status: done                       # planned -> in_progress -> done
depends_on: []                     # ID задач-зависимостей (строки: ["01", "02"])
wave: 3                            # волна параллельного выполнения
skills: [documentation-writing]    # МАССИВ скиллов для загрузки
verify: bash                       # инструмент верификации (опционально: curl, bash, user)
reviewers: [dev-code-reviewer]     # явно указать. Пусто = fallback на defaults
teammate_name:                     # имя агента-исполнителя (опционально; если не задано — генерируется по описанию задачи)
---

# Task 10: Update documentation

## Required Skills

Перед выполнением задачи загрузи:
- `/skill:documentation-writing` — [skills/documentation-writing/SKILL.md](~/.claude/skills/documentation-writing/SKILL.md)

## Description

Релиз 0.11.0 добавил четыре экрана (5 типов отчёта: `bgwriter`, `replslots`, `stat_io`,
`stat_io_time`, `statements_jit`), которые были помечены `NotRecordable: true` — работали
только в живом TUI и не записывались/не проигрывались. Эта фича (008) снимает запрет на запись
и подключает их к `pgcenter record`/`report` с новыми CLI-флагами `-B`/`--bgwriter`,
`-L`/`--replslots`, `-J`/`--io c|t` и расширением `-X`/`--statements j`.

После кодовых задач (Wave 1–2) проектная документация всё ещё врёт: называет эти четыре экрана
«TUI-only / NotRecordable». Эта задача приводит доки в соответствие с реальностью: убирает
утверждения про TUI-only/незаписываемость для четырёх экранов и документирует новую
возможность record/report и новые CLI-флаги. Задача неблокирующая для кода (Wave 3,
непересекающиеся файлы), но обязательная для релиза — иначе доки расходятся с поведением.

Это документационная задача (skill `documentation-writing`) — кода нет.

## What to do

1. **`.claude/skills/project-knowledge/overview.md`** — в секции «Supported PostgreSQL
   Statistics» убрать хвост «TUI-only, not recordable in 0.11.0» из строк про `pg_stat_bgwriter`,
   `pg_replication_slots`, `pg_stat_io` и `pg_stat_statements` (JIT-подэкран). Заменить на
   формулировку, что экраны теперь записываются и проигрываются через `record`/`report` (как
   tables/wal/statements_*). Не раздувать — поправить только релевантные фрагменты строк.

2. **`.claude/skills/project-knowledge/architecture.md`** — в секции «PostgreSQL Version
   Handling» обновить четыре абзаца, которые сейчас называют эти вьюхи `NotRecordable` /
   «TUI-only» / «first/second/third/fourth NotRecordable user»:
   - абзац про `bgwriter` view (строка ~70: «first view registered with `NotRecordable: true`
     — TUI-only, excluded from record/report»);
   - абзац про `replslots` view (строка ~72: «the second `NotRecordable` user»);
   - абзац про `pg_stat_io` (строка ~74: «The third and fourth `NotRecordable` views»).
   Переформулировать на исторический/нейтральный тон: эти экраны были введены как TUI-only в
   0.11.0, а фича 008 включила им record/report (сняла `NotRecordable`, добавила CLI-флаги).
   Не описывать механику рекордера заново — он не менялся; достаточно зафиксировать, что вьюхи
   теперь recordable и как они проигрываются (version-aware раскладка по версии записи).
   При необходимости кратко упомянуть новые флаги report (`-B`/`-L`/`-J c|t`/`-X j`).

3. **`docs/features-catalog.md`** — в записях [004]/[005]/[006]/[007] обновить строки
   «Limitations: TUI-only ... `NotRecordable`» (строки 76, 97, 118, 137) и связанные
   «Touches»-упоминания «N-th `NotRecordable` view» (строки 102, 124, 142): отразить, что
   record/report-поддержка добавлена в фиче [008-feat-record-report-0-11-views] и больше не
   является ограничением. Снять формулировки «TUI-only»/«not recordable»/«deferred to backlog».

4. Проверить отсутствие оставшихся ложных утверждений: grep по трём файлам на
   `NotRecordable`, `TUI-only`, `not recordable`, `deferred` — для четырёх экранов таких
   утверждений остаться не должно (упоминание самого механизма `NotRecordable` как сущности
   допустимо, если оно фактически верно, но не как ярлык на этих четырёх вьюхах).

5. Прогнать `make lint` (на случай, если линтер цепляет markdown/прочее в репозитории) и
   перечитать изменённые секции глазами на связность.

## Acceptance Criteria

- [ ] `overview.md` больше не называет bgwriter/replslots/pg_stat_io/JIT «TUI-only, not
      recordable» и упоминает их поддержку в record/report.
- [ ] `architecture.md` обновлён: четыре абзаца не называют эти вьюхи `NotRecordable`/TUI-only
      как текущее ограничение; отражено, что фича 008 включила им запись/проигрывание.
- [ ] `docs/features-catalog.md` записи [004]/[005]/[006]/[007]: строки Limitations и Touches
      перестают называть экраны TUI-only/`NotRecordable`/незаписываемыми, ссылаются на [008].
- [ ] grep по трём файлам не находит ложных «TUI-only / NotRecordable / not recordable /
      deferred» утверждений про эти четыре экрана.
- [ ] `make lint` проходит без новых замечаний; изменённые секции читаются связно.

## Context Files

**Feature artifacts:**
- [008-feat-record-report-0-11-views.md](008-feat-record-report-0-11-views.md) — user-spec
- [008-feat-record-report-0-11-views-tech-spec.md](008-feat-record-report-0-11-views-tech-spec.md) — tech-spec
- [008-feat-record-report-0-11-views-decisions.md](008-feat-record-report-0-11-views-decisions.md) — decisions log (создаётся в ходе фичи)

**Project knowledge:**
- [overview.md](../../../.claude/skills/project-knowledge/overview.md) — файл на правку (supported stats / record-report capability)
- [architecture.md](../../../.claude/skills/project-knowledge/architecture.md) — файл на правку (NotRecordable-упоминания этих вьюх)
- [.claude/CLAUDE.md](../../../.claude/CLAUDE.md) — project constitution (контекст: команды, стек, ссылки на PK)
- [documentation-writing SKILL.md](~/.claude/skills/documentation-writing/SKILL.md) — методология правки доков

**Code files (правка доков, не кода):**
- [overview.md](../../../.claude/skills/project-knowledge/overview.md) — секция «Supported PostgreSQL Statistics»
- [architecture.md](../../../.claude/skills/project-knowledge/architecture.md) — секция «PostgreSQL Version Handling»
- [docs/features-catalog.md](../../features-catalog.md) — записи [004]/[005]/[006]/[007]

## Verification Steps

- Шаг 1: `grep -rn -i "TUI-only\|NotRecordable\|not recordable\|deferred" .claude/skills/project-knowledge/overview.md .claude/skills/project-knowledge/architecture.md docs/features-catalog.md` — для четырёх экранов ложных утверждений не остаётся.
- Шаг 2: перечитать изменённые секции в трёх файлах — формулировки про record/report корректны и связны (новые флаги `-B`/`-L`/`-J c|t`/`-X j` упомянуты там, где уместно).
- Шаг 3: `make lint` — проходит без новых замечаний.

## Details

**Files:**
- `.claude/skills/project-knowledge/overview.md` — секция «Supported PostgreSQL Statistics»,
  4 строки (bgwriter, replslots, pg_stat_io, pg_stat_statements/JIT) сейчас оканчиваются на
  «TUI-only, not recordable in 0.11.0». Убрать/переписать только этот хвост, оставив описание
  экранов как есть.
- `.claude/skills/project-knowledge/architecture.md` — секция «PostgreSQL Version Handling»:
  абзац bgwriter (~стр.70) «first view registered with `NotRecordable: true` — TUI-only,
  excluded from `pgcenter record`/`report`»; абзац replslots (~стр.72) «the second
  `NotRecordable` user»; абзац pg_stat_io (~стр.74) «The third and fourth `NotRecordable`
  views». Переписать на «введены как TUI-only в 0.11.0; фича 008 включила запись/проигрывание
  (сняла `NotRecordable`, добавила флаги `-B`/`-L`/`-J c|t`/`-X j`)».
- `docs/features-catalog.md` — записи [004]/[005]/[006]/[007]:
  - стр.76 (Limitations [004]): «TUI-only in 0.11.0 — the view is `NotRecordable` ... deferred to a backlog feature.»
  - стр.97 (Limitations [005]): «TUI-only in 0.11.0 — ... record/report is the planned next phase.»
  - стр.102 (Touches [005]): «second view ... registered `NotRecordable`.»
  - стр.118 (Limitations [006]): «TUI-only in 0.11.0 — both views are `NotRecordable` ...»
  - стр.124 (Touches [006]): «third + fourth `NotRecordable` views.»
  - стр.137 (Limitations [007]): «TUI-only in 0.11.0 — the view is `NotRecordable` ...»
  - стр.142 (Touches [007]): «fifth `NotRecordable` view added in 0.11.0 ...»
  Обновить так, чтобы record/report-поддержка указывалась как добавленная в [008], а не как
  действующее ограничение.

**Dependencies:** нет зависимостей от других задач по файлам (Wave 3, непересекающиеся
файлы). Логически опирается на реализацию Wave 1–2 (флаги/запись уже включены) — но правит
только доки.

**Edge cases:**
- Не удалять упоминание самого механизма `NotRecordable` как сущности там, где это фактически
  верно (например, синтетический тест дроп-ветки) — менять только ярлык «эти 4 экрана
  TUI-only/незаписываемы».
- В architecture.md «first/second/third/fourth NotRecordable user» — это исторические маркеры
  очерёдности; либо переформулировать в прошедшем времени, либо убрать счётчик, чтобы не
  создавать впечатление действующего ограничения.
- JIT — подэкран pg_stat_statements; в overview.md правка внутри длинной строки про
  `pg_stat_statements`, аккуратно тронуть только JIT-фрагмент.

**Implementation hints:**
- Минимальные точечные правки — не переписывать секции целиком, не «улучшать» соседний текст.
- Новые флаги для справки: `-B`/`--bgwriter`, `-L`/`--replslots`, `-J`/`--io` (`c`→stat_io,
  `t`→stat_io_time), `-X`/`--statements j`→statements_jit; `report -d <флаг>` печатает описание.
- Версии по экранам: bgwriter/replslots — PG14+, statements_jit — PG15+, stat_io/stat_io_time — PG16+.

## Reviewers

- **dev-code-reviewer** → `008-feat-record-report-0-11-views-task-10-dev-code-reviewer-review.json`

## Post-completion

- [ ] Записать краткий отчёт в [008-feat-record-report-0-11-views-decisions.md](008-feat-record-report-0-11-views-decisions.md) (Summary: 1-3 предложения, ревью со ссылками на JSON, без таблиц файндингов и дампов)
- [ ] Если отклонились от спека — описать отклонение и причину
- [ ] Обновить user-spec/tech-spec если что-то изменилось
