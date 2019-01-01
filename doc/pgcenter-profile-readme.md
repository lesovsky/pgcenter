### README: pgcenter profile

`pgcenter profile` is the tool for profiling [wait events](https://www.postgresql.org/docs/current/monitoring-stats.html#WAIT-EVENT-TABLE) occured during queries execution. 

- [General information](#general-information)
- [Main functions](#main-functions)
- [Limitations](#limitations)
- [Usage](#usage)
---

#### General information
In cases of long query, you might be interested what this query does. Using `EXPLAIN` utility you can observe detailed query execution plan. But if query spends time in waitings `EXPLAIN` will not show that. Using `pgcenter profile` you can see what wait events occur during query execution. Below an example of wait events occured in heavy `UPDATE` query on system with poor IO performance:
```
------ ------------ -----------------------------
% time      seconds wait_event                     query: update pgbench_accounts set abalance = abalance + 100;
------ ------------ -----------------------------
72.15     30.205671 IO.DataFileRead
20.10      8.415921 Running
5.50       2.303926 LWLock.WALWriteLock
1.28       0.535915 IO.DataFileWrite
0.54       0.225117 IO.WALWrite
0.36       0.152407 IO.WALInitSync
0.03       0.011429 IO.WALInitWrite
0.03       0.011355 LWLock.WALBufMappingLock
------ ------------ -----------------------------
99.99     41.861741
```
In the example, you can see that a most of time is spent on awaiting IO when reading data files.

Exploring your queries with `pgcenter profiler` you can see many other interesting things. 

#### Main functions
- using `pid`, `wait_event_type`, `wait_event` from `pg_stat_activity` statistics for profiling;
- specify the PID for profiling a specific Postgres backend;
- change the frequency of profiling interval; default is 100, means to profile with 10ms interval.

#### Limitations
- [Wait events](https://www.postgresql.org/docs/current/monitoring-stats.html#WAIT-EVENT-TABLE) has been introduced in Postgres 9.6, hence the profiling is possible for 9.6 and newer versions of Postgres.
- Profiling is not accounting wait events for parallel workers, because there is no guaranteed way to associate master process with its workers. 

#### Usage
Run `profile` and specify backend PID which want to profile to:
```
pgcenter profile -U postgres -P 12345 
```

See other usage examples [here](examples.md).