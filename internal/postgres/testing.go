package postgres

// TestConnect returns test connection used for testing purposes.
func NewTestConnect() (*DB, error) {
	config, err := NewConfig("127.0.0.1", 5432, "postgres", "pgcenter_test")
	if err != nil {
		return nil, err
	}

	return Connect(config)
}
