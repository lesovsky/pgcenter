// This is an open source non-commercial project. Dear PVS-Studio, please check it.
// PVS-Studio Static Code Analyzer for C, C++ and C#: http://www.viva64.com

/*
 ****************************************************************************
 * pgf.c
 *      postgres related functions.
 *
 * (C) 2016 by Alexey V. Lesovsky (lesovsky <at> gmail.com)
 * 
 ****************************************************************************
 */
#include "include/pgf.h"

/*
 ****************************************************************************
 * Open connections to PostgreSQL using conninfo string from tab struct.
 ****************************************************************************
 */
void open_connections(struct tab_s * tabs[], PGconn * conns[])
{
    unsigned int i;
    for ( i = 0; i < MAX_TABS; i++ ) {
        if (tabs[i]->conn_used) {
            conns[i] = PQconnectdb(tabs[i]->conninfo);
            if ( PQstatus(conns[i]) == CONNECTION_BAD && PQconnectionNeedsPassword(conns[i]) == 1) {
                printf("%s:%s %s@%s require ", 
                                tabs[i]->host, tabs[i]->port,
                                tabs[i]->user, tabs[i]->dbname);
                snprintf(tabs[i]->password, sizeof(tabs[i]->password), "%s",
            password_prompt("password: ", sizeof(tabs[i]->password), false));
        snprintf(tabs[i]->conninfo + strlen(tabs[i]->conninfo),
            sizeof(tabs[i]->conninfo) - strlen(tabs[i]->conninfo),
                    " password=%s", tabs[i]->password);
                    conns[i] = PQconnectdb(tabs[i]->conninfo);
            } else if ( PQstatus(conns[i]) == CONNECTION_BAD ) {
                mreport(false, msg_error, "ERROR: Connection to %s:%s with %s@%s failed (tab %i).\n",
                        tabs[i]->host, tabs[i]->port,
                        tabs[i]->user, tabs[i]->dbname, i + 1);
                continue;
            }

            /* determine is it a local PostgreSQL or remote */
            check_pg_listen_addr(tabs[i], conns[i]);
            
            /* get specific details about system and postgres */
            get_sys_special(conns[i], tabs[i]);
            get_pg_special(conns[i], tabs[i]);

            PGresult * res;
            char errmsg[ERRSIZE];
            /* suppress log messages with log_min_duration_statement */
            if ((res = do_query(conns[i], PG_SUPPRESS_LOG_QUERY, errmsg)) != NULL)
               PQclear(res);
            /* increase our work_mem */
            if ((res = do_query(conns[i], PG_INCREASE_WORK_MEM_QUERY, errmsg)) != NULL)
                PQclear(res);
            
            /* install/uninstall stats schema */
            if (tabs[i]->install_stats)
                install_stats_schema(tabs[i], conns[i]);
            if (tabs[i]->uninstall_stats)
                uninstall_stats_schema(conns[i]);
        }
    }
}

/*
 ****************************************************************************
 * Close all connections to postgres. Used at program quit.
 ****************************************************************************
 */
void close_connections(struct tab_s * tabs[], PGconn * conns[])
{
    unsigned int i;
    for (i = 0; i < MAX_TABS; i++)
        if (tabs[i]->conn_used)
            PQfinish(conns[i]);
}

/*
 ****************************************************************************
 * Send query to postgres and return query result or error message.
 ****************************************************************************
 */
PGresult * do_query(PGconn * conn, const char * query, char errmsg[])
{
    PGresult    *res;

    res = PQexec(conn, query);
    switch (PQresultStatus(res)) {
        case PG_CMD_OK: case PG_TUP_OK:
            return res;
            break;
        default:
	    snprintf(errmsg, ERRSIZE, "%s: %s\nDETAIL: %s\nHINT: %s",
			PQresultErrorField(res, PG_DIAG_SEVERITY),
			PQresultErrorField(res, PG_DIAG_MESSAGE_PRIMARY),
			PQresultErrorField(res, PG_DIAG_MESSAGE_DETAIL),
			PQresultErrorField(res, PG_DIAG_MESSAGE_HINT));
            PQclear(res);
            return NULL;
            break;
    }
}

/*
 ****************************************************************************
 * Get GUC value from postgres config.
 ****************************************************************************
 */
void get_conf_value(PGconn * conn, const char * config_option_name, char * config_option_value)
{
    PGresult * res;
    char errmsg[ERRSIZE],
         query[QUERY_MAXLEN];

    snprintf(query, sizeof(query), "%s%s%s", PG_SETTINGS_SINGLE_OPT_P1, config_option_name, PG_SETTINGS_SINGLE_OPT_P2);

    res = do_query(conn, query, errmsg);
    
    if (PQntuples(res) != 0 && !strcmp(PQgetvalue(res, 0, 0), config_option_name))
        snprintf(config_option_value, M_BUF_LEN, "%s", PQgetvalue(res, 0, 1));
    else
        config_option_value[0] = '\0';
    
    PQclear(res);
}

/*
 ****************************************************************************
 * Get various information about running OS.
 ****************************************************************************
 */
void get_sys_special(PGconn * conn, struct tab_s * tab)
{
    /* get system clock resolution */
    get_HZ(tab, conn);

    /* get number of block and network devices */
    tab->sys_special.bdev = count_devices(BLKDEV, tab->conn_local, conn);
    tab->sys_special.idev = count_devices(NETDEV, tab->conn_local, conn);
}

/*
 ****************************************************************************
 * Get various information about postgres and save into tab opts.
 ****************************************************************************
 */
void get_pg_special(PGconn * conn, struct tab_s * tab)
{
    PGresult * res;
    char errmsg[ERRSIZE];
    char av_max_workers[8], pg_max_conns[8], pg_max_preps[8];

    /* get postgres version information */
    get_conf_value(conn, GUC_SERVER_VERSION_NUM, tab->pg_special.pg_version_num);
    get_conf_value(conn, GUC_SERVER_VERSION, tab->pg_special.pg_version);
    if (strlen(tab->pg_special.pg_version_num) == 0)
        snprintf(tab->pg_special.pg_version_num, sizeof(tab->pg_special.pg_version_num), "-.-.-");
    if (strlen(tab->pg_special.pg_version) == 0)
        snprintf(tab->pg_special.pg_version, sizeof(tab->pg_special.pg_version_num), "-.-.-");

    /* pg_is_in_recovery() */
    if ((res = do_query(conn, PG_IS_IN_RECOVERY_QUERY, errmsg)) != NULL) {
        (!strcmp(PQgetvalue(res, 0, 0), "f"))
	    ? (tab->pg_special.pg_is_in_recovery = false)
	    : (tab->pg_special.pg_is_in_recovery = true);
        PQclear(res);
    }

    /* get autovacuum_max_workers */
    get_conf_value(conn, GUC_AV_MAX_WORKERS, av_max_workers);
    (strlen(av_max_workers) == 0)
	? (tab->pg_special.av_max_workers = 0)
	: (tab->pg_special.av_max_workers = atoi(av_max_workers));

    /* get max connections limit */
    get_conf_value(conn, GUC_MAX_CONNS, pg_max_conns);
    (strlen(pg_max_conns) == 0)
	? (tab->pg_special.pg_max_conns = 0)
	: (tab->pg_special.pg_max_conns = atoi(pg_max_conns));
    
    /* get max prepared transactions limit */
    get_conf_value(conn, GUC_MAX_PREPS, pg_max_preps);
    (strlen(pg_max_preps) == 0)
    ? (tab->pg_special.pg_max_preps = 0)
    : (tab->pg_special.pg_max_preps = atoi(pg_max_preps));
}

/*
 ****************************************************************************
 * Check connection state, try to reconnect if connection failed.
 * It's helpful for restore connection when postgres restarts.
 ****************************************************************************
 */
void reconnect_if_failed(WINDOW * window, PGconn * conns[], struct tab_s * tabs[], int tab_index, bool *reconnected)
{
    if (PQstatus(conns[tab_index]) == CONNECTION_BAD) {
        wclear(window);
        PQreset(conns[tab_index]);
        wprintw(window, "The connection to the server was lost. Attempting reconnect.");
        wrefresh(window);
        /* reset previous query results after reconnect */
        *reconnected = true;
        sleep(1);
    }
    
    /* get pg and os details after successful reconnect */
    if (*reconnected == true) {
        free_iostat(tabs, tab_index);
        free_ifstat(tabs, tab_index);
        get_sys_special(conns[tab_index], tabs[tab_index]);
        get_pg_special(conns[tab_index], tabs[tab_index]);
        init_iostat(tabs, tab_index);
        init_ifstat(tabs, tab_index);
    }
}

/*
 ****************************************************************************
 * Prepare a query using current tab query context.
 ****************************************************************************
 */
void prepare_query(struct tab_s * tab, char * query)
{
    /* determine proper wal function which depends on recovery state */
    char wal_function[S_BUF_LEN];
    if (tab->pg_special.pg_is_in_recovery == false)
        snprintf(wal_function, S_BUF_LEN, "%s", PG_STAT_REPLICATION_NOREC);
    else
        snprintf(wal_function, S_BUF_LEN, "%s", PG_STAT_REPLICATION_REC);
    
    switch (tab->current_context) {
        case pg_stat_database: default:
            (atoi(tab->pg_special.pg_version_num) < PG92)
                ? snprintf(query, QUERY_MAXLEN, "%s", PG_STAT_DATABASE_91_QUERY)
                : snprintf(query, QUERY_MAXLEN, "%s", PG_STAT_DATABASE_QUERY);
            break;
        case pg_stat_replication:
            if (atoi(tab->pg_special.pg_version_num) < PG95) {
                snprintf(query, QUERY_MAXLEN, "%s%s%s%s%s%s%s", PG_STAT_REPLICATION_94_QUERY_P1,
                         wal_function, PG_STAT_REPLICATION_94_QUERY_P2,
                         wal_function, PG_STAT_REPLICATION_94_QUERY_P3,
                         wal_function, PG_STAT_REPLICATION_94_QUERY_P4);
            } else {
                snprintf(query, QUERY_MAXLEN, "%s%s%s%s%s%s%s", PG_STAT_REPLICATION_QUERY_P1,
                         wal_function, PG_STAT_REPLICATION_QUERY_P2,
                         wal_function, PG_STAT_REPLICATION_94_QUERY_P3,
                         wal_function, PG_STAT_REPLICATION_QUERY_P4);
            }
            break;
        case pg_stat_tables:
            snprintf(query, QUERY_MAXLEN, "%s%s%s", PG_STAT_TABLES_QUERY_P1,
			tab->pg_stat_sys ? "all" : "user", PG_STAT_TABLES_QUERY_P2);
            break;
        case pg_stat_indexes:
            snprintf(query, QUERY_MAXLEN, "%s%s%s%s%s", 
			PG_STAT_INDEXES_QUERY_P1, tab->pg_stat_sys ? "all" : "user",
                        PG_STAT_INDEXES_QUERY_P2, tab->pg_stat_sys ? "all" : "user",
                        PG_STAT_INDEXES_QUERY_P3);
            break;
        case pg_statio_tables:
            snprintf(query, QUERY_MAXLEN, "%s%s%s",
                        PG_STATIO_TABLES_QUERY_P1, tab->pg_stat_sys ? "all" : "user",
                        PG_STATIO_TABLES_QUERY_P2);
            break;
        case pg_tables_size:
            snprintf(query, QUERY_MAXLEN, "%s%s%s",
                        PG_TABLES_SIZE_QUERY_P1, tab->pg_stat_sys ? "all" : "user",
                        PG_TABLES_SIZE_QUERY_P2);
            break;
        case pg_stat_activity_long:
            /* 
             * build query from several parts, 
             * thus user can change duration which is used in WHERE clause.
             */
            if (atoi(tab->pg_special.pg_version_num) < PG92) {
                snprintf(query, QUERY_MAXLEN, "%s%s%s%s%s",
                        PG_STAT_ACTIVITY_LONG_91_QUERY_P1, tab->pg_stat_activity_min_age,
                        PG_STAT_ACTIVITY_LONG_91_QUERY_P2, tab->pg_stat_activity_min_age,
                        PG_STAT_ACTIVITY_LONG_91_QUERY_P3);
            } else if (atoi(tab->pg_special.pg_version_num) > PG92 && atoi(tab->pg_special.pg_version_num) < PG96) {
                snprintf(query, QUERY_MAXLEN, "%s%s%s%s%s",
                        PG_STAT_ACTIVITY_LONG_95_QUERY_P1, tab->pg_stat_activity_min_age,
                        PG_STAT_ACTIVITY_LONG_95_QUERY_P2, tab->pg_stat_activity_min_age,
                        PG_STAT_ACTIVITY_LONG_95_QUERY_P3);
	    } else {
                snprintf(query, QUERY_MAXLEN, "%s%s%s%s%s",
                        PG_STAT_ACTIVITY_LONG_QUERY_P1, tab->pg_stat_activity_min_age,
                        PG_STAT_ACTIVITY_LONG_QUERY_P2, tab->pg_stat_activity_min_age,
                        PG_STAT_ACTIVITY_LONG_QUERY_P3);
            }
            break;
        case pg_stat_functions:
            snprintf(query, QUERY_MAXLEN, "%s", PG_STAT_FUNCTIONS_QUERY_P1);
            break;
        case pg_stat_statements_timing:
    	    atoi(tab->pg_special.pg_version_num) < PG92
                ? snprintf(query, QUERY_MAXLEN, "%s", PG_STAT_STATEMENTS_TIMING_91_QUERY_P1)
                : snprintf(query, QUERY_MAXLEN, "%s", PG_STAT_STATEMENTS_TIMING_QUERY_P1);
            break;
        case pg_stat_statements_general:
            atoi(tab->pg_special.pg_version_num) < PG92
                ? snprintf(query, QUERY_MAXLEN, "%s", PG_STAT_STATEMENTS_GENERAL_91_QUERY_P1)
                : snprintf(query, QUERY_MAXLEN, "%s", PG_STAT_STATEMENTS_GENERAL_QUERY_P1);
            break;
        case pg_stat_statements_io:
            atoi(tab->pg_special.pg_version_num) < PG92
                ? snprintf(query, QUERY_MAXLEN, "%s", PG_STAT_STATEMENTS_IO_91_QUERY_P1)
                : snprintf(query, QUERY_MAXLEN, "%s", PG_STAT_STATEMENTS_IO_QUERY_P1);
            break;
        case pg_stat_statements_temp:
            snprintf(query, QUERY_MAXLEN, "%s", PG_STAT_STATEMENTS_TEMP_QUERY_P1);
            break;
        case pg_stat_statements_local:
            atoi(tab->pg_special.pg_version_num) < PG92
                ? snprintf(query, QUERY_MAXLEN, "%s", PG_STAT_STATEMENTS_LOCAL_91_QUERY_P1)
                : snprintf(query, QUERY_MAXLEN, "%s", PG_STAT_STATEMENTS_LOCAL_QUERY_P1);
            break;
        case pg_stat_progress_vacuum:
            snprintf(query, QUERY_MAXLEN, "%s", PG_STAT_PROGRESS_VACUUM_QUERY);
            break;
    }
}

/*
 ****************************************************************************
 * Get postgres uptime
 ****************************************************************************
 */
void get_pg_uptime(PGconn * conn, char * uptime)
{
    static char errmsg[ERRSIZE];
    PGresult * res;

    if ((res = do_query(conn, PG_UPTIME_QUERY, errmsg)) != NULL) {
        snprintf(uptime, S_BUF_LEN, "%s", PQgetvalue(res, 0, 0));
        PQclear(res);
    } else {
        snprintf(uptime, S_BUF_LEN, "--:--:--");
    }
}

/* 
 ****************************************************************************
 * Get the status of the pgcenter's current connection.
 ****************************************************************************
 */
int get_conn_status(PGconn *conn)
{
    int status;
    
    switch (PQstatus(conn)) {
        case CONNECTION_OK:
            status = 0; 	/* ok */
            break;
        case CONNECTION_BAD:
            status = 1; 	/* failed */
            break;
        default:
            status = 2; 	/* unknown */
            break;
    }
    
    return status;
}

/* 
 ****************************************************************************
 * Write the status of the pgcenter's current connection.
 ****************************************************************************
 */
void write_conn_status(WINDOW * window, PGconn *conn, unsigned int tab_no, int st_index)
{
    const char * states[] = { "ok", "failed", "unknown" };
    char buffer[CONNINFO_TITLE_LEN];
        
    snprintf(buffer, sizeof(buffer), "conn%i [%s]: %s:%s %s@%s",
            tab_no, states[st_index],
            PQhost(conn), PQport(conn),
            PQuser(conn), PQdb(conn));

    mvwprintw(window, 0, COLS / 2, "%s", buffer);
    wrefresh(window);
}

/* 
 ****************************************************************************
 * Get and print information about current postgres activity.
 ****************************************************************************
 */
void get_summary_pg_activity(WINDOW * window, struct tab_s * tab, PGconn * conn)
{
    unsigned int t_count = 0,		/* total number of connections */
        	 i_count = 0,			/* number of idle connections */
        	 x_count = 0,			/* number of idle in xact */
        	 a_count = 0,			/* number of active connections */
        	 w_count = 0,			/* number of waiting connections */
        	 o_count = 0,			/* other, unclassiffied */
        	 p_count = 0;			/* number of prepared xacts */
    PGresult *res;
    static char errmsg[ERRSIZE];
    char query[QUERY_MAXLEN];

    if (atoi(tab->pg_special.pg_version_num) < PG96)
    	snprintf(query, QUERY_MAXLEN, "%s", PG_STAT_ACTIVITY_COUNT_95_QUERY);
    else
        snprintf(query, QUERY_MAXLEN, "%s", PG_STAT_ACTIVITY_COUNT_QUERY);

    if ((res = do_query(conn, query, errmsg)) != NULL) {
        t_count = atoi(PQgetvalue(res, 0, 0));
        i_count = atoi(PQgetvalue(res, 0, 1));
        x_count = atoi(PQgetvalue(res, 0, 2));
        a_count = atoi(PQgetvalue(res, 0, 3));
        w_count = atoi(PQgetvalue(res, 0, 4));
        o_count = atoi(PQgetvalue(res, 0, 5));
        p_count = atoi(PQgetvalue(res, 0, 6));
        PQclear(res);
    }

    mvwprintw(window, 1, COLS / 2,
            "  activity:%3i/%i conns,%3i/%i prepared,%3i idle,%3i idle_xact,%3i active,%3i waiting,%3i others",
            t_count, tab->pg_special.pg_max_conns,
            p_count, tab->pg_special.pg_max_preps,
            i_count, x_count, a_count, w_count, o_count);
    wrefresh(window);
}

/* 
 ****************************************************************************
 * Get and print information about current postgres (auto)vacuum activity.
 ****************************************************************************
 */
void get_summary_vac_activity(WINDOW * window, struct tab_s * tab, PGconn * conn) 
{
    unsigned int av_count = 0,		/* total number of autovacuum workers */
		 avw_count = 0,		/* number of wraparound workers */
		 mv_count = 0;		/* number of manual vacuums executed by user */
    char vac_maxtime[XS_BUF_LEN] = "--:--:--";		/* the longest worker or vacuum */
    PGresult *res;
    static char errmsg[ERRSIZE];
    
    if ((res = do_query(conn, PG_STAT_ACTIVITY_AV_COUNT_QUERY, errmsg)) != NULL) {
        av_count = atoi(PQgetvalue(res, 0, 0));
        avw_count = atoi(PQgetvalue(res, 0, 1));
        mv_count = atoi(PQgetvalue(res, 0, 2));
        snprintf(vac_maxtime, sizeof(vac_maxtime), "%s", PQgetvalue(res, 0, 3));
        PQclear(res);
    }
    
    mvwprintw(window, 2, COLS / 2, "autovacuum: %2u/%u workers/max, %2u manual, %2u wraparound, %s vac_maxtime",
                    av_count, tab->pg_special.av_max_workers, mv_count, avw_count, vac_maxtime);
    wrefresh(window);
}

/* 
 ****************************************************************************
 * Get and print info about xacts and queries from pgss and pgsa.
 ****************************************************************************
 */
void get_pgss_summary(WINDOW * window, PGconn * conn, unsigned long interval)
{
    float avgtime;
    static unsigned int qps, prev_queries = 0;
    unsigned int divisor;
    char x_maxtime[XS_BUF_LEN] = "--:--:--", p_maxtime[XS_BUF_LEN] = "--:--:--";
    PGresult *res;
    char errmsg[ERRSIZE];

    if (PQstatus(conn) == CONNECTION_BAD) {
        avgtime = qps = 0;
    } 

    divisor = interval / 1000000;
    if ((res = do_query(conn, PG_STAT_STATEMENTS_SYS_QUERY, errmsg)) != NULL) {
        avgtime = atof(PQgetvalue(res, 0, 0));
        qps = (atoi(PQgetvalue(res, 0, 1)) - prev_queries) / divisor;
        prev_queries = atoi(PQgetvalue(res, 0, 1));
        PQclear(res);
    } else {
        avgtime = 0;
        qps = 0;
    }

    if ((res = do_query(conn, PG_STAT_ACTIVITY_SYS_QUERY, errmsg)) != NULL) {
        snprintf(x_maxtime, sizeof(x_maxtime), "%s", PQgetvalue(res, 0, 0));
        snprintf(p_maxtime, sizeof(p_maxtime), "%s", PQgetvalue(res, 0, 1));
        PQclear(res);
    }

    mvwprintw(window, 3, COLS / 2,
            "statements: %3i stmt/s, %3.3f stmt_avgtime, %s xact_maxtime, %s prep_maxtime",
            qps, avgtime, x_maxtime, p_maxtime);
    wrefresh(window);
}

/* 
 ****************************************************************************
 * Check pgcenter's statview existance.
 ****************************************************************************
 */
bool check_view_exists(PGconn * conn, char * view)
{
    PGresult *res;
    char query[QUERY_MAXLEN], errmsg[ERRSIZE];
    bool exists;
    
    snprintf(query, QUERY_MAXLEN, "SELECT 1 FROM %s LIMIT 1", view);
    if ((res = do_query(conn, query, errmsg)) != NULL && PQntuples(res) > 0) {
        exists = true;
        PQclear(res);
    } else {
        exists = false;
    }
    return exists;
}

/* 
 ****************************************************************************
 * Install stats schema and functions into the database.
 ****************************************************************************
 */
void install_stats_schema(struct tab_s * tab, PGconn * conn)
{
    int sql_fd, i, n_files, n_read;
    struct stat stats;
    PGresult *res;
    char query[QUERY_MAXLEN], errmsg[ERRSIZE], sql_file[PATH_MAX];
    char * buffer;
    
    /* Perhaps in the future, functions on other languages will be added. */
    char * sql_plperlu_files_list[2] = { REMOTE_STATS_SCHEMA_PL_FUNCS_FILE, REMOTE_STATS_SCHEMA_VIEWS_FILE };
    char * (* sql_files_list)[2];       /* pointer to set of files used in the loop below */

    mreport(false, msg_notice, "INFO: installing stats schema into %s@%s:%s/%s\n",
            PQuser(conn), PQhost(conn), PQport(conn), PQdb(conn));
    
    /* sql schema depend on the chosen language */
    if (!strcmp(tab->stats_lang, "plperlu") || strlen(tab->stats_lang) == 0) {
        sql_files_list = &sql_plperlu_files_list;          /* assign pointer to particular set of files */
        snprintf(tab->stats_lang, 8, "plperlu");
    } else {
        mreport(false, msg_warning, "ERROR: %s language is not supported.\n", tab->stats_lang);
        return;
    }
    
    n_files = ARRAY_SIZE((*sql_files_list));
    for (i = 0; i < n_files; i++) {
        if ((stat((*sql_files_list)[i], &stats)) == -1) {
            mreport(false, msg_warning, "ERROR: failed to get size of %s.\n", sql_file);
            return;
        }

        /* it's too strange, if sql file has size more than 1MB */
        if (stats.st_size > XXXL_BUF_LEN * 128) {
            mreport(false, msg_warning, "ERROR: %s too big.\n", sql_file);
            return;
        }

        if ((buffer = (char *) malloc((size_t) stats.st_size)) == NULL) {
            mreport(false, msg_warning, "ERROR: malloc() for buffer failed (install stats schema).\n");
            return;
        }

        snprintf(sql_file, PATH_MAX, "%s", (*sql_files_list)[i]);
        if ((sql_fd = open(sql_file, O_RDONLY)) == -1 ) {
            mreport(false, msg_warning, "ERROR: failed to open stats schema file: %s.\n", sql_file);
            return;
        }

        if ((n_read = read(sql_fd, buffer, stats.st_size)) > 0) {
            buffer[n_read] = '\0';
            snprintf(query, QUERY_MAXLEN, "%s", buffer);

            if ((res = do_query(conn, query, errmsg)) != NULL) {
                PQclear(res);       /* success */
            } else {
                mreport(false, msg_warning, "ERROR: install stats schema into %s@%s:%s/%s failed:\n%s\n",
                        PQuser(conn), PQhost(conn), PQport(conn), PQdb(conn), errmsg);
                free(buffer);
                close(sql_fd);
                return;
            }
        } else {
            mreport(false, msg_warning, "ERROR: %s read failed.\n", sql_file);
            free(buffer);
            close(sql_fd);
            return;
        }

        free(buffer);
        close(sql_fd);
    }
    /* if we are here, report about success */
    mreport(false, msg_notice, "INFO: stats schema installed (procedural language: %s) on %s@%s:%s/%s\n",
            tab->stats_lang, PQuser(conn), PQhost(conn), PQport(conn), PQdb(conn));
}

/* 
 ****************************************************************************
 * Uninstall stats schema from the database.
 ****************************************************************************
 */
void uninstall_stats_schema(PGconn * conn)
{
    PGresult *res;
    char query[QUERY_MAXLEN], errmsg[ERRSIZE];

    snprintf(query, QUERY_MAXLEN, "%s", PG_DROP_STATS_SCHEMA_QUERY);

    mreport(false, msg_notice, "INFO: remove stats schema from %s@%s:%s/%s\n",
            PQuser(conn), PQhost(conn), PQport(conn), PQdb(conn));
    if ((res = do_query(conn, query, errmsg)) != NULL) {
        mreport(false, msg_notice, "INFO: stats schema removed from %s@%s:%s/%s\n",
            PQuser(conn), PQhost(conn), PQport(conn), PQdb(conn));
        PQclear(res);
    } else {
        mreport(false, msg_notice, "ERROR: failed to remove stats schema from %s@%s:%s/%s:\n%s\n",
            PQuser(conn), PQhost(conn), PQport(conn), PQdb(conn), errmsg);
    }
    return;
}
