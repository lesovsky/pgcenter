package stat

const (
	PgStatDatabaseDescription = `pg_stat_database view shows database-wide statistics:

  column	origin		description
- datname	datname		Name of this database
- commits	xact_commit	Number of transactions in this database that have been committed
- rollbacks	xact_rollback	Number of transactions in this database that have been rolled back
- reads		blks_read	Number of disk blocks read in this database
- hits		blks_hit	Number of times disk blocks were found already in the buffer cache, so that a read was 
				not necessary (this only includes hits in the PostgreSQL buffer cache, not the operating
				system's file system cache)
- returned	tup_returned	Number of rows returned by queries in this database
- fetched	tup_fetched	Number of rows fetched by queries in this database
- inserts	tup_inserted	Number of rows inserted by queries in this database
- updates	tup_updated	Number of rows updated by queries in this database
- deletes	tup_deleted	Number of rows deleted by queries in this database
- conflicts	conflicts	Number of queries canceled due to conflicts with recovery in this database. (Conflicts
				occur only on standby servers; see pg_stat_database_conflicts for details.)
- deadlocks	deadlocks	Number of deadlocks detected in this database
- temp_files	temp_files	Number of temporary files created by queries in this database. All temporary files are 
				counted, regardless of why the temporary file was created (e.g., sorting or hashing), 
				and regardless of the log_temp_files setting.
- temp_bytes	temp_bytes	Total amount of data written to temporary files by queries in this database. All 
				temporary files are counted, regardless of why the temporary file was created, and 
				regardless of the log_temp_files setting.
- read_t	blk_read_time	Time spent reading data file blocks by backends in this database, in milliseconds
- write_t	blk_write_time	Time spent writing data file blocks by backends in this database, in milliseconds
- stats_age*	stats_reset	Age of collected statistics in the moment when stats are taken from this database

* - derivative value, calculated using additional functions.

Details: https://www.postgresql.org/docs/current/static/monitoring-stats.html#PG-STAT-DATABASE-VIEW`

	//PgStatReplicationDescription = ``

	//PgStatTablesDescription = ``

	//PgStatIndexesDescription = ``

	//PgStatFunctionsDescription = ``

	//PgStatSizesDescription = ``

	//PgStatActivityDescription = ``

	//PgStatVacuumDescription = ``

	//PgStatStatementsDescription = ``
)
