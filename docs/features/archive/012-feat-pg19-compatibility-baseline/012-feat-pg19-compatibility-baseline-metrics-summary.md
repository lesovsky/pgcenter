# Metrics Summary: pg19-compatibility-baseline

## Context

| Dimension | Value |
|-----------|-------|
| Model | Opus 5 (1M context) |
| Feature size | M |
| Started | 2026-07-25 05:46 UTC |
| Completed | 2026-07-25 09:48 UTC |

## Timeline

| Phase | Duration (min) | Touch Time (min) | Wait Time (min) |
|-------|---------------|-------------------|-----------------|
| User Spec | 93 | 71 | 22 |
| Tech Spec | 28 | 28 | 0 |
| Task Decomposition | 34 | 34 | 0 |
| Feature Execution | 74 | 68 | 6 |
| Done | 8 | 8 | 0 |
| **Total** | **237** | **209** | **28** |

## Flow Efficiency

- Total duration: 237 min
- Agent active time: 209 min
- Human wait time: 28 min
- **Flow efficiency: 88%**

## Quality

| Metric | Value |
|--------|-------|
| Validation rounds (by phase) | user_spec: 3, tech_spec: 3, task_decomposition: 2 |
| Validation findings (crit/major/minor) | 3 / 23 / 59 |
| Review rounds (by task) | tasks executed directly, no per-task reviewer agents |
| Review findings (crit/major/minor) | n/a |
| First pass rate | n/a |

## Volume

| Metric | Value |
|--------|-------|
| Interview questions | 21 (8 batches) |
| Tasks | 10 (in 7 waves) |
| Agents spawned | 35 |
| Commits | 16 on the feature line |

## Notes

Two conclusions were reached confidently and then overturned, both worth remembering.

The probe was reported as "no PG 19 packages exist anywhere" after four checks — all four made the same
mistake (no major-version component in the apt source line). The user's pointer to the repository FAQ
turned a "feature blocked until September" verdict into a passing probe. A negative result obtained the
same wrong way four times is not four pieces of evidence.

The security audit's proposed diff guard was adopted into the tech-spec and then removed by the
completeness validator, which showed the reachability premise was false — verified directly in the replay
loop. Validators disagreeing with each other caught what neither caught alone.
