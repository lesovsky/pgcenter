package postgres

import (
	"context"
	"fmt"
	"github.com/jackc/pgconn"
	"github.com/jackc/pgx/v4"
	"os"
	"strconv"
	"strings"
)

// Config contains configuration suitable for used database driver.
type Config struct {
	Config *pgx.ConnConfig
}

// DB describes connection settings to Postgres specified by user.
type DB struct {
	Config Config
	Conn   *pgx.Conn
	Local  bool // is Postgres running on localhost?
}

// NewConfig checks connection parameters passed by user, assembles connection string and creates config.
func NewConfig(host string, port int, user string, dbname string) (Config, error) {
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
		return Config{}, err
	}

	// use PreferSimpleProtocol disables implicit prepared statement usage and enable compatibility with Pgbouncer.
	pgConfig.PreferSimpleProtocol = true

	// process PGOPTIONS explicitly, because used jackc/pgx driver supports a limited set of libpq environment variables.
	if options := os.Getenv("PGOPTIONS"); options != "" {
		pgConfig.RuntimeParams["options"] = options
	}

	return Config{
		Config: pgConfig,
	}, nil
}

// Connect connects to Postgres using provided config and returns DB object.
func Connect(config Config) (*DB, error) {
	conn, err := pgx.ConnectConfig(context.TODO(), config.Config)
	if err != nil {
		return nil, err
	}

	return &DB{
		Config: config,
		Conn:   conn,
		Local:  strings.HasPrefix(config.Config.Host, "/"),
	}, nil
}

// Reconnect reconnects to Postgres using existing config and swaps failed DB connection.
func Reconnect(db *DB) error {
	newdb, err := Connect(db.Config)
	if err != nil {
		return err
	}

	*db = *newdb

	return nil
}

// Exec is a wrapper over pgx.Exec.
func (db *DB) Exec(query string, args ...interface{}) (pgconn.CommandTag, error) {
	return db.Conn.Exec(context.TODO(), query, args...)
}

// QueryRow is a wrapper over pgx.QueryRow.
func (db *DB) QueryRow(query string, args ...interface{}) pgx.Row {
	return db.Conn.QueryRow(context.TODO(), query, args...)
}

// Query is a wrapper over pgx.Query.
func (db *DB) Query(query string, args ...interface{}) (pgx.Rows, error) {
	return db.Conn.Query(context.TODO(), query, args...)
}

// Close closes connection to Postgres.
func (db *DB) Close() {
	if err := db.Conn.Close(context.TODO()); err != nil {
		fmt.Printf("close connection failed: %s; ignore", err)
	}
}

func (db *DB) PQstatus() error {
	var s string
	return db.QueryRow("SELECT 1").Scan(&s)
}
