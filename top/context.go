// Context defines what kind of stats should be displayed to user. Context described in 'contextUnit' struct.
// A single unit defines the following settings:
// 1. SQL query used to get stats;
// 2. Which columns should be displayed as a delta between current and previous stats, and which columns should be displayed as is;
// 3. Which order should be used -- descending or ascending;
// 4. patterns used for filtering.
// All available contexts grouped into a single list. When user switches between displayed stats, internally he switches
// between context units. Current context and its settings are saved into the list, and new context with its
// settings is loaded instead of current. Hence, all settings such as ordering, filtering amd others are permanent between switches.

package top

import (
	"fmt"
	"github.com/jroimartin/gocui"
	"github.com/lesovsky/pgcenter/lib/stat"
	"regexp"
	"strings"
)

const (
	colsWidthIncr = iota // increase width of an active column
	colsWidthDecr        // decrease width of an active column

	colsWidthMax  = 256 // max width allowed for columnn, they can't be wider than that value
	colsWidthStep = 4   // minimal step of changing column's width, 1 is too boring and 4 looks good
)

// Container for context settings.
type context struct {
	current       *stat.ContextUnit // Current unit in use
	contextList   stat.ContextList  // List of all available units
	sharedOptions stat.Options      // Queries' settings that depends on Postgres version
	aux           auxType           // Type of current auxiliary stats
}

var (
	// Context's container
	ctx context

	// List of available units in 'pgcenter top' program
	ctxList = stat.ContextList{
		stat.DatabaseView:            &stat.PgStatDatabaseUnit,
		stat.ReplicationView:         &stat.PgStatReplicationUnit,
		stat.TablesView:              &stat.PgStatTablesUnit,
		stat.IndexesView:             &stat.PgStatIndexesUnit,
		stat.SizesView:               &stat.PgTablesSizesUnit,
		stat.FunctionsView:           &stat.PgStatFunctionsUnit,
		stat.ProgressVacuumView:      &stat.PgStatProgressVacuumUnit,
		stat.ProgressClusterView:     &stat.PgStatProgressClusterUnit,
		stat.ProgressCreateIndexView: &stat.PgStatProgressCreateIndexUnit,
		stat.ActivityView:            &stat.PgStatActivityUnit,
		stat.StatementsTimingView:    &stat.PgSSTimingUnit,
		stat.StatementsGeneralView:   &stat.PgSSGeneralUnit,
		stat.StatementsIOView:        &stat.PgSSIoUnit,
		stat.StatementsTempView:      &stat.PgSSTempUnit,
		stat.StatementsLocalView:     &stat.PgSSLocalUnit,
	}
)

// Initial setup of the context. Set defaults and override settings which depends on Postgres version, recovery status, etc.
func (c *context) Setup(pginfo stat.PgInfo) {
	c.contextList = ctxList

	// Select default context unit
	c.current = c.contextList[stat.ActivityView]

	// Aux stats is not displayed by default
	c.aux = auxNone

	// Adjust queries depending on Postgres version
	c.contextList.AdjustQueries(pginfo)
	c.sharedOptions.Adjust(pginfo, "top")
}

// Switch sort order to left column
func orderKeyLeft(_ *gocui.Gui, _ *gocui.View) error {
	ctx.current.OrderKey--
	if ctx.current.OrderKey < 0 {
		ctx.current.OrderKey = ctx.current.Ncols - 1
	}
	doUpdate <- 1
	return nil
}

// Switch sort order to right column
func orderKeyRight(_ *gocui.Gui, _ *gocui.View) error {
	ctx.current.OrderKey++
	if ctx.current.OrderKey >= ctx.current.Ncols {
		ctx.current.OrderKey = 0
	}
	doUpdate <- 1
	return nil
}

// Increase or decrease width of an active column
func changeWidth(d int) func(_ *gocui.Gui, _ *gocui.View) error {
	return func(g *gocui.Gui, v *gocui.View) error {
		var width int
		cidx := ctx.current.OrderKey                               // index of an active column
		clen := len(stats.DiffPGresult.Cols[ctx.current.OrderKey]) // length of the column's name

		// set new width
		switch d {
		case colsWidthIncr:
			width = ctx.current.ColsWidth[cidx] + colsWidthStep
		case colsWidthDecr:
			width = ctx.current.ColsWidth[cidx] - colsWidthStep
		default:
			width = ctx.current.ColsWidth[cidx] // should never be here.
		}

		// new width should not be less than column's name or longer than defined limit
		switch {
		case width < clen:
			width = clen
		case width > colsWidthMax:
			width = colsWidthMax
		}

		ctx.current.ColsWidth[cidx] = width

		doUpdate <- 1
		return nil
	}
}

// Switch sort order direction between descend and ascend
func switchSortOrder(g *gocui.Gui, _ *gocui.View) error {
	ctx.current.OrderDesc = !ctx.current.OrderDesc
	printCmdline(g, "Switch sort order")
	doUpdate <- 1
	return nil
}

// Set filter pattern for current column
func setFilter(g *gocui.Gui, v *gocui.View, answer string) {
	var err error

	answer = strings.TrimPrefix(string(v.Buffer()), dialogPrompts[dialogFilter])
	answer = strings.TrimSuffix(answer, "\n")

	// clear used pattern if empty string is entered, otherwise set input as pattern
	if answer == "\n" || answer == "" {
		delete(ctx.current.Filters, ctx.current.OrderKey)
		printCmdline(g, "Regexp cleared")
	} else {
		ctx.current.Filters[ctx.current.OrderKey], err = regexp.Compile(answer)
		if err != nil {
			printCmdline(g, "Do nothing. Failed to compile regexp: %s", err)
		}
	}
}

// Switch from context unit to another one
func switchContextTo(c string) func(g *gocui.Gui, v *gocui.View) error {
	return func(g *gocui.Gui, v *gocui.View) error {
		// in case of switching to pg_stat_statements and it isn't available - keep current stats context
		if stats.PgStatStatementsAvail == false && c == stat.StatementsView {
			printCmdline(g, msgPgStatStatementsUnavailable)
			return nil
		}

		// Save current context unit with its settings into context list
		ctx.contextList[ctx.current.Name] = ctx.current

		// Load new context unit (with settings) from the list
		switch c {
		case stat.StatementsView:
			// fall through another switch and select appropriate pg_stat_statements stats
			switch ctx.current.Name {
			case stat.StatementsTimingView:
				ctx.current = ctx.contextList[stat.StatementsGeneralView]
			case stat.StatementsGeneralView:
				ctx.current = ctx.contextList[stat.StatementsIOView]
			case stat.StatementsIOView:
				ctx.current = ctx.contextList[stat.StatementsTempView]
			case stat.StatementsTempView:
				ctx.current = ctx.contextList[stat.StatementsLocalView]
			case stat.StatementsLocalView:
				ctx.current = ctx.contextList[stat.StatementsTimingView]
			default:
				ctx.current = ctx.contextList[stat.StatementsTimingView]
			}
		case stat.ProgressView:
			// fall through another switch and select appropriate pg_stat_progress_* stats
			switch ctx.current.Name {
			case stat.ProgressVacuumView:
				ctx.current = ctx.contextList[stat.ProgressClusterView]
			case stat.ProgressClusterView:
				ctx.current = ctx.contextList[stat.ProgressCreateIndexView]
			case stat.ProgressCreateIndexView:
				ctx.current = ctx.contextList[stat.ProgressVacuumView]
			default:
				ctx.current = ctx.contextList[stat.ProgressVacuumView]
			}
		default:
			ctx.current = ctx.contextList[c]
		}

		printCmdline(g, ctx.current.Msg)

		doUpdate <- 1
		return nil
	}
}

// TODO: looks like these two functions below are redundant, their code is the same - possibly they should be replaced with switchContextTo() function

// Switch pg_stat_statements context units
func switchContextToPgss(g *gocui.Gui, c string) {
	// Save current context unit with its settings into context list
	ctx.contextList[ctx.current.Name] = ctx.current

	// Load new context unit (with settings) from the list
	ctx.current = ctx.contextList[c]

	printCmdline(g, ctx.current.Msg)
	doUpdate <- 1
}

// Switch pg_stat_progress_* context units
func switchContextToProgress(g *gocui.Gui, c string) {
	// Save current context unit with its settings into context list
	ctx.contextList[ctx.current.Name] = ctx.current

	// Load new context unit (with settings) from the list
	ctx.current = ctx.contextList[c]

	printCmdline(g, ctx.current.Msg)
	doUpdate <- 1
}

// A toggle to show system tables stats
func toggleSysTables(g *gocui.Gui, _ *gocui.View) error {
	switch ctx.sharedOptions.ViewType {
	case "user":
		ctx.sharedOptions.ViewType = "all"
		printCmdline(g, "Show system tables: on")
	case "all":
		ctx.sharedOptions.ViewType = "user"
		printCmdline(g, "Show system tables: off")
	default: // never should be here, but paranoia check
		ctx.sharedOptions.ViewType = "user"
		printCmdline(g, "Show system tables: on")
	}

	doUpdate <- 1
	return nil
}

// Change age threshold for queries and transactions (pg_stat_activity only)
func changeQueryAge(g *gocui.Gui, v *gocui.View, answer string) {
	answer = strings.TrimPrefix(string(v.Buffer()), dialogPrompts[dialogChangeAge])
	answer = strings.TrimSuffix(answer, "\n")

	if answer == "" {
		printCmdline(g, "Reset to default - 00:00:00.")
		ctx.sharedOptions.QueryAgeThresh = "00:00:00"
		return
	}

	var hour, min, sec, msec int

	n, err := fmt.Sscanf(answer, "%d:%d:%d.%d", &hour, &min, &sec, &msec)

	if n < 3 && err != nil || ((hour > 23) || (min > 59) || (sec > 59) || (msec > 999999)) {
		printCmdline(g, "Nothing to do. Failed read or invalid value.")
		return
	}

	ctx.sharedOptions.QueryAgeThresh = answer
}

// A toggle to show 'idle' connections (pg_stat_activity only)
func toggleIdleConns(g *gocui.Gui, _ *gocui.View) error {
	ctx.sharedOptions.ShowNoIdle = !ctx.sharedOptions.ShowNoIdle

	if ctx.sharedOptions.ShowNoIdle {
		printCmdline(g, "Show idle connections: off.")
	} else {
		printCmdline(g, "Show idle connections: on.")
	}

	doUpdate <- 1
	return nil
}
