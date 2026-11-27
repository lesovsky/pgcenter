#!/usr/bin/env bash
# kind: new
# covers: AC-41
# base_checkpoint: same_pg19_layout_change_unsupported
# Replay same-PG19 old/new layouts in both directions with blank/populated stats_age and requested sorting.

set -euo pipefail
source "$(dirname "${BASH_SOURCE[0]}")/lib/common.sh"
acc_init SC-10
trap 'acc_on_exit $?' EXIT

acc_require_tools go python3 timeout sha256sum awk
cd "$ACC_REPO_ROOT"
acc_build_current "$ACC_TMP/pgcenter-current"

for screen in tables indexes; do
    flag=-T
    if [ "$screen" = indexes ]; then flag=-I; fi
    output="$ACC_TMP/describe-$screen.out"
    if acc_run "$output" 30 "$ACC_TMP/pgcenter-current" report -d "$flag"; then :; else :; fi
    if [ "$ACC_RUN_STATUS" -ne 0 ]; then acc_finish 2 environment describe_command_failed; fi
    acc_assert_sanitized "$output"
done
if grep -q '^- stats_age' "$ACC_TMP/describe-tables.out" && grep -q '^- stats_age' "$ACC_TMP/describe-indexes.out"; then
    mode=feature
elif ! grep -q '^- stats_age' "$ACC_TMP/describe-tables.out" && ! grep -q '^- stats_age' "$ACC_TMP/describe-indexes.out"; then
    mode=base
else
    acc_finish 1 expectation inconsistent_pg19_layout_capability
fi

for screen in tables indexes; do
    for direction in old-to-new new-to-old; do
        for age in blank populated; do
            archive="$ACC_TMP/layout-$screen-$direction-$age.tar"
            python3 "$ACC_LIB_DIR/fixtures.py" generate --archive "$archive" --kind layout \
                --screen "$screen" --direction "$direction" --age "$age"
            python3 "$ACC_LIB_DIR/fixtures.py" validate --archive "$archive" --kind layout \
                --screen "$screen" --direction "$direction" --age "$age"
        done
    done
    for layout in old new; do
        for age in blank populated; do
            archive="$ACC_TMP/control-$screen-$layout-$age.tar"
            python3 "$ACC_LIB_DIR/fixtures.py" generate --archive "$archive" --kind control \
                --screen "$screen" --control-layout "$layout" --age "$age"
            python3 "$ACC_LIB_DIR/fixtures.py" validate --archive "$archive" --kind control \
                --screen "$screen" --control-layout "$layout" --age "$age"
        done
    done
done

run_control() {
    local screen="$1" layout="$2" age="$3"
    local flag=-T
    if [ "$screen" = indexes ]; then flag=-I; fi
    local archive="$ACC_TMP/control-$screen-$layout-$age.tar"
    local output="$ACC_TMP/control-$screen-$layout-$age.out"
    mapfile -t bounds < <(python3 "$ACC_LIB_DIR/fixtures.py" bounds --archive "$archive")
    if acc_run "$output" 45 "$ACC_TMP/pgcenter-current" report "$flag" -f "$archive" \
        -s "${bounds[0]}" -e "${bounds[1]}" -o stats_age --desc; then :; else :; fi
    if [ "$ACC_RUN_STATUS" -eq 124 ]; then acc_finish 2 environment report_timeout; fi
    acc_assert_sanitized "$output"
    acc_check same_layout_positive_control_failed \
        python3 "$ACC_LIB_DIR/reports.py" control --output "$output" --status "$ACC_RUN_STATUS" \
        --screen "$screen" --layout "$layout" --age "$age"
}

for screen in tables indexes; do
    if [ "$mode" = base ]; then
        run_control "$screen" old blank
        run_control "$screen" old populated
    else
        for layout in old new; do
            for age in blank populated; do
                run_control "$screen" "$layout" "$age"
            done
        done
    fi
done

for screen in tables indexes; do
    flag=-T
    if [ "$screen" = indexes ]; then flag=-I; fi
    for direction in old-to-new new-to-old; do
        for age in blank populated; do
            archive="$ACC_TMP/layout-$screen-$direction-$age.tar"
            output="$ACC_TMP/layout-$screen-$direction-$age.out"
            mapfile -t bounds < <(python3 "$ACC_LIB_DIR/fixtures.py" bounds --archive "$archive")
            if acc_run "$output" 45 "$ACC_TMP/pgcenter-current" report "$flag" -f "$archive" \
                -s "${bounds[0]}" -e "${bounds[1]}" -o stats_age --desc; then :; else :; fi
            if [ "$ACC_RUN_STATUS" -eq 124 ]; then acc_finish 2 environment report_timeout; fi
            acc_assert_sanitized "$output"
            if [ "$mode" = feature ]; then
                acc_check same_pg19_layout_contract_violation \
                    python3 "$ACC_LIB_DIR/reports.py" layout-feature --output "$output" --status "$ACC_RUN_STATUS" \
                    --screen "$screen" --direction "$direction" --age "$age"
            else
                set +e
                python3 "$ACC_LIB_DIR/reports.py" layout-base --output "$output" --status "$ACC_RUN_STATUS" \
                    --screen "$screen" --direction "$direction" --age "$age"
                classification=$?
                set -e
                case "$classification" in
                    0) ;;
                    1) acc_finish 1 expectation unexpected_base_layout_behavior ;;
                    *) acc_finish 2 fixture base_classification_tool_failed ;;
                esac
            fi
            acc_hash "layout_${screen}_${direction}_${age}" "$output"
        done
    done
done

if [ "$mode" = base ]; then
    acc_finish 1 feature_missing same_pg19_layout_change_unsupported
fi
acc_finish 0 pass complete
