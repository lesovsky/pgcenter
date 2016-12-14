// This is an open source non-commercial project. Dear PVS-Studio, please check it.
// PVS-Studio Static Code Analyzer for C, C++ and C#: http://www.viva64.com

/*
 ****************************************************************************
 * hotkeys.c
 *      hotkeys associated functions.
 *
 * (C) 2016 by Alexey V. Lesovsky (lesovsky <at> gmail.com)
 * 
 ****************************************************************************
 */
#include "include/common.h"
#include "include/hotkeys.h"


/*
 ****************************************************************************
 * Trap keys in program. Return 1 if key is pressed or 0 if not.
 ****************************************************************************
 */
bool key_is_pressed(void)
{
    int ch = getch();

    if (ch != ERR) {
        ungetch(ch);
        return true;
    } else
        return false;
}

/*
 ****************************************************************************
 * Print internal help tab.
 ****************************************************************************
 */
void print_help_tab(bool * first_iter)
{
    WINDOW * w;
    int ch;

    w = subwin(stdscr, 0, 0, 0, 0);
    cbreak();
    nodelay(w, FALSE);
    keypad(w, TRUE);

    wclear(w);
    wprintw(w, "Help for interactive commands - %s version %.1f.%d\n\n",
                PROGRAM_NAME, PROGRAM_VERSION, PROGRAM_RELEASE);
    wprintw(w, "general actions:\n\
  a,d,i,f,r       mode: 'a' activity, 'd' databases, 'i' indexes, 'f' functions, 'r' replication,\n\
  s,t,T,v         's' tables sizes, 't' tables, 'T' tables IO, 'v' vacuum progress,\n\
  x,X             'x' pg_stat_statements switch, 'X' pg_stat_statements menu.\n\
  Left,Right,/,F  'Left,Right' change column sort, '/' change sort desc/asc, 'F' set filter.\n\
  C,E,R           config: 'C' show config, 'E' edit configs, 'R' reload config.\n\
  p                       'p' start psql session.\n\
  l               'l' open log file with pager.\n\
  N,Ctrl+D,W      'N' add new connection, Ctrl+D close current connection, 'W' write connections info.\n\
  1..8            switch between tabs.\n\
subtab actions:\n\
  B,I,L           'B' iostat, 'I' nicstat, 'L' logtail.\n\
activity actions:\n\
  -,_             '-' cancel backend by pid, '_' terminate backend by pid.\n\
  >,.             '>' set new mask, '.' show current mask.\n\
  Del,Shift+Del   'Del' cancel backend group using mask, 'Shift+Del' terminate backend group using mask.\n\
  A               change activity age threshold.\n\
  G               get report about query using hash.\n\n\
other actions:\n\
  , Q             ',' show system tables on/off, 'Q' reset postgresql statistics counters.\n\
  z,Z             'z' set refresh interval, 'Z' change color scheme.\n\
  space           pause program execution.\n\
  h,F1            show help tab.\n\
  q               quit.\n\n");
    wprintw(w, "Type 'Esc' to continue.\n");

    do {
        ch = wgetch(w);
    } while (ch != 27);

    *first_iter = true;
    cbreak();
    nodelay(w, TRUE);
    keypad(w, FALSE);
    delwin(w);
}

/*
 ****************************************************************************
 * Set sort.
 ****************************************************************************
 */
void change_sort_order(struct tab_s * tab, bool increment, bool * first_iter)
{
    unsigned int max = 0, i;

    /* Determine max limit of range where cursor can move */
    switch (tab->current_context) {
        case pg_stat_database:
            (atoi(tab->pg_special.pg_version_num) < PG92)
                ? (max = PG_STAT_DATABASE_CMAX_91)
                : (max = PG_STAT_DATABASE_CMAX_LT);
            break;
        case pg_stat_replication:
            max = PG_STAT_REPLICATION_CMAX_LT;
            break;
        case pg_stat_tables:
            max = PG_STAT_TABLES_CMAX_LT;
            break;
        case pg_stat_indexes:
            max = PG_STAT_INDEXES_CMAX_LT;
            break;
        case pg_statio_tables:
            max = PG_STATIO_TABLES_CMAX_LT;
            break;
        case pg_tables_size:
            max = PG_TABLES_SIZE_CMAX_LT;
            break;
        case pg_stat_activity_long:
            if (atoi(tab->pg_special.pg_version_num) < PG92)
                max = PG_STAT_ACTIVITY_LONG_CMAX_91;
            else if (atoi(tab->pg_special.pg_version_num) < PG96)
                max = PG_STAT_ACTIVITY_LONG_CMAX_95;
            else
                max = PG_STAT_ACTIVITY_LONG_CMAX_LT;
            break;
        case pg_stat_functions:
            max = PG_STAT_FUNCTIONS_CMAX_LT;
            *first_iter = true;
            break;
        case pg_stat_statements_timing:
            (atoi(tab->pg_special.pg_version_num) < PG92)
                ? (max = PGSS_TIMING_CMAX_91)
                : (max = PGSS_TIMING_CMAX_LT);
            *first_iter = true;
            break;
        case pg_stat_statements_general:
            max = PGSS_GENERAL_CMAX_LT;
            *first_iter = true;
            break;
        case pg_stat_statements_io:
            (atoi(tab->pg_special.pg_version_num) < PG92)
                ? (max = PGSS_IO_CMAX_91)
                : (max = PGSS_IO_CMAX_LT);
            *first_iter = true;
            break;
        case pg_stat_statements_temp:
            max = PGSS_TEMP_CMAX_LT;
            *first_iter = true;
            break;
        case pg_stat_statements_local:
            (atoi(tab->pg_special.pg_version_num) < PG92)
                ? (max = PGSS_LOCAL_CMAX_91)
                : (max = PGSS_LOCAL_CMAX_LT);
            *first_iter = true;
            break;
        case pg_stat_progress_vacuum:
            max = PG_STAT_PROGRESS_VACUUM_CMAX_LT;
            break;
        default:
            break;
    }

    /* Change number of column used for sort */
    for (i = 0; i < TOTAL_CONTEXTS; i++) {
        if (tab->current_context == tab->context_list[i].context) {
            if (increment) {
                /* switch from last to first column */
                if (++tab->context_list[i].order_key > max) {
                    tab->context_list[i].order_key = 0;
                }
            } else {
                /* switch from first to last column */
                if (--tab->context_list[i].order_key < 0) {
                    tab->context_list[i].order_key = max;
                }
            }
        }
    }
}

/*
 ****************************************************************************
 * Change sort order from desc to asc and vice-versa.
 ****************************************************************************
 */
void change_sort_order_direction(struct tab_s * tab, bool * first_iter)
{
    unsigned int i;
    for (i = 0; i < TOTAL_CONTEXTS; i++) {
        if (tab->current_context == tab->context_list[i].context) {
	    tab->context_list[i].order_desc ^= 1;
        }
        *first_iter = true;
    }
}

/*
 ****************************************************************************
 * Set filter or reset one.
 ****************************************************************************
 */
void set_filter(WINDOW * win, struct tab_s * tab, PGresult * res, bool * first_iter) {
    int i;
    bool with_esc;
    char pattern[S_BUF_LEN], msg[S_BUF_LEN];
    struct context_s ctx;

    /* get current context and its filter strings array */
    for (i = 0; i < TOTAL_CONTEXTS; i++)
        if (tab->current_context == tab->context_list[i].context)
            ctx = tab->context_list[i];

    snprintf(msg, S_BUF_LEN, "Set filter, current: \"%s\": ", ctx.fstrings[ctx.order_key]);

    cmd_readline(win, msg, strlen(msg), &with_esc, pattern, sizeof(pattern), true);
    if (strlen(pattern) > 0 && with_esc == false) {
        snprintf(ctx.fstrings[ctx.order_key], sizeof(ctx.fstrings[ctx.order_key]), "%s", pattern);
    } else if (strlen(pattern) == 0 && with_esc == false ) {
        wprintw(win, "Reset filtering.");
        snprintf(ctx.fstrings[ctx.order_key], sizeof(ctx.fstrings[ctx.order_key]), "%s", "");
    }

    /* Save pattern to context */
    for (i = 0; i < TOTAL_CONTEXTS; i++)
        if (tab->current_context == tab->context_list[i].context)
            tab->context_list[i] = ctx;

    PQclear(res);
    *first_iter = true;
}

/*
 ****************************************************************************
 * Switch to another tab. Return index of destination tab.
 ****************************************************************************
 */
unsigned int switch_tab(WINDOW * window, struct tab_s * tabs[],
                unsigned int ch, unsigned int tab_index, unsigned int tab_no, PGresult * res, bool * first_iter)
{
    /* transform keycodes to digits. for example: 2 has keycode 50, then 50 - '0' = 2 */
    unsigned int dest_tab_no = ch - '0';
    unsigned int dest_tab_index = dest_tab_no - 1;

    wclear(window);
    if ( tabs[dest_tab_index]->conn_used ) {
        wprintw(window, "Switch to tab %i.", dest_tab_no);
        *first_iter = true;
        PQclear(res);
        return dest_tab_index;
    } else {
        wprintw(window, "No connection associated, stay on tab %i.", tab_no);
        return tab_index;
    }
}

/*
 ****************************************************************************
 * Switch statistics context in the curent tab.
 ****************************************************************************
 */
void switch_context(WINDOW * window, struct tab_s * tab, 
                    enum context context, PGresult * res, bool * first_iter)
{
    wclear(window);
    switch (context) {
        case pg_stat_database:
            wprintw(window, "Show databases statistics");
            break;
        case pg_stat_replication:
            wprintw(window, "Show replication statistics");
            break;
        case pg_stat_tables:
            wprintw(window, "Show tables statistics");
            break;
        case pg_stat_indexes:
            wprintw(window, "Show indexes statistics");
            break;
        case pg_statio_tables:
            wprintw(window, "Show tables IO statistics");
            break;
        case pg_tables_size:
            wprintw(window, "Show tables sizes");
            break;
        case pg_stat_activity_long:
            wprintw(window, "Show activity (age threshold: %s)", tab->pg_stat_activity_min_age);
            break;
        case pg_stat_functions:
            wprintw(window, "Show functions statistics");
            break;
        case pg_stat_statements_timing:
            wprintw(window, "Show pg_stat_statements timings");
            break;
        case pg_stat_statements_general:
            wprintw(window, "Show pg_stat_statements general");
            break;
        case pg_stat_statements_io:
            wprintw(window, "Show pg_stat_statements io");
            break;
        case pg_stat_statements_temp:
            wprintw(window, "Show pg_stat_statements temp");
            break;
        case pg_stat_statements_local:
            wprintw(window, "Show pg_stat_statements local io");
            break;
        case pg_stat_progress_vacuum:
            wprintw(window, "Show vacuum progress");
            break;
        default:
            break;
    }

    tab->current_context = context;
    if (res && *first_iter == false)
        PQclear(res);
    *first_iter = true;
}

/*
 ****************************************************************************
 * Change query age in the pg_stat_activity stat context.
 ****************************************************************************
 */
void change_min_age(WINDOW * window, struct tab_s * tab, PGresult *res, bool *first_iter)
{
    if (tab->current_context != pg_stat_activity_long) {
        wprintw(window, "Long query min age is not allowed here.");
        return;
    }

    unsigned int hour, min, sec;
    bool with_esc;
    char min_age[XS_BUF_LEN],
         msg[] = "Enter new min age, format: HH:MM:SS[.NN]: ";

    cmd_readline(window, msg, strlen(msg), &with_esc, min_age, sizeof(min_age), true);
    if (strlen(min_age) != 0 && with_esc == false) {
        if ((sscanf(min_age, "%u:%u:%u", &hour, &min, &sec)) == 0 || (hour > 23 || min > 59 || sec > 59)) {
            wprintw(window, "Nothing to do. Failed read or invalid value.");
        } else {
	    snprintf(tab->pg_stat_activity_min_age, sizeof(tab->pg_stat_activity_min_age), "%s", min_age);
        }
    } else if (strlen(min_age) == 0 && with_esc == false ) {
        wprintw(window, "Nothing to do. Leave min age %s", tab->pg_stat_activity_min_age);
    }
   
    PQclear(res);
    *first_iter = true;
}

/*
 ****************************************************************************
 * Clear connection options in specified tab.
 ****************************************************************************
 */
void clear_tab_connopts(struct tab_s * tabs[], unsigned int i)
{
    tabs[i]->host[0] = '\0';
    tabs[i]->port[0] = '\0';
    tabs[i]->user[0] = '\0';
    tabs[i]->dbname[0] = '\0';
    tabs[i]->password[0] = '\0';
    tabs[i]->conninfo[0] = '\0';
    tabs[i]->conn_used = false;
}

/*
 ****************************************************** key press function **
 * Open new connection in the new tab.
 ****************************************************************************
 */
unsigned int add_tab(WINDOW * window, struct tab_s * tabs[],
                PGconn * conns[], unsigned int tab_index)
{
    unsigned int i;
    char params[CONNINFO_MAXLEN],
         msg[] = "Enter new connection parameters, format \"host port username dbname\": ",
         msg2[] = "Required password: ";
    bool with_esc, with_esc2;
    
    for (i = 0; i < MAX_TABS; i++) {
        /* search free tab */
        if (tabs[i]->conn_used == false) {

            /* read user input */
            cmd_readline(window, msg, strlen(msg), &with_esc, params, sizeof(params), true);
            if (strlen(params) != 0 && with_esc == false) {
                /* parse user input */
                if ((sscanf(params, "%s %s %s %s",
                    tabs[i]->host,   tabs[i]->port,
                    tabs[i]->user,   tabs[i]->dbname)) == 0) {
                        wprintw(window, "Nothing to do. Failed read or invalid value.");
                        break;
                }
                /* setup tab conninfo settings */
                tabs[i]->conn_used = true;
		snprintf(tabs[i]->conninfo, sizeof(tabs[i]->conninfo),
			 "host=%s port=%s user=%s dbname=%s",
			 tabs[i]->host, tabs[i]->port,  tabs[i]->user, tabs[i]->dbname);

                /* establish new connection */
                conns[i] = PQconnectdb(tabs[i]->conninfo);
                /* if password required, ask user for password */
                if ( PQstatus(conns[i]) == CONNECTION_BAD && PQconnectionNeedsPassword(conns[i]) == 1) {
                    PQfinish(conns[i]);
                    wclear(window);

                    /* read password and add to conn options */
                    cmd_readline(window, msg2, strlen(msg2), &with_esc2, params, sizeof(params), false);
                    if (strlen(params) != 0 && with_esc2 == false) {
                        snprintf(tabs[i]->password, sizeof(tabs[i]->password), "%s", params);
			snprintf(tabs[i]->conninfo + strlen(tabs[i]->conninfo),
				 sizeof(tabs[i]->conninfo) - strlen(tabs[i]->conninfo), " password=%s", tabs[i]->password);
                        /* try establish connection and finish work */
                        conns[i] = PQconnectdb(tabs[i]->conninfo);
                        if ( PQstatus(conns[i]) == CONNECTION_BAD ) {
                            wclear(window);
                            wprintw(window, "Nothing to fo. Connection failed.");
                            PQfinish(conns[i]);
                            clear_tab_connopts(tabs, i);
                        } else {
                            wclear(window);
                            wprintw(window, "Successfully connected.");
                            tab_index = tabs[i]->tab;
                            get_pg_special(conns[i], tabs[i]);
                        }
                    } else if (with_esc) {
                        clear_tab_connopts(tabs, i);
                    }
                /* finish work if connection establish failed */
                } else if ( PQstatus(conns[i]) == CONNECTION_BAD ) {
                    wprintw(window, "Nothing to do. Connection failed.");
                    PQfinish(conns[i]);
                    clear_tab_connopts(tabs, i);
                /* if no error occured, print about success and finish work */
                } else {
                    wclear(window);
                    wprintw(window, "Successfully connected.");
                    tab_index = tabs[i]->tab;
                    get_pg_special(conns[i], tabs[i]);
                }
                break;
            /* finish work if user input empty or cancelled */
            } else if (strlen(params) == 0 && with_esc == false) {
                wprintw(window, "Nothing to do.");
                break;
            } else 
                break;
        /* also finish work if no available tabs */
        } else if (i == MAX_TABS - 1) {
            wprintw(window, "No free tabs.");
        }
    }

    return tab_index;
}

/*
 ****************************************************************************
 * Shift tabs when current tab closed.
 ****************************************************************************
 */
void shift_tabs(struct tab_s * tabs[], PGconn * conns[], unsigned int i)
{
    while (tabs[i + 1]->conn_used != false) {
        snprintf(tabs[i]->host, sizeof(tabs[i]->host), "%s", tabs[i + 1]->host);
        snprintf(tabs[i]->port, sizeof(tabs[i]->port), "%s", tabs[i + 1]->port);
        snprintf(tabs[i]->user, sizeof(tabs[i]->user), "%s", tabs[i + 1]->user);
        snprintf(tabs[i]->dbname, sizeof(tabs[i]->dbname), "%s", tabs[i + 1]->dbname);
        snprintf(tabs[i]->password, sizeof(tabs[i]->password), "%s", tabs[i + 1]->password);
        snprintf(tabs[i]->pg_special.pg_version_num, sizeof(tabs[i]->pg_special.pg_version_num), "%s",
		tabs[i + 1]->pg_special.pg_version_num);
        snprintf(tabs[i]->pg_special.pg_version, sizeof(tabs[i]->pg_special.pg_version), "%s",
		tabs[i + 1]->pg_special.pg_version);
        tabs[i]->subtab =        tabs[i + 1]->subtab;
        snprintf(tabs[i]->log_path, sizeof(tabs[i]->log_path), "%s", tabs[i + 1]->log_path);
        tabs[i]->log_fd =            tabs[i + 1]->log_fd;
        tabs[i]->current_context =   tabs[i + 1]->current_context;
        snprintf(tabs[i]->pg_stat_activity_min_age, sizeof(tabs[i]->pg_stat_activity_min_age), "%s",
		tabs[i + 1]->pg_stat_activity_min_age);
        tabs[i]->signal_options =    tabs[i + 1]->signal_options;
        tabs[i]->pg_stat_sys =       tabs[i + 1]->pg_stat_sys;

        conns[i] = conns[i + 1];
        i++;
        if (i == MAX_TABS - 1)
            break;
    }
    clear_tab_connopts(tabs, i);
}

/*
 ****************************************************************************
 * Close current tab, close connection and return index of the previous tab.
 ****************************************************************************
 */
unsigned int close_tab(WINDOW * window, struct tab_s * tabs[],
                PGconn * conns[], unsigned int tab_index, bool * first_iter)
{
    unsigned int i = tab_index;
    PQfinish(conns[tab_index]);

    wprintw(window, "Close current connection.");
    if (i == 0) {                               /* first active tab */
        if (tabs[i + 1]->conn_used) {
        shift_tabs(tabs, conns, i);
        } else {
            wrefresh(window);
            endwin();
            exit(EXIT_SUCCESS);
        }
    } else if (i == (MAX_TABS - 1)) {           /* last possible active tab */
        clear_tab_connopts(tabs, i);
        tab_index = tab_index - 1;
    } else {                                    /* middle active tab */
        if (tabs[i + 1]->conn_used) {
            shift_tabs(tabs, conns, i);
        } else {
            clear_tab_connopts(tabs, i);
            tab_index = tab_index - 1;
        }
    }

    *first_iter = true;
    return tab_index;
}

/*
 ****************************************************************************
 * Write info about opened connections into the ~/.pgcenterrc.
 ****************************************************************************
 */
void write_pgcenterrc(WINDOW * window, struct tab_s * tabs[], PGconn * conns[], struct args_s * args)
{
    unsigned int i = 0;
    FILE *fp;
    static char pgcenterrc_path[PATH_MAX];
    struct passwd *pw = getpwuid(getuid());
    struct stat statbuf;

    /* 
     * write conninfo into file which specified in --file=FILENAME,
     * or use default ~/.pgcenterrc
     */
    if (strlen(args->connfile) != 0)
        snprintf(pgcenterrc_path, sizeof(pgcenterrc_path), "%s", args->connfile);
    else {
        snprintf(pgcenterrc_path, sizeof(pgcenterrc_path), "%s/%s", pw->pw_dir, PGCENTERRC_FILE);
    }

    if ((fp = fopen(pgcenterrc_path, "w")) != NULL ) {
        for (i = 0; i < MAX_TABS; i++) {
            if (tabs[i]->conn_used) {
                fprintf(fp, "%s:%s:%s:%s:%s\n",
                        PQhost(conns[i]), PQport(conns[i]),
                        PQdb(conns[i]), PQuser(conns[i]),
                        PQpass(conns[i]));
            }
        }
        wprintw(window, "Wrote configuration to '%s'", pgcenterrc_path);

        fclose(fp);

        stat(pgcenterrc_path, &statbuf);
        if (statbuf.st_mode & (S_IRWXG | S_IRWXO)) {
            chmod(pgcenterrc_path, S_IRUSR|S_IWUSR);
        }
    } else {
        wprintw(window, "Failed to write configuration into '%s'", pgcenterrc_path);
    }
}

/*
 ****************************************************************************
 * Reload postgres
 ****************************************************************************
 */
void reload_conf(WINDOW * window, PGconn * conn)
{
    PGresult * res;
    bool with_esc;
    char errmsg[ERRSIZE],
         confirmation[1],
         msg[] = "Reload configuration files (y/n): ";

    cmd_readline(window, msg, strlen(msg), &with_esc, confirmation, 1, true);
    if (!strcmp(confirmation, "n") || !strcmp(confirmation, "N"))
        wprintw(window, "Do nothing. Canceled.");
    else if (!strcmp(confirmation, "y") || !strcmp(confirmation, "Y")) {
        res = do_query(conn, PG_RELOAD_CONF_QUERY, errmsg);
        if (res != NULL) {
            wprintw(window, "Reload issued.");
            PQclear(res);
        } else {
            wclear(window);
            wprintw(window, "Reload failed. %s", errmsg);
        }
    } else if ((strlen(confirmation) == 0) & (with_esc == false)) {
        wprintw(window, "Do nothing. Nothing entered.");
    } else if (with_esc) {
        ;
    } else 
        wprintw(window, "Do nothing. Not confirmed.");
}

/*
 ****************************************************************************
 * Get postgres listen_addresses and check is that local address or not.
 * Return true if listen_addresses is local and false if not.
 ****************************************************************************
 */
bool check_pg_listen_addr(struct tab_s * tab, PGconn * conn)
{
    /* an absoulute path means the unix socket is used and it is always local */
    if (!strncmp(tab->host, "/", 1)
	|| ((PQhost(conn)!= NULL) && !strncmp(PQhost(conn), "/", 1))
	|| (PQstatus(conn) == CONNECTION_OK && PQhost(conn) == NULL)) {
        return true;
    }

    struct ifaddrs *ifaddr, *ifa;
    int family, s;
    char host[NI_MAXHOST];
    
    if (getifaddrs(&ifaddr) == -1) {
        freeifaddrs(ifaddr);
        return false;
    }
    for (ifa = ifaddr; ifa != NULL; ifa = ifa->ifa_next) {
        if (ifa->ifa_addr == NULL)
            continue;

        family = ifa->ifa_addr->sa_family;

        /* Check AF_INET* interface addresses */
        if (family == AF_INET || family == AF_INET6) {
            s = getnameinfo(ifa->ifa_addr,
                            (family == AF_INET) ? sizeof(struct sockaddr_in) :
                                                  sizeof(struct sockaddr_in6),
                            host, NI_MAXHOST,
                            NULL, 0, NI_NUMERICHOST);
            if (s != 0) {
                mreport(false, msg_warning,
                        "WARNING: getnameinfo() failed: %s\n", gai_strerror(s));
                return false;
            }
            if (!strcmp(host, tab->host)) {
                freeifaddrs(ifaddr);
                return true;
                break;
            }
        }
    }

    freeifaddrs(ifaddr);
    return false;
}

/*
 ****************************************************************************
 * Edit configuration settings. Open configuration file in $EDITOR.
 ****************************************************************************
 */
void edit_config(WINDOW * window, struct tab_s * tab, PGconn * conn, const char * config_file_guc)
{
    static char config_path[PATH_MAX];
    pid_t pid;

    if (check_pg_listen_addr(tab, conn)) {
        get_conf_value(conn, config_file_guc, config_path);
        if (strlen(config_path) != 0) {
            /* if we want edit recovery.conf, attach config name to data_directory path */
            if (!strcmp(config_file_guc, GUC_DATA_DIRECTORY)) {
		snprintf(config_path + strlen(config_path), sizeof(config_path) - strlen(config_path), "/%s", PG_RECOVERY_FILE);
            }
            /* escape from ncurses mode */
            refresh();
            endwin();
            pid = fork();                   /* start child */
            if (pid == 0) {
                /* use editor from environment variables, otherwise use default editor */
                static char editor[S_BUF_LEN];
                if (getenv("EDITOR") != NULL)
                    snprintf(editor, sizeof(editor), "%s", getenv("EDITOR"));
                else
                    snprintf(editor, sizeof(editor), "%s", DEFAULT_EDITOR);
                execlp(editor, editor, config_path, NULL);
                exit(EXIT_FAILURE);
            } else if (pid < 0) {
                wprintw(window, "ERROR: fork failed, can't open %s", config_path);
                return;
            } else if (waitpid(pid, NULL, 0) != pid) {
                wprintw(window, "ERROR: waitpid failed.");
                return;
            }
        } else {
            wprintw(window, "Do nothing. Config option not found (not SUPERUSER?).");
        }
    } else {
        wprintw(window, "Do nothing. Edit config not supported for remote hosts.");
    }

    /* return to ncurses mode */
    refresh();
    return;
}

/*
 ****************************************************************************
 * Allocate memory for column attributes struct.
 ****************************************************************************
 */
struct colAttrs * init_colattrs(unsigned int n_cols) {
    struct colAttrs *cols;
    if ((cols = (struct colAttrs *) malloc(sizeof(struct colAttrs) * n_cols)) == NULL) {
        mreport(true, msg_fatal, "FATAL: malloc for column attributes failed.\n");
    }
    return cols;
}

/*
 ****************************************************************************
 * Calculate column width for output data.
 ****************************************************************************
 */
void calculate_width(struct colAttrs *columns, PGresult *res,
    struct tab_s * tab, char ***arr, unsigned int n_rows, unsigned int n_cols)
{
    unsigned int i, col, row;
    struct context_s ctx;

    /* determine current context */
    if (tab != NULL)
        for (i = 0; i < TOTAL_CONTEXTS; i++) {
            if (tab->current_context == tab->context_list[i].context)
                ctx = tab->context_list[i];
        }

    for (col = 0, i = 0; col < n_cols; col++, i++) {
        /* determine length of column names */
        if (strlen(ctx.fstrings[i]) > 0 && tab != NULL)
            /* mark columns with filtration */
            snprintf(columns[i].name, sizeof(columns[i].name), "%s*", PQfname(res, col));
        else
            snprintf(columns[i].name, sizeof(columns[i].name), "%s", PQfname(res, col));

        unsigned int width = strlen(PQfname(res, col));
        if (arr == NULL) {
            for (row = 0; row < n_rows; row++ ) {
                unsigned int val_len = strlen(PQgetvalue(res, row, col));
                if ( val_len >= width )
                    width = val_len;
            }
        } else {
            /* determine length of values from result array */
            for (row = 0; row < n_rows; row++ ) {
                unsigned int val_len = strlen(arr[row][col]);
                if ( val_len >= width )
                    width = val_len;
            }
        }
        /* set column width equal to longest value + 2 spaces */
        columns[i].width = width + 2;
    }
}

/*
 ****************************************************************************
 * Show postgres configuration settings.
 ****************************************************************************
 */
void show_config(WINDOW * window, PGconn * conn)
{
    unsigned int  row_count, col_count, row, col, i;
    FILE * fpout;
    PGresult * res;
    char errmsg[ERRSIZE],
         pager[S_BUF_LEN] = "";
    struct colAttrs * columns;

    if (getenv("PAGER") != NULL)
        snprintf(pager, sizeof(pager), "%s", getenv("PAGER"));
    else
        snprintf(pager, sizeof(pager), "%s", DEFAULT_PAGER);
    if ((fpout = popen(pager, "w")) == NULL) {
        wprintw(window, "Do nothing. Failed to open pipe to %s", pager);
        return;
    }

    /* escape from ncurses mode */
    refresh();
    endwin();

    res = do_query(conn, PG_SETTINGS_QUERY, errmsg);
    row_count = PQntuples(res);
    col_count = PQnfields(res);
    columns = init_colattrs(col_count);
    calculate_width(columns, res, NULL, NULL, row_count, col_count);
    
    fprintf(fpout, " PostgreSQL configuration: %i rows\n", row_count);
    /* print column names */
    for (col = 0, i = 0; col < col_count; col++, i++)
        fprintf(fpout, " %-*s", columns[i].width, PQfname(res, col));
    fprintf(fpout, "\n\n");
    /* print rows */
    for (row = 0; row < row_count; row++) {
        for (col = 0, i = 0; col < col_count; col++, i++)
            fprintf(fpout, " %-*s", columns[i].width, PQgetvalue(res, row, col));
        fprintf(fpout, "\n");
    }

    PQclear(res);
    pclose(fpout);
    free(columns);
    
    /* return to ncurses mode */
    refresh();
}

/*
 ****************************************************************************
 * Allocate memory for menu items.
 ****************************************************************************
 */
ITEM ** init_menuitems(unsigned int n_choices) {
    ITEM ** items;
    if ((items = (ITEM**) malloc(sizeof(ITEM *) * (n_choices))) == NULL) {
        mreport(true, msg_fatal, "FATAL: malloc for menu items failed.");
    }
    return items;
}

/*
 ****************************************************************************
 * Print the menu with list of config files (for further editing).
 ****************************************************************************
 */
void edit_config_menu(WINDOW * w_cmd, WINDOW * w_dba, struct tab_s * tab, PGconn * conn, bool *first_iter)
{
    const char * choices[] = { "postgresql.conf", "pg_hba.conf", "pg_ident.conf", "recovery.conf" };
    WINDOW *menu_win;
    MENU *menu;
    ITEM **items;
    unsigned int n_choices, i;
    int ch;
    bool done = false;

    cbreak();
    noecho();
    keypad(stdscr, TRUE);

    /* allocate stuff */
    n_choices = ARRAY_SIZE(choices);
    items = init_menuitems(n_choices + 1);
    for (i = 0; i < n_choices; i++)
        items[i] = new_item(choices[i], NULL);
    items[n_choices] = (ITEM *)NULL;
    menu = new_menu((ITEM **)items);

    /* construct menu, outer window for header and inner window for menu */
    menu_win = newwin(10,54,5,0);
    keypad(menu_win, TRUE);
    set_menu_win(menu, menu_win);
    set_menu_sub(menu, derwin(menu_win, 4,20,1,0));

    /* clear stuff from db answer window */
    wclear(w_dba);
    wrefresh(w_dba);
    /* print menu header */
    mvwprintw(menu_win, 0, 0, "Edit configuration file (Enter to edit, Esc to exit):");
    post_menu(menu);
    wrefresh(menu_win);
    
    while (1) {
        if (done)
            break;
        ch = wgetch(menu_win);
        switch (ch) {
            case KEY_DOWN:
                menu_driver(menu, REQ_DOWN_ITEM);
                break;
            case KEY_UP:
                menu_driver(menu, REQ_UP_ITEM);
                break;
            case 10:
                if (!strcmp(item_name(current_item(menu)), PG_CONF_FILE))
                    edit_config(w_cmd, tab, conn, GUC_CONFIG_FILE);
                else if (!strcmp(item_name(current_item(menu)), PG_HBA_FILE))
                    edit_config(w_cmd, tab, conn, GUC_HBA_FILE);
                else if (!strcmp(item_name(current_item(menu)), PG_IDENT_FILE))
                    edit_config(w_cmd, tab, conn, GUC_IDENT_FILE);
                else if (!strcmp(item_name(current_item(menu)), PG_RECOVERY_FILE))
                    edit_config(w_cmd, tab, conn, GUC_DATA_DIRECTORY);
                else
                    wprintw(w_cmd, "Do nothing. Unknown file.");     /* never should be here. */
                done = true;
                break;
            case 27:
                done = true;
                break;
        }       
    }
 
    /* clear menu items from tab */
    clear();
    refresh();

    /* free stuff */
    unpost_menu(menu);
    for (i = 0; i < n_choices; i++)
        free_item(items[i]);
    free_menu(menu);
    delwin(menu_win);
    *first_iter = true;
}

/*
 ****************************************************************************
 * Print the menu for pg_stat_statements stats contexts.
 ****************************************************************************
 */
void pgss_menu(WINDOW * w_cmd, WINDOW * w_dba, struct tab_s * tab, bool *first_iter)
{
    const char * choices[] = { 
	"pg_stat_statements timings",
	"pg_stat_statements general",
	"pg_stat_statements input/output",
	"pg_stat_statements temp input/output",
	"pg_stat_statements local input/output" };
    WINDOW *menu_win;
    MENU *menu;
    ITEM **items;
    unsigned int n_choices, i;
    int ch;
    bool done = false;

    cbreak();
    noecho();
    keypad(stdscr, TRUE);

    /* allocate stuff */
    n_choices = ARRAY_SIZE(choices);
    items = init_menuitems(n_choices + 1);
    for (i = 0; i < n_choices; i++)
        items[i] = new_item(choices[i], NULL);
    items[n_choices] = (ITEM *)NULL;
    menu = new_menu((ITEM **)items);

    /* construct menu, outer window for header and inner window for menu */
    menu_win = newwin(11,64,5,0);
    keypad(menu_win, TRUE);
    set_menu_win(menu, menu_win);
    set_menu_sub(menu, derwin(menu_win, 5,40,1,0));

    /* clear stuff from db answer window */
    wclear(w_dba);
    wrefresh(w_dba);
    /* print menu header */
    mvwprintw(menu_win, 0, 0, "Choose pg_stat_statements mode (Enter to choose, Esc to exit):");
    post_menu(menu);
    wrefresh(menu_win);
    
    while (1) {
        if (done)
            break;
        ch = wgetch(menu_win);
        switch (ch) {
            case KEY_DOWN:
                menu_driver(menu, REQ_DOWN_ITEM);
                break;
            case KEY_UP:
                menu_driver(menu, REQ_UP_ITEM);
                break;
            case 10:
                if (!strcmp(item_name(current_item(menu)), "pg_stat_statements timings"))
                    tab->current_context = pg_stat_statements_timing;
                else if (!strcmp(item_name(current_item(menu)), "pg_stat_statements general"))
                    tab->current_context = pg_stat_statements_general;
                else if (!strcmp(item_name(current_item(menu)), "pg_stat_statements input/output"))
                    tab->current_context = pg_stat_statements_io;
                else if (!strcmp(item_name(current_item(menu)), "pg_stat_statements temp input/output"))
                    tab->current_context = pg_stat_statements_temp;
                else if (!strcmp(item_name(current_item(menu)), "pg_stat_statements local input/output"))
                    tab->current_context = pg_stat_statements_local;
                else
                    wprintw(w_cmd, "Do nothing. Unknown mode.");     /* never should be here. */
                done = true;
                break;
            case 27:
                done = true;
                break;
        }       
    }
 
    /* clear menu items from tab */
    clear();
    refresh();

    /* free stuff */
    unpost_menu(menu);
    for (i = 0; i < n_choices; i++)
        free_item(items[i]);
    free_menu(menu);
    delwin(menu_win);
    *first_iter = true;
}

/*
 ****************************************************************************
 * Switch to next pg_stat_statements contexts.
 ****************************************************************************
 */
void pgss_switch(WINDOW * w_cmd, struct tab_s * tab, PGresult * p_res, bool *first_iter)
{
    /*
     * Check current context and switch to pg_stat_statements.
     * any -> pgss_timing -> pgss_general -> pgss_io -> pgss_temp -> pgss_local -> pgss_timing -> ...
     */
    switch (tab->current_context) {
	case pg_stat_statements_timing:
            switch_context(w_cmd, tab, pg_stat_statements_general, p_res, first_iter);
            break;
	case pg_stat_statements_general:
            switch_context(w_cmd, tab, pg_stat_statements_io, p_res, first_iter);
            break;
	case pg_stat_statements_io:
            switch_context(w_cmd, tab, pg_stat_statements_temp, p_res, first_iter);
            break;
	case pg_stat_statements_temp:
            switch_context(w_cmd, tab, pg_stat_statements_local, p_res, first_iter);
            break;
	case pg_stat_statements_local: default:
            switch_context(w_cmd, tab, pg_stat_statements_timing, p_res, first_iter);
            break;
    }
}

/*
 ****************************************************************************
 * Cancel query or terminate postgres backend using pid.
 ****************************************************************************
 */
void signal_single_backend(WINDOW * window, struct tab_s *tab, PGconn * conn, bool do_terminate)
{
    if (tab->current_context != pg_stat_activity_long) {
        wprintw(window, "Terminate or cancel backend allowed in long queries tab.");
        return;
    } 

    char errmsg[ERRSIZE],
         query[QUERY_MAXLEN],
         msg[S_BUF_LEN],
         pid[6];
    char * actions[] = { "Terminate", "Cancel" };
    int actions_idx;
    PGresult * res;
    bool with_esc;

    if (do_terminate) {
        actions_idx = 0;	/* Terminate */
        snprintf(msg, sizeof(msg), "Terminate single backend, enter pid: ");
    } else {
        actions_idx = 1;	/* Cancel */
        snprintf(msg, sizeof(msg), "Cancel single backend, enter pid: ");
    }

    cmd_readline(window, msg, strlen(msg), &with_esc, pid, sizeof(pid), true);
    if (atoi(pid) > 0) {
        if (do_terminate) {
	    snprintf(query, sizeof(query), "%s%s%s", PG_TERM_BACKEND_P1, pid, PG_TERM_BACKEND_P2);
        } else {
	    snprintf(query, sizeof(query), "%s%s%s", PG_CANCEL_BACKEND_P1, pid, PG_CANCEL_BACKEND_P2);
        }

        res = do_query(conn, query, errmsg);
        if (res != NULL) {
            wprintw(window, "%s backend with pid %s.", actions[actions_idx], pid);
            PQclear(res);
        } else {
            wprintw(window, "%s backend failed. %s", actions[actions_idx], errmsg);
        }
    } else if (strlen(pid) == 0 && with_esc == false) {
        wprintw(window, "Do nothing. Nothing entered.");
    } else if (with_esc) {
        ;
    } else
        wprintw(window, "Do nothing. Incorrect input value.");
}

/*
 ****************************************************************************
 * Print current mask for group cancel/terminate.
 ****************************************************************************
 */
void get_statemask(WINDOW * window, struct tab_s * tab)
{
    if (tab->current_context != pg_stat_activity_long) {
        wprintw(window, "Current mask can viewed in activity tab.");
        return;
    }

    wprintw(window, "Mask: ");
    if (tab->signal_options == 0)
        wprintw(window, "empty");
    if (tab->signal_options & GROUP_ACTIVE)
        wprintw(window, "active ");
    if (tab->signal_options & GROUP_IDLE)
        wprintw(window, "idle ");
    if (tab->signal_options & GROUP_IDLE_IN_XACT)
        wprintw(window, "idle in xact ");
    if (tab->signal_options & GROUP_WAITING)
        wprintw(window, "waiting ");
    if (tab->signal_options & GROUP_OTHER)
        wprintw(window, "other ");
}

/*
 ****************************************************************************
 * Set new state mask for group cancel/terminate.
 ****************************************************************************
 */
void set_statemask(WINDOW * window, struct tab_s * tab)
{
    if (tab->current_context != pg_stat_activity_long) {
        wprintw(window, "State mask setup allowed in activity tab.");
        return;
    } 

    unsigned int i;
    char mask[5] = "",
         msg[] = "";        /* set empty message, we don't want show msg from cmd_readline */
    bool with_esc;

    wprintw(window, "Set state mask for group backends [");
    wattron(window, A_BOLD | A_UNDERLINE);
    wprintw(window, "a");
    wattroff(window, A_BOLD | A_UNDERLINE);
    wprintw(window, "ctive/");
    wattron(window, A_BOLD | A_UNDERLINE);
    wprintw(window, "i");
    wattroff(window, A_BOLD | A_UNDERLINE);
    wprintw(window, "dle/idle_in_");
    wattron(window, A_BOLD | A_UNDERLINE);
    wprintw(window, "x");
    wattroff(window, A_BOLD | A_UNDERLINE);
    wprintw(window, "act/");
    wattron(window, A_BOLD | A_UNDERLINE);
    wprintw(window, "w");
    wattroff(window, A_BOLD | A_UNDERLINE);
    wprintw(window, "aiting/");
    wattron(window, A_BOLD | A_UNDERLINE);
    wprintw(window, "o");
    wattroff(window, A_BOLD | A_UNDERLINE);
    wprintw(window, "ther]: ");

    /* use offset 77 that equals message constructed above and printed by ncurses */
    cmd_readline(window, msg, 77, &with_esc, mask, sizeof(mask), true);
    if (strlen(mask) == 0 && with_esc == false) {           /* mask not entered */
        wprintw(window, "Do nothing. Mask not specified.");
    } else if (with_esc) {                                  /* user escaped */
        ;			/* do nothing here, info message will be printed by cmd_readline */
    } else {                                                /* user enter string with valid length */
        /* reset previous mask */
        tab->signal_options = 0;
        for (i = 0; i < strlen(mask); i++) {
            switch (mask[i]) {
                case 'a':
                    tab->signal_options |= GROUP_ACTIVE;
                    break;
                case 'i':
                    tab->signal_options |= GROUP_IDLE;
                    break;
                case 'x':
                    tab->signal_options |= GROUP_IDLE_IN_XACT;
                    break;
                case 'w':
                    tab->signal_options |= GROUP_WAITING;
                    break;
                case 'o':
                    tab->signal_options |= GROUP_OTHER;
                    break;
            }
        }
        get_statemask(window, tab);
    }
}

/*
 ****************************************************************************
 * Cancel queries or terminate postgres backends using state mask.
 ****************************************************************************
 */
void signal_group_backend(WINDOW * window, struct tab_s *tab, PGconn * conn, bool do_terminate)
{
    if (tab->current_context != pg_stat_activity_long) {
        wprintw(window, "Terminate or cancel backend allowed in long queries tab.");
        return;
    } 
    if (tab->signal_options == 0) {
        wprintw(window, "Do nothing. Mask not specified.");
        return;
    }

    char query[QUERY_MAXLEN],
         mask[5] = "",
         state[M_BUF_LEN];
    const char * actions[] = { "terminate", "cancel" };
    PGresult * res;
    unsigned int i, actions_idx, signaled = 0;

    if (do_terminate)
	actions_idx = 0;		/* terminate */
    else
        actions_idx = 1;		/* cancel */
    
    if (tab->signal_options & GROUP_ACTIVE)
        strncat(mask, "a", 1);
    if (tab->signal_options & GROUP_IDLE)
        strncat(mask, "i", 1);
    if (tab->signal_options & GROUP_IDLE_IN_XACT)
        strncat(mask, "x", 1);
    if (tab->signal_options & GROUP_WAITING)
        strncat(mask, "w", 1);
    if (tab->signal_options & GROUP_OTHER)
        strncat(mask, "o", 1);

    for (i = 0; i < strlen(mask); i++) {
        switch (mask[i]) {
            case 'a':
                snprintf(state, sizeof(state), "state = 'active'");
                break;
            case 'i':
                snprintf(state, sizeof(state), "state = 'idle'");
                break;
            case 'x':
                snprintf(state, sizeof(state), "state IN ('idle in transaction (aborted)', 'idle in transaction')");
                break;
            case 'w':
		if (atoi(tab->pg_special.pg_version_num) < PG96)
	            snprintf(state, sizeof(state), "waiting");
		else 
	            snprintf(state, sizeof(state), "wait_event IS NOT NULL OR wait_event_type IS NOT NULL");
                break;
            case 'o':
                snprintf(state, sizeof(state), "state IN ('fastpath function call', 'disabled')");
                break;
            default:
                break;
        }
	snprintf(query, sizeof(query), "%s%s%s%s%s%s%s%s%s",
		 PG_SIG_GROUP_BACKEND_P1, actions[actions_idx],
		 PG_SIG_GROUP_BACKEND_P2, state,
		 PG_SIG_GROUP_BACKEND_P3, tab->pg_stat_activity_min_age,
		 PG_SIG_GROUP_BACKEND_P4, tab->pg_stat_activity_min_age,
		 PG_SIG_GROUP_BACKEND_P5);
        
        char errmsg[ERRSIZE];
        res = do_query(conn, query, errmsg);
        signaled = signaled + PQntuples(res);
        PQclear(res);
    }

    if (do_terminate)
        wprintw(window, "Terminated %i processes.", signaled);
    else
        wprintw(window, "Canceled %i processes.", signaled);
}

/*
 ****************************************************************************
 * Start psql using current tab connection options.
 ****************************************************************************
 */
void start_psql(WINDOW * window, struct tab_s * tab)
{
    pid_t pid;
    char psql[XS_BUF_LEN] = DEFAULT_PSQL;

    /* escape from ncurses mode */
    refresh();
    endwin();
    /* ignore Ctrl+C in child when psql running */
    signal(SIGINT, SIG_IGN);
    pid = fork();                   /* start child */
    if (pid == 0) {
        execlp(psql, psql,
                "-h", tab->host,
                "-p", tab->port,
                "-U", tab->user,
                "-d", tab->dbname,
                NULL);
        exit(EXIT_SUCCESS);         /* finish child */
    } else if (pid < 0) {
        wprintw(window, "ERROR: fork failed, can't exec %s.", psql);
    } else if (waitpid(pid, NULL, 0) != pid) {
        wprintw(window, "ERROR: waitpid failed.");
    }

    /* 
     * Reinit signals handling. After psql session, pgcenter may not reset terminal properly.
     */
    signal(SIGINT, SIG_DFL);
    init_signal_handlers();

    /* return to ncurses mode */
    refresh();
}

/*
 ****************************************************************************
 * Change refresh interval.
 ****************************************************************************
 */
unsigned long change_refresh(WINDOW * window, unsigned long interval)
{
    unsigned long interval_save = interval;
    static char msg[S_BUF_LEN],                 /* prompt */
                str[XS_BUF_LEN];                /* entered value */
    bool with_esc;
    unsigned int offset = 0;			/* additional offset for message */

    wprintw(window, "Change refresh (min 1, max 300, current %i) to ", interval / 1000000);
    wrefresh(window);

    /* calculate additional offset equals number of digits in interval */
    while ((interval / 1000000) != 0) {
      interval /= 10;
      ++offset;
    }
    /* restore current interval */
    interval = interval_save;

    /* use offset 45 that equals message constructed above and printed by ncurses */
    cmd_readline(window, msg, 45 + offset, &with_esc, str, sizeof(str), true);

    if (strlen(str) != 0 && with_esc == false) {
        interval = atol(str);
        if (interval < 1) {
            wprintw(window, "Should not be less than 1 second.");
            interval = interval_save;
        } else if (interval * 1000000 > INTERVAL_MAXLEN) {
            wprintw(window, "Should not be more than 300 seconds.");
            interval = INTERVAL_MAXLEN;
        } else {
            interval = interval * 1000000;
        }
    } else if (strlen(str) == 0 && with_esc == false ) {
        wprintw(window, "Leave old value: %i seconds.", interval_save / 1000000);
        interval = interval_save;
    }

    return interval;
}

/*
 ****************************************************************************
 * Set pgcenter on pause.
 ****************************************************************************
 */
void do_noop(WINDOW * window, unsigned long interval)
{
    bool paused = true;
    unsigned int sleep_usec;
    int ch;

    while (paused != false) {
        wprintw(window, "Pause, press any key to resume.");
        wrefresh(window);
        /* sleep */
        for (sleep_usec = 0; sleep_usec < interval; sleep_usec += INTERVAL_STEP) {
            if ((ch = getch()) != ERR) {
                paused = false;
                break;
            } else {
                usleep(INTERVAL_STEP);
                if (interval > DEFAULT_INTERVAL && sleep_usec == DEFAULT_INTERVAL) {
                    wrefresh(window);
                    wclear(window);
                }
            }
        }
        wclear(window);
    }
}

/*
 ****************************************************************************
 * Toggle on/off displaying content from system views
 ****************************************************************************
 */
void system_view_toggle(WINDOW * window, struct tab_s * tab, bool * first_iter)
{
    tab->pg_stat_sys ^= 1;
    if (tab->pg_stat_sys)
        wprintw(window, "Show system tables: on");
    else
        wprintw(window, "Show system tables: off");

    *first_iter = true;
}

/*
 ****************************************************************************
 * Get postgresql logfile path
 ****************************************************************************
 */
void get_logfile_path(char * path, PGconn * conn)
{
    PGresult *res;
    char errmsg[ERRSIZE],
         q1[] = "show data_directory",
         q2[] = "show log_directory",
         q3[] = "show log_filename",
         q4[] = "select to_char(pg_postmaster_start_time(), 'HH24MISS')",
         logdir[PATH_MAX], logfile[NAME_MAX], datadir[PATH_MAX],
         path_tpl[PATH_MAX + NAME_MAX],
         path_log[PATH_MAX + NAME_MAX],
         path_log_fallback[PATH_MAX + NAME_MAX] = "";

    path[0] = '\0';
    if ((res = do_query(conn, q2, errmsg)) == NULL) {
        PQclear(res);
        return;
    }
    snprintf(logdir, sizeof(logdir), "%s", PQgetvalue(res, 0, 0));
    PQclear(res);

    if ( logdir[0] != '/' ) {
        if ((res = do_query(conn, q1, errmsg)) == NULL) {
            PQclear(res);
            return;
        }
	snprintf(datadir, sizeof(datadir), "%s", PQgetvalue(res, 0, 0));
	snprintf(path_tpl, sizeof(path_tpl), "%s/%s/", datadir, logdir);
        PQclear(res);
    } else {
	snprintf(path_tpl, sizeof(path_tpl), "%s/", logdir);
    }

    if ((res = do_query(conn, q3, errmsg)) == NULL) {
        PQclear(res);
        return;
    }
    snprintf(logfile, sizeof(logfile), "%s", PQgetvalue(res, 0, 0));
    snprintf(path_tpl + strlen(path_tpl), sizeof(path_tpl) - strlen(path_tpl), "%s", logfile);
    PQclear(res);
    
    /*
     * PostgreSQL defaults for log_filename is postgresql-%Y-%m-%d_%H%M%S.log. 
     * It can be issue, because log file named by this template on postamster startup.
     * Therefore we must know postmaster startup time for determining real log name.
     * But, on log rotation, new log name get _000000.log suffix. Thus log can have two names:
     * _123456.log (example) or _000000.log. If first log doesn't exist we try to use _000000.log.
     */
    /* check that the log_filename have %H%M%S pattern */
    if (strstr(path_tpl, "%H%M%S") != NULL) {
        snprintf(path_log, sizeof(path_log), "%s", path_tpl);
        snprintf(path_log_fallback, sizeof(path_log_fallback), "%s", path_tpl);
        if((res = do_query(conn, q4, errmsg)) == NULL) {
            PQclear(res);
            return;
        }
        strrpl(path_log, "%H%M%S", PQgetvalue(res, 0, 0), sizeof(path_log));
        strrpl(path_log_fallback, "%H%M%S", "000000", sizeof(path_log_fallback));
        PQclear(res);
    } else {
        snprintf(path_log, sizeof(path_log), "%s", path_tpl);
    }

    /* translate log_filename pattern string in real path */
    time_t rawtime;
    struct tm *info;

    time( &rawtime );
    info = localtime( &rawtime );
    strftime(path, PATH_MAX, path_log, info);

    /* if file exists, return path */
    if (access(path, F_OK ) != -1 ) {
        return;
    } 
    
    /* if previous condition failed, try use _000000.log name */
    if (strlen(path_log_fallback) != 0) {
        strftime(path, PATH_MAX, path_log_fallback, info);
        return;
    } else {
        path[0] = '\0';
        return;
    }
}

/*
 ****************************************************************************
 * Aux stats managing. Open iostat/nicstat/logtail.
 ****************************************************************************
 */
void subtab_process(WINDOW * window, WINDOW ** w_sub, struct tab_s * tab, PGconn * conn, unsigned int subtab)
{
    if (!tab->subtab_enabled) {
        /* open subtab */
        switch (subtab) {
            case SUBTAB_LOGTAIL:
                if (check_pg_listen_addr(tab, conn)) {
                    *w_sub = newwin(0, 0, ((LINES * 2) / 3), 0);
                    wrefresh(window);
                    /* get logfile path  */
                    get_logfile_path(tab->log_path, conn);
    
                    if (strlen(tab->log_path) == 0) {
                        wprintw(window, "Do nothing. Unable to determine log filename or no access permissions.");
                        return;
                    }
                    if ((tab->log_fd = open(tab->log_path, O_RDONLY)) == -1 ) {
                        wprintw(window, "Do nothing. Failed to open %s", tab->log_path);
                        return;
                    }
                    tab->subtab = SUBTAB_LOGTAIL;
                    tab->subtab_enabled = true;
                    wprintw(window, "Open postgresql log: %s", tab->log_path);
                    return;
                } else {
                    wprintw(window, "Do nothing. Log file view is not supported for remote hosts.");
                    return;
                }
                break;
            case SUBTAB_IOSTAT:
                if (access(DISKSTATS_FILE, R_OK) == -1) {
                    wprintw(window, "Do nothing. No access to %s.", DISKSTATS_FILE);
                    return;
                }
                wprintw(window, "Show iostat");
                *w_sub = newwin(0, 0, ((LINES * 2) / 3), 0);
                tab->subtab = SUBTAB_IOSTAT;
                tab->subtab_enabled = true;
                break;
            case SUBTAB_NICSTAT:
                if (access(NETDEV_FILE, R_OK) == -1) {
                    wprintw(window, "Do nothing. No access to %s.", NETDEV_FILE);
                    return;
                }
                wprintw(window, "Show nicstat");
                *w_sub = newwin(0, 0, ((LINES * 2) / 3), 0);
                tab->subtab = SUBTAB_NICSTAT;
                tab->subtab_enabled = true;
                break;
            case SUBTAB_NONE:
                tab->subtab = SUBTAB_NONE;
                tab->subtab_enabled = false;
        }
    } else {
        /* close subtab */
        wclear(*w_sub);
        wrefresh(*w_sub);
        if (tab->log_fd > 0)
            close(tab->log_fd);
        tab->subtab = SUBTAB_NONE;
        tab->subtab_enabled = false;
        return;
    }
}

/*
 ****************************************************************************
 * Tail postgresql log in aux stat area.
 ****************************************************************************
 */
void print_log(WINDOW * window, WINDOW * w_cmd, struct tab_s * tab, PGconn * conn)
{
    unsigned int x, y;                                          /* window coordinates */
    unsigned int n_lines = 1, n_cols = 1;                       /* number of rows and columns for printing */
    struct stat stats;                                          /* file stat struct */
    off_t end_pos;                                              /* end of file position */
    off_t pos;                                                  /* from this position start read of file */
    size_t bytes_read;                                          /* bytes read from file to buffer */
    char buffer[XXXL_BUF_LEN] = "";                               /* init empty buffer */
    unsigned int i, nl_count = 0, len, scan_pos;                /* iterator, newline counter, buffer length, in-buffer scan position */
    char *nl_ptr;                                               /* in-buffer newline pointer */

    getbegyx(window, y, x);                                     /* get window coordinates */
    /* calculate number of rows for log tailing, 2 is the number of lines for tab header */
    n_lines = LINES - y - 2;                                    /* calculate number of rows for log tailing */
    n_cols = COLS - x - 1;                                      /* calculate number of chars in row for cutting multiline log entries */
    wclear(window);                                             /* clear log window */

    if ((fstat(tab->log_fd, &stats)) == -1) {
	wprintw(w_cmd, "Failed to stat %s", tab->log_path);
	wrefresh(w_cmd);
	return;
    }
    if (S_ISREG (stats.st_mode) && stats.st_size != 0) {        /* log should be a non-empty regular file */
        end_pos = lseek(tab->log_fd, 0, SEEK_END);           /* get end of file position */
        pos = end_pos;                                          /* set position to the end of file */
        bytes_read = XXXL_BUF_LEN;                                /* read with 8KB block */
        if (end_pos < XXXL_BUF_LEN)                               /* if end file pos less than buffer */
            pos = 0;                                            /* than set read position to the begin of file */
        else                                                    /* if end file pos more than buffer */
            pos = pos - bytes_read;                             /* than set read position into end of file minus buffer size */
        lseek(tab->log_fd, pos, SEEK_SET);                       /* set determined position in file */
        bytes_read = read(tab->log_fd, buffer, bytes_read);      /* read file to the buffer */

        len = strlen(buffer);                                   /* determine the buffer length */
        scan_pos = len;                                         /* set in-buffer scan position equal buffer length */

        /* print header */
        wattron(window, A_BOLD);
        wprintw(window, "\ntail %s\n", tab->log_path);
        wattroff(window, A_BOLD);

        for (i = 0; i < len; i++)                               /* get number of newlines in the buffer */
            if (buffer[i] == '\n')
                nl_count++;
        if (n_lines > nl_count) {                               /* if number of newlines less than required */
            wprintw(window, "%s", buffer);                      /* then print out buffer content */
            wrefresh(window);
            return;                                             /* and finish work */
        }

        /*
         * at this place, log size more than buffersize, fill the buffer 
         * and find \n position from which start printing.
         */
        unsigned int n_lines_save = n_lines;                    /* save number of lines need for tail. */
        do {
            nl_ptr = memrchr(buffer, '\n', scan_pos);           /* find \n from scan_pos */
            if (nl_ptr != NULL) {                               /* if found */
                scan_pos = (nl_ptr - buffer);                   /* remember this place */
            } else {                                            /* if not found */
                break;                                          /* finish work */
            }
            n_lines--;                                          /* after each iteration decrement the line counter */
        } while (n_lines != 0);                                 /* stop cycle when line counter equal zero - we found need amount of lines */

        /* now we should cut multiline log entries to tab length */
        char str[n_cols];                                       /* use var for one line */
        char tmp[XXXL_BUF_LEN];                                   /* tmp var for line from buffer */
        do {                                                    /* scan buffer from begin */
            nl_ptr = strstr(buffer, "\n");                      /* find \n in buffer */
            if (nl_ptr != NULL) {                               /* if found */
                if (nl_count > n_lines_save) {                              /* and if lines too much, skip them */
                    snprintf(buffer, sizeof(buffer), "%s", nl_ptr + 1);     /* decrease buffer, cut skipped line */
                    nl_count--;                                             /* decrease newline counter */
                    continue;                                   /* start next iteration */
                }                                                       /* at this place we have sufficient number of lines for tail */
                snprintf(tmp, nl_ptr - buffer + 1, "%s", buffer);       /* copy log line into temp buffer */
                if (strlen(tmp) > n_cols) {                             /* if line longer than tab size (multiline), truncate line to tab size */
                    snprintf(str, n_cols - 4, "%s", buffer);
                } else {                                                /* if line have normal size, copy line as is */
                    snprintf(str, strlen(tmp) + 1, "%s", buffer);
                }
                wprintw(window, "%s\n", str);                           /* print line to log tab */
                snprintf(buffer, sizeof(buffer), "%s", nl_ptr + 1);     /* decrease buffer, cut printed line */
            } else {
                break;                                                  /* if \n not found, finish work */
            }
            n_lines++;                                                  /* after each iteration, increase newline counter */
        } while (n_lines != n_lines_save);                              /* print lines until newline counter not equal saved newline counter */
    } else {
        wprintw(w_cmd, "Do nothing. Log is not a regular file or empty.");     /* if file not regular or empty */
        subtab_process(w_cmd, &window, tab, conn, SUBTAB_NONE);    /* close log file and log tab */
    }
    
    wrefresh(window);
}

/*
 ****************************************************************************
 * Open postrges log in $PAGER.
 ****************************************************************************
 */
void show_full_log(WINDOW * window, struct tab_s * tab, PGconn * conn)
{
    pid_t pid;

    if (check_pg_listen_addr(tab, conn)) {
        /* get logfile path  */
        get_logfile_path(tab->log_path, conn);
        if (strlen(tab->log_path) != 0) {
            /* escape from ncurses mode */
            refresh();
            endwin();
            pid = fork();                   /* start child */
            if (pid == 0) {
                /* get pager from environment variables, otherwise use default pager */
                static char pager[S_BUF_LEN];
                if (getenv("PAGER") != NULL)
                    snprintf(pager, sizeof(pager), "%s", getenv("PAGER"));
                else
                    snprintf(pager, sizeof(pager), "%s", DEFAULT_PAGER);
                execlp(pager, pager, tab->log_path, NULL);
                exit(EXIT_SUCCESS);
            } else if (pid < 0) {
                wprintw(window, "ERROR: fork failed, can't open %s.", tab->log_path);
                return;
            } else if (waitpid(pid, NULL, 0) != pid) {
                wprintw(window, "ERROR: waitpid failed.");
                return;
            }
        } else {
            wprintw(window, "Do nothing. Unable to determine log filename (not SUPERUSER?) or no access permissions.");
        }
    } else {
        wprintw(window, "Do nothing. Log file viewing is not supported for remote hosts.");
    }

    /* return to ncurses mode */
    refresh();
    return;
}

/*
 ****************************************************************************
 * Reset postres stats counters
 ****************************************************************************
 */
void pg_stat_reset(WINDOW * window, PGconn * conn, bool * reseted)
{
    char errmsg[ERRSIZE];
    PGresult * res;

    if ((res = do_query(conn, PG_STAT_RESET_QUERY, errmsg)) != NULL) {
        wprintw(window, "Reset statistics");
        *reseted = true;
    } else
        wprintw(window, "Reset statistics failed: %s", errmsg);
    
    PQclear(res);
}

/*
 ****************************************************************************
 * Get query text using pseudo pg_stat_statements.queryid.
 ****************************************************************************
 */
void get_query_by_id(WINDOW * window, struct tab_s * tab, PGconn * conn)
{
    if (tab->current_context != pg_stat_statements_timing
            && tab->current_context != pg_stat_statements_general
            && tab->current_context != pg_stat_statements_io
            && tab->current_context != pg_stat_statements_temp
            && tab->current_context != pg_stat_statements_local) {
        wprintw(window, "Get query text is not allowed here.");
        return;
    }
    
    PGresult * res;
    bool with_esc;
    char msg[] = "Enter queryid: ",
         query[QUERY_MAXLEN],
         pager[M_BUF_LEN] = "";
    char queryid[XS_BUF_LEN];
    char errmsg[ERRSIZE];
    FILE * fpout;

    cmd_readline(window, msg, strlen(msg), &with_esc, queryid, sizeof(queryid), true);
    if (check_string(queryid, is_alfanum) == -1) {
        wprintw(window, "Do nothing. Value is not valid.");
        return;
    }

    if (strlen(queryid) != 0 && with_esc == false) {
        /* do query and send result into less */
	snprintf(query, sizeof(query), "%s%s%s", PG_GET_QUERYREP_BY_QUERYID_QUERY_P1, queryid, PG_GET_QUERYREP_BY_QUERYID_QUERY_P2);
        if ((res = do_query(conn, query, errmsg)) == NULL) {
            wprintw(window, "%s", errmsg);
            return;
        }

        /* finish work if empty answer */
        if (PQntuples(res) == 0) {
            wprintw(window, "Do nothing. Empty answer for %s", queryid);
            PQclear(res);
            return;
        }

        if (getenv("PAGER") != NULL)
            snprintf(pager, sizeof(pager), "%s", getenv("PAGER"));
        else
            snprintf(pager, sizeof(pager), "%s", DEFAULT_PAGER);
        
        if ((fpout = popen(pager, "w")) == NULL) {
            wprintw(window, "Do nothing. Failed to open pipe to %s", pager);
            return;
        }

        /* escape from ncurses mode */
        refresh();
        endwin();

        /* print result */
        fprintf(fpout, "summary:\n\ttotal_time: %s, cpu_time: %s, io_time: %s (ALL: %s%%, CPU: %s%%, IO: %s%%),\ttotal queries: %s\n\
query info:\n\
\tusename:\t\t\t\t%s,\n\
\tdatname:\t\t\t\t%s,\n\
\tcalls (relative to all queries):\t%s (%s%%),\n\
\trows (relative to all queries):\t\t%s (%s%%),\n\
\ttotal time (relative to all queries):\t%s (ALL: %s%%, CPU: %s%%, IO: %s%%),\n\
\taverage time (only for this query):\t%sms, cpu_time: %sms, io_time: %sms, (ALL: %s%%, CPU: %s%%, IO: %s%%),\n\n\
query text (id: %s):\n%s",
        /* summary */
        PQgetvalue(res, 0, REP_ALL_TOTAL_TIME), PQgetvalue(res, 0, REP_ALL_CPU_TIME), PQgetvalue(res, 0, REP_ALL_IO_TIME),
        PQgetvalue(res, 0, REP_ALL_TOTAL_TIME_PCT), PQgetvalue(res, 0, REP_ALL_CPU_TIME_PCT), PQgetvalue(res, 0, REP_ALL_IO_TIME_PCT), 
        PQgetvalue(res, 0, REP_ALL_TOTAL_QUERIES),
        /* user, dbname */
        PQgetvalue(res, 0, REP_USER), PQgetvalue(res, 0, REP_DBNAME),
        /* calls and rows */
        PQgetvalue(res, 0, REP_CALLS), PQgetvalue(res, 0, REP_CALLS_PCT),
        PQgetvalue(res, 0, REP_ROWS), PQgetvalue(res, 0, REP_ROWS_PCT),
        /* timings */
        PQgetvalue(res, 0, REP_TOTAL_TIME), PQgetvalue(res, 0, REP_TOTAL_TIME_PCT), PQgetvalue(res, 0, REP_CPU_TIME_PCT), PQgetvalue(res, 0, REP_IO_TIME_PCT),
        /* averages */
        PQgetvalue(res, 0, REP_AVG_TIME), PQgetvalue(res, 0, REP_AVG_CPU_TIME), PQgetvalue(res, 0, REP_AVG_IO_TIME), 
        PQgetvalue(res, 0, REP_AVG_TIME_PCT), PQgetvalue(res, 0, REP_AVG_CPU_TIME_PCT), PQgetvalue(res, 0, REP_AVG_IO_TIME_PCT),
        /* query */
        queryid, PQgetvalue(res, 0, REP_QUERY));

        /* clean */
        PQclear(res);
        pclose(fpout);

        /* return to ncurses mode */
        refresh();
    } else if (strlen(queryid) == 0 && with_esc == false) {
        wprintw(window, "Nothing to do. Nothing entered");
    } else if (with_esc) {
        ;
    } else {
        wprintw(window, "Nothing to do.");
    }
}

/*
 ****************************************************************************
 * Print internal help about color-change tab.
 * @ws_color            Sysstat window current color.
 * @wc_color            Cmdline window current color.
 * @wa_color            Database answer window current color.
 * @wl_color            Subtab window current color.
 ****************************************************************************
 */
void draw_color_help(WINDOW * w,
        unsigned long long int * ws_color, unsigned long long int * wc_color, unsigned long long int * wa_color,
        unsigned long long int * wl_color, unsigned long long int target, unsigned long long int * target_color)
{
    wclear(w);
    wprintw(w, "Help for color mapping - %s, version %.1f.%d\n\n",
            PROGRAM_NAME, PROGRAM_VERSION, PROGRAM_RELEASE);
    wattron(w, COLOR_PAIR(*ws_color));
    wprintw(w, "\tpgcenter: 2015-08-03 16:12:16, load average: 0.54, 0.43, 0.41\n\
\t    %%cpu:  4.8 us,  5.0 sy,  0.0 ni, 90.2 id,  0.0 wa,  0.0 hi,  0.0 si,  \n\
\t  conn 1: 127.0.0.1:5432 postgres@pgbench        conn state: ok\n\
\tactivity:  9 total,  8 idle,  0 idle_in_xact,  1 active,  0 waiting,\n");
    wattroff(w, COLOR_PAIR(*ws_color));

    wattron(w, COLOR_PAIR(*wc_color));
    wprintw(w, "\tNasty message or input prompt.\n");
    wattroff(w, COLOR_PAIR(*wc_color));

    wattron(w, COLOR_PAIR(*wa_color));
    wattron(w, A_BOLD);
    wprintw(w, "\tuser      database  calls  calls/s  total_time  read_time  write_time  cpu_\n");
    wattroff(w, A_BOLD);
    wprintw(w, "\tpostgres  pgbench   83523  3        9294.62     0.00       0.00        9294\n\
\tadmin     pgbench   24718  0        30731.86    28672.12   0.00        2059\n\n");
    wattroff(w, COLOR_PAIR(*wa_color));

    wattron(w, COLOR_PAIR(*wl_color));
    wprintw(w, "\t< 2015-08-03 16:17:55.848 YEKT >LOG:  database system is ready to accept co\n\
\t< 2015-08-03 16:17:55.848 YEKT >LOG:  autovacuum launcher started\n\n");
    wattroff(w, COLOR_PAIR(*wl_color));

    wprintw(w, "1) Select a target as an upper case letter, current target is  %c :\n\
\tS = Summary Data, M = Messages/Prompt, P = PostgreSQL Information, L = Additional tab\n", target);
    wprintw(w, "2) Select a color as a number, current color is  %i :\n\
\t0 = default,  1 = black,    2 = red,    3 = green,  4 = yellow,\n\
\t5 = blue,     6 = magenta,  7 = cyan,   8 = white\n", *target_color);
    wprintw(w, "3) Then use keys: 'Esc' to abort changes, 'Enter' to commit and end.\n");

    touchwin(w);
    wrefresh(w);
}

/*
 ****************************************************************************
 * Change output colors.
 * @ws_color            Sysstat window current color.
 * @wc_color            Cmdline window current color.
 * @wa_color            Database answer window current color.
 * @wl_color            Subtab window current color.
 ****************************************************************************
 */
void change_colors(unsigned long long int * ws_color, unsigned long long int * wc_color,
            unsigned long long int * wa_color, unsigned long long int * wl_color)
{
    WINDOW * w;
    int ch;
    unsigned long long int target = 'S',
        * target_color = ws_color;
    unsigned long long int ws_save = *ws_color,
        wc_save = *wc_color,
        wa_save = *wa_color,
        wl_save = *wl_color;
    
    w = subwin(stdscr, 0, 0, 0, 0);
    echo();
    cbreak();
    nodelay(w, FALSE);
    keypad(w, TRUE);

    do {
        draw_color_help(w, ws_color, wc_color, wa_color, wl_color, target, target_color);
        ch = wgetch(w);
        switch (ch) {
            case 'S':
                target = 'S';
                target_color = ws_color;
                break;
            case 'M':
                target = 'M';
                target_color = wc_color;
                break;
            case 'P':
                target = 'P';
                target_color = wa_color;
                break;
            case 'L':
                target = 'L';
                target_color = wl_color;
                break;
            case '0': case '1': case '2': case '3':
            case '4': case '5': case '6': case '7': case '8':
                *target_color = ch - '0';
                break;
            default:
                break;
        }
    } while (ch != '\n' && ch != 27);

    /* if Esc entered, restore previous colors */
    if (ch == 27) {
        *ws_color = ws_save;
        *wc_color = wc_save;
        *wa_color = wa_save;
        *wl_color = wl_save;
    }

    noecho();
    cbreak();
    nodelay(w, TRUE);
    keypad(w, FALSE);
    delwin(w);
}
