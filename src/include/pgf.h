/*
 ****************************************************************************
 * pgf.h
 *      postgres related definitions and macros.
 *
 * (C) 2016 by Alexey V. Lesovsky (lesovsky <at> gmail.com)
 * 
 ****************************************************************************
 */
#ifndef __PGF_H__
#define __PGF_H__

#include "common.h"
#include "queries.h"
#include "stats.h"

#define QUERY_MAXLEN		XXXL_BUF_LEN
#define CONNINFO_TITLE_LEN	48

/* 
 * PostgreSQL version notations:
 * PostgreSQL stores his version in XXYYY format, where XX is major version 
 * and YYY is minor. For example, 90540 means 9.5.4.
 * */
#define PG92 90200
#define PG96 90600

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
#define GUC_MAX_CONNS           "max_connections"

/* PostgreSQL answers, see PQresultStatus() at http://www.postgresql.org/docs/current/static/libpq-exec.html */
#define PG_CMD_OK       PGRES_COMMAND_OK
#define PG_TUP_OK       PGRES_TUPLES_OK
#define PG_FATAL_ERR    PGRES_FATAL_ERROR

void open_connections(struct tab_s * tabs[], PGconn * conns[]);
void close_connections(struct tab_s * tabs[], PGconn * conns[]);
PGresult * do_query(PGconn * conn, const char * query, char errmsg[]);
void get_conf_value(PGconn * conn, const char * config_option_name, char * config_option_value);
void get_pg_special(PGconn * conn, struct tab_s * tab);
void get_sys_special(PGconn * conn, struct tab_s * tab);
void reconnect_if_failed(WINDOW * window, PGconn * conn, struct tab_s * tab, bool *reconnected);
void prepare_query(struct tab_s * tab, char * query);
void get_pg_uptime(PGconn * conn, char * uptime);
int get_conn_status(PGconn *conn);
void write_conn_status(WINDOW * window, PGconn *conn, unsigned int tab_no, int st_index);
void get_summary_pg_activity(WINDOW * window, struct tab_s * tab, PGconn * conn);
void get_summary_vac_activity(WINDOW * window, struct tab_s * tab, PGconn * conn);
void get_pgss_summary(WINDOW * window, PGconn * conn, unsigned long interval);
bool check_view_exists(PGconn * conn, char * view);
#endif /* __PGF_H__ */
