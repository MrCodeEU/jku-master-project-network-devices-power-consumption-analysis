package loadgen

import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcap"
)

// Layer2Generator generates raw Ethernet frames for load testing
type Layer2Generator struct {
	mu                sync.RWMutex
	handles           map[string]*pcap.Handle
	bytesSent         uint64
	packetsSent       uint64
	startTime         time.Time
	interfaceThroughput map[string]*InterfaceThroughput
	targetThroughput  map[string]float64
	stopChans         map[string]chan struct{}
	// Per-interface atomic counters for throughput calculation
	interfaceBytesSent   map[string]*uint64
	interfacePacketsSent map[string]*uint64
}

// NewLayer2Generator creates a new Layer2 generator
func NewLayer2Generator() *Layer2Generator {
	return &Layer2Generator{
		handles:              make(map[string]*pcap.Handle),
		interfaceThroughput:  make(map[string]*InterfaceThroughput),
		targetThroughput:     make(map[string]float64),
		stopChans:            make(map[string]chan struct{}),
		interfaceBytesSent:   make(map[string]*uint64),
		interfacePacketsSent: make(map[string]*uint64),
	}
}

// getPcapDeviceName maps a friendly interface name to the pcap device name
func getPcapDeviceName(friendlyName string) (string, error) {
	devices, err := pcap.FindAllDevs()
	if err != nil {
		return "", fmt.Errorf("failed to enumerate pcap devices: %w", err)
	}

	// Get the network interface to match MAC address
	iface, err := net.InterfaceByName(friendlyName)
	if err != nil {
		return "", fmt.Errorf("failed to get interface %s: %w", friendlyName, err)
	}

	targetMAC := iface.HardwareAddr.String()

	// Try to find matching device by MAC address or name
	for _, device := range devices {
		// Check if device name contains the friendly name
		if strings.Contains(strings.ToLower(device.Description), strings.ToLower(friendlyName)) {
			return device.Name, nil
		}

		// Check MAC address match
		for _, addr := range device.Addresses {
			if addr.IP == nil {
				continue
			}
			// Try to get interface by IP to compare MAC
			ifaces, _ := net.Interfaces()
			for _, ifc := range ifaces {
				addrs, _ := ifc.Addrs()
				for _, a := range addrs {
					if ipnet, ok := a.(*net.IPNet); ok && ipnet.IP.Equal(addr.IP) {
						if ifc.HardwareAddr.String() == targetMAC {
							return device.Name, nil
						}
					}
				}
			}
		}
	}

	// If no match found, try direct name match (works on Linux)
	for _, device := range devices {
		if device.Name == friendlyName {
			return device.Name, nil
		}
	}

	// Last resort: return first device that's not loopback
	for _, device := range devices {
		if !strings.Contains(strings.ToLower(device.Description), "loopback") && len(device.Addresses) > 0 {
			fmt.Printf("Warning: Could not find exact match for '%s', using %s (%s)\n", friendlyName, device.Name, device.Description)
			return device.Name, nil
		}
	}

	return "", fmt.Errorf("no suitable pcap device found for interface '%s'", friendlyName)
}

// StartLayer2 starts Layer 2 load generation
func (lg *NetworkLoadGenerator) StartLayer2(ctx context.Context, config Config) error {
	if lg.layer2Gen == nil {
		lg.layer2Gen = NewLayer2Generator()
	}

	lg.layer2Gen.startTime = time.Now()

	// Start workers for each interface
	for _, ifaceConfig := range config.InterfaceConfigs {
		if ifaceConfig.Name == "" {
			return fmt.Errorf("interface name required for Layer 2 load generation")
		}

		// Parse target MAC address
		targetMAC, err := net.ParseMAC(config.TargetMAC)
		if err != nil {
			return fmt.Errorf("invalid target MAC address: %w", err)
		}

		// Get interface hardware address
		iface, err := net.InterfaceByName(ifaceConfig.Name)
		if err != nil {
			return fmt.Errorf("failed to get interface %s: %w", ifaceConfig.Name, err)
		}

		// Get pcap device name for this interface
		pcapDeviceName, err := getPcapDeviceName(ifaceConfig.Name)
		if err != nil {
			return fmt.Errorf("failed to find pcap device for %s: %w", ifaceConfig.Name, err)
		}

		// Open pcap handle for this interface with optimizations:
		// - snaplen: 65536 (large buffer)
		// - promisc: false (not capturing, only sending)
		// - timeout: immediate mode for max throughput
		inactive, err := pcap.NewInactiveHandle(pcapDeviceName)
		if err != nil {
			return fmt.Errorf("failed to create inactive handle for %s: %w", ifaceConfig.Name, err)
		}
		defer inactive.CleanUp()

		// Set buffer size (16MB for high throughput)
		if err := inactive.SetBufferSize(16 * 1024 * 1024); err != nil {
			fmt.Printf("Warning: Could not set buffer size for %s: %v\n", ifaceConfig.Name, err)
		}

		// Set snaplen
		if err := inactive.SetSnapLen(65536); err != nil {
			return fmt.Errorf("failed to set snaplen: %w", err)
		}

		// Disable promiscuous mode (not needed for sending)
		if err := inactive.SetPromisc(false); err != nil {
			return fmt.Errorf("failed to set promisc: %w", err)
		}

		// Set immediate mode for lower latency / higher throughput
		if err := inactive.SetImmediateMode(true); err != nil {
			fmt.Printf("Warning: Could not set immediate mode for %s: %v\n", ifaceConfig.Name, err)
		}

		// Set timeout (not critical for sending, but set anyway)
		if err := inactive.SetTimeout(time.Millisecond); err != nil {
			return fmt.Errorf("failed to set timeout: %w", err)
		}

		// Activate the handle
		handle, err := inactive.Activate()
		if err != nil {
			return fmt.Errorf("failed to activate pcap on %s (device: %s): %w", ifaceConfig.Name, pcapDeviceName, err)
		}

		lg.layer2Gen.mu.Lock()
		lg.layer2Gen.handles[ifaceConfig.Name] = handle
		lg.layer2Gen.interfaceThroughput[ifaceConfig.Name] = &InterfaceThroughput{}
		lg.layer2Gen.targetThroughput[ifaceConfig.Name] = ifaceConfig.TargetThroughput
		lg.layer2Gen.stopChans[ifaceConfig.Name] = make(chan struct{})
		// Initialize atomic counters for this interface
		var byteCounter uint64 = 0
		var packetCounter uint64 = 0
		lg.layer2Gen.interfaceBytesSent[ifaceConfig.Name] = &byteCounter
		lg.layer2Gen.interfacePacketsSent[ifaceConfig.Name] = &packetCounter
		lg.layer2Gen.mu.Unlock()

		// Start workers for this interface
		for i := 0; i < ifaceConfig.Workers; i++ {
			go lg.layer2Worker(ctx, ifaceConfig, iface.HardwareAddr, targetMAC, handle, config.PacketSize)
		}

		// Start throughput updater for this interface
		go lg.updateLayer2Throughput(ctx, ifaceConfig.Name)
	}

	return nil
}

// layer2Worker sends raw Ethernet frames
func (lg *NetworkLoadGenerator) layer2Worker(ctx context.Context, ifaceConfig InterfaceConfig, srcMAC, dstMAC net.HardwareAddr, handle *pcap.Handle, payloadSize int) {
	ifaceName := ifaceConfig.Name

	// Create payload buffer
	payload := make([]byte, payloadSize)
	for i := range payload {
		payload[i] = byte(i % 256)
	}

	// Calculate wire size for Ethernet frame
	// Preamble (8) + Ethernet Header (14) + Payload + FCS (4) + IFG (12)
	const (
		preamble  = 8
		ethHeader = 14
		fcs       = 4
		ifg       = 12
		minPayload = 46
	)

	actualPayload := payloadSize
	if actualPayload < minPayload {
		actualPayload = minPayload
	}
	wireBytes := preamble + ethHeader + actualPayload + fcs + ifg

	// Pre-serialize packet for efficiency
	ethLayer := &layers.Ethernet{
		SrcMAC:       srcMAC,
		DstMAC:       dstMAC,
		EthernetType: layers.EthernetTypeIPv4,
	}

	buffer := gopacket.NewSerializeBuffer()
	opts := gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}
	err := gopacket.SerializeLayers(buffer, opts, ethLayer, gopacket.Payload(payload))
	if err != nil {
		fmt.Printf("Failed to serialize packet: %v\n", err)
		return
	}
	packetData := buffer.Bytes()

	// Get stop channel
	lg.layer2Gen.mu.RLock()
	stopChan := lg.layer2Gen.stopChans[ifaceName]
	lg.layer2Gen.mu.RUnlock()

	// Rate limiting setup
	targetThroughput := ifaceConfig.TargetThroughput
	var packetDelay time.Duration

	if targetThroughput > 0 {
		// Calculate delay between packets for this worker
		targetBitsPerSecond := targetThroughput * 1_000_000 // Mbps to bps
		targetBytesPerSecond := targetBitsPerSecond / 8
		bytesPerWorker := targetBytesPerSecond / float64(ifaceConfig.Workers)
		packetsPerSecond := bytesPerWorker / float64(wireBytes)
		if packetsPerSecond > 0 {
			packetDelay = time.Duration(float64(time.Second) / packetsPerSecond)
		}
	}

	// Get atomic counters for this interface
	lg.layer2Gen.mu.RLock()
	ifaceBytesPtr := lg.layer2Gen.interfaceBytesSent[ifaceName]
	ifacePacketsPtr := lg.layer2Gen.interfacePacketsSent[ifaceName]
	lg.layer2Gen.mu.RUnlock()

	// Optimization: Send packets in bursts to reduce context switching overhead
	const burstSize = 128 // Send 128 packets before checking context or rate limiting
	var burstBytes uint64
	var burstPackets uint64

	// For rate limiting, calculate burst delay
	var burstDelay time.Duration
	if packetDelay > 0 {
		burstDelay = packetDelay * burstSize
	}

	// Ticker to periodically check for cancellation (reduces overhead)
	checkTicker := time.NewTicker(10 * time.Millisecond)
	defer checkTicker.Stop()

	var errorCount uint64 = 0
	const maxErrors = 100 // Stop worker if too many consecutive errors

	for {
		// Periodically check for cancellation (less frequently than every burst)
		select {
		case <-ctx.Done():
			return
		case <-stopChan:
			return
		case <-checkTicker.C:
			// Continue with burst sending
		default:
			// Fast path: just continue sending
		}

		// Send burst of packets in tight loop
		burstBytes = 0
		burstPackets = 0

		for i := 0; i < burstSize; i++ {
			err := handle.WritePacketData(packetData)
			if err != nil {
				errorCount++
				if errorCount > maxErrors {
					fmt.Printf("Too many errors on %s, stopping worker: %v\n", ifaceName, err)
					return
				}
				// Brief backoff on error
				time.Sleep(10 * time.Microsecond)
				continue
			}

			errorCount = 0 // Reset error count on success
			burstBytes += uint64(wireBytes)
			burstPackets++
		}

		// Update counters once per burst (reduces atomic contention)
		if burstPackets > 0 {
			atomic.AddUint64(&lg.layer2Gen.bytesSent, burstBytes)
			atomic.AddUint64(&lg.layer2Gen.packetsSent, burstPackets)
			atomic.AddUint64(ifaceBytesPtr, burstBytes)
			atomic.AddUint64(ifacePacketsPtr, burstPackets)
		}

		// Rate limiting after burst (if enabled)
		if burstDelay > 0 {
			PreciseSleep(burstDelay)
		}
		// No sleep if unlimited throughput - maximize send rate!
	}
}

// updateLayer2Throughput periodically updates interface throughput
func (lg *NetworkLoadGenerator) updateLayer2Throughput(ctx context.Context, ifaceName string) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	lg.layer2Gen.mu.RLock()
	ifaceBytesPtr := lg.layer2Gen.interfaceBytesSent[ifaceName]
	ifacePacketsPtr := lg.layer2Gen.interfacePacketsSent[ifaceName]
	lg.layer2Gen.mu.RUnlock()

	var lastBytes, lastPackets uint64
	lastUpdate := time.Now()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Calculate throughput from accumulated bytes
			currentBytes := atomic.LoadUint64(ifaceBytesPtr)
			currentPackets := atomic.LoadUint64(ifacePacketsPtr)

			elapsed := time.Since(lastUpdate).Seconds()
			if elapsed > 0 {
				bytesDiff := currentBytes - lastBytes
				packetsDiff := currentPackets - lastPackets

				mbps := float64(bytesDiff*8) / (1_000_000 * elapsed)

				lg.layer2Gen.mu.Lock()
				if ifaceTput, ok := lg.layer2Gen.interfaceThroughput[ifaceName]; ok {
					ifaceTput.mu.Lock()
					ifaceTput.Mbps = mbps
					ifaceTput.BytesSent = bytesDiff
					ifaceTput.PacketsSent = packetsDiff
					ifaceTput.mu.Unlock()
				}
				lg.layer2Gen.mu.Unlock()

				lastBytes = currentBytes
				lastPackets = currentPackets
				lastUpdate = time.Now()
			}
		}
	}
}

// GetLayer2Throughput returns total Layer 2 throughput
// Sum of all interface throughputs (measured over last second window)
func (lg *NetworkLoadGenerator) GetLayer2Throughput() float64 {
	if lg.layer2Gen == nil {
		return 0
	}

	lg.layer2Gen.mu.RLock()
	defer lg.layer2Gen.mu.RUnlock()

	var total float64
	for _, tput := range lg.layer2Gen.interfaceThroughput {
		tput.mu.RLock()
		total += tput.Mbps
		tput.mu.RUnlock()
	}

	return total
}

// GetLayer2ThroughputByInterface returns per-interface Layer 2 throughput
func (lg *NetworkLoadGenerator) GetLayer2ThroughputByInterface() map[string]float64 {
	if lg.layer2Gen == nil {
		return nil
	}

	lg.layer2Gen.mu.RLock()
	defer lg.layer2Gen.mu.RUnlock()

	result := make(map[string]float64)
	for ifaceName, tput := range lg.layer2Gen.interfaceThroughput {
		tput.mu.RLock()
		result[ifaceName] = tput.Mbps
		tput.mu.RUnlock()
	}

	return result
}

// StopLayer2 stops Layer 2 load generation
func (lg *NetworkLoadGenerator) StopLayer2() {
	if lg.layer2Gen == nil {
		return
	}

	lg.layer2Gen.mu.Lock()
	defer lg.layer2Gen.mu.Unlock()

	// Close all stop channels
	for _, stopChan := range lg.layer2Gen.stopChans {
		close(stopChan)
	}

	// Close all pcap handles
	for _, handle := range lg.layer2Gen.handles {
		handle.Close()
	}

	// Reset
	lg.layer2Gen.handles = make(map[string]*pcap.Handle)
	lg.layer2Gen.stopChans = make(map[string]chan struct{})
}

// SetLayer2InterfaceTargetThroughput updates target throughput for an interface
func (lg *NetworkLoadGenerator) SetLayer2InterfaceTargetThroughput(ifaceName string, targetMbps float64) {
	if lg.layer2Gen == nil {
		return
	}

	lg.layer2Gen.mu.Lock()
	defer lg.layer2Gen.mu.Unlock()

	lg.layer2Gen.targetThroughput[ifaceName] = targetMbps
}

// GetLayer2TargetThroughputByInterface returns target throughput per interface
func (lg *NetworkLoadGenerator) GetLayer2TargetThroughputByInterface() map[string]float64 {
	if lg.layer2Gen == nil {
		return nil
	}

	lg.layer2Gen.mu.RLock()
	defer lg.layer2Gen.mu.RUnlock()

	result := make(map[string]float64)
	for ifaceName, target := range lg.layer2Gen.targetThroughput {
		result[ifaceName] = target
	}

	return result
}
