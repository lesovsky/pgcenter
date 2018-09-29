### README: pgcenter record

`pgcenter record` is the first tool from "Poor man's monitoring" which collect metrics from Postgres to local files.

- [General information](#general-information)
- [Main functions](#main-functions)
- [Usage](#usage)
---

#### General information
`pgcenter record` can be used in cases when no monitoring is available, but there is a need to collect Postgres performance statistics over time. It can also be used as an ad-hoc statistics collecting tool when there is a need in  gathering  statistics over short period of time for purposes of later analysis, for example collecting stats at benchmarking.

`pgcenter record` connects to Postgres, reads stats and writes this information into JSON files into a tar archive. File names contain name of statistics view and timestamp when stats have been recorded. Hence, it's possible to unpack statistics using `tar`. Once unpacked, stats can be used in any way required. 

For reading and building of various different reports there is an alternative tool: `pgcenter report`. See details [here](pgcenter-report-readme.md).

#### Main functions
- continuous recording of statistics into JSON files packed into tar file;
- recording of statistics with specified interval or specified number of times;
- oneshot mode - record single snapshot of statistics and append it into an existing file.

`pgcenter record` doesn't support recording of system statistics, but if you are interested in  such tool, take a look at `sar` utility from `sysstat` package.

#### Usage
Run `record` command to connect to Postgres, poll statistics and continuously save to a local file:
```
pgcenter record -f /tmp/stats.tar -U postgres production_db
```

See other usage examples [here](examples.md).