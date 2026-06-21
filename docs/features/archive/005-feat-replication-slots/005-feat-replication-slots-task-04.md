---
status: planned                    # planned -> in_progress -> done
depends_on: []                     # ID задач-зависимостей (строки: ["01", "02"])
wave: 1                            # волна параллельного выполнения
skills: [deploy-pipeline]          # МАССИВ скиллов для загрузки
verify: bash                       # инструмент верификации (опционально: curl, bash, user)
reviewers: [dev-deploy-reviewer, dev-code-reviewer]  # явно указать. Пусто = fallback на defaults
teammate_name:                     # имя агента-исполнителя (опционально; если не задано — генерируется по описанию задачи)
---

# Task 04: Test-image bump for logical-slot support

## Required Skills

Перед выполнением задачи загрузи:
- `/skill:deploy-pipeline` — [skills/deploy-pipeline/SKILL.md](~/.claude/skills/deploy-pipeline/SKILL.md)

## Description

Эта задача готовит тестовую инфраструктуру под интеграционные тесты логических
replication-слотов (task-05). Сейчас все кластеры в тестовом образе
`lesovsky/pgcenter-testing` стартуют с дефолтным `wal_level=replica`, при котором
нельзя создать логический слот (`pg_create_logical_replication_slot`). Нужно включить
`wal_level=logical` в скрипте подготовки окружения и поднять версию тестового образа
`0.0.9 → 0.0.10`, чтобы CI начал тянуть новый образ с включённым логическим WAL.

Важно: сборка и публикация Docker-образа `lesovsky/pgcenter-testing:0.0.10` —
**ручной шаг мейнтейнера** (нужны DockerHub-креды, см. deployment.md). CI-джобы для
сборки тестового образа нет. Агент меняет только файлы в репозитории; реальный
`docker build`/`docker push` выполняет человек.

Связь с остальной фичей слабая (decoupled): тест логического слота в task-05
защитный — он делает `t.Skipf`, пока `wal_level != logical`. Поэтому пуш образа и
мердж кода можно делать в любом порядке: CI остаётся зелёным на старом образе
(`wal_level=replica`), а тест начинает реально проверять логический слот только после
того, как образ `0.0.10` опубликован и тег в workflow-файлах поднят.

## What to do

1. В `testing/prepare-test-environment.sh` добавить `wal_level = logical` в heredoc,
   который пишет per-cluster параметры в `postgresql.auto.conf` (блок между строками 18–31).
   Рядом поставить inline-комментарий, помечающий параметр как test-only, чтобы он не
   попал в пользовательские примеры конфигурации.
2. В `testing/Dockerfile` поднять ОБА литерала версии `0.0.9 → 0.0.10`: и `LABEL version="0.0.9"`
   (строка 6), и строку echo в `CMD ["echo", "pgcenter-testing 0.0.9: ..."]` (строка 38).
3. В `testing/Dockerfile` исправить устаревший комментарий-заголовок в строке 2:
   `PostgreSQL 14-17` → `PostgreSQL 14-18` (образ давно содержит PG18).
4. В `.github/workflows/default.yml` (строка 9) обновить тег контейнера
   `lesovsky/pgcenter-testing:0.0.9` → `:0.0.10`.
5. В `.github/workflows/release.yml` (строка 11) обновить тег контейнера
   `lesovsky/pgcenter-testing:0.0.9` → `:0.0.10`.
6. Задокументировать ручной шаг мейнтейнера (см. раздел Verification Steps): после мерджа
   мейнтейнер выполняет `docker build -t lesovsky/pgcenter-testing:0.0.10 testing/` и
   `docker push lesovsky/pgcenter-testing:0.0.10`. Это НЕ действие агента.

## Acceptance Criteria

- [ ] `testing/prepare-test-environment.sh`: в heredoc auto.conf добавлен `wal_level = logical`
      с inline-комментарием, помечающим параметр как test-only; скрипт остаётся валидным bash
      (`bash -n` без ошибок).
- [ ] `testing/Dockerfile`: оба литерала `0.0.9` (LABEL + CMD echo) заменены на `0.0.10`;
      больше ни одного `0.0.9` в файле не осталось.
- [ ] `testing/Dockerfile`: комментарий в строке 2 — `PostgreSQL 14-18` (а не `14-17`).
- [ ] `.github/workflows/default.yml`: `container:` ссылается на `lesovsky/pgcenter-testing:0.0.10`.
- [ ] `.github/workflows/release.yml`: `container:` (job `test`) ссылается на `lesovsky/pgcenter-testing:0.0.10`.
- [ ] Оба workflow-файла — валидный YAML; `make build` проходит (sanity-check, что репозиторий цел).
- [ ] Ручной шаг мейнтейнера (`docker build`/`docker push` образа `0.0.10`) явно задокументирован
      в задаче как не-агентское действие.

## Context Files

**Feature artifacts:**
- [005-feat-replication-slots.md](005-feat-replication-slots.md) — user-spec
- [005-feat-replication-slots-tech-spec.md](005-feat-replication-slots-tech-spec.md) — tech-spec (Task 4, Decision 5, Risks)
- [005-feat-replication-slots-decisions.md](005-feat-replication-slots-decisions.md) — decisions log

**Project knowledge:**
- [overview.md](.claude/skills/project-knowledge/overview.md) — обзор проекта (project.md отсутствует; актуальный обзор — overview.md)
- [architecture.md](.claude/skills/project-knowledge/architecture.md) — раскладка пакетов, PG-version handling
- [deployment.md](.claude/skills/project-knowledge/deployment.md) — релизный процесс, CI/CD, тестовый образ, ручная сборка/пуш образа

**Code files:**
- [testing/prepare-test-environment.sh](testing/prepare-test-environment.sh) — добавить `wal_level = logical` в auto.conf heredoc
- [testing/Dockerfile](testing/Dockerfile) — бамп версии 0.0.9→0.0.10 (×2) + фикс комментария 14-17→14-18
- [.github/workflows/default.yml](.github/workflows/default.yml) — тег контейнера →0.0.10
- [.github/workflows/release.yml](.github/workflows/release.yml) — тег контейнера →0.0.10

## Verification Steps

- `bash -n testing/prepare-test-environment.sh` — скрипт парсится без ошибок.
- `grep -n "wal_level" testing/prepare-test-environment.sh` — параметр присутствует внутри heredoc auto.conf, с test-only комментарием.
- `grep -n "0.0.9" testing/Dockerfile` — пусто (ни одного оставшегося литерала).
- `grep -n "0.0.10" testing/Dockerfile` — две строки (LABEL + CMD echo).
- `grep -n "14-18" testing/Dockerfile` — присутствует; `grep -n "14-17" testing/Dockerfile` — пусто.
- `grep -rn "pgcenter-testing:0.0" .github/workflows/` — обе ссылки указывают на `:0.0.10`.
- `make build` — проходит без ошибок (sanity-check целостности репозитория).
- **Ручной шаг мейнтейнера (НЕ агент):** мейнтейнер с DockerHub-кредами выполняет
  `docker build -t lesovsky/pgcenter-testing:0.0.10 testing/` и
  `docker push lesovsky/pgcenter-testing:0.0.10`. После пуша CI на следующем прогоне
  тянет образ `0.0.10`, тесты task-05 перестают скипаться. Без пуша CI остаётся зелёным
  на старом образе через защитный `t.Skipf` (Decision 5).

## Details

**Files:**

- `testing/prepare-test-environment.sh` — heredoc на строках 18–31 пишет per-cluster параметры
  в `${datadir}/postgresql.auto.conf`. Добавить строку `wal_level = logical` внутрь этого
  heredoc (например, после `shared_preload_libraries = 'pg_stat_statements'`), с inline-комментарием
  вида `# test-only: enables logical replication slots, do not copy into user configs`.
  `wal_level = logical` включает запись логического WAL для всех 5 кластеров (PG14–18). Это
  единственное содержательное изменение скрипта; остальная логика (createcluster, start,
  fixtures) не трогается.

- `testing/Dockerfile` — три точечные правки:
  - строка 2: `# PostgreSQL 14-17 on Ubuntu 22.04` → `# PostgreSQL 14-18 on Ubuntu 22.04`;
  - строка 6: `LABEL version="0.0.9"` → `LABEL version="0.0.10"`;
  - строка 38: `CMD ["echo", "pgcenter-testing 0.0.9: PostgreSQL 14-18 on Ubuntu 22.04"]`
    → `... pgcenter-testing 0.0.10: ...`.
  Содержимое образа (apt-пакеты, COPY) НЕ меняется — только метаданные версии и комментарий.

- `.github/workflows/default.yml` — строка 9: `container: lesovsky/pgcenter-testing:0.0.9`
  → `:0.0.10`.

- `.github/workflows/release.yml` — строка 11 (внутри job `test`): тот же бамп `:0.0.9` → `:0.0.10`.
  Job `release` (goreleaser, login-action) НЕ трогается — он публикует продуктовый образ
  `lesovsky/pgcenter`, а не тестовый.

**Dependencies:** Нет зависимостей от других задач (`depends_on: []`, wave 1). Decision 5
делает эту задачу независимой по времени от task-05 (защитный `t.Skipf`). Внешняя зависимость —
ручной пуш образа мейнтейнером (вне CI).

**Edge cases:**
- Не оставить ни одного `0.0.9` в Dockerfile — оба литерала (LABEL и CMD) должны быть подняты,
  иначе образ будет иметь рассинхронизированную версию.
- `wal_level` добавлять ИМЕННО внутрь heredoc (между `<< EOF` и `EOF`), а не в код скрипта,
  иначе он не попадёт в auto.conf.
- Тег образа в release.yml поднимать только в job `test` (строка 11); job `release` к тестовому
  образу отношения не имеет.
- До публикации образа CI будет требовать `0.0.10`, которого может ещё не быть в registry.
  Предпочтительный порядок: мейнтейнер пушит образ → затем мерджится бамп тега. Защитный
  `t.Skipf` (Decision 5) снимает жёсткую зависимость на стороне тестов, но сам тег контейнера
  должен указывать на существующий образ к моменту прогона CI.

**Implementation hints:**
- См. `deployment.md` («To rebuild and push») — там зафиксирована точная пара команд
  `docker build`/`docker push` и заметка «Then update container tag in both default.yml and
  release.yml», что в точности соответствует этой задаче.
- После правок прогнать `grep`-проверки из Verification Steps как быстрый self-check.
- `make build` тут не покрывает изменения напрямую (это shell/Docker/YAML), но служит
  sanity-check'ом, что репозиторий компилируется и ничего не сломано побочно.

## Reviewers

- **dev-deploy-reviewer** → `005-feat-replication-slots-task-04-dev-deploy-reviewer-review.json`
- **dev-code-reviewer** → `005-feat-replication-slots-task-04-dev-code-reviewer-review.json`

## Post-completion

- [ ] Записать краткий отчёт в [005-feat-replication-slots-decisions.md](005-feat-replication-slots-decisions.md) (Summary: 1-3 предложения, ревью со ссылками на JSON, без таблиц файндингов и дампов)
- [ ] Отметить, что публикация образа `0.0.10` — отдельный ручной шаг мейнтейнера (статус: pending/done)
- [ ] Если отклонились от спека — описать отклонение и причину
- [ ] Обновить deployment.md, если упоминание версии тестового образа (`Current version: 0.0.9`) стало неактуальным
