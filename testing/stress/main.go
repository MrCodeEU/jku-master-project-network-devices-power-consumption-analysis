package main

import (
    "crypto/rand"
    "flag"
    "fmt"
    "log"
    "net"
    "sync"
    "sync/atomic"
    "time"
)

type Stats struct {
    bytesSent uint64
    packetsSent uint64
}

func main() {
    // Command line flags
    targetIP := flag.String("target", "", "Target AP IP address")
    targetPort := flag.Int("port", 9, "Target port (default: 9 for discard service)")
    protocol := flag.String("proto", "udp", "Protocol: tcp or udp")
    workers := flag.Int("workers", 8, "Number of concurrent workers")
    packetSize := flag.Int("size", 1400, "Packet size in bytes (max 1472 for UDP without fragmentation)")
    duration := flag.Int("duration", 30, "Test duration in seconds")
    bindInterface := flag.String("interface", "", "Local interface IP to bind to (e.g., 192.168.1.100)")
    
    flag.Parse()

    if *targetIP == "" {
        log.Fatal("Target IP is required. Use -target flag")
    }

    fmt.Printf("Starting stress test:\n")
    fmt.Printf("  Target: %s:%d\n", *targetIP, *targetPort)
    fmt.Printf("  Protocol: %s\n", *protocol)
    fmt.Printf("  Workers: %d\n", *workers)
    fmt.Printf("  Packet size: %d bytes\n", *packetSize)
    fmt.Printf("  Duration: %d seconds\n", *duration)
    if *bindInterface != "" {
        fmt.Printf("  Bound to interface: %s\n", *bindInterface)
    }
    fmt.Println()

    stats := &Stats{}
    var wg sync.WaitGroup

    // Start workers
    for i := 0; i < *workers; i++ {
        wg.Add(1)
        go func(workerID int) {
            defer wg.Done()
            
            if *protocol == "udp" {
                runUDPWorker(workerID, *targetIP, *targetPort, *packetSize, *duration, *bindInterface, stats)
            } else {
                runTCPWorker(workerID, *targetIP, *targetPort, *packetSize, *duration, *bindInterface, stats)
            }
        }(i)
    }

    // Stats reporter
    stopStats := make(chan bool)
    go reportStats(stats, stopStats)

    // Wait for duration
    time.Sleep(time.Duration(*duration) * time.Second)
    close(stopStats)
    
    // Give workers a moment to finish
    done := make(chan bool)
    go func() {
        wg.Wait()
        done <- true
    }()

    select {
    case <-done:
        fmt.Println("\nAll workers completed")
    case <-time.After(5 * time.Second):
        fmt.Println("\nTimeout waiting for workers")
    }

    // Final stats
    totalBytes := atomic.LoadUint64(&stats.bytesSent)
    totalPackets := atomic.LoadUint64(&stats.packetsSent)
    
    fmt.Printf("\n=== Final Statistics ===\n")
    fmt.Printf("Total bytes sent: %d (%.2f GB)\n", totalBytes, float64(totalBytes)/(1024*1024*1024))
    fmt.Printf("Total packets sent: %d\n", totalPackets)
    fmt.Printf("Average throughput: %.2f Mbps\n", float64(totalBytes*8)/float64(*duration)/1000000)
}

func runUDPWorker(id int, targetIP string, port int, packetSize int, duration int, bindInterface string, stats *Stats) {
    // Create local address if interface binding is specified
    var localAddr *net.UDPAddr
    if bindInterface != "" {
        localAddr = &net.UDPAddr{
            IP: net.ParseIP(bindInterface),
            Port: 0,
        }
    }

    // Resolve target address
    targetAddr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%d", targetIP, port))
    if err != nil {
        log.Printf("Worker %d: Failed to resolve address: %v\n", id, err)
        return
    }

    // Create UDP connection
    conn, err := net.DialUDP("udp", localAddr, targetAddr)
    if err != nil {
        log.Printf("Worker %d: Failed to create UDP connection: %v\n", id, err)
        return
    }
    defer conn.Close()

    // Set write buffer size
    conn.SetWriteBuffer(4 * 1024 * 1024) // 4MB buffer

    // Pre-allocate buffer with random data
    buffer := make([]byte, packetSize)
    rand.Read(buffer)

    endTime := time.Now().Add(time.Duration(duration) * time.Second)

    for time.Now().Before(endTime) {
        n, err := conn.Write(buffer)
        if err != nil {
            log.Printf("Worker %d: Write error: %v\n", id, err)
            return
        }
        
        atomic.AddUint64(&stats.bytesSent, uint64(n))
        atomic.AddUint64(&stats.packetsSent, 1)
    }
}

func runTCPWorker(id int, targetIP string, port int, packetSize int, duration int, bindInterface string, stats *Stats) {
    // Create local address if interface binding is specified
    var localAddr *net.TCPAddr
    if bindInterface != "" {
        localAddr = &net.TCPAddr{
            IP: net.ParseIP(bindInterface),
            Port: 0,
        }
    }

    // Resolve target address
    targetAddr, err := net.ResolveTCPAddr("tcp", fmt.Sprintf("%s:%d", targetIP, port))
    if err != nil {
        log.Printf("Worker %d: Failed to resolve address: %v\n", id, err)
        return
    }

    // Create dialer with local address
    dialer := &net.Dialer{
        LocalAddr: localAddr,
        Timeout:   5 * time.Second,
    }

    // Connect
    conn, err := dialer.Dial("tcp", targetAddr.String())
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
    buffer := make([]byte, packetSize)
    rand.Read(buffer)

    endTime := time.Now().Add(time.Duration(duration) * time.Second)

    for time.Now().Before(endTime) {
        n, err := conn.Write(buffer)
        if err != nil {
            log.Printf("Worker %d: Write error: %v\n", id, err)
            return
        }
        
        atomic.AddUint64(&stats.bytesSent, uint64(n))
        atomic.AddUint64(&stats.packetsSent, 1)
    }
}

func reportStats(stats *Stats, stop chan bool) {
    ticker := time.NewTicker(1 * time.Second)
    defer ticker.Stop()

    var lastBytes uint64
    var lastTime = time.Now()

    for {
        select {
        case <-stop:
            return
        case <-ticker.C:
            currentBytes := atomic.LoadUint64(&stats.bytesSent)
            currentPackets := atomic.LoadUint64(&stats.packetsSent)
            now := time.Now()
            
            elapsed := now.Sub(lastTime).Seconds()
            bytesDiff := currentBytes - lastBytes
            
            throughputMbps := float64(bytesDiff*8) / elapsed / 1000000
            
            fmt.Printf("Throughput: %.2f Mbps | Total: %.2f MB | Packets: %d\n",
                throughputMbps,
                float64(currentBytes)/(1024*1024),
                currentPackets)
            
            lastBytes = currentBytes
            lastTime = now
        }
    }
}