---
status: planned                    # planned -> in_progress -> done
depends_on: []                     # ID задач-зависимостей (строки: ["01", "02"])
wave: 1                            # волна параллельного выполнения
skills: [infrastructure-setup]     # МАССИВ скиллов для загрузки
verify: bash                       # собрать образ, поднять кластер PG 19 на 21919, залить fixtures
reviewers: [dev-code-reviewer, dev-security-auditor, dev-infrastructure-reviewer]
teammate_name:
---

# Task 01: PG 19 probe and test-image environment

## Required Skills

Перед выполнением задачи загрузи:
- `/skill:infrastructure-setup` — [skills/infrastructure-setup/SKILL.md](~/.claude/skills/infrastructure-setup/SKILL.md)

## Description

Это **probe-задача фичи 012 — retirement самого крупного риска, идёт ДО любого Go-кода.**
Её результат определяет судьбу всей фичи, поэтому она в Wave 1 в одиночку.

Что делаем: добавляем кластер PostgreSQL 19 в тестовый образ `lesovsky/pgcenter-testing`.
Бета-канал pgdg (`jammy-pgdg-testing`) подключается **отдельным** `.list`-файлом и **пиннится**
(Decision 5a), из него явно ставятся `postgresql-19` + `postgresql-plperl-19`, а в
`prepare-test-environment.sh` версия `19` добавляется в шесть циклов. Порт `21919` выводится из
номера версии автоматически (`port="219${v}"`) — отдельной правки портовой логики не требуется.

**Почему пин обязателен (Decision 5a).** apt выбирает пакет по наибольшей доступной версии среди
всех включённых источников, а не по приоритету источника. Если просто добавить бета-канал, бета-сборки
могут молча заменить уже установленные `postgresql-14..18` **и сам `postgresql-common`** — а это,
помимо порчи пяти существующих кластеров, обнуляет утверждение «`pg_createcluster` приезжает из
стабильного канала». Пин (`Pin-Priority: 100`) оставляет бета-канал достижимым только для тех пакетов,
которые запрошены из него явно (`apt-get install -t <beta-suite>`). Ключ переиспользуется существующий
(`/usr/share/keyrings/pgdg.gpg`, `signed-by=`) — тот же издатель, тот же хост, отдельный keyring
выигрыша не даёт. **`[trusted=yes]` использовать нельзя.**

**Развилка probe — исполнитель обязан явно доложить исход, а не молча выбрать ветку.**
Три ступени лестницы (user-spec «Ограничения», Risks в tech-spec):
1. PG 19 ставится на текущей базе `ubuntu:22.04` → **фича продолжается**, база не трогается, переезд
   на `ubuntu:24.04` остаётся отдельной задачей вне этой фичи;
2. пакетов под jammy нет → **миграция базового образа на `ubuntu:24.04` (noble) втягивается В эту
   фичу и блокирует её**; объём фичи пересматривается. Останови работу и доложи оркестратору/пользователю
   ДО того, как менять `FROM` — это изменение scope, а не деталь реализации;
3. пакетов нет и под noble → **фича целиком ставится на паузу**, релиз 0.12.0 идёт со следующими
   фичами roadmap ([013]–[015], ни одна от PG 19 не зависит). Урезанный вариант (код без живого
   кластера) отвергнут в user-spec.

**Классы отказов не путать.** «Пакетов нет в канале» — это probe failure, он запускает лестницу выше.
А вот «пакеты встали, но кластер не стартует» (например, GUC из `postgresql.auto.conf` удалён или
переименован в PG 19) и «`fixtures.sql` не заливается из-за семантики `plperlu`/CPAN-модулей» — это
**проблемы совместимости, разбираются здесь же по месту и probe failure НЕ считаются**: смена базового
образа их не починила бы. Третий отдельный режим отказа — «`pg_createcluster` не знает раскладку
PG 19»: `postgresql-common` приезжает из **основного** pgdg-канала и должен уже уметь PG 19; это
проверяется в этой же пробе.

**Собранный образ и поднятый кластер оставить работающими** — все последующие задачи фичи
верифицируются против этого локального кластера PG 19 на порту 21919.

**Явно вне scope (Decision 4):** `testing/e2e.sh` и теги контейнера в
`.github/workflows/{default,release}.yml`. Они правятся в задаче 9 и не должны попасть в мерж раньше,
чем образ опубликован, иначе CI станет красным без пути к skip. Push образа в DockerHub — тоже не здесь
(задача 8). Обновление документации (`deployment.md`, `testing/README.md`) — задача 7.

## What to do

1. **`testing/Dockerfile`** — подключить бета-канал pgdg отдельным source-файлом с `signed-by` на
   существующий keyring, положить рядом apt-preferences с `Pin-Priority: 100` на бета-suite, и только
   после этого поставить `postgresql-19` + `postgresql-plperl-19` **с явным `-t <beta-suite>`**.
   Пин и source-файл должны быть записаны в одном `RUN`-слое до `apt-get update`, иначе установка
   PG 19 пойдёт мимо пина. Обновить шапку-комментарий, `LABEL version`, строку в `CMD` — образ
   становится `0.0.11`, «PostgreSQL 14-19 on Ubuntu 22.04». Кэш apt по-прежнему подчищается в конце.

2. **`testing/prepare-test-environment.sh`** — добавить `19` в шесть циклов по версиям (создание
   кластеров, конфигурация, старт, ожидание готовности, заливка fixtures, финальная проверка).
   Ничего больше в скрипте не менять: порт `21919` и путь к datadir выводятся из `$v`.

3. **Собрать образ локально** и прогнать пробу: кластер PG 19 создаётся `pg_createcluster`, стартует,
   отвечает на 21919, `fixtures.sql` заливается без ошибок (включая `CREATE EXTENSION plperlu` и
   plperlu-функции на `Linux::Ethtool::Settings`).

4. **Доказать, что пин работает**: версии пакетов `postgresql-14..18` и `postgresql-common` в новом
   образе совпадают с версиями в образе `0.0.10`, а не подтянулись из бета-канала.

5. **Оставить контейнер запущенным** с поднятыми кластерами и доступным с хоста портом 21919 —
   он нужен задачам 2–10.

6. **Доложить исход пробы явным текстом** в отчёте задачи: какая ступень лестницы сработала, из какого
   канала и какой версии приехали пакеты PG 19, какая версия `postgresql-common`, и — если что-то
   сломалось — к какому классу отказа это отнесено (probe failure vs проблема совместимости).

## Acceptance Criteria

- [ ] Бета-канал объявлен **в собственном** файле в `/etc/apt/sources.list.d/` (не дописан в `pgdg.list`),
      с `signed-by=/usr/share/keyrings/pgdg.gpg`; `[trusted=yes]` отсутствует; новый keyring не заводится.
- [ ] Рядом лежит apt-preferences с `Pin-Priority: 100` на бета-suite; `apt-cache policy` подтверждает
      приоритет 100 для бета-источника.
- [ ] `postgresql-19` и `postgresql-plperl-19` устанавливаются с явным `-t <beta-suite>`.
- [ ] Версии `postgresql-14..18`, `postgresql-plperl-14..18` и `postgresql-common` в собранном образе
      **идентичны** версиям в `lesovsky/pgcenter-testing:0.0.10` (пин отработал).
- [ ] `LABEL version`, шапка-комментарий и строка `CMD` обновлены на `0.0.11` / «PostgreSQL 14-19».
- [ ] `prepare-test-environment.sh` содержит `19` во всех **шести** циклах по версиям; иной логики
      портов/путей не добавлено.
- [ ] Образ собирается локально; `pg_lsclusters` показывает шесть кластеров 14–19 в состоянии `online`.
- [ ] `pg_isready -p 21919 -d pgcenter_fixtures` отвечает; `SELECT version()` на 21919 показывает PG 19.
- [ ] `fixtures.sql` залит на все шесть кластеров без ошибок, включая `plperlu`-функции.
- [ ] Кластеры PG 14–18 стартуют и принимают соединения как раньше (регрессии в образе нет).
- [ ] Контейнер с поднятыми кластерами оставлен работающим и доступен для следующих задач.
- [ ] Исход пробы (ступень лестницы, версии пакетов, класс любого отказа) явно зафиксирован в отчёте.
- [ ] `testing/e2e.sh`, `.github/workflows/*.yml`, `testing/fixtures.sql` **не изменены**; образ в
      DockerHub **не пушится**.

## Context Files

**Feature artifacts:**
- [012-feat-pg19-compatibility-baseline.md](012-feat-pg19-compatibility-baseline.md) — user-spec
  (раздел «Ограничения» — трёхступенчатая лестница probe)
- [012-feat-pg19-compatibility-baseline-tech-spec.md](012-feat-pg19-compatibility-baseline-tech-spec.md)
  — tech-spec: Task 1, Decision 5a (пин бета-канала), Decision 4 (порядок мержа e2e/workflows),
  таблица Risks (классы отказов пробы)
- [012-feat-pg19-compatibility-baseline-decisions.md](012-feat-pg19-compatibility-baseline-decisions.md)
  — decisions log (создаётся в Post-completion)

**Project knowledge:**
- [overview.md](../../../.claude/skills/project-knowledge/overview.md) — что за проект, какие статистики поддерживаются
- [architecture.md](../../../.claude/skills/project-knowledge/architecture.md) — раскладка пакетов, работа с версиями PG
- [deployment.md](../../../.claude/skills/project-knowledge/deployment.md) — тестовый контейнер: текущий тег `0.0.10`,
  карта портов, порядок пересборки и публикации, где прописан тег в workflows

**Code files:**
- [testing/Dockerfile](../../../testing/Dockerfile) — ubuntu:22.04, `LABEL version="0.0.10"`, один `RUN`
  с apt-ключом pgdg, `pgdg.list`, установкой `postgresql-14..18` + `postgresql-plperl-14..18`,
  cpanm-модулями и очисткой apt-кэша; в конце `COPY` скрипта и fixtures + `CMD` с echo. **Изменить:**
  добавить пиннутый бета-канал и пакеты PG 19, обновить версию образа.
- [testing/prepare-test-environment.sh](../../../testing/prepare-test-environment.sh) — `set -e`, шесть
  циклов `for v in 14 15 16 17 18` (создание кластеров, дозапись `postgresql.auto.conf`, `pg_hba.conf`,
  старт, ожидание `pg_isready`, заливка fixtures, финальная проверка); порт считается как `219${v}`.
  **Изменить:** добавить `19` во все шесть циклов.
- [testing/fixtures.sql](../../../testing/fixtures.sql) — **только читать.** Создаёт БД
  `pgcenter_fixtures` / `pgcenter_fixtures_config`, `CREATE EXTENSION pg_stat_statements`,
  `CREATE EXTENSION plperlu`, схему `pgcenter` и четыре `plperlu`-функции (одна использует
  CPAN-модуль `Linux::Ethtool::Settings`). Главный кандидат на отказ класса «совместимость».
- [testing/e2e.sh](../../../testing/e2e.sh) — **не трогать** (Decision 4, задача 9).

## Verification Steps

- Сборка: `docker build -t lesovsky/pgcenter-testing:0.0.11 testing/` — успешна; в логе видно, что
  PG 19 ставится из бета-suite, а `postgresql-14..18` **не переустанавливаются и не апгрейдятся**.
- Пин: в контейнере `apt-cache policy` показывает бета-источник с приоритетом 100;
  `apt-cache policy postgresql-18 postgresql-common` — установленная версия из основного канала.
- Сравнение версий: `dpkg -l 'postgresql-*'` в новом образе и в `lesovsky/pgcenter-testing:0.0.10` —
  строки по 14–18 и `postgresql-common` совпадают.
- Запуск: контейнер поднят, внутри выполнен `/usr/local/bin/prepare-test-environment.sh` — скрипт
  отработал до конца без ошибок (`set -e`).
- Кластеры: `pg_lsclusters` — шесть строк 14–19, все `online`, порты 21914–21919.
- PG 19: `psql -h 127.0.0.1 -p 21919 -U postgres -c 'select version()'` — PostgreSQL 19 beta.
- Fixtures: `psql -h 127.0.0.1 -p 21919 -U postgres -d pgcenter_fixtures -c '\dx'` — есть
  `pg_stat_statements` и `plperlu`; `\df pgcenter.*` — четыре функции на месте.
- Доступность с хоста: `pg_isready -h 127.0.0.1 -p 21919` (или проверка портов 21914–21919) — нужна
  задачам 2–10, которые гоняют `go test` с хоста.
- Регрессия: `pg_isready` на 21914–21918 отвечает, fixtures на них залиты.
- Отчёт: в тексте задачи явно назван исход пробы и, при отказе, его класс.

## Details

**Files:**
- `testing/Dockerfile` — сейчас один большой `RUN`: `apt-get update` → базовые пакеты (locales, curl,
  gnupg, make, gcc, git, perl/cpanminus, ssl) → `locale-gen` → скачивание ключа `ACCC4CF8.asc` в
  `/usr/share/keyrings/pgdg.gpg` → `pgdg.list` с `jammy-pgdg main` → `apt-get update` → установка пяти
  пар `postgresql-N` / `postgresql-plperl-N` → `cpanm Module::Build`, `cpanm Linux::Ethtool::Settings`
  → `mkdir /usr/local/testing/` → `rm -rf /var/lib/apt/lists/*`. Ниже — `COPY prepare-test-environment.sh`,
  `COPY fixtures.sql`, `CMD echo`. **Что сделать:** внутри того же `RUN` (или соседнего, но до установки
  PG 19) создать второй source-файл на бета-suite и файл apt-preferences с пином, затем `apt-get update`
  и `apt-get install -y -t <beta-suite> postgresql-19 postgresql-plperl-19`. Обновить строку 2
  (`PostgreSQL 14-19`), `LABEL version="0.0.11"` и текст в `CMD`.
- `testing/prepare-test-environment.sh` — шесть мест с `for v in 14 15 16 17 18`. **Что сделать:**
  дописать `19` в каждый. Больше ничего: `port="219${v}"` даёт 21919, `datadir=/var/lib/postgresql/19/main`
  и `log_filename='postgresql-19.log'` тоже выводятся из `$v`.

**Dependencies:** зависимостей от других задач нет (`depends_on: []`) — эта задача первая по порядку и
блокирует Wave 2+. Внешне: доступность бета-канала pgdg (`*-pgdg-testing`) для jammy, наличие
`postgresql-common` с поддержкой PG 19 в основном канале, docker на машине исполнителя.

**Edge cases:**
- **Имя бета-suite.** Ожидается `jammy-pgdg-testing`; точное имя (и то, что в нём есть PG 19)
  проверить фактически — например, посмотреть `dists/` на `apt.postgresql.org` или
  `apt-cache policy` после добавления источника. Пин по `n=`/`a=` должен совпадать с реальным
  значением из `apt-cache policy`, иначе пин не применится и молча превратится в «канал без пина» —
  это самый вероятный тихий провал этой задачи.
- **Пин не сработал.** Симптом: в логе сборки `postgresql-14..18` или `postgresql-common` идут на
  апгрейд. Это нарушение AC, а не «ну и ладно» — чинить пин, а не принимать результат.
- **`pg_createcluster` не знает PG 19** — отдельный класс отказа: значит `postgresql-common` в основном
  канале ещё не догнал релиз. Зафиксировать явно; вариант обхода (взять `postgresql-common` из
  бета-канала тем же `-t`) допустим, но это отклонение от Decision 5a — согласовать и записать.
- **Кластер не стартует.** Скорее всего GUC из `postgresql.auto.conf` (`shared_preload_libraries`,
  `wal_level`, `track_*`, ssl-параметры) удалён/переименован в PG 19. Смотреть
  `/var/log/postgresql/postgresql-19.log`. Это проблема совместимости — правится здесь (при
  необходимости — версионной веткой в конфиг-цикле), лестницу probe **не** запускает.
- **`fixtures.sql` падает.** Наиболее вероятно на `CREATE EXTENSION plperlu` (пакет
  `postgresql-plperl-19` собран под другую сборку perl) или на функции с `Linux::Ethtool::Settings`.
  Тоже класс «совместимость»: разбираться по месту, не считать провалом пробы.
- **`set -e` в скрипте** — любая ошибка на любом кластере обрывает подготовку окружения целиком,
  включая уже поднятые 14–18. При отладке запускать скрипт вручную внутри контейнера, чтобы видеть,
  на каком именно цикле упало.
- **Бета-канал живёт до GA** — это осознанный временный долг: при каждой пересборке PG 19 будет
  приезжать из беты. Удаление канала на GA-пересборке уже записано как deferred item в tech-spec,
  дублировать его в код не нужно.

**Implementation hints:**
- Ключ pgdg уже лежит в образе (`/usr/share/keyrings/pgdg.gpg`) — переиспользовать его в `signed-by=`
  для бета-источника; новый ключ не скачивать.
- Пин-файл — обычный apt-preferences в `/etc/apt/preferences.d/`: три поля (`Package: *`,
  `Pin: release <suite-match>`, `Pin-Priority: 100`). 100 = «ниже, чем уже установленное», поэтому
  апгрейд существующих пакетов из этого источника не произойдёт, а явный `-t` установку разрешает.
- Порядок внутри `RUN` важен: source-файл + preferences → `apt-get update` → `install -t`. Если
  поставить PG 19 до появления пин-файла, пин ничего не защитит.
- Чтобы образ остался воспроизводимым, `rm -rf /var/lib/apt/lists/*` должен остаться последним шагом
  слоя, как сейчас.
- Локальный прогон удобно делать так: поднять контейнер в фоне долгоживущей командой, затем
  `docker exec` запустить `/usr/local/bin/prepare-test-environment.sh` (штатный `CMD` — просто `echo`,
  сам по себе кластеры он не поднимает). Порты 21914–21919 должны быть достижимы с хоста — либо
  проброс портов, либо host-сеть; это требование следующих задач, которые гоняют `go test` с хоста.
- Сравнение версий пакетов до/после проще всего снять из обоих образов одной командой
  `dpkg -l 'postgresql*'` и сдиффить вывод.
- НЕ править `testing/e2e.sh` и `container:` в workflows — Decision 4, задача 9. НЕ пушить образ —
  задача 8. НЕ обновлять `deployment.md` / `testing/README.md` — задача 7.

## Reviewers

- **dev-code-reviewer** → `012-feat-pg19-compatibility-baseline-task-01-dev-code-reviewer-review.json`
- **dev-security-auditor** → `012-feat-pg19-compatibility-baseline-task-01-dev-security-auditor-review.json`
- **dev-infrastructure-reviewer** → `012-feat-pg19-compatibility-baseline-task-01-dev-infrastructure-reviewer-review.json`

## Post-completion

- [ ] Записать краткий отчёт в [012-feat-pg19-compatibility-baseline-decisions.md](012-feat-pg19-compatibility-baseline-decisions.md) (Summary: 1-3 предложения, ревью со ссылками на JSON, без таблиц файндингов и дампов)
- [ ] **Явно зафиксировать исход пробы**: ступень лестницы, версии PG 19 и `postgresql-common`, имя
      бета-suite, класс любого встреченного отказа
- [ ] Если отклонились от спека — описать отклонение и причину
- [ ] Обновить user-spec/tech-spec если что-то изменилось (в частности — если сработала ступень 2 или 3
      лестницы, объём фичи пересматривается)
