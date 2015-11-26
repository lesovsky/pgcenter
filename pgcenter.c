/*
 * pgcenter: adminitrative console for PostgreSQL.
 * (C) 2015 by Alexey V. Lesovsky (lesovsky <at> gmail.com)
 */

#define _GNU_SOURCE
#include <ctype.h>
#include <errno.h>
#include <fcntl.h>
#include <getopt.h>
#include <limits.h>
#include <menu.h>
#include <ncurses.h>
#include <netdb.h>
#include <ifaddrs.h>
#include <pwd.h>
#include <signal.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/stat.h>
#include <sys/types.h>
#include <sys/wait.h>
#include <termios.h>
#include <time.h>
#include <unistd.h>
#include "libpq-fe.h"
#include "pgcenter.h"
#include "qstats.h"

/*
 ******************************************************** startup function **
 * Print usage.
 ****************************************************************************
 */
void print_usage(void)
{
    printf("%s is the adminitrative console for PostgreSQL.\n\n", PROGRAM_NAME);
    printf("Usage:\n \
  %s [OPTION]... [DBNAME [USERNAME]]\n\n", PROGRAM_NAME);
    printf("General options:\n \
  -?, --help                show this help, then exit.\n \
  -V, --version             print version, then exit.\n\n");
    printf("Options:\n \
  -h, --host=HOSTNAME       database server host or socket directory\n \
  -p, --port=PORT           database server port (default: \"5432\")\n \
  -U, --username=USERNAME   database user name (default: \"current user\")\n \
  -d, --dbname=DBNAME       database name (default: \"current user\")\n \
  -f, --file=FILENAME       conninfo file (default: \"~/.pgcenterrc\")\n \
  -w, --no-password         never prompt for password\n \
  -W, --password            force password prompt (should happen automatically)\n\n");
    printf("Report bugs to %s.\n", PROGRAM_AUTHORS_CONTACTS);
}

/*
 *********************************************************** init function **
 * Signal handler
 *
 * IN:
 * @signo           Signal number.
 ****************************************************************************
 */
void sig_handler(int signo)
{
    switch (signo) {
        default: case SIGINT:
            endwin();
            exit(EXIT_SUCCESS);
            break;
    }
}

/*
 *********************************************************** init function **
 * Assign signal handlers to signals.
 ****************************************************************************
 */
void init_signal_handlers(void)
{
    if (signal(SIGINT, sig_handler) == SIG_ERR) {
        fprintf(stderr, "ERROR, can't establish SIGINT handler\n");
        exit(EXIT_FAILURE);
    }
}

/*
 ******************************************************** routine function **
 * Trap keys in program.
 *
 * RETURNS:
 * 1 if key is pressed or 0 if not.
 ****************************************************************************
 */
int key_is_pressed(void)
{
    int ch = getch();
    if (ch != ERR) {
        ungetch(ch);
        return 1;
    } else
        return 0;
}

/*
 ******************************************************** routine function **
 * Replace substring in string.
 *
 * IN:
 * @o_string                Original string.
 * @s_string                String to search for.
 * @r_string                Replace string.
 *
 * RETURNS:
 * @o_string                Modified string.
 ****************************************************************************
 */
void strrpl(char * o_string, char * s_string, char * r_string)
{
    char buffer[1024];
    char * ch;
             
    if(!(ch = strstr(o_string, s_string)))
        return;
                 
    strncpy(buffer, o_string, ch-o_string);
    buffer[ch-o_string] = 0;
    sprintf(buffer+(ch - o_string), "%s%s", r_string, ch + strlen(s_string));
    o_string[0] = 0;
    strcpy(o_string, buffer);
    strrpl(o_string, s_string, r_string);

    return;
}

/*
 ******************************************************** routine function **
 * Check string validity.
 *
 * IN:
 * @string              String which should be checked.
 *
 * RETURNS:
 * 0 if string is valid, -1 otherwise.
 *
 * NOTE:
 * In future, this function can be extended in case when string must be checked
 * with different conditions (numeric, alfa, alfanumeric, etc.).
 ****************************************************************************
 */
int check_string(char * string)
{
    int i;
    for (i = 0; string[i] != '\0'; i++) {
        if (!isalnum(string[i])) {
            /* non-alfanumeric char found */
            return -1;
        }
    }

    /* string ok */
    return 0;
}

/*
 *********************************************************** init function **
 * Allocate memory for screens options struct array.
 *
 * OUT:
 * @screen_s   Initialized array of screens options.
 ****************************************************************************
 */
void init_screens(struct screen_s *screens[])
{
    int i;
    for (i = 0; i < MAX_SCREEN; i++) {
        if ((screens[i] = (struct screen_s *) malloc(SCREEN_SIZE)) == NULL) {
                perror("malloc");
                exit(EXIT_FAILURE);
        }
        memset(screens[i], 0, SCREEN_SIZE);
        screens[i]->screen = i;
        screens[i]->conn_used = false;
        strcpy(screens[i]->host, "");
        strcpy(screens[i]->port, "");
        strcpy(screens[i]->user, "");
        strcpy(screens[i]->dbname, "");
        strcpy(screens[i]->password, "");
        strcpy(screens[i]->conninfo, "");
        screens[i]->subscreen_enabled = false;
        screens[i]->subscreen = SUBSCREEN_NONE;
        strcpy(screens[i]->log_path, "");
        screens[i]->current_context = DEFAULT_QUERY_CONTEXT;
        strcpy(screens[i]->pg_stat_activity_min_age, PG_STAT_ACTIVITY_MIN_AGE_DEFAULT);
        screens[i]->signal_options = 0;
        screens[i]->pg_stat_sys = false;

        screens[i]->context_list[PG_STAT_DATABASE_NUM].context = pg_stat_database;
        screens[i]->context_list[PG_STAT_DATABASE_NUM].order_key = PG_STAT_DATABASE_ORDER_MIN;
        screens[i]->context_list[PG_STAT_DATABASE_NUM].order_desc = true;
        screens[i]->context_list[PG_STAT_REPLICATION_NUM].context = pg_stat_replication;
        screens[i]->context_list[PG_STAT_REPLICATION_NUM].order_key = PG_STAT_REPLICATION_ORDER_MIN;
        screens[i]->context_list[PG_STAT_REPLICATION_NUM].order_desc = true;
        screens[i]->context_list[PG_STAT_TABLES_NUM].context = pg_stat_tables;
        screens[i]->context_list[PG_STAT_TABLES_NUM].order_key = PG_STAT_TABLES_ORDER_MIN;
        screens[i]->context_list[PG_STAT_TABLES_NUM].order_desc = true;
        screens[i]->context_list[PG_STAT_INDEXES_NUM].context = pg_stat_indexes;
        screens[i]->context_list[PG_STAT_INDEXES_NUM].order_key = PG_STAT_INDEXES_ORDER_MIN;
        screens[i]->context_list[PG_STAT_INDEXES_NUM].order_desc = true;
        screens[i]->context_list[PG_STATIO_TABLES_NUM].context = pg_statio_tables;
        screens[i]->context_list[PG_STATIO_TABLES_NUM].order_key = PG_STATIO_TABLES_ORDER_MIN;
        screens[i]->context_list[PG_STATIO_TABLES_NUM].order_desc = true;
        screens[i]->context_list[PG_TABLES_SIZE_NUM].context = pg_tables_size;
        screens[i]->context_list[PG_TABLES_SIZE_NUM].order_key = PG_TABLES_SIZE_ORDER_MIN;
        screens[i]->context_list[PG_TABLES_SIZE_NUM].order_desc = true;
        screens[i]->context_list[PG_STAT_ACTIVITY_LONG_NUM].context = pg_stat_activity_long;
        screens[i]->context_list[PG_STAT_ACTIVITY_LONG_NUM].order_key = PG_STAT_ACTIVITY_LONG_ORDER_MIN;
        screens[i]->context_list[PG_STAT_ACTIVITY_LONG_NUM].order_desc = true;
        screens[i]->context_list[PG_STAT_FUNCTIONS_NUM].context = pg_stat_functions;
        screens[i]->context_list[PG_STAT_FUNCTIONS_NUM].order_key = PG_STAT_FUNCTIONS_ORDER_MIN;
        screens[i]->context_list[PG_STAT_FUNCTIONS_NUM].order_desc = true;
        screens[i]->context_list[PG_STAT_STATEMENTS_TIMING_NUM].context = pg_stat_statements_timing;
        screens[i]->context_list[PG_STAT_STATEMENTS_TIMING_NUM].order_key = PG_STAT_STATEMENTS_TIMING_ORDER_MIN;
        screens[i]->context_list[PG_STAT_STATEMENTS_TIMING_NUM].order_desc = true;
        screens[i]->context_list[PG_STAT_STATEMENTS_GENERAL_NUM].context = pg_stat_statements_general;
        screens[i]->context_list[PG_STAT_STATEMENTS_GENERAL_NUM].order_key = PG_STAT_STATEMENTS_GENERAL_ORDER_MIN;
        screens[i]->context_list[PG_STAT_STATEMENTS_GENERAL_NUM].order_desc = true;
        screens[i]->context_list[PG_STAT_STATEMENTS_IO_NUM].context = pg_stat_statements_io;
        screens[i]->context_list[PG_STAT_STATEMENTS_IO_NUM].order_key = PG_STAT_STATEMENTS_IO_ORDER_MIN;
        screens[i]->context_list[PG_STAT_STATEMENTS_IO_NUM].order_desc = true;
        screens[i]->context_list[PG_STAT_STATEMENTS_TEMP_NUM].context = pg_stat_statements_temp;
        screens[i]->context_list[PG_STAT_STATEMENTS_TEMP_NUM].order_key = PG_STAT_STATEMENTS_TEMP_ORDER_MIN;
        screens[i]->context_list[PG_STAT_STATEMENTS_TEMP_NUM].order_desc = true;
    }
}

/*
 ******************************************************** startup function **
 * Password prompt.
 *
 * IN:
 * @prompt          Text of prompt.
 * @maxlen          Length of input string.
 * @echo            Echo input string.
 *
 * RETURNS:
 * @password        Password.
 ****************************************************************************
 */
char * password_prompt(const char *prompt, int maxlen, bool echo)
{
    struct termios t_orig, t;
    char *password;
    password = (char *) malloc(maxlen + 1);

    if (!echo) {
        tcgetattr(fileno(stdin), &t);
        t_orig = t;
        t.c_lflag &= ~ECHO;
        tcsetattr(fileno(stdin), TCSAFLUSH, &t);
    }

    fputs(prompt, stdout);
    if (fgets(password, maxlen + 1, stdin) == NULL)
        password[0] = '\0';

    if (!echo) {
        tcsetattr(fileno(stdin), TCSAFLUSH, &t_orig);
        fputs("\n", stdout);
        fflush(stdout);
    }

    return password;
}

/*
 ******************************************************** startup function **
 * Initialize empty struct for input arguments.
 *
 * OUT:
 * @args        Empty struct.
 ****************************************************************************
 */
void init_args_struct(struct args_s * args)
{
    args->count = 0;
    strcpy(args->connfile, "");
    strcpy(args->host, "");
    strcpy(args->port, "");
    strcpy(args->user, "");
    strcpy(args->dbname, "");
    args->need_passwd = false;                      /* by default password not need */
}

/*
 ******************************************************** startup function **
 * Parse input arguments
 *
 * IN:
 * @argc            Input arguments count.
 * @argv[]          Input arguments array.
 *
 * OUT:
 * @args            Struct with input args.
 ****************************************************************************
 */
void arg_parse(int argc, char *argv[], struct args_s *args)
{
    int param, option_index;

    /* short options */
    const char * short_options = "f:h:p:U:d:wW?";

    /* long options */
    const struct option long_options[] = {
        {"help", no_argument, NULL, '?'},
        {"file", required_argument, NULL, 'f'},
        {"host", required_argument, NULL, 'h'},
        {"port", required_argument, NULL, 'p'},
        {"dbname", required_argument, NULL, 'd'},
        {"no-password", no_argument, NULL, 'w'},
        {"password", no_argument, NULL, 'W'},
        {"user", required_argument, NULL, 'U'},
        {NULL, 0, NULL, 0}
    };

    if (argc > 1) {
        if ((strcmp(argv[1], "-?") == 0)
                || (argc == 2 && (strcmp(argv[1], "--help") == 0)))
        {
            print_usage();
            exit(EXIT_SUCCESS);
        }
        if (strcmp(argv[1], "--version") == 0
                || strcmp(argv[1], "-V") == 0)
        {
            printf("%s %.1f.%d\n", PROGRAM_NAME, PROGRAM_VERSION, PROGRAM_RELEASE);
            exit(EXIT_SUCCESS);
        }
    }
    
    while ( (param = getopt_long(argc, argv,
                short_options, long_options, &option_index)) != -1 ) {
        switch (param) {
            case 'h':
                strcpy(args->host, optarg);
                args->count++;
                break;
            case 'f':
                strcpy(args->connfile, optarg);
                args->count++;
                break;
            case 'p':
                strcpy(args->port, optarg);
                args->count++;
                break;
            case 'U':
                strcpy(args->user, optarg);
                args->count++;
                break;
            case 'd':
                args->count++;
                strcpy(args->dbname, optarg);
                break;
            case 'w':
                args->need_passwd = false;
                break;
            case 'W':
                args->need_passwd = true;
                break;
            case '?': default:
                fprintf(stderr,"Try \"%s --help\" for more information.\n", argv[0]);
                exit(EXIT_SUCCESS);
                break;
        }
    }

    /* handle extra parameters if exists, first - dbname, second - user, others - ignore */
    while (argc - optind >= 1) {
        if ( (argc - optind > 1)
                && strlen(args->user) == 0
                && strlen(args->dbname) == 0 ) {
            strcpy(args->dbname, argv[optind]);
            strcpy(args->user, argv[optind + 1]);
            optind++;
            args->count++;
        }
        else if ( (argc - optind >= 1) && strlen(args->user) != 0 && strlen(args->dbname) == 0 ) {
            strcpy(args->dbname, argv[optind]);
            args->count++;
        } else if ( (argc - optind >= 1) && strlen(args->user) == 0 && strlen(args->dbname) != 0 ) {
            strcpy(args->user, argv[optind]);
            args->count++;
        } else if ( (argc - optind >= 1) && strlen(args->user) == 0 && strlen(args->dbname) == 0 ) {
            strcpy(args->dbname, argv[optind]);
            args->count++;
        } else
            fprintf(stderr,
                    "%s: warning: extra command-line argument \"%s\" ignored\n",
                    argv[0], argv[optind]);
        optind++;
    }
}

/*
 ******************************************************** startup function **
 * Take input parameters and add them into connections options.
 *
 * IN:
 * @args            Struct with input arguments.
 *
 * OUT:
 * @screens[]       Array with connections options.
 ****************************************************************************
 */
void create_initial_conn(struct args_s * args, struct screen_s * screens[])
{
    struct passwd *pw = getpwuid(getuid());

    if ( strlen(args->host) != 0 )
        strcpy(screens[0]->host, args->host);

    if ( strlen(args->port) != 0 )
        strcpy(screens[0]->port, args->port);

    if ( strlen(args->user) == 0 )
        strcpy(screens[0]->user, pw->pw_name);
    else
        strcpy(screens[0]->user, args->user);

    if ( strlen(args->dbname) == 0 && strlen(args->user) == 0)
        strcpy(screens[0]->dbname, pw->pw_name);
    else if ( strlen(args->dbname) == 0 && strlen(args->user) != 0)
        strcpy(screens[0]->dbname, args->user);
    else if ( strlen(args->dbname) != 0 && strlen(args->user) == 0) {
        strcpy(screens[0]->dbname, args->dbname);
        strcpy(screens[0]->user, pw->pw_name);
    } else
        strcpy(screens[0]->dbname, args->dbname);

    if ( args->need_passwd )
        strcpy(screens[0]->password, password_prompt("Password: ", 100, false));

    if ( strlen(screens[0]->user) != 0 && strlen(screens[0]->dbname) == 0 )
        strcpy(screens[0]->dbname, screens[0]->user);

    screens[0]->conn_used = true;
}

/*
 ******************************************************** startup function **
 * Read ~/.pgcenterrc file and fill up conrections options array.
 *
 * IN:
 * @args            Struct with input arguments.
 * @pos             Start position inside array.
 *
 * OUT:
 * @screens         Connections options array.
 *
 * RETURNS:
 * Success or failure.
 ****************************************************************************
 */
int create_pgcenterrc_conn(struct args_s * args, struct screen_s * screens[], const int pos)
{
    FILE *fp;
    static char pgcenterrc_path[PATH_MAX];
    struct stat statbuf;
    char strbuf[BUFFERSIZE];
    int i = pos;
    struct passwd *pw = getpwuid(getuid());

    if (strlen(args->connfile) == 0) {
        strcpy(pgcenterrc_path, pw->pw_dir);
        strcat(pgcenterrc_path, "/");
        strcat(pgcenterrc_path, PGCENTERRC_FILE);
    } else {
        strcpy(pgcenterrc_path, args->connfile);
    }

    if (access(pgcenterrc_path, F_OK) == -1 && strlen(args->connfile) != 0) {
        fprintf(stderr,
                    "WARNING: no access to %s.\n", pgcenterrc_path);
        return PGCENTERRC_READ_ERR;
    }

    stat(pgcenterrc_path, &statbuf);
    if ( statbuf.st_mode & (S_IRWXG | S_IRWXO) && access(pgcenterrc_path, F_OK) != -1) {
        fprintf(stderr,
                    "WARNING: %s has wrong permissions.\n", pgcenterrc_path);
        return PGCENTERRC_READ_ERR;
    }

    /* read connections settings from .pgcenterrc */
    if ((fp = fopen(pgcenterrc_path, "r")) != NULL) {
        while ((fgets(strbuf, 4096, fp) != 0) && (i < MAX_SCREEN)) {
            sscanf(strbuf, "%[^:]:%[^:]:%[^:]:%[^:]:%[^:\n]",
                        screens[i]->host, screens[i]->port,
                        screens[i]->dbname,   screens[i]->user,
                        screens[i]->password);
                        screens[i]->screen = i;
                        screens[i]->conn_used = true;
            /* if "null" read from file, than we should connecting through unix socket */
            if (!strcmp(screens[i]->host, "(null)")) {
                strcpy(screens[i]->host, "\0");
            }
            i++;
        }
        fclose(fp);
        return PGCENTERRC_READ_OK;
    } else {
        return PGCENTERRC_READ_ERR;
    }
}

/*
 ******************************************************** routine function **
 * Check connection state, try reconnect if failed.
 *
 * IN:
 * @window          Window where status will be printed.
 * @conn            Connection associated with current screen.
 * @screen          Current screen.
 * @reconnected     True if conn failed and reconnect performed.
 ****************************************************************************
 */
void reconnect_if_failed(WINDOW * window, PGconn * conn, struct screen_s * screen, bool *reconnected)
{
    if (PQstatus(conn) == CONNECTION_BAD) {
        wclear(window);
        PQreset(conn);
        wprintw(window,
                "The connection to the server was lost. Attempting reconnect.");
        wrefresh(window);
        /* reset previous query results after reconnect */
        *reconnected = true;
        sleep(1);
    }
    
    /* get PostgreSQL version if reconnect successful */
    if (reconnected) {
        get_pg_version(conn, screen);
    }
}

/*
 ******************************************************** startup function **
 * Prepare conninfo string for PQconnectdb.
 *
 * IN:
 * @screens       Screens options array without filled conninfo.
 *
 * OUT:
 * @screens       Screens options array with conninfo.
 */
void prepare_conninfo(struct screen_s * screens[])
{
    int i;
    for ( i = 0; i < MAX_SCREEN; i++ )
        if (screens[i]->conn_used) {
            if (strlen(screens[i]->host) != 0) {
                strcat(screens[i]->conninfo, "host=");
                strcat(screens[i]->conninfo, screens[i]->host);
            }
            if (strlen(screens[i]->port) != 0) {
                strcat(screens[i]->conninfo, " port=");
                strcat(screens[i]->conninfo, screens[i]->port);
            }
            strcat(screens[i]->conninfo, " user=");
            strcat(screens[i]->conninfo, screens[i]->user);
            strcat(screens[i]->conninfo, " dbname=");
            strcat(screens[i]->conninfo, screens[i]->dbname);
            if ((strlen(screens[i]->password)) != 0) {
                strcat(screens[i]->conninfo, " password=");
                strcat(screens[i]->conninfo, screens[i]->password);
            }
        }
}

/*
 ******************************************************** startup function **
 * Open connections to PostgreSQL using conninfo string from screen struct.
 *
 * IN:
 * @screens         Screens options array.
 *
 * OUT:
 * @conns           Array of connections.
 ****************************************************************************
 */
void open_connections(struct screen_s * screens[], PGconn * conns[])
{
    int i;
    for ( i = 0; i < MAX_SCREEN; i++ ) {
        if (screens[i]->conn_used) {
            conns[i] = PQconnectdb(screens[i]->conninfo);
            if ( PQstatus(conns[i]) == CONNECTION_BAD && PQconnectionNeedsPassword(conns[i]) == 1) {
                printf("%s:%s %s@%s require ", 
                                screens[i]->host, screens[i]->port,
                                screens[i]->user, screens[i]->dbname);
                strcpy(screens[i]->password, password_prompt("password: ", 100, false));
                strcat(screens[i]->conninfo, " password=");
                strcat(screens[i]->conninfo, screens[i]->password);
                conns[i] = PQconnectdb(screens[i]->conninfo);
            } else if ( PQstatus(conns[i]) == CONNECTION_BAD ) {
                fprintf(stderr, "ERROR: Connection to %s:%s with %s@%s failed.\n",
                screens[i]->host, screens[i]->port,
                screens[i]->user, screens[i]->dbname);
                continue;
            }

            /* get PostgreSQL version */
            get_pg_version(conns[i], screens[i]);

            PGresult * res;
            char * errmsg = (char *) malloc(sizeof(char) * 1024);
            /* suppress log messages with log_min_duration_statement */
            if ((res = do_query(conns[i], PG_SUPPRESS_LOG_QUERY, errmsg)) != NULL)
               PQclear(res);
            /* increase work_mem */
            if ((res = do_query(conns[i], PG_INCREASE_WORK_MEM_QUERY, errmsg)) != NULL)
                PQclear(res);
            if (errmsg)
                free(errmsg);
        }
    }
}

/*
 **************************************************** end program function **
 * Close connections to postgresql.
 *
 * IN:
 * @conns       Array of connections.
 ****************************************************************************
 */
void close_connections(struct screen_s * screens[], PGconn * conns[])
{
    int i;
    for (i = 0; i < MAX_SCREEN; i++)
        if (screens[i]->conn_used)
            PQfinish(conns[i]);
}

/*
 **************************************************** end program function **
 * Graceful quit.
 *
 * IN:
 * @conns       Array of connections.
 ****************************************************************************
 */
void exit_prog(struct screen_s * screens[], PGconn * conns[])
{
    endwin();
    close_connections(screens, conns);
    exit(EXIT_SUCCESS);
}

/*
 ****************************************************************************
 * Prepare query using current screen query context.
 *
 * IN:
 * @screen              Current screen where query context is stored.
 * @query               Text of query.
 ****************************************************************************
 */
void prepare_query(struct screen_s * screen, char * query)
{
    char tmp[2];
    char view[] = DEFAULT_VIEW_TYPE;
    switch (screen->current_context) {
        case pg_stat_database: default:
            if (atoi(screen->pg_version_num) < 90200)
                strcpy(query, PG_STAT_DATABASE_91_QUERY);
            else
                strcpy(query, PG_STAT_DATABASE_QUERY);
            break;
        case pg_stat_replication:
            strcpy(query, PG_STAT_REPLICATION_QUERY);
            break;
        case pg_stat_tables:
            if (screen->pg_stat_sys)
                strcpy(view, FULL_VIEW_TYPE);
            strcpy(query, PG_STAT_TABLES_QUERY_P1);
            strcat(query, view);
            strcat(query, PG_STAT_TABLES_QUERY_P2);
            break;
        case pg_stat_indexes:
            if (screen->pg_stat_sys)
                strcpy(view, FULL_VIEW_TYPE);
            strcpy(query, PG_STAT_INDEXES_QUERY_P1);
            strcat(query, view);
            strcat(query, PG_STAT_INDEXES_QUERY_P2);
            strcat(query, view);
            strcat(query, PG_STAT_INDEXES_QUERY_P3);
            break;
        case pg_statio_tables:
            if (screen->pg_stat_sys)
                strcpy(view, FULL_VIEW_TYPE);
            strcpy(query, PG_STATIO_TABLES_QUERY_P1);
            strcat(query, view);
            strcat(query, PG_STATIO_TABLES_QUERY_P2);
            break;
        case pg_tables_size:
            if (screen->pg_stat_sys)
                strcpy(view, FULL_VIEW_TYPE);
            strcpy(query, PG_TABLES_SIZE_QUERY_P1);
            strcat(query, view);
            strcat(query, PG_TABLES_SIZE_QUERY_P2);
            break;
        case pg_stat_activity_long:
            /* 
             * build query from several parts, 
             * thus user can change duration which is used in WHERE clause.
             */
            if (atoi(screen->pg_version_num) < 90200) {
                strcpy(query, PG_STAT_ACTIVITY_LONG_91_QUERY_P1);
                strcat(query, screen->pg_stat_activity_min_age);
                strcat(query, PG_STAT_ACTIVITY_LONG_91_QUERY_P2);
                strcat(query, screen->pg_stat_activity_min_age);
                strcat(query, PG_STAT_ACTIVITY_LONG_91_QUERY_P3);
            } else {
                strcpy(query, PG_STAT_ACTIVITY_LONG_QUERY_P1);
                strcat(query, screen->pg_stat_activity_min_age);
                strcat(query, PG_STAT_ACTIVITY_LONG_QUERY_P2);
                strcat(query, screen->pg_stat_activity_min_age);
                strcat(query, PG_STAT_ACTIVITY_LONG_QUERY_P3);
            }
            break;
        case pg_stat_functions:
            /* here we use query native ORDER BY, and we should incrementing order key */
            sprintf(tmp, "%d", screen->context_list[PG_STAT_FUNCTIONS_NUM].order_key + 1);
            strcpy(query, PG_STAT_FUNCTIONS_QUERY_P1);
            strcat(query, tmp);             /* insert number of field into ORDER BY .. */
            strcat(query, PG_STAT_FUNCTIONS_QUERY_P2);
            break;
        case pg_stat_statements_timing:
            /* here we use query native ORDER BY, and we should incrementing order key */
            sprintf(tmp, "%d", screen->context_list[PG_STAT_STATEMENTS_TIMING_NUM].order_key + 1);
            if (atoi(screen->pg_version_num) < 90200) {
                strcpy(query, PG_STAT_STATEMENTS_TIMING_91_QUERY_P1);
            } else {
                strcpy(query, PG_STAT_STATEMENTS_TIMING_QUERY_P1);
            }
            strcat(query, tmp);             /* insert number of field into ORDER BY .. */
            strcat(query, PG_STAT_STATEMENTS_TIMING_QUERY_P2);
            break;
        case pg_stat_statements_general:
            /* here we use query native ORDER BY, and we should incrementing order key */
            sprintf(tmp, "%d", screen->context_list[PG_STAT_STATEMENTS_GENERAL_NUM].order_key + 1);
            if (atoi(screen->pg_version_num) < 90200) {
                strcpy(query, PG_STAT_STATEMENTS_GENERAL_91_QUERY_P1);
            } else {
                strcpy(query, PG_STAT_STATEMENTS_GENERAL_QUERY_P1);
            }
            strcat(query, tmp);             /* insert number of field into ORDER BY .. */
            strcat(query, PG_STAT_STATEMENTS_GENERAL_QUERY_P2);
            break;
        case pg_stat_statements_io:
            /* here we use query native ORDER BY, and we should incrementing order key */
            sprintf(tmp, "%d", screen->context_list[PG_STAT_STATEMENTS_IO_NUM].order_key + 1);
            if (atoi(screen->pg_version_num) < 90200) {
                strcpy(query, PG_STAT_STATEMENTS_IO_91_QUERY_P1);
            } else {
                strcpy(query, PG_STAT_STATEMENTS_IO_QUERY_P1);
            }
            strcat(query, tmp);             /* insert number of field into ORDER BY .. */
            strcat(query, PG_STAT_STATEMENTS_IO_QUERY_P2);
            break;
        case pg_stat_statements_temp:
            /* here we use query native ORDER BY, and we should incrementing order key */
            sprintf(tmp, "%d", screen->context_list[PG_STAT_STATEMENTS_TEMP_NUM].order_key + 1);
            strcpy(query, PG_STAT_STATEMENTS_TEMP_QUERY_P1);
            strcat(query, tmp);             /* insert number of field into ORDER BY .. */
            strcat(query, PG_STAT_STATEMENTS_TEMP_QUERY_P2);
            break;
    }
}

/*
 ********************************************************* routine function **
 * Send query to PostgreSQL.
 *
 * IN:
 * @window          Window for printing errors if query failed.
 * @conn            PostgreSQL connection.
 * @query_context   Type of query.
 *
 * OUT:
 * @errmsg          Error message returned by postgres.
 *
 * RETURNS:
 * Answer from PostgreSQL.
 ****************************************************************************
 */
PGresult * do_query(PGconn * conn, char * query, char *errmsg)
{
    PGresult    *res;

    res = PQexec(conn, query);
    switch (PQresultStatus(res)) {
        case PG_FATAL_ERR:
            strcpy(errmsg, "FATAL: ");
            strcat(errmsg, PQerrorMessage(conn));
            PQclear(res);
            return NULL;
            break;
        case PG_CMD_OK: case PG_TUP_OK:
            return res;
            break;
        default:
            strcpy(errmsg, PQresultErrorField(res, PG_DIAG_SEVERITY));
            strcat(errmsg, ": ");
            strcat(errmsg, PQresultErrorField(res, PG_DIAG_MESSAGE_PRIMARY));
            PQclear(res);
            return NULL;
            break;
    }
}

/*
 ************************************************* summary window function **
 * Print current time.
 *
 * OUT:
 * @strtime         Current time.
 ****************************************************************************
 */
void get_time(char * strtime)
{
    time_t rawtime;
    struct tm *timeinfo;

    time(&rawtime);
    timeinfo = localtime(&rawtime);
    strftime(strtime, 20, "%Y-%m-%d %H:%M:%S", timeinfo);
}

/*
 ************************************************* summary window function **
 * Print title into summary window: program name and current time.
 *
 * IN:
 * @window          Window where title will be printed.
 ****************************************************************************
 */
void print_title(WINDOW * window, char * progname)
{
    char *strtime = (char *) malloc(sizeof(char) * 20);
    get_time(strtime);
    wprintw(window, "%s: %s, ", progname, strtime);
    free(strtime);
}

/*
 ************************************************* summary window function **
 * Read /proc/loadavg and return load average value.
 *
 * IN:
 * @m       Minute value.
 *
 * RETURNS:
 * Load average for 1, 5 or 15 minutes.
 ****************************************************************************
 */
float get_loadavg(int m)
{
    if ( m != 1 && m != 5 && m != 15 )
        m = 1;

    float avg = 0, avg1, avg5, avg15;
    FILE *loadavg_fd;

    if ((loadavg_fd = fopen(LOADAVG_FILE, "r")) == NULL) {
        fprintf(stderr, "can't open %s\n", LOADAVG_FILE);
        exit(EXIT_FAILURE);
    } else {
        fscanf(loadavg_fd, "%f %f %f", &avg1, &avg5, &avg15);
        fclose(loadavg_fd);
    }

    switch (m) {
        case 1: avg = avg1; break;
        case 5: avg = avg5; break;
        case 15: avg = avg15; break;
    }
    return avg;
}

/*
 ************************************************* summary window function **
 * Print load average into summary window.
 *
 * IN:
 * @window      Window where load average will be printed.
 ****************************************************************************
 */
void print_loadavg(WINDOW * window)
{
    wprintw(window, "load average: %.2f, %.2f, %.2f\n",
                    get_loadavg(1), get_loadavg(5), get_loadavg(15));
}

/*
 ************************************************* summary window function **
 * Print current connection info.
 *
 * IN:
 * @window          Window where info will be printed.
 * @conn            Current connection.
 * @console_no      Current console number.
 ****************************************************************************
 */
void print_conninfo(WINDOW * window, PGconn *conn, int console_no)
{
    static char state[8];
    char buffer[128];
    switch (PQstatus(conn)) {
        case CONNECTION_OK:
            strcpy(state, "ok");
            break;
        case CONNECTION_BAD:
            strcpy(state, "failed");
            break;
        default:
            strcpy(state, "unknown");
            break;
    }

    sprintf(buffer, "conn%i [%s]: %s:%s %s@%s",
                console_no, state,
                PQhost(conn), PQport(conn),
                PQuser(conn), PQdb(conn));

    if (strlen(buffer) > 48) {
        buffer[48] = '~';
        buffer[49] = '\0';
    }

    mvwprintw(window, 0, COLS / 2, "%s", buffer);

    wrefresh(window);
}

/*
 ************************************************** system window function **
 * Print current postgres process activity: total/idle/idle in transaction/
 * /active/waiting/others backends.
 *
 * IN:
 * @window          Window where info will be printed.
 * @conn            Current postgres connection.
 ****************************************************************************
 */
void print_postgres_activity(WINDOW * window, PGconn * conn)
{
    int t_count, i_count, x_count, a_count, w_count, o_count;
    PGresult *res;
    char *errmsg = (char *) malloc(sizeof(char) * 1024);

    if (PQstatus(conn) == CONNECTION_BAD) {
        t_count = 0;
        i_count = 0;
        x_count = 0;
        a_count = 0;
        w_count = 0;
        o_count = 0;
    } 
    if ((res = do_query(conn, PG_STAT_ACTIVITY_COUNT_TOTAL_QUERY, errmsg)) != NULL) {
        t_count = atoi(PQgetvalue(res, 0, 0));
        PQclear(res);
    } else {
        t_count = 0;
    }

    if ((res = do_query(conn, PG_STAT_ACTIVITY_COUNT_IDLE_QUERY, errmsg)) != NULL) {
        i_count = atoi(PQgetvalue(res, 0, 0));
        PQclear(res);
    } else {
        i_count = 0;
    }

    if ((res = do_query(conn, PG_STAT_ACTIVITY_COUNT_IDLE_IN_T_QUERY, errmsg)) != NULL) {
        x_count = atoi(PQgetvalue(res, 0, 0));
        PQclear(res);
    } else {
        x_count = 0;
    }

    if ((res = do_query(conn, PG_STAT_ACTIVITY_COUNT_ACTIVE_QUERY, errmsg)) != NULL) {
        a_count = atoi(PQgetvalue(res, 0, 0));
        PQclear(res);
    } else {
        a_count = 0;
    }

    if ((res = do_query(conn, PG_STAT_ACTIVITY_COUNT_WAITING_QUERY, errmsg)) != NULL) {
        w_count = atoi(PQgetvalue(res, 0, 0));
        PQclear(res);
    } else {
        w_count = 0;
    }

    if ((res = do_query(conn, PG_STAT_ACTIVITY_COUNT_OTHERS_QUERY, errmsg)) != NULL) {
        o_count = atoi(PQgetvalue(res, 0, 0));
        PQclear(res);
    } else {
        o_count = 0;
    }

    mvwprintw(window, 1, COLS / 2,
            "  activity:%3i total,%3i idle,%3i idle_in_xact,%3i active,%3i waiting,%3i others",
            t_count, i_count, x_count, a_count, w_count, o_count);
    wrefresh(window);
    free(errmsg);
}

/*
 ************************************************** system window function **
 * Print pg_stat_statements related info.
 *
 * IN:
 * @window          Window where info will be printed.
 * @conn            Current postgres connection.
 * @interval        Screen refresh interval.
 ****************************************************************************
 */
void print_pgstatstmt_info(WINDOW * window, PGconn * conn, long int interval)
{
    float avgtime;
    static int qps, prev_queries = 0;
    int divisor;
    char maxtime[16] = "";
    PGresult *res;
    char *errmsg = (char *) malloc(sizeof(char) * 1024);

    if (PQstatus(conn) == CONNECTION_BAD) {
        avgtime = 0;
        qps = 0;
        strcpy(maxtime, "--:--:--");
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
        strcpy(maxtime, PQgetvalue(res, 0, 0));
        PQclear(res);
    } else {
        strcpy(maxtime, "--:--:--");
    }

    mvwprintw(window, 3, COLS / 2,
            "statements: %3i stmt/s,  %3.3f stmt_avgtime, %s xact_maxtime",
            qps, avgtime, maxtime);
    wrefresh(window);
    free(errmsg);
}

/*
 ******************************************************* get stat function **
 * Allocate memory for statistics structs.
 *
 * OUT:
 * @st_cpu          Struct for cpu statistics.
 * @st_mem_short    Struct for mem statistics.
 ****************************************************************************
 */
void init_stats(struct stats_cpu_struct *st_cpu[], struct stats_mem_short_struct **st_mem_short)
{
    int i;
    /* Allocate structures for CPUs "all" and 0 */
    for (i = 0; i < 2; i++) {
        if ((st_cpu[i] = (struct stats_cpu_struct *) malloc(STATS_CPU_SIZE * 2)) == NULL) {
            perror("malloc for cpu stats failed");
            exit(EXIT_FAILURE);
        }
        memset(st_cpu[i], 0, STATS_CPU_SIZE * 2);
    }

    /* Allocate structures for memory */
    if ((*st_mem_short = (struct stats_mem_short_struct *) malloc(STATS_MEM_SHORT_SIZE)) == NULL) {
            perror("malloc for mem stats failed");
            exit(EXIT_FAILURE);
    }
    memset(*st_mem_short, 0, STATS_MEM_SHORT_SIZE);
}

/*
 ******************************************************* get stat function **
 * Allocate memory for IO statistics structs.
 *
 * OUT:
 * @c_ios       Struct for current stats snapshot.
 * @p_ios       Struct for previous stats snapshot.
 * @x_ios       Struct for extended stats.
 * @ndev        Number of block devices.
 ****************************************************************************
 */
void init_iostats(struct dstats *c_ios[], struct dstats *p_ios[], struct ext_dstats *x_ios[], int ndev)
{
    int i;
    for (i = 0; i < ndev; i++) {
        if ((c_ios[i] = (struct dstats *) malloc(STATS_IOSTAT_SIZE)) == NULL) {
            perror("malloc for iostat stats failed");
            exit(EXIT_FAILURE);
        }
        if ((p_ios[i] = (struct dstats *) malloc(STATS_IOSTAT_SIZE)) == NULL) {
            perror("malloc for iostat stats failed");
            exit(EXIT_FAILURE);
        }
        if ((x_ios[i] = (struct ext_dstats *) malloc(STATS_EXT_IOSTAT_SIZE)) == NULL) {
            perror("malloc for extended iostat stats failed");
            exit(EXIT_FAILURE);
        }
    }
}

/*
 ******************************************************* get stat function **
 * Free memory consumed by IO statistics structs.
 *
 * OUT:
 * @c_ios       Struct for current stats snapshot.
 * @p_ios       Struct for previous stats snapshot.
 * @x_ios       Struct for extended stats.
 * @ndev        Number of block devices.
 ****************************************************************************
 */
void free_iostats(struct dstats *c_ios[], struct dstats *p_ios[], struct ext_dstats *x_ios[], int ndev)
{
    int i;
    for (i = 0; i < ndev; i++) {
        free(c_ios[i]);
        free(p_ios[i]);
        free(x_ios[i]);
    }
}

/*
 *************************************************** get cpu stat function **
 * Get system clock resolution.
 *
 * OUT:
 * @hz      Number of intervals per second.
 ****************************************************************************
 */
void get_HZ(void)
{
    long ticks;
    if ((ticks = sysconf(_SC_CLK_TCK)) == -1)
        perror("sysconf");
    
    hz = (unsigned int) ticks;
}

/*
 *************************************************** get cpu stat function **
 * Read machine uptime independently of the number of processors.
 *
 * OUT:
 * @uptime          Uptime value in jiffies.
 ****************************************************************************
 */
void read_uptime(unsigned long long *uptime)
{
    FILE *fp;
    char line[128];
    unsigned long up_sec, up_cent;

    if ((fp = fopen(UPTIME_FILE, "r")) == NULL)
        return;

    if (fgets(line, sizeof(line), fp) == NULL) {
        fclose(fp);
        return;
    }

    sscanf(line, "%lu.%lu", &up_sec, &up_cent);
    *uptime = (unsigned long long) up_sec * HZ +
    (unsigned long long) up_cent * HZ / 100;
    fclose(fp);
}

/*
 **************************************************** get cpu stat function **
 * Read cpu statistics from /proc/stat. Also calculate uptime if 
 * read_uptime() function return NULL.
 *
 * IN:
 * @st_cpu          Struct where stat will be saved.
 * @nbr             Total number of CPU (including cpu "all").
 *
 * OUT:
 * @st_cpu          Struct with statistics.
 * @uptime          Machine uptime multiplied by the number of processors.
 * @uptime0         Machine uptime. Filled only if previously set to zero.
 ****************************************************************************
 */
void read_cpu_stat(struct stats_cpu_struct *st_cpu, int nbr,
                            unsigned long long *uptime, unsigned long long *uptime0)
{
    FILE *stat_fp;
    struct stats_cpu_struct *st_cpu_i;
    struct stats_cpu_struct sc;
    char line[8192];
    int proc_nb;

    if ((stat_fp = fopen(STAT_FILE, "r")) == NULL) {
        fprintf(stderr, "Cannot open %s: %s\n", STAT_FILE, strerror(errno));
        exit(EXIT_FAILURE);
    }

    while ( (fgets(line, sizeof(line), stat_fp)) != NULL ) {
        if (!strncmp(line, "cpu ", 4)) {
            memset(st_cpu, 0, STATS_CPU_SIZE);
            sscanf(line + 5, "%llu %llu %llu %llu %llu %llu %llu %llu %llu %llu",
                            &st_cpu->cpu_user,      &st_cpu->cpu_nice,
                            &st_cpu->cpu_sys,       &st_cpu->cpu_idle,
                            &st_cpu->cpu_iowait,    &st_cpu->cpu_hardirq,
                            &st_cpu->cpu_softirq,   &st_cpu->cpu_steal,
                            &st_cpu->cpu_guest,     &st_cpu->cpu_guest_nice);
                            *uptime = st_cpu->cpu_user + st_cpu->cpu_nice +
                                st_cpu->cpu_sys + st_cpu->cpu_idle +
                                st_cpu->cpu_iowait + st_cpu->cpu_steal +
                                st_cpu->cpu_hardirq + st_cpu->cpu_softirq +
                                st_cpu->cpu_guest + st_cpu->cpu_guest_nice;
        } else if (!strncmp(line, "cpu", 3)) {
            if (nbr > 1) {
                memset(&sc, 0, STATS_CPU_SIZE);
                sscanf(line + 3, "%d %llu %llu %llu %llu %llu %llu %llu %llu %llu %llu",
                                &proc_nb,           &sc.cpu_user,
                                &sc.cpu_nice,       &sc.cpu_sys,
                                &sc.cpu_idle,       &sc.cpu_iowait,
                                &sc.cpu_hardirq,    &sc.cpu_softirq,
                                &sc.cpu_steal,      &sc.cpu_guest,
                                &sc.cpu_guest_nice);

                                if (proc_nb < (nbr - 1)) {
                                    st_cpu_i = st_cpu + proc_nb + 1;
                                    *st_cpu_i = sc;
                                }

                                if (!proc_nb && !*uptime0) {
                                    *uptime0 = sc.cpu_user + sc.cpu_nice   +
                                    sc.cpu_sys     + sc.cpu_idle   +
                                    sc.cpu_iowait  + sc.cpu_steal  +
                                    sc.cpu_hardirq + sc.cpu_softirq;
                                    printf("read_cpu_stat: uptime0 = %llu\n", *uptime0);
                                }
            }
        }
    }
    fclose(stat_fp);
}

/*
 *************************************************** get cpu stat function **
 * Compute time interval.
 *
 * IN:
 * @prev_uptime     Previous uptime value in jiffies.
 * @curr_interval   Current uptime value in jiffies.
 *
 * RETURNS:
 * Interval of time in jiffies.
 ****************************************************************************
 */
unsigned long long get_interval(unsigned long long prev_uptime,
                                        unsigned long long curr_uptime)
{
    unsigned long long itv;
    
    /* first run prev_uptime=0 so displaying stats since system startup */
    itv = curr_uptime - prev_uptime;

    if (!itv) {     /* Paranoia checking */
        itv = 1;
    }

    return itv;
}

/*
 *************************************************** get cpu stat function **
 * Workaround for CPU counters read from /proc/stat: Dyn-tick kernels
 * have a race issue that can make those counters go backward.
 ****************************************************************************
 */
double ll_sp_value(unsigned long long value1, unsigned long long value2,
                unsigned long long itv)
{
    if (value2 < value1)
        return (double) 0;
    else
        return SP_VALUE(value1, value2, itv);
}

/*
 *************************************************** get cpu stat function **
 * Display cpu statistics in specified window.
 *
 * IN:
 * @window      Window in which spu statistics will be printed.
 * @st_cpu      Struct with cpu statistics.
 * @curr        Index in array for current sample statistics.
 * @itv         Interval of time.
 ****************************************************************************
 */
void write_cpu_stat_raw(WINDOW * window, struct stats_cpu_struct *st_cpu[],
                int curr, unsigned long long itv)
{
    wprintw(window, 
            "    %%cpu: %4.1f us, %4.1f sy, %4.1f ni, %4.1f id, %4.1f wa, %4.1f hi, %4.1f si, %4.1f st\n",
            ll_sp_value(st_cpu[!curr]->cpu_user, st_cpu[curr]->cpu_user, itv),
            ll_sp_value(st_cpu[!curr]->cpu_sys + st_cpu[!curr]->cpu_softirq + st_cpu[!curr]->cpu_hardirq,
            st_cpu[curr]->cpu_sys + st_cpu[curr]->cpu_softirq + st_cpu[curr]->cpu_hardirq, itv),
            ll_sp_value(st_cpu[!curr]->cpu_nice, st_cpu[curr]->cpu_nice, itv),
            (st_cpu[curr]->cpu_idle < st_cpu[!curr]->cpu_idle) ?
            0.0 :
            ll_sp_value(st_cpu[!curr]->cpu_idle, st_cpu[curr]->cpu_idle, itv),
            ll_sp_value(st_cpu[!curr]->cpu_iowait, st_cpu[curr]->cpu_iowait, itv),
            ll_sp_value(st_cpu[!curr]->cpu_hardirq, st_cpu[curr]->cpu_hardirq, itv),
            ll_sp_value(st_cpu[!curr]->cpu_softirq, st_cpu[curr]->cpu_softirq, itv),
            ll_sp_value(st_cpu[!curr]->cpu_steal, st_cpu[curr]->cpu_steal, itv));
    wrefresh(window);
}

/*
 ************************************************** system window function **
 * Composite function which read cpu stats and uptime then print out stats 
 * to specified window.
 *
 * IN:
 * @window      Window where spu statistics will be printed.
 * @st_cpu      Struct with cpu statistics.
 ****************************************************************************
 */
void print_cpu_usage(WINDOW * window, struct stats_cpu_struct *st_cpu[])
{
    static unsigned long long uptime[2]  = {0, 0};
    static unsigned long long uptime0[2] = {0, 0};
    static int curr = 1;
    static unsigned long long itv;

    uptime0[curr] = 0;
    read_uptime(&(uptime0[curr]));
    read_cpu_stat(st_cpu[curr], 2, &(uptime[curr]), &(uptime0[curr]));
    itv = get_interval(uptime[!curr], uptime[curr]);
    write_cpu_stat_raw(window, st_cpu, curr, itv);
    itv = get_interval(uptime0[!curr], uptime0[curr]);
    curr ^= 1;
}

/*
 ************************************************** system window function **
 * Print memory usage statistics
 *
 * IN:
 * @window          Window where mem statistics will be printed.
 * @st_mem_short    Struct with mem statistics.
 ****************************************************************************
 */
void print_mem_usage(WINDOW * window, struct stats_mem_short_struct *st_mem_short)
{
    FILE *mem_fp;
    char buffer[121];
    char key[80];
    unsigned long long value;
    
    if ((mem_fp = fopen(MEMINFO_FILE, "r")) == NULL) {
        fprintf(stderr, "Cannot open %s: %s\n", MEMINFO_FILE, strerror(errno));
        exit(EXIT_FAILURE);
    }

    while (fgets(buffer, 120, mem_fp) != NULL) {
        sscanf(buffer, "%s %llu", key, &value);
        if (!strcmp(key,"MemTotal:"))
            st_mem_short->mem_total = value / 1024;
        else if (!strcmp(key,"MemFree:"))
            st_mem_short->mem_free = value / 1024;
        else if (!strcmp(key,"SwapTotal:"))
            st_mem_short->swap_total = value / 1024;
        else if (!strcmp(key,"SwapFree:"))
            st_mem_short->swap_total = value / 1024;
        else if (!strcmp(key,"Cached:"))
            st_mem_short->cached = value / 1024;
        else if (!strcmp(key,"Dirty:"))
            st_mem_short->dirty = value / 1024;
        else if (!strcmp(key,"Writeback:"))
            st_mem_short->writeback = value / 1024;
        else if (!strcmp(key,"Buffers:"))
            st_mem_short->buffers = value / 1024;
        else if (!strcmp(key,"Slab:"))
            st_mem_short->slab = value / 1024;
    }
    st_mem_short->mem_used = st_mem_short->mem_total - st_mem_short->mem_free
            - st_mem_short->cached - st_mem_short->buffers - st_mem_short->slab;
    st_mem_short->swap_used = st_mem_short->swap_total - st_mem_short->swap_free;

    fclose(mem_fp);

    wprintw(window, " MiB mem: %6llu total, %6llu free, %6llu used, %8llu buff/cached\n",
            st_mem_short->mem_total,
            st_mem_short->mem_free,
            st_mem_short->mem_used,
            st_mem_short->cached + st_mem_short->buffers + st_mem_short->slab);
    wprintw(window, "MiB swap: %6llu total, %6llu free, %6llu used, %6llu/%llu dirty/writeback\n",
            st_mem_short->swap_total,
            st_mem_short->swap_free,
            st_mem_short->swap_used,
            st_mem_short->dirty,
            st_mem_short->writeback);
}

/*
 ************************************************** system window function **
 * Save current io statistics snapshot.
 *
 * IN:
 * @curr        Current statistics snapshot which must be saved.
 * @prev        Struct for saving stat snapshot.
 * @n_dev       Number of block devices.
 ****************************************************************************
 */
void replace_dstats(struct dstats *curr[], struct dstats *prev[], int n_dev)
{
    int i;
    for (i = 0; i < n_dev; i++) {
        prev[i]->r_completed = curr[i]->r_completed;
        prev[i]->r_merged = curr[i]->r_merged;
        prev[i]->r_sectors = curr[i]->r_sectors;
        prev[i]->r_spent = curr[i]->r_spent;
        prev[i]->w_completed = curr[i]->w_completed;
        prev[i]->w_merged = curr[i]->w_merged;
        prev[i]->w_sectors = curr[i]->w_sectors;
        prev[i]->w_spent = curr[i]->w_spent;
        prev[i]->io_in_progress = curr[i]->io_in_progress;
        prev[i]->t_spent = curr[i]->t_spent;
        prev[i]->t_weighted = curr[i]->t_weighted;
    }
}

/*
 ****************************************************** subscreen function **
 * Print IO statistics from /proc/diskstats.
 *
 * IN:
 * @window          Window where stat will be printed.
 * @w_cmd           Window for errors and messaged.
 * @c_ios           Snapshot for current stat.
 * @p_ios           Snapshot for previous stat.
 * @x_ios           Struct for extended stat.
 * @ndev            Number of devices.
 * @repaint         Repaint subscreen flag.
 ****************************************************************************
 */
void print_iostat(WINDOW * window, WINDOW * w_cmd, struct dstats *c_ios[],
        struct dstats *p_ios[], struct ext_dstats *x_ios[], int ndev, bool * repaint)
{
    /* if number of devices is changed, we should realloc structs and repaint subscreen */
    if (ndev != count_block_devices()) {
        wprintw(w_cmd, "The number of devices has changed. ");
        *repaint = true;
        return;
    }

    FILE *fp;
    static unsigned long long uptime0[2] = {0, 0};
    static unsigned long long itv;
    static int curr = 1;
    int i = 0;
    char line[128];

    int major, minor;
    char devname[64];
    unsigned long r_completed, r_merged, r_sectors, r_spent,
                  w_completed, w_merged, w_sectors, w_spent,
                  io_in_progress, t_spent, t_weighted;
    double r_await[ndev], w_await[ndev];
    
    uptime0[curr] = 0;
    read_uptime(&(uptime0[curr]));

    /*
     * If read /proc/diskstats failed, fire up repaint flag.
     * Next when subscreen repainting fails, subscreen will be closed.
     */
    if ((fp = fopen(DISKSTATS_FILE, "r")) == NULL) {
        wclear(window);
        wprintw(window, "Do nothing. Can't open %s", DISKSTATS_FILE);
        wrefresh(window);
        *repaint = true;
        return;
    }

    while (fgets(line, sizeof(line), fp) != NULL) {
        sscanf(line, "%i %i %s %lu %lu %lu %lu %lu %lu %lu %lu %lu %lu %lu",
                    &major, &minor, devname,
                    &r_completed, &r_merged, &r_sectors, &r_spent,
                    &w_completed, &w_merged, &w_sectors, &w_spent,
                    &io_in_progress, &t_spent, &t_weighted);
        c_ios[i]->major = major;
        c_ios[i]->minor = minor;
        strcpy(c_ios[i]->devname, devname);
        c_ios[i]->r_completed = r_completed;
        c_ios[i]->r_merged = r_merged;
        c_ios[i]->r_sectors = r_sectors;
        c_ios[i]->r_spent = r_spent;
        c_ios[i]->w_completed = w_completed;
        c_ios[i]->w_merged = w_merged;
        c_ios[i]->w_sectors = w_sectors;
        c_ios[i]->w_spent = w_spent;
        c_ios[i]->io_in_progress = io_in_progress;
        c_ios[i]->t_spent = t_spent;
        c_ios[i]->t_weighted = t_weighted;
        i++;
    }
    fclose(fp);

    itv = get_interval(uptime0[!curr], uptime0[curr]);
                    
    for (i = 0; i < ndev; i++) {
        x_ios[i]->util = S_VALUE(p_ios[i]->t_spent, c_ios[i]->t_spent, itv);
        x_ios[i]->await = ((c_ios[i]->r_completed + c_ios[i]->w_completed) - (p_ios[i]->r_completed + p_ios[i]->w_completed)) ?
            ((c_ios[i]->r_spent - p_ios[i]->r_spent) + (c_ios[i]->w_spent - p_ios[i]->w_spent)) /
            ((double) ((c_ios[i]->r_completed + c_ios[i]->w_completed) - (p_ios[i]->r_completed + p_ios[i]->w_completed))) : 0.0;
        x_ios[i]->arqsz = ((c_ios[i]->r_completed + c_ios[i]->w_completed) - (p_ios[i]->r_completed + p_ios[i]->w_completed)) ?
            ((c_ios[i]->r_sectors - p_ios[i]->r_sectors) + (c_ios[i]->w_sectors - p_ios[i]->w_sectors)) /
            ((double) ((c_ios[i]->r_completed + c_ios[i]->w_completed) - (p_ios[i]->r_completed + p_ios[i]->w_completed))) : 0.0;

        r_await[i] = (c_ios[i]->r_completed - p_ios[i]->r_completed) ?
            (c_ios[i]->r_spent - p_ios[i]->r_spent) /
            ((double) (c_ios[i]->r_completed - p_ios[i]->r_completed)) : 0.0;
        w_await[i] = (c_ios[i]->w_completed - p_ios[i]->w_completed) ?
            (c_ios[i]->w_spent - p_ios[i]->w_spent) /
            ((double) (c_ios[i]->w_completed - p_ios[i]->w_completed)) : 0.0;
    }

    /* print headers */
    wclear(window);
    wattron(window, A_BOLD);
    wprintw(window, "\nDevice:           rrqm/s  wrqm/s      r/s      w/s    rMB/s    wMB/s avgrq-sz avgqu-sz     await   r_await   w_await   %%util\n");
    wattroff(window, A_BOLD);

    /* print statistics */
    for (i = 0; i < ndev; i++) {
        wprintw(window, "%s\t\t", c_ios[i]->devname);
        wprintw(window, "%8.2f%8.2f",
                S_VALUE(p_ios[i]->r_merged, c_ios[i]->r_merged, itv),
                S_VALUE(p_ios[i]->w_merged, c_ios[i]->w_merged, itv));
        wprintw(window, "%9.2f%9.2f",
                S_VALUE(p_ios[i]->r_completed, c_ios[i]->r_completed, itv),
                S_VALUE(p_ios[i]->w_completed, c_ios[i]->w_completed, itv));
        wprintw(window, "%9.2f%9.2f%9.2f%9.2f",
                S_VALUE(p_ios[i]->r_sectors, c_ios[i]->r_sectors, itv) / 2048,
                S_VALUE(p_ios[i]->w_sectors, c_ios[i]->w_sectors, itv) / 2048,
                x_ios[i]->arqsz,
                S_VALUE(p_ios[i]->t_weighted, c_ios[i]->t_weighted, itv) / 1000.0);
        wprintw(window, "%10.2f%10.2f%10.2f", x_ios[i]->await, r_await[i], w_await[i]);
        wprintw(window, "%8.2f", x_ios[i]->util / 10.0);
        wprintw(window, "\n");
    }
    wrefresh(window);

    /* save current stats snapshot and */
    replace_dstats(c_ios, p_ios, ndev);
    curr ^= 1;
}

/*
 ******************************************************** routine function **
 * Calculate column width for output data.
 *
 * IN:
 * @n_rows          Number of rows in query result.
 * @n_cols          Number of columns in query result.
 * @res             Query result.
 * @arr             Array with sorted result.
 *
 * OUT:
 * @columns         Struct with column names and their max width.
 ****************************************************************************
 */
void calculate_width(struct colAttrs *columns, PGresult *res, char ***arr, int n_rows, int n_cols)
{
    int i, col, row;

    for (col = 0, i = 0; col < n_cols; col++, i++) {
        /* determine length of column names */
        strcpy(columns[i].name, PQfname(res, col));
        int colname_len = strlen(PQfname(res, col));
        int width = colname_len;
        if (arr == NULL) {
            for (row = 0; row < n_rows; row++ ) {
                int val_len = strlen(PQgetvalue(res, row, col));
                if ( val_len >= width )
                    width = val_len;
            }
        } else {
            /* determine length of values from result array */
            for (row = 0; row < n_rows; row++ ) {
                int val_len = strlen(arr[row][col]);
                if ( val_len >= width )
                    width = val_len;
            }
        }
        columns[i].width = width + 2;
    }
}

/*
 ************************************************** system window function **
 * Get PostgreSQL uptime
 *
 * IN:
 * @conn
 ****************************************************************************
 */
void get_pg_uptime(PGconn * conn, char * uptime)
{
    char * errmsg = (char *) malloc(sizeof(char) * 1024);
    PGresult * res;

    if ((res = do_query(conn, PG_UPTIME_QUERY, errmsg)) != NULL) {
        strcpy(uptime, PQgetvalue(res, 0, 0));
        PQclear(res);
        free(errmsg);
    } else {
        strcpy(uptime, "--:--:--");
    }
}

/*
 ************************************************** system window function **
 * Print PostgreSQL general info
 *
 * IN:
 * @window          Window where resultwill be printing.
 * @screen          Screen with postgres version info.
 * @conn            Current connection.
 ****************************************************************************
 */
void print_pg_general(WINDOW * window, struct screen_s * screen, PGconn * conn)
{
    static char uptime[32];
    get_pg_uptime(conn, uptime);

    wprintw(window, " (ver: %s, up %s)", screen->pg_version, uptime);
}

/*
 ************************************************** system window function **
 * Print autovacuum info.
 *
 * IN:
 * @window          Window where resultwill be printing.
 * @ewindow         Window for error printing if query failed.
 * @conn            Current postgres connection.
 ****************************************************************************
 */
void print_autovac_info(WINDOW * window, PGconn * conn)
{
    int av_count, avw_count;
    char av_max_time[16];
    PGresult *res;
    char *errmsg = (char *) malloc(sizeof(char) * 1024);
    
    if ((res = do_query(conn, PG_STAT_ACTIVITY_AV_COUNT_QUERY, errmsg)) != NULL) {
        av_count = atoi(PQgetvalue(res, 0, 0));
        PQclear(res);
    } else {
        av_count = 0;
    }
    
    if ((res = do_query(conn, PG_STAT_ACTIVITY_AVW_COUNT_QUERY, errmsg)) != NULL) {
        avw_count = atoi(PQgetvalue(res, 0, 0));
        PQclear(res);
    } else {
        avw_count = 0;
    }
    
    if ((res = do_query(conn, PG_STAT_ACTIVITY_AV_LONGEST_QUERY, errmsg)) != NULL) {
        strcpy(av_max_time, PQgetvalue(res, 0, 0));
        PQclear(res);
    } else {
        strcpy(av_max_time, "--:--:--");
    }

    mvwprintw(window, 2, COLS / 2, "autovacuum: %2i workers, %2i wraparound, %s avw_maxtime",
                    av_count, avw_count, av_max_time);
    wrefresh(window);
    free(errmsg);
}

/*
 ****************************************************** key press function **
 * Switch console using specified number.
 *
 * IN:
 * @window          Window where cmd status will be written.
 * @screens[]       Struct array with screens options.
 * @ch              Intercepted key (number from 1 to 8).
 * @console_no      Active console number.
 * @console_index   Index of active console.
 * @first_iter      Reset previous results.
 *
 * RETURNS:
 * Index console on which performed switching.
 ****************************************************************************
 */
int switch_conn(WINDOW * window, struct screen_s * screens[],
                int ch, int console_index, int console_no, PGresult * res, bool * first_iter)
{
    wclear(window);
    if ( screens[ch - '0' - 1]->conn_used ) {
        console_no = ch - '0', console_index = console_no - 1;
        wprintw(window, "Switch to another pgbouncer connection (console %i)",
                console_no);
        *first_iter = true;
        PQclear(res);
    } else
        wprintw(window, "Do not switch because no connection associated (stay on console %i)",
                console_no);

    return console_index;
}

/*
 ******************************************************** routine function **
 * Allocate memory for 3D pointer array.
 *
 * IN:
 * @arr             3D pointer array.
 * @n_rows          Number of rows of current query result.
 * @n_cols          Number of columns of current query result.
 *
 * RETURNS:
 * Returns allocated space based on rows and column numbers.
 ****************************************************************************
 */
char *** init_array(char ***arr, int n_rows, int n_cols)
{
    int i, j;

    arr = malloc(sizeof(char **) * n_rows);
    for (i = 0; i < n_rows; i++) {
        arr[i] = malloc(sizeof(char *) * n_cols);
            for (j = 0; j < n_cols; j++)
                arr[i][j] = malloc(sizeof(char) * BUFSIZ);
    }
    return arr;
}

/*
 ******************************************************** routine function **
 * Free space occupied by 3D pointer arrays
 *
 * IN:      
 * @arr             3D pointer array.
 * @n_rows          Number of rows of current query result.
 * @n_cols          Number of columns of current query result.
 *
 * RETURNS:
 * Returns pointer to empty 3D pointer array.
 ****************************************************************************
 */
char *** free_array(char ***arr, int n_rows, int n_cols)
{
    int i, j;

    for (i = 0; i < n_rows; i++) {
        for (j = 0; j < n_cols; j++)
            free(arr[i][j]);
        free(arr[i]);
    }
    free(arr);
    return arr;
}

/*
 ******************************************************** routine function **
 * Copy database query results into array.
 *
 * IN:
 * @arr             3D pointer array where query results will be stored.
 * @res             Database query result.
 * @n_rows          Number of rows in query result.
 * @n_cols          Number of cols in query result.
 ****************************************************************************
 */
void pgrescpy(char ***arr, PGresult *res, int n_rows, int n_cols)
{
    int i, j;

    for (i = 0; i < n_rows; i++)
        for (j = 0; j < n_cols; j++) {
            strncpy(arr[i][j], PQgetvalue(res, i, j), BUFSIZ);
            arr[i][j][BUFSIZ] = '\0';
        }
}

/*
 ******************************************************** routime function **
 * Diff arrays and build array with deltas.
 *
 * IN:
 * @p_arr           Array with results of previous query.
 * @c_arr           Array with results of current query.
 * @context         Current used query.
 * @n_rows          Total number of rows from query result.
 * @n_cols          Total number of columns from query result.
 *
 * OUT:
 * @res_arr         Array where difference result will be stored.
 ****************************************************************************
 */
void diff_arrays(char ***p_arr, char ***c_arr, char ***res_arr, struct screen_s * screen, int n_rows, int n_cols, long int interval)
{
    int i, j, min = 0, max = 0;
    int divisor;
 
    switch (screen->current_context) {
        case pg_stat_database:
            min = PG_STAT_DATABASE_ORDER_MIN;
            if (atoi(screen->pg_version_num) < 90200)
                max = PG_STAT_DATABASE_ORDER_91_MAX;
            else
                max = PG_STAT_DATABASE_ORDER_LATEST_MAX;
            break;
        case pg_stat_replication:
            min = PG_STAT_REPLICATION_ORDER_MIN;
            /* don't diff last column */
            max = PG_STAT_REPLICATION_ORDER_MAX - 1;
            break;
        case pg_stat_tables:
            min = PG_STAT_TABLES_ORDER_MIN;
            max = PG_STAT_TABLES_ORDER_MAX;
            break;
        case pg_stat_indexes:
            min = PG_STAT_INDEXES_ORDER_MIN;
            max = PG_STAT_INDEXES_ORDER_MAX;
            break;
        case pg_statio_tables:
            min = PG_STATIO_TABLES_ORDER_MIN;
            max = PG_STATIO_TABLES_ORDER_MAX;
            break;
        case pg_tables_size:
            min = PG_TABLES_SIZE_ORDER_MIN;
            max = PG_TABLES_SIZE_ORDER_MAX;
            break;
        case pg_stat_activity_long:
            /* 
             * use INVALID_ORDER_KEY because here we no need array or sort diff.
             * copy current array content in result array as is.
             */
            min = PG_STAT_ACTIVITY_LONG_ORDER_MIN;
            max = PG_STAT_ACTIVITY_LONG_ORDER_MAX;
            break;
        case pg_stat_functions:
            /* only one column for diff */
            min = max = PG_STAT_FUNCTIONS_DIFF_COL;
            break;
        case pg_stat_statements_timing:
            if (atoi(screen->pg_version_num) < 90200) {
                min = PG_STAT_STATEMENTS_TIMING_DIFF_91_MIN;
                max = PG_STAT_STATEMENTS_TIMING_DIFF_91_MAX;
            } else {
                min = PG_STAT_STATEMENTS_TIMING_DIFF_LATEST_MIN;
                max = PG_STAT_STATEMENTS_TIMING_DIFF_LATEST_MAX;
            }
            break;
        case pg_stat_statements_general:
            min = PG_STAT_STATEMENTS_GENERAL_DIFF_MIN;
            max = PG_STAT_STATEMENTS_GENERAL_DIFF_MAX;
            break;
        case pg_stat_statements_io:
            if (atoi(screen->pg_version_num) < 90200) {
                min = PG_STAT_STATEMENTS_IO_DIFF_91_MIN;
                max = PG_STAT_STATEMENTS_IO_DIFF_91_MAX;
            } else {
                min = PG_STAT_STATEMENTS_IO_DIFF_LATEST_MIN;
                max = PG_STAT_STATEMENTS_IO_DIFF_LATEST_MAX;
            }
            break;
        case pg_stat_statements_temp:
            min = PG_STAT_STATEMENTS_TEMP_DIFF_MIN;
            max = PG_STAT_STATEMENTS_TEMP_DIFF_MAX;
            break;
        default:
            break;
    }

    divisor = interval / 1000000;
    for (i = 0; i < n_rows; i++) {
        for (j = 0; j < n_cols; j++)
            if (j < min || j > max)
                strcpy(res_arr[i][j], c_arr[i][j]);     /* copy unsortable values as is */
            else {
                int n = snprintf(NULL, 0, "%lli", atoll(c_arr[i][j]) - atoll(p_arr[i][j]));
                char buf[n+1];
                snprintf(buf, n+1, "%lli", (atoll(c_arr[i][j]) - atoll(p_arr[i][j])) / divisor);
                strcpy(res_arr[i][j], buf);
            }
    }
}

/*
 ******************************************************** routine function **
 * Sort array using specified order key (column number).
 *
 * IN:
 * @res_arr         Array which content will be sorted.
 * @n_rows          Number of rows in query result.
 * @n_cols          Number of columns in query result.
 * @screen          Current screen.
 *
 * OUT:
 * @res_arr         Sorted array.
 ****************************************************************************
 */
void sort_array(char ***res_arr, int n_rows, int n_cols, struct screen_s * screen)
{
    int i, j, x, order_key = 0;
    bool desc = false;

    for (i = 0; i < TOTAL_CONTEXTS; i++)
        if (screen->current_context == screen->context_list[i].context) {
            order_key = screen->context_list[i].order_key;
            desc = screen->context_list[i].order_desc;
        }

    /* some context show absolute values, and sorting perform only for one column */
    if (screen->current_context == pg_stat_functions && order_key != PG_STAT_FUNCTIONS_DIFF_COL)
        return;
    /* todo: here we not check pg_version_num and in old pg versions may have unexpected bahaviour */
    if (screen->current_context == pg_stat_statements_timing
            && order_key < PG_STAT_STATEMENTS_TIMING_DIFF_LATEST_MIN
            && order_key > PG_STAT_STATEMENTS_TIMING_DIFF_LATEST_MAX)
        return;
    if (screen->current_context == pg_stat_statements_general 
            && order_key < PG_STAT_STATEMENTS_GENERAL_DIFF_MIN 
            && order_key > PG_STAT_STATEMENTS_GENERAL_DIFF_MAX)
    /* todo: here we not check pg_version_num and in old pg versions may have unexpected bahaviour */
    if (screen->current_context == pg_stat_statements_io 
            && order_key < PG_STAT_STATEMENTS_IO_DIFF_LATEST_MIN 
            && order_key > PG_STAT_STATEMENTS_IO_DIFF_LATEST_MAX)
        return;
    if (screen->current_context == pg_stat_statements_temp 
            && order_key < PG_STAT_STATEMENTS_TEMP_DIFF_MIN 
            && order_key > PG_STAT_STATEMENTS_TEMP_DIFF_MAX)

    if (order_key == INVALID_ORDER_KEY)
        return;

    char *temp = malloc(sizeof(char) * BUFSIZ);
    for (i = 0; i < n_rows; i++) {
        for (j = i + 1; j < n_rows; j++) {
            if (desc)
                if (atoll(res_arr[j][order_key]) > atoll(res_arr[i][order_key])) {        // desc: j > i
                    for (x = 0; x < n_cols; x++) {
                        strcpy(temp, res_arr[i][x]);
                        strcpy(res_arr[i][x], res_arr[j][x]);
                        strcpy(res_arr[j][x], temp);
                    }
                }
            if (!desc)
                if (atoll(res_arr[i][order_key]) > atoll(res_arr[j][order_key])) {        // asc: i > j
                    for (x = 0; x < n_cols; x++) {
                        strcpy(temp, res_arr[i][x]);
                        strcpy(res_arr[i][x], res_arr[j][x]);
                        strcpy(res_arr[j][x], temp);
                    }
                }
        }
    }

    free(temp);
}

/*
 ******************************************************** routine function **
 * Print array content into ncurses screen.
 *
 * IN:
 * @window          Ncurses window where result will be printed.
 * @res             Query result, used for column width calculation.
 * @arr             Array which content will be printed.
 * @n_rows          Number of rows in query result.
 * @n_cols          Number of columns in query result.
 * @screen          Current screen, used for getting order key and highlight 
 *                  appropriate column.
 ****************************************************************************
 */
void print_data(WINDOW *window, PGresult *res, char ***arr, int n_rows, int n_cols, struct screen_s * screen)
{
    int i, j, x, order_key = 0;
    int winsz_x, winsz_y;
    struct colAttrs *columns = (struct colAttrs *) malloc(sizeof(struct colAttrs) * n_cols);

    calculate_width(columns, res, arr, n_rows, n_cols);
    wclear(window);

    for (i = 0; i < TOTAL_CONTEXTS; i++)
        if (screen->current_context == screen->context_list[i].context)
            order_key = screen->context_list[i].order_key;

    /* print header */
    wattron(window, A_BOLD);
    for (j = 0, x = 0; j < n_cols; j++, x++) {
        /* truncate last field length to end of screen */
        if (j == n_cols - 1) {
            getyx(window, winsz_y, winsz_x);
            columns[x].width = COLS - winsz_x - 1;
            /* dirty hack for supress gcc warning about "variable set but not used" */
            winsz_y--;
        } 
        /* mark sort column */
        if (j == order_key) {
            wattron(window, A_REVERSE);
            wprintw(window, "%-*s", columns[x].width, columns[x].name);
            wattroff(window, A_REVERSE);
        } else
            wprintw(window, "%-*s", columns[x].width, columns[x].name);
    }
    wprintw(window, "\n");
    wattroff(window, A_BOLD);

    /* print data from array */
    for (i = 0; i < n_rows; i++) {
        for (j = 0, x = 0; j < n_cols; j++, x++) {
            /* truncate last field length to end of screen */
            if (j == n_cols - 1) {
                getyx(window, winsz_y, winsz_x);
                columns[x].width = COLS - winsz_x;
                strncpy(arr[i][j], arr[i][j], columns[x].width);
                arr[i][j][columns[x].width] = '\0';
            }
            wprintw(window, "%-*s", columns[x].width, arr[i][j]);
        }
    }
    wrefresh(window);
    free(columns);
}

/*
 ****************************************************** key-press function **
 * Change column-based sort
 *
 * IN:
 * @screen              Current screen.
 * @increment           Direction (left or right column).
 * @first_iter          Flag for resetting previous query results.
 ****************************************************************************
 */
void change_sort_order(struct screen_s * screen, bool increment, bool * first_iter)
{
    int min = 0, max = 0, i;
    switch (screen->current_context) {
        case pg_stat_database:
            min = PG_STAT_DATABASE_ORDER_MIN;
            if (atoi(screen->pg_version_num) < 90200)
                max = PG_STAT_DATABASE_ORDER_91_MAX;
            else
                max = PG_STAT_DATABASE_ORDER_LATEST_MAX;
            break;
        case pg_stat_replication:
            min = PG_STAT_REPLICATION_ORDER_MIN;
            max = PG_STAT_REPLICATION_ORDER_MAX;
            break;
        case pg_stat_tables:
            min = PG_STAT_TABLES_ORDER_MIN;
            max = PG_STAT_TABLES_ORDER_MAX;
            break;
        case pg_stat_indexes:
            min = PG_STAT_INDEXES_ORDER_MIN;
            max = PG_STAT_INDEXES_ORDER_MAX;
            break;
        case pg_statio_tables:
            min = PG_STATIO_TABLES_ORDER_MIN;
            max = PG_STATIO_TABLES_ORDER_MAX;
            break;
        case pg_tables_size:
            min = PG_TABLES_SIZE_ORDER_MIN - 3;
            max = PG_TABLES_SIZE_ORDER_MAX;
            break;
        case pg_stat_activity_long:
            min = PG_STAT_ACTIVITY_LONG_ORDER_MIN;
            max = PG_STAT_ACTIVITY_LONG_ORDER_MIN;
            break;
        case pg_stat_functions:
            min = PG_STAT_FUNCTIONS_ORDER_MIN;
            max = PG_STAT_FUNCTIONS_ORDER_MAX;
            *first_iter = true;
            break;
        case pg_stat_statements_timing:
            min = PG_STAT_STATEMENTS_TIMING_ORDER_MIN;
            if (atoi(screen->pg_version_num) < 90200)
                max = PG_STAT_STATEMENTS_TIMING_ORDER_91_MAX;
            else
                max = PG_STAT_STATEMENTS_TIMING_ORDER_LATEST_MAX;
            *first_iter = true;
            break;
        case pg_stat_statements_general:
            min = PG_STAT_STATEMENTS_GENERAL_ORDER_MIN;
            max = PG_STAT_STATEMENTS_GENERAL_ORDER_MAX;
            *first_iter = true;
            break;
        case pg_stat_statements_io:
            min = PG_STAT_STATEMENTS_IO_ORDER_MIN;
            if (atoi(screen->pg_version_num) < 90200)
                max = PG_STAT_STATEMENTS_IO_ORDER_91_MAX;
            else
                max = PG_STAT_STATEMENTS_IO_ORDER_LATEST_MAX;
            *first_iter = true;
            break;
        case pg_stat_statements_temp:
            min = PG_STAT_STATEMENTS_TEMP_ORDER_MIN;
            max = PG_STAT_STATEMENTS_TEMP_ORDER_MAX;
            *first_iter = true;
            break;
        default:
            break;
    }
    if (increment) {
        for (i = 0; i < TOTAL_CONTEXTS; i++) {
            if (screen->current_context == screen->context_list[i].context) {
                if (screen->context_list[i].order_key + 1 > max)
                    screen->context_list[i].order_key = min;
                else 
                    screen->context_list[i].order_key++;
            }
        }
    }

    if (!increment)
        for (i = 0; i < TOTAL_CONTEXTS; i++) {
            if (screen->current_context == screen->context_list[i].context) {
                if (screen->context_list[i].order_key - 1 < min)
                    screen->context_list[i].order_key = max;
                else
                    screen->context_list[i].order_key--;
            }
        }
}

/*
 ****************************************************** key-press function **
 * Change column-based sort
 *
 * IN:
 * @screen              Current screen.
 ****************************************************************************
 */
void change_sort_order_direction(struct screen_s * screen, bool * first_iter)
{
    int i;
    for (i = 0; i < TOTAL_CONTEXTS; i++) {
        if (screen->current_context == screen->context_list[i].context) {
            if (screen->context_list[i].order_desc)
                screen->context_list[i].order_desc = false;
            else
                screen->context_list[i].order_desc = true;
        }
        *first_iter = true;
    }
}

/*
 ***************************************************** cmd window function **
 * Read input from cmd window.
 *
 * IN:
 * @window          Window where pause status will be printed.
 * @msg             Message prompt.
 * @pos             When you delete wrong input, cursor do not moving beyond.
 * @len             Max allowed length of string.
 * @echoing         Show characters typed by the user.
 *
 * OUT:
 * @with_esc        Flag which determines when function finish with ESC.
 * @str             Entered string.             
 *
 * RETURNS:
 * Pointer to the input string.
 ****************************************************************************
 */
void cmd_readline(WINDOW *window, char * msg, int pos, bool * with_esc, char * str, int len, bool echoing)
{
    int ch;
    int i = 0;
    bool done = false;

    if (echoing)
        echo();
    cbreak();
    nodelay(window, FALSE);
    keypad(window, TRUE);

    /* show prompt if msg not empty */
    if (strlen(msg) != 0) {
        wprintw(window, "%s", msg);
        wrefresh(window);
    }

    memset(str, 0, len);
    while (1) {
        if (done)
            break;
        ch = wgetch(window);
        switch (ch) {
            case ERR:
                strcpy(str, "\0");
                flushinp();
                done = true;
                break;
            case 27:                            /* Esc */
                wclear(window);
                wprintw(window, "Do nothing. Operation canceled. ");
                nodelay(window, TRUE);
                *with_esc = true;
                strcpy(str, "\0");
                flushinp();
                done = true;
                break;
            case 10:                            /* Enter */
                strncpy(str, str, len);
                str[len] = '\0';
                flushinp();
                nodelay(window, TRUE);
                *with_esc = false;              /* normal finish with \n */
                done = true;
                break;
            case 263: case 330: case 127:       /* Backspace, Delete, */
                if (i > 0) {
                    i--;
                    wdelch(window);
                    continue;
                } else {
                    wmove(window, 0, pos);
                    continue;
                }
                break;
            default:
                if (strlen(str) < len + 1) {
                    str[i] = ch;
                    i++;
                }
                break;
        }
    }

    noecho();
    cbreak();
    nodelay(window, TRUE);
    keypad(window, FALSE);
}

/*
 ****************************************************** key-press function **
 * Change pg_stat_activity long queries min age
 *
 * IN:
 * @window              Ncurses window where msg will be printed.
 * @screen              Current screen.
 ****************************************************************************
 */
void change_min_age(WINDOW * window, struct screen_s * screen, PGresult *res, bool *first_iter)
{
    if (screen->current_context != pg_stat_activity_long) {
        wprintw(window, "Long query min age not allowed here.");
        return;
    }

    unsigned int hour, min, sec;
    bool * with_esc = (bool *) malloc(sizeof(bool));
    char min_age[BUFFERSIZE_S],
         msg[] = "Enter new min age, format: HH:MM:SS[.NN]: ";

    cmd_readline(window, msg, 42, with_esc, min_age, 16, true);
    if (strlen(min_age) != 0 && *with_esc == false) {
        if ((sscanf(min_age, "%u:%u:%u", &hour, &min, &sec)) == 0 || (hour > 23 || min > 59 || sec > 59)) {
            wprintw(window, "Nothing to do. Failed read or invalid value.");
        } else {
            strncpy(screen->pg_stat_activity_min_age, min_age, sizeof(screen->pg_stat_activity_min_age) - 1);
            screen->pg_stat_activity_min_age[sizeof(screen->pg_stat_activity_min_age) - 1] = '\0';
        }
    } else if (strlen(min_age) == 0 && *with_esc == false ) {
        wprintw(window, "Nothing to do. Leave min age %s", screen->pg_stat_activity_min_age);
    }
   
    PQclear(res);
    *first_iter = true;

    free(with_esc);
}

/*
 ******************************************************** routine function **
 * Clear single connection options.
 *
 * IN:
 * @screens         Screens array.
 * @i               Index of entry in conn_opts array which should cleared.
 ****************************************************************************
 */
void clear_screen_connopts(struct screen_s * screens[], int i)
{
    strcpy(screens[i]->host, "");
    strcpy(screens[i]->port, "");
    strcpy(screens[i]->user, "");
    strcpy(screens[i]->dbname, "");
    strcpy(screens[i]->password, "");
    strcpy(screens[i]->conninfo, "");
    screens[i]->conn_used = false;
}

/*
 ******************************************************** routine function **
 * Shift screens when current screen closes.
 *
 * IN:
 * @screens         Screens array.
 * @conns           Connections array.
 * @i               Index of closed console.
 *
 * RETURNS:
 * Array of screens without closed connection.
 ****************************************************************************
 */
void shift_screens(struct screen_s * screens[], PGconn * conns[], int i)
{
    while (screens[i + 1]->conn_used != false) {
        strcpy(screens[i]->host,        screens[i + 1]->host);
        strcpy(screens[i]->port,        screens[i + 1]->port);
        strcpy(screens[i]->user,        screens[i + 1]->user);
        strcpy(screens[i]->dbname,      screens[i + 1]->dbname);
        strcpy(screens[i]->password,    screens[i + 1]->password);
        strcpy(screens[i]->pg_version_num,  screens[i + 1]->pg_version_num);
        strcpy(screens[i]->pg_version,  screens[i + 1]->pg_version);
        screens[i]->subscreen =        screens[i + 1]->subscreen;
        strcpy(screens[i]->log_path,    screens[i + 1]->log_path);
        screens[i]->log_fd =            screens[i + 1]->log_fd;
        screens[i]->current_context =   screens[i + 1]->current_context;
        strcpy(screens[i]->pg_stat_activity_min_age, screens[i + 1]->pg_stat_activity_min_age);
        screens[i]->signal_options =    screens[i + 1]->signal_options;
        screens[i]->pg_stat_sys =       screens[i + 1]->pg_stat_sys;

        conns[i] = conns[i + 1];
        i++;
        if (i == MAX_SCREEN - 1)
            break;
    }
    clear_screen_connopts(screens, i);
}

/*
 ****************************************************** key press function **
 * Open new one connection.
 *
 * IN:
 * @window          Window where result is printed.
 * @screen          Current screen.
 *
 * OUT:
 * @conns           Array of connections.
 * @screen          Connections options array.
 * @console_index   Index of screen (for internal usage).
 *
 * RETURNS:
 * Add connection into conns array and return new console index.
 ****************************************************************************
 */
int add_connection(WINDOW * window, struct screen_s * screens[],
                PGconn * conns[], int console_index)
{
    int i;
    char params[128],
         msg[] = "Enter new connection parameters, format \"host port username dbname\": ",
         msg2[] = "Required password: ";
    bool * with_esc = (bool *) malloc(sizeof(bool)),
         * with_esc2 = (bool *) malloc(sizeof(bool));
    
    for (i = 0; i < MAX_SCREEN; i++) {
        /* search free screen */
        if (screens[i]->conn_used == false) {

            /* read user input */
            cmd_readline(window, msg, 69, with_esc, params, 128, true);
            if (strlen(params) != 0 && *with_esc == false) {
                /* parse user input */
                if ((sscanf(params, "%s %s %s %s",
                    screens[i]->host,   screens[i]->port,
                    screens[i]->user,   screens[i]->dbname)) == 0) {
                        wprintw(window, "Nothing to do. Failed read or invalid value.");
                        break;
                }
                /* setup screen conninfo settings */
                screens[i]->conn_used = true;
                strcat(screens[i]->conninfo, "host=");
                strcat(screens[i]->conninfo, screens[i]->host);
                strcat(screens[i]->conninfo, " port=");
                strcat(screens[i]->conninfo, screens[i]->port);
                strcat(screens[i]->conninfo, " user=");
                strcat(screens[i]->conninfo, screens[i]->user);
                strcat(screens[i]->conninfo, " dbname=");
                strcat(screens[i]->conninfo, screens[i]->dbname);

                /* establish new connection */
                conns[i] = PQconnectdb(screens[i]->conninfo);
                /* if password required, ask user for password */
                if ( PQstatus(conns[i]) == CONNECTION_BAD && PQconnectionNeedsPassword(conns[i]) == 1) {
                    PQfinish(conns[i]);
                    wclear(window);

                    /* read password and add to conn options */
                    cmd_readline(window, msg2, 19, with_esc2, params, 128, false);
                    if (strlen(params) != 0 && *with_esc2 == false) {
                        strcpy(screens[i]->password, params);
                        strcat(screens[i]->conninfo, " password=");
                        strcat(screens[i]->conninfo, screens[i]->password);
                        /* try establish connection and finish work */
                        conns[i] = PQconnectdb(screens[i]->conninfo);
                        if ( PQstatus(conns[i]) == CONNECTION_BAD ) {
                            wclear(window);
                            wprintw(window, "Nothing to fo. Connection failed.");
                            PQfinish(conns[i]);
                            clear_screen_connopts(screens, i);
                        } else {
                            wclear(window);
                            wprintw(window, "Successfully connected.");
                            console_index = screens[i]->screen;
                        }
                    } else if (with_esc) {
                        clear_screen_connopts(screens, i);
                    }
                /* finish work if connection establish failed */
                } else if ( PQstatus(conns[i]) == CONNECTION_BAD ) {
                    wprintw(window, "Nothing to do. Connection failed.");
                    PQfinish(conns[i]);
                    clear_screen_connopts(screens, i);
                /* if no error occured, print about success and finish work */
                } else {
                    wclear(window);
                    wprintw(window, "Successfully connected.");
                    console_index = screens[i]->screen;
                }
                break;
            /* finish work if user input empty or cancelled */
            } else if (strlen(params) == 0 && *with_esc == false) {
                wprintw(window, "Nothing to do.");
                break;
            } else 
                break;
        /* also finish work if no available screens */
        } else if (i == MAX_SCREEN - 1) {
            wprintw(window, "No free consoles.");
        }
    }

    /* get PostgreSQL version */
    if (PQstatus(conns[i]) == CONNECTION_OK) {
        get_pg_version(conns[i], screens[i]);
    }
    
    /* finish work */
    free(with_esc);
    free(with_esc2);

    return console_index;
}

/*
 ***************************************************** key press functions **
 * Close current connection.
 *
 * IN:
 * @window          Window where result is printed.
 * @screens         Screens array.
 *
 * OUT:
 * @conns           Array of connections.
 * @screens         Modified screens array.
 * @first_iter      Reset stats counters.
 *
 * RETURNS:
 * Close current connection (remove from conns array) and return prvious
 * console index.
 ****************************************************************************
 */
int close_connection(WINDOW * window, struct screen_s * screens[],
                PGconn * conns[], int console_index, bool * first_iter)
{
    int i = console_index;
    PQfinish(conns[console_index]);

    wprintw(window, "Close current connection.");
    if (i == 0) {                               /* first console active */
        if (screens[i + 1]->conn_used) {
        shift_screens(screens, conns, i);
        } else {
            wrefresh(window);
            endwin();
            exit(EXIT_SUCCESS);
        }
    } else if (i == (MAX_SCREEN - 1)) {        /* last possible console active */
        clear_screen_connopts(screens, i);
        console_index = console_index - 1;
    } else {                                    /* in the middle console active */
        if (screens[i + 1]->conn_used) {
            shift_screens(screens, conns, i);
        } else {
            clear_screen_connopts(screens, i);
            console_index = console_index - 1;
        }
    }

    *first_iter = true;
    return console_index;
}

/*
 ****************************************************** key press function **
 * Write connection information into ~/.pgcenterrc.
 *
 * IN:
 * @window          Window where result will be printed.
 * @screens         Array of screens options.
 * @args            Struct where stored input args.
 ****************************************************************************
 */
void write_pgcenterrc(WINDOW * window, struct screen_s * screens[], PGconn * conns[], struct args_s * args)
{
    int i = 0;
    FILE *fp;
    static char pgcenterrc_path[PATH_MAX];
    struct passwd *pw = getpwuid(getuid());
    struct stat statbuf;

    /* 
     * write conninfo into file which specified in --file=FILENAME,
     * or use default ~/.pgcenterrc
     */
    if (strlen(args->connfile) != 0)
        strcpy(pgcenterrc_path, args->connfile);
    else {
        strcpy(pgcenterrc_path, pw->pw_dir);
        strcat(pgcenterrc_path, "/");
        strcat(pgcenterrc_path, PGCENTERRC_FILE);
    }

    if ((fp = fopen(pgcenterrc_path, "w")) != NULL ) {
        for (i = 0; i < MAX_SCREEN; i++) {
            if (screens[i]->conn_used) {
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
        wprintw(window, "Failed write configuration to '%s'", pgcenterrc_path);
    }
}

/*
 ****************************************************** key press function **
 * Show the current configuration settings, one per row.
 *
 * IN:
 * @window      Window where result will be printed.
 * @conn        Current postgres connection.
 ****************************************************************************
 */
void show_config(WINDOW * window, PGconn * conn)
{
    int  row_count, col_count, row, col, i;
    FILE * fpout;
    PGresult * res;
    char * errmsg;
    char pager[32] = "";
    struct colAttrs * columns;

    if (getenv("PAGER") != NULL)
        strcpy(pager, getenv("PAGER"));
    else
        strcpy(pager, DEFAULT_PAGER);
    if ((fpout = popen(pager, "w")) == NULL) {
        wprintw(window, "Do nothing. Failed to open pipe to %s", pager);
        return;
    }

    /* escape from ncurses mode */
    refresh();
    endwin();

    errmsg = (char *) malloc(sizeof(char) * 1024);
    res = do_query(conn, PG_SETTINGS_QUERY, errmsg);
    row_count = PQntuples(res);
    col_count = PQnfields(res);
    columns = (struct colAttrs *) malloc(sizeof(struct colAttrs) * col_count);
    calculate_width(columns, res, NULL, row_count, col_count);
    
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
    free(errmsg);
    
    /* return to ncurses mode */
    refresh();
}

/*
 ****************************************************** key press function **
 * Reload postgres
 *
 * IN:
 * @window      Window where resilt will be printed.
 * @conn        Current postgres connection.
 ****************************************************************************
 */
void reload_conf(WINDOW * window, PGconn * conn)
{
    PGresult * res;
    bool * with_esc = (bool *) malloc(sizeof(bool));
    char * errmsg = (char *) malloc(sizeof(char) * 1024);
    char confirmation[1],
         msg[] = "Reload configuration files (y/n): ";

    cmd_readline(window, msg, 34, with_esc, confirmation, 1, true);
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
    } else if (strlen(confirmation) == 0 && *with_esc == false) {
        wprintw(window, "Do nothing. Nothing etntered.");
    } else if (*with_esc == true) {
        ;
    } else 
        wprintw(window, "Do nothing. Not confirmed.");

    free(with_esc);
    free(errmsg);
}

/*
 ******************************************************** routine function **
 * Get postgres listen_addresses and check is that local address or not.
 *
 * IN:
 * @screen       Connections options.
 *
 * RETURNS:
 * Return true if listen_addresses is local and false if not.
 ****************************************************************************
 */
bool check_pg_listen_addr(struct screen_s * screen)
{
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
                printf("getnameinfo() failed: %s\n", gai_strerror(s));
                return false;
            }
            if (!strcmp(host, screen->host) || !strncmp(screen->host, "/", 1)) {
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
 ******************************************************** routine function **
 * Get GUC value from postgres config.
 *
 * IN:
 * @window                  Window for printing errors if occurs.
 * @conn                    Current connection.
 * @config_option_name      Option name.
 *
 * OUT:
 * @config_option_value     Config option value or empty string.
 ****************************************************************************
 */
void get_conf_value(PGconn * conn, char * config_option_name, char * config_option_value)
{
    PGresult * res;
    char *errmsg = (char *) malloc(sizeof(char) * 1024);
    char query[BUFSIZ];

    strcpy(query, PG_SETTINGS_SINGLE_OPT_P1);
    strcat(query, config_option_name);
    strcat(query, PG_SETTINGS_SINGLE_OPT_P2);

    res = do_query(conn, query, errmsg);
    
    if (PQntuples(res) != 0 && !strcmp(PQgetvalue(res, 0, 0), config_option_name))
        strcpy(config_option_value, PQgetvalue(res, 0, 1));
    else
        strcpy(config_option_value, "");
    
    free(errmsg);
    PQclear(res);
}

/*
 ******************************************************** routine function **
 * Get postgres version and save into screen opts.
 *
 * IN:
 * @conn                    Current connection.
 * @screen                  Current screen.
 ****************************************************************************
 */
void get_pg_version(PGconn * conn, struct screen_s * screen)
{
    get_conf_value(conn, GUC_SERVER_VERSION_NUM, screen->pg_version_num);
    get_conf_value(conn, GUC_SERVER_VERSION, screen->pg_version);
    if (strlen(screen->pg_version_num) == 0)
        strcpy(screen->pg_version_num, "-.-.-");
    if (strlen(screen->pg_version) == 0)
        strcpy(screen->pg_version, "-.-.-");
}

/*
 ****************************************************** key press function **
 * Edit the current configuration settings.
 *
 * IN:
 * @window          Window where errors will be displayed.
 * @screen          Screen options.
 * @conn            Current connection.
 * @config_file_guc GUC option associated with postgresql/pg_hba/pg_ident
 *
 * RETURNS:
 * Open configuration file in $EDITOR.
 ****************************************************************************
 */
void edit_config(WINDOW * window, struct screen_s * screen, PGconn * conn, char * config_file_guc)
{
    char * config_path = (char *) malloc(sizeof(char) * 128);
    pid_t pid;

    if (check_pg_listen_addr(screen)
                || (PQstatus(conn) == CONNECTION_OK && PQhost(conn) == NULL)) {
        get_conf_value(conn, config_file_guc, config_path);
        if (strlen(config_path) != 0) {
            /* if we want edit recovery.conf, attach config name to data_directory path */
            if (!strcmp(config_file_guc, GUC_DATA_DIRECTORY)) {
                strcat(config_path, "/");
                strcat(config_path, PG_RECOVERY_FILE);
            }
            /* escape from ncurses mode */
            refresh();
            endwin();
            pid = fork();                   /* start child */
            if (pid == 0) {
                char * editor = (char *) malloc(sizeof(char) * 128);
                if ((editor = getenv("EDITOR")) == NULL)
                    editor = DEFAULT_EDITOR;
                execlp(editor, editor, config_path, NULL);
                free(editor);
                exit(EXIT_FAILURE);
            } else if (pid < 0) {
                wprintw(window, "Can't open %s: fork failed.", config_path);
                return;
            } else if (waitpid(pid, NULL, 0) != pid) {
                wprintw(window, "Unknown error: waitpid failed.");
                return;
            }
        } else {
            wprintw(window, "Do nothing. Config option not found (not SUPERUSER?).");
        }
    } else {
        wprintw(window, "Do nothing. Edit config not supported for remote hosts.");
    }
    free(config_path);

    /* return to ncurses mode */
    refresh();
    return;
}

/*
 ****************************************************** key press function **
 * Invoke menu for config editing.
 *
 * IN:
 * @w_cmd           Window where errors will be displayed.
 * @w_dba           Window where db answers printed.
 * @screen          Current screen settings.
 * @conn            Current connection.
 * @first_iter      Reset counters when function ends.
 ****************************************************************************
 */
void edit_config_menu(WINDOW * w_cmd, WINDOW * w_dba, struct screen_s * screen, PGconn * conn, bool *first_iter)
{
    char *choices[] = { "postgresql.conf", "pg_hba.conf", "pg_ident.conf", "recovery.conf" };
    WINDOW *menu_win;
    MENU *menu;
    ITEM **items;
    int n_choices, c, i;
    bool done = false;

    cbreak();
    noecho();
    keypad(stdscr, TRUE);

    /* allocate stuff */
    n_choices = ARRAY_SIZE(choices);
    items = (ITEM**) malloc(sizeof(ITEM *) * (n_choices + 1));
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
        c = wgetch(menu_win);
        switch (c) {
            case KEY_DOWN:
                menu_driver(menu, REQ_DOWN_ITEM);
                break;
            case KEY_UP:
                menu_driver(menu, REQ_UP_ITEM);
                break;
            case 10:
                if (!strcmp(item_name(current_item(menu)), PG_CONF_FILE))
                    edit_config(w_cmd, screen, conn, GUC_CONFIG_FILE);
                else if (!strcmp(item_name(current_item(menu)), PG_HBA_FILE))
                    edit_config(w_cmd, screen, conn, GUC_HBA_FILE);
                else if (!strcmp(item_name(current_item(menu)), PG_IDENT_FILE))
                    edit_config(w_cmd, screen, conn, GUC_IDENT_FILE);
                else if (!strcmp(item_name(current_item(menu)), PG_RECOVERY_FILE))
                    edit_config(w_cmd, screen, conn, GUC_DATA_DIRECTORY);
                else
                    wprintw(w_cmd, "Do nothing. Unknown file.");     /* never should be here. */
                done = true;
                break;
            case 27:
                done = true;
                break;
        }       
    }
 
    /* clear menu items from screen */
    clear();
    refresh();

    /* free stuff */
    unpost_menu(menu);
    for (i = 0; i < n_choices; i++)
        free_item(items[i]);
    free_menu(menu);
    free(items);
    delwin(menu_win);
    *first_iter = true;
}

/*
 ****************************************************** key press function **
 * Cancel or terminate postgres backend.
 *
 * IN:
 * @window          Window where resilt will be printed.
 * @screen          Current screen info.
 * @conn            Current postgres connection.
 * @do_terminate    Do terminate backend if true or cancel if false.
 ****************************************************************************
 */
void signal_single_backend(WINDOW * window, struct screen_s *screen, PGconn * conn, bool do_terminate)
{
    if (screen->current_context != pg_stat_activity_long) {
        wprintw(window, "Terminate or cancel backend allowed in long queries screen.");
        return;
    } 

    char query[BUFSIZ],
         action[10],
         msg[64],
         pid[6];
    PGresult * res;
    bool * with_esc = (bool *) malloc(sizeof(bool));
    int msg_offset;
    char * errmsg = (char *) malloc(sizeof(char) * 1024);

    if (do_terminate) {
        strcpy(action, "Terminate");
        strcpy(msg, "Terminate single backend, enter pid: ");
        msg_offset = 37;
    } else {
        strcpy(action, "Cancel");
        strcpy(msg, "Cancel single backend, enter pid: ");
        msg_offset = 34;
    }

    cmd_readline(window, msg, msg_offset, with_esc, pid, 64, true);
    if (atoi(pid) > 0) {
        if (do_terminate) {
            strcpy(query, PG_TERM_BACKEND_P1);
            strcat(query, pid);
            strcat(query, PG_TERM_BACKEND_P2);
        } else {
            strcpy(query, PG_CANCEL_BACKEND_P1);
            strcat(query, pid);
            strcat(query, PG_CANCEL_BACKEND_P2);
        }

        res = do_query(conn, query, errmsg);
        if (res != NULL) {
            wprintw(window, "%s backend with pid %s.", action, pid);
            PQclear(res);
        } else {
            wprintw(window, "%s backend failed. %s", action, errmsg);
        }
    } else if (strlen(pid) == 0 && *with_esc == false) {
        wprintw(window, "Do nothing. Nothing etntered.");
    } else if (*with_esc == true) {
        ;
    } else
        wprintw(window, "Do nothing. Incorrect input value.");

    free(with_esc);
    free(errmsg);
}

/*
 ****************************************************** key press function **
 * Print current mask for group cancel/terminate
 *
 * IN:
 * @window          Window where resilt will be printed.
 * @screen          Current screen info.
 ****************************************************************************
 */
void get_statemask(WINDOW * window, struct screen_s * screen)
{
    if (screen->current_context != pg_stat_activity_long) {
        wprintw(window, "Get current mask can viewed in long queries screen.");
        return;
    }

    wprintw(window, "Mask: ");
    if (screen->signal_options == 0)
        wprintw(window, "empty");
    if (screen->signal_options & GROUP_ACTIVE)
        wprintw(window, "active ");
    if (screen->signal_options & GROUP_IDLE)
        wprintw(window, "idle ");
    if (screen->signal_options & GROUP_IDLE_IN_XACT)
        wprintw(window, "idle in xact ");
    if (screen->signal_options & GROUP_WAITING)
        wprintw(window, "waiting ");
    if (screen->signal_options & GROUP_OTHER)
        wprintw(window, "other ");
}


/*
 ****************************************************** key press function **
 * Set state mask for group cancel/terminate
 *
 * IN:
 * @window          Window where resilt will be printed.
 * @screen          Current screen info.
 ****************************************************************************
 */
void set_statemask(WINDOW * window, struct screen_s * screen)
{
    if (screen->current_context != pg_stat_activity_long) {
        wprintw(window, "State mask setup allowed in long queries screen.");
        return;
    } 

    int i;
    char mask[5],
         msg[] = "";        /* set empty message, we don't want show msg from cmd_readline */
    bool * with_esc = (bool *) malloc(sizeof(bool));

    wprintw(window, "Set action mask for group backends [");
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

    cmd_readline(window, msg, 77, with_esc, mask, 5, true);
    if (strlen(mask) > 5) {                                 /* entered mask too long */
        wprintw(window, "Do nothing. Mask too long.");
    } else if (strlen(mask) == 0 && *with_esc == false) {   /* mask not entered */
        wprintw(window, "Do nothing. Mask not entered.");
    } else if (*with_esc == true) {                         /* user escaped */
        ;
    } else {                                                /* user enter string with valid length */
        /* reset previous mask */
        screen->signal_options = 0;
        for (i = 0; i < strlen(mask); i++) {
            switch (mask[i]) {
                case 'a':
                    screen->signal_options |= GROUP_ACTIVE;
                    break;
                case 'i':
                    screen->signal_options |= GROUP_IDLE;
                    break;
                case 'x':
                    screen->signal_options |= GROUP_IDLE_IN_XACT;
                    break;
                case 'w':
                    screen->signal_options |= GROUP_WAITING;
                    break;
                case 'o':
                    screen->signal_options |= GROUP_OTHER;
                    break;
            }
        }
        get_statemask(window, screen);
    }

    free(with_esc);
}

/*
 ****************************************************** key press function **
 * Cancel or terminate postgres backends using state mask.
 *
 * IN:
 * @window          Window where resilt will be printed.
 * @screen          Current screen info.
 * @conn            Current postgres connection.
 * @do_terminate    Do terminate backend if true or cancel if false.
 ****************************************************************************
 */
void signal_group_backend(WINDOW * window, struct screen_s *screen, PGconn * conn, bool do_terminate)
{
    if (screen->current_context != pg_stat_activity_long) {
        wprintw(window, "Terminate or cancel backend allowed in long queries screen.");
        return;
    } 
    if (screen->signal_options == 0) {
        wprintw(window, "Do nothing. Mask not set.");
        return;
    }

    char query[512],
         mask[5] = "",
         action[10],
         state[80];
    PGresult * res;
    int i, signaled = 0;

    if (do_terminate)
        strcpy(action, "terminate");
    else
        strcpy(action, "cancel");
    
    if (screen->signal_options & GROUP_ACTIVE)
        strcat(mask, "a");
    if (screen->signal_options & GROUP_IDLE)
        strcat(mask, "i");
    if (screen->signal_options & GROUP_IDLE_IN_XACT)
        strcat(mask, "x");
    if (screen->signal_options & GROUP_WAITING)
        strcat(mask, "w");
    if (screen->signal_options & GROUP_OTHER)
        strcat(mask, "o");

    for (i = 0; i < strlen(mask); i++) {
        switch (mask[i]) {
            case 'a':
                strcpy(state, "state = 'active'");
                break;
            case 'i':
                strcpy(state, "state = 'idle'");
                break;
            case 'x':
                strcpy(state, "state IN ('idle in transaction (aborted)', 'idle in transaction')");
                break;
            case 'w':
                strcpy(state, "waiting");
                break;
            case 'o':
                strcpy(state, "state IN ('fastpath function call', 'disabled')");
                break;
            default:
                break;
        }
        strcpy(query, PG_SIG_GROUP_BACKEND_P1);
        strcat(query, action);
        strcat(query, PG_SIG_GROUP_BACKEND_P2);
        strcat(query, state);
        strcat(query, PG_SIG_GROUP_BACKEND_P3);
        strcat(query, screen->pg_stat_activity_min_age);
        strcat(query, PG_SIG_GROUP_BACKEND_P4);
        strcat(query, screen->pg_stat_activity_min_age);
        strcat(query, PG_SIG_GROUP_BACKEND_P5);
        
        char * errmsg = (char *) malloc(sizeof(char) * 1024);
        res = do_query(conn, query, errmsg);
        signaled = signaled + PQntuples(res);
        PQclear(res);
        free(errmsg);
    }

    if (do_terminate)
        wprintw(window, "Terminated %i processes.", signaled);
    else
        wprintw(window, "Canceled %i processes.", signaled);
}

/*
 ****************************************************** key press function **
 * Start psql using screen connection options.
 *
 * IN:
 * @window          Window where errors will be displayed if occurs.
 * @screen          Screen options.
 ****************************************************************************
 */
void start_psql(WINDOW * window, struct screen_s * screen)
{
    pid_t pid;
    char psql[16] = DEFAULT_PSQL;

    /* escape from ncurses mode */
    refresh();
    endwin();
    /* ignore Ctrl+C in child when psql running */
    signal(SIGINT, SIG_IGN);
    pid = fork();                   /* start child */
    if (pid == 0) {
        execlp(psql, psql,
                "-h", screen->host,
                "-p", screen->port,
                "-U", screen->user,
                "-d", screen->dbname,
                NULL);
        exit(EXIT_SUCCESS);         /* finish child */
    } else if (pid < 0) {
        wprintw(window, "Can't exec %s: fork failed.", psql);
    } else if (waitpid(pid, NULL, 0) != pid) {
        wprintw(window, "Unknown error: waitpid failed.");
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
 ****************************************************** key press function **
 * Change refresh interval.
 *
 * IN:
 * @window              Window where prompt will be printed.
 * 
 * OUT:
 * @interval            Interval.
 ****************************************************************************
 */
long int change_refresh(WINDOW * window, long int interval)
{
    long int interval_save = interval;
    char value[8],
         msg[64];
    bool * with_esc = (bool *) malloc(sizeof(bool));
    char * str = (char *) malloc(sizeof(char) * 128);

    wprintw(window, "Change refresh interval from %i to ", interval / 1000000);
    wrefresh(window);

    cmd_readline(window, msg, 36, with_esc, str, 8, true);
    strcpy(value, str);
    free(str);

    if (strlen(value) != 0 && *with_esc == false) {
        if (strlen(value) != 0) {
            interval = atol(value);
            if (interval < 1) {
                wprintw(window, "Should not be less than 1 second.");
                interval = interval_save;
            } else {
                interval = interval * 1000000;
            }
        }
    } else if (strlen(value) == 0 && *with_esc == false ) {
        interval = interval_save;
    }

    free(with_esc);
    return interval;
}

/*
 ****************************************************** key press function **
 * Pause pgcenter execution.
 *
 * IN:
 * @window              Window where diag message will be printed.
 * @interval            Sleep interval.
 ****************************************************************************
 */
void do_noop(WINDOW * window, long int interval)
{
    bool paused = true;
    int sleep_usec,
        ch;

    while (paused != false) {
        wprintw(window, "pgCenter suspended, press any key to resume.");
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
 ****************************************************** key-press function **
 * Switch on/off displaying content from system view
 *
 * IN:
 * @window              Window where diag messages will be printed.
 * @screen              Current screen.
 * @first_iter          Reset counters flag.
 ****************************************************************************
 */
void system_view_toggle(WINDOW * window, struct screen_s * screen, bool * first_iter)
{
    if (screen->pg_stat_sys) {
        screen->pg_stat_sys = false;
        wprintw(window, "System tables show: off");
    } else {
        screen->pg_stat_sys = true;
        wprintw(window, "System tables show: on");
    }
    *first_iter = true;

}

/*
 ***************************************************** log process routine **
 * Get current postgresql logfile path
 *
 * IN:
 * @path                Log file location.
 * @conn                Current postgresql connection.
 ****************************************************************************
 */
void get_logfile_path(char * path, PGconn * conn)
{
    PGresult *res;
    char q1[] = "show data_directory";
    char q2[] = "show log_directory";
    char q3[] = "show log_filename";
    char q4[] = "select to_char(pg_postmaster_start_time(), 'HH24MISS')";
    char *errmsg = (char *) malloc(sizeof(char) * 1024);
    char ld[64], lf[64], dd[64];
    char path_tpl[64 * 3], path_log[64 * 3], path_log_fallback[64 * 3] = "";

    strcpy(path, "\0");
    if ((res = do_query(conn, q2, errmsg)) == NULL) {
        PQclear(res);
        free(errmsg);
        return;
    }
    strcpy(ld, PQgetvalue(res, 0, 0));
    PQclear(res);

    if ( ld[0] != '/' ) {
        if ((res = do_query(conn, q1, errmsg)) == NULL) {
            PQclear(res);
            free(errmsg);
            return;
        }
        strcpy(dd, PQgetvalue(res, 0, 0));
        strcpy(path_tpl, dd);
        strcat(path_tpl, "/");
        strcat(path_tpl, ld);
        strcat(path_tpl, "/");
        PQclear(res);
    } else {
        strcpy(path_tpl, ld);
        strcat(path_tpl, "/");
    }

    if ((res = do_query(conn, q3, errmsg)) == NULL) {
        PQclear(res);
        free(errmsg);
        return;
    }
    strcpy(lf, PQgetvalue(res, 0, 0));
    strcat(path_tpl, lf);
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
        strcpy(path_log, path_tpl);
        strcpy(path_log_fallback, path_tpl);
        if((res = do_query(conn, q4, errmsg)) == NULL) {
            PQclear(res);
            free(errmsg);
            return;
        }
        strrpl(path_log, "%H%M%S", PQgetvalue(res, 0, 0));
        strrpl(path_log_fallback, "%H%M%S", "000000");
        PQclear(res);
    } else {
        strcpy(path_log, path_tpl);
    }

    free(errmsg);

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
        strcpy(path, "\0");
        return;
    }
}

/*
 *************************************************** iostat stuff function **
 * Count block devices in /proc/diskstat.
 *
 * RETURNS:
 * Return number of block devices.
 ****************************************************************************
 */
int count_block_devices(void)
{
    FILE * fp;
    int ndev = 0;
    char ch;

    if ((fp = fopen(DISKSTATS_FILE, "r")) == NULL) {
        return -1;
    }

    while (!feof(fp)) {
        ch = fgetc(fp);
        if (ch == '\n')
            ndev++;
    }

    fclose(fp);
    return ndev;
}

/*
 ****************************************************** key press function **
 * Log processing, open log in separate window or close if already opened.
 *
 * IN:
 * @window              Window where cmd status will be printed.
 * @w_sub               Pointer to window where log will be shown.
 * @screen              Array of connections options.
 * @conn                Current postgresql connection.
 ****************************************************************************
 */
void subscreen_process(WINDOW * window, WINDOW ** w_sub, struct screen_s * screen, PGconn * conn, int subscreen)
{
    if (!screen->subscreen_enabled) {
        /* open subscreen */
        switch (subscreen) {
            case SUBSCREEN_LOGTAIL:
                if (check_pg_listen_addr(screen) 
                        || (PQstatus(conn) == CONNECTION_OK && PQhost(conn) == NULL)) {
                    *w_sub = newwin(0, 0, ((LINES * 2) / 3), 0);
                    wrefresh(window);
                    /* get logfile path  */
                    get_logfile_path(screen->log_path, conn);
    
                    if (strlen(screen->log_path) == 0) {
                        wprintw(window, "Do nothing. Log filename not determined or no access permissions.");
                        return;
                    }
                    if ((screen->log_fd = open(screen->log_path, O_RDONLY)) == -1 ) {
                        wprintw(window, "Do nothing. Failed to open %s", screen->log_path);
                        return;
                    }
                    screen->subscreen = SUBSCREEN_LOGTAIL;
                    screen->subscreen_enabled = true;
                    wprintw(window, "Open postgresql log: %s", screen->log_path);
                    return;
                } else {
                    wprintw(window, "Do nothing. Log file view not supported for remote hosts.");
                    return;
                }
                break;
            case SUBSCREEN_IOSTAT:
                if (access(DISKSTATS_FILE, R_OK) == -1) {
                    wprintw(window, "Do nothing. No access to %s.", DISKSTATS_FILE);
                    return;
                }
                wprintw(window, "Show iostat");
                *w_sub = newwin(0, 0, ((LINES * 2) / 3), 0);
                screen->subscreen = SUBSCREEN_IOSTAT;
                screen->subscreen_enabled = true;
                break;
            case SUBSCREEN_NONE:
                screen->subscreen = SUBSCREEN_NONE;
                screen->subscreen_enabled = false;
        }
    } else {
        /* close subscreen */
        wclear(*w_sub);
        wrefresh(*w_sub);
        if (screen->log_fd > 0)
            close(screen->log_fd);
        screen->subscreen = SUBSCREEN_NONE;
        screen->subscreen_enabled = false;
        return;
    }
}

/*
 ******************************************************** routine function **
 * Tail postgresql log. 
 *
 * IN:
 * @window          Window where log will be printed.
 * @w_cmd           Window where diag messages will be printed.
 * @screen          Current screen.
 * @conn            Current postgresql connection.
 ****************************************************************************
 */
void print_log(WINDOW * window, WINDOW * w_cmd, struct screen_s * screen, PGconn * conn)
{
    int x, y;                                                   /* window coordinates */
    int n_lines = 1, n_cols = 1;                                /* number of rows and columns for printing */
    struct stat stats;                                          /* file stat struct */
    off_t end_pos;                                              /* end of file position */
    off_t pos;                                                  /* from this position start read of file */
    size_t bytes_read;                                          /* bytes readen from file to buffer */
    char buffer[BUFSIZ] = "";                                   /* init empty buffer */
    int i, nl_count = 0, len, scan_pos;                         /* iterator, newline counter, buffer length, in-buffer scan position */
    char *nl_ptr;                                               /* in-buffer newline pointer */

    getbegyx(window, y, x);                                     /* get window coordinates */
    /* calculate number of rows for log tailing, 2 is the number of lines for screen header */
    n_lines = LINES - y - 2;                                    /* calculate number of rows for log tailing */
    n_cols = COLS - x - 1;                                      /* calculate number of chars in row for cutting multiline log entries */
    wclear(window);                                             /* clear log window */

    fstat(screen->log_fd, &stats);                                     /* handle error here ? */
    if (S_ISREG (stats.st_mode) && stats.st_size != 0) {            /* log should be regular file and not be empty */
        end_pos = lseek(screen->log_fd, 0, SEEK_END);                  /* get end of file position */   
        pos = end_pos;                                              /* set position to the end of file */
        bytes_read = BUFSIZ;                                        /* read with 8KB block */
        if (end_pos < BUFSIZ)                                       /* if end file pos less than buffer */
            pos = 0;                                                /* than set read position ti the begin of file */
        else                                                        /* if end file pos more than buffer */
            pos = pos - bytes_read;                                 /* than set read position into end of file minus buffer size */
        lseek(screen->log_fd, pos, SEEK_SET);                          /* set determined position in file */
        bytes_read = read(screen->log_fd, buffer, bytes_read);         /* read file to buffer */

        len = strlen(buffer);                                       /* determine buffer length */
        scan_pos = len;                                             /* set in-buffer scan position equal buffer length, */

        for (i = 0; i < len; i++)                                   /* get number of newlines in buffer */
            if (buffer[i] == '\n')
                nl_count++;
        if (n_lines > nl_count) {                                   /* if number of newlines less than required */
            wprintw(window, "%s", buffer);                          /* than print out buffer content */
            wrefresh(window);
            return;                                                 /* and finish work */
        }

        /* print header */
        wattron(window, A_BOLD);
        wprintw(window, "\ntail %s\n", screen->log_path);
        wattroff(window, A_BOLD);

        /*
         * at this place, we have log more than buffersize, we fill buffer 
         * and we need find \n position from which we start print log.
         */
        int n_lines_save = n_lines;                                 /* save number of lines need for tail. */
        do {
            nl_ptr = memrchr(buffer, '\n', scan_pos);               /* find \n from scan_pos */
            if (nl_ptr != NULL) {                                   /* if found */
                scan_pos = (nl_ptr - buffer);                       /* remember this place */
            } else {                                                /* if not found */
                break;                                              /* finish work */
            }
            n_lines--;                                              /* after each iteration decrement line counter */
        } while (n_lines != 0);                                     /* stop cycle when line counter equal zero - we found need amount of lines */

        /* now we should cut multiline log entries to screen length */
        char str[n_cols];                                           /* use var for one line */
        char tmp[BUFSIZ];                                           /* tmp var for line from buffer */
        do {                                                        /* scan buffer from begin */
            nl_ptr = strstr(buffer, "\n");                          /* find \n in buffer */
            if (nl_ptr != NULL) {                                   /* if found */
                if (nl_count > n_lines_save) {                      /* and if lines too much, skip them */
                    strcpy(buffer, nl_ptr + 1);                     /* decrease buffer, cut skipped line */
                    nl_count--;                                     /* decrease newline counter */
                    continue;                                       /* start next iteration */
                }                                                   /* at this place we have sufficient number of lines for tail */
                strncpy(tmp, buffer, nl_ptr - buffer);              /* copy log line into temp buffer */
                tmp[nl_ptr - buffer] = '\0';                                     
                if (strlen(tmp) > n_cols) {                         /* if line longer than screen size (multiline) than truncate line to screen size */
                    strncpy(str, buffer, n_cols);
                    str[n_cols] = '\0';
                } else {                                            /* if line have normal size, copy line as is */
                    strncpy(str, buffer, strlen(tmp));
                    str[strlen(tmp)] = '\0';
                }
                wprintw(window, "%s\n", str);                       /* print line to log screen */
                strcpy(buffer, nl_ptr + 1);                         /* decrease buffer, cut printed line */
            } else {
                break;                                              /* if \n not found, finish work */
            }
            n_lines++;                                              /* after each iteration, increase newline counter */
        } while (n_lines != n_lines_save);                          /* print lines until newline counter not equal saved newline counter */
    } else {
        wprintw(w_cmd, "Do nothing. Log not a regular file or empty.");         /* if file not regular or empty */
        subscreen_process(w_cmd, &window, screen, conn, SUBSCREEN_NONE);              /* close log file and log screen */
    }
    
    wrefresh(window);
}

/*
 ****************************************************** key press function **
 * Open log in $PAGER.
 *
 * IN:
 * @window          Window where errors will be displayed.
 * @screen          Screen options.
 * @conn            Current connection.
 *
 * RETURNS:
 * Open log file in $PAGER.
 ****************************************************************************
 */
void show_full_log(WINDOW * window, struct screen_s * screen, PGconn * conn)
{
    pid_t pid;

    if (check_pg_listen_addr(screen)
            || (PQstatus(conn) == CONNECTION_OK && PQhost(conn) == NULL)) {
        /* get logfile path  */
        get_logfile_path(screen->log_path, conn);
        if (strlen(screen->log_path) != 0) {
            /* escape from ncurses mode */
            refresh();
            endwin();
            pid = fork();                   /* start child */
            if (pid == 0) {
                char * pager = (char *) malloc(sizeof(char) * 128);
                if ((pager = getenv("PAGER")) == NULL)
                    pager = DEFAULT_PAGER;
                execlp(pager, pager, screen->log_path, NULL);
                free(pager);
                exit(EXIT_SUCCESS);
            } else if (pid < 0) {
                wprintw(window, "Can't open %s: fork failed.", screen->log_path);
                return;
            } else if (waitpid(pid, NULL, 0) != pid) {
                wprintw(window, "Unknown error: waitpid failed.");
                return;
            }
        } else {
            wprintw(window, "Do nothing. Log filename not determined (not SUPERUSER?) or no access permissions.");
        }
    } else {
        wprintw(window, "Do nothing. Log file view not supported for remote hosts.");
    }

    /* return to ncurses mode */
    refresh();
    return;
}

/*
 ****************************************************** key-press function **
 * Reset PostgreSQL stat counters
 *
 * IN:
 * @window              Window where result will be printed.
 * @conn                Current PostgreSQL connection.
 ****************************************************************************
 */
void pg_stat_reset(WINDOW * window, PGconn * conn, bool * reseted)
{
    char * errmsg = (char *) malloc(sizeof(char) * 1024);
    PGresult * res;

    if ((res = do_query(conn, PG_STAT_RESET_QUERY, errmsg)) != NULL) {
        wprintw(window, "Reset statistics");
        *reseted = true;
    } else
        wprintw(window, "Reset statistics failed: %s", errmsg);
    
    PQclear(res);
    free(errmsg);
}

/*
 ****************************************************** key-press function **
 * Get query text using pg_stat_statements.queryid (only for 9.4 and never).
 *
 * IN:
 * @window              Window where result will be printed.
 * @screen              Current screen settings.
 * @conn                Current PostgreSQL connection.
 ****************************************************************************
 */
void get_query_by_id(WINDOW * window, struct screen_s * screen, PGconn * conn)
{
    if (screen->current_context != pg_stat_statements_timing
            && screen->current_context != pg_stat_statements_general
            && screen->current_context != pg_stat_statements_io
            && screen->current_context != pg_stat_statements_temp) {
        wprintw(window, "Get query text not allowed here.");
        return;
    }
    
    PGresult * res;
    bool * with_esc = (bool *) malloc(sizeof(bool));
    char msg[] = "Enter queryid: ",
         query[BUFSIZ],
         pager[32] = "";
    char * queryid = (char *) malloc(sizeof(char) * 16);
    char * errmsg = (char *) malloc(sizeof(char) * 1024);
    FILE * fpout;

    cmd_readline(window, msg, 15, with_esc, queryid, 16, true);
    if (check_string(queryid) == -1) {
        wprintw(window, "Do nothing. Value not valid.");
        return;
    }

    if (strlen(queryid) != 0 && *with_esc == false) {
        /* do query and send result into less */
        strcpy(query, PG_GET_QUERYTEXT_BY_QUERYID_QUERY_P1);
        strcat(query, queryid);
        strcat(query, PG_GET_QUERYTEXT_BY_QUERYID_QUERY_P2);
        if ((res = do_query(conn, query, errmsg)) == NULL) {
            wprintw(window, "%s", errmsg);
            free(errmsg);
            free(with_esc);
            return;
        }

        /* finish work if empty answer */
        if (PQntuples(res) == 0) {
            wprintw(window, "Do nothing. Empty answer for %s", queryid);
            free(with_esc);
            PQclear(res);
            return;
        }

        if (getenv("PAGER") != NULL)
            strcpy(pager, getenv("PAGER"));
        else
            strcpy(pager, DEFAULT_PAGER);
        
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
    } else if (strlen(queryid) == 0 && *with_esc == false) {
        wprintw(window, "Nothing to do. Nothing entered");
    } else if (*with_esc == true) {
        ;
    } else {
        wprintw(window, "Nothing to do.");
    }
    
    free(with_esc);
    free(errmsg);
}

/*
 *********************************************************** init function **
 * Init output colors.
 *
 * IN:
 * @ws_color            Sysstat window current color.
 * @wc_color            Cmdline window current color.
 * @wa_color            Database answer window current color.
 * @wl_color            Subscreen window current color.
 ****************************************************************************
 */
void init_colors(int * ws_color, int * wc_color, int * wa_color, int * wl_color)
{
    start_color();
    init_pair(0, COLOR_BLACK,   COLOR_BLACK);
    init_pair(1, COLOR_RED,     COLOR_BLACK);
    init_pair(2, COLOR_GREEN,   COLOR_BLACK);
    init_pair(3, COLOR_YELLOW,  COLOR_BLACK);
    init_pair(4, COLOR_BLUE,    COLOR_BLACK);
    init_pair(5, COLOR_MAGENTA, COLOR_BLACK);
    init_pair(6, COLOR_CYAN,    COLOR_BLACK);
    init_pair(7, COLOR_WHITE,   COLOR_BLACK);
    /* set defaults */
    *ws_color = 7;
    *wc_color = 7;
    *wa_color = 7;
    *wl_color = 7;
}

/*
 ************************************************** color-related function **
 * Draw help of color-change screen.
 *
 * IN:
 * @ws_color            Sysstat window current color.
 * @wc_color            Cmdline window current color.
 * @wa_color            Database answer window current color.
 * @wl_color            Subscreen window current color.
 * @target              Short name of the area which color will be changed.
 * @target_color        Next color of the area.
 ****************************************************************************
 */
void draw_color_help(WINDOW * w, int * ws_color, int * wc_color, int * wa_color, int * wl_color, int target, int * target_color)
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
\tS = Summary Data, M = Messages/Prompt, P = PostgreSQL Information, L = Additional screen\n", target);
    wprintw(w, "2) Select a color as a number, current color is  %i :\n\
\t0 = black,  1 = red,      2 = green,  3 = yellow,\n\
\t4 = blue,   5 = magenta,  6 = cyan,   7 = white\n", *target_color);
    wprintw(w, "3) Then use keys: 'Esc' to abort changes, 'Enter' to commit and end.\n");

    touchwin(w);
    wrefresh(w);
}

/*
 ****************************************************** key press function **
 * Change output colors.
 *
 * IN:
 * @ws_color            Sysstat window current color.
 * @wc_color            Cmdline window current color.
 * @wa_color            Database answer window current color.
 * @wl_color            Subscreen window current color.
 ****************************************************************************
 */
void change_colors(int * ws_color, int * wc_color, int * wa_color, int * wl_color)
{
    WINDOW * w;
    int ch,
        target = 'S',
        * target_color = ws_color;
    int ws_save = *ws_color,
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
            case '4': case '5': case '6': case '7':
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

/*
 ****************************************************** key-press function **
 * Switch statistics context.
 *
 * IN:
 * @window              Window for printing diag messages.
 * @screen              Current screen.
 * @context             New statistics context.
 * @res                 Array with previous query results.
 * @first_iter          Flag for resetting previous query results.
 ****************************************************************************
 */
void switch_context(WINDOW * window, struct screen_s * screen, 
                    enum context context, PGresult * res, bool * first_iter)
{
    wclear(window);
    switch (context) {
        case pg_stat_database:
            wprintw(window, "Show database wide statistics");
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
            wprintw(window, "Show table IO statistics");
            break;
        case pg_tables_size:
            wprintw(window, "Show tables sizes");
            break;
        case pg_stat_activity_long:
            wprintw(window, "Show activity (age threshold: %s)", screen->pg_stat_activity_min_age);
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
        default:
            break;
    }

    screen->current_context = context;
    if (res && *first_iter == false)
        PQclear(res);
    *first_iter = true;
}

/*
 ****************************************************** key press function **
 * Print on-program help.
 *
 * IN:
 * @first_iter              Reset stats counter after help.
 ****************************************************************************
 */
void print_help_screen(bool * first_iter)
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
  s,t,T           's' sizes, 't' tables, 'T' tables IO,\n\
  x,X,c,v         'x' stmt timings, 'X' stmt general, 'c' stmt IO, 'v' stmt temp.\n\
  Left,Right,/    'Left,Right' change column sort, '/' change sort desc/asc.\n\
  C,E,R           config: 'C' show config, 'E' edit configs, 'R' reload config.\n\
  p                       'p' start psql session.\n\
  l               'l' open log file with pager.\n\
  N,Ctrl+D,W      'N' add new connection, Ctrl+D close current connection, 'W' write connections info.\n\
  1..8            switch between consoles.\n\
subscreen actions:\n\
  B,L             'B' iostat, 'L' logtail.\n\
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
  F1              show help screen.\n\
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
 * Main program
 ****************************************************************************
 */
int main(int argc, char *argv[])
{
    struct args_s *args;                                /* struct for input args */
    struct screen_s *screens[MAX_SCREEN];               /* array of screens */
    struct stats_cpu_struct *st_cpu[2];                 /* cpu usage struct */
    struct stats_mem_short_struct *st_mem_short;        /* mem usage struct */

    WINDOW *w_sys, *w_cmd, *w_dba, *w_sub;              /* ncurses windows  */
    int ch;                                             /* store key press  */
    bool *first_iter = (bool *) malloc(sizeof(bool));   /* first-run flag   */
    *first_iter = true;
    static int console_no = 1;                          /* console number   */
    static int console_index = 0;                       /* console index in screen array   */

    PGconn      *conns[8];                              /* connections array    */
    PGresult    *p_res = NULL,
                *c_res = NULL;                          /* query results        */
    char query[4096];                                   /* query text           */
    int n_rows, n_cols, n_prev_rows = 0;                /* query results opts   */
    char *errmsg = (char *) malloc(sizeof(char) * 1024);/* query err message    */

    long int interval = DEFAULT_INTERVAL,               /* sleep interval       */
             sleep_usec = 0;                            /* time spent in sleep  */

    char ***p_arr = NULL,
         ***c_arr = NULL,
         ***r_arr = NULL;                  /* 3d arrays for query results  */

    int * ws_color = (int *) malloc(sizeof(int)),
        * wc_color = (int *) malloc(sizeof(int)),
        * wa_color = (int *) malloc(sizeof(int)),
        * wl_color = (int *) malloc(sizeof(int));
    args = (struct args_s *) malloc(sizeof(struct args_s));

    /* init iostat stuff */
    int ndev = count_block_devices();
    struct dstats *c_ios[ndev];
    struct dstats *p_ios[ndev];
    struct ext_dstats *x_ios[ndev];
    bool *repaint = (bool *) malloc(sizeof(bool));      /* repaint iostat if number of devices changed */
    *repaint = false;

    /* init various stuff */
    init_signal_handlers();
    init_args_struct(args);
    init_screens(screens);
    init_stats(st_cpu, &st_mem_short);
    init_iostats(c_ios, p_ios, x_ios, ndev);
    get_HZ();

    /* process cmd args */
    if (argc > 1) {
        arg_parse(argc, argv, args);
        if (strlen(args->connfile) != 0 && args->count == 1) {
            if (create_pgcenterrc_conn(args, screens, 0) == PGCENTERRC_READ_ERR) {
                create_initial_conn(args, screens);
            }
        } else {
            create_initial_conn(args, screens);
            create_pgcenterrc_conn(args, screens, 1);
        }
    } else {
        if (create_pgcenterrc_conn(args, screens, 0) == PGCENTERRC_READ_ERR)
            create_initial_conn(args, screens);
    }

    /* open connections to postgres */
    prepare_conninfo(screens);
    open_connections(screens, conns);

    /* init screens */
    initscr();
    cbreak();
    noecho();
    nodelay(stdscr, TRUE);
    keypad(stdscr,TRUE);
    ESCDELAY = 100;                 /* milliseconds to wait after escape */

    w_sys = newwin(5, 0, 0, 0);
    w_cmd = newwin(1, 0, 4, 0);
    w_dba = newwin(0, 0, 5, 0);

    init_colors(ws_color, wc_color, wa_color, wl_color);
    curs_set(0);

    /* main loop */
    while (1) {
        /* colors on */
        wattron(w_sys, COLOR_PAIR(*ws_color));
        wattron(w_dba, COLOR_PAIR(*wa_color));
        wattron(w_cmd, COLOR_PAIR(*wc_color));
        wattron(w_sub, COLOR_PAIR(*wl_color));

        /* trap keys */
        if (key_is_pressed()) {
            curs_set(1);
            wattron(w_cmd, COLOR_PAIR(*wc_color));
            ch = getch();
            switch (ch) {
                case '1': case '2': case '3': case '4': case '5': case '6': case '7': case '8':
                    console_index = switch_conn(w_cmd, screens, ch, console_index, console_no, p_res, first_iter);
                    console_no = console_index + 1;
                    break;
                case 'N':               /* open new screen with new connection */
                    console_index = add_connection(w_cmd, screens, conns, console_index);
                    console_no = console_index + 1;
                    *first_iter = true;
                    break;
                case 4:                 /* close current screen with Ctrl + D */
                    console_index = close_connection(w_cmd, screens, conns, console_index, first_iter);
                    console_no = console_index + 1;
                    break;
                case 'W':               /* write connections info into .pgcenterrc */
                    write_pgcenterrc(w_cmd, screens, conns, args);
                    break;
                case 'C':               /* open current postgresql config in pager */
                    show_config(w_cmd, conns[console_index]);
                    break;
                case 'E':               /* edit configuration files */
                    edit_config_menu(w_cmd, w_dba, screens[console_index], conns[console_index], first_iter);
                    break;
                case 'R':               /* reload postgresql */
                    reload_conf(w_cmd, conns[console_index]);
                    break;
                case 'L':               /* logtail subscreen on/off */
                    if (screens[console_index]->subscreen != SUBSCREEN_LOGTAIL)
                        subscreen_process(w_cmd, &w_sub, screens[console_index], conns[console_index], SUBSCREEN_NONE);
                    subscreen_process(w_cmd, &w_sub, screens[console_index], conns[console_index], SUBSCREEN_LOGTAIL);
                    break;
                case 'B':               /* iostat subscreen on/off */
                    if (screens[console_index]->subscreen != SUBSCREEN_IOSTAT)
                        subscreen_process(w_cmd, &w_sub, screens[console_index], conns[console_index], SUBSCREEN_NONE);
                    subscreen_process(w_cmd, &w_sub, screens[console_index], conns[console_index], SUBSCREEN_IOSTAT);
                    break;
                case 410:               /* when subscreen enabled and window has resized, repaint subscreen */
                    if (screens[console_index]->subscreen != SUBSCREEN_NONE) {
                        /* save current subscreen, for restore it later */
                        int save = screens[console_index]->subscreen;
                        subscreen_process(w_cmd, &w_sub, screens[console_index], conns[console_index], SUBSCREEN_NONE);
                        subscreen_process(w_cmd, &w_sub, screens[console_index], conns[console_index], save);
                    }
                    break;
                case 'l':               /* open postgresql log in pager */
                    show_full_log(w_cmd, screens[console_index], conns[console_index]);
                    break;
                case '-':               /* do cancel postgres backend */
                    signal_single_backend(w_cmd, screens[console_index], conns[console_index], false);
                    break;
                case '_':               /* do terminate postgres backend */
                    signal_single_backend(w_cmd, screens[console_index], conns[console_index], true);
                    break;
                case '.':               /* get current cancel/terminate mask */
                    get_statemask(w_cmd, screens[console_index]);
                    break;
                case '>':               /* set new cancel/terminate mask */
                    set_statemask(w_cmd, screens[console_index]);
                    break;
                case 330:               /* do cancel of backend group using mask with Del */
                    signal_group_backend(w_cmd, screens[console_index], conns[console_index], false);
                    break;
                case 383:               /* do terminate of backends group using mask with Shift+Del */
                    signal_group_backend(w_cmd, screens[console_index], conns[console_index], true);
                    break;
                case 260:               /* shift sort order with left arrow */
                    change_sort_order(screens[console_index], false, first_iter);
                    break;
                case 261:               /* shift sort order with right arrow */
                    change_sort_order(screens[console_index], true, first_iter);
                    break;
                case 47:                /* switch order desc/asc */
                    change_sort_order_direction(screens[console_index], first_iter);
                    PQclear(p_res);
                    break;
                case 'p':               /* start psql session to current postgres */
                    start_psql(w_cmd, screens[console_index]);
                    break;
                case 'd':               /* open pg_stat_database screen */
                    switch_context(w_cmd, screens[console_index], pg_stat_database, p_res, first_iter);
                    break;
                case 'r':               /* open pg_stat_replication screen */
                    switch_context(w_cmd, screens[console_index], pg_stat_replication, p_res, first_iter);
                    break;
                case 't':               /* open pg_stat_tables screen */
                    switch_context(w_cmd, screens[console_index], pg_stat_tables, p_res, first_iter);
                    break;
                case 'i':               /* open pg_stat(io)_indexes screen */
                    switch_context(w_cmd, screens[console_index], pg_stat_indexes, p_res, first_iter);
                    break;
                case 'T':               /* open pg_statio_tables screen */
                    switch_context(w_cmd, screens[console_index], pg_statio_tables, p_res, first_iter);
                    break;
                case 's':               /* open database object sizes screen */
                    switch_context(w_cmd, screens[console_index], pg_tables_size, p_res, first_iter);
                    break;
                case 'a':               /* show pg_stat_activity screen */
                    switch_context(w_cmd, screens[console_index], pg_stat_activity_long, p_res, first_iter);
                    break;
                case 'f':               /* open pg_stat_functions screen */
                    switch_context(w_cmd, screens[console_index], pg_stat_functions, p_res, first_iter);
                    break;
                case 'x':               /* open pg_stat_statements_timing screen */
                    switch_context(w_cmd, screens[console_index], pg_stat_statements_timing, p_res, first_iter);
                    break;
                case 'X':               /* open pg_stat_statements_general screen */
                    switch_context(w_cmd, screens[console_index], pg_stat_statements_general, p_res, first_iter);
                    break;
                case 'c':               /* open pg_stat_statements_io screen */
                    switch_context(w_cmd, screens[console_index], pg_stat_statements_io, p_res, first_iter);
                    break;
                case 'v':               /* open pg_stat_statements_temp screen */
                    switch_context(w_cmd, screens[console_index], pg_stat_statements_temp, p_res, first_iter);
                    break;
                case 'A':               /* change duration threshold in pg_stat_activity wcreen */
                    change_min_age(w_cmd, screens[console_index], p_res, first_iter);
                    break;
                case ',':               /* show system view on/off toggle */
                    system_view_toggle(w_cmd, screens[console_index], first_iter);
                    PQclear(p_res);
                    break;
                case 'Q':               /* reset pg stat counters */
                    pg_stat_reset(w_cmd, conns[console_index], first_iter);
                    PQclear(p_res);
                    break;
                case 'G':               /* get query text using pg_stat_statements.queryid */
                    get_query_by_id(w_cmd, screens[console_index], conns[console_index]);
                    break;
                case 'z':               /* change refresh interval */
                    interval = change_refresh(w_cmd, interval);
                    break;
                case 'Z':               /* change screens colors */
                    change_colors(ws_color, wc_color, wa_color, wl_color);
                    break;
                case 32:                /* pause program execution with space */
                    do_noop(w_cmd, interval);
                    break;
                case 265: case 'h':     /* print help with F1 */
                    print_help_screen(first_iter);
                    break;
                case 'q':               /* exit program */
                    exit_prog(screens, conns);
                    break;
                default:                /* show default msg on wrong input */
                    wprintw(w_cmd, "Unknown command - try 'h' for help.");
                    flushinp();
                    break;
            }
            wattroff(w_cmd, COLOR_PAIR(*wc_color));
            curs_set(0);
        } else {
            reconnect_if_failed(w_cmd, conns[console_index], screens[console_index], first_iter);

            /* 
             * Sysstat screen.
             */
            wclear(w_sys);
            print_title(w_sys, argv[0]);
            print_loadavg(w_sys);
            print_cpu_usage(w_sys, st_cpu);
            print_mem_usage(w_sys, st_mem_short);
            print_conninfo(w_sys, conns[console_index], console_no);
            print_pg_general(w_sys, screens[console_index], conns[console_index]);
            print_postgres_activity(w_sys, conns[console_index]);
            print_autovac_info(w_sys, conns[console_index]);
            print_pgstatstmt_info(w_sys, conns[console_index], interval);
            wrefresh(w_sys);

            /* 
             * Database screen. 
             */
            prepare_query(screens[console_index], query);
            if ((c_res = do_query(conns[console_index], query, errmsg)) == NULL) {
                /* if error occured print SQL error message into cmd */
                PQclear(c_res);
                c_res = NULL;
                p_res = NULL;
                *first_iter = true;
                wclear(w_dba);
                wprintw(w_dba, "%s", errmsg);
                wrefresh(w_dba);
                sleep(1);
                continue;
            }
            n_rows = PQntuples(c_res);
            n_cols = PQnfields(c_res);

            /* 
             * on startup or when context switched current data snapshot copied 
             * to previous data snapshot and restart cycle
             */
            if (*first_iter) {
                p_res = PQcopyResult(c_res, PG_COPYRES_ATTRS | PG_COPYRES_TUPLES);
                PQclear(c_res);
                usleep(10000);
                *first_iter = false;
                continue;
            }

            /* 
             * when number of rows changed (when db/table/index created or 
             * droped), update previous snapshot to current state and start new 
             * iteration. 
             */
            if (n_prev_rows < n_rows) {
                PQclear(p_res);
                p_res = PQcopyResult(c_res, PG_COPYRES_ATTRS | PG_COPYRES_TUPLES);
                PQclear(c_res);
                n_prev_rows = n_rows;
                usleep(10000);
                continue;
            }

            /* create storages for values from PQgetvalue */
            p_arr = init_array(p_arr, n_rows, n_cols);
            c_arr = init_array(c_arr, n_rows, n_cols);
            r_arr = init_array(r_arr, n_rows, n_cols);

            /* copy whole query results (current, previous) into arrays */
            pgrescpy(p_arr, p_res, n_rows, n_cols);
            pgrescpy(c_arr, c_res, n_rows, n_cols);

            /* diff current and previous arrays and build result array */
            diff_arrays(p_arr, c_arr, r_arr, screens[console_index], n_rows, n_cols, interval);

            /* sort result array using order key */
            sort_array(r_arr, n_rows, n_cols, screens[console_index]);

            /* print sorted result array */
            print_data(w_dba, c_res, r_arr, n_rows, n_cols, screens[console_index]);

            /* replace previous database query result with current result */
            PQclear(p_res);
            p_res = PQcopyResult(c_res, PG_COPYRES_ATTRS | PG_COPYRES_TUPLES);
            n_prev_rows = n_rows;
            PQclear(c_res);

            /* free memory allocated for arrays */
            free_array(p_arr, n_rows, n_cols);
            free_array(c_arr, n_rows, n_cols);
            free_array(r_arr, n_rows, n_cols);

            wrefresh(w_cmd);
            wclear(w_cmd);
            
            /*
             * Additional subscreen.
             */
            switch (screens[console_index]->subscreen) {
                case SUBSCREEN_LOGTAIL:
                    print_log(w_sub, w_cmd, screens[console_index], conns[console_index]);
                    break;
                case SUBSCREEN_IOSTAT:
                    print_iostat(w_sub, w_cmd, c_ios, p_ios, x_ios, ndev, repaint);
                    if (*repaint) {
                        free_iostats(c_ios, p_ios, x_ios, ndev);
                        ndev = count_block_devices();
                        init_iostats(c_ios, p_ios, x_ios, ndev);
                        subscreen_process(w_cmd, &w_sub, screens[console_index], conns[console_index], SUBSCREEN_NONE);
                        subscreen_process(w_cmd, &w_sub, screens[console_index], conns[console_index], SUBSCREEN_IOSTAT);
                        *repaint = false;
                    }
                    break;
                case SUBSCREEN_NONE: default:
                    break;
            }

            /* sleep loop */
            for (sleep_usec = 0; sleep_usec < interval; sleep_usec += INTERVAL_STEP) {
                if (key_is_pressed())
                    break;
                else {
                    usleep(INTERVAL_STEP);
                    if (interval > DEFAULT_INTERVAL && sleep_usec == DEFAULT_INTERVAL) {
                        wrefresh(w_cmd);
                        wclear(w_cmd);
                    }
                }
            }   
            /* end sleep loop */
        }
    }
    /* colors off */
    wattroff(w_sub, COLOR_PAIR(*wl_color));
    wattroff(w_sys, COLOR_PAIR(*ws_color));
    wattroff(w_dba, COLOR_PAIR(*wa_color));
    wattroff(w_cmd, COLOR_PAIR(*wc_color));
}
