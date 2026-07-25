# Decisions log — [012] PostgreSQL 19 compatibility baseline

## Task 01 — PG 19 probe: FAILED (blocking)

**Date:** 2026-07-25
**Outcome:** the feature is blocked on an external dependency. No code was written.

### What was probed

The probe was run before any Go code, as the tech-spec requires. It did not get as far as building the
test image: the packages the image would install do not exist.

| Base image | apt suite | `postgresql-19` |
|---|---|---|
| ubuntu:22.04 | `jammy-pgdg-testing` | absent — candidate `(none)`, empty version table; `postgresql-plperl-19` not found at all |
| ubuntu:24.04 | `noble-pgdg-testing` | absent — package list ends at `postgresql-18` |
| ubuntu:24.04 | `noble-pgdg` (stable) | absent |
| ubuntu:22.04 | `jammy-pgdg-snapshot` | absent — only 14–18 present |

The suites themselves resolve correctly: on jammy, `apt-cache policy postgresql-common` shows entries from
both `jammy-pgdg` (priority 500) and `jammy-pgdg-testing` (priority 100), so the beta channel was reachable
and participating in version resolution. The absence of the packages is real, not a misconfiguration.

### Why this is not a base-image problem

The user-spec's probe ladder assumed the failure mode would be "beta packages are not built for jammy",
whose remedy is migrating the image to ubuntu:24.04. That is not what happened: PG 19 is missing on **both**
bases and in **all three** PGDG channels, so changing the base image cannot help. This is rung 3 of the
ladder.

For the record, PostgreSQL 19 itself is on schedule — Beta 1 shipped 2026-06-04 and Beta 2 on 2026-07-16,
with GA expected September/October 2026. What has not happened is the PGDG apt packaging of it.

### Decision

Per the user-spec's ladder, rung 3: **the feature is paused in full.** The trimmed variant — landing the
Go code without a live cluster and without the verification pass — was considered and rejected when the
spec was written, because it turns the release's "tested on PG 19" promise back into "should work".

Nothing was implemented: no branch, no code changes, no image build. All planning artifacts (user-spec,
tech-spec, ten task files, code research) are complete and remain valid; the feature resumes at Task 01
when the packages appear.

### Resume trigger

Re-run the probe — a one-minute container check, no image build needed — and proceed if
`apt-cache policy postgresql-19` reports a candidate:

```
docker run --rm ubuntu:22.04 bash -c 'apt-get update -qq >/dev/null 2>&1;
  apt-get install -y -qq curl ca-certificates gnupg >/dev/null 2>&1;
  curl -fsSL https://www.postgresql.org/media/keys/ACCC4CF8.asc | gpg --dearmor -o /usr/share/keyrings/pgdg.gpg;
  echo "deb [signed-by=/usr/share/keyrings/pgdg.gpg] https://apt.postgresql.org/pub/repos/apt jammy-pgdg-testing main" > /etc/apt/sources.list.d/pgdg-testing.list;
  apt-get update -qq >/dev/null 2>&1; apt-cache policy postgresql-19'
```

Note for the resumed run: if the packages appear only for noble and not for jammy, that is rung 2 and the
base-image migration enters this feature, per the user's decision.
