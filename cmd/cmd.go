// Entry point for root command - pgcenter

package cmd

import (
	"fmt"
	"github.com/lesovsky/pgcenter/cmd/config"
	"github.com/lesovsky/pgcenter/cmd/profile"
	"github.com/lesovsky/pgcenter/cmd/record"
	"github.com/lesovsky/pgcenter/cmd/report"
	"github.com/lesovsky/pgcenter/cmd/top"
	"github.com/spf13/cobra"
)

var Root = &cobra.Command{
	Use:     ProgramName,
	Short:   "Admin tool for PostgreSQL",
	Long:    "pgCenter is a command line admin tool for PostgreSQL.",
	Version: "dummy", // use constants from version.go
}

func init() {
	// Use this in case when you want to make something in root 'pgcenter' command
	//Root.Run = runRoot

	Root.PersistentFlags().BoolP("help", "?", false, "show this help and exit")

	// Setup help and versions templates for main program
	Root.SetVersionTemplate(PrintVersion())
	Root.SetHelpTemplate(printMainHelp())

	// Setup 'config' sub-command
	Root.AddCommand(config.CommandDefinition)
	config.CommandDefinition.SetVersionTemplate(PrintVersion())
	config.CommandDefinition.SetHelpTemplate(printConfigHelp())
	config.CommandDefinition.SetUsageTemplate(printConfigHelp())

	// Setup 'profile' sub-command
	Root.AddCommand(profile.CommandDefinition)
	profile.CommandDefinition.SetVersionTemplate(PrintVersion())
	profile.CommandDefinition.SetHelpTemplate(printProfileHelp())
	profile.CommandDefinition.SetUsageTemplate(printProfileHelp())

	// Setup 'record' sub-command
	Root.AddCommand(record.CommandDefinition)
	record.CommandDefinition.SetVersionTemplate(PrintVersion())
	record.CommandDefinition.SetHelpTemplate(printRecordHelp())
	record.CommandDefinition.SetUsageTemplate(printRecordHelp())

	// Setup 'report' sub-command
	Root.AddCommand(report.CommandDefinition)
	report.CommandDefinition.SetVersionTemplate(PrintVersion())
	report.CommandDefinition.SetHelpTemplate(printReportHelp())
	report.CommandDefinition.SetUsageTemplate(printReportHelp())

	// Setup 'top' sub-command
	Root.AddCommand(top.CommandDefinition)
	top.CommandDefinition.SetVersionTemplate(PrintVersion())
	top.CommandDefinition.SetHelpTemplate(printTopHelp())
	top.CommandDefinition.SetUsageTemplate(printTopHelp())
}

// Things executed in root 'pgcenter' command.
func runRoot(cmd *cobra.Command, args []string) {
	// debug purpose
	fmt.Printf("%#v\n", args)
}
