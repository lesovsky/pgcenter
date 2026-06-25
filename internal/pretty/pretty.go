package pretty

import (
	"fmt"
	stdmath "math"
)

// Size returns human-readable value of passed size in bytes
func Size(v float64) string {
	switch {
	case v == 0:
		return "0"
	case v < 1024:
		return fmt.Sprintf("%.0fB", v)
	case v < 1048576:
		return fmt.Sprintf("%.1fK", v/1024)
	case v < 1073741824:
		return fmt.Sprintf("%.1fM", v/1048576)
	case v < 1099511627776:
		return fmt.Sprintf("%.1fG", v/1073741824)
	default:
		return fmt.Sprintf("%.1fT", v/1099511627776)
	}
}

// Rate-unit families for RateUnit. Disk streams are reported in MB/s (binary, like Size),
// network streams in Mbps (decimal, the nicstat panel convention — see printNetdev).
const (
	FamilyDisk = "disk"
	FamilyNet  = "net"
)

// Ceil rounds a float64 up to the nearest integer (the ceiling) and returns it as int.
// Verbose mode shows whole-number aggregates rounded up, whereas full panels show
// fractional values. Net-new: the repository had no math.Ceil wrapper anywhere.
func Ceil(v float64) int {
	return int(stdmath.Ceil(v))
}

// ReserveWidth formats an integer into a right-aligned, space-padded fixed-width field.
// The layout (column position) stays static and only the digits change — the same %Nd
// pattern already used in printSysstat/printPgstat (e.g. %6d, %3d), extracted into a
// reusable helper. If the value is wider than the reserve, the field deterministically
// widens (Go's %*d never truncates) rather than silently breaking the layout.
func ReserveWidth(v int, width int) string {
	return fmt.Sprintf("%*d", width, v)
}

// RateUnit formats a rate value into a fixed-digit-reserve field with a unit suffix,
// switching to the next higher unit when the rounded integer no longer fits the reserve.
// family selects the unit pair and divisor:
//   - FamilyDisk: MB/s -> GB/s, divisor 1024 (binary, consistent with Size)
//   - FamilyNet:  Mbps -> Gbps, divisor 1000 (decimal SI, the nicstat Mbps convention)
//
// The value is rounded up (ceil) before width checks so verbose stays whole-number. On
// overflow the value is divided once, the unit is promoted, and the smaller number is
// laid back into the reserve. At theoretical extremes the divided value may still exceed
// the reserve; the field then widens deterministically (ReserveWidth never truncates).
func RateUnit(v float64, family string, width int) string {
	field, unit := rateUnitParts(v, family, width)
	return field + unit
}

// RateUnitPrefixed is the read/write-prefixed companion to RateUnit: it returns the same
// fixed-digit-reserve field and dynamic unit, but with " " + prefix inserted between the
// digits and the unit (e.g. "9999 rMB/s", "  10 wGB/s"). The verbose disk/net rows use it
// to mark read vs write streams. It shares the exact boundary/overflow logic of RateUnit.
func RateUnitPrefixed(v float64, family, prefix string, width int) string {
	field, unit := rateUnitParts(v, family, width)
	return field + " " + prefix + unit
}

// rateUnitParts computes the shared building blocks of RateUnit and RateUnitPrefixed: the
// base/high/divisor selection, the maxFit reserve, the ceil rounding, and the reserve-width
// formatting. It returns the numeric field (e.g. "9999", "  10") and the resolved unit
// (e.g. "MB/s", "GB/s") separately, with no separator or prefix.
func rateUnitParts(v float64, family string, width int) (field, unit string) {
	base, high := "MB/s", "GB/s"
	var divisor float64 = 1024
	if family == FamilyNet {
		base, high = "Mbps", "Gbps"
		divisor = 1000
	}

	// Largest integer that fits the reserve, e.g. width 4 -> 9999.
	maxFit := 1
	for i := 0; i < width; i++ {
		maxFit *= 10
	}
	maxFit-- // 10^width - 1

	if Ceil(v) <= maxFit {
		return ReserveWidth(Ceil(v), width), base
	}
	return ReserveWidth(Ceil(v/divisor), width), high
}
