package config

import (
	"context"
	"fmt"
	"github.com/lesovsky/pgcenter/internal/postgres"
	"github.com/lesovsky/pgcenter/internal/query"
)

const (
	// Flags which tells to pgcenter install or uninstall schema.
	Install = iota
	Uninstall
)

// RunMain is the main entry point for 'pgcenter config' command.
func RunMain(dbConfig postgres.Config, mode int) error {
	db, err := postgres.Connect(dbConfig)
	if err != nil {
		return err
	}
	defer db.Close()

	switch mode {
	case Install:
		if err := doInstall(db); err != nil {
			return err
		}
		fmt.Printf("pgCenter schema installed.")
	case Uninstall:
		if err := doUninstall(db); err != nil {
			return err
		}
		fmt.Printf("pgCenter schema uninstalled.")
	default:
		// should not be here, but who knows...
		fmt.Printf("do nothing, unknown mode selected.")
		return fmt.Errorf("unknown mode selected")
	}

	return nil
}

// doInstall begins transaction and create pgcenter schema, functions and views.
func doInstall(db *postgres.DB) error {
	queries := []string{
		query.StatSchemaCreateSchema,
		query.StatSchemaCreateFunction1,
		query.StatSchemaCreateFunction2,
		query.StatSchemaCreateFunction3,
		query.StatSchemaCreateView1,
		query.StatSchemaCreateView2,
		query.StatSchemaCreateView3,
		query.StatSchemaCreateView4,
		query.StatSchemaCreateView5,
		query.StatSchemaCreateView6,
	}

	tx, err := db.Conn.Begin(context.Background())
	if err != nil {
		return err
	}

	for _, q := range queries {
		_, err := tx.Exec(context.Background(), q)
		if err != nil {
			_ = tx.Rollback(context.Background())
			return err
		}
	}

	err = tx.Commit(context.Background())
	if err != nil {
		return err
	}

	return nil
}

// doUninstall drops pgcenter stats schema.
func doUninstall(db *postgres.DB) error {
	_, err := db.Exec(query.StatSchemaDropSchema)
	if err != nil {
		return err
	}

	return nil
}
