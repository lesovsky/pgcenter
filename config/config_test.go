package config

import (
	"github.com/lesovsky/pgcenter/internal/postgres"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestRunMain(t *testing.T) {
	config, err := postgres.NewTestConfig()
	assert.NoError(t, err)

	// run tests in dedicated database to avoid interfering with other test which depends on stats schema
	config.Config.Database = config.Config.Database + "_config"

	assert.NoError(t, RunMain(config, Install))
	assert.NoError(t, RunMain(config, Uninstall))
}
