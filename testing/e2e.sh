#!/bin/bash

#
# E2E tests for pgcenter.
#

set -euxo pipefail

rm -rf /tmp/pgcenter-e2e
mkdir /tmp/pgcenter-e2e

#
# pgcenter record
#
for port in 21995 21996 21910 21911 21912 21913 21914; do
  pgcenter record -i 1s -c 5 -h 127.0.0.1 -p $port -U postgres -d pgcenter_fixtures -f /tmp/pgcenter-e2e/pgcenter.stat.$port.tar
done

#
# pgcenter report
#
for port in 21995 21996 21910 21911 21912 21913 21914; do
  for arg in -A -R -D -T -I -S -F -Xm -Xg -Xi -Xt -Xl -Xw -Pv -Pc -Pi -Pa -Pb -Pz; do
    pgcenter report $arg -f /tmp/pgcenter-e2e/pgcenter.stat.$port.tar > /tmp/pgcenter-e2e/pgcenter.stat.$port.$arg.out
  done
done
