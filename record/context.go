// Stuff related to recorder options settings, what to record and how.

package record

import (
	"github.com/lesovsky/pgcenter/lib/stat"
)

func (o *RecordOptions) Setup(pginfo stat.PgInfo) {
	o.contextList = stat.ContextList{
		stat.DatabaseView:          &stat.PgStatDatabaseUnit,
		stat.ReplicationView:       &stat.PgStatReplicationUnit,
		stat.TablesView:            &stat.PgStatTablesUnit,
		stat.IndexesView:           &stat.PgStatIndexesUnit,
		stat.SizesView:             &stat.PgTablesSizesUnit,
		stat.FunctionsView:         &stat.PgStatFunctionsUnit,
		stat.VacuumView:            &stat.PgStatVacuumUnit,
		stat.ActivityView:          &stat.PgStatActivityUnit,
		stat.StatementsTimingView:  &stat.PgSSTimingUnit,
		stat.StatementsGeneralView: &stat.PgSSGeneralUnit,
		stat.StatementsIOView:      &stat.PgSSIoUnit,
		stat.StatementsTempView:    &stat.PgSSTempUnit,
		stat.StatementsLocalView:   &stat.PgSSLocalUnit,
	}

	// Adjust queries depending on Postgres version
	o.contextList.AdjustQueries(pginfo)
	o.sharedOptions.Adjust(pginfo)
}
