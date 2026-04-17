package debug_divide

// Divide returns a / b, guarding against divide-by-zero.
func Divide(a, b float64) float64 {
	if b == 0 {
		return 0
	}
	return a / b
}
