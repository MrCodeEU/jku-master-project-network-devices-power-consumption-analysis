package network

import (
	"fmt"
	"net"
	"strings"
)

// Interface represents a network interface with its details
type Interface struct {
	Name      string   `json:"name"`
	Addresses []string `json:"addresses"`
	IsUp      bool     `json:"is_up"`
	IsLoopback bool    `json:"is_loopback"`
}

// GetAvailableInterfaces returns all non-loopback interfaces with IPv4 addresses
func GetAvailableInterfaces() ([]Interface, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, fmt.Errorf("failed to get interfaces: %w", err)
	}

	var result []Interface

	for _, iface := range ifaces {
		// Skip interfaces that are down
		isUp := iface.Flags&net.FlagUp != 0
		isLoopback := iface.Flags&net.FlagLoopback != 0

		// Get addresses for this interface
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		var ipv4Addrs []string
		for _, addr := range addrs {
			// Get IP from address
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}

			// Only include IPv4 addresses
			if ip != nil && ip.To4() != nil && !ip.IsLoopback() {
				ipv4Addrs = append(ipv4Addrs, ip.String())
			}
		}

		// Only include interfaces with at least one IPv4 address (or loopback for completeness)
		if len(ipv4Addrs) > 0 || isLoopback {
			result = append(result, Interface{
				Name:       iface.Name,
				Addresses:  ipv4Addrs,
				IsUp:       isUp,
				IsLoopback: isLoopback,
			})
		}
	}

	return result, nil
}

// GetInterfaceIP returns the first IPv4 address for a given interface name
func GetInterfaceIP(ifaceName string) (string, error) {
	iface, err := net.InterfaceByName(ifaceName)
	if err != nil {
		return "", fmt.Errorf("interface %s not found: %w", ifaceName, err)
	}

	addrs, err := iface.Addrs()
	if err != nil {
		return "", fmt.Errorf("failed to get addresses for %s: %w", ifaceName, err)
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
			return ip.String(), nil
		}
	}

	return "", fmt.Errorf("no IPv4 address found for interface %s", ifaceName)
}

// FormatInterfaceDisplay creates a human-readable string for an interface
func FormatInterfaceDisplay(iface Interface) string {
	status := "down"
	if iface.IsUp {
		status = "up"
	}
	if iface.IsLoopback {
		return fmt.Sprintf("%s (loopback)", iface.Name)
	}
	return fmt.Sprintf("%s (%s) - %s", iface.Name, status, strings.Join(iface.Addresses, ", "))
}
