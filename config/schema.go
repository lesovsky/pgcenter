// pgCenter statistics schema.
// Schema is deployed within pgCenter binary, but there is no things like 'go-bindata' or 'migrate'. pgCenter uses
// simple approach to store schema definition - all functions' and views' bodies are stored in text constants and
// organized in sequential set of SQL commands. At schema installation, this set of SQL commands is executed within single
// transaction.

package config

const (
	// Name: pgcenter; Type: SCHEMA; Schema: -
	// The 'IF NOT EXISTS' clause isn't used because it's supported since 9.3.
	createSchemaSql = `CREATE SCHEMA pgcenter;`

	// Name: get_netdev_link_settings(character varying); Type: FUNCTION; Schema: pgcenter
	createProc1FunctionSql = `CREATE FUNCTION pgcenter.get_netdev_link_settings(INOUT iface CHARACTER VARYING, OUT speed BIGINT, OUT duplex INTEGER) RETURNS RECORD
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
	createProc2FunctionSql = `CREATE FUNCTION pgcenter.get_sys_clk_ticks() RETURNS integer
    LANGUAGE plperlu
    AS $$
    use POSIX;
    $clock_ticks = POSIX::sysconf( &POSIX::_SC_CLK_TCK );
    return $clock_ticks;
	$$;`

	// Name: get_proc_stats(character varying, character varying, character varying, integer); Type: FUNCTION; Schema: pgcenter
	createProc3FunctionSql = `CREATE FUNCTION pgcenter.get_proc_stats(character varying, character varying, character varying, integer) RETURNS SETOF record
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

	// Name: sys_proc_diskstats; Type: VIEW; Schema: pgcenter
	createView1Sql = `CREATE VIEW pgcenter.sys_proc_diskstats AS
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
    	get_proc_stats.col13 AS weighted
   	FROM pgcenter.get_proc_stats('/proc/diskstats'::character varying, ' '::character varying, ''::character varying, 0)
   	AS (col0 integer, col1 integer, col2 character varying, col3 bigint, col4 bigint, col5 bigint, col6 bigint, col7 bigint, col8 bigint, col9 bigint, col10 bigint, col11 bigint, col12 bigint, col13 bigint);`

	// Name: sys_proc_loadavg; Type: VIEW; Schema: pgcenter
	createView2Sql = `CREATE VIEW pgcenter.sys_proc_loadavg AS
 	SELECT get_proc_stats.col0 AS min1,
    	get_proc_stats.col1 AS min5,
    	get_proc_stats.col2 AS min15,
    	get_proc_stats.col3 AS procnum,
    	get_proc_stats.col4 AS last_pid
   	FROM pgcenter.get_proc_stats('/proc/loadavg'::character varying, ' '::character varying, ''::character varying, 0)
   	AS (col0 double precision, col1 double precision, col2 double precision, col3 character varying, col4 integer);`

	// Name: sys_proc_meminfo; Type: VIEW; Schema: pgcenter
	createView3Sql = `CREATE VIEW pgcenter.sys_proc_meminfo AS
 	SELECT get_proc_stats.col0 AS metric,
    	get_proc_stats.col1 AS metric_value,
    	get_proc_stats.col2 AS unit
   	FROM pgcenter.get_proc_stats('/proc/meminfo'::character varying, ' '::character varying, ''::character varying, 0)
  	AS (col0 character varying, col1 bigint, col2 character varying);`

	// Name: sys_proc_netdev; Type: VIEW; Schema: pgcenter
	createView4Sql = `CREATE VIEW pgcenter.sys_proc_netdev AS
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
   	AS (col0 character varying, col1 bigint, col2 bigint, col3 bigint, col4 bigint, col5 bigint, col6 bigint, col7 bigint, col8 bigint, col9 bigint, col10 bigint, col11 bigint, col12 bigint, col13 bigint, col14 bigint, col15 bigint, col16 bigint);`

	// Name: sys_proc_stat; Type: VIEW; Schema: pgcenter
	createView5Sql = `CREATE VIEW pgcenter.sys_proc_stat AS
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
	createView6Sql = `CREATE VIEW pgcenter.sys_proc_uptime AS
 	SELECT get_proc_stats.col0 AS seconds_total,
    	get_proc_stats.col1 AS seconds_idle
   	FROM pgcenter.get_proc_stats('/proc/uptime'::character varying, ' '::character varying, ''::character varying, 0)
   	AS (col0 numeric, col1 numeric);`

	// Name: pgcenter; Type: SCHEMA; Schema: -
	dropSchemaSql = `DROP SCHEMA pgcenter CASCADE;`
)

var (
	createSchemaSqlSet = []string{
		createSchemaSql,
		createProc1FunctionSql,
		createProc2FunctionSql,
		createProc3FunctionSql,
		createView1Sql,
		createView2Sql,
		createView3Sql,
		createView4Sql,
		createView5Sql,
		createView6Sql,
	}

	dropSchemaSqlSet = []string{
		dropSchemaSql,
	}
)
