package postgres

// TestConnect returns test connection used for testing purposes.
func NewTestConnect() (*DB, error) {
	config, err := NewConfig("127.0.0.1", 5432, "postgres", "pgcenter_fixtures")
	if err != nil {
		return nil, err
	}
	return Connect(config)
}

// NewTestConnectVersion connects to test Postgres.
// Necessary Postgres instances have to be up and running on specified ports.
func NewTestConnectVersion(version int) (*DB, error) {
	ports := map[int]int{
		130000: 21913,
		120000: 21912,
		110000: 21911,
		100000: 21910,
		90600:  21996,
		90500:  21995,
	}

	config, err := NewConfig("127.0.0.1", ports[version], "postgres", "pgcenter_fixtures")
	if err != nil {
		return nil, err
	}
	return Connect(config)
}
