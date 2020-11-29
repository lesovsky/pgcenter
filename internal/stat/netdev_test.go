package stat

import (
	"github.com/lesovsky/pgcenter/internal/postgres"
	"github.com/stretchr/testify/assert"
	"testing"
)

func Test_readNetdevs(t *testing.T) {
	conn, err := postgres.NewTestConnect()
	assert.NoError(t, err)

	// test "local" reading
	conn.Local = true
	got, err := readNetdevs(conn, false)
	assert.NoError(t, err)
	assert.Greater(t, len(got), 0)

	// test "remote" reading
	conn.Local = false
	//got, err = readNetdevs(conn, true)
	//assert.NoError(t, err)
	//assert.Greater(t, len(got), 0)

	// test "remote", but when schema is not available
	got, err = readNetdevs(conn, false)
	assert.NoError(t, err)
	assert.Equal(t, len(got), 0)
}

func Test_readNetdevsLocal(t *testing.T) {
	testcases := []struct {
		statfile string
		valid    bool
		want     Netdevs
	}{
		{
			statfile: "testdata/proc/netdev.v1.golden",
			valid:    true,
			want: Netdevs{
				Netdev{
					Ifname: "br-1234567",
					Speed:  0, Duplex: 0,
					Rbytes: 197975757, Rpackets: 583782, Rerrs: 10, Rdrop: 20, Rfifo: 30, Rframe: 40, Rcompressed: 50, Rmulticast: 60,
					Tbytes: 8688001214, Tpackets: 1460628, Terrs: 70, Tdrop: 80, Tfifo: 90, Tcolls: 100, Tcarrier: 110, Tcompressed: 120,
					Saturation: 410,
				},
				Netdev{
					Ifname: "wlx1234567",
					Speed:  0, Duplex: 0,
					Rbytes: 19442146228, Rpackets: 14953729, Rerrs: 15, Rdrop: 25, Rfifo: 35, Rframe: 45, Rcompressed: 55, Rmulticast: 65,
					Tbytes: 653429694, Tpackets: 3893477, Terrs: 75, Tdrop: 85, Tfifo: 95, Tcolls: 105, Tcarrier: 115, Tcompressed: 125,
					Saturation: 440,
				},
			},
		},
		{statfile: "testdata/proc/netdev.invalid.1", valid: false},
		{statfile: "testdata/proc/netdev.invalid.2", valid: false},
		{statfile: "testdata/proc/netdev.unknown", valid: false},
	}

	for _, tc := range testcases {
		got, err := readNetdevsLocal(tc.statfile)
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

func Test_readNetdevsRemote(t *testing.T) {
	conn, err := postgres.NewTestConnect()
	assert.NoError(t, err)

	got, err := readNetdevsRemote(conn)
	assert.NoError(t, err)
	assert.Greater(t, len(got), 0)

	// Check device value is not empty
	for i := range got {
		assert.NotEqual(t, got[i].Ifname, "")
	}

	conn.Close()
	_, err = readNetdevsRemote(conn)
	assert.Error(t, err)
}

func Test_countNetdevsUsage(t *testing.T) {
	prev, err := readNetdevsLocal("testdata/proc/netdev.v1.golden")
	assert.NoError(t, err)

	curr, err := readNetdevsLocal("testdata/proc/netdev.v2.golden")
	assert.NoError(t, err)

	// as a workaround copy Uptime value from 'got' because it's read from real /proc/stat.
	for i := range prev {
		prev[i].Uptime = 0
		curr[i].Uptime = 100
	}

	got := countNetdevsUsage(prev, curr, 100)

	want := Netdevs{
		{
			Ifname: "br-1234567", Speed: 0, Duplex: 0,
			Rbytes: 12373, Rpackets: 186, Rerrs: 448.00000000000006, Rdrop: 0, Rfifo: 0, Rframe: 0, Rcompressed: 0, Rmulticast: 0,
			Tbytes: 5.943444e+06, Tpackets: 643, Terrs: 351, Tdrop: 0, Tfifo: 0, Tcolls: 685, Tcarrier: 0, Tcompressed: 0,
			Packets: 2.045239e+06, Raverage: 66.52150537634408, Taverage: 9243.303265940902, Saturation: 2157, Rutil: 0, Tutil: 0, Utilization: 0, Uptime: 0,
		},
		{
			Ifname: "wlx1234567", Speed: 0, Duplex: 0,
			Rbytes: 1.18816895e+08, Rpackets: 125113.00000000001, Rerrs: 39, Rdrop: 0, Rfifo: 0, Rframe: 0, Rcompressed: 0, Rmulticast: 0,
			Tbytes: 1.81552713e+08, Tpackets: 130758, Terrs: 412, Tdrop: 0, Tfifo: 0, Tcolls: 370, Tcarrier: 0, Tcompressed: 0,
			Packets: 1.9103077e+07, Raverage: 949.676652306315, Taverage: 1388.4635203964576, Saturation: 2377, Rutil: 0, Tutil: 0, Utilization: 0, Uptime: 0,
		},
	}

	assert.Equal(t, want, got)
}
