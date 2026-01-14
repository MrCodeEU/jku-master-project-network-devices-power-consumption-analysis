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
	Name             string        // Interface name (empty = OS routing)
	Workers          int           // Number of workers for this interface
	TargetThroughput float64       // Target throughput in Mbps (0 = unlimited)
	RampSteps        int           // Number of ramp-up steps (0 = no ramping)
	PreTime          time.Duration // Additional pre-delay before this interface starts (on top of global pre-test)
	RampDuration     time.Duration // How long the ramping should take (0 = spread over full test duration)
}

// LoadGenerator defines the interface for generating network load
type LoadGenerator interface {
	Start(ctx context.Context, config Config) error
	GetThroughput() float64                            // Returns total throughput in Mbps
	GetThroughputByInterface() map[string]float64      // Returns throughput per interface
	GetTargetThroughputByInterface() map[string]float64 // Returns target throughput per interface
	SetTargetThroughput(mbps float64)                  // Set target throughput for rate limiting (global)
	SetInterfaceTargetThroughput(ifaceName string, mbps float64) // Set target for specific interface
	GetTargetThroughput() float64                      // Get current target throughput
}

// InterfaceThroughput tracks throughput for a single interface
type InterfaceThroughput struct {
	mu               sync.Mutex
	bytesSent        uint64
	lastUpdate       time.Time
	throughput       float64
	targetThroughput float64 // Current target for this interface (can be updated during ramping)
	workers          int     // Number of workers for this interface
}

// NetworkLoadGenerator floods the target with packets
type NetworkLoadGenerator struct {
	mu                   sync.Mutex
	bytesSent            uint64
	lastUpdate           time.Time
	throughput           float64 // Total Mbps
	targetThroughput     float64 // Target Mbps (0 = unlimited) - global fallback
	numWorkers           int     // Total number of workers for rate calculation
	interfaceThroughputs map[string]*InterfaceThroughput
}

func NewNetworkLoadGenerator() *NetworkLoadGenerator {
	return &NetworkLoadGenerator{
		lastUpdate:           time.Now(),
		interfaceThroughputs: make(map[string]*InterfaceThroughput),
	}
}

// SetTargetThroughput updates the target throughput dynamically
func (g *NetworkLoadGenerator) SetTargetThroughput(mbps float64) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.targetThroughput = mbps
}

// SetInterfaceTargetThroughput updates the target throughput for a specific interface
func (g *NetworkLoadGenerator) SetInterfaceTargetThroughput(ifaceName string, mbps float64) {
	if ifaceName == "" {
		ifaceName = "default"
	}
	
	g.mu.Lock()
	it, exists := g.interfaceThroughputs[ifaceName]
	g.mu.Unlock()
	
	if exists {
		it.mu.Lock()
		oldTarget := it.targetThroughput
		it.targetThroughput = mbps
		it.mu.Unlock()
		
		// Calculate expected delay for this new target (for diagnostics)
		if mbps > 0 && it.workers > 0 {
			bytesPerSec := (mbps * 1_000_000 / 8) / float64(it.workers)
			// Assuming 1400 byte packets for estimate
			packetsPerSec := bytesPerSec / 1400
			expectedDelay := time.Duration(float64(time.Second) / packetsPerSec)
			fmt.Printf("[SetInterfaceTargetThroughput] %s: %.1f -> %.1f Mbps (expected delay: %v per worker)\n", 
				ifaceName, oldTarget, mbps, expectedDelay)
		} else {
			fmt.Printf("[SetInterfaceTargetThroughput] %s: %.1f -> %.1f Mbps (unlimited)\n", ifaceName, oldTarget, mbps)
		}
	} else {
		fmt.Printf("[WARNING] SetInterfaceTargetThroughput: interface '%s' not found in throughput map\n", ifaceName)
		fmt.Printf("[DEBUG] Available interfaces: %v\n", g.getInterfaceNames())
	}
}

// GetTargetThroughput returns the current target throughput
func (g *NetworkLoadGenerator) GetTargetThroughput() float64 {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.targetThroughput
}

// getInterfaceNames returns a list of interface names in the throughput map (for debugging)
func (g *NetworkLoadGenerator) getInterfaceNames() []string {
	g.mu.Lock()
	defer g.mu.Unlock()
	
	names := make([]string, 0, len(g.interfaceThroughputs))
	for name := range g.interfaceThroughputs {
		names = append(names, name)
	}
	return names
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
	
	// With PreciseSleep using high-resolution Windows timers + spin-wait,
	// we can achieve microsecond precision. Apply minimal compensation
	// for system call overhead (~5-10µs).
	if delay < 10*time.Microsecond {
		return 0 // Too fast for any sleep to be useful
	}
	
	// Reduce delay slightly to compensate for syscall overhead
	compensatedDelay := time.Duration(float64(delay) * 0.95) // 5% compensation
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

// GetThroughputByInterface returns throughput for each interface
func (g *NetworkLoadGenerator) GetThroughputByInterface() map[string]float64 {
	g.mu.Lock()
	defer g.mu.Unlock()
	
	result := make(map[string]float64)
	for name, it := range g.interfaceThroughputs {
		it.mu.Lock()
		result[name] = it.throughput
		it.mu.Unlock()
	}
	return result
}

// GetTargetThroughputByInterface returns the current target throughput for each interface
func (g *NetworkLoadGenerator) GetTargetThroughputByInterface() map[string]float64 {
	g.mu.Lock()
	defer g.mu.Unlock()
	
	result := make(map[string]float64)
	for name, it := range g.interfaceThroughputs {
		it.mu.Lock()
		result[name] = it.targetThroughput
		it.mu.Unlock()
	}
	return result
}

// getOrCreateInterfaceThroughput gets or creates a throughput tracker for an interface
func (g *NetworkLoadGenerator) getOrCreateInterfaceThroughput(ifaceName string) *InterfaceThroughput {
	g.mu.Lock()
	defer g.mu.Unlock()
	
	if ifaceName == "" {
		ifaceName = "default"
	}
	
	if it, exists := g.interfaceThroughputs[ifaceName]; exists {
		return it
	}
	
	it := &InterfaceThroughput{
		lastUpdate: time.Now(),
	}
	g.interfaceThroughputs[ifaceName] = it
	return it
}

// initInterfaceThroughput initializes a throughput tracker with config values
func (g *NetworkLoadGenerator) initInterfaceThroughput(ic InterfaceConfig) *InterfaceThroughput {
	ifaceName := ic.Name
	if ifaceName == "" {
		ifaceName = "default"
	}
	
	g.mu.Lock()
	defer g.mu.Unlock()
	
	// Determine initial target throughput:
	// - If ramping is enabled (RampSteps > 0), start at 0 so ramping can gradually increase
	// - Otherwise, start at full target (0 = unlimited)
	initialTarget := ic.TargetThroughput
	if ic.RampSteps > 0 && ic.TargetThroughput > 0 {
		initialTarget = 0 // Ramping will set the first step value
	}
	
	it := &InterfaceThroughput{
		lastUpdate:       time.Now(),
		targetThroughput: initialTarget,
		workers:          ic.Workers,
	}
	g.interfaceThroughputs[ifaceName] = it
	
	fmt.Printf("[initInterfaceThroughput] Initialized '%s': initialTarget=%.1f Mbps, workers=%d, rampSteps=%d\n",
		ifaceName, initialTarget, ic.Workers, ic.RampSteps)
	
	return it
}

func (g *NetworkLoadGenerator) updateThroughput(bytesSent int) {
	g.mu.Lock()
	defer g.mu.Unlock()
	
	g.bytesSent += uint64(bytesSent)
	now := time.Now()
	elapsed := now.Sub(g.lastUpdate).Seconds()
	
	// Update throughput every second
	// NOTE: This measures actual bytes sent via socket API, which closely reflects
	// what the NIC transmits. The calculation accounts for UDP/IP overhead in the
	// payload size, so reported throughput = actual wire throughput.
	if elapsed >= 1.0 {
		// Convert bytes per second to Megabits per second
		// 1 byte = 8 bits, 1 Mbps = 1,000,000 bits/s
		g.throughput = (float64(g.bytesSent) * 8.0) / (elapsed * 1_000_000)
		g.bytesSent = 0
		g.lastUpdate = now
	}
}

// updateInterfaceThroughput updates throughput for a specific interface
func (g *NetworkLoadGenerator) updateInterfaceThroughput(ifaceName string, bytesSent int) {
	// Update total throughput
	g.updateThroughput(bytesSent)
	
	// Update interface-specific throughput
	it := g.getOrCreateInterfaceThroughput(ifaceName)
	it.mu.Lock()
	defer it.mu.Unlock()
	
	it.bytesSent += uint64(bytesSent)
	now := time.Now()
	elapsed := now.Sub(it.lastUpdate).Seconds()
	
	if elapsed >= 1.0 {
		it.throughput = (float64(it.bytesSent) * 8.0) / (elapsed * 1_000_000)
		it.bytesSent = 0
		it.lastUpdate = now
	}
}

func (g *NetworkLoadGenerator) Start(ctx context.Context, config Config) error {
	ifaceConfigs := config.InterfaceConfigs
	if len(ifaceConfigs) == 0 {
		ifaceConfigs = []InterfaceConfig{{Name: "", Workers: 10, TargetThroughput: 0, RampSteps: 0}}
	}

	// Calculate total workers and total target throughput
	totalWorkers := 0
	totalThroughput := 0.0
	for _, ic := range ifaceConfigs {
		totalWorkers += ic.Workers
		totalThroughput += ic.TargetThroughput
		// Initialize per-interface throughput tracker with config
		g.initInterfaceThroughput(ic)
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

// getWorkerDelayForInterface calculates delay per packet for a specific interface
// Uses the dynamic target from the interface throughput tracker
func (g *NetworkLoadGenerator) getWorkerDelayForInterface(packetSize int, ifaceName string) time.Duration {
	if ifaceName == "" {
		ifaceName = "default"
	}
	
	g.mu.Lock()
	it, exists := g.interfaceThroughputs[ifaceName]
	g.mu.Unlock()
	
	if !exists {
		return 0 // No rate limiting if interface not found
	}
	
	it.mu.Lock()
	target := it.targetThroughput
	workers := it.workers
	it.mu.Unlock()

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
	
	// With PreciseSleep using high-resolution Windows timers + spin-wait,
	// we can achieve microsecond precision. Apply minimal compensation
	// for system call overhead (~5-10µs).
	if delay < 10*time.Microsecond {
		return 0 // Too fast for any sleep to be useful
	}
	
	// Reduce delay slightly to compensate for syscall overhead
	compensatedDelay := time.Duration(float64(delay) * 0.95) // 5% compensation
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

	// Get interface name for delay calculation
	ifaceName := ic.Name

	// Batching optimization: send multiple packets before sleeping to reduce overhead
	// For high throughput targets, batching reduces PreciseSleep calls significantly
	const batchSize = 10 // Send 10 packets before sleeping
	packetCount := 0

	for {
		select {
		case <-ctx.Done():
			return
		default:
			delay := g.getWorkerDelayForInterface(config.PacketSize, ifaceName)
			
			// Send packet
			n, err := conn.Write(buffer)
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				log.Printf("Worker %d: Write error: %v\n", id, err)
				PreciseSleep(100 * time.Millisecond)
				continue
			}
			g.updateInterfaceThroughput(ic.Name, n)
			packetCount++
			
			// Batch delay: only sleep after every batchSize packets
			// This reduces PreciseSleep overhead from N calls to N/batchSize calls
			if delay > 0 && packetCount >= batchSize {
				PreciseSleep(delay * batchSize)
				packetCount = 0
			} else if delay == 0 {
				// No rate limiting - reset counter to avoid overflow
				packetCount = 0
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

	// Get interface name for delay calculation
	ifaceName := ic.Name

	for {
		select {
		case <-ctx.Done():
			return
		default:
			delay := g.getWorkerDelayForInterface(config.PacketSize, ifaceName)
			if delay > 0 {
				PreciseSleep(delay)
			}

			n, err := conn.Write(buffer)
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				log.Printf("Worker %d: Write error: %v\n", id, err)
				return
			} else {
				g.updateInterfaceThroughput(ic.Name, n)
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
