// Package bytesize renders byte counts for human-readable explanations. It uses
// binary units because allocated storage is reported in blocks.
package bytesize

import "fmt"

// Format renders value with one decimal place from KiB upward.
func Format(value int64) string {
	const unit = 1024
	if value > -unit && value < unit {
		return fmt.Sprintf("%d B", value)
	}
	size := float64(value)
	for _, suffix := range []string{"KiB", "MiB", "GiB", "TiB"} {
		size /= unit
		if size > -unit && size < unit {
			return fmt.Sprintf("%.1f %s", size, suffix)
		}
	}
	return fmt.Sprintf("%.1f PiB", size/unit)
}

// FormatRange renders a savings range, collapsing equal bounds.
func FormatRange(low, high int64) string {
	if low == high {
		return Format(low)
	}
	return fmt.Sprintf("%s–%s", Format(low), Format(high))
}
