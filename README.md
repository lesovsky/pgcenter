#### README: pgCenter [![Build Status](https://travis-ci.org/lesovsky/pgcenter.svg)](https://travis-ci.org/lesovsky/pgcenter)

PostgreSQL provides various statistics which include information about tables, indexes, functions and other database objects and their usage. Moreover, statistics has detailed information about connections, current queries and database operations (INSERT/DELETE/UPDATE). But most of these statistics are provided as permanently incremented counters. The pgCenter provides convenient interface to postgres statistics and allow viewing statistics changes in time interval, eg. per second. Also pgCenter provides fast access for database management task, such as editing configuration files, reloading postgres service, viewing log files and canceling or terminating database backends (by pid or using state mask). However, if need to execute some specific operations, pgCenter able to start psql session for this purposes.

#### Features:
- top-like interface with sorting and filtration functions;
- use same connection options as with psql;
- tabs support, possible to work concurrently with many postgres services (limited by 8).
- show current system load and cpu/memory/swap usage;
- show input/output statistics for devices and partitions like iostat;
- show network traffic statistics for network interfaces like nicstat;
- show current postgres state (connections, longest transaction, autovacuum)
- show statistics about tables, indexes, functions, current activity, replication;
- show pg_stat_statements statistics: calls, rows;
- show pg_stat_statements statistics: cpu timings, IO timings;
- show pg_stat_statements statistics: block IO (hits, reads, dirtied, written, temp);
- show pg_stat_statements statistics: local (temp tables) IO, temp (temp files) IO;
- show relations sizes info;
- show vacuum progress (since 9.6);
- configuration files editor and postgres reload;
- log files viewing (view entire log or tail last lines);
- cancel queries or terminate processes by their pid or handle whole group using state mask;
- query reporting;

#### PostgreSQL statistics:
- current postgres activity - postgres uptime and version, number of clients and their states, number of (auto)vacuums tasks, statements per second, age of a longest transaction;
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

#### System activity statistics
- load average and cpu usage (user, system, nice, idle, iowait, software and hardware interrupts, steal);
- memory and swap usage, cached, dirty memory and writeback activity;
- storage devices stats: iops, throughput, latencies, average queue and requests size, device utilization;
- network interfaces stats: throughput in bytes and packets, different kind of errors, saturation and utilization.

pgCenter is able to show current system activity on the host where postgres runs. Particularly, it's a load average, cpu utilization, memory and swap usage, iostat and nicstat - all these information collected from /proc filesystem from database's host. How it works? In short, pgCenter gets system stats through postgres connection using sql functions. At startup, when pgCenter connects to postgres service, it determines is it a remote or local service. In case of remote, pgCenter installs its functions written on procedural language and views based on these functions. Next, pgCenter using these functions and views collects stats from procfs and shows to user.
Installing and removing the functions are available through pgCenter startup parameters (see program built-in help).
Currently, only plperlu is supported and it has several limitations:
- plperlu procedural language must be installed in the database you want to connect.
- perl module Linux::Ethtool::Settings shoul be installed in the system, it used to get speed and duplex of network interfaces and properly calculate some metrics.

Anyway, pgCenter can runs without these stats functions, in this case, zeroes will be shown in the system stats interface (la,cpu,mem,swap,io,network) and a lot of errors will be appear in postgres log.
Stats functions and views bodies aren't hard-coded, stored in the source code tree and available for free use. See share/ directory.

#### Actions:
- Show current configuration, edit configuration files and reloading PostgreSQL service;
- Tail and viewing log files;
- Cancel queries or terminate backends using backend pid;
- Cancel queries or terminate group of backends using state of backends;
- Toggle displaying system tables and indexes for tables and indexes statistics;
- Reset PostgreSQL statistics counters;
- Show detailed report about query (based on pg_stat_statements);
- Start psql session.

#### Recommendations:
- run pgCenter on the same host with PostgreSQL. When pgCenter works with remote PostgreSQL, some features will not work, eg. config editing, logfile viewing.
- run pgCenter under database SUPERUSER account, eg. postgres. Some internal PostgreSQL information, system functions or data from views are available only for privileged accounts.

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

##### Install from RHEL/CentOS/Fedora
Fortunately, pgCenter in the RHEL-family is available from several sources:
- official [PostgreSQL YUM repository](https://yum.postgresql.org/).
- Extra Packages for Enterprise Linux (EPEL)
```
$ sudo yum install epel-release
$ sudo yum --enablerepo=epel-testing install pgcenter
```
- Essential Kaos testing repo.
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

##### Run in Docker (works on Mac OS X)

```
$ docker build -t pgcenter .
Sending build context to Docker daemon 968.2 kB
[...]
$ docker run -it pgcenter -h mydbhost.com -U user -d dbname
mydbhost.com: user@dbname require password: ******
```

##### Connect to PostgreSQL server #####
pgCenter able to connect to Postgres with different ways:
```
$ pgcenter -h <host> -p <port> -U <username> -d <dbname>
$ pgcenter -U <username> -d <dbname>
$ pgcenter -U <username>
$ pgcenter <dbname> <username>
$ pgcenter <dbname>
$ pgcenter
```
- Allowed to use libpq environment variables, such as PGHOST, PGPORT, PGUSER, PGDATABASE, PGPASSWORD. These settings have the lowest priority and used in case when no connections settings specified via input arguments and connection file (.pgcenterrc) not found or not specified.
- Connection file stores connection settings and their number is limited by eight connections (max number of tabs). This file is used when input arguments are not specified. If connection options are specified at startup, connection starts in the first tab, other connections from the file start in the next tabs.
- Connection settings specified with input arguments have top priority, connection with these settings will opens in the first tab.

#### Known issues
- mainly developed and tested under PostgreSQL 9.5/9.6 (but tested with others 9.x releases).
- this is a beta software, in some circumstances segfaults may occur. When segfaults occur, you may help me to make this software better:
  - Build pgcenter from the latest sources (see instructions above).
  - Enable coredumps with ```ulimit -c unlimited```
  - Reproduce the segafult (after pgcenter crash, a coredump file will be created in the current directory).
  - Run pgcenter with gdb ```gdb ./pgcenter <coredump>```
  - In gdb console, run ```where``` command, get the latest 15 lines and create an [issue](https://github.com/lesovsky/pgcenter/issues).

#### Thanks
- Sebastien Godard for [sysstat](https://github.com/sysstat/sysstat).
- Brendan Gregg and Tim Cook for [nicstat](http://sourceforge.net/projects/nicstat/).
- Pavel Stěhule for his [articles](http://postgres.cz/wiki/PostgreSQL)
