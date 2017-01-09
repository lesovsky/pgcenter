#### README: pgCenter [![Build Status](https://travis-ci.org/lesovsky/pgcenter.svg)](https://travis-ci.org/lesovsky/pgcenter)

PostgreSQL provides various statistics which include information about tables, indexes, functions and other database objects and their usage. These statistics hold detailed information about a lot of things: connections, current queries, database operations (INSERT/DELETE/UPDATE), etc. However, most of these provided as permanently incremented counters. pgCenter provides convenient interface that allows viewing PostgreSQL statistics' updates in a per-second intervals. pgCenter also provides fast access for database management tasks, such as editing configuration files, reloading postgres service, viewing log files and canceling or terminating database backends (by pid or using the state mask). In addition, if you need to execute specific operations, pgCenter allows you to start psql session.

#### Key features:
- top-like interface with sort and filtration functions;
- use same connection options as psql;
- tabs support, allow concurrent workflow with multiple postgres services (limited by 8).
- shows current system load and cpu/memory/swap usage;
- shows input/output statistics for devices and partitions like iostat;
- shows network traffic statistics for network interfaces like nicstat;
- shows current postgres status (connections, longest transaction, autovacuum);
- provides statistics on tables, indexes, functions, current query activity, replication;
- shows pg_stat_statements statistics: calls, rows;
- shows pg_stat_statements statistics: cpu timings, IO timings;
- shows pg_stat_statements statistics: block IO (hits, reads, dirtied, written, temp);
- shows pg_stat_statements statistics: local (temp tables) IO, temp (temp files) IO;
- provides relations sizes info;
- shows vacuum progress (since 9.6);
- includes configuration files editor and postgres service reload;
- allows viewing log files (view entire log or tail last lines of the log);
- allows to cancel queries or terminate processes by their pid or handling entire group using a state mask;
- provides query reporting;

#### PostgreSQL statistics:
- current postgres activity - postgres uptime and version, number of clients and their states, number of (auto)vacuums tasks, statements per second, age of a longest transaction;
- pg_stat_activity - long running queries and abnormal activity, i.e. idle in transaction, aborted or waiting processes;
- pg_stat_database - database-wide statistics;
- pg_stat_replication - WAL sender process statistics about replication to a particular sender connected to a standby server;
- pg_stat_user_tables - statistics per table within specific database, shows information about access to that particular table;
- pg_statio_user_tables - statistics per table within specific database, shows information about I/O on that particular table;
- pg_stat_user_indexes, pg_statio_user_indexes - per index statistics within the specific database, shows information about access and I/O to these specific indexes;
- pg_stat_user_functions -  statistics for tracked functions, shows information about executions of these functions;
- pg_stat_statements - query executions and resource usage statistics per distinct database ID, user ID and query ID;
- statistics about tables sizes based on pg_relation_size() and pg_total_relation_size() functions;
- pg_stat_progress_vacuum - contains one row for each backend (including autovacuum worker processes) that is currently vacuuming.

#### System activity statistics
- load average and cpu usage (user, system, nice, idle, iowait, software and hardware interrupts, steal);
- memory and swap usage, cached, dirty memory and writeback activity;
- storage devices stats: iops, throughput, latencies, average queue and requests size, device utilization;
- network interfaces stats: throughput in bytes and packets, different kind of errors, saturation and utilization.

pgCenter is able to show current system activity on the host where postgres runs. Particularly, it's a load average, cpu utilization, memory and swap usage, iostat and nicstat - all these information collected from /proc filesystem from database's host. How it works? In short, pgCenter gets system stats through postgres connection using sql functions. At startup, when pgCenter connects to postgres service, it determines is it a remote or local service. In case of remote, pgCenter installs its functions written on procedural language and views based on these functions. Next, pgCenter using these functions and views collects stats from procfs and shows these to user.
Installing and removing the functions are available through pgCenter startup parameters (see program built-in help).
Currently, only *plperlu* is supported and it has several limitations:
- plperlu procedural language must be installed in the database you want to connect.
- perl module *Linux::Ethtool::Settings* shoul be installed in the system, it used to get speed and duplex of network interfaces and properly calculate some metrics.

Anyway, pgCenter can runs without these stats functions, in this case, zeroes will be shown in the system stats interface (la,cpu,mem,swap,io,network) and a lot of errors will be appear in postgres log.
Stats functions and views bodies aren't hard-coded, stored in the source code tree and available for free use. See share/ directory.

#### Actions:
- Show current configuration, edit configuration files and reloading PostgreSQL service;
- Tail and view log files;
- Cancel queries or terminate backends using backend pid;
- Cancel queries or terminate group of backends using state of backends;
- Toggle displaying system tables and indexes for tables and indexes statistics;
- Reset of PostgreSQL statistics counters;
- Show detailed report about query (based on pg_stat_statements);
- Start psql session.

#### Recommendations:
- run pgCenter on the same host with PostgreSQL. When pgCenter works with remote PostgreSQL, some features will not function, eg. config editing, logfile view.
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
**Please note!**: *Launchpad uses it's own buildfarm for creating packages and pgCenter installed from Launchpad may crashe in some circumstances.*

##### Install from RHEL/CentOS/Fedora
pgCenter in the RHEL-family is available from several sources:
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
- It is also possible to use libpq environment variables, such as PGHOST, PGPORT, PGUSER, PGDATABASE, PGPASSWORD. These settings have lowest priority and used in case no connections settings were specified via input arguments and connection file (.pgcenterrc) was not found or not specified.
- Connection file stores connection settings and number of connections is limited to eight (maximum number of tabs). This file is used when input arguments are not specified. If connection options are specified during startup, first connection starts in the first tab, while other connections would start in the following tabs.
- Connection settings specified with input arguments would have top priority and connection with such settings will opens in the first tab.

--
Known issues:
developed and tested under PostgreSQL 9.5/9.6 (also tested with other 9.x releases).
this is a beta software, in some circumstances segfaults may occur. When segfaults occur please let me know - this would help me making necessary improvements in this software:
build pgcenter from the latest sources (see instructions above);
enable coredumps with ulimit -c unlimited;
reproduce the segafult (after pgcenter crash, a coredump file will be created in the current directory);
run pgcenter with gdb gdb ./pgcenter <coredump>;
in gdb console, run where command, get the latest 15 lines and create an issue.



#### Known issues
- developed and tested under PostgreSQL 9.5/9.6 (also tested with others 9.x releases).
- this is a beta software, in some circumstances segfaults may occur. When segfaults occur please let me know - this would help me making necessary improvements in this software:
  - Build pgcenter from the latest sources (see instructions above).
  - Enable coredumps with ```ulimit -c unlimited```
  - Check coredumps store location with ```sysctl kernel.core_pattern```
  - Optionally you can reconfigure store location to current directory with ```sysctl -w kernel.core_pattern=core```
  - Reproduce the segfault (after pgCenter crash, a coredump file will be created regarding sysctl core_pattern parameter).
  - Run pgCenter with gdb ```gdb ./pgcenter <coredump_filename_here>```
  - In gdb console, run ```where``` command, get the output and create an [issue](https://github.com/lesovsky/pgcenter/issues). 
  - Also, leave information about operating system its release version and version of PostgreSQL.

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
