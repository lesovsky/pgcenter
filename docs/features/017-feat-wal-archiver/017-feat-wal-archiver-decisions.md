# Decisions Log: WAL / Archiver

Отчёты агентов о выполнении задач. Каждая запись создаётся агентом, выполнившим задачу.

---

## Task 02: PG 19 FPI column on the wal screen

**Status:** Done
**Commit:** 7a523a1
**Agent:** основной агент
**Summary:** В `internal/query/wal.go` добавлена константа `PgStatWALPG19` — это `PgStatWALDefault` плюс ровно одно выражение `round(wal_fpi_bytes / 1024, 2) AS "fpi,KiB"`, вставленное сразу после счётчика `wal_fpi`, чтобы количество full page images и объём, который они стоят, стояли рядом и оба попадали внутрь диффуемого диапазона. `SelectStatWALQuery` получил третью ветку `version >= PostgresV19` → `(PgStatWALPG19, 8, [2]int{2, 6})`; `stats_age` сместился на колонку 7 и остался вне интервала — это текстовое значение `date_trunc`, попадание которого внутрь интервала роняет весь сэмпл на `strconv.ParseInt`, а не одну ячейку. Ветки PG 14–17 и PG 18 не тронуты: diff `wal.go` состоит только из добавлений. `internal/view/view.go` править не потребовалось — его `case "wal":` уже делегирует селектору.

**Deviations:** Отклонений от спека по существу нет — все AC выполнены. Четыре уточнения по факту исполнения:
1. Тестов добавлено больше, чем в TDD Anchor. Сверх двух заявленных no-Postgres гардов добавлен деривационный ассерт: `PgStatWALPG19` приравнивается к `PgStatWALDefault` с ровно одной вставленной подстрокой. Причина — находка dev-test-reviewer (major), подтверждённая эмпирически: единственное новое выражение задачи содержит конверсию `/1024`, и ни один из четырёх ассертов TDD Anchor её не пиннил — замена на голый `wal_fpi_bytes` оставляла всё зелёным, а на экране колонка с заголовком KiB показывала бы байты (ошибка в 1024 раза). Деривация закрывает заодно и второй minor: `Ncols=8` и сохранность остальных семи колонок теперь гарантированы структурно, без фикстуры.
2. `assert.NoError` после `conn.Query` и `Format` в `Test_StatWALQueries` заменён на `require.NoError` — то есть отступление от прецедента `io_test.go`, на который ссылалась задача. Причина: ровно тот failure mode, который предсказывает caveat задачи (переименование `wal_fpi_bytes` на beta3/RC), при нефатальном ассерте тонет в каскаде из трёх производных падений вместо одного сообщения по существу. Обоими ревьюерами предложено независимо.
3. Полнопакетный гейт в CI-образе с первого раза не запустился: параллельный агент в этот момент держал `internal/query/archiver_test.go` в TDD-красном состоянии без реализации, и пакет не компилировался. WAL-скоуп прогнан на зеркале репозитория в scratchpad без этого файла; после того как соседний агент влил `archiver.go`, полный `go test -race -p 1 ./internal/query/...` прогнан в образе и зелёный. Файлы соседнего агента не редактировались, коммит сделан с явным pathspec на два файла.
4. Метрики в `017-feat-wal-archiver-metrics.json` не писались: по `metrics-protocol.md` метрики пишет только lead-агент, субагенты задач — нет.

**Tech debt:**
1. Doc-комментарий `PgStatWALDefault` по-прежнему гласит «PG 18+», хотя PG 19+ теперь уходит в свою ветку — константа покрывает только PG 18. Там же расходится имя: с тремя ветками «Default» больше не означает «самый новый layout», в отличие от `bgwriter.go`, где каждая константа версионно ограничена. Не исправлено осознанно: AC требует, чтобы diff `wal.go` состоял только из добавлений. Просится переименование `PgStatWALDefault` → `PgStatWALPG18` в задаче, которая следующей владеет этим файлом (ссылок вне `internal/query` нет).
2. В соседних ветках селектора смешаны именованная константа (`version >= PostgresV19`) и литерал (`version >= 180000`). Задача явно выносит нормализацию литерала из скоупа — забирать вместе с пунктом 1.
3. Сильнейшие утверждения задачи (живой счёт колонок и упорядоченный PG 19 header) стоят за `t.Skipf`, а скип зелёный. В этом прогоне подтверждено, что сабтест `pg_stat_wal/190000` реально выполнился (PASS, не SKIP), но ничто в тесте этого не обеспечивает: CI-образ, в котором фикстура PG 19 не поднялась, отрапортует успех. Предложенный ревьюером env-гейт (`PGCENTER_FIXTURES_REQUIRED`) не применён — он требует правки общей для пакета idiom `t.Skipf` и инвокации CI, а задача явно предписывает `t.Skipf` сохранить. Экспозиция зафиксирована здесь, чтобы не потерялась.
4. `wal_fpi_bytes` проверен против **PG 19beta2**. Если имя уедет на beta3/RC — радиус поражения ровно одна изменённая строка запроса, и живой тест упадёт с undefined column, что и есть верный сигнал.

**Reviews:**

*Round 1:*
- dev-code-reviewer: approved_with_suggestions, 4 minor (все опциональные; два конфликтуют с AC и вынесены в Tech debt) → [017-feat-wal-archiver-task-02-dev-code-reviewer-review.json](017-feat-wal-archiver-task-02-dev-code-reviewer-review.json)
- dev-security-auditor: approved, 0 findings → [017-feat-wal-archiver-task-02-dev-security-auditor-review.json](017-feat-wal-archiver-task-02-dev-security-auditor-review.json)
- dev-test-reviewer: needs_improvement, 1 major + 3 minor → [017-feat-wal-archiver-task-02-dev-test-reviewer-review.json](017-feat-wal-archiver-task-02-dev-test-reviewer-review.json)

Major и один minor от dev-test-reviewer закрыты деривационным ассертом, ещё один minor — переходом на `require.NoError` (см. Deviations 1–2). Оставшийся minor (env-гейт против зелёного скипа) и два minor от dev-code-reviewer (комментарий `PgStatWALDefault`, литерал `180000`) отклонены как конфликтующие с AC либо явно вынесенные задачей из скоупа — вынесены в Tech debt. Третий minor dev-code-reviewer — отсутствие этого файла — закрыт данной записью.

**Verification:**
- Полный `go test -race -p 1 -timeout 300s ./internal/query/...` в образе `lesovsky/pgcenter-testing:0.0.11` с фикстурами PG 14–19 → `ok ... 3.488s`. Сабтест `Test_StatWALQueries/pg_stat_wal/190000` → **PASS**, не SKIP: живой `FieldDescriptions()` вернул ровно 8 колонок в порядке `source, waldir_size, wal,KiB, records, fpi, fpi,KiB, buffers_full, stats_age`.
- `gofmt -l internal/query` — пусто; `golangci-lint run ./internal/query/...` → 0 issues; `go build -o /dev/null ./cmd` — ok; diff `wal.go` — только добавления (17 строк, 0 удалений).
- Мутационный контроль — каждая мутация применена к продакшн-коду, падение наблюдалось, мутация откачена:
  - **M1** `[2]int{2, 7}` (stats_age втянут в диффуемый диапазон) → красный дважды: `Test_SelectStatWALQuery` строки 190000 и 200000 (`expected [2]int{2,6}, actual [2]int{2,7}`) и на живой фикстуре `Test_StatWALQueries/pg_stat_wal/190000` — `"8" is not greater than "8"`, «DiffIntvl upper bound must leave at least one column (stats_age) outside».
  - **M2** выражение `"fpi,KiB"` перенесено в конец select-листа, после `stats_age` → красный `Test_SelectStatWALQuery_PG19ColumnOrder` (`"fpi,KiB" must precede wal_buffers_full` + нарушен суффикс `AS stats_age FROM pg_stat_wal`) и на фикстуре — упорядоченный список заголовков разошёлся именно перестановкой `fpi,KiB` в хвост, плюс `the last diffed column must be buffers_full: expected buffers_full, actual stats_age`.
  - **M3** `version >= PostgresV19` → `version == PostgresV19` → красной стала ровно строка 200000 в `Test_SelectStatWALQuery`; строка 190000 осталась зелёной, что и подтверждает адресность форвард-строки.
  - **M4** `wal_fpi_bytes` добавлен в `PgStatWALDefault` вместо отдельной константы, ветка PG 19 убрана (константа `PgStatWALPG19` оставлена определённой, иначе пакет не компилируется и красного теста не будет, а будет ошибка сборки) → красный `Test_SelectStatWALQuery_LegacyBranchesUntouched`: «should not contain "wal_fpi_bytes" — wal_fpi_bytes does not exist before PG 19».
  - **M5** (сверх AC, для проверки исправления по ревью) `round(wal_fpi_bytes / 1024, 2)` → `wal_fpi_bytes` → до исправления все четыре гарда оставались зелёными; после добавления деривационного ассерта → красный `Test_SelectStatWALQuery_PG19ColumnOrder`, «PG 19 query must be the PG 18 query plus exactly the fpi,KiB expression».
- Мутации M1–M4 независимо воспроизведены dev-code-reviewer в отдельном worktree с тем же результатом.

---

## Task 04: report CLI — `-W` becomes a string flag

**Status:** Done
**Commit:** 9ec944b
**Agent:** основной агент
**Summary:** `options.showWAL` переведён с `bool` на `string`, флаг `-W` — с `BoolVarP` на `StringVarP`, в `selectReport` добавлен закрытый whitelist `w` → `wal`, `a` → `archiver` без `default`-ветки. Ключевое решение — Decision 17: отсутствие `default` во внутреннем switch несущее, а не упущение: неотображённое значение выпадает из обоих switch к финальному `return ""`, и `validate()` отклоняет его до конструирования `report.Config`, поэтому в `ReportType` (фильтр tar-записей в `isFilenameOK` и ключ карты view в `newApp`) пользовательские байты попасть не могут. Ломающее изменение принято осознанно: обе задокументированные failure shapes (`flag needs an argument: 'W' in -W` и `report type is not specified, quit`) закреплены тестами с точными литералами.

**Deviations:** Нет отклонений от спека. Три уточнения по факту:
1. Спек утверждает, что описание флага видно в `pgcenter report --help`. Фактически `--help` для `report` полностью перекрыт рукописным текстом `printReportHelp()` в `cmd/help.go:170` (`SetHelpTemplate`/`SetUsageTemplate` в `cmd/pgcenter.go:56-57`), поэтому строка `Usage` у cobra-флага пользователю не показывается. Строка задана verbatim как требует AC и закреплена `Test_walFlagDefinition`, но `cmd/help.go` вне двухфайлового скоупа задачи и ни одной задачей фичи не покрыт — см. Tech debt.
2. Тестов добавлено больше, чем в TDD Anchor: `Test_selectReport_WALPrecedence` вырос из одного ассерта в таблицу из 4 строк, в whitelist-таблицу добавлены строки с ведущим и хвостовым пробелом, добавлены сквозные проверки `validate() → Config.ReportType`. Все — по результатам ревью, каждая закрывает мутацию, остававшуюся зелёной.
3. `Test_walFlagDefinition` закрепляет `NoOptDefVal == ""` — поле, не названное в TDD Anchor. Без него cobra-шим, явно отвергнутый Decision 7, проходил весь suite незамеченным.

**Tech debt:**
1. `cmd/help.go:170` по-прежнему печатает `-W, --wal    show pg_stat_wal statistics` — текст описывает булев флаг и не упоминает селекторы `w`/`a`. Это единственная справка, которую реально видит пользователь `pgcenter report --help`. Правка однострочная (по образцу строк `-D`/`-X`/`-P` с `SELECTOR`), но файл вне скоупа задачи 04 и не назначен ни одной задаче фичи — нужно назначить до релиза, иначе задача 09 сошлётся в release notes на текст, которого пользователь не увидит.
2. Ошибка `report type is not specified, quit` печатается, но процесс завершается с кодом **0** (`main()` в `cmd/pgcenter.go:67` печатает ошибку без `os.Exit(1)`). Дефект pre-existing и общерепозиторный (`-J q` ведёт себя так же), но именно это изменение делает его значимым: обёртка вида `pgcenter report -W -f dump.tar > out.txt || alert` теперь пишет пустой файл и рапортует успех. Details задачи утверждают «the command exits non-zero» — фактически это не так.
3. `github.com/spf13/pflag` теперь импортируется тестом напрямую, но в `go.mod:27` помечен `// indirect`. Сборка, тесты и линт проходят, `go mod tidy -diff` в CI нет; нужен `go mod tidy` в момент, когда в ветке не работают параллельные агенты.
4. Ожидаемо и намеренно не компенсировано: `-W a` отображается корректно, но view `archiver` (задача 05) и запись в `describeReport` (задача 07) появляются в следующих волнах, поэтому до их слияния `-W a` даёт пустой отчёт. Merge-гейт фичи, а не дефект задачи.

**Reviews:**

*Round 1:*
- dev-code-reviewer: approved_with_suggestions, 2 minor → [017-feat-wal-archiver-task-04-dev-code-reviewer-review.json](017-feat-wal-archiver-task-04-dev-code-reviewer-review.json)
- dev-security-auditor: approved, 1 major + 3 minor (major и два minor — вне скоупа, вынесены в Tech debt) → [017-feat-wal-archiver-task-04-dev-security-auditor-review.json](017-feat-wal-archiver-task-04-dev-security-auditor-review.json)
- dev-test-reviewer: needs_improvement, 2 major + 4 minor → [017-feat-wal-archiver-task-04-dev-test-reviewer-review.json](017-feat-wal-archiver-task-04-dev-test-reviewer-review.json)

*Round 2 (после исправлений):*
- dev-code-reviewer: approved, 0 findings → [017-feat-wal-archiver-task-04-dev-code-reviewer-review-round2.json](017-feat-wal-archiver-task-04-dev-code-reviewer-review-round2.json)
- dev-test-reviewer: passed, 1 minor (закрыт после ревью) → [017-feat-wal-archiver-task-04-dev-test-reviewer-review-round2.json](017-feat-wal-archiver-task-04-dev-test-reviewer-review-round2.json)

Одна рекомендация round 1 отклонена: строить зеркальный `pflag.FlagSet` из полей `Lookup("wal")` вместо литералов. Причина — производное зеркало всегда несёт пустой `NoOptDefVal` независимо от реального флага, то есть при установленном шиме оно продолжало бы рапортовать `flag needs an argument`, пока реальный CLI молча принимает голый `-W`. Оба ревьюера в round 2 согласились и сняли предложение.

**Verification:**
- `go test ./cmd/report/... -v` → 10 passed, все новые имена тестов присутствуют; зелено под `-race` и `-shuffle=on`
- `go build ./...`, `go vet ./cmd/...`, `gofmt -l cmd/report` (пусто), `golangci-lint run ./cmd/report/...` → 0 issues
- Мутационный контроль (каждая применена, наблюдалась красной, откачена): `default: return "wal"` → `Test_selectReport_WALWhitelistIsClosed`; `case "a"` возвращает `"wal"` → `Test_selectReport`; правка одного символа в help-тексте → `Test_walFlagDefinition`; `NoOptDefVal = "w"` → `Test_walFlagDefinition`; перенос ветки выше `showDatabases`/`showFunctions` и ниже `showBgwriter` → `Test_selectReport_WALPrecedence`; `strings.TrimSpace`/`TrimLeft`/`ToLower` → `Test_selectReport_WALWhitelistIsClosed`; `ReportType` захардкожен в `validate()` → `Test_selectReport_WALWhitelistIsClosed`
- Ручная проверка: `go run ./cmd report -W` → `flag needs an argument: 'W' in -W`; `go run ./cmd report -W -f /nonexistent.tar` → `report type is not specified, quit` (файл не открывается); `-W x` → та же ошибка; `-W w` / `-W a` / `-A -W a` доходят до открытия файла
