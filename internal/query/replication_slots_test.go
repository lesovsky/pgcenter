package query

import (
	"fmt"
	"github.com/lesovsky/pgcenter/internal/postgres"
	"github.com/stretchr/testify/assert"
	"testing"
)

func Test_SelectStatReplicationSlotsQuery(t *testing.T) {
	testcases := []struct {
		version       int
		wantNcols     int
		wantDiffIntvl [2]int
	}{
		{version: 140000, wantNcols: 15, wantDiffIntvl: [2]int{6, 13}},
		{version: 150000, wantNcols: 15, wantDiffIntvl: [2]int{6, 13}},
		{version: 160000, wantNcols: 15, wantDiffIntvl: [2]int{6, 13}},
		{version: 170000, wantNcols: 15, wantDiffIntvl: [2]int{6, 13}},
		{version: 180000, wantNcols: 15, wantDiffIntvl: [2]int{6, 13}},
	}

	for _, tc := range testcases {
		t.Run(fmt.Sprintf("version/%d", tc.version), func(t *testing.T) {
			_, gotNcols, gotDiffIntvl := SelectStatReplicationSlotsQuery(tc.version)
			assert.Equal(t, tc.wantNcols, gotNcols)
			assert.Equal(t, tc.wantDiffIntvl, gotDiffIntvl)
		})
	}
}

// Test_StatReplicationSlotsQueries tests query execution against all supported Postgres versions.
func Test_StatReplicationSlotsQueries(t *testing.T) {
	versions := []int{140000, 150000, 160000, 170000, 180000}

	for _, version := range versions {
		t.Run(fmt.Sprintf("pg_stat_replication_slots/%d", version), func(t *testing.T) {
			tmpl, wantNcols, _ := SelectStatReplicationSlotsQuery(version)

			opts := NewOptions(version, "f", "off", 256, "public")
			q, err := Format(tmpl, opts)
			assert.NoError(t, err)

			conn, err := postgres.NewTestConnectVersion(version)
			if err != nil {
				t.Skipf("postgres %d not available in test environment", version)
			}
			defer conn.Close()

			rows, err := conn.Query(q)
			assert.NoError(t, err)

			// Assert the live column count matches the declared Ncols. The hybrid
			// pg_replication_slots LEFT JOIN pg_stat_replication_slots subset is stable across
			// PG 14-18, so this gate verifies no schema divergence broke the 15-column shape.
			assert.Len(t, rows.FieldDescriptions(), wantNcols)
			rows.Close()
			assert.NoError(t, rows.Err())
		})
	}
}

// dropReplicationSlotIfExists drops the named replication slot but only when it is present in
// pg_replication_slots. The guard makes both setup (drop-if-exists before create) and teardown
// (defer) idempotent: pg_drop_replication_slot on a missing slot errors, so a prior SIGKILL'ed run
// or an early t.Skipf must not turn cleanup into a failure. The slot name is passed as a $1 bind
// parameter (mirroring pg_terminate_backend($1) in postgres_test.go) rather than concatenated.
func dropReplicationSlotIfExists(t *testing.T, conn *postgres.DB, slotName string) {
	t.Helper()

	var exists bool
	err := conn.QueryRow(
		"SELECT EXISTS (SELECT 1 FROM pg_replication_slots WHERE slot_name = $1)", slotName,
	).Scan(&exists)
	assert.NoError(t, err)
	if !exists {
		return
	}

	_, err = conn.Exec("SELECT pg_drop_replication_slot($1)", slotName)
	assert.NoError(t, err)
}

// findSlotRow runs the formatted replslots query and returns the row whose slot_name (column 0)
// matches slotName. The boolean reports whether the row was found. Values are read via
// rows.Values() so callers can inspect mixed-typed columns (bigint, text) by index.
func findSlotRow(t *testing.T, conn *postgres.DB, q string, slotName string) ([]any, bool) {
	t.Helper()

	rows, err := conn.Query(q)
	assert.NoError(t, err)
	defer rows.Close()

	for rows.Next() {
		values, err := rows.Values()
		assert.NoError(t, err)
		if name, ok := values[0].(string); ok && name == slotName {
			return values, true
		}
	}
	assert.NoError(t, rows.Err())

	return nil, false
}

// Test_StatReplicationSlotsQueries_PhysicalSlot is a tier-2 live test (wal_level=replica is enough).
// It creates a physical slot with immediate WAL reservation and asserts the hybrid query returns the
// slot with a non-NULL retained,KiB and the eight logical-decoding diff columns rendered as "0" - a
// physical slot is absent from pg_stat_replication_slots, so coalesce(...,0) in SQL must yield 0
// rather than NULL (Decision 2: an empty diffed column would abort the sample via ParseInt("")).
func Test_StatReplicationSlotsQueries_PhysicalSlot(t *testing.T) {
	const slotName = "pgcenter_test_phys"
	versions := []int{140000, 150000, 160000, 170000, 180000}

	for _, version := range versions {
		t.Run(fmt.Sprintf("pg_stat_replication_slots/%d", version), func(t *testing.T) {
			tmpl, _, _ := SelectStatReplicationSlotsQuery(version)

			opts := NewOptions(version, "f", "off", 256, "public")
			q, err := Format(tmpl, opts)
			assert.NoError(t, err)

			conn, err := postgres.NewTestConnectVersion(version)
			if err != nil {
				t.Skipf("postgres %d not available in test environment", version)
			}
			defer conn.Close()

			// Idempotent setup: drop a leftover slot from an interrupted run, then create the slot
			// with immediate WAL reservation (second arg true) so restart_lsn/retained,KiB are set.
			dropReplicationSlotIfExists(t, conn, slotName)
			_, err = conn.Exec("SELECT pg_create_physical_replication_slot($1, true)", slotName)
			assert.NoError(t, err)
			defer dropReplicationSlotIfExists(t, conn, slotName)

			values, found := findSlotRow(t, conn, q, slotName)
			assert.True(t, found, "physical slot %s must be present in result", slotName)
			if !found {
				return
			}

			// retained,KiB (col 4) must be non-NULL: the slot reserved WAL on creation.
			assert.NotNil(t, values[4])

			// The eight diffed logical-decoding counters (cols 6-13) must render as "0": the physical
			// slot is absent from pg_stat_replication_slots, so coalesce(...,0) replaces the LEFT JOIN
			// NULLs with 0.
			for col := 6; col <= 13; col++ {
				assert.Equal(t, "0", fmt.Sprint(values[col]), "diff column %d must render as 0", col)
			}
		})
	}
}

// Test_StatReplicationSlotsQueries_LogicalSlot is a tier-3 live test that needs wal_level=logical and
// the test_decoding plugin. It t.Skipf's gracefully when either is unavailable (Decision 5), so CI
// stays green on the older wal_level=replica image without coordinating with the 0.0.10 image push.
// It asserts the logical slot is present and the spill/stream diff columns exist in the result.
func Test_StatReplicationSlotsQueries_LogicalSlot(t *testing.T) {
	const slotName = "pgcenter_test_logical"
	versions := []int{140000, 150000, 160000, 170000, 180000}

	for _, version := range versions {
		t.Run(fmt.Sprintf("pg_stat_replication_slots/%d", version), func(t *testing.T) {
			tmpl, wantNcols, _ := SelectStatReplicationSlotsQuery(version)

			opts := NewOptions(version, "f", "off", 256, "public")
			q, err := Format(tmpl, opts)
			assert.NoError(t, err)

			conn, err := postgres.NewTestConnectVersion(version)
			if err != nil {
				t.Skipf("postgres %d not available in test environment", version)
			}
			defer conn.Close()

			var walLevel string
			err = conn.QueryRow("SHOW wal_level").Scan(&walLevel)
			assert.NoError(t, err)
			if walLevel != "logical" {
				t.Skipf("wal_level is %q, logical slots require wal_level=logical", walLevel)
			}

			// Idempotent setup: drop a leftover slot, then create the logical slot. Creation can fail
			// if the test_decoding plugin is unavailable - skip gracefully rather than fail.
			dropReplicationSlotIfExists(t, conn, slotName)
			_, err = conn.Exec("SELECT pg_create_logical_replication_slot($1, 'test_decoding')", slotName)
			if err != nil {
				t.Skipf("cannot create logical slot (test_decoding plugin unavailable?): %v", err)
			}
			defer dropReplicationSlotIfExists(t, conn, slotName)

			values, found := findSlotRow(t, conn, q, slotName)
			assert.True(t, found, "logical slot %s must be present in result", slotName)
			if !found {
				return
			}

			// The spill/stream diff columns (block [6,13]) must be present and, for a freshly created
			// logical slot that has not decoded anything yet, render as "0".
			assert.Len(t, values, wantNcols)
			for col := 6; col <= 13; col++ {
				assert.Equal(t, "0", fmt.Sprint(values[col]), "diff column %d must render as 0", col)
			}
		})
	}
}
