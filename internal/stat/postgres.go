package stat

import (
	"fmt"
	"github.com/lesovsky/pgcenter/internal/postgres"
	"github.com/lesovsky/pgcenter/lib/stat"
)

// PostgresProperties is the container for details about Postgres
type PostgresProperties struct {
	State                   string  // state of Postgres - up or down
	VersionNum              int     // Numeric representation of Postgres version, e.g. XXYYZZ
	Version                 string  // String representation of Postgres version, e.g. X.Y.Z
	StartTime               float64 // Postgres start time
	Recovery                string  // Recovery state
	GucTrackCommitTimestamp string  // value of track_commit_timestamp GUC
	GucAVMaxWorkers         int     // value of autovacuum_max_workers GUC
	GucMaxConnections       int     // value of max_connections GUC
	GucMaxPrepXacts         int     // value of max_prepared_transactions GUC
	ExtPGSSAvail            bool    // is 'pg_stat_statements' extension installed?
	SchemaPgcenterAvail     bool    // is 'pgcenter' schema installed?
	SysTicks                float64 // ad-hoc implementation of GET_CLK for cases when Postgres is remote
}

func collectPostgresStat(db *postgres.DB, prev Pgstat) (Pgstat, error) {
	summary, err := collectSummaryStat(db, prev)
	if err != nil {
		return Pgstat{}, err
	}

	stats := Pgstat{
		Activity: summary,
		Prev:     prev.Curr,
	}

	opts := stat.Options{
		ShowNoIdle:     true,
		QueryAgeThresh: "00:00:00.0",
	}

	query, err := stat.PrepareQuery(stat.PgStatActivityQueryDefault, opts)
	if err != nil {
		return Pgstat{}, err
	}

	/* lessqqmorepewpew: адский хардкод тут конечно, подстраиваемся под взятие pg_stat_activity статы */
	err = stats.GetPgstatDiff(db, query, 1, stat.NoDiff, 0, true, 0)
	if err != nil {
		return Pgstat{}, err
	}

	return stats, nil
}

func collectSummaryStat(db *postgres.DB, prev Pgstat) (PostgresActivity, error) {
	s := PostgresActivity{}
	if state := getPgState(db); state != "ok" {
		return s, fmt.Errorf("postgres state is not ok")
	}

	if err := db.QueryRow(PgGetUptimeQuery).Scan(&s.Uptime); err != nil {
		s.Uptime = "--:--:--"
	}

	db.QueryRow(PgGetRecoveryStatusQuery).Scan(&s.Recovery)

	queryActivity := PgActivityQueryDefault
	queryAutovac := PgAutovacQueryDefault

	/* lessqqmorepewpew: доделать выбор запроса в зависимости от версии */
	//switch {
	//case s.PgVersionNum < 90400:
	//  queryActivity = PgActivityQueryBefore94
	//  queryAutovac = PgAutovacQueryBefore94
	//case s.PgVersionNum < 90600:
	//  queryActivity = PgActivityQueryBefore96
	//case s.PgVersionNum < 100000:
	//  queryActivity = PgActivityQueryBefore10
	//default:
	//  // use defaults
	//}

	db.QueryRow(queryActivity).Scan(
		&s.ConnTotal, &s.ConnIdle, &s.ConnIdleXact,
		&s.ConnActive, &s.ConnWaiting, &s.ConnOthers,
		&s.ConnPrepared)

	db.QueryRow(queryAutovac).Scan(&s.AVWorkers, &s.AVAntiwrap, &s.AVManual, &s.AVMaxTime)

	// read pg_stat_statements only if it's available
	//if s.PgStatStatementsAvail == true {
	/* lessqqmorepewpew: пока временно предполагаем что pg_stat_statements установлена в базе и наш интервал всегда 1 секунда */
	if true {
		db.QueryRow(PgStatementsQuery).Scan(&s.StmtAvgTime, &s.Calls)
		s.CallsRate = (s.Calls - prev.Activity.Calls) / 1
	}

	db.QueryRow(PgActivityTimeQuery).Scan(&s.XactMaxTime, &s.PrepMaxTime)

	return s, nil
}
