#!/usr/bin/env bash
# kind: regression
# covers: AC-33
# Replay tables and indexes across a recorded PostgreSQL 18 to 19 boundary with version-correct layouts.

set -euo pipefail
source "$(dirname "${BASH_SOURCE[0]}")/lib/common.sh"
acc_init SC-2
trap 'acc_on_exit $?' EXIT

acc_require_tools go python3 timeout sha256sum awk
cd "$ACC_REPO_ROOT"
acc_build_current "$ACC_TMP/pgcenter-current"

acc_run "$ACC_TMP/describe-tables.out" 30 "$ACC_TMP/pgcenter-current" report -d -T
if [ "$ACC_RUN_STATUS" -ne 0 ]; then
    acc_finish 2 environment describe_tables_failed
fi
acc_run "$ACC_TMP/describe-indexes.out" 30 "$ACC_TMP/pgcenter-current" report -d -I
if [ "$ACC_RUN_STATUS" -ne 0 ]; then
    acc_finish 2 environment describe_indexes_failed
fi
acc_assert_sanitized "$ACC_TMP/describe-tables.out"
acc_assert_sanitized "$ACC_TMP/describe-indexes.out"

tables_capability=false
indexes_capability=false
if grep -q '^- stats_age' "$ACC_TMP/describe-tables.out"; then tables_capability=true; fi
if grep -q '^- stats_age' "$ACC_TMP/describe-indexes.out"; then indexes_capability=true; fi
if [ "$tables_capability" != "$indexes_capability" ]; then
    acc_finish 2 harness inconsistent_pg19_layout_capability
fi

pg19_layout=old
feature_args=()
if [ "$tables_capability" = true ]; then
    pg19_layout=new
    feature_args=(--feature)
fi
for screen in tables indexes; do
    flag=-T
    sort_column=seq_scan
    if [ "$screen" = indexes ]; then
        flag=-I
        sort_column=scan
    fi
    describe="$ACC_TMP/describe-$screen.out"
    acc_check describe_capability_mismatch \
        python3 "$ACC_LIB_DIR/reports.py" describe --output "$describe" --status 0 --screen "$screen" "${feature_args[@]}"

    archive="$ACC_TMP/mixed-$screen.tar"
    python3 "$ACC_LIB_DIR/fixtures.py" generate --archive "$archive" --kind mixed \
        --screen "$screen" --pg19-layout "$pg19_layout"
    python3 "$ACC_LIB_DIR/fixtures.py" validate --archive "$archive" --kind mixed \
        --screen "$screen" --pg19-layout "$pg19_layout"
    mapfile -t bounds < <(python3 "$ACC_LIB_DIR/fixtures.py" bounds --archive "$archive")
    output="$ACC_TMP/mixed-$screen.out"
    if acc_run "$output" 45 "$ACC_TMP/pgcenter-current" report "$flag" -f "$archive" \
        -s "${bounds[0]}" -e "${bounds[1]}" -o "$sort_column" --desc; then :; else :; fi
    acc_assert_sanitized "$output"
    acc_check mixed_version_replay_mismatch \
        python3 "$ACC_LIB_DIR/reports.py" mixed --output "$output" --status "$ACC_RUN_STATUS" \
        --screen "$screen" --pg19-layout "$pg19_layout"
    acc_hash "mixed_${screen}_${pg19_layout}" "$output"
done

acc_finish 0 pass complete
