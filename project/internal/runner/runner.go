package runner

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"project/internal/fritzbox"
	"project/internal/loadgen"
)

type TestConfig struct {
	Duration     time.Duration
	Interval     time.Duration
	PreTestTime  time.Duration
	PostTestTime time.Duration
	Description  string
	TestName     string // User-defined test name
	DeviceName   string // Device under test name

	// Load Generation
	LoadEnabled bool
	LoadConfig  loadgen.Config // Complete load generation configuration
}

// Phase represents the current test phase
type Phase string

const (
	PhasePreTest  Phase = "pre"
	PhaseLoad     Phase = "load"
	PhasePostTest Phase = "post"
)

// EventType represents the type of marker/event
type EventType string

const (
	EventPhaseChange     EventType = "phase"
	EventRampStep        EventType = "ramp"
	EventInterfaceStart  EventType = "iface_start"
	EventInterfaceStop   EventType = "iface_stop"
	EventCustom          EventType = "custom"
)

// Event represents a marker or event in the timeline
type Event struct {
	Type      EventType `json:"type"`
	Message   string    `json:"message"`
	Timestamp time.Time `json:"timestamp"`
}

type DataPoint struct {
	Timestamp                   time.Time          `json:"timestamp"`
	PowerMW                     float64            `json:"power_mw"`
	ThroughputMbps              float64            `json:"throughput_mbps"`
	ThroughputByInterface       map[string]float64 `json:"throughput_by_interface,omitempty"`
	TargetThroughputByInterface map[string]float64 `json:"target_throughput_by_interface,omitempty"`
	Phase                       Phase              `json:"phase"`
	Events                      []Event            `json:"events,omitempty"`
}

type TestResult struct {
	Config     TestConfig
	DataPoints []DataPoint
	StartTime  time.Time
	EndTime    time.Time
}

type Runner struct {
	meter      fritzbox.PowerMeter
	loadGen    loadgen.LoadGenerator
	eventMu    sync.Mutex
	eventChan  chan Event
	testActive bool
}

func NewRunner(meter fritzbox.PowerMeter, lg loadgen.LoadGenerator) *Runner {
	return &Runner{
		meter:   meter,
		loadGen: lg,
	}
}

func (r *Runner) TestFritzboxConnection() error {
	return r.meter.TestConnection()
}

func (r *Runner) TestTargetConnection(targetIP string, targetPort int) error {
	if targetIP == "" {
		return fmt.Errorf("target IP is empty")
	}
	timeout := 2 * time.Second
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", targetIP, targetPort), timeout)
	if err != nil {
		return err
	}
	conn.Close()
	return nil
}

// AddCustomMarker adds a custom marker during an active test
func (r *Runner) AddCustomMarker(message string) bool {
	r.eventMu.Lock()
	defer r.eventMu.Unlock()
	
	if !r.testActive || r.eventChan == nil {
		return false
	}
	
	select {
	case r.eventChan <- Event{
		Type:      EventCustom,
		Message:   message,
		Timestamp: time.Now(),
	}:
		return true
	default:
		return false
	}
}

// addEvent queues an event (internal use)
func (r *Runner) addEvent(eventType EventType, message string) {
	r.eventMu.Lock()
	defer r.eventMu.Unlock()
	
	if r.eventChan != nil {
		select {
		case r.eventChan <- Event{
			Type:      eventType,
			Message:   message,
			Timestamp: time.Now(),
		}:
		default:
		}
	}
}

// IsTestActive returns whether a test is currently running
func (r *Runner) IsTestActive() bool {
	r.eventMu.Lock()
	defer r.eventMu.Unlock()
	return r.testActive
}

// RunTest starts a test and streams data points to the updateChan
func (r *Runner) RunTest(ctx context.Context, config TestConfig, updateChan chan<- DataPoint) (*TestResult, error) {
	result := &TestResult{
		Config:     config,
		DataPoints: make([]DataPoint, 0),
		StartTime:  time.Now(),
	}

	// Initialize event channel
	r.eventMu.Lock()
	r.eventChan = make(chan Event, 100)
	r.testActive = true
	r.eventMu.Unlock()
	
	defer func() {
		r.eventMu.Lock()
		r.testActive = false
		close(r.eventChan)
		r.eventChan = nil
		r.eventMu.Unlock()
	}()

	ticker := time.NewTicker(config.Interval)
	defer ticker.Stop()

	// Pending events buffer (events that occur between data points)
	var pendingEvents []Event
	var pendingEventsMu sync.Mutex

	// Goroutine to collect events
	go func() {
		for evt := range r.eventChan {
			pendingEventsMu.Lock()
			pendingEvents = append(pendingEvents, evt)
			pendingEventsMu.Unlock()
		}
	}()

	// Helper function to collect data for a phase
	collectData := func(phaseDuration time.Duration, phase Phase, phaseStart bool) error {
		if phaseDuration == 0 {
			return nil
		}

		// Add phase change event
		if phaseStart {
			phaseNames := map[Phase]string{PhasePreTest: "Pre-Test Baseline", PhaseLoad: "Load Test", PhasePostTest: "Post-Test Baseline"}
			r.addEvent(EventPhaseChange, phaseNames[phase])
		}

		fmt.Printf("Starting %s phase (Duration: %s)\n", phase, phaseDuration)
		timer := time.NewTimer(phaseDuration)
		defer timer.Stop()

		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-timer.C:
				return nil
			case t := <-ticker.C:
				power, err := r.meter.GetCurrentPower()
				if err != nil {
					fmt.Printf("Error reading power: %v\n", err)
					continue
				}

				throughput := 0.0
				var throughputByInterface map[string]float64
			var targetThroughputByInterface map[string]float64
			if phase == PhaseLoad && config.LoadEnabled {
				throughput = r.loadGen.GetThroughput()
				throughputByInterface = r.loadGen.GetThroughputByInterface()
				targetThroughputByInterface = r.loadGen.GetTargetThroughputByInterface()
			}

			// Collect pending events
			pendingEventsMu.Lock()
			events := pendingEvents
			pendingEvents = nil
			pendingEventsMu.Unlock()

			dp := DataPoint{
				Timestamp:                   t,
				PowerMW:                     power,
				ThroughputMbps:              throughput,
				ThroughputByInterface:       throughputByInterface,
				TargetThroughputByInterface: targetThroughputByInterface,
				Phase:                       phase,
				Events:                      events,
			}

			result.DataPoints = append(result.DataPoints, dp)

			select {
			case updateChan <- dp:
			default:
				}
			}
		}
	}

	fmt.Printf("Starting test: %s\n", config.Description)

	// Phase 1: Pre-test baseline (no load)
	if config.PreTestTime > 0 {
		if err := collectData(config.PreTestTime, PhasePreTest, true); err != nil {
			result.EndTime = time.Now()
			return result, err
		}
	}

	// Phase 2: Load test
	var loadCancel context.CancelFunc
	var loadCtx context.Context
	if config.LoadEnabled && (config.LoadConfig.TargetIP != "" || config.LoadConfig.TargetMAC != "") {
		loadCtx, loadCancel = context.WithCancel(ctx)

		// Start interfaces with their individual pre-delays
		for _, ic := range config.LoadConfig.InterfaceConfigs {
			ifaceConfig := ic // capture for goroutine
			go func() {
				ifaceName := ifaceConfig.Name
				if ifaceName == "" {
					ifaceName = "OS-routing"
				}

				// Wait for interface-specific pre-delay
				if ifaceConfig.PreTime > 0 {
					fmt.Printf("[%s] Waiting %.1fs before starting...\n", ifaceName, ifaceConfig.PreTime.Seconds())
					select {
					case <-loadCtx.Done():
						return
					case <-time.After(ifaceConfig.PreTime):
					}
				}

				// Notify interface start
				r.addEvent(EventInterfaceStart, fmt.Sprintf("Interface %s started", ifaceName))

				// Create per-interface load config
				perInterfaceConfig := config.LoadConfig
				perInterfaceConfig.InterfaceConfigs = []loadgen.InterfaceConfig{ifaceConfig}

				err := r.loadGen.Start(loadCtx, perInterfaceConfig)
				if err != nil {
					fmt.Printf("Load generation error [%s]: %v\n", ifaceName, err)
				}
			}()

			// Handle per-interface ramping
			if ic.RampSteps > 0 && ic.TargetThroughput > 0 {
				go r.runInterfaceRamping(loadCtx, ic)
			}
		}
	}

	if err := collectData(config.Duration, PhaseLoad, true); err != nil {
		if loadCancel != nil {
			loadCancel()
		}
		result.EndTime = time.Now()
		return result, err
	}

	// Stop load generation before post-test
	if loadCancel != nil {
		loadCancel()
		time.Sleep(500 * time.Millisecond) // Allow load gen to stop cleanly
	}

	// Phase 3: Post-test baseline (no load)
	if config.PostTestTime > 0 {
		if err := collectData(config.PostTestTime, PhasePostTest, true); err != nil {
			result.EndTime = time.Now()
			return result, err
		}
	}

	result.EndTime = time.Now()
	fmt.Printf("Test completed. Total data points: %d\n", len(result.DataPoints))
	return result, nil
}

// runInterfaceRamping gradually increases throughput for a specific interface
func (r *Runner) runInterfaceRamping(ctx context.Context, ic loadgen.InterfaceConfig) {
	if ic.RampSteps <= 0 || ic.TargetThroughput <= 0 {
		return
	}

	ifaceName := ic.Name
	if ifaceName == "" {
		ifaceName = "OS-routing"
	}

	// Wait for interface pre-delay first
	if ic.PreTime > 0 {
		select {
		case <-ctx.Done():
			return
		case <-time.After(ic.PreTime):
		}
	}

	// Determine ramp duration (use configured or default to 80% of a reasonable time)
	rampDuration := ic.RampDuration
	if rampDuration == 0 {
		// Default: spread steps evenly across 60 seconds or steps * 5 seconds, whichever is larger
		rampDuration = time.Duration(ic.RampSteps) * 5 * time.Second
		if rampDuration < 30*time.Second {
			rampDuration = 30 * time.Second
		}
	}

	stepDuration := rampDuration / time.Duration(ic.RampSteps)
	stepSize := ic.TargetThroughput / float64(ic.RampSteps)

	fmt.Printf("Ramping [%s]: %d steps over %s, step size: %.1f Mbps, target: %.1f Mbps\n", 
		ifaceName, ic.RampSteps, rampDuration, stepSize, ic.TargetThroughput)

	// Start at step 1 (first increment)
	for step := 1; step <= ic.RampSteps; step++ {
		currentTarget := stepSize * float64(step)
		// Update the per-interface target (not global)
		r.loadGen.SetInterfaceTargetThroughput(ic.Name, currentTarget)
		
		// Add ramp step event
		r.addEvent(EventRampStep, fmt.Sprintf("[%s] Ramp %d/%d: %.1f Mbps", ifaceName, step, ic.RampSteps, currentTarget))
		
		fmt.Printf("Ramp step %d/%d [%s]: Target = %.1f Mbps\n", 
			step, ic.RampSteps, ifaceName, currentTarget)

		select {
		case <-ctx.Done():
			return
		case <-time.After(stepDuration):
			// Continue to next step
		}
	}
	
	// Add event when ramp completes
	r.addEvent(EventRampStep, fmt.Sprintf("[%s] Ramp complete: %.1f Mbps", ifaceName, ic.TargetThroughput))
}
