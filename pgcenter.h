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
#define BUFFERSIZE          4096
#define MAX_CONSOLE         8

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
    pg_stat_replication
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
    enum context query_context;
    int order_key;
    bool order_desc;
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

/*
 * Macros used to display statistics values.
 * NB: Define SP_VALUE() to normalize to %;
 */
#define SP_VALUE(m,n,p) (((double) ((n) - (m))) / (p) * 100)

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
    FROM pg_stat_database  \
    ORDER BY datname"

#define PG_STAT_DATABASE_QUERY_ORDER_MIN    1
#define PG_STAT_DATABASE_QUERY_ORDER_MAX    14

#define PG_STAT_REPLICATION_QUERY ""

#endif
