package messy_calc

// Divide returns a / b and guards against divide-by-zero.
func Divide(a, b float64) float64 {
	if b == 0 {
		return 0
	}
	return a / b
}
