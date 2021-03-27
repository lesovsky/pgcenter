package main

import (
	"fmt"
	"github.com/lesovsky/pgcenter/cmd/config"
	"github.com/lesovsky/pgcenter/cmd/profile"
	"github.com/lesovsky/pgcenter/cmd/record"
	"github.com/lesovsky/pgcenter/cmd/report"
	"github.com/lesovsky/pgcenter/cmd/top"
	"github.com/lesovsky/pgcenter/internal/version"
	"github.com/spf13/cobra"
)

// pgcenter describes the root command of program
var pgcenter = &cobra.Command{
	Short:         "Admin tool for PostgreSQL",
	Long:          "pgCenter is a command line admin tool for PostgreSQL.",
	SilenceUsage:  true,
	SilenceErrors: true,
}

func init() {
	pgcenter.PersistentFlags().BoolP("help", "?", false, "show this help and exit")

	name, tag, commit, branch := version.Version()
	versionStr := fmt.Sprintf("%s %s %s-%s\n", name, tag, commit, branch)

	pgcenter.Use = name
	pgcenter.Version = versionStr

	// Setup help and versions templates for main program
	pgcenter.SetVersionTemplate(versionStr)
	pgcenter.SetHelpTemplate(printMainHelp())

	// Setup 'config' sub-command
	pgcenter.AddCommand(config.CommandDefinition)
	config.CommandDefinition.SetVersionTemplate(versionStr)
	config.CommandDefinition.SetHelpTemplate(printConfigHelp())
	config.CommandDefinition.SetUsageTemplate(printConfigHelp())

	// Setup 'profile' sub-command
	pgcenter.AddCommand(profile.CommandDefinition)
	profile.CommandDefinition.SetVersionTemplate(versionStr)
	profile.CommandDefinition.SetHelpTemplate(printProfileHelp())
	profile.CommandDefinition.SetUsageTemplate(printProfileHelp())

	// Setup 'record' sub-command
	pgcenter.AddCommand(record.CommandDefinition)
	record.CommandDefinition.SetVersionTemplate(versionStr)
	record.CommandDefinition.SetHelpTemplate(printRecordHelp())
	record.CommandDefinition.SetUsageTemplate(printRecordHelp())

	// Setup 'report' sub-command
	pgcenter.AddCommand(report.CommandDefinition)
	report.CommandDefinition.SetVersionTemplate(versionStr)
	report.CommandDefinition.SetHelpTemplate(printReportHelp())
	report.CommandDefinition.SetUsageTemplate(printReportHelp())

	// Setup 'top' sub-command
	pgcenter.AddCommand(top.CommandDefinition)
	top.CommandDefinition.SetVersionTemplate(versionStr)
	top.CommandDefinition.SetHelpTemplate(printTopHelp())
	top.CommandDefinition.SetUsageTemplate(printTopHelp())
}

func main() {
	if err := pgcenter.Execute(); err != nil {
		fmt.Println(err)
	}
}
