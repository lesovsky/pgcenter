package top

import (
	"fmt"
	"github.com/jroimartin/gocui"
	"github.com/lesovsky/pgcenter/internal/math"
	"github.com/lesovsky/pgcenter/internal/query"
	"github.com/lesovsky/pgcenter/internal/view"
	"regexp"
	"strconv"
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
func setFilter(answer string, view view.View) string {
	// Clear used pattern if empty string is entered.
	if answer == "\n" || answer == "" {
		delete(view.Filters, view.OrderKey)
		return "Filters: regular expression cleared"
	}

	// Compile regexp and store to filters.
	re, err := regexp.Compile(answer)
	if err != nil {
		return fmt.Sprintf("Filters: %s", err)
	}

	view.Filters[view.OrderKey] = re
	return "Filters: ok"
}

// switchViewTo switches from current view to requested using high-level logic.
func switchViewTo(app *app, c string) func(g *gocui.Gui, _ *gocui.View) error {
	return func(g *gocui.Gui, _ *gocui.View) error {
		// in case of switching to pg_stat_statements and it isn't available - keep current view
		if !app.postgresProps.ExtPGSSAvail && c == "statements" {
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
func changeQueryAge(answer string, config *config) string {
	// Reset threshold if empty answer.
	if answer == "" {
		answer = "00:00:00"
	}

	// Parse user input.
	err := parseHumanTimeString(answer)
	if err != nil {
		return fmt.Sprintf("Activity age: do nothing, %s", err.Error())
	}

	// Remember current age to restore it if formatting new query will fail.
	fallbackAge := config.queryOptions.QueryAgeThresh

	// Update query options and format activity query.
	config.queryOptions.QueryAgeThresh = answer
	q, err := query.Format(config.view.QueryTmpl, config.queryOptions)
	if err != nil {
		config.queryOptions.QueryAgeThresh = fallbackAge // restore fallback
		return fmt.Sprintf("Activity age: do nothing, %s", err.Error())
	}

	// Update query and view.
	config.view.Query = q
	config.viewCh <- config.view

	return "Activity age: set " + answer
}

// parseHumanTimeString parses time in human-readable format and validates its correctness.
func parseHumanTimeString(t string) error {
	pattern := `^([0-9]{1,2}):([0-9]{1,2}):([0-9]{1,2})(\.[0-9]{1,6})?$`
	re := regexp.MustCompile(pattern)

	if !re.MatchString(t) {
		return fmt.Errorf("invalid input")
	}

	parts := re.FindStringSubmatch(t)
	if len(parts) != 5 {
		return fmt.Errorf("invalid input")
	}

	var hour, min, sec, msec int
	if parts[4] == "" {
		_, err := fmt.Sscanf(t, "%d:%d:%d", &hour, &min, &sec)
		if err != nil {
			return err
		}
	} else {
		_, err := fmt.Sscanf(t, "%d:%d:%d.%d", &hour, &min, &sec, &msec)
		if err != nil {
			return err
		}
	}

	if (hour < 0 || hour > 23) || (min < 0 || min > 59) || (sec < 0 || sec > 59) || (msec < 0 || msec > 999999) {
		return fmt.Errorf("invalid input")
	}

	return nil
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
func changeRefresh(answer string, config *config) string {
	if answer == "" {
		return "Refresh: do nothing"
	}

	interval, err := strconv.Atoi(answer)
	if err != nil {
		return "Refresh: do nothing, invalid input"
	}

	if interval < 1 || interval > 300 {
		return "Refresh: input value should be between 1 and 300"
	}

	// Set refresh interval, send it to stats channel and reset interval in the view.
	// Refresh interval should not be saved as a per-view setting. It's used as a setting for stats goroutine.
	config.view.Refresh = time.Duration(interval) * time.Second
	config.viewCh <- config.view
	config.view.Refresh = 0

	return "Refresh: ok"
}
