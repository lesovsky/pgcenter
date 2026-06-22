---
status: done                       # planned -> in_progress -> done
depends_on: []                     # ID задач-зависимостей (строки: ["01", "02"])
wave: 1                            # волна параллельного выполнения
skills: [code-writing]             # МАССИВ скиллов для загрузки
verify: bash                       # go test ./internal/view/... ./record/...
reviewers: [dev-code-reviewer, dev-test-reviewer]
teammate_name:
---

# Task 01: Enable recording + fix view/filter count tests

## Required Skills

Перед выполнением задачи загрузи:
- `/skill:code-writing` — [skills/code-writing/SKILL.md](~/.claude/skills/code-writing/SKILL.md)

## Description

Сейчас 4 экрана 0.11.0 (5 report-типов: `bgwriter`, `replslots`, `stat_io`, `stat_io_time`,
`statements_jit`) помечены в `internal/view/view.go` флагом `NotRecordable: true`. Из-за этого
`record/filterViews` отбрасывает их ещё до проверки версии, и `pgcenter record` их не собирает.

Эта задача — единственное изменение, превращающее эти экраны в записываемые view: убрать пять
строк `NotRecordable: true`. После этого каждый из view проходит дальше по `filterViews` к
version-gate (а `statements_jit` — ещё и к pgss-gate), и попадает в общий pure-SQL collect/write
путь рекордера без каких-либо изменений самого рекордера.

Вся остальная работа в этой задаче — починка тестов, чьи числовые ожидания сдвигаются от снятия
флага: пять per-view assertion'ов `NotRecordable` в `view_test.go` (с правкой их комментариев),
а также per-version счётчики в `Test_filterViews` в `record_test.go` (меняются ТОЛЬКО строки для
PG14) с переписыванием большого пояснительного комментария, который теперь стал неверным.
Дополнительно — поправить устаревший комментарий в `record.go`, который приводит `bgwriter` как
пример NotRecordable-view (после фичи ни один production-view не ставит `NotRecordable: true`).

Сам механизм drop-ветки в `filterViews` и его синтетический guard-тест
`TestFilterViews_dropsExplicitNotRecordable` оставляем без изменений — это становится единственным
покрытием механизма NotRecordable. `TestNew` (общее число view = 27) НЕ меняем.

Рационал — см. Decisions §1 и §6 в tech-spec. Точные литералы и номера строк — в code-research.

## What to do

1. В `internal/view/view.go` удалить строку `NotRecordable: true` из 5 определений view:
   - `bgwriter` (строка 151)
   - `replslots` (строка 164)
   - `stat_io` (строка 178)
   - `stat_io_time` (строка 192)
   - `statements_jit` (строка 242)
   Удаляется только эта строка в каждом блоке; остальные поля (Name, MinRequiredVersion,
   QueryTmpl, Ncols, DiffIntvl, OrderKey, UniqueKey, Msg и т.д.) остаются как есть.

2. В `internal/view/view_test.go` перевернуть 5 per-view assertion'ов с
   `assert.True(t, X.NotRecordable)` на `assert.False(t, X.NotRecordable)`:
   - строка 21 (`jit.NotRecordable`)
   - строка 40 (`statio.NotRecordable`)
   - строка 58 (`statioTime.NotRecordable`)
   - строка 75 (`replslots.NotRecordable`)
   - строка 91 (`bgwriter.NotRecordable`)
   И поправить doc-комментарии этих тест-функций (строки 15, 34, 52, 70, 86), убрав формулировку
   «excluded from recording (NotRecordable)» — теперь это фактически неверно. Заменить на
   формулировку, отражающую, что view теперь записываемый (recordable).

3. В `record/record_test.go` обновить `Test_filterViews`:
   - Изменить ТОЛЬКО две строки PG14 в списке testcases (строки 127-128):
     - `{version: 140000, pgssSchema: "", wantN: 11, wantV: 16}` → `wantN: 9, wantV: 18`
     - `{version: 140000, pgssSchema: "public", wantN: 5, wantV: 22}` → `wantN: 3, wantV: 24`
   - Строки для PG13/12/11/10 (129-132) оставить БЕЗ изменений — эти view version-gated и на
     PG<14 всё равно отбрасываются version-gate'ом, так что счётчики не меняются.
   - Переписать большой пояснительный комментарий (строки 108-126): сейчас он описывает, как
     каждый NotRecordable-view добавляет +1 к `wantN`. После снятия флага логика инвертируется —
     комментарий нужно переписать так, чтобы он объяснял новую картину (см. Implementation hints).

4. В `record/record.go` поправить устаревший комментарий (строки 205-207), который приводит
   `bgwriter` как пример NotRecordable-view. Сделать его generic: после фичи ни один
   production-view не ставит `NotRecordable: true`, drop-ветка остаётся ради механизма и его
   синтетического guard-теста. Саму drop-ветку (`if v.NotRecordable { ... }`) НЕ трогать.

5. НЕ менять `TestNew` (строка 11, остаётся `27`). НЕ трогать
   `TestFilterViews_dropsExplicitNotRecordable` (синтетический guard-тест drop-ветки).

## TDD Anchor

Это задача-«снятие флага + починка существующих тестов», новых тестов не пишем — корректность
изменения проверяется уже существующими тестами, чьи ожидания мы приводим в соответствие:

- `internal/view/view_test.go::TestNew_StatementsJITView` — `jit.NotRecordable == false`
- `internal/view/view_test.go::TestNew_StatIOView` — `statio.NotRecordable == false`
- `internal/view/view_test.go::TestNew_StatIOTimeView` — `statioTime.NotRecordable == false`
- `internal/view/view_test.go::TestNew_ReplslotsView` — `replslots.NotRecordable == false`
- `internal/view/view_test.go::TestNew_BgwriterView` — `bgwriter.NotRecordable == false`
- `internal/view/view_test.go::TestNew` — общее число view остаётся 27 (регресс-гард, не менять)
- `record/record_test.go::Test_filterViews` — PG14 даёт 9/18 (s="") и 3/24 (s="public");
  PG13/12/11/10 без изменений
- `record/record_test.go::TestFilterViews_dropsExplicitNotRecordable` — механизм drop-ветки всё
  ещё работает на синтетическом view (регресс-гард, не менять)

Порядок: сначала перевернуть assertion'ы и счётчики в тестах (тесты должны упасть на старом коде),
затем снять `NotRecordable: true` из view.go (тесты должны позеленеть).

## Acceptance Criteria

- [ ] `NotRecordable: true` удалён из всех 5 view-определений в `internal/view/view.go`
- [ ] 5 per-view assertion'ов в `view_test.go` перевёрнуты на `assert.False`, их комментарии
      больше не утверждают «excluded from recording»
- [ ] `Test_filterViews`: строки PG14 = 9/18 (s="") и 3/24 (s="public"); строки PG13/12/11/10
      без изменений; пояснительный комментарий переписан и фактически верен
- [ ] Устаревший комментарий в `record.go` (bgwriter как NotRecordable-пример) сделан generic;
      сама drop-ветка не тронута
- [ ] `TestNew` остаётся `27`; `TestFilterViews_dropsExplicitNotRecordable` не тронут
- [ ] `go test ./internal/view/... ./record/...` зелёный

## Context Files

**Feature artifacts:**
- [008-feat-record-report-0-11-views.md](008-feat-record-report-0-11-views.md) — user-spec
- [008-feat-record-report-0-11-views-tech-spec.md](008-feat-record-report-0-11-views-tech-spec.md) — tech-spec (Decisions §1, §6)
- [008-feat-record-report-0-11-views-decisions.md](008-feat-record-report-0-11-views-decisions.md) — decisions log (создаётся при выполнении)
- [008-feat-record-report-0-11-views-code-research.md](008-feat-record-report-0-11-views-code-research.md) — code research (§1, §2, §5)

**Project knowledge:**
- [overview.md](.claude/skills/project-knowledge/overview.md) — фичи, поддерживаемая статистика
- [architecture.md](.claude/skills/project-knowledge/architecture.md) — раскладка пакетов, data flow, обработка версий PG
- [patterns.md](.claude/skills/project-knowledge/patterns.md) — кодовые/тестовые паттерны, version branching

**Code files:**
- [internal/view/view.go](internal/view/view.go) — удалить 5 строк `NotRecordable: true` (151, 164, 178, 192, 242)
- [internal/view/view_test.go](internal/view/view_test.go) — перевернуть 5 assertion'ов (21, 40, 58, 75, 91) + комментарии (15, 34, 52, 70, 86)
- [record/record.go](record/record.go) — поправить комментарий 205-207; drop-ветку не трогать
- [record/record_test.go](record/record_test.go) — `Test_filterViews`: PG14 строки 127-128 + комментарий 108-126

## Verification Steps

- Запустить `go test ./internal/view/... ./record/...` — все тесты зелёные.
- Убедиться, что `TestNew` по-прежнему ожидает 27 view (не менялся).
- Убедиться, что `TestFilterViews_dropsExplicitNotRecordable` проходит (механизм drop-ветки жив).
- `grep -rn "NotRecordable: true" internal/view/view.go` — пусто (ни одного production-view).

## Details

**Files:**
- `internal/view/view.go` — текущее состояние: 5 view-блоков (`bgwriter` 140-152, `replslots`
  153-165, `stat_io` 166-179, `stat_io_time` 180-193, `statements_jit` 230-243) содержат
  `NotRecordable: true` последней строкой блока. Удалить ровно эти 5 строк. Остальные view
  (`statements_timings`, `statements_general`, `statements_io`, `statements_temp` и т.д.) не
  трогать — у них этого флага нет.
- `internal/view/view_test.go` — текущее состояние: 5 guard-тестов (`TestNew_StatementsJITView`
  и т.д.) ассертят `assert.True(t, X.NotRecordable)`. Их doc-комментарии содержат фразу
  «excluded from recording (NotRecordable)». `TestNew` (строка 11) ассертит `27` — НЕ менять.
- `record/record.go` — текущее состояние: `filterViews` (200-233) содержит drop-ветку
  `if v.NotRecordable {...}` (208-212) с комментарием выше (205-207), приводящим `bgwriter` как
  пример. Drop-ветку оставить, только обобщить комментарий.
- `record/record_test.go` — текущее состояние: `Test_filterViews` (101-140) с 6 testcases
  (127-132) и большим объяснительным комментарием (108-126). Менять только PG14 строки и
  комментарий.

**Dependencies:** нет (depends_on: []). Wave 1. Файлы этой задачи не пересекаются с задачами 2/3.

**Edge cases:**
- `statements_jit` имеет префикс `statements_`, поэтому при `pgssSchema == ""` он всё равно
  отбрасывается pgss-gate'ом (не version-gate, не NotRecordable). Поэтому в строке PG14 s="" он
  НЕ добавляется к kept — там добавляются только `bgwriter` + `replslots` (не statements-префикс).
  Это и даёт дельту +2 kept (16→18), а не +5.
- На PG14 `stat_io`/`stat_io_time` (V16) и `statements_jit` (V15) после снятия флага всё ещё
  отбрасываются, но теперь version-gate'ом (а jit — pgss-gate'ом при s=""), а не NotRecordable.
  Они остаются в `wantN` (filtered), просто меняется причина. Поэтому net по PG14 = -2/+2, а не
  -5/+5.
- PG13 и ниже: все 5 view version-gated (V14/V15/V16), отбрасываются version-gate'ом независимо
  от NotRecordable. Поэтому их строки в `Test_filterViews` НЕ меняются.

**Implementation hints:**
- Переписать комментарий 108-126 так, чтобы он отражал новое поведение: `bgwriter`/`replslots`
  (PostgresV14) стали recordable и на PG14 проходят в kept (wantV +2 → 18; wantN -2 → 9 для
  s=""); для s="public" они тоже kept (wantV 22→24, wantN 5→3). `stat_io`/`stat_io_time` (V16) и
  `statements_jit` (V15) остаются отброшенными на PG14, но теперь по version-gate / pgss-gate, а
  не по NotRecordable, — поэтому они по-прежнему в счётчике filtered и нижние строки PG<14 не
  меняются.
- Делать точечные правки через Edit; убрать ведущий tab+пробелы у удаляемой строки целиком (вместе
  с переводом строки), чтобы не остался пустой trailing-blank внутри struct-литерала.
- Не «улучшать» соседний код, форматирование или комментарии вне scope этой задачи.

## Reviewers

- **dev-code-reviewer** → `008-feat-record-report-0-11-views-task-01-dev-code-reviewer-review.json`
- **dev-test-reviewer** → `008-feat-record-report-0-11-views-task-01-dev-test-reviewer-review.json`

## Post-completion

- [ ] Записать краткий отчёт в [008-feat-record-report-0-11-views-decisions.md](008-feat-record-report-0-11-views-decisions.md) (Summary: 1-3 предложения, ревью со ссылками на JSON, без таблиц файндингов и дампов)
- [ ] Если отклонились от спека — описать отклонение и причину
- [ ] Обновить user-spec/tech-spec если что-то изменилось
