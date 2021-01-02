package postgres

import (
	"github.com/stretchr/testify/assert"
	"strconv"
	"testing"
)

func TestConnectionOptions_ParseExtraArgs(t *testing.T) {
	var testcases = []struct {
		desc       string
		opts       *ConnectionOptions
		args       []string
		wantdbname string
		wantuser   string
	}{
		{
			desc:       "dbname and user are not specified",
			opts:       &ConnectionOptions{Host: "127.0.0.1", Port: 5432},
			args:       []string{},
			wantdbname: "", wantuser: "",
		},
		{
			desc:       "dbname specified as argument",
			opts:       &ConnectionOptions{Host: "127.0.0.1", Port: 5432},
			args:       []string{"newdb"},
			wantdbname: "newdb", wantuser: "",
		},
		{
			desc:       "dbname and user specified as argument",
			opts:       &ConnectionOptions{Host: "127.0.0.1", Port: 5432},
			args:       []string{"newdb", "newuser"},
			wantdbname: "newdb", wantuser: "newuser",
		},
		{
			desc:       "dbname specified as a parameter's values",
			opts:       &ConnectionOptions{Host: "127.0.0.1", Port: 5432, Dbname: "postgres"},
			args:       []string{"newdb"},
			wantdbname: "postgres", wantuser: "",
		},
		{
			desc:       "dbname and user are specified as a parameter's values",
			opts:       &ConnectionOptions{Host: "127.0.0.1", Port: 5432, Dbname: "postgres", User: "postgres"},
			args:       []string{"newdb", "newuser"},
			wantdbname: "postgres", wantuser: "postgres",
		},
	}

	for i, tc := range testcases {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			tc.opts.ParseExtraArgs(tc.args)
			assert.Equal(t, tc.wantuser, tc.opts.User)
			assert.Equal(t, tc.wantdbname, tc.opts.Dbname)
		})
	}
}
