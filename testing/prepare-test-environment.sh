#!/bin/bash

# copy configuration files into data directories
for v in 9.5 9.6 10 11 12 13 14; do
  su - postgres -c "mv /etc/postgresql/${v}/main/postgresql.conf /var/lib/postgresql/${v}/main/"
done

# add extra configuration parameters
for v in 9.5 9.6 10 11 12 13 14; do
  port="219$(echo $v |tr -d .)"
  {
    echo "listen_addresses = '*'
port = $port
shared_buffers = 16MB
ssl = on
ssl_cert_file = '/etc/ssl/certs/ssl-cert-snakeoil.pem'
ssl_key_file = '/etc/ssl/private/ssl-cert-snakeoil.key'
logging_collector = on
log_directory = '/var/log/postgresql'
log_filename = 'postgresql-$v.log'
track_io_timing = on
track_functions = all
shared_preload_libraries = 'pg_stat_statements'"
  } >> /var/lib/postgresql/${v}/main/postgresql.auto.conf

  {
    echo "local all all              trust
host all all 0.0.0.0/0 trust"
  } > /etc/postgresql/${v}/main/pg_hba.conf

  mkdir /var/lib/postgresql/${v}/main/conf.d
done

# run main postgres
for v in 9.5 9.6 10 11 12 13 14; do
  su - postgres -c "/usr/lib/postgresql/${v}/bin/pg_ctl -w -t 30 -l /var/log/postgresql/startup-${v}.log -D /var/lib/postgresql/${v}/main start"
done

# install pgcenter schema
for v in 9.5 9.6 10 11 12 13 14; do
  port="219$(echo $v |tr -d .)"
  su - postgres -c "psql -p $port -f /usr/local/testing/fixtures.sql"
done

# check services availability
for v in 9.5 9.6 10 11 12 13 14; do
  port="219$(echo $v |tr -d .)"
  pg_isready -t 10 -h 127.0.0.1 -p "$port" -U postgres -d pgcenter_fixtures
done
