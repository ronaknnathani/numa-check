package v1

import (
	"math"
	"strconv"
)

// FormatCPUQuantity formats a CPU cores value as a k8s resource.Quantity string.
// Whole-core values render as "<N>"; fractional values round to the nearest
// millicore and render as "<N>m".
func FormatCPUQuantity(cores float64) string {
	milli := int64(math.Round(cores * 1000))
	if milli%1000 == 0 {
		return strconv.FormatInt(milli/1000, 10)
	}
	return strconv.FormatInt(milli, 10) + "m"
}

// FormatMemoryQuantity formats a byte count as a k8s resource.Quantity string
// using the largest binary IEC suffix (Ki/Mi/Gi/Ti/Pi/Ei) that divides cleanly.
// Falls back to a raw decimal byte count when no suffix matches.
func FormatMemoryQuantity(bytes int64) string {
	if bytes <= 0 {
		return strconv.FormatInt(bytes, 10)
	}
	suffixes := []struct {
		unit  int64
		label string
	}{
		{1 << 60, "Ei"},
		{1 << 50, "Pi"},
		{1 << 40, "Ti"},
		{1 << 30, "Gi"},
		{1 << 20, "Mi"},
		{1 << 10, "Ki"},
	}
	for _, s := range suffixes {
		if bytes%s.unit == 0 {
			return strconv.FormatInt(bytes/s.unit, 10) + s.label
		}
	}
	return strconv.FormatInt(bytes, 10)
}

// FormatIntQuantity formats a non-negative integer count as a Quantity string.
func FormatIntQuantity(n int64) string {
	return strconv.FormatInt(n, 10)
}
