package postgres

import (
	"github.com/jackc/pgx/v4"
	"strconv"
	"strings"
)

// Conninfo describes connection settings to Postgres specified by user.
type DB struct {
	Config *pgx.ConnConfig // Config used for creating connection
	Conn   *pgx.Conn       // Connection object
	Local  bool            // is Postgres running on localhost?
}

// NewConninfo checks connection parameters passed by user, assembles connection string and creates
// new DB object with connection config.
func NewDB(host string, port int, user string, dbname string) (*DB, error) {
	var connStr string
	if host != "" {
		connStr = "host=" + host
	}
	if port > 0 {
		connStr = connStr + " port=" + strconv.Itoa(port)
	}
	if user != "" {
		connStr = connStr + " user=" + user
	}
	if dbname != "" {
		connStr = connStr + " dbname=" + dbname
	}

	connStr = strings.TrimSpace(connStr)

	// pgx.ParseConfig produces config for connecting to Postgres even from empty string.
	pgConfig, err := pgx.ParseConfig(connStr)
	if err != nil {
		return nil, err
	}

	return &DB{
		Config: pgConfig,
		Local:  strings.HasPrefix(host, "/"),
	}, nil
}
