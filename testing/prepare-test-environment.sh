#!/bin/bash

set -e

# Create clusters for versions that don't have one yet
for v in 14 15 16 17 18; do
  if ! pg_lsclusters | grep -q "^$v "; then
    pg_createcluster "$v" main
  fi
done

# Configure each cluster
for v in 14 15 16 17 18; do
  port="219${v}"
  datadir="/var/lib/postgresql/${v}/main"

  # Write runtime params into auto.conf (in data directory)
  cat >> "${datadir}/postgresql.auto.conf" << EOF
listen_addresses = '*'
port = ${port}
shared_buffers = 16MB
ssl = on
ssl_cert_file = '/etc/ssl/certs/ssl-cert-snakeoil.pem'
ssl_key_file = '/etc/ssl/private/ssl-cert-snakeoil.key'
logging_collector = on
log_directory = '/var/log/postgresql'
log_filename = 'postgresql-${v}.log'
track_io_timing = on
track_functions = all
shared_preload_libraries = 'pg_stat_statements'
# test-only: enables logical replication slots for integration tests; do NOT copy into production configs
wal_level = logical
EOF

  # Allow all local connections
  cat > "/etc/postgresql/${v}/main/pg_hba.conf" << EOF
local all all              trust
host all all 0.0.0.0/0 trust
EOF
done

# Start all instances
for v in 14 15 16 17 18; do
  pg_ctlcluster "$v" main start
done

# Wait for all instances to be ready
for v in 14 15 16 17 18; do
  port="219${v}"
  until pg_isready -h 127.0.0.1 -p "$port" -U postgres -t 5 -q; do
    echo "Waiting for PostgreSQL $v on port $port..."
  done
done

# Install pgcenter schema
for v in 14 15 16 17 18; do
  port="219${v}"
  su - postgres -c "psql -h 127.0.0.1 -p $port -f /usr/local/testing/fixtures.sql"
done

# Final availability check
for v in 14 15 16 17 18; do
  port="219${v}"
  pg_isready -t 10 -h 127.0.0.1 -p "$port" -U postgres -d pgcenter_fixtures
done
