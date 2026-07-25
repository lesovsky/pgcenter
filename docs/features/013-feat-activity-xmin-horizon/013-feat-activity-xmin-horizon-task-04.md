---
status: planned                    # planned -> in_progress -> done
depends_on: ["01"]                 # ID задач-зависимостей (строки: ["01", "02"])
wave: 2                            # волна параллельного выполнения
skills: [code-writing]             # МАССИВ скиллов для загрузки
verify: "bash — go test ./report/... and pgcenter report -d -A"
reviewers: [dev-code-reviewer, dev-security-auditor, dev-test-reviewer]  # явно указать. Пусто = fallback на defaults
teammate_name:                     # имя агента-исполнителя (опционально; если не задано — генерируется по описанию задачи)
---

# Task 04: Describe the new columns and their caveats

## Required Skills

Перед выполнением задачи загрузи:
- `/skill:code-writing` — [skills/code-writing/SKILL.md](~/.claude/skills/code-writing/SKILL.md)

## Description

`pgcenter report -d -A` (and `pgcenter top` context help, which reads the same constants) prints
`pgStatActivityDescription` from `report/describe.go` — a per-column table of *column name → origin →
description*, followed where relevant by a `Note:` line about version dependence. Task 1 gives the
`activity` screen three new columns on PG 13+ — `leader`, `backend_xid`, `horizon_xacts` — and that
description block currently documents a 14-column layout that no longer matches what a PG 13+ server
returns.

This task closes that gap. It is documentation, but it is the operationally load-bearing kind: this
screen is where a DBA decides whether to `pg_terminate_backend` a session during an incident, and
each of the three new columns is easy to misread in a way that leads to the wrong decision. So the
block gets three new rows **and four caveats**, not just the rows.

Why four and not three. The user-spec's acceptance list names three caveats; the tech-spec
Acceptance Criteria and the Risks table name a **fourth** — the privileges one — and it is the one
with the highest operational stakes. `pg_stat_activity` returns NULL rather than an error for
sessions the current role may not inspect, so a blank `horizon_xacts` or `backend_xid` can mean
"this session holds nothing" *or* "you are not allowed to see what it holds". There is nothing to
trap in code (Decision 8 and the user-spec's "Нехватка привилегий" edge case), so this caveat exists
only here — if it is dropped, the risk is unmitigated. Treat "four caveats" as the requirement and
the three-item list in the user-spec as the superseded earlier draft.

Second half of the task: pin the **order** of the description rows with a test. The existing
`Test_describeReport` compares each description against its own constant by identity, so it cannot
notice a row that landed in the wrong slot — a description that lists columns in an order the query
does not emit documents a layout that does not exist. Feature 012 hit exactly this and left the
precedent to copy: `Test_describeProgressColumnOrder` (`report/report_test.go:1216`), which walks a
list of `"\n- name"` markers and asserts each is present and appears after the previous one.

Scope boundary: **do not touch `internal/stat/help.go`.** It carries a near-duplicate of these
description blocks, has no consumers anywhere in the repository, and is already stale (it still
calls `horizon_xacts` by an old name). Editing it would spread the new columns into dead code and
make it look maintained — tech-spec Decision 8. Task 5 registers it as tech debt instead.

## What to do

1. Rewrite the column table inside `pgStatActivityDescription` (`report/describe.go:180`) so it
   lists all 17 columns in exactly the PG 13+ query order from Data Models: `pid`, **`leader`**,
   `cl_addr`, `cl_port`, `datname`, `usename`, `appname`, `backend_type`, `wait_etype`,
   `wait_event`, `state`, **`backend_xid`**, **`horizon_xacts`**, `xact_age`, `query_age`,
   `change_age`, `query`. Only the three new rows are added; the existing fourteen keep their
   current text and simply move.
2. Write each new row in the file's house format — column name, `origin` (the catalog field or
   fields the value derives from, comma-separated when there is more than one, as
   `wait_event_type,wait_event` already does in the progress blocks), then a one-line description —
   and keep the tab alignment of the block intact.
3. Add the version note in the house style of that file (`Note: … are available since PG19.` at
   `describe.go:221` and `:287`), stating that the three new columns require PG 13+.
4. Add four caveats to the block, each stating the thing an operator would otherwise get wrong:
   - `leader` is the leader's pid when there is one and the backend's **own** pid otherwise — it is
     a derived value, not the raw `leader_pid`, which is empty for the leader itself. Without this
     a reader assumes an empty `leader` marks a non-parallel backend.
   - the horizon shown here covers **backend sources only**. Replication slots, prepared
     transactions and standby feedback also hold the xmin horizon and do not appear in
     `pg_stat_activity` at all, so "no session holds it" does not follow from an empty column.
   - `horizon_xacts` on this screen is computed by a **different formula** than the same-named
     column on the `replication` screen (`age(backend_xmin)` here versus a `pg_last_committed_xact()`
     subtraction there — read `internal/query/replication.go:28` and Task 1's activity query for the
     two). Same name, two screens, not directly comparable numbers.
   - a **blank cell may mean missing privileges**, not an absent value: `pg_stat_activity` hides
     other sessions' state from unprivileged roles by returning NULL rather than by erroring.
5. Decide where the caveats live in the block and keep it consistent — the `Note:` line and the
   caveats sit between the column table and the trailing `Details:` URL, which every block in the
   file ends with.
6. Add order tests to `report/report_test.go` modelled on `Test_describeProgressColumnOrder`: one
   that pins the row order of the activity block, and one that pins the presence of the PG 13+ note
   and of all four caveats. The second one is what stops a future edit from quietly dropping a
   caveat, so assert each of the four separately rather than checking a single blob.
7. Re-read the rendered output (`go run . report -d -A`, or `pgcenter report -d -A` from `make
   build`) by eye before declaring done — tab alignment is only visible when rendered.

## TDD Anchor

Tests first, run them, watch them fail for the right reason, then edit `describe.go`.

- `report/report_test.go::Test_describeActivityColumnOrder` — `pgStatActivityDescription` contains a
  row for every one of the 17 PG 13+ columns, and their positions increase monotonically in the
  query's order. Follow `Test_describeProgressColumnOrder` exactly, including its presence-before-order
  check: `strings.Index` returns `-1` for a missing marker and `-1` is less than everything, so an
  ordering-only assertion passes on a row that is not there at all. Markers must be `"\n- name"`, not
  bare `"name"` — `state` and `query` are substrings of other words in the block.
  First run: fails on the three missing rows.
- `report/report_test.go::Test_describeActivityCaveats` — the block carries the PG 13+ note and all
  four caveats, asserted as four independent assertions with distinct messages naming which caveat is
  missing (derived `leader`; backend sources only; formula differs from `replication`; blank may mean
  missing privileges). First run: fails four times plus the note.
- `report/report_test.go::Test_describeReport` — already exists and must stay green; it asserts
  `describeReport(w, "activity")` writes the constant verbatim, so it is the guard that the block is
  still reachable through the dispatch after the edit. Do not modify it.

If `Test_describeActivityColumnOrder` passes on the first run, the markers are wrong — check that
they carry the `"\n- "` prefix.

## Acceptance Criteria

- [ ] `pgStatActivityDescription` lists all 17 columns in the PG 13+ query order, with `leader` at
      position 1, `backend_xid` at 11 and `horizon_xacts` at 12 (0-based, matching Data Models)
- [ ] Each new row names its origin catalog field(s) and reads as a sentence an operator can act on
- [ ] The block carries a `Note:` line, in the house style of the file, stating that the three
      columns require PG 13+
- [ ] The block carries **four** caveats: (1) `leader` is derived — leader's pid if any, otherwise
      the backend's own pid, not the raw `leader_pid`; (2) the horizon covers backend sources only,
      replication slots / prepared transactions / standby feedback are not in `pg_stat_activity`;
      (3) `horizon_xacts` is computed differently here than on the `replication` screen; (4) a blank
      cell may mean the viewer lacks privileges to see another session's state, not that the session
      holds nothing
- [ ] `Test_describeActivityColumnOrder` pins the row order and fails if a row is missing or moved
- [ ] `Test_describeActivityCaveats` asserts each of the four caveats separately, so dropping any one
      of them reddens a test with a message naming it
- [ ] `Test_describeReport` and the rest of `report` are green, and no golden file changed
- [ ] `internal/stat/help.go` is **not** modified (tech-spec Decision 8)
- [ ] No file outside `report/describe.go` and `report/report_test.go` is modified
- [ ] `pgcenter report -d -A` renders the block with intact tab alignment — the three new rows line
      up with the existing fourteen
- [ ] `make lint` green

## Context Files

**Feature artifacts:**
- [013-feat-activity-xmin-horizon.md](013-feat-activity-xmin-horizon.md) — user-spec; the column
  layout table (`:102-119`), "Ключевые компоненты" (`:132-141`) — the wording of what each new
  column means, "Ограничения" (`:232-235`) — the backend-sources-only limitation, Риск 2
  (`:263-267`) — the three reasons a cell can be blank, and the technical decision on why `leader`
  is derived rather than raw (`:296-299`). Note its acceptance list says *three* caveats — that is
  the superseded count; see the tech-spec.
- [013-feat-activity-xmin-horizon-tech-spec.md](013-feat-activity-xmin-horizon-tech-spec.md) —
  Decision 8 (documentation goes only to `describe.go`), Decision 7 and Data Models (the exact
  column order and each column's SQL source), the Risks row on blank-cells-vs-privileges, the
  Acceptance Criteria bullet listing **four** caveats, and the Task 4 entry
- [013-feat-activity-xmin-horizon-code-research.md](013-feat-activity-xmin-horizon-code-research.md) —
  codebase research for the feature; read the sections on `report/describe.go` and on
  `internal/stat/help.go` before assuming anything about either
- [013-feat-activity-xmin-horizon-decisions.md](013-feat-activity-xmin-horizon-decisions.md) —
  decisions log (created during execution); check Task 1's entry for the **final** column order and
  aliases as implemented, in case they deviated from Data Models

**Project knowledge:**
- [overview.md](../../../.claude/skills/project-knowledge/overview.md) — what pgcenter is, which
  stats screens exist, supported PG versions
- [architecture.md](../../../.claude/skills/project-knowledge/architecture.md) — package layout, PG
  version handling, the `SelectStatActivityQuery` branch description (`:61`, stale until Task 5)
- [patterns.md](../../../.claude/skills/project-knowledge/patterns.md) — testing conventions and the
  version-specific query pattern

**Code files:**
- [report/describe.go](../../../report/describe.go) — the file to modify;
  `pgStatActivityDescription` at `:180`, the `Note:` house style at `:221`/`:287`/`:450`, and the
  `replication` block at `:75-76` which is where the other `horizon_xacts` is documented
- [report/report_test.go](../../../report/report_test.go) — the file to modify; the new tests go
  here. `Test_describeReport` (`:1172`) must stay green; `Test_describeProgressColumnOrder`
  (`:1216`) is the pattern to copy
- [internal/query/activity.go](../../../internal/query/activity.go) — read-only; after Task 1 this
  is the source of truth for the column aliases and their order. Transcribe the row order from the
  SQL, not from a spec table
- [internal/query/replication.go](../../../internal/query/replication.go) — read-only; `:28` and
  `:50` carry the `replication` screen's `horizon_xacts` formula, the one the third caveat says is
  different
- [internal/stat/help.go](../../../internal/stat/help.go) — read-only, **must not be modified**;
  open it only to confirm it is the dead duplicate Decision 8 describes

## Verification Steps

- `go test ./report/ -run 'Test_describeActivity|Test_describeReport|Test_describeProgress' -v` —
  the two new tests pass and neither existing describe test broke.
- `go test ./report/...` — the whole package is green.
- `git status --short` — only `report/describe.go` and `report/report_test.go` are modified. Any
  `.golden` appearing as changed means something outside this task's scope moved; investigate, do
  not regenerate.
- `make build && ./bin/pgcenter report -d -A` — the activity block renders with all 17 rows
  aligned in three columns, the PG 13+ note, and four caveats. Read it as an operator would: is it
  clear from the text alone that a blank `horizon_xacts` does not prove the session holds nothing?
- `make lint` — green.
- Drop-a-caveat check: temporarily delete one caveat line, re-run `go test ./report/ -run
  Test_describeActivityCaveats`, confirm it goes red with a message naming that caveat, then
  restore. Report the observed result in the decisions log — this is the only proof the count of
  four is actually enforced.

## Details

**Files:**

- `report/describe.go` — one constant changes: `pgStatActivityDescription` (`:180-199`). It is a raw
  backquoted string, so tabs and newlines are literal. Structure to preserve: a title line, a blank
  line, the `  column\torigin\t\t\tdescription` header, the `- name` rows, a blank line, then the
  `Details:` URL. Version notes elsewhere in the file go on their own line before that URL
  (`:221`, `:287`, `:309`, `:450`). No Go code changes — `describeReport` dispatches on the report
  name and already routes `"activity"` here.
- `report/report_test.go` — two new test functions appended near the existing describe tests.
  Package-level imports `strings`, `testing`, `assert`, `require` are already present (see
  `Test_describeProgressColumnOrder`); no new import should be needed.

**Dependencies:**

- Task 1 must land first — it owns the PG 13+ query and therefore the authoritative alias names and
  order. Writing this description against the spec table while Task 1 landed something slightly
  different is the exact drift the order test is supposed to prevent, and the test would not catch
  it because both would be wrong together. Read `internal/query/activity.go` as merged.
- Task 5 (tech debt register + PK sentence) is a sibling in the same wave and touches only
  `docs/tech-debt.md` and `.claude/skills/project-knowledge/architecture.md` — no conflict.
- No new Go modules.

**Tab alignment — the block's invisible constraint:**

Rendered with 8-column tab stops, the block puts the `origin` field at column 16 and the
`description` at column 40. Verified against the current file:

- `- pid` (5 chars) → two tabs → 16; `- backend_type` (14) → one tab → 16; `- wait_event` (12) →
  one tab → 16.
- `pid` (3) → three tabs → 40; `client_addr` (11) → two tabs → 40; `application_name` (16) → one
  tab → 40.

The three new names all fit the same grid with a single tab after them (`- leader` is 8 chars,
`- backend_xid` 13, `- horizon_xacts` 15 — all land at or before 16). Their origins fit with two
tabs if kept at 15 characters or shorter. A long origin string would blow past column 40 and shift
that row's description out of line, which is only visible when rendered — hence the eyeball step.

**Edge cases — each of these produces a green test that proves nothing:**

- **Markers without the `"\n- "` prefix.** `state`, `query`, `pid` and `leader` all occur inside
  other words and inside the description prose (`leader_pid`, `query_start`, `Process ID`), so a
  bare-substring marker matches the wrong offset and the order assertion becomes meaningless.
- **Ordering asserted without presence.** `strings.Index` returns `-1` for a missing row; `-1` is
  less than any real index, so a missing row satisfies a monotonic-increase check. The existing
  progress test carries a `require.NotEqual(t, -1, pos, …)` for exactly this and its comment says
  so — copy it.
- **All four caveats asserted as one `Contains` on a joined blob.** Then dropping one caveat can
  still pass, or fails with a message that does not say which. Four separate assertions, four
  distinct messages.
- **Caveats worded so loosely that they assert nothing.** The test should key on a phrase specific
  to each caveat's *claim* (privileges, `replication`, `leader_pid`, replication slots), not on a
  generic word like "note" that survives a rewrite that deletes the meaning.
- **Order transcribed from the spec table instead of the merged query.** See Dependencies.
- **`Test_describeReport` updated to match.** It compares against the constant by identity, so it
  needs no change; if it fails after the edit, something in the dispatch broke — investigate rather
  than adjusting the test.

**Implementation hints:**

- Origin field for the new rows: `leader_pid,pid` for `leader` (the comma form is established —
  `wait_event_type,wait_event` at `:212`, `lockers_total,lockers_done` at `:260`), `backend_xid`
  for `backend_xid`, `backend_xmin` for `horizon_xacts`. The origin column names the *catalog*
  fields the value derives from, not the SQL expression.
- `replication`'s row for the same column name is `- horizon_xacts\tbackend_xmin\t\tNumber of
  transactions have to be replayed on this standby server` (`:75`). Read it before writing the
  activity one: the two descriptions must not read as if they were the same number, and the third
  caveat is what carries that.
- The four caveats are four distinct claims; whether they render as one `Note:`-style paragraph or
  as separate lines is a judgement call — pick whichever reads better in the terminal at 80
  columns and keep it consistent with the rest of the file. What is not a judgement call is that
  all four claims are present and each is separately assertable.
- Wording guidance for the privileges caveat, since it is the one an implementer is most likely to
  soften into uselessness: it has to say that a blank cell is *not evidence* the session holds
  nothing — the viewer may simply lack the privileges to see it. "May be empty due to permissions"
  does not carry that.
- Keep the trailing `Details:` URL unchanged; it points at the `pg_stat_activity` view docs, which
  still document all three new source fields.

## Reviewers

- **dev-code-reviewer** → `docs/features/013-feat-activity-xmin-horizon/013-feat-activity-xmin-horizon-task-04-dev-code-reviewer-review.json`
- **dev-security-auditor** → `docs/features/013-feat-activity-xmin-horizon/013-feat-activity-xmin-horizon-task-04-dev-security-auditor-review.json`
- **dev-test-reviewer** → `docs/features/013-feat-activity-xmin-horizon/013-feat-activity-xmin-horizon-task-04-dev-test-reviewer-review.json`

## Post-completion

- [ ] Записать краткий отчёт в [013-feat-activity-xmin-horizon-decisions.md](013-feat-activity-xmin-horizon-decisions.md) (Summary: 1-3 предложения, ревью со ссылками на JSON, без таблиц файндингов и дампов) — включая результат drop-a-caveat проверки и подтверждение, что `internal/stat/help.go` не тронут
- [ ] Если отклонились от спека — описать отклонение и причину
- [ ] Обновить user-spec/tech-spec если что-то изменилось
