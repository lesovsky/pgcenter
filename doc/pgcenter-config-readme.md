### README: pgcenter config

`pgcenter config` is a supplementary tool which allows management of pgCenter’s additional SQL functions.

- [General information](#general-information)
- [Main functions](#main-functions)
- [Usage](#usage)
---

#### General information
As mentioned earlier, pgCenter tracks system's usage through local `procfs` filesystem. It works very well when pgCenter runs on the same host with Postgres, however, what do you do if you want to run pgCenter on your laptop and connect it to a remote Postgres on a far datacenter? 

It's not an issue and pgCenter can track remote system statistics through established Postgres connection using pgCenter's own SQL functions. All you need is to install Postgres built-in procedural language and pgCenter's functions into a remote database and connect as usual. 

*Note: when pgCenter runs on the same host with Postgres, it reads stats directly from /proc and doesn't use Postgres connection for reading system stats.*

Installing and removing functions is possible with `pgcenter config`, however, with few limitations:

- `plperlu` (which means *untrusted plperl*) procedural language must be installed manually in the database you want to connect pgCenter to (see details [here](https://www.postgresql.org/docs/current/static/plperl.html)).
- perl module `Linux::Ethtool::Settings` should be installed in the system, it's used to get speed and duplex of network interfaces and properly calculate some metrics.

Of course, `pgcenter top` can also work with remote Postgres which don't have these SQL functions installed. In this case zeroes will be shown in the system stats interface (load average, cpu, memory, swap, io, network) and multiple errors will  appear in Postgres log. For easier distribution, SQL functions and views used by pgCenter are hard-coded into the source code, but their usage is not limited, so feel free to use it.

Another limitation is related to `procfs` filesystem, which is Linux-specific file system, hence there might be problematic to run pgCenter on operation systems other than Linux. But you can still run pgCenter in Docker.

#### Main functions
- installing and removing SQL functions and views in desired database.

#### Usage
In general, a prefered way is installing dependencies using distro’s default package manager, but it might happen that `Linux::Ethtool::Settings` will not be  available in the official package repo. In this case, you can install perl module using CPAN, but extra dependencies would have to be resolved, such as `make` and `gcc`. Below is an example for Ubuntu Linux.

```
apt install gcc make perl
mcpan Linux::Ethtool::Settings
```

Perhaps it’s possible to use the same approach in other distros, because of perl module name is the same.

Run `config` command and install stats function into a database:
```
pgcenter config --install -h 1.2.3.4 -U postgres db_production
```

See other usage examples [here](examples.md).