# Decisions Log: tech-debt-paydown (011)

Отчёты агентов о выполнении задач.

---

## Task 01: [009] Defensive allocation cap on tar entries

**Status:** Done
**Commit:** 9a3c630
**Agent:** implementer (general-purpose, opus)
**Summary:** Added exported `stat.MaxResultFileSize int64 = 256 << 20` and an int64-only guard in `NewPGresultFile` (negative → distinct error, over-limit → `result file size %d exceeds limit %d bytes`) before `make([]byte, bufsz)`; added an inline defense-in-depth cap on the `report.readTar` sysinfo branch. The meta/stat branches inherit the cap via `NewPGresultFile` unchanged.
**Deviations:** The over-limit report test builds the tar via `tw.WriteHeader` and emits the header only (no body) — `tar.Writer` enforces `Size == bytes-written`, so a huge declared `Size` cannot be paired with a small body; `readTar` rejects on header `Size` before reading any body, which validly drives all three branches.
**Tech debt:** Нет.

**Reviews:**

*Round 1:*
- dev-code-reviewer: approved, 2 cosmetic minor, no tech debt → [011-refactor-tech-debt-paydown-task-01-dev-code-reviewer-round1.json]
- dev-security-auditor: approved, 0 critical/major/minor → [011-refactor-tech-debt-paydown-task-01-dev-security-auditor-round1.json]
- dev-test-reviewer: approved, 2 minor → [011-refactor-tech-debt-paydown-task-01-dev-test-reviewer-round1.json]

**Verification:**
- `go test ./internal/stat/... ./report/...` → ok
- `golangci-lint run` → 0 issues; `gosec` → 0 (no G115); `make vuln` → clean

---

## Task 02: [011] Consolidate the rate-formatting helper

**Status:** Done
**Commit:** ee623fa
**Agent:** implementer (general-purpose, opus)
**Summary:** Extracted shared rate-format logic into unexported `pretty.rateUnitParts(v, family, width) (field, unit)`; `RateUnit` (byte-identical "9999MB/s" form) and new exported `RateUnitPrefixed` (`field+" "+prefix+unit`) both delegate to it. Deleted `top/stat.go:rateField` and repointed its 4 verbose disk/net call sites. New `TestRateUnitPrefixed` boundary table (hardcoded literals) locks byte-identity.
**Deviations:** `top/stat_test.go` needed no edits — the verbose disk/net goldens stayed byte-identical through delegation, so the commit touches 3 files instead of 4 (a positive signal of equivalence).
**Tech debt:** Нет.

**Reviews:**

*Round 1:*
- dev-code-reviewer: approved, behavior-preserving, no tech debt → [011-refactor-tech-debt-paydown-task-02-dev-code-reviewer-round1.json]
- dev-test-reviewer: approved (8/8 litmus, non-circular anchor), 2 advisory minor → [011-refactor-tech-debt-paydown-task-02-dev-test-reviewer-round1.json]

**Verification:**
- `go test ./internal/pretty/... ./top/...` → ok
- `golangci-lint run` → 0 issues; `gosec` → 0; `make vuln` → clean
- `grep -n rateField top/stat.go` → empty (function fully removed)

---

## Task 03: [012] Fixed-width verbose Size fields

**Status:** Done
**Commit:** c89b686
**Agent:** implementer (general-purpose, opus)
**Summary:** Added exported `pretty.SizeWidth(v, width)` (right-aligns `Size(v)` via `%*s`, never truncating — `ReserveWidth` model, digits/units identical to `Size`) and applied it with a single named const `sizeFieldWidth = 8` to the 5 verbose pgstat Size fields (databases size/growth, replication lag/retain/archiving-backlog), replacing their bare `naLiteral` n/a fallbacks with `naReserve(sizeFieldWidth)`. The trailing labels no longer breathe across ticks or between value and n/a states.
**Deviations:** Нет. `wal size` deliberately left as bare `pretty.Size` (Decision 5 — first field on its row, pushes no trailing label).
**Tech debt:** Нет.

**Reviews:**

*Round 1:*
- dev-code-reviewer: approved, 0 findings, no tech debt → [011-refactor-tech-debt-paydown-task-03-dev-code-reviewer-round1.json]
- dev-test-reviewer: approved, RED-before empirically confirmed, 3 advisory minor → [011-refactor-tech-debt-paydown-task-03-dev-test-reviewer-round1.json]

**Verification:**
- `go test ./internal/pretty/... ./top/...` → ok (new value-vs-n/a offset assertion was RED pre-impl, green after)
- `golangci-lint run` → 0 issues; `gosec` → 0; `make vuln` → clean
- Manual `v` check (deferred to Final Wave QA): Size columns/labels hold steady horizontal position across ticks.
