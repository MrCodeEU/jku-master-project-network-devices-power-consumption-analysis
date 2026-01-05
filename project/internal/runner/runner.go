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
	Duration    time.Duration
	Interval    time.Duration
	Description string
	
	// Load Generation
	LoadEnabled bool
	TargetIP    string
	TargetPort  int
	Protocol    string
	Workers     int
	PacketSize  int
}

type DataPoint struct {
	Timestamp      time.Time `json:"timestamp"`
	PowerMW        float64   `json:"power_mw"`
	ThroughputMbps float64   `json:"throughput_mbps"`
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

	// Start Load Generation if enabled
	if config.LoadEnabled && config.TargetIP != "" {
		go func() {
			loadConfig := loadgen.Config{
				TargetIP:   config.TargetIP,
				TargetPort: config.TargetPort,
				Protocol:   config.Protocol,
				Workers:    config.Workers,
				PacketSize: config.PacketSize,
			}
			err := r.loadGen.Start(ctx, loadConfig)
			if err != nil {
				fmt.Printf("Load generation error: %v\n", err)
			}
		}()
	}

	ticker := time.NewTicker(config.Interval)
	defer ticker.Stop()

	fmt.Printf("Starting test: %s (Duration: %s)\n", config.Description, config.Duration)

	timer := time.NewTimer(config.Duration)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			result.EndTime = time.Now()
			return result, ctx.Err()
		case <-timer.C:
			result.EndTime = time.Now()
			return result, nil
		case t := <-ticker.C:
			power, err := r.meter.GetCurrentPower()
			if err != nil {
				fmt.Printf("Error reading power: %v\n", err)
				continue
			}

			// Get current throughput
			throughput := 0.0
			if config.LoadEnabled {
				throughput = r.loadGen.GetThroughput()
			}

			// fmt.Printf("Read power: %.2f mW, Throughput: %.2f Mbps\n", power, throughput)

			dp := DataPoint{
				Timestamp:      t,
				PowerMW:        power,
				ThroughputMbps: throughput,
			}

			result.DataPoints = append(result.DataPoints, dp)
			
			// Non-blocking send to update channel
			select {
			case updateChan <- dp:
			default:
			}
		}
	}
}
