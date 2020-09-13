package top

import (
	"fmt"
	"github.com/jroimartin/gocui"
	"github.com/lesovsky/pgcenter/internal/view"
	"regexp"
	"strings"
)

const (
	colsWidthIncr = iota // increase width of an active column
	colsWidthDecr        // decrease width of an active column

	colsWidthMax  = 256 // max width allowed for column, they can't be wider than that value
	colsWidthStep = 4   // minimal step of changing column's width, 1 is too boring and 4 looks good
)

func orderKeyLeft(v *view.View, doUpdate chan int) func(_ *gocui.Gui, _ *gocui.View) error {
	return func(_ *gocui.Gui, _ *gocui.View) error {
		v.OrderKey--
		if v.OrderKey < 0 {
			v.OrderKey = v.Ncols - 1
		}
		doUpdate <- 1
		return nil
	}
}

// Switch sort order to right column
func orderKeyRight(v *view.View, doUpdate chan int) func(_ *gocui.Gui, _ *gocui.View) error {
	return func(_ *gocui.Gui, _ *gocui.View) error {
		v.OrderKey++
		if v.OrderKey >= v.Ncols {
			v.OrderKey = 0
		}
		doUpdate <- 1
		return nil
	}
}

// Increase or decrease width of an active column
func changeWidth(app *app, d int) func(_ *gocui.Gui, _ *gocui.View) error {
	return func(g *gocui.Gui, v *gocui.View) error {
		var width int
		cidx := app.config.view.OrderKey                           // index of an active column
		clen := len(app.stats.Diff.Cols[app.config.view.OrderKey]) // length of the column's name

		// set new width
		switch d {
		case colsWidthIncr:
			width = app.config.view.ColsWidth[cidx] + colsWidthStep
		case colsWidthDecr:
			width = app.config.view.ColsWidth[cidx] - colsWidthStep
		default:
			width = app.config.view.ColsWidth[cidx] // should never be here.
		}

		// new width should not be less than column's name or longer than defined limit
		switch {
		case width < clen:
			width = clen
		case width > colsWidthMax:
			width = colsWidthMax
		}

		app.config.view.ColsWidth[cidx] = width

		app.doUpdate <- 1
		return nil
	}
}

// Switch sort order direction between descend and ascend
func switchSortOrder(v *view.View, doUpdate chan int) func(g *gocui.Gui, _ *gocui.View) error {
	return func(g *gocui.Gui, _ *gocui.View) error {
		v.OrderDesc = !v.OrderDesc
		printCmdline(g, "Switch sort order")
		doUpdate <- 1
		return nil
	}
}

// Set filter pattern for current column
func setFilter(g *gocui.Gui, v *gocui.View, answer string, view *view.View) {
	var err error

	answer = strings.TrimPrefix(string(v.Buffer()), dialogPrompts[dialogFilter])
	answer = strings.TrimSuffix(answer, "\n")

	// clear used pattern if empty string is entered, otherwise set input as pattern
	if answer == "\n" || answer == "" {
		delete(view.Filters, view.OrderKey)
		printCmdline(g, "Regexp cleared")
	} else {
		view.Filters[view.OrderKey], err = regexp.Compile(answer)
		if err != nil {
			printCmdline(g, "Do nothing. Failed to compile regexp: %s", err)
		}
	}
}

// Switch from context unit to another one
func switchContextTo(app *app, c string) func(g *gocui.Gui, v *gocui.View) error {
	return func(g *gocui.Gui, v *gocui.View) error {
		// in case of switching to pg_stat_statements and it isn't available - keep current stats context
		if app.stats.Properties.ExtPGSSAvail == false && c == "statements" {
			printCmdline(g, msgPgStatStatementsUnavailable)
			return nil
		}

		// Save current context unit with its settings into context list
		app.config.views[app.config.view.Name] = app.config.view

		// Load new context unit (with settings) from the list
		switch c {
		case "statements":
			// fall through another switch and select appropriate pg_stat_statements stats
			switch app.config.view.Name {
			case "statements_timings":
				app.config.view = app.config.views["statements_general"]
			case "statements_general":
				app.config.view = app.config.views["statements_io"]
			case "statements_io":
				app.config.view = app.config.views["statements_temp"]
			case "statements_temp":
				app.config.view = app.config.views["statements_local"]
			case "statements_local":
				app.config.view = app.config.views["statements_timings"]
			default:
				app.config.view = app.config.views["statements_timings"]
			}
		case "progress":
			// fall through another switch and select appropriate pg_stat_progress_* stats
			switch app.config.view.Name {
			case "progress_vacuum":
				app.config.view = app.config.views["progress_cluster"]
			case "progress_cluster":
				app.config.view = app.config.views["progress_index"]
			case "progress_index":
				app.config.view = app.config.views["progress_vacuum"]
			default:
				app.config.view = app.config.views["progress_vacuum"]
			}
		default:
			app.config.view = app.config.views[c]
		}

		printCmdline(g, app.config.view.Msg)

		app.doUpdate <- 1
		return nil
	}
}

// TODO: looks like these two functions below are redundant, their code is the same - possibly they should be replaced with switchContextTo() function

// Switch pg_stat_statements context units
func switchContextToPgss(app *app, c string) {
	// Save current context unit with its settings into context list
	app.config.views[app.config.view.Name] = app.config.view

	// Load new context unit (with settings) from the list
	app.config.view = app.config.views[c]

	printCmdline(app.ui, app.config.view.Msg)
	app.doUpdate <- 1
}

// Switch pg_stat_progress_* context units
func switchContextToProgress(app *app, c string) {
	// Save current context unit with its settings into context list
	app.config.views[app.config.view.Name] = app.config.view

	// Load new context unit (with settings) from the list
	app.config.view = app.config.views[c]

	printCmdline(app.ui, app.config.view.Msg)
	app.doUpdate <- 1
}

// A toggle to show system tables stats
func toggleSysTables(c *config, doUpdate chan int) func(g *gocui.Gui, _ *gocui.View) error {
	return func(g *gocui.Gui, _ *gocui.View) error {
		switch c.sharedOptions.ViewType {
		case "user":
			c.sharedOptions.ViewType = "all"
			printCmdline(g, "Show system tables: on")
		case "all":
			c.sharedOptions.ViewType = "user"
			printCmdline(g, "Show system tables: off")
		default: // never should be here, but paranoia check
			c.sharedOptions.ViewType = "user"
			printCmdline(g, "Show system tables: on")
		}

		doUpdate <- 1
		return nil
	}
}

// Change age threshold for queries and transactions (pg_stat_activity only)
func changeQueryAge(g *gocui.Gui, v *gocui.View, answer string, c *config) {
	answer = strings.TrimPrefix(string(v.Buffer()), dialogPrompts[dialogChangeAge])
	answer = strings.TrimSuffix(answer, "\n")

	if answer == "" {
		printCmdline(g, "Reset to default - 00:00:00.")
		c.sharedOptions.QueryAgeThresh = "00:00:00"
		return
	}

	var hour, min, sec, msec int

	n, err := fmt.Sscanf(answer, "%d:%d:%d.%d", &hour, &min, &sec, &msec)

	if n < 3 && err != nil || ((hour > 23) || (min > 59) || (sec > 59) || (msec > 999999)) {
		printCmdline(g, "Nothing to do. Failed read or invalid value.")
		return
	}

	c.sharedOptions.QueryAgeThresh = answer
}

// A toggle to show 'idle' connections (pg_stat_activity only)
func toggleIdleConns(c *config, doUpdate chan int) func(g *gocui.Gui, _ *gocui.View) error {
	return func(g *gocui.Gui, _ *gocui.View) error {
		c.sharedOptions.ShowNoIdle = !c.sharedOptions.ShowNoIdle

		if c.sharedOptions.ShowNoIdle {
			printCmdline(g, "Show idle connections: off.")
		} else {
			printCmdline(g, "Show idle connections: on.")
		}

		doUpdate <- 1
		return nil
	}
}
