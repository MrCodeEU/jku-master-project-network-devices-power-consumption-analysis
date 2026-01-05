package loadgen

import (
	"context"
	"crypto/rand"
	"fmt"
	"log"
	"net"
	"sync"
	"time"
)

type Config struct {
	TargetIP         string
	TargetPort       int
	Protocol         string             // "udp" or "tcp"
	PacketSize       int
	InterfaceConfigs []InterfaceConfig  // Per-interface configuration
}

// InterfaceConfig holds settings for a single network interface
type InterfaceConfig struct {
	Name             string  // Interface name (empty = OS routing)
	Workers          int     // Number of workers for this interface
	TargetThroughput float64 // Target throughput in Mbps (0 = unlimited)
	RampSteps        int     // Number of ramp-up steps (0 = no ramping)
}

// LoadGenerator defines the interface for generating network load
type LoadGenerator interface {
	Start(ctx context.Context, config Config) error
	GetThroughput() float64           // Returns current throughput in Mbps
	SetTargetThroughput(mbps float64) // Set target throughput for rate limiting
	GetTargetThroughput() float64     // Get current target throughput
}

// NetworkLoadGenerator floods the target with packets
type NetworkLoadGenerator struct {
	mu               sync.Mutex
	bytesSent        uint64
	lastUpdate       time.Time
	throughput       float64 // Mbps
	targetThroughput float64 // Target Mbps (0 = unlimited)
	numWorkers       int     // Total number of workers for rate calculation
}

func NewNetworkLoadGenerator() *NetworkLoadGenerator {
	return &NetworkLoadGenerator{
		lastUpdate: time.Now(),
	}
}

// SetTargetThroughput updates the target throughput dynamically
func (g *NetworkLoadGenerator) SetTargetThroughput(mbps float64) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.targetThroughput = mbps
}

// GetTargetThroughput returns the current target throughput
func (g *NetworkLoadGenerator) GetTargetThroughput() float64 {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.targetThroughput
}

// getWorkerDelay calculates delay per packet to achieve target throughput
func (g *NetworkLoadGenerator) getWorkerDelay(packetSize int) time.Duration {
	g.mu.Lock()
	target := g.targetThroughput
	workers := g.numWorkers
	g.mu.Unlock()

	if target <= 0 || workers <= 0 {
		return 0 // No rate limiting
	}

	// Calculate bytes per second for this worker
	// target Mbps / workers = Mbps per worker
	// Mbps * 1_000_000 / 8 = bytes per second
	bytesPerSecond := (target * 1_000_000 / 8) / float64(workers)
	if bytesPerSecond <= 0 {
		return 0
	}

	// Calculate packets per second and delay between packets
	packetsPerSecond := bytesPerSecond / float64(packetSize)
	if packetsPerSecond <= 0 {
		return time.Second // Very slow
	}

	// Calculate base delay
	delay := time.Duration(float64(time.Second) / packetsPerSecond)
	
	// Apply overhead compensation factor (time.Sleep has ~1ms minimum on Windows,
	// plus context switch overhead). Reduce delay by 30% to compensate.
	// For very small delays (<100µs), skip rate limiting entirely as the
	// overhead dominates and we can't achieve microsecond precision.
	if delay < 100*time.Microsecond {
		return 0 // Let it run at full speed
	}
	
	compensatedDelay := time.Duration(float64(delay) * 0.7) // 30% faster to compensate for overhead
	if compensatedDelay < time.Microsecond {
		return 0
	}
	
	return compensatedDelay
}

func (g *NetworkLoadGenerator) GetThroughput() float64 {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.throughput
}

func (g *NetworkLoadGenerator) updateThroughput(bytesSent int) {
	g.mu.Lock()
	defer g.mu.Unlock()
	
	g.bytesSent += uint64(bytesSent)
	now := time.Now()
	elapsed := now.Sub(g.lastUpdate).Seconds()
	
	// Update throughput every second
	if elapsed >= 1.0 {
		// Convert bytes per second to Megabits per second
		// 1 byte = 8 bits, 1 Mbps = 1,000,000 bits/s
		g.throughput = (float64(g.bytesSent) * 8.0) / (elapsed * 1_000_000)
		g.bytesSent = 0
		g.lastUpdate = now
	}
}

func (g *NetworkLoadGenerator) Start(ctx context.Context, config Config) error {
	ifaceConfigs := config.InterfaceConfigs
	if len(ifaceConfigs) == 0 {
		ifaceConfigs = []InterfaceConfig{{Name: "", Workers: 8, TargetThroughput: 0, RampSteps: 0}}
	}

	// Calculate total workers and total target throughput
	totalWorkers := 0
	totalThroughput := 0.0
	for _, ic := range ifaceConfigs {
		totalWorkers += ic.Workers
		totalThroughput += ic.TargetThroughput
	}
	
	g.mu.Lock()
	g.numWorkers = totalWorkers
	g.targetThroughput = totalThroughput
	g.mu.Unlock()

	fmt.Printf("Starting load generation: %s://%s:%d (Size: %d bytes)\n",
		config.Protocol, config.TargetIP, config.TargetPort, config.PacketSize)
	
	for _, ic := range ifaceConfigs {
		throughputStr := "unlimited"
		if ic.TargetThroughput > 0 {
			throughputStr = fmt.Sprintf("%.1f Mbps", ic.TargetThroughput)
		}
		rampStr := "none"
		if ic.RampSteps > 0 {
			rampStr = fmt.Sprintf("%d steps", ic.RampSteps)
		}
		ifaceName := ic.Name
		if ifaceName == "" {
			ifaceName = "OS-routing"
		}
		fmt.Printf("  Interface %s: %d workers, target %s, ramp %s\n", ifaceName, ic.Workers, throughputStr, rampStr)
	}

	var wg sync.WaitGroup

	// Start workers for each interface with their own config
	for _, ifaceConfig := range ifaceConfigs {
		ic := ifaceConfig // capture for goroutine
		for i := 0; i < ic.Workers; i++ {
			wg.Add(1)
			go func(workerID int) {
				defer wg.Done()
				if config.Protocol == "udp" {
					g.runUDPWorkerWithConfig(ctx, workerID, config, ic)
				} else {
					g.runTCPWorkerWithConfig(ctx, workerID, config, ic)
				}
			}(i)
		}
	}

	// Wait for context cancellation
	<-ctx.Done()

	// Wait for workers to finish (they should check ctx)
	wg.Wait()
	fmt.Println("Load generation stopped")
	return nil
}

// getLocalAddr returns a local address bound to the specified interface
func (g *NetworkLoadGenerator) getLocalAddr(ifaceName string, network string) (net.Addr, error) {
	if ifaceName == "" {
		return nil, nil // Use OS routing
	}

	iface, err := net.InterfaceByName(ifaceName)
	if err != nil {
		return nil, fmt.Errorf("interface %s not found: %w", ifaceName, err)
	}

	addrs, err := iface.Addrs()
	if err != nil {
		return nil, fmt.Errorf("failed to get addresses for %s: %w", ifaceName, err)
	}

	for _, addr := range addrs {
		var ip net.IP
		switch v := addr.(type) {
		case *net.IPNet:
			ip = v.IP
		case *net.IPAddr:
			ip = v.IP
		}

		if ip != nil && ip.To4() != nil && !ip.IsLoopback() {
			if network == "udp" {
				return &net.UDPAddr{IP: ip, Port: 0}, nil
			}
			return &net.TCPAddr{IP: ip, Port: 0}, nil
		}
	}

	return nil, fmt.Errorf("no IPv4 address found for interface %s", ifaceName)
}

// getWorkerDelayForInterface calculates delay per packet for a specific interface config
func (g *NetworkLoadGenerator) getWorkerDelayForInterface(packetSize int, ic InterfaceConfig) time.Duration {
	target := ic.TargetThroughput
	workers := ic.Workers

	if target <= 0 || workers <= 0 {
		return 0 // No rate limiting
	}

	// Calculate bytes per second for this worker
	bytesPerSecond := (target * 1_000_000 / 8) / float64(workers)
	if bytesPerSecond <= 0 {
		return 0
	}

	packetsPerSecond := bytesPerSecond / float64(packetSize)
	if packetsPerSecond <= 0 {
		return time.Second
	}

	delay := time.Duration(float64(time.Second) / packetsPerSecond)
	
	// For very small delays, skip rate limiting
	if delay < 100*time.Microsecond {
		return 0
	}
	
	// Apply overhead compensation (30% faster)
	compensatedDelay := time.Duration(float64(delay) * 0.7)
	if compensatedDelay < time.Microsecond {
		return 0
	}
	
	return compensatedDelay
}

func (g *NetworkLoadGenerator) runUDPWorkerWithConfig(ctx context.Context, id int, config Config, ic InterfaceConfig) {
	// Resolve target address
	targetAddr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%d", config.TargetIP, config.TargetPort))
	if err != nil {
		log.Printf("Worker %d: Failed to resolve address: %v\n", id, err)
		return
	}

	// Get local address for interface binding
	localAddr, err := g.getLocalAddr(ic.Name, "udp")
	if err != nil {
		log.Printf("Worker %d: Failed to get local address for %s: %v\n", id, ic.Name, err)
		return
	}

	var localUDPAddr *net.UDPAddr
	if localAddr != nil {
		localUDPAddr = localAddr.(*net.UDPAddr)
		log.Printf("Worker %d [%s]: Binding to %s\n", id, ic.Name, localUDPAddr.IP)
	}

	conn, err := net.DialUDP("udp", localUDPAddr, targetAddr)
	if err != nil {
		log.Printf("Worker %d: Failed to create UDP connection: %v\n", id, err)
		return
	}
	defer conn.Close()

	conn.SetWriteBuffer(4 * 1024 * 1024)

	buffer := make([]byte, config.PacketSize)
	rand.Read(buffer)

	for {
		select {
		case <-ctx.Done():
			return
		default:
			delay := g.getWorkerDelayForInterface(config.PacketSize, ic)
			if delay > 0 {
				time.Sleep(delay)
			}

			n, err := conn.Write(buffer)
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				log.Printf("Worker %d: Write error: %v\n", id, err)
				time.Sleep(100 * time.Millisecond)
			} else {
				g.updateThroughput(n)
			}
		}
	}
}

func (g *NetworkLoadGenerator) runTCPWorkerWithConfig(ctx context.Context, id int, config Config, ic InterfaceConfig) {
	targetAddr, err := net.ResolveTCPAddr("tcp", fmt.Sprintf("%s:%d", config.TargetIP, config.TargetPort))
	if err != nil {
		log.Printf("Worker %d: Failed to resolve address: %v\n", id, err)
		return
	}

	localAddr, err := g.getLocalAddr(ic.Name, "tcp")
	if err != nil {
		log.Printf("Worker %d: Failed to get local address for %s: %v\n", id, ic.Name, err)
		return
	}

	dialer := &net.Dialer{
		Timeout:   5 * time.Second,
		LocalAddr: localAddr,
	}

	if localAddr != nil {
		log.Printf("Worker %d [%s]: Binding to %s\n", id, ic.Name, localAddr.(*net.TCPAddr).IP)
	}

	conn, err := dialer.DialContext(ctx, "tcp", targetAddr.String())
	if err != nil {
		log.Printf("Worker %d: Failed to connect: %v\n", id, err)
		return
	}
	defer conn.Close()

	if tcpConn, ok := conn.(*net.TCPConn); ok {
		tcpConn.SetNoDelay(true)
		tcpConn.SetWriteBuffer(4 * 1024 * 1024)
	}

	buffer := make([]byte, config.PacketSize)
	rand.Read(buffer)

	for {
		select {
		case <-ctx.Done():
			return
		default:
			delay := g.getWorkerDelayForInterface(config.PacketSize, ic)
			if delay > 0 {
				time.Sleep(delay)
			}

			n, err := conn.Write(buffer)
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				log.Printf("Worker %d: Write error: %v\n", id, err)
				return
			} else {
				g.updateThroughput(n)
			}
		}
	}
}

// Legacy worker functions for backward compatibility
func (g *NetworkLoadGenerator) runUDPWorker(ctx context.Context, id int, config Config, ifaceName string) {
	ic := InterfaceConfig{Name: ifaceName, Workers: 8, TargetThroughput: 0}
	g.runUDPWorkerWithConfig(ctx, id, config, ic)
}

func (g *NetworkLoadGenerator) runTCPWorker(ctx context.Context, id int, config Config, ifaceName string) {
	ic := InterfaceConfig{Name: ifaceName, Workers: 8, TargetThroughput: 0}
	g.runTCPWorkerWithConfig(ctx, id, config, ic)
}
