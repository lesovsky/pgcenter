---
status: planned                    # planned -> in_progress -> done
depends_on: []                     # ID задач-зависимостей (строки: ["01", "02"])
wave: 1                            # волна параллельного выполнения
skills: [code-writing]             # МАССИВ скиллов для загрузки
verify: bash                       # инструмент верификации (опционально: curl, bash, user)
reviewers: [dev-code-reviewer, dev-security-auditor, dev-test-reviewer]  # явно указать. Пусто = fallback на defaults
teammate_name:                     # имя агента-исполнителя (опционально; если не задано — генерируется по описанию задачи)
---

# Task 01: [009] Defensive allocation cap on tar entries

## Required Skills

Перед выполнением задачи загрузи:
- `/skill:code-writing` — [skills/code-writing/SKILL.md](~/.claude/skills/code-writing/SKILL.md)

## Description

Закрываем зарегистрированный техдолг **[009]** — неограниченную аллокацию памяти в `stat.NewPGresultFile`.
Сейчас `NewPGresultFile(r io.Reader, bufsz int64)` сразу делает `data := make([]byte, bufsz)`, где `bufsz` —
это `hdr.Size` из заголовка tar-записи. Заголовок контролируется содержимым архива (для недоверенного архива
`pgcenter record` — фактически атакующим), поэтому крафтовый заголовок с многогигабайтным `Size` приводит к
исчерпанию памяти при `pgcenter report` ещё ДО чтения каких-либо данных. Это классический CWE-789
(pre-allocation from an attacker-influenceable header).

Решение (Decision 1 tech-spec): вводим экспортируемую константу `stat.MaxResultFileSize int64 = 256 << 20`
(256 MiB) и в `NewPGresultFile` отклоняем `bufsz` вне диапазона (`bufsz < 0` или `bufsz > MaxResultFileSize`)
с понятной ошибкой **до** `make`. Две настоящие pre-alloc-точки в `report.readTar` (ветки `meta.*` и stat)
наследуют защиту через `NewPGresultFile`. Ветка `sysinfo.*` использует `io.ReadAll(io.LimitReader(...))` и
сама по себе **не** является pre-alloc-сливом (буфер растёт по реально прочитанным байтам), но мы добавляем
тот же предел inline для defense-in-depth и единообразия поведения — с поясняющим комментарием.

256 MiB ≈ в 300 раз больше самой крупной реальной записи в golden-фикстурах (~817 KB `statements_timings`),
поэтому предел никогда не отклоняет легитимные данные, но ограничивает крафтовый multi-GB заголовок.

## What to do

- В `internal/stat` (рядом с `NewPGresultFile` в `postgres.go`) добавить экспортируемую константу
  `MaxResultFileSize int64 = 256 << 20` с doc-комментарием о её назначении (cap для tar-entry pre-allocation).
- В `NewPGresultFile` добавить guard ДО `data := make([]byte, bufsz)`:
  - при `bufsz < 0` — вернуть `PGresult{}` и отдельную ошибку про отрицательный размер;
  - при `bufsz > MaxResultFileSize` — вернуть `PGresult{}` и ошибку формата
    `result file size %d exceeds limit %d bytes` (с `bufsz` и `MaxResultFileSize`);
  - `bufsz == 0` остаётся разрешённым (пустой буфер, текущее поведение `io.ReadFull` на 0-len — без изменений).
  - Сравнение строго в `int64` — никаких `int(hdr.Size)` / `int(bufsz)` в guard'е (иначе gosec G115).
- В `report/report.go`, ветка `sysinfo.*` (`io.ReadAll(io.LimitReader(r, hdr.Size))`): добавить inline-проверку
  `hdr.Size` против `stat.MaxResultFileSize` (и `< 0`) перед `io.ReadAll`, возвращая ошибку через существующий
  return-путь `readTar`. Обязательно сопроводить поясняющим комментарием: это defense-in-depth/единообразие,
  а не закрытие pre-alloc-слива (буфер `io.ReadAll` растёт по реальным байтам).
- Ветки `meta.*` (report.go:170) и stat (report.go:203) НЕ менять — они уже наследуют предел через
  `NewPGresultFile`.
- Написать тесты (см. TDD Anchor) ДО реализации.
- Закоммитить задачу независимо (отдельный commit для [009]).

## TDD Anchor

Тесты пишем ДО реализации: пишем → запускаем → убеждаемся что падают → пишем код → убеждаемся что проходят.

- `internal/stat/postgres_test.go::Test_NewPGresultFile_sizeCap` — новый табличный тест на `NewPGresultFile`
  через in-memory `bytes.Reader`:
  - валидный JSON размером под лимитом — читается без ошибки, `Values`/`Cols` не nil;
  - `bufsz == MaxResultFileSize` — разрешён (граница включительно);
  - `bufsz == MaxResultFileSize + 1` — отклонён с ошибкой `result file size ... exceeds limit ...`,
    БЕЗ аллокации (ошибка возвращается до `make`);
  - `bufsz == 0` — разрешён (пустой результат, текущее поведение);
  - `bufsz < 0` (например `-1`) — отклонён с ошибкой про отрицательный размер, `PGresult{}` возвращён.
- `report/report_test.go::Test_readTar_sizeCap` (имя на усмотрение исполнителя) — синтетический
  in-memory tar (ADR 008). ВАЖНО: существующий `writeEntry` (`report_test.go:463`) хардкодит
  `Size: int64(len(payload))`, поэтому для крафтового over-limit заголовка его использовать НЕЛЬЗЯ —
  построить `tar.Header{Name, Size, Mode}` напрямую через `tw.WriteHeader`, выставив `Size` сверх лимита
  независимо от реальной (маленькой/нулевой) длины payload. Прогнать на каждой из трёх веток
  (`meta.*`, `sysinfo.*`, stat): `readTar` возвращает ошибку лимита, в `dataCh` не уходит ни одной `data`.
  Развести ассерты по code-path: ветки `meta.*`/stat бьют по новому guard'у в `NewPGresultFile`, а
  `sysinfo.*` — по новой inline-проверке в `report.go` (это разные пути, ошибки могут отличаться текстом).
  Отдельный кейс: легитимная под-лимитная запись по-прежнему реплеится (отправляется в канал).

## Acceptance Criteria

- [ ] `stat.MaxResultFileSize int64 = 256 << 20` (256 MiB) объявлена как экспортируемая константа в `internal/stat`.
- [ ] `NewPGresultFile` возвращает ошибку (без аллокации) при `bufsz < 0` и при `bufsz > MaxResultFileSize`;
      `bufsz == limit` и `bufsz == 0` разрешены.
- [ ] Over-limit ошибка имеет формат `result file size %d exceeds limit %d bytes`; отрицательный размер — отдельная ошибка.
- [ ] Guard сравнивает в `int64`, без `int(...)`-конверсий (нет gosec G115).
- [ ] Все три ветки `report.readTar` (`meta.*`, `sysinfo.*`, stat) под единым пределом; `sysinfo.*` имеет
      inline-проверку с поясняющим комментарием.
- [ ] `go test ./internal/stat/... ./report/...` — зелёный; существующие тесты не сломаны.
- [ ] `make test`, `make lint` (golangci-lint v2 + gosec, без G115), `make vuln` — зелёные.
- [ ] Задача закоммичена независимо.

## Context Files

**Feature artifacts:**
- [011-refactor-tech-debt-paydown.md](docs/features/011-refactor-tech-debt-paydown/011-refactor-tech-debt-paydown.md) — user-spec
- [011-refactor-tech-debt-paydown-tech-spec.md](docs/features/011-refactor-tech-debt-paydown/011-refactor-tech-debt-paydown-tech-spec.md) — tech-spec (Decision 1, Architecture → How it works [009], Risks)
- [011-refactor-tech-debt-paydown-code-research.md](docs/features/011-refactor-tech-debt-paydown/011-refactor-tech-debt-paydown-code-research.md) — code research (sections 1, 5, 7, 8, 9)
- [011-refactor-tech-debt-paydown-decisions.md](docs/features/011-refactor-tech-debt-paydown/011-refactor-tech-debt-paydown-decisions.md) — decisions log (создаётся при выполнении)

**Project knowledge:**
- [overview.md](.claude/skills/project-knowledge/overview.md) — обзор проекта
- [architecture.md](.claude/skills/project-knowledge/architecture.md) — раскладка пакетов, поток данных, record/report
- [patterns.md](.claude/skills/project-knowledge/patterns.md) — паттерны кода и тестирования (testify, table/golden tests)

**Code files:**
- [internal/stat/postgres.go](internal/stat/postgres.go) — `NewPGresultFile` (517-538), `make([]byte, bufsz)` на 519; добавить const + guard
- [report/report.go](report/report.go) — `readTar` (139-222), ветка `sysinfo.*` на 186-200 (`io.ReadAll`/`io.LimitReader` на 191)
- [internal/stat/postgres_test.go](internal/stat/postgres_test.go) — `Test_NewPGresultFile` (293-329); добавить cap-табличный тест
- [report/report_test.go](report/report_test.go) — in-memory tar harness (`writeEntry` ~461-467 хардкодит `Size=len(payload)`; для over-limit строить `tar.Header` напрямую); добавить over-limit тест на 3 ветки

## Verification Steps

- Запустить целевые тесты: `go test ./internal/stat/... ./report/...` — все зелёные, новые cap-тесты проходят.
- Убедиться, что over-limit отклоняется до аллокации, легитимная golden-запись по-прежнему читается/реплеится.
- Прогнать `make test` (race + coverage), `make lint` (golangci-lint v2 + gosec — отдельно проверить отсутствие G115),
  `make vuln` — все зелёные.
- Проверить grep'ом, что в новом guard'е нет `int(hdr.Size)` / `int(bufsz)`.

## Details

**Files:**
- `internal/stat/postgres.go` — текущее состояние: `NewPGresultFile(r io.Reader, bufsz int64) (PGresult, error)`
  (строки 517-538), первая строка тела — `data := make([]byte, bufsz)` (519), затем `io.ReadFull`, `json.Unmarshal`,
  `validate`. Что сделать: добавить экспортируемую `const MaxResultFileSize int64 = 256 << 20` рядом с функцией;
  вставить guard в самое начало тела `NewPGresultFile` (перед `make`).
- `report/report.go` — текущее состояние: `readTar` (139-222), три ветки в `switch` (168-208). Ветки `meta.*` (170)
  и `default`/stat (203) идут через `stat.NewPGresultFile(r, hdr.Size)` — НЕ трогать. Ветка `sysinfo.*` (186-200)
  делает `io.ReadAll(io.LimitReader(r, hdr.Size))` (191) — добавить inline-проверку `hdr.Size` против
  `stat.MaxResultFileSize` (и `< 0`) перед этой строкой, возврат через существующий error-путь (как 192-194).
  Пакет `report` уже импортирует `internal/stat` — новый импорт не нужен.
- `internal/stat/postgres_test.go` — добавить новый табличный тест (рядом с `Test_NewPGresultFile`, 293-329),
  используя `bytes.NewReader`. Для under-limit-кейса можно взять реальный валидный JSON `PGresult` (или
  переиспользовать паттерн существующих тестов), для over-limit/negative — важно проверить именно error до
  аллокации (payload может быть пустым/маленьким, главное — крафтовый `bufsz`).
- `report/report_test.go` — переиспользовать harness `writeEntry`/`tar.NewWriter(&tarBuf)` (~458-476) и паттерн
  чтения из канала (`dataCh`/`doneCh`/`wg`). Записать запись с крафтовым `hdr.Size` сверх лимита (payload может
  быть маленьким — `Size` в заголовке не обязан совпадать с длиной payload), убедиться что `readTar` вернул
  ошибку лимита и в канал ничего не ушло; отдельно — что легитимная запись реплеится.

**Dependencies:**
- Зависимостей от других задач нет (`depends_on: []`, wave 1). Файлы disjoint с Task 2/3 (Decision 4).
- Только стандартная библиотека: `archive/tar`, `io`, `encoding/json`, `bytes` (в тестах). Новых пакетов нет.

**Edge cases:**
- `bufsz == 0` — разрешён (пустой буфер; `io.ReadFull` на 0-len возвращает nil) — НЕ отклонять.
- `bufsz == MaxResultFileSize` — разрешён (граница включительно).
- `bufsz == MaxResultFileSize + 1` — отклонён (граница исключительно сверху).
- `bufsz < 0` — отклонён (иначе `make([]byte, negative)` паникует); отдельная ошибка.
- `sysinfo.*` ветка: предел применяется как defense-in-depth, НЕ как закрытие pre-alloc-слива (комментарий обязателен).

**Implementation hints:**
- `MaxResultFileSize` должен быть именно `int64`, чтобы `bufsz > MaxResultFileSize` был чистым int64-сравнением
  без конверсий — это ключ к отсутствию gosec G115.
- Over-limit ошибка — закреплённый формат: `fmt.Errorf("result file size %d exceeds limit %d bytes", bufsz, MaxResultFileSize)`.
- Negative-size — отдельная distinct-ошибка (например `fmt.Errorf("result file size %d is negative", bufsz)` — текст
  на усмотрение, но смысл отличается от over-limit).
- Активный техдолг [016] (`internal/stat/*` молча глотают ошибки) касается этого файла, но НЕ в скоупе: здесь мы
  добавляем именно ВОЗВРАЩАЕМУЮ ошибку на error-пути `NewPGresultFile`, что не ухудшает [016].
- revive `redefines-builtin-id` (severity error) — не теневить builtins в именах (прецедент: commit 8f1d588).

## Reviewers

- **dev-code-reviewer** → `docs/features/011-refactor-tech-debt-paydown/011-refactor-tech-debt-paydown-task-01-dev-code-reviewer-review.json`
- **dev-security-auditor** → `docs/features/011-refactor-tech-debt-paydown/011-refactor-tech-debt-paydown-task-01-dev-security-auditor-review.json`
- **dev-test-reviewer** → `docs/features/011-refactor-tech-debt-paydown/011-refactor-tech-debt-paydown-task-01-dev-test-reviewer-review.json`

## Post-completion

- [ ] Записать краткий отчёт в [011-refactor-tech-debt-paydown-decisions.md](docs/features/011-refactor-tech-debt-paydown/011-refactor-tech-debt-paydown-decisions.md) (Summary: 1-3 предложения, ревью со ссылками на JSON, без таблиц файндингов и дампов)
- [ ] Если отклонились от спека — описать отклонение и причину
- [ ] Обновить user-spec/tech-spec если что-то изменилось
