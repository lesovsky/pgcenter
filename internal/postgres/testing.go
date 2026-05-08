package postgres

// NewTestConfig returns test config used for testing purposes.
func NewTestConfig() (Config, error) {
	return NewConfig("127.0.0.1", 21914, "postgres", "pgcenter_fixtures")
}

// NewTestConnect returns default test connection used for testing purposes.
func NewTestConnect() (*DB, error) {
	return NewTestConnectVersion(140000)
}

// NewTestConnectVersion connects to test Postgres of specific version.
// Returns an error if the requested version is not available in the test environment.
// Callers should use t.Skip() when this returns an error for EOL versions.
func NewTestConnectVersion(version int) (*DB, error) {
	ports := map[int]int{
		// active versions (available in pgcenter-testing:0.0.8+)
		170000: 21917,
		160000: 21916,
		150000: 21915,
		140000: 21914,
		// EOL versions kept for reference; connection will fail if not running
		130000: 21913,
		120000: 21912,
		110000: 21911,
		100000: 21910,
		90600:  21996,
		90500:  21995,
		90400:  21994,
	}

	port, ok := ports[version]
	if !ok {
		port = ports[140000]
	}

	config, err := NewConfig("127.0.0.1", port, "postgres", "pgcenter_fixtures")
	if err != nil {
		return nil, err
	}
	return Connect(config)
}
