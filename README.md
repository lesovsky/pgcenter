# pgCenter 	[![Build Status](https://travis-ci.org/lesovsky/pgcenter.svg)](https://travis-ci.org/lesovsky/pgcenter)

pgCenter is a command-line admin tool for observing and troubleshooting Postgres.

- [Main goal](#main-goal)
- [Key features](#key-features)
- [Supported statistics](#supported-statistics)
  - [PostgreSQL statistics](#postgresql-statistics)
  - [System statistics](#system-statistics)
- [Install notes](#install-notes)
- [Known issues](#known-issues)
- [Thanks](#thanks)
---

#### Main goal
Postgres provides various activity statistics that include detailed information about its behaviour: connections, statements, database operations, replication, resources usage and more. General purpose of the statistics is to help DBAs to monitor and troubleshoot Postgres. However, these statistics provided in textual form retrieved from SQL functions and views, and Postgres doesn't provide any tools for working with them.
pgCenter's main goal is to help Postgres DBA manage statistics that theу have in their databases and see all the necessary data in convenient format based on builtin stats views and functions.

#### Key features
- Top-like interface that allows you to monitor stats changes as you go. See details [here](doc/pgcenter-top-readme.md).
- Configuration management function  allows viewing and editing of current configuration files and reloading the service, if needed.
- Logfiles functions allow you to quickly check Postgres logs without stopping statistics monitoring.
- "Poor man’s monitoring" allows you to collect Postgres statistics into files and build reports later on. See details [here](doc/pgcenter-record-readme.md).

#### Supported statistics
When troubleshooting Postgres it's always important to keep an eye not only on Postgres metrics, but also system metrics, since Postgres utilizes system resources, such as cpu, memory, storage and network when working. pgCenter allows you to see both kinds of statistics related to Postgres and your system.

##### PostgreSQL statistics
pgCenter supports majority of statistics views available in Postgres, and at the same time, uses additional SQL functions applied to statistics to show these in a more convenient way. The following stats are available:

- current summary activity - a compilation/selection  of metrics from different sources - postgres uptime, version, recovery status, number of clients grouped by their states, number of (auto)vacuums, statements per second, age of the longest transaction and the longest vacuum;
- [pg_stat_activity](https://www.postgresql.org/docs/current/static/monitoring-stats.html#PG-STAT-ACTIVITY-VIEW) - information related to the current activity of connected clients and Postgres background processes.
- [pg_stat_database](https://www.postgresql.org/docs/current/static/monitoring-stats.html#PG-STAT-DATABASE-VIEW) - database-wide statistics, such as number of commits/rollbacks, handled tuples, deadlocks, temporary files, etc.
- [pg_stat_replication](https://www.postgresql.org/docs/current/static/monitoring-stats.html#PG-STAT-REPLICATION-VIEW) - statistics on replication, connected standby hosts and their activity.
- [pg_stat_user_tables](https://www.postgresql.org/docs/current/static/monitoring-stats.html#PG-STAT-ALL-TABLES-VIEW), [pg_statio_user_tables](https://www.postgresql.org/docs/current/static/monitoring-stats.html#PG-STATIO-ALL-TABLES-VIEW) - statistics on accesses (including IO) to tables.
- [pg_stat_user_indexes](https://www.postgresql.org/docs/current/static/monitoring-stats.html#PG-STAT-ALL-INDEXES-VIEW), [pg_statio_user_indexes](https://www.postgresql.org/docs/current/static/monitoring-stats.html#PG-STATIO-ALL-INDEXES-VIEW) - statistics on accesses (including IO) to indexes.
- [pg_stat_user_functions](https://www.postgresql.org/docs/current/static/monitoring-stats.html#PG-STAT-USER-FUNCTIONS-VIEW) - statistics on execution of functions.
- [pg_stat_statements](https://www.postgresql.org/docs/current/static/pgstatstatements.html) - statistics on SQL statements executed including time and resources usage.
- statistics on tables sizes based on `pg_relation_size()` and `pg_total_relation_size()` functions;
- [pg_stat_progress_vacuum](https://www.postgresql.org/docs/current/static/progress-reporting.html#VACUUM-PROGRESS-REPORTING) - information about (auto)vacuums status.

##### System statistics
`pgcenter top` also provides system usage information based on statistics from `procfs` filesystem:

- load average and CPU usage time (user, system, nice, idle, iowait, software and hardware interrupts, steal);
- memory and swap usage, amount of cached and dirty memory, writeback activity;
- storage devices statistics: iops, throughput, latencies, average queue and requests size, devices utilization;
- network interfaces statistics: throughput in bytes and packets, different kind of errors, saturation and utilization.

In case of connecting to remote Postgres there is possibility to use additional SQL functions used for retrieving `/proc` statistics from remote host. For more information see details here.

#### Install notes
pgCenter is written on Go language and distributed as a single precompiled binary file. Download it from [releases](https://github.com/lesovsky/pgcenter/releases), unpack and it's ready to use.

Additional information and usage examples available [here](https://github.com/lesovsky/pgcenter/blob/master/doc/examples.md).

#### Known issues
pgCenter is beta software, thus in some circumstances, segfaults and panics may occur. When panics occur please do let me know - this helps me in making necessary changes and improve this software. To make sure that I can reproduce an issue you’ve been having and can address it accordingly please follow these steps:

- build pgCenter from the master branch and try to reproduce the bug/crash. 
- create an [issue](https://github.com/lesovsky/pgcenter/issues) and include clear instructions on how the bug could be reproduced.
- also, please list the information about your operating system, its release version and version of Postgres.

#### Thanks
- Sebastien Godard for [sysstat](https://github.com/sysstat/sysstat).
- Brendan Gregg and Tim Cook for [nicstat](http://sourceforge.net/projects/nicstat/).
- Pavel Stěhule for his [articles](http://postgres.cz/wiki/PostgreSQL).
- Pavel Alexeev, package maintainer on EPEL testing repo (Fedora/Centos).
- Manuel Rüger, ebuild maintainer on [mrueg overlay](https://gpo.zugaina.org/dev-db/pgcenter) (Gentoo Linux).
- Anton Novojilov, package maintainer on RHEL/CentOS Linux (Essential Kaos repo).
- Nikolay A. Fetisov, package maintainer at [Sisyphus](http://www.sisyphus.ru/ru/srpm/pgcenter) ALT Linux.
- Devrim Gündüz, package maintainer on official [PostgreSQL yum repo](https://yum.postgresql.org/).
- Thank you for using pgCenter!
