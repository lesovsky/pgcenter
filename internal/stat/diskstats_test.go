package stat

import (
	"github.com/lesovsky/pgcenter/internal/postgres"
	"github.com/stretchr/testify/assert"
	"testing"
)

func Test_readDiskstats(t *testing.T) {
	conn, err := postgres.NewTestConnect()
	assert.NoError(t, err)

	// test "local" reading
	conn.Local = true
	got, err := readDiskstats(conn, false)
	assert.NoError(t, err)
	assert.Greater(t, len(got), 0)

	// test "remote" reading
	conn.Local = false
	got, err = readDiskstats(conn, true)
	assert.NoError(t, err)
	assert.Greater(t, len(got), 0)

	// test "remote", but when schema is not available
	got, err = readDiskstats(conn, false)
	assert.NoError(t, err)
	assert.Equal(t, len(got), 0)
}

//
func Test_readDiskstatsLocal(t *testing.T) {
	testcases := []struct {
		statfile string
		valid    bool
		want     Diskstats
	}{
		{
			statfile: "testdata/proc/diskstats.v1.golden",
			valid:    true,
			want: Diskstats{
				Diskstat{
					Major: 8, Minor: 0, Device: "sda",
					Rcompleted: 364890, Rmerged: 90257, Rsectors: 15905820, Rspent: 98256,
					Wcompleted: 5112729, Wmerged: 4404633, Wsectors: 312756632, Wspent: 3722346,
					Ioinprogress: 0, Tspent: 3986076, Tweighted: 1949872,
				},
				Diskstat{
					Major: 8, Minor: 16, Device: "sdb",
					Rcompleted: 307367, Rmerged: 16786, Rsectors: 12135182, Rspent: 8494946,
					Wcompleted: 84857424, Wmerged: 8238986, Wsectors: 1047928880, Wspent: 1649683162,
					Ioinprogress: 0, Tspent: 114626504, Tweighted: 1553521352,
				},
			},
		},
		{
			statfile: "testdata/proc/diskstats.v2.golden",
			valid:    true,
			want: Diskstats{
				Diskstat{
					Major: 8, Minor: 0, Device: "sda",
					Rcompleted: 365227, Rmerged: 90272, Rsectors: 15921204, Rspent: 98369,
					Wcompleted: 5150316, Wmerged: 4436838, Wsectors: 318033448, Wspent: 3768000,
					Ioinprogress: 0, Tspent: 4015784, Tweighted: 1972664,
					Dcompleted: 391141, Dmerged: 0, Dsectors: 242608824, Dspent: 1702883,
				},
				Diskstat{
					Major: 8, Minor: 16, Device: "sdb",
					Rcompleted: 309617, Rmerged: 16786, Rsectors: 12153678, Rspent: 8579283,
					Wcompleted: 85792400, Wmerged: 8398482, Wsectors: 1060036272, Wspent: 1671783781,
					Ioinprogress: 0, Tspent: 115793700, Tweighted: 1574520404,
					Dcompleted: 0, Dmerged: 0, Dsectors: 0, Dspent: 0,
				},
			},
		},
		{
			statfile: "testdata/proc/diskstats.v3.golden",
			valid:    true,
			want: Diskstats{
				Diskstat{
					Major: 8, Minor: 0, Device: "sda",
					Rcompleted: 365208, Rmerged: 90272, Rsectors: 15920380, Rspent: 98361,
					Wcompleted: 5144476, Wmerged: 4431757, Wsectors: 316780704, Wspent: 3758592,
					Ioinprogress: 0, Tspent: 4010804, Tweighted: 1967660,
					Dcompleted: 390788, Dmerged: 0, Dsectors: 242426032, Dspent: 1702032,
					Fcompleted: 1452785, Fspent: 1458752145,
				},
				Diskstat{
					Major: 8, Minor: 16, Device: "sdb",
					Rcompleted: 309388, Rmerged: 16786, Rsectors: 12151846, Rspent: 8569789,
					Wcompleted: 85675301, Wmerged: 8382129, Wsectors: 1058502704, Wspent: 1669180952,
					Ioinprogress: 0, Tspent: 115656192, Tweighted: 1572052888,
					Dcompleted: 0, Dmerged: 0, Dsectors: 0, Dspent: 0,
					Fcompleted: 4587, Fspent: 12547854,
				},
			},
		},
		{statfile: "testdata/proc/diskstats.invalid", valid: false},
		{statfile: "testdata/proc/diskstats.unknown", valid: false},
	}

	for _, tc := range testcases {
		got, err := readDiskstatsLocal(tc.statfile)
		if tc.valid {
			// as a workaround copy Uptime value from 'got' because it's read from real /proc/stat.
			for i := range got {
				tc.want[i].Uptime = got[i].Uptime
			}

			assert.NoError(t, err)
			assert.Equal(t, tc.want, got)
		} else {
			assert.Error(t, err)
		}
	}
}

func Test_readDiskstatsRemote(t *testing.T) {
	conn, err := postgres.NewTestConnect()
	assert.NoError(t, err)

	got, err := readDiskstatsRemote(conn)
	assert.NoError(t, err)
	assert.Greater(t, len(got), 0)

	// Check device value is not empty
	for i := range got {
		assert.NotEqual(t, got[i].Device, "")
	}

	conn.Close()
	_, err = readDiskstatsRemote(conn)
	assert.Error(t, err)
}

func Test_countDiskstatsUsage(t *testing.T) {
	prev, err := readDiskstatsLocal("testdata/proc/diskstats.v2.golden")
	assert.NoError(t, err)

	curr, err := readDiskstatsLocal("testdata/proc/diskstats.v2.2.golden")
	assert.NoError(t, err)

	// as a workaround copy Uptime value from 'got' because it's read from real /proc/stat.
	for i := range prev {
		prev[i].Uptime = 0
		curr[i].Uptime = 100
	}

	got := countDiskstatsUsage(prev, curr, 100)

	want := Diskstats{
		Diskstat{
			Major: 8, Minor: 0, Device: "sda",
			Rcompleted: 0, Rmerged: 0, Rsectors: 0, Rspent: 0,
			Wcompleted: 79, Wmerged: 2, Wsectors: 37.03515625, Wspent: 0,
			Ioinprogress: 0, Tspent: 0, Tweighted: 0.08,
			Dcompleted: 0, Dmerged: 0, Dsectors: 0, Dspent: 0,
			Completed: 5.515622e+06, Rawait: 0, Wawait: 2.721518987341772, Await: 2.721518987341772, Arqsz: 960.1012658227849, Util: 24,
		},
		Diskstat{
			Major: 8, Minor: 16, Device: "sdb",
			Rcompleted: 0, Rmerged: 0, Rsectors: 0, Rspent: 0,
			Wcompleted: 105, Wmerged: 0, Wsectors: 0.55859375, Wspent: 0,
			Ioinprogress: 0, Tspent: 0, Tweighted: 0.764,
			Dcompleted: 0, Dmerged: 0, Dsectors: 0, Dspent: 0,
			Completed: 8.6102122e+07, Rawait: 0, Wawait: 8.304761904761905, Await: 8.304761904761905, Arqsz: 10.895238095238096, Util: 19.2,
		},
	}

	assert.Equal(t, want, got)
}
