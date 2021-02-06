package top

import (
	"bytes"
	"github.com/jackc/pgx/v4"
	"github.com/jroimartin/gocui"
	"github.com/lesovsky/pgcenter/internal/postgres"
	"github.com/lesovsky/pgcenter/internal/query"
	"os"
	"os/exec"
	"strings"
	"text/template"
)

// report defines data container with report values.
// All values are string because all calculations are made on Postgres side, pgcenter just receives and prints final values.
type report struct {
	Query                  string // query text from pg_stat_statements.query
	QueryID                string // query ID from pg_stat_statements.queryid
	Usename                string // username based on pg_stat_statements.userid
	Datname                string // database name based on pg_database.datname
	TotalCalls             string // total number of calls for all queries
	TotalRows              string // total number of rows for all queries
	TotalAllTime           string // total amount of time spent executing for all queries (including plan, exec, IO, etc)
	TotalPlanTime          string // total amount of time spent planning for all queries
	TotalPlanTimeDistRatio string // ratio of total planning time across total time
	TotalCPUTime           string // total amount of time spent executing (excluding planning and doing IO) for all queries
	TotalCPUTimeDistRatio  string // ratio of total CPU time across total time
	TotalIOTime            string // total amount of time spent doing IO for all queries
	TotalIOTimeDistRatio   string // ratio of total IO time across total time
	Calls                  string // number of calls for particular query
	CallsRatio             string // calls ratio to total number of calls
	Rows                   string // number of rows for particular query
	RowsRatio              string // rows ratio to total number of rows
	AllTime                string // total amount of time spent doing particular query
	AllTimeRatio           string // ratio of query's total time to total time of all queries
	PlanTime               string // total amount of time spent planning particular query
	PlanTimeRatio          string // ratio of query's planning time to total time of all queries
	CPUTime                string // total amount of time spent executing particular query (with no planning, doing IO, etc)
	CPUTimeRatio           string // ratio of query's executing time to total time of all queries
	IOTime                 string // total amount of time spent doing IO for particular query
	IOTimeRatio            string // ratio of query's IO time to total time of all queries
	AvgAllTime             string // query's average time
	AvgPlanTime            string // query's average planning time
	AvgCPUTime             string // query's average CPU time
	AvgIOTime              string // query's average IO time
	PlanTimeDistRatio      string // ratio of planning time to total query time
	CPUTimeDistRatio       string // ratio of CPU time to total query time
	IOTimeDistRatio        string // ratio of IO time to total query time
}

const (
	// reportTemplate is the template for the report shown to user
	reportTemplate = `summary:
    total queries: {{.TotalCalls}}
    total rows: {{.TotalRows}}
    total_time: {{.TotalAllTime}}, 100% 
        total_plan_time: {{.TotalPlanTime}},  {{.TotalPlanTimeDistRatio}}%
        total_cpu_time: {{.TotalCPUTime}},  {{.TotalCPUTimeDistRatio}}%
        total_io_time: {{.TotalIOTime}},  {{.TotalIOTimeDistRatio}}%

query info:
    queryid:                               {{.QueryID}}
    username:                              {{.Usename}},
    database:                              {{.Datname}},
    calls (relative to total):             {{.Calls}},  {{.CallsRatio}}%,
    rows (relative to total):              {{.Rows}},  {{.RowsRatio}}%,
    total times (relative to total):       {{.AllTime}},  {{.AllTimeRatio}}%
        planning:                          {{.PlanTime}},  {{.PlanTimeRatio}}%
        cpu:                               {{.CPUTime}},  {{.CPUTimeRatio}}%
        io:                                {{.IOTime}},  {{.IOTimeRatio}}%
    average times (in-query distribution): {{.AvgAllTime}}ms,  100%
        planning:                          {{.AvgPlanTime}}ms,  {{.PlanTimeDistRatio}}%
        cpu:                               {{.AvgCPUTime}}ms,  {{.CPUTimeDistRatio}}%
        io:                                {{.AvgIOTime}}ms,  {{.IOTimeDistRatio}}%

    query text:
	{{.Query}}

	* cpu_time means execution time excluding time spent on reading or writing block IO. cpu_time includes waiting time implicitly.
`
)

// getQueryReport queries statements stats, generate the report and returns it.
func getQueryReport(answer string, version int, db *postgres.DB) (report, string) {
	answer = strings.TrimPrefix(answer, dialogPrompts[dialogQueryReport])
	answer = strings.TrimSuffix(answer, "\n")

	if answer == "" {
		return report{}, "Report: do nothing"
	}

	var r report
	err := db.QueryRow(query.SelectQueryReportQuery(version), answer).Scan(
		&r.Query, &r.QueryID, &r.Usename, &r.Datname, &r.TotalCalls, &r.TotalRows, &r.TotalAllTime,
		&r.TotalPlanTime, &r.TotalPlanTimeDistRatio, &r.TotalCPUTime, &r.TotalCPUTimeDistRatio, &r.TotalIOTime, &r.TotalIOTimeDistRatio,
		&r.Calls, &r.CallsRatio, &r.Rows, &r.RowsRatio,
		&r.AllTime, &r.AllTimeRatio, &r.PlanTime, &r.PlanTimeRatio, &r.CPUTime, &r.CPUTimeRatio, &r.IOTime, &r.IOTimeRatio,
		&r.AvgAllTime, &r.AvgPlanTime, &r.AvgCPUTime, &r.AvgIOTime,
		&r.PlanTimeDistRatio, &r.CPUTimeDistRatio, &r.IOTimeDistRatio,
	)

	if err == pgx.ErrNoRows {
		return report{}, "Report: no statistics for such queryid"
	}

	return r, ""
}

// printQueryReport prints report in $PAGER program.
func printQueryReport(g *gocui.Gui, r report, uiExit chan int) string {
	t, err := template.New("query").Parse(reportTemplate)
	if err != nil {
		return err.Error()
	}

	var buf bytes.Buffer
	err = t.Execute(&buf, r)
	if err != nil {
		return err.Error()
	}

	var pager string
	if pager = os.Getenv("PAGER"); pager == "" {
		pager = "less"
	}

	// Exit from UI, will restore it after $PAGER is closed.
	uiExit <- 1
	g.Close()

	cmd := exec.Command(pager) // #nosec G204
	cmd.Stdin = strings.NewReader(buf.String())
	cmd.Stdout = os.Stdout

	err = cmd.Run()
	if err != nil {
		return err.Error()
	}

	return ""
}
