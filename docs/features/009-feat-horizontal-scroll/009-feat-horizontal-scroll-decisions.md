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
