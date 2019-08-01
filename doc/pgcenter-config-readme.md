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

#### Main functions
- installing and removing SQL functions and views in desired database.

#### Usage

Run `config` command and install stats schema into a database:
```
pgcenter config --install -h 1.2.3.4 -U postgres db_production
```

If `Linux::Ethtool::Settings` module is not installed in the system, `pgcenter config -i` will fail with the following error:

```
# pgcenter config -i     
ERROR: Can't locate Linux/Ethtool/Settings.pm in @INC (@INC contains: /usr/local/lib64/perl5 /usr/local/share/perl5 /usr/lib64/perl5/vendor_perl /usr/share/perl5/vendor_perl /usr/lib64/perl5 /usr/share/perl5 .) at line 2.
BEGIN failed--compilation aborted at line 2.
DETAIL: 
HINT: 
STATEMENT: CREATE FUNCTION pgcenter.get_netdev_link_settings(INOUT iface CHARACTER VARYING, OUT speed BIGINT, OUT duplex INTEGER) RETURNS RECORD
    LANGUAGE plperlu
    AS $$
    use Linux::Ethtool::Settings;
    if (my $settings = Linux::Ethtool::Settings->new($_[0])) {
      my $if_speed  = $settings->speed();
      my $if_duplex = $settings->duplex() ? 1 : 0;
      return {iface => $_[0], speed => $if_speed, duplex => $if_duplex};
    } else {
      return {iface => $_[0], speed => 0, duplex => -1};
    }
        $$;
```

As you can see the problem is related to `pgcenter.get_netdev_link_settings()` function which depends on `Linux::Ethtool::Settings` module. To fix the issue you need to install the module into your system. 

In general, a preferred way is installing dependencies using distro's default package manager, but it might happen that `Linux::Ethtool::Settings` will not be  available in the official package repo. In this case, you can install perl module using CPAN, but extra dependencies would have to be resolved, such as `make`,`gcc` and others.

Here is a complete example reproduced in Docker environment using using [official docker image for postgresql](https://hub.docker.com/r/centos/postgresql).

```
# yum install -y postgresql-plperl
# yum install -y gcc make perl cpan
# cpan Module::Build
# cpan Linux::Ethtool::Settings
# psql -U postgres -c 'CREATE LANGUAGE plperlu'
# pgcenter config -i -U postgres
# psql -U postgres -c "select * from pgcenter.get_netdev_link_settings('eth0')"
 iface | speed | duplex 
-------+-------+--------
 eth0  | 10000 |      1
(1 row)
```
As you can see, finally function `pgcenter.get_netdev_link_settings()` works well.

Perhaps it’s possible to use the same approach in other distros, because of perl module name is the same, but names of other packages may vary (eg. `postgresql-10-plperl` instead of `postgresql-plperl`).

#### Other notes

Of course, `pgcenter top` can also work with remote Postgres which don't have these SQL functions installed. In this case zeroes will be shown in the system stats interface (load average, cpu, memory, swap, io, network) and multiple errors will  appear in Postgres log. For easier distribution, SQL functions and views used by pgCenter are hard-coded into the source code, but their usage is not limited, so feel free to use it.

Another limitation is related to `procfs` filesystem, which is Linux-specific file system, hence there might be problematic to run pgCenter on operation systems other than Linux. But you can still run pgCenter in Docker.


See other usage examples [here](examples.md).
