# Execution Plan: iodelay Columns in procpidstat Screen

**Создан:** 2026-05-19

---

## Wave 1 (независимые)

### Task 01: Extend procpidstat stat layer and screen handler
- **Skill:** code-writing
- **Reviewers:** dev-code-reviewer, dev-security-auditor, dev-test-reviewer
- **Verify:** bash — `make build && make lint`
- **Files:** internal/stat/procpidstat.go, internal/view/view.go, internal/stat/stat.go, top/config_view.go, record/record.go, internal/stat/procpidstat_test.go (call-sites)

## Wave 2 (зависит от Wave 1, задачи 02+03 параллельно)

### Task 02: Add new tests and golden files
- **Skill:** code-writing
- **Reviewers:** dev-code-reviewer, dev-test-reviewer
- **Verify:** bash — `make test`
- **Files:** internal/stat/procpidstat_test.go, testdata golden files (new), internal/stat/stat_test.go, record/record_test.go

### Task 03: Update project knowledge and ADR log
- **Skill:** documentation-writing
- **Reviewers:** dev-code-reviewer
- **Verify:** bash — `git diff --stat docs/`
- **Files:** docs/tech-debt.md, docs/decisions-log.md, docs/features-catalog.md

## Wave 3 / Final Wave (зависит от Wave 2)

### Task 04: Pre-deploy QA
- **Skill:** pre-deploy-qa
- **Reviewers:** none
- **Verify:** bash — `make test && make lint && make vuln`

---

## Проверки, требующие участия пользователя

- [ ] **Task 04 (QA) — позитивный сценарий:** `sysctl -w kernel.task_delayacct=1` → запустить pgcenter → `Shift+S` → выполнить IO-нагружающий запрос → убедиться что `%iodelay > 0` и `iodelay_total,s` меняется между тиками
- [ ] **Task 04 (QA) — негативный сценарий:** `sysctl -w kernel.task_delayacct=0` → `Shift+S` → убедиться что iodelay-колонки `""` и предупреждение появляется в cmdline area
- [ ] **Task 04 (QA) — первый тик:** сразу после `Shift+S` — `%iodelay = ""`, `iodelay_total,s` показывает ненулевое значение
