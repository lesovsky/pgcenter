---
status: planned                    # planned -> in_progress -> done
depends_on: ["04"]                 # ID задач-зависимостей (строки: ["01", "02"])
wave: 3                            # волна параллельного выполнения
skills: [documentation-writing]    # МАССИВ скиллов для загрузки
verify: bash — `grep -n 'report type is not specified, quit' doc/release-notes/v0.12.0.md && grep -n 'diff failed' doc/release-notes/v0.12.0.md` # инструмент верификации (опционально: curl, bash, user)
reviewers: [dev-code-reviewer]      # явно указать. Пусто = fallback на defaults
teammate_name:                     # имя агента-исполнителя (опционально; если не задано — генерируется по описанию задачи)
---

# Task 09: User-facing documentation

## Required Skills

Перед выполнением задачи загрузи:
- `/skill:documentation-writing` — [skills/documentation-writing/SKILL.md](~/.claude/skills/documentation-writing/SKILL.md)

The skill's workflows are written for `.claude/skills/project-knowledge/`; this task writes a
different artefact (a user-facing release note). Take from it the writing principles — prose over
dumps, one place per fact, no duplication — and take the **format** from the existing
`doc/release-notes/v0.9.0.md`, which is the real template here.

## Description

This feature ships one breaking CLI change and two behavioural changes that a user can only
recognise if someone wrote them down. This task writes them down, in a new
`doc/release-notes/v0.12.0.md`.

**Why a release note is the whole mitigation, not a footnote.** `pgcenter report -W` becomes a
string flag (`-W w` → `pg_stat_wal`, `-W a` → the new archiver report, Decision 7). The common
legacy invocation is `pgcenter report -W -f dump.tar`, and it does **not** fail with a complaint
about `-W`: pflag takes `-f` as the value of `-W`, `selectReport` finds no known report type, and the
command exits with `report type is not specified, quit`
([cmd/report/report.go:95](../../../cmd/report/report.go)). That message says nothing about `-W`,
nothing about `-f`, and nothing about a flag having changed type. A user staring at it has no way
back to the cause except this file. That is why the tech-spec requires the message to be quoted
**literally** — the user finds their own mistake by matching the text.

The second item is the one known replay incompatibility: a `wal` recording made **on PG 19** by a
pre-0.12 pgcenter holds 7 columns, and 0.12 replays it against the new 8-column PG 19 layout, where
`stats_age` falls inside the diffed range — so `report -W w` fails with an error beginning
`diff failed` ([internal/stat/postgres.go:593](../../../internal/stat/postgres.go)). Narrow (PG 19 is
still beta), accepted in the user-spec, documented rather than fixed.

The third is a short note that the verbose panel's archiving backlog now comes from
`pg_ls_archive_statusdir()`, which tolerates a missing `archive_status` directory where the old
`pg_ls_dir` call errored — so on a cluster whose `archive_status` directory is gone the panel reports
a backlog of zero instead of `n/a` (Decision 11). The gain that pays for it — a `pg_monitor`-only
role finally seeing the backlog at all — belongs in the same note, otherwise the item reads as a pure
regression.

**Scope is fixed by Decision 13, including what is left out, and the omissions are deliberate — do
not "fix" them here:**

- There is **no README or `doc/` flag documentation to update.** `-W` is documented nowhere in the
  tree, and `doc/pgcenter-report-readme.md` carries no flag reference at all — not for `-W`, not for
  `-J`. Writing CLI flag documentation from scratch is separate, unestimated work, recorded in
  Decision 13 so the gap stays visible. Do not start it in this task.
- The flag's **own help string** (what `pgcenter report --help` prints) is changed by **Task 04**, not
  here. Do not edit `cmd/report/report.go`.
- `doc/Changelog` and `docs/roadmap-0.12.0.md` are not this task's targets either. The roadmap is a
  planning document that gets archived when the release ships; the release note is the durable home.

`doc/release-notes/` has been unused since v0.9.0 — this revives an existing convention rather than
inventing one, which is exactly why the new file must look like its neighbours.

## What to do

- Create `doc/release-notes/v0.12.0.md` in the prose style of `doc/release-notes/v0.9.0.md`: same
  heading shape, same section rhythm (an `### Overview` list of one-liners, then the prose sections
  that expand them). v0.9.0 already documents this exact class of change twice — a hotkey that
  changed meaning (`g`/`G`) and a removed flag (`--rate`) — so follow how it does it.
- Document the breaking `-W` change: it is now a string flag; `-W w` gives the `pg_stat_wal` report,
  `-W a` gives the new archiver report. Show the new invocations, and quote **verbatim** the message a
  legacy `pgcenter report -W -f dump.tar` produces: `report type is not specified, quit`. Explain in
  one sentence why that message does not name the flag — pflag consumes `-f` as the value of `-W` —
  so a reader who hits it recognises their own case.
- Document the PG 19 legacy-archive limitation: a `wal` recording made on PG 19 by a pgcenter older
  than 0.12 fails to replay under `report -W w` with an error beginning `diff failed`; recordings made
  on PG 14–18 are unaffected because the layout is chosen from each sample's recorded PostgreSQL
  version. Say plainly that this is a known, accepted limitation, and that the practical answer is to
  re-record on 0.12.
- Add the short archiving-backlog note: the verbose panel (`v`) now reads the backlog through
  `pg_ls_archive_statusdir()`, so a role holding only `pg_monitor` sees a real value instead of `n/a`;
  as a side effect, on a cluster whose `archive_status` directory is missing the panel now shows a
  backlog of zero where it previously showed `n/a`.
- Keep the file to this feature's changes. Release 0.12.0 carries other features ([012]–[020]) that
  will extend this same file later; leave the structure open for them, and do not invent entries,
  summaries or a feature list for work that is not in this branch.
- Do not fabricate a release date. v0.9.0 carries a `Release date:` line because it shipped; 0.12.0
  has not. Either omit the line or leave it explicitly unset (`TBD`) — an invented date is worse than
  no date.

## Acceptance Criteria

- [ ] `doc/release-notes/v0.12.0.md` exists and reads as a sibling of `v0.9.0.md` — prose, not a diff
      dump, with the same heading and section shape.
- [ ] The string `report type is not specified, quit` appears in the file verbatim, attached to the
      `pgcenter report -W -f dump.tar` case, with a one-sentence explanation of why the message does
      not mention `-W`.
- [ ] The file states the new flag forms `-W w` (pg_stat_wal) and `-W a` (archiver) and says plainly
      that the change is breaking for existing scripts.
- [ ] The string `diff failed` appears in the file verbatim, attached to the PG 19 pre-0.12 recording
      case, and the note says PG 14–18 recordings are unaffected.
- [ ] The archiving-backlog note is present: `pg_monitor` roles now see a value instead of `n/a`, and
      a missing `archive_status` directory now yields a zero backlog instead of `n/a`.
- [ ] No release date is invented.
- [ ] `git diff --name-only` lists exactly one file: `doc/release-notes/v0.12.0.md`. In particular
      `cmd/report/report.go` (Task 04), `README.md` and `doc/pgcenter-report-readme.md` are untouched.
- [ ] Every literal quoted from the program matches the source (see Details → Implementation hints);
      nothing is quoted as a screen literal that the code does not actually print.

## Context Files

**Feature artifacts:**
- [017-feat-wal-archiver.md](017-feat-wal-archiver.md) — user-spec: «Граничные случаи» (the two `-W`
  failure shapes and the PG 19 archive exception, with the exact texts), «Дизайн и интерфейс →
  Verbose-панель» (the `n/a` → zero behaviour change), and the last acceptance criterion, which fixes
  what the release notes must contain and states that README is deliberately out of scope
- [017-feat-wal-archiver-tech-spec.md](017-feat-wal-archiver-tech-spec.md) — **Decision 13** (target,
  scope, and what is deliberately left out — the source of truth for this task), Decision 7 (why `-W`
  broke), Decision 11 (`n/a` → zero), Decision 8 (the `pg_monitor` gain that pays for it), **Backward
  Compatibility** (the exact failure shapes), Implementation Tasks → Wave 3 → Task 9
- [017-feat-wal-archiver-decisions.md](017-feat-wal-archiver-decisions.md) — decisions log; read
  Task 04's entry before writing, to quote the flag forms exactly as they landed
- [017-feat-wal-archiver-code-research.md](017-feat-wal-archiver-code-research.md) — background on the
  report/replay path

**Project knowledge:**
- [overview.md](../../../.claude/skills/project-knowledge/overview.md) — what pgcenter is, which
  commands exist, which PostgreSQL versions are supported (this repo has no `project.md`;
  `overview.md` is its equivalent)
- [architecture.md](../../../.claude/skills/project-knowledge/architecture.md) — package layout and
  the record/report data flow, so the PG 19 replay note is described correctly
- [deployment.md](../../../.claude/skills/project-knowledge/deployment.md) — release process:
  GoReleaser generates GitHub release notes from commits, which is why a prose note for a breaking
  change needs its own file in the tree

**Code files:**
- [doc/release-notes/v0.12.0.md](../../../doc/release-notes/v0.12.0.md) — **create** (does not exist
  yet)
- [doc/release-notes/v0.9.0.md](../../../doc/release-notes/v0.9.0.md) — **read only**: the format to
  follow; note how the `g`/`G` hotkey change is described in prose under "New features" and how the
  removed `--rate` flag is a one-liner under "Other"
- [cmd/report/report.go](../../../cmd/report/report.go) — **read only**: the literal
  `report type is not specified, quit` (`:95`), the `-W` flag definition (`:70`) and `selectReport`
  (`:151`) as Task 04 leaves them
- [internal/stat/postgres.go](../../../internal/stat/postgres.go) — **read only**: the wrapped
  `diff failed: %w` error (`:593`) that the PG 19 legacy archive produces
- [top/stat.go](../../../top/stat.go) — **read only**: the verbose replication row (`:711-732`) — how
  the archiving backlog and its `n/a` sentinel are actually rendered
- [internal/pretty/pretty.go](../../../internal/pretty/pretty.go) — **read only**: `Size` (`:9-24`) —
  zero renders as `0`, which is why the backlog note must not quote `0 B` as a screen literal

## Verification Steps

- Run `grep -n 'report type is not specified, quit' doc/release-notes/v0.12.0.md` — one or more hits.
  This is half the `verify` gate.
- Run `grep -n 'diff failed' doc/release-notes/v0.12.0.md` — one or more hits. This is the other half.
- Run `grep -n '\-W w\|\-W a' doc/release-notes/v0.12.0.md` — both new flag forms are present.
- Run `grep -n 'n/a' doc/release-notes/v0.12.0.md` — the backlog note is present.
- Cross-check each quoted literal against its source: `grep -n 'report type is not specified'
  cmd/report/report.go` and `grep -n 'diff failed' internal/stat/postgres.go`. A quoted message that
  does not exist in the code is worse than no quote at all.
- Run `git diff --name-only` — exactly `doc/release-notes/v0.12.0.md`, nothing else.
- Read the file end to end next to `doc/release-notes/v0.9.0.md`: same heading level, same tone, no
  section that exists only in one of them without a reason.

## Details

<!-- All details for task execution — technical, organizational, any other. -->

**Files:**
- `doc/release-notes/v0.12.0.md` — **new file, the only file this task writes.** The directory
  currently holds `v0.8.0.md` and `v0.9.0.md`; both start with `## Release X.Y.Z`, a `Release date:`
  line, one framing sentence, an `### Overview` bullet list, then `### New features` (numbered prose
  items), `### Fixes` and `### Other`. Reuse that skeleton, minus the sections you have nothing to put
  in. The 0.9.0 file's image links point at Google Drive; **do not add images** — you have no
  screenshots for this feature and a broken link is worse than none.

**Dependencies:**
- Task 04 changes `-W` from `bool` to `string` and rewrites the flag's help string
  (`show pg_stat_wal / pg_stat_archiver report (w - wal, a - archiver)`). This task documents what
  Task 04 shipped, so read `cmd/report/report.go` after Task 04 landed and describe the flag as it
  actually is, not as the spec predicted. If they differ, the code wins and the divergence goes in the
  Post-completion report.
- No dependency on Tasks 01–03 or 05–08: the archiver screen, the PG 19 FPI column and the golden
  tests are not documented here beyond `-W a` existing as a report type.

**Edge cases:**
- **The other `-W` failure shape.** `pgcenter report -W` with `-W` as the last token fails earlier,
  in cobra, with `flag needs an argument: 'W' in -W`. It is fine to mention it, but the required one
  is the `-W -f dump.tar` shape — that is the form real scripts have, and the tech-spec singles it out
  for exactly that reason. Do not let the cobra message displace it.
- **`pgcenter report -W x`** (any unmapped letter) fails with the same
  `report type is not specified, quit`, by design (Decision 17 — the mapping is a closed whitelist).
  One clause is enough.
- **Old archives still work.** A reader who sees "breaking change" will ask whether their recordings
  are still readable. Answer it: recordings made on PG 14–18 replay unchanged, because the layout
  comes from each sample's recorded PostgreSQL version. Only the PG 19 pre-0.12 case is broken.

**Implementation hints:**
- **`diff failed` is a wrapped error.** The source is `fmt.Errorf("diff failed: %w", err)`
  (`internal/stat/postgres.go:593`), so what the user sees is `diff failed: <details>`, not the bare
  two words. Write it as `diff failed: …` or as "an error beginning `diff failed`" — the substring
  stays greppable and the claim stays true. Do not present `diff failed` as the complete message.
- **Do not quote `0 B` as a screen literal.** The user-spec and the tech-spec both write the new
  backlog value as `0 B` in prose, but the panel formats sizes through `pretty.Size`, whose zero case
  returns the bare string `"0"` (`internal/pretty/pretty.go:11-12`), rendered into the row as
  `… 0 archiving backlog …` (`top/stat.go:721-731`). `n/a` **is** a real literal
  (`naReserve`/`naLiteral`), so quoting that one is correct. Describe the new state in prose — "shows
  a backlog of zero instead of `n/a`" — rather than inventing a rendering the program does not
  produce. Note this in the Post-completion report: it is a correction to the spec's wording, not a
  deviation from its intent.
- **Lead the backlog item with the gain, not the regression.** The reason the panel changed is that
  `pg_ls_dir` is superuser-only, so the most common monitoring role (`pg_monitor`) never saw the
  backlog at all. Stated in that order the zero-instead-of-`n/a` case reads as what it is — a narrow
  side effect on a damaged data directory.
- Address the reader who has `-W` in a script: what breaks, what the new form is, what error they will
  see. That is the whole job of item one; everything else about the archiver screen belongs to the
  feature sections other tasks and later features will write.
- Keep it short. v0.9.0 spends three to six sentences per item. This file has three items; it does not
  need more than a page.

## Reviewers

- **dev-code-reviewer** → `docs/features/017-feat-wal-archiver/017-feat-wal-archiver-task-09-dev-code-reviewer-review.json`

## Post-completion

- [ ] Записать краткий отчёт в [017-feat-wal-archiver-decisions.md](017-feat-wal-archiver-decisions.md) (Summary: 1-3 предложения, ревью со ссылками на JSON, без таблиц файндингов и дампов)
- [ ] Если отклонились от спека — описать отклонение и причину. Одно расхождение известно заранее:
      спеки пишут новое значение backlog как `0 B`, а программа печатает `0` — в release notes
      формулировка прозой, литерал не цитируется
- [ ] Обновить user-spec/tech-spec если что-то изменилось


## Scope added during Wave 1 (2026-08-06)

Task 04 discovered that **the flag help users actually see is not cobra's.** `printReportHelp()` in
`cmd/help.go` is installed via `SetHelpTemplate`/`SetUsageTemplate` and fully overrides cobra's flag
usage, so the `StringVarP` description task 04 updated never reaches `pgcenter report --help`.
`cmd/help.go:170` still reads `-W, --wal    show pg_stat_wal statistics` — describing a boolean flag
with no selectors. No task in this feature covered `cmd/help.go`: every `help.go` reference in the
tech-spec and the other tasks means `top/help.go`, the TUI screen.

**This task now also updates `cmd/help.go:170`**, following the `SELECTOR` pattern the neighbouring
`-D`, `-X` and `-P` lines already use. Without it, the release notes this task writes would point at
help text nobody sees.

**Also required in the release notes, measured during Wave 1:** the failure exits with **code 0**.
`pgcenter report -W -f dump.tar` prints `report type is not specified, quit` and returns success, so a
wrapper using `|| alert` will not fire and will keep an empty output file. Say so explicitly — this is
the difference between a loud failure and a silent one for anything scripted.
