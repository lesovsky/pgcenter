# lesovsky/pgcenter-testing
# PostgreSQL 14-19 on Ubuntu 22.04 (19 is a beta from the pgdg-testing channel)
# Go and lint tools are installed by CI, not bundled here
FROM ubuntu:22.04

LABEL version="0.0.11"

ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get update && \
    apt-get install -y \
        locales curl ca-certificates gnupg make gcc git \
        libssl-dev ssl-cert \
        perl libperl-dev cpanminus \
        libfilesys-df-perl && \
    sed -i '/en_US.UTF-8/s/^# //g' /etc/locale.gen && \
    locale-gen && \
    curl -fsSL https://www.postgresql.org/media/keys/ACCC4CF8.asc \
        | gpg --dearmor -o /usr/share/keyrings/pgdg.gpg && \
    echo "deb [signed-by=/usr/share/keyrings/pgdg.gpg] https://apt.postgresql.org/pub/repos/apt jammy-pgdg main" \
        > /etc/apt/sources.list.d/pgdg.list && \
    # The beta channel is declared with the major-version component, not "main": apt.postgresql.org
    # publishes each major in its own component, and with "main" postgresql-19 is simply invisible.
    # Listing only "19" also keeps this channel from offering anything except PG 19 packages.
    # Remove this file at the GA rebuild and install 19 from the stable channel above.
    echo "deb [signed-by=/usr/share/keyrings/pgdg.gpg] https://apt.postgresql.org/pub/repos/apt jammy-pgdg-testing 19" \
        > /etc/apt/sources.list.d/pgdg-testing.list && \
    apt-get update && \
    apt-get install -y \
        postgresql-14 postgresql-plperl-14 \
        postgresql-15 postgresql-plperl-15 \
        postgresql-16 postgresql-plperl-16 \
        postgresql-17 postgresql-plperl-17 \
        postgresql-18 postgresql-plperl-18 && \
    # -t is required: the stable channel outranks the beta one for the shared libpq5, so without an
    # explicit target release the install fails on "Depends: libpq5 (>= 19~beta2)". libpq5 moving to
    # the beta version is expected and backward compatible.
    apt-get install -y -t jammy-pgdg-testing \
        postgresql-19 postgresql-plperl-19 && \
    cpanm --notest Module::Build && \
    cpanm --notest Linux::Ethtool::Settings && \
    mkdir /usr/local/testing/ && \
    rm -rf /var/lib/apt/lists/*

# copy prepare test environment scripts
COPY prepare-test-environment.sh /usr/local/bin/
COPY fixtures.sql /usr/local/testing/

CMD ["echo", "pgcenter-testing 0.0.11: PostgreSQL 14-19 on Ubuntu 22.04"]
