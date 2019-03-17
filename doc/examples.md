### README: Examples 

- [General notes](#general-notes)
- [Download](#download)
- [Install devel](#install-from-dev-branch)
- [Run in Docker](#run-in-docker)
- [pgCenter usage](#pgcenter-usage)
---

#### General notes
- run pgCenter on the same host with Postgres, otherwise some features will not work, e.g. config editing, logfile view.
- run pgCenter using database `SUPERUSER` account, e.g. postgres. Some kind of stats aren't available for unprivileged accounts.
- Connections established to Postgres are managed by [lib/pq](https://godoc.org/github.com/lib/pq) driver which supports [.pgpass](https://www.postgresql.org/docs/current/static/libpq-pgpass.html) and libpq [environment variables](https://www.postgresql.org/docs/current/static/libpq-envars.html), such as PGHOST, PGPORT, PGUSER, PGDATABASE, PGPASSWORD, PGOPTIONS.

#### Download
Download latest release from [release page](https://github.com/lesovsky/pgcenter/releases) and unpack, after that pgCenter is ready to run.

#### Install from dev branch
- Install fresh version of `golang` into the system. Install it with package manager or download it directly from the [official site](https://golang.org/dl/). In this case after downloading you need to unpack the archive and copy binaries into the $PATH.
- Clone pgcenter's repo, switch to `dev` branch.
```
git clone https://github.com/lesovsky/pgcenter
cd pgcenter
git checkout --track origin/dev
```
- Build `pgcenter` using `Makefile`. It supposes `go` binary is already is in the PATH.
```
make
```
- Optionally you can install `pgcenter` into `/usr/bin` using `Makefile`. Root privileges required.
```
make install
```
Now `pgcenter` is ready for use.

#### Run in Docker
```
docker build -t pgcenter .
docker run -it --rm pgcenter top -h 1.2.3.4 -U user -d production_db
```

#### pgCenter usage
pgCenter's functionality is splitted among several sub-comands, run specific one to achieve your goals.
In most cases, connection setting can be omitted.

- Run `top` command to connect to Postgres and watching statistics:
    ```
    pgcenter top -h 1.2.3.4 -U postgres production_db
    ```

- Run `profile` command to connect to Postgres and profile backend with PID 12345:
    ```
    pgcenter profile -U postgres -P 12345 production_db
    ```

- Run `profile` command to profile backend with PID 12345 with frequency 50 (every 20ms):
    ```
    pgcenter profile -U postgres -P 12345 -F 50 production_db
    ```

- Run `record` command to connect to Postgres, poll statistics and continuously save to a local file:
    ```
    pgcenter record -f /tmp/stats.tar -U postgres production_db
    ```

- Run `report` command to read previously written file and build a report:
    ```
    pgcenter report -f /tmp/stats.tar --database
    ```

- Run `report` command, build activity report with start time 12:30:00 and end time 12:50:00:
    ```
    pgcenter report --activity --start 123000 --end 125000
    ```
    
- Run `report` command, build tables report order by `seq_scan` column and show only 2 tables per snapshot:
    ```
    pgcenter report --tables --order seq_scan --limit 2
    ```
- Run `report` command, build statements report and show statements that have `UPDATE` word in `query` column:
    ```
    pgcenter report --statements m --grep query:UPDATE
    ```
    
Full list of available parameters available in a built-in help for particular command, use `--help` parameter.

```
pgcenter report --help
```
