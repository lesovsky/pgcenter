/*
 * pgcenter: adminitrative console for PostgreSQL.
 * (C) 2015 by Alexey V. Lesovsky (lesovsky <at> gmail.com)
 */

#include <errno.h>
#include <ncurses.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <time.h>
#include <unistd.h>
#include "pgcenter.h"

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
float get_loadavg(const int m)
{
    if ( m != 1 && m != 5 && m != 15 )
        fprintf(stderr, "invalid value for load average\n");

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

int main(int argc, char *argv[])
{
    struct stats_cpu_struct *st_cpu[2];
    WINDOW *w_sys, *w_cmd, *w_dba;
    int a = 0;

    /* CPU stats related actions */
    init_stats(st_cpu);
    get_HZ();

    /* init screens */
    initscr();
    cbreak();
    noecho();
    nodelay(stdscr, TRUE);

    w_sys = newwin(5, 0, 0, 0);
    w_cmd = newwin(1, 0, 4, 0);
    w_dba = newwin(0, 0, 5, 0);

    /* main loop */
    while (1) {
        wclear(w_sys);
        wclear(w_cmd);
        wclear(w_dba);
        print_title(w_sys, argv[0]);
        print_loadavg(w_sys);
        print_cpu_usage(w_sys, st_cpu);
        wprintw(w_cmd, "w_cmd: %i", a++);
        wprintw(w_dba, "w_dba: %i", a++);
        wrefresh(w_sys);
        wrefresh(w_cmd);
        wrefresh(w_dba);

        /* refresh interval */
        sleep(1);
    }
}
