/*
 ****************************************************************************
 * common.h
 *      widely used common definitions and macros.
 * 
 * (C) 2016 by Alexey V. Lesovsky (lesovsky <at> gmail.com)
 * 
 ****************************************************************************
 */
#ifndef __COMMON_H__
#define __COMMON_H__

#define _GNU_SOURCE

#include <ctype.h>      /* isdigit, isalnum */
#include <errno.h>
#include <fcntl.h>
#include <getopt.h>
#include <ifaddrs.h>
#include <limits.h>
#include <linux/types.h>
#include <ncurses.h>
#include <netdb.h>
#include <signal.h>
#include <stdbool.h>
#include <stdio.h>
#include <stdlib.h>     /* malloc, free */
#include <string.h>     /* memset */
#include <stdarg.h>     /* va_start, va_end */
#include <termios.h>    /* tcsetattr */
#include <unistd.h>     /* sysconf */
#include "libpq-fe.h"

#define PROGRAM_NAME        "pgcenter"
#define PROGRAM_VERSION     0.3
#define PROGRAM_RELEASE     0
#define PROGRAM_ISSUES_URL  "https://github.com/lesovsky/pgcenter/issues"

/* sizes, limits and defaults */
#define XS_BUF_LEN	16
#define S_BUF_LEN	64
#define M_BUF_LEN	128
#define L_BUF_LEN	256
#define X_BUF_LEN	512
#define XL_BUF_LEN	1024
#define XXL_BUF_LEN	4096
#define XXXL_BUF_LEN	BUFSIZ

#define ERRSIZE             128
#define MAX_TABS            8
#define MAX_COLS            20              /* filtering purposes */

/* others defaults */
#define DEFAULT_PAGER       "less"
#define DEFAULT_EDITOR      "vi"
#define DEFAULT_PSQL        "psql"

#define TOTAL_CONTEXTS          14
#define DEFAULT_QUERY_CONTEXT   pg_stat_database

#define CONN_ARG_MAXLEN		S_BUF_LEN
#define CONNINFO_MAXLEN		S_BUF_LEN * 5	/* host, port, username, dbname, password */

#define PGCENTERRC_FILE         ".pgcenterrc"

/* enum for program internal messages */
enum mtype
{
    msg_notice,
    msg_warning,
    msg_error,
    msg_fatal
};

/* type of checks for string */
enum chk_type
{
    is_alfanum,
    is_number,
    is_float
};

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
    pg_stat_statements_local,
    pg_stat_progress_vacuum
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

/* struct for specific details about system when postgres runs */
struct sys_special_s
{
    int sys_hz;             /* system clock resolution */
    unsigned int bdev;      /* number of block devices */
    unsigned int idev;      /* number of network interfaces */
};

#define SYS_SPECIAL_SIZE (sizeof(struct sys_special_s))

/* struct for postgres specific details, get that when connected to postgres server */
struct pg_special_s
{
    bool pg_is_in_recovery;			/* is postgres a standby? - true/false */
    unsigned int av_max_workers;		/* autovacuum_max_workers GUC value */
    unsigned int pg_max_conns;                  /* max_connections GUC value */
    char pg_version_num[XS_BUF_LEN];		/* postgresql version XXYYZZ format */
    char pg_version[XS_BUF_LEN];		/* postgresql version X.Y.Z format */
};

#define PG_SPECIAL_SIZE (sizeof(struct pg_special_s))

/* struct for context list used in tab */
struct context_s
{
    enum context context;
    unsigned int order_key;
    bool order_desc;
    char fstrings[MAX_COLS][S_BUF_LEN];         /* filtering patterns */
};

/* struct which define connection options */
struct tab_s
{
    int tab;
    bool conn_used;
    bool conn_local;
    char host[CONN_ARG_MAXLEN];
    char port[CONN_ARG_MAXLEN];
    char user[CONN_ARG_MAXLEN];
    char dbname[CONN_ARG_MAXLEN];
    char password[CONN_ARG_MAXLEN];
    char conninfo[CONNINFO_MAXLEN];
    struct pg_special_s pg_special;
    struct sys_special_s sys_special;       /* details about os when pg runs */
    bool subtab_enabled;                     /* subtab status: on/off */
    int subtab;                              /* subtab type: logtail, iostat, etc. */
    char log_path[PATH_MAX];                    /* logfile path for logtail subtab */
    int log_fd;                                 /* logfile fd for log viewing */
    enum context current_context;
    char pg_stat_activity_min_age[XS_BUF_LEN];
    struct context_s context_list[TOTAL_CONTEXTS];
    int signal_options;
    bool pg_stat_sys;
};

#define TAB_SIZE (sizeof(struct tab_s))

/* simple comparison functions */
#define min(a,b)    (a > b) ? b : a
#define max(a,b)    (a > b) ? a : b

/* function declarations */
void mreport(bool do_exit, enum mtype mtype, const char * msg, ...);
void strrpl(char * o_string, const char * s_string, const char * r_string, unsigned int buf_size);
int check_string(const char * string, enum chk_type ctype);
char * password_prompt(const char *prompt, unsigned int pw_maxlen, bool echo);
void cmd_readline(WINDOW *window, const char * msg, unsigned int pos, bool * with_esc, char * str, unsigned int len, bool echoing);
void sig_handler(int signo);
void init_signal_handlers(void);
void check_pg_listen_addr(struct tab_s * tab, PGconn * conn);

void get_HZ(struct tab_s * tab, PGconn * conn);
#endif /* __COMMON_H__ */
