---
created: 2026-06-25
status: draft
branch: develop
size: M
---

# Tech Spec: Tech-debt paydown — allocation cap, rate-helper consolidation, fixed-width verbose sizes

## Solution

Three independent, registered tech-debt items ([009], [011], [012]) closed in one feature with
three per-task-committed tasks, all pure-logic/render changes verified by unit/table/golden tests
(no live PostgreSQL, no E2E):

- **[009]** Add an exported `int64` cap `stat.MaxResultFileSize` (256 MiB) and reject an out-of-range
  tar-entry size **before** the `make([]byte, …)` allocation in `NewPGresultFile`; apply the same cap
  to all three `readTar` branches in `report` (including the `sysinfo` branch that bypasses
  `NewPGresultFile`).
- **[011]** Extract the shared overflow/divisor/ceil logic of `rateField` and `RateUnit` into one
  unexported core in `internal/pretty`; `RateUnit` (no separator) and a new exported
  `RateUnitPrefixed` (`" " + r/w` prefix) both delegate to it. Delete `rateField`, repoint its four
  call sites. Output stays byte-identical, locked by a pre-refactor boundary table.
- **[012]** Add a fixed-width `pretty.Size` variant and apply it (plus `naReserve` for the `n/a`
  fallback) to the five verbose pgstat Size fields so columns/labels stop shifting between samples.
  Values and units are unchanged — only right-aligned padding is added.

## Architecture

### What we're building/modifying

- **`internal/stat/postgres.go`** — exported `MaxResultFileSize int64` const + bound/`<0` guard inside
  `NewPGresultFile`, returning a wrapped error before allocation. ([009])
- **`report/report.go`** — the `sysinfo.*` `readTar` branch references the same const to cap `hdr.Size`
  before `io.ReadAll`/`io.LimitReader`; the `meta.*` and stat branches inherit the cap via
  `NewPGresultFile`. ([009])
- **`internal/pretty/pretty.go`** — unexported `rateUnitParts` core; `RateUnit` becomes a thin wrapper;
  new exported `RateUnitPrefixed`; new exported fixed-width `SizeWidth`. ([011], [012])
- **`top/stat.go`** — delete `rateField`, repoint its four call sites to `pretty.RateUnitPrefixed`;
  wrap the five verbose Size fields and their `n/a` fallbacks in `pretty.SizeWidth` + `naReserve`. ([011], [012])

### How it works

- **[009]** `report.readTar` reads each tar entry. The two **true CWE-789 pre-allocation sinks** are the
  `meta.*` and stat branches: both go through `NewPGresultFile(r, hdr.Size)`, which does
  `make([]byte, hdr.Size)` — a buffer pre-sized from the attacker-influenceable header before any data is
  read. The guard checks `hdr.Size` against `MaxResultFileSize` (and `< 0`) and returns an error before
  `make`. The `sysinfo.*` branch is **not** a pre-allocation sink: `io.ReadAll(io.LimitReader(r, hdr.Size))`
  grows its buffer by bytes actually read (≤ the real entry payload), so a lying oversized header alone
  cannot over-allocate there. We still add the same cap inline (with an explanatory comment) as
  defense-in-depth and for uniform behavior, but its purpose is consistency, not closing a pre-alloc sink.
  An over-limit entry returns through the existing `readTar` error path (clean abort, `doneCh` still
  fires, no panic); `pgcenter report` exits with the error on stderr.
- **[011]** `rateUnitParts(v, family, width)` computes the ceil-rounded value, applies the
  reserve-width formatting, and promotes the unit on overflow, returning `(field, unit)`.
  `RateUnit` returns `field+unit` (e.g. `"9999MB/s"`); `RateUnitPrefixed` returns
  `field+" "+prefix+unit` (e.g. `"9999 rMB/s"`). The four verbose disk/net call sites use
  `RateUnitPrefixed`.
- **[012]** `SizeWidth(v, width)` right-aligns `Size(v)` into `width` (`fmt.Sprintf("%*s", width, Size(v))`),
  never truncating (widens deterministically, the `ReserveWidth` model). The five verbose fields print
  `SizeWidth(v, sizeFieldWidth)`; their `n/a` fallbacks print `naReserve(sizeFieldWidth)` so the
  trailing label position is identical in value and `n/a` states.

## Decisions

### Decision 1: [009] hard per-entry cap + error before allocation
**Decision:** A single exported `const MaxResultFileSize int64 = 256 << 20` (256 MiB) in
`internal/stat`. `NewPGresultFile` returns an error (e.g. `fmt.Errorf("result file size %d exceeds limit %d
bytes", bufsz, MaxResultFileSize)`, and a distinct negative-size error) when
`bufsz < 0 || bufsz > MaxResultFileSize`, **before** `make([]byte, bufsz)`. The `meta.*` and stat
branches (the two real pre-alloc sinks) inherit the guard via `NewPGresultFile`; the `sysinfo.*` branch
adds the same cap inline as defense-in-depth (see Architecture → How it works). Type is `int64` so
`bufsz > MaxResultFileSize` is a pure int64 compare — no `int` conversion, no gosec G115; the
implementation must not introduce `int(hdr.Size)` anywhere in the guard.
**Rationale:** `json.Unmarshal` needs the full buffer, so a cap+reject is clearer than a truncated read.
256 MiB is ~300× the largest real entry observed in the golden fixtures (~817 KB
`statements_timings`), so it never rejects legitimate data while bounding a multi-GB malicious header.
**Alternatives considered:** streaming `io.LimitReader` truncation (rejected — yields a partial buffer
`json.Unmarshal` cannot use); archive signing/trust (rejected — out of scope; the goal is to bound the
allocation, not authenticate the archive).

### Decision 2: [011] shared `rateUnitParts` core, `RateUnit` + new `RateUnitPrefixed` delegate
**Decision:** Unexported `rateUnitParts(v float64, family string, width int) (field, unit string)` holds
the base/high/divisor selection, `maxFit` loop, ceil, and reserve-width formatting. `RateUnit(v, family,
width)` returns `field+unit`; new exported `RateUnitPrefixed(v, family, prefix string, width int)`
returns `field+" "+prefix+unit`. `rateField` is deleted; the four call sites
(`top/stat.go` disk r/w, net r/w) call `pretty.RateUnitPrefixed`.
**Rationale:** `RateUnit` currently has no production callers (only tests), and the verbose rows used the
divergent copy `rateField`; the two output forms differ only by the `" "+prefix` separator, so the
shared piece is the parts computation and the two public funcs assemble differently. Keeping `RateUnit`
byte-identical preserves its existing test suite and the feature-010 ADR.
**Alternatives considered:** generalize `RateUnit` with a separator/prefix param (rejected — changes its
`"9999MB/s"` output and breaks its tests); delete `RateUnit` as dead code (rejected — it is the
documented feature-010 API with a property-test suite; churn without benefit).

### Decision 3: [012] `pretty.SizeWidth` + single reserve width 8
**Decision:** New exported `SizeWidth(v float64, width int) string = fmt.Sprintf("%*s", width, Size(v))`.
The five verbose fields use a single `sizeFieldWidth = 8` (named const in `top/stat.go` near
`cacheHitWidth`); their `n/a` fallbacks use `naReserve(sizeFieldWidth)`.
**Rationale:** The widest realistic `Size` string is 7 chars (`"1023.9M"`/`"1023.9G"`/`"1023.9T"`);
reserve 8 gives one column of margin and right-aligns cleanly. A value beyond the reserve widens
deterministically (`%*s` never truncates), matching `ReserveWidth` semantics. One reserve is simpler
than per-field budgets and sufficient (user-confirmed).
**Alternatives considered:** per-field width budgets (rejected — more complex, no benefit); changing
`Size` itself to be fixed-width (rejected — `Size` has other callers that must stay variable-width).

### Decision 4: task ordering — [011] and [012] share files, must be sequential
**Decision:** Wave 1 = Task 1 [009] ∥ Task 2 [011] (disjoint files). Wave 2 = Task 3 [012] (after [011]).
Each task commits independently.
**Rationale:** [009] touches `internal/stat/postgres.go` + `report/report.go`; [011] and [012] both touch
`internal/pretty/pretty.go` **and** `top/stat.go`, so they cannot run as parallel waves. [009] is
disjoint from both and runs alongside [011]. Independent commits mean a stalled [012] does not block the
merged [009]/[011].

### Decision 5: wal size field excluded from [012]
**Decision:** `top/stat.go:601` `pretty.Size(o.WalSize)` (the replication row's first field) stays a bare
`Size`, out of [012] scope.
**Rationale:** It is the first field on its row, so it pushes no trailing label and does not visibly
"breathe". The five named fields each precede a trailing label, which is what shifts. Including wal size
would be scope creep with no visible benefit.

## Data Models

No schema, storage, or migration. Relevant in-memory types are unchanged:
- `stat.PGresult` — JSON-deserialized result built by `NewPGresultFile`.
- `stat.PgstatOverview` — backs the verbose rows; the [012] fields are `TotalSize`, `GrowthPerSec`,
  `LagBytes`, `RetainedBytes`, `ArchivingBacklog` (`int64` bytes) + their `*Valid` flags.

## Dependencies

### New packages
- None.

### Using existing (from project)
- `internal/pretty` — `Size`, `ReserveWidth`, `Ceil`, `FamilyDisk`/`FamilyNet` reused by the new helpers.
- `top/stat.go` — `naReserve`, `naLiteral`, `cacheHitWidth` precedent for fixed-width reservation.
- `report/report_test.go` `writeEntry`/in-memory `tar.Writer` harness (ADR 008) — reused with a crafted
  oversized `hdr.Size` for the [009] over-limit test.
- Standard library only for [009]: `archive/tar`, `io`, `encoding/json`.

## Testing Strategy

**Feature size:** M

### Unit tests
- **[009]** Table test on `NewPGresultFile` with an in-memory `bytes.Reader`: `bufsz` under limit reads
  OK; `== limit` allowed; `== limit+1` rejected with no allocation; `0` allowed (empty); negative
  rejected. Report-path test (synthetic in-memory tar, ADR 008) with a crafted over-limit `hdr.Size` on
  each of the `meta.*`, `sysinfo.*`, and stat branches: `readTar` returns the limit error, no `data`
  sent; a legitimate under-limit entry still replays.
- **[011]** New boundary table comparing `RateUnitPrefixed` output against the captured pre-refactor
  `rateField` strings (disk/net, `r`/`w`, at `maxFit` and `maxFit+1`), proving byte-identical output
  including the space and prefix. Existing `RateUnit` tests must stay green (byte-identical). The
  `top/stat_test.go` verbose disk/net golden lines must stay byte-identical after delegation.
- **[012]** Extend `Test_renderPgstat_verboseNAWidthStatic` to assert the trailing label sits at the
  identical byte offset across two samples of different-width values **and** between value and `n/a`, for
  the five Size fields (the latter assertion fails today — the regression to lock). New `pretty.SizeWidth`
  table test (padding, widen-on-overflow, `Size` digits unchanged). Existing verbose goldens updated to
  the padded form (values/units identical).

### Integration tests
- None — all three changes are pure logic/render with no live PostgreSQL or terminal dependency
  (user-confirmed).

### E2E tests
- None — no new user flow; unit/table/golden coverage is sufficient (user-confirmed).

## Agent Verification Plan

**Source:** user-spec "Как проверить" section.

### Verification approach
Automated gates (`make test`, `make lint`, `make vuln`) cover correctness, lint (golangci-lint v2 +
gosec, incl. no G115), and vulnerabilities. The [011] byte-identity and [009] over-limit behavior are
asserted by the new tests. The [012] cosmetic result additionally needs a human visual check.

### Per-task verification
| Task | verify: | What to check |
|------|---------|--------------|
| 1 [009] | bash | `go test ./internal/stat/... ./report/...` — over-limit rejected, legitimate replays |
| 2 [011] | bash | `go test ./internal/pretty/... ./top/...` — boundary table byte-identical, goldens green |
| 3 [012] | bash + user | `go test ./internal/pretty/... ./top/...`; user presses `v` in `pgcenter top` and confirms Size columns/labels do not shift |
| 4 QA | bash | `make test && make lint && make vuln` all green |

### Tools required
bash (go test, make). No MCP/Playwright. Manual TUI check by the user for [012].

## Backward Compatibility

**Breaking changes:** no.

**Migration strategy:** N/A — tar archive format unchanged; older archives still replay (the cap only
rejects pathological entries far above any real size). `RateUnit` output unchanged; `rateField` was
unexported (internal to `top`). `MaxResultFileSize` and the new `pretty` helpers are additive exports.

**DB migration compatibility:** N/A — no database changes.

**Consumer impact:** `rateField` had four in-package call sites (all repointed). `RateUnit` has no
production callers (tests only). `NewPGresultFile` signature unchanged — only adds an error path for
out-of-range sizes. No external consumers.

## Risks

| Risk | Mitigation |
|------|-----------|
| [009] sysinfo branch bypasses `NewPGresultFile` (uses `io.ReadAll`, so it is bounded by actual bytes, **not** a pre-alloc CWE-789 sink) | Add the cap inline for defense-in-depth/consistency with an explanatory comment; report-path test still covers all three branches |
| [009] negative/zero `hdr.Size` (`make` panic on negative) | Guard `bufsz < 0` (reject) and `== 0` (allow, empty); covered by the table test |
| [009] cap too low rejects a legitimate large recording | 256 MiB ≈ 300× the largest observed entry; "legitimate entry still reads" test is the guard |
| [011] subtle output drift (lost space/prefix, off-by-one at promotion boundary) | Capture pre-refactor `rateField` output as the TDD anchor table before deleting it |
| [012] reserve too narrow re-introduces breathing for large values | Reserve 8 ≥ widest realistic Size string + deterministic widen; mandatory manual `v` check |
| Wave conflict: [011] and [012] both edit `internal/pretty/pretty.go` and `top/stat.go` | Sequence them across waves (Decision 4); never parallel |
| revive `redefines-builtin-id` (error severity) on new helper param names | Avoid shadowing builtins (recent commit 8f1d588 precedent); `make lint` gate |

## Acceptance Criteria

- [ ] `stat.MaxResultFileSize int64 = 256 MiB`; `NewPGresultFile` errors (no allocation) on `bufsz < 0`
      or `bufsz > limit`; cap enforced in all three `report.readTar` branches.
- [ ] `rateField` removed; `RateUnit` + `RateUnitPrefixed` delegate to a shared core; verbose disk/net
      rows output byte-identical to the pre-refactor strings (boundary table + existing goldens green).
- [ ] `pretty.SizeWidth` added; the five verbose Size fields + their `n/a` fallbacks render fixed-width;
      column/label positions stable across samples and between value/`n/a`; digits/units identical to
      `pretty.Size`.
- [ ] `make test` (race+coverage), `make lint` (golangci-lint v2 + gosec, no G115), `make vuln` — green.
- [ ] No regressions in existing `internal/pretty`, `top`, `internal/stat`, `report` tests.
- [ ] Three tasks committed independently.

## Implementation Tasks

### Wave 1 (независимые)

#### Task 1: [009] Defensive allocation cap on tar entries
- **Description:** Add an exported `int64` size cap in `internal/stat` and reject an out-of-range tar
  entry size before allocating in `NewPGresultFile`, so a crafted `pgcenter record` archive cannot
  exhaust memory during `report`. Enforce the same cap across all three `report.readTar` branches,
  including the `sysinfo` branch that bypasses `NewPGresultFile`.
- **Skill:** code-writing
- **Reviewers:** dev-code-reviewer, dev-security-auditor, dev-test-reviewer
- **Verify:** bash — `go test ./internal/stat/... ./report/...`
- **Files to modify:** `internal/stat/postgres.go`, `report/report.go`, `internal/stat/postgres_test.go`, `report/report_test.go`
- **Files to read:** `docs/features/011-refactor-tech-debt-paydown/011-refactor-tech-debt-paydown-code-research.md`

#### Task 2: [011] Consolidate the rate-formatting helper
- **Description:** Extract the shared overflow/divisor/ceil logic into one unexported core in
  `internal/pretty`; have `RateUnit` and a new exported `RateUnitPrefixed` delegate to it. Delete the
  duplicated `rateField` and repoint its four verbose disk/net call sites. Output must stay
  byte-identical, locked by a boundary table captured from the pre-refactor `rateField`.
- **Skill:** code-writing
- **Reviewers:** dev-code-reviewer, dev-test-reviewer
- **Verify:** bash — `go test ./internal/pretty/... ./top/...`
- **Files to modify:** `internal/pretty/pretty.go`, `top/stat.go`, `internal/pretty/pretty_test.go`, `top/stat_test.go`
- **Files to read:** `docs/features/011-refactor-tech-debt-paydown/011-refactor-tech-debt-paydown-code-research.md`

### Wave 2 (зависит от Wave 1 — общие файлы с Task 2)

#### Task 3: [012] Fixed-width verbose Size fields
- **Description:** Add a fixed-width `pretty.Size` variant and apply it (plus `naReserve` for the `n/a`
  state) to the five verbose pgstat Size fields so columns and trailing labels stop shifting between
  samples. Displayed numbers and units stay identical to `pretty.Size`; only right-aligned padding is
  added.
- **Skill:** code-writing
- **Reviewers:** dev-code-reviewer, dev-test-reviewer
- **Verify:** bash + user — `go test ./internal/pretty/... ./top/...`; user presses `v` in `pgcenter top` and confirms Size columns/labels do not shift
- **Files to modify:** `internal/pretty/pretty.go`, `top/stat.go`, `internal/pretty/pretty_test.go`, `top/stat_test.go`
- **Files to read:** `docs/features/011-refactor-tech-debt-paydown/011-refactor-tech-debt-paydown-code-research.md`

### Final Wave

#### Task 4: Pre-deploy QA
- **Description:** Acceptance testing: run all tests (`make test`, `make lint`, `make vuln`), verify the
  acceptance criteria from user-spec and tech-spec, and confirm the mandatory manual verbose `v` check
  for [012].
- **Skill:** pre-deploy-qa
- **Reviewers:** none
