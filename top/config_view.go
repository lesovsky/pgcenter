package top

import (
	"fmt"
	"github.com/jroimartin/gocui"
	"github.com/lesovsky/pgcenter/internal/query"
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

// orderKeyLeft handles 'KeyArrowLeft' button and switches sort order to left column.
func orderKeyLeft(config *config) func(_ *gocui.Gui, _ *gocui.View) error {
	return func(_ *gocui.Gui, _ *gocui.View) error {
		config.view.OrderKey--
		if config.view.OrderKey < 0 {
			config.view.OrderKey = config.view.Ncols - 1
		}

		config.viewCh <- config.view
		return nil
	}
}

// orderKeyRight handles 'KeyArrowRight' button and switches sort order to right column.
func orderKeyRight(config *config) func(_ *gocui.Gui, _ *gocui.View) error {
	return func(_ *gocui.Gui, _ *gocui.View) error {
		config.view.OrderKey++
		if config.view.OrderKey >= config.view.Ncols {
			config.view.OrderKey = 0
		}

		config.viewCh <- config.view
		return nil
	}
}

// Increase or decrease width of an active column
func changeWidth(config *config, d int) func(_ *gocui.Gui, _ *gocui.View) error {
	return func(g *gocui.Gui, v *gocui.View) error {
		var width int
		cidx := config.view.OrderKey                        // index of an active column
		clen := len(config.view.Cols[config.view.OrderKey]) // length of the column's name

		// set new width
		switch d {
		case colsWidthIncr:
			width = config.view.ColsWidth[cidx] + colsWidthStep
		case colsWidthDecr:
			width = config.view.ColsWidth[cidx] - colsWidthStep
		default:
			width = config.view.ColsWidth[cidx] // should never be here.
		}

		// new width should not be less than column's name or longer than defined limit
		switch {
		case width < clen:
			width = clen
		case width > colsWidthMax:
			width = colsWidthMax
		}

		config.view.ColsWidth[cidx] = width

		config.viewCh <- config.view
		return nil
	}
}

// switchSortOrder handles switching sort order of active column from descend to ascend and vice versa.
func switchSortOrder(config *config) func(g *gocui.Gui, _ *gocui.View) error {
	return func(g *gocui.Gui, _ *gocui.View) error {
		config.view.OrderDesc = !config.view.OrderDesc
		printCmdline(g, "Switch sort order")

		config.viewCh <- config.view
		return nil
	}
}

// Set filter pattern for current column
func setFilter(g *gocui.Gui, v *gocui.View, answer string, view view.View) {
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

// TODO: имеет смысл switchContextTo переписать так чтобы под капотом оно вызывало viewSwitchHandler.

// Switch from context unit to another one
func switchContextTo(app *app, c string) func(g *gocui.Gui, v *gocui.View) error {
	return func(g *gocui.Gui, v *gocui.View) error {
		// in case of switching to pg_stat_statements and it isn't available - keep current stats context
		if app.postgresProps.ExtPGSSAvail == false && c == "statements" {
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

		app.config.viewCh <- app.config.view

		printCmdline(g, app.config.view.Msg)

		app.doUpdate <- 1
		return nil
	}
}

// viewSwitchHandler switch current view to specified.
func viewSwitchHandler(config *config, c string) {
	// Save current active view, select target pg_stat_statements view and send it to channel.
	config.views[config.view.Name] = config.view
	config.view = config.views[c]
	config.viewCh <- config.view
}

// A toggle to show system tables stats
func toggleSysTables(config *config) func(g *gocui.Gui, _ *gocui.View) error {
	return func(g *gocui.Gui, _ *gocui.View) error {
		name := config.view.Name
		if name != "tables" && name != "indexes" && name != "sizes" {
			return nil
		}

		switch config.queryOptions.ViewType {
		case "user":
			config.queryOptions.ViewType = "all"
		case "all":
			config.queryOptions.ViewType = "user"
		default: // never should be here, but paranoia check
			config.queryOptions.ViewType = "user"
		}

		for _, t := range []string{"tables", "indexes", "sizes"} {
			q, err := query.PrepareQuery(config.views[t].QueryTmpl, config.queryOptions)
			if err != nil {
				return err
			}
			v := config.views[t]
			v.Query = q
			config.views[t] = v
		}

		config.view = config.views[name]
		config.viewCh <- config.view

		printCmdline(g, "Show relations: "+config.queryOptions.ViewType)
		return nil
	}
}

// Change age threshold for queries and transactions (pg_stat_activity only)
func changeQueryAge(g *gocui.Gui, v *gocui.View, answer string, config *config) {
	answer = strings.TrimPrefix(v.Buffer(), dialogPrompts[dialogChangeAge])
	answer = strings.TrimSuffix(answer, "\n")

	if answer == "" {
		printCmdline(g, "Reset to default - 00:00:00.")
		config.queryOptions.QueryAgeThresh = "00:00:00"
	} else {
		var hour, min, sec, msec int
		n, err := fmt.Sscanf(answer, "%d:%d:%d.%d", &hour, &min, &sec, &msec)
		if (n < 3 && err != nil) || ((hour > 23) || (min > 59) || (sec > 59) || (msec > 999999)) {
			printCmdline(g, "Nothing to do. Invalid input.")
			return
		}
		config.queryOptions.QueryAgeThresh = answer
	}

	q, err := query.PrepareQuery(config.view.QueryTmpl, config.queryOptions)
	if err != nil {
		printCmdline(g, "Nothing to do. Failed: %s", err.Error())
		return
	}

	config.view.Query = q
	config.viewCh <- config.view
}

// A toggle to show 'idle' connections (pg_stat_activity only)
func toggleIdleConns(config *config) func(g *gocui.Gui, _ *gocui.View) error {
	return func(g *gocui.Gui, _ *gocui.View) error {
		if config.view.Name != "activity" {
			return nil
		}

		config.queryOptions.ShowNoIdle = !config.queryOptions.ShowNoIdle

		q, err := query.PrepareQuery(config.view.QueryTmpl, config.queryOptions)
		if err != nil {
			return err
		}

		config.view.Query = q
		config.viewCh <- config.view

		if config.queryOptions.ShowNoIdle {
			printCmdline(g, "Show idle connections: off.")
		} else {
			printCmdline(g, "Show idle connections: on.")
		}

		//doUpdate <- 1
		return nil
	}
}
