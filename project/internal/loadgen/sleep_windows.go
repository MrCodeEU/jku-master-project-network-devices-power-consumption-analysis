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
	highResTimer     syscall.Handle
	timerInitOnce    sync.Once
	timerInitSuccess bool
	perfFreq         int64
)

func init() {
	// Query performance counter frequency
	procQueryPerformanceFrequency.Call(uintptr(unsafe.Pointer(&perfFreq)))
	
	// Set Windows timer resolution to 1ms for better time.Sleep() behavior as fallback
	procTimeBeginPeriod.Call(1)
}

// initHighResTimer attempts to create a high-resolution waitable timer (Windows 10 1803+)
func initHighResTimer() {
	timerInitOnce.Do(func() {
		ret, _, _ := procCreateWaitableTimerExW.Call(
			0, // lpTimerAttributes
			0, // lpTimerName
			CREATE_WAITABLE_TIMER_HIGH_RESOLUTION,
			TIMER_ALL_ACCESS,
		)
		if ret != 0 {
			highResTimer = syscall.Handle(ret)
			timerInitSuccess = true
		}
	})
}

// highResolutionNow returns the current time using QueryPerformanceCounter
func highResolutionNow() time.Duration {
	var counter int64
	procQueryPerformanceCounter.Call(uintptr(unsafe.Pointer(&counter)))
	// Convert to nanoseconds
	return time.Duration(counter * 1e9 / perfFreq)
}

// preciseSleepWindows implements a hybrid sleep using high-resolution timer + spin-wait
// Based on https://blog.bearcats.nl/perfect-sleep-function/
func preciseSleepWindows(duration time.Duration) {
	if duration <= 0 {
		return
	}

	initHighResTimer()

	target := highResolutionNow() + duration
	
	if timerInitSuccess && duration > 50*time.Microsecond {
		// Use high-resolution waitable timer for the bulk of the sleep
		// We leave a tolerance buffer to avoid overshooting
		const toleranceNs = 1020000 // ~1ms tolerance
		const periodNs = 1000000    // 1ms scheduler period
		const maxTicksNs = periodNs * 95 / 10 // 9.5ms max per sleep to avoid quirk
		
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
				uintptr(highResTimer),
				uintptr(unsafe.Pointer(&dueTime)),
				0, // lPeriod (0 = one-shot)
				0, // pfnCompletionRoutine
				0, // lpArgToCompletionRoutine
				0, // WakeContext
				0, // TolerableDelay
			)
			
			procWaitForSingleObject.Call(uintptr(highResTimer), INFINITE)
		}
	} else if duration > time.Millisecond {
		// Fallback: Use regular time.Sleep for the bulk, leaving 1ms + tolerance for spin
		sleepDuration := duration - 1500*time.Microsecond
		if sleepDuration > 0 {
			time.Sleep(sleepDuration)
		}
	}
	
	// Spin-wait for remaining time to achieve precise timing
	for highResolutionNow() < target {
		// Yield processor to avoid burning too much power
		// This is a no-op pause instruction on x86
		runtime_procYield()
	}
}

// runtime_procYield is implemented in assembly for precise CPU yielding
// For Go, we can approximate with runtime.Gosched() but it's less precise
// So we'll just do a tight loop for very short durations
func runtime_procYield() {
	// On x86, this would be PAUSE instruction
	// In Go, we can't easily do this without assembly, so we'll just continue
	// The tight loop is actually what we want for sub-millisecond precision
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
