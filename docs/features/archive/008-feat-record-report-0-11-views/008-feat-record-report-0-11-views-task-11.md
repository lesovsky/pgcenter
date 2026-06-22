---
status: done                       # planned -> in_progress -> done
depends_on: ["01", "02", "03", "04", "05", "06", "07", "08", "09", "10"]
wave: 4
skills: [pre-deploy-qa]
verify: bash — `make test && make lint && make vuln`
reviewers: []                      # QA is its own verification — no reviewers
teammate_name:
---

# Task 11: Pre-deploy QA

## Required Skills

Перед выполнением задачи загрузи:
- `/skill:pre-deploy-qa` — [skills/pre-deploy-qa/SKILL.md](~/.claude/skills/pre-deploy-qa/SKILL.md)

## Description

Финальная волна — приёмочное тестирование всей фичи record/report для четырёх экранов
0.11.0 (bgwriter, replslots, pg_stat_io count/time, statements_jit). Все задачи реализации
(Wave 1–3) уже выполнены: снят `NotRecordable`, добавлены CLI-флаги и describe-строки,
написаны golden replay-тесты, выплачен техдолг [007]/[004], обновлены доки.

Эта задача — последний гейт перед мержем. Она прогоняет полный набор проверок качества
(`make test` с race+coverage, `make lint` = golangci-lint+gosec, `make vuln` = govulncheck),
сверяет все критерии приёмки из user-spec и tech-spec, и закрывает шов «живая запись ↔ живое
проигрывание», который сознательно оставлен за пределами автоматических тестов (Decision 3:
такого e2e-харнесса в проекте нет). Шов закрывается ручной проверкой против работающего
PostgreSQL: `pgcenter record`, затем `pgcenter report -B/-L/-J c|t/-X j` и `-d`, со сравнением
дельт одного кумулятивного экрана с тем, что показывает TUI на тех же данных. Также
подтверждается, что CI-матрица PG14–18 зелёная.

Это QA-задача — она ничего не пишет в код. Если найдены дефекты, они фиксируются как находки
(severity + issue + fix) и возвращаются оркестратору для фикс-цикла, а не правятся здесь.

## What to do

- Прочитать user-spec, tech-spec и decisions.md; собрать сводный чек-лист всех критериев
  приёмки (пользовательских + технических) и отклонений, зафиксированных предыдущими задачами.
- Прогнать полный гейт качества: `make test` (race + coverage), `make lint`
  (golangci-lint + gosec), `make vuln` (govulncheck) — все должны быть зелёными.
- Собрать бинарь (`make build`) и выполнить ручную проверку против живого PostgreSQL:
  записать архив `pgcenter record`, проиграть его через `report -B/-L/-J c/-J t/-X j` и
  `report -d` для каждого флага, проверить старый архив (только заголовок).
- Закрыть шов «запись ↔ проигрывание»: для одного кумулятивного экрана (bgwriter или
  stat_io) сравнить дельты живого отчёта со значениями TUI на тех же данных.
- Подтвердить, что последний CI-прогон ветки фичи на матрице PG14–18 зелёный.
- Свести результат в QA-вердикт: PASS либо структурированный список находок для фикс-цикла.

## Acceptance Criteria

- [ ] `make test` зелёный — race detector + coverage, включая обновлённые тесты фильтрации
      (`Test_filterViews`, `view_test`, `TestNew`=27), новые golden replay-тесты пяти типов
      отчётов (включая версионные варианты), CLI/describe unit-кейсы и поведенческий
      zero-cell diff-тест [007].
- [ ] `make lint` зелёный — golangci-lint + gosec без замечаний.
- [ ] `make vuln` зелёный — govulncheck чистый.
- [ ] Все пользовательские критерии приёмки из user-spec (§ «Критерии приёмки») подтверждены.
- [ ] Все технические критерии приёмки из tech-spec (§ Acceptance Criteria) подтверждены.
- [ ] Ручная проверка против живого PG выполнена: `pgcenter record` пишет архив, затем
      `pgcenter report -B`, `-L`, `-J c`, `-J t`, `-X j` печатают отчёты с корректными
      колонками и дельтами; `report -d` для каждого флага печатает описание колонок; старый
      архив без новых данных печатает только заголовок без ошибок.
- [ ] Шов «запись ↔ проигрывание» закрыт: для хотя бы одного кумулятивного экрана (bgwriter
      или stat_io) значения живого отчёта сходятся с тем, что показывает TUI на тех же данных.
- [ ] CI-матрица PG14–18 зелёная.
- [ ] Сформирован QA-вердикт: PASS (всё зелёное, шов закрыт) или список находок (severity,
      issue, fix) для фикс-цикла.

## Context Files

**Feature artifacts:**
- [008-feat-record-report-0-11-views.md](docs/features/008-feat-record-report-0-11-views/008-feat-record-report-0-11-views.md) — user-spec (критерии приёмки, раздел «Как проверить»)
- [008-feat-record-report-0-11-views-tech-spec.md](docs/features/008-feat-record-report-0-11-views/008-feat-record-report-0-11-views-tech-spec.md) — tech-spec (Acceptance Criteria, Agent Verification Plan, Decision 3)
- [008-feat-record-report-0-11-views-decisions.md](docs/features/008-feat-record-report-0-11-views/008-feat-record-report-0-11-views-decisions.md) — decisions log (отклонения предыдущих задач)

**Project knowledge:**
- [overview.md](.claude/skills/project-knowledge/overview.md) — фичи, поддерживаемые статистики
- [architecture.md](.claude/skills/project-knowledge/architecture.md) — раскладка пакетов, поток данных, обработка версий PG
- [patterns.md](.claude/skills/project-knowledge/patterns.md) — конвенции тестирования, version branching, Git workflow
- [deployment.md](.claude/skills/project-knowledge/deployment.md) — процесс релиза, CI/CD, матрица PG14–18

## Verification Steps

1. Прочитать user-spec, tech-spec и decisions.md; составить чек-лист всех критериев приёмки
   (пользовательских + технических).
2. Среда: это Go CLI без docker-окружения для самого приложения — фаза Docker-rebuild из
   скилла пропускается; гейт — нативные `make`-таргеты.
3. Прогнать `make test` — ожидание: все тесты зелёные (race + coverage), включая
   обновлённые тесты фильтрации, пять групп golden replay-тестов и zero-cell diff-тест.
4. Прогнать `make lint` — ожидание: golangci-lint + gosec без замечаний.
5. Прогнать `make vuln` — ожидание: govulncheck чистый.
6. Собрать бинарь (`make build`) для ручной проверки.
7. Ручная проверка против живого PostgreSQL (шов записи↔проигрывания, Decision 3):
   - `pgcenter record -f /tmp/s.tar` на работающем PG (дать ему собрать ≥2 тика);
   - `pgcenter report -f /tmp/s.tar -B`, `-L`, `-J c`, `-J t`, `-X j` — отчёты печатаются
     с корректными колонками и дельтами;
   - `pgcenter report -d -B` (и для остальных флагов) — печатается описание колонок;
   - для одного кумулятивного экрана (bgwriter или stat_io) сравнить дельты живого отчёта
     с тем, что показывает TUI на тех же данных — значения должны сходиться;
   - проверить старый архив без новых entry — отчёт печатает только заголовок, без ошибок.
8. Подтвердить, что CI-прогон на матрице PG14–18 зелёный (последний прогон ветки фичи).
9. Сверить каждый критерий приёмки с фактическим результатом; зафиксировать вердикт.
10. Если есть дефекты — вернуть находки (severity / issue / fix), не править код здесь.

## Details

<!-- All details for task execution — technical, organizational, any other. -->

**Files:** ничего не модифицируется — это приёмочная проверка. Код, тесты и доки уже
поставлены задачами 01–10.

**Dependencies:** все задачи 01–10 должны быть `done` (depends_on). Без них гейт неполон.

**Edge cases (что специально проверить руками, т.к. не покрыто автотестами):**
- Шов «живая запись ↔ живое проигрывание» — единственное, что не покрыто golden-тестами
  (Decision 3 / tech-spec E2E: «None»). Это главная цель ручной части.
- Старый архив (записан до фичи) для новых флагов → только заголовок, без падения и без
  лишних INFO/WARNING (Decision 5).
- Пустые данные: нет слотов у replslots; `jit_functions=0` у JIT → только заголовок.
- Версионная корректность: ручная проверка идёт на одной живой версии PG; версионную
  раскладку (14/17/18 и т.д.) подтверждают golden-тесты + CI-матрица PG14–18.

**Implementation hints (НЕ псевдокод):**
- Это Go CLI без сетевой поверхности и без docker-compose для самого приложения — фазу
  «Environment Rebuild» из pre-deploy-qa скилла пропустить, гейтом считать `make` таргеты
  из .claude/CLAUDE.md (`make build/test/lint/vuln`).
- Если `make test` падает на счётчиках фильтрации — это типичный регресс задачи 1 (сдвиг
  per-version counts при снятии NotRecordable); зафиксировать как находку, не чинить.
- Для ручной проверки нужен доступный живой PostgreSQL; версия определяет, какие из пяти
  экранов запишутся (bgwriter/replslots — PG14+, statements_jit — PG15+, stat_io/stat_io_time
  — PG16+). Указать в вердикте, на какой версии PG прогонялась ручная часть.
- TUI-сравнение: запустить `pgcenter` интерактивно, открыть соответствующий экран
  (bgwriter / pg_stat_io) и сопоставить поинтервальные дельты с теми, что печатает report
  на архиве, снятом в то же окно.
- Находки возвращать структурно: `severity` (critical/major/minor), `issue`, `fix`.

## Reviewers

Нет. QA — самостоятельная форма верификации (reviewers: []). Результат — QA-вердикт,
а не код на ревью.

## Post-completion

- [ ] Записать краткий QA-отчёт в [008-feat-record-report-0-11-views-decisions.md](docs/features/008-feat-record-report-0-11-views/008-feat-record-report-0-11-views-decisions.md): итог гейта (test/lint/vuln), результат ручной проверки шва запись↔проигрывание (версия PG, какой экран сверяли с TUI), статус CI-матрицы PG14–18, финальный вердикт (PASS / список находок)
- [ ] Если найдены дефекты — вернуть находки (severity / issue / fix) оркестратору для фикс-цикла; не править код в этой задаче
- [ ] Если выявлены отклонения от спека — описать отклонение и причину
