---
status: planned                    # planned -> in_progress -> done
depends_on: []                     # ID задач-зависимостей (строки: ["01", "02"])
wave: 1                            # волна параллельного выполнения
skills: [code-writing]             # МАССИВ скиллов для загрузки
verify: bash — `go test ./cmd/report/...`; `-W w` and `-W a` map correctly and every unmapped value (including other flags' letters, e.g. `c`, `t`, `g`) fails closed
reviewers: [dev-code-reviewer, dev-security-auditor, dev-test-reviewer]
teammate_name:
---

# Task 04: report CLI — `-W` becomes a string flag

## Required Skills

Перед выполнением задачи загрузи:
- `/skill:code-writing` — [skills/code-writing/SKILL.md](~/.claude/skills/code-writing/SKILL.md)

## Description

`pgcenter report` currently exposes one WAL screen behind a boolean `-W`. This feature adds a second
screen in the same hotkey group (`archiver`), so the flag must name *which* of the two screens the
report is for. Decision 7 settles the shape: `showWAL` changes from `bool` to `string`, `-W w`
selects the wal report and `-W a` selects the archiver report — the exact idiom `-J c|t` already uses
for the two `pg_stat_io` screens (ADR [008], `docs/decisions-log.md:446`).

This is a **knowingly accepted breaking change** to a public CLI flag. The roadmap owner rejected
both a deprecation period and a cobra `NoOptDefVal="w"` shim, because the shim keeps bare `-W`
working while making `-W a` silently fail. Two user-visible failure shapes replace the old behaviour,
and both must stay loud (they are quoted verbatim in the release notes written by Task 9):

- `pgcenter report -W` with `-W` as the last token → cobra/pflag: `flag needs an argument: 'W' in -W`.
- `pgcenter report -W -f dump.tar` — the shape real legacy scripts have — → pflag consumes `-f` as
  the flag's **value** (`showWAL == "-f"`), `selectReport` returns `""`, and the command exits with
  `report type is not specified, quit`.

The second half of the task is the part that carries risk. **Decision 17: the mapping is a CLOSED
whitelist** — only `w` and `a` map, everything else falls through to `""`. This is not stylistic
tidiness. `ReportType` is not an inert label: it is the tar-entry filter in `isFilenameOK`
(`report/report.go:460-476`) and the key into the view map in `newApp` (`report/report.go:85`). An
unmapped value leaking through — say a `default: return "wal"` fallback, or a mapping widened "just
in case" — would select a **zero-value `view.View`** and print a clean, silently **empty** report
instead of erroring. For a report tool that is the worst possible outcome: exit code 0, no data, no
message, and an operator who believes the archive is empty. The whitelist failing closed is the only
thing standing between a typo and a lie.

The change is mechanically tiny — `showWAL` has exactly **four** references in the tree, re-verified
today (struct field, flag definition, `selectReport` case, one test case) and all four change
together. The substance is in the tests.

## What to do

1. Change the `showWAL` field on the `options` struct (`cmd/report/report.go:25`) from `bool` to
   `string`, and update its comment to name both screens. `gofmt` will re-align the comment column of
   the whole struct — that realignment is expected, not scope creep.
2. Change the flag definition (`cmd/report/report.go:70`) from `BoolVarP` to `StringVarP` with an
   empty default and the description **verbatim**:
   `show pg_stat_wal / pg_stat_archiver report (w - wal, a - archiver)`.
   This string is user-visible in `pgcenter report --help` and is pinned by a test — it is the text
   the release notes point at.
3. Replace the `case opts.showWAL:` arm in `selectReport` (`cmd/report/report.go:151-152`)
   **in place** with the guarded outer case plus an inner switch mapping `w` → `"wal"` and
   `a` → `"archiver"`, copying the `case opts.showStatIO != "":` shape (`report.go:157-163`)
   including its deliberate **absence of a `default`**. Position matters: `selectReport` is a
   `switch { case … }` chain where first match wins, so the arm stays exactly where it is, between
   `showFunctions` and `showBgwriter`.
4. Update and extend `Test_selectReport` (`cmd/report/report_test.go:34-73`) and `Test_options_validate`,
   and add the flag-definition and whitelist tests listed under TDD Anchor. Write them first, run
   them, see them fail, then implement.
5. Run the two named mutations from Acceptance Criteria against the finished code and confirm the
   suite turns **red** for each. A whitelist test that cannot fail is worse than no test — that is
   the standing rule in `patterns.md` ("Extract the decision out of the unreachable closure").

## TDD Anchor

Tests to write BEFORE the implementation. All live in `cmd/report/report_test.go` (the file exists —
extend it, never overwrite). Package is `report` (the `cmd/report` one), testify `assert`,
table-driven, matching the file's existing style.

- `cmd/report/report_test.go::Test_selectReport` — existing table, edited rows:
  `{opts: options{showWAL: "w"}, want: "wal"}` (replaces the `showWAL: true` row at `:46`),
  plus `{opts: options{showWAL: "a"}, want: "archiver"}`. Both map to the exact report-type strings
  the view map and the tar-entry prefix use.
- `cmd/report/report_test.go::Test_selectReport_WALWhitelistIsClosed` — a dedicated table proving the
  whitelist fails closed, with each unmapped value asserted to yield `""`. It must cover, explicitly
  and by name: **other flags' letters** `c` and `t` (valid for `-J`), `g` (valid for `-D` and `-X`),
  the case-variant `W`, the spelled-out `wal` and `archiver`, an arbitrary `x`, and the literal
  `-f` — the value pflag actually assigns on the legacy `-W -f dump.tar` invocation. Each row carries
  a comment saying why that value is dangerous, not just that it is invalid.
- `cmd/report/report_test.go::Test_selectReport_WALPrecedence` — `options{showActivity: true, showWAL: "a"}`
  yields `"activity"`. Pins that the new arm did not move up the first-match-wins chain and change
  flag precedence.
- `cmd/report/report_test.go::Test_options_validate` — existing table, one new invalid row:
  `options{showWAL: "-f"}` → error. Additionally assert the message is exactly
  `report type is not specified, quit`, because that literal is what the release notes quote for the
  legacy `-W -f dump.tar` shape.
- `cmd/report/report_test.go::Test_walFlagDefinition` — read-only assertions via
  `CommandDefinition.Flags().Lookup("wal")`: `Value.Type() == "string"`, `Shorthand == "W"`,
  `DefValue == ""`, and `Usage` equal to the verbatim description. This is what makes the
  local-FlagSet test below faithful, and it is the only guard on the help text users read.
- `cmd/report/report_test.go::Test_walFlagPflagFailureShapes` — the two documented failure shapes,
  driven through a **locally constructed** `pflag.FlagSet` that mirrors the `-W` and `-f`
  definitions: `Parse([]string{"-W"})` returns an error whose message is
  `flag needs an argument: 'W' in -W`; `Parse([]string{"-W", "-f", "dump.tar"})` succeeds with the
  `-W` value `"-f"`, which fed to `selectReport` yields `""`. A comment must state that the FlagSet
  is a mirror and that `Test_walFlagDefinition` is what keeps the mirror honest — do not parse
  through `CommandDefinition.Flags()`, which writes the package-level `opts` and makes the suite
  order-dependent.

## Acceptance Criteria

- [ ] `options.showWAL` is a `string`; the flag is `StringVarP` with default `""`.
- [ ] The flag description is exactly `show pg_stat_wal / pg_stat_archiver report (w - wal, a - archiver)`.
- [ ] `selectReport` maps `w` → `"wal"` and `a` → `"archiver"`, and **nothing else** — no `default`
      arm, no normalisation (no lowercasing, no trimming, no prefix matching).
- [ ] Every unmapped value yields `""` → `validate()` returns `report type is not specified, quit`.
      Covered explicitly for `c`, `t`, `g`, `W`, `wal`, `archiver`, `x` and `-f`.
- [ ] Flag precedence is unchanged: the `showWAL` arm sits between `showFunctions` and `showBgwriter`,
      and `-A` still beats `-W`.
- [ ] Exactly the four known `showWAL` sites changed; `grep -rn showWAL --include=*.go .` returns
      four hits and no more.
- [ ] **Mutation 1 — the fallback.** Adding `default: return "wal"` to the inner switch (or making the
      outer arm `return "wal"` unconditionally) turns `Test_selectReport_WALWhitelistIsClosed` **red**.
      Run it, observe the failure, revert. A green suite under this mutation means the whitelist is
      untested and the task is not done.
- [ ] **Mutation 2 — the swapped letter.** Changing `case "a"` to return `"wal"` turns
      `Test_selectReport` **red**. Run it, observe, revert.
- [ ] **Mutation 3 — the help text.** Editing one character of the flag description turns
      `Test_walFlagDefinition` **red**. Run it, observe, revert.
- [ ] `go test ./cmd/report/...` passes; `gofmt -l cmd/report` is empty; `make lint` clean.

## Context Files

**Feature artifacts:**
- [017-feat-wal-archiver.md](docs/features/017-feat-wal-archiver/017-feat-wal-archiver.md) — user-spec (the `-W w` / `-W a` invocations and their edge cases)
- [017-feat-wal-archiver-tech-spec.md](docs/features/017-feat-wal-archiver/017-feat-wal-archiver-tech-spec.md) — tech-spec: Decision 7, **Decision 17**, Backward Compatibility, Task 4
- [017-feat-wal-archiver-decisions.md](docs/features/017-feat-wal-archiver/017-feat-wal-archiver-decisions.md) — decisions log (created on completion)
- [017-feat-wal-archiver-code-research.md](docs/features/017-feat-wal-archiver/017-feat-wal-archiver-code-research.md) — §3.3 (`-J c|t` precedent), §7.5 (all four `showWAL` sites), §10.E (the exact new branch, the pflag message provenance)

**Project knowledge:**
- [overview.md](.claude/skills/project-knowledge/overview.md) — what the report command is for
- [architecture.md](.claude/skills/project-knowledge/architecture.md) — package layout, `cmd/` → `report/` boundary
- [patterns.md](.claude/skills/project-knowledge/patterns.md) — testing conventions; "Extract the decision out of the unreachable closure" (name the mutation, see it red); naming conventions

**Project docs:**
- [decisions-log.md](docs/decisions-log.md) — `:446` ADR [008] "report CLI: one string flag for two screens"

**Code files:**
- [cmd/report/report.go](cmd/report/report.go) — the four production sites: field `:25`, flag `:70`, `selectReport` arm `:151-152`; `validate()` `:91-129` produces the error message
- [cmd/report/report_test.go](cmd/report/report_test.go) — `Test_selectReport` `:34-73` (row `:46` changes; `:65-66` are the `-J`/`-X` invalid-value rows to mirror), `Test_options_validate` `:10-32`
- [report/report.go](report/report.go) — read only: why `ReportType` matters — `newApp` `:85` keys the view map, `isFilenameOK` `:460-476` filters tar entries, `describeReport` `:664-…` keys the description map

## Verification Steps

- `go test ./cmd/report/... -v` — green; the `-v` list names every new test function. Check the names
  appear: `-run` filters are case-sensitive and a typo'd or wrongly-packaged test silently runs
  nothing while still exiting 0.
- Apply Mutation 1 (`default: return "wal"` inside the inner switch), run `go test ./cmd/report/...`,
  confirm **FAIL**, revert. Repeat for Mutations 2 and 3. This step is the actual acceptance gate for
  Decision 17 — a green run under a mutation means the test is decorative.
- `grep -rn "showWAL" --include=*.go .` — exactly four hits (three in `cmd/report/report.go`, one in
  `cmd/report/report_test.go`; the test file will have more rows referencing it — count *sites*, and
  confirm no production file outside `cmd/report/report.go` mentions it).
- `go build ./...` — the whole tree still compiles (nothing else consumes `options`).
- `gofmt -l cmd/report` — empty output.
- `make lint` — clean.
- Sanity by hand: `go run . report -W 2>&1` prints `flag needs an argument: 'W' in -W`;
  `go run . report -W -f /nonexistent.tar 2>&1` prints `report type is not specified, quit` — the
  error must come from the flag mapping, **before** any attempt to open the file.

## Details

**Files:**

- `cmd/report/report.go` — three edits, no more:
  1. `:25` — `showWAL bool` → `showWAL string`, comment becomes
     `// Show stats from pg_stat_wal / pg_stat_archiver`. The field currently sits in the `bool`
     column block of the struct; after the change `gofmt` re-aligns the comment column across the
     whole struct and the diff will carry whitespace-only lines. Let it — do not hand-align.
  2. `:70` — `BoolVarP(&opts.showWAL, "wal", "W", false, "show pg_stat_wal report")` →
     `StringVarP(&opts.showWAL, "wal", "W", "", "show pg_stat_wal / pg_stat_archiver report (w - wal, a - archiver)")`.
     Keep the line in place among the other flag registrations; the ordering there is cosmetic but
     stable.
  3. `:151-152` — the arm becomes `case opts.showWAL != "":` with an inner
     `switch opts.showWAL` mapping `"w"` → `"wal"` and `"a"` → `"archiver"`. No `default`. Shape is
     copied verbatim from `case opts.showStatIO != "":` at `:157-163`.
- `cmd/report/report_test.go` — edit row `:46`, add the archiver row, add the four new test
  functions. The file's conventions: `package report`, `testify/assert`, one table per function,
  `assert.Equal(t, tc.want, …)` in a plain range loop. `Test_walFlagPflagFailureShapes` needs
  `github.com/spf13/pflag` imported directly — it is already an indirect dependency via cobra and is
  pinned in `go.mod` at v1.0.10.

**Dependencies:** none — `depends_on: []`, wave 1. No other wave-1 task touches `cmd/`. Downstream:
Task 7 adds the `"archiver"` entry to `describeReport`'s map and Task 5 registers the `archiver`
view — **until those land, `-W a` maps correctly here but has nothing behind it**. That is expected
inside the feature branch. Do not compensate for it in this task (no "not implemented yet" guard, no
temporary alias to `wal`): a compensating branch is exactly the silent-wrong-report failure Decision
17 exists to prevent, and it would have to be removed again two tasks later.

**Edge cases:**

- `-W ""` (explicit empty value) — the outer `!= ""` guard skips the arm entirely; result `""` →
  "report type is not specified, quit". Same as passing no report flag at all. Correct.
- `-W -f dump.tar` — pflag treats `-f` as the *value*, not as the next flag, because a string flag
  demands an argument. `showWAL == "-f"`, the whitelist rejects it, the command exits non-zero. The
  input file is never opened, so no "file not found" masks the real cause.
- `-W` as the last token — pflag errors before `RunE` ever runs; the message is pflag's own
  (`pflag@v1.0.10/errors.go:75`, `flag needs an argument: %q in -%s`), not ours. Do not try to
  reword it; do assert its exact rendering, since the release notes quote it.
- `-W W`, `-W wal`, `-W Archiver` — all unmapped, all fail closed. Resist adding case-insensitivity
  or full-word aliases: every widening is a new way to select a report the operator did not ask for,
  and none was requested.
- Combining flags, e.g. `-A -W a` — first match wins, `activity` reports. Unchanged behaviour, pinned
  by `Test_selectReport_WALPrecedence`.
- `-d -W a` (describe) short-circuits in `RunMain` before the archive is opened, so it needs no `-f`.
  It will error on the missing description entry until Task 7 lands — not a defect of this task.

**Implementation hints:**

- The whole diff in `selectReport` is six lines; the `-J` arm two cases below is the template. Read it
  first and copy its shape rather than inventing one.
- **No `default` in the inner switch.** Go has no implicit fallthrough, so an unmatched inner switch
  falls out of the *outer* switch too and reaches the final `return ""`. That is the mechanism that
  makes the whitelist closed — it is load-bearing, not an omission. If a linter suggests adding a
  `default`, the answer is a comment, not a `default`.
- Do not "improve" the neighbouring `-J`, `-D`, `-X` or `-P` arms while you are in the function. They
  already fail closed; they are out of scope.
- Do not parse arguments through `CommandDefinition.Flags()` in tests. Those flags are bound to the
  package-level `opts` var, so parsing mutates shared state and makes tests order-dependent; build a
  local `pflag.FlagSet` mirroring the definitions instead, and let `Test_walFlagDefinition` guard
  that the mirror still matches reality.
- Assert error *messages*, not just non-nil errors, wherever the message is a documented contract
  (`report type is not specified, quit`, `flag needs an argument: 'W' in -W`). Task 9 greps the
  release notes for both literals; a test that only checks `assert.Error` would let the wording drift
  out from under them.
- When writing the whitelist table, pick the values from the *other flags' help text* rather than
  inventing letters. `c`/`t` are `-J`'s, `g` is `-D`'s and `-X`'s — those are the values a user
  actually mistypes, and they are what Decision 17 names.

## Reviewers

- **dev-code-reviewer** → `docs/features/017-feat-wal-archiver/017-feat-wal-archiver-task-04-dev-code-reviewer-review.json`
- **dev-security-auditor** → `docs/features/017-feat-wal-archiver/017-feat-wal-archiver-task-04-dev-security-auditor-review.json`
- **dev-test-reviewer** → `docs/features/017-feat-wal-archiver/017-feat-wal-archiver-task-04-dev-test-reviewer-review.json`

## Post-completion

- [ ] Записать краткий отчёт в [017-feat-wal-archiver-decisions.md](docs/features/017-feat-wal-archiver/017-feat-wal-archiver-decisions.md) (Summary: 1-3 предложения, ревью со ссылками на JSON, без таблиц файндингов и дампов)
- [ ] Если отклонились от спека — описать отклонение и причину
- [ ] Обновить user-spec/tech-spec если что-то изменилось
