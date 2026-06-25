# Execution Plan: Verbose Mode for the Top Summary Panels (010-feat-overview-dashboard)

**Создан:** 2026-06-25
**Ветка:** feature/010-overview-dashboard
**Гейт:** подтверждение пользователя ПЕРЕД каждой волной.

---

## Wave 1 (независимые)

### Task 01: Net-new formatting helpers
- **Skill:** code-writing
- **Reviewers:** dev-code-reviewer, dev-test-reviewer
- **Verify:** bash — `go test ./internal/pretty/...`

### Task 02: Verbose toggle plumbing
- **Skill:** code-writing
- **Reviewers:** dev-code-reviewer, dev-test-reviewer
- **Verify:** bash — `go test ./top/...`

### Task 03: io.Writer refactor of printSysstat/printPgstat
- **Skill:** code-writing
- **Reviewers:** dev-code-reviewer, dev-test-reviewer
- **Verify:** bash — `go test ./top/...` (compact output byte-identical)

### Task 04: GUC + data_directory reads
- **Skill:** code-writing
- **Reviewers:** dev-code-reviewer, dev-security-auditor, dev-test-reviewer
- **Verify:** bash — `go test ./internal/query/... ./internal/stat/...`

## Wave 2 (зависит от Wave 1)

### Task 05: New aggregate SQL queries + collection
- **Skill:** code-writing
- **Reviewers:** dev-code-reviewer, dev-security-auditor, dev-test-reviewer
- **Verify:** bash — `go test ./internal/...`
- **depends_on:** 02, 04

### Task 06: Verbose-aware layout() geometry
- **Skill:** code-writing
- **Reviewers:** dev-code-reviewer, dev-test-reviewer
- **Verify:** user — verbose band + height-guard on several terminal sizes
- **depends_on:** 02

## Wave 3 (зависит от Wave 2)

### Task 07: All-three system collection branch
- **Skill:** code-writing
- **Reviewers:** dev-code-reviewer, dev-test-reviewer
- **Verify:** bash — `go test ./internal/stat/...`
- **depends_on:** 02, 05

## Wave 4 (зависит от Wave 3)

### Task 08: Verbose row composers (both panels)
- **Skill:** code-writing
- **Reviewers:** dev-code-reviewer, dev-security-auditor, dev-test-reviewer
- **Verify:** user — rows consistent with full B/N/F/d/r/b panels
- **depends_on:** 01, 03, 05, 07

## Wave 5 (зависит от Wave 4)

### Task 09: Tiering + latency guard + first-tick handling
- **Skill:** code-writing
- **Reviewers:** dev-code-reviewer, dev-test-reviewer
- **Verify:** user — slow source stays stale; first-tick collecting hint clears
- **depends_on:** 05, 07, 08

## Wave 6 (Final)

### Task 10: Pre-deploy QA
- **Skill:** pre-deploy-qa
- **Reviewers:** none
- **Verify:** bash — `make build && make test && make lint`
- **depends_on:** 01–09

## Проверки, требующие участия пользователя

- [ ] Task 06: verbose-раскладка панелей и height-guard на разных размерах терминала (узкий/широкий/низкий).
- [ ] Task 08: консистентность verbose-строк с полными панелями (`B`/`N`/`F`) и экранами (`d`/`r`/`b`); первый тик → `n/a`.
- [ ] Task 09: медленный источник держит stale-значение; `collecting...` на первом тике и при OFF→ON.
- [ ] Final (Task 10): полная приёмка — toggle/персистентность, деградация (remote/standby/archive_mode=off), отсутствие регрессий.
