package runner

import (
	"context"
	"fmt"
	"net"
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
	
	// Load Generation
	LoadEnabled      bool
	TargetIP         string
	TargetPort       int
	Protocol         string
	Workers          int
	PacketSize       int
	Interfaces       []string // Network interfaces to use for load generation
	TargetThroughput float64  // Target throughput in Mbps (0 = unlimited)
	RampSteps        int      // Number of ramp-up steps (0 = no ramping)
}

// Phase represents the current test phase
type Phase string

const (
	PhasePreTest  Phase = "pre"
	PhaseLoad     Phase = "load"
	PhasePostTest Phase = "post"
)

type DataPoint struct {
	Timestamp      time.Time `json:"timestamp"`
	PowerMW        float64   `json:"power_mw"`
	ThroughputMbps float64   `json:"throughput_mbps"`
	Phase          Phase     `json:"phase"`
}

type TestResult struct {
	Config     TestConfig
	DataPoints []DataPoint
	StartTime  time.Time
	EndTime    time.Time
}

type Runner struct {
	meter   fritzbox.PowerMeter
	loadGen loadgen.LoadGenerator
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

// RunTest starts a test and streams data points to the updateChan
func (r *Runner) RunTest(ctx context.Context, config TestConfig, updateChan chan<- DataPoint) (*TestResult, error) {
	result := &TestResult{
		Config:     config,
		DataPoints: make([]DataPoint, 0),
		StartTime:  time.Now(),
	}

	ticker := time.NewTicker(config.Interval)
	defer ticker.Stop()

	// Helper function to collect data for a phase
	collectData := func(phaseDuration time.Duration, phase Phase, loadCtx context.Context) error {
		if phaseDuration == 0 {
			return nil
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
				if phase == PhaseLoad && config.LoadEnabled {
					throughput = r.loadGen.GetThroughput()
				}

				dp := DataPoint{
					Timestamp:      t,
					PowerMW:        power,
					ThroughputMbps: throughput,
					Phase:          phase,
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
		if err := collectData(config.PreTestTime, PhasePreTest, nil); err != nil {
			result.EndTime = time.Now()
			return result, err
		}
	}

	// Phase 2: Load test
	var loadCancel context.CancelFunc
	var loadCtx context.Context
	if config.LoadEnabled && config.TargetIP != "" {
		loadCtx, loadCancel = context.WithCancel(ctx)
		go func() {
			loadConfig := loadgen.Config{
				TargetIP:         config.TargetIP,
				TargetPort:       config.TargetPort,
				Protocol:         config.Protocol,
				Workers:          config.Workers,
				PacketSize:       config.PacketSize,
				Interfaces:       config.Interfaces,
				TargetThroughput: config.TargetThroughput,
			}
			err := r.loadGen.Start(loadCtx, loadConfig)
			if err != nil {
				fmt.Printf("Load generation error: %v\n", err)
			}
		}()

		// Handle ramping if configured
		if config.RampSteps > 0 && config.TargetThroughput > 0 {
			go r.runRamping(loadCtx, config)
		}
	}

	if err := collectData(config.Duration, PhaseLoad, loadCtx); err != nil {
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
		if err := collectData(config.PostTestTime, PhasePostTest, nil); err != nil {
			result.EndTime = time.Now()
			return result, err
		}
	}

	result.EndTime = time.Now()
	fmt.Printf("Test completed. Total data points: %d\n", len(result.DataPoints))
	return result, nil
}

// runRamping gradually increases throughput from 0 to target over the test duration
func (r *Runner) runRamping(ctx context.Context, config TestConfig) {
	if config.RampSteps <= 0 || config.TargetThroughput <= 0 {
		return
	}

	stepDuration := config.Duration / time.Duration(config.RampSteps)
	stepSize := config.TargetThroughput / float64(config.RampSteps)

	fmt.Printf("Ramping: %d steps over %s, step size: %.1f Mbps\n", 
		config.RampSteps, config.Duration, stepSize)

	// Start at step 1 (first increment)
	for step := 1; step <= config.RampSteps; step++ {
		currentTarget := stepSize * float64(step)
		r.loadGen.SetTargetThroughput(currentTarget)
		fmt.Printf("Ramp step %d/%d: Target throughput = %.1f Mbps\n", 
			step, config.RampSteps, currentTarget)

		select {
		case <-ctx.Done():
			return
		case <-time.After(stepDuration):
			// Continue to next step
		}
	}
}
