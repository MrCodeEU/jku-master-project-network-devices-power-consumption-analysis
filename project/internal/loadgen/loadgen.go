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
	Protocol         string   // "udp" or "tcp"
	Workers          int      // Workers per interface
	PacketSize       int
	Interfaces       []string // List of interface names to use (empty = OS routing)
	TargetThroughput float64  // Target throughput in Mbps (0 = unlimited)
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

	return time.Duration(float64(time.Second) / packetsPerSecond)
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
	interfaces := config.Interfaces
	if len(interfaces) == 0 {
		interfaces = []string{""} // Empty string means use OS routing
	}

	// Calculate total workers and set target throughput
	totalWorkers := len(interfaces) * config.Workers
	g.mu.Lock()
	g.numWorkers = totalWorkers
	g.targetThroughput = config.TargetThroughput
	g.mu.Unlock()

	throughputStr := "unlimited"
	if config.TargetThroughput > 0 {
		throughputStr = fmt.Sprintf("%.1f Mbps", config.TargetThroughput)
	}

	fmt.Printf("Starting load generation: %s://%s:%d (Workers: %d per interface, Size: %d, Interfaces: %v, Target: %s)\n",
		config.Protocol, config.TargetIP, config.TargetPort, config.Workers, config.PacketSize, interfaces, throughputStr)

	var wg sync.WaitGroup

	// Start workers for each interface
	for _, ifaceName := range interfaces {
		for i := 0; i < config.Workers; i++ {
			wg.Add(1)
			go func(workerID int, iface string) {
				defer wg.Done()
				if config.Protocol == "udp" {
					g.runUDPWorker(ctx, workerID, config, iface)
				} else {
					g.runTCPWorker(ctx, workerID, config, iface)
				}
			}(i, ifaceName)
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

func (g *NetworkLoadGenerator) runUDPWorker(ctx context.Context, id int, config Config, ifaceName string) {
	// Resolve target address
	targetAddr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%d", config.TargetIP, config.TargetPort))
	if err != nil {
		log.Printf("Worker %d: Failed to resolve address: %v\n", id, err)
		return
	}

	// Get local address for interface binding
	localAddr, err := g.getLocalAddr(ifaceName, "udp")
	if err != nil {
		log.Printf("Worker %d: Failed to get local address for %s: %v\n", id, ifaceName, err)
		return
	}

	var localUDPAddr *net.UDPAddr
	if localAddr != nil {
		localUDPAddr = localAddr.(*net.UDPAddr)
		log.Printf("Worker %d: Binding to interface %s (%s)\n", id, ifaceName, localUDPAddr.IP)
	}

	// Create UDP connection with optional interface binding
	conn, err := net.DialUDP("udp", localUDPAddr, targetAddr)
	if err != nil {
		log.Printf("Worker %d: Failed to create UDP connection: %v\n", id, err)
		return
	}
	defer conn.Close()

	// Set write buffer size
	conn.SetWriteBuffer(4 * 1024 * 1024) // 4MB buffer

	// Pre-allocate buffer with random data
	buffer := make([]byte, config.PacketSize)
	rand.Read(buffer)

	for {
		select {
		case <-ctx.Done():
			return
		default:
			// Apply rate limiting if target throughput is set
			delay := g.getWorkerDelay(config.PacketSize)
			if delay > 0 {
				time.Sleep(delay)
			}

			n, err := conn.Write(buffer)
			if err != nil {
				// Check if it's a temporary error or context closed
				if ctx.Err() != nil {
					return
				}
				log.Printf("Worker %d: Write error: %v\n", id, err)
				time.Sleep(100 * time.Millisecond) // Backoff slightly on error
			} else {
				g.updateThroughput(n)
			}
		}
	}
}

func (g *NetworkLoadGenerator) runTCPWorker(ctx context.Context, id int, config Config, ifaceName string) {
	// Resolve target address
	targetAddr, err := net.ResolveTCPAddr("tcp", fmt.Sprintf("%s:%d", config.TargetIP, config.TargetPort))
	if err != nil {
		log.Printf("Worker %d: Failed to resolve address: %v\n", id, err)
		return
	}

	// Get local address for interface binding
	localAddr, err := g.getLocalAddr(ifaceName, "tcp")
	if err != nil {
		log.Printf("Worker %d: Failed to get local address for %s: %v\n", id, ifaceName, err)
		return
	}

	dialer := &net.Dialer{
		Timeout:   5 * time.Second,
		LocalAddr: localAddr,
	}

	if localAddr != nil {
		log.Printf("Worker %d: Binding to interface %s (%s)\n", id, ifaceName, localAddr.(*net.TCPAddr).IP)
	}

	conn, err := dialer.DialContext(ctx, "tcp", targetAddr.String())
	if err != nil {
		log.Printf("Worker %d: Failed to connect: %v\n", id, err)
		return
	}
	defer conn.Close()

	// Set TCP options for maximum throughput
	if tcpConn, ok := conn.(*net.TCPConn); ok {
		tcpConn.SetNoDelay(true)
		tcpConn.SetWriteBuffer(4 * 1024 * 1024)
	}

	// Pre-allocate buffer
	buffer := make([]byte, config.PacketSize)
	rand.Read(buffer)

	for {
		select {
		case <-ctx.Done():
			return
		default:
			// Apply rate limiting if target throughput is set
			delay := g.getWorkerDelay(config.PacketSize)
			if delay > 0 {
				time.Sleep(delay)
			}

			n, err := conn.Write(buffer)
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				log.Printf("Worker %d: Write error: %v\n", id, err)
				return // TCP connection broken, exit worker (or could try to reconnect)
			} else {
				g.updateThroughput(n)
			}
		}
	}
}
