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

// Switch sort order to left column
func orderKeyLeft(context *context, doUpdate chan int) func(_ *gocui.Gui, _ *gocui.View) error {
	return func(_ *gocui.Gui, _ *gocui.View) error {
		context.current.OrderKey--
		if context.current.OrderKey < 0 {
			context.current.OrderKey = context.current.Ncols - 1
		}
		doUpdate <- 1
		return nil
	}
}

// Switch sort order to right column
func orderKeyRight(context *context, doUpdate chan int) func(_ *gocui.Gui, _ *gocui.View) error {
	return func(_ *gocui.Gui, _ *gocui.View) error {
		context.current.OrderKey++
		if context.current.OrderKey >= context.current.Ncols {
			context.current.OrderKey = 0
		}
		doUpdate <- 1
		return nil
	}
}

// Increase or decrease width of an active column
func changeWidth(app *app, d int) func(_ *gocui.Gui, _ *gocui.View) error {
	return func(g *gocui.Gui, v *gocui.View) error {
		var width int
		cidx := app.context.current.OrderKey                                   // index of an active column
		clen := len(app.stats.DiffPGresult.Cols[app.context.current.OrderKey]) // length of the column's name

		// set new width
		switch d {
		case colsWidthIncr:
			width = app.context.current.ColsWidth[cidx] + colsWidthStep
		case colsWidthDecr:
			width = app.context.current.ColsWidth[cidx] - colsWidthStep
		default:
			width = app.context.current.ColsWidth[cidx] // should never be here.
		}

		// new width should not be less than column's name or longer than defined limit
		switch {
		case width < clen:
			width = clen
		case width > colsWidthMax:
			width = colsWidthMax
		}

		app.context.current.ColsWidth[cidx] = width

		app.doUpdate <- 1
		return nil
	}
}

// Switch sort order direction between descend and ascend
func switchSortOrder(context *context, doUpdate chan int) func(g *gocui.Gui, _ *gocui.View) error {
	return func(g *gocui.Gui, _ *gocui.View) error {
		context.current.OrderDesc = !context.current.OrderDesc
		printCmdline(g, "Switch sort order")
		doUpdate <- 1
		return nil
	}
}

// Set filter pattern for current column
func setFilter(g *gocui.Gui, v *gocui.View, answer string, context *context) {
	var err error

	answer = strings.TrimPrefix(string(v.Buffer()), dialogPrompts[dialogFilter])
	answer = strings.TrimSuffix(answer, "\n")

	// clear used pattern if empty string is entered, otherwise set input as pattern
	if answer == "\n" || answer == "" {
		delete(context.current.Filters, context.current.OrderKey)
		printCmdline(g, "Regexp cleared")
	} else {
		context.current.Filters[context.current.OrderKey], err = regexp.Compile(answer)
		if err != nil {
			printCmdline(g, "Do nothing. Failed to compile regexp: %s", err)
		}
	}
}

// Switch from context unit to another one
func switchContextTo(app *app, c string) func(g *gocui.Gui, v *gocui.View) error {
	return func(g *gocui.Gui, v *gocui.View) error {
		// in case of switching to pg_stat_statements and it isn't available - keep current stats context
		if app.stats.PgStatStatementsAvail == false && c == stat.StatementsView {
			printCmdline(g, msgPgStatStatementsUnavailable)
			return nil
		}

		// Save current context unit with its settings into context list
		app.context.contextList[app.context.current.Name] = app.context.current

		// Load new context unit (with settings) from the list
		switch c {
		case stat.StatementsView:
			// fall through another switch and select appropriate pg_stat_statements stats
			switch app.context.current.Name {
			case stat.StatementsTimingView:
				app.context.current = app.context.contextList[stat.StatementsGeneralView]
			case stat.StatementsGeneralView:
				app.context.current = app.context.contextList[stat.StatementsIOView]
			case stat.StatementsIOView:
				app.context.current = app.context.contextList[stat.StatementsTempView]
			case stat.StatementsTempView:
				app.context.current = app.context.contextList[stat.StatementsLocalView]
			case stat.StatementsLocalView:
				app.context.current = app.context.contextList[stat.StatementsTimingView]
			default:
				app.context.current = app.context.contextList[stat.StatementsTimingView]
			}
		case stat.ProgressView:
			// fall through another switch and select appropriate pg_stat_progress_* stats
			switch app.context.current.Name {
			case stat.ProgressVacuumView:
				app.context.current = app.context.contextList[stat.ProgressClusterView]
			case stat.ProgressClusterView:
				app.context.current = app.context.contextList[stat.ProgressCreateIndexView]
			case stat.ProgressCreateIndexView:
				app.context.current = app.context.contextList[stat.ProgressVacuumView]
			default:
				app.context.current = app.context.contextList[stat.ProgressVacuumView]
			}
		default:
			app.context.current = app.context.contextList[c]
		}

		printCmdline(g, app.context.current.Msg)

		app.doUpdate <- 1
		return nil
	}
}

// TODO: looks like these two functions below are redundant, their code is the same - possibly they should be replaced with switchContextTo() function

// Switch pg_stat_statements context units
func switchContextToPgss(app *app, c string) {
	// Save current context unit with its settings into context list
	app.context.contextList[app.context.current.Name] = app.context.current

	// Load new context unit (with settings) from the list
	app.context.current = app.context.contextList[c]

	printCmdline(app.ui, app.context.current.Msg)
	app.doUpdate <- 1
}

// Switch pg_stat_progress_* context units
func switchContextToProgress(app *app, c string) {
	// Save current context unit with its settings into context list
	app.context.contextList[app.context.current.Name] = app.context.current

	// Load new context unit (with settings) from the list
	app.context.current = app.context.contextList[c]

	printCmdline(app.ui, app.context.current.Msg)
	app.doUpdate <- 1
}

// A toggle to show system tables stats
func toggleSysTables(context *context, doUpdate chan int) func(g *gocui.Gui, _ *gocui.View) error {
	return func(g *gocui.Gui, _ *gocui.View) error {
		switch context.sharedOptions.ViewType {
		case "user":
			context.sharedOptions.ViewType = "all"
			printCmdline(g, "Show system tables: on")
		case "all":
			context.sharedOptions.ViewType = "user"
			printCmdline(g, "Show system tables: off")
		default: // never should be here, but paranoia check
			context.sharedOptions.ViewType = "user"
			printCmdline(g, "Show system tables: on")
		}

		doUpdate <- 1
		return nil
	}
}

// Change age threshold for queries and transactions (pg_stat_activity only)
func changeQueryAge(g *gocui.Gui, v *gocui.View, answer string, context *context) {
	answer = strings.TrimPrefix(string(v.Buffer()), dialogPrompts[dialogChangeAge])
	answer = strings.TrimSuffix(answer, "\n")

	if answer == "" {
		printCmdline(g, "Reset to default - 00:00:00.")
		context.sharedOptions.QueryAgeThresh = "00:00:00"
		return
	}

	var hour, min, sec, msec int

	n, err := fmt.Sscanf(answer, "%d:%d:%d.%d", &hour, &min, &sec, &msec)

	if n < 3 && err != nil || ((hour > 23) || (min > 59) || (sec > 59) || (msec > 999999)) {
		printCmdline(g, "Nothing to do. Failed read or invalid value.")
		return
	}

	context.sharedOptions.QueryAgeThresh = answer
}

// A toggle to show 'idle' connections (pg_stat_activity only)
func toggleIdleConns(context *context, doUpdate chan int) func(g *gocui.Gui, _ *gocui.View) error {
	return func(g *gocui.Gui, _ *gocui.View) error {
		context.sharedOptions.ShowNoIdle = !context.sharedOptions.ShowNoIdle

		if context.sharedOptions.ShowNoIdle {
			printCmdline(g, "Show idle connections: off.")
		} else {
			printCmdline(g, "Show idle connections: on.")
		}

		doUpdate <- 1
		return nil
	}
}
