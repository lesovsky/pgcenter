package align

import (
	"github.com/lesovsky/pgcenter/internal/math"
	"github.com/lesovsky/pgcenter/internal/stat"
)

const (
	// colsTruncMinLimit is the  minimal allowed value for values truncation
	colsTruncMinLimit = 1
)

// SetAlign method aligns length of values depending of the columns width
func SetAlign(r stat.PGresult, truncLimit int, dynamic bool) (map[int]int, []string) {
	lastColMaxWidthDefault := 8
	lastColTruncLimit := math.Max(truncLimit, colsTruncMinLimit)
	truncLimit = math.Max(truncLimit, colsTruncMinLimit)
	widthes := make(map[int]int)

	// no rows in result, set width using length of a column name and return with error (because not aligned using result's values)
	if len(r.Values) == 0 {
		for colidx, colname := range r.Cols { // walk per-column
			widthes[colidx] = math.Max(len(colname), colsTruncMinLimit)
		}
		return widthes, r.Cols
	}

	/* calculate max length of columns based on the longest value of the column */
	var valuelen, colnamelen int
	for colidx, colname := range r.Cols { // walk per-column
		for rownum := 0; rownum < len(r.Values); rownum++ { // walk through rows
			// Minimum possible value length is 1 (colsTruncMinLimit)
			valuelen = math.Max(len(r.Values[rownum][colidx].String), colsTruncMinLimit)

			// Minimum possible length for columns is 8
			colnamelen = math.Max(len(colname), 8) // eight is a minimal colname length, if column name too short.

			switch {
			// for non-empty values, but for those whose length less than length of colnames, use length based on length of column name, but no longer than already set
			case aligningIsLessThanColname(valuelen, colnamelen, widthes[colidx]):
				widthes[colidx] = colnamelen
			// for non-empty values, but for those whose length longer than length of colnames, use length based on length of value, but no longer than already set
			case aligningIsMoreThanColname(valuelen, colnamelen, widthes[colidx], truncLimit, colidx, r.Ncols):
				// dynamic aligning is used in 'report' when you can't adjust width on the fly
				// fixed aligning is used in 'top' because it's quite uncomfortable when width is changing constantly
				if dynamic {
					widthes[colidx] = valuelen
				} else {
					if valuelen > colnamelen*2 {
						widthes[colidx] = math.Min(valuelen, 32)
					} else {
						widthes[colidx] = valuelen
					}
				}
			// for last column set width using truncation limit
			case colidx == r.Ncols-1:
				// if truncation disabled, use width of the longest value, otherwise use the user-defined truncation limit
				if lastColTruncLimit == colsTruncMinLimit {
					if lastColMaxWidthDefault < valuelen {
						lastColMaxWidthDefault = valuelen
					}
					widthes[colidx] = lastColMaxWidthDefault
				} else {
					widthes[colidx] = truncLimit
				}
			// do nothing if length of value or column is less (or equal) than already specified width
			case aligningIsLengthLessOrEqualWidth(valuelen, colnamelen, widthes[colidx]):

			// for very long values, truncate value and set length limited by truncLimit value,
			case valuelen >= truncLimit:
				r.Values[rownum][colidx].String = r.Values[rownum][colidx].String[:truncLimit-1] + "~"
				widthes[colidx] = truncLimit
				//default:	// default case is used for debug purposes for catching cases that don't meet upper conditions
				//	fmt.Printf("*** DEBUG %s -- %s, %d:%d:%d ***", colname, r.Result[rownum][colnum].String, widthes[colidx], colnamelen, valuelen)
			}
		}
	}
	return widthes, r.Cols
}

// aligningIsLessThanColname is the aligning helper: returns true if passed non-empty values, but if its length less than length of colnames
func aligningIsLessThanColname(vlen, cnlen, width int) bool {
	return vlen > 0 && vlen <= cnlen && vlen >= width
}

// aligningIsMoreThanColname is the aligning helper: returns true if passed non-empty values, but if its length longer than length of colnames
func aligningIsMoreThanColname(vlen, cnlen, width, trunclim, colidx, cols int) bool {
	return vlen > 0 && vlen > cnlen && vlen < trunclim && vlen >= width && colidx < cols-1
}

// aligningIsLengthLessOrEqualWidth is the aligning helper: returns true if length of value or column is less (or equal) than already specified width
func aligningIsLengthLessOrEqualWidth(vlen, cnlen, width int) bool {
	return vlen <= width && cnlen <= width
}
