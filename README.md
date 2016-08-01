#### README: pgCenter [![Build Status](https://travis-ci.org/lesovsky/pgcenter.svg)](https://travis-ci.org/lesovsky/pgcenter)

PostgreSQL provides various statistics which includes information about tables, indexes, functions and other database objects and their usage. Moreover, statistics has detailed information about connections, current queries and database operations (INSERT/DELETE/UPDATE). But most of this statistics are  provided as permanently incremented counters. The pgcenter provides convenient interface to this statistics and allow viewing statistics changes in time interval, eg. per second. The pgcenter provides fast access for database management task, such as editing configuration files, reloading services, viewing log files and canceling or terminating database backends (by pid or using state mask). However if need execute some specific operations, pgcenter can start psql session for this purposes.

#### Features:
- top-like interface;
- use same connection options as with psql;
- show current system load and cpu/memory/swap usage;
- show input/output statistics for devices and partitions like iostat;
- show network traffic statistics for network interfaces like nicstat;
- show current postgres state (connections state, longest transaction, autovacuum)
- show statistics about tables, indexes, functions, activity, replication;
- show pg_stat_statements statistics: calls, rows;
- show pg_stat_statements statistics: cpu timings, io timings;
- show pg_stat_statements statistics: blks i/o (hits, reads, dirtied, written, temp);
- show relations sizes info;
- show vacuum progress (since 9.6);
- configuration files editor and postgres reload;
- log files viewing (full log or tail);
- cancel/terminate queries or processes by pid or whole group;
- query reporting;

#### Statistics:
- pg_stat_activity - long running queries and abnormal activity, i.e. idle in transaction, aborted or waiting processes;
- pg_stat_database - database-wide statistics;
- pg_stat_replication - WAL sender process statistics about replication to that sender's connected standby server;
- pg_stat_user_tables - statistics for each table in the current database, showing info about accesses to that specific table;
- pg_statio_user_tables - statistics for each table in the current database, showing info about I/O on that specific table;
- pg_stat_user_indexes, pg_statio_user_indexes - statistics for each index in the current database, showing info about accesses and I/O to that specific index;
- pg_stat_user_functions -  statistics for each tracked function, showing info about executions of that function;
- pg_stat_statements - query executions and resource usage statistics for each distinct database ID, user ID and query ID;
- statistics about tables sizes based on pg_relation_size() and pg_total_relation_size();
- pg_stat_progress_vacuum - contains one row for each backend (including autovacuum worker processes) that is currently vacuuming.

#### Actions:
- Show current configuration, edit configuration files and reloading PostgreSQL service;
- Tail and viewing log files;
- Show iostat/nicstat statistics;
- Cancel queries or terminate backends using backend pid;
- Cancel queries or terminate group of backends using state of backends;
- Toggle displaying system tables and indexes for tables and indexes statistics;
- Reset PostgreSQL statistics counters;
- Show detailed report about query;
- Start psql session.

#### Recommendations:
- run pgCenter on the same host with PostgreSQL. When pgCenter works with remote PostgreSQL, some features will not work, eg. config editing, logfile viewing, system monitoring functions or iostat/nicstat.
- run pgCenter under database SUPERUSER account, eg. postgres. Some internal PostgreSQL information, system functions or data from views is available only for privileged accounts.

#### Install notes:

##### Install from Ubuntu
- install PPA and update
```
$ sudo add-apt-repository ppa:lesovsky/pgcenter
$ sudo apt-get update
$ sudo apt-get install pgcenter
```
Debian users can create package using this [link](https://wiki.debian.org/CreatePackageFromPPA) or download deb package from [Launchpad page](https://launchpad.net/~lesovsky/+archive/ubuntu/pgcenter/+packages) and install pgCenter with *dpkg -i*.
**Warning**: *Launchpad uses it's own buildfarm for packages creating and pgCenter installed from Launchpad crashes in some circumstances.*

##### Install from RHEL6/CentOS6
- install pgCenter from Essential Kaos testing repo.
```
$ sudo yum install http://release.yum.kaos.io/i386/kaos-repo-6.8-0.el6.noarch.rpm
$ sudo yum --enablerepo=kaos-testing install pgcenter
```

##### Install from sources
- install git, make, gcc, postgresql devel and ncurses devel packages.
- clone sources and build.
```
$ git clone https://github.com/lesovsky/pgcenter
$ cd pgcenter
$ make
$ sudo make install
$ pgcenter
```

#### Known issues
- mainly developed and tested under PostgreSQL 9.5/9.6 (but tested with others 9.x releases).
- this is beta software, in some circumstances may occurs segfaults. When segfaults occur, you may help me to do this software better:
  - Build pgcenter from latest sources (see above instructions).
  - Enable coredump with ```ulimit -c unlimited```
  - Reproduce segafult (after pgcenter crash, coredump file will be created in current directory).
  - Run pgcenter with gdb ```gdb ./pgcenter <coredump>```
  - In gdb console, run ```where``` command, get the latest 15 lines and create [issue](https://github.com/lesovsky/pgcenter/issues).

#### Thanks
- Sebastien Godard for [sysstat](https://github.com/sysstat/sysstat).
- Brendan Gregg and Tim Cook for [nicstat](http://sourceforge.net/projects/nicstat/).
