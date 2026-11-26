#!/usr/bin/env bash
# kind: new
# covers: AC-21, AC-22, AC-26, AC-27, AC-28, AC-31
# base_checkpoint: pg19_stats_age_report_unavailable
# Record a fresh PG19 fixture and verify relation stats_age plus the deliberate live-TUI-only exclusion.

set -euo pipefail
source "$(dirname "${BASH_SOURCE[0]}")/lib/common.sh"
acc_init SC-1
trap 'acc_on_exit $?' EXIT

acc_require_tools docker go python3 timeout sha256sum awk date
cd "$ACC_REPO_ROOT"
acc_build_current "$ACC_TMP/pgcenter-current"

container_name="acc018-${RANDOM}-$$"
if ! docker run --detach --name "$container_name" --publish 127.0.0.1::21919 \
    --entrypoint /bin/bash lesovsky/pgcenter-testing:0.0.11 -c 'sleep infinity' \
    >"$ACC_TMP/container-id" 2>"$ACC_TMP/docker-run.log"; then
    acc_finish 2 environment disposable_pg19_start_failed
fi
ACC_CONTAINER="$container_name"

if ! timeout 180 docker exec -i "$ACC_CONTAINER" bash -s >"$ACC_TMP/cluster-setup.log" 2>&1 <<'CONTAINER_SETUP'
set -euo pipefail
if ! pg_lsclusters | awk '$1 == "19" {found=1} END {exit !found}'; then
    pg_createcluster 19 main
fi
cat >>/var/lib/postgresql/19/main/postgresql.auto.conf <<'EOF'
listen_addresses = '*'
port = 21919
ssl = off
logging_collector = off
shared_preload_libraries = ''
EOF
cat >/etc/postgresql/19/main/pg_hba.conf <<'EOF'
local all all trust
host all all 0.0.0.0/0 trust
EOF
pg_ctlcluster 19 main start
ready=false
for _ in $(seq 1 60); do
    if pg_isready -h 127.0.0.1 -p 21919 -U postgres -t 2 -q; then
        ready=true
        break
    fi
    sleep 1
done
if [ "$ready" != true ]; then
    exit 1
fi
CONTAINER_SETUP
then
    acc_finish 2 environment disposable_pg19_setup_failed
fi
acc_assert_sanitized "$ACC_TMP/cluster-setup.log"

mapping="$(docker port "$ACC_CONTAINER" 21919/tcp | awk 'NR==1 {print $NF}')"
port="${mapping##*:}"
if ! [[ "$port" =~ ^[0-9]+$ ]] || [ -z "$mapping" ]; then
    acc_finish 2 environment disposable_pg19_port_unavailable
fi

if ! timeout 30 docker exec "$ACC_CONTAINER" psql -h 127.0.0.1 -p 21919 -U postgres -d postgres \
    -X -v ON_ERROR_STOP=1 -qAtc 'CREATE DATABASE acc018' >"$ACC_TMP/create-db.log" 2>&1; then
    acc_finish 2 environment fixture_database_create_failed
fi
if ! timeout 30 docker exec -i "$ACC_CONTAINER" psql -h 127.0.0.1 -p 21919 -U postgres -d acc018 \
    -X -v ON_ERROR_STOP=1 -qAt >"$ACC_TMP/fixture.log" 2>&1 <<'FIXTURE_SQL'
CREATE TABLE public.acc018_active (id integer, payload text);
CREATE TABLE public.acc018_untouched (id integer, payload text);
CREATE INDEX acc018_active_idx ON public.acc018_active (id);
CREATE INDEX acc018_untouched_idx ON public.acc018_untouched (id);
INSERT INTO public.acc018_active SELECT i, 'synthetic' FROM generate_series(1, 100) AS i;
INSERT INTO public.acc018_untouched SELECT i, 'synthetic' FROM generate_series(1, 100) AS i;
SELECT pg_stat_reset();
SELECT count(*) FROM public.acc018_active;
BEGIN;
SET LOCAL enable_seqscan = off;
SELECT payload FROM public.acc018_active WHERE id = 1;
COMMIT;
FIXTURE_SQL
then
    acc_finish 2 fixture synthetic_relations_create_failed
fi
acc_assert_sanitized "$ACC_TMP/fixture.log"

capture_sql="SELECT json_build_object('clock', clock_timestamp(), 'server_version_num', current_setting('server_version_num'), 'database_name', current_database(), 'database_stats_reset', (SELECT stats_reset FROM pg_stat_database WHERE datname = current_database()), 'active_table_stats_reset', (SELECT stats_reset FROM pg_stat_user_tables WHERE relname = 'acc018_active'), 'active_index_stats_reset', (SELECT stats_reset FROM pg_stat_user_indexes WHERE indexrelname = 'acc018_active_idx'), 'untouched_table_stats_reset', (SELECT stats_reset FROM pg_stat_user_tables WHERE relname = 'acc018_untouched'), 'untouched_index_stats_reset', (SELECT stats_reset FROM pg_stat_user_indexes WHERE indexrelname = 'acc018_untouched_idx'))"
capture_bounds() {
    local name="$1"
    python3 -c 'import time; print(time.time())' >"$ACC_TMP/$name-host-before"
    if ! timeout 20 docker exec "$ACC_CONTAINER" psql -h 127.0.0.1 -p 21919 -U postgres -d acc018 \
        -X -v ON_ERROR_STOP=1 -qAtc "$capture_sql" >"$ACC_TMP/$name-db.json" 2>"$ACC_TMP/$name-error.log"; then
        acc_finish 2 environment db_clock_capture_failed
    fi
    python3 -c 'import time; print(time.time())' >"$ACC_TMP/$name-host-after"
    acc_assert_sanitized "$ACC_TMP/$name-db.json"
    acc_assert_sanitized "$ACC_TMP/$name-error.log"
}

report_start="$(date -d '1 minute ago' '+%Y-%m-%d %H:%M:%S')"
capture_bounds pre
archive="$ACC_TMP/live.tar"
if acc_run "$ACC_TMP/record.raw" 75 "$ACC_TMP/pgcenter-current" record -i 1s -c 3 \
    -h 127.0.0.1 -p "$port" -U postgres -d acc018 -f "$archive"; then :; else :; fi
record_status=$ACC_RUN_STATUS
capture_bounds post
report_end="$(date -d '1 minute' '+%Y-%m-%d %H:%M:%S')"
if [ "$record_status" -ne 0 ]; then
    acc_finish 2 environment pgcenter_record_failed
fi
sanitize_file "$ACC_TMP/record.raw" "$ACC_TMP/record.sanitized"
acc_assert_sanitized "$ACC_TMP/record.sanitized"

for screen in tables indexes; do
    flag=-T
    if [ "$screen" = indexes ]; then flag=-I; fi
    if acc_run "$ACC_TMP/$screen.out" 45 "$ACC_TMP/pgcenter-current" report "$flag" -f "$archive" \
        -s "$report_start" -e "$report_end" -t 128; then :; else :; fi
    eval "${screen}_status=$ACC_RUN_STATUS"
    acc_assert_sanitized "$ACC_TMP/$screen.out"
done

mode=control
if grep -q 'stats_age' "$ACC_TMP/tables.out" && grep -q 'stats_age' "$ACC_TMP/indexes.out"; then
    mode=feature
elif grep -q 'stats_age' "$ACC_TMP/tables.out" || grep -q 'stats_age' "$ACC_TMP/indexes.out"; then
    acc_finish 1 expectation inconsistent_live_pg19_layout
fi

acc_check live_recording_contract_violation \
    python3 "$ACC_LIB_DIR/reports.py" live --mode "$mode" --archive "$archive" \
    --record "$ACC_TMP/record.sanitized" --tables "$ACC_TMP/tables.out" --tables-status "$tables_status" \
    --indexes "$ACC_TMP/indexes.out" --indexes-status "$indexes_status" \
    --pre-db "$ACC_TMP/pre-db.json" --pre-host-before "$ACC_TMP/pre-host-before" \
    --pre-host-after "$ACC_TMP/pre-host-after" --post-db "$ACC_TMP/post-db.json" \
    --post-host-before "$ACC_TMP/post-host-before" --post-host-after "$ACC_TMP/post-host-after"

acc_hash live_archive "$archive"
acc_hash sanitized_record_INFO "$ACC_TMP/record.sanitized"
acc_hash live_tables_report "$ACC_TMP/tables.out"
acc_hash live_indexes_report "$ACC_TMP/indexes.out"
if [ "$mode" = control ]; then
    acc_finish 1 feature_missing pg19_stats_age_report_unavailable
fi
acc_finish 0 pass complete
