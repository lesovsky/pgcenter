package top

import (
	"database/sql"
	"fmt"
	"github.com/lesovsky/pgcenter/internal/postgres"
	"github.com/lesovsky/pgcenter/internal/query"
	"strings"
)

// doReload performs reload of Postgres service by executing pg_reload_conf().
func doReload(answer string, db *postgres.DB) string {
	answer = strings.TrimPrefix(answer, dialogPrompts[dialogPgReload])
	answer = strings.TrimSuffix(answer, "\n")

	var message string
	switch answer {
	case "y":
		var status sql.NullBool

		err := db.QueryRow(query.ExecReloadConf).Scan(&status)
		if err != nil {
			message = fmt.Sprintf("Reload: failed, %s", err.Error())
			return message
		}

		if status.Bool {
			message = "Reload: successful"
		} else {
			message = "Reload: no error, got NULL response"
		}
	case "n":
		message = "Reload: do nothing, canceled"
	default:
		message = "Reload: do nothing, invalid input"
	}

	return message
}
