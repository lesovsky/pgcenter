#!/usr/bin/env bash

# Shared process, evidence, and result handling for the black-box scenarios.

export PYTHONDONTWRITEBYTECODE=1

ACC_SCENARIO="${ACC_SCENARIO:-unknown}"
ACC_ACCEPTANCE_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
ACC_REPO_ROOT="$(cd "$ACC_ACCEPTANCE_DIR/../../../.." && pwd)"
ACC_LIB_DIR="$ACC_ACCEPTANCE_DIR/scenarios/lib"
ACC_TMP=""
ACC_CODE=""
ACC_ORIGIN=""
ACC_CHECKPOINT=""
ACC_CONTAINER=""

acc_init() {
    ACC_SCENARIO="$1"
    umask 077
    ACC_TMP="$(mktemp -d "${TMPDIR:-/tmp}/pgcenter-acc018-${ACC_SCENARIO}.XXXXXX")" || exit 2
    chmod 0700 "$ACC_TMP"
    case "$ACC_TMP" in
        "$ACC_REPO_ROOT"/*) printf 'temporary directory must be outside Git\n' >&2; exit 2 ;;
    esac
}

scenario_cleanup() {
    :
}

acc_cleanup() {
    set +e
    scenario_cleanup
    if [ -n "$ACC_CONTAINER" ]; then
        timeout 15 docker rm -f "$ACC_CONTAINER" >/dev/null 2>&1
        ACC_CONTAINER=""
    fi
    if [ -n "$ACC_TMP" ]; then
        rm -rf -- "$ACC_TMP"
        ACC_TMP=""
    fi
}

acc_on_exit() {
    local status="$1"
    trap - EXIT
    acc_cleanup
    if [ -z "$ACC_CODE" ]; then
        if [ "$status" -eq 0 ]; then
            ACC_CODE=2
            ACC_ORIGIN=harness
            ACC_CHECKPOINT=missing_scenario_result
        else
            ACC_CODE=2
            ACC_ORIGIN=harness
            ACC_CHECKPOINT=unexpected_exit
        fi
    fi
    printf 'ACC_RESULT origin=%s checkpoint=%s\n' "$ACC_ORIGIN" "$ACC_CHECKPOINT"
    exit "$ACC_CODE"
}

acc_finish() {
    ACC_CODE="$1"
    ACC_ORIGIN="$2"
    ACC_CHECKPOINT="$3"
    exit "$ACC_CODE"
}

acc_require_tools() {
    local tool
    for tool in "$@"; do
        if ! command -v "$tool" >/dev/null 2>&1; then
            acc_finish 2 environment "missing_${tool}"
        fi
    done
}

acc_build_current() {
    local output="$1"
    if ! timeout 240 go build -o "$output" ./cmd >"$ACC_TMP/build-current.log" 2>&1; then
        acc_emit_sanitized_log build-current "$ACC_TMP/build-current.log"
        acc_finish 2 environment current_binary_build_failed
    fi
    assert_sanitized "$ACC_TMP/build-current.log" || acc_finish 2 harness build_log_sanitization_failed
}

acc_run() {
    local output="$1"
    local seconds="$2"
    shift 2
    set +e
    timeout "$seconds" "$@" >"$output" 2>&1
    ACC_RUN_STATUS=$?
    set -e
    return "$ACC_RUN_STATUS"
}

acc_hash() {
    local label="$1"
    local file="$2"
    local digest
    digest="$(sha256sum "$file" | awk '{print $1}')"
    printf 'ACC_OBSERVATION name=%s sha256=%s\n' "$label" "$digest"
}

sanitize_file() {
    local input="$1"
    local output="$2"
    local replacements=(--replace "$ACC_TMP=<TMP>")
    if [ -n "$ACC_CONTAINER" ]; then
        replacements+=(--replace "$ACC_CONTAINER=<CONTAINER>")
    fi
    python3 "$ACC_LIB_DIR/sanitize.py" sanitize \
        --input "$input" --output "$output" "${replacements[@]}"
}

assert_sanitized() {
    python3 "$ACC_LIB_DIR/sanitize.py" check --input "$1"
}

secret_scan() {
    assert_sanitized "$1"
}

acc_assert_sanitized() {
    local file="$1"
    if ! assert_sanitized "$file"; then
        acc_finish 2 harness sanitization_check_failed
    fi
}

acc_emit_sanitized_log() {
    local name="$1"
    local input="$2"
    local output="$ACC_TMP/$name.sanitized"
    local line
    if ! sanitize_file "$input" "$output"; then
        printf 'ACC_DIAGNOSTIC name=%s text=redaction_failed\n' "$name"
        return
    fi
    while IFS= read -r line || [ -n "$line" ]; do
        printf 'ACC_DIAGNOSTIC name=%s text=%s\n' "$name" "$line"
    done <"$output"
}

acc_check() {
    local checkpoint="$1"
    shift
    set +e
    "$@"
    local status=$?
    set -e
    case "$status" in
        0) return 0 ;;
        1) acc_finish 1 expectation "$checkpoint" ;;
        *) acc_finish 2 fixture assertion_tool_failed ;;
    esac
}

acc_safe_base_ref() {
    local ref="${ACCEPTANCE_BASE_REF:-master}"
    case "$ref" in
        -*|*..*|*@\{*|*//*|*/|.*) acc_finish 2 harness unsafe_base_ref ;;
    esac
    if ! [[ "$ref" =~ ^[A-Za-z0-9][A-Za-z0-9._/-]*$ ]]; then
        acc_finish 2 harness unsafe_base_ref
    fi
    if ! git -C "$ACC_REPO_ROOT" rev-parse --verify --quiet "$ref^{commit}" >/dev/null; then
        acc_finish 2 environment base_ref_unavailable
    fi
    printf '%s\n' "$ref"
}

acc_build_archived_ref() {
    local ref="$1"
    local name="$2"
    local source="$ACC_TMP/source-$name"
    local archive="$ACC_TMP/source-$name.tar"
    mkdir -p "$source"
    chmod 0700 "$source"
    if ! git -C "$ACC_REPO_ROOT" archive --format=tar --output="$archive" "$ref"; then
        acc_finish 2 environment source_archive_failed
    fi
    if ! tar -xf "$archive" -C "$source"; then
        acc_finish 2 harness source_extract_failed
    fi
    if ! (cd "$source" && timeout 240 go build -buildvcs=false -o "$ACC_TMP/pgcenter-$name" ./cmd) >"$ACC_TMP/build-$name.log" 2>&1; then
        acc_emit_sanitized_log "build-$name" "$ACC_TMP/build-$name.log"
        acc_finish 2 environment "${name}_binary_build_failed"
    fi
    assert_sanitized "$ACC_TMP/build-$name.log" || acc_finish 2 harness build_log_sanitization_failed
}
