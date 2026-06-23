# Decisions Log: Horizontal Scroll (009)

Отчёты агентов о выполнении задач. Каждая запись создаётся агентом, выполнившим задачу.

---

## Task 01: Pure column-window function + scroll-offset state

**Status:** Done
**Commit:** 079cfd2, ff832af
**Agent:** основной агент (исполнитель + round-1 фиксы)
**Summary:** Добавлено поле `config.scrollOffset int` (эфемерное, на `config`, не на `view.View` — Decision 1) и чистая функция `visibleColumns(ncols, colsWidth, termWidth, offset)` в `top/stat.go`, возвращающая видимый диапазон скроллируемых колонок, зажатый offset и флаги `hiddenLeft`/`hiddenRight`. Первая колонка всегда в бюджете ширины; чтение `ColsWidth` строго по `[0,ncols)` (защита от issue #99); re-clamp на каждом вызове (single source of truth). Рендеринг и хоткеи не тронуты.
**Deviations:** Нет.
**Tech debt:** Нет. Дублирование forward/backward-обхода бюджета вынесено в приватный хелпер `countFit` (round 1).

**Reviews:**

*Round 1:*
- dev-code-reviewer: approved_with_suggestions, 3 minor (рефактор-дубликат, читаемость, непокрытый отрицательный бюджет) → [009-feat-horizontal-scroll-task-01-dev-code-reviewer-round1.json]
- dev-test-reviewer: needs_improvement, 1 major (тавтологичные ассерты в кейсе #99) + 2 minor → [009-feat-horizontal-scroll-task-01-dev-test-reviewer-round1.json]

Все находки устранены в round 1 (commit ff832af): точные ассерты для кейса #99 (first=1,last=4,clamped=0,hiddenRight=true), точные значения для very-narrow, кейсы отрицательного offset и отрицательного бюджета, инвариант «последняя колонка видна на maxOffset», рефактор `countFit`. Major закрыт детерминированными значениями, подтверждёнными `go test`.

**Verification:**
- `go test ./top/ -run Test_visibleColumns` → 10/10 подкейсов passed
- `go test ./top/` → без новых регрессий (предсуществующий `Test_doReload` падает без живого PostgreSQL — tech-debt [005], не связан)
- `make build` → ок; gofmt/go vet → чисто

---

## Task 02: Render frozen column + visible window in header and data

**Status:** Done
**Commit:** 3a11407, d17b389
**Agent:** основной агент (исполнитель + round-1 фиксы)
**Summary:** `printStatHeader`/`printStatData` переведены на оконный рендер: замороженная колонка 0 + видимое окно из `visibleColumns`, маркеры `‹`/`›`, bold-имя замороженной колонки с приоритетом подсветки сортировки на col0 (Decision 4). Удалён счётчик `colnum` — индексация значений строго по абсолютному индексу. В `printDbstat`/`renderDbstat` добавлен write-back `config.scrollOffset = clamped` (защита от runaway offset). Print-функции переведены на `io.Writer`+`columnWindow` для тестируемости (gocui.View нельзя сконструировать в юнит-тестах); окно считается один раз в `renderDbstat` и передаётся параметром.
**Deviations:** Сигнатуры print-функций изменены (`io.Writer`+`columnWindow` вместо чтения `v.Size()` внутри) — оправдано тестируемостью, согласовано во всех вызовах, одобрено code-review. `visibleColumns` теперь возвращает структуру `columnWindow` и резервирует ширину маркеров в бюджете (двухпроходное разрешение цикла маркер↔окно).
**Tech debt:** Незначительный, необязательный (из round-2 ревью): переименовать промежуточный `hiddenRight` для читаемости; задокументировать предусловие тест-хелперов (width ≥ len(name)); добавить прогон пустого окна через принтеры; усилить вторую ассерту в sort-priority подтесте. Все косметические, не блокеры.

**Reviews:**

*Round 1:*
- dev-code-reviewer: changes_required, 1 major (бюджет маркеров не зарезервирован → рассинхрон header/data, Decision 5) + 2 minor → [009-feat-horizontal-scroll-task-02-dev-code-reviewer-round1.json]
- dev-test-reviewer: needs_improvement, 2 major (тот же дефект + левый маркер не тестируется) + 3 minor → [009-feat-horizontal-scroll-task-02-dev-test-reviewer-round1.json]

*Round 2 (после исправлений, commit d17b389):*
- dev-code-reviewer: approved — major закрыт (проверено перебором по пространству параметров) → [009-feat-horizontal-scroll-task-02-dev-code-reviewer-round2.json]
- dev-test-reviewer: passed — оба major закрыты (проверено мутациями), minor закрыты → [009-feat-horizontal-scroll-task-02-dev-test-reviewer-round2.json]

**Verification:**
- `go test ./top/` → все render-тесты + `Test_visibleColumns` + `Test_render_alignmentInvariant` (litmus выравнивания) зелёные; без новых регрессий
- `make build` → ок; gofmt/go vet → чисто

---

## Task 03: Scroll hotkeys, offset reset, and help text

**Status:** Done
**Commit:** 3e09740
**Agent:** основной агент
**Summary:** Добавлены хендлеры `scrollLeft` (`[`) и `scrollRight` (`]`) в `top/config_view.go`: `scrollLeft` декрементирует `config.scrollOffset` с зажимом по 0; `scrollRight` инкрементирует без верхнего предела в хендлере (верхний зажим — write-back при рендере из Task 2) + дешёвый guard от int-переполнения. Оба шлют `config.view` на `viewCh` только для перерисовки (view не мутируется — Decision 1). Клавиши `[`/`]` зарегистрированы на `sysstat`. `config.scrollOffset` сбрасывается в 0 на обоих путях переключения: `viewSwitchHandler` и `switchViewToProcPidStat` (Decision 3). Help-экран дополнен описанием клавиш.
**Deviations:** Сброс offset в `switchViewToProcPidStat` помещён ДО guard `app.db.Local` (а не после probe). Причина: код после guard вызывает `app.db.QueryRow` (nil Conn → паника), что делает TDD-тест без живого PostgreSQL невозможным. На remote-пути сброс идемпотентен и безвреден (экран не меняется, offset эфемерный). Оценено обоими ревьюерами как приемлемое.
**Tech debt:** Нет (только optional-предложения ревью: `assert.Same` на не-мутацию view в тесте, коммент-инвариант в хендлере procpidstat — не блокеры).

**Reviews:**

*Round 1:*
- dev-code-reviewer: approved_with_suggestions, 0 critical/major, 3 optional minor → [009-feat-horizontal-scroll-task-03-dev-code-reviewer-round1.json]
- dev-test-reviewer: passed, 0 critical/major, 2 minor (tech_debt, не блокеры) → [009-feat-horizontal-scroll-task-03-dev-test-reviewer-round1.json]

**Verification:**
- `go test ./top/` → `Test_scrollLeft/Right`, `Test_scrollOrthogonalToSort`, оба reset-теста + существующие (`Test_orderKey*`, `Test_switchViewTo`) зелёные
- `make build` → ок; gofmt/go vet → чисто

---

## Task 04: Pre-deploy QA

**Status:** Done
**Commit:** (QA-отчёт, без изменений кода)
**Agent:** основной агент (pre-deploy-qa)
**Summary:** Приёмочное тестирование без изменений production-кода. Автоматические гейты: `make build` PASS, `make lint` PASS, тесты фичи (`go test ./top/`, 14 функций) PASS. Критерии приёмки: 7 PASS (авто), 11 MANUAL (визуальные TUI, требуют ручной проверки пользователем), 1 PARTIAL (сводный гейт), 0 FAIL, 0 блокеров. QA-отчёт: 009-feat-horizontal-scroll-qa-report.json.
**Deviations:** Нет. Реальных багов фичи не выявлено.
**Tech debt:** Не относящееся к фиче, к отслеживанию: (1) `make vuln` падает на предсуществующем GO-2026-5037 (stdlib crypto/x509, go1.25.10→1.25.11; через postgres/cobra, не код фичи; ADR [004] уже бампил в CI — локальный toolchain отстаёт); (2) tech-debt [005] — PG-фикстуры (контейнеры 21914-21918) не подняты, поэтому PG-зависимые тесты пакета top падают на connection refused (не регрессия, не заходят в код скролла).

**Reviews:** none (QA — собственная верификация).

**Verification:**
- `make build` PASS; `make lint` PASS; `go test ./top/` (тесты фичи) PASS
- 11 визуальных критериев (US-1…US-11) — ожидают ручной проверки пользователем в узком терминале

---

## QA-фиксы (после ручной проверки Phase 4)

**Status:** Done
**Commit:** a235af7, 1158b3b
**Agent:** основной агент (systematic-debugging + TDD)
**Summary:** Ручная проверка выявила два бага, единая корневая причина — `countFit` в `visibleColumns` включал скроллируемую колонку только при ПОЛНОМ помещении, из-за чего намеренно широкая последняя колонка `query` (activity/statements) выпадала из окна. Баг 1: query исчезала (раньше печаталась обрезанной по краю gocui). Баг 2: спурьёзный маркер `›` на широком экране. Фикс 1 (a235af7): семантика `countFit` изменена на «начало в бюджете» — последняя колонка показывается частично (обрезается gocui), `hiddenRight=false` когда за ней ничего нет. Фикс 2 (1158b3b): code-review нашёл critical-рассинхрон — backward-walk (`maxOffset`) не резервировал гарантированный левый маркер, из-за чего последняя колонка была недостижима на правом крае; добавлен резерв левого маркера в backward-walk + property-тест перебором (ncols×widths×termWidth), доказывающий инвариант «на maxOffset последняя колонка достижима, `›` гаснет».
**Deviations:** Изменена семантика `visibleColumns` относительно первоначальной реализации task-01/02 (частичная видимость вместо полного помещения) — это исправление дефекта, выявленного только при визуальной проверке; покрыто обновлёнными и новыми тестами.
**Tech debt:** Нет (оставшиеся round-3 minor — токенные ассерты в render-тестах вместо точного равенства — необязательные).

**Reviews:**

*Round 3 (фикс 1, commit a235af7):*
- dev-code-reviewer: changes_required, 1 critical (рассинхрон forward/backward maxOffset — недостижимая последняя колонка) → [009-feat-horizontal-scroll-task-02-dev-code-reviewer-round3.json]
- dev-test-reviewer: passed (оба бага закрыты тестами, litmus подтверждён откатом семантики) → [009-feat-horizontal-scroll-task-02-dev-test-reviewer-round3.json]

Critical устранён фиксом 2 (commit 1158b3b): property-тест `Test_visibleColumns_maxOffsetReachesLastColumn` перебором (15 конфигов падали до фикса, все зелёные после). Корректность подтверждена чтением кода (фикспойнт-логика резерва) и зелёным go test.

**Verification:**
- `go test ./top/` → все scroll/render тесты + property-тест зелёные
- `make build` → ок; gofmt/go vet → чисто
- Требует финальной визуальной проверки пользователем (query видна и скроллится; маркеры корректны; доскролл до последней колонки гасит `›`)
