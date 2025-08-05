package office

// Helper functions shared across office document readers

// ptrFloat64 returns a pointer to a float64 value
func ptrFloat64(f float64) *float64 {
	return &f
}

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
