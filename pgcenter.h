/*
 * pgcenter: administrative console for PostgreSQL.
 * (C) 2015 by Alexey V. Lesovsky (lesovsky <at> gmail.com)
 */

#ifndef PGCENTER_H
#define PGCENTER_H

#define PROGRAM_NAME        "pgcenter"
#define PROGRAM_VERSION     0.2
#define PROGRAM_RELEASE     0
#define PROGRAM_AUTHORS_CONTACTS    "<lesovsky@gmail.com>"

/* sizes, limits and defaults */
#define XS_BUF_LEN	16
#define S_BUF_LEN	64
#define M_BUF_LEN	128
#define L_BUF_LEN	1024
#define X_BUF_LEN	4096
#define XL_BUF_LEN	BUFSIZ

#define CONN_ARG_MAXLEN		S_BUF_LEN
#define CONNINFO_MAXLEN		S_BUF_LEN * 5	/* host, port, username, dbname, password */
#define COL_MAXLEN		S_BUF_LEN
#define CONNINFO_TITLE_LEN	48
#define QUERY_MAXLEN		XL_BUF_LEN

#define ERRSIZE             128
#define MAX_SCREEN          8
#define INVALID_ORDER_KEY   99
#define PG_STAT_ACTIVITY_MIN_AGE_DEFAULT "00:00:00.5"

#define LOADAVG_FILE            "/proc/loadavg"
#define STAT_FILE               "/proc/stat"
#define UPTIME_FILE             "/proc/uptime"
#define MEMINFO_FILE            "/proc/meminfo"
#define DISKSTATS_FILE          "/proc/diskstats"
#define NETDEV_FILE             "/proc/net/dev"
#define PGCENTERRC_FILE         ".pgcenterrc"
#define PG_CONF_FILE            "postgresql.conf"
#define PG_HBA_FILE             "pg_hba.conf"
#define PG_IDENT_FILE           "pg_ident.conf"
#define PG_RECOVERY_FILE        "recovery.conf"

/* 
 * GUC 
 * These definitions are used in edit_config() for editing postgres config files.
 * But here we have one issue - if we want to edit the recovery.conf, but GUC for 
 * the recovery.conf doesn't exists. For this reason we use data_directory GUC.
 * See details in get_conf_value() function.
 */
#define GUC_CONFIG_FILE         "config_file"
#define GUC_HBA_FILE            "hba_file"
#define GUC_IDENT_FILE          "ident_file"
#define GUC_DATA_DIRECTORY      "data_directory"
#define GUC_SERVER_VERSION      "server_version"
#define GUC_SERVER_VERSION_NUM  "server_version_num"
#define GUC_AV_MAX_WORKERS	"autovacuum_max_workers"

/* 
 * PostgreSQL version notations:
 * PostgreSQL stores his version in XXYYY format, where XX is major version 
 * and YYY is minor. For example, 90540 means 9.5.4.
 * */
#define PG92 90200

#define PGCENTERRC_READ_OK  0
#define PGCENTERRC_READ_ERR 1

/* others defaults */
#define DEFAULT_PAGER       "less"
#define DEFAULT_EDITOR      "vi"
#define DEFAULT_PSQL        "psql"
#define DEFAULT_INTERVAL    1000000
#define INTERVAL_MAXLEN	    300			/* in seconds */
#define INTERVAL_STEP       200000

#define HZ                  hz
unsigned int hz;

/* information contexts */
#define TOTAL_CONTEXTS      		13
#define PG_STAT_DATABASE_NUM                    0
#define PG_STAT_REPLICATION_NUM                 1
#define PG_STAT_TABLES_NUM                      2
#define PG_STAT_INDEXES_NUM                     3
#define PG_STATIO_TABLES_NUM                    4
#define PG_TABLES_SIZE_NUM                      5
#define PG_STAT_ACTIVITY_LONG_NUM               6
#define PG_STAT_FUNCTIONS_NUM                   7
#define PG_STAT_STATEMENTS_TIMING_NUM           8
#define PG_STAT_STATEMENTS_GENERAL_NUM          9
#define PG_STAT_STATEMENTS_IO_NUM               10
#define PG_STAT_STATEMENTS_TEMP_NUM             11
#define PG_STAT_STATEMENTS_LOCAL_NUM		12

#define GROUP_ACTIVE        1 << 0
#define GROUP_IDLE          1 << 1
#define GROUP_IDLE_IN_XACT  1 << 2
#define GROUP_WAITING       1 << 3
#define GROUP_OTHER         1 << 4

#define SUBSCREEN_NONE      0
#define SUBSCREEN_LOGTAIL   1
#define SUBSCREEN_IOSTAT    2
#define SUBSCREEN_NICSTAT   3

/* enum for query context */
enum context
{
    pg_stat_database,
    pg_stat_replication,
    pg_stat_tables,
    pg_stat_indexes,
    pg_statio_tables,
    pg_tables_size,
    pg_stat_activity_long,
    pg_stat_functions,
    pg_stat_statements_timing,
    pg_stat_statements_general,
    pg_stat_statements_io,
    pg_stat_statements_temp,
    pg_stat_statements_local
};

#define DEFAULT_QUERY_CONTEXT   pg_stat_database

/* struct for context list used in screen */
struct context_s
{
    enum context context;
    int order_key;
    bool order_desc;
};

/* struct for input args */
struct args_s
{
    int count;
    char connfile[PATH_MAX];
    char host[CONN_ARG_MAXLEN];
    char port[CONN_ARG_MAXLEN];
    char user[CONN_ARG_MAXLEN];
    char dbname[CONN_ARG_MAXLEN];
    bool need_passwd;
};

#define ARGS_SIZE (sizeof(struct args_s))

/* struct for postgres specific details, get that when connected to postgres server */
struct pg_special_s
{
    bool pg_is_in_recovery;			/* is postgres a standby? - true/false */
    unsigned int av_max_workers;		/* autovacuum_max_workers GUC value */
    char pg_version_num[XS_BUF_LEN];		/* postgresql version XXYYZZ format */
    char pg_version[XS_BUF_LEN];		/* postgresql version X.Y.Z format */
};

#define PG_SPECIAL_SIZE (sizeof(struct pg_special_s))

/* struct which define connection options */
struct screen_s
{
    int screen;
    bool conn_used;
    char host[CONN_ARG_MAXLEN];
    char port[CONN_ARG_MAXLEN];
    char user[CONN_ARG_MAXLEN];
    char dbname[CONN_ARG_MAXLEN];
    char password[CONN_ARG_MAXLEN];
    char conninfo[CONNINFO_MAXLEN];
    struct pg_special_s pg_special;
    bool subscreen_enabled;                     /* subscreen status: on/off */
    int subscreen;                              /* subscreen type: logtail, iostat, etc. */
    char log_path[PATH_MAX];                    /* logfile path for logtail subscreen */
    int log_fd;                                 /* logfile fd for log viewing */
    enum context current_context;
    char pg_stat_activity_min_age[XS_BUF_LEN];
    struct context_s context_list[TOTAL_CONTEXTS];
    int signal_options;
    bool pg_stat_sys;
};

#define SCREEN_SIZE (sizeof(struct screen_s))

/* struct which used for cpu statistic */
struct cpu_s {
    unsigned long long cpu_user;
    unsigned long long cpu_nice;
    unsigned long long cpu_sys;
    unsigned long long cpu_idle;
    unsigned long long cpu_iowait;
    unsigned long long cpu_steal;
    unsigned long long cpu_hardirq;
    unsigned long long cpu_softirq;
    unsigned long long cpu_guest;
    unsigned long long cpu_guest_nice;
};

#define STATS_CPU_SIZE (sizeof(struct cpu_s))

/* struct which used for memory statistics */
struct mem_s {
    unsigned long long mem_total;
    unsigned long long mem_free;
    unsigned long long mem_used;
    unsigned long long swap_total;
    unsigned long long swap_free;
    unsigned long long swap_used;
    unsigned long long cached;
    unsigned long long buffers;
    unsigned long long dirty;
    unsigned long long writeback;
    unsigned long long slab;
};

#define STATS_MEM_SIZE (sizeof(struct mem_s))

/* struct which used for io statistics */
struct iodata_s
{
    int major;
    int minor;
    char devname[S_BUF_LEN];
    unsigned long r_completed;          /* reads completed successfully */
    unsigned long r_merged;             /* reads merged */
    unsigned long r_sectors;            /* sectors read */
    unsigned long r_spent;              /* time spent reading (ms) */
    unsigned long w_completed;          /* writes completed */
    unsigned long w_merged;             /* writes merged */
    unsigned long w_sectors;            /* sectors written */
    unsigned long w_spent;              /* time spent writing (ms) */
    unsigned long io_in_progress;       /* I/Os currently in progress */
    unsigned long t_spent;              /* time spent doing I/Os (ms) */
    unsigned long t_weighted;           /* weighted time spent doing I/Os (ms) */
    double arqsz;                       /* average request size */
    double await;                       /* latency */
    double util;                        /* device utilization */
};

#define STATS_IODATA_SIZE (sizeof(struct iodata_s))

/* This may be defined by <linux/ethtool.h> */
#ifndef DUPLEX_UNKNOWN
#define DUPLEX_UNKNOWN          0xff
#endif /* DUPLEX_UNKNOWN */

/* struct for NIC data (settings and stats) */
struct nicdata_s
{
    char ifname[IF_NAMESIZE + 1];
    long speed;
    int duplex;
    unsigned long rbytes;
    unsigned long rpackets;
    unsigned long ierr;
    unsigned long wbytes;
    unsigned long wpackets;
    unsigned long oerr;
    unsigned long coll;
    unsigned long sat;
};

#define STATS_NICDATA_SIZE (sizeof(struct nicdata_s))

/*
 * Macros used to display statistics values.
 * NB: Define SP_VALUE() to normalize to %;
 */
#define SP_VALUE(m,n,p) (((double) ((n) - (m))) / (p) * 100)
#define S_VALUE(m,n,p) (((double) ((n) - (m))) / (p) * HZ)

/* Macros used to determine array size */
#define ARRAY_SIZE(a) (sizeof(a) / sizeof(a[0]))

/* struct for column widths */
struct colAttrs {
    char name[COL_MAXLEN];
    int width;
};

/* PostgreSQL answers, see PQresultStatus() at http://www.postgresql.org/docs/9.4/static/libpq-exec.html */
#define PG_CMD_OK       PGRES_COMMAND_OK
#define PG_TUP_OK       PGRES_TUPLES_OK
#define PG_FATAL_ERR    PGRES_FATAL_ERROR

/* sysstat screen queries */
#define PG_STAT_ACTIVITY_COUNT_QUERY \
    "WITH pgsa AS (SELECT * FROM pg_stat_activity) \
       SELECT \
         (SELECT count(*) AS total FROM pgsa), \
         (SELECT count(*) AS idle FROM pgsa WHERE state = 'idle'), \
         (SELECT count(*) AS idle_in_xact FROM pgsa WHERE state IN ('idle in transaction', 'idle in transaction (aborted)')), \
         (SELECT count(*) AS active FROM pgsa WHERE state = 'active'), \
         (SELECT count(*) AS waiting FROM pgsa WHERE waiting), \
         (SELECT count(*) AS others FROM pgsa WHERE state IN ('fastpath function call','disabled'));"

#define PG_STAT_ACTIVITY_AV_COUNT_QUERY \
    "WITH pgsa AS (SELECT * FROM pg_stat_activity) \
       SELECT \
         (SELECT count(*) AS av_workers FROM pgsa WHERE query ~* '^autovacuum:' AND pid <> pg_backend_pid()), \
         (SELECT count(*) AS av_wrap FROM pgsa WHERE query ~* '^autovacuum:.*to prevent wraparound' AND pid <> pg_backend_pid()), \
	 (SELECT count(*) AS v_manual FROM pgsa WHERE query ~* '^vacuum' AND pid <> pg_backend_pid()), \
	 (SELECT coalesce(date_trunc('seconds', max(now() - xact_start)), '00:00:00') AS av_maxtime FROM pgsa \
	 WHERE (query ~* '^autovacuum:' OR query ~* '^vacuum') AND pid <> pg_backend_pid());"

#define PG_STAT_STATEMENTS_SYS_QUERY \
        "SELECT (sum(total_time) / sum(calls))::numeric(6,3) AS avg_query, sum(calls) AS total_calls FROM pg_stat_statements"
#define PG_STAT_ACTIVITY_SYS_QUERY \
        "SELECT coalesce(date_trunc('seconds', max(now() - xact_start)), '00:00:00') FROM pg_stat_activity"

/* context queries */
#define PG_STAT_DATABASE_91_QUERY \
    "SELECT \
        datname, \
        xact_commit AS commit, xact_rollback AS rollback, \
        blks_read AS reads, blks_hit AS hits, \
        tup_returned AS returned, tup_fetched AS fetched, \
        tup_inserted AS inserts, tup_updated AS updates, tup_deleted AS deletes, \
        conflicts \
    FROM pg_stat_database \
    ORDER BY datname"

#define PG_STAT_DATABASE_QUERY \
    "SELECT \
        datname, \
        xact_commit AS commit, xact_rollback AS rollback, \
        blks_read AS reads, blks_hit AS hits, \
        tup_returned AS returned, tup_fetched AS fetched, \
        tup_inserted AS inserts, tup_updated AS updates, tup_deleted AS deletes, \
        conflicts, deadlocks, \
        temp_files AS tmp_files, temp_bytes AS tmp_bytes, \
        blk_read_time AS read_t, blk_write_time AS write_t \
    FROM pg_stat_database \
    ORDER BY datname"

#define PG_STAT_DATABASE_ORDER_MIN          1
#define PG_STAT_DATABASE_ORDER_91_MAX       10
#define PG_STAT_DATABASE_ORDER_LATEST_MAX   15

#define PG_STAT_REPLICATION_QUERY_P1 \
    "SELECT \
        client_addr AS client, usename AS user, application_name AS name, \
        state, sync_state AS mode, \
	(pg_xlog_location_diff("
#define PG_STAT_REPLICATION_QUERY_P2 \
    ",sent_location) / 1024)::int as pending, \
	(pg_xlog_location_diff(sent_location,write_location) / 1024)::int as write, \
	(pg_xlog_location_diff(write_location,flush_location) / 1024)::int as flush, \
	(pg_xlog_location_diff(flush_location,replay_location) / 1024)::int as replay, \
	(pg_xlog_location_diff("
#define PG_STAT_REPLICATION_QUERY_P3 \
    ",replay_location))::int / 1024 as total_lag FROM pg_stat_replication"

/* use functions depending on recovery */
#define PG_STAT_REPLICATION_NOREC "pg_current_xlog_location()"
#define PG_STAT_REPLICATION_REC "pg_last_xlog_receive_location()"

#define PG_STAT_REPLICATION_ORDER_MIN 5
#define PG_STAT_REPLICATION_ORDER_MAX 9

#define PG_STAT_TABLES_QUERY_P1 \
    "SELECT \
        schemaname || '.' || relname as relation, \
        seq_scan, seq_tup_read as seq_read, \
        idx_scan, idx_tup_fetch as idx_fetch, \
        n_tup_ins as inserts, n_tup_upd as updates, \
        n_tup_del as deletes, n_tup_hot_upd as hot_updates, \
        n_live_tup as live, n_dead_tup as dead \
    FROM pg_stat_"
#define PG_STAT_TABLES_QUERY_P2 "_tables ORDER BY 1"

#define PG_STAT_TABLES_ORDER_MIN 1
#define PG_STAT_TABLES_ORDER_MAX 10

#define PG_STATIO_TABLES_QUERY_P1 \
    "SELECT \
        schemaname ||'.'|| relname as relation, \
        heap_blks_read * (SELECT current_setting('block_size')::int / 1024) AS heap_read, \
        heap_blks_hit * (SELECT current_setting('block_size')::int / 1024) AS heap_hit, \
        idx_blks_read * (SELECT current_setting('block_size')::int / 1024) AS idx_read, \
        idx_blks_hit * (SELECT current_setting('block_size')::int / 1024) AS idx_hit, \
        toast_blks_read * (SELECT current_setting('block_size')::int / 1024) AS toast_read, \
        toast_blks_hit * (SELECT current_setting('block_size')::int / 1024) AS toast_hit, \
        tidx_blks_read * (SELECT current_setting('block_size')::int / 1024) AS tidx_read, \
        tidx_blks_hit * (SELECT current_setting('block_size')::int / 1024) AS tidx_hit \
    FROM pg_statio_"
#define PG_STATIO_TABLES_QUERY_P2 "_tables ORDER BY 1"

#define PG_STATIO_TABLES_ORDER_MIN 1
#define PG_STATIO_TABLES_ORDER_MAX 8

#define PG_STAT_INDEXES_QUERY_P1 \
    "SELECT \
        s.schemaname ||'.'|| s.relname as relation, s.indexrelname AS index, \
        s.idx_scan, s.idx_tup_read, s.idx_tup_fetch, \
        i.idx_blks_read * (SELECT current_setting('block_size')::int / 1024) AS idx_read, \
        i.idx_blks_hit * (SELECT current_setting('block_size')::int / 1024) AS idx_hit \
    FROM \
        pg_stat_"
#define PG_STAT_INDEXES_QUERY_P2 "_indexes s, pg_statio_"
#define PG_STAT_INDEXES_QUERY_P3 "_indexes i WHERE s.indexrelid = i.indexrelid ORDER BY 1"

#define PG_STAT_INDEXES_ORDER_MIN 2
#define PG_STAT_INDEXES_ORDER_MAX 6

#define PG_TABLES_SIZE_QUERY_P1 \
    "SELECT \
        s.schemaname ||'.'|| s.relname AS relation, \
        pg_total_relation_size((s.schemaname ||'.'|| s.relname)::regclass) / 1024 AS total_size, \
        pg_relation_size((s.schemaname ||'.'|| s.relname)::regclass) / 1024 AS rel_size, \
        (pg_total_relation_size((s.schemaname ||'.'|| s.relname)::regclass) / 1024) - \
            (pg_relation_size((s.schemaname ||'.'|| s.relname)::regclass) / 1024) AS idx_size, \
        pg_total_relation_size((s.schemaname ||'.'|| s.relname)::regclass) / 1024 AS total_change, \
        pg_relation_size((s.schemaname ||'.'|| s.relname)::regclass) / 1024 AS rel_change, \
        (pg_total_relation_size((s.schemaname ||'.'|| s.relname)::regclass) / 1024) - \
            (pg_relation_size((s.schemaname ||'.'|| s.relname)::regclass) / 1024) AS idx_change \
        FROM pg_stat_"
#define PG_TABLES_SIZE_QUERY_P2 "_tables s, pg_class c WHERE s.relid = c.oid ORDER BY 1"

#define PG_TABLES_SIZE_ORDER_MIN 4
#define PG_TABLES_SIZE_ORDER_MAX 6

#define PG_STAT_ACTIVITY_LONG_91_QUERY_P1 \
    "SELECT \
        procpid AS pid, client_addr AS cl_addr, client_port AS cl_port, \
        datname, usename, waiting, \
        date_trunc('seconds', clock_timestamp() - xact_start) AS xact_age, \
        date_trunc('seconds', clock_timestamp() - query_start) AS query_age, \
        regexp_replace( \
        regexp_replace( \
        regexp_replace( \
        regexp_replace( \
        regexp_replace(current_query, \
            E'\\\\?(::[a-zA-Z_]+)?( *, *\\\\?(::[a-zA-Z_]+)?)+', '?', 'g'), \
            E'\\\\$[0-9]+(::[a-zA-Z_]+)?( *, *\\\\$[0-9]+(::[a-zA-Z_]+)?)*', '$N', 'g'), \
            E'--.*$', '', 'ng'), \
            E'/\\\\*.*?\\\\*\\/', '', 'g'), \
            E'\\\\s+', ' ', 'g') AS query \
    FROM pg_stat_activity \
    WHERE ((clock_timestamp() - xact_start) > '"
#define PG_STAT_ACTIVITY_LONG_91_QUERY_P2 \
    "'::interval OR (clock_timestamp() - query_start) > '"
#define PG_STAT_ACTIVITY_LONG_91_QUERY_P3 \
    "'::interval) AND current_query <> '<IDLE>' AND procpid <> pg_backend_pid() \
    ORDER BY COALESCE(xact_start, query_start)"

#define PG_STAT_ACTIVITY_LONG_QUERY_P1 \
    "SELECT \
        pid, client_addr AS cl_addr, client_port AS cl_port, \
        datname, usename, state, waiting, \
        date_trunc('seconds', clock_timestamp() - xact_start) AS xact_age, \
        date_trunc('seconds', clock_timestamp() - query_start) AS query_age, \
        date_trunc('seconds', clock_timestamp() - state_change) AS change_age, \
        regexp_replace( \
        regexp_replace( \
        regexp_replace( \
        regexp_replace( \
        regexp_replace(query, \
            E'\\\\?(::[a-zA-Z_]+)?( *, *\\\\?(::[a-zA-Z_]+)?)+', '?', 'g'), \
            E'\\\\$[0-9]+(::[a-zA-Z_]+)?( *, *\\\\$[0-9]+(::[a-zA-Z_]+)?)*', '$N', 'g'), \
            E'--.*$', '', 'ng'), \
            E'/\\\\*.*?\\\\*\\/', '', 'g'), \
            E'\\\\s+', ' ', 'g') AS query \
    FROM pg_stat_activity \
    WHERE ((clock_timestamp() - xact_start) > '"
#define PG_STAT_ACTIVITY_LONG_QUERY_P2 \
    "'::interval OR (clock_timestamp() - query_start) > '"
#define PG_STAT_ACTIVITY_LONG_QUERY_P3 \
    "'::interval) AND state <> 'idle' AND pid <> pg_backend_pid() \
    ORDER BY COALESCE(xact_start, query_start)"

/* don't use array sorting when showing long activity, row order defined in query */
#define PG_STAT_ACTIVITY_LONG_ORDER_MIN INVALID_ORDER_KEY
#define PG_STAT_ACTIVITY_LONG_ORDER_MAX INVALID_ORDER_KEY

#define PG_STAT_FUNCTIONS_QUERY_P1 \
    "SELECT \
        funcid, schemaname ||'.'||funcname AS function, \
        calls AS total_calls, calls AS calls, \
        date_trunc('seconds', total_time / 1000 * '1 second'::interval) AS total_t, \
        date_trunc('seconds', self_time / 1000 * '1 second'::interval) AS self_t, \
        round((total_time / calls)::numeric, 4) AS avg_t, \
        round((self_time / calls)::numeric, 4) AS avg_self_t \
    FROM pg_stat_user_functions \
    ORDER BY "
#define PG_STAT_FUNCTIONS_QUERY_P2 " DESC"

/* diff array using only one column */
#define PG_STAT_FUNCTIONS_DIFF_COL     3
#define PG_STAT_FUNCTIONS_ORDER_MIN    2
#define PG_STAT_FUNCTIONS_ORDER_MAX    7

#define PG_STAT_STATEMENTS_TIMING_91_QUERY_P1 \
    "SELECT \
        a.rolname AS user, d.datname AS database, \
        date_trunc('seconds', round(sum(p.total_time)) / 1000 * '1 second'::interval) AS t_all_t, \
        round(sum(p.total_time)) AS all_t, \
        sum(p.calls) AS calls, \
        left(md5(d.datname || a.rolname || p.query ), 10) AS queryid, \
        regexp_replace( \
        regexp_replace( \
        regexp_replace( \
        regexp_replace( \
        regexp_replace(p.query, \
            E'\\\\?(::[a-zA-Z_]+)?( *, *\\\\?(::[a-zA-Z_]+)?)+', '?', 'g'), \
            E'\\\\$[0-9]+(::[a-zA-Z_]+)?( *, *\\\\$[0-9]+(::[a-zA-Z_]+)?)*', '$N', 'g'), \
            E'--.*$', '', 'ng'), \
            E'/\\\\*.*?\\\\*\\/', '', 'g'), \
            E'\\\\s+', ' ', 'g') AS query \
    FROM pg_stat_statements p \
    JOIN pg_authid a ON a.oid=p.userid \
    JOIN pg_database d ON d.oid=p.dbid \
    WHERE d.datname != 'postgres' AND calls > 50 \
    GROUP BY a.rolname, d.datname, query ORDER BY "

#define PG_STAT_STATEMENTS_TIMING_QUERY_P1 \
    "SELECT \
        a.rolname AS user, d.datname AS database, \
        date_trunc('seconds', round(sum(p.total_time)) / 1000 * '1 second'::interval) AS t_all_t, \
        date_trunc('seconds', round(sum(p.blk_read_time)) / 1000 * '1 second'::interval) AS t_read_t, \
        date_trunc('seconds', round(sum(p.blk_write_time)) / 1000 * '1 second'::interval) AS t_write_t, \
        date_trunc('seconds', round((sum(p.total_time) - (sum(p.blk_read_time) + sum(p.blk_write_time)))) / 1000 * '1 second'::interval) AS t_cpu_t, \
        round(sum(p.total_time)) AS all_t, \
        round(sum(p.blk_read_time)) AS read_t, \
        round(sum(p.blk_write_time)) AS write_t, \
        round((sum(p.total_time) - (sum(p.blk_read_time) + sum(p.blk_write_time)))) AS cpu_t, \
        sum(p.calls) AS calls, \
        left(md5(d.datname || a.rolname || p.query ), 10) AS queryid, \
        regexp_replace( \
        regexp_replace( \
        regexp_replace( \
        regexp_replace( \
        regexp_replace(p.query, \
            E'\\\\?(::[a-zA-Z_]+)?( *, *\\\\?(::[a-zA-Z_]+)?)+', '?', 'g'), \
            E'\\\\$[0-9]+(::[a-zA-Z_]+)?( *, *\\\\$[0-9]+(::[a-zA-Z_]+)?)*', '$N', 'g'), \
            E'--.*$', '', 'ng'), \
            E'/\\\\*.*?\\\\*\\/', '', 'g'), \
            E'\\\\s+', ' ', 'g') AS query \
    FROM pg_stat_statements p \
    JOIN pg_authid a ON a.oid=p.userid \
    JOIN pg_database d ON d.oid=p.dbid \
    WHERE d.datname != 'postgres' AND calls > 50 \
    GROUP BY a.rolname, d.datname, query ORDER BY "
#define PG_STAT_STATEMENTS_TIMING_QUERY_P2 " DESC"

#define PG_STAT_STATEMENTS_TIMING_ORDER_MIN         2
#define PG_STAT_STATEMENTS_TIMING_ORDER_91_MAX      4
#define PG_STAT_STATEMENTS_TIMING_ORDER_LATEST_MAX  10
#define PG_STAT_STATEMENTS_TIMING_DIFF_91_MIN  3
#define PG_STAT_STATEMENTS_TIMING_DIFF_91_MAX  4
#define PG_STAT_STATEMENTS_TIMING_DIFF_LATEST_MIN  6
#define PG_STAT_STATEMENTS_TIMING_DIFF_LATEST_MAX  10

#define PG_STAT_STATEMENTS_GENERAL_91_QUERY_P1 \
    "SELECT \
        a.rolname AS user, d.datname AS database, \
        sum(p.calls) AS t_calls, sum(p.rows) as t_rows, \
        sum(p.calls) AS calls, sum(p.rows) as rows, \
        left(md5(d.datname || a.rolname || p.query ), 10) AS queryid, \
        regexp_replace( \
        regexp_replace( \
        regexp_replace( \
        regexp_replace( \
        regexp_replace(p.query, \
            E'\\\\?(::[a-zA-Z_]+)?( *, *\\\\?(::[a-zA-Z_]+)?)+', '?', 'g'), \
            E'\\\\$[0-9]+(::[a-zA-Z_]+)?( *, *\\\\$[0-9]+(::[a-zA-Z_]+)?)*', '$N', 'g'), \
            E'--.*$', '', 'ng'), \
            E'/\\\\*.*?\\\\*\\/', '', 'g'), \
            E'\\\\s+', ' ', 'g') AS query \
    FROM pg_stat_statements p \
    JOIN pg_authid a ON a.oid=p.userid \
    JOIN pg_database d ON d.oid=p.dbid \
    WHERE d.datname != 'postgres' AND calls > 50 \
    GROUP BY a.rolname, d.datname, query ORDER BY "

#define PG_STAT_STATEMENTS_GENERAL_QUERY_P1 \
    "SELECT \
        a.rolname AS user, d.datname AS database, \
        sum(p.calls) AS t_calls, sum(p.rows) as t_rows, \
        sum(p.calls) AS calls, sum(p.rows) as rows, \
        left(md5(d.datname || a.rolname || p.query ), 10) AS queryid, \
        regexp_replace( \
        regexp_replace( \
        regexp_replace( \
        regexp_replace( \
        regexp_replace(p.query, \
            E'\\\\?(::[a-zA-Z_]+)?( *, *\\\\?(::[a-zA-Z_]+)?)+', '?', 'g'), \
            E'\\\\$[0-9]+(::[a-zA-Z_]+)?( *, *\\\\$[0-9]+(::[a-zA-Z_]+)?)*', '$N', 'g'), \
            E'--.*$', '', 'ng'), \
            E'/\\\\*.*?\\\\*\\/', '', 'g'), \
            E'\\\\s+', ' ', 'g') AS query \
    FROM pg_stat_statements p \
    JOIN pg_authid a ON a.oid=p.userid \
    JOIN pg_database d ON d.oid=p.dbid \
    WHERE d.datname != 'postgres' AND calls > 50 \
    GROUP BY a.rolname, d.datname, query ORDER BY "
#define PG_STAT_STATEMENTS_GENERAL_QUERY_P2 " DESC"

#define PG_STAT_STATEMENTS_GENERAL_ORDER_MIN    2
#define PG_STAT_STATEMENTS_GENERAL_ORDER_MAX    5
#define PG_STAT_STATEMENTS_GENERAL_DIFF_MIN    4
#define PG_STAT_STATEMENTS_GENERAL_DIFF_MAX    5

#define PG_STAT_STATEMENTS_IO_91_QUERY_P1 \
    "SELECT \
        a.rolname AS user, d.datname AS database, \
        (sum(p.shared_blks_hit) + sum(p.local_blks_hit)) \
            * (SELECT current_setting('block_size')::int / 1024) as t_hits, \
        (sum(p.shared_blks_read) + sum(p.local_blks_read)) \
            * (SELECT current_setting('block_size')::int / 1024) as t_reads, \
        (sum(p.shared_blks_written) + sum(p.local_blks_written)) \
            * (SELECT current_setting('block_size')::int / 1024) as t_written, \
        (sum(p.shared_blks_hit) + sum(p.local_blks_hit)) \
            * (SELECT current_setting('block_size')::int / 1024) as hits, \
        (sum(p.shared_blks_read) + sum(p.local_blks_read)) \
            * (SELECT current_setting('block_size')::int / 1024) as reads, \
        (sum(p.shared_blks_written) + sum(p.local_blks_written)) \
            * (SELECT current_setting('block_size')::int / 1024) as written, \
        sum(p.calls) AS calls, \
        left(md5(d.datname || a.rolname || p.query ), 10) AS queryid, \
        regexp_replace( \
        regexp_replace( \
        regexp_replace( \
        regexp_replace( \
        regexp_replace(p.query, \
            E'\\\\?(::[a-zA-Z_]+)?( *, *\\\\?(::[a-zA-Z_]+)?)+', '?', 'g'), \
            E'\\\\$[0-9]+(::[a-zA-Z_]+)?( *, *\\\\$[0-9]+(::[a-zA-Z_]+)?)*', '$N', 'g'), \
            E'--.*$', '', 'ng'), \
            E'/\\\\*.*?\\\\*\\/', '', 'g'), \
            E'\\\\s+', ' ', 'g') AS query \
    FROM pg_stat_statements p \
    JOIN pg_authid a ON a.oid=p.userid \
    JOIN pg_database d ON d.oid=p.dbid \
    WHERE d.datname != 'postgres' AND calls > 50 \
    GROUP BY a.rolname, d.datname, query ORDER BY "

#define PG_STAT_STATEMENTS_IO_QUERY_P1 \
    "SELECT \
        a.rolname AS user, d.datname AS database, \
        (sum(p.shared_blks_hit) + sum(p.local_blks_hit)) \
            * (SELECT current_setting('block_size')::int / 1024) as t_hits, \
        (sum(p.shared_blks_read) + sum(p.local_blks_read)) \
            * (SELECT current_setting('block_size')::int / 1024) as t_reads, \
        (sum(p.shared_blks_dirtied) + sum(p.local_blks_dirtied)) \
            * (SELECT current_setting('block_size')::int / 1024) as t_dirtied, \
        (sum(p.shared_blks_written) + sum(p.local_blks_written)) \
            * (SELECT current_setting('block_size')::int / 1024) as t_written, \
        (sum(p.shared_blks_hit) + sum(p.local_blks_hit)) \
            * (SELECT current_setting('block_size')::int / 1024) as hits, \
        (sum(p.shared_blks_read) + sum(p.local_blks_read)) \
            * (SELECT current_setting('block_size')::int / 1024) as reads, \
        (sum(p.shared_blks_dirtied) + sum(p.local_blks_dirtied)) \
            * (SELECT current_setting('block_size')::int / 1024) as dirtied, \
        (sum(p.shared_blks_written) + sum(p.local_blks_written)) \
            * (SELECT current_setting('block_size')::int / 1024) as written, \
        sum(p.calls) AS calls, \
        left(md5(d.datname || a.rolname || p.query ), 10) AS queryid, \
        regexp_replace( \
        regexp_replace( \
        regexp_replace( \
        regexp_replace( \
        regexp_replace(p.query, \
            E'\\\\?(::[a-zA-Z_]+)?( *, *\\\\?(::[a-zA-Z_]+)?)+', '?', 'g'), \
            E'\\\\$[0-9]+(::[a-zA-Z_]+)?( *, *\\\\$[0-9]+(::[a-zA-Z_]+)?)*', '$N', 'g'), \
            E'--.*$', '', 'ng'), \
            E'/\\\\*.*?\\\\*\\/', '', 'g'), \
            E'\\\\s+', ' ', 'g') AS query \
    FROM pg_stat_statements p \
    JOIN pg_authid a ON a.oid=p.userid \
    JOIN pg_database d ON d.oid=p.dbid \
    WHERE d.datname != 'postgres' AND calls > 50 \
    GROUP BY a.rolname, d.datname, query ORDER BY "
#define PG_STAT_STATEMENTS_IO_QUERY_P2 " DESC"

#define PG_STAT_STATEMENTS_IO_ORDER_MIN    2
#define PG_STAT_STATEMENTS_IO_ORDER_91_MAX    8
#define PG_STAT_STATEMENTS_IO_ORDER_LATEST_MAX    10
#define PG_STAT_STATEMENTS_IO_DIFF_91_MIN    5
#define PG_STAT_STATEMENTS_IO_DIFF_91_MAX    8
#define PG_STAT_STATEMENTS_IO_DIFF_LATEST_MIN    6
#define PG_STAT_STATEMENTS_IO_DIFF_LATEST_MAX    10

#define PG_STAT_STATEMENTS_TEMP_QUERY_P1 \
    "SELECT \
        a.rolname AS user, d.datname AS database, \
        sum(p.temp_blks_read) \
            * (SELECT current_setting('block_size')::int / 1024) as t_tmp_read, \
        sum(p.temp_blks_written) \
            * (SELECT current_setting('block_size')::int / 1024) as t_tmp_write, \
        sum(p.temp_blks_read) \
            * (SELECT current_setting('block_size')::int / 1024) as tmp_read, \
        sum(p.temp_blks_written) \
            * (SELECT current_setting('block_size')::int / 1024) as tmp_write, \
        sum(p.calls) AS calls, \
        left(md5(d.datname || a.rolname || p.query ), 10) AS queryid, \
        regexp_replace( \
        regexp_replace( \
        regexp_replace( \
        regexp_replace( \
        regexp_replace(p.query, \
            E'\\\\?(::[a-zA-Z_]+)?( *, *\\\\?(::[a-zA-Z_]+)?)+', '?', 'g'), \
            E'\\\\$[0-9]+(::[a-zA-Z_]+)?( *, *\\\\$[0-9]+(::[a-zA-Z_]+)?)*', '$N', 'g'), \
            E'--.*$', '', 'ng'), \
            E'/\\\\*.*?\\\\*\\/', '', 'g'), \
            E'\\\\s+', ' ', 'g') AS query \
    FROM pg_stat_statements p \
    JOIN pg_authid a ON a.oid=p.userid \
    JOIN pg_database d ON d.oid=p.dbid \
    WHERE d.datname != 'postgres' AND calls > 50 \
    GROUP BY a.rolname, d.datname, query ORDER BY "
#define PG_STAT_STATEMENTS_TEMP_QUERY_P2 " DESC"

#define PG_STAT_STATEMENTS_TEMP_ORDER_MIN    2
#define PG_STAT_STATEMENTS_TEMP_ORDER_MAX    6
#define PG_STAT_STATEMENTS_TEMP_DIFF_MIN    4
#define PG_STAT_STATEMENTS_TEMP_DIFF_MAX    6

#define PG_STAT_STATEMENTS_LOCAL_91_QUERY_P1 \
    "SELECT \
        a.rolname AS user, d.datname AS database, \
        (sum(p.local_blks_hit)) * (SELECT current_setting('block_size')::int / 1024) as t_lo_hits, \
        (sum(p.local_blks_read)) * (SELECT current_setting('block_size')::int / 1024) as t_lo_reads, \
        (sum(p.local_blks_written)) * (SELECT current_setting('block_size')::int / 1024) as t_lo_written, \
        (sum(p.local_blks_hit)) * (SELECT current_setting('block_size')::int / 1024) as lo_hits, \
        (sum(p.local_blks_read)) * (SELECT current_setting('block_size')::int / 1024) as lo_reads, \
        (sum(p.local_blks_written)) * (SELECT current_setting('block_size')::int / 1024) as lo_written, \
        sum(p.calls) AS calls, \
        left(md5(d.datname || a.rolname || p.query ), 10) AS queryid, \
        regexp_replace( \
        regexp_replace( \
        regexp_replace( \
        regexp_replace( \
        regexp_replace(p.query, \
            E'\\\\?(::[a-zA-Z_]+)?( *, *\\\\?(::[a-zA-Z_]+)?)+', '?', 'g'), \
            E'\\\\$[0-9]+(::[a-zA-Z_]+)?( *, *\\\\$[0-9]+(::[a-zA-Z_]+)?)*', '$N', 'g'), \
            E'--.*$', '', 'ng'), \
            E'/\\\\*.*?\\\\*\\/', '', 'g'), \
            E'\\\\s+', ' ', 'g') AS query \
    FROM pg_stat_statements p \
    JOIN pg_authid a ON a.oid=p.userid \
    JOIN pg_database d ON d.oid=p.dbid \
    WHERE d.datname != 'postgres' AND calls > 50 \
    GROUP BY a.rolname, d.datname, query ORDER BY "

#define PG_STAT_STATEMENTS_LOCAL_QUERY_P1 \
    "SELECT \
        a.rolname AS user, d.datname AS database, \
        (sum(p.local_blks_hit)) * (SELECT current_setting('block_size')::int / 1024) as t_lo_hits, \
        (sum(p.local_blks_read)) * (SELECT current_setting('block_size')::int / 1024) as t_lo_reads, \
        (sum(p.local_blks_dirtied)) * (SELECT current_setting('block_size')::int / 1024) as t_lo_dirtied, \
        (sum(p.local_blks_written)) * (SELECT current_setting('block_size')::int / 1024) as t_lo_written, \
        (sum(p.local_blks_hit)) * (SELECT current_setting('block_size')::int / 1024) as lo_hits, \
        (sum(p.local_blks_read)) * (SELECT current_setting('block_size')::int / 1024) as lo_reads, \
        (sum(p.local_blks_dirtied)) * (SELECT current_setting('block_size')::int / 1024) as lo_dirtied, \
        (sum(p.local_blks_written)) * (SELECT current_setting('block_size')::int / 1024) as lo_written, \
        sum(p.calls) AS calls, \
        left(md5(d.datname || a.rolname || p.query ), 10) AS queryid, \
        regexp_replace( \
        regexp_replace( \
        regexp_replace( \
        regexp_replace( \
        regexp_replace(p.query, \
            E'\\\\?(::[a-zA-Z_]+)?( *, *\\\\?(::[a-zA-Z_]+)?)+', '?', 'g'), \
            E'\\\\$[0-9]+(::[a-zA-Z_]+)?( *, *\\\\$[0-9]+(::[a-zA-Z_]+)?)*', '$N', 'g'), \
            E'--.*$', '', 'ng'), \
            E'/\\\\*.*?\\\\*\\/', '', 'g'), \
            E'\\\\s+', ' ', 'g') AS query \
    FROM pg_stat_statements p \
    JOIN pg_authid a ON a.oid=p.userid \
    JOIN pg_database d ON d.oid=p.dbid \
    WHERE d.datname != 'postgres' AND calls > 50 \
    GROUP BY a.rolname, d.datname, query ORDER BY "
#define PG_STAT_STATEMENTS_LOCAL_QUERY_P2 " DESC"

#define PG_STAT_STATEMENTS_LOCAL_ORDER_MIN    2
#define PG_STAT_STATEMENTS_LOCAL_ORDER_91_MAX    8
#define PG_STAT_STATEMENTS_LOCAL_ORDER_LATEST_MAX    10
#define PG_STAT_STATEMENTS_LOCAL_DIFF_91_MIN    5
#define PG_STAT_STATEMENTS_LOCAL_DIFF_91_MAX    8
#define PG_STAT_STATEMENTS_LOCAL_DIFF_LATEST_MIN    6
#define PG_STAT_STATEMENTS_LOCAL_DIFF_LATEST_MAX    10

/* other queries */
/* don't log our queries */
#define PG_SUPPRESS_LOG_QUERY "SET log_min_duration_statement TO 10000"

/* set work_mem for pg_stat_statements queries */
#define PG_INCREASE_WORK_MEM_QUERY "SET work_mem TO '32MB'"

/* check pg_is_in_recovery() */
#define PG_IS_IN_RECOVERY_QUERY "SELECT pg_is_in_recovery()"

/* get full config query */
#define PG_SETTINGS_QUERY "SELECT name, setting, unit, category FROM pg_settings ORDER BY 4"

/* get one setting query */
#define PG_SETTINGS_SINGLE_OPT_P1 "SELECT name, setting FROM pg_settings WHERE name = '"
#define PG_SETTINGS_SINGLE_OPT_P2 "'"

/* reload postgres */
#define PG_RELOAD_CONF_QUERY "SELECT pg_reload_conf()"

/* cancel/terminate backend */
#define PG_CANCEL_BACKEND_P1 "SELECT pg_cancel_backend("
#define PG_CANCEL_BACKEND_P2 ")"
#define PG_TERM_BACKEND_P1 "SELECT pg_terminate_backend("
#define PG_TERM_BACKEND_P2 ")"

/* cancel/terminate group of backends */
#define PG_SIG_GROUP_BACKEND_P1 "SELECT pg_"
#define PG_SIG_GROUP_BACKEND_P2 "_backend(pid) FROM pg_stat_activity WHERE "
#define PG_SIG_GROUP_BACKEND_P3 " AND ((clock_timestamp() - xact_start) > '"
#define PG_SIG_GROUP_BACKEND_P4 "'::interval OR (clock_timestamp() - query_start) > '"
#define PG_SIG_GROUP_BACKEND_P5 "'::interval) AND pid <> pg_backend_pid()"

/* reset statistics query */
#define PG_STAT_RESET_QUERY "SELECT pg_stat_reset(), pg_stat_statements_reset()"

/* postmaster uptime query */
#define PG_UPTIME_QUERY "SELECT date_trunc('seconds', now() - pg_postmaster_start_time())"

/* start end exit functions */
void sig_handler(int signo);
void init_signal_handlers(void);
void init_screens(struct screen_s *screens[]);
struct args_s * init_args_mem(void);
void init_args_struct(struct args_s *args);
void check_portnum(const char * portnum);
void arg_parse(int argc, char *argv[], struct args_s *args);
void create_initial_conn(struct args_s * args, struct screen_s * screens[]);
unsigned int create_pgcenterrc_conn(struct args_s * args, struct screen_s * screens[], unsigned int pos);
void exit_prog(struct screen_s * screens[], PGconn * conns[]);

/* connections and queries unctions */
char * password_prompt(const char *prompt, unsigned int pw_maxlen, bool echo);
void reconnect_if_failed(WINDOW * window, PGconn * conn, struct screen_s * screen, bool *reconnected);
void prepare_conninfo(struct screen_s * screens[]);
void open_connections(struct screen_s * screens[], PGconn * conns[]);
void close_connections(struct screen_s * screens[], PGconn * conns[]);
void prepare_query(struct screen_s * screen, char * query);
PGresult * do_query(PGconn * conn, const char * query, char errmsg[]);

/* system resources functions */
void get_time(char * strtime);
float * get_loadavg();
void print_loadavg(WINDOW * window);
void init_stats(struct cpu_s *st_cpu[], struct mem_s **st_mem_short);
void init_iostats(struct iodata_s *c_ios[], struct iodata_s *p_ios[], unsigned int bdev);
void free_iostats(struct iodata_s *c_ios[], struct iodata_s *p_ios[], unsigned int bdev);
void init_nicdata(struct nicdata_s *c_nicdata[], struct nicdata_s *p_nicdata[], unsigned int idev);
void free_nicdata(struct nicdata_s *c_nicdata[], struct nicdata_s *p_nicdata[], unsigned int idev);
void get_HZ(void);
void read_uptime(unsigned long long *uptime);
void read_cpu_stat(struct cpu_s *st_cpu, unsigned int nbr,
        unsigned long long *uptime, unsigned long long *uptime0);
unsigned long long get_interval(unsigned long long prev_uptime,
        unsigned long long curr_uptime);
double ll_sp_value(unsigned long long value1, unsigned long long value2,
        unsigned long long itv);
void write_cpu_stat_raw(WINDOW * window, struct cpu_s *st_cpu[],
        unsigned int curr, unsigned long long itv);
void print_iostat(WINDOW * window, WINDOW * w_cmd, struct iodata_s *c_ios[],
        struct iodata_s *p_ios[], unsigned int bdev, bool * repaint);
void get_speed_duplex(struct nicdata_s * nicdata);

/* print screen functions */
void print_title(WINDOW * window, const char * progname);
void print_cpu_usage(WINDOW * window, struct cpu_s *st_cpu[]);
void print_conninfo(WINDOW * window, PGconn *conn, unsigned int console_no);
void print_pg_general(WINDOW * window, struct screen_s * screen, PGconn * conn);
void print_postgres_activity(WINDOW * window, PGconn * conn);
void print_vacuum_info(WINDOW * window, struct screen_s * screen, PGconn * conn);
void print_pgss_info(WINDOW * window, PGconn * conn, unsigned long interval);
void print_data(WINDOW *window, PGresult *res, char ***arr, 
        unsigned int n_rows, unsigned int n_cols, struct screen_s * screen);
void print_log(WINDOW * window, WINDOW * w_cmd, struct screen_s * screen, PGconn * conn);

/* data arrays functions */
char *** init_array(char ***arr, unsigned int n_rows, unsigned int n_cols);
char *** free_array(char ***arr, unsigned int n_rows, unsigned int n_cols);
void pgrescpy(char ***arr, PGresult *res, unsigned int n_rows, unsigned int n_cols);
void diff_arrays(char ***p_arr, char ***c_arr, char ***res_arr, struct screen_s * screen, 
        unsigned int n_rows, unsigned int n_cols, unsigned long interval);
void sort_array(char ***res_arr, unsigned int n_rows, unsigned int n_cols, struct screen_s * screen);

/* key-press functions */
unsigned int switch_conn(WINDOW * window, struct screen_s * screens[],
        unsigned int ch, unsigned int console_index, unsigned int console_no, PGresult * res, bool * first_iter);
void change_sort_order(struct screen_s * screen, bool increment, bool * first_iter);
void change_sort_order_direction(struct screen_s * screen, bool * first_iter);
void change_min_age(WINDOW * window, struct screen_s * screen, PGresult *res, bool *first_iter);
unsigned int add_connection(WINDOW * window, struct screen_s * screens[],
        PGconn * conns[], unsigned int console_index);
unsigned int close_connection(WINDOW * window, struct screen_s * screens[],
        PGconn * conns[], unsigned int console_index, bool *first_iter);
void write_pgcenterrc(WINDOW * window, struct screen_s * screens[], PGconn * conns[], struct args_s * args);
void show_config(WINDOW * window, PGconn * conn);
void reload_conf(WINDOW * window, PGconn * conn);
void edit_config(WINDOW * window, struct screen_s * screen, PGconn * conn, const char * config_file_guc);
void edit_config_menu(WINDOW * w_cmd, WINDOW * w_dba, struct screen_s * screen, PGconn * conn, bool *first_iter);
void pgss_switch(WINDOW * w_cmd, struct screen_s * screen, PGresult * p_res, bool *first_iter);
void pgss_menu(WINDOW * w_cmd, WINDOW * w_dba, struct screen_s * screen, bool *first_iter);
void signal_single_backend(WINDOW * window, struct screen_s *screen, PGconn * conn, bool do_terminate);
void get_statemask(WINDOW * window, struct screen_s * screen);
void set_statemask(WINDOW * window, struct screen_s * screen);
void signal_group_backend(WINDOW * window, struct screen_s *screen, PGconn * conn, bool do_terminate);
void start_psql(WINDOW * window, struct screen_s * screen);
unsigned long change_refresh(WINDOW * window, unsigned long interval);
void do_noop(WINDOW * window, unsigned long interval);
void system_view_toggle(WINDOW * window, struct screen_s * screen, bool * first_iter);
void log_process(WINDOW * window, WINDOW ** w_log, struct screen_s * screen, PGconn * conn, unsigned int subscreen);
void show_full_log(WINDOW * window, struct screen_s * screen, PGconn * conn);
void get_query_by_id(WINDOW * window, struct screen_s * screen, PGconn * conn);
void pg_stat_reset(WINDOW * window, PGconn * conn, bool * reseted);
void switch_context(WINDOW * window, struct screen_s * screen,
        enum context context, PGresult * res, bool * first_iter);

/* functions routines */
double min(double d1, double d2);
double max(double d1, double d2);
bool key_is_pressed(void);
void strrpl(char * o_string, const char * s_string, const char * r_string, unsigned int buf_size);
int check_string(const char * string);
struct colAttrs * init_colattrs(unsigned int n_cols);
void calculate_width(struct colAttrs *columns, PGresult *res, char ***arr, unsigned int n_rows, unsigned int n_cols);
void cmd_readline(WINDOW *window, const char * msg, unsigned int pos, bool * with_esc, char * str, unsigned int len, bool echoing);
void clear_screen_connopts(struct screen_s * screens[], unsigned int i);
void shift_screens(struct screen_s * screens[], PGconn * conns[], unsigned int i);
bool check_pg_listen_addr(struct screen_s * screen, PGconn * conn);
void get_conf_value(PGconn * conn, const char * config_option_name, char * config_option_value);
void get_pg_special(PGconn * conn, struct screen_s * screen);
void get_logfile_path(char * path, PGconn * conn);
void get_pg_uptime(PGconn * conn, char * uptime);
unsigned int count_block_devices(void);
unsigned int count_nic_devices(void);
void replace_iodata(struct iodata_s *curr[], struct iodata_s *prev[], unsigned int bdev);
void replace_nicdata(struct nicdata_s *curr[], struct nicdata_s *prev[], unsigned int idev);
ITEM ** init_menuitems(unsigned int n_choices);

/* color functions */
void init_colors(unsigned int * ws_color, unsigned int * wc_color, unsigned int * wa_color, unsigned int * wl_color);
void draw_color_help(WINDOW * w, unsigned int * ws_color, unsigned int * wc_color,
        unsigned int * wa_color, unsigned int * wl_color, unsigned int target, unsigned int * target_color);
void change_colors(unsigned int * ws_color, unsigned int * wc_color, unsigned int * wa_color, unsigned int * wl_color);

/* help functions */
void print_help_screen(bool * first_iter);
void print_usage(void);

#endif /* PGCENTER_H */
