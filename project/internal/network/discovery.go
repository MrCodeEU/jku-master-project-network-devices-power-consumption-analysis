package network

import (
	"context"
	"fmt"
	"net"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcap"
)

// DiscoveredDevice represents a device found on the network
type DiscoveredDevice struct {
	IPAddress  string `json:"ip_address"`
	MACAddress string `json:"mac_address"`
	Interface  string `json:"interface"`
	Hostname   string `json:"hostname,omitempty"`
	Vendor     string `json:"vendor,omitempty"`
	LastSeen   time.Time `json:"last_seen"`
}

// Discovery handles network device discovery
type Discovery struct {
	mu      sync.RWMutex
	devices map[string]*DiscoveredDevice // Key: MAC address
}

// NewDiscovery creates a new discovery instance
func NewDiscovery() *Discovery {
	return &Discovery{
		devices: make(map[string]*DiscoveredDevice),
	}
}

// ScanInterface scans a specific interface for devices
func (d *Discovery) ScanInterface(ctx context.Context, ifaceName string) error {
	iface, err := net.InterfaceByName(ifaceName)
	if err != nil {
		return fmt.Errorf("failed to get interface %s: %w", ifaceName, err)
	}

	// Get interface addresses
	addrs, err := iface.Addrs()
	if err != nil {
		return fmt.Errorf("failed to get addresses for %s: %w", ifaceName, err)
	}

	// Find IPv4 address and network
	var ipNet *net.IPNet
	var srcIP net.IP
	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && ipnet.IP.To4() != nil {
			ipNet = ipnet
			srcIP = ipnet.IP
			break
		}
	}

	if ipNet == nil {
		return fmt.Errorf("no IPv4 address found for interface %s", ifaceName)
	}

	// Get pcap device name for this interface
	pcapDeviceName, err := getPcapDeviceName(ifaceName)
	if err != nil {
		return fmt.Errorf("failed to find pcap device for %s: %w", ifaceName, err)
	}

	// Open pcap handle for capturing responses
	handle, err := pcap.OpenLive(pcapDeviceName, 65536, true, pcap.BlockForever)
	if err != nil {
		return fmt.Errorf("failed to open pcap on %s (device: %s): %w", ifaceName, pcapDeviceName, err)
	}
	defer handle.Close()

	// Set filter for ARP packets
	if err := handle.SetBPFFilter("arp"); err != nil {
		return fmt.Errorf("failed to set BPF filter: %w", err)
	}

	// Start listening for ARP responses
	packetSource := gopacket.NewPacketSource(handle, handle.LinkType())
	packets := packetSource.Packets()

	// Start ARP response listener in background
	responseDone := make(chan struct{})
	go func() {
		defer close(responseDone)
		timeout := time.After(5 * time.Second)
		for {
			select {
			case <-ctx.Done():
				return
			case <-timeout:
				return
			case packet := <-packets:
				if packet == nil {
					continue
				}
				d.processARPPacket(packet, ifaceName)
			}
		}
	}()

	// Send ARP requests to all IPs in the subnet
	if err := d.sendARPRequests(handle, iface, srcIP, ipNet); err != nil {
		return err
	}

	// Wait for responses
	<-responseDone

	return nil
}

// sendARPRequests sends ARP requests to all IPs in the subnet
func (d *Discovery) sendARPRequests(handle *pcap.Handle, iface *net.Interface, srcIP net.IP, ipNet *net.IPNet) error {
	// Calculate network range
	ip := ipNet.IP.Mask(ipNet.Mask)
	var ips []net.IP

	for ip := ip.Mask(ipNet.Mask); ipNet.Contains(ip); inc(ip) {
		ips = append(ips, net.IP(append([]byte{}, ip...)))
	}

	fmt.Printf("Scanning %d addresses on %s (%s)\n", len(ips), iface.Name, ipNet)

	// Send ARP request for each IP
	for _, dstIP := range ips {
		if dstIP.Equal(srcIP) {
			continue // Skip our own IP
		}

		// Create ARP request
		eth := &layers.Ethernet{
			SrcMAC:       iface.HardwareAddr,
			DstMAC:       net.HardwareAddr{0xff, 0xff, 0xff, 0xff, 0xff, 0xff},
			EthernetType: layers.EthernetTypeARP,
		}

		arp := &layers.ARP{
			AddrType:          layers.LinkTypeEthernet,
			Protocol:          layers.EthernetTypeIPv4,
			HwAddressSize:     6,
			ProtAddressSize:   4,
			Operation:         layers.ARPRequest,
			SourceHwAddress:   iface.HardwareAddr,
			SourceProtAddress: srcIP.To4(),
			DstHwAddress:      []byte{0, 0, 0, 0, 0, 0},
			DstProtAddress:    dstIP.To4(),
		}

		// Serialize and send
		buf := gopacket.NewSerializeBuffer()
		opts := gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}
		if err := gopacket.SerializeLayers(buf, opts, eth, arp); err != nil {
			continue
		}

		if err := handle.WritePacketData(buf.Bytes()); err != nil {
			continue
		}

		// Small delay to avoid flooding
		time.Sleep(time.Millisecond)
	}

	return nil
}

// processARPPacket processes an ARP packet and extracts device information
func (d *Discovery) processARPPacket(packet gopacket.Packet, ifaceName string) {
	arpLayer := packet.Layer(layers.LayerTypeARP)
	if arpLayer == nil {
		return
	}

	arp, _ := arpLayer.(*layers.ARP)
	if arp.Operation != layers.ARPReply {
		return
	}

	// Extract information
	macAddr := net.HardwareAddr(arp.SourceHwAddress)
	ipAddr := net.IP(arp.SourceProtAddress)

	// Try to resolve hostname
	hostname := ""
	names, err := net.LookupAddr(ipAddr.String())
	if err == nil && len(names) > 0 {
		hostname = names[0]
	}

	// Add or update device
	d.mu.Lock()
	defer d.mu.Unlock()

	device := &DiscoveredDevice{
		IPAddress:  ipAddr.String(),
		MACAddress: macAddr.String(),
		Interface:  ifaceName,
		Hostname:   hostname,
		LastSeen:   time.Now(),
	}

	d.devices[macAddr.String()] = device
}

// GetDevices returns all discovered devices
func (d *Discovery) GetDevices() []*DiscoveredDevice {
	d.mu.RLock()
	defer d.mu.RUnlock()

	devices := make([]*DiscoveredDevice, 0, len(d.devices))
	for _, device := range d.devices {
		devices = append(devices, device)
	}

	return devices
}

// Clear clears all discovered devices
func (d *Discovery) Clear() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.devices = make(map[string]*DiscoveredDevice)
}

// inc increments an IP address
func inc(ip net.IP) {
	for j := len(ip) - 1; j >= 0; j-- {
		ip[j]++
		if ip[j] > 0 {
			break
		}
	}
}

// ScanAllInterfaces scans all available interfaces
func (d *Discovery) ScanAllInterfaces(ctx context.Context) error {
	interfaces, err := GetAvailableInterfaces()
	if err != nil {
		return err
	}

	var wg sync.WaitGroup
	errChan := make(chan error, len(interfaces))

	for _, iface := range interfaces {
		if !iface.IsUp || iface.IsLoopback || len(iface.Addresses) == 0 {
			continue
		}

		wg.Add(1)
		go func(ifaceName string) {
			defer wg.Done()
			if err := d.ScanInterface(ctx, ifaceName); err != nil {
				errChan <- fmt.Errorf("%s: %w", ifaceName, err)
			}
		}(iface.Name)
	}

	wg.Wait()
	close(errChan)

	// Collect errors
	var errs []error
	for err := range errChan {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		return fmt.Errorf("scan completed with errors: %v", errs)
	}

	return nil
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

	return "", fmt.Errorf("no suitable pcap device found for interface '%s'. Available devices: %d", friendlyName, len(devices))
}

// PcapDevice represents a pcap device
type PcapDevice struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Addresses   []string `json:"addresses"`
}

// ListPcapDevices returns all available pcap devices
func ListPcapDevices() ([]PcapDevice, error) {
	devices, err := pcap.FindAllDevs()
	if err != nil {
		return nil, fmt.Errorf("failed to enumerate pcap devices: %w", err)
	}

	result := make([]PcapDevice, 0, len(devices))
	for _, device := range devices {
		addresses := make([]string, 0, len(device.Addresses))
		for _, addr := range device.Addresses {
			if addr.IP != nil {
				addresses = append(addresses, addr.IP.String())
			}
		}

		result = append(result, PcapDevice{
			Name:        device.Name,
			Description: device.Description,
			Addresses:   addresses,
		})
	}

	return result, nil
}

// GetARPCacheDevices returns devices from the system ARP cache
// This is faster and more reliable than ARP scanning, especially on Windows
func (d *Discovery) GetARPCacheDevices() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Run 'arp -a' command
	cmd := exec.Command("arp", "-a")
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to run arp -a: %w", err)
	}

	// Parse output
	// Windows format (English):
	//   Interface: 192.168.50.107 --- 0xc
	//     Internet Address      Physical Address      Type
	//     192.168.50.1          xx-xx-xx-xx-xx-xx     dynamic
	// Windows format (German):
	//   Schnittstelle: 172.17.0.1 --- 0x24
	//     Internetadresse       Physische Adresse     Typ
	//     172.17.15.255         ff-ff-ff-ff-ff-ff     statisch

	lines := strings.Split(string(output), "\n")
	var currentInterface string

	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Parse interface line (multilingual support)
		if strings.HasPrefix(line, "Interface:") || strings.HasPrefix(line, "Schnittstelle:") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				currentInterface = parts[1]
			}
			continue
		}

		// Skip header lines (multilingual support)
		if line == "" ||
		   strings.Contains(line, "Internet Address") ||
		   strings.Contains(line, "Physical Address") ||
		   strings.Contains(line, "Internetadresse") ||
		   strings.Contains(line, "Physische Adresse") {
			continue
		}

		// Parse ARP entry: IP, MAC, Type
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}

		ipAddr := fields[0]

		// Check if MAC address field exists and is not empty
		var macAddr string
		if len(fields) >= 2 && fields[1] != "" {
			macAddr = fields[1]
		} else {
			// MAC address missing (incomplete entry)
			continue
		}

		// Skip invalid/special MAC addresses
		if macAddr == "(incomplete)" ||
		   macAddr == "" ||
		   macAddr == "ff-ff-ff-ff-ff-ff" || // Broadcast
		   strings.HasPrefix(macAddr, "01-00-5e") { // Multicast
			continue
		}

		// Skip multicast and broadcast IP addresses
		if strings.HasPrefix(ipAddr, "224.") ||
		   strings.HasPrefix(ipAddr, "239.") ||
		   strings.HasSuffix(ipAddr, ".255") ||
		   ipAddr == "255.255.255.255" {
			continue
		}

		// Validate MAC address format (should have 5 separators)
		if strings.Count(macAddr, "-") != 5 {
			continue
		}

		// Normalize MAC address format (Windows uses - instead of :)
		macAddr = strings.ReplaceAll(macAddr, "-", ":")

		// Try to resolve hostname
		hostname := ""
		names, err := net.LookupAddr(ipAddr)
		if err == nil && len(names) > 0 {
			hostname = names[0]
		}

		// Find which interface this belongs to
		interfaceName := currentInterface
		if interfaceName != "" {
			// Try to get the friendly interface name
			ifaces, _ := net.Interfaces()
			for _, iface := range ifaces {
				addrs, _ := iface.Addrs()
				for _, addr := range addrs {
					if ipnet, ok := addr.(*net.IPNet); ok && ipnet.IP.String() == interfaceName {
						interfaceName = iface.Name
						break
					}
				}
			}
		}

		// Add or update device
		device := &DiscoveredDevice{
			IPAddress:  ipAddr,
			MACAddress: macAddr,
			Interface:  interfaceName,
			Hostname:   hostname,
			LastSeen:   time.Now(),
		}

		d.devices[macAddr] = device
	}

	return nil
}
