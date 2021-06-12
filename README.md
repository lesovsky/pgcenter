<img width="255" alt="" src="https://github.com/lesovsky/pgcenter/raw/master/doc/images/pgcenter-logo.png" align="right">

[![Web site](https://img.shields.io/badge/pgCenter-org-orange.svg)](https://pgcenter.org)
[![GitHub release](https://img.shields.io/github/release/lesovsky/pgcenter.svg?style=flat)](https://github.com/lesovsky/pgcenter/releases/)
![Github Actions](https://github.com/lesovsky/pgcenter/actions/workflows/default.yml/badge.svg)
[![Go Report Card](https://goreportcard.com/badge/lesovsky/pgcenter)](https://goreportcard.com/report/lesovsky/pgcenter)

---
pgCenter is a command-line admin tool for observing and troubleshooting Postgres.

- [Main goal](#main-goal)
- [Key features](#key-features)
- [Quick start](#quick-start)
- [Supported statistics](#supported-statistics)
  - [PostgreSQL statistics](#postgresql-statistics)
  - [System statistics](#system-statistics)
- [Install notes](#install-notes)
- [Usage notes](#usage-notes)
- [Development, testing and contribution](#development-testing-and-contribution)
- [Thanks](#thanks)

---
#### Main goal
Postgres provides various activity statistics about its runtime, such as connections, statements, database operations, replication, resources usage and more. The general purpose of the statistics is to help DBAs to monitor and troubleshoot Postgres. However, these statistics provided in textual form retrieved from SQL functions and views, and Postgres doesn't provide native tools for working with statistics views.

pgCenter's main goal is to help Postgres DBA working with statistics and provide a convenient way to observe Postgres in runtime.  

![](doc/images/pgcenter-demo.gif)

#### Key features
- Top-like interface that allows you to monitor stats changes as you go. See details [here](doc/pgcenter-top-readme.md).
- Configuration management function  allows viewing and editing of current configuration files and reloading the service, if needed.
- Logfiles functions allow you to quickly check Postgres logs without stopping statistics monitoring.
- "Poor man’s monitoring" allows you to collect Postgres statistics into files and build reports later on. See details [here](doc/pgcenter-record-readme.md).
- Wait events profiler allows seeing what wait events occur during queries execution. See details [here](doc/pgcenter-profile-readme.md).

#### Quick start
Pull Docker image from [DockerHub](https://hub.docker.com/r/lesovsky/pgcenter); run pgcenter and connect to the database.
```
docker pull lesovsky/pgcenter:latest
docker run -it --rm lesovsky/pgcenter:latest pgcenter top -h 1.2.3.4 -U user -d dbname
```

#### Supported statistics

##### PostgreSQL statistics
- summary activity - a compilation/selection  of metrics from different sources - postgres uptime, version, recovery status, number of clients grouped by their states, number of (auto)vacuums, statements per second, age of the longest transaction and the longest vacuum;
- [pg_stat_activity](https://www.postgresql.org/docs/current/static/monitoring-stats.html#PG-STAT-ACTIVITY-VIEW) - activity of connected clients and background processes.
- [pg_stat_database](https://www.postgresql.org/docs/current/static/monitoring-stats.html#PG-STAT-DATABASE-VIEW) - database-wide and sessions statistics, such as number of commits/rollbacks, processed tuples, deadlocks, temporary files, etc.
- [pg_stat_replication](https://www.postgresql.org/docs/current/static/monitoring-stats.html#PG-STAT-REPLICATION-VIEW) - replication statistics, like connected standbys, their activity and replication lag.
- [pg_stat_user_tables](https://www.postgresql.org/docs/current/static/monitoring-stats.html#PG-STAT-ALL-TABLES-VIEW), [pg_statio_user_tables](https://www.postgresql.org/docs/current/static/monitoring-stats.html#PG-STATIO-ALL-TABLES-VIEW) - statistics on accesses (including IO) to tables.
- [pg_stat_user_indexes](https://www.postgresql.org/docs/current/static/monitoring-stats.html#PG-STAT-ALL-INDEXES-VIEW), [pg_statio_user_indexes](https://www.postgresql.org/docs/current/static/monitoring-stats.html#PG-STATIO-ALL-INDEXES-VIEW) - statistics on accesses (including IO) to indexes.
- [pg_stat_user_functions](https://www.postgresql.org/docs/current/static/monitoring-stats.html#PG-STAT-USER-FUNCTIONS-VIEW) - statistics on execution of functions.
- [pg_stat_wal](https://www.postgresql.org/docs/current/static/monitoring-stats.html#PG-STAT-WAL-VIEW) - WAL usage statistics.
- [pg_stat_statements](https://www.postgresql.org/docs/current/static/pgstatstatements.html) - statistics on SQL statements executed including time and resources usage.
- statistics on tables sizes based on `pg_relation_size()` and `pg_total_relation_size()` functions;
- [pg_stat_progress_vacuum](https://www.postgresql.org/docs/current/progress-reporting.html#VACUUM-PROGRESS-REPORTING) - progress of (auto)vacuums operations.
- [pg_stat_progress_cluster](https://www.postgresql.org/docs/current/progress-reporting.html#CLUSTER-PROGRESS-REPORTING) - progress of CLUSTER and VACUUM FULL operations.
- [pg_stat_progress_create_index](https://www.postgresql.org/docs/current/progress-reporting.html#CREATE-INDEX-PROGRESS-REPORTING) - progress of CREATE INDEX and REINDEX operations.
- [pg_stat_progress_analyze](https://www.postgresql.org/docs/current/progress-reporting.html#ANALYZE-PROGRESS-REPORTING) - progress of ANALYZE operations.
- [pg_stat_progress_basebackup](https://www.postgresql.org/docs/current/progress-reporting.html#BASEBACKUP-PROGRESS-REPORTING) - progress of basebackup operations.
- [pg_stat_progress_copy](https://www.postgresql.org/docs/current/progress-reporting.html#COPY-PROGRESS-REPORTING) - progress of COPY operations.

##### System statistics
`pgcenter top` also provides system usage information based on statistics from `procfs` filesystem:

- load average and CPU usage time (user, system, nice, idle, iowait, software, and hardware interrupts, steal);
- memory and swap usage, amount of cached and dirty memory, writeback activity;
- storage devices statistics: IOPS, throughput, latencies, average queue and requests size, devices utilization;
- network interfaces statistics: throughput in bytes and packets, different kind of errors, saturation and utilization.
- mounted filesystems' usage statistics: total size, amount of free/used/reserved space and inodes. 

In the case of connecting to remote Postgres, there is possibility to use additional SQL functions used for retrieving `/proc` statistics from a remote host. For more information, see details [here](doc/pgcenter-config-readme.md).

#### Install notes
Packages for DEB, RPM, APK are available on [releases](https://github.com/lesovsky/pgcenter/releases) page.

#### Usage notes
pgCenter has been developed to work on Linux and hasn't been tested on other OS (operating systems); therefore, it is not recommended using it on alternative systems because it will not operate properly.

pgCenter supports a wide range of PostgreSQL versions, despite the difference in statistics between each version. If pgCenter is unable to read a particular stat, it will show a descriptive error message.

Ideally, pgCenter requires `SUPERUSER` database privileges, or at least privileges to view statistics, read settings, logfiles and send signals to other backends. Roles with such privileges (except reading logs) have been introduced in Postgres 10; see details [here](https://www.postgresql.org/docs/current/static/default-roles.html).

It is recommended to run pgCenter on the same host where Postgres is running. This is because for Postgres, pgCenter is just a simple client application, and it may have the same problems as other applications that work with Postgres, such as network-related problems, slow responses, etc.

It is possible to run pgCenter on one host and connect to Postgres, which runs on another host, but some functions may not work - this fully applies to `pgcenter top` command.

pgCenter also supports Amazon RDS for PostgreSQL, but as mentioned above, some functions will not work, and also system stats will not be available, because of PostgreSQL RDS instances don't support untrusted procedural languages due to security reasons.

#### Development, testing and contribution
To help development you are encouraged to:
- provide [suggestion/feedback](https://github.com/lesovsky/pgcenter/discussions) or [issue](https://github.com/lesovsky/pgcenter/issues) (follow the provided issue template carefully).
- pull requests for bug fixes of improvements; this [docs](./doc/development.md) might be helpful.
- star the project

#### Thanks
- Thank you for using pgCenter!
- Sebastien Godard for [sysstat](https://github.com/sysstat/sysstat).
- Brendan Gregg and Tim Cook for [nicstat](http://sourceforge.net/projects/nicstat/).
- Pavel Stěhule for his [articles](http://postgres.cz/wiki/PostgreSQL).
- Pavel Alexeev, package maintainer on EPEL testing repo (Fedora/Centos).
- Manuel Rüger, ebuild maintainer on [mrueg overlay](https://gpo.zugaina.org/dev-db/pgcenter) (Gentoo Linux).
- Anton Novojilov, package maintainer on RHEL/CentOS Linux (Essential Kaos repo).
- Nikolay A. Fetisov, package maintainer at [Sisyphus](http://www.sisyphus.ru/ru/srpm/pgcenter) ALT Linux.
- Devrim Gündüz, package maintainer on official [PostgreSQL yum repo](https://yum.postgresql.org/).
