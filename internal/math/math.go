package math

// Min returns minimum of two integers
func Min(a, b int) int {
	if a > b {
		return b
	}
	return a
}

// Max returns maximum of two integers
func Max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
