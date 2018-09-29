### README: pgcenter top

`pgcenter top` provides top-like interface for Postgres statistics with extended set of functions that make online monitoring and troubleshooting Postgres much easier.

- [General information](#general-information)
- [Main functions](#main-functions)
- [Admin functions](#admin-functions)
- [System statistics notes](#system-statistics-notes)
- [Usage](#usage)
---

#### General information
`pgcenter top` relies on two types of statistics: 

1. statistics about system resources usage from `procfs` filesystem; 
2. activity statistics from Postgres. 

At launch, pgCenter connects to Postgres and starts continuously reading statistics views. Comparing stats snapshots pgCenter calculates differences and shows it to user. Same goes for system stats, pgCenter reads stats files from `/proc` filesystem, calculates differences and shows results to user.  

#### Main functions
It may be surprising, but Postgres can provide hundreds and thousands of stats metrics distributed across several functions and views. `pgcenter top` helps not to drown in statistics with:
- console-based top-like interface;
- keyboard shortcuts to switch between different kind of stats;
- ascending and descending sort order based on values from particular columns;
- ability to filter unnecessary statistics and only focus on relevant data.

#### Admin functions:
`pgcenter top` also provides admin functions that assist in Postgres administration and troubleshooting. It allows user to:
- view current configuration, edit configuration files and reload Postgres service;
- view log files in pager or view log's tail on the fly;
- cancel queries or terminate backends using backend's pid;
- cancel group of queries or terminate group of backends based on their states;
- toggle displaying system tables and indexes for tables and indexes statistics;
- reset Postgres statistics counters;
- view detailed reports about statements (based on `pg_stat_statements`);
- start `psql` session (if you prefer a hands-on approach).

Note, though admin functions allows managing Postgres configuration, pgCenter is not a comprehensive tool for Postgres configurations and services management.

#### System statistics notes
- system statistics are available through `procfs` filesystem which is available on Linux operating system. It is not available on other operating systems, e.g. Windows. 

  Though, `procfs` is  available in other UNIX systems, its variants may differ from Linux `procfs` so may not be supported by pgCenter.

- `pgcenter top` can connect to remote Postgres services and retrieve system statistics through additional SQL functions that are shipped with pgCenter. See details [here]().

#### Usage
Run `top` command to connect to Postgres and watching statistics:
```
pgcenter top -h 1.2.3.4 -U postgres production_db
```

See other usage examples [here](examples.md).