#!/usr/bin/env bash
# kind: new
# covers: AC-36
# base_checkpoint: relation_stats_age_descriptions_missing
# Describe the trailing PG19 stats_age column and its stats_reset origin for both relation screens.

set -euo pipefail
source "$(dirname "${BASH_SOURCE[0]}")/lib/common.sh"
acc_init SC-3
trap 'acc_on_exit $?' EXIT

acc_require_tools go python3 timeout
cd "$ACC_REPO_ROOT"
acc_build_current "$ACC_TMP/pgcenter-current"

for screen in tables indexes; do
    flag=-T
    if [ "$screen" = indexes ]; then flag=-I; fi
    output="$ACC_TMP/describe-$screen.out"
    if acc_run "$output" 30 "$ACC_TMP/pgcenter-current" report -d "$flag"; then :; else :; fi
    acc_assert_sanitized "$output"
    if [ "$ACC_RUN_STATUS" -ne 0 ]; then
        acc_finish 2 environment describe_command_failed
    fi
done

if ! grep -q '^- stats_age' "$ACC_TMP/describe-tables.out" && ! grep -q '^- stats_age' "$ACC_TMP/describe-indexes.out"; then
    acc_check relation_stats_age_descriptions_missing \
        python3 "$ACC_LIB_DIR/reports.py" describe --output "$ACC_TMP/describe-tables.out" --status 0 --screen tables
    acc_check relation_stats_age_descriptions_missing \
        python3 "$ACC_LIB_DIR/reports.py" describe --output "$ACC_TMP/describe-indexes.out" --status 0 --screen indexes
    acc_finish 1 feature_missing relation_stats_age_descriptions_missing
fi
if ! grep -q '^- stats_age' "$ACC_TMP/describe-tables.out" || ! grep -q '^- stats_age' "$ACC_TMP/describe-indexes.out"; then
    acc_finish 1 expectation inconsistent_relation_descriptions
fi

for screen in tables indexes; do
    acc_check relation_description_contract_violation \
        python3 "$ACC_LIB_DIR/reports.py" describe --output "$ACC_TMP/describe-$screen.out" \
        --status 0 --screen "$screen" --feature
    acc_hash "describe_${screen}" "$ACC_TMP/describe-$screen.out"
done
acc_finish 0 pass complete
