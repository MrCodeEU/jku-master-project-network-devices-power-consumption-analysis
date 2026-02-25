package runner

import (
	"bufio"
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"project/internal/loadgen"
)

// iperf3 output line regex: extract Mbits/sec from interval lines
// Example: [  5]   0.00-1.00   sec  112 MBytes  940 Mbits/sec    0   286 KBytes
var iperf3LineRegex = regexp.MustCompile(`\[\s*\d+\]\s+[\d.]+-[\d.]+\s+sec\s+[\d.]+\s+\w+\s+([\d.]+)\s+Mbits/sec`)

// runIperf3 starts an iperf3 client subprocess and parses its output for real-time throughput.
func (r *Runner) runIperf3(ctx context.Context, config loadgen.Config, duration time.Duration) error {
	// Build iperf3 command arguments
	args := []string{
		"-c", config.TargetIP,
		"-p", strconv.Itoa(config.TargetPort),
		"-t", strconv.Itoa(int(duration.Seconds())),
		"-f", "m",          // Format output in Mbits
		"--forceflush",     // Flush output after each interval
	}

	if config.Reverse {
		args = append(args, "-R")
	}

	fmt.Printf("[iperf3] Starting: iperf3 %s\n", strings.Join(args, " "))

	cmd := exec.CommandContext(ctx, "iperf3", args...)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("iperf3: failed to create stdout pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("iperf3: failed to create stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("iperf3: failed to start (is iperf3 installed and in PATH?): %w", err)
	}

	// Collect stderr in background for error reporting
	var stderrOutput strings.Builder
	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			line := scanner.Text()
			stderrOutput.WriteString(line + "\n")
			fmt.Printf("[iperf3 stderr] %s\n", line)
		}
	}()

	// Parse stdout line by line
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		line := scanner.Text()
		fmt.Printf("[iperf3] %s\n", line)

		matches := iperf3LineRegex.FindStringSubmatch(line)
		if len(matches) >= 2 {
			mbps, parseErr := strconv.ParseFloat(matches[1], 64)
			if parseErr == nil {
				r.iperf3Mu.Lock()
				r.iperf3Readings = append(r.iperf3Readings, mbps)
				r.iperf3Mu.Unlock()
			}
		}
	}

	// Wait for process to finish
	waitErr := cmd.Wait()

	// Check if we were cancelled (normal stop)
	if ctx.Err() != nil {
		fmt.Println("[iperf3] Process stopped (context cancelled)")
		return nil
	}

	if waitErr != nil {
		errMsg := stderrOutput.String()
		if errMsg != "" {
			return fmt.Errorf("iperf3 exited with error: %w\nstderr: %s", waitErr, errMsg)
		}
		return fmt.Errorf("iperf3 exited with error: %w", waitErr)
	}

	fmt.Println("[iperf3] Process completed successfully")
	return nil
}
