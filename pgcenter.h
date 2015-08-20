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

/* sizes, limits and defaults */
#define BUFFERSIZE_S        16
#define BUFFERSIZE_M        256
#define BUFFERSIZE          4096
#define MAX_SCREEN          8
#define TOTAL_CONTEXTS      10
#define INVALID_ORDER_KEY   99
#define PG_STAT_ACTIVITY_MIN_AGE_DEFAULT "00:00:10.0"

#define LOADAVG_FILE            "/proc/loadavg"
#define STAT_FILE               "/proc/stat"
#define UPTIME_FILE             "/proc/uptime"
#define PGCENTERRC_FILE         ".pgcenterrc"
#define PG_RECOVERY_CONF_FILE   "recovery.conf"

/* 
 * GUC 
 * This definitions used in edit_config() for edititing postgres config files. 
 * But here we have one issue - we want edit recovery.conf, but GUC for 
 * recovery.conf doesn't exists. For this reason we use data_directory GUC.
 * Details see in get_conf_value() function.
 */
#define GUC_CONFIG_FILE         "config_file"
#define GUC_HBA_FILE            "hba_file"
#define GUC_IDENT_FILE          "ident_file"
#define GUC_DATA_DIRECTORY       "data_directory"

#define PGCENTERRC_READ_OK  0
#define PGCENTERRC_READ_ERR 1

/* connections defaults */
#define DEFAULT_HOST        "/tmp"
#define DEFAULT_PORT        "5432"
#define DEFAULT_USER        "postgres"

/* others defaults */
#define DEFAULT_PAGER       "less"
#define DEFAULT_EDITOR      "vi"
#define DEFAULT_PSQL        "psql"
#define DEFAULT_INTERVAL    1000000
#define INTERVAL_STEP       200000
#define DEFAULT_VIEW_TYPE   "user"
#define FULL_VIEW_TYPE      "all"

#define HZ                  hz
unsigned int hz;

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

#define GROUP_ACTIVE        1 << 0
#define GROUP_IDLE          1 << 1
#define GROUP_IDLE_IN_XACT  1 << 2
#define GROUP_WAITING       1 << 3
#define GROUP_OTHER         1 << 4

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
    pg_stat_statements_general
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
    char connfile[BUFFERSIZE];
    char host[BUFFERSIZE];
    char port[BUFFERSIZE];
    char user[BUFFERSIZE];
    char dbname[BUFFERSIZE];
    bool need_passwd;
};

#define ARGS_SIZE (sizeof(struct args_s))

/* struct which define connection options */
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
    char log_path[PATH_MAX];
    int log_fd;
    enum context current_context;
    char pg_stat_activity_min_age[BUFFERSIZE_S];
    struct context_s context_list[TOTAL_CONTEXTS];
    int signal_options;
    bool pg_stat_sys;
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

/* struct for column widths */
struct colAttrs {
    char name[40];
    int width;
};

/* PostgreSQL answers, see PQresultStatus() at http://www.postgresql.org/docs/9.4/static/libpq-exec.html */
#define PG_CMD_OK       PGRES_COMMAND_OK
#define PG_TUP_OK       PGRES_TUPLES_OK
#define PG_FATAL_ERR    PGRES_FATAL_ERROR

/* sysstat screen queries */
#define PG_STAT_ACTIVITY_COUNT_TOTAL_QUERY \
        "SELECT count(*) FROM pg_stat_activity"
#define PG_STAT_ACTIVITY_COUNT_IDLE_QUERY \
        "SELECT count(*) FROM pg_stat_activity WHERE state = 'idle'"
#define PG_STAT_ACTIVITY_COUNT_IDLE_IN_T_QUERY \
        "SELECT count(*) FROM pg_stat_activity WHERE state IN ('idle in transaction', 'idle in transaction (aborted)')"
#define PG_STAT_ACTIVITY_COUNT_ACTIVE_QUERY \
        "SELECT count(*) FROM pg_stat_activity WHERE state = 'active'"
#define PG_STAT_ACTIVITY_COUNT_WAITING_QUERY \
        "SELECT count(*) FROM pg_stat_activity WHERE waiting"
#define PG_STAT_ACTIVITY_COUNT_OTHERS_QUERY \
        "SELECT count(*) FROM pg_stat_activity WHERE state IN ('fastpath function call','disabled')"
#define PG_STAT_ACTIVITY_AV_COUNT_QUERY \
        "SELECT count(*) FROM pg_stat_activity WHERE query ~* '^autovacuum:' AND pid <> pg_backend_pid()"
#define PG_STAT_ACTIVITY_AVW_COUNT_QUERY \
        "SELECT count(*) FROM pg_stat_activity WHERE query ~* '^autovacuum:.*to prevent wraparound' AND pid <> pg_backend_pid()"
#define PG_STAT_ACTIVITY_AV_LONGEST_QUERY \
        "SELECT coalesce(date_trunc('seconds', max(now() - xact_start)), '00:00:00') \
        FROM pg_stat_activity WHERE query ~* '^autovacuum:' AND pid <> pg_backend_pid()"
#define PG_STAT_STATEMENTS_SYS_QUERY \
        "SELECT (sum(total_time) / sum(calls))::numeric(6,3) AS avg_query, sum(calls) AS total_calls FROM pg_stat_statements"
#define PG_STAT_ACTIVITY_SYS_QUERY \
        "SELECT coalesce(date_trunc('seconds', max(now() - xact_start)), '00:00:00') FROM pg_stat_activity"

/* context queries */
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

#define PG_STAT_DATABASE_ORDER_MIN    1
#define PG_STAT_DATABASE_ORDER_MAX    15

#define PG_STAT_REPLICATION_QUERY \
    "SELECT \
        client_addr as client, usename as user, application_name as name, \
        state, sync_state as mode, \
        (pg_xlog_location_diff(sent_location, '0/0') / 1024)::bigint as sent, \
        (pg_xlog_location_diff(write_location, '0/0') / 1024)::bigint as write, \
        (pg_xlog_location_diff(flush_location, '0/0') / 1024)::bigint as flush, \
        (pg_xlog_location_diff(replay_location, '0/0') / 1024)::bigint as replay, \
        (pg_xlog_location_diff(sent_location,replay_location) / 1024)::bigint as lag \
    FROM pg_stat_replication \
    ORDER BY client_addr"

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

#define PG_STAT_ACTIVITY_LONG_QUERY_P1 \
    "SELECT \
        pid, client_addr AS cl_addr, client_port AS cl_port, \
        datname, usename, state, waiting, \
        date_trunc('seconds', clock_timestamp() - xact_start) AS xact_age, \
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


#define PG_STAT_STATEMENTS_TIMING_QUERY_P1 \
    "SELECT \
        a.rolname AS user, d.datname AS database, \
        date_trunc('seconds', round(sum(p.total_time)) / 1000 * '1 second'::interval) AS total_t, \
        date_trunc('seconds', round(sum(p.blk_read_time)) / 1000 * '1 second'::interval) AS read_t, \
        date_trunc('seconds', round(sum(p.blk_write_time)) / 1000 * '1 second'::interval) AS write_t, \
        date_trunc('seconds', round((sum(p.total_time) - (sum(p.blk_read_time) + sum(p.blk_write_time)))) / 1000 * '1 second'::interval) AS cpu_t, \
        regexp_replace( \
        regexp_replace( \
        regexp_replace( \
        regexp_replace(p.query, \
            E'\\\\?(::[a-zA-Z_]+)?( *, *\\\\?(::[a-zA-Z_]+)?)+', '?', 'g'), \
            E'\\\\$[0-9]+(::[a-zA-Z_]+)?( *, *\\\\$[0-9]+(::[a-zA-Z_]+)?)*', '$N', 'g'), \
            E'--.*$', '', 'ng'), \
            E'/\\\\*.*?\\\\*\\/', '', 'g') AS query \
    FROM pg_stat_statements p \
    JOIN pg_authid a ON a.oid=p.userid \
    JOIN pg_database d ON d.oid=p.dbid \
    WHERE d.datname != 'postgres' AND calls > 50 \
    GROUP BY a.rolname, d.datname, query ORDER BY "
#define PG_STAT_STATEMENTS_TIMING_QUERY_P2 " DESC"

#define PG_STAT_STATEMENTS_TIMING_ORDER_MIN    2
#define PG_STAT_STATEMENTS_TIMING_ORDER_MAX    5

#define PG_STAT_STATEMENTS_GENERAL_QUERY_P1 \
    "SELECT \
        a.rolname AS user, d.datname AS database, \
        sum(p.calls) AS total_calls, sum(p.rows) as total_rows, \
        sum(p.calls) AS calls, sum(p.rows) as rows, \
        regexp_replace( \
        regexp_replace( \
        regexp_replace( \
        regexp_replace(p.query, \
            E'\\\\?(::[a-zA-Z_]+)?( *, *\\\\?(::[a-zA-Z_]+)?)+', '?', 'g'), \
            E'\\\\$[0-9]+(::[a-zA-Z_]+)?( *, *\\\\$[0-9]+(::[a-zA-Z_]+)?)*', '$N', 'g'), \
            E'--.*$', '', 'ng'), \
            E'/\\\\*.*?\\\\*\\/', '', 'g') AS query \
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

/* other queries */
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

void print_usage(void);
void sig_handler(int signo);
void init_signal_handlers(void);
int key_is_pressed(void);
void strrpl(char * o_string, char * s_string, char * r_string);
void init_screens(struct screen_s *screens[]);
char * password_prompt(const char *prompt, int maxlen, bool echo);
void init_args_struct(struct args_s * args);
void arg_parse(int argc, char *argv[], struct args_s *args);
void create_initial_conn(struct args_s * args, struct screen_s * screens[]);
int create_pgcenterrc_conn(struct args_s * args, struct screen_s * screens[], const int pos);
void reconnect_if_failed(WINDOW * window, PGconn * conn, bool *reconnected);
void prepare_conninfo(struct screen_s * screens[]);
void open_connections(struct screen_s * screens[], PGconn * conns[]);
void exit_prog(struct screen_s * screens[], PGconn * conns[]);
void close_connections(struct screen_s * screens[], PGconn * conns[]);
void prepare_query(struct screen_s * screen, char * query);
PGresult * do_query(PGconn * conn, char * query, char *errmsg);
void get_time(char * strtime);
void print_title(WINDOW * window, char * progname);
float get_loadavg(int m);
void print_loadavg(WINDOW * window);
void print_conninfo(WINDOW * window, struct screen_s * screen, PGconn *conn, int console_no);
void print_postgres_activity(WINDOW * window, PGconn * conn);
void print_pgstatstmt_info(WINDOW * window, PGconn * conn, long int interval);
void init_stats(struct stats_cpu_struct *st_cpu[]);
void get_HZ(void);
void read_uptime(unsigned long long *uptime);
void read_cpu_stat(struct stats_cpu_struct *st_cpu, int nbr,
        unsigned long long *uptime, unsigned long long *uptime0);
unsigned long long get_interval(unsigned long long prev_uptime,
        unsigned long long curr_uptime);
double ll_sp_value(unsigned long long value1, unsigned long long value2,
        unsigned long long itv);
void write_cpu_stat_raw(WINDOW * window, struct stats_cpu_struct *st_cpu[],
        int curr, unsigned long long itv);
void print_cpu_usage(WINDOW * window, struct stats_cpu_struct *st_cpu[]);
void calculate_width(struct colAttrs *columns, PGresult *res, char ***arr, int n_rows, int n_cols);
void print_autovac_info(WINDOW * window, PGconn * conn);
int switch_conn(WINDOW * window, struct screen_s * screens[],
        int ch, int console_index, int console_no);
char *** init_array(char ***arr, int n_rows, int n_cols);
char *** free_array(char ***arr, int n_rows, int n_cols);
void pgrescpy(char ***arr, PGresult *res, int n_rows, int n_cols);
void diff_arrays(char ***p_arr, char ***c_arr, char ***res_arr, 
        enum context context, int n_rows, int n_cols, long int interval);
void sort_array(char ***res_arr, int n_rows, int n_cols, struct screen_s * screen);
void print_data(WINDOW *window, PGresult *res, char ***arr, 
        int n_rows, int n_cols, struct screen_s * screen);
void change_sort_order(struct screen_s * screen, bool increment, bool * first_iter);
void change_sort_order_direction(struct screen_s * screen, bool * first_iter);
void cmd_readline(WINDOW *window, int pos, bool * with_esc, char * str);
void change_min_age(WINDOW * window, struct screen_s * screen, PGresult *res, bool *first_iter);
void clear_screen_connopts(struct screen_s * screens[], int i);
void shift_screens(struct screen_s * screens[], PGconn * conns[], int i);
int add_connection(WINDOW * window, struct screen_s * screens[],
        PGconn * conns[], int console_index);
int close_connection(WINDOW * window, struct screen_s * screens[],
        PGconn * conns[], int console_index);
void write_pgcenterrc(WINDOW * window, struct screen_s * screens[], struct args_s * args);
void show_config(WINDOW * window, PGconn * conn);
void reload_conf(WINDOW * window, PGconn * conn);
bool check_pg_listen_addr(struct screen_s * screen);
void get_conf_value(WINDOW * window, PGconn * conn,
        char * config_option_name, char * config_option_value);
void edit_config(WINDOW * window, struct screen_s * screen, PGconn * conn, char * guc);
void signal_single_backend(WINDOW * window, struct screen_s *screen, PGconn * conn, bool do_terminate);
void get_statemask(WINDOW * window, struct screen_s * screen);
void set_statemask(WINDOW * window, struct screen_s * screen);
void signal_group_backend(WINDOW * window, struct screen_s *screen, PGconn * conn, bool do_terminate);
void start_psql(WINDOW * window, struct screen_s * screen);
long int change_refresh(WINDOW * window, long int interval);
void do_noop(WINDOW * window, long int interval);
void system_view_toggle(WINDOW * window, struct screen_s * screen, bool * first_iter);
void get_logfile_path(char * path, PGconn * conn);
void log_process(WINDOW * window, WINDOW ** w_log, struct screen_s * screen, PGconn * conn);
void print_log(WINDOW * window, WINDOW * w_cmd, struct screen_s * screen, PGconn * conn);
void show_full_log(WINDOW * window, struct screen_s * screen, PGconn * conn);
void pg_stat_reset(WINDOW * window, PGconn * conn, bool * reseted);
void init_colors(int * ws_color, int * wc_color, int * wa_color, int * wl_color);
void draw_color_help(WINDOW * w, int * ws_color, int * wc_color,
        int * wa_color, int * wl_color, int target, int * target_color);
void change_colors(int * ws_color, int * wc_color, int * wa_color, int * wl_color);
void switch_context(WINDOW * window, struct screen_s * screen,
        enum context context, PGresult * res, bool * first_iter);
void print_help_screen(void);
#endif
