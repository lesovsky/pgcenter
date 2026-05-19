# Execution Plan: Record/Report for Per-Process System Stats (003)

**Создан:** 2026-05-19
**Ветка:** feature/procpidstat-record-report

---

## Wave 1 (независимые)

### Task 01: MVC split of buildProcPidResult + export GetSysticksLocal
- **Skill:** code-writing
- **Reviewers:** dev-code-reviewer, dev-security-auditor, dev-test-reviewer
- **Verify:** bash — `go test ./internal/stat/... -run BuildProcPidResult|FormatProc|GetSysticks|SysInfo` → all pass
- **Files:** `internal/stat/procpidstat.go`, `internal/stat/procpidstat_test.go`, `internal/stat/stat.go`, `internal/stat/netdev_test.go`, `internal/stat/diskstats_test.go`, `internal/stat/stat_test.go`

## Wave 2 (зависит от Wave 1)

### Task 02: tarRecorder — stateful procfs enrichment + sysinfo write
- **Skill:** code-writing
- **Reviewers:** dev-code-reviewer, dev-security-auditor, dev-test-reviewer
- **Verify:** bash — `go test ./record/... -run TarRecorder|FilterViews|app_record` → pass; `go build ./cmd/pgcenter` → clean
- **Files:** `record/recorder.go`, `record/record.go`

### Task 03: Report pipeline + -N flag + view config
- **Skill:** code-writing
- **Reviewers:** dev-code-reviewer, dev-security-auditor, dev-test-reviewer
- **Verify:** bash — `go build ./cmd/pgcenter` → clean; `go test ./report/... -run ReadMeta|isFilename` → pass
- **Files:** `internal/view/view.go`, `report/report.go`, `cmd/report/report.go`

## Wave 3 (зависит от Wave 2)

### Task 04: Test suite update
- **Skill:** code-writing
- **Reviewers:** dev-code-reviewer, dev-security-auditor, dev-test-reviewer
- **Verify:** bash — `make test` → all green; `make lint` → no new warnings; E2E record+report -N → ≥1 line output
- **Files:** `record/record_test.go`, `report/report_test.go`, `report/testdata/`

## Wave 4 / Final (зависит от Wave 3)

### Task 05: Pre-deploy QA
- **Skill:** pre-deploy-qa
- **Reviewers:** none
- **Verify:** bash — Full E2E: make build, make test, make lint, record 3 ticks + report -N → ≥1 line output with timestamp

---

## Проверки, требующие участия пользователя

- [ ] После Task 04 / Task 05: запустить `pgcenter record` на 30 сек под реальной нагрузкой, затем `pgcenter report -N` — убедиться что вывод непустой и форматирование корректное
- [ ] TUI regression: `pgcenter top → Shift+S` — экран per-process stats работает как раньше
- [ ] `pgcenter report -N -o "%all"` — строки отсортированы по убыванию
