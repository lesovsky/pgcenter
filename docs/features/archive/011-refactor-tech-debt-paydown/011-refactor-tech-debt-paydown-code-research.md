# Code Research — 011-refactor-tech-debt-paydown

Internal-quality tech-debt paydown bundling three registered debt items from `docs/tech-debt.md`:
[009] unbounded allocation in `NewPGresultFile`, [011] `rateField` duplicates `pretty.RateUnit`,
[012] verbose pgstat Size fields width-breathe. Single PR, size M, no live PostgreSQL / E2E
(per interview). All three are pure-logic/render changes verified by unit/table/golden tests.

This document keys section findings to the three debt items as [009]/[011]/[012].

---

## 1. Entry Points

### [009] internal/stat/postgres.go — `NewPGresultFile`
- `internal/stat/postgres.go:517-538` — `func NewPGresultFile(r io.Reader, bufsz int64) (PGresult, error)`.
  - Line 519: `data := make([]byte, bufsz)` — the unbounded allocation. `bufsz` is `hdr.Size`
    from the tar header (attacker-controlled for untrusted archives). No upper bound today.
  - Line 521: `io.ReadFull(r, data)`; line 527: `json.Unmarshal(data, &res)`; line 532: `res.validate()`.
  - This is the only allocation gate. The change: add an exported `const` (the interview names
    `stat.MaxResultFileSize = 256 MiB`) and reject `bufsz > limit` (and defensively `bufsz < 0`)
    **before** `make`, returning a wrapped error.

### [009] report/report.go — `readTar` (three tar branches)
- `report/report.go:139-222` — `func readTar(r *tar.Reader, config Config, dataCh chan data, doneCh chan struct{}) error`.
  - `report.go:170` — `meta.*` branch: `res, err := stat.NewPGresultFile(r, hdr.Size)`.
  - `report.go:191` — `sysinfo.*` branch: `buf, err := io.ReadAll(io.LimitReader(r, hdr.Size))`.
    This branch does NOT go through `NewPGresultFile`; the cap is `hdr.Size` itself, so it must be
    brought under the same limit independently (interview edge case + AC: limit applied uniformly to
    all three branches).
  - `report.go:203` — `default` (stat) branch: `res, err = stat.NewPGresultFile(r, hdr.Size)`.
  - Existing error path: any returned error from the branch propagates out of `readTar` (lines 171-173,
    192-194, 204-206) and the `defer doneCh <- struct{}{}` (line 145) still fires — so an abort is clean,
    no panic. Over-limit must use this same return path.

### [011] top/stat.go — `rateField` (to be deleted) and its two call sites
- `top/stat.go:326-344` — `func rateField(v float64, family string, prefix string, width int) string`.
  Re-implements `pretty.RateUnit`'s overflow/divisor/ceil logic; the ONLY difference is it inserts
  `" " + prefix` between digits and unit (`pretty.ReserveWidth(...) + " " + prefix + base`, lines
  341/343) producing e.g. `"1135 rMB/s"`, whereas `RateUnit` emits `"9999MB/s"` (no space/prefix).
  base/high/divisor selection (327-332) and `maxFit` loop (334-338) are byte-for-byte the same as
  `RateUnit` (pretty.go:60-72).
- Call sites (all in `renderSysstatVerbose`, four total):
  - `top/stat.go:363` — `rateField(d.Rsectors, pretty.FamilyDisk, "r", 4)` (disk read).
  - `top/stat.go:365` — `rateField(d.Wsectors, pretty.FamilyDisk, "w", 4)` (disk write).
  - `top/stat.go:383` — `rateField(n.Rbytes/1024/128, pretty.FamilyNet, "r", 4)` (net read).
  - `top/stat.go:384` — `rateField(n.Tbytes/1024/128, pretty.FamilyNet, "w", 4)` (net write).
  These must delegate to a shared `internal/pretty` helper after `rateField` is removed.

### [012] top/stat.go — `renderPgstatVerbose` (5 bare `pretty.Size` fields)
- `top/stat.go:546-625` — `func renderPgstatVerbose(w io.Writer, o stat.PgstatOverview, props stat.PostgresProperties) error`.
  The 5 variable-width `pretty.Size(...)` fields (the cosmetic target):
  - `top/stat.go:561` — `size = pretty.Size(float64(o.TotalSize))` (databases size).
  - `top/stat.go:563` — `growth = pretty.Size(float64(o.GrowthPerSec))` (growth/s).
  - `top/stat.go:590` — `lag = pretty.Size(float64(o.LagBytes))` (replication lag).
  - `top/stat.go:594` — `retain = pretty.Size(float64(o.RetainedBytes))` (slots retain).
  - `top/stat.go:598` — `backlog = pretty.Size(float64(o.ArchivingBacklog))` (archiving backlog).
  - Also `top/stat.go:601` — `pretty.Size(float64(o.WalSize))` (wal size) is a 6th bare Size call; the
    interview/tech-debt scope names exactly 5 fields and excludes wal size — confirm in tech-spec whether
    wal size stays out (it is the FIRST field on its row, so it does not push a trailing label; the named
    5 are the ones whose label/column "breathes"). Note: `size`/`growth`/`lag`/`retain`/`backlog` each
    sit BEFORE a trailing label on their line, hence the visible breathing.
  - The n/a fallbacks for these fields use the bare `naLiteral` (`"n/a"`, lines 559, 588, 592, 596),
    NOT `naReserve` — so even the n/a state is variable-relative. The fix must wrap both value and n/a
    in the fixed reserve.

---

## 2. Data Layer

No schema, storage, or migration. The tar archive format is unchanged. Relevant in-memory structs:
- `stat.PGresult` (`internal/stat/postgres.go:443-450`) — the JSON-deserialized result; `NewPGresultFile`
  builds it. `json.Unmarshal` needs the FULL buffer, which is why [009] is a cap+error (reject) rather
  than a streaming `io.LimitReader` truncation (interview Technical Decisions).
- `stat.PgstatOverview` (`internal/stat/postgres.go:30-100`) — the flat aggregate backing the 5 verbose
  pgstat rows; the `[012]` fields are `TotalSize`/`GrowthPerSec`/`LagBytes`/`RetainedBytes`/`ArchivingBacklog`
  (all `int64` bytes) plus their `*Valid` availability flags (`TotalSizeValid`, `LagBytesValid`,
  `RetainedValid`, `ArchivingBacklogValid`).

---

## 3. Similar Features / Existing Patterns to Reuse

- **internal/pretty as the home for shared formatting** (feature 010). `RateUnit`, `ReserveWidth`,
  `Ceil`, `Size` already live there as pure functions (`internal/pretty/pretty.go:8-78`). [011]'s shared
  rate helper and [012]'s fixed-width Size variant belong in this same file, matching the established
  convention. ADR `[010-feat-overview-dashboard]` is the precedent for verbose-panel formatting helpers.
- **`ReserveWidth` (pretty.go:45-47)** — `fmt.Sprintf("%*d", width, v)`; never truncates, widens
  deterministically. This is the exact model for [012]'s fixed-width Size (right-align into a reserved
  width, widen on overflow). The `naReserve` helper (`top/stat.go:527-532`) already does the same for the
  n/a sentinel (`fmt.Sprintf("%*s", width, naLiteral)`), enforcing a floor of `len(naLiteral)`.
- **`RateUnit` (pretty.go:59-78)** is the consolidation target for [011]. The shared helper should be
  parametrized so `RateUnit` (no separator) and the verbose rows (`" " + r/w prefix`) both delegate —
  e.g. an optional middle-string argument, exact signature deferred to tech-spec (interview).
- **naReserve/naInt drop-in pattern (`top/stat.go:527-542`)** — the established way to keep a trailing
  label static when a field degrades to n/a. [012] extends this pattern to the Size fields: a fixed-width
  Size value AND a `naReserve`-padded n/a so the label position is identical in both states.
- **Synthetic in-memory tar for report replay tests** — ADR
  `[008-feat-record-report-0-11-views] Replay tests: synthetic in-memory tar + golden files`
  (decisions-log.md:462). The `report_test.go` `writeEntry` helper (`report/report_test.go:461-467`)
  builds a `tar.Header{Name, Size, Mode}` + `tw.Write(payload)` in a `bytes.Buffer`. This is the exact
  harness [009]'s report-path test reuses — but with a CRAFTED oversized `hdr.Size` (independent of the
  real payload length) to drive the over-limit branch.

---

## 4. Integration Points

- `internal/pretty/pretty.go` — add (a) shared rate helper consumed by `RateUnit` and top/stat verbose
  rows; (b) fixed-width `Size` variant consumed by `renderPgstatVerbose`. No new external deps.
- `internal/stat/postgres.go` — add exported `MaxResultFileSize` const + bound check in `NewPGresultFile`.
  Importers of `stat`: `report/report.go`, `top/*`. The const is exported so `report` (and tests) can
  reference the limit in the sysinfo branch and in assertions.
- `report/report.go` — the `sysinfo.*` branch (line 191) must reference the same `stat.MaxResultFileSize`
  to cap `hdr.Size` before `io.ReadAll`/`io.LimitReader` (the other two branches inherit the cap via
  `NewPGresultFile`).
- `top/stat.go` — delete `rateField`, repoint its 4 call sites (363/365/383/384) to the shared helper;
  wrap the 5 Size fields (561/563/590/594/598) + their n/a fallbacks in the fixed-width variant + naReserve.
- `pretty` imports in top/stat.go are already present (`pretty.FamilyDisk`, `pretty.FamilyNet`,
  `pretty.Ceil`, `pretty.ReserveWidth`, `pretty.Size`), so no new imports for [011]/[012].

---

## 5. Existing Tests (which test files cover each target)

### [009] `NewPGresultFile`
- `internal/stat/postgres_test.go:293-329` — `Test_NewPGresultFile`. Table over `{valid, filename}`;
  opens a `testdata/*.tar`, iterates `tar.Reader`, calls `NewPGresultFile(r, hdr.Size)`, asserts
  no-error + non-nil Values/Cols (valid) or error + zero `PGresult{}` (invalid). Uses real golden tars
  (`testdata/pgcenter.stat.golden.tar`, `testdata/pgcenter.stat.invalid.tar`).
  - For the over-limit case the tech-spec should ADD an in-memory `bytes.Reader` table case (under-limit
    reads OK; `== limit` allowed; `== limit+1` rejected before allocation; `0` allowed; negative rejected) —
    the existing test only covers the on-disk golden, not a crafted oversized `bufsz`.
- `report/report_test.go:200-235` — `Test_readTar` (golden-tar path, asserts 10 data items). And
  `report/report_test.go` ~440-514 (the `Test_app_doReport_procpidstat`-style synthetic-tar test) — the
  `writeEntry`/`tar.NewWriter(&tarBuf)` harness ([009] crafted-`hdr.Size` test reuses this; set
  `hdr.Size` to an over-limit value while writing a small/no payload, assert `readTar` returns the limit
  error and no `data` is sent; a legitimate under-limit entry still replays).

### [011] `RateUnit` / `ReserveWidth` / `rateField`
- `internal/pretty/pretty_test.go` — 6 test funcs (`TestSize:11`, `TestCeil:30`, `TestReserveWidth:51`,
  `TestRateUnit:77`, `TestRateUnit_boundary:120`, `TestRateUnit_property:148`).
  - `TestRateUnit` (77-116): table of `{name, v, family, width, want}`; covers below/at/over the 9999
    overflow boundary for disk and net, the widen-beyond-reserve regime, and the unknown-family default.
  - `TestRateUnit_boundary` (120-141): focused threshold-1/threshold/threshold+1 suffix switch.
  - `TestRateUnit_property` (148+): walks a wide range, asserts per-regime layout (padded to exactly
    reserve while fitting; widens-never-truncates beyond). Property-test precedent for the shared helper.
  - These are the byte-identical anchors. The interview's [011] AC ("boundary table proves byte-identical
    output incl. the space + r/w prefix") means a NEW table comparing the shared-helper output against the
    pre-refactor `rateField` strings (`"9999 rMB/s"`, `"  10 rGB/s"`, at maxFit and maxFit+1, r and w, disk
    and net). Capture the current `rateField` output BEFORE refactor as the TDD anchor.
- `top/stat_test.go:157-203` — `Test_renderSysstat_verboseIostatMaxUtil`, `_verboseNicstatConversion`,
  `_verboseFirstTickNA` exercise the rows that call `rateField`; their golden strings (full-line) lock the
  `"NNNN rMB/s"` / `"NNNN wMbps"` format and must stay byte-identical after delegation.

### [012] verbose pgstat composers
- `top/stat_test.go:356-398` — `Test_renderPgstat_verboseAvailable`: full-line golden per row. Line 385
  golden is the databases row (`"   databases: 1.0T per  7 databases, 1.0M growth/s, ..."`) and 391 the
  replication row (`" replication: 1.0G wal size, 1.0M lag,  1/1.0G slots/retain, 1.0M archiving backlog, ..."`).
  After [012] these goldens change to the fixed-width (padded) form — the values/units stay identical,
  only leading padding is added.
- `top/stat_test.go:406-443` — `Test_renderPgstat_verboseNAWidthStatic`: the KEY pattern. Renders rows
  with value vs n/a and asserts the trailing label sits at the IDENTICAL byte offset
  (`strings.Index(valRows[i], label) == strings.Index(naRows[i], label)`). [012]'s new test reuses this
  exact technique across two SAMPLES (value A vs value B of different widths) to prove the column/label
  stay put — and extends the n/a-vs-value offset assertion to the 5 Size fields (currently they use bare
  `naLiteral`, so this assertion would FAIL today — that is the regression to lock).
- `top/stat_test.go:314-355` — `Test_renderPgstat_verboseNA` (all-n/a golden), and
  `Test_renderPgstat_compactUnchanged:448` (verbose appends only, compact rows untouched) — regression
  guards to keep green.

### Test infra notes
- Framework: `testify` (`assert.*`). Runner: `make test` (`go test -race -coverprofile`).
- Pattern conventions: pure-function **table tests** + **property tests** in `internal/pretty`;
  **full-line golden** assertions + **byte-offset** assertions in `top/stat_test.go`; **synthetic
  in-memory tar** (ADR 008) in `report`. Reuse all three — no new test framework or fixture needed.

---

## 6. Shared Utilities to Reuse

- `pretty.Size(v float64) string` (pretty.go:8-24) — the value/unit source of truth; the fixed-width
  variant wraps/right-pads its output, must NOT change the digits/units.
- `pretty.ReserveWidth(v int, width int) string` (pretty.go:45-47) — `%*d` right-align; the model for the
  Size fixed-width (use `%*s` over the Size string) and already used pervasively.
- `pretty.Ceil(v float64) int` (pretty.go:36-38) and the `FamilyDisk`/`FamilyNet` consts (pretty.go:29-30)
  — reused unchanged by the consolidated rate helper.
- `pretty.RateUnit(v, family, width)` (pretty.go:59-78) — becomes a thin caller of the shared helper.
- `naReserve(width int) string` (top/stat.go:527-532) — pads `n/a` into a reserve (floor `len("n/a")`=3);
  apply to the 5 Size-field n/a fallbacks so the degraded state holds the same width as the value.
- `naLiteral = "n/a"` (top/stat.go:314), `cacheHitWidth = 7` (top/stat.go:319) — width-reservation
  precedent for the trailing-label-static contract.

---

## 7. Potential Problems

- **[009] sysinfo branch is the easy-to-miss one.** It bypasses `NewPGresultFile` entirely
  (`report.go:191`, `io.ReadAll(io.LimitReader(r, hdr.Size))`). A fix that only guards `NewPGresultFile`
  leaves this branch unbounded. The interview AC explicitly requires all THREE branches under the limit;
  cover the sysinfo branch in the report-path test.
- **[009] negative/zero `hdr.Size`.** `tar.Header.Size` is `int64`; a crafted negative value would make
  `make([]byte, negative)` panic. Guard `bufsz < 0` (and `== 0` is allowed → empty, today
  `io.ReadFull` on a 0-len buffer returns nil). Interview edge cases name this.
- **[009] limit value.** 256 MiB must be far above any real single-view JSON snapshot — the "legitimate
  entry still reads" test (the golden tar) is the guard against a too-low limit rejecting real data.
- **[011] byte-identical risk.** The only behavioral difference between `rateField` and `RateUnit` is the
  `" " + prefix` separator. A lost space, dropped prefix, or off-by-one at the `maxFit` promotion boundary
  is the failure mode — mitigated by capturing the pre-refactor `rateField` output as the TDD anchor table
  BEFORE deleting it.
- **[012] reserve too narrow re-introduces breathing.** Size strings range from `"0"`/`"512B"` to
  `"465.9T"` (max realistic width ~6-7 chars from `TestSize` cases). The reserve must cover the widest
  realistic Size string; a value beyond it must widen deterministically (ReserveWidth semantics) without
  breaking the layout. Exact reserve width deferred to tech-spec (interview).
- **[012] wal size field (top/stat.go:601) is NOT in the named 5** but is also a bare `pretty.Size`. It is
  the first field on the replication row (no trailing label pushed), so it does not "breathe" visibly;
  confirm in tech-spec it stays out of scope to avoid scope creep.
- **Active tech-debt [016]** (`internal/stat/*` swallow errors silently) touches `internal/stat/postgres.go`
  (the [009] file) — Severity Low. NOT in this feature's scope; [009] should ADD a proper returned error
  for the over-limit case (it is on the error-returning path of `NewPGresultFile`, not a swallowed
  collector error), so it does not worsen [016]. No interaction beyond sharing the file.
- **Settled ADRs that constrain approach:**
  - `[008-feat-record-report-0-11-views] Replay tests: synthetic in-memory tar + golden files`
    (decisions-log.md:462) — [009]'s report-path test MUST follow this (no live PostgreSQL, no new fixture
    tar on disk; craft the over-limit entry in a `bytes.Buffer`).
  - `[010-feat-overview-dashboard]` verbose-panel ADRs (decisions-log.md:574+) — establish that verbose
    formatting helpers live in `internal/pretty` and that the n/a-vs-value width-static contract is the
    rendering invariant; [011]/[012] must preserve it.

---

## 8. Constraints & Infrastructure

- **Go 1.25+** (`go.mod:3` — `go 1.25.0`). Testing: `testify`. TUI: `gocui` (not touched here).
- **Lint:** golangci-lint **v2** (`.golangci.yml` `version: "2"`), linters `gocritic` + `revive` (+ default
  set). `revive` rules active: `redefines-builtin-id` (severity **error**), `var-naming`, `unused-parameter`,
  `superfluous-else`. Note the most recent commit `8f1d588 fix(lint): rename builtin-shadowing vars to
  satisfy revive` — avoid shadowing builtins in any new helper (the shared rate helper / Size variant
  param names). `errcheck` is excluded only in `_test.go`.
- **gosec** (`make lint` runs `gosec -quiet ./...`, Makefile:28) and **govulncheck** (`make vuln`,
  Makefile:31) must stay clean. [009] is itself a gosec-class fix (CWE-789); ensure no `G115` integer
  conversion overflow is introduced by the bound check (compare as `int64` consistently;
  `MaxResultFileSize` should be an `int64` const so `bufsz > MaxResultFileSize` is a pure int64 compare,
  no conversion).
- **Verification:** `make test` (race+coverage), `make lint`, `make vuln` (interview verification_strategy).
- **Branch:** `develop` (Git Flow). No config/env/migration changes; archive format unchanged.

---

## 9. External Libraries

No new external dependencies. Standard library only for [009]: `archive/tar` (`tar.Header.Size int64`),
`io` (`io.ReadFull`, `io.ReadAll`, `io.LimitReader`), `encoding/json`. `io.LimitReader` already caps the
read length but NOT the allocation — the fix is an explicit `make` guard, not a reader swap (interview
Technical Decisions: streaming LimitReader rejected because `json.Unmarshal` needs the full buffer).
`internal/pretty` uses only `fmt` and stdlib `math` (aliased `stdmath`, pretty.go:5).

---

## Decomposition Hint

The three items are independently testable and touch disjoint logic, mapping to a natural 3-task split:
- Task [009]: `stat.MaxResultFileSize` + `NewPGresultFile` guard + sysinfo-branch cap + crafted-tar test.
- Task [011]: shared rate helper in `internal/pretty`, `RateUnit` delegates, `rateField` deleted, 4 call
  sites repointed, boundary byte-identity table.
- Task [012]: fixed-width Size variant in `internal/pretty`, applied + `naReserve` to the 5 verbose fields,
  width-static golden/offset test.
[011] and [012] both modify `internal/pretty/pretty.go` AND `top/stat.go` — a wave-conflict to flag for
the tech-spec (sequence them, or one task owns `internal/pretty` additions).
