/*
 * pgcenter: administrative console for PostgreSQL.
 * (C) 2015 by Alexey V. Lesovsky (lesovsky <at> gmail.com)
 */

#ifndef __PGCENTER_H__
#define __PGCENTER_H__

#define PROGRAM_NAME        "pgcenter"
#define PROGRAM_VERSION     0.1
#define PROGRAM_RELEASE     1
#define PROGRAM_AUTHORS_CONTACTS    "<lesovsky@gmail.com>"

/* sizes and limits */
#define BUFFERSIZE_S        16
#define BUFFERSIZE          4096
#define MAX_CONSOLE         8
#define TOTAL_CONTEXTS      7
#define INVALID_ORDER_KEY   99
#define PG_STAT_ACTIVITY_MIN_AGE_DEFAULT "00:00:10.0"

#define LOADAVG_FILE        "/proc/loadavg"
#define STAT_FILE           "/proc/stat"
#define UPTIME_FILE         "/proc/uptime"
#define PGCENTERRC_FILE     ".pgcenterrc"

#define PGCENTERRC_READ_OK  0
#define PGCENTERRC_READ_ERR 1

/* connectins defaults */
#define DEFAULT_HOST        "/tmp"
#define DEFAULT_PORT        "5432"
#define DEFAULT_USER        "postgres"

#define HZ                  hz
unsigned int hz;

/* enum for query context */
enum context
{
    pg_stat_database,
    pg_stat_replication,
    pg_stat_user_tables,
    pg_stat_user_indexes,
    pg_statio_user_tables,
    pg_tables_size,
    pg_stat_activity_long
};

#define DEFAULT_QUERY_CONTEXT   pg_stat_database

/* struct for context list used in screen */
struct context_s
{
    enum context context;
    int order_key;
    bool order_desc;
};

/* Struct which define connection options */
struct screen_s
{
    int screen;
    bool conn_used;
    char host[BUFFERSIZE];
    char port[BUFFERSIZE];
    char user[BUFFERSIZE];
    char dbname[BUFFERSIZE];
    char password[BUFFERSIZE];
    char conninfo[BUFFERSIZE];
    bool log_opened;
    FILE *log;
    enum context current_context;
    char pg_stat_activity_min_age[BUFFERSIZE_S];
    struct context_s context_list[TOTAL_CONTEXTS];
};

#define SCREEN_SIZE (sizeof(struct screen_s))

/* struct which used for cpu statistic */
struct stats_cpu_struct {
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

#define STATS_CPU_SIZE (sizeof(struct stats_cpu_struct))

/*
 * Macros used to display statistics values.
 * NB: Define SP_VALUE() to normalize to %;
 */
#define SP_VALUE(m,n,p) (((double) ((n) - (m))) / (p) * 100)

/* enum for password purpose */
enum trivalue
{
    TRI_DEFAULT,
    TRI_NO,
    TRI_YES
};

/* struct for column widths */
struct colAttrs {
    char name[40];
    int width;
};

/* PostgreSQL answers */
#define PG_CMD_OK PGRES_COMMAND_OK
#define PG_TUP_OK PGRES_TUPLES_OK

#define PG_STAT_DATABASE_QUERY \
    "SELECT \
        datname, \
        xact_commit as commit, xact_rollback as rollback, \
        blks_read as reads, blks_hit as hits, \
        tup_returned as returned, tup_fetched as fetched, \
        tup_inserted as inserts, tup_updated as updates, tup_deleted as deletes, \
        conflicts, \
        temp_files as tmp_files, temp_bytes as tmp_bytes, \
        blk_read_time as read_t, blk_write_time as write_t \
    FROM pg_stat_database \
    ORDER BY datname"

#define PG_STAT_DATABASE_ORDER_MIN    1
#define PG_STAT_DATABASE_ORDER_MAX    14

#define PG_STAT_REPLICATION_QUERY \
    "SELECT \
        client_addr as client, application_name as name, \
        state, sync_state as mode, \
        (pg_xlog_location_diff(sent_location, '0/0') / 1024)::int as \"sent KB/s\", \
        (pg_xlog_location_diff(write_location, '0/0') / 1024)::int as \"write KB/s\", \
        (pg_xlog_location_diff(flush_location, '0/0') / 1024)::int as \"flush KB/s\", \
        (pg_xlog_location_diff(replay_location, '0/0') / 1024)::int as \"replay KB/s\", \
        (pg_xlog_location_diff(sent_location,replay_location) / 1024)::int as \"lag KB/s\" \
    FROM pg_stat_replication \
    ORDER BY client_addr"

#define PG_STAT_REPLICATION_ORDER_MIN 4
#define PG_STAT_REPLICATION_ORDER_MAX 7

#define PG_STAT_USER_TABLES_QUERY \
    "SELECT \
        schemaname || '.' || relname as relation, \
        seq_scan, seq_tup_read, idx_scan, idx_tup_fetch, \
        n_tup_ins as inserts, n_tup_upd as updates, \
        n_tup_del as deletes, n_tup_hot_upd as hot_updates, \
        n_live_tup as live, n_dead_tup as dead \
    FROM pg_stat_user_tables \
    ORDER BY 1"

#define PG_STAT_USER_TABLES_ORDER_MIN 1
#define PG_STAT_USER_TABLES_ORDER_MAX 10

#define PG_STATIO_USER_TABLES_QUERY \
    "SELECT \
        schemaname ||'.'|| relname as relation, \
        heap_blks_read, heap_blks_hit, idx_blks_read, idx_blks_hit, \
        toast_blks_read, toast_blks_hit, tidx_blks_read, tidx_blks_hit \
    FROM pg_statio_user_tables \
    ORDER BY 1"

#define PG_STATIO_USER_TABLES_ORDER_MIN 1
#define PG_STATIO_USER_TABLES_ORDER_MAX 8

#define PG_STAT_USER_INDEXES_QUERY \
    "SELECT \
        s.schemaname ||'.'|| s.relname as relation, s.indexrelname as index, \
        s.idx_scan, s.idx_tup_read, s.idx_tup_fetch, \
        i.idx_blks_read, i.idx_blks_hit \
    FROM \
        pg_stat_user_indexes s, \
        pg_statio_user_indexes i \
    WHERE s.relid = i.relid \
    ORDER BY 1"

#define PG_STAT_USER_INDEXES_ORDER_MIN 2
#define PG_STAT_USER_INDEXES_ORDER_MAX 6

#define PG_TABLES_SIZE_QUERY \
    "SELECT \
        schemaname ||'.'|| relname as relation, \
        pg_total_relation_size(relname::regclass) / 1024 as \"total size, KB/s\", \
        pg_relation_size(relname::regclass) / 1024 as \"size w/o indexes, KB/s\", \
        (pg_total_relation_size(relname::regclass) / 1024) - (pg_relation_size(relname::regclass) / 1024) as \"indexes, KB/s\", \
        pg_total_relation_size(relname::regclass) / 1024 as \"total changes, KB/s\", \
        pg_relation_size(relname::regclass) / 1024 as \"changes w/o indexes, KB/s\", \
        (pg_total_relation_size(relname::regclass) / 1024) - (pg_relation_size(relname::regclass) / 1024) as \"changes indexes, KB/s\" \
        FROM pg_stat_user_tables \
        ORDER BY 1"

#define PG_TABLES_SIZE_ORDER_MIN 4
#define PG_TABLES_SIZE_ORDER_MAX 6

#define PG_STAT_ACTIVITY_LONG_QUERY_P1 \
    "SELECT \
        pid, client_addr as cl_addr, client_port as cl_port, \
        datname, usename, state, waiting, \
        date_trunc('seconds', clock_timestamp() - xact_start) AS txn_age, \
        date_trunc('seconds', clock_timestamp() - query_start) AS query_age, \
        date_trunc('seconds', clock_timestamp() - state_change) AS change_age, \
        query \
    FROM pg_stat_activity \
    WHERE ((clock_timestamp() - xact_start) > '"
#define PG_STAT_ACTIVITY_LONG_QUERY_P2 \
    "'::interval OR (clock_timestamp() - query_start) > '"
#define PG_STAT_ACTIVITY_LONG_QUERY_P3 \
    "'::interval) AND state <> 'idle' AND pid <> pg_backend_pid() \
    ORDER BY COALESCE(xact_start, query_start)"

#define PG_STAT_ACTIVITY_LONG_ORDER_MIN INVALID_ORDER_KEY
#define PG_STAT_ACTIVITY_LONG_ORDER_MAX INVALID_ORDER_KEY

#endif
