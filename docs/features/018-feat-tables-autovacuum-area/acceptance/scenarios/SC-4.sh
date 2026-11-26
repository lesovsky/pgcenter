#!/usr/bin/env bash
# kind: regression
# covers: AC-23, AC-29, AC-30
# Compare legacy PG17/PG18 tables, indexes, functions, and sizes reports byte-for-byte against master.

set -euo pipefail
source "$(dirname "${BASH_SOURCE[0]}")/lib/common.sh"
acc_init SC-4
trap 'acc_on_exit $?' EXIT

acc_require_tools git go python3 tar timeout cmp sha256sum awk
base_ref="$(acc_safe_base_ref)"
acc_build_archived_ref HEAD current
acc_build_archived_ref "$base_ref" base

for version in 170000 180000; do
    archive="$ACC_TMP/legacy-$version.tar"
    python3 "$ACC_LIB_DIR/fixtures.py" generate --archive "$archive" --kind legacy --version "$version"
    python3 "$ACC_LIB_DIR/fixtures.py" validate --archive "$archive" --kind legacy --version "$version"
    mapfile -t bounds < <(python3 "$ACC_LIB_DIR/fixtures.py" bounds --archive "$archive")
    for screen in tables indexes functions sizes; do
        case "$screen" in
            tables) flag=-T ;;
            indexes) flag=-I ;;
            functions) flag=-F ;;
            sizes) flag=-S ;;
        esac
        current_out="$ACC_TMP/$version-$screen-current.out"
        base_out="$ACC_TMP/$version-$screen-base.out"
        if acc_run "$current_out" 45 "$ACC_TMP/pgcenter-current" report "$flag" -f "$archive" \
            -s "${bounds[0]}" -e "${bounds[1]}"; then :; else :; fi
        current_status=$ACC_RUN_STATUS
        if acc_run "$base_out" 45 "$ACC_TMP/pgcenter-base" report "$flag" -f "$archive" \
            -s "${bounds[0]}" -e "${bounds[1]}"; then :; else :; fi
        base_status=$ACC_RUN_STATUS
        acc_assert_sanitized "$current_out"
        acc_assert_sanitized "$base_out"
        acc_check legacy_positive_control_failed \
            python3 "$ACC_LIB_DIR/reports.py" legacy --output "$current_out" --status "$current_status" --screen "$screen"
        acc_check legacy_positive_control_failed \
            python3 "$ACC_LIB_DIR/reports.py" legacy --output "$base_out" --status "$base_status" --screen "$screen"
        if ! cmp -s "$current_out" "$base_out"; then
            acc_finish 1 expectation legacy_report_bytes_differ
        fi
        acc_hash "legacy_${version}_${screen}" "$current_out"
    done
done

acc_finish 0 pass complete
