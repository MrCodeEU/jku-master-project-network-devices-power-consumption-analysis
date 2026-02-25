//go:build windows
// +build windows

package loadgen

import (
	"sync"
	"syscall"
	"time"
	"unsafe"
)

var (
	kernel32                      = syscall.NewLazyDLL("kernel32.dll")
	procCreateWaitableTimerExW    = kernel32.NewProc("CreateWaitableTimerExW")
	procSetWaitableTimerEx        = kernel32.NewProc("SetWaitableTimerEx")
	procWaitForSingleObject       = kernel32.NewProc("WaitForSingleObject")
	procQueryPerformanceFrequency = kernel32.NewProc("QueryPerformanceFrequency")
	procQueryPerformanceCounter   = kernel32.NewProc("QueryPerformanceCounter")
	procTimeBeginPeriod           = syscall.NewLazyDLL("winmm.dll").NewProc("timeBeginPeriod")
	procTimeEndPeriod             = syscall.NewLazyDLL("winmm.dll").NewProc("timeEndPeriod")
)

const (
	CREATE_WAITABLE_TIMER_HIGH_RESOLUTION = 0x00000002
	TIMER_ALL_ACCESS                      = 0x1F0003
	INFINITE                              = 0xFFFFFFFF
)

var (
	highResTimerSupported bool
	perfFreq              int64
)

// highResTimerPool provides per-goroutine high-resolution timer handles.
// The old code used a SINGLE shared highResTimer handle across all goroutines,
// which caused a race condition: SetWaitableTimerEx from one goroutine would
// overwrite another's pending timer, and auto-reset WaitForSingleObject would
// steal signals meant for other waiters. This caused wildly inaccurate sleep
// durations under concurrent use (10 workers → ~5x overshoot → ~21% throughput).
var highResTimerPool = sync.Pool{
	New: func() interface{} {
		if !highResTimerSupported {
			return syscall.Handle(0)
		}
		ret, _, _ := procCreateWaitableTimerExW.Call(
			0, 0,
			CREATE_WAITABLE_TIMER_HIGH_RESOLUTION,
			TIMER_ALL_ACCESS,
		)
		if ret != 0 {
			return syscall.Handle(ret)
		}
		return syscall.Handle(0)
	},
}

func init() {
	// Query performance counter frequency
	procQueryPerformanceFrequency.Call(uintptr(unsafe.Pointer(&perfFreq)))

	// Set Windows timer resolution to 1ms for better time.Sleep() behavior as fallback
	procTimeBeginPeriod.Call(1)

	// Probe whether high-resolution waitable timers are supported (Windows 10 1803+)
	ret, _, _ := procCreateWaitableTimerExW.Call(
		0, 0,
		CREATE_WAITABLE_TIMER_HIGH_RESOLUTION,
		TIMER_ALL_ACCESS,
	)
	if ret != 0 {
		highResTimerSupported = true
		syscall.CloseHandle(syscall.Handle(ret)) // close probe handle
	}
}

// highResolutionNow returns the current time using QueryPerformanceCounter
func highResolutionNow() time.Duration {
	var counter int64
	procQueryPerformanceCounter.Call(uintptr(unsafe.Pointer(&counter)))
	// Convert to nanoseconds
	return time.Duration(counter * 1e9 / perfFreq)
}

// preciseSleepWindows implements a hybrid sleep using high-resolution timer + spin-wait.
// Each call obtains its OWN timer handle from a pool so concurrent goroutines
// never interfere with each other.
// Based on https://blog.bearcats.nl/perfect-sleep-function/
func preciseSleepWindows(duration time.Duration) {
	if duration <= 0 {
		return
	}

	target := highResolutionNow() + duration

	if highResTimerSupported && duration > 200*time.Microsecond {
		// Get a dedicated timer handle for this sleep call
		timer := highResTimerPool.Get().(syscall.Handle)
		if timer != 0 {
			// Use high-resolution waitable timer for the bulk of the sleep.
			// Reduced tolerance from 1 ms → 200 µs to cut CPU spin-wait by 5×.
			const toleranceNs = 200000 // 200µs spin tolerance
			const maxTicksNs = 9500000 // 9.5ms max per timer wait

			for {
				remaining := (target - highResolutionNow()).Nanoseconds()
				if remaining <= toleranceNs {
					break
				}

				// Calculate sleep ticks (in 100ns units for Windows)
				ticks := (remaining - toleranceNs) / 100
				if ticks <= 0 {
					break
				}
				if ticks > maxTicksNs*10 {
					ticks = maxTicksNs * 10
				}

				// Negative value means relative time
				dueTime := -ticks
				procSetWaitableTimerEx.Call(
					uintptr(timer),
					uintptr(unsafe.Pointer(&dueTime)),
					0, // lPeriod (0 = one-shot)
					0, // pfnCompletionRoutine
					0, // lpArgToCompletionRoutine
					0, // WakeContext
					0, // TolerableDelay
				)

				procWaitForSingleObject.Call(uintptr(timer), INFINITE)
			}

			// Return timer to pool for reuse
			highResTimerPool.Put(timer)
		}
	} else if duration > time.Millisecond {
		// Fallback: Use regular time.Sleep for the bulk, leaving 500µs for spin
		sleepDuration := duration - 500*time.Microsecond
		if sleepDuration > 0 {
			time.Sleep(sleepDuration)
		}
	}

	// Spin-wait for remaining time to achieve precise timing
	// With 200µs tolerance this is brief and accurate
	for highResolutionNow() < target {
		// Tight spin for sub-200µs precision
	}
}

// PreciseSleep performs a high-precision sleep on Windows
// Falls back to spin-wait for very short durations
func PreciseSleep(duration time.Duration) {
	if duration <= 0 {
		return
	}

	// For durations less than 50µs, just spin-wait
	// Windows timer resolution makes sleeping pointless here
	if duration < 50*time.Microsecond {
		target := highResolutionNow() + duration
		for highResolutionNow() < target {
			// Tight spin loop
		}
		return
	}

	// For longer durations, use hybrid approach
	preciseSleepWindows(duration)
}
