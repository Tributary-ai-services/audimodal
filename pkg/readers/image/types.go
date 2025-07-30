package image

// TextRegion represents a detected text region
type TextRegion struct {
	Text       string
	X          int
	Y          int
	Width      int
	Height     int
	Confidence float64
}

// Helper function
func ptrFloat64(f float64) *float64 {
	return &f
}