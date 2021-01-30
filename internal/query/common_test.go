package query

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestSelectActivityStatementsQuery(t *testing.T) {
	testcases := []struct {
		version int
		want    string
	}{
		{version: 120000, want: SelectActivityStatementsPG12},
		{version: 130000, want: SelectActivityStatementsLatest},
	}

	for _, tc := range testcases {
		assert.Equal(t, tc.want, SelectActivityStatementsQuery(tc.version))
	}
}
