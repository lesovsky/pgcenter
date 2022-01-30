# lesovsky/pgcenter-testing
# __release_tag__ postgres 14.1 was released 2021-11-11
# __release_tag__ postgres 13.5 was released 2021-11-11
# __release_tag__ postgres 12.9 was released 2021-11-11
# __release_tag__ postgres 11.14 was released 2021-11-11
# __release_tag__ postgres 10.19 was released 2021-11-11
# __release_tag__ postgres 9.6.24 was released 2021-11-11
# __release_tag__ postgres 9.5.24 was released 2020-12-03 -- EOL
# __release_tag__ golang 1.17.6 was released 2022-01-06
# __release_tag__ golangci-lint v1.44.0 was released 2022-01-25
# __release_tag__ gosec v2.9.6 was released 2022-01-10
FROM ubuntu:20.04

LABEL version="0.0.7"

ENV DEBIAN_FRONTEND=noninteractive

# install dependencies
RUN apt-get update && \
    apt-get install -y locales curl ca-certificates gnupg make gcc git && \
    sed -i '/en_US.UTF-8/s/^# //g' /etc/locale.gen && \
    locale-gen && \
    curl -s https://www.postgresql.org/media/keys/ACCC4CF8.asc -o /tmp/ACCC4CF8.asc && \
    apt-key add /tmp/ACCC4CF8.asc && \
    echo "deb http://apt.postgresql.org/pub/repos/apt focal-pgdg main 14" > /etc/apt/sources.list.d/pgdg.list && \
    apt-get update && \
    apt-get install -y postgresql-9.5 postgresql-9.6 postgresql-10 postgresql-11 postgresql-12 postgresql-13 postgresql-14 \
        postgresql-plperl-9.5 postgresql-plperl-9.6 postgresql-plperl-10 postgresql-plperl-11 postgresql-plperl-12 postgresql-plperl-13 postgresql-plperl-14\
        libfilesys-df-perl && \
    cpan Module::Build && \
    cpan Linux::Ethtool::Settings && \
    curl -s -L https://go.dev/dl/go1.17.6.linux-amd64.tar.gz -o - | \
        tar xzf - -C /usr/local && \
    cp /usr/local/go/bin/go /usr/local/bin/ && \
    curl -s -L https://github.com/golangci/golangci-lint/releases/download/v1.44.0/golangci-lint-1.44.0-linux-amd64.tar.gz -o - | \
        tar xzf - -C /usr/local golangci-lint-1.44.0-linux-amd64/golangci-lint && \
    cp /usr/local/golangci-lint-1.44.0-linux-amd64/golangci-lint /usr/local/bin/ && \
    curl -s -L https://github.com/securego/gosec/releases/download/v2.9.6/gosec_2.9.6_linux_amd64.tar.gz -o - | \
        tar xzf - -C /usr/local/bin gosec && \
    mkdir /usr/local/testing/ && \
    rm -rf /var/lib/apt/lists/*

# copy prepare test environment scripts
COPY prepare-test-environment.sh /usr/local/bin/
COPY fixtures.sql /usr/local/testing/

CMD ["echo", "I'm pgcenter-testing 0.0.7"]
