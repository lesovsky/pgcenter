// Stuff related to query reporting.

package top

import (
	"bytes"
	"database/sql"
	"github.com/jroimartin/gocui"
	"github.com/lesovsky/pgcenter/internal/postgres"
	"os"
	"os/exec"
	"strings"
	"text/template"
)

// Container for report
type report struct {
	AllTotalTime    string
	AllIoTime       string
	AllCpuTime      string
	AllTotalTimePct float64
	AllIoTimePct    float64
	AllCpuTimePct   float64
	AllTotalQueries string
	TotalTimePct    float64
	IoTimePct       float64
	CpuTimePct      float64
	AvgTotalTimePct float64
	AvgIoTimePct    float64
	AvgCpuTimePct   float64
	TotalTime       string
	AvgTotalTime    float64
	AvgIoTime       float64
	AvgCpuTime      float64
	Calls           string
	CallPct         float64
	Rows            string
	RowsPct         float64
	Dbname          string
	Username        string
	Query           string
	QueryId         string
}

const (
	// pgssReportQuery queries statements from pg_stat_statements and process stat on Postgres-side
	pgssReportQuery = `WITH pg_stat_statements_normalized AS (
        SELECT *,
            regexp_replace(
            regexp_replace(
            regexp_replace(
            regexp_replace(
            regexp_replace(query,
            E'\\\\?(::[a-zA-Z_]+)?( *, *\\\\?(::[a-zA-Z_]+)?)+', '?', 'g'),
            E'\\\\$[0-9]+(::[a-zA-Z_]+)?( *, *\\\\$[0-9]+(::[a-zA-Z_]+)?)*', '$N', 'g'),
            E'--.*$', '', 'ng'),
            E'/\\\\*.*?\\\\*\\/', '', 'g'),
            E'\\\\s+', ' ', 'g')
            AS query_normalized
        FROM pg_stat_statements
    ),
    totals AS (
        SELECT
            sum(total_time) AS total_time,
            greatest(sum(blk_read_time+blk_write_time), 1) AS io_time,
            sum(total_time-blk_read_time-blk_write_time) AS cpu_time,
            sum(calls) AS ncalls, sum(rows) AS total_rows
        FROM pg_stat_statements
    ),
    _pg_stat_statements AS (
        SELECT
            d.datname AS database, a.rolname AS username,
            replace(
            (array_agg(query ORDER BY length(query)))[1],
            E'-- \n', E'--\n') AS query,
            sum(total_time) AS total_time,
            sum(blk_read_time) AS blk_read_time, sum(blk_write_time) AS blk_write_time,
            sum(calls) AS calls, sum(rows) AS rows
        FROM pg_stat_statements_normalized p
        JOIN pg_roles a ON a.oid=p.userid
        JOIN pg_database d ON d.oid=p.dbid
        WHERE TRUE AND left(md5(p.dbid::text || p.userid || p.queryid), 10) = $1
        GROUP BY d.datname, a.rolname, query_normalized
    ),
    totals_readable AS (
        SELECT
            to_char(interval '1 millisecond' * total_time, 'HH24:MI:SS') AS all_total_time,
            to_char(interval '1 millisecond' * io_time, 'HH24:MI:SS') AS all_io_time,
            to_char(interval '1 millisecond' * cpu_time, 'HH24:MI:SS') AS all_cpu_time,
            (100*total_time/total_time)::numeric(20,2) AS all_total_time_percent,
            (100*io_time/total_time)::numeric(20,2) AS all_io_time_percent,
            (100*cpu_time/total_time)::numeric(20,2) AS all_cpu_time_percent,
            to_char(ncalls, 'FM999,999,999,990') AS all_total_queries
        FROM totals
    ),
    statements AS (
        SELECT
            (100*total_time/(select total_time FROM totals)) AS time_percent,
            (100*(blk_read_time+blk_write_time)/(select io_time FROM totals)) AS io_time_percent,
            (100*(total_time-blk_read_time-blk_write_time)/(select cpu_time FROM totals)) AS cpu_time_percent,
            to_char(interval '1 millisecond' * total_time, 'HH24:MI:SS') AS total_time,
            (total_time::numeric/calls)::numeric(20,2) AS avg_time,
            ((total_time-blk_read_time-blk_write_time)::numeric/calls)::numeric(20, 2) AS avg_cpu_time,
            ((blk_read_time+blk_write_time)::numeric/calls)::numeric(20, 2) AS avg_io_time,
            to_char(calls, 'FM999,999,999,990') AS calls,
            (100*calls/(select ncalls FROM totals))::numeric(20, 2) AS calls_percent,
            to_char(rows, 'FM999,999,999,990') AS rows,
            (100*rows/(select total_rows FROM totals))::numeric(20, 2) AS row_percent,
            database, username, query
        FROM _pg_stat_statements
    ),
    statements_readable AS (
        SELECT
            to_char(time_percent, 'FM990.0') AS time_percent,
            to_char(io_time_percent, 'FM990.0') AS io_time_percent,
            to_char(cpu_time_percent, 'FM990.0') AS cpu_time_percent,
            to_char(avg_time*100/(coalesce(nullif(avg_time, 0), 1)), 'FM990.0') AS avg_time_percent,
            to_char(avg_io_time*100/(coalesce(nullif(avg_time, 0), 1)), 'FM990.0') AS avg_io_time_percent,
            to_char(avg_cpu_time*100/(coalesce(nullif(avg_time, 0), 1)), 'FM990.0') AS avg_cpu_time_percent,
            total_time, avg_time, avg_cpu_time, avg_io_time,
            calls, calls_percent, rows, row_percent,
            database, username, query
        FROM statements s
    )
    SELECT * FROM totals_readable CROSS JOIN statements_readable`

	// reportTemplate is the template for the report shown to user
	reportTemplate = `summary:
	total_time: {{.AllTotalTime}}, cpu_time: {{.AllCpuTime}}, io_time: {{.AllIoTime}} (ALL: {{.AllTotalTimePct}}%, CPU: {{.AllCpuTimePct}}%, IO: {{.AllIoTimePct}}%)
	total queries: {{.AllTotalQueries}}

query info:
	usename:				{{.Username}},
	datname:				{{.Dbname}},
	calls (relative to all queries):	{{.Calls}} ({{.CallPct}}%),
	rows (relative to all queries):		{{.Rows}} ({{.RowsPct}}%),
	total time (relative to all queries):	{{.TotalTime}} (ALL: {{.TotalTimePct}}%, CPU: {{.CpuTimePct}}%, IO: {{.IoTimePct}}%),
	average time (only for this query):	{{.AvgTotalTime}}ms, cpu_time: {{.AvgCpuTime}}ms, io_time: {{.AvgIoTime}}ms, (ALL: {{.AvgTotalTimePct}}%, CPU: {{.AvgCpuTimePct}}%, IO: {{.AvgIoTimePct}}%),

query text (id: {{.QueryId}}):
	{{.Query}}`
)

// buildQueryReport queries statements stats, generate the report and shows it.
func buildQueryReport(g *gocui.Gui, v *gocui.View, answer string, db *postgres.DB, doExit chan int) {
	answer = strings.TrimPrefix(string(v.Buffer()), dialogPrompts[dialogQueryReport])
	answer = strings.TrimSuffix(answer, "\n")

	if answer == "" {
		printCmdline(g, "Do nothing.")
		return
	}

	var r report
	err := db.QueryRow(pgssReportQuery, answer).Scan(&r.AllTotalTime, &r.AllIoTime, &r.AllCpuTime,
		&r.AllTotalTimePct, &r.AllIoTimePct, &r.AllCpuTimePct,
		&r.AllTotalQueries, &r.TotalTimePct, &r.IoTimePct, &r.CpuTimePct,
		&r.AvgTotalTimePct, &r.AvgIoTimePct, &r.AvgCpuTimePct,
		&r.TotalTime, &r.AvgTotalTime, &r.AvgCpuTime, &r.AvgIoTime, &r.Calls, &r.CallPct, &r.Rows, &r.RowsPct,
		&r.Dbname, &r.Username, &r.Query)

	if err == sql.ErrNoRows {
		printCmdline(g, "No stats for such queryid.")
		return
	}

	r.QueryId = answer
	if err := r.Print(g, doExit); err != nil {
		printCmdline(g, "Failed to show query report.")
	}

	return
}

// Print method prints report in $PAGER program.
func (r *report) Print(g *gocui.Gui, doExit chan int) error {
	t := template.Must(template.New("query").Parse(reportTemplate))
	buf := &bytes.Buffer{}
	if err := t.Execute(buf, r); err != nil {
		return err
	}

	var pager string
	if pager = os.Getenv("PAGER"); pager == "" {
		pager = "less"
	}

	// Exit from UI, will restore it after $PAGER is closed.
	doExit <- 1
	g.Close()

	cmd := exec.Command(pager)
	cmd.Stdin = strings.NewReader(buf.String())
	cmd.Stdout = os.Stdout

	if err := cmd.Run(); err != nil {
		// If external program fails, save error and show it to user in next UI iteration
		errSaved = err
		return err
	}

	return nil
}
