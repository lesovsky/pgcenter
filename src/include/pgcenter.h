/*
 ****************************************************************************
 * pgcenter.h
 *      main program definitions and macros.
 *
 * (C) 2016 by Alexey V. Lesovsky (lesovsky <at> gmail.com)
 * 
 ****************************************************************************
 */

#ifndef PGCENTER_H
#define PGCENTER_H

#define INVALID_ORDER_KEY   99
#define PG_STAT_ACTIVITY_MIN_AGE_DEFAULT "00:00:00.0"

/* .pgcenterrc read statuses */
#define PGCENTERRC_READ_OK  0
#define PGCENTERRC_READ_ERR 1

/* init stuff functions */
struct args_s * init_args_mem(void);
void init_args_struct(struct args_s *args);
void init_tabs(struct tab_s *tabs[]);
char *** init_array(char ***arr, unsigned int n_rows, unsigned int n_cols);
char *** free_array(char ***arr, unsigned int n_rows, unsigned int n_cols);
void init_colors(unsigned long long int * ws_color, unsigned long long int * wc_color,
        unsigned long long int * wa_color, unsigned long long int * wl_color);

/* arguments handling functions */
void check_portnum(const char * portnum);
void arg_parse(int argc, char *argv[], struct args_s *args);
void create_initial_conn(struct args_s * args, struct tab_s * tabs[]);
unsigned int create_pgcenterrc_conn(struct args_s * args, struct tab_s * tabs[], unsigned int pos);

/* connections handling functions */
void prepare_conninfo(struct tab_s * tabs[]);

/* comparing values functions */
int str_cmp_desc(const void * a, const void * b, void * arg);
int str_cmp_asc(const void * a, const void * b, void * arg);
int int_cmp_desc(const void * a, const void * b, void * arg);
int int_cmp_asc(const void * a, const void * b, void * arg);
int fl_cmp_desc(const void * a, const void * b, void * arg);
int fl_cmp_asc(const void * a, const void * b, void * arg);

/* arrays functions */
void diff_arrays(char ***p_arr, char ***c_arr, char ***res_arr, struct tab_s * tab, 
        unsigned int n_rows, unsigned int n_cols, unsigned long interval);
void sort_array(char ***res_arr, unsigned int n_rows, struct tab_s * tab);
void pgrescpy(char ***arr, PGresult *res, unsigned int n_rows, unsigned int n_cols);

/* print info functions */
void print_title(WINDOW * window);
void print_loadavg(WINDOW * window, struct tab_s * tab, PGconn * conn);
void print_cpu_usage(WINDOW * window, struct cpu_s *st_cpu[], struct tab_s * tabs, PGconn * conn);
void print_mem_usage(WINDOW * window, struct mem_s *st_mem_short, struct tab_s * tab, PGconn * conn);
void print_conninfo(WINDOW * window, PGconn *conn, unsigned int tab_no);
void print_pg_general(WINDOW * window, struct tab_s * tab, PGconn * conn);
void print_postgres_activity(WINDOW * window, struct tab_s * tab, PGconn * conn);
void print_vacuum_info(WINDOW * window, struct tab_s * tab, PGconn * conn);
void print_pgss_info(WINDOW * window, PGconn * conn, unsigned long interval);
void print_data(WINDOW *window, PGresult *res, char ***arr, 
        unsigned int n_rows, unsigned int n_cols, struct tab_s * tab);
void print_iostat(WINDOW * window, WINDOW * w_cmd, struct tab_s * tab, PGconn * conn, bool * repaint);
void print_nicstat(WINDOW * window, WINDOW * w_cmd, struct tab_s * tab, struct nicdata_s *c_nicd[],
        struct nicdata_s *p_nicd[], unsigned int idev, bool * repaint);

/* other functions */
void print_usage(void);
void exit_prog(struct tab_s * tabs[], PGconn * conns[]);
#endif /* __PGCENTER_H__ */
