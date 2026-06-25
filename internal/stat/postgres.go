package stat

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/lesovsky/pgcenter/internal/postgres"
	"github.com/lesovsky/pgcenter/internal/query"
	"io"
	"sort"
	"strconv"
	"strings"
)

// Pgstat describes collected Postgres stats.
type Pgstat struct {
	Activity Activity
	Result   PGresult
	Overview PgstatOverview
}

// PgstatOverview holds the flat aggregates backing the five verbose pgstat panel rows
// (workload, databases, workers, replication, bgwr/ckpt). Absolute counters are stored so rates
// can be computed against the previous snapshot; already-computed rates carry the *Rate suffix.
// Signals that may be unavailable (no replication, no slots, archive_mode=off, missing privilege,
// first tick with no prev) use sql.NullInt64/availability flags so Task 8 can render a literal
// "n/a" that is distinguishable from a real 0.
type PgstatOverview struct {
	// Valid is true once at least one tick has populated the struct; the rate fields below are only
	// meaningful when a previous snapshot existed (HasPrev).
	Valid   bool
	HasPrev bool // false on the first tick (no prev snapshot) -> delta-based fields are n/a

	// workload row. tps = (Δcommits + Δrollbacks)/itv. ins/upd/del/ret/tmp are per-second rates.
	// others = Δ(deadlocks + conflicts + checksum_failures) over the interval (not /s).
	WorkloadCommits   int64 // absolute, for rate
	WorkloadRollbacks int64 // absolute, for rate
	WorkloadInserts   int64 // absolute, for rate
	WorkloadUpdates   int64 // absolute, for rate
	WorkloadDeletes   int64 // absolute, for rate
	WorkloadReturned  int64 // absolute, for rate
	WorkloadTempFiles int64 // absolute, for rate
	WorkloadOthers    int64 // absolute sum of deadlocks+conflicts+checksum_failures, for interval delta

	TPSRate        int64 // (Δcommits+Δrollbacks)/itv
	InsertsRate    int64 // Δinserts/itv
	UpdatesRate    int64 // Δupdates/itv
	DeletesRate    int64 // Δdeletes/itv
	ReturnedRate   int64 // Δreturned/itv
	TempFilesRate  int64 // Δtemp_files/itv
	OthersInterval int64 // Δothers over the interval (value, not /s)

	// databases row.
	DatabasesCount int64 // number of databases
	TotalSize      int64 // sum(pg_database_size), bytes; n/a if the (privileged) aggregate failed
	TotalSizeValid bool  // false -> TotalSize/GrowthPerSec are n/a
	GrowthPerSec   int64 // Go-side delta of TotalSize over the interval, bytes/s

	BlksHit  int64 // absolute, for per-interval cache hit ratio
	BlksRead int64 // absolute, for per-interval cache hit ratio
	// CacheHitRatio is the per-interval Δhit/Δ(hit+read), as a percentage [0,100].
	// CacheHitRatioValid is false when Δ(hit+read) == 0 (no I/O in the interval) or on the first tick.
	CacheHitRatio      float64
	CacheHitRatioValid bool

	// workers row. Active counts from pg_stat_activity; limits from GUCs.
	WorkersUmbrellaActive int // background-worker backends occupying max_worker_processes slots
	WorkersLogicalActive  int // logical replication worker backends
	WorkersParallelActive int // parallel worker backends

	// replication row.
	WalSize int64 // pg_wal directory size, bytes

	LagBytes      int64 // worst-case replication lag, bytes
	LagBytesValid bool  // false when there are no standbys (n/a)

	SlotsCount    int64 // number of replication slots
	RetainedBytes int64 // largest retained WAL across slots, bytes
	RetainedValid bool  // false when there are no slots (n/a)

	ArchivingBacklog      int64 // count(.ready) * wal_segment_size, bytes
	ArchivingBacklogValid bool  // false on archive_mode=off / missing privilege (42501) -> n/a

	Senders   int // active walsenders
	Receivers int // active walreceivers

	// bgwr/ckpt row. timed/req are absolute cumulative; write/sync ms are per-interval deltas.
	CkptTimed int64 // absolute
	CkptReq   int64 // absolute

	CkptWriteMs      float64 // absolute, for rate
	CkptSyncMs       float64 // absolute, for rate
	CkptWriteMsDelta float64 // Δwrite_ms over the interval
	CkptSyncMsDelta  float64 // Δsync_ms over the interval

	MaxWritten      int64 // absolute, for rate
	MaxWrittenDelta int64 // Δmaxwritten over the interval
}

// collectPostgresStat collect Postgres activity stats and stats returned by passed query.
func collectPostgresStat(db *postgres.DB, query string) (PGresult, error) {
	res, err := NewPGresultQuery(db, query)
	if err != nil {
		return PGresult{}, err
	}

	return res, nil
}

// Activity describes Postgres' current activity stats.
type Activity struct {
	State        string  // state of Postgres - up or down
	ConnTotal    int     // total number of connections
	ConnIdle     int     // number of idle connections
	ConnIdleXact int     // number of idle transactions
	ConnActive   int     // number of active connections
	ConnWaiting  int     // number of waiting backends
	ConnOthers   int     // connections with misc. states
	ConnPrepared int     // number of prepared transactions
	AVWorkers    int     // number of regular autovacuum workers
	AVAntiwrap   int     // number of antiwraparound vacuum workers
	AVUser       int     // number of vacuums started by user
	XactMaxTime  string  // duration of the longest running xact or query
	PrepMaxTime  string  // duration of the longest running prepared xact
	AVMaxTime    string  // duration of the longest (auto)vacuum
	StmtAvgTime  float32 // average duration of queries
	Uptime       string  // Postgres uptime (since start)
	Recovery     string  // Postgres recovery state
	Calls        int     // Number of calls
	CallsRate    int     // Number of calls per refresh interval
}

// collectActivityStat collects Postgres runtime activity about connected clients and workload.
func collectActivityStat(db *postgres.DB, version int, pgssSchema string, itv int, prevCalls int) (Activity, error) {
	var s Activity

	if err := db.QueryRow(query.GetUptime).Scan(&s.Uptime); err != nil {
		s.Uptime = "--:--:--"
	}

	if err := db.QueryRow(query.GetRecoveryStatus).Scan(&s.Recovery); err != nil {
		return s, err
	}

	// Depending on Postgres version select proper queries.
	queryActivity := query.SelectActivityActivityQuery(version)
	queryAutovacuum := query.SelectActivityAutovacuumQuery(version)

	err := db.QueryRow(queryActivity).Scan(
		&s.ConnTotal, &s.ConnIdle, &s.ConnIdleXact, &s.ConnActive, &s.ConnWaiting, &s.ConnOthers, &s.ConnPrepared)
	if err != nil {
		return s, err
	}

	err = db.QueryRow(queryAutovacuum).Scan(&s.AVWorkers, &s.AVAntiwrap, &s.AVUser, &s.AVMaxTime)
	if err != nil {
		return s, err
	}

	// read pg_stat_statements only if it's available
	if pgssSchema != "" {
		tmpl := query.SelectActivityStatementsQuery(version)
		q, err := query.Format(tmpl, query.Options{PGSSSchema: pgssSchema})
		if err != nil {
			return s, err
		}

		err = db.QueryRow(q).Scan(&s.StmtAvgTime, &s.Calls)
		if err != nil {
			return s, err
		}
		s.CallsRate = (s.Calls - prevCalls) / itv
	}

	err = db.QueryRow(query.SelectActivityTimes).Scan(&s.XactMaxTime, &s.PrepMaxTime)
	if err != nil {
		return s, err
	}

	s.State = "ok"
	return s, nil
}

// collectOverviewStat collects the flat aggregates backing the five verbose pgstat panel rows.
// It mirrors collectActivityStat: a version dispatcher selects the proper queries, each independent
// aggregate is scanned via its OWN QueryRow, and Go-side rates are computed against the prev
// snapshot. Expensive/privileged aggregates (sum of database sizes, archiving backlog) run as their
// own QueryRow so a failure degrades only that field to n/a (availability flag) without aborting the
// sample or surfacing the raw PG error text. itv is the refresh interval in seconds (>= 1).
func collectOverviewStat(db *postgres.DB, props PostgresProperties, itv int, prev PgstatOverview) PgstatOverview {
	var s PgstatOverview
	s.Valid = true
	s.HasPrev = prev.Valid

	if itv < 1 {
		itv = 1
	}
	version := props.VersionNum

	// Build recovery-aware options for the WAL-function templates.
	opts := query.NewOptions(version, props.Recovery, props.GucTrackCommitTimestamp, 0, "")

	// workload aggregate (single QueryRow over pg_stat_database). deadlocks/conflicts/checksum_failures
	// are scanned into locals and summed into WorkloadOthers.
	var deadlocks, conflicts, csumFailures int64
	err := db.QueryRow(query.OverviewWorkload).Scan(
		&s.WorkloadCommits, &s.WorkloadRollbacks, &s.WorkloadInserts, &s.WorkloadUpdates,
		&s.WorkloadDeletes, &s.WorkloadReturned, &s.WorkloadTempFiles,
		&deadlocks, &conflicts, &csumFailures,
	)
	if err == nil {
		s.WorkloadOthers = deadlocks + conflicts + csumFailures
		if s.HasPrev {
			s.TPSRate = ((s.WorkloadCommits + s.WorkloadRollbacks) - (prev.WorkloadCommits + prev.WorkloadRollbacks)) / int64(itv)
			s.InsertsRate = (s.WorkloadInserts - prev.WorkloadInserts) / int64(itv)
			s.UpdatesRate = (s.WorkloadUpdates - prev.WorkloadUpdates) / int64(itv)
			s.DeletesRate = (s.WorkloadDeletes - prev.WorkloadDeletes) / int64(itv)
			s.ReturnedRate = (s.WorkloadReturned - prev.WorkloadReturned) / int64(itv)
			s.TempFilesRate = (s.WorkloadTempFiles - prev.WorkloadTempFiles) / int64(itv)
			s.OthersInterval = s.WorkloadOthers - prev.WorkloadOthers
		}
	}

	// databases aggregate: the cheap, always-available signals (count + cache counters).
	if err := db.QueryRow(query.OverviewDatabases).Scan(
		&s.DatabasesCount, &s.BlksHit, &s.BlksRead,
	); err == nil {
		// Per-interval cache hit ratio: Δhit / Δ(hit + read). Division by zero (no I/O) -> n/a.
		if s.HasPrev {
			s.CacheHitRatio, s.CacheHitRatioValid = cacheHitRatio(s.BlksHit-prev.BlksHit, s.BlksRead-prev.BlksRead)
		}
	}

	// databases aggregate: sum(pg_database_size(...)) is expensive/privileged, so it runs as its OWN
	// QueryRow. A failure degrades only the size/growth field to n/a (TotalSizeValid stays false),
	// leaving count and cache-hit above intact. The raw error is swallowed (paths must not surface).
	if err := db.QueryRow(query.OverviewDatabasesSize).Scan(&s.TotalSize); err == nil {
		s.TotalSizeValid = true
		if s.HasPrev && prev.TotalSizeValid {
			s.GrowthPerSec = (s.TotalSize - prev.TotalSize) / int64(itv)
		}
	}

	// workers aggregate. On error the active counts stay zero (the struct remains Valid).
	_ = db.QueryRow(query.OverviewWorkers).Scan(
		&s.WorkersUmbrellaActive, &s.WorkersLogicalActive, &s.WorkersParallelActive,
	)

	// replication: wal size (own QueryRow; reads pg_wal directory).
	_ = db.QueryRow(query.OverviewWalSize).Scan(&s.WalSize)

	// replication: worst-case lag bytes (template). NULL (no standbys) -> n/a.
	if q, ferr := query.Format(query.OverviewReplicationLag, opts); ferr == nil {
		var lag sql.NullInt64
		if err := db.QueryRow(q).Scan(&lag); err == nil && lag.Valid {
			s.LagBytes = lag.Int64
			s.LagBytesValid = true
		}
	}

	// replication: slots count + largest retained WAL (template). retained NULL (no slots) -> n/a.
	if q, ferr := query.Format(query.OverviewReplicationSlots, opts); ferr == nil {
		var retained sql.NullInt64
		if err := db.QueryRow(q).Scan(&s.SlotsCount, &retained); err == nil && retained.Valid {
			s.RetainedBytes = retained.Int64
			s.RetainedValid = true
		}
	}

	// replication: archiving backlog. OWN QueryRow: pg_ls_dir requires pg_monitor/superuser; a 42501
	// privilege error or archive_mode=off degrades this field to n/a. The raw error (which contains a
	// filesystem path) is deliberately swallowed and never logged or surfaced.
	var backlog sql.NullInt64
	if err := db.QueryRow(query.OverviewArchivingBacklog).Scan(&backlog); err == nil && backlog.Valid {
		s.ArchivingBacklog = backlog.Int64
		s.ArchivingBacklogValid = true
	}

	// replication: senders/receivers.
	_ = db.QueryRow(query.OverviewSendRecv).Scan(&s.Senders, &s.Receivers)

	// bgwr/ckpt: reuse SelectStatBgwriterQuery (no new SQL). Column layout differs across PG14-16/17/18,
	// so scan generically by column name.
	collectOverviewBgwriter(db, version, &s, prev)

	return s
}

// collectOverviewBgwriter scans the bgwr/ckpt aggregate via SelectStatBgwriterQuery and fills the
// timed/req (absolute), write/sync ms (delta) and maxwritten fields. Column positions differ between
// PG14-16, 17 and 18, so values are mapped by column name rather than ordinal.
func collectOverviewBgwriter(db *postgres.DB, version int, s *PgstatOverview, prev PgstatOverview) {
	q, _, _ := query.SelectStatBgwriterQuery(version)

	rows, err := db.Query(q)
	if err != nil {
		return
	}
	defer rows.Close()

	descs := rows.FieldDescriptions()
	ncols := len(descs)
	if !rows.Next() {
		return
	}

	values := make([]sql.NullString, ncols)
	pointers := make([]any, ncols)
	for i := range pointers {
		pointers[i] = &values[i]
	}
	if err := rows.Scan(pointers...); err != nil {
		return
	}

	byName := make(map[string]string, ncols)
	for i, d := range descs {
		byName[string(d.Name)] = values[i].String
	}

	s.CkptTimed = parseInt64(byName["ckpt_timed"])
	s.CkptReq = parseInt64(byName["ckpt_req"])
	s.CkptWriteMs = parseFloat64(byName["ckpt_write,ms"])
	s.CkptSyncMs = parseFloat64(byName["ckpt_sync,ms"])
	s.MaxWritten = parseInt64(byName["maxwritten"])

	if s.HasPrev {
		s.CkptWriteMsDelta = s.CkptWriteMs - prev.CkptWriteMs
		s.CkptSyncMsDelta = s.CkptSyncMs - prev.CkptSyncMs
		s.MaxWrittenDelta = s.MaxWritten - prev.MaxWritten
	}
}

// cacheHitRatio computes the per-interval cache hit ratio as a percentage [0,100] from the deltas
// of blks_hit and blks_read. The second return value is false when there was no I/O in the interval
// (Δ(hit+read) <= 0): the ratio is then n/a, never NaN or a stale cumulative value.
func cacheHitRatio(dHit, dRead int64) (float64, bool) {
	denom := dHit + dRead
	if denom <= 0 {
		return 0, false
	}
	return float64(dHit) / float64(denom) * 100, true
}

// parseInt64 parses an integer string, returning 0 on any error (the field is treated as absent).
func parseInt64(s string) int64 {
	v, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	if err != nil {
		return 0
	}
	return v
}

// parseFloat64 parses a float string, returning 0 on any error.
func parseFloat64(s string) float64 {
	v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return 0
	}
	return v
}

// PostgresProperties is the container for details about Postgres
type PostgresProperties struct {
	VersionNum                      int     // Numeric representation of Postgres version, e.g. XXYYZZ
	Version                         string  // String representation of Postgres version, e.g. X.Y.Z
	StartTime                       float64 // Postgres start time
	Recovery                        string  // Recovery state
	GucTrackCommitTimestamp         string  // value of track_commit_timestamp GUC
	GucAVMaxWorkers                 int     // value of autovacuum_max_workers GUC
	GucMaxConnections               int     // value of max_connections GUC
	GucMaxPrepXacts                 int     // value of max_prepared_transactions GUC
	GucSharedPreLibraries           string  // value of shared_preload_libraries GUC
	GucMaxWorkerProcesses           int     // value of max_worker_processes GUC
	GucMaxLogicalReplicationWorkers int     // value of max_logical_replication_workers GUC
	GucMaxParallelWorkers           int     // value of max_parallel_workers GUC
	GucWalSegmentSize               int64   // value of wal_segment_size GUC, in bytes
	DataDirectory                   string  // value of data_directory GUC
	ExtPGSSSchema                   string  // Schema where 'pg_stat_statements' extension installed (empty if not installed)
	SchemaPgcenterAvail             bool    // is 'pgcenter' schema installed?
	SysTicks                        float64 // ad-hoc implementation of GET_CLK for cases when Postgres is remote
}

// GetPostgresProperties queries necessary properties from Postgres about it.
func GetPostgresProperties(db *postgres.DB) (PostgresProperties, error) {
	props := PostgresProperties{}
	err := db.QueryRow(query.SelectCommonProperties).Scan(
		&props.Version,
		&props.VersionNum,
		&props.GucTrackCommitTimestamp,
		&props.GucMaxConnections,
		&props.GucAVMaxWorkers,
		&props.GucSharedPreLibraries,
		&props.Recovery,
		&props.StartTime,
		&props.GucMaxWorkerProcesses,
		&props.GucMaxLogicalReplicationWorkers,
		&props.GucMaxParallelWorkers,
		&props.GucWalSegmentSize,
		&props.DataDirectory,
	)
	if err != nil {
		return PostgresProperties{}, err
	}

	// Is pg_stat_statement available?
	if strings.Contains(props.GucSharedPreLibraries, "pg_stat_statements") {
		props.ExtPGSSSchema = extensionSchema(db, "pg_stat_statements")
	}

	// In case of remote Postgres we should to know remote CLK_TCK
	if !db.Local {
		if isSchemaExists(db, "pgcenter") {
			props.SchemaPgcenterAvail = true
			err := db.QueryRow(query.SelectRemoteProcSysTicks).Scan(&props.SysTicks)
			if err != nil {
				return PostgresProperties{}, err
			}
		}
	}

	return props, nil
}

// PGresult is the container for basic Postgres stats collected from pg_stat_* views
type PGresult struct {
	Values [][]sql.NullString /* values */
	Cols   []string           /* list of columns' names */
	Ncols  int                /* numbers of columns in Result */
	Nrows  int                /* number of rows in Result */
	Valid  bool               /* Used for result invalidations, on context switching for example */
}

// NewPGresultQuery creates PGresult using passed database connection and query.
func NewPGresultQuery(db *postgres.DB, query string) (PGresult, error) {
	if query == "" {
		return PGresult{}, fmt.Errorf("no query defined")
	}

	rows, err := db.Query(query)
	if err != nil {
		return PGresult{}, err
	}

	var (
		descs = rows.FieldDescriptions()
		ncols = len(descs)
		nrows int
	)

	// Storage used for data extracted from rows.
	// Scan operation supports only slice of interfaces, 'pointers' slice is the intermediate store where all values written.
	// Next values from 'pointers' associated with type-strict slice - 'values'. When Scan is writing to the 'pointers' it
	// also writing to the 'values' under the hood. When all pointers/values have been scanned, put them into 'rowsStore'.
	// Finally we get queryResult iterable store with data and information about stored rows, columns and columns names.
	var rowsStore = make([][]sql.NullString, 0, 10)

	for rows.Next() {
		pointers := make([]any, ncols)
		values := make([]sql.NullString, ncols)

		for i := range pointers {
			pointers[i] = &values[i]
		}

		err = rows.Scan(pointers...)
		if err != nil {
			// log.Warnf("skip collecting stats: %w", err) // TODO: add error handling and notification
			continue
		}
		rowsStore = append(rowsStore, values)
		nrows++
	}

	rows.Close()

	// Convert pgproto3.FieldDescription into string.
	colnames := make([]string, ncols)
	for i, d := range descs {
		colnames[i] = string(d.Name)
	}

	res := PGresult{
		Nrows:  nrows,
		Ncols:  ncols,
		Cols:   colnames,
		Values: rowsStore,
		Valid:  true,
	}

	err = res.validate()
	if err != nil {
		return PGresult{}, err
	}

	return res, nil
}

// NewPGresultFile creates PGresult using reader interface.
func NewPGresultFile(r io.Reader, bufsz int64) (PGresult, error) {
	data := make([]byte, bufsz)

	if _, err := io.ReadFull(r, data); err != nil {
		return PGresult{}, err
	}

	// initialize an empty struct and unmarshal data from the buffer
	res := PGresult{}
	err := json.Unmarshal(data, &res)
	if err != nil {
		return PGresult{}, err
	}

	err = res.validate()
	if err != nil {
		return PGresult{}, err
	}

	return res, nil
}

// validate validates content of PGresult
func (r *PGresult) validate() error {
	// Check that number or values in rows are equal to number of columns names.
	for _, row := range r.Values {
		if len(row) != len(r.Cols) {
			return fmt.Errorf("invalid number of values in row")
		}
	}

	// Check number of rows is the same as declared
	if r.Nrows != len(r.Values) {
		return fmt.Errorf("invalid number of rows and values")
	}

	return nil
}

// Compare is public wrapper over calculateDelta.
func Compare(curr, prev PGresult, itv int, interval [2]int, skey int, desc bool, ukey int) (PGresult, error) {
	return calculateDelta(curr, prev, itv, interval, skey, desc, ukey)
}

// calculateDelta compares two PGresult structs and returns ordered delta PGresult.
func calculateDelta(curr, prev PGresult, itv int, interval [2]int, skey int, desc bool, ukey int) (PGresult, error) {
	// Make prev snapshot using current snap, at startup or at context switching
	if !prev.Valid {
		return curr, nil
	}

	var delta PGresult
	var err error

	// Diff previous and current stats snapshot
	if interval != [2]int{0, 0} {
		delta, err = diff(curr, prev, itv, interval, ukey)
		if err != nil {
			return PGresult{}, fmt.Errorf("diff failed: %w", err)
		}
	} else {
		delta = curr
	}

	delta.sort(skey, desc)

	return delta, nil
}

// diff compares two PGresult values and produces new differential PGresult.
func diff(curr PGresult, prev PGresult, itv int, interval [2]int, ukey int) (PGresult, error) {
	var diff PGresult
	var found bool

	diff.Values = make([][]sql.NullString, curr.Nrows)
	diff.Cols = curr.Cols
	diff.Ncols = len(curr.Cols)
	diff.Nrows = curr.Nrows

	// Take every row from 'current' snapshot and check its existing in 'previous' snapshot. If row exists in both snapshots
	// make diff between them. If target row is not found in 'previous' snapshot, no diff needed, hence append this row
	// as-is into 'result' snapshot.
	// Thus in the end, all rows that aren't exist in the 'current' snapshot, but exist in 'previous', will be skipped.
	for i, cv := range curr.Values {
		// Allocate container for target row and reset 'found' flag
		diff.Values[i] = make([]sql.NullString, curr.Ncols)
		found = false

		for j, pv := range prev.Values {
			if cv[ukey].String != pv[ukey].String {
				// Row is not exist in previous snapshot
				continue
			}

			found = true

			// Do diff
			for l := 0; l < curr.Ncols; l++ {
				if l < interval[0] || l > interval[1] {
					diff.Values[i][l].String = curr.Values[i][l].String // don't diff, copy value as-is
					diff.Values[i][l].Valid = curr.Values[i][l].Valid
				} else {
					// Values with dots or in scientific notation consider as floats and integer otherwise.
					v, err := diffPair(curr.Values[i][l].String, prev.Values[j][l].String, itv)
					if err != nil {
						return diff, err
					}
					diff.Values[i][l].String = v
					diff.Values[i][l].Valid = true
				}
			}
			break // Go to searching next row from current snapshot
		}

		// End of the searching in 'previous' snapshot, if we reached here it means row not found and it simply should be added as is.
		if !found {
			for l := 0; l < curr.Ncols; l++ {
				diff.Values[i][l].String = curr.Values[i][l].String // don't diff, copy value as-is
				diff.Values[i][l].Valid = curr.Values[i][l].Valid
			}
		}
	}

	diff.Valid = true
	return diff, nil
}

// sort performs sorting of PGresult using order key and order.
func (r *PGresult) sort(key int, desc bool) {
	if r.Nrows == 0 {
		return /* nothing to sort */
	}

	sample := r.Values[0][key].String

	if _, err := strconv.ParseFloat(sample, 64); err == nil {
		// numeric sort
		sort.SliceStable(r.Values, func(i, j int) bool {
			// TODO: handle errors
			l, _ := strconv.ParseFloat(r.Values[i][key].String, 64)
			m, _ := strconv.ParseFloat(r.Values[j][key].String, 64)
			if desc {
				return l > m /* desc order: 10 -> 0 */
			}
			return l < m /* asc order: 0 -> 10 */
		})
	} else if _, err := parseDuration(sample); err == nil {
		// duration sort: handles "HH:MM:SS" and "N days HH:MM:SS" so that values
		// with 3+ digit hours (e.g. "791:04:45") sort correctly instead of as strings.
		sort.SliceStable(r.Values, func(i, j int) bool {
			li, _ := parseDuration(r.Values[i][key].String)
			lj, _ := parseDuration(r.Values[j][key].String)
			if desc {
				return li > lj
			}
			return li < lj
		})
	} else {
		// string sort (fallback)
		sort.SliceStable(r.Values, func(i, j int) bool {
			if desc {
				return r.Values[i][key].String > r.Values[j][key].String /* desc order: 'z' -> 'a' */
			}
			return r.Values[i][key].String < r.Values[j][key].String /* asc order: 'a' -> 'z' */
		})
	}
}

// parseDuration parses a PostgreSQL interval string into total seconds.
// Supported formats:
//   - "HH:MM:SS" where HH may have any number of digits (e.g. "791:04:45")
//   - "N days HH:MM:SS" / "N day HH:MM:SS" (PostgreSQL interval text output)
func parseDuration(s string) (int64, error) {
	var days, hours, minutes, seconds int64

	if idx := strings.Index(s, " day"); idx != -1 {
		dayStr := strings.TrimSpace(s[:idx])
		var err error
		days, err = strconv.ParseInt(dayStr, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("parse days in %q: %w", s, err)
		}
		rest := strings.TrimSpace(strings.TrimPrefix(s[idx+len(" day"):], "s"))
		if rest == "" {
			return days * 86400, nil
		}
		s = rest
	}

	parts := strings.SplitN(s, ":", 3)
	if len(parts) != 3 {
		return 0, fmt.Errorf("not a duration %q: expected HH:MM:SS", s)
	}
	var err error
	hours, err = strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse hours in %q: %w", s, err)
	}
	minutes, err = strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse minutes in %q: %w", s, err)
	}
	// seconds may carry fractional part; truncate to integer
	secStr := strings.SplitN(parts[2], ".", 2)[0]
	seconds, err = strconv.ParseInt(secStr, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse seconds in %q: %w", s, err)
	}
	return days*86400 + hours*3600 + minutes*60 + seconds, nil
}

// diffPair produces a delta of two string values.
func diffPair(curr, prev string, itv int) (string, error) {
	if strings.Contains(prev, ".") || strings.Contains(prev, "e") ||
		strings.Contains(curr, ".") || strings.Contains(curr, "e") {
		cv, pv, err := parsePairFloat(curr, prev)
		if err != nil {
			return "", err
		}
		return strconv.FormatFloat((cv-pv)/float64(itv), 'f', 2, 64), nil
	}

	cv, pv, err := parsePairInt(curr, prev)
	if err != nil {
		return "", err
	}
	return strconv.FormatInt((cv-pv)/int64(itv), 10), nil
}

// parsePairFloat parses pair of string and return two float64 values.
func parsePairFloat(curr, prev string) (float64, float64, error) {
	cv, err := strconv.ParseFloat(curr, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("convert '%s' to float64 failed: %w", curr, err)
	}
	pv, err := strconv.ParseFloat(prev, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("convert '%s' to float64 failed: %w", prev, err)
	}

	return cv, pv, nil
}

// parsePairInt parses pair of string and return two int64 values.
func parsePairInt(curr, prev string) (int64, int64, error) {
	cv, err := strconv.ParseInt(curr, 10, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("convert '%s' to int failed: %w", curr, err)
	}
	pv, err := strconv.ParseInt(prev, 10, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("convert '%s' to int failed: %w", curr, err)
	}

	return cv, pv, nil
}

// Fprint prints content of PGresult container to buffer.
func (r *PGresult) Fprint(buf *bytes.Buffer) error {
	// do simple ad-hoc aligning for current PGresult, do align using the longest value in the column
	widthMap := map[int]int{}
	var valuelen int
	for colnum := range r.Cols {
		for rownum := 0; rownum < len(r.Values); rownum++ {
			valuelen = len(r.Values[rownum][colnum].String)
			if valuelen > widthMap[colnum] {
				widthMap[colnum] = valuelen
			}
		}
	}

	/* print header */
	for colidx, colname := range r.Cols {
		_, err := fmt.Fprintf(buf, "%-*s", widthMap[colidx]+2, colname)
		if err != nil {
			return err
		}
	}
	_, err := fmt.Fprintf(buf, "\n\n")
	if err != nil {
		return err
	}

	/* print data to buffer */
	for colnum, rownum := 0, 0; rownum < r.Nrows; rownum, colnum = rownum+1, 0 {
		for range r.Cols {
			/* m[row][column] */
			_, err := fmt.Fprintf(buf, "%-*s", widthMap[colnum]+2, r.Values[rownum][colnum].String)
			if err != nil {
				return err
			}
			colnum++
		}
		_, err := fmt.Fprintf(buf, "\n")
		if err != nil {
			return err
		}
	}

	return nil
}

// extensionSchema returns schema where the requested extension is installed in the database. Return empty string if not found.
func extensionSchema(db *postgres.DB, name string) string {
	var schema string
	err := db.QueryRow(query.GetExtensionSchema, name).Scan(&schema)
	if err != nil {
		// TODO: enable when proper logging will be implemented
		// fmt.Println("failed to check extensions in pg_extension: ", err)
		schema = ""
	}

	return schema
}

// isSchemaExists returns 'true' if requested schema exists in the database, and 'false' if not.
func isSchemaExists(db *postgres.DB, name string) bool {
	var exists bool
	err := db.QueryRow(query.CheckSchemaExists, name).Scan(&exists)
	if err != nil {
		// TODO: enable when proper logging will be implemented
		// fmt.Println("failed to check schema in information_schema: ", err)
		exists = false
	}

	return exists
}
