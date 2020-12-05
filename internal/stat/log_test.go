package stat

import (
	"github.com/lesovsky/pgcenter/internal/postgres"
	"github.com/stretchr/testify/assert"
	"os"
	"testing"
)

func TestLogfile(t *testing.T) {
	l := Logfile{Path: "./testdata/log/postgresql.log"}

	// Test log opening
	assert.NoError(t, l.Open())
	assert.NotNil(t, l.File)

	// Test reading
	buf, err := l.Read(18, 3000)
	assert.NoError(t, err)
	assert.NotNil(t, buf)
	assert.Contains(t, string(buf), "2020-12-05 00:03:46 +05 [1361]: [6517-1] LOG:")
	assert.Contains(t, string(buf), "2020-12-05 10:01:20 +05 [1361]: [6756-1] LOG:")

	// Test close
	assert.NoError(t, l.Close())

	// Test opening unknown log
	l = Logfile{Path: "./testdata/log/invalid.log"}
	assert.Error(t, l.Open())
}

func TestReadLogPath(t *testing.T) {
	conn, err := postgres.NewTestConnect()
	assert.NoError(t, err)

	// Test PG96 and older
	logfile, err := GetPostgresCurrentLogfile(conn, 96000)
	assert.NoError(t, err)
	assert.NotEqual(t, "", logfile)

	// Test PG10 and newer
	logfile, err = GetPostgresCurrentLogfile(conn, 100000)
	assert.NoError(t, err)
	assert.NotEqual(t, "", logfile)
}

func Test_lookupPostgresLogfile(t *testing.T) {
	conn, err := postgres.NewTestConnect()
	assert.NoError(t, err)

	s, err := lookupPostgresLogfile(conn)
	assert.NoError(t, err)
	assert.NotEqual(t, "", s)
}

func Test_assemblePostgresLogfile(t *testing.T) {
	_, err := os.Create("/tmp/test-000000.log")
	assert.NoError(t, err)
	_, err = os.Create("/tmp/test-123456.log")
	assert.NoError(t, err)

	defer func() {
		assert.NoError(t, os.Remove("/tmp/test-000000.log"))
		assert.NoError(t, os.Remove("/tmp/test-123456.log"))
	}()

	testcases := []struct {
		datadir     string
		logdir      string
		logfilename string
		startTime   string
		timezone    string
		want        string
	}{
		{
			datadir: "/tmp", logdir: "/var/log/postgresql", logfilename: "postgresql.log",
			want: "/var/log/postgresql/postgresql.log",
		},
		{
			datadir: "/tmp", logdir: "pg_log", logfilename: "postgresql.log",
			want: "/tmp/pg_log/postgresql.log",
		},
		{
			datadir: "/tmp", logdir: "/tmp", logfilename: "test-%H%M%S.log",
			startTime: "123456",
			want:      "/tmp/test-123456.log",
		},
		{
			datadir: "/tmp", logdir: "/tmp", logfilename: "test-%H%M%S.log",
			startTime: "111111",
			want:      "/tmp/test-000000.log",
		},
		{
			// case with log specified but which is not exists
			datadir: "/tmp", logdir: "/tmp", logfilename: "test-%y%m%d.log",
			want: "",
		},
	}

	for _, tc := range testcases {
		got := assemblePostgresLogfile(tc.datadir, tc.logdir, tc.logfilename, tc.startTime, tc.timezone)
		assert.Equal(t, tc.want, got)
	}
}
