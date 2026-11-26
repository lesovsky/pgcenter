### Development

Download the pgcenter-testing Docker image, run a container, publish the PostgreSQL 14–19 fixture ports, and start the environment preparation script.
```shell
$ docker pull lesovsky/pgcenter-testing:0.0.11
$ docker run --rm -p 21914:21914 -p 21915:21915 -p 21916:21916 -p 21917:21917 -p 21918:21918 -p 21919:21919 -ti lesovsky/pgcenter-testing:0.0.11 /bin/bash
# prepare-test-environment.sh
```

Clone the repo.
```shell
$ git clone https://github.com/lesovsky/pgcenter
$ cd pgcenter
```

Before running tests or building, use Go 1.26 or newer. The `go.mod` toolchain directive selects Go 1.26.10.

Run checks.
```shell
$ make lint
$ make test
$ make vuln
```

Build.
```shell
$ make build
```

### Names convention
All statistics fields have to follow the naming convention. 
1. Add '_total' suffix to **cumulative** integer/float values:
    - `calls_total 12345` - Total number of calls.
    - `backends_total 123` - Current number of connected backends.


2. Add '_total' or '_age' (depends on value context) suffix to **cumulative** time values in human-readable format:
    - `exec_total 03:30:41` - Total time spent executing queries.
    - `stats_age 04:15:33` - Age of stats since last reset.
   

3. Don't use suffixes, for **rate** values:
    - `commits 123` - Number of commits per second.
    - `inserts 456` - Number of inserts per second.


4. Add unit for values measured in bytes, seconds or ratios:
    - `read,ms 10.00` - Number of milliseconds spent reading table.
    - `size_total,KiB 45.00` - Total size of something.
    - `processed,% 85.05` - Current ratio of something processed.


5. Use free form for fields with text values:
    - `database pgbench` - Name of database. 
