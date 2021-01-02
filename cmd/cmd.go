// Entry point for root command - pgcenter

package cmd

import (
	"github.com/lesovsky/pgcenter/cmd/config"
	"github.com/lesovsky/pgcenter/cmd/top"
	"github.com/spf13/cobra"
)

// Root describes the CLI command of main program
var Root = &cobra.Command{
	Use:     programName,
	Short:   "Admin tool for PostgreSQL",
	Long:    "pgCenter is a command line admin tool for PostgreSQL.",
	Version: printVersion(), // use constants from version.go
}

func init() {
	Root.PersistentFlags().BoolP("help", "?", false, "show this help and exit")

	// Setup help and versions templates for main program
	Root.SetVersionTemplate(printVersion())
	Root.SetHelpTemplate(printMainHelp())

	// Setup 'config' sub-command
	Root.AddCommand(config.CommandDefinition)
	config.CommandDefinition.SetVersionTemplate(printVersion())
	config.CommandDefinition.SetHelpTemplate(printConfigHelp())
	config.CommandDefinition.SetUsageTemplate(printConfigHelp())

	// Setup 'profile' sub-command
	//Root.AddCommand(profile.CommandDefinition)
	//profile.CommandDefinition.SetVersionTemplate(printVersion())
	//profile.CommandDefinition.SetHelpTemplate(printProfileHelp())
	//profile.CommandDefinition.SetUsageTemplate(printProfileHelp())

	// Setup 'record' sub-command
	//Root.AddCommand(record.CommandDefinition)
	//record.CommandDefinition.SetVersionTemplate(printVersion())
	//record.CommandDefinition.SetHelpTemplate(printRecordHelp())
	//record.CommandDefinition.SetUsageTemplate(printRecordHelp())

	// Setup 'report' sub-command
	//Root.AddCommand(report.CommandDefinition)
	//report.CommandDefinition.SetVersionTemplate(printVersion())
	//report.CommandDefinition.SetHelpTemplate(printReportHelp())
	//report.CommandDefinition.SetUsageTemplate(printReportHelp())

	// Setup 'top' sub-command
	Root.AddCommand(top.CommandDefinition)
	top.CommandDefinition.SetVersionTemplate(printVersion())
	top.CommandDefinition.SetHelpTemplate(printTopHelp())
	top.CommandDefinition.SetUsageTemplate(printTopHelp())
}
