# Decisions Log: Pause Display (016)

Отчёты агентов о выполнении задач. Каждая запись создаётся агентом, выполнившим задачу.

---

## Task 01: Non-mutating data cell rendering

**Status:** Done
**Agent:** исполнитель задачи 01 (волна 1)
**Summary:** `printDataCell` больше не пишет обрезанное значение обратно в `s.Result.Values[rownum][i].String` — значение читается в локальную переменную, обрезается в ней и печатается из неё, так что рендер стал read-only. Байты вывода не изменились: это структурный фикс без видимых изменений поведения, и он снимает необходимость в глубокой копии кадра в задаче 03 (Decision 10).
**Deviations:** Нет отклонений от спека. Байтовая (не рунная) нарезка сохранена намеренно, как требует safety property.
**Tech debt:** Новый долг не заведён. Инвариант «массив может принадлежать снапшоту коллектора, UI обязан только читать» теперь держится на doc-комментарии `printDataCell` — писателей в `top/` не осталось, но типом это не выражено. Отдельно ревьюер отметил, что аргумент Low-severity у долга [029] (`docs/tech-debt.md:41`) опирается на «таблица перерисовывается каждый тик и самолечится», и пауза этот аргумент снимает — это уже учтено в tech-spec и отдано задаче 10.

**Verification:**
- `go test ./top/ -run 'printDataCell|printStatData'` → 10/10 функций passed, включая нетронутый `Test_printStatData_truncation`; `-race` чисто
- Негативный контроль: с `top/stat.go`, откаченным на HEAD, падают ровно `Test_printDataCell_doesNotMutateSource` и `Test_printDataCell_widenAfterTruncation`, а четыре новых edge-case теста проходят на обеих версиях — это и есть доказательство неизменности вывода
- Байтовая идентичность проверена отдельно: 726 строк дампов рендера (обрезка, без обрезки, пустые и невалидные значения, multi-byte, нулевая и отрицательная ширина) совпали до и после правки
- AC1: `grep -rn "Result.Values\[" top/ --include='*.go'` вне тестов даёт два хита, оба — чтения
- `go vet ./top/` чисто; `golangci-lint run ./top/` → 0 issues; `gosec ./top/...` чисто; `gofmt -l` пусто
- Полный `go test ./top/...` не гонялся: нужны живые фикстуры PostgreSQL на портах 21914-21919

**Reviews:**

*Round 1:*
- dev-code-reviewer: approved_with_suggestions, 0 critical / 0 major / 3 minor → [016-feat-pause-display-task-01-dev-code-reviewer-review.json](016-feat-pause-display-task-01-dev-code-reviewer-review.json)
- dev-security-auditor: approved, 0 critical / 0 major / 3 minor → [016-feat-pause-display-task-01-dev-security-auditor-review.json](016-feat-pause-display-task-01-dev-security-auditor-review.json)
- dev-test-reviewer: needs_improvement, 0 critical / 1 major / 5 minor → [016-feat-pause-display-task-01-dev-test-reviewer-review.json](016-feat-pause-display-task-01-dev-test-reviewer-review.json)

*Round 2:*
- dev-code-reviewer: approved_with_suggestions, 0 critical / 0 major / 1 minor → [016-feat-pause-display-task-01-dev-code-reviewer-review-round2.json](016-feat-pause-display-task-01-dev-code-reviewer-review-round2.json)
- dev-test-reviewer: passed, 0 critical / 0 major / 2 minor → [016-feat-pause-display-task-01-dev-test-reviewer-review-round2.json](016-feat-pause-display-task-01-dev-test-reviewer-review-round2.json)

Major раунда 1 (ветка нулевой/отрицательной ширины из AC3 не покрыта нигде в `top/`) закрыт прямым тестом `Test_printDataCell_zeroOrNegativeWidth`. Оба ревьюера раунда 2 независимо показали мутацией, что этот тест **не** пиннит порядок проверок: обе его ширины используют 5-байтное значение, поэтому и текущая, и «поднятая» версия guard'а возвращают ошибку. Добавлен `Test_printDataCell_zeroWidthShortValueDoesNotError` (пустое значение при ширине 0 → без ошибки, вывод `"  "`) — мутант с поднятым guard'ом убивается только им, а порядок проверок задача требует сохранить дословно. Заодно закрыт случай `Valid: false`.

Остальные находки — точечные тесты на границы, которые дают уникальное покрытие: `exactWidthNotTruncated` (единственный ловит `>` → `>=`), `multiByteIsByteSliced` (единственный ловит переход на рунную нарезку), `truncatedCellExactBytes` (локализует регрессию до ячейки). Два комментария, которые обещали больше, чем пиннят тесты, исправлены по замечаниям раунда 2. Предсуществующие тесты не правились и не ослаблялись: диф `top/stat_test.go` содержит 0 удалённых строк.

---

## Task 02: Pause flag, `Space` binding and the `[PAUSED]` token

**Status:** Done
**Agent:** исполнитель задачи 02 (волна 1)
**Summary:** Добавлено поле `config.paused atomic.Bool` с комментарием о горутинах-писателях (gocui) и читателе (`statLoop`, задача 03). Создан `top/pause.go` с двумя функциями: `pauseToken` (nil-safe, ровно один вариант `[PAUSED]` — именно это делает маркер недеградируемым) и `togglePause` (переключает флаг и делает ровно одну тихую перерисовку cmdline через `printCmdline(g, "")`, без текстового сообщения — класс дефектов [027]). Токен подключён в `cmdlineTokens` левее фильтр-токена, `composeCmdline`/`renderCmdlineTokens` не тронуты. В `top/keybindings.go` одна строка: `{"sysstat", gocui.KeySpace, togglePause(app)}` — константа, не руна `' '` (Decision 13).
**Deviations:** Нет отклонений от спека. Одна рекомендация ревьюера отклонена осознанно (см. ниже).
**Tech debt:** Пакетное, не этой задачи: ни один юнит-тест в `top/` не пиннит запись в cmdline из какого-либо key handler — `writeCmdline` выходит на nil-Gui, а `gocui.View` в юнит-тесте не сконструировать. Та же дыра у `toggleVerbose` (`top/verbose.go:29-31`). Одна индиректа на уровне `writeCmdline` закрыла бы это сразу для всех обработчиков; регистрация в `docs/tech-debt.md` оставлена вне scope задачи.

**Открытый пункт для задачи 10 (стенд):** три критерия не достижимы юнит-тестом и все отложены на один и тот же прогон — (1) `togglePause` делает ровно одну запись в cmdline и без текста; (2) ветка `layout` возвращает маркер после выхода из pager/editor через `printCmdline(app.ui, "%s", app.uiError)`; (3) привязка `gocui.KeySpace` в контексте `"sysstat"` вообще срабатывает (таблица `keys` — локальный слайс, покрытия нет by construction). Стенд-прогон нужно сделать явным для всех трёх.

**Reviews:**

*Round 1:*
- dev-code-reviewer: approved_with_suggestions, 0 critical / 0 major / 4 minor → [016-feat-pause-display-task-02-dev-code-reviewer-review.json](016-feat-pause-display-task-02-dev-code-reviewer-review.json)
- dev-security-auditor: approved, 0 critical / 0 major / 3 minor → [016-feat-pause-display-task-02-dev-security-auditor-review.json](016-feat-pause-display-task-02-dev-security-auditor-review.json)
- dev-test-reviewer: needs_improvement, 0 critical / 2 major / 3 minor → [016-feat-pause-display-task-02-dev-test-reviewer-review.json](016-feat-pause-display-task-02-dev-test-reviewer-review.json)

*Round 2:*
- dev-test-reviewer: passed, 0 critical / 0 major / 1 minor → [016-feat-pause-display-task-02-dev-test-reviewer-review-round2.json](016-feat-pause-display-task-02-dev-test-reviewer-review-round2.json)

Major 2 (тест Decision 8 не касался `layout`) исправлен: `Test_cmdlineMarkerAfterUIRebuild` теперь берёт ошибку из реального `layout(&app{config: c, ui: &gocui.Gui{}})(nil)` — `gocui.Gui.Size()` читает два поля структуры, поэтому ветка 0×0 достижима без терминала. Подтверждено мутацией: замена `fmt.Errorf("")` на непустую ошибку роняет тест.

Major 1 (перерисовка в `togglePause` не покрыта) отклонён и ревьюером принят как отклонённый: предложенный seam `var cmdlineRefresh = ...` считает только вызовы, прошедшие через него, поэтому прямой второй `printCmdline` его не сломает — то есть он пиннит лишь половину критерия, причём не ту, которую задача называет дефектом [027]. Вместо seam дыра явно названа в doc-комментарии `togglePause` и вынесена в техдолг как пакетная.

Minor-находки (тавтологичное ожидание в подтесте, ассерты, которые не могут упасть, `Store(!Load())` без CAS) закрыты: ожидание переписано на литерал, лишние ассерты убраны, а безопасность read-modify-write задокументирована в `top/pause.go` (единственный писатель — gocui-горутина; атомик существует ради читателя-воркера).

**Verification:**
- `go test ./top/ -run '[Pp]ause|[Cc]mdline' -race -v` → 10/10 функций passed, включая `Test_pauseToken`, `Test_togglePause`, `Test_cmdlineTokensPauseNeverDegrades`, `Test_cmdlineMarkerAfterUIRebuild` и предсуществующие `Test_composeCmdline*`
- `go vet ./top/...` → чисто (copylocks не срабатывает: `config` везде передаётся как `*config`)
- `golangci-lint run` → 0 issues; `gofmt -l top/` → пусто; `go build ./...` → ок
- Полный `go test ./top/...` не гонялся: нужны живые фикстуры PostgreSQL на портах 21914-21919

---

## Task 03: The gate, the frame store and the shared render core

**Status:** Done
**Commit:** 8ae86f3 (+ 444a080 — правки по ревью)
**Agent:** gatekeeper (волна 2)
**Summary:** Селект-цикл вынесен из `doWork` в `statLoop(ctx, uiExit, statCh, paused, render, repaint)` с причиной выхода — в паузе кадр принимается, выбрасывается и отвечается перерисовкой, приём остаётся безусловным, а `wg.Wait()` по-прежнему только на ctx-выходе. `frameStore` живёт на `app`, пишется и читается исключительно в gocui-горутине (публикация в хвосте `renderFrame`, чтение внутри замыкания перерисовки), без единого примитива синхронизации; замыкание всегда возвращает `nil`, а об ошибке сообщает защёлка ровно один раз на переход. `renderFrame` — общее ядро обоих путей, уже принимающее и отметку времени, и источник logtail (швы для задач 04 и 09), плюс закрыт голый `statCh <-` в `collectStat`.
**Deviations:**
- `publishFrame` пока принимает только `(store, params, frame)` и не берёт буфер/путь logtail: захват буфера — задача 09, которая владеет и `top/stat.go`, и `top/pause.go`, так что расширение хелпера внутри её периметра. Поля `logBuf`/`logPath` в `renderParams` уже есть и на пути перерисовки уже потребляются.
- Промежуточное поведение logtail в волнах 2-4: пока буфер не заполняется, перерисовка отдаёт пустой источник, `printLogtail` ничего не печатает и не вызывает `v.Clear()` (`top/stat.go:1230-1232`), поэтому панель сохраняет прежнее содержимое. Это документированное правильное промежуточное состояние из раздела Edge cases задачи, а не дефект — «чинить» его чтением файла на перерисовке запрещено.
- Защёлка сделана методом `frameStore.recordRepaint` (задача разрешает «метод на сторе или отдельный тип рядом»): отдельный тип ради одного bool не заводился.
- Источник logtail в `renderParams` выражен дискриминатором `fromFile` плюс сохранённым буфером, а не функцией-источником: функция была бы вынуждена замыкаться на `g`/`app`, которых нет в точке построения параметров.
- Ревью запускал не исполнитель, а тимлид, поэтому фаза Post-work скилла `code-writing` (спавн ревьюеров) исполнителем пропущена намеренно.

**Tech debt:** Новый долг не заведён. Отмечены две предсуществующие вещи, обе явно вне периметра задачи: закрытый коллектором `statCh` даёт нулевые значения в плотном цикле, пока не выберется `ctx.Done()` (сегодняшнее поведение, задача запрещает добавлять ветку закрытого канала), и ветка `valid == true` замыкания перерисовки недостижима из юнит-теста, потому что `*gocui.Gui`/`*gocui.View` не сконструировать вне пакета gocui — ровно то ограничение, ради которого защёлка и вынесена в отдельно вызываемую единицу. Обе пойдут на стенд в задаче 10.

**Reviews:**

*Round 1:*
- dev-code-reviewer: approved, 0 critical / 0 major / 2 minor → [016-feat-pause-display-task-03-dev-code-reviewer-review.json](016-feat-pause-display-task-03-dev-code-reviewer-review.json)
- dev-security-auditor: approved, 0 critical / 0 major / 1 minor → [016-feat-pause-display-task-03-dev-security-auditor-review.json](016-feat-pause-display-task-03-dev-security-auditor-review.json)
- dev-test-reviewer: passed, 0 critical / 0 major / 1 minor → [016-feat-pause-display-task-03-dev-test-reviewer-review.json](016-feat-pause-display-task-03-dev-test-reviewer-review.json)

Обе минорки dev-code-reviewer применены в 444a080: перевёрстан комментарий EXTENSION POINT у `renderParams` и `Test_statLoop_exitOnUIExit` теперь джойнит свою горутину-продюсера вместо того, чтобы оставлять её на растерзание teardown (обе ветки её `select` завершают горутину, поэтому джойн не может зависнуть; утверждение теста — только причина выхода — не менялось). Минорка dev-security-auditor (плотный цикл на закрытом `statCh`) и минорка dev-test-reviewer (недостижимая ветка `valid == true`) не применялись: первая — предсуществующее поведение, явно выведенное задачей из области видимости, вторая — врождённое ограничение gocui, которое задача заранее учитывает.

**Verification:**
- `go test ./top/ -run 'statLoop|frameStore|repaint|renderFrame|Latch' -race -count=5` → 8 функций × 5 прогонов passed, гонок нет
- Мутационная проверка пяти несущих свойств: снятие guard'а `!p.publish`, удаление проверки `valid`, удаление подавления повторов в защёлке, рендер вместо отбрасывания в гейте и схлопывание двух причин выхода — каждая мутация роняет минимум один тест
- `go vet ./top/` → чисто; `go build ./...` → ок; `gofmt -l top/` → пусто
- `make build` → ок; `make lint` → 0 issues + gosec чисто; `make vuln` → 0 уязвимостей
- `grep -n "statCh <-" top/stat.go` → два хита, оба внутри `select` с `ctx.Done()`; `grep -n "app\.frame\|frameStore" top/*.go` → ни одного хита внутри `statLoop`
- Весь пакет с `-race`, за вычетом пяти файлов, требующих живого Postgres (`reload_test.go`, `reset_test.go`, `signal_test.go`, `report_test.go`, `top_test.go`) → ок; голый `go test ./top/` падает на фикстурах, как и описано в задаче

---

## Task 04: Frozen header clock

**Status:** Done
**Commit:** efa4c4c
**Agent:** clocksmith (волна 3)
**Summary:** `renderSysstat` больше не читает часы сам — отметка времени кадра приходит параметром `at` (последним, после `refresh`), `printSysstat` её просто пробрасывает, а единственная продовая точка вызова внутри `renderFrame` передаёт уже существующее поле `p.at` из `renderParams` задачи 03. Новых захватов `time.Now()` не добавлено: на живом пути он остаётся ровно один, в замыкании `printStat`, и кормит одновременно стор и шапку, а перерисовка переиспользует сохранённую отметку — именно это и замораживает часы. Байты строки 1 при заданной отметке не изменились: раскладка, `refresh: %ds` и формат load average прежние.
**Deviations:**
- Позиция параметра — в конец сигнатуры, после `refresh`, а не перед ним. Задача явно разрешала любую из двух («either position is fine as long as both functions agree»); выбран вариант, при котором порядок существующих аргументов не сдвигается и пять механических правок в тестах становятся чистым добавлением.
- В `top/stat_test.go` заведена пакетная переменная-фикстура `var testRenderTime` вместо литерала в каждой из пяти точек. Задача называла эти правки «механическими» и имени фикстуры не задавала. Значение неизменяемое и передаётся по значению, состояние между тестами не течёт; ни один ассерт не ослаблен и не удалён — диф `top/stat_test.go` не содержит ни одной удалённой строки ассерта.
- Свойство «перерисовка печатает замороженные часы над замороженными данными» доказано **композиционно**, а не одним тестом: два теста этой задачи закрывают чистоту `renderSysstat` от (s, at), а `Test_frameStore_publishOnlyOnLivePath` и `Test_frameStore_repaintRendersIdenticalBytes` в `top/pause_test.go` — что перерисовка не переставляет отметку. Соединить их одним тестом нельзя: `renderFrame` достижим только через живой `*gocui.Gui`. Это записано как известный пробел, а не закрыто.
- Ревью запускал тимлид, поэтому фаза Post-work скилла `code-writing` (спавн ревьюеров) исполнителем пропущена намеренно.

**Tech debt:** Новый долг не заведён. Зафиксирован один пробел покрытия, врождённый для gocui и предусмотренный самой задачей: сквозной инвариант из третьего пункта Deviations не имеет автоматического триггера — если изменить любой из трёх тестов, остальные два этого не заметят. Единственная сквозная проверка — ручной стенд-прогон задачи 10. Отклонённый задачей вариант «дважды вызвать `renderSysstat` с одним `at`» был бы тавтологией: он остаётся зелёным даже с возвращённым `time.Now()` на пути перерисовки.

**Reviews:**

*Round 1:*
- dev-code-reviewer: approved, 0 findings → [016-feat-pause-display-task-04-dev-code-reviewer-review.json](016-feat-pause-display-task-04-dev-code-reviewer-review.json)
- dev-security-auditor: approved, 0 findings → [016-feat-pause-display-task-04-dev-security-auditor-review.json](016-feat-pause-display-task-04-dev-security-auditor-review.json)
- dev-test-reviewer: passed, 0 critical / 0 major / 1 minor → [016-feat-pause-display-task-04-dev-test-reviewer-review.json](016-feat-pause-display-task-04-dev-test-reviewer-review.json)

Оба кодовых ревьюера независимо воспроизвели мутационную проверку (вернуть `time.Now()` в `renderSysstat` → оба новых теста краснеют → откатить), а dev-test-reviewer отдельно построчно сверил все пять предсуществующих точек вызова и подтвердил, что ослабленных или удалённых ассертов нет. Единственная минорка — композиционность доказательства из третьего пункта Deviations; кода она не требовала, ревьюер предложил лишь перекрёстную ссылку для будущих читателей. Ссылка добавлена: `top/pause_test.go` уже указывал на тесты задачи 04, встречный указатель теперь стоит в `top/stat_test.go` (блок COMPOSITIONAL PROOF, называющий все три теста и причину, по которой они не сводятся к одному).

**Verification:**
- `go test ./top/ -run 'Sysstat' -race -v` → 13 функций passed, включая оба новых теста и все восемь, идущих через `verboseSysstatLines`
- Мутационная проверка фальсифицируемости: с возвращённым `time.Now()` в `renderSysstat` краснеют оба новых теста, пять предсуществующих остаются зелёными (они намеренно не пиннят точную отметку); откат подтверждён
- `grep -n 'time.Now()' top/stat.go` → внутри `renderSysstat` ни одного; на живом пути ровно один захват (замыкание `printStat`, задача 03); в `top/ui.go` — ноль
- `go vet ./top/` → чисто; `go build ./...` → ок; `gofmt -l top/` → пусто
- `make build` → ок; `make lint` → 0 issues + gosec чисто (бинарь `golangci-lint` не в PATH по умолчанию, нужен `~/go/bin`)
- `git status --short` → из кода тронуты только `top/stat.go` и `top/stat_test.go`; `top/pause.go` и `top/help.go` не тронуты
- Полный `go test ./top/...` не гонялся: нужны живые фикстуры PostgreSQL на портах 21914-21919

---

## Task 05: Lifting the pause in handlers that need fresh data

**Status:** Done
**Commit:** d0bc90d (+ 2d2b973 — правки по ревью)
**Agent:** lifter (волна 3)
**Summary:** В `top/pause.go` добавлены два хелпера снятия — `liftPause(config)` (только сбрасывает флаг, без `*gocui.Gui`, потому что у `viewSwitchHandler` и `changeQueryAge` его нет) и `liftPauseRefresh(g, config)` (сброс через `CompareAndSwap(true, false)` и ровно одна тихая перерисовка cmdline только при удавшемся свопе), и расставлены по одиннадцати местам строго ниже всех ранних возвратов, так что действие, которое ничего не изменило, заморозку сохраняет. Вместе с ними поехали два причинных фикса: ветка логтейла `showExtra` больше не пишет в `config.logtail` до своих ранних возвратов (локальное значение `stat.Logfile`, одно присваивание после успешного `Open`), а `decreaseWidth` проверяет границы `config.view.Cols` перед индексированием — предсуществующий креш, который пауза превращала из двухсекундного окна в бессрочное.
**Deviations:**
- Отредактирован комментарий к чужому тесту `Test_frameStore_repaintRendersIdenticalBytes` (задача 03, файл в этой волне принадлежит задаче 05): обоснование «`renderSysstat` всё ещё зовёт `time.Now()`» устарело после задачи 04. Правка санкционирована тимлидом, только комментарий, ни одно утверждение не менялось.
- `Test_menuConfPathDoesNotLift`, предписанный TDD-якорем задачи (`:167-170`), удалён по итогам ревью и **не заменён** другим тестом (маршрут (a) из двух предложенных). Он утверждал на локальном `config`, который `editPgConfig` не получает — в её сигнатуре нет параметра `*config`, — то есть не мог упасть ни при какой регрессии и при этом читался как страж самого ошибкоопасного исключения фичи. На его месте оставлен комментарий с настоящей гарантией: две сигнатуры (`editPgConfig` нечего снимать; ветка зовёт её напрямую, мимо `viewSwitchHandler`), ревью диффа `top/menu.go`/`top/pgconfig.go` и шаг 7 стенда. Маршрут (b) рассмотрен и отклонён: `menuSelect` требует настоящего `*gocui.View` для `v.Cursor()`, а заводить шов ради тестируемости задача прямо запрещает.
- Форма `liftPauseRefresh` от спека **не** отклонялась: `CompareAndSwap` реализован как предписано, поэтому строки-обоснования отклонения не требуется.
**Tech debt:** Новый долг не заведён. Без unit-покрытия осознанно остались четыре пути, все закрыты иначе: `S` на локальном подключении (сразу под guard'ом живой `QueryRow`) и два глубоких no-op'а логтейла (`:51`/`:54`/`:58`, за `GetPostgresCurrentLogfile`) — шагом 7 стенда плюс сам причинный фикс, после которого до ранних возвратов не остаётся ни одной записи в `config.*`; ветка `menuConf` — сигнатурами, ревью диффа и стендом (см. Deviations). Ревьюер безопасности отдельно подтвердил как предсуществующие и вне периметра задачи две вещи в `top/extra.go`, идентичные в `d0bc90d~1`: утечку дескриптора логтейла при прямом переключении между панелями и TOCTOU между `os.Stat` и `Open`. Принятая мелочь: `Test_showExtraCloseLifts` намеренно оставляет припаркованную горутину — это записано в комментарии, чтобы будущий детектор утечек получил исключение, а не «починку».

**Reviews:**

*Round 1:*
- dev-code-reviewer: approved_with_suggestions, 0 critical / 0 major / 1 minor → [016-feat-pause-display-task-05-dev-code-reviewer-review.json](016-feat-pause-display-task-05-dev-code-reviewer-review.json)
- dev-security-auditor: approved, 0 critical / 0 major / 3 minor → [016-feat-pause-display-task-05-dev-security-auditor-review.json](016-feat-pause-display-task-05-dev-security-auditor-review.json)
- dev-test-reviewer: needs_improvement, 0 critical / 1 major / 1 minor → [016-feat-pause-display-task-05-dev-test-reviewer-review.json](016-feat-pause-display-task-05-dev-test-reviewer-review.json)

Major и единственная minor-находка code-reviewer'а — это один и тот же дефект, найденный двумя ревьюерами независимо: вакуумный `Test_menuConfPathDoesNotLift`. Исправлен в 2d2b973 удалением (разбор маршрутов — в Deviations). Minor тест-ревьюера про припаркованную горутину применён комментарием. Три minor'а ревьюера безопасности не применялись: две — предсуществующие и явно вне периметра, третья — подозрение на флейк, не воспроизведённое (см. Verification).

*Round 2:*
- dev-test-reviewer: passed, 0 critical / 0 major / 2 minor → [016-feat-pause-display-task-05-dev-test-reviewer-review-round2.json](016-feat-pause-display-task-05-dev-test-reviewer-review-round2.json)

Обе minor'ки консультативные, одна применена. Ревьюер не поверил обоснованию на слово, а проверил его: нулевой `&gocui.View{}` конструируется вне пакета gocui и отвечает на `v.Cursor()`, так что названный в комментарии механизм неверен; загнав `menuSelect` в настоящую ветку `menuConf`, он упёрся в настоящий блокер — безусловный `return menuClose(g, v)` на всех ветках, а `menuClose` зовёт `g.DeleteView`/`g.SetCurrentView` на живом `*gocui.Gui`, который бывает только из `gocui.NewGui` с терминалом. Вывод (тест невозможен без запрещённого шва) устоял, обоснование в комментарии исправлено на верное. Вторая minor'ка — подтверждение, что заметка про припаркованную горутину закрыта в раунде 1.

**Verification:**
- Прицельный прогон из `verify` с `-race` → 19 функций passed, включая все восемь групп TDD-якоря и предсуществующие `Test_decreaseWidth`/`Test_increaseWidth`/`Test_toggleVerbose`
- Весь пакет с `-race` за вычетом пяти файлов, требующих живого Postgres → ок; голый `go test ./top/...` не гонялся (фикстуры на портах 21914-21919)
- Мутационная проверка шести несущих свойств: снятие guard'а границ в `decreaseWidth`, подъём снятия выше guard'ов `toggleSysTables` и `changeQueryAge`, удаление снятия из `viewSwitchHandler` и из пути закрытия `showExtra`, подъём снятия выше `return err` от `logtail.Close()` — каждая мутация роняет именной тест. Те же шесть независимо воспроизведены тест-ревьюером
- Подтверждение по одиннадцати местам: `grep -n 'liftPauseRefresh(' top/*.go | grep -v _test` → 3 вызова (`orderKeyLeft`, `orderKeyRight`, путь закрытия `showExtra`) плюс определение; `grep -n 'liftPause(' top/*.go | grep -v _test` → 8 вызовов плюс определение; `grep -n 'liftPause' top/menu.go top/pgconfig.go` → пусто. Каждое место сверено с таблицей Details поимённо, включая «кто печатает cmdline на этом пути»
- Подтверждение формы `liftPauseRefresh`: `CompareAndSwap(true, false)`, перерисовка внутри ветки удавшегося свопа — как предписано, без отклонений
- `go vet ./top/` → чисто; `go build ./...` → ок; `gofmt -l top/` → пусто; `make build` → ок; `make lint` → 0 issues + gosec чисто
- `git show --stat` обоих коммитов → только семь разрешённых файлов; `top/stat.go`, `top/help.go`, `top/ui.go`, `top/dialog.go`, `top/menu.go`, `top/keybindings.go` не тронуты
- Подозрение ревьюера безопасности на флейк не воспроизвелось: 4 параллельных процесса × `-count=10` под нагрузкой → 40 итераций, ни одного FAIL и ни одного `DATA RACE`

---

## Task 06: Help screen entry

**Status:** Done
**Commit:** cd542da (+ fb73a34 — правки по ревью)
**Agent:** scribe (волна 3)
**Summary:** В блок `general actions:` встроенной справки добавлена двухстрочная запись про `Space` — посимвольно та, что согласована в интервью по user-spec, сразу после строки `[,]` и перед `C,E,R config:`. Перечень снимающих паузу действий записан как `sort, screen switch, ',', I, A, v, B, N, F, L`: `S` и выбор пункта в меню `D`/`X`/`P`/`J` намеренно свёрнуты в формулировку `screen switch` — осознанный компромисс между полнотой и шириной экрана, санкционированный задачей; ревьюер независимо перечислил все 11 точек вызова `liftPause`/`liftPauseRefresh` из задачи 05 и подтвердил, что документированный набор совпадает с реализованным. Создан `top/help_test.go` — до этой задачи `helpTemplate` не проверялся ни одним тестом, хотя эта константа является форматом для `fmt.Fprintf`.

**Deviations:**
- Две проверки сверх TDD Anchor: расположение записи (ровно между строками `[,]` и `C,E,R`) и ширина обеих строк ≤ 80 колонок. Обе закрывают критерии приёмки, которые сам TDD Anchor не называл; выравнивание при этом по-прежнему сравнивается с вычисленной колонкой соседней строки, а не с магическим числом.
- В таблице перечня действий вместо голых букв используются разграниченные подстроки (`" I,"`, `" A,"`, `" v,"`, `" L)"`). Голые буквы на строке продолжения тоже проходят, но разграниченная форма переживает переформулировку соседнего текста, не деградируя молча.
- Визуальная проверка (`./bin/pgcenter top`, нажатие `h` на живом кластере) из Verification Steps **не выполнялась** — нужен терминал и живой Postgres; отложена в задачу 10. Текстовая проверка сделана: 41 строка рендера, описания всего блока выстроены в колонку 22, обе новые строки ровно по 80 символов.
- Справка выросла с 39 до 41 строки рендера. Следствие: порог, ниже которого нижняя строка `Type 'q' or 'Esc' to continue.` обрезается, сместился с терминала в 39 строк на терминал в 41 строку (см. Tech debt).
- Полный `go test ./top/...` не гонялся: нужны живые фикстуры PostgreSQL на портах 21914-21919. Прогон точечный по `-run`, как предписывает задача.
- Ревьюеров запускал тимлид, а не исполнитель, поэтому фаза Post-work скилла `code-writing` (спавн ревьюеров) исполнителем пропущена намеренно.

**Tech debt:** Новый долг не заведён, зафиксированы две предсуществующие вещи. Первая — вьюха `help` создаётся без `Wrap` и `Autoscroll`, и на неё не навешано ни одной клавиши скролла (`top/help.go:54`), поэтому на терминале ниже ~41-43 строк низ справки, включая строку «как закрыть», молча обрезается и доскроллить до него нечем; эта задача не создала проблему, но расширила её на две строки — чинить нужно вьюпорт, а не текст, и это вне периметра задачи. Вторая — остальные ~35 строк `helpTemplate` по-прежнему не покрыты ничем: разумный следующий шаг не в том, чтобы пиннить каждую строку литералом, а в лёгкой структурной проверке (у всех строк блока описание начинается с одной колонки), если справку будут править снова.

**Reviews:**

*Round 1:*
- dev-code-reviewer: approved_with_suggestions, 0 critical / 0 major / 1 minor → [016-feat-pause-display-task-06-dev-code-reviewer-review.json](016-feat-pause-display-task-06-dev-code-reviewer-review.json)
- dev-test-reviewer: needs_improvement, 0 critical / 0 major / 2 minor → [016-feat-pause-display-task-06-dev-test-reviewer-review.json](016-feat-pause-display-task-06-dev-test-reviewer-review.json)

`dev-security-auditor` в наборе нет by design: задача правит display-only строковую константу и не обрабатывает ввод (решение tech-spec, примечание после Task 6).

Обе минорки dev-test-reviewer применены в fb73a34, обе — без изменения поведения справки. Первая: свободный текст первой строки (`actions that need fresh data`) не пиннился ничем, и переформулировка прошла бы мимо всех трёх тестов, хотя задача требует запись дословно; теперь описание строки сравнивается целиком, но именно описание, нарезанное по вычисленной колонке, а не сырая строка — литерал с отступом ломался бы на переотбивке блока, которую вычисляемая проверка выравнивания как раз обязана переживать. Вторая: комментарий у `Test_helpTemplate_formatVerbs` обещал защиту от случайного `%`, тогда как `go vet` (его `go test` гоняет при сборке) ловит это раньше и жёстче; проверено мутациями на месте — голый `%` даёт `format % o reads arg #2, but call has 1 arg` до того, как выполнится хоть один ассерт, а удвоенный `%%` проходит vet чисто и ловится только счётчиком в этом тесте. Ассерты не тронуты, исправлен комментарий.

Минорка dev-code-reviewer (обрезание справки на низком терминале) не применялась осознанно: предсуществующее ограничение вьюхи, расширенное на две строки, — записано в Tech debt.

**Verification:**
- `go test ./top/ -run 'helpTemplate' -race -count=5` → 3 функции × 5 прогонов passed (`Test_helpTemplate_pauseEntry`, `_pauseLiftingActions`, `_formatVerbs`), гонок нет
- Красный шаг TDD был настоящим: тесты компилировались и падали на отсутствии записи, а не на ошибке сборки
- Мутационная проверка, четыре штуки, каждая роняет ровно ожидаемый тест: убрать `v` из перечня → `pauseLiftingActions/verbose_mode`; сдвинуть запись на колонку влево → `pauseEntry`; заменить `need` на `require` → `pauseEntry` (до правок по ревью эта мутация проходила молча); удвоить `%%` → `formatVerbs`
- `go vet ./top/` → чисто; `go build ./...` → ок; `gofmt -l top/help.go top/help_test.go` → пусто
- `make build` → ок; `make lint` → 0 issues + gosec чисто
- Геометрия: 41 строка рендера (было 39), максимальная ширина шаблона 100 не изменилась, обе новые строки по 80 колонок, описания блока `general actions:` в колонке 22
- Изоляция от параллельных задач: `git show --stat` подтверждает ровно два файла; в момент прогона gate соседняя задача 05 держала пакет в некомпилируемом состоянии (свой красный шаг в `top/pause_test.go`), поэтому gate дополнительно прогнан на чистом дереве, развёрнутом из `git archive HEAD` — там vet, build, `make build`, `make lint` и все три теста зелёные

---

## Task 07: Repaint on terminal resize

**Status:** Done
**Commit:** d12dd96
**Agent:** resizer (волна 4)
**Summary:** В `top/ui.go` добавлен `resizeDetector` — `lastX/lastY` плюс `observe(paused, x, y) bool`, который не касается ни `app`, ни gocui: размер сначала запоминается, потом возвращается `paused`, а на неизменившемся размере ответ всегда `false`. В `layout` это одна переменная рядом с `verboseTooShortShown` (состояние на один `Gui`, обнуляемое перезапуском UI) и один `if` перед `return nil` — после блока `extra` и ниже guard'а нулевой геометрии, — вызывающий существующий `repaintStored(app)`. Тот же детектор бесплатно закрывает второй критерий приёмки: нулевое стартовое состояние делает первый проход нового `Gui` изменением, поэтому замороженный кадр возвращается на экран после выхода из pager/editor/psql.

**Deviations:**
- Отдельный `sizeChanged(lastX, lastY, x, y) bool` **не** заведён — это предписанное самой задачей отклонение от буллета Testing Strategy в tech-spec, и оно фиксируется здесь по её же требованию. Сравнение — одно выражение, `observe` тестируется таблицей целиком, поэтому второй именованный слой был бы обёрткой без собственного вызывающего.
- Тест на latch отказа перерисовки в `top/ui_test.go` **не** добавлен, хотя буллет «What to do» его требует: TDD Anchor и Details той же задачи запрещают его дважды и явным аргументом — паттерн `-run 'Test_repaintFailureLatch…'` совпал бы с тестом задачи 03 по префиксу, и верификация задачи 07 позеленела бы, не будучи написанной. Задача 03 отгрузила latch отдельно вызываемой единицей (`frameStore.recordRepaint`) вместе с `Test_repaintFailureLatch_reportsOncePerTransition` (`top/pause_test.go:178`), так что критерий приёмки закрыт её покрытием. Оба ревьюера — code и test — независимо прочитали задачу как противоречащую самой себе и независимо признали запрет из TDD Anchor главнее; тест-ревьюер дополнительно прогнал тест задачи 03 и подтвердил, что последовательность fail/fail/fail/success/fail даёт `true` ровно на переходе. Стоп-условие «задача 03 не отгрузила latch вызываемой единицей» не наступило, докладывать оркестратору нечего.
- Ссылки на строки в тексте задачи устарели, она писалась до волн 2-3: `layout` теперь на `top/ui.go:224`, а не `:138`; `top/ui_test.go` — 641 строка и 13 тестов, а не 365 и 9. Тесты дописаны в конец файла, после `Test_statLoop_exitOnContextCancel`, а не после `Test_printCmdlineNilGui`, который концом файла быть перестал.
- Единственная minor-находка code-reviewer'а (обоснование якобы продублировано в doc-комментарии типа и в комментарии у места вызова) осознанно не применялась. Пересечения по существу нет: doc-комментарий типа держит три факта про gocui, про состояние на один `Gui` и про порядок «запомнить раньше, чем ответить», а комментарий у места вызова говорит другое — почему кадру под паузой нужна перерисовка, почему она уходит через `g.Update` вместо синхронного рендера и почему стоит ниже guard'а нулевой геометрии. Находка помечена самим ревьюером как optional и cosmetic.
- Фазу Post-work скилла `code-writing` (спавн ревьюеров) исполнитель не выполнял: ревьюеров запускал тимлид.

**Tech debt:** Новый долг не заведён, зафиксированы две предсуществующие вещи. Первая — недетерминированный порядок доставки `g.Update`, класс дефектов [028]: `layout` может в одном проходе поставить в очередь и подсказку height-guard, и перерисовку по ресайзу, а gocui v0.5.0 запускает под каждый `Update` горутину и порядок между ними не определяет. Задача расширяет предсуществующий паттерн вторым эмиттером, но не создаёт его; на кону только то, чей транзиентный текст выиграет, — `[PAUSED]` собирается через `cmdlineTokens` в обоих случаях. Чинить, если когда-нибудь станет видно пользователю, нужно путь записи в cmdline, а не расставлять порядок здесь. Вторая — сама проводка в `layout` (вызов `observe` и вызов `repaintStored` под ним) не покрыта unit-тестом и покрыта быть не может: `layout` читает `app.ui.Size()`, а `*gocui.Gui` с ненулевым размером вне пакета gocui не сконструировать (`maxX`/`maxY` не экспортированы). Единственное её покрытие — стенд задачи 10, шаг 5 user-spec (`tmux resize-window` 190 → 60 → 190 под паузой); если стенд не будет прогнан, эта строка уедет в релиз без покрытия вообще.

**Reviews:**

*Round 1:*
- dev-code-reviewer: approved, 0 critical / 0 major / 1 minor → [016-feat-pause-display-task-07-dev-code-reviewer-review.json](016-feat-pause-display-task-07-dev-code-reviewer-review.json)
- dev-security-auditor: approved, 0 critical / 0 major / 1 minor → [016-feat-pause-display-task-07-dev-security-auditor-review.json](016-feat-pause-display-task-07-dev-security-auditor-review.json)
- dev-test-reviewer: passed, 0 critical / 0 major / 1 minor → [016-feat-pause-display-task-07-dev-test-reviewer-review.json](016-feat-pause-display-task-07-dev-test-reviewer-review.json)

Правок по итогам раунда не потребовалось, второго раунда нет. Все три minor'ки консультативные и разобраны выше: дубль обоснования в комментариях (отклонена по существу), [028] и непокрываемая проводка (обе в Tech debt). Ни один ревьюер не поверил обоснованиям на слово: аудитор безопасности прочитал вендоренный исходник gocui v0.5.0 и подтвердил, что до приложения не доходит ни одно событие ресайза и что поставленная в очередь перерисовка не может перезапустить сама себя; тест-ревьюер прогнал две мутации `observe` — «ответить раньше, чем запомнить» (баг «перерисовывать вечно») ловится шагом-повтором в каждом подтесте, а сравнение только по `x` роняет ровно кейс height-only.

**Verification:**
- `go test ./top/ -run 'Test_resizeDetector' -race` → 6/6 подтестов passed, гонок нет
- Красный шаг TDD был настоящим: до реализации прогон падал на `undefined: resizeDetector`, а не на упавшем ассерте
- Предсуществующие тесты `top/ui_test.go` (`Test_composeCmdline|Test_cmdlineTokens|Test_filterToken|Test_setCmdlineConfig|Test_printCmdlineNilGui`) → ок, ни один не тронут; `git diff --numstat top/ui_test.go` → `96 0`, только добавления
- Соседи из задач 02/03/05 с `-race` (`Test_statLoop*`, `Test_pauseToken`, `Test_togglePause`, `Test_repaint*`, `Test_frameStore*`, `Test_liftPause*`, `Test_liftingHandlers`, `Test_noOpHandlersKeepPause`, `Test_showExtra*`, `Test_cmdlineMarkerAfterUIRebuild`) → ок; последний важен отдельно, он гоняет настоящий `layout` и упирается в guard нулевой геометрии выше детектора
- `go vet ./top/` → чисто; `go build ./...` → ок; `gofmt -l top/` → пусто; `make build` → ок; `make lint` → 0 issues + gosec чисто
- `grep -n "func sizeChanged\|Latch\|latch" top/ui.go` → пусто; `grep -n "repaintStored" top/ui.go` → три хита, из которых новый ровно один (`:335`, внутри `layout` после блока `extra`), второй — колбэк `statLoop` из задачи 03 (`:129`), третий — комментарий
- `git show --stat d12dd96` → ровно `top/ui.go` и `top/ui_test.go`; `top/pause.go` не тронут, `top/dialog.go` параллельной задачи 08 в коммит не попал
- Полный `go test ./top/...` не гонялся: фикстурный кластер PostgreSQL на `127.0.0.1:21917` недоступен (connection refused). Проверено явно, а не предположено

---

## Task 08: Filter dialog under pause

**Status:** Done
**Commit:** 32d557a
**Agent:** dialogist (волна 4)
**Summary:** Ветка `dialogFilter` в `dialogFinish` получила хелпер `applyFilter(answer, config, repaint func()) string`: он зовёт `setFilter`, при поднятом флаге паузы дёргает `repaint()` и возвращает сообщение `setFilter` без изменений — второй записи в cmdline нет (класс дефектов [027]). Сам `setFilter` не тронут и push в `viewCh` не получил: симметричная правка прогнала бы `c.Reset()` и показала один кадр с почти нулевыми скоростями в **живом** режиме, где фильтром пользуются куда чаще, чем под паузой (Decision 4); перерисовка приходит голым `func()`, что и держит стор недосягаемым с этой точки вызова (Decision 1). Вторая половина задачи — не реализация, а **проверка**: бюджет ширины диалога из [015] выдерживает маркер `[PAUSED]` в префиксе без единой правки `dialogPromptFit`/`dialogInputX0`.

**Deviations:**
- `grep -n "viewCh" top/dialog.go` из Verification Steps больше не пустой — одно попадание в строке 253, **внутри doc-комментария** `applyFilter`. Убрать его нельзя: секция «What to do» той же задачи требует объяснить, почему этой ветке нужна перерисовка, а пяти её соседям нет, а объяснение — это ровно «они пушат на `viewCh`, а она нет». Code-reviewer оценил по существу: это проза, а не ссылка, ни кода, ни импорта, ни связности с `config_view.go` не добавляет; переформулировка предложена как косметическая опция. Отклонение **принято ревью, а не исправлено** — комментарий, который внятно объясняет асимметрию, дороже механически чистого grep'а.
- Сообщение об ошибке компиляции regexp в `Test_applyFilter_repaintsOnUnchangedFilter/invalid_regexp` сверяется точным литералом, а не префиксом. Это текст стандартной библиотеки, но так уже написан предсуществующий `Test_setFilter` (`top/config_view_test.go:421`) — выбрана согласованность с прецедентом, а не новая локальная конвенция. Тест-ревьюер подтвердил, что это не привнесённая задачей хрупкость (см. Tech debt).
- В отчёте исполнителя нижняя граница развёртки для префикса `{paused + filter}` названа **31** — арифметическая ошибка в отчёте, не в коде: `renderCmdlineTokens` склеивает токены **без** разделителя, поэтому префикс это `[PAUSED][F:datname]` = 19 рун, а граница = 19 + `minDialogInputWidth` + 1 = **30** (для `{paused}` — 19). Тест границу вычисляет, а не хардкодит, поэтому развёртка всё время шла от правильного значения; ревьюер проверил её на тесноту, прощупав `maxX == bound-1`.
- Двухрядная таблица в `Test_applyFilter_repaintsWhenPaused` (один валидный паттерн, меняется только флаг) была вынесена исполнителем как возможное отклонение — тест-ревьюер разобрал и постановил, что это **не** отклонение: формулировка самого TDD-якоря предписывает ровно такую форму.
- Полный `go test ./top/...` не гонялся: пакет требует живых фикстур PostgreSQL, без них `top/report_test.go:14` паникует на nil-соединении. Прогон точечный по `-run`, как предписывает задача.
- Ревьюеров запускал тимлид, а не исполнитель, поэтому фаза Post-work скилла `code-writing` (спавн ревьюеров) исполнителем пропущена намеренно.

**Tech debt:** Новый долг не заведён, зафиксированы две предсуществующие вещи. Первая — класс [028], недетерминированный порядок доставки `g.Update`: `dialogFinish` уже обрамляет каждую ветку двумя `printCmdline` (очистка и отчёт), а эта задача вставляет между ними третий эмиттер `g.Update` — но только на ветке фильтра и только под паузой. Ревьюер безопасности закрыл вопрос по существу: `repaintStoredFrame` на успешном пути не трогает вьюху `cmdline` вовсе (пишет в неё только на собственном пути отказа, отдельным событием), поэтому переупорядочивание не может ни испортить, ни съесть текст `Filters: ok`; худший исход — лишняя безвредная перерисовка уже актуального замороженного кадра. Общее лечение [028] эта точка вызова получит автоматически. Вторая — та самая сверка с текстом ошибки стандартной библиотеки, которую тест-ревьюер пометил `tech_debt: true`: чинить её имеет смысл не здесь, а разом в обоих местах (перейти на `strings.HasPrefix(message, "Filters: error parsing regexp")` и тут, и в `Test_setFilter`), если стиль последнего будут править. Вопрос ReDoS ревьюер безопасности закрыл по существу: движок `regexp` в Go построен на RE2 с линейной гарантией, катастрофический бэктрекинг невозможен — и это свойство `setFilter` в любом случае предсуществующее.

**Reviews:**

*Round 1:*
- dev-code-reviewer: approved_with_suggestions, 0 critical / 0 major / 1 minor → [016-feat-pause-display-task-08-dev-code-reviewer-review.json](016-feat-pause-display-task-08-dev-code-reviewer-review.json)
- dev-security-auditor: approved, 0 critical / 0 major / 1 minor → [016-feat-pause-display-task-08-dev-security-auditor-review.json](016-feat-pause-display-task-08-dev-security-auditor-review.json)
- dev-test-reviewer: passed, 0 critical / 0 major / 2 minor → [016-feat-pause-display-task-08-dev-test-reviewer-review.json](016-feat-pause-display-task-08-dev-test-reviewer-review.json)

Задача закрыта в один раунд, правок не потребовалось. Все три минорки консультативные и ни одна не применялась: попадание `viewCh` в комментарии и точный литерал ошибки regexp — принятые и обоснованные отклонения (см. Deviations), порядок доставки `g.Update` и вторая минорка тест-ревьюера — предсуществующие (см. Tech debt). Ещё одна минорка тест-ревьюера (нет строк «живой режим» для невалидного паттерна и пустого ответа) не применялась по его же разбору: guard в `applyFilter` — это `if config.paused.Load()`, он не зависит ни от возврата `setFilter`, ни от мутации карты, поэтому такая строка не прошла бы ни по одной ветке, которую существующие тесты пропускают.

**Verification:**
- Прицельный прогон из `verify` с `-race` → 14 функций passed: девять предсуществующих тестов `top/dialog_test.go` в исходном виде плюс пять новых; тест-ревьюер независимо повторил под `-race -count=5`
- Красный шаг TDD был настоящим: тесты падали на `undefined: applyFilter` в трёх точках вызова, реализация писалась после
- Мутационная проверка несущего свойства, проведена тест-ревьюером: замена перерисовки на снятие паузы внутри `applyFilter` роняет ровно `Test_applyFilter_repaintsWhenPaused` и `Test_applyFilter_repaintsOnUnchangedFilter` и не задевает ни одного другого теста пакета
- Теснота нижней границы развёртки подтверждена: на `maxX == bound-1` инвариант ломается законно (префикс один уже не помещается в бюджет) для обоих префиксов — `{paused}` и `{paused + filter}`
- Разница ширины подсказки под паузой ровно 9 колонок (маркер плюс разделяющий пробел), выведена из `composeCmdline(pausedTokens, "", 200)`, литерала `9` в тесте нет; обе подсказки предварительно проверены на то, что действительно обрезаны, иначе сравнение было бы вакуумным
- `setFilter` не тронут: `git diff` не показывает `top/config_view.go`; `grep -n "viewCh" top/config_view.go` — тот же набор пушей, что и на `master`
- `grep -c "printCmdline" top/dialog.go` → 8, столько же, сколько на `master`: ветка фильтра не добавила ни одной записи в cmdline
- `git diff top/dialog_test.go` → только добавления, 0 удалённых строк; единственное удаление в коммите — заменённая строка вызова `setFilter` в `top/dialog.go`
- `go vet ./top/` → чисто; `go build ./...` → ок; `gofmt -l top/` → пусто; `make build` → ок; `make lint` → 0 issues + gosec чисто (`exit=0`)
- Изоляция от параллельных задач: `git show --stat 32d557a` → ровно два файла, `top/dialog.go` и `top/dialog_test.go`; `top/ui.go` и `top/ui_test.go`, которые в этой волне держит задача 07, в индекс не попадали (staging только по явным путям)
- Отложено в стенд задачи 10 (здесь непроверяемо): пауза, `/`, ввод выражения, `Enter` → замороженный экран перерисовывается с меньшим числом строк, `[PAUSED]` остаётся на месте; то же на `-x 60` с активным фильтром

---

## Task 09: Logtail panel freeze and restore

**Status:** Done
**Commit:** 0ae64cf (+ 1b8de07 — правки по ревью)
**Agent:** tailcatcher (волна 5)
**Summary:** Последняя панель, содержимое которой не приходило из кадра, заморожена: под паузой лог-файл не трогается вовсе (ни `os.Stat`, ни `Logfile.Read`, ни `Reopen`, ни записи `logtail.Size`), а панель перерисовывается из буфера, сохранённого рядом с кадром — без него после перестроения UI (пейджер, редактор, `psql`) нижняя треть экрана возвращалась пустой под живым `[PAUSED]`, потому что показанные строки не хранились в приложении нигде, кроме строчного буфера самой вьюхи gocui. Захват и сброс сведены в один метод `frameStore.syncLogtail(show, path, buf)` с обоими предикатами внутри, а вызывается он на живой стороне `publishFrame` — того самого шва, который задача 03 нарезала как точку расширения. `printLogtail` разделён на тонкую обёртку над `*gocui.View` и писательское ядро `renderLogtail`, а по итогам ревью выбор источника вынесен в отдельную функцию `selectLogtail`.

**Deviations:**
- **Тронут `top/pause_test.go`**, которого нет в списке файлов задачи (само задание помечало это как известное отклонение). Оно к тому же вынужденное: расширение сигнатуры `publishFrame` ломает компиляцию файла, поэтому четыре предсуществующие точки вызова пришлось поправить по арности в любом случае. Ни один ассерт не ослаблен и не удалён — единственные удаления в диффе тестов — это те самые четыре строки вызова; тест-ревьюер сверил независимо. Заодно `Test_frameStore_publishOnlyOnLivePath` **усилен**: на перерисовочном вызове ему теперь подсовывается ДРУГАЯ пара logtail, и тест пиннит, что стор не тронут — это и есть критерий «перерисовка не пишет в стор».
- **Отдельной функции `repaintLogtail` нет**, хотя её имя фигурирует в имени теста. Задача разрешала это прямым текстом: шов задачи 03 отдаёт источнику `*gocui.View`, значит адаптируемся на границе обёртки (`printLogtail`), а io.Writer-ядром остаётся `renderLogtail` — по прецеденту `printSysstat`→`renderSysstat`. Отдельная `repaintLogtail(w io.Writer, p renderParams)` была бы однострочной обёрткой без продового вызова (перерисовке нужен `Clear` обёртки), то есть мёртвым кодом. Оба кодовых ревьюера оценили решение как верное прочтение escape-clause.
- **`publishFrame` вырос с 3 до 6 позиционных параметров** (`f, p, s, show, logPath, logBuf`). Свернуть `show` внутрь функции нельзя: задача 03 намеренно типизировала хелпер на `*frameStore`, а не на `*app`, чтобы он оставался юнит-тестируемым от голого `var f frameStore` — значит `ShowExtra` обязан приходить параметром. Ревью признало это согласованным с прецедентом пакета (`printSysstat`/`renderSysstat` берут 7) и оставило предложение про parameter-struct как необязательное: переоткрывать форму `renderParams` посреди фичи ради косметики задача 03 запрещает.
- **`selectLogtail` — добавление по итогам ревью, в тексте задачи его не было.** Раунд 1 показал (двумя независимыми мутациями), что ключевое свойство задачи ничем не запиннено: `Test_repaintLogtail_noFileAccess` звал `renderLogtail` напрямую, а сигнатура `renderLogtail` физически не может дотянуться до `config.logtail` — значит вся конструкция с реальным открытым файлом-«диверсантом» была инертной. Замена `if !p.fromFile {` на `if false {` в `renderFrame` (перерисовка уходит прямо в живое чтение файла — ровно та регрессия, ради которой Decision 7 и существует) оставляла весь прицельный прогон зелёным. Причина структурная: выбор источника сидел внутри `switch`'а `renderFrame`, недостижимого из юнит-теста, потому что `*gocui.Gui`/`*gocui.View` не сконструировать вне пакета gocui. Поэтому выбор вынесен в `selectLogtail(p renderParams, read logtailReader)`: всё, чему нужны `g` или `v` (геометрия `v.Size()` внутри `readLogfileRecent`, `v.Clear()`, поход `Reopen` в базу, запись ошибки в cmdline, бухгалтерия `logtail.Size`), осталось внутри thunk'а, наружу вышел только сам выбор. Сигнатуры `renderFrame` и `renderParams` не изменились, шов задачи 03 не тронут; побочный выигрыш — обе ветки сходятся в один вызов `printLogtail` вместо двух. Отклонение сознательное и одобрено обоими ревьюерами в раунде 2.
- **Сброс сохранённой пары при закрытии панели отклонением не является** — правило поднято в tech-spec, Decision 7, и выполнено: решение принимается одним безусловным вызовом ПОСЛЕ блока `if app.config.view.ShowExtra > stat.CollectNone`, потому что закрытая панель (`stat.CollectNone`) этот блок не входит вовсе, а это и есть главный случай сброса.
- Полный `go test ./top/...` не гонялся: пакет требует живых фикстур PostgreSQL на портах 21914-21919, без них `top/report_test.go:14` паникует на nil-соединении. Прогон точечный по `-run`, как предписывает задача.
- Ревьюеров запускал тимлид, а не исполнитель, поэтому фаза Post-work скилла `code-writing` (спавн ревьюеров) исполнителем пропущена намеренно.

**Tech debt:** Новый долг не заведён, зафиксированы две предсуществующие вещи, обе отданы в `/done`. Первая — **удерживаемый дескриптор и приколотый инод**: файл держится открытым всю паузу, поэтому ротированный лог сохраняет свой инод, и если новый файл успеет перерасти замороженный `logtail.Size` до снятия паузы, размерный детектор ротации не сработает и панель покажет устаревшие строки. Окно существует и сегодня, шириной в один интервал обновления, пауза его лишь расширяет; ревьюер безопасности отдельно проверил, что это НЕ утечка — `Reopen` всегда закрывает перед открытием, одновременно открыт ровно один дескриптор, циклы пауза/возобновление их не накапливают. Закрывать нужно проверкой идентичности (`os.SameFile`/mtime) вместо сравнения размеров — отдельное изменение, вне периметра. Названо в комментарии в коде. Вторая — **расширенное окно удержания сырого содержимого лога**: `frameStore.logBuf` теперь держит последнее непустое чтение всю паузу и рендерится тем же сквозным пропуском escape-последовательностей, что и раньше. Нового стока нет — буфер остаётся в памяти процесса, на диск не пишется и в подсистему record/report не попадает, — меняется только длительность: строка, оказавшаяся на экране в момент `Space`, теперь переживает всю паузу и возвращается после похода в пейджер. Ревьюер безопасности привязал это к существующему долгу **[029]** (неэкранированные управляющие последовательности в содержимом лога) и просил на `/done` дописать туда про расширение паузой, а не заводить новый пункт.

**Reviews:**

*Round 1:*
- dev-code-reviewer: approved_with_suggestions, 0 critical / 1 major / 2 minor → [016-feat-pause-display-task-09-dev-code-reviewer-review.json](016-feat-pause-display-task-09-dev-code-reviewer-review.json)
- dev-security-auditor: approved, 0 critical / 0 major / 2 minor → [016-feat-pause-display-task-09-dev-security-auditor-review.json](016-feat-pause-display-task-09-dev-security-auditor-review.json)
- dev-test-reviewer: needs_improvement, 0 critical / 1 major / 1 minor → [016-feat-pause-display-task-09-dev-test-reviewer-review.json](016-feat-pause-display-task-09-dev-test-reviewer-review.json)

Мажор был один и тот же у обоих кодовых ревьюеров, найден мутацией, а не чтением, и оказался серьёзнее, чем исполнитель оценил его сам: незапиннутое свойство «перерисовка не читает файл» (см. Deviations, `selectLogtail`). Применён вместе с минором тест-ревьюера — из `Test_repaintLogtail_noFileAccess` убрана побайтовая сверка вывода, дублировавшая `Test_renderLogtail_outputUnchanged`: после переделки этот тест отвечает за то, КАКУЮ пару выбирает перерисовка, а за байты рендера отвечает соседний. Не применялись: минорка про 6 позиционных параметров (оба ревьюера признали её согласованной с прецедентом пакета) и обе минорки безопасности — предсуществующие, см. Tech debt.

*Round 2 (после исправлений):*
- dev-code-reviewer: approved, 0 critical / 0 major / 0 minor → [016-feat-pause-display-task-09-dev-code-reviewer-review-round2.json](016-feat-pause-display-task-09-dev-code-reviewer-review-round2.json)
- dev-test-reviewer: passed, 0 critical / 0 major / 0 minor → [016-feat-pause-display-task-09-dev-test-reviewer-review-round2.json](016-feat-pause-display-task-09-dev-test-reviewer-review-round2.json)

Оба перепроверили мутацией самостоятельно и получили ровно заявленные падения. Кодовый ревьюер дополнительно подтвердил, что thunk ленивый (вызывается только внутри ветки `fromFile`, не вычисляется заранее), что наружу из замыкания не утекло ничего, завязанного на `g`/`v`, что схлопывание двух вызовов `printLogtail` в один сохраняет поведение (сверка потока управления против `0ae64cf`) и что сигнатуры `renderFrame` и `renderParams` байт-в-байт прежние. Тест-ревьюер отдельно прощупал зеркальную дыру — `selectLogtail`, который не читает НИ НА ОДНОМ пути, — и подтвердил, что её ловит `Test_selectLogtail_livePathReadsTheFile`, то есть тест на невызов не удовлетворяется просто сломанным выбором.

**Verification:**
- Прицельный прогон из `verify` (`go test ./top/ -run 'Logtail|logtail'`) с `-race` → 9 функций passed; накопленный набор фичи (`statLoop|frameStore|repaint|Latch|Sysstat|Pause|lift|helpTemplate|cmdline|Width|Verbose|Extra|resizeDetector|applyFilter|dialog|DataCell|Logtail|logtail`) под `-race -count=2` → ок; все 135 тестов пакета, кроме пяти файлов, требующих живого Postgres, под `-race` → ок
- Красный шаг TDD был настоящим в обоих раундах: сначала `too many arguments in call to publishFrame`, во втором раунде — `undefined: selectLogtail`; реализация писалась после
- **Мутация-критерий приёмки раунда 2:** `if !p.fromFile {` → `if false {` роняет `Test_selectLogtail_repaintNeverInvokesTheRead` (по `t.Fatal` внутри read-thunk'а) и `Test_repaintLogtail_noFileAccess` (в вывод приходят путь и байты «диверсанта», замороженный `Size` перезаписывается); инверсия `if p.fromFile {` роняет все четыре теста выбора источника. Обе мутации откачены, обе перепроверены ревьюерами независимо
- Мутационная проверка остальных несущих свойств: снятие «keep» на пустом чтении, снятие сброса для не-logtail `ShowExtra` и перенос `syncLogtail` выше guard'а `!p.publish` — каждая роняет свой тест; повторена после рефакторинга раунда 2, покрытие не потеряно
- Размещение проверено чтением, а не выводом: локали `logPath`/`logBuf` объявлены ДО блока `if ShowExtra > stat.CollectNone`, единственный вызов `publishFrame` стоит после его закрывающей скобки и безусловен, а сама запись в стор отсекается guard'ом `!p.publish` внутри хелпера
- `grep -n "readLogfileRecent(\|\.Reopen(\|logtail\.Size ="` по `top/stat.go` → три попадания, все внутри thunk'а живого чтения, плюс определение `readLogfileRecent`; на пути перерисовки — ни одного
- `git diff --stat top/extra.go` → пусто, задача 05 не тронута; в индекс попадали только явные пути
- `go vet ./top/` → чисто; `go build ./...` → ок; `gofmt -l top/` → пусто; `make build` → ок; `make lint` → 0 issues + gosec чисто
- **Стендовые проверки НЕ выполнялись и относятся к задаче 10:** реальный терминал, живой PostgreSQL и пишущийся лог-файл здесь недоступны. Отложено туда целиком: открыть панель по `L`, нажать `Space`, переждать несколько интервалов, сходить в пейджер `p` и вернуться — панель должна вернуться с теми же строками и тем же путём в заголовке; поведение при ротации лога во время паузы (три случая из §17.4); тихий лог и повторное открытие панели на другом файле

---

## Task 10: Pre-deploy QA

**Status:** Done (автоматическая и инспекционная половины; стендовый прогон остаётся открытым)
**Commit:** —
**Agent:** gatekeeper-qa (волна 6)
**Summary:** Прогон приёмки перед merge. Автоматическая половина выполнена целиком и зелёная, все инспекционные критерии проверены диффом и чтением тел тестов. Ручная половина — прогон на стенде — **не выполнялась**: адрес стенда эфемерный и запрашивается у владельца проекта перед каждым прогоном, в этот прогон он не передавался; `tmux` локально не установлен, а подменять стенд локальным терминалом задача запрещает. Вердикт **GO** в узком смысле: «автоматические и инспекционные ворота пройдены, стендовый прогон остаётся человеческим гейтом», а не «фича проверена».

**Команды:** `make build` (host) exit 0 · `make test` (контейнер `lesovsky/pgcenter-testing:0.0.11`, фикстуры PG 14–19 на портах 21914–21919) exit 0, `-race` без единого `DATA RACE`, покрытие 71.8% · `make lint` (host) exit 0, `0 issues` + gosec тихо · `make vuln` (host) exit 0, `No vulnerabilities found`. `profile.Test_profileLoop` (долг [030]) не падал — повторный прогон не потребовался. Весь накопленный набор фичи впервые прогнан целиком вместе с фикстурами: тестов, падающих только в полном прогоне и не падающих в точечных `-run`-подмножествах, нет.

**Критерии:**

| Набор | PASS | FAIL | NOT VERIFIABLE HERE |
|---|---|---|---|
| User-spec «Критерии приёмки» (21) | 6 (№2, 11, 17, 18, 19, 21) | 0 | 15 (№1, 3–10, 12–16, 20) |
| Tech-spec «Acceptance Criteria» (8) | 7 (T1–T7) | 0 | 1 (T8 — стендовая половина) |
| Containment по инспекции (5) | 5 | 0 | 0 |

Ни один терминальный критерий не помечен PASS на основании юнит-теста. Критерий 10 (панель логов) помечен NOT VERIFIABLE HERE прямо по требованию задачи — тесты задачи 09 не заменяют стенд.

**Containment (проверено диффом `master...HEAD`, не доверием):** продовый код за пределами `top/` не менялся — 18 файлов, все в `top/`, `go.mod`/`go.sum` не тронуты; `git diff -- internal/ record/ report/ profile/` пуст, `view.View` не получила ни одного поля; в `top/stat_test.go` ровно пять удалённых строк — те самые пять точек вызова `renderSysstat`, каждая возвращается идентичной плюс `, testRenderTime`, ни один голден и ни один ассерт не ослаблен (регексп таймстампа на `:76` байт-в-байт как на master); единственные удаления в остальных предсуществующих тестах — три строки устаревшего комментария в `top/ui_test.go`. `git status` по `top/` чист — задача кода не меняла.

**Замечания (minor, оба не блокирующие):**
- Номера строк точек вызова `renderSysstat` в тексте задачи (`:60`, `:97`, `:192`, `:440`, `:441`) — это координаты master, и они совпадают с ним точно; в рабочем дереве те же пять сидят на `:69`, `:106`, `:294`, `:544`, `:545`, плюс две новые точки в тестах задачи 04. Это замечание к тексту спека, а не FAIL.
- **Для `/done`, долг [029]** (неэкранированные значения строк): пауза добавляет два новых следствия по времени жизни — враждебное значение теперь переживает всю паузу на замороженном экране и возвращается после похода в пейджер/редактор; то же относится к сохранённому буферу логтейла (`frameStore.logBuf`), который переигрывается на каждой перерисовке без перечитывания файла. **Только запись** — правка `docs/tech-debt.md` принадлежит `/done`.

**Известное, но не воспроизведённое:** ограничение с «чёрной дырой» на соединении (Risks tech-spec) — шаг 7b его НЕ воспроизводит: остановленный кластер закрывает сокет и отдаёт ошибку сразу, интерфейс остаётся отзывчивым. Ни покрытым не считается, ни дефектом не заводится.

**Остаётся человеку на стенде:** 15 строк таблицы «Как проверить» (шаги 2–11 с подшагами 3a, 7a–7d) плюс три добавленных шага — 6a (диалог на `-x 60`), 7e (круг через редактор), 7f (справка на паузе); второй бинарник из `master` для отделения регресса от давнего поведения; уборка стенда (сброс GUC с проверкой через `SHOW`, перезапуск кластера, `tmux kill-session`).

**Отчёт:** [016-feat-pause-display-qa-report.json](016-feat-pause-display-qa-report.json)
