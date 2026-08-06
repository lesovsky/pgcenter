---
status: done                       # planned -> in_progress -> done
depends_on: ["01", "02"]           # ID задач-зависимостей (строки: ["01", "02"])
wave: 2                            # волна параллельного выполнения
skills: [code-writing]             # МАССИВ скиллов для загрузки
verify: bash — `go test ./internal/view/...` and `go test ./record/... -run Test_filterViews` (the full record package needs the CI image; Test_tarRecorder panics without PostgreSQL)
reviewers: [dev-code-reviewer, dev-security-auditor, dev-test-reviewer]  # явно указать. Пусто = fallback на defaults
teammate_name:                     # имя агента-исполнителя (опционально; если не задано — генерируется по описанию задачи)
---

# Task 05: Register the archiver view and update every layout-pinning test

## Required Skills

Перед выполнением задачи загрузи:
- `/skill:code-writing` — [skills/code-writing/SKILL.md](~/.claude/skills/code-writing/SKILL.md)

## Description

Wave 1 built the data source: `query.PgStatArchiverDefault` and `query.SelectStatArchiverQuery`
(task 01), plus the PG 19 branch of the wal selector (task 02). Nothing consumes them yet — the
`archiver` screen does not exist as far as the rest of pgcenter is concerned.

This task registers the `archiver` view in `view.New()` and adds its `case` to `Configure()`. That
single map entry is what makes the screen reachable from the TUI (task 06), recordable by
`pgcenter record`, and replayable by `pgcenter report -W a` (tasks 07, 08). Everything downstream in
Wave 3 depends on this entry existing.

Registering a view in this project is never a one-line change: the view registry is pinned by
count-based tests in two packages, and those counts are the only thing standing between a
mis-registered view and a silent regression. `internal/view/view_test.go` pins the total view count
and per-version availability; `record/record_test.go` pins how many views `filterViews` drops versus
keeps at each version. Both must be updated to new **correct** numbers in the same commit.

`MinRequiredVersion: query.PostgresV14` is the load-bearing field here, and it is mandatory, not
cosmetic (Decision 10). **There is no "common floor" in this project** — the registry still serves
clusters down to PG 9.4 and `TestViews_Configure` exercises `90400`. Left at its zero value the
`archiver` view would be offered on PG ≤ 11, where `pg_ls_archive_statusdir()` does not exist (it
arrived in PG 12): the TUI screen would render a PostgreSQL error every tick, and worse,
`pgcenter record` would abort the **entire recording** on the first query error
(`record/recorder.go:136-139`) — one unsupported screen would take down the whole capture.
`PostgresV14` is also the floor every recent view uses (`wal`, `bgwriter`, `replslots`) and the
oldest cluster in the test image, so the `TestView_VersionOK` rows at ≤ PG13 stay untouched.

This task is the **sole owner** of `internal/view/view_test.go` in this feature — no other task
edits that file. That ownership carries one inherited obligation: task 02 added the PG 19 branch to
`query.SelectStatWALQuery` but deliberately did not touch `view_test.go`, pointing at this task to add
the `wal` assertions to `TestViews_Configure`. Nothing currently pins that `Configure()` actually
carries the new wal layout into the registered view — the selector's own table test covers the
selector, not the wiring. **This task adds that assertion** (step 4b below), so the two tasks stop
pointing at each other and the gap closes.

## What to do

1. Register the `archiver` entry in `New()` (`internal/view/view.go`), structurally next to the
   `wal` (`:129-140`) and `bgwriter` (`:141-152`) entries, with exactly these values:
   - `Name: "archiver"` — must equal the map key; it is also the report type string and the tar
     entry prefix.
   - `MinRequiredVersion: query.PostgresV14` — mandatory, see Description and Decision 10.
   - `QueryTmpl: query.PgStatArchiverDefault` — the seed; `Configure` reassigns it via the selector.
   - `DiffIntvl: [2]int{0, 0}` — nothing is diffed (Decision 2).
   - `Ncols: 9`, `OrderKey: 0`, `OrderDesc: true`.
   - `UniqueKey: 0` — the zero value; column 0 is the constant `'Archiver'` literal.
   - `ColsWidth: map[int]int{}` and `Filters: map[int]*regexp.Regexp{}` — both must be non-nil,
     they are written into in place by the align and filter code.
   - `Msg: "Show archiver statistics (requires archive_mode=on)"` — verbatim (Decision 5).
   - `NotRecordable` — **not set**, the zero value `false`. Pure-SQL views need no recorder change
     (ADR [008]); `record/record.go:filterViews` needs no edit.

2. Add `case "archiver":` to the `switch k` in `Configure()` (`internal/view/view.go:373-413`),
   next to `case "wal":`, assigning `view.QueryTmpl, view.Ncols, view.DiffIntvl` from
   `query.SelectStatArchiverQuery(opts.Version)` and writing the view back into the map.
   The selector is version-independent, so this case re-assigns the same three values the static
   entry already carries — say so in a short comment so a reviewer does not read it as dead code.
   The rationale for keeping it: `stat_io_time` is likewise version-independent and still has its
   case (`view.go:401-403`), and having the case makes a future version branch a one-line change in
   one file instead of two.

3. **Do not touch `case "wal":`** — it already delegates to `query.SelectStatWALQuery`, which task 02
   updated. No edit is required in `view.go` for the PG 19 wal change. The *test-side* pin for that
   delegation is step 4b — production code stays as it is, the assertion is what is missing.

4. Add the guard test `TestNew_ArchiverView` to `internal/view/view_test.go`, modelled on the
   existing `TestNew_BgwriterView` (`:87-97`) and `TestNew_StatIOView` (`:36-49`), pinning
   `NotRecordable`, `MinRequiredVersion`, `Ncols`, `DiffIntvl`, `OrderKey`, `OrderDesc`, `UniqueKey`
   and the `archive_mode=on` substring of `Msg`.

4b. Add `wal` assertions to `TestViews_Configure` (`internal/view/view_test.go:99-253`) — the wiring
   guard task 02 left to this task. The test has a `switch tc.version` with existing `case 190000:`
   and `case 140000:` arms that already assert on the progress screens; add to them, do not
   restructure:
   - in `case 190000:` — `views["wal"].QueryTmpl == query.PgStatWALPG19`, `Ncols == 8`,
     `DiffIntvl == [2]int{2, 6}` (the layout task 02 introduced).
   - in `case 140000:` — `views["wal"].QueryTmpl == query.PgStatWALPG14`, `Ncols == 11`,
     `DiffIntvl == [2]int{2, 9}`, pinning that the PG 14–17 side did not move.
   Add the `archiver` assertion in the same two arms:
   `views["archiver"].QueryTmpl == query.PgStatArchiverDefault`, `Ncols == 9`,
   `DiffIntvl == [2]int{0, 0}` — version-independent, so both arms assert the same values.
   Follow the arms' existing comment style (one line saying what the assertion protects).

5. Update the count-based tests to their new **correct** values (before → after below). Extend the
   explanatory comments that sit next to those numbers — they reason about the counts and would
   become wrong otherwise.

6. Run the tests. `internal/view` runs fully on a bare host: `go test ./internal/view/...`.
   The `record` package does **not** — `Test_tarRecorder` panics without a cluster — so on the host
   scope it: `go test ./record/... -run Test_filterViews`. Run the full `record` package inside the CI
   image (`lesovsky/pgcenter-testing:0.0.11`, command in code-research §8) if the wave gate asks for it.

## TDD Anchor

Tests first, red before green. The registration is a single map entry, so the tests must be written
and observed failing **against the current tree** (no `archiver` key) before `view.go` is touched.

- `internal/view/view_test.go::TestNew_ArchiverView` — the `archiver` key exists in `New()` and
  carries `MinRequiredVersion == query.PostgresV14`, `Ncols == 9`, `DiffIntvl == [2]int{0,0}`,
  `OrderKey == 0`, `OrderDesc == true`, `UniqueKey == 0`, `NotRecordable == false`, and a `Msg`
  containing `archive_mode=on`. Red today: the map lookup returns `ok == false`.
- `internal/view/view_test.go::TestViews_Configure` — the wiring guard inherited from task 02. In the
  existing `case 190000:` arm assert `views["wal"]` has `QueryTmpl == query.PgStatWALPG19`,
  `Ncols == 8`, `DiffIntvl == [2]int{2, 6}`; in `case 140000:` assert `query.PgStatWALPG14`, `11`,
  `[2]int{2, 9}`. Add `views["archiver"]` (`query.PgStatArchiverDefault`, `Ncols == 9`,
  `DiffIntvl == [2]int{0,0}`) to both arms.

  **Be honest about what each half proves.** The `archiver` assertions are red *today* only because the
  view is not registered yet; once step 1 lands they pass, and they can never be reddened by removing
  `case "archiver":` from `Configure()` — `New()` already sets the same `Ncols`/`DiffIntvl`, so the
  selector call changes nothing observable here. They are a regression guard, not a proof that
  `Configure()` is wired. The `wal` half is the real gate, and its mutation (below) is what makes it
  one; it is green from the moment it is written, because task 02 has already landed in Wave 1.
- `internal/view/view_test.go::TestNew` — total view count `27` → **28**. Red until the entry is
  added. The trailing comment "27 is the total number of views have to be returned" moves with it.
- `internal/view/view_test.go::TestView_VersionOK` — rows at version ≥ 140000 gain one:
  `{190000: 27 → 28}`, `{160000: 27 → 28}`, `{140000: 24 → 25}`. The `130000`, `120000`, `110000`
  and `100000` rows are **unchanged** — the PG14 gate drops `archiver` there. This is the test that
  proves `MinRequiredVersion` is set: with the field dropped, the ≤ PG13 rows go red.
- `record/record_test.go::Test_filterViews` — the view is recordable, so it is kept on ≥ PG14 and
  dropped by the version gate below that:
  - `wantV + 1` on the three ≥ PG14 rows: `{190000,"public"}` `27 → 28`, `{140000,""}` `18 → 19`,
    `{140000,"public"}` `24 → 25`. Their `wantN` is unchanged.
  - `wantN + 1` on the four ≤ PG13 rows: `{130000,"public"}` `8 → 9`, `{120000,"public"}`
    `11 → 12`, `{110000,"public"}` `13 → 14`, `{100000,"public"}` `13 → 14`. Their `wantV` is
    unchanged.
  - The block comment at `record/record_test.go:116-134` reasons explicitly about these counts
    (including "all 27 registered views survive") and must be extended, not left stale.

**Mutation checks — run each, see red, revert.** A count test that cannot fail is worse than no test
(`patterns.md`, "Extract the decision out of the unreachable closure"): name the mutation, run it,
and see red before believing the green.

- Delete `MinRequiredVersion` from the entry → `TestView_VersionOK` must go red on the `130000`,
  `120000`, `110000`, `100000` rows, and `Test_filterViews` red on its four ≤ PG13 rows. If those
  stay green, the availability gate is untested.
- Change `Ncols` to `8` or `DiffIntvl` to `[2]int{0,1}` → `TestNew_ArchiverView` must go red.
- Set `NotRecordable: true` → `Test_filterViews` must go red on the **three ≥ PG14 rows only**
  (`{190000,"public"}`, `{140000,""}`, `{140000,"public"}`): there the view stops being counted in
  `wantV` and starts being counted in `wantN`. The **four ≤ PG13 rows stay green** — `filterViews`
  deletes the view and does `filtered++` in both branches (`record/record.go:205-217`), so swapping
  the *reason* for dropping it (NotRecordable instead of the version gate) leaves both numbers
  identical there. Expecting red on every row is arithmetically wrong; red on the three ≥ PG14 rows
  is the correct, sufficient signal.
- Drop `archive_mode=on` from `Msg` → `TestNew_ArchiverView` must go red.

**Never** delete or loosen a count-based test to make the suite pass — update it to the new correct
number. `Test_filterViews` itself runs **without** PostgreSQL, so a stale count there is a real
failure. Beware the environment though: the **whole** `record` package does *not* run on a bare host —
`Test_tarRecorder` (`record/recorder_test.go:37`) calls `stat.GetPostgresProperties` on a nil
connection and **panics** (nil-pointer in `postgres.(*DB).QueryRow`), taking the package binary down.
It does not skip. So scope the host run with `-run Test_filterViews`, or run the package inside the CI
image; and never dismiss a red `record` package as "just the connection-refused tests" without
looking.

## Acceptance Criteria

- [ ] `view.New()` returns an `archiver` entry with `MinRequiredVersion: query.PostgresV14`,
      `QueryTmpl: query.PgStatArchiverDefault`, `Ncols: 9`, `DiffIntvl: [2]int{0,0}`, `OrderKey: 0`,
      `OrderDesc: true`, `UniqueKey: 0`, non-nil empty `ColsWidth` and `Filters`, and
      `Msg == "Show archiver statistics (requires archive_mode=on)"`.
- [ ] `NotRecordable` is left at its zero value `false`, and `record/record.go` is not modified.
- [ ] `Configure()` has a `case "archiver":` that assigns from `query.SelectStatArchiverQuery`, with
      a comment explaining why a functionally no-op case is kept.
- [ ] `case "wal":` in `Configure()` is byte-identical to what it was before this task.
- [ ] `TestNew_ArchiverView` exists and pins every field listed above, including the `Msg` substring.
- [ ] `TestViews_Configure` gained `wal` assertions in its `case 190000:` arm
      (`query.PgStatWALPG19`, `Ncols 8`, `DiffIntvl {2,6}`) and its `case 140000:` arm
      (`query.PgStatWALPG14`, `Ncols 11`, `DiffIntvl {2,9}`), plus `archiver` assertions
      (`query.PgStatArchiverDefault`, `Ncols 9`, `DiffIntvl {0,0}`) in both. The `wal` half closes the
      gap task 02 pointed here; the `archiver` half is a regression guard that cannot be reddened by a
      `Configure()` mutation, and the criterion below is the one that gates the wiring.
- [ ] Mutation check: reverting task 02's PG 19 branch in `SelectStatWALQuery` (so it returns the
      PG 18 layout `7 / {2,5}` at 190000) turns `TestViews_Configure` red. If it stays green the
      wiring is still unpinned.
- [ ] `TestNew` asserts `28`, and its trailing comment says 28.
- [ ] `TestView_VersionOK` asserts `{190000: 28}`, `{160000: 28}`, `{140000: 25}`; the `130000`,
      `120000`, `110000`, `100000` rows are byte-identical to before.
- [ ] `Test_filterViews` asserts `wantV` `28 / 19 / 25` on the three ≥ PG14 rows and `wantN`
      `9 / 12 / 14 / 14` on the four ≤ PG13 rows, and its block comment reflects the new counts.
- [ ] Mutation check: dropping `MinRequiredVersion` from the entry turns `TestView_VersionOK` and
      `Test_filterViews` red on their ≤ PG13 rows. Observed, not reasoned about.
- [ ] Mutation check: setting `NotRecordable: true` turns `Test_filterViews` red on the three ≥ PG14
      rows (the ≤ PG13 rows correctly stay green — the view is dropped either way there).
- [ ] Mutation check: perturbing `Ncols`, `DiffIntvl` or the `Msg` substring turns
      `TestNew_ArchiverView` red.
- [ ] No existing test is deleted, skipped or loosened; every changed number is a new correct value.
- [ ] `go test ./internal/view/...` passes on the host, and so does
      `go test ./record/... -run Test_filterViews`. (The **full** `record` package needs the CI image:
      `Test_tarRecorder` panics on a bare host — that is pre-existing, not caused by this task.)
- [ ] `make lint` is clean.

## Context Files

**Feature artifacts:**
- [017-feat-wal-archiver.md](017-feat-wal-archiver.md) — user-spec
- [017-feat-wal-archiver-tech-spec.md](017-feat-wal-archiver-tech-spec.md) — tech-spec: Task 5,
  Data Models (the 9-column table), Decisions 2, 5 and 10
- [017-feat-wal-archiver-decisions.md](017-feat-wal-archiver-decisions.md) — decisions log
- [017-feat-wal-archiver-code-research.md](017-feat-wal-archiver-code-research.md) — §1.2 (view
  registration), §5.1 (count-based tests), §10.C (the registration block, field-by-field), §10.G
  (every test whose numbers change, with values read from the tree), §8 (CI image command)

**Project knowledge:**
- [overview.md](../../../.claude/skills/project-knowledge/overview.md) — project context
  (there is no `project.md` in this repo; `overview.md` plays that role)
- [architecture.md](../../../.claude/skills/project-knowledge/architecture.md) — package layout,
  PostgreSQL version handling
- [patterns.md](../../../.claude/skills/project-knowledge/patterns.md) — "Adding a New View — test
  counts that must be updated", "Extract the decision out of the unreachable closure" (the
  mutation-must-turn-it-red rule), "Version-Specific Query Pattern"

**Code files:**
- [internal/view/view.go](../../../internal/view/view.go) — add the `archiver` entry to `New()` and
  the `case "archiver":` to `Configure()`
- [internal/view/view_test.go](../../../internal/view/view_test.go) — add `TestNew_ArchiverView`;
  update `TestNew`, `TestView_VersionOK` and `TestViews_Configure` (the `wal`/`archiver` wiring
  assertions inherited from task 02). **Sole owner** — no other task in this feature edits it.
- [record/record_test.go](../../../record/record_test.go) — update `Test_filterViews` counts and its
  block comment
- [internal/query/archiver.go](../../../internal/query/archiver.go) — read-only; produced by task 01,
  source of `PgStatArchiverDefault` and `SelectStatArchiverQuery`
- [internal/query/wal.go](../../../internal/query/wal.go) — read-only; the selector `case "wal":`
  already delegates to, updated by task 02. Source of `PgStatWALPG19` / `PgStatWALPG14` and the
  `8 / {2,6}` and `11 / {2,9}` layouts asserted in `TestViews_Configure`
- [record/record.go](../../../record/record.go) — read-only; `filterViews` (`:199-233`) is the code
  the `record` counts exercise. **Not modified by this task.**

## Verification Steps

- Every test this task *touches* runs without PostgreSQL, but its *package* may not. On the host run
  exactly:
  - `go test ./internal/view/...` — expect PASS on `TestNew`, `TestNew_ArchiverView`,
    `TestViews_Configure` and `TestView_VersionOK`.
  - `go test ./record/... -run Test_filterViews` — expect PASS. Do **not** run the bare
    `go test ./record/...` on the host: `Test_tarRecorder` panics on the nil connection and the whole
    package binary dies, hiding the result you care about.
- Run each mutation from the TDD Anchor, confirm the named test goes **red**, revert the mutation.
  Record which mutation reddened which test in the decisions-log entry.
- `git diff record/record.go` must be empty.
- `git diff internal/view/view.go` must show no change to `case "wal":`.
- `make lint` clean.
- Full `make test` requires PG 14–19 fixtures and runs only inside the project CI image
  (`lesovsky/pgcenter-testing:0.0.11`) — the exact `docker run` command is in code-research §8. Not
  required for this task's own verification, but run it if the wave gate asks for it.

## Details

**Files:**
- `internal/view/view.go` — currently 432 lines. `New()` (`:38-361`) returns the static map of 27
  views; the `wal` entry is at `:129-140` and `bgwriter` at `:141-152` — put `archiver` next to them.
  `Configure()` (`:367-427`) has one `case` per version-aware view in the `switch k` at `:373-413`
  (`case "wal":` is `:389-391`), followed by a second loop (`:417-424`) that runs every view's
  `QueryTmpl` through `query.Format`. Add the entry and the case; change nothing else.
- `internal/view/view_test.go` — currently 280 lines. `TestNew` at `:9-12` (`assert.Equal(t, 27,
  len(v))` with the trailing comment on the same line). Guard tests at `:17-97`, one per non-trivial
  view — `TestNew_BgwriterView` (`:87-97`) is the closest model. `TestViews_Configure` (`:99-253`)
  today asserts only on progress/replication/activity screens and has **no `wal` or `archiver`
  assertion** — this task adds them (step 4b) into the existing `case 190000:` (`:183-192`) and
  `case 140000:` (`:193-202`) arms of its `switch tc.version`; the version table (`:106-173`) and the
  trailing `assert.NotEqual(t, "", v.Query)` loop stay untouched. `TestView_VersionOK` at `:255-280`
  with the seven-row table at `:260-266`.
- `record/record_test.go` — currently 219 lines. `Test_filterViews` at `:109-149`, with the
  reasoning block comment at `:116-134` and the table at `:135-141`. `Test_app_record` (`:32-107`)
  derives its expectation from `countRecordable(view.New())` at `:37` and needs **no** change.
  `TestFilterViews_NotRecordable` (`:151-…`) and `TestFilterViews_dropsExplicitNotRecordable` build
  synthetic view maps and are count-independent — leave them alone.

**Dependencies:**
- Task 01 must be merged first: it creates `internal/query/archiver.go` with
  `PgStatArchiverDefault` and `SelectStatArchiverQuery(_ int) (string, int, [2]int)`. Without it this
  task does not compile.
- Task 02 must be merged first: it adds the PG 19 branch to `SelectStatWALQuery`. This task does not
  edit anything wal-related, but it shares the Wave 1 → Wave 2 gate.
- No new Go packages.

**Edge cases:**
- **PG ≤ 11 / PG 9.4.** `TestViews_Configure` runs `Configure` at `90400`, so the `archiver` entry
  is built and `query.Format`-ed at every version, including ones where the query would fail on a
  live cluster. That is fine — `Configure` never executes SQL. What keeps the screen off those
  clusters is `MinRequiredVersion`, checked by `VersionOK` in the TUI and in `filterViews`.
- **The `%` in the query's `'%.ready'` literal.** `query.Format` is `text/template`, which only
  reacts to `{{`/`}}`, so `Format` is safe. This is task 01's concern; noted here because
  `Configure`'s second loop runs the constant through `Format` and a reviewer may ask.
- **`ColsWidth` / `Filters` must not be nil.** `align.SetAlign` writes into `ColsWidth` and
  `setFilter`/`clearAllFilters` (`top/config_view.go:147-213`) write into `Filters` in place. A nil
  map here is a runtime panic on first use, and no count test would catch it.
- **`DiffIntvl{0,0}` is deliberate, not an omission.** `calculateDelta` short-circuits on a `{0,0}`
  interval and never enters `diff()`, which is exactly what keeps the `'Archiver'` string literal at
  column 0 and the four NULL-able columns away from `strconv.ParseInt` (Decision 2). Do not
  "fix" it into a diffed range.
- **A wrong count can pass locally and fail in CI** for other views, but not for the tests here —
  every one of them (`TestNew`, `TestNew_ArchiverView`, `TestViews_Configure`, `TestView_VersionOK`,
  `Test_filterViews`) executes no SQL and needs no fixture. The only catch is packaging: reaching
  `Test_filterViews` on the host requires `-run`, because a sibling test in the same package panics
  without a cluster. There is no excuse for shipping a stale number in this task.

**Implementation hints:**
- Code-research §10.C.2 has the exact registration block and a field-by-field justification table;
  §10.C.3 has the `Configure` case; §10.C.4 has the guard test in the project's existing style.
  Use them rather than re-deriving.
- `UniqueKey` and `NotRecordable` are intentionally **omitted** from the literal (both zero-valued),
  matching how `wal` and `bgwriter` are written. The guard test still asserts them explicitly, so
  the intent is pinned even though the field is absent from the source.
- `Refresh`, `ShowExtra`, `CollectExtra`, `Verbose`, `IOAvailable`, `DelayAcctAvailable`, `Aligned`,
  `Cols` and `Query` are all runtime/zero-value fields — omit them, exactly as `wal` and `bgwriter`
  do.
- When updating `Test_filterViews`, work the arithmetic per row rather than applying a blanket `+1`:
  the ≥ PG14 rows move `wantV`, the ≤ PG13 rows move `wantN`, and each row's other column stays put.
  Getting this backwards produces a test that passes for the wrong reason.

## Reviewers

- **dev-code-reviewer** → `docs/features/017-feat-wal-archiver/017-feat-wal-archiver-task-05-dev-code-reviewer-review.json`
- **dev-security-auditor** → `docs/features/017-feat-wal-archiver/017-feat-wal-archiver-task-05-dev-security-auditor-review.json`
- **dev-test-reviewer** → `docs/features/017-feat-wal-archiver/017-feat-wal-archiver-task-05-dev-test-reviewer-review.json`

## Post-completion

- [ ] Записать краткий отчёт в [017-feat-wal-archiver-decisions.md](017-feat-wal-archiver-decisions.md) (Summary: 1-3 предложения, ревью со ссылками на JSON, без таблиц файндингов и дампов) — включить, какая мутация какой тест уронила
- [ ] Если отклонились от спека — описать отклонение и причину
- [ ] Обновить user-spec/tech-spec если что-то изменилось
