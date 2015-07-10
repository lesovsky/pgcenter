/*
 * pgcenter: adminitrative console for PostgreSQL.
 * (C) 2015 by Alexey V. Lesovsky (lesovsky <at> gmail.com)
 */

#include <errno.h>
#include <getopt.h>
#include <limits.h>
#include <ncurses.h>
#include <pwd.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/stat.h>
#include <sys/types.h>
#include <termios.h>
#include <time.h>
#include <unistd.h>
#include "libpq-fe.h"
#include "pgcenter.h"

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
  -h, --host=HOSTNAME       database server host or socket directory (default: \"/tmp\")\n \
  -p, --port=PORT           database server port (default: \"5432\")\n \
  -U, --username=USERNAME   database user name (default: \"current user\")\n \
  -d, --dbname=DBNAME       database name (default: \"current user\")\n \
  -w, --no-password         never prompt for password\n \
  -W, --password            force password prompt (should happen automatically)\n\n");
    printf("Report bugs to %s.\n", PROGRAM_AUTHORS_CONTACTS);
}

/*
 ******************************************************** routine function **
 * Trap keys in program main mode
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
 *********************************************************** init function **
 * Allocate memory for connections options struct array
 *
 * OUT:
 * @conn_opts   Initialized array of connection options
 ****************************************************************************
 */
void init_conn_opts(struct conn_opts_struct *conn_opts[])
{
    int i;
    for (i = 0; i < MAX_CONSOLE; i++) {
        if ((conn_opts[i] = (struct conn_opts_struct *) malloc(CONN_OPTS_SIZE)) == NULL) {
                perror("malloc");
                exit(EXIT_FAILURE);
        }
        memset(conn_opts[i], 0, CONN_OPTS_SIZE);
        conn_opts[i]->terminal = i;
        conn_opts[i]->conn_used = false;
        strcpy(conn_opts[i]->host, "");
        strcpy(conn_opts[i]->port, "");
        strcpy(conn_opts[i]->user, "");
        strcpy(conn_opts[i]->dbname, "");
        strcpy(conn_opts[i]->password, "");
        strcpy(conn_opts[i]->conninfo, "");
        conn_opts[i]->log_opened = false;
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
 * Take input parameters and add them into connections options.
 *
 * IN:
 * @argc            Input arguments count.
 * @argv[]          Input arguments array.
 *
 * OUT:
 * @conn_opts[]     Array where connections options will be saved.
 ****************************************************************************
 */
void create_initial_conn(int argc, char *argv[],
                struct conn_opts_struct * conn_opts[])
{
    struct passwd *pw = getpwuid(getuid());

    /* short options */
    const char * short_options = "h:p:U:d:wW?";

    /* long options */
    const struct option long_options[] = {
        {"help", no_argument, NULL, '?'},
        {"host", required_argument, NULL, 'h'},
        {"port", required_argument, NULL, 'p'},
        {"dbname", required_argument, NULL, 'd'},
        {"no-password", no_argument, NULL, 'w'},
        {"password", no_argument, NULL, 'W'},
        {"user", required_argument, NULL, 'U'},
        {NULL, 0, NULL, 0}
    };

    int param, option_index;
    enum trivalue prompt_password = TRI_DEFAULT;

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
                strcpy(conn_opts[0]->host, optarg);
                break;
            case 'p':
                strcpy(conn_opts[0]->port, optarg);
                break;
            case 'U':
                strcpy(conn_opts[0]->user, optarg);
                break;
            case 'd':
                strcpy(conn_opts[0]->dbname, optarg);
                break;
            case 'w':
                prompt_password = TRI_NO;
                break;
            case 'W':
                prompt_password = TRI_YES;
                break;
            case '?': default:
                fprintf(stderr,"Try \"%s --help\" for more information.\n", argv[0]);
                exit(EXIT_SUCCESS);
                break;
        }
    }
    while (argc - optind >= 1) {
        if ( (argc - optind > 1)
                && strlen(conn_opts[0]->user) == 0
                && strlen(conn_opts[0]->dbname) == 0 )
            strcpy(conn_opts[0]->user, argv[optind]);
        else if ( (argc - optind >= 1) && strlen(conn_opts[0]->dbname) == 0 )
            strcpy(conn_opts[0]->dbname, argv[optind]);
        else
            fprintf(stderr,
                    "%s: warning: extra command-line argument \"%s\" ignored\n",
                    argv[0], argv[optind]);
        optind++;
    }
    if ( strlen(conn_opts[0]->host) == 0 )
        strcpy(conn_opts[0]->host, DEFAULT_HOST);

    if ( strlen(conn_opts[0]->port) == 0 )
        strcpy(conn_opts[0]->port, DEFAULT_PORT);

    if ( strlen(conn_opts[0]->user) == 0 )
        strcpy(conn_opts[0]->user, pw->pw_name);

    if ( prompt_password == TRI_YES )
        strcpy(conn_opts[0]->password, password_prompt("Password: ", 100, false));

    if ( strlen(conn_opts[0]->user) != 0 && strlen(conn_opts[0]->dbname) == 0 )
        strcpy(conn_opts[0]->dbname, conn_opts[0]->user);

    conn_opts[0]->conn_used = true;
}

/*
 ******************************************************** startup function **
 * Read ~/.pgcenterrc cfile and fill up conrections options array.
 *
 * IN:
 * @argc            Input arguments count.
 * @argv[]          Input arguments array.
 * @pos             Start position inside array.
 *
 * OUT:
 * @conn_opts       Connections options array.
 *
 * RETURNS:
 * Success or failure.
 ****************************************************************************
 */
int create_pgcenterrc_conn(int argc, char *argv[],
                struct conn_opts_struct * conn_opts[], const int pos)
{
    FILE *fp;
    static char pgcenterrc_path[PATH_MAX];
    struct stat statbuf;
    char strbuf[BUFFERSIZE];
    int i = pos;
    struct passwd *pw = getpwuid(getuid());

    strcpy(pgcenterrc_path, pw->pw_dir);
    strcat(pgcenterrc_path, "/");
    strcat(pgcenterrc_path, PGCENTERRC_FILE);

    if (access(pgcenterrc_path, F_OK) == -1)
        return PGCENTERRC_READ_ERR;

    stat(pgcenterrc_path, &statbuf);
    if ( statbuf.st_mode & (S_IRWXG | S_IRWXO) ) {
        fprintf(stderr,
                    "WARNING: %s has wrong permissions.\n", pgcenterrc_path);
        return PGCENTERRC_READ_ERR;
    }

    /* read connections settings from .pgcenterrc */
    if ((fp = fopen(pgcenterrc_path, "r")) != NULL) {
        while (fgets(strbuf, 4096, fp) != 0) {
            sscanf(strbuf, "%[^:]:%[^:]:%[^:]:%[^:]:%[^:\n]",
                        conn_opts[i]->host, conn_opts[i]->port,
                        conn_opts[i]->dbname,   conn_opts[i]->user,
                        conn_opts[i]->password);
                        conn_opts[i]->terminal = i;
                        conn_opts[i]->conn_used = true;
                        i++;
        }
        fclose(fp);
        return PGCENTERRC_READ_OK;
    } else {
        fprintf(stdout,
                    "WARNING: failed to open %s. Try use defaults.\n", pgcenterrc_path);
        return PGCENTERRC_READ_ERR;
    }
}

/*
 ******************************************************** startup function **
 * Prepare conninfo string for PQconnectdb.
 *
 * IN:
 * @conn_opts       Connections options array without filled conninfo.
 *
 * OUT:
 * @conn_opts       Connections options array with conninfo.
 ****************************************************************************
 */
void prepare_conninfo(struct conn_opts_struct * conn_opts[])
{
    int i;
    for ( i = 0; i < MAX_CONSOLE; i++ )
        if (conn_opts[i]->conn_used) {
            strcat(conn_opts[i]->conninfo, "host=");
            strcat(conn_opts[i]->conninfo, conn_opts[i]->host);
            strcat(conn_opts[i]->conninfo, " port=");
            strcat(conn_opts[i]->conninfo, conn_opts[i]->port);
            strcat(conn_opts[i]->conninfo, " user=");
            strcat(conn_opts[i]->conninfo, conn_opts[i]->user);
            strcat(conn_opts[i]->conninfo, " dbname=");
            strcat(conn_opts[i]->conninfo, conn_opts[i]->dbname);
            if ((strlen(conn_opts[i]->password)) != 0) {
                strcat(conn_opts[i]->conninfo, " password=");
                strcat(conn_opts[i]->conninfo, conn_opts[i]->password);
            }
        }
}

/*
 ******************************************************** startup function **
 * Open connections to pgbouncer using conninfo string from conn_opts.
 *
 * IN:
 * @conn_opts       Connections options array.
 *
 * OUT:
 * @conns           Array of connections.
 ****************************************************************************
 */
void open_connections(struct conn_opts_struct * conn_opts[], PGconn * conns[])
{
    int i;
    for ( i = 0; i < MAX_CONSOLE; i++ ) {
        if (conn_opts[i]->conn_used) {
            conns[i] = PQconnectdb(conn_opts[i]->conninfo);
            if ( PQstatus(conns[i]) == CONNECTION_BAD && PQconnectionNeedsPassword(conns[i]) == 1) {
                printf("%s:%s %s@%s require ", 
                                conn_opts[i]->host, conn_opts[i]->port,
                                conn_opts[i]->user, conn_opts[i]->dbname);
                strcpy(conn_opts[i]->password, password_prompt("password: ", 100, false));
                strcat(conn_opts[i]->conninfo, " password=");
                strcat(conn_opts[i]->conninfo, conn_opts[i]->password);
                conns[i] = PQconnectdb(conn_opts[i]->conninfo);
            } else if ( PQstatus(conns[i]) == CONNECTION_BAD ) {
                printf("Unable to connect to %s:%s %s@%s",
                conn_opts[i]->host, conn_opts[i]->port,
                conn_opts[i]->user, conn_opts[i]->dbname);
            }
        }
    }
}

/*
 ************************************************* summary window function **
 * Print current time.
 *
 * RETURNS:
 * Return current time.
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
 **************************************************** get cpu stat function **
 * Allocate memory for cpu statistics struct.
 *
 * OUT:
 * @st_cpu      Struct for cpu statistics.
 ****************************************************************************
 */
void init_stats(struct stats_cpu_struct *st_cpu[])
{
    int i;
    /* Allocate structures for CPUs "all" and 0 */
    for (i = 0; i < 2; i++) {
        if ((st_cpu[i] = (struct stats_cpu_struct *) 
            malloc(STATS_CPU_SIZE * 2)) == NULL) {
            perror("malloc");
            exit(EXIT_FAILURE);
        }
        memset(st_cpu[i], 0, STATS_CPU_SIZE * 2);
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
                            &st_cpu->cpu_iowait,    &st_cpu->cpu_steal,
                            &st_cpu->cpu_hardirq,   &st_cpu->cpu_softirq,
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
                                &sc.cpu_steal,      &sc.cpu_hardirq,
                                &sc.cpu_softirq,    &sc.cpu_guest,
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
            "      %%cpu: %4.1f us, %4.1f sy, %4.1f ni, %4.1f id, %4.1f wa, %4.1f hi, %4.1f si, %4.1f st\n",
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
 ************************************************* summary window function **
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
 ********************************************************* routine function **
 * Send query to pgbouncer.
 *
 * IN:
 * @conn            Pgbouncer connection.
 * @query_context   Type of query.
 *
 * RETURNS:
 * Answer from pgbouncer.
 ****************************************************************************
 */
PGresult * do_query(PGconn *conn, enum context query_context)
{
    PGresult    *res;
    char query[1024];
    switch (query_context) {
        case pg_stat_database: default:
            strcpy(query, PG_STAT_DATABASE_QUERY);
            break;
        case pg_stat_replication:
            strcpy(query, PG_STAT_REPLICATION_QUERY);
            break;
    }
    res = PQexec(conn, query);
    if ( PQresultStatus(res) != PG_TUP_OK ) {
        puts("We didn't get any data.");
        PQclear(res);
        return NULL;
    } else
        return res;
}

/*
 ******************************************************** routine function **
 * Calculate column width for output data.
 *
 * IN:
 * @row_count       Number of rows in query result.
 * @col_count       Number of columns in query result.
 * @res             Query result.
 *
 * OUT:
 * @columns         Struct with column names and their max width.
 ****************************************************************************
 */
struct colAttrs * calculate_width(struct colAttrs *columns, int row_count, int col_count, PGresult *res)
{
    int i, col, row;

    for ( col = 0, i = 0; col < col_count; col++, i++) {
        strcpy(columns[i].name, PQfname(res, col));
        int colname_len = strlen(PQfname(res, col));
        int width = colname_len;
        for (row = 0; row < row_count; row++ ) {
            int val_len = strlen(PQgetvalue(res, row, col));
            if ( val_len >= width )
                width = val_len;
        }
        columns[i].width = width + 3;
    }
    return columns;
}

/*
 ******************************************************** routine function **
 * Print answer from pgbouncer to the program main window.
 *
 * IN:
 * @window              Window which is used for print.
 * @query_context       Type of query.
 * @res                 Answer from pgbouncer.
 ****************************************************************************
 */
void print_data(WINDOW * window, enum context query_context, PGresult *res)
{
    int    row_count, col_count, row, col, i;
    row_count = PQntuples(res);
    col_count = PQnfields(res);
    struct colAttrs *columns = (struct colAttrs *) malloc(sizeof(struct colAttrs) * col_count);

    columns = calculate_width(columns, row_count, col_count, res);
    wclear(window);
    /* print column names */
    wattron(window, A_BOLD);
    for ( col = 0, i = 0; col < col_count; col++, i++ )
        wprintw(window, "%-*s", columns[i].width, PQfname(res,col));
    wprintw(window, "\n");
    wattroff(window, A_BOLD);
    /* print rows */
    for ( row = 0; row < row_count; row++ ) {
        for ( col = 0, i = 0; col < col_count; col++, i++ ) {
            wprintw(window,
                "%-*s", columns[i].width, PQgetvalue(res, row, col));
        }
        wprintw(window, "\n");
    }
    wprintw(window, "\n");
    wrefresh(window);

    PQclear(res);
    free(columns);
}

int main(int argc, char *argv[])
{
    struct conn_opts_struct *conn_opts[MAX_CONSOLE];
    struct stats_cpu_struct *st_cpu[2];
    WINDOW *w_sys, *w_cmd, *w_dba;
    int ch;                             /* key press */
//    static int console_no = 1;
    static int console_index = 0;

    PGconn *conns[8];
    PGresult    * res;

    enum context query_context = pg_stat_database;

    /* Process args... */
    init_conn_opts(conn_opts);
    if ( argc > 1 ) {
        create_initial_conn(argc, argv, conn_opts);
        create_pgcenterrc_conn(argc, argv, conn_opts, 1);
    } else
        if (create_pgcenterrc_conn(argc, argv, conn_opts, 0) == PGCENTERRC_READ_ERR)
            create_initial_conn(argc, argv, conn_opts);

    /* CPU stats related actions */
    init_stats(st_cpu);
    get_HZ();

    /* open connections to postgres */
    prepare_conninfo(conn_opts);
    open_connections(conn_opts, conns);

    /* init screens */
    initscr();
    cbreak();
    noecho();
    nodelay(stdscr, TRUE);

    w_sys = newwin(5, 0, 0, 0);
    w_cmd = newwin(1, 0, 4, 0);
    w_dba = newwin(0, 0, 5, 0);

    curs_set(0);

    /* main loop */
    while (1) {
        if (key_is_pressed()) {
            curs_set(1);
//            wattron(w_cmdline, COLOR_PAIR(*wc_color));
            ch = getch();
            switch (ch) {
                case '1':
                    break;
            }
            curs_set(0);
        } else {
            wclear(w_sys);
            print_title(w_sys, argv[0]);
            print_loadavg(w_sys);
            print_cpu_usage(w_sys, st_cpu);
            wrefresh(w_sys);

            res = do_query(conns[console_index], query_context);
            print_data(w_dba, query_context, res);
            wrefresh(w_cmd);
            wclear(w_cmd);

            /* refresh interval */
            sleep(1);
        }
    }
}
