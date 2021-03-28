package postgres

import "fmt"

// NewTestConfig returns test config used for testing purposes.
func NewTestConfig() (Config, error) {
	return NewConfig("127.0.0.1", 21913, "postgres", "pgcenter_fixtures")
}

// NewTestConnect returns default test connection used for testing purposes.
func NewTestConnect() (*DB, error) {
	return NewTestConnectVersion(130000)
}

// NewTestConnectVersion connects to test Postgres of specific version.
// Necessary Postgres instances have to be up and running on specified ports.
func NewTestConnectVersion(version int) (*DB, error) {
	if version < 90400 || version > 140000 {
		return nil, fmt.Errorf("unsupported version selected")
	}

	ports := map[int]int{
		130000: 21913,
		120000: 21912,
		110000: 21911,
		100000: 21910,
		90600:  21996,
		90500:  21995,
		90400:  21994,
	}

	config, err := NewConfig("127.0.0.1", ports[version], "postgres", "pgcenter_fixtures")
	if err != nil {
		return nil, err
	}
	return Connect(config)
}
