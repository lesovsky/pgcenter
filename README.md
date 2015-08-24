#### README: pgCenter

PostgreSQL provides various statistics which includes information about tables, indexes, functions and other database objects and their usage. Moreover, statistics has detailed information about connections, current queries and database operations (INSERT/DELETE/UPDATE). But most of this statistics are  provided as permanently incremented counters. The pgcenter provides convenient interface to this statistics and allow viewing statistics changes in time interval, eg. per second. The pgcenter provides fast access for database management task, such as editing configuration files, reloading services, viewing log files and canceling or terminating database backends (by pid or using state mask). However if need execute some specific operations, pgcenter can start psql session for this purposes.

#### Recomendations:
- run pgCenter on the same host with PostgreSQL. When pgCenter works with remote PostgreSQL, some features will not work, eg. config editing, logfile viewing, system monitoring functions.
- run pgCenter under database SUPERUSER account, eg. postgres. Some internal PostgreSQL information, system functions or data from views available only for privileged accounts.

#### Features:
- top-like interface;
- use same connection options as with psql;
- show current system load and cpu usage on localhost;
- show current postgres state (connections state, longest transaction, autovacuum)
- show statistics about tables, indexes, functions, activity, replication;
- show pg_stat_statements statistics (calls, rows, cpu usage, io usage);
- show relations sizes info;
- configuration files editor and postgres reload;
- log files viewing (full log or tail);
- cancel/terminate queries or processes by pid or whole group.

#### Statistics:
- pg_stat_activity - long running queries and abnormal activity, i.e. idle in transaction, aborted or waiting processes;
- pg_stat_database - database-wide statistics;
- pg_stat_replication - WAL sender process statistics about replication to that sender's connected standby server;
- pg_stat_user_tables - statistics for each table in the current database, showing info about accesses to that specific table;
- pg_statio_user_tables - statistics for each table in the current database, showing info about I/O on that specific table;
- pg_stat_user_indexes, pg_statio_user_indexes - statistics for each index in the current database, showing info about accesses and I/O to that specific index;
- pg_stat_user_functions -  statistics for each tracked function, showing info about executions of that function;
- pg_stat_statements - query executions statistics for each distinct database ID, user ID and query ID;
- statistics about tables sizes based on pg_relation_size() and pg_total_relation_size().

#### Actions:
- Show current configuration, edit configuration files and reloading PostgreSQL service;
- Tail and viewing log files;
- Cancel queries or terminate backends using backend pid;
- Cancel queries or terminate group of backends using state of backends;
- Toggle displaying system tables and indexes for tables and indexes statistics;
- Reset PostgreSQL statistics counters;
- Start psql session.

#### Install notes:

##### Install from Ubuntu
- install PPA and update
```
$ sudo add-apt-repository ppa:lesovsky/pgcenter
$ sudo apt-get update
$ sudo apt-get install pgcenter
```
Debian users can create package using this [link](https://wiki.debian.org/CreatePackageFromPPA).

##### Install from RHEL/CentOS
- not available yet.

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
- developed and tested under PostgreSQL 9.4 (minimal testing with 9.3).
- PostgreSQL 9.1 and older not supported (different column names in stat views).
- this is beta software, in some circumstances segfaults occurs.
