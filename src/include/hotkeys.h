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

#include <menu.h>
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

#define SUBTAB_NONE      0
#define SUBTAB_LOGTAIL   1
#define SUBTAB_IOSTAT    2
#define SUBTAB_NICSTAT   3

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
void print_help_tab(bool * first_iter);
void clear_tab_connopts(struct tab_s * tabs[], unsigned int i);
void do_noop(WINDOW * window, unsigned long interval);

/* main hotkeys functions */
void change_sort_order(struct tab_s * tab, bool increment, bool * first_iter);
void change_sort_order_direction(struct tab_s * tab, bool * first_iter);
void set_filter(WINDOW * win, struct tab_s * tab, PGresult * res, bool * first_iter);
unsigned int switch_tab(WINDOW * window, struct tab_s * tabs[],
        unsigned int ch, unsigned int tab_index, unsigned int tab_no, PGresult * res, bool * first_iter);
void switch_context(WINDOW * window, struct tab_s * tab, enum context context, PGresult * res, bool * first_iter);
void change_min_age(WINDOW * window, struct tab_s * tab, PGresult *res, bool *first_iter);
unsigned int add_tab(WINDOW * window, struct tab_s * tabs[],
        PGconn * conns[], unsigned int tab_index);
void shift_tabs(struct tab_s * tabs[], PGconn * conns[], unsigned int i);
unsigned int close_tab(WINDOW * window, struct tab_s * tabs[],
        PGconn * conns[], unsigned int tab_index, bool *first_iter);
void write_pgcenterrc(WINDOW * window, struct tab_s * tabs[], PGconn * conns[], struct args_s * args);
void reload_conf(WINDOW * window, PGconn * conn);
void edit_config(WINDOW * window, struct tab_s * tab, PGconn * conn, const char * config_file_guc);
void calculate_width(struct colAttrs *columns, PGresult *res, struct tab_s * tab, char ***arr, unsigned int n_rows, unsigned int n_cols);
void show_config(WINDOW * window, PGconn * conn);
void edit_config_menu(WINDOW * w_cmd, WINDOW * w_dba, struct tab_s * tab, PGconn * conn, bool *first_iter);
void pgss_menu(WINDOW * w_cmd, WINDOW * w_dba, struct tab_s * tab, bool *first_iter);
void pgss_switch(WINDOW * w_cmd, struct tab_s * tab, PGresult * p_res, bool *first_iter);
void signal_single_backend(WINDOW * window, struct tab_s *tab, PGconn * conn, bool do_terminate);
void get_statemask(WINDOW * window, struct tab_s * tab);
void set_statemask(WINDOW * window, struct tab_s * tab);
void signal_group_backend(WINDOW * window, struct tab_s *tab, PGconn * conn, bool do_terminate);
void start_psql(WINDOW * window, struct tab_s * tab);
unsigned long change_refresh(WINDOW * window, unsigned long interval);
void system_view_toggle(WINDOW * window, struct tab_s * tab, bool * first_iter);
void get_logfile_path(char * path, PGconn * conn);
void log_process(WINDOW * window, WINDOW ** w_log, struct tab_s * tab, PGconn * conn, unsigned int subtab);
void show_full_log(WINDOW * window, struct tab_s * tab, PGconn * conn);
void print_log(WINDOW * window, WINDOW * w_cmd, struct tab_s * tab, PGconn * conn);
void subtab_process(WINDOW * window, WINDOW ** w_sub, struct tab_s * tab, PGconn * conn, unsigned int subtab);
void get_query_by_id(WINDOW * window, struct tab_s * tab, PGconn * conn);
void pg_stat_reset(WINDOW * window, PGconn * conn, bool * reseted);
void draw_color_help(WINDOW * w,
        unsigned long long int * ws_color, unsigned long long int * wc_color, unsigned long long int * wa_color,
        unsigned long long int * wl_color, unsigned long long int target, unsigned long long int * target_color);
void change_colors(unsigned long long int * ws_color, unsigned long long int * wc_color,
        unsigned long long int * wa_color, unsigned long long int * wl_color);
#endif /* __HOTKEYS_H__ */
