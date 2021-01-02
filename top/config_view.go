package top

import (
	"fmt"
	"github.com/jroimartin/gocui"
	"github.com/lesovsky/pgcenter/internal/math"
	"github.com/lesovsky/pgcenter/internal/query"
	"github.com/lesovsky/pgcenter/internal/view"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	colsWidthMax  = 256 // max width allowed for column, they can't be wider than that value
	colsWidthStep = 4   // minimal step of changing column's width, 1 is too boring and 4 looks good
)

// orderKeyLeft switches sort order to left column.
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

// orderKeyRight switches sort order to right column.
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

// increaseWidth increases visible width of current column.
func increaseWidth(config *config) func(_ *gocui.Gui, _ *gocui.View) error {
	return func(_ *gocui.Gui, _ *gocui.View) error {
		idx := config.view.OrderKey // index of the current selected column

		// Increase the width using current width. Clamp the value, it should not be greater than max allowed limit.
		config.view.ColsWidth[idx] = math.Min(config.view.ColsWidth[idx]+colsWidthStep, colsWidthMax)

		config.viewCh <- config.view
		return nil
	}
}

// decreaseWidth decreases visible width of current column.
func decreaseWidth(config *config) func(_ *gocui.Gui, _ *gocui.View) error {
	return func(_ *gocui.Gui, _ *gocui.View) error {
		idx := config.view.OrderKey // index of the current selected column

		// Decrease the width using current width. Clamp the value, it should not be less than width of column's name.
		config.view.ColsWidth[idx] = math.Max(config.view.ColsWidth[idx]-colsWidthStep, len(config.view.Cols[idx]))

		config.viewCh <- config.view
		return nil
	}
}

// switchSortOrder switches sort order of current column between DESC and ASC.
func switchSortOrder(config *config) func(g *gocui.Gui, _ *gocui.View) error {
	return func(g *gocui.Gui, _ *gocui.View) error {
		config.view.OrderDesc = !config.view.OrderDesc
		printCmdline(g, "Switch sort order")

		config.viewCh <- config.view
		return nil
	}
}

// setFilter adds pattern for filtering values in the current column.
func setFilter(g *gocui.Gui, buf string, view view.View) {
	answer := strings.TrimPrefix(buf, dialogPrompts[dialogFilter])
	answer = strings.TrimSuffix(answer, "\n")

	// Clear used pattern if empty string is entered.
	if answer == "\n" || answer == "" {
		delete(view.Filters, view.OrderKey)
		printCmdline(g, "Regexp cleared")
		return
	}

	// Compile regexp and store to filters.
	re, err := regexp.Compile(answer)
	if err != nil {
		printCmdline(g, "Do nothing. Failed to compile regexp: %s", err)
		return
	}

	view.Filters[view.OrderKey] = re
}

// switchViewTo switches from current view to requested using high-level logic.
func switchViewTo(app *app, c string) func(g *gocui.Gui, _ *gocui.View) error {
	return func(g *gocui.Gui, _ *gocui.View) error {
		// in case of switching to pg_stat_statements and it isn't available - keep current view
		if app.postgresProps.ExtPGSSAvail == false && c == "statements" {
			printCmdline(g, "NOTICE: pg_stat_statements is not available in this database")
			return nil
		}

		// Switch to requested view.
		switch c {
		case "statements":
			// fall through another switch and select appropriate pg_stat_statements stats
			switch app.config.view.Name {
			case "statements_timings":
				viewSwitchHandler(app.config, "statements_general")
			case "statements_general":
				viewSwitchHandler(app.config, "statements_io")
			case "statements_io":
				viewSwitchHandler(app.config, "statements_temp")
			case "statements_temp":
				viewSwitchHandler(app.config, "statements_local")
			case "statements_local":
				viewSwitchHandler(app.config, "statements_timings")
			default:
				viewSwitchHandler(app.config, "statements_timings")
			}
		case "progress":
			// fall through another switch and select appropriate pg_stat_progress_* stats
			switch app.config.view.Name {
			case "progress_vacuum":
				viewSwitchHandler(app.config, "progress_cluster")
			case "progress_cluster":
				viewSwitchHandler(app.config, "progress_index")
			case "progress_index":
				viewSwitchHandler(app.config, "progress_vacuum")
			default:
				viewSwitchHandler(app.config, "progress_vacuum")
			}
		default:
			viewSwitchHandler(app.config, c)
		}

		printCmdline(g, app.config.view.Msg)
		return nil
	}
}

// viewSwitchHandler is routine handler which switches views and notify channel.
func viewSwitchHandler(config *config, c string) {
	config.views[config.view.Name] = config.view
	config.view = config.views[c]
	config.viewCh <- config.view
}

// toggleSysTables toggles showing system tables/indexes.
func toggleSysTables(config *config) func(g *gocui.Gui, _ *gocui.View) error {
	return func(g *gocui.Gui, _ *gocui.View) error {
		name := config.view.Name
		if name != "tables" && name != "indexes" && name != "sizes" {
			return nil
		}

		// If current view type is 'user' - switch to 'all', and vice versa.
		if config.queryOptions.ViewType == "user" {
			config.queryOptions.ViewType = "all"
		} else {
			config.queryOptions.ViewType = "user"
		}

		// Recreate dependant queries accordingly to new view type.
		for _, t := range []string{"tables", "indexes", "sizes"} {
			q, err := query.Format(config.views[t].QueryTmpl, config.queryOptions)
			if err != nil {
				// TODO: log error
				continue
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

// changeQueryAge changes age threshold for showing queries and transactions (pg_stat_activity only).
func changeQueryAge(g *gocui.Gui, buf string, config *config) {
	answer := strings.TrimPrefix(buf, dialogPrompts[dialogChangeAge])
	answer = strings.TrimSuffix(answer, "\n")

	// Reset threshold if empty answer.
	if answer != "" {
		var hour, min, sec, msec int
		n, err := fmt.Sscanf(answer, "%d:%d:%d.%d", &hour, &min, &sec, &msec)
		if (n < 3 && err != nil) || ((hour > 23) || (min > 59) || (sec > 59) || (msec > 999999)) {
			printCmdline(g, "Nothing to do. Invalid input.")
			return
		}
		config.queryOptions.QueryAgeThresh = answer
	} else {
		printCmdline(g, "Reset to default - 00:00:00.")
		config.queryOptions.QueryAgeThresh = "00:00:00"
	}

	q, err := query.Format(config.view.QueryTmpl, config.queryOptions)
	if err != nil {
		printCmdline(g, "Nothing to do. Failed: %s", err.Error())
		config.queryOptions.QueryAgeThresh = "00:00:00" // reset to default
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

		q, err := query.Format(config.view.QueryTmpl, config.queryOptions)
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

		return nil
	}
}

// changeRefresh changes current refresh interval.
func changeRefresh(g *gocui.Gui, buf string, config *config) {
	answer := strings.TrimPrefix(buf, dialogPrompts[dialogChangeRefresh])
	answer = strings.TrimSuffix(answer, "\n")

	if answer == "" {
		printCmdline(g, "Do nothing. Empty input.")
		return
	}

	interval, err := strconv.Atoi(answer)
	if err != nil {
		printCmdline(g, "Do nothing. Invalid input.")
		return
	}

	if interval < 1 || interval > 300 {
		printCmdline(g, "Value should be between 1 and 300.")
		return
	}

	// Set refresh interval, send it to stats channel and reset interval in the view.
	// Refresh interval should not be saved as a per-view setting. It's used as a setting for stats goroutine.
	config.view.Refresh = time.Duration(interval) * time.Second
	config.viewCh <- config.view
	config.view.Refresh = 0
}
