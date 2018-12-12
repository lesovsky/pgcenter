// PostgreSQL related functions.

package utils

import (
	"database/sql"
	"fmt"
	"github.com/lib/pq"
	"golang.org/x/crypto/ssh/terminal"
	"net"
)

const (
	dbDriver = "postgres"

	errCodeInvalidPassword = "28P01"

	// Self-identification queries
	PQhostQuery   = "SELECT inet_server_addr() inet, current_setting('unix_socket_directories') unix;"
	PQportQuery   = "SELECT coalesce(inet_server_port(),5432)"
	PQuserQuery   = "SELECT current_user"
	PQdbQuery     = "SELECT current_database()"
	PQstatusQuery = "SELECT 1"

	// Session parameters used by pgCenter at connection start
	LogMinDurationQuery   = "SET log_min_duration_statement TO 10000"
	StatementTimeoutQuery = "SET statement_timeout TO 5000"
	LockTimeoutQuery      = "SET lock_timeout TO 2000"
	DeadlockTimeoutQuery  = "SET deadlock_timeout TO 1000"
)

// Assembles libpq connection string, connect to Postgres and returns 'connection' object
func CreateConn(c *Conninfo) (conn *sql.DB, err error) {
	// Assemble libpq-style connection string
	connstr := assembleConnstr(c)
	// Connect to Postgres using assembled connection string
	if conn, err = PQconnectdb(c, connstr); err != nil {
		return nil, err
	}
	// Fill empty settings by normal values
	if err = replaceEmptySettings(c, conn); err != nil {
		return nil, err
	}
	// Determine whether Postgres is local or not.
	checkLocality(c)
	// Set session's safe settings.
	setSafeSession(conn)

	return conn, nil
}

// Build connection string using connection settings
func assembleConnstr(c *Conninfo) string {
	s := "application_name=pgcenter "
	if c.Host != "" {
		s = fmt.Sprintf("%s host=%s ", s, c.Host)
	}
	if c.Port != 0 {
		s = fmt.Sprintf("%s port=%d ", s, c.Port)
	}
	if c.User != "" {
		s = fmt.Sprintf("%s user=%s ", s, c.User)
	}
	if c.Dbname != "" {
		s = fmt.Sprintf("%s dbname=%s ", s, c.Dbname)
	}
	return s
}

// Connect to Postgres, ask password if required.
func PQconnectdb(c *Conninfo, connstr string) (conn *sql.DB, err error) {
	conn, err = sql.Open(dbDriver, connstr)
	if err = PQstatus(conn); err != nil {
		if pqerr, ok := err.(*pq.Error); ok {
			switch {
			// Password required -- ask user and retry connection
			case pqerr.Code == errCodeInvalidPassword:
				fmt.Printf("Password for user %s: ", c.User)
				bytePassword, err := terminal.ReadPassword(0)
				connstr = fmt.Sprintf("%s password=%s ", connstr, string(bytePassword))
				conn, err = sql.Open(dbDriver, connstr)
				if err = PQstatus(conn); err != nil {
					return nil, err
				}
			}
		}
	}
	return conn, nil
}

// Fill empty connection settings by normal values.
func replaceEmptySettings(c *Conninfo, conn *sql.DB) (err error) {
	if c.Host == "" {
		c.Host, err = PQhost(conn)
		if err != nil {
			return err
		}
	}
	if c.Port == 0 {
		c.Port, _ = PQport(conn)
	}
	if c.User == "" {
		c.User, _ = PQuser(conn)
	}
	if c.Dbname == "" {
		c.Dbname, _ = PQdb(conn)
	}
	return nil
}

// Gets list of local network addresses and compare address specified for connection with addresses in the list.
// By default or in case of errors, assume that there is a remote Postgres
func checkLocality(c *Conninfo) {
	aa, err := net.InterfaceAddrs()
	if err != nil {
		fmt.Println("ERROR: failed to check whether Postgres is local or remote")
		c.ConnLocal = false // Suppose this is a remote Postgres
	} else {
		for _, a := range aa {
			addr, _, err := net.ParseCIDR(a.String())
			if err != nil {
				continue // Skip this address
			}
			if c.Host == addr.String() || c.Host[0] == byte('/') {
				c.ConnLocal = true // An address from the list is the same as specified address (or it's a UNIX socket)
				break
			}
		}
	}
}

// Set session's safe settings. It's possible to pass these parameters via connection string at startup, but they're not logged then.
func setSafeSession(conn *sql.DB) {
	for _, query := range []string{StatementTimeoutQuery, LockTimeoutQuery, DeadlockTimeoutQuery, LogMinDurationQuery} {
		_, err := conn.Exec(query)
		// Trying to SET superuser-only parameters without SUPERUSER privileges will lead to error, but it's not critical.
		// Notice about occured error, clear it and go ahead.
		if err, ok := err.(*pq.Error); ok {
			fmt.Printf("%s: %s\nSTATEMENT: %s\n", err.Severity, err.Message, query)
		}
		err = nil
	}
}

// Returns endpoint (hostname or UNIX-socket) to which pgCenter is connected
func PQhost(c *sql.DB) (_ string, err error) {
	// Don't use query with coalesce(inet, unix), because 'inet' and 'unix' are the different types,
	// at casting 'inet' to text, netmask is added to the final address and the netmask is unnecessary.
	var i, u sql.NullString
	err = c.QueryRow(PQhostQuery).Scan(&i, &u)
	if err != nil {
		return "", err
	}
	if i.String != "" {
		return i.String, err
	}
	return u.String, err
}

// Returns port number to which pgCenter is connected
func PQport(c *sql.DB) (i int, err error) {
	err = c.QueryRow(PQportQuery).Scan(&i)
	return i, err
}

// Returns username which is used by pgCenter
func PQuser(c *sql.DB) (s string, err error) {
	err = c.QueryRow(PQuserQuery).Scan(&s)
	return s, err
}

// Returns database name to which pgCenter is connected
func PQdb(c *sql.DB) (s string, err error) {
	err = c.QueryRow(PQdbQuery).Scan(&s)
	return s, err
}

// Returns connections status
func PQstatus(c *sql.DB) error {
	var s string
	return c.QueryRow(PQstatusQuery).Scan(&s)
}
