/*
 ****************************************************************************
 * hotkeys.h
 *      definitions and macros for functions associated hotkeys.
 *
 * (C) 2016 by Alexey V. Lesovsky (lesovsky <at> gmail.com)
 * 
 ****************************************************************************
 */
#ifndef __HOTKEYS_H__
#define __HOTKEYS_H__

#include <ifaddrs.h>
#include <menu.h>
#include <netdb.h>
#include <pwd.h>
#include <sys/socket.h>
#include <sys/stat.h>
#include <sys/types.h>
#include <sys/wait.h>
#include "common.h"
#include "pgf.h"
#include "qstats.h"
#include "stats.h"

#define COL_MAXLEN		S_BUF_LEN

#define INTERVAL_MAXLEN	    300000000			/* 300 seconds */
#define DEFAULT_INTERVAL    1000000
#define INTERVAL_STEP       200000

/* process states according to pg_stat_activity */
#define GROUP_ACTIVE        1 << 0
#define GROUP_IDLE          1 << 1
#define GROUP_IDLE_IN_XACT  1 << 2
#define GROUP_WAITING       1 << 3
#define GROUP_OTHER         1 << 4

#define SUBSCREEN_NONE      0
#define SUBSCREEN_LOGTAIL   1
#define SUBSCREEN_IOSTAT    2
#define SUBSCREEN_NICSTAT   3

/* Macros used to determine array size */
#define ARRAY_SIZE(a) (sizeof(a) / sizeof(a[0]))

/* struct for column widths */
struct colAttrs {
    char name[COL_MAXLEN];
    int width;
};

/* aux functions */
struct colAttrs * init_colattrs(unsigned int n_cols);
ITEM ** init_menuitems(unsigned int n_choices);
bool key_is_pressed(void);
void print_help_screen(bool * first_iter);
void clear_screen_connopts(struct screen_s * screens[], unsigned int i);
void do_noop(WINDOW * window, unsigned long interval);

/* main hotkeys functions */
void change_sort_order(struct screen_s * screen, bool increment, bool * first_iter);
void change_sort_order_direction(struct screen_s * screen, bool * first_iter);
void set_filter(WINDOW * win, struct screen_s * screen, PGresult * res, bool * first_iter);
unsigned int switch_conn(WINDOW * window, struct screen_s * screens[],
        unsigned int ch, unsigned int console_index, unsigned int console_no, PGresult * res, bool * first_iter);
void switch_context(WINDOW * window, struct screen_s * screen, enum context context, PGresult * res, bool * first_iter);
void change_min_age(WINDOW * window, struct screen_s * screen, PGresult *res, bool *first_iter);
unsigned int add_connection(WINDOW * window, struct screen_s * screens[],
        PGconn * conns[], unsigned int console_index);
void shift_screens(struct screen_s * screens[], PGconn * conns[], unsigned int i);
unsigned int close_connection(WINDOW * window, struct screen_s * screens[],
        PGconn * conns[], unsigned int console_index, bool *first_iter);
void write_pgcenterrc(WINDOW * window, struct screen_s * screens[], PGconn * conns[], struct args_s * args);
void reload_conf(WINDOW * window, PGconn * conn);
bool check_pg_listen_addr(struct screen_s * screen, PGconn * conn);
void edit_config(WINDOW * window, struct screen_s * screen, PGconn * conn, const char * config_file_guc);
void calculate_width(struct colAttrs *columns, PGresult *res, struct screen_s * screen, char ***arr, unsigned int n_rows, unsigned int n_cols);
void show_config(WINDOW * window, PGconn * conn);
void edit_config_menu(WINDOW * w_cmd, WINDOW * w_dba, struct screen_s * screen, PGconn * conn, bool *first_iter);
void pgss_menu(WINDOW * w_cmd, WINDOW * w_dba, struct screen_s * screen, bool *first_iter);
void pgss_switch(WINDOW * w_cmd, struct screen_s * screen, PGresult * p_res, bool *first_iter);
void signal_single_backend(WINDOW * window, struct screen_s *screen, PGconn * conn, bool do_terminate);
void get_statemask(WINDOW * window, struct screen_s * screen);
void set_statemask(WINDOW * window, struct screen_s * screen);
void signal_group_backend(WINDOW * window, struct screen_s *screen, PGconn * conn, bool do_terminate);
void start_psql(WINDOW * window, struct screen_s * screen);
unsigned long change_refresh(WINDOW * window, unsigned long interval);
void system_view_toggle(WINDOW * window, struct screen_s * screen, bool * first_iter);
void get_logfile_path(char * path, PGconn * conn);
void log_process(WINDOW * window, WINDOW ** w_log, struct screen_s * screen, PGconn * conn, unsigned int subscreen);
void show_full_log(WINDOW * window, struct screen_s * screen, PGconn * conn);
void print_log(WINDOW * window, WINDOW * w_cmd, struct screen_s * screen, PGconn * conn);
void subscreen_process(WINDOW * window, WINDOW ** w_sub, struct screen_s * screen, PGconn * conn, unsigned int subscreen);
void get_query_by_id(WINDOW * window, struct screen_s * screen, PGconn * conn);
void pg_stat_reset(WINDOW * window, PGconn * conn, bool * reseted);
void draw_color_help(WINDOW * w, unsigned int * ws_color, unsigned int * wc_color,
        unsigned int * wa_color, unsigned int * wl_color, unsigned int target, unsigned int * target_color);
void change_colors(unsigned int * ws_color, unsigned int * wc_color, unsigned int * wa_color, unsigned int * wl_color);
#endif /* __HOTKEYS_H__ */
