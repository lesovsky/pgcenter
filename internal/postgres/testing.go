package postgres

import (
	"fmt"
	"regexp"
)

// testRoleNameRE constrains the role name SetupTestRole interpolates into its statements. The name
// is an SQL identifier, so it cannot travel as a $1 placeholder; this turns "callers pass literal
// constants" from a comment into an enforced invariant. Lowercase-only is not a restriction: the
// identifier positions are unquoted, so PostgreSQL down-folds the name anyway.
var testRoleNameRE = regexp.MustCompile(`^[a-z_][a-z0-9_]*$`)

// NewTestConfig returns test config used for testing purposes.
func NewTestConfig() (Config, error) {
	return NewConfig("127.0.0.1", 21917, "postgres", "pgcenter_fixtures")
}

// NewTestConnect returns default test connection used for testing purposes.
func NewTestConnect() (*DB, error) {
	return NewTestConnectVersion(170000)
}

// NewTestConnectVersion connects to test Postgres of specific version.
// Returns an error if the requested version is not available in the test environment.
// Callers should use t.Skip() when this returns an error for EOL versions.
func NewTestConnectVersion(version int) (*DB, error) {
	ports := map[int]int{
		// active versions (available in pgcenter-testing:0.0.9+)
		190000: 21919,
		180000: 21918,
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
		return nil, fmt.Errorf("postgres version %d has no test cluster port mapping", version)
	}

	config, err := NewConfig("127.0.0.1", port, "postgres", "pgcenter_fixtures")
	if err != nil {
		return nil, err
	}
	return Connect(config)
}

// SetupTestRole ensures a test role exists on the connected cluster and switches the session to it.
// Creation is idempotent (DO block guarded on pg_roles): the role is created NOLOGIN and
// non-superuser when missing, and granted pg_monitor when pgMonitor is true. The caller is
// responsible for RESET ROLE, normally in a defer immediately after a successful call.
//
// It returns an error rather than taking *testing.T on purpose: this file carries no build tag and
// is compiled into the released pgcenter binary, so it must not import the testing package.
//
// The role name is an SQL identifier, not a value, so it cannot travel as a $1 placeholder and is
// interpolated instead. Callers must pass literal constants - never user input.
func SetupTestRole(db *DB, name string, pgMonitor bool) error {
	if !testRoleNameRE.MatchString(name) {
		return fmt.Errorf("invalid test role name %q", name)
	}

	create := fmt.Sprintf(
		"DO $$ BEGIN IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = '%s') "+
			"THEN CREATE ROLE %s NOLOGIN NOSUPERUSER; END IF; END $$", name, name,
	)
	if _, err := db.Exec(create); err != nil {
		return fmt.Errorf("create role %s failed: %w", name, err)
	}

	// GRANT is naturally idempotent, unlike bare CREATE ROLE. A role that must hold nothing gets no
	// GRANT at all rather than a REVOKE, so two roles created by neighbouring tests never interfere.
	if pgMonitor {
		if _, err := db.Exec(fmt.Sprintf("GRANT pg_monitor TO %s", name)); err != nil {
			return fmt.Errorf("grant pg_monitor to %s failed: %w", name, err)
		}
	}

	if _, err := db.Exec(fmt.Sprintf("SET ROLE %s", name)); err != nil {
		return fmt.Errorf("set role %s failed: %w", name, err)
	}

	return nil
}
