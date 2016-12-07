// This is an open source non-commercial project. Dear PVS-Studio, please check it.
// PVS-Studio Static Code Analyzer for C, C++ and C#: http://www.viva64.com

/*
 ****************************************************************************
 * stats.c
 *      stats handling functions.
 *
 * (C) 2016 by Alexey V. Lesovsky (lesovsky <at> gmail.com)
 * 
 ****************************************************************************
 */
#include <linux/ethtool.h>
#include "include/stats.h"

/*
 ****************************************************************************
 * Allocate memory for cpu and mem statistics structs.
 ****************************************************************************
 */
void init_stats(struct cpu_s *st_cpu[], struct mem_s **st_mem_short)
{
    unsigned int i;
    /* Allocate structures for CPUs "all" and 0 */
    for (i = 0; i < 2; i++) {
        if ((st_cpu[i] = (struct cpu_s *) malloc(STATS_CPU_SIZE * 2)) == NULL) {
            mreport(true, msg_fatal, "FATAL: malloc for cpu stats failed.\n");
        }
        memset(st_cpu[i], 0, STATS_CPU_SIZE * 2);
    }

    /* Allocate structures for memory */
    if ((*st_mem_short = (struct mem_s *) malloc(STATS_MEM_SIZE)) == NULL) {
        mreport(true, msg_fatal, "FATAL: malloc for memory stats failed.\n");
    }
    memset(*st_mem_short, 0, STATS_MEM_SIZE);
}

/*
 ****************************************************************************
 * Allocate memory for IO statistics structs.
 ****************************************************************************
 */
void init_iostats(struct iodata_s *c_ios[], struct iodata_s *p_ios[], unsigned int bdev)
{
    unsigned int i;
    for (i = 0; i < bdev; i++) {
        if ((c_ios[i] = (struct iodata_s *) malloc(STATS_IODATA_SIZE)) == NULL ||
            (p_ios[i] = (struct iodata_s *) malloc(STATS_IODATA_SIZE)) == NULL) {
                mreport(true, msg_fatal, "FATAL: malloc for iostat failed.\n");
        }
    }
}

/*
 ****************************************************************************
 * Free memory consumed by IO statistics structs.
 ****************************************************************************
 */
void free_iostats(struct iodata_s *c_ios[], struct iodata_s *p_ios[], unsigned int bdev)
{
    unsigned int i;
    for (i = 0; i < bdev; i++) {
        free(c_ios[i]);
        free(p_ios[i]);
    }
}

/*
 ****************************************************************************
 * Allocate memory for NIC data structs.
 ****************************************************************************
 */
void init_nicdata(struct nicdata_s *c_nicdata[], struct nicdata_s *p_nicdata[], unsigned int idev)
{
    unsigned int i;
    for (i = 0; i < idev; i++) {
        if ((c_nicdata[i] = (struct nicdata_s *) malloc(STATS_NICDATA_SIZE)) == NULL ||
            (p_nicdata[i] = (struct nicdata_s *) malloc(STATS_NICDATA_SIZE)) == NULL ) {
                mreport(true, msg_fatal, "FATAL: malloc for nicstat failed.\n");
        }

	/* initialize interfaces with unknown speed and duplex */
	c_nicdata[i]->speed = -1;
	c_nicdata[i]->duplex = DUPLEX_UNKNOWN;
    }
}

/*
 ****************************************************************************
 * Free memory consumed by NIC data structs.
 ****************************************************************************
 */
void free_nicdata(struct nicdata_s *c_nicdata[], struct nicdata_s *p_nicdata[], unsigned int idev)
{
    unsigned int i;
    for (i = 0; i < idev; i++) {
        free(c_nicdata[i]);
        free(p_nicdata[i]);
    }
}

/*
 ****************************************************************************
 * Get system clock resolution.
 ****************************************************************************
 */
void get_HZ(void)
{
    unsigned long ticks;
    if ((ticks = sysconf(_SC_CLK_TCK)) == -1)
        mreport(false, msg_error, "ERROR: sysconf failure.\n");
            
    sys_hz = (unsigned int) ticks;
}

/*
 ****************************************************************************
 * Count block devices in /proc/diskstat.
 ****************************************************************************
 */
unsigned int count_block_devices(void)
{
    FILE * fp;
    unsigned int bdev = 0;
    char ch;

    /* At program start, if statfile read failed, then allocate array for 10 devices. */
    if ((fp = fopen(DISKSTATS_FILE, "r")) == NULL) {
        return 10;
    }

    while (!feof(fp)) {
        ch = fgetc(fp);
        if (ch == '\n')
            bdev++;
    }

    fclose(fp);
    return bdev;
}

/*
 ****************************************************************************
 * Count NIC devices in /proc/net/dev.
 ****************************************************************************
 */
unsigned int count_nic_devices(void)
{
    FILE * fp;
    unsigned int idev = 0;
    char ch;

    /* At program start, if statfile read failed, then allocate array for 10 devices. */
    if ((fp = fopen(NETDEV_FILE, "r")) == NULL) {
        return 10;
    }

    while (!feof(fp)) {
        ch = fgetc(fp);
        if (ch == '\n')
            idev++;
    }

    /* header has two lines */
    idev = idev - 2;

    fclose(fp);
    return idev;
}

/*
 ****************************************************************************
 * Read /proc/loadavg and return load average values.
 ****************************************************************************
 */
float * get_loadavg()
{
    static float la[3];
    FILE *fp;

    if ((fp = fopen(LOADAVG_FILE, "r")) != NULL) {
        if ((fscanf(fp, "%f %f %f", &la[0], &la[1], &la[2])) != 3)
            la[0] = la[1] = la[2] = 0;            /* something goes wrong */
        fclose(fp);
    } else {
        la[0] = la[1] = la[2] = 0;                /* can't read statfile */
    }

    return la;
}

/*
 ****************************************************************************
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
 ****************************************************************************
 * Read machine uptime independently of the number of processors.
 ****************************************************************************
 */
void read_uptime(unsigned long long *uptime)
{
    FILE *fp;
    unsigned long up_sec, up_cent;

    if ((fp = fopen(UPTIME_FILE, "r")) != NULL) {
        if ((fscanf(fp, "%lu.%lu", &up_sec, &up_cent)) != 2) {
	    fclose(fp);
	    return;
    	}
    } else
        return;

    *uptime = (unsigned long long) up_sec * HZ + (unsigned long long) up_cent * HZ / 100;
    fclose(fp);
}

/*
 ****************************************************************************
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
void read_cpu_stat(struct cpu_s *st_cpu, unsigned int nbr,
                            unsigned long long *uptime, unsigned long long *uptime0)
{
    FILE *fp;
    struct cpu_s *st_cpu_i;
    struct cpu_s sc;
    char line[XL_BUF_LEN];
    unsigned int proc_nb;

    if ((fp = fopen(STAT_FILE, "r")) == NULL) {
        /* zeroing stats if stats read failed */
        memset(st_cpu, 0, STATS_CPU_SIZE);
        return;
    }

    while ( (fgets(line, sizeof(line), fp)) != NULL ) {
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
                sscanf(line + 3, "%u %llu %llu %llu %llu %llu %llu %llu %llu %llu %llu",
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
    fclose(fp);
}

/*
 ****************************************************************************
 * Compute time interval.
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
 ****************************************************************************
 * Display cpu statistics in specified window.
 ****************************************************************************
 */
void write_cpu_stat_raw(WINDOW * window, struct cpu_s *st_cpu[],
                unsigned int curr, unsigned long long itv)
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
 ****************************************************************************
 * Read /proc/meminfo and save results into struct.
 ****************************************************************************
 */
void read_mem_stat(struct mem_s *st_mem_short) {
    FILE *mem_fp;
    char buffer[XL_BUF_LEN];
    char key[M_BUF_LEN];
    unsigned long long value;
    
    if ((mem_fp = fopen(MEMINFO_FILE, "r")) != NULL) {
        while (fgets(buffer, XL_BUF_LEN, mem_fp) != NULL) {
            sscanf(buffer, "%s %llu", key, &value);
            if (!strcmp(key,"MemTotal:"))
                st_mem_short->mem_total = value / 1024;
            else if (!strcmp(key,"MemFree:"))
                st_mem_short->mem_free = value / 1024;
            else if (!strcmp(key,"SwapTotal:"))
                st_mem_short->swap_total = value / 1024;
            else if (!strcmp(key,"SwapFree:"))
                st_mem_short->swap_free = value / 1024;
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
    } else {
        /* failed to read /proc/meminfo, zeroing stats */
        st_mem_short->mem_total = st_mem_short->mem_free = st_mem_short->mem_used = 0;
        st_mem_short->cached = st_mem_short->buffers = st_mem_short->slab = 0;
        st_mem_short->swap_total = st_mem_short->swap_free = st_mem_short->swap_used = 0;
        st_mem_short->dirty = st_mem_short->writeback = 0;
    }
}

/*
 ****************************************************************************
 * Print content of mem struct to the ncurses window
 ****************************************************************************
 */
void write_mem_stat(WINDOW * window, struct mem_s *st_mem_short) {
    wprintw(window, " MiB mem: %6llu total, %6llu free, %6llu used, %8llu buff/cached\n",
            st_mem_short->mem_total, st_mem_short->mem_free, st_mem_short->mem_used,
            st_mem_short->cached + st_mem_short->buffers + st_mem_short->slab);
    wprintw(window, "MiB swap: %6llu total, %6llu free, %6llu used, %6llu/%llu dirty/writeback\n",
            st_mem_short->swap_total, st_mem_short->swap_free, st_mem_short->swap_used,
            st_mem_short->dirty, st_mem_short->writeback);
}

/*
 ****************************************************************************
 * Save current io statistics snapshot.
 ****************************************************************************
 */
void replace_iodata(struct iodata_s *curr[], struct iodata_s *prev[], unsigned int bdev)
{
    unsigned int i;
    for (i = 0; i < bdev; i++) {
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
        prev[i]->arqsz = curr[i]->arqsz;
        prev[i]->await = curr[i]->await;
        prev[i]->util = curr[i]->util;
    }
}

/*
 ****************************************************************************
 * Get interface speed and duplex settings.
 ****************************************************************************
 */
void get_speed_duplex(struct nicdata_s * nicdata)
{
    struct ifreq ifr;
    struct ethtool_cmd edata;
    int status, sock;

    sock = socket(PF_INET, SOCK_DGRAM, IPPROTO_IP);
    if (sock < 0) {
        return;
    }

    snprintf(ifr.ifr_name, sizeof(ifr.ifr_name), "%s", nicdata->ifname);
    ifr.ifr_data = (void *) &edata;
    edata.cmd = ETHTOOL_GSET;
    status = ioctl(sock, SIOCETHTOOL, &ifr);
    close(sock);
    
    if (status < 0) {
        return;
    }

    nicdata->speed = edata.speed * 1000000;
    nicdata->duplex = edata.duplex;

}

/*
 ****************************************************************************
 * Save current nicstat snapshot.
 ****************************************************************************
 */
void replace_nicdata(struct nicdata_s *curr[], struct nicdata_s *prev[], unsigned int idev)
{
    unsigned int i;
    for (i = 0; i < idev; i++) {
        prev[i]->rbytes = curr[i]->rbytes;
        prev[i]->rpackets = curr[i]->rpackets;
        prev[i]->wbytes = curr[i]->wbytes;
        prev[i]->wpackets = curr[i]->wpackets;
        prev[i]->ierr = curr[i]->ierr;
        prev[i]->oerr = curr[i]->oerr;
        prev[i]->coll = curr[i]->coll;
        prev[i]->sat = curr[i]->sat;
    }
}

/*
 ****************************************************************************
 * Print current time.
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
 ****************************************************************************
 * Read /proc/diskstats and save stats.
 ****************************************************************************
 */
void read_diskstats(WINDOW * window, struct iodata_s *c_ios[], bool * repaint)
{
    FILE *fp;
    char line[M_BUF_LEN];

    unsigned int major, minor;
    char devname[S_BUF_LEN];
    unsigned long r_completed, r_merged, r_sectors, r_spent,
                  w_completed, w_merged, w_sectors, w_spent,
                  io_in_progress, t_spent, t_weighted;
    unsigned int i = 0;
    
    /*
     * If /proc/diskstats read failed, fire up repaint flag.
     * Next when subtab repainting fails, subtab will be closed.
     */
    if ((fp = fopen(DISKSTATS_FILE, "r")) == NULL) {
        wclear(window);
        wprintw(window, "Do nothing. Can't open %s", DISKSTATS_FILE);
        *repaint = true;
        return;
    }

    while (fgets(line, sizeof(line), fp) != NULL) {
        sscanf(line, "%u %u %s %lu %lu %lu %lu %lu %lu %lu %lu %lu %lu %lu",
                    &major, &minor, devname,
                    &r_completed, &r_merged, &r_sectors, &r_spent,
                    &w_completed, &w_merged, &w_sectors, &w_spent,
                    &io_in_progress, &t_spent, &t_weighted);
        c_ios[i]->major = major;
        c_ios[i]->minor = minor;
        snprintf(c_ios[i]->devname, S_BUF_LEN, "%s", devname);
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
}

/*
 ****************************************************************************
 * Calculate IO stats and print it out.
 ****************************************************************************
 */
void write_iostat(WINDOW * window, struct iodata_s *c_ios[], struct iodata_s *p_ios[], unsigned int bdev, unsigned long long itv)
{
    unsigned int i = 0;
    double r_await[bdev], w_await[bdev];
    
    for (i = 0; i < bdev; i++) {
        c_ios[i]->util = S_VALUE(p_ios[i]->t_spent, c_ios[i]->t_spent, itv);
        c_ios[i]->await = ((c_ios[i]->r_completed + c_ios[i]->w_completed) - (p_ios[i]->r_completed + p_ios[i]->w_completed)) ?
            ((c_ios[i]->r_spent - p_ios[i]->r_spent) + (c_ios[i]->w_spent - p_ios[i]->w_spent)) /
            ((double) ((c_ios[i]->r_completed + c_ios[i]->w_completed) - (p_ios[i]->r_completed + p_ios[i]->w_completed))) : 0.0;
        c_ios[i]->arqsz = ((c_ios[i]->r_completed + c_ios[i]->w_completed) - (p_ios[i]->r_completed + p_ios[i]->w_completed)) ?
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
    for (i = 0; i < bdev; i++) {
        /* skip devices without iops */
        if (c_ios[i]->r_completed == 0 && c_ios[i]->w_completed == 0) {
            continue;
        }
        wprintw(window, "%6s:\t\t", c_ios[i]->devname);
        wprintw(window, "%8.2f%8.2f",
                S_VALUE(p_ios[i]->r_merged, c_ios[i]->r_merged, itv),
                S_VALUE(p_ios[i]->w_merged, c_ios[i]->w_merged, itv));
        wprintw(window, "%9.2f%9.2f",
                S_VALUE(p_ios[i]->r_completed, c_ios[i]->r_completed, itv),
                S_VALUE(p_ios[i]->w_completed, c_ios[i]->w_completed, itv));
        wprintw(window, "%9.2f%9.2f%9.2f%9.2f",
                S_VALUE(p_ios[i]->r_sectors, c_ios[i]->r_sectors, itv) / 2048,
                S_VALUE(p_ios[i]->w_sectors, c_ios[i]->w_sectors, itv) / 2048,
                c_ios[i]->arqsz,
                S_VALUE(p_ios[i]->t_weighted, c_ios[i]->t_weighted, itv) / 1000.0);
        wprintw(window, "%10.2f%10.2f%10.2f", c_ios[i]->await, r_await[i], w_await[i]);
        wprintw(window, "%8.2f", c_ios[i]->util / 10.0);
        wprintw(window, "\n");
    }
    wrefresh(window);
}

/*
 ****************************************************************************
 * Read /proc/net/dev and save stats.
 ****************************************************************************
 */
void read_proc_net_dev(WINDOW * window, struct nicdata_s *c_nicd[], bool * repaint)
{
    FILE *fp;
    unsigned int i = 0, j = 0;
    char line[L_BUF_LEN];
    char ifname[IF_NAMESIZE + 1];
    unsigned long lu[16];
    
    /*
     * If read /proc/net/dev failed, fire up repaint flag.
     * Next when subtab repainting fails, subtab will be closed.
     */
    if ((fp = fopen(NETDEV_FILE, "r")) == NULL) {
        wclear(window);
        wprintw(window, "Do nothing. Can't open %s", NETDEV_FILE);
        *repaint = true;
        return;
    }
    
    while (fgets(line, sizeof(line), fp) != NULL) {
        if (j < 2) {
            j++;
            continue;       /* skip headers */
        }
        sscanf(line, "%s %lu %lu %lu %lu %lu %lu %lu %lu %lu %lu %lu %lu %lu %lu %lu %lu",
                ifname,
             /* rbps    rpps    rerrs   rdrop   rfifo   rframe  rcomp   rmcast */
                &lu[0], &lu[1], &lu[2], &lu[3], &lu[4], &lu[5], &lu[6], &lu[7],
             /* wbps    wpps    werrs    wdrop    wfifo    wcoll    wcarrier wcomp */
                &lu[8], &lu[9], &lu[10], &lu[11], &lu[12], &lu[13], &lu[14], &lu[15]);
        snprintf(c_nicd[i]->ifname, IF_NAMESIZE + 1, "%s", ifname);
        c_nicd[i]->rbytes = lu[0];
        c_nicd[i]->rpackets = lu[1];
        c_nicd[i]->wbytes = lu[8];
        c_nicd[i]->wpackets = lu[9];
        c_nicd[i]->ierr = lu[2];
        c_nicd[i]->oerr = lu[10];
        c_nicd[i]->coll = lu[13];
        c_nicd[i]->sat = lu[2];
        c_nicd[i]->sat += lu[3];
        c_nicd[i]->sat += lu[11];
        c_nicd[i]->sat += lu[12];
        c_nicd[i]->sat += lu[13];
        c_nicd[i]->sat += lu[14];
        i++;
    }
    fclose(fp);
}

/*
 ****************************************************************************
 * Compute NIC stats and print it out.
 ****************************************************************************
 */
void write_nicstats(WINDOW * window, struct nicdata_s *c_nicd[], struct nicdata_s *p_nicd[], unsigned int idev, unsigned long long itv)
{
    /* print headers */
    wclear(window);
    wattron(window, A_BOLD);
    wprintw(window, "\n    Interface:   rMbps   wMbps    rPk/s    wPk/s     rAvs     wAvs     IErr     OErr     Coll      Sat   %%rUtil   %%wUtil    %%Util\n");
    wattroff(window, A_BOLD);

    double rbps, rpps, wbps, wpps, ravs, wavs, ierr, oerr, coll, sat, rutil, wutil, util;
    unsigned int i = 0;

    for (i = 0; i < idev; i++) {
        /* skip interfaces which never seen packets */
        if (c_nicd[i]->rpackets == 0 && c_nicd[i]->wpackets == 0) {
           continue;
        }

        rbps = S_VALUE(p_nicd[i]->rbytes, c_nicd[i]->rbytes, itv);
        wbps = S_VALUE(p_nicd[i]->wbytes, c_nicd[i]->wbytes, itv);
        rpps = S_VALUE(p_nicd[i]->rpackets, c_nicd[i]->rpackets, itv);
        wpps = S_VALUE(p_nicd[i]->wpackets, c_nicd[i]->wpackets, itv);
        ierr = S_VALUE(p_nicd[i]->ierr, c_nicd[i]->ierr, itv);
        oerr = S_VALUE(p_nicd[i]->oerr, c_nicd[i]->oerr, itv);
        coll = S_VALUE(p_nicd[i]->coll, c_nicd[i]->coll, itv);
        sat = S_VALUE(p_nicd[i]->sat, c_nicd[i]->sat, itv);

	/* if no data about pps, zeroing averages */
        (rpps > 0) ? ( ravs = rbps / rpps ) : ( ravs = 0 );
        (wpps > 0) ? ( wavs = wbps / wpps ) : ( wavs = 0 );

        /* Calculate utilisation */
        if (c_nicd[i]->speed > 0) {
            /*
             * The following have a mysterious "800",
             * it is 100 for the % conversion, and 8 for bytes2bits.
             */
            rutil = min(rbps * 800 / c_nicd[i]->speed, 100);
            wutil = min(wbps * 800 / c_nicd[i]->speed, 100);
            if (c_nicd[i]->duplex == 2) {
                /* Full duplex */
                util = max(rutil, wutil);
            } else {
                /* Half Duplex */
                util = min((rbps + wbps) * 800 / c_nicd[i]->speed, 100);
            }
        } else {
            util = rutil = wutil = 0;
        }

        /* print statistics */
        wprintw(window, "%14s", c_nicd[i]->ifname);
        wprintw(window, "%8.2f%8.2f", rbps / 1024 / 128, wbps / 1024 / 128);
        wprintw(window, "%9.2f%9.2f", rpps, wpps);
        wprintw(window, "%9.2f%9.2f", ravs, wavs);
        wprintw(window, "%9.2f%9.2f%9.2f%9.2f", ierr, oerr, coll, sat);
        wprintw(window, "%9.2f%9.2f%9.2f", rutil, wutil, util);
        wprintw(window, "\n");
    }

    wrefresh(window);
}
