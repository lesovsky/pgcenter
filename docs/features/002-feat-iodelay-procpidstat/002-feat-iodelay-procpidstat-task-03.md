---
status: planned
depends_on: ["01"]
wave: 2
skills: [documentation-writing]
verify: "bash — git diff --stat docs/"
reviewers: [dev-code-reviewer]
teammate_name:
---

# Task 03: Update project knowledge and ADR log

## Required Skills

Before starting, load:
- `/skill:documentation-writing` — [SKILL.md](~/.claude/skills/documentation-writing/SKILL.md)

## Description

This task closes out the documentation trail for feature 002-feat-iodelay-procpidstat. Three project-level docs need updating to reflect that the feature shipped:

1. **`docs/tech-debt.md`** — Tech debt item [001] ("procpidstat iodelay — Netlink taskstats not implemented") was the direct motivation for this feature. It must be moved from Active Debt to Resolved with a note explaining how it was resolved (via `/proc/[pid]/stat` field 42, no Netlink required).

2. **`docs/decisions-log.md`** — The ADR log currently holds an entry from [001-feat-per-process-system-stats] saying iodelay was deferred because Netlink taskstats was required. That decision is now superseded. A new ADR entry for [002-feat-iodelay-procpidstat] must document the three key decisions: using `/proc/[pid]/stat` field 42 instead of Netlink, the `CheckDelayAcctAvailable()` probe via `/proc/sys/kernel/task_delayacct`, and that `%iodelay` is not normalized by cpuCount (wall-clock blocked time).

3. **`docs/features-catalog.md`** — The [001-feat-per-process-system-stats] entry currently says "Per-process iowait (`wa%`) is not available without Netlink taskstats API — deferred to a future issue" and lists 17 columns. Both are now stale. The catalog also needs a new entry for [002-feat-iodelay-procpidstat].

This task runs in Wave 2, after Task 01 (core implementation) completes. No code is written — only documentation files are modified.

## What to do

1. In `docs/tech-debt.md`: move item [001] from the Active Debt section to a new Resolved Debt section (currently the Resolved section says "*(none yet)*"). Add resolution note: "Resolved by 002-feat-iodelay-procpidstat: implemented via `/proc/[pid]/stat` field 42 (`delayacct_blkio_ticks`) — no Netlink required."

2. In `docs/decisions-log.md`: add a new ADR block for [002-feat-iodelay-procpidstat] that covers three decisions:
   - Decision on data source: `/proc/[pid]/stat` field 42 (`delayacct_blkio_ticks`) chosen over Netlink taskstats. Supersedes the prior "iodelay deferred" entry from [001-feat-per-process-system-stats].
   - Decision on availability probe: `CheckDelayAcctAvailable()` reads `/proc/sys/kernel/task_delayacct`; returns true iff content is `"1"`. No PID needed. Covers all cases: kernel support absent, sysctl disabled, sysctl enabled.
   - Decision on `%iodelay` not normalized by cpuCount: formula `ΔIODelay / (itv × ticks) × 100` with no division by `cpuCount`. `delayacct_blkio_ticks` counts wall-clock ticks the process spent blocked, not CPU utilization.

3. In `docs/features-catalog.md`:
   - Update the [001-feat-per-process-system-stats] entry: remove the limitation "Per-process iowait (`wa%`) is not available without Netlink taskstats API — deferred to a future issue"; update the column count reference from 17 to 19 in the key scenarios bullet; add a note that iodelay columns are now available via [002-feat-iodelay-procpidstat].
   - Add a new entry for [002-feat-iodelay-procpidstat] describing the two new columns (`iodelay_total,s` and `%iodelay`), the key scenarios (positive: columns show values; negative: columns show `""` with warning), and the limitations (requires `CONFIG_TASK_DELAY_ACCT=y` and `kernel.task_delayacct=1`; local mode only; probe is single-shot at screen open).

## Acceptance Criteria

- [ ] `docs/tech-debt.md`: item [001] is absent from Active Debt section
- [ ] `docs/tech-debt.md`: item [001] appears in Resolved Debt section with resolution note referencing 002-feat-iodelay-procpidstat and `/proc/[pid]/stat` field 42
- [ ] `docs/decisions-log.md`: new ADR entry for [002-feat-iodelay-procpidstat] exists and covers the three decisions (data source, availability probe, `%iodelay` normalization)
- [ ] `docs/decisions-log.md`: new ADR entry explicitly states it supersedes the prior "iodelay deferred" decision from [001-feat-per-process-system-stats]
- [ ] `docs/features-catalog.md`: [001-feat-per-process-system-stats] entry no longer contains the "Netlink taskstats" limitation sentence
- [ ] `docs/features-catalog.md`: new entry for [002-feat-iodelay-procpidstat] exists describing the two new iodelay columns and their requirements
- [ ] `git diff --stat docs/` shows changes to all three files

## Context Files

**Feature artifacts:**
- [002-feat-iodelay-procpidstat.md](docs/features/002-feat-iodelay-procpidstat/002-feat-iodelay-procpidstat.md) — user-spec
- [002-feat-iodelay-procpidstat-tech-spec.md](docs/features/002-feat-iodelay-procpidstat/002-feat-iodelay-procpidstat-tech-spec.md) — tech-spec
- [002-feat-iodelay-procpidstat-decisions.md](docs/features/002-feat-iodelay-procpidstat/002-feat-iodelay-procpidstat-decisions.md) — decisions log

**Project knowledge:**
- [overview.md](.claude/skills/project-knowledge/overview.md)
- [architecture.md](.claude/skills/project-knowledge/architecture.md)

**Documentation files to modify:**
- [docs/tech-debt.md](docs/tech-debt.md) — move item [001] from Active to Resolved
- [docs/decisions-log.md](docs/decisions-log.md) — add new ADR entry for [002-feat-iodelay-procpidstat]
- [docs/features-catalog.md](docs/features-catalog.md) — update [001] entry and add [002] entry

## Verification Steps

- Run `git diff --stat docs/` — must show modifications to `docs/tech-debt.md`, `docs/decisions-log.md`, and `docs/features-catalog.md`
- Confirm `docs/tech-debt.md` Active Debt no longer contains [001]; Resolved section has the entry with resolution note
- Confirm `docs/decisions-log.md` has a new [002-feat-iodelay-procpidstat] block with three ADR entries and explicit supersession note
- Confirm `docs/features-catalog.md` [001-feat-per-process-system-stats] entry has no "Netlink taskstats" limitation sentence and a new [002-feat-iodelay-procpidstat] entry exists

## Details

**Files:**

- `docs/tech-debt.md` — Current state: item [001] is in Active Debt section titled "procpidstat iodelay — Netlink taskstats not implemented". The Resolved section at the bottom reads "*(none yet)*". Action: cut the full [001] block from Active Debt, replace "*(none yet)*" with the [001] block, and append a resolution note "Resolved by 002-feat-iodelay-procpidstat: implemented via `/proc/[pid]/stat` field 42 (`delayacct_blkio_ticks`) — no Netlink required." Also add a "Resolved:" date line (2026-05-19).

- `docs/decisions-log.md` — Current state: five ADR entries for [001-feat-per-process-system-stats], the last of which is the "iodelay (per-process iowait) deferred — requires Netlink taskstats" entry with status Accepted. Action: append three new ADR entries under a new `[002-feat-iodelay-procpidstat]` header. The first entry must reference and supersede the prior "iodelay deferred" entry. The three entries to add:
  1. `/proc/[pid]/stat` field 42 instead of Netlink taskstats — rationale: no new dependencies, sufficient precision, minimal implementation delta. Supersedes [001-feat-per-process-system-stats] "iodelay deferred" decision.
  2. Availability probe via `/proc/sys/kernel/task_delayacct` sysctl — rationale: authoritative runtime state, readable without root, single probe covers all kernel/runtime cases.
  3. `%iodelay` not normalized by cpuCount — rationale: `delayacct_blkio_ticks` is wall-clock blocked time, not CPU utilization; normalizing by cpuCount would produce misleadingly small numbers on multi-core machines.

- `docs/features-catalog.md` — Current state: [001-feat-per-process-system-stats] entry lists "17 columns" in the key scenarios bullet and has a Limitations bullet: "Per-process iowait (`wa%`) is not available without Netlink taskstats API — deferred to a future issue". Action: (a) remove that Limitations bullet; (b) update the key scenarios bullet to reference 19 columns and note that iodelay columns are added by [002-feat-iodelay-procpidstat]; (c) append a new entry for [002-feat-iodelay-procpidstat] following the same format as the [001] entry: What it does, Key scenarios, Limitations, Touches.

**Dependencies:** Task 01 must be complete — this task documents decisions already implemented in the code. The resolution note and ADR entries must accurately reflect what was actually built.

**Edge cases:**
- The decisions-log.md prior "iodelay deferred" ADR entry must be marked superseded (update its Status field to "Superseded by [002-feat-iodelay-procpidstat]") rather than deleted — it is historical record.
- Do not remove items [002] and [003] from Active Debt in tech-debt.md — they are unrelated to this feature.
- Do not modify the existing five ADR entries for [001-feat-per-process-system-stats] except the "iodelay deferred" entry's Status field.

**Implementation hints:**
- The tech-spec Decisions section contains the authoritative rationale text for all three ADR entries — use it as the source for decision descriptions and rationale.
- The features-catalog entry format follows the [001] entry: bold "What it does" paragraph, "Key scenarios" bullet list, "Limitations" bullet list, "Touches" line.

## Reviewers

- **dev-code-reviewer** → `docs/features/002-feat-iodelay-procpidstat/002-feat-iodelay-procpidstat-task-03-dev-code-reviewer-review.json`

## Post-completion

- [ ] Write report to [002-feat-iodelay-procpidstat-decisions.md](docs/features/002-feat-iodelay-procpidstat/002-feat-iodelay-procpidstat-decisions.md) (include all review rounds with links to JSON reports)
- [ ] If deviated from spec — describe deviation and reason
- [ ] Update user-spec/tech-spec if anything changed
