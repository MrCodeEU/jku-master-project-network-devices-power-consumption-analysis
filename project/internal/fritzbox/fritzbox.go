package fritzbox

import (
	"fmt"
	"math/rand"
	"net/url"
	"strings"

	"github.com/nitram509/gofritz/pkg/soap"
	"github.com/nitram509/gofritz/pkg/tr064/gateway"
)

// PowerMeter defines the interface for reading power consumption
type PowerMeter interface {
	// GetCurrentPower returns the current power consumption in milliwatts (mW)
	GetCurrentPower() (float64, error)
	// TestConnection checks if the power meter is reachable
	TestConnection() error
}

// MockPowerMeter generates random power consumption data for testing
type MockPowerMeter struct {
	basePower float64
}

func NewMockPowerMeter() *MockPowerMeter {
	return &MockPowerMeter{
		basePower: 5000.0, // Start with 5W
	}
}

func (m *MockPowerMeter) GetCurrentPower() (float64, error) {
	// Simulate some fluctuation
	change := (rand.Float64() * 1000) - 500
	m.basePower += change
	if m.basePower < 0 {
		m.basePower = 0
	}
	return m.basePower, nil
}

func (m *MockPowerMeter) TestConnection() error {
	return nil
}

// RealPowerMeter will implement the actual TR-064 communication
type RealPowerMeter struct {
	Session *soap.SoapSession
	AIN     string
}

func NewRealPowerMeter(urlStr, username, password, ain string) *RealPowerMeter {
	// Extract host from URL if present
	host := urlStr
	if strings.Contains(urlStr, "://") {
		u, err := url.Parse(urlStr)
		if err == nil {
			host = u.Host
		}
	}
	
	// gofritz/soap/session.go usually expects just the host (and optional port)
	// It constructs the full URL internally.
	
	fmt.Printf("Initializing RealPowerMeter with Host: %s, User: %s, AIN: %s\n", host, username, ain)

	return &RealPowerMeter{
		Session: soap.NewSession(host, username, password),
		AIN:     ain,
	}
}

func (r *RealPowerMeter) GetCurrentPower() (float64, error) {
	resp, err := gateway.GetSpecificDeviceInfos(r.Session, r.AIN)
	if err != nil {
		return 0, err
	}
	
	// Debug log to see what we get
	fmt.Printf("Device Info: Power=%d, Energy=%d, Present=%s, Switch=%s\n", 
		resp.MultimeterPower, resp.MultimeterEnergy, resp.Present, resp.SwitchState)

	// MultimeterPower is in 0.01 W (centiwatt)
	// We want mW. 1 cW = 10 mW.
	return float64(resp.MultimeterPower) * 10.0, nil
}

func (r *RealPowerMeter) TestConnection() error {
	_, err := gateway.GetSpecificDeviceInfos(r.Session, r.AIN)
	return err
}
