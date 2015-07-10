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

/* Struct which define connection options */
struct conn_opts_struct
{
    int terminal;
    bool conn_used;
    char host[BUFFERSIZE];
    char port[BUFFERSIZE];
    char user[BUFFERSIZE];
    char dbname[BUFFERSIZE];
    char password[BUFFERSIZE];
    char conninfo[BUFFERSIZE];
    bool log_opened;
    FILE *log;
};

#define CONN_OPTS_SIZE (sizeof(struct conn_opts_struct))

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

/*
 * Macros used to display statistics values.
 * NB: Define SP_VALUE() to normalize to %;
 */
#define SP_VALUE(m,n,p) (((double) ((n) - (m))) / (p) * 100)

#endif
