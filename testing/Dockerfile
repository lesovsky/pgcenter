# lesovsky/pgcenter-testing
# __release_tag__ postgres 13.1 was released 2020-12-03
# __release_tag__ postgres 12.5 was released 2020-12-03
# __release_tag__ postgres 11.10 was released 2020-12-03
# __release_tag__ postgres 10.15 was released 2020-12-03
# __release_tag__ postgres 9.6.20 was released 2020-12-03
# __release_tag__ postgres 9.5.24 was released 2020-12-03
# __release_tag__ golang 1.15.8 was released 2021-02-04
# __release_tag__ golangci-lint v1.36.0 was released 2021-01-26
# __release_tag__ gosec v2.6.1 was released 2021-01-22
FROM ubuntu:20.04

LABEL version="v0.0.1"

ENV DEBIAN_FRONTEND=noninteractive

# install dependencies
RUN apt-get update && \
    apt-get install -y locales curl ca-certificates gnupg make gcc && \
    sed -i '/en_US.UTF-8/s/^# //g' /etc/locale.gen && \
    locale-gen && \
    curl -s https://www.postgresql.org/media/keys/ACCC4CF8.asc -o /tmp/ACCC4CF8.asc && \
    apt-key add /tmp/ACCC4CF8.asc && \
    echo "deb http://apt.postgresql.org/pub/repos/apt focal-pgdg main" > /etc/apt/sources.list.d/pgdg.list && \
    apt-get update && \
    apt-get install -y postgresql-9.5 postgresql-9.6 postgresql-10 postgresql-11 postgresql-12 postgresql-13 \
        postgresql-plperl-9.5 postgresql-plperl-9.6 postgresql-plperl-10 postgresql-plperl-11 postgresql-plperl-12 postgresql-plperl-13 && \
    cpan Module::Build && \
    cpan Linux::Ethtool::Settings && \
    curl -s -L https://dl.google.com/go/go1.15.8.linux-amd64.tar.gz -o - | \
        tar xzf - -C /usr/local && \
    cp /usr/local/go/bin/go /usr/local/bin/ && \
    curl -s -L https://github.com/golangci/golangci-lint/releases/download/v1.36.0/golangci-lint-1.36.0-linux-amd64.tar.gz -o - | \
        tar xzf - -C /usr/local golangci-lint-1.36.0-linux-amd64/golangci-lint && \
    cp /usr/local/golangci-lint-1.36.0-linux-amd64/golangci-lint /usr/local/bin/ && \
    curl -s -L https://github.com/securego/gosec/releases/download/v2.6.1/gosec_2.6.1_linux_amd64.tar.gz -o - | \
        tar xzf - -C /usr/local/bin gosec && \
    mkdir /usr/local/testing/ && \
    rm -rf /var/lib/apt/lists/*

# copy prepare test environment scripts
COPY prepare-test-environment.sh /usr/local/bin/
COPY fixtures.sql /usr/local/testing/

CMD ["echo", "I'm pgcenter-testing v0.0.1"]
