package pretty

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSize(t *testing.T) {
	testcases := []struct {
		v    float64
		want string
	}{
		{v: 0, want: "0"},
		{v: 512, want: "512B"},
		{v: 9425, want: "9.2K"},
		{v: 425681, want: "415.7K"},
		{v: 512548751, want: "488.8M"},
		{v: 512254851486, want: "477.1G"},
		{v: 512254851486475, want: "465.9T"},
	}

	for _, tc := range testcases {
		assert.Equal(t, tc.want, Size(tc.v))
	}
}

func TestCeil(t *testing.T) {
	testcases := []struct {
		v    float64
		want int
	}{
		{v: 0, want: 0},         // zero stays zero, no panic
		{v: 0.1, want: 1},       // any positive fraction rounds up
		{v: 1.0, want: 1},       // exact integer stays integer
		{v: 1.1, want: 2},       // fraction above integer rounds up
		{v: 9.6, want: 10},      // rounds up past a digit boundary
		{v: 99.0, want: 99},     // exact integer stays
		{v: 99.0001, want: 100}, // tiny fraction still rounds up
		{v: -1.0, want: -1},     // negative exact integer (must not panic)
		{v: -1.5, want: -1},     // ceil of negative rounds toward zero
	}

	for _, tc := range testcases {
		assert.Equal(t, tc.want, Ceil(tc.v), "Ceil(%v)", tc.v)
	}
}

func TestReserveWidth(t *testing.T) {
	testcases := []struct {
		v     int
		width int
		want  string
	}{
		{v: 0, width: 2, want: " 0"},        // padded to reserve
		{v: 7, width: 2, want: " 7"},        // single digit padded
		{v: 42, width: 2, want: "42"},       // exactly fills the reserve
		{v: 100, width: 2, want: "100"},     // wider than reserve: not silently truncated, expands
		{v: 5, width: 5, want: "    5"},     // r/s reserve (5 digits)
		{v: 12345, width: 5, want: "12345"}, // exactly fills 5-digit reserve
		{v: 9, width: 3, want: "  9"},       // max-util reserve (3 digits)
		{v: 100, width: 3, want: "100"},     // 100% util fits exactly in 3-digit reserve
	}

	for _, tc := range testcases {
		got := ReserveWidth(tc.v, tc.width)
		assert.Equal(t, tc.want, got, "ReserveWidth(%d, %d)", tc.v, tc.width)
		// Invariant: result is never shorter than the reserve.
		assert.GreaterOrEqual(t, len(got), tc.width, "ReserveWidth(%d, %d) must be at least reserve wide", tc.v, tc.width)
	}
}

func TestRateUnit(t *testing.T) {
	// Reserve = 4 digits. Disk overflow threshold (MB/s -> GB/s) at 1024 (binary divisor),
	// the value 9999.x still fits the integer reserve (4 digits) so stays MB/s until it
	// would need a 5th digit; we switch the unit once the ceil'd integer no longer fits 4 digits.
	testcases := []struct {
		name   string
		v      float64
		family string
		width  int
		want   string
	}{
		// Disk family: MB/s -> GB/s, divisor 1024.
		{name: "disk below threshold", v: 9998.0, family: FamilyDisk, width: 4, want: "9998MB/s"},
		{name: "disk just under boundary", v: 9999.0, family: FamilyDisk, width: 4, want: "9999MB/s"},
		{name: "disk at overflow boundary", v: 10000.0, family: FamilyDisk, width: 4, want: "  10GB/s"},
		{name: "disk above boundary", v: 20480.0, family: FamilyDisk, width: 4, want: "  20GB/s"},
		{name: "disk small value", v: 5.0, family: FamilyDisk, width: 4, want: "   5MB/s"},
		{name: "disk fraction rounds up", v: 5.1, family: FamilyDisk, width: 4, want: "   6MB/s"},

		// Network family: Mbps -> Gbps, divisor 1000 (decimal, SI network convention).
		{name: "net below threshold", v: 9999.0, family: FamilyNet, width: 4, want: "9999Mbps"},
		{name: "net at overflow boundary", v: 10000.0, family: FamilyNet, width: 4, want: "  10Gbps"},
		{name: "net above boundary", v: 40000.0, family: FamilyNet, width: 4, want: "  40Gbps"},
		{name: "net small value", v: 25.0, family: FamilyNet, width: 4, want: "  25Mbps"},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, RateUnit(tc.v, tc.family, tc.width), "RateUnit(%v, %q, %d)", tc.v, tc.family, tc.width)
		})
	}
}

// TestRateUnit_boundary focuses tightly on threshold-1 / threshold / threshold+1
// for both families, where the suffix switches.
func TestRateUnit_boundary(t *testing.T) {
	// With a 4-digit reserve the largest integer that fits is 9999.
	// threshold = first value whose ceil'd integer needs a 5th digit = 10000.
	type bcase struct {
		v      float64
		family string
		suffix string
	}
	cases := []bcase{
		{v: 9999.0, family: FamilyDisk, suffix: "MB/s"},  // threshold-1: still base unit
		{v: 10000.0, family: FamilyDisk, suffix: "GB/s"}, // threshold: switched
		{v: 10001.0, family: FamilyDisk, suffix: "GB/s"}, // threshold+1: switched
		{v: 9999.0, family: FamilyNet, suffix: "Mbps"},
		{v: 10000.0, family: FamilyNet, suffix: "Gbps"},
		{v: 10001.0, family: FamilyNet, suffix: "Gbps"},
	}
	for _, c := range cases {
		got := RateUnit(c.v, c.family, 4)
		assert.True(t, strings.HasSuffix(got, c.suffix),
			"RateUnit(%v, %q) = %q, want suffix %q", c.v, c.family, got, c.suffix)
	}
}

// TestRateUnit_property walks a wide range and asserts the invariant: the numeric
// part of the output always fits within the reserved number of digits (the layout
// never silently breaks). Precedent: visibleColumns walk-test in [009].
func TestRateUnit_property(t *testing.T) {
	const width = 4
	// Realistic range: a single unit step (MB/s->GB/s) keeps the value within reserve up
	// to ~9999 GB/s, i.e. ~9999*1024 MB/s for disk and ~9999*1000 Mbps for net. Within this
	// range the numeric part must always fit the reserve. Beyond it (theoretical max) the
	// behavior is documented as deterministic widen, not silent break.
	for _, family := range []string{FamilyDisk, FamilyNet} {
		var top float64 = 9999 * 1000 // smaller (decimal) ceiling covers both families
		for v := 0.0; v <= top; v = v*1.7 + 1 {
			got := RateUnit(v, family, width)
			// Strip the suffix to isolate the numeric (possibly space-padded) field.
			numeric := got
			for _, suf := range []string{"MB/s", "GB/s", "Mbps", "Gbps"} {
				if strings.HasSuffix(numeric, suf) {
					numeric = strings.TrimSuffix(numeric, suf)
					break
				}
			}
			digits := strings.TrimSpace(numeric)
			assert.LessOrEqualf(t, len(digits), width,
				"RateUnit(%g, %q) numeric part %q exceeds reserve %d (output=%q)",
				v, family, digits, width, got)
			// And the field is padded to exactly the reserve width (layout stays static).
			assert.Equalf(t, width, len(numeric),
				"RateUnit(%g, %q) numeric field %q not padded to reserve %d (output=%q)",
				v, family, numeric, width, got)
		}
	}
}
