package top

import (
	"bytes"
	"database/sql"
	"fmt"
	"regexp"
	"testing"

	"github.com/jroimartin/gocui"
	"github.com/lesovsky/pgcenter/internal/stat"
	"github.com/lesovsky/pgcenter/internal/view"
)

func TestPageOffset(t *testing.T) {
	tests := []struct {
		name, direction string
		current, size   int
		want            int
	}{
		{name: "down", direction: "down", current: 0, size: 10, want: 10},
		{name: "multiple down", direction: "down", current: 10, size: 10, want: 20},
		{name: "up", direction: "up", current: 20, size: 10, want: 10},
		{name: "up clamps at top", direction: "up", current: 3, size: 10, want: 0},
		{name: "zero page size", direction: "down", current: 0, size: 0, want: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			direction := 1
			if tt.direction == "up" {
				direction = -1
			}
			if got := pageOffset(tt.current, direction, tt.size); got != tt.want {
				t.Fatalf("pageOffset(%d, %d, %d) = %d, want %d", tt.current, direction, tt.size, got, tt.want)
			}
		})
	}
}

func TestRenderedDbstatRowsHonorsFilters(t *testing.T) {
	s := stat.Stat{Pgstat: stat.Pgstat{Result: stat.PGresult{
		Nrows: 3,
		Ncols: 2,
		Values: [][]sql.NullString{
			{{String: "keep", Valid: true}, {String: "one", Valid: true}},
			{{String: "drop", Valid: true}, {String: "two", Valid: true}},
			{{String: "keep", Valid: true}, {String: "three", Valid: true}},
		},
	}}}

	filters := map[int]*regexp.Regexp{0: regexp.MustCompile("keep")}
	if got := renderedDbstatRows(s, filters); got != 2 {
		t.Fatalf("renderedDbstatRows(filtered) = %d, want 2", got)
	}
	if got := renderedDbstatRows(s, nil); got != 3 {
		t.Fatalf("renderedDbstatRows(unfiltered) = %d, want 3", got)
	}
}

func TestRenderedDbstatRowsIgnoresFiltersOutsideResult(t *testing.T) {
	s := makeRenderResult(2, 20)
	filters := map[int]*regexp.Regexp{7: regexp.MustCompile("keep")}

	if got := renderedDbstatRows(s, filters); got != s.Result.Nrows {
		t.Fatalf("renderedDbstatRows(stale filter) = %d, want %d", got, s.Result.Nrows)
	}

	cfg := makeRenderConfig(2, 10)
	cfg.view.Filters = filters
	var buf bytes.Buffer
	win := visibleColumns(s.Result.Ncols, cfg.view.ColsWidth, 80, 0)
	if err := printStatDataRange(&buf, s, cfg, true, win, 0, -1); err != nil {
		t.Fatal(err)
	}
	if got := bytes.Count(buf.Bytes(), []byte("\n")); got != s.Result.Nrows {
		t.Fatalf("renderer printed %d rows with stale filter, want %d", got, s.Result.Nrows)
	}
}

func TestRenderDbstatWindowPreservesSnapshotForRedraw(t *testing.T) {
	cfg := makeRenderConfig(2, 10)
	cfg.view.Filters[0] = regexp.MustCompile("suffix$")
	s := makeRenderResult(2, 1)
	s.Result.Values[0][0].String = "long-relation-suffix"

	for redraw := 1; redraw <= 2; redraw++ {
		var buf bytes.Buffer
		if err := renderDbstatWindow(&buf, cfg, s, 80, 0, 1); err != nil {
			t.Fatal(err)
		}
		if got := renderedDbstatRows(s, cfg.view.Filters); got != 1 {
			t.Fatalf("redraw %d changed filtered row count to %d, want 1", redraw, got)
		}
		if got := s.Result.Values[0][0].String; got != "long-relation-suffix" {
			t.Fatalf("redraw %d changed snapshot value to %q", redraw, got)
		}
	}
}

func TestRenderDbstatWindowKeepsHeaderWhenPaged(t *testing.T) {
	cfg := makeRenderConfig(2, 10)
	s := makeRenderResult(2, 20)
	var buf bytes.Buffer

	if err := renderDbstatWindow(&buf, cfg, s, 80, 10, 3); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !bytes.Contains([]byte(out), []byte("col0")) {
		t.Fatal("paged render must keep the header")
	}
	if !bytes.Contains([]byte(out), []byte(fmt.Sprintf("r%d-c0", 10))) {
		t.Fatal("paged render must start at the requested data row")
	}
	if bytes.Contains([]byte(out), []byte("r0-c0")) {
		t.Fatal("paged render must omit rows before the requested offset")
	}
}

func TestRenderDbstatWindowKeepsHorizontalOffset(t *testing.T) {
	cfg := makeRenderConfig(7, 10)
	cfg.scrollOffset = 1
	s := makeRenderResult(7, 20)
	var buf bytes.Buffer

	wantOffset := visibleColumns(s.Result.Ncols, cfg.view.ColsWidth, 40, cfg.scrollOffset).clamped
	if err := renderDbstatWindow(&buf, cfg, s, 40, 10, 3); err != nil {
		t.Fatal(err)
	}
	if cfg.scrollOffset != wantOffset {
		t.Fatalf("vertical paging changed horizontal offset to %d, want %d", cfg.scrollOffset, wantOffset)
	}
	if !bytes.Contains(buf.Bytes(), []byte("col2")) {
		t.Fatal("paged render must keep the horizontally selected columns")
	}
	if bytes.Contains(buf.Bytes(), []byte("col1")) {
		t.Fatal("paged render must not reset the horizontal window")
	}
}

func TestClampVerticalOffset(t *testing.T) {
	tests := []struct {
		name        string
		offset      int
		dataRows    int
		visibleRows int
		want        int
	}{
		{name: "empty", offset: 4, dataRows: 0, visibleRows: 10, want: 0},
		{name: "fits", offset: 2, dataRows: 5, visibleRows: 10, want: 0},
		{name: "exact page", offset: 1, dataRows: 10, visibleRows: 10, want: 1},
		{name: "clamp bottom", offset: 99, dataRows: 20, visibleRows: 10, want: 11},
		{name: "clamp top", offset: -1, dataRows: 20, visibleRows: 10, want: 0},
		{name: "zero height", offset: 3, dataRows: 2, visibleRows: 0, want: 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := clampVerticalOffset(tt.offset, tt.dataRows, tt.visibleRows); got != tt.want {
				t.Fatalf("clampVerticalOffset(%d, %d, %d) = %d, want %d", tt.offset, tt.dataRows, tt.visibleRows, got, tt.want)
			}
		})
	}
}

func TestPageSizeLeavesRoomForHeader(t *testing.T) {
	for _, tt := range []struct {
		height, want int
	}{{20, 19}, {2, 1}, {1, 1}, {0, 1}, {-1, 1}} {
		pageSize := scrollPageSize(tt.height)
		if pageSize != tt.want {
			t.Fatalf("page size for height %d = %d, want %d", tt.height, pageSize, tt.want)
		}
	}
}

func TestScrollPageHandlersRequestRedrawWithoutChangingView(t *testing.T) {
	g := &gocui.Gui{}
	if _, err := g.SetView("dbstat", 0, 0, 80, 21); err != gocui.ErrUnknownView {
		t.Fatal(err)
	}

	cfg := newConfig()
	cfg.redrawCh = make(chan struct{}, 2)
	cfg.viewCh = make(chan view.View, 2)

	if err := scrollPageDown(cfg)(g, nil); err != nil {
		t.Fatal(err)
	}
	if cfg.verticalOffset != 19 {
		t.Fatalf("verticalOffset = %d, want 19", cfg.verticalOffset)
	}
	select {
	case <-cfg.redrawCh:
	default:
		t.Fatal("page down did not request redraw")
	}

	if err := scrollPageUp(cfg)(g, nil); err != nil {
		t.Fatal(err)
	}
	if cfg.verticalOffset != 0 {
		t.Fatalf("verticalOffset after page up = %d, want 0", cfg.verticalOffset)
	}
	select {
	case <-cfg.redrawCh:
	default:
		t.Fatal("page up did not request redraw")
	}

	select {
	case <-cfg.viewCh:
		t.Fatal("page scroll must not send a view update to the collector")
	default:
	}
}
