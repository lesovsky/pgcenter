package postgres

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewTestConnectVersion_unmappedVersion(t *testing.T) {
	// An unmapped version must be refused outright. The previous behaviour — falling back to the
	// oldest active cluster — made a forgotten port entry invisible: every subtest for the new
	// version passed while exercising a completely different server.
	// This case must hold with no PostgreSQL running at all, so the error has to be returned
	// before any connection attempt.
	db, err := NewTestConnectVersion(200000)
	if db != nil {
		db.Close()
	}
	require.Error(t, err, "an unmapped version must be refused, not silently served by another cluster")
	assert.Nil(t, db)
	assert.Contains(t, err.Error(), "200000")
}

func TestNewTestConnectVersion_mappedVersionIsAccepted(t *testing.T) {
	// A mapped version must get past the lookup. Whether the connection itself succeeds depends on
	// the cluster running, so only the lookup rejection is asserted here.
	db, err := NewTestConnectVersion(190000)
	if err != nil {
		assert.NotContains(t, err.Error(), "no test cluster port mapping")
		t.Skipf("postgres 190000 not available in test environment: %v", err)
	}
	assert.NotNil(t, db)
	db.Close()
}
