package pretty

import (
	"fmt"
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
