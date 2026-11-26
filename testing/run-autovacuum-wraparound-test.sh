#!/bin/bash

set -Eeuo pipefail

image=lesovsky/pgcenter-testing:0.0.11
self_test=""
if [[ $# -eq 2 && $1 == "--self-test" && ($2 == "timeout" || $2 == "sigkill") ]]; then
    self_test=$2
elif [[ $# -ne 0 ]]; then
    echo "usage: testing/run-autovacuum-wraparound-test.sh [--self-test timeout|sigkill]" >&2
    exit 2
fi

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
go_root=$(go env GOROOT)
go_mod_cache=$(go env GOMODCACHE)
if [[ -z $go_root || -z $go_mod_cache ]]; then
    echo "Go root and module cache must be available" >&2
    exit 1
fi
(cd "$root" && go mod download)

suffix="$$-$(date +%s)"
pg_name="pgcenter-autovacuum-wraparound-pg-$suffix"
runner_name="pgcenter-autovacuum-wraparound-test-$suffix"
runner_job=""

destroy_containers() {
    docker rm -f "$runner_name" >/dev/null 2>&1 || true
    docker rm -f "$pg_name" >/dev/null 2>&1 || true
}
trap destroy_containers EXIT INT TERM HUP

assert_containers_destroyed() {
    local failed=0 name
    for name in "$runner_name" "$pg_name"; do
        if docker inspect "$name" >/dev/null 2>&1; then
            echo "FAIL: container $name still exists" >&2
            failed=1
        fi
    done
    if [[ $failed -ne 0 ]]; then
        return 1
    fi
    echo "PASS: external supervisor destroyed the test runner and dedicated PG 19 container"
}

docker run -d --name "$pg_name" \
    --label pgcenter.autovacuum-wraparound.dedicated=1 \
    --entrypoint bash "$image" \
    -lc 'prepare-test-environment.sh && exec sleep infinity' >/dev/null

identity=""
for _ in $(seq 1 120); do
    if identity=$(docker exec "$pg_name" psql -h 127.0.0.1 -p 21919 -U postgres \
        -d pgcenter_fixtures -Atqc 'SELECT system_identifier::text FROM pg_control_system()' 2>/dev/null) &&
        [[ $identity =~ ^[0-9]+$ ]]; then
        break
    fi
    identity=""
    if [[ $(docker inspect -f '{{.State.Running}}' "$pg_name" 2>/dev/null) != "true" ]]; then
        docker logs --tail 40 "$pg_name" >&2
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

docker run --name "$runner_name" --network "container:$pg_name" \
    --user "$(id -u):$(id -g)" \
    -v "$root:/src:ro" -v "$go_root:$go_root:ro" -v "$go_mod_cache:$go_mod_cache" \
    -w /src "${runner_env[@]}" --entrypoint "$go_root/bin/go" "$image" \
    test ./internal/query/ -run '^Test_StatAutovacuumScoresQuery_WraparoundException$' -count=1 -v &
runner_job=$!

wait_for_runner() {
    local limit=$1
    local status
    local deadline=$((SECONDS + limit))
    while kill -0 "$runner_job" 2>/dev/null; do
        if ((SECONDS >= deadline)); then
            echo "supervisor timeout: forcibly removing the targeted test container" >&2
            docker rm -f "$runner_name" >/dev/null 2>&1 || true
            wait "$runner_job" 2>/dev/null || true
            return 124
        fi
        sleep 0.1
    done
    wait "$runner_job"
    status=$?
    return "$status"
}

wait_for_self_test_start() {
    local logs
    for _ in $(seq 1 300); do
        logs=$(docker logs "$runner_name" 2>&1 || true)
        if [[ $logs == *"self-test $self_test: blocked before database mutation"* ]]; then
            return 0
        fi
        if ! kill -0 "$runner_job" 2>/dev/null; then
            echo "FAIL: self-test process exited before its blocking checkpoint" >&2
            return 1
        fi
        sleep 0.1
    done
    echo "FAIL: self-test process did not reach its blocking checkpoint" >&2
    return 1
}

case "$self_test" in
    "")
        if wait_for_runner 300; then
            status=0
        else
            status=$?
        fi
        destroy_containers
        assert_containers_destroyed
        if [[ $status -ne 0 ]]; then
            exit "$status"
        fi
        echo "PASS: targeted wraparound test completed in the dedicated network namespace"
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
        destroy_containers
        assert_containers_destroyed
        echo "SELF-TEST PASS: timeout caused external destruction without a shared fixture"
        ;;
    sigkill)
        wait_for_self_test_start
        docker kill --signal=SIGKILL "$runner_name" >/dev/null
        if wait "$runner_job"; then
            echo "FAIL: SIGKILL self-test unexpectedly completed successfully" >&2
            exit 1
        fi
        runner_exit=$(docker inspect -f '{{.State.ExitCode}}' "$runner_name")
        if [[ $runner_exit -ne 137 ]]; then
            echo "FAIL: SIGKILL self-test exit code is $runner_exit instead of 137" >&2
            exit 1
        fi
        destroy_containers
        assert_containers_destroyed
        echo "SELF-TEST PASS: test-process SIGKILL caused external destruction without a shared fixture"
        ;;
esac
