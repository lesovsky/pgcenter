package align

import (
	"database/sql"
	"github.com/lesovsky/pgcenter/internal/stat"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestSetAlign(t *testing.T) {
	testcases := []struct {
		res         stat.PGresult
		limit       int
		dynamic     bool
		wantcols    []string
		wantwidthes map[int]int
	}{
		{
			// When no rows, width is equal to colnames.
			res: stat.PGresult{
				Valid: true, Ncols: 2, Nrows: 0, Cols: []string{"col11", "col123"},
				Values: [][]sql.NullString{},
			},
			dynamic: false, limit: 1000, wantcols: []string{"col11", "col123"}, wantwidthes: map[int]int{0: 5, 1: 6},
		},
		{
			// When values and colnames are short, width is expanded to 8 chars.
			res: stat.PGresult{
				Valid: true, Ncols: 2, Nrows: 1, Cols: []string{"col12", "col123"},
				Values: [][]sql.NullString{
					{{String: "123456", Valid: true}, {String: "123456", Valid: true}},
				},
			},
			dynamic: false, limit: 1000, wantcols: []string{"col12", "col123"}, wantwidthes: map[int]int{0: 8, 1: 8},
		},
		{
			// When values longer than 8 chars but shorter than 16, width truncated to 8 chars.
			res: stat.PGresult{
				Valid: true, Ncols: 2, Nrows: 1, Cols: []string{"col123", "col12345"},
				Values: [][]sql.NullString{
					{{String: "123456789012", Valid: true}, {String: "45879812", Valid: true}},
				},
			},
			dynamic: false, limit: 1000, wantcols: []string{"col123", "col12345"}, wantwidthes: map[int]int{0: 8, 1: 8},
		},
		{
			// When values longer than 16 chars, width expanded to their length.
			res: stat.PGresult{
				Valid: true, Ncols: 2, Nrows: 1, Cols: []string{"col123", "col12345"},
				Values: [][]sql.NullString{
					{{String: "12345678901234511", Valid: true}, {String: "45879812", Valid: true}},
				},
			},
			dynamic: false, limit: 1000, wantcols: []string{"col123", "col12345"}, wantwidthes: map[int]int{0: 17, 1: 8},
		},
		{
			// For last column, width is equal to specified limit.
			res: stat.PGresult{
				Valid: true, Ncols: 2, Nrows: 1, Cols: []string{"col123", "col12345"},
				Values: [][]sql.NullString{
					{{String: "123456", Valid: true}, {String: "458798sadad12", Valid: true}},
				},
			},
			dynamic: false, limit: 1000, wantcols: []string{"col123", "col12345"}, wantwidthes: map[int]int{0: 8, 1: 1000},
		},
		{
			// For last column when limit is not specified, width is equal to value length.
			res: stat.PGresult{
				Valid: true, Ncols: 2, Nrows: 2, Cols: []string{"col123", "col12345"},
				Values: [][]sql.NullString{
					{{String: "123456", Valid: true}, {String: "458798sadad12", Valid: true}},
					{{String: "45864", Valid: true}, {String: "4587524458", Valid: true}},
				},
			},
			dynamic: false, limit: 1, wantcols: []string{"col123", "col12345"}, wantwidthes: map[int]int{0: 8, 1: 13},
		},
		{
			// For non-last column when value longer than limit, width equal to value length.
			res: stat.PGresult{
				Valid: true, Ncols: 2, Nrows: 2, Cols: []string{"col123", "col12345"},
				Values: [][]sql.NullString{
					{{String: "12345dfqassdas26", Valid: true}, {String: "425418", Valid: true}},
					{{String: "45864", Valid: true}, {String: "15487", Valid: true}},
				},
			},
			dynamic: false, limit: 1, wantcols: []string{"col123", "col12345"}, wantwidthes: map[int]int{0: 8, 1: 8},
		},
		{
			// for dynamic width, width is equal to max length among all values in column.
			res: stat.PGresult{
				Valid: true, Ncols: 2, Nrows: 2, Cols: []string{"col123", "col12345"},
				Values: [][]sql.NullString{
					{{String: "12345dfqassdas26", Valid: true}, {String: "425418", Valid: true}},
					{{String: "45864", Valid: true}, {String: "15487", Valid: true}},
				},
			},
			dynamic: true, limit: 1000, wantcols: []string{"col123", "col12345"}, wantwidthes: map[int]int{0: 16, 1: 1000},
		},
	}

	for _, tc := range testcases {
		widthes, cols, err := SetAlign(tc.res, tc.limit, tc.dynamic)
		assert.NoError(t, err)
		assert.Equal(t, tc.wantwidthes, widthes)
		assert.Equal(t, tc.wantcols, cols)
	}
}

func Test_aligningIsLessThanColname(t *testing.T) {
	testcases := []struct {
		valuelen   int
		colnamelen int
		currwidth  int
		want       bool
	}{
		// returns TRUE if passed non-empty values, but if its length less than length of colnames
		{valuelen: 8, colnamelen: 10, currwidth: 6, want: true},
		{valuelen: 8, colnamelen: 6, currwidth: 8, want: false},
		{valuelen: 0, colnamelen: 6, currwidth: 5, want: false},
	}

	for _, tc := range testcases {
		assert.Equal(t, tc.want, aligningIsLessThanColname(tc.valuelen, tc.colnamelen, tc.currwidth))
	}
}

func Test_aligningIsMoreThanColname(t *testing.T) {
	testcases := []struct {
		valuelen   int
		colnamelen int
		currwidth  int
		limit      int
		colidx     int
		ncols      int
		want       bool
	}{
		// returns TRUE if passed non-empty values, but if its length longer than length of colnames
		{valuelen: 8, colnamelen: 6, currwidth: 4, limit: 1000, colidx: 1, ncols: 5, want: true},
		{valuelen: 8, colnamelen: 10, currwidth: 4, limit: 1000, colidx: 1, ncols: 5, want: false}, // valuelen less than colname
		{valuelen: 8, colnamelen: 6, currwidth: 10, limit: 1000, colidx: 1, ncols: 5, want: false}, // valuelen less than width
		{valuelen: 8, colnamelen: 6, currwidth: 4, limit: 1000, colidx: 4, ncols: 5, want: false},  // last column
		{valuelen: 0, colnamelen: 6, currwidth: 4, limit: 1000, colidx: 1, ncols: 5, want: false},  // empty valuelen
	}

	for _, tc := range testcases {
		assert.Equal(t, tc.want, aligningIsMoreThanColname(tc.valuelen, tc.colnamelen, tc.currwidth, tc.limit, tc.colidx, tc.ncols))
	}
}

func Test_aligningIsLengthLessOrEqualWidth(t *testing.T) {
	testcases := []struct {
		valuelen   int
		colnamelen int
		currwidth  int
		want       bool
	}{
		// returns true if length of value or column is less (or equal) than already specified width
		{valuelen: 4, colnamelen: 6, currwidth: 8, want: true},
		{valuelen: 8, colnamelen: 6, currwidth: 6, want: false},
		{valuelen: 0, colnamelen: 6, currwidth: 5, want: false},
		// return vlen <= width && cnlen <= width
	}

	for _, tc := range testcases {
		assert.Equal(t, tc.want, aligningIsLengthLessOrEqualWidth(tc.valuelen, tc.colnamelen, tc.currwidth))
	}
}
