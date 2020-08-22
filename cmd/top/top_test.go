package top

import (
	"github.com/stretchr/testify/assert"
	"strconv"
	"testing"
)

func Test_handleExtraArgs(t *testing.T) {
	var testcases = []struct {
		desc       string
		opts       *options
		args       []string
		wantdbname string
		wantuser   string
	}{
		{
			desc:       "dbname and user are not specified",
			opts:       &options{host: "127.0.0.1", port: 5432},
			args:       []string{},
			wantdbname: "", wantuser: "",
		},
		{
			desc:       "dbname specified as argument",
			opts:       &options{host: "127.0.0.1", port: 5432},
			args:       []string{"newdb"},
			wantdbname: "newdb", wantuser: "",
		},
		{
			desc:       "dbname and user specified as argument",
			opts:       &options{host: "127.0.0.1", port: 5432},
			args:       []string{"newdb", "newuser"},
			wantdbname: "newdb", wantuser: "newuser",
		},
		{
			desc:       "dbname specified as a parameter's values",
			opts:       &options{host: "127.0.0.1", port: 5432, dbname: "postgres"},
			args:       []string{"newdb"},
			wantdbname: "postgres", wantuser: "",
		},
		{
			desc:       "dbname and user are specified as a parameter's values",
			opts:       &options{host: "127.0.0.1", port: 5432, dbname: "postgres", user: "postgres"},
			args:       []string{"newdb", "newuser"},
			wantdbname: "postgres", wantuser: "postgres",
		},
	}

	for i, tc := range testcases {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			tc.opts.handleExtraArgs(tc.args)
			assert.Equal(t, tc.wantuser, tc.opts.user)
			assert.Equal(t, tc.wantdbname, tc.opts.dbname)
		})
	}
}
