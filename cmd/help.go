// Help templates

package cmd

import (
	"fmt"
	"github.com/lesovsky/pgcenter/cmd/config"
	profile "github.com/lesovsky/pgcenter/cmd/profile"
	record "github.com/lesovsky/pgcenter/cmd/record"
	report "github.com/lesovsky/pgcenter/cmd/report"
	top "github.com/lesovsky/pgcenter/cmd/top"
)

const (
	mainHelpTemplate = `%s

Usage:
  pgcenter [flags]
  pgcenter [command] [command-flags] [args]

Available commands:
  config	%s
  profile	%s
  record	%s
  report	%s
  top		%s

Flags:
  -?, --help		show this help and exit
      --version		show version information and exit

Use "pgcenter [command] --help" for more information about a command.

Report bugs to %s
`
	configHelpTemplate = `%s

Usage:
  pgcenter config [OPTIONS]... [DBNAME [USERNAME]]

Options:
  -i, --install			install pgcenter's stats schema
  -u, --uninstall		uninstall pgcenter's stats schema
  -d, --dbname DBNAME		database name to connect to
  -h, --host HOSTNAME		database server host or socket directory.
  -p, --port PORT		database server port (default 5432)
  -U, --username USERNAME	database user name

General options:
  -?, --help		show this help and exit
      --version		show version information and exit

Report bugs to %s
`
	profileHelpTemplate = `%s

Usage:
  pgcenter profile [OPTIONS]... [DBNAME [USERNAME]]

Options:
  -d, --dbname DBNAME		database name to connect to
  -h, --host HOSTNAME		database server host or socket directory.
  -p, --port PORT		database server port (default 5432)
  -U, --username USERNAME	database user name

  -P, --pid PID			backend PID to profile to
  -F, --freq FREQ		profile at this frequency (min 1, max 1000)
  -s, --strsize SIZE		limit length of print query strings to STRSIZE chars (default 128)	

General options:
  -?, --help		show this help and exit
      --version		show version information and exit

Report bugs to %s
`
	topHelpTemplate = `%s

Usage:
  pgcenter top [OPTIONS]... [DBNAME [USERNAME]]

Options:
  -d, --dbname DBNAME		database name to connect to
  -h, --host HOSTNAME		database server host or socket directory.
  -p, --port PORT		database server port (default 5432)
  -U, --username USERNAME	database user name

General options:
  -?, --help		show this help and exit
      --version		show version information and exit

Report bugs to %s
`
	recordHelpTemplate = `%s

Usage:
  pgcenter record [OPTIONS]... [DBNAME [USERNAME]]

Options:
  -d, --dbname DBNAME		database name to connect to
  -h, --host HOSTNAME		database server host or socket directory.
  -p, --port PORT		database server port (default 5432)
  -U, --username USERNAME	database user name

  -i, --interval		polling interval (default: 1s)
  -c, --count			number of stats samples to collect
  -f, --file			file name where statistics to write to (default: pgcenter.stat.tar)
  -a, --append			append statistics to file, instead of creating a new file
  -t, --truncate		maximum query length to record (default: 0, no limit)
  -1, --oneshot			append single statistics snapshot and exit (alias for --append --interval 0 --count 1)

General options:
  -?, --help		show this help and exit
      --version		show version information and exit

Report bugs to %s
`
	reportHelpTemplate = `%s

Usage:
  pgcenter report [OPTIONS]...

Options:
  -f, --file			read stats from file (default: pgcenter.stat.tar)
  -s, --start			starting time of the report (format: [YYYYMMDD-]HHMMSS)
  -e, --end			ending time of the report (format: [YYYYMMDD-]HHMMSS)
  -o, --order			order values by column (default descending, use '+' sign before a column name for ascending order)
  -g, --grep			filter values in specfied column (format: colname:filtertext)
  -l, --limit			print only limited number of rows per sample (default: unlimited)
  -t, --truncate		maximum string size to print (default: 32, 0 disables truncate)
  -i, --interval		delta interval (default: 1s)

Report options:
  -A, --activity		show pg_stat_activity statistics
  -S, --sizes			show statistics about tables sizes
  -D, --databases		show pg_stat_database statistics
  -F, --functions		show pg_stat_user_functions statistics
  -R, --replication		show pg_stat_replication statistics
  -T, --tables			show pg_stat_user_tables statistics
  -I, --indexes			show pg_stat_user_indexes and pg_statio_user_indexes statistics
  -V, --vacuum			show pg_stat_progress_vacuum statistics
  -X, --statements [X]		show pg_stat_statements statistics, use additional selector to choose stats.
				'm' - timings; 'g' - general; 'i' - io; 't' - temp files io; 'l' - local files io. 

  -d, --describe		show statistics description, combined with one of the report options

General options:
  -?, --help		show this help and exit
      --version		show version information and exit

Report bugs to %s
`
)

func printMainHelp() string {
	return fmt.Sprintf(mainHelpTemplate,
		Root.Long,
		config.CommandDefinition.Short,
		profile.CommandDefinition.Short,
		record.CommandDefinition.Short,
		report.CommandDefinition.Short,
		top.CommandDefinition.Short,
		ProgramIssuesUrl)
}

func printConfigHelp() string {
	return fmt.Sprintf(configHelpTemplate,
		config.CommandDefinition.Long,
		ProgramIssuesUrl)
}

func printProfileHelp() string {
	return fmt.Sprintf(profileHelpTemplate,
		profile.CommandDefinition.Long,
		ProgramIssuesUrl)
}

func printTopHelp() string {
	return fmt.Sprintf(topHelpTemplate,
		top.CommandDefinition.Long,
		ProgramIssuesUrl)
}

func printRecordHelp() string {
	return fmt.Sprintf(recordHelpTemplate,
		record.CommandDefinition.Long,
		ProgramIssuesUrl)
}

func printReportHelp() string {
	return fmt.Sprintf(reportHelpTemplate,
		report.CommandDefinition.Long,
		ProgramIssuesUrl)
}
