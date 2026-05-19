# Execution Plan: Per-process System Stats Screen

**Создан:** 2026-05-18
**Ветка:** feature/per-process-system-stats

---

## Wave 1 (независимые)

### Task 01: Procfs parser types and reader functions
- **Skill:** code-writing
- **Reviewers:** dev-code-reviewer, dev-security-auditor, dev-test-reviewer
- **Verify:** bash — `go test ./internal/stat/... -run ProcPid`

### Task 02: Simplified pg_stat_activity SQL query
- **Skill:** code-writing
- **Reviewers:** dev-code-reviewer, dev-security-auditor, dev-test-reviewer
- **Verify:** bash — `go test ./internal/query/...`

## Wave 2 (зависит от Wave 1)

### Task 03: Result builder, CPU formatter, and PID validation
- **Depends on:** 01
- **Skill:** code-writing
- **Reviewers:** dev-code-reviewer, dev-security-auditor, dev-test-reviewer
- **Verify:** bash — `go test ./internal/stat/... -run BuildProcPid|FormatCPU`

### Task 04: View registration, new View fields, and record skip
- **Depends on:** 02
- **Skill:** code-writing
- **Reviewers:** dev-code-reviewer, dev-security-auditor, dev-test-reviewer
- **Verify:** bash — `go test ./record/... && make build`

## Wave 3 (зависит от Wave 2)

### Task 05: Collector integration — snapshot management, enrichment, and Reset()
- **Depends on:** 03, 04
- **Skill:** code-writing
- **Reviewers:** dev-code-reviewer, dev-security-auditor, dev-test-reviewer
- **Verify:** bash — `go test ./internal/stat/... -run TestCollector`

### Task 06: Hotkey, local-mode guard, and filter guard extensions
- **Depends on:** 04
- **Skill:** code-writing
- **Reviewers:** dev-code-reviewer, dev-security-auditor, dev-test-reviewer
- **Verify:** bash — `make build && make lint`

## Final Wave (Wave 4, зависит от Wave 3)

### Task 07: Pre-deploy QA
- **Depends on:** 05, 06
- **Skill:** pre-deploy-qa
- **Reviewers:** none
- **Verify:** bash — `make test`

---

## Проверки, требующие участия пользователя

- [ ] Task 07 (QA): запустить `pgcenter top` (от пользователя `postgres` или через `sudo`), нажать `Shift+S` — 17 колонок
- [ ] Task 07 (QA): запустить тяжёлый CPU-запрос, сравнить `%all` с `top -p <pid>`
- [ ] Task 07 (QA): проверить `read,KiB/s` с `pidstat -d 1 -p <pid>`
- [ ] Task 07 (QA): проверить фильтры `I` и `A`
- [ ] Task 07 (QA): запустить от непривилегированного пользователя — warning + пустые IO-колонки
- [ ] Task 07 (QA): подключиться к remote PG, нажать `Shift+S` — предупреждение о local mode
