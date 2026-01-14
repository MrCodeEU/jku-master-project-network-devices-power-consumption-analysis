//go:build !windows
// +build !windows

package loadgen

import (
	"time"
)

// PreciseSleep performs a sleep on non-Windows platforms
// On Linux/Mac, time.Sleep is already quite precise
func PreciseSleep(duration time.Duration) {
	if duration <= 0 {
		return
	}
	time.Sleep(duration)
}
