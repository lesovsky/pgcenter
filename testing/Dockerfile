# lesovsky/pgcenter-testing
# PostgreSQL 14-17 on Ubuntu 22.04
# Go and lint tools are installed by CI, not bundled here
FROM ubuntu:22.04

LABEL version="0.0.9"

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
    apt-get update && \
    apt-get install -y \
        postgresql-14 postgresql-plperl-14 \
        postgresql-15 postgresql-plperl-15 \
        postgresql-16 postgresql-plperl-16 \
        postgresql-17 postgresql-plperl-17 \
        postgresql-18 postgresql-plperl-18 && \
    cpanm --notest Module::Build && \
    cpanm --notest Linux::Ethtool::Settings && \
    mkdir /usr/local/testing/ && \
    rm -rf /var/lib/apt/lists/*

# copy prepare test environment scripts
COPY prepare-test-environment.sh /usr/local/bin/
COPY fixtures.sql /usr/local/testing/

CMD ["echo", "pgcenter-testing 0.0.9: PostgreSQL 14-18 on Ubuntu 22.04"]
