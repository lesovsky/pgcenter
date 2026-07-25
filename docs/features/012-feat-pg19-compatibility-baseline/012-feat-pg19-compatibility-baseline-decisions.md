# Decisions log — [012] PostgreSQL 19 compatibility baseline

## Task 01 — PG 19 probe: PASSED

**Date:** 2026-07-25
**Outcome:** the feature proceeds on the existing `ubuntu:22.04` base. No base-image migration, rung 1 of
the probe ladder.

### Verified on a live cluster

- `postgresql-19` / `postgresql-plperl-19` version `19~beta2-1.pgdg22.04+1` install on jammy.
- `pg_createcluster 19 main --port 21919 --start` works — `postgresql-common` 293 from the **stable**
  channel already knows the PG 19 layout, so that failure mode is retired.
- `select version()` reports `PostgreSQL 19beta2 (Ubuntu 19~beta2-1.pgdg22.04+1)`.
- `CREATE EXTENSION plperlu` succeeds, so `fixtures.sql` will load.
- The three new columns exist in the live catalog, confirming the documentation check:
  - `pg_stat_progress_vacuum`: `… delay_time, mode, started_by`
  - `pg_stat_progress_analyze`: `… delay_time, started_by`
  - `pg_stat_progress_basebackup`: `… backup_type`
- `pg_stat_progress_cluster` still exposes all 12 columns including `command`, so the nine pgcenter reads
  are intact and no change is needed there.
- The catalog carries 8 progress views — `pg_stat_progress_repack` and `pg_stat_progress_data_checksums`
  are real, as recorded in the roadmap backlog.

### The install recipe — this is the part that is easy to get wrong

**The beta suite must carry the major-version component.** The apt repository publishes each major version
in its own component, so the source line is:

```
deb [signed-by=…/pgdg.gpg] https://apt.postgresql.org/pub/repos/apt jammy-pgdg-testing 19
```

Declaring it as `… jammy-pgdg-testing main` — the obvious-looking form, and the one this feature's planning
documents implied — leaves `postgresql-19` **invisible**: `apt-cache policy` reports candidate `(none)` with
an empty version table, which reads exactly like "the packages do not exist". The first probe run made that
mistake and wrongly concluded the feature was blocked.

Using **only** the `19` component (not `main 19`) is deliberate: it limits what the beta channel can offer
to PG 19 packages, which is a stronger guarantee than the apt pin.

**Installation needs an explicit target release:**

```
apt-get install -y -t jammy-pgdg-testing postgresql-19 postgresql-plperl-19
```

Without it the install fails on `postgresql-client-19 : Depends: libpq5 (>= 19~beta2) but 18.4-… is to be
installed` — the stable channel outranks the beta one for the shared client library.

### Correction to Decision 5a (apt pinning)

The tech-spec's Decision 5a assumed the beta channel needs an apt-preferences pin at priority 100 to stop it
displacing the PG 14–18 packages. **The beta suite already ships `NotAutomatic`, so apt gives it priority
100 on its own** — the pin as designed restates the default rather than adding protection. What actually
provides the guarantee is the component restriction above; what is actually needed is the opposite of a
pin — an explicit target release so the wanted packages can be installed at all.

Verified by installing PG 18 from stable first and PG 19 afterwards: `postgresql-18` stayed at
`18.4-1.pgdg22.04+1` and `postgresql-common` at `293`, both from the stable channel.

**One shared package does move to the beta version and must not be mistaken for a broken pin:** `libpq5`
becomes `19~beta2-1.pgdg22.04+1`, because the client library is shared by every cluster and PG 19's client
requires it. This is expected and backward compatible — the acceptance criterion about package origins has
to exempt it explicitly.

### Note for the executor of task 01

Installing the package auto-creates a `19/main` cluster on its default port, so `pg_createcluster 19 main
--port 21919` fails with "cluster configuration already exists". Drop it first, or create with the port in
one step after dropping. The existing environment script's `pg_lsclusters` guard already deals with this
for versions 14–18; PG 19 needs the same treatment.
