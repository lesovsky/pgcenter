---
status: in_progress                    # planned -> in_progress -> done
depends_on: []                     # ID задач-зависимостей (строки: ["01", "02"])
wave: 1                            # волна параллельного выполнения
skills: [code-writing]             # МАССИВ скиллов для загрузки
verify: bash — go test ./internal/pretty/... ./top/...
reviewers: [dev-code-reviewer, dev-test-reviewer]
teammate_name:
---

# Task 02: [011] Consolidate the rate-formatting helper

## Required Skills

Перед выполнением задачи загрузи:
- `/skill:code-writing` — [skills/code-writing/SKILL.md](~/.claude/skills/code-writing/SKILL.md)

## Description

`top/stat.go` has an unexported `rateField` (lines 326-344) that re-implements, byte-for-byte, the
overflow/divisor/ceil/reserve-width logic of `pretty.RateUnit` (`internal/pretty/pretty.go:59-78`). The
**only** behavioral difference is that `rateField` inserts `" " + prefix` (a read/write marker) between
the digits and the unit — `rateField` emits `"1135 rMB/s"`, `RateUnit` emits `"9999MB/s"`. This is
registered tech-debt [011]: a duplicated formatter that must be kept in sync by hand.

This task removes the duplication. Extract the shared parts computation into one unexported core
`rateUnitParts(v float64, family string, width int) (field, unit string)` in `internal/pretty`. Keep
`RateUnit` as a thin wrapper returning `field+unit` (BYTE-IDENTICAL to today, e.g. `"9999MB/s"`). Add a
new **exported** `RateUnitPrefixed(v float64, family, prefix string, width int)` that returns
`field+" "+prefix+unit` (e.g. `"9999 rMB/s"`) — it must be exported because it is called from package
`top`. Delete `rateField` and repoint its four call sites (disk r/w, net r/w in `renderSysstatVerbose`)
to `pretty.RateUnitPrefixed`.

Output must stay byte-identical at every call site. The verbose disk/net golden lines in
`top/stat_test.go` and the existing `RateUnit` test suite in `internal/pretty/pretty_test.go` are the
regression guards; a new boundary table captured from the pre-refactor `rateField` strings is the TDD
anchor proving the prefixed form survives the move unchanged.

This is pure-logic/render refactoring — no live PostgreSQL, no terminal, no E2E.

**Wave conflict (Decision 4 of the tech-spec):** Task 03 [012] edits the SAME two files
(`internal/pretty/pretty.go` and `top/stat.go`) in Wave 2 and depends on this task. This task is Wave 1
and must be committed independently before Task 03 starts.

## What to do

1. In `internal/pretty/pretty.go`, add an unexported core
   `rateUnitParts(v float64, family string, width int) (field, unit string)` that holds the
   base/high/divisor selection, the `maxFit` loop, the ceil, and the reserve-width formatting currently
   inside `RateUnit`. It returns the numeric `field` (e.g. `"9999"`, `"  10"`) and the resolved `unit`
   (e.g. `"MB/s"`, `"GB/s"`) separately — no separator, no prefix.
2. Rewrite `RateUnit` as a thin wrapper that calls `rateUnitParts` and returns `field + unit`. Its output
   must be byte-identical to today (`"9999MB/s"` form).
3. Add a new exported `RateUnitPrefixed(v float64, family, prefix string, width int) string` that calls
   `rateUnitParts` and returns `field + " " + prefix + unit` (e.g. `"9999 rMB/s"`). Document it briefly,
   mirroring the existing `RateUnit` doc comment.
4. In `top/stat.go`, delete the `rateField` function (lines 326-344) and its doc comment (321-325).
5. Repoint the four `rateField` call sites in `renderSysstatVerbose` to `pretty.RateUnitPrefixed` with the
   same arguments (the parameter order `(v, family, prefix, width)` is identical):
   - line 363 — disk read: `pretty.RateUnitPrefixed(d.Rsectors, pretty.FamilyDisk, "r", 4)`
   - line 365 — disk write: `pretty.RateUnitPrefixed(d.Wsectors, pretty.FamilyDisk, "w", 4)`
   - line 383 — net read: `pretty.RateUnitPrefixed(n.Rbytes/1024/128, pretty.FamilyNet, "r", 4)`
   - line 384 — net write: `pretty.RateUnitPrefixed(n.Tbytes/1024/128, pretty.FamilyNet, "w", 4)`
6. Avoid shadowing Go builtins in any new parameter or variable name (revive `redefines-builtin-id` is
   error severity — see recent commit `8f1d588`). In particular do not name a return value or param
   `len`, `cap`, `new`, `min`, `max`, etc.

## TDD Anchor

Tests to write BEFORE implementation. CRITICAL ORDER: capture the current `rateField` output as the
golden boundary table **before** deleting `rateField`, then make the new helpers reproduce it.

- `internal/pretty/pretty_test.go::TestRateUnitPrefixed` — NEW table comparing `RateUnitPrefixed` output
  against the captured pre-refactor `rateField` strings: disk and net, `r` and `w`, at `maxFit` (9999,
  base unit, e.g. `"9999 rMB/s"`) and `maxFit+1` (10000, promoted unit, e.g. `"  10 wGB/s"`). Asserts the
  space, the prefix, and the promotion boundary are all byte-identical to `rateField`. This is the golden
  anchor — write it first, capturing the exact strings `rateField` produces today. The `want` values MUST
  be hardcoded string literals computed by hand from the current `rateField` body (e.g. `"9999 rMB/s"`) —
  do NOT call `rateField` in the test (it is being deleted, and calling it would make the anchor circular).
- `internal/pretty/pretty_test.go::TestRateUnit` (existing, lines 77-116) — must stay GREEN unchanged
  (proves `RateUnit` output is still byte-identical, `"9999MB/s"` form, no space/prefix).
- `internal/pretty/pretty_test.go::TestRateUnit_boundary` (existing, lines 120-141) — must stay GREEN
  unchanged.
- `internal/pretty/pretty_test.go::TestRateUnit_property` (existing, lines 148-202) — must stay GREEN
  unchanged.
- `top/stat_test.go::Test_renderSysstat_verboseIostatMaxUtil` (existing, ~157-175) — golden line
  `"  iostat:  2 devices,  80% max util, 1135 rMB/s, 34152 r/s, 1546 wMB/s, 17852 w/s"` must stay
  byte-identical after the call sites delegate to `pretty.RateUnitPrefixed`.
- `top/stat_test.go::Test_renderSysstat_verboseNicstatConversion` (existing, ~180-198) — golden line
  `" nicstat:  1 devices,  60% max util, 4345 rMbps, 6543 wMbps, 3451/0 err/coll"` must stay
  byte-identical.
- `top/stat_test.go::Test_renderSysstat_verboseFirstTickNA` (existing, ~203+) — must stay GREEN.

## Acceptance Criteria

- [ ] `internal/pretty/pretty.go` has an unexported `rateUnitParts(v float64, family string, width int) (field, unit string)` core.
- [ ] `RateUnit` is a thin wrapper over `rateUnitParts`; its output is byte-identical to today (`"9999MB/s"` form).
- [ ] New exported `pretty.RateUnitPrefixed(v float64, family, prefix string, width int) string` returns `field+" "+prefix+unit`.
- [ ] `rateField` is deleted from `top/stat.go`; its four call sites (363, 365, 383, 384) call `pretty.RateUnitPrefixed`.
- [ ] New `TestRateUnitPrefixed` boundary table (disk/net, r/w, maxFit and maxFit+1) is byte-identical to the pre-refactor `rateField` output.
- [ ] Existing `RateUnit` tests (`TestRateUnit`, `_boundary`, `_property`) stay green unchanged.
- [ ] Existing `top/stat_test.go` verbose disk/net golden lines stay byte-identical.
- [ ] No builtin-shadowing param/var names (revive `redefines-builtin-id` clean).
- [ ] `make test`, `make lint`, `make vuln` green.
- [ ] Task committed independently.

## Context Files

**Feature artifacts:**
- [011-refactor-tech-debt-paydown.md](011-refactor-tech-debt-paydown.md) — user-spec
- [011-refactor-tech-debt-paydown-tech-spec.md](011-refactor-tech-debt-paydown-tech-spec.md) — tech-spec (Decision 2 is this task)
- [011-refactor-tech-debt-paydown-decisions.md](011-refactor-tech-debt-paydown-decisions.md) — decisions log
- [011-refactor-tech-debt-paydown-code-research.md](011-refactor-tech-debt-paydown-code-research.md) — code research (section 1 [011], section 5 [011], section 7 [011])

**Project knowledge:**
- [overview.md](../../../.claude/skills/project-knowledge/overview.md)
- [architecture.md](../../../.claude/skills/project-knowledge/architecture.md)
- [patterns.md](../../../.claude/skills/project-knowledge/patterns.md) — "Dynamic unit-suffix rate formatter (010)", "Testable TUI Rendering", "Linting", "Naming Conventions"

**Code files:**
- [internal/pretty/pretty.go](../../../internal/pretty/pretty.go) — add `rateUnitParts` core, rewrite `RateUnit` as wrapper, add `RateUnitPrefixed`
- [top/stat.go](../../../top/stat.go) — delete `rateField`, repoint 4 call sites
- [internal/pretty/pretty_test.go](../../../internal/pretty/pretty_test.go) — add `TestRateUnitPrefixed`; keep existing `RateUnit` tests unchanged
- [top/stat_test.go](../../../top/stat_test.go) — verbose disk/net goldens stay byte-identical

## Verification Steps

- Run `go test ./internal/pretty/... ./top/...` — all green, including the new `TestRateUnitPrefixed` and the untouched verbose goldens.
- Confirm `grep -n rateField top/stat.go` returns nothing (function deleted, no stale call sites).
- Run `make lint` — golangci-lint v2 + gosec clean; specifically no revive `redefines-builtin-id` on the new helper.
- Run `make test && make vuln` — race+coverage and govulncheck green.

## Details

**Files:**
- `internal/pretty/pretty.go` — currently `RateUnit` (lines 59-78) inlines: base/high/divisor selection
  (60-65), the `maxFit` loop (68-72), and the `Ceil` + `ReserveWidth` formatting (74-77). Extract this
  into `rateUnitParts` returning `(field, unit)` where `field` is the `ReserveWidth(...)` string and
  `unit` is `base` or `high`. `RateUnit` then returns `field+unit`; `RateUnitPrefixed` returns
  `field+" "+prefix+unit`. Reuse the existing `pretty.Ceil`, `pretty.ReserveWidth`, `FamilyDisk`,
  `FamilyNet` — no new imports. The promotion logic (`if Ceil(v) <= maxFit` use base+`Ceil(v)`, else
  high+`Ceil(v/divisor)`) lives in the core so both public funcs share the exact same boundary.
- `top/stat.go` — delete `rateField` (326-344) and its doc comment (321-325). The four call sites are in
  `renderSysstatVerbose`; the argument lists are unchanged, only the function name changes from
  `rateField` to `pretty.RateUnitPrefixed` (same `(v, family, prefix, width)` order). `pretty` is already
  imported and used in this file (`pretty.FamilyDisk`, `pretty.Ceil`, `pretty.ReserveWidth`) — no import
  change. After deleting `rateField`, no leftover unused vars (its local base/high/divisor go with it).
- `internal/pretty/pretty_test.go` — add `TestRateUnitPrefixed`. Do NOT modify `TestRateUnit`,
  `TestRateUnit_boundary`, `TestRateUnit_property` — they are the byte-identity guard for the wrapper.
- `top/stat_test.go` — no edits expected; the verbose goldens must pass unchanged. If they do not, the
  refactor introduced drift — fix the helper, not the golden.

**Dependencies:** none (no depends_on, Wave 1). Note Task 03 [012] depends on this task and shares both
files — commit independently and first.

**Edge cases:**
- The `maxFit` promotion boundary (Ceil(v)==maxFit stays base unit; Ceil(v)==maxFit+1 promotes) must be
  identical between `RateUnit` and `RateUnitPrefixed` — this is where a refactor off-by-one would hide.
  The new boundary table tests exactly maxFit (9999) and maxFit+1 (10000).
- The `" " + prefix` separator: a lost space or dropped prefix is the classic failure mode — assert the
  full string, not just the suffix.
- Widen-beyond-reserve regime (value > ~10238976 disk) where the promoted value itself exceeds the
  reserve — `RateUnit`'s property test already covers this for the no-prefix form; the shared core means
  `RateUnitPrefixed` inherits the same behavior.
- Unknown/empty family falls back to the disk MB/s pair (existing default branch) — preserve it in the core.

**Implementation hints:**
- Name the return values `field, unit` (per Decision 2 signature). Avoid `len`/`cap`/`new`/`min`/`max`
  as identifiers (revive error-severity builtin shadowing).
- Capture the golden anchor strings by reading the current `rateField` body and computing what it returns
  for the boundary inputs (disk/net × r/w × maxFit/maxFit+1) — these are the exact `want` values in
  `TestRateUnitPrefixed`. Write that test before deleting `rateField`.
- `pretty.RateUnitPrefixed` is exported (capital R) because package `top` calls it across the package
  boundary; `rateUnitParts` stays unexported (internal to `internal/pretty`).

## Reviewers

- **dev-code-reviewer** → `011-refactor-tech-debt-paydown-task-02-dev-code-reviewer-review.json`
- **dev-test-reviewer** → `011-refactor-tech-debt-paydown-task-02-dev-test-reviewer-review.json`

## Post-completion

- [ ] Записать краткий отчёт в [011-refactor-tech-debt-paydown-decisions.md](011-refactor-tech-debt-paydown-decisions.md) (Summary: 1-3 предложения, ревью со ссылками на JSON, без таблиц файндингов и дампов)
- [ ] Если отклонились от спека — описать отклонение и причину
- [ ] Обновить user-spec/tech-spec если что-то изменилось
