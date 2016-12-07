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

            /* get PostgreSQL details */
            get_pg_special(conns[i], tabs[i]);

            PGresult * res;
            char errmsg[ERRSIZE];
            /* suppress log messages with log_min_duration_statement */
            if ((res = do_query(conns[i], PG_SUPPRESS_LOG_QUERY, errmsg)) != NULL)
               PQclear(res);
            /* increase our work_mem */
            if ((res = do_query(conns[i], PG_INCREASE_WORK_MEM_QUERY, errmsg)) != NULL)
                PQclear(res);
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
 * Get various information about postgres and save into tab opts.
 ****************************************************************************
 */
void get_pg_special(PGconn * conn, struct tab_s * tab)
{
    PGresult * res;
    char errmsg[ERRSIZE];
    char av_max_workers[8], pg_max_conns[8];

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
}

/*
 ****************************************************************************
 * Check connection state, try to reconnect if connection failed.
 * It's helpful for restore connection when postgres restarts.
 ****************************************************************************
 */
void reconnect_if_failed(WINDOW * window, PGconn * conn, struct tab_s * tab, bool *reconnected)
{
    if (PQstatus(conn) == CONNECTION_BAD) {
        wclear(window);
        PQreset(conn);
        wprintw(window, "The connection to the server was lost. Attempting reconnect.");
        wrefresh(window);
        /* reset previous query results after reconnect */
        *reconnected = true;
        sleep(1);
    }
    
    /* get PostgreSQL details after successful reconnect */
    if (*reconnected == true) {
        get_pg_special(conn, tab);
    }
}

/*
 ****************************************************************************
 * Prepare a query using current tab query context.
 ****************************************************************************
 */
void prepare_query(struct tab_s * tab, char * query)
{
    switch (tab->current_context) {
        case pg_stat_database: default:
            (atoi(tab->pg_special.pg_version_num) < PG92)
                ? snprintf(query, QUERY_MAXLEN, "%s", PG_STAT_DATABASE_91_QUERY)
                : snprintf(query, QUERY_MAXLEN, "%s", PG_STAT_DATABASE_QUERY);
            break;
        case pg_stat_replication:
            snprintf(query, QUERY_MAXLEN, "%s%s%s%s%s", PG_STAT_REPLICATION_QUERY_P1,
			tab->pg_special.pg_is_in_recovery == false
				? PG_STAT_REPLICATION_NOREC
				: PG_STAT_REPLICATION_REC,
			PG_STAT_REPLICATION_QUERY_P2,
			tab->pg_special.pg_is_in_recovery == false
				? PG_STAT_REPLICATION_NOREC
				: PG_STAT_REPLICATION_REC,
			PG_STAT_REPLICATION_QUERY_P3);
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
        	 o_count = 0;			/* other, unclassiffied */
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
        PQclear(res);
    }

    mvwprintw(window, 1, COLS / 2,
            "  activity:%3i/%i total/max,%3i idle,%3i idle_xact,%3i active,%3i waiting,%3i others",
            t_count, tab->pg_special.pg_max_conns, i_count, x_count, a_count, w_count, o_count);
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
    char maxtime[XS_BUF_LEN] = "";
    PGresult *res;
    char errmsg[ERRSIZE];

    if (PQstatus(conn) == CONNECTION_BAD) {
        avgtime = qps = 0;
        snprintf(maxtime, sizeof(maxtime), "--:--:--");
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
        snprintf(maxtime, sizeof(maxtime), "%s", PQgetvalue(res, 0, 0));
        PQclear(res);
    } else {
        snprintf(maxtime, sizeof(maxtime), "--:--:--");
    }

    mvwprintw(window, 3, COLS / 2,
            "statements: %3i stmt/s,  %3.3f stmt_avgtime, %s xact_maxtime",
            qps, avgtime, maxtime);
    wrefresh(window);
}
