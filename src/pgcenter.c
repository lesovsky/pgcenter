// This is an open source non-commercial project. Dear PVS-Studio, please check it.
// PVS-Studio Static Code Analyzer for C, C++ and C#: http://www.viva64.com

/*
 ****************************************************************************
 * pgcenter.c
 *      top-like admin tool for PostgreSQL.
 *
 * (C) 2016 by Alexey V. Lesovsky (lesovsky <at> gmail.com)
 * 
 ****************************************************************************
 */

#include "include/common.h"
#include "include/stats.h"
#include "include/pgf.h"
#include "include/hotkeys.h"
#include "include/pgcenter.h"

/*
 ****************************************************************************
 * Allocate memory for input arguments struct.
 ****************************************************************************
 */
struct args_s * init_args_mem(void) {
    struct args_s *args;
    if ((args = (struct args_s *) malloc(sizeof(struct args_s))) == NULL) {
        mreport(true, msg_fatal, "FATAL: malloc() for input args failed.\n");
    }
    return args;
}

/*
 ****************************************************************************
 * Initialize empty values for input arguments.
 ****************************************************************************
 */
void init_args_struct(struct args_s *args)
{
    args->count = 0;
    args->connfile[0] = '\0';
    args->host[0] = '\0';
    args->port[0] = '\0';
    args->user[0] = '\0';
    args->dbname[0] = '\0';
    args->need_passwd = false;             /* by default password not need */
}

/*
 ****************************************************************************
 * Allocate memory for tabs options struct array.
 ****************************************************************************
 */
void init_tabs(struct tab_s *tabs[])
{
    unsigned int i, j, k;       /* all are iterators */
    for (i = 0; i < MAX_TABS; i++) {
        if ((tabs[i] = (struct tab_s *) malloc(TAB_SIZE)) == NULL) {
            mreport(true, msg_fatal, "FATAL: malloc() for tabs failed.\n");
        }
        memset(tabs[i], 0, TAB_SIZE);        
        tabs[i]->tab = i;
        tabs[i]->conn_used = false;
        tabs[i]->host[0] = '\0';
        tabs[i]->port[0] = '\0';
        tabs[i]->user[0] = '\0';
        tabs[i]->dbname[0] = '\0';
        tabs[i]->password[0] = '\0';
        tabs[i]->conninfo[0] = '\0';
        tabs[i]->subtab_enabled = false;
        tabs[i]->subtab = SUBTAB_NONE;
        tabs[i]->log_path[0] = '\0';
        tabs[i]->current_context = DEFAULT_QUERY_CONTEXT;
        snprintf(tabs[i]->pg_stat_activity_min_age, XS_BUF_LEN, "%s", PG_STAT_ACTIVITY_MIN_AGE_DEFAULT);
        tabs[i]->signal_options = 0;
        tabs[i]->pg_stat_sys = false;
        
        if ((tabs[i]->curr_iostat = (struct iodata_s **) malloc(STATS_IODATA_SIZE)) == NULL ||
            (tabs[i]->prev_iostat = (struct iodata_s **) malloc(STATS_IODATA_SIZE)) == NULL) {
            mreport(true, msg_fatal, "FATAL: malloc() for tabs (iostat) failed.\n");
        }

        for (j = 0; j < TOTAL_CONTEXTS; j++) {
            switch (j) {
                case 0:
                    tabs[i]->context_list[j].context = pg_stat_database;
                    break;
                case 1:
                    tabs[i]->context_list[j].context = pg_stat_replication;
                    break;
                case 2:
                    tabs[i]->context_list[j].context = pg_stat_tables;
                    break;
                case 3:
                    tabs[i]->context_list[j].context = pg_stat_indexes;
                    break;
                case 4:
                    tabs[i]->context_list[j].context = pg_statio_tables;
                    break;
                case 5:
                    tabs[i]->context_list[j].context = pg_tables_size;
                    break;
                case 6:
                    tabs[i]->context_list[j].context = pg_stat_activity_long;
                    break;
                case 7:
                    tabs[i]->context_list[j].context = pg_stat_functions;
                    break;
                case 8:
                    tabs[i]->context_list[j].context = pg_stat_statements_timing;
                    break;
                case 9:
                    tabs[i]->context_list[j].context = pg_stat_statements_general;
                    break;
                case 10:
                    tabs[i]->context_list[j].context = pg_stat_statements_io;
                    break;
                case 11:
                    tabs[i]->context_list[j].context = pg_stat_statements_temp;
                    break;
                case 12:
                    tabs[i]->context_list[j].context = pg_stat_statements_local;
                    break;
                case 13:
                    tabs[i]->context_list[j].context = pg_stat_progress_vacuum;
                    break;
            }
            /* initiate sorting */
            tabs[i]->context_list[j].order_key = 0;
            tabs[i]->context_list[j].order_desc = true;
            /* create empty array for filtration patterns */
            for (k = 0; k < MAX_COLS; k++)
                tabs[i]->context_list[j].fstrings[k][0] = '\0';
        }
    }
}

/*
 ****************************************************************************
 * Allocate memory for array of 3D pointers.
 * This array is used for storing stats returned by postgres.
 ****************************************************************************
 */
char *** init_array(char ***arr, unsigned int n_rows, unsigned int n_cols)
{
    unsigned int i, j;

    if ((arr = malloc(sizeof(char **) * n_rows)) == NULL) {
        mreport(true, msg_fatal, "FATAL: malloc for stats array failed.\n");
    }
    for (i = 0; i < n_rows; i++) {
        if ((arr[i] = malloc(sizeof(char *) * n_cols)) == NULL) {
            mreport(true, msg_fatal, "FATAL: malloc for rows stats failed.\n");
        }
        for (j = 0; j < n_cols; j++)
            /* allocate a big room only for values in the last column */
            if (j != n_cols - 1) {
              if ((arr[i][j] = malloc(sizeof(char) * S_BUF_LEN)) == NULL) {
                  mreport(true, msg_fatal, "FATAL: malloc for cols stats failed.\n");
              }
            } else {
              if ((arr[i][j] = malloc(sizeof(char) * XL_BUF_LEN)) == NULL) {
                  mreport(true, msg_fatal, "FATAL: malloc for cols stats failed.\n");
              }
            }
    }
    return arr;
}

/*
 ****************************************************************************
 * Free space occupied by 3D pointer's array.
 ****************************************************************************
 */
char *** free_array(char ***arr, unsigned int n_rows, unsigned int n_cols)
{
    unsigned int i, j;

    for (i = 0; i < n_rows; i++) {
        for (j = 0; j < n_cols; j++)
            free(arr[i][j]);
        free(arr[i]);
    }
    free(arr);
    return arr;
}

/*
 ****************************************************************************
 * Init output colors.
 * Setup colors for sysstat, cmdline, main stat and aux stat windows.
 ****************************************************************************
 */
void init_colors(unsigned long long int * ws_color, unsigned long long int * wc_color,
        unsigned long long int * wa_color, unsigned long long int * wl_color)
{
    use_default_colors();
    start_color();
    init_pair(1, COLOR_BLACK,   -1);
    init_pair(2, COLOR_RED,     -1);
    init_pair(3, COLOR_GREEN,   -1);
    init_pair(4, COLOR_YELLOW,  -1);
    init_pair(5, COLOR_BLUE,    -1);
    init_pair(6, COLOR_MAGENTA, -1);
    init_pair(7, COLOR_CYAN,    -1);
    init_pair(8, COLOR_WHITE,   -1);
    /* set white as default */
    *ws_color = 0;
    *wc_color = 0;
    *wa_color = 0;
    *wl_color = 0;
}

/*
 ****************************************************************************
 * pgncenter --help
 ****************************************************************************
 */
void print_usage(void)
{
    printf("%s is the admin tool for PostgreSQL.\n\n", PROGRAM_NAME);
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
    printf("Report bugs to %s.\n", PROGRAM_ISSUES_URL);

    exit(EXIT_SUCCESS);
}

/*
 ****************************************************************************
 * Check port number argument at program startup.
 ****************************************************************************
 */
void check_portnum(const char * portstr)
{
    unsigned int portnum = atoi(portstr);
    if ( portnum < 1 || portnum > 65535) {
        mreport(true, msg_fatal, "Invalid port number: %s. Check input options or conninfo file.\n", portstr);
    }
}

/*
 ****************************************************************************
 * Basic function for parsing arguments which passed at startup.
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
        if ((strcmp(argv[1], "-?") == 0) || (argc == 2 && (strcmp(argv[1], "--help") == 0))) {
            print_usage();
        }
        if (strcmp(argv[1], "--version") == 0 || strcmp(argv[1], "-V") == 0) {
            mreport(true, msg_notice, "%s %.1f.%d\n", PROGRAM_NAME, PROGRAM_VERSION, PROGRAM_RELEASE);
        }
    }
    
    while ( (param = getopt_long(argc, argv,
                short_options, long_options, &option_index)) != -1 ) {
        switch (param) {
            case 'h':
                snprintf(args->host, sizeof(args->host), "%s", optarg);
                args->count++;
                break;
            case 'f':
                snprintf(args->connfile, sizeof(args->connfile), "%s", optarg);
                args->count++;
                break;
            case 'p':
                snprintf(args->port, sizeof(args->port), "%s", optarg);
        		check_portnum(args->port);
                args->count++;
                break;
            case 'U':
                snprintf(args->user, sizeof(args->user), "%s", optarg);
                args->count++;
                break;
            case 'd':
                snprintf(args->dbname, sizeof(args->dbname), "%s", optarg);
                args->count++;
                break;
            case 'w':
                args->need_passwd = false;
                break;
            case 'W':
                args->need_passwd = true;
                break;
            case '?': default:
                mreport(true, msg_fatal, "Try \"%s --help\" for more information.\n", argv[0]);
                break;
        }
    }

    /* handle extra parameters if they're exist, first - dbname, second - user, others - ignore */
    while (argc - optind >= 1) {
        if ( (argc - optind > 1)
                && strlen(args->user) == 0
                && strlen(args->dbname) == 0 ) {
            snprintf(args->dbname, sizeof(args->dbname), "%s", argv[optind]);
            snprintf(args->user, sizeof(args->user), "%s", argv[optind + 1]);
            optind++;
            args->count++;
        }
        else if ( (argc - optind >= 1) && strlen(args->user) != 0 && strlen(args->dbname) == 0 ) {
            snprintf(args->dbname, sizeof(args->dbname), "%s", argv[optind]);
            args->count++;
        } else if ( (argc - optind >= 1) && strlen(args->user) == 0 && strlen(args->dbname) != 0 ) {
            snprintf(args->user, sizeof(args->user), "%s", argv[optind]);
            args->count++;
        } else if ( (argc - optind >= 1) && strlen(args->user) == 0 && strlen(args->dbname) == 0 ) {
            snprintf(args->dbname, sizeof(args->dbname), "%s", argv[optind]);
            args->count++;
        } else
            mreport(false, msg_warning,
                    "WARNING: extra command-line argument \"%s\" ignored\n",
                    argv[optind]);
        optind++;
    }
}

/*
 ****************************************************************************
 * Make the libpq connection options from input args. Put the args always 
 * in the first tab.
 * If connections options aren't passed at startup try to use env vars.
 ****************************************************************************
 */
void create_initial_conn(struct args_s * args, struct tab_s * tabs[])
{
    const struct passwd *pw = getpwuid(getuid());

    /* get connection options from environment variables */
    if (getenv("PGHOST") != NULL)
        snprintf(tabs[0]->host, sizeof(tabs[0]->host), "%s", getenv("PGHOST"));
    if (getenv("PGPORT") != NULL)
        snprintf(tabs[0]->port, sizeof(tabs[0]->port), "%s", getenv("PGPORT"));
    if (getenv("PGUSER") != NULL)
        snprintf(tabs[0]->user, sizeof(tabs[0]->user), "%s", getenv("PGUSER"));
    if (getenv("PGDATABASE") != NULL)
        snprintf(tabs[0]->dbname, sizeof(tabs[0]->dbname), "%s", getenv("PGDATABASE"));
    if (getenv("PGPASSWORD") != NULL)
        snprintf(tabs[0]->password, sizeof(tabs[0]->password), "%s", getenv("PGPASSWORD"));
    
    /* if host specified via arg, use the given host */
    if ( strlen(args->host) != 0 )
        snprintf(tabs[0]->host, sizeof(tabs[0]->host), "%s", args->host);

    /* if port specified via arg, use the given port */
    if ( strlen(args->port) != 0 )
        snprintf(tabs[0]->port, sizeof(tabs[0]->port), "%s", args->port);

    /* when PGUSER env isn't set and the user arg isn't specified, set the user as current logged username */
    if ( strlen(args->user) == 0 && strlen(tabs[0]->user) == 0 )
        snprintf(tabs[0]->user, sizeof(tabs[0]->user), "%s", pw->pw_name);

    /* but if the user specified via arg, use the given name */
    if ( strlen(args->user) > 0 )
        snprintf(tabs[0]->user, sizeof(tabs[0]->user), "%s", args->user);

    /* if the dbname specified via arg, use the given dbname */
    if ( strlen(args->dbname) > 0 )
        snprintf(tabs[0]->dbname, sizeof(tabs[0]->dbname), "%s", args->dbname);

    /* dbname and username env aren't set and args are empty too, use logged username as dbname */
    if ( strlen(args->dbname) == 0 && strlen(args->user) == 0 && strlen(tabs[0]->dbname) == 0)
        snprintf(tabs[0]->dbname, sizeof(tabs[0]->dbname), "%s", pw->pw_name);
    /* dbname arg isn't specified, but user arg is specified, use username arg as dbname */
    else if ( strlen(args->dbname) == 0 && strlen(args->user) != 0 && strlen(tabs[0]->dbname) == 0 )
        snprintf(tabs[0]->dbname, sizeof(tabs[0]->dbname), "%s", args->user);
    /* dbname arg is set, and user isn't set. use the logged username as an username */
    else if ( strlen(args->dbname) != 0 && strlen(args->user) == 0 && strlen(tabs[0]->user) == 0 ) {
        snprintf(tabs[0]->dbname, sizeof(tabs[0]->dbname), "%s", args->dbname);
        snprintf(tabs[0]->user, sizeof(tabs[0]->user), "%s", pw->pw_name);
    /* default: use dbname specified with arg */
    } else if (strlen(tabs[0]->dbname) == 0)
        snprintf(tabs[0]->dbname, sizeof(tabs[0]->dbname), "%s", args->dbname);

    /* password required, but isn't set with env, ask pass */
    if ( args->need_passwd && strlen(tabs[0]->password) == 0 )
        snprintf(tabs[0]->password, sizeof(tabs[0]->password), "%s",
		          password_prompt("Password: ", sizeof(tabs[0]->password), false));

    /* a user is set and a dbname is still empty, use the username as the dbname */
    if ( strlen(tabs[0]->user) != 0 && strlen(tabs[0]->dbname) == 0 )
        snprintf(tabs[0]->dbname, sizeof(tabs[0]->dbname), "%s", tabs[0]->user);

    tabs[0]->conn_used = true;
}

/*
 ****************************************************************************
 * Read file with connection settings and setup other tabs.
 ****************************************************************************
 */
unsigned int create_pgcenterrc_conn(struct args_s * args, struct tab_s * tabs[], unsigned int pos)
{
    FILE *fp;
    static char pgcenterrc_path[PATH_MAX];
    struct stat statbuf;
    char strbuf[XXXL_BUF_LEN];
    unsigned int i = pos;
    const struct passwd *pw = getpwuid(getuid());

    if (strlen(args->connfile) == 0) {
	snprintf(pgcenterrc_path, sizeof(pgcenterrc_path), "%s/%s", pw->pw_dir, PGCENTERRC_FILE);
    } else {
        snprintf(pgcenterrc_path, sizeof(pgcenterrc_path), "%s", args->connfile);
    }

    if (access(pgcenterrc_path, F_OK) == -1 && strlen(args->connfile) != 0) {
        mreport(false, msg_error, "ERROR: no access to %s.\n", pgcenterrc_path);
        return PGCENTERRC_READ_ERR;
    }

    stat(pgcenterrc_path, &statbuf);
    if ( statbuf.st_mode & (S_IRWXG | S_IRWXO) && access(pgcenterrc_path, F_OK) != -1) {
        mreport(false, msg_error, "ERROR: %s has wrong permissions.\n", pgcenterrc_path);
        return PGCENTERRC_READ_ERR;
    }

    /* read connections settings from .pgcenterrc */
    if ((fp = fopen(pgcenterrc_path, "r")) != NULL) {
        while ((fgets(strbuf, XXXL_BUF_LEN, fp) != 0) && (i < MAX_TABS)) {
            sscanf(strbuf, "%[^:]:%[^:]:%[^:]:%[^:]:%[^:\n]",
                        tabs[i]->host,	tabs[i]->port,
                        tabs[i]->dbname,	tabs[i]->user,
                        tabs[i]->password);
                        tabs[i]->tab = i;
                        tabs[i]->conn_used = true;
            check_portnum(tabs[i]->port);
            /* if "null" read from file, than we should connecting through unix socket */
            if (!strcmp(tabs[i]->host, "(null)")) {
                tabs[i]->host[0] = '\0';
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
 ****************************************************************************
 * Make a full connection string for PQconnectdb() using connections options.
 ****************************************************************************
 */
void prepare_conninfo(struct tab_s * tabs[])
{
    unsigned int i;
    for ( i = 0; i < MAX_TABS; i++ ) {
        if (tabs[i]->conn_used) {
            if (strlen(tabs[i]->host) != 0) {
		snprintf(tabs[i]->conninfo + strlen(tabs[i]->conninfo),
			 sizeof(tabs[i]->conninfo) - strlen(tabs[i]->conninfo),
			 "host=%s", tabs[i]->host);
            }
            if (strlen(tabs[i]->port) != 0) {
		snprintf(tabs[i]->conninfo + strlen(tabs[i]->conninfo),
			 sizeof(tabs[i]->conninfo) - strlen(tabs[i]->conninfo),
			 " port=%s", tabs[i]->port);
            }
	    snprintf(tabs[i]->conninfo + strlen(tabs[i]->conninfo),
		     sizeof(tabs[i]->conninfo) - strlen(tabs[i]->conninfo),
		     " user=%s", tabs[i]->user);
	    snprintf(tabs[i]->conninfo + strlen(tabs[i]->conninfo),
		     sizeof(tabs[i]->conninfo) - strlen(tabs[i]->conninfo),
		     " dbname=%s", tabs[i]->dbname);
            if ((strlen(tabs[i]->password)) != 0) {
	    	snprintf(tabs[i]->conninfo + strlen(tabs[i]->conninfo),
			 sizeof(tabs[i]->conninfo) - strlen(tabs[i]->conninfo),
			 " password=%s", tabs[i]->password);
            }
        }
    }
}

/*
 ****************************************************************************
 * String comparison function for qsort (descending order).
 * The 'arg' here is the number of column in the array.
 ****************************************************************************
 */
int str_cmp_desc(const void * a, const void * b, void * arg)
{
    int *key = (int *) arg;
    const char *pa = ((const char ***) a)[0][*key];
    const char *pb = ((const char ***) b)[0][*key];

    return -strcmp(pa, pb);
}

/*
 ****************************************************************************
 * String comparison function for qsort (ascending order).
 * The 'arg' here is the number of column in the array.
 ****************************************************************************
 */
int str_cmp_asc(const void * a, const void * b, void * arg)
{
    int *key = (int *) arg;
    const char *pa = ((const char ***) a)[0][*key];
    const char *pb = ((const char ***) b)[0][*key];

    return strcmp(pa, pb);
}

/*
 ****************************************************************************
 * Integer comparison function for qsort (descending order).
 * The 'arg' here is the number of column in the array.
 ****************************************************************************
 */
int int_cmp_desc(const void * a, const void * b, void * arg)
{
    int *key = (int *) arg;
    const char *ia = ((const char ***) a)[0][*key];
    const char *ib = ((const char ***) b)[0][*key];

    return atoll(ib) - atoll(ia);
}

/*
 ****************************************************************************
 * Integer comparison function for qsort (ascending order).
 * The 'arg' here is the number of column in the array.
 ****************************************************************************
 */
int int_cmp_asc(const void * a, const void * b, void * arg)
{
    int *key = (int *) arg;
    const char *ia = ((const char ***) a)[0][*key];
    const char *ib = ((const char ***) b)[0][*key];

    return atoll(ia) - atoll(ib);
}

/*
 ****************************************************************************
 * Float comparison function for qsort (descending order).
 * The 'arg' here is the number of column in the array.
 ****************************************************************************
 */
int fl_cmp_desc(const void * a, const void * b, void * arg)
{
    int *key = (int *) arg;
    const char *fa = ((const char ***) a)[0][*key];
    const char *fb = ((const char ***) b)[0][*key];

    return atof(fb) > atof(fa);
}

/*
 ****************************************************************************
 * Float comparison function for qsort (descending order).
 * The 'arg' here is the number of column in the array.
 ****************************************************************************
 */
int fl_cmp_asc(const void * a, const void * b, void * arg)
{
    int *key = (int *) arg;
    const char *fa = ((const char ***) a)[0][*key];
    const char *fb = ((const char ***) b)[0][*key];

    return atof(fb) < atof(fa);
}

/*
 ****************************************************************************
 * Compare two arrays and build third array with deltas.
 * Take the 'p_res' with previous values and 'c_res' with current values,
 * compare them (x = current - previous) and put results into the third 
 * 'res_arr' array.
 * Comparing is based on range of allowed columns for comparison which has 
 * MIN and MAX values, so we don't compare values that are not in the range
 * (e.g. string values like a tables, indexes, queries, ...).
 ****************************************************************************
 */
void diff_arrays(char ***p_arr, char ***c_arr, char ***res_arr, struct tab_s * tab,
		unsigned int n_rows, unsigned int n_cols, unsigned long interval)
{
    unsigned int i, j, min = 0, max = 0;
    unsigned int divisor;
 
    switch (tab->current_context) {
        case pg_stat_database:
            min = PG_STAT_DATABASE_DIFF_MIN;
            (atoi(tab->pg_special.pg_version_num) < PG92)
                ? (max = PG_STAT_DATABASE_DIFF_MAX_91)
                : (max = PG_STAT_DATABASE_DIFF_MAX_LT);
            break;
        case pg_stat_replication:
            /* diff nothing, use returned values as-is */
            min = max = INVALID_ORDER_KEY;
            break;
        case pg_stat_tables:
            min = PG_STAT_TABLES_DIFF_MIN;
            max = PG_STAT_TABLES_DIFF_MAX;
            break;
        case pg_stat_indexes:
            min = PG_STAT_INDEXES_DIFF_MIN;
            max = PG_STAT_INDEXES_DIFF_MAX;
            break;
        case pg_statio_tables:
            min = PG_STATIO_TABLES_DIFF_MIN;
            max = PG_STATIO_TABLES_DIFF_MAX;
            break;
        case pg_tables_size:
            min = PG_TABLES_SIZE_DIFF_MIN;
            max = PG_TABLES_SIZE_DIFF_MAX;
            break;
        case pg_stat_activity_long:
            /* diff nothing, use returned values as-is */
            min = max = INVALID_ORDER_KEY;
            break;
        case pg_stat_functions:
            /* only one column for diff */
            min = max = PG_STAT_FUNCTIONS_DIFF_MIN;
            break;
        case pg_stat_statements_timing:
            if (atoi(tab->pg_special.pg_version_num) < PG92) {
                min = PGSS_TIMING_DIFF_MIN_91;
                max = PGSS_TIMING_DIFF_MAX_91;
            } else {
                min = PGSS_TIMING_DIFF_MIN_LT;
                max = PGSS_TIMING_DIFF_MAX_LT;
            }
            break;
        case pg_stat_statements_general:
            min = PGSS_GENERAL_DIFF_MIN_LT;
            max = PGSS_GENERAL_DIFF_MAX_LT;
            break;
        case pg_stat_statements_io:
            if (atoi(tab->pg_special.pg_version_num) < PG92) {
                min = PGSS_IO_DIFF_MIN_91;
                max = PGSS_IO_DIFF_MAX_91;
            } else {
                min = PGSS_IO_DIFF_MIN_LT;
                max = PGSS_IO_DIFF_MAX_LT;
            }
            break;
        case pg_stat_statements_temp:
            min = PGSS_TEMP_DIFF_MIN_LT;
            max = PGSS_TEMP_DIFF_MAX_LT;
            break;
        case pg_stat_statements_local:
            if (atoi(tab->pg_special.pg_version_num) < PG92) {
                min = PGSS_LOCAL_DIFF_MIN_91;
                max = PGSS_LOCAL_DIFF_MAX_91;
            } else {
                min = PGSS_LOCAL_DIFF_MIN_LT;
                max = PGSS_LOCAL_DIFF_MAX_LT;
            }
            break;
        case pg_stat_progress_vacuum:
            /* diff nothing, use returned values as-is */
            min = max = INVALID_ORDER_KEY;
            break;
        default:
            break;
    }

    divisor = interval / 1000000;
    for (i = 0; i < n_rows; i++) {
        for (j = 0; j < n_cols; j++)
            if (j < min || j > max)
                snprintf(res_arr[i][j], XXXL_BUF_LEN, "%s", c_arr[i][j]);     /* copy unsortable values as is */
            else {
                snprintf(res_arr[i][j], XXXL_BUF_LEN, "%lli", (atoll(c_arr[i][j]) - atoll(p_arr[i][j])) / divisor);
            }
    }
}

/*
 ****************************************************************************
 * Get the array and sort its content using the order key - number of column.
 * Order key isn't constant and can be changed by user.
 ****************************************************************************
 */
void sort_array(char ***res_arr, unsigned int n_rows, struct tab_s * tab)
{
    unsigned int i, order_key = 0;
    bool desc = false;

    for (i = 0; i < TOTAL_CONTEXTS; i++)
        if (tab->current_context == tab->context_list[i].context) {
            order_key = tab->context_list[i].order_key;
            desc = tab->context_list[i].order_desc;
        }

    /* don't sort arrays with invalid key */
    if (order_key == INVALID_ORDER_KEY)
        return;

    /* 
     * Comparator function depends on column data type. 
     * So check first element of an array, is it a number, float or string. 
     */
    if (check_string(&res_arr[0][order_key][0], is_number) == 0) {
        (desc)
            ? qsort_r(res_arr, n_rows, sizeof(char **), int_cmp_desc, &order_key)
            : qsort_r(res_arr, n_rows, sizeof(char **), int_cmp_asc, &order_key);
    } else if (check_string(&res_arr[0][order_key][0], is_float) == 0) {
        (desc)
            ? qsort_r(res_arr, n_rows, sizeof(char **), fl_cmp_desc, &order_key)
            : qsort_r(res_arr, n_rows, sizeof(char **), fl_cmp_asc, &order_key);
    } else {
        (desc)
            ? qsort_r(res_arr, n_rows, sizeof(char **), str_cmp_desc, &order_key)
            : qsort_r(res_arr, n_rows, sizeof(char **), str_cmp_asc, &order_key);
    }
}

/*
 ****************************************************************************
 * Get query results returned by postgres and put it into an array.
 ****************************************************************************
 */
void pgrescpy(char ***arr, PGresult *res, unsigned int n_rows, unsigned int n_cols)
{
    unsigned int i, j;

    for (i = 0; i < n_rows; i++)
        for (j = 0; j < n_cols; j++) {
            /* allocate a big room only for values in the last column */
            if (j != n_cols - 1)
              snprintf(arr[i][j], S_BUF_LEN, "%s", PQgetvalue(res, i, j));
            else
              snprintf(arr[i][j], XL_BUF_LEN, "%s", PQgetvalue(res, i, j));
        }
}

/*
 ****************************************************************************
 * Print title to the sysstat area: program name and current time.
 ****************************************************************************
 */
void print_title(WINDOW * window)
{
    static char strtime[20];
    get_time(strtime);
    wprintw(window, "%s: %s, ", PROGRAM_NAME, strtime);
}

/*
 ****************************************************************************
 * Get load average and print to the sysstat area.
 ****************************************************************************
 */
void print_loadavg(WINDOW * window, struct tab_s * tab, PGconn * conn)
{
    float * la;
    tab->conn_local ? (la = get_loadavg()) : (la = get_remote_loadavg(conn));
    wprintw(window, "load average: %.2f, %.2f, %.2f\n", la[0], la[1], la[2]);
}

/*
 ****************************************************************************
 * Composite function which read cpu stats and uptime then print out stats 
 * to the sysstat area.
 ****************************************************************************
 */
void print_cpu_usage(WINDOW * window, struct cpu_s *st_cpu[], struct tab_s * tab, PGconn * conn)
{
    static unsigned long long uptime[2]  = {0, 0};
    static unsigned long long uptime0[2] = {0, 0};
    static unsigned int curr = 1;
    static unsigned long long itv;

    uptime0[curr] = 0;
    if (tab->conn_local) {
        read_uptime(&(uptime0[curr]), tab);
        read_cpu_stat(st_cpu[curr], 2, &(uptime[curr]), &(uptime0[curr]));
    } else {
        read_remote_uptime(&(uptime0[curr]), tab, conn);
        read_remote_cpu_stat(st_cpu[curr], 2, &(uptime[curr]), &(uptime0[curr]), conn);
    }
    itv = get_interval(uptime[!curr], uptime[curr]);
    write_cpu_stat_raw(window, st_cpu, curr, itv);
    itv = get_interval(uptime0[!curr], uptime0[curr]);
    curr ^= 1;
}

/*
 ****************************************************************************
 * Get mem stats and print it to sysstat area.
 ****************************************************************************
 */
void print_mem_usage(WINDOW * window, struct mem_s *st_mem_short, struct tab_s * tab, PGconn * conn)
{
    if (tab->conn_local)
        read_mem_stat(st_mem_short);
    else 
        read_remote_mem_stat(st_mem_short, conn);
    
    write_mem_stat(window, st_mem_short);
}

/*
 ****************************************************************************
 * Get current connection status and print it to the pgstat area.
 ****************************************************************************
 */
void print_conninfo(WINDOW * window, PGconn *conn, unsigned int tab_no)
{    
    int st_index = get_conn_status(conn);
    write_conn_status(window, conn, tab_no, st_index);
}

/*
 ****************************************************************************
 * Get pg general info (version, uptime) and print it to the pgstat area.
 ****************************************************************************
 */
void print_pg_general(WINDOW * window, struct tab_s * tab, PGconn * conn)
{
    static char uptime[S_BUF_LEN];
    get_pg_uptime(conn, uptime);

    wprintw(window, " (ver: %s, up %s)", tab->pg_special.pg_version, uptime);
}

/*
 ****************************************************************************
 * Get current pg activity and print in to the pgstat area.
 * That info describes the number of total, idle, idle_xact, active, waiting
 * and others backends.
 ****************************************************************************
 */
void print_postgres_activity(WINDOW * window, struct tab_s * tab, PGconn * conn)
{
    get_summary_pg_activity(window, tab, conn);
}

/*
 ****************************************************************************
 * Get current (auto)vacuum activity and print it to the pgstat area.
 ****************************************************************************
 */
void print_vacuum_info(WINDOW * window, struct tab_s * tab, PGconn * conn)
{
    get_summary_vac_activity(window, tab, conn);
}

/*
 ****************************************************************************
 * Get info about queries and xacts from pg_stat_statements and 
 * pg_stat_activity and print it to the pgstat area.
 ****************************************************************************
 */
void print_pgss_info(WINDOW * window, PGconn * conn, unsigned long interval)
{
    get_pgss_summary(window, conn, interval);
}

/*
 ****************************************************************************
 * Print array content to the general stat area.
 * When previous and current stats snaphots compared and result array is 
 * sorted by order key, print array's content to the general stats area.
 ****************************************************************************
 */
void print_data(WINDOW *window, PGresult *res, char ***arr, unsigned int n_rows, unsigned int n_cols, struct tab_s * tab)
{
    unsigned int winsz_x, winsz_y, i, j, x;
    struct colAttrs *columns = init_colattrs(n_cols);
    struct context_s ctx;
    bool print = true, filter = false;

    calculate_width(columns, res, tab, arr, n_rows, n_cols);
    wclear(window);

    for (i = 0; i < TOTAL_CONTEXTS; i++)
        if (tab->current_context == tab->context_list[i].context)
            ctx = tab->context_list[i];

    /* enable filtration if there is an any filter pattern */
    for (i = 0; i < MAX_COLS; i++) {
        if (strlen(ctx.fstrings[i]) > 0) {
            filter = true;
            break;
        } else
            filter = false;
    }

    /* print header */
    wattron(window, A_BOLD);
    for (j = 0, x = 0; j < n_cols; j++, x++) {
        /* truncate last field length to end of tab */
        if (j == n_cols - 1) {
            getyx(window, winsz_y, winsz_x);
            columns[x].width = COLS - winsz_x - 1;
            /* dirty hack for supress gcc warning about "variable set but not used" */
            winsz_y--;
        } 
        /* mark sort column */
        if (j == ctx.order_key) {
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
        /* filtering cycle - searching filter pattern */
        if (filter)
            for (j = 0; j < n_cols; j++) {
                if (!strstr(arr[i][j], ctx.fstrings[j]) && strlen(ctx.fstrings[j]) > 0)
                    print = false;          /* pattern not found */
                else if (strlen(ctx.fstrings[j]) == 0)
                    continue;               /* skip empty pattern */
                else {
                    print = true;           /* pattern found */
                    break;
                }
            }
        /* printing cycle - don't print filtered rows */
        for (j = 0, x = 0; j < n_cols; j++, x++) {
            /* truncate last field length to end of tab */
            if (j == n_cols - 1) {
                getyx(window, winsz_y, winsz_x);
                columns[x].width = COLS - winsz_x;
                arr[i][j][columns[x].width] = '\0';
            }
            if (print)
                wprintw(window, "%-*s", columns[x].width, arr[i][j]);
        }
    }
    wrefresh(window);
    free(columns);
}

/*
 ****************************************************************************
 * Composite function which get disks usage stats then print out stats to the
 * aux-stats area.
 ****************************************************************************
 */
void print_iostat(WINDOW * window, WINDOW * w_cmd, struct tab_s * tab, PGconn * conn, bool * repaint)
{
    static int tab_save = -1;
    
    /* if number of devices is changed, we should realloc structs and repaint subtab */
    if (tab->sys_special.bdev != count_block_devices(tab, conn)) {
        wprintw(w_cmd, "The number of devices is changed. ");
        *repaint = true;
        return;
    }

    static unsigned long long uptime0[2] = {0, 0};
    static unsigned long long itv;
    static unsigned int curr = 1;

    /* reset uptime when tabs switched */
    if (tab->tab != tab_save) {
        uptime0[0] = 0, uptime0[1] = 0;
        tab_save = tab->tab;
    }

    uptime0[curr] = 0;
    read_uptime(&(uptime0[curr]), tab);
    if (tab->conn_local)
        read_local_diskstats(window, tab->curr_iostat, tab->sys_special.bdev, repaint);
    else
        read_remote_diskstats(window, tab->curr_iostat, tab->sys_special.bdev, conn, repaint);
    
    itv = get_interval(uptime0[!curr], uptime0[curr]);
    write_iostat(window, tab->curr_iostat, tab->prev_iostat, tab->sys_special.bdev, itv, tab->sys_special.sys_hz);

    /* save current stats snapshot */
    replace_iostat(tab->curr_iostat, tab->prev_iostat, tab->sys_special.bdev);
    curr ^= 1;
}

/*
 ****************************************************************************
 * Composite function which get network interfaces usage stats then print 
 * out stats to the aux-stats area.
 ****************************************************************************
 */
void print_nicstat(WINDOW * window, WINDOW * w_cmd, struct tab_s * tab, struct nicdata_s *c_nicd[],
        struct nicdata_s *p_nicd[], unsigned int idev, bool * repaint)
{
    /* if number of devices is changed, we should realloc structs and repaint subtab */
    if (idev != count_nic_devices()) {
        wprintw(w_cmd, "The number of devices is changed.");
        *repaint = true;
        return;
    }

    static unsigned long long uptime0[2] = {0, 0};
    static unsigned long long itv;
    static unsigned int curr = 1;
    unsigned int i = 0;
    static bool first = true;

    if (tab->conn_local) {
        uptime0[curr] = 0;
        read_uptime(&(uptime0[curr]), tab);
        read_proc_net_dev(window, c_nicd, repaint);
    }
    
    if (first) {
        for (i = 0; i < idev; i++)
            get_speed_duplex(c_nicd[i]);
        first = false;
    }

    itv = get_interval(uptime0[!curr], uptime0[curr]);
    write_nicstats(window, c_nicd, p_nicd, idev, itv, tab->sys_special.sys_hz);

    /* save current stats snapshot */
    replace_nicdata(c_nicd, p_nicd, idev);
    curr ^= 1;
}

/*
 ****************************************************************************
 * Graceful quit.
 ****************************************************************************
 */
void exit_prog(struct tab_s * tabs[], PGconn * conns[])
{
    endwin();
    close_connections(tabs, conns);
    exit(EXIT_SUCCESS);
}

/*
 ****************************************************************************
 * Main program
 ****************************************************************************
 */
int main(int argc, char *argv[])
{
    struct args_s *args = init_args_mem();              /* struct for input args */
    struct tab_s *tabs[MAX_TABS];               /* array of tabs */
    struct cpu_s *st_cpu[2];                            /* cpu usage struct */
    struct mem_s *st_mem_short;                         /* mem usage struct */

    WINDOW *w_sys, *w_cmd, *w_dba, *w_sub;              /* ncurses windows  */
    int ch;                                    		/* store key press  */
    bool first_iter = true;                             /* first-run flag   */
    static unsigned int tab_no = 1;                 /* tab number   */
    static unsigned int tab_index = 0;              /* tab index in tab array */

    PGconn      *conns[MAX_TABS];                     /* connections array    */
    PGresult    *p_res = NULL,
                *c_res = NULL;                          /* query results        */
    char query[QUERY_MAXLEN];                           /* query text           */
    unsigned int n_rows, n_cols, n_prev_rows = 0;       /* query results opts   */
    char errmsg[ERRSIZE];                               /* query error message  */

    unsigned long interval = DEFAULT_INTERVAL,          /* sleep interval       */
             sleep_usec = 0;                            /* time spent in sleep  */

    char ***p_arr = NULL,
         ***c_arr = NULL,
         ***r_arr = NULL;                               /* 3d arrays for query results  */

    unsigned int long long ws_color, wc_color, wa_color, wl_color;/* colors for text zones */

    /* init nicstat stuff */
    unsigned int idev = count_nic_devices();
    struct nicdata_s *c_nicdata[idev];
    struct nicdata_s *p_nicdata[idev];

    /* repaint iostat/nicstat if number of devices changed */
    bool repaint = false;

    /* init various stuff */
    init_signal_handlers();
    init_args_struct(args);
    init_tabs(tabs);
    init_stats(st_cpu, &st_mem_short);
    init_nicdata(c_nicdata, p_nicdata, idev);

    /* 
     * Handling connection settings. The main idea here is combine together: 
     * 1) connection settings specified directly at program startup;
     * 2) connection settings specified at connection file;
     * 3) use environment variables.
     * So,
     * 1) conn. specified directly at program startup is always opened in the tab 1;
     * 2) conn. settings from connfile are opened in the next tabs;
     * 3) use env. variables when there are neither direct settings nor connfile;
     * 4) env. vars don't used when conn. settings are specified or connfile exists.
     */
    
    if (argc > 1) {
        arg_parse(argc, argv, args);
        if (strlen(args->connfile) != 0 && args->count == 1) {
            if (create_pgcenterrc_conn(args, tabs, 0) == PGCENTERRC_READ_ERR) {
                create_initial_conn(args, tabs);
            }
        } else {
            create_initial_conn(args, tabs);
            create_pgcenterrc_conn(args, tabs, 1);
        }
    } else {
        if (create_pgcenterrc_conn(args, tabs, 0) == PGCENTERRC_READ_ERR)
            create_initial_conn(args, tabs);
    }

    /* open connections to postgres */
    prepare_conninfo(tabs);
    open_connections(tabs, conns);
    
    /* when conns are opened, init other system-specific stats */
    init_iostat(tabs, -1);

    /* init ncurses */
    initscr();
    cbreak();
    noecho();
    nodelay(stdscr, TRUE);
    keypad(stdscr,TRUE);
    set_escdelay(100);                 /* milliseconds to wait after escape */

    w_sys = newwin(5, 0, 0, 0);
    w_cmd = newwin(1, 0, 4, 0);
    w_dba = newwin(0, 0, 5, 0);
    w_sub = NULL;

    init_colors(&ws_color, &wc_color, &wa_color, &wl_color);
    curs_set(0);

    /* main loop */
    while (1) {
        /* colors on */
        wattron(w_sys, COLOR_PAIR(ws_color));
        wattron(w_dba, COLOR_PAIR(wa_color));
        wattron(w_cmd, COLOR_PAIR(wc_color));
        wattron(w_sub, COLOR_PAIR(wl_color));

        /* trap keys */
        if (key_is_pressed()) {
            curs_set(1);
            wattron(w_cmd, COLOR_PAIR(wc_color));
            ch = getch();
            switch (ch) {
                case '1': case '2': case '3': case '4': case '5': case '6': case '7': case '8':
                    tab_index = switch_tab(w_cmd, tabs, ch, tab_index, tab_no, p_res, &first_iter);
                    tab_no = tab_index + 1;
                    break;
                case 'N':               /* open new tab with new connection */
                    tab_index = add_tab(w_cmd, tabs, conns, tab_index);
                    tab_no = tab_index + 1;
                    first_iter = true;
                    break;
                case 4:                 /* close current tab with Ctrl + D */
                    tab_index = close_tab(w_cmd, tabs, conns, tab_index, &first_iter);
                    tab_no = tab_index + 1;
                    break;
                case 'W':               /* write connections info into .pgcenterrc */
                    write_pgcenterrc(w_cmd, tabs, conns, args);
                    break;
                case 'C':               /* open current postgresql config in pager */
                    show_config(w_cmd, conns[tab_index]);
                    break;
                case 'E':               /* edit configuration files */
                    edit_config_menu(w_cmd, w_dba, tabs[tab_index], conns[tab_index], &first_iter);
                    break;
                case 'R':               /* reload postgresql */
                    reload_conf(w_cmd, conns[tab_index]);
                    break;
                case 'L':               /* logtail subtab on/off */
                    if (tabs[tab_index]->subtab != SUBTAB_LOGTAIL)
                        subtab_process(w_cmd, &w_sub, tabs[tab_index], conns[tab_index], SUBTAB_NONE);
                    subtab_process(w_cmd, &w_sub, tabs[tab_index], conns[tab_index], SUBTAB_LOGTAIL);
                    break;
                case 'B':               /* iostat subtab on/off */
                    if (tabs[tab_index]->subtab != SUBTAB_IOSTAT)
                        subtab_process(w_cmd, &w_sub, tabs[tab_index], conns[tab_index], SUBTAB_NONE);
                    subtab_process(w_cmd, &w_sub, tabs[tab_index], conns[tab_index], SUBTAB_IOSTAT);
                    break;
                case 'I':               /* nicstat subtab on/off */
                    if (tabs[tab_index]->subtab != SUBTAB_NICSTAT)
                        subtab_process(w_cmd, &w_sub, tabs[tab_index], conns[tab_index], SUBTAB_NONE);
                    subtab_process(w_cmd, &w_sub, tabs[tab_index], conns[tab_index], SUBTAB_NICSTAT);
                    break;
                case 410:               /* when subtab enabled and window has resized, repaint subtab */
                    if (tabs[tab_index]->subtab != SUBTAB_NONE) {
                        /* save current subtab, for restore it later */
                        unsigned int save = tabs[tab_index]->subtab;
                        subtab_process(w_cmd, &w_sub, tabs[tab_index], conns[tab_index], SUBTAB_NONE);
                        subtab_process(w_cmd, &w_sub, tabs[tab_index], conns[tab_index], save);
                    }
                    break;
                case 'l':               /* open postgresql log in pager */
                    show_full_log(w_cmd, tabs[tab_index], conns[tab_index]);
                    break;
                case '-':               /* do cancel postgres backend */
                    signal_single_backend(w_cmd, tabs[tab_index], conns[tab_index], false);
                    break;
                case '_':               /* do terminate postgres backend */
                    signal_single_backend(w_cmd, tabs[tab_index], conns[tab_index], true);
                    break;
                case '.':               /* get current cancel/terminate mask */
                    get_statemask(w_cmd, tabs[tab_index]);
                    break;
                case '>':               /* set new cancel/terminate mask */
                    set_statemask(w_cmd, tabs[tab_index]);
                    break;
                case 330:               /* do cancel of backend group using mask with Del */
                    signal_group_backend(w_cmd, tabs[tab_index], conns[tab_index], false);
                    break;
                case 383:               /* do terminate of backends group using mask with Shift+Del */
                    signal_group_backend(w_cmd, tabs[tab_index], conns[tab_index], true);
                    break;
                case 260:               /* shift sort order with left arrow */
                    change_sort_order(tabs[tab_index], false, &first_iter);
                    break;
                case 261:               /* shift sort order with right arrow */
                    change_sort_order(tabs[tab_index], true, &first_iter);
                    break;
                case 47:                /* switch order desc/asc */
                    change_sort_order_direction(tabs[tab_index], &first_iter);
                    PQclear(p_res);
                    break;
                case 'p':               /* start psql session to current postgres */
                    start_psql(w_cmd, tabs[tab_index]);
                    break;
                case 'd':               /* open pg_stat_database tab */
                    switch_context(w_cmd, tabs[tab_index], pg_stat_database, p_res, &first_iter);
                    break;
                case 'r':               /* open pg_stat_replication tab */
                    switch_context(w_cmd, tabs[tab_index], pg_stat_replication, p_res, &first_iter);
                    break;
                case 't':               /* open pg_stat_tables tab */
                    switch_context(w_cmd, tabs[tab_index], pg_stat_tables, p_res, &first_iter);
                    break;
                case 'i':               /* open pg_stat(io)_indexes tab */
                    switch_context(w_cmd, tabs[tab_index], pg_stat_indexes, p_res, &first_iter);
                    break;
                case 'T':               /* open pg_statio_tables tab */
                    switch_context(w_cmd, tabs[tab_index], pg_statio_tables, p_res, &first_iter);
                    break;
                case 's':               /* open database object sizes tab */
                    switch_context(w_cmd, tabs[tab_index], pg_tables_size, p_res, &first_iter);
                    break;
                case 'a':               /* show pg_stat_activity tab */
                    switch_context(w_cmd, tabs[tab_index], pg_stat_activity_long, p_res, &first_iter);
                    break;
                case 'f':               /* open pg_stat_functions tab */
                    switch_context(w_cmd, tabs[tab_index], pg_stat_functions, p_res, &first_iter);
                    break;
                case 'x':               /* switch to next pg_stat_statements tab */
                    pgss_switch(w_cmd, tabs[tab_index], p_res, &first_iter);
                    break;
                case 'X':               /* open pg_stat_statements menu */
                    pgss_menu(w_cmd, w_dba, tabs[tab_index], &first_iter);
                    break;
                case 'v':               /* show pg_stat_activity tab */
                    switch_context(w_cmd, tabs[tab_index], pg_stat_progress_vacuum, p_res, &first_iter);
                    break;
                case 'A':               /* change duration threshold in pg_stat_activity wcreen */
                    change_min_age(w_cmd, tabs[tab_index], p_res, &first_iter);
                    break;
                case ',':               /* show system view on/off toggle */
                    system_view_toggle(w_cmd, tabs[tab_index], &first_iter);
                    PQclear(p_res);
                    break;
                case 'Q':               /* reset pg stat counters */
                    pg_stat_reset(w_cmd, conns[tab_index], &first_iter);
                    PQclear(p_res);
                    break;
                case 'G':               /* get query text using pg_stat_statements.queryid */
                    get_query_by_id(w_cmd, tabs[tab_index], conns[tab_index]);
                    break;
                case 'F':               /* set filtering for a column */
                    set_filter(w_cmd, tabs[tab_index], p_res, &first_iter);
                    break;
                case 'z':               /* change refresh interval */
                    interval = change_refresh(w_cmd, interval);
                    break;
                case 'Z':               /* change tabs colors */
                    change_colors(&ws_color, &wc_color, &wa_color, &wl_color);
                    break;
                case 32:                /* pause program execution with 'space' */
                    do_noop(w_cmd, interval);
                    break;
                case 265: case 'h':     /* print help with F1 or 'h' */
                    print_help_tab(&first_iter);
                    break;
                case 'q':               /* exit program */
                    exit_prog(tabs, conns);
                    break;
                default:                /* show default msg on wrong input */
                    wprintw(w_cmd, "Unknown command - try 'h' for help.");
                    flushinp();
                    break;
            }
            wattroff(w_cmd, COLOR_PAIR(wc_color));
            curs_set(0);
        } else {
            reconnect_if_failed(w_cmd, conns[tab_index], tabs[tab_index], &first_iter);

            /* 
             * Sysstat tab.
             */
            wclear(w_sys);
            print_title(w_sys);
            print_loadavg(w_sys, tabs[tab_index], conns[tab_index]);
            print_cpu_usage(w_sys, st_cpu, tabs[tab_index], conns[tab_index]);
            print_mem_usage(w_sys, st_mem_short, tabs[tab_index], conns[tab_index]);
            print_conninfo(w_sys, conns[tab_index], tab_no);
            print_pg_general(w_sys, tabs[tab_index], conns[tab_index]);
            print_postgres_activity(w_sys, tabs[tab_index], conns[tab_index]);
            print_vacuum_info(w_sys, tabs[tab_index], conns[tab_index]);
            print_pgss_info(w_sys, conns[tab_index], interval);
            wrefresh(w_sys);

            /* 
             * Database tab. 
             */
            prepare_query(tabs[tab_index], query);
            if ((c_res = do_query(conns[tab_index], query, errmsg)) == NULL) {
                /* if error occured print SQL error message into cmd */
                PQclear(c_res);
                c_res = NULL;
                p_res = NULL;
                first_iter = true;
                wclear(w_dba);
                wprintw(w_dba, "%s", errmsg);
                wrefresh(w_dba);
                sleep(1);
                continue;
            }
            n_rows = PQntuples(c_res);
            n_cols = PQnfields(c_res);

            /* 
             * on startup or when context is switched, copy current data snapshot 
             * to previous data snapshot and restart cycle
             */
            if (first_iter) {
                p_res = PQcopyResult(c_res, PG_COPYRES_ATTRS | PG_COPYRES_TUPLES);
                PQclear(c_res);
                usleep(10000);
                first_iter = false;
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
            diff_arrays(p_arr, c_arr, r_arr, tabs[tab_index], n_rows, n_cols, interval);

            /* sort result array using order key */
            sort_array(r_arr, n_rows, tabs[tab_index]);

            /* print sorted result array */
            print_data(w_dba, c_res, r_arr, n_rows, n_cols, tabs[tab_index]);

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
             * Additional subtab.
             */
            switch (tabs[tab_index]->subtab) {
                case SUBTAB_LOGTAIL:
                    print_log(w_sub, w_cmd, tabs[tab_index], conns[tab_index]);
                    break;
                case SUBTAB_IOSTAT:
                    print_iostat(w_sub, w_cmd, tabs[tab_index], conns[tab_index], &repaint);
                    if (repaint) {
                        free_iostat(tabs, tab_index);
                        tabs[tab_index]->sys_special.bdev = count_block_devices(tabs[tab_index], conns[tab_index]);
                        init_iostat(tabs, tab_index);
                        subtab_process(w_cmd, &w_sub, tabs[tab_index], conns[tab_index], SUBTAB_NONE);
                        subtab_process(w_cmd, &w_sub, tabs[tab_index], conns[tab_index], SUBTAB_IOSTAT);
                        repaint = false;
                    }
                    break;
                case SUBTAB_NICSTAT:
                    print_nicstat(w_sub, w_cmd, tabs[tab_index], c_nicdata, p_nicdata, idev, &repaint);
                    if (repaint) {
                        free_nicdata(c_nicdata, p_nicdata, idev);
                        idev = count_nic_devices();
                        init_nicdata(c_nicdata, p_nicdata, idev);
                        subtab_process(w_cmd, &w_sub, tabs[tab_index], conns[tab_index], SUBTAB_NONE);
                        subtab_process(w_cmd, &w_sub, tabs[tab_index], conns[tab_index], SUBTAB_NICSTAT);
                        repaint = false;
                    }
                    break;
                case SUBTAB_NONE: default:
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
    wattroff(w_sub, COLOR_PAIR(wl_color));
    wattroff(w_sys, COLOR_PAIR(ws_color));
    wattroff(w_dba, COLOR_PAIR(wa_color));
    wattroff(w_cmd, COLOR_PAIR(wc_color));
}
