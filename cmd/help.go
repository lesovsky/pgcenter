// Help templates

package cmd

import (
	"fmt"
	"github.com/lesovsky/pgcenter/cmd/config"
	"github.com/lesovsky/pgcenter/cmd/profile"
	"github.com/lesovsky/pgcenter/cmd/record"
	"github.com/lesovsky/pgcenter/cmd/report"
	top "github.com/lesovsky/pgcenter/cmd/top"
)

func printMainHelp() string {
	return fmt.Sprintf(`%s

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

Report bugs to <%s>.
`,
		Root.Long,
		config.CommandDefinition.Short,
		profile.CommandDefinition.Short,
		record.CommandDefinition.Short,
		report.CommandDefinition.Short,
		top.CommandDefinition.Short,
		programIssuesUrl)
}

func printConfigHelp() string {
	return fmt.Sprintf(`%s

Usage:
  pgcenter config [OPTIONS]... [DBNAME [USERNAME]]

Options:
  -i, --install			install pgcenter's stats schema
  -u, --uninstall		uninstall pgcenter's stats schema
  -d, --dbname DBNAME		database name to connect to
  -h, --host HOSTNAME		database server host or socket directory
  -p, --port PORT		database server port (default 5432)
  -U, --username USERNAME	database user name

General options:
  -?, --help		show this help and exit

Report bugs to <%s>.
`,
		config.CommandDefinition.Long,
		programIssuesUrl)
}

func printProfileHelp() string {
	return fmt.Sprintf(`%s

Usage:
 pgcenter profile [OPTIONS]... [DBNAME [USERNAME]]

Options:
 -d, --dbname DBNAME		database name to connect to
 -h, --host HOSTNAME		database server host or socket directory
 -p, --port PORT		database server port (default 5432)
 -U, --username USERNAME	database user name

 -P, --pid PID			backend PID to profile to
 -F, --freq FREQ		profile at this frequency (default: 100ms, min: 1ms, max: 1s)
 -s, --strsize SIZE		limit length of print query strings to STRSIZE chars (default 128)

General options:
 -?, --help		show this help and exit

Report bugs to <%s>.
`,
		profile.CommandDefinition.Long,
		programIssuesUrl)
}

func printTopHelp() string {
	return fmt.Sprintf(`%s

Usage:
  pgcenter top [OPTIONS]... [DBNAME [USERNAME]]

Options:
  -d, --dbname DBNAME		database name to connect to
  -h, --host HOSTNAME		database server host or socket directory
  -p, --port PORT		database server port (default 5432)
  -U, --username USERNAME	database user name

General options:
  -?, --help		show this help and exit

Report bugs to <%s>.
`,
		top.CommandDefinition.Long,
		programIssuesUrl)
}

func printRecordHelp() string {
	return fmt.Sprintf(`%s

Usage:
 pgcenter record [OPTIONS]... [DBNAME [USERNAME]]

Options:
 -d, --dbname DBNAME		database name to connect to
 -h, --host HOSTNAME		database server host or socket directory
 -p, --port PORT		database server port (default 5432)
 -U, --username USERNAME	database user name

 -i, --interval DURATION	statistics recording interval (default: 1s)
 -c, --count INT		number of statistics samples to record
 -f, --file FILENAME		file name where statistics to write to (default: pgcenter.stat.tar)
 -t, --truncate			truncate statistics file, before starting (defailt: false)
 -s, --strlimit INT		maximum query length to record (default: 0, no limit)
 -1, --oneshot			append single statistics snapshot and exit (alias for --interval 0 --count 1)

General options:
 -?, --help		show this help and exit

Report bugs to <%s>.
`,
		record.CommandDefinition.Long,
		programIssuesUrl)
}

func printReportHelp() string {
	return fmt.Sprintf(`%s

Usage:
 pgcenter report [OPTIONS]...

Options:
 -f, --file FILE		read stats from file (default: pgcenter.stat.tar)
 -s, --start TIMESTAMP		starting time of the report (format: [YYYYMMDD-]HHMMSS)
 -e, --end TIMESTAMP		ending time of the report (format: [YYYYMMDD-]HHMMSS)
 -o, --order COLNAME		order values by column
     --desc			use descendant order (default)
     --asc			use ascendant order
 -g, --grep COLNAME:PATTERN	filter values in specfied column (format: colname:filtertext)
 -l, --limit INT		print only limited number of rows per sample (default: unlimited)
 -t, --strlimit INT		maximum string size to print (default: 32, 0 disables truncate)
 -r, --rate DURATION		statistics changes rate interval (default: 1s)

Report options:
 -A, --activity			show pg_stat_activity statistics
 -R, --replication		show pg_stat_replication statistics
 
 -D, --databases		show pg_stat_database statistics
 -T, --tables			show pg_stat_user_tables statistics
 -I, --indexes			show pg_stat_user_indexes and pg_statio_user_indexes statistics
 -S, --sizes			show statistics about tables sizes
 -F, --functions		show pg_stat_user_functions statistics
 -X, --statements SELECTOR	show pg_stat_statements statistics, use additional selector to choose stats
				'm' - timings; 'g' - general; 'i' - io; 't' - temp files io; 'l' - local files io
 -P, --progress SELECTOR	show pg_stat_progress_* statistics, use additional selector to choose stats
				'v' - vacuum; 'c' - cluster; 'i' - create index

 -d, --describe			show statistics description, combined with one of the report options

General options:
 -?, --help		show this help and exit

Report bugs to <%s>.
`,
		report.CommandDefinition.Long,
		programIssuesUrl)
}
