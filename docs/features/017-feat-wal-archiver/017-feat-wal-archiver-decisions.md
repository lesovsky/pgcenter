# Decisions Log: WAL / Archiver

Отчёты агентов о выполнении задач. Каждая запись создаётся агентом, выполнившим задачу.

---

## Task 05: Регистрация view `archiver` и обновление счётных тестов

**Status:** Done
**Commit:** 13c33f2
**Agent:** основной агент

**Summary:** В `view.New()` добавлена запись `archiver` (`MinRequiredVersion: query.PostgresV14`, `QueryTmpl: query.PgStatArchiverDefault`, `Ncols: 9`, `DiffIntvl{0,0}`, `OrderKey 0` / `OrderDesc true`, непустые `ColsWidth`/`Filters`, `NotRecordable` оставлен нулевым), в `Configure()` — соответствующий `case "archiver":`. Ключевое решение — `MinRequiredVersion` здесь несущий, а не косметический: общего нижнего порога в проекте нет (реестр обслуживает вплоть до PG 9.4, `TestViews_Configure` гоняет `90400`), и при нулевом значении экран предлагался бы на PG ≤ 11, где `pg_ls_archive_statusdir()` не существует, а `pgcenter record` обрывает **всю** запись на первой же ошибке запроса. Попутно закрыт долг задачи 02: в `TestViews_Configure` добавлены `wal`-ассерты в ветки `case 190000:` и `case 140000:` — до этого ничто не пиннило, что `Configure()` реально доносит PG 19 layout до зарегистрированного view (селекторный табличный тест проверяет селектор, а не проводку).

**Deviations:** Отклонений от спека нет — все AC выполнены. Три уточнения по факту исполнения:

1. `TestNew_ArchiverView` пиннит больше полей, чем перечисляет шаг 4 задачи: добавлены `Name`, `QueryTmpl`-seed и непустота `ColsWidth`/`Filters`. Задача внутренне противоречива — шаг 4 даёт узкий список, а AC №1 требует «pins every field listed above», включая non-nil карты. Разошлись в пользу AC: каждое из трёх полей провалило litmus-тест (мутация оставляла все пять тестов зелёными), причём потеря карт — не неверное число, а паника в gocui-обработчике при первом расширении колонки или установке фильтра.
2. `Msg` закреплён полным равенством, а не подстрокой (`assert.Contains` → `assert.Equal`). AC №1 требует строку verbatim; равенство — строгое надмножество подстроки, поэтому мутация «убрать `archive_mode=on`» по-прежнему краснеет, но теперь ловится и опечатка в префиксе, которую `Contains` пропускал.
3. Ассерт принадлежности `archiver` в `Test_filterViews` сделан через lookup в карте + `assert.Equal`, а не через предложенный ревьюером `assert.Contains`: падение `Contains` на `view.Views` печатает все 28 структур `View` (141 КБ вывода) и хоронит единственный нужный бит.

**Tech debt:**

1. Инварианты реестра (`key == v.Name`, непустые `ColsWidth`/`Filters`) не проверяются ни для одного view кроме `archiver`. Гарды `TestNew_BgwriterView`, `TestNew_ReplslotsView`, `TestNew_StatIOView`, `TestNew_StatIOTimeView`, `TestNew_StatementsJITView` пропускают их все. Реестро-широкий `TestNew_ViewMapInvariants` предложен обоими ревьюерами и сознательно не добавлен: он охраняет view, принадлежащие другим задачам и фичам, то есть делает эту задачу владельцем падений, которые она не может вызвать. Оба ревьюера в round 2 согласились с отсрочкой; dev-test-reviewer прогнал предложенный инвариант против текущего реестра — все 28 view его удовлетворяют, дефекта за отсрочкой не прячется. Правильное место записи — раздел `patterns.md` «Adding a New View», куда заглянет следующая регистрация.
2. `record/recorder.go` роняет всю запись при ошибке любого одного view (`record/record.go:172-175`). Для `archiver` это Decision 4 by design (роль без `pg_monitor` теряет экран целиком), и `wal` с тем же порогом PG14 уже вызывает `pg_ls_waldir()` в том же классе привилегий — то есть экспозиция не новая. Долговременное решение — пропускать сбойный view с INFO-строкой, как это уже делает ветка `pg_stat_statements not found`, в `tarRecorder.collect()`, а не в реестре. Найдено dev-security-auditor.
3. Имена WAL-файлов (колонки 3 и 6) рендерятся без escape-санитизации. Decision 16 корректен для настоящего сервера (`VALID_XFN_CHARS` не пропускает ESC), но не для враждебного эндпоинта, говорящего по протоколу; это уже зафиксированный техдолг [029] (`docs/tech-debt.md:47`), покрывающий все текстовые колонки всех экранов. Здесь не расширен; чинить один раз в `printDataCell`.
4. Комментарий-обоснование над таблицей `Test_filterViews` вырос до 25 строк на 7 строк данных и накапливает археологию четырёх фич. Предложение dev-code-reviewer свернуть историю в один инвариант отклонено (задача явно предписывает комментарий **расширить**, а слои документируют, почему держатся числа в остальных строках), и в round 2 сам ревьюер его снял. Оставлено как housekeeping для следующего владельца файла.
5. Раздел Edge cases самого task-файла (и первая редакция комментария в тесте) приписывает запись в `ColsWidth` функции `align.SetAlign`. Фактически `internal/align/align.go:18` строит новую карту, а `top/stat.go:788` присваивает её целиком — этот путь nil-карту **чинит**, а не роняет. Реальные незащищённые in-place writer'ы — `top/config_view.go:100`, `:124` и `:166`. В комментарии теста исправлено; в task-файле — нет.

**Reviews:**

*Round 1:*
- dev-code-reviewer: approved_with_suggestions, 1 major + 3 minor → [017-feat-wal-archiver-task-05-dev-code-reviewer-review.json](017-feat-wal-archiver-task-05-dev-code-reviewer-review.json)
- dev-security-auditor: approved, 0 critical / 0 major, 2 minor (оба вне скоупа, вынесены в Tech debt) → [017-feat-wal-archiver-task-05-dev-security-auditor-review.json](017-feat-wal-archiver-task-05-dev-security-auditor-review.json)
- dev-test-reviewer: needs_improvement, 2 major + 5 minor → [017-feat-wal-archiver-task-05-dev-test-reviewer-review.json](017-feat-wal-archiver-task-05-dev-test-reviewer-review.json)

*Round 2 (после исправлений):*
- dev-code-reviewer: approved_with_suggestions, 1 minor (фактическая неточность комментария про `align.SetAlign` — исправлена) → [017-feat-wal-archiver-task-05-dev-code-reviewer-review-round2.json](017-feat-wal-archiver-task-05-dev-code-reviewer-review-round2.json)
- dev-test-reviewer: passed, 2 minor (отсроченный реестровый инвариант + опечатка в комментарии — исправлена) → [017-feat-wal-archiver-task-05-dev-test-reviewer-review-round2.json](017-feat-wal-archiver-task-05-dev-test-reviewer-review-round2.json)

Приняты и применены все major-находки round 1 (`ColsWidth`/`Filters`, `Name`) и все minor, кроме двух: сворачивание исторического комментария в `record_test.go` (отклонено, ревьюер снял в round 2) и реестро-широкий `TestNew_ViewMapInvariants` (отсрочено, оба ревьюера согласились — см. Tech debt 1). Оба ревьюера round 2 независимо воспроизвели весь мутационный набор в песочнице и подтвердили результаты построчно.

**Verification:**
- Счётчики before → after: `TestNew` 27 → 28; `TestView_VersionOK` 190000 27 → 28, 160000 27 → 28, 140000 24 → 25, строки 130000/120000/110000/100000 **не тронуты**; `Test_filterViews` `wantV` 27/18/24 → 28/19/25 на трёх строках ≥ PG14 и `wantN` 8/11/13/13 → 9/12/14/14 на четырёх строках ≤ PG13, блок-комментарий расширен.
- Хост: `go test ./internal/view/...` → ok; `go test ./record/... -run Test_filterViews` → ok. Полный пакет `record` в образе `lesovsky/pgcenter-testing:0.0.11`: `go test -race -p 1 -timeout 300s ./internal/view/... ./record/...` → оба ok (в т.ч. `Test_app_record`, который считает recordable views динамически). Дополнительно в образе прогнаны `./report/...` и `./top/...` → ok, чтобы исключить незамеченный счётный пин вне двух известных мест.
- `gofmt -l` пусто, `go vet` чисто, `golangci-lint run ./internal/view/... ./record/...` → 0 issues, `gosec` → 0 issues, `go build -o /dev/null ./cmd` → ok. `git diff record/record.go` пуст; `case "wal":` в `view.go` — только контекстная строка диффа.
- Мутационный контроль — каждая мутация применена к продакшн-коду, падение **наблюдалось**, мутация откачена:
  - **M1** удаление `MinRequiredVersion` → `TestView_VersionOK` красный ровно на четырёх строках ≤ PG13 (130000 `19/20`, 120000 `16/17`, 110000 `14/15`, 100000 `14/15`); `Test_filterViews` красный на тех же четырёх строках, и после доработки — с поимённым `archiver kept? version=…` на каждой.
  - **M2** `Ncols` 9 → 8 и `DiffIntvl{0,0}` → `{0,1}` → `TestNew_ArchiverView` (`TestViews_Configure` при этом остаётся зелёным: селектор переприсваивает 9 — это и есть честная граница того, что пиннят archiver-ассерты).
  - **M3** `Msg` без `archive_mode=on` → `TestNew_ArchiverView`.
  - **M4** `NotRecordable: true` → `Test_filterViews` красный **ровно на трёх строках ≥ PG14** (`{190000,"public"}` `0/1` и `28/27`, `{140000,""}` `9/10` и `19/18`, `{140000,"public"}` `3/4` и `25/24`); четыре строки ≤ PG13 остаются **зелёными** — `filterViews` удаляет view и делает `filtered++` в обеих ветках, так что подмена *причины* отбрасывания там ничего не сдвигает. Ожидать красноты на каждой строке арифметически неверно.
  - **M5** откат ветки PG 19 из `SelectStatWALQuery` (задача 02) → `TestViews_Configure` красный в ветке `case 190000:` по тексту запроса, `Ncols` `8/7` и `DiffIntvl` `{2,6}/{2,5}`. Это и есть настоящий гейт проводки.
  - **M6** удаление `ColsWidth` и `Filters` из записи → `TestNew_ArchiverView`, два падения `NotNil`.
  - **M7** `Name` → `"archive"` → `TestNew_ArchiverView` (все остальные тесты ищут view по ключу карты и остаются зелёными).
  - **M8** `QueryTmpl`-seed → `query.PgStatWALPG14` → `TestNew_ArchiverView`.
  - **M9** опечатка только в префиксе `Msg` (`"Show archiver stats (requires archive_mode=on)"`) → `TestNew_ArchiverView`; под прежним `assert.Contains` эта мутация оставалась зелёной.
  - **Отрицательный контроль (задокументирован, не дефект):** удаление `case "archiver":` из `Configure()` оставляет пакет зелёным — `New()` уже выставляет те же `QueryTmpl`/`Ncols`/`DiffIntvl`. Archiver-ассерты в `TestViews_Configure` — регрессионный страж дрейфа между `SelectStatArchiverQuery` и статической записью, а не доказательство проводки; оговорка вынесена в комментарий рядом с самими ассертами.

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

---

## Task 01: Archiver query, selector and the shared test-role helper

**Status:** Done
**Commit:** ae50197 (round-1 содержимое тех же трёх файлов попало в 23a7b1f — см. Deviations)
**Agent:** основной агент

**Summary:** Добавлены `internal/query/archiver.go` с константой `PgStatArchiverDefault` (9 колонок в зафиксированном порядке: `source, ready, archived, last_archived, archived_age, failed, last_failed, failed_age, stats_age`) и селектором `SelectStatArchiverQuery(_ int) (string, int, [2]int)` → `(PgStatArchiverDefault, 9, [2]int{0,0})`; версионной ветки нет, потому что `pg_stat_archiver` схемно идентичен на PG 14–19, а `pg_ls_archive_statusdir()` есть на всех (форма `SelectStatIOTimeQuery`, `io.go:99`). В `internal/postgres/testing.go` добавлен общий хелпер `SetupTestRole(db *DB, name string, pgMonitor bool) error` — идемпотентное создание роли через `DO`-блок, опциональный `GRANT pg_monitor`, `SET ROLE`; он возвращает `error` и не тянет пакет `testing`, потому что файл не имеет build-тега и попадает в релизный бинарь. Привилегии Decision 4 доказаны тестами в обе стороны: роль с `pg_monitor` выполняет запрос, роль без него получает SQLSTATE `42501` с именем `pg_ls_archive_statusdir` в сообщении.

**Deviations:**

1. **TDD Anchor противоречит сам себе по тесту 3.** Преамбула утверждает, что тесты 1 и 3 идут без PostgreSQL, а собственный буллет теста 3 требует сканирования строк фикстуры в `sql.NullString` и проверки `Valid` — это невозможно без сервера. Разрешено расщеплением: проверка «в запросе нет `coalesce`» вынесена в бессерверный `Test_StatArchiverQuery_Structure`, живая проверка NULL/значений осталась в `Test_StatArchiverQuery_NullsStayNull`. Оба ревьюера round 2 подтвердили, что это правильный выбор.
2. **Тестов шесть, а не пять.** Сверх пяти из TDD Anchor добавлены `Test_StatArchiverQuery_Structure` (бессерверная фиксация предиката `.ready` и порядка алиасов — по major-находке test-ревьюера: на фикстурах пустой каталог статусов, поэтому `count(*) FILTER (WHERE name LIKE '%.ready')` и голый `count(*)` живьём неразличимы) и `Test_SetupTestRole_RejectsUnsafeName` (по сходящейся находке code-ревьюера и test-ревьюера round 2 — новая проверка имени роли иначе не покрыта ничем).
3. **Мутация M7 краснеет иначе, чем обещает AC.** AC требует, чтобы `pgMonitor: false` в позитивном тесте дал красный «с SQLSTATE 42501». После усиления гварда (проверка `pg_has_role` и точного состава членства) тест краснеет раньше — на самом гварде, до запроса, — и 42501 в этой мутации больше не достигается. В round 1, до усиления, 42501 наблюдался. Зависимость от `42501` теперь утверждается не разовой мутацией, а постоянно — тестом `Test_StatArchiverQuery_WithoutPgMonitorFails` на каждом прогоне. Формулировку чек-бокса в task-файле стоит поправить.
4. **Коммит не тот, который планировался.** Round-1 содержимое всех трёх файлов было заметено параллельным агентом задачи 04 в чужой коммит `23a7b1f` (он сделал `git add`/commit поверх моего staged-состояния). `internal/query/archiver.go` с тех пор не менялся, поэтому в коммите ae50197 его нет — там только правки по итогам ревью в двух оставшихся файлах. Содержимое дерева корректное; пострадала только атрибуция.

**Tech debt:**

1. Идемпотентность `SetupTestRole` (ветка «роль уже существует») проверяется только процедурой Verification Step 3 — два прогона в одном контейнере, — но не автотестом. Тест на это должен лежать в `internal/postgres/testing_test.go`, а это четвёртый файл, запрещённый критерием приёмки №1. Вынесено в follow-up; оба ревьюера round 2 согласились, что отложить правильно.
2. Роли `pgcenter_test_archiver_monitor` / `pgcenter_test_archiver_norole` остаются на кластере навсегда (teardown запрещён — на нём держится критерий переиспользуемости). Для эфемерных CI-контейнеров это верный размен; на долгоживущем кластере роли надо снимать вручную. Роли `NOLOGIN`, без членов, `SET ROLE` в них требует суперюзера — практического доступа не дают.
3. Регексп `^[a-z_][a-z0-9_]*$` ограничивает синтаксис, но не семантику: `none`, `default`, `public` его проходят. Три из них падают громко, `SET ROLE NONE` — тихо (сброс к session user). Сегодня недостижимо (оба вызова передают константы) и ловится ниже `assertRestrictedSession`. Записано, чтобы регексп позже не читали как более сильный контракт.
4. Рекомендация security-аудитора заменить якоря `^`/`$` на `\A`/`\z` не применена: в Go `regexp.Perl` включает `OneLine`, поэтому обхода через завершающий `\n` нет — оба ревьюера проверили это эмпирически. Ценность правки только в переносимости паттерна в другой язык.

**Reviews:**

*Round 1:*
- dev-code-reviewer: approved_with_suggestions, 3 minor → [017-feat-wal-archiver-task-01-dev-code-reviewer-review.json](017-feat-wal-archiver-task-01-dev-code-reviewer-review.json)
- dev-security-auditor: approved, 3 minor → [017-feat-wal-archiver-task-01-dev-security-auditor-review.json](017-feat-wal-archiver-task-01-dev-security-auditor-review.json)
- dev-test-reviewer: needs_improvement, 2 major + 7 minor → [017-feat-wal-archiver-task-01-dev-test-reviewer-review.json](017-feat-wal-archiver-task-01-dev-test-reviewer-review.json)

*Round 2 (после исправлений):*
- dev-code-reviewer: approved_with_suggestions, 4 minor → [017-feat-wal-archiver-task-01-dev-code-reviewer-review-round2.json](017-feat-wal-archiver-task-01-dev-code-reviewer-review-round2.json)
- dev-security-auditor: approved, 3 minor (round-1 находка 1 закрыта) → [017-feat-wal-archiver-task-01-dev-security-auditor-review-round2.json](017-feat-wal-archiver-task-01-dev-security-auditor-review-round2.json)
- dev-test-reviewer: passed, 5 minor → [017-feat-wal-archiver-task-01-dev-test-reviewer-review-round2.json](017-feat-wal-archiver-task-01-dev-test-reviewer-review-round2.json)

Одна рекомендация round 1 отклонена по существу: живая проверка предиката `.ready` через `VALUES`-литерал. Она проверяла бы семантику `LIKE` самого PostgreSQL, а не код репозитория, и пропускалась бы на хосте; подстрочная фиксация в бессерверном тесте краснеет на всех тех же мутациях (снятый FILTER, `%.done`, `%ready%`). Test-ревьюер в round 2 согласился и снял рекомендацию, назвав бессерверную фиксацию не более дешёвым, а более удачным инструментом.

**Verification:**
- `go test -race -p 1 -timeout 300s ./internal/query/... ./internal/postgres/...` в образе `lesovsky/pgcenter-testing:0.0.11` → зелено; `-v`-прогон: **0 пропущенных** подтестов Archiver, все PG 14–19 реально отработали
- Переиспользуемость: два прогона подряд в одной сессии контейнера → оба зелёные (создание роли идемпотентно, второй прогон встречает существующие роли)
- `make lint` (golangci-lint + gosec) → 0 issues; `make vuln` → чисто; `gofmt -l` пусто; `go build -o /dev/null ./cmd` → ок; `grep -n '"testing"' internal/postgres/testing.go` → пусто
- Мутационный контроль — 11 мутаций, каждая применена в контейнере, наблюдалась **красной**, откачена; итоговое дерево побайтово совпадает с исходным:
  - M1 `Ncols` 9→8 → `Test_SelectStatArchiverQuery`
  - M2 `DiffIntvl {0,0}`→`{2,5}` → `Test_SelectStatArchiverQuery` (и только он)
  - M3 удаление колонки `ready` → `Test_StatArchiverQueries` и по счётчику, и по списку имён
  - M4 перестановка `ready`/`archived` → `Test_StatArchiverQueries` по **порядку имён** (счётчик не сработал) + `Test_StatArchiverQuery_Structure`
  - M5 `coalesce(last_archived_wal,'-')` → `Test_StatArchiverQuery_NullsStayNull` + `Test_StatArchiverQuery_Structure`
  - M6 пропуск `SetupTestRole`/`SET ROLE` в позитивном тесте → `Test_StatArchiverQuery_PgMonitorRoleSucceeds` ровно на своём гварде (`current_user` + `rolsuper`), до запроса
  - M7 `pgMonitor: false` после `DROP ROLE` на всех шести кластерах → `Test_StatArchiverQuery_PgMonitorRoleSucceeds` (в round 1 — с SQLSTATE 42501, после усиления гварда — на членстве; см. Deviations 3); `Test_StatArchiverQuery_WithoutPgMonitorFails` в том же прогоне остался зелёным → `RESET ROLE` не утекает
  - M8 подстановка литерала `0` вместо привилегированного вызова → `Test_StatArchiverQuery_WithoutPgMonitorFails`
  - M9 предикат `'%.ready'` → `'%.done'` → `Test_StatArchiverQuery_Structure`
  - M10 выдача `pg_monitor` deny-роли после `DROP ROLE` → `Test_StatArchiverQuery_WithoutPgMonitorFails` на «Should be empty, but was [pg_monitor]»
  - M11 расширение регекспа имени роли до верхнего регистра → `Test_SetupTestRole_RejectsUnsafeName`

---

## Task 03: Verbose panel backlog on a pg_monitor-accessible function

**Status:** Done
**Commit:** e3ac49d
**Agent:** основной агент

**Summary:** `OverviewArchivingBacklog` переведён с `pg_ls_dir('pg_wal/archive_status') AS name` на `pg_ls_archive_statusdir()` — контракт вывода посимвольно тот же (один `bigint`, байты, `count(.ready) × wal_segment_size`), алиас `AS name` снят как мёртвый синтаксис (функция сама отдаёт OUT-колонку `name`). Причина ровно одна: у `pg_ls_dir` ACL `{postgres}` — только суперюзер, поэтому роль с одним `pg_monitor` получала 42501 на каждом тике и панель показывала `n/a` вместо первого сигнала об остановке архивации (Decision 8 отменяет ADR [010]). Механизм деградации в `collectOverviewStat` (собственный `QueryRow`, проглоченная ошибка, `ArchivingBacklogValid`) не тронут — изменился только комментарий над ним. Роли под тесты создаются общим хелпером `postgres.SetupTestRole` из задачи 01; собственного хелпера и inline `CREATE ROLE`/`GRANT` не добавлено, `internal/postgres/testing.go` не изменялся.

**Deviations:**

1. **Тестов четыре, а не три.** Сверх TDD Anchor добавлен бессерверный `Test_ArchivingBacklogQuery_Structure` — по major-находке test-ревьюера и по прецеденту задачи 01: на фикстурах `archive_mode=off` и пустой каталог статусов, поэтому любая живая проверка бэклога сводится к `0 >= 0`, и снятие FILTER `.ready` либо множителя `pg_size_bytes(...)` осталось бы зелёным во всех живых тестах. Обе мутации наблюдались красными только на нём.
2. **`assert.True(t, got.Valid)` в collect-тесте заменён на сравнение с суперюзерским baseline'ом.** TDD Anchor называет `Valid`, но `collectOverviewStat` выставляет `s.Valid = true` безусловно (`postgres.go:202`) — ассерция не могла бы покраснеть никогда. Вместо неё снимается образец под суперюзером до `SET ROLE` и сравниваются `TotalSizeValid`/`DatabasesCount`: утверждение «остальная выборка не пострадала» стало фальсифицируемым.
3. **Поправлен пятый комментарий сверх четырёх названных** — `internal/stat/postgres.go:84`, комментарий поля `ArchivingBacklogValid`: он утверждал, что поле становится `n/a` при `archive_mode=off`. Это неверно (каталог статусов создаётся initdb, агрегат возвращает настоящий `0`) и противоречило сразу Decision 11, переписанному комментарию потребителя и `Test_collectOverviewStat_Degradation`. Major-находка code-ревьюера; правка в одну строку внутри файла, уже входящего в скоуп.
4. **Роль для `internal/stat` отдельная** (`pgcenter_test_backlog_collect`), а не общая с `internal/query` (`pgcenter_test_backlog_monitor` / `pgcenter_test_backlog_norole`). DO-блок в `SetupTestRole` не атомарен, и общий объект между пакетами дал бы гонку на `pg_authid` вне `-p 1`; сходящаяся minor-находка code- и test-ревьюеров.
5. **Рекомендация перевести новые тесты на `connectArchiverFixture` отклонена** — форма `NewTestConnectVersion` + `t.Skipf` предписана Implementation Hints самой задачи и совпадает с остальными тестами файла, а аккуратное переиспользование требовало бы переименования хелпера в `archiver_test.go` (файл задачи 01). Test-ревьюер снял находку, записав переименование в follow-up.

**Tech debt:**

1. `assertRestrictedSession`/`resetRole` продублированы: в `internal/query` они пакетные (задача 01), в `internal/stat` guard написан инлайном. Объединение требует переноса рядом с `SetupTestRole` в `internal/postgres/testing.go` — файл вне скоупа обеих задач. На финализацию.
2. Ни один тест не наблюдает **ненулевой** бэклог: фикстуры работают с `archive_mode=off`. Поведенческая проверка отнесена к стендовому прогону (задача 10); на этом слое её заменяет структурный тест.
3. Ролей на кластере стало на три больше (`pgcenter_test_backlog_*`), teardown'а нет по Decision 18 — для эфемерных CI-контейнеров это верный размен, на долгоживущем кластере роли снимаются вручную.
4. ADR [010] в `docs/decisions-log.md` по-прежнему называет `pg_monitor` достаточным для `pg_ls_dir` — не трогается в этой задаче намеренно, правка на финализации фичи.

**Reviews:**

*Round 1:*
- dev-code-reviewer: approved_with_suggestions, 1 major + 4 minor → [017-feat-wal-archiver-task-03-dev-code-reviewer-review.json](017-feat-wal-archiver-task-03-dev-code-reviewer-review.json)
- dev-security-auditor: approved, 3 minor → [017-feat-wal-archiver-task-03-dev-security-auditor-review.json](017-feat-wal-archiver-task-03-dev-security-auditor-review.json)
- dev-test-reviewer: needs_improvement, 1 major + 4 minor → [017-feat-wal-archiver-task-03-dev-test-reviewer-review.json](017-feat-wal-archiver-task-03-dev-test-reviewer-review.json)

*Round 2 (после исправлений):*
- dev-code-reviewer: approved_with_suggestions, 0 critical/major → [017-feat-wal-archiver-task-03-dev-code-reviewer-review-round2.json](017-feat-wal-archiver-task-03-dev-code-reviewer-review-round2.json)
- dev-security-auditor: approved, 2 minor (обе вне скоупа задачи) → [017-feat-wal-archiver-task-03-dev-security-auditor-review-round2.json](017-feat-wal-archiver-task-03-dev-security-auditor-review-round2.json)
- dev-test-reviewer: passed, 0 находок → [017-feat-wal-archiver-task-03-dev-test-reviewer-review-round2.json](017-feat-wal-archiver-task-03-dev-test-reviewer-review-round2.json)

**Verification:**
- `go test -race -p 1 -count=1 -timeout 300s ./internal/query/... ./internal/stat/...` в образе `lesovsky/pgcenter-testing:0.0.11` (PG 14–19) → зелено; два подряд некэшированных прогона в одном контейнере → оба зелёные (создание ролей идемпотентно)
- `go build ./cmd`, `go vet`, `gofmt` по четырём файлам → чисто; `make lint` (golangci-lint + gosec) → 0 issues; `make vuln` → чисто
- Грепы из Verification Steps: `grep -rn "pg_ls_dir" internal/` → только объясняющие комментарии и `NotContains`-ассерция; `grep -rn "has pg_monitor" internal/` → пусто
- Мутационный контроль (каждая применена, наблюдалась **красной**, откачена):
  - M1 `FROM` откачен на `pg_ls_dir('pg_wal/archive_status') AS name` → `Test_ArchivingBacklogQuery_PgMonitorRole` и `Test_collectOverviewStat_PgMonitorRole` с `permission denied for function pg_ls_dir (SQLSTATE 42501)`, плюс `Test_ArchivingBacklogQuery_Structure`
  - M2 пропуск вызова `SetupTestRole` в позитивном тесте → красный на собственном гварде: `current_user` = `postgres`, `rolsuper` = true, членство пустое — до запроса
  - M3 `SetupTestRole(conn, backlogRoleMonitor, false)` на **свежем контейнере** → красный на гварде членства; с дополнительно перевёрнутым ожиданием гварда — красный на самом запросе с `permission denied for function pg_ls_archive_statusdir (SQLSTATE 42501)`
  - M4 `SetupTestRole(conn, backlogRoleNoRole, true)` на **свежем контейнере** (отравляет кластер, контейнер выброшен) → `…_NoPrivilegeRole` красный на «Should be empty, but was [pg_monitor]»; с перевёрнутым ожиданием гварда — красный на «An error is expected but got nil»
  - M5 снят FILTER `.ready` → `Test_ArchivingBacklogQuery_Structure`
  - M6 снят множитель `pg_size_bytes(current_setting('wal_segment_size'))` → `Test_ArchivingBacklogQuery_Structure`
- Мутации M3/M4 прогонялись на копии дерева внутри свежего контейнера, поэтому рабочее дерево ими не затрагивалось; M1/M2/M5/M6 применялись к дереву и откатывались
