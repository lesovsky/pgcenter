/*
 * based on https://github.com/PostgreSQL-Consulting/pg-utils/blob/master/sql/global_reports/query_stat_total.sql
 */

#define PG_GET_QUERYTEXT_BY_QUERYID_QUERY_P1 \
    "WITH pg_stat_statements_normalized AS ( \
        SELECT *, \
            regexp_replace( \
            regexp_replace( \
            regexp_replace( \
            regexp_replace( \
            regexp_replace(query, \
            E'\\\\?(::[a-zA-Z_]+)?( *, *\\\\?(::[a-zA-Z_]+)?)+', '?', 'g'), \
            E'\\\\$[0-9]+(::[a-zA-Z_]+)?( *, *\\\\$[0-9]+(::[a-zA-Z_]+)?)*', '$N', 'g'), \
            E'--.*$', '', 'ng'), \
            E'/\\\\*.*?\\\\*\\/', '', 'g'), \
            E'\\\\s+', ' ', 'g') \
            AS query_normalized \
        FROM pg_stat_statements \
    ), \
    totals AS ( \
        SELECT  \
            sum(total_time) AS total_time, \
            greatest(sum(blk_read_time+blk_write_time), 1) AS io_time, \
            sum(total_time-blk_read_time-blk_write_time) AS cpu_time, \
            sum(calls) AS ncalls, sum(rows) AS total_rows \
        FROM pg_stat_statements \
    ), \
    _pg_stat_statements AS ( \
        SELECT \
            d.datname AS database, a.rolname AS username, \
            replace( \
            (array_agg(query ORDER BY length(query)))[1], \
            E'-- \n', E'--\n') AS query, \
            sum(total_time) AS total_time, \
            sum(blk_read_time) AS blk_read_time, sum(blk_write_time) AS blk_write_time, \
            sum(calls) AS calls, sum(rows) AS rows \
        FROM pg_stat_statements_normalized p \
        JOIN pg_authid a ON a.oid=p.userid \
        JOIN pg_database d ON d.oid=p.dbid \
        WHERE TRUE AND left(md5(d.datname || a.rolname || p.query ), 10) = '"

#define PG_GET_QUERYTEXT_BY_QUERYID_QUERY_P2 \
    "' \
        GROUP BY d.datname, a.rolname, query_normalized \
    ), \
    totals_readable AS ( \
        SELECT \
            to_char(interval '1 millisecond' * total_time, 'HH24:MI:SS') AS all_total_time, \
            to_char(interval '1 millisecond' * io_time, 'HH24:MI:SS') AS all_io_time, \
            to_char(interval '1 millisecond' * cpu_time, 'HH24:MI:SS') AS all_cpu_time, \
            (100*total_time/total_time)::numeric(20,2) AS all_total_time_percent, \
            (100*io_time/total_time)::numeric(20,2) AS all_io_time_percent, \
            (100*cpu_time/total_time)::numeric(20,2) AS all_cpu_time_percent, \
            to_char(ncalls, 'FM999,999,999,990') AS all_total_queries \
        FROM totals \
    ), \
    statements AS ( \
        SELECT \
            (100*total_time/(select total_time FROM totals)) AS time_percent, \
            (100*(blk_read_time+blk_write_time)/(select io_time FROM totals)) AS io_time_percent, \
            (100*(total_time-blk_read_time-blk_write_time)/(select cpu_time FROM totals)) AS cpu_time_percent, \
            to_char(interval '1 millisecond' * total_time, 'HH24:MI:SS') AS total_time, \
            (total_time::numeric/calls)::numeric(20,2) AS avg_time, \
            ((total_time-blk_read_time-blk_write_time)::numeric/calls)::numeric(20, 2) AS avg_cpu_time, \
            ((blk_read_time+blk_write_time)::numeric/calls)::numeric(20, 2) AS avg_io_time, \
            to_char(calls, 'FM999,999,999,990') AS calls, \
            (100*calls/(select ncalls FROM totals))::numeric(20, 2) AS calls_percent, \
            to_char(rows, 'FM999,999,999,990') AS rows, \
            (100*rows/(select total_rows FROM totals))::numeric(20, 2) AS row_percent, \
            database, username, query \
        FROM _pg_stat_statements \
    ), \
    statements_readable AS ( \
        SELECT \
            to_char(time_percent, 'FM990.0') AS time_percent, \
            to_char(io_time_percent, 'FM990.0') AS io_time_percent, \
            to_char(cpu_time_percent, 'FM990.0') AS cpu_time_percent, \
            to_char(avg_time*100/(coalesce(nullif(avg_time, 0), 1)), 'FM990.0') AS avg_time_percent, \
            to_char(avg_io_time*100/(coalesce(nullif(avg_time, 0), 1)), 'FM990.0') AS avg_io_time_percent, \
            to_char(avg_cpu_time*100/(coalesce(nullif(avg_time, 0), 1)), 'FM990.0') AS avg_cpu_time_percent, \
            total_time, avg_time, avg_cpu_time, avg_io_time, \
            calls, calls_percent, rows, row_percent, \
            database, username, query \
        FROM statements s \
    ) \
    SELECT * FROM totals_readable CROSS JOIN statements_readable" 

#define REP_ALL_TOTAL_TIME          0
#define REP_ALL_IO_TIME             1
#define REP_ALL_CPU_TIME            2
#define REP_ALL_TOTAL_TIME_PCT      3
#define REP_ALL_IO_TIME_PCT         4
#define REP_ALL_CPU_TIME_PCT        5
#define REP_ALL_TOTAL_QUERIES       6
#define REP_TOTAL_TIME_PCT          7
#define REP_IO_TIME_PCT             8
#define REP_CPU_TIME_PCT            9
#define REP_AVG_TIME_PCT            10
#define REP_AVG_IO_TIME_PCT         11
#define REP_AVG_CPU_TIME_PCT        12
#define REP_TOTAL_TIME              13
#define REP_AVG_TIME                14
#define REP_AVG_CPU_TIME            15
#define REP_AVG_IO_TIME             16
#define REP_CALLS                   17
#define REP_CALLS_PCT               18
#define REP_ROWS                    19
#define REP_ROWS_PCT                20
#define REP_DBNAME                  21
#define REP_USER                    22
#define REP_QUERY                   23
