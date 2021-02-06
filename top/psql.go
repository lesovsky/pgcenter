package top

import (
	"fmt"
	"github.com/jroimartin/gocui"
	"github.com/lesovsky/pgcenter/internal/postgres"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
)

// runPsql starts psql session to the current connected database.
func runPsql(db *postgres.DB, uiExit chan int) func(g *gocui.Gui, _ *gocui.View) error {
	return func(g *gocui.Gui, _ *gocui.View) error {
		// Ignore interrupts in pgCenter, because Ctrl+C in psql interrupts pgCenter.
		signal.Ignore(os.Interrupt)

		// exit from UI and stats loop... will restore it after psql is closed.
		uiExit <- 1
		g.Close()

		cfg := db.Config.Config

		cmd := exec.Command(
			"psql",
			"-h", cfg.Host,
			"-p", strconv.Itoa(int(cfg.Port)),
			"-U", cfg.User,
			"-d", cfg.Database,
		) // #nosec G204

		cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr

		if err := cmd.Run(); err != nil {
			return fmt.Errorf("run psql failed: %s", err)
		}

		return nil
	}
}
