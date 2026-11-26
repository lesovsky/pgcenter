#!/bin/bash

set -Eeuo pipefail

image=lesovsky/pgcenter-testing:0.0.11
self_test=""
if [[ $# -eq 2 && $1 == "--self-test" && ($2 == "timeout" || $2 == "sigkill" || $2 == "daemon-error") ]]; then
    self_test=$2
elif [[ $# -ne 0 ]]; then
    echo "usage: testing/run-autovacuum-wraparound-test.sh [--self-test timeout|sigkill|daemon-error]" >&2
    exit 2
fi

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)

run_cleanup_daemon_error_control() {
    local real_docker temporary status control_output present name
    local -a names=()
    real_docker=$(command -v docker)
    temporary=$(mktemp -d)
    chmod 700 "$temporary"
    export PGCENTER_AUTOVACUUM_TEST_REAL_DOCKER="$real_docker"
    export PGCENTER_AUTOVACUUM_TEST_STUB_DIR="$temporary"

    cat > "$temporary/docker" <<'STUB'
#!/bin/bash
set -u
case "${1:-}" in
    run)
        for ((i = 1; i <= $#; i++)); do
            if [[ ${!i} == --name ]]; then
                next=$((i + 1))
                printf '%s\n' "${!next}" >> "$PGCENTER_AUTOVACUUM_TEST_STUB_DIR/containers"
            fi
        done
        ;;
    rm)
        touch "$PGCENTER_AUTOVACUUM_TEST_STUB_DIR/daemon-failed"
        echo "simulated Docker daemon error during rm" >&2
        exit 1
        ;;
    inspect)
        if [[ -f $PGCENTER_AUTOVACUUM_TEST_STUB_DIR/daemon-failed ]]; then
            echo "simulated Docker daemon error during inspect" >&2
            exit 1
        fi
        ;;
    container)
        if [[ ${2:-} == ls && -f $PGCENTER_AUTOVACUUM_TEST_STUB_DIR/daemon-failed ]]; then
            echo "simulated Docker daemon error during container ls" >&2
            exit 1
        fi
        ;;
esac
exec "$PGCENTER_AUTOVACUUM_TEST_REAL_DOCKER" "$@"
STUB
    chmod +x "$temporary/docker"

    cleanup_control() {
        local cleanup_name
        if [[ -f $temporary/containers ]]; then
            while IFS= read -r cleanup_name; do
                if [[ $cleanup_name == pgcenter-autovacuum-wraparound-* ]]; then
                    "$real_docker" rm -f "$cleanup_name" >/dev/null 2>&1 || true
                fi
            done < "$temporary/containers"
        fi
        rm -rf "$temporary"
    }
    trap cleanup_control EXIT INT TERM HUP

    if control_output=$(PATH="$temporary:$PATH" "$root/testing/run-autovacuum-wraparound-test.sh" 2>&1); then
        status=0
    else
        status=$?
    fi
    printf '%s\n' "$control_output"

    if [[ -f $temporary/containers ]]; then
        mapfile -t names < "$temporary/containers"
    fi
    if [[ ${#names[@]} -ne 2 ]]; then
        echo "FAIL: cleanup control did not observe both created containers" >&2
        exit 1
    fi
    for name in "${names[@]}"; do
        if [[ $name != pgcenter-autovacuum-wraparound-* ]]; then
            echo "FAIL: unexpected container name in cleanup control" >&2
            exit 1
        fi
        if ! "$real_docker" rm -f "$name" >/dev/null; then
            echo "FAIL: could not remove control container $name" >&2
            exit 1
        fi
    done
    for name in "${names[@]}"; do
        if ! present=$("$real_docker" container ls --all --filter "name=^/$name$" --format '{{.Names}}'); then
            echo "FAIL: could not verify control container removal" >&2
            exit 1
        fi
        if [[ -n $present ]]; then
            echo "FAIL: control container $name still exists" >&2
            exit 1
        fi
    done
    cleanup_control
    trap - EXIT INT TERM HUP

    if [[ $status -eq 0 ]]; then
        echo "FAIL: cleanup daemon errors were accepted as success" >&2
        exit 1
    fi
    if [[ $control_output == *"PASS: external supervisor destroyed"* ||
        $control_output == *"PASS: targeted wraparound test completed"* ]]; then
        echo "FAIL: cleanup daemon errors produced a destruction or targeted PASS" >&2
        exit 1
    fi
    echo "SELF-TEST PASS: cleanup daemon errors were fatal and produced no destruction or targeted PASS"
}

if [[ $self_test == daemon-error ]]; then
    run_cleanup_daemon_error_control
    exit 0
fi

go_root=$(go env GOROOT)
go_mod_cache=$(go env GOMODCACHE)
if [[ -z $go_root || -z $go_mod_cache ]]; then
    echo "Go root and module cache must be available" >&2
    exit 1
fi
if ! command -v python3 >/dev/null 2>&1; then
    echo "python3 is required to validate go test -json evidence" >&2
    exit 1
fi
(cd "$root" && go mod download)

suffix="$$-$(date +%s)"
pg_name="pgcenter-autovacuum-wraparound-pg-$suffix"
runner_name="pgcenter-autovacuum-wraparound-test-$suffix"
pg_id=""
runner_id=""
runner_exit=""

remove_container() {
    local id=$1 output
    if ! output=$(docker rm -f "$id" 2>&1); then
        echo "FAIL: Docker could not remove container $id: $output" >&2
        return 1
    fi
}

destroy_containers() {
    local failed=0
    if [[ -n $runner_id ]] && ! remove_container "$runner_id"; then
        failed=1
    fi
    if [[ -n $pg_id ]] && ! remove_container "$pg_id"; then
        failed=1
    fi
    return "$failed"
}

best_effort_cleanup() {
    destroy_containers >/dev/null 2>&1 || true
}
trap best_effort_cleanup EXIT INT TERM HUP

verify_containers_absent() {
    local failed=0 id present
    for id in "$runner_id" "$pg_id"; do
        if [[ -z $id ]]; then
            continue
        fi
        if ! present=$(docker container ls --all --filter "id=$id" --format '{{.ID}}' 2>&1); then
            echo "FAIL: Docker could not verify absence of container $id: $present" >&2
            failed=1
            continue
        fi
        if [[ -n $present ]]; then
            echo "FAIL: container $id still exists" >&2
            failed=1
        fi
    done
    return "$failed"
}

finalize_cleanup() {
    local failed=0
    if ! destroy_containers; then
        failed=1
    fi
    if ! verify_containers_absent; then
        failed=1
    fi
    if [[ $failed -ne 0 ]]; then
        return 1
    fi
    echo "PASS: external supervisor destroyed the test runner and dedicated PG 19 container"
}

if ! pg_id=$(docker run -d --name "$pg_name" --network none \
    --label pgcenter.autovacuum-wraparound.dedicated=1 \
    --entrypoint bash "$image" \
    -lc 'prepare-test-environment.sh && exec sleep infinity'); then
    echo "FAIL: could not create the dedicated PG 19 container" >&2
    exit 1
fi
if [[ ! $pg_id =~ ^[0-9a-f]{64}$ ]]; then
    echo "FAIL: Docker returned an invalid PG container ID" >&2
    exit 1
fi

identity=""
for _ in $(seq 1 120); do
    if identity=$(docker exec "$pg_id" psql -h 127.0.0.1 -p 21919 -U postgres \
        -d pgcenter_fixtures -Atqc 'SELECT system_identifier::text FROM pg_control_system()' 2>/dev/null) &&
        [[ $identity =~ ^[0-9]+$ ]]; then
        break
    fi
    identity=""
    if ! running=$(docker inspect -f '{{.State.Running}}' "$pg_id" 2>&1); then
        echo "FAIL: could not inspect the dedicated PG 19 container: $running" >&2
        exit 1
    fi
    if [[ $running != "true" ]]; then
        docker logs --tail 40 "$pg_id" >&2
        echo "FAIL: dedicated PG 19 container exited before becoming ready" >&2
        exit 1
    fi
    sleep 1
done
if [[ -z $identity ]]; then
    echo "FAIL: timed out discovering the dedicated PG 19 system_identifier" >&2
    exit 1
fi

runner_env=(
    -e HOME=/tmp
    -e GOCACHE=/tmp/pgcenter-autovacuum-go-build
    -e GOMODCACHE="$go_mod_cache"
    -e GOFLAGS=-mod=readonly
    -e PGCENTER_AUTOVACUUM_WRAPAROUND_DEDICATED=1
    -e PGCENTER_AUTOVACUUM_WRAPAROUND_SYSTEM_IDENTIFIER="$identity"
)
if [[ -n $self_test ]]; then
    runner_env+=(-e PGCENTER_AUTOVACUUM_WRAPAROUND_SELF_TEST="$self_test")
fi

if ! runner_id=$(docker run -d --name "$runner_name" --network "container:$pg_id" \
    --label pgcenter.autovacuum-wraparound.dedicated=1 \
    --user "$(id -u):$(id -g)" \
    -v "$root:/src:ro" -v "$go_root:$go_root:ro" -v "$go_mod_cache:$go_mod_cache" \
    -w /src "${runner_env[@]}" --entrypoint "$go_root/bin/go" "$image" \
    test ./internal/query/ -run '^Test_StatAutovacuumScoresQuery_WraparoundException$' -count=1 -v -json); then
    echo "FAIL: could not start the targeted test runner" >&2
    exit 1
fi
if [[ ! $runner_id =~ ^[0-9a-f]{64}$ ]]; then
    echo "FAIL: Docker returned an invalid test runner container ID" >&2
    exit 1
fi

wait_for_runner() {
    local limit=$1 output status
    if output=$(timeout "${limit}s" docker wait "$runner_id"); then
        if [[ ! $output =~ ^[0-9]+$ ]]; then
            echo "FAIL: Docker returned an invalid runner exit code: $output" >&2
            return 1
        fi
        runner_exit=$output
        return 0
    else
        status=$?
    fi
    if [[ $status -eq 124 ]]; then
        echo "supervisor timeout: targeted test exceeded ${limit}s" >&2
        return 124
    fi
    echo "FAIL: Docker wait failed for the targeted test runner" >&2
    return "$status"
}

wait_for_self_test_start() {
    local logs running
    for _ in $(seq 1 300); do
        if ! logs=$(docker logs "$runner_id" 2>&1); then
            echo "FAIL: could not read self-test logs" >&2
            return 1
        fi
        if [[ $logs == *"self-test $self_test: blocked before database mutation"* ]]; then
            return 0
        fi
        if ! running=$(docker inspect -f '{{.State.Running}}' "$runner_id" 2>&1); then
            echo "FAIL: could not inspect the self-test runner: $running" >&2
            return 1
        fi
        if [[ $running != "true" ]]; then
            echo "FAIL: self-test process exited before its blocking checkpoint" >&2
            return 1
        fi
        sleep 0.1
    done
    echo "FAIL: self-test process did not reach its blocking checkpoint" >&2
    return 1
}

require_targeted_test_pass() {
    printf '%s\n' "$1" | python3 -c '
import json
import sys

target = "Test_StatAutovacuumScoresQuery_WraparoundException"
passes = 0
for line in sys.stdin:
    if not line.strip():
        continue
    try:
        event = json.loads(line)
    except json.JSONDecodeError as error:
        print(f"FAIL: invalid go test JSON event: {error}", file=sys.stderr)
        sys.exit(1)
    if event.get("Test") != target:
        continue
    action = event.get("Action")
    if action in ("fail", "skip"):
        print(f"FAIL: targeted test emitted {action}, not pass", file=sys.stderr)
        sys.exit(1)
    if action == "pass":
        passes += 1
if passes != 1:
    print("FAIL: exactly one targeted test-level pass event is required", file=sys.stderr)
    sys.exit(1)
'
}

case "$self_test" in
    "")
        if wait_for_runner 300; then
            target_status=0
            if [[ $runner_exit -ne 0 ]]; then
                target_status=1
            fi
        else
            target_status=$?
        fi
        if events=$(docker logs "$runner_id" 2>&1); then
            printf '%s\n' "$events"
            if [[ $target_status -eq 0 ]] && ! require_targeted_test_pass "$events"; then
                target_status=1
            fi
        else
            echo "FAIL: could not collect targeted test JSON evidence" >&2
            target_status=1
        fi
        if ! finalize_cleanup; then
            exit 1
        fi
        if [[ $target_status -ne 0 ]]; then
            exit "$target_status"
        fi
        echo "PASS: targeted wraparound test completed with a test-level pass in the dedicated network namespace"
        ;;
    timeout)
        wait_for_self_test_start
        if wait_for_runner 2; then
            echo "FAIL: blocking self-test unexpectedly completed" >&2
            exit 1
        else
            status=$?
        fi
        if [[ $status -ne 124 ]]; then
            echo "FAIL: timeout self-test returned $status instead of 124" >&2
            exit 1
        fi
        if ! finalize_cleanup; then
            exit 1
        fi
        echo "SELF-TEST PASS: timeout caused external destruction without a shared fixture"
        ;;
    sigkill)
        wait_for_self_test_start
        docker kill --signal=SIGKILL "$runner_id" >/dev/null
        if ! wait_for_runner 30; then
            echo "FAIL: could not collect the SIGKILLed runner exit" >&2
            exit 1
        fi
        if [[ $runner_exit -ne 137 ]]; then
            echo "FAIL: SIGKILL self-test exit code is $runner_exit instead of 137" >&2
            exit 1
        fi
        if ! finalize_cleanup; then
            exit 1
        fi
        echo "SELF-TEST PASS: test-process SIGKILL caused external destruction without a shared fixture"
        ;;
esac
