package query

// pgCenter statistics schema.
// Schema is deployed within pgCenter binary, but there is no things like 'go-bindata' or 'migrate'. pgCenter uses
// simple approach to store schema definition - all functions' and views' bodies are stored in text constants and
// organized in sequential set of SQL commands. At schema installation, this set of SQL commands is executed within single
// transaction.

const (
	// Name: pgcenter; Type: SCHEMA; Schema: -
	StatSchemaCreateSchema = `CREATE SCHEMA IF NOT EXISTS pgcenter`

	// Name: get_netdev_link_settings(character varying); Type: FUNCTION; Schema: pgcenter
	StatSchemaCreateFunction1 = `CREATE OR REPLACE FUNCTION pgcenter.get_netdev_link_settings(INOUT iface CHARACTER VARYING, OUT speed BIGINT, OUT duplex INTEGER) RETURNS RECORD
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
$$;`

	// Name: get_sys_clk_ticks(); Type: FUNCTION; Schema: pgcenter
	StatSchemaCreateFunction2 = `CREATE OR REPLACE FUNCTION pgcenter.get_sys_clk_ticks() RETURNS integer
LANGUAGE plperlu
AS $$
use POSIX;
$clock_ticks = POSIX::sysconf( &POSIX::_SC_CLK_TCK );
return $clock_ticks;
$$;`

	// Name: get_proc_stats(character varying, character varying, character varying, integer); Type: FUNCTION; Schema: pgcenter
	StatSchemaCreateFunction3 = `CREATE OR REPLACE FUNCTION pgcenter.get_proc_stats(character varying, character varying, character varying, integer) RETURNS SETOF record
LANGUAGE plperlu
AS $$
open FILE, $_[0];
my @cntn = (); $i = 0;
while (<FILE>) {
	# skip header if required.
    if ($i < $_[3]) { $i++; next; }
    chomp;
    my @items = map {s/^\s+|\s+$//g; $_;} split ($_[1]);
    my %iitems;
    # use filter if required.
    if ($items[0] =~ $_[2] && $_[2] ne "") {
    	@iitems{map 'col'.$_, 0..$#items} = @items;
        push @cntn, \%iitems;
	} elsif ($_[2] eq "") {
    	@iitems{map 'col'.$_, 0..$#items} = @items;
        push @cntn, \%iitems;
	}
    $i++
}
close FILE;
return \@cntn;
$$;`

	// Name: get_filesystem_stats; Type: FUNCTION; Schema: pgcenter
	StatSchemaCreateFunction4 = `CREATE OR REPLACE FUNCTION pgcenter.get_filesystem_stats(INOUT mountpoint CHARACTER VARYING, OUT size_bytes NUMERIC, OUT free_bytes NUMERIC, OUT avail_bytes NUMERIC, OUT used_bytes NUMERIC, OUT reserved_bytes NUMERIC, OUT used_bytes_ratio NUMERIC, OUT size_files NUMERIC, OUT free_files NUMERIC, OUT used_files NUMERIC, OUT used_files_ratio NUMERIC) RETURNS RECORD
LANGUAGE plperlu
AS $$
use Filesys::Df;

my $ref = df($_[0], 1);
if(defined($ref)) {
    my $size_bytes = $ref->{blocks};
    my $free_bytes = $ref->{bfree};
    my $avail_bytes = $ref->{bavail};
    my $used_bytes = $ref->{used};
    my $reserved_bytes = $ref->{bfree} - $ref->{bavail};
    my $used_bytes_ratio = $ref->{per};
    
    my $size_files = $ref->{files};
    my $free_files = $ref->{ffree};
    my $used_files = $ref->{files} - $ref->{ffree};
    my $used_files_ratio = $ref->{fper};
    
    return {
        mountpoint => $_[0],
        size_bytes => $size_bytes,
        free_bytes => $free_bytes,
        avail_bytes => $avail_bytes,
        used_bytes => $used_bytes,
        reserved_bytes => $reserved_bytes,
        used_bytes_ratio => $used_bytes_ratio,
        size_files => $size_files,
        free_files => $free_files,
        used_files => $used_files,
        used_files_ratio => $used_files_ratio,
    };
} else {
    return {
        mountpoint => $_[0], size_bytes => 0, free_bytes => 0, avail_bytes => 0,
        used_bytes => 0, reserved_bytes => 0, used_bytes_ratio => 0,
        size_files => 0, free_files => 0, used_files => 0, used_files_ratio => 0,
    };
}
$$;`

	// Name: sys_proc_diskstats; Type: VIEW; Schema: pgcenter
	StatSchemaCreateView1 = `CREATE OR REPLACE VIEW pgcenter.sys_proc_diskstats AS
SELECT get_proc_stats.col0 AS maj,
get_proc_stats.col1 AS min,
get_proc_stats.col2 AS dev,
get_proc_stats.col3 AS reads,
get_proc_stats.col4 AS rmerges,
get_proc_stats.col5 AS rsects,
get_proc_stats.col6 AS rspent,
get_proc_stats.col7 AS writes,
get_proc_stats.col8 AS wmerges,
get_proc_stats.col9 AS wsects,
get_proc_stats.col10 AS wspent,
get_proc_stats.col11 AS inprog,
get_proc_stats.col12 AS spent,
get_proc_stats.col13 AS weighted,
COALESCE(get_proc_stats.col14, (0)::double precision) AS discards,
COALESCE(get_proc_stats.col15, (0)::double precision) AS dmerges,
COALESCE(get_proc_stats.col16, (0)::double precision) AS dsectors,
COALESCE(get_proc_stats.col17, (0)::double precision) AS dspent,
COALESCE(get_proc_stats.col18, (0)::double precision) AS flushes,
COALESCE(get_proc_stats.col19, (0)::double precision) AS fspent
FROM pgcenter.get_proc_stats('/proc/diskstats'::character varying, ' '::character varying, ''::character varying, 0) get_proc_stats(col0 integer, col1 integer, col2 character varying, col3 double precision, col4 double precision, col5 double precision, col6 double precision, col7 double precision, col8 double precision, col9 double precision, col10 double precision, col11 double precision, col12 double precision, col13 double precision, col14 double precision, col15 double precision, col16 double precision, col17 double precision, col18 double precision, col19 double precision);`

	// Name: sys_proc_loadavg; Type: VIEW; Schema: pgcenter
	StatSchemaCreateView2 = `CREATE OR REPLACE VIEW pgcenter.sys_proc_loadavg AS
SELECT get_proc_stats.col0 AS min1,
get_proc_stats.col1 AS min5,
get_proc_stats.col2 AS min15,
get_proc_stats.col3 AS procnum,
get_proc_stats.col4 AS last_pid
FROM pgcenter.get_proc_stats('/proc/loadavg'::character varying, ' '::character varying, ''::character varying, 0)
AS (col0 double precision, col1 double precision, col2 double precision, col3 character varying, col4 integer);`

	// Name: sys_proc_meminfo; Type: VIEW; Schema: pgcenter
	StatSchemaCreateView3 = `CREATE OR REPLACE VIEW pgcenter.sys_proc_meminfo AS
SELECT get_proc_stats.col0 AS metric,
get_proc_stats.col1 AS metric_value,
get_proc_stats.col2 AS unit
FROM pgcenter.get_proc_stats('/proc/meminfo'::character varying, ' '::character varying, ''::character varying, 0)
AS (col0 character varying, col1 bigint, col2 character varying);`

	// Name: sys_proc_netdev; Type: VIEW; Schema: pgcenter
	StatSchemaCreateView4 = `CREATE OR REPLACE VIEW pgcenter.sys_proc_netdev AS
SELECT get_proc_stats.col0 AS iface,
get_proc_stats.col1 AS recv_bytes,
get_proc_stats.col2 AS recv_pckts,
get_proc_stats.col3 AS recv_err,
get_proc_stats.col4 AS recv_drop,
get_proc_stats.col5 AS recv_fifo,
get_proc_stats.col6 AS recv_frame,
get_proc_stats.col7 AS recv_cmpr,
get_proc_stats.col8 AS recv_mcast,
get_proc_stats.col9 AS sent_bytes,
get_proc_stats.col10 AS sent_pckts,
get_proc_stats.col11 AS sent_err,
get_proc_stats.col12 AS sent_drop,
get_proc_stats.col13 AS sent_fifo,
get_proc_stats.col14 AS sent_colls,
get_proc_stats.col15 AS sent_carrier,
get_proc_stats.col16 AS sent_cmpr
FROM pgcenter.get_proc_stats('/proc/net/dev'::character varying, ' '::character varying, ''::character varying, 2)
AS (col0 character varying, col1 float, col2 float, col3 float, col4 float, col5 float, col6 float, col7 float, col8 float, col9 float, col10 float, col11 float, col12 float, col13 float, col14 float, col15 float, col16 float)`

	// Name: sys_proc_stat; Type: VIEW; Schema: pgcenter
	StatSchemaCreateView5 = `CREATE OR REPLACE VIEW pgcenter.sys_proc_stat AS
SELECT get_proc_stats.col0 AS cpu,
get_proc_stats.col1 AS us_time,
get_proc_stats.col2 AS ni_time,
get_proc_stats.col3 AS sy_time,
get_proc_stats.col4 AS id_time,
get_proc_stats.col5 AS wa_time,
get_proc_stats.col6 AS hi_time,
get_proc_stats.col7 AS si_time,
get_proc_stats.col8 AS st_time,
get_proc_stats.col9 AS quest_time,
get_proc_stats.col10 AS guest_ni_time
FROM pgcenter.get_proc_stats('/proc/stat'::character varying, ' '::character varying, 'cpu'::character varying, 0)
AS (col0 character varying, col1 bigint, col2 bigint, col3 bigint, col4 bigint, col5 bigint, col6 bigint, col7 bigint, col8 bigint, col9 bigint, col10 bigint);`

	// Name: sys_proc_uptime; Type: VIEW; Schema: pgcenter
	StatSchemaCreateView6 = `CREATE OR REPLACE VIEW pgcenter.sys_proc_uptime AS
SELECT get_proc_stats.col0 AS seconds_total,
get_proc_stats.col1 AS seconds_idle
FROM pgcenter.get_proc_stats('/proc/uptime'::character varying, ' '::character varying, ''::character varying, 0)
AS (col0 numeric, col1 numeric);`

	// Name: sys_proc_mounts; Type: VIEW; Schema: pgcenter
	StatSchemaCreateView7 = `CREATE OR REPLACE VIEW pgcenter.sys_proc_mounts AS
SELECT get_proc_stats.col0 AS device,
get_proc_stats.col1 AS mountpoint,
get_proc_stats.col2 AS fstype,
get_proc_stats.col3 AS options,
get_proc_stats.col4 AS dump,
get_proc_stats.col5 AS fsck_order
FROM pgcenter.get_proc_stats('/proc/mounts'::character varying, ' '::character varying, ''::character varying, 0)
AS (col0 character varying, col1 character varying, col2 character varying, col3 character varying, col4 integer, col5 integer);`

	// Name: pgcenter; Type: SCHEMA; Schema: -
	StatSchemaDropSchema = "DROP SCHEMA IF EXISTS pgcenter CASCADE"
)
