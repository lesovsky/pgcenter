---
status: planned                    # planned -> in_progress -> done
depends_on: ["08"]                 # ID задач-зависимостей (строки: ["01", "02"])
wave: 6                            # волна параллельного выполнения
skills: [deploy-pipeline]          # МАССИВ скиллов для загрузки
verify: bash                       # инструмент верификации (опционально: curl, bash, user)
reviewers: [dev-code-reviewer, dev-security-auditor, dev-deploy-reviewer]  # явно указать. Пусто = fallback на defaults
teammate_name:                     # имя агента-исполнителя (опционально; если не задано — генерируется по описанию задачи)
---

# Task 09: Switch CI to the new image and extend the e2e script

## Required Skills

Перед выполнением задачи загрузи:
- `/skill:deploy-pipeline` — [skills/deploy-pipeline/SKILL.md](~/.claude/skills/deploy-pipeline/SKILL.md)

## Description

Последний инфраструктурный шаг фичи: перевести оба GitHub Actions workflow на новый тег тестового
образа (`lesovsky/pgcenter-testing:0.0.10` → `:0.0.11`) и добавить порт PG 19 (21919) в e2e-скрипт.
Обе правки едут одним изменением и **только после** того, как мейнтейнер опубликовал образ (Task 08).

Почему порядок жёсткий, а не косметический — Decision 4 tech-spec'а:

- **`prepare-test-environment.sh` CI запускает из копии, ЗАПЕЧЁННОЙ В ОБРАЗ** (`COPY prepare-test-environment.sh
  /usr/local/bin/`, шаг workflow вызывает просто `prepare-test-environment.sh` из PATH). Поэтому правка
  этого скрипта, сделанная в Task 01, инертна в CI до тех пор, пока образ не опубликован и тег не поднят —
  она физически не может ничего сломать заранее.
- **`e2e.sh` CI запускает ИЗ ЧЕКАУТА** (`run: ./testing/e2e.sh`). Скрипт работает под `set -euxo pipefail`
  и не имеет никакого механизма пропуска: `pgcenter record -p 21919` против несуществующего кластера
  завершится ненулевым кодом и уронит весь прогон. Значит правка e2e начинает действовать немедленно,
  в том же прогоне, где смёрджена.

Отсюда и отличие от прецедента проекта: ADR [005] развязал ручной пуш образа и мёрдж кода защитными
`t.Skipf` — для Go-тестов, карты портов и версионных циклов это по-прежнему работает (они чисто скипаются
на старом образе). На `e2e.sh` это не переносится, поэтому правка e2e и оба бампа тега приземляются вместе,
после публикации образа, а не по обычной схеме расцепления.

В e2e-скрипте **два независимых цикла по портам** — `record` и `report`; порт 21919 нужен в обоих.
Существующий набор аргументов отчёта уже включает `-Pv -Pa -Pb`, поэтому новые колонки PG 19
(`started_by`, `mode`, `backup_type`) получают сквозной прогон `record` → `report` без единого нового
аргумента и без нового скрипта.

Это же первая точка, где **весь Go-набор тестов прогоняется против опубликованного образа**, а не против
локально собранного кластера: все предыдущие задачи фичи верифицировались на кластере, который Task 01
поднял локально. Расхождение между локальной сборкой и опубликованным образом (например, бета-пакеты PG 19
съехали между сборками) вскроется именно здесь.

## What to do

- Убедиться, что образ с новым тегом реально опубликован в DockerHub (Task 08 закрыта). Если образа нет —
  задачу не начинать: любой прогон CI после мёрджа упадёт на скачивании образа.
- Взять точный тег из `testing/Dockerfile` (литералы `LABEL version` и строка `CMD ["echo", ...]`,
  их владелец — Task 01) и сверить его с тем, что лежит в реестре. Тег не выдумывать: в спеке заявлен
  `0.0.11`, но источник истины — Dockerfile + реестр.
- Поднять тег контейнера в `.github/workflows/default.yml` (job `test`).
- Поднять тег контейнера в `.github/workflows/release.yml` (job `test`; у job `release` контейнера нет —
  он туда не добавляется).
- Добавить порт `21919` в **оба** цикла `testing/e2e.sh` — и в цикл `pgcenter record`, и в цикл
  `pgcenter report`. Порядок портов — по возрастанию, как сейчас.
- Прогнать полную верификацию против опубликованного образа: `make test` плюс `./testing/e2e.sh`,
  убедившись, что артефакты по порту 21919 созданы и непустые.
- Проверить grep'ом, что в репозитории не осталось ссылок на старый тег, кроме тех, чей владелец —
  другие задачи (`.claude/skills/project-knowledge/deployment.md` — Task 07).

## Acceptance Criteria

- [ ] `.github/workflows/default.yml`: `container:` указывает на новый тег тестового образа.
- [ ] `.github/workflows/release.yml`: `container:` в job `test` указывает на тот же новый тег; job
      `release` не изменён.
- [ ] Тег в обоих workflow совпадает с литералом версии в `testing/Dockerfile` и с тем, что опубликовано
      в DockerHub.
- [ ] `testing/e2e.sh`: порт `21919` присутствует в обоих циклах (record и report), скрипт по-прежнему
      под `set -euxo pipefail`, других изменений в нём нет.
- [ ] `./testing/e2e.sh` проходит целиком против опубликованного образа; в `/tmp/pgcenter-e2e/` есть
      `pgcenter.stat.21919.tar` и непустые `.out`-файлы для всех аргументов, включая `-Pv`, `-Pa`, `-Pb`.
- [ ] `make test` зелёный против опубликованного образа (PG 19-подтесты выполняются, а не скипаются).
- [ ] Никаких других файлов задача не трогает: ни Dockerfile, ни `prepare-test-environment.sh`,
      ни документацию.

## Context Files

**Feature artifacts:**
- [012-feat-pg19-compatibility-baseline.md](012-feat-pg19-compatibility-baseline.md) — user-spec
  (сценарий 5 «мейнтейнер добавляет PG 19 в CI», раздел про жёсткий порядок операций с образом)
- [012-feat-pg19-compatibility-baseline-tech-spec.md](012-feat-pg19-compatibility-baseline-tech-spec.md) —
  tech-spec (Decision 4 — источник истины по порядку; Task 9 в Implementation Tasks; раздел E2E tests)
- [012-feat-pg19-compatibility-baseline-decisions.md](012-feat-pg19-compatibility-baseline-decisions.md) —
  decisions log (отчёты предыдущих задач, в т.ч. Task 01 и Task 08 — фактический тег образа)

**Project knowledge:**
- [overview.md](../../../.claude/skills/project-knowledge/overview.md) — что за проект, поддерживаемые версии PG
- [architecture.md](../../../.claude/skills/project-knowledge/architecture.md) — раскладка пакетов, обработка версий PG
- [deployment.md](../../../.claude/skills/project-knowledge/deployment.md) — CI/CD, release process, тестовый
  образ и карта портов (файл правит Task 07, здесь только читаем)
- [patterns.md](../../../.claude/skills/project-knowledge/patterns.md) — чеклист добавления новой версии PG

**Code files:**
- [testing/e2e.sh](../../../testing/e2e.sh) — добавить порт 21919 в оба цикла
- [.github/workflows/default.yml](../../../.github/workflows/default.yml) — поднять тег `container:`
- [.github/workflows/release.yml](../../../.github/workflows/release.yml) — поднять тег `container:` в job `test`
- [testing/prepare-test-environment.sh](../../../testing/prepare-test-environment.sh) — только чтение:
  подтверждает, что кластер 19 на порту 21919 создаётся, конфигурируется, стартует и получает фикстуры
- [testing/Dockerfile](../../../testing/Dockerfile) — только чтение: источник истины по тегу образа

## Verification Steps

<!-- How to verify task is complete. For code — run tests. For deploy — check logs. For user-action — user confirmation. -->

- Убедиться, что образ доступен в реестре: `docker pull lesovsky/pgcenter-testing:<новый тег>` проходит.
  Если нет — стоп, Task 08 не закрыта.
- Поднять контейнер из **опубликованного** образа с проброшенными портами 21914–21919 и выполнить внутри
  него `prepare-test-environment.sh`. Ожидание: шесть кластеров стартовали, финальный `pg_isready` по
  всем шести портам (включая 21919) успешен.
- С хоста: `make test` — весь набор зелёный, PG 19-подтесты **выполняются**, а не скипаются (проверить по
  выводу `-v`/по отсутствию SKIP на 190000).
- С хоста: `make build && make install`, затем `./testing/e2e.sh`. Ожидание: скрипт отрабатывает до конца
  с нулевым кодом (он под `-x`, каждая команда видна в логе).
- Проверить артефакты: `/tmp/pgcenter-e2e/pgcenter.stat.21919.tar` существует, и для порта 21919 созданы
  `.out`-файлы по всем 19 аргументам; `-Pv`, `-Pa`, `-Pb` — непустые.
- `grep -rn "pgcenter-testing:0\.0\.10" .` — совпадений в `.github/workflows/` нет.
- Локально сверить: тег в обоих workflow == литерал версии в `testing/Dockerfile`.

## Details

<!-- All details for task execution — technical, organizational, any other. -->

**Files:**
- `testing/e2e.sh` — 26 строк, `set -euxo pipefail` в шапке. Два цикла:
  `for port in 21914 21915 21916 21917 21918` для `pgcenter record` (строка ~15) и такой же список для
  вложенного цикла `pgcenter report` (строка ~22). В **оба** списка добавляется `21919` в конец. Больше в
  файле ничего не меняется: набор аргументов отчёта (`-A -R -D -T -I -S -F -Xm -Xg -Xi -Xt -Xl -Xw -Pv -Pc
  -Pi -Pa -Pb -Pz`) уже покрывает все progress-экраны.
- `.github/workflows/default.yml` — строка 9: `container: lesovsky/pgcenter-testing:0.0.10` → новый тег.
  Единственное изменение в файле.
- `.github/workflows/release.yml` — строка 11: то же самое, в job `test`. Job `release` (goreleaser,
  docker login) выполняется на голом `ubuntu-latest` без `container:` — его не трогать.
- `testing/Dockerfile` (read-only здесь) — тег живёт в двух литералах: `LABEL version="0.0.10"` (строка 6)
  и `CMD ["echo", "pgcenter-testing 0.0.10: ..."]` (строка 38). Их бампит Task 01; здесь они читаются как
  источник истины.
- `testing/prepare-test-environment.sh` (read-only) — шесть циклов по версиям (создание, конфигурация,
  старт, ожидание готовности, фикстуры, финальная проверка), порт выводится как `219${v}`. После Task 01
  в списках есть `19` → порт 21919. Этот файл CI берёт **из образа**, а не из чекаута.

**Dependencies:**
- Task 08 (публикация образа в DockerHub) — жёсткая блокировка, не «желательно».
- Task 01 — определил содержимое образа и литералы тега в Dockerfile.
- Внешне: docker (для локальной проверки), сеть до DockerHub, установленный `pgcenter` в PATH для e2e.

**Edge cases:**
- **Образ не опубликован / опубликован под другим тегом.** Не мёрджить и не «предполагать» `0.0.11`:
  тег берётся из Dockerfile и проверяется `docker pull`/`docker manifest inspect`. Расхождение между
  Dockerfile, реестром и `deployment.md` (Task 07) — повод остановиться и эскалировать, а не выбрать
  один из вариантов молча.
- **Правка только e2e без бампа тегов** (или наоборот) — прямое нарушение Decision 4: CI покраснеет на
  первом же пуше. Обе правки в одном изменении.
- **Пропущен второй цикл в e2e.sh.** Тогда tar по 21919 запишется, но ни один отчёт по нему не построится —
  и сквозная проверка новых колонок, ради которой всё затевалось, не произойдёт. Скрипт при этом останется
  зелёным, то есть ошибка молчаливая.
- **`set -euxo pipefail` без skip.** В e2e нет условных пропусков: любой недоступный кластер = падение
  всего прогона. Это ожидаемое поведение, добавлять пробы порта/`|| true` не нужно (Decision 4 явно
  отклонил guard на порт как машинерию под разовую задачу упорядочивания).
- **`doc/development.md`** содержит устаревший список портов (21910–21914) и тег `:latest` — это
  предсуществующая рассинхронизация, вне скоупа этой задачи (в files_to_modify её нет).
- **`.claude/skills/project-knowledge/deployment.md`** упоминает тег `0.0.10` в трёх местах — владелец
  Task 07, здесь не править, чтобы не было конфликта правок.
- **Порт 21919 vs существующие 21995/21996** из старых docker-run примеров — к делу не относятся, карта
  портов кластеров задаётся формулой `219${v}` в prepare-скрипте.

**Implementation hints:**
- Правка e2e — ровно две строки: списки портов в обоих `for`. Не переписывать на диапазон/seq и не
  выносить список в переменную: минимальная правка, соответствующая стилю файла.
- В workflow-файлах менять только тег, не трогая версию Go, ключи кэшей и порядок шагов — они не имеют
  отношения к PG 19 и любые «попутные улучшения» раздувают ревью-поверхность деплойного изменения.
- Локальный прогон: образ не содержит Go (его ставит CI), поэтому практичнее запустить контейнер с
  публикацией портов и `prepare-test-environment.sh` внутри, а `make test` / `e2e.sh` гонять с хоста —
  тесты и `pgcenter` ходят на `127.0.0.1:219xx`.
- В e2e `pgcenter` берётся из PATH, поэтому перед прогоном нужен `make install` (в CI это отдельный шаг
  перед «Run E2E tests»).
- `rm -rf /tmp/pgcenter-e2e` в начале скрипта — повторный прогон безопасен, старые артефакты не путаются
  с новыми.
- После правок полезно перечитать оба workflow целиком: они почти идентичны, и рассинхронизация тега
  между ними — самая вероятная ошибка этой задачи.

## Reviewers

- **dev-code-reviewer** → `docs/features/012-feat-pg19-compatibility-baseline/012-feat-pg19-compatibility-baseline-task-09-dev-code-reviewer-review.json`
- **dev-security-auditor** → `docs/features/012-feat-pg19-compatibility-baseline/012-feat-pg19-compatibility-baseline-task-09-dev-security-auditor-review.json`
- **dev-deploy-reviewer** → `docs/features/012-feat-pg19-compatibility-baseline/012-feat-pg19-compatibility-baseline-task-09-dev-deploy-reviewer-review.json`

## Post-completion

- [ ] Записать краткий отчёт в [012-feat-pg19-compatibility-baseline-decisions.md](012-feat-pg19-compatibility-baseline-decisions.md) (Summary: 1-3 предложения, ревью со ссылками на JSON, без таблиц файндингов и дампов)
- [ ] Если отклонились от спека — описать отклонение и причину
- [ ] Обновить user-spec/tech-spec если что-то изменилось
