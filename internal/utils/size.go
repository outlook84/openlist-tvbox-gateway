package utils

import "fmt"

func FormatSize(size int64) string {
	if size < 0 {
		size = 0
	}
	units := []string{"B", "KB", "MB", "GB", "TB"}
	value := float64(size)
	unit := 0
	for value >= 1024 && unit < len(units)-1 {
		value /= 1024
		unit++
	}
	if unit == 0 {
		return fmt.Sprintf("%d%s", size, units[unit])
	}
	return fmt.Sprintf("%.2f%s", value, units[unit])
}
