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
	TargetIP   string
	TargetPort int
	Protocol   string // "udp" or "tcp"
	Workers    int
	PacketSize int
}

// LoadGenerator defines the interface for generating network load
type LoadGenerator interface {
	Start(ctx context.Context, config Config) error
	GetThroughput() float64 // Returns current throughput in Mbps
}

// NetworkLoadGenerator floods the target with packets
type NetworkLoadGenerator struct {
	mu         sync.Mutex
	bytesSent  uint64
	lastUpdate time.Time
	throughput float64 // Mbps
}

func NewNetworkLoadGenerator() *NetworkLoadGenerator {
	return &NetworkLoadGenerator{
		lastUpdate: time.Now(),
	}
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
	fmt.Printf("Starting load generation: %s://%s:%d (Workers: %d, Size: %d)\n",
		config.Protocol, config.TargetIP, config.TargetPort, config.Workers, config.PacketSize)

	var wg sync.WaitGroup

	for i := 0; i < config.Workers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			if config.Protocol == "udp" {
				g.runUDPWorker(ctx, workerID, config)
			} else {
				g.runTCPWorker(ctx, workerID, config)
			}
		}(i)
	}

	// Wait for context cancellation
	<-ctx.Done()

	// Wait for workers to finish (they should check ctx)
	wg.Wait()
	fmt.Println("Load generation stopped")
	return nil
}

func (g *NetworkLoadGenerator) runUDPWorker(ctx context.Context, id int, config Config) {
	// Resolve target address
	targetAddr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%d", config.TargetIP, config.TargetPort))
	if err != nil {
		log.Printf("Worker %d: Failed to resolve address: %v\n", id, err)
		return
	}

	// Create UDP connection
	// We use DialUDP to establish a "connected" UDP socket which is slightly more efficient for repeated writes
	conn, err := net.DialUDP("udp", nil, targetAddr)
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

func (g *NetworkLoadGenerator) runTCPWorker(ctx context.Context, id int, config Config) {
	// Resolve target address
	targetAddr, err := net.ResolveTCPAddr("tcp", fmt.Sprintf("%s:%d", config.TargetIP, config.TargetPort))
	if err != nil {
		log.Printf("Worker %d: Failed to resolve address: %v\n", id, err)
		return
	}

	dialer := &net.Dialer{
		Timeout: 5 * time.Second,
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
