### Development

Download pgcenter-testing docker image, run container and start script which prepares testing environment.
```shell
$ docker pull lesovsky/pgcenter-testing:latest
$ docker run --rm -p 21995:21995 -p 21996:21996 -p 21910:21910 -p 21911:21911 -p 21912:21912 -p 21913:21913 -p 21914:21914 -ti lesovsky/pgcenter-testing:latest /bin/bash
# prepare-test-environment.sh
```

Clone the repo.
```shell
$ git clone https://github.com/lesovsky/pgcenter
$ cd pgcenter
```

Before running tests or building make sure your go version is 1.16.

Run tests.
```shell
$ make lint
$ make test
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
