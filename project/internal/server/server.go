package server

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"math"
	"net/http"
	"strconv"
	"sync"
	"time"

	"project/internal/database"
	"project/internal/loadgen"
	"project/internal/network"
	"project/internal/runner"
)

type Server struct {
	runner    *runner.Runner
	db        *database.Database
	broker    *Broker
	discovery *network.Discovery
	mu        sync.Mutex
	cancel    context.CancelFunc
}

func NewServer(r *runner.Runner, db *database.Database) *Server {
	return &Server{
		runner:    r,
		db:        db,
		broker:    NewBroker(),
		discovery: network.NewDiscovery(),
	}
}

func (s *Server) Start(addr string) error {
	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("web/static"))))
	http.HandleFunc("/", s.handleIndex)
	http.HandleFunc("/analysis", s.handleAnalysis)
	http.HandleFunc("/start", s.handleStart)
	http.HandleFunc("/stop", s.handleStop)
	http.HandleFunc("/marker", s.handleAddMarker)
	http.HandleFunc("/test-fritzbox", s.handleTestFritzbox)
	http.HandleFunc("/test-target", s.handleTestTarget)
	http.HandleFunc("/interfaces", s.handleGetInterfaces)
	http.HandleFunc("/events", s.broker.ServeHTTP)

	// Database endpoints
	http.HandleFunc("/tests", s.handleListTests)
	http.HandleFunc("/tests/", s.handleGetTest)
	http.HandleFunc("/tests/delete/", s.handleDeleteTest)

	// Discovery endpoints
	http.HandleFunc("/discover", s.handleDiscover)
	http.HandleFunc("/discovered-devices", s.handleGetDiscoveredDevices)
	http.HandleFunc("/pcap-devices", s.handleListPcapDevices)

	log.Printf("Server listening on %s", addr)
	return http.ListenAndServe(addr, nil)
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	tmpl, err := template.ParseFiles("web/templates/index.html")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	tmpl.Execute(w, nil)
}

func (s *Server) handleAnalysis(w http.ResponseWriter, r *http.Request) {
	tmpl, err := template.ParseFiles("web/templates/analysis.html")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	tmpl.Execute(w, nil)
}

func (s *Server) handleStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.mu.Lock()
	if s.cancel != nil {
		s.mu.Unlock()
		http.Error(w, "Test already running", http.StatusConflict)
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.mu.Unlock()

	// Parse form values
	testName := r.FormValue("test_name")
	if testName == "" {
		testName = "Unnamed Test"
	}

	deviceName := r.FormValue("device_name")
	if deviceName == "" {
		deviceName = "Unknown Device"
	}

	durationStr := r.FormValue("duration")
	duration, _ := time.ParseDuration(durationStr)
	if duration == 0 {
		duration = 1 * time.Minute
	}

	pollIntervalStr := r.FormValue("poll_interval")
	pollInterval, _ := time.ParseDuration(pollIntervalStr)
	if pollInterval == 0 {
		pollInterval = 60 * time.Second
	}

	preTestStr := r.FormValue("pre_test_time")
	preTestTime, _ := time.ParseDuration(preTestStr)

	postTestStr := r.FormValue("post_test_time")
	postTestTime, _ := time.ParseDuration(postTestStr)

	loadEnabled := r.FormValue("load_enabled") == "on"
	targetIP := r.FormValue("target_ip")
	
	targetPort, _ := strconv.Atoi(r.FormValue("target_port"))
	if targetPort == 0 {
		targetPort = 9 // Default discard
	}

	protocol := r.FormValue("protocol")
	if protocol == "" {
		protocol = "udp"
	}

	targetMAC := r.FormValue("target_mac")

	packetSize, _ := strconv.Atoi(r.FormValue("packet_size"))
	if packetSize == 0 {
		packetSize = 1400
	}

	// Parse per-interface configurations
	r.ParseForm()
	interfaces := r.Form["interfaces"]
	
	var interfaceConfigs []loadgen.InterfaceConfig
	for _, ifaceName := range interfaces {
		workers, _ := strconv.Atoi(r.FormValue("workers_" + ifaceName))
		if workers == 0 {
			workers = 10 // Default: 10 workers for good balance
		}
		throughput, _ := strconv.ParseFloat(r.FormValue("throughput_" + ifaceName), 64)
		rampSteps, _ := strconv.Atoi(r.FormValue("ramp_" + ifaceName))
		preTime, _ := time.ParseDuration(r.FormValue("pretime_" + ifaceName))
		rampDuration, _ := time.ParseDuration(r.FormValue("rampduration_" + ifaceName))

		interfaceConfigs = append(interfaceConfigs, loadgen.InterfaceConfig{
			Name:             ifaceName,
			Workers:          workers,
			TargetThroughput: throughput,
			RampSteps:        rampSteps,
			PreTime:          preTime,
			RampDuration:     rampDuration,
		})
	}

	// If no interfaces selected, use OS routing with default config
	if len(interfaceConfigs) == 0 {
		interfaceConfigs = []loadgen.InterfaceConfig{{
			Name:             "",
			Workers:          16,
			TargetThroughput: 0,
			RampSteps:        0,
			PreTime:          0,
			RampDuration:     0,
		}}
	}

	// Build load generation config
	loadConfig := loadgen.Config{
		TargetIP:         targetIP,
		TargetPort:       targetPort,
		Protocol:         protocol,
		TargetMAC:        targetMAC,
		PacketSize:       packetSize,
		InterfaceConfigs: interfaceConfigs,
	}

	config := runner.TestConfig{
		Duration:     duration,
		Interval:     pollInterval,
		PreTestTime:  preTestTime,
		PostTestTime: postTestTime,
		Description:  "Web UI Test",
		TestName:     testName,
		DeviceName:   deviceName,
		LoadEnabled:  loadEnabled,
		LoadConfig:   loadConfig,
	}

	go func() {
		defer func() {
			s.mu.Lock()
			s.cancel = nil
			s.mu.Unlock()
			s.broker.Broadcast([]byte("event: done\ndata: Test finished\n\n"))
		}()

		updateChan := make(chan runner.DataPoint)
		
		// Forward updates to SSE broker
		go func() {
			for dp := range updateChan {
				data, _ := json.Marshal(dp)
				msg := fmt.Sprintf("data: %s\n\n", data)
				s.broker.Broadcast([]byte(msg))
			}
		}()

		result, err := s.runner.RunTest(ctx, config, updateChan)
		if err != nil {
			log.Printf("Test failed: %v", err)
		} else {
			log.Printf("Test finished. Collected %d data points.", len(result.DataPoints))

			// Save to database
			if s.db != nil {
				if err := s.saveTestToDatabase(result); err != nil {
					log.Printf("Failed to save test to database: %v", err)
				} else {
					log.Printf("Test saved to database successfully")
				}
			}
		}
		close(updateChan)
	}()

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Test started"))
}

func (s *Server) handleStop(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
		w.Write([]byte("Test stopped"))
	} else {
		w.Write([]byte("No test running"))
	}
}

func (s *Server) handleAddMarker(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	message := r.FormValue("message")
	if message == "" {
		http.Error(w, "Message is required", http.StatusBadRequest)
		return
	}

	if !s.runner.IsTestActive() {
		http.Error(w, "No test running", http.StatusConflict)
		return
	}

	if s.runner.AddCustomMarker(message) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Marker added"))
	} else {
		http.Error(w, "Failed to add marker", http.StatusInternalServerError)
	}
}

func (s *Server) handleGetInterfaces(w http.ResponseWriter, r *http.Request) {
	ifaces, err := network.GetAvailableInterfaces()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Filter to only show up and non-loopback interfaces
	var filtered []network.Interface
	for _, iface := range ifaces {
		if iface.IsUp && !iface.IsLoopback && len(iface.Addresses) > 0 {
			filtered = append(filtered, iface)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(filtered)
}

func (s *Server) handleTestFritzbox(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	log.Println("Testing Fritzbox connection...")
	err := s.runner.TestFritzboxConnection()
	if err != nil {
		log.Printf("Fritzbox connection failed: %v", err)
	} else {
		log.Println("Fritzbox connection successful")
	}

	response := map[string]interface{}{
		"ok":    err == nil,
		"error": "",
	}
	if err != nil {
		response["error"] = err.Error()
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (s *Server) handleTestTarget(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	targetIP := r.FormValue("target_ip")
	targetPort, _ := strconv.Atoi(r.FormValue("target_port"))
	if targetPort == 0 {
		targetPort = 80
	}

	log.Printf("Testing Target connection to %s:%d...", targetIP, targetPort)
	err := s.runner.TestTargetConnection(targetIP, targetPort)
	if err != nil {
		log.Printf("Target connection failed: %v", err)
	} else {
		log.Println("Target connection successful")
	}

	response := map[string]interface{}{
		"ok":    err == nil,
		"error": "",
	}
	if err != nil {
		response["error"] = err.Error()
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// Broker handles SSE clients
type Broker struct {
	clients    map[chan []byte]bool
	newClients chan chan []byte
	defunct    chan chan []byte
	messages   chan []byte
}

func NewBroker() *Broker {
	b := &Broker{
		clients:    make(map[chan []byte]bool),
		newClients: make(chan chan []byte),
		defunct:    make(chan chan []byte),
		messages:   make(chan []byte),
	}
	go b.start()
	return b
}

func (b *Broker) start() {
	for {
		select {
		case s := <-b.newClients:
			b.clients[s] = true
		case s := <-b.defunct:
			delete(b.clients, s)
			close(s)
		case msg := <-b.messages:
			for s := range b.clients {
				s <- msg
			}
		}
	}
}

func (b *Broker) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	messageChan := make(chan []byte)
	b.newClients <- messageChan

	notify := r.Context().Done()

	go func() {
		<-notify
		b.defunct <- messageChan
	}()

	for {
		msg, open := <-messageChan
		if !open {
			break
		}
		w.Write(msg)
		w.(http.Flusher).Flush()
	}
}

func (b *Broker) Broadcast(msg []byte) {
	b.messages <- msg
}

// saveTestToDatabase saves a test result to the database
func (s *Server) saveTestToDatabase(result *runner.TestResult) error {
	// Marshal config and data to JSON
	configJSON, err := json.Marshal(result.Config)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	dataJSON, err := json.Marshal(result.DataPoints)
	if err != nil {
		return fmt.Errorf("failed to marshal data: %w", err)
	}

	// Calculate summary statistics
	summary := s.calculateTestSummary(result)
	summaryJSON, err := json.Marshal(summary)
	if err != nil {
		return fmt.Errorf("failed to marshal summary: %w", err)
	}

	// Create test record
	record := &database.TestRecord{
		TestName:   result.Config.TestName,
		DeviceName: result.Config.DeviceName,
		Timestamp:  result.StartTime,
		Config:     string(configJSON),
		Data:       string(dataJSON),
		Summary:    string(summaryJSON),
	}

	_, err = s.db.SaveTest(record)
	return err
}

// calculateTestSummary calculates summary statistics from test data
func (s *Server) calculateTestSummary(result *runner.TestResult) *database.TestSummary {
	summary := &database.TestSummary{
		DurationSeconds: result.EndTime.Sub(result.StartTime).Seconds(),
		PhaseStats:      make(map[string]database.PhaseStats),
	}

	if len(result.DataPoints) == 0 {
		return summary
	}

	// Calculate overall statistics
	var totalPower, minPower, maxPower float64
	var totalThroughput, maxThroughput float64
	minPower = math.MaxFloat64

	// Group data points by phase
	phaseData := make(map[runner.Phase][]runner.DataPoint)

	for _, dp := range result.DataPoints {
		// Overall stats
		totalPower += dp.PowerMW
		if dp.PowerMW < minPower {
			minPower = dp.PowerMW
		}
		if dp.PowerMW > maxPower {
			maxPower = dp.PowerMW
		}
		totalThroughput += dp.ThroughputMbps
		if dp.ThroughputMbps > maxThroughput {
			maxThroughput = dp.ThroughputMbps
		}

		// Group by phase
		phaseData[dp.Phase] = append(phaseData[dp.Phase], dp)
	}

	summary.AveragePowerMW = totalPower / float64(len(result.DataPoints))
	summary.MinPowerMW = minPower
	summary.MaxPowerMW = maxPower
	summary.AverageThroughputMbps = totalThroughput / float64(len(result.DataPoints))
	summary.MaxThroughputMbps = maxThroughput
	summary.TotalDataPoints = len(result.DataPoints)

	// Calculate per-phase statistics
	for phase, points := range phaseData {
		if len(points) == 0 {
			continue
		}

		var powerSum, throughputSum float64
		var powerValues, throughputValues []float64

		for _, dp := range points {
			powerSum += dp.PowerMW
			throughputSum += dp.ThroughputMbps
			powerValues = append(powerValues, dp.PowerMW)
			throughputValues = append(throughputValues, dp.ThroughputMbps)
		}

		avgPower := powerSum / float64(len(points))
		avgThroughput := throughputSum / float64(len(points))

		// Calculate standard deviation
		var powerVariance, throughputVariance float64
		for _, v := range powerValues {
			diff := v - avgPower
			powerVariance += diff * diff
		}
		for _, v := range throughputValues {
			diff := v - avgThroughput
			throughputVariance += diff * diff
		}

		powerStdDev := math.Sqrt(powerVariance / float64(len(points)))
		throughputStdDev := math.Sqrt(throughputVariance / float64(len(points)))

		phaseName := string(phase)
		summary.PhaseStats[phaseName] = database.PhaseStats{
			DurationSeconds:       float64(len(points)) * result.Config.Interval.Seconds(),
			AveragePowerMW:        avgPower,
			PowerStdDevMW:         powerStdDev,
			AverageThroughputMbps: avgThroughput,
			ThroughputStdDevMbps:  throughputStdDev,
			DataPointCount:        len(points),
		}
	}

	return summary
}

// handleListTests returns all saved tests
func (s *Server) handleListTests(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	tests, err := s.db.ListTests()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Return lightweight version (without full data)
	type TestListItem struct {
		ID         int64     `json:"id"`
		TestName   string    `json:"test_name"`
		DeviceName string    `json:"device_name"`
		Timestamp  time.Time `json:"timestamp"`
		CreatedAt  time.Time `json:"created_at"`
	}

	var items []TestListItem
	for _, test := range tests {
		items = append(items, TestListItem{
			ID:         test.ID,
			TestName:   test.TestName,
			DeviceName: test.DeviceName,
			Timestamp:  test.Timestamp,
			CreatedAt:  test.CreatedAt,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(items)
}

// handleGetTest returns a specific test by ID
func (s *Server) handleGetTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract ID from URL path
	id, err := strconv.ParseInt(r.URL.Path[len("/tests/"):], 10, 64)
	if err != nil {
		http.Error(w, "Invalid test ID", http.StatusBadRequest)
		return
	}

	test, err := s.db.GetTest(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(test)
}

// handleDeleteTest deletes a test by ID
func (s *Server) handleDeleteTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete && r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract ID from URL path
	id, err := strconv.ParseInt(r.URL.Path[len("/tests/delete/"):], 10, 64)
	if err != nil {
		http.Error(w, "Invalid test ID", http.StatusBadRequest)
		return
	}

	err = s.db.DeleteTest(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Test deleted"))
}

// handleDiscover starts network device discovery
func (s *Server) handleDiscover(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ifaceName := r.FormValue("interface")

	// Clear previous results
	s.discovery.Clear()

	// Start discovery in background
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		// First, try to get devices from ARP cache (fast and reliable)
		log.Printf("Reading system ARP cache")
		if err := s.discovery.GetARPCacheDevices(); err != nil {
			log.Printf("ARP cache read error (non-fatal): %v", err)
		} else {
			cacheDevices := s.discovery.GetDevices()
			log.Printf("Found %d devices in ARP cache", len(cacheDevices))
		}

		// Then, optionally do active ARP scanning (slower but more thorough)
		var err error
		if ifaceName != "" {
			log.Printf("Starting active ARP scan on interface: %s", ifaceName)
			err = s.discovery.ScanInterface(ctx, ifaceName)
		} else {
			log.Printf("Starting active ARP scan on all interfaces")
			err = s.discovery.ScanAllInterfaces(ctx)
		}

		if err != nil {
			log.Printf("Active ARP scan error: %v", err)
		}

		devices := s.discovery.GetDevices()
		log.Printf("Discovery completed. Total devices found: %d", len(devices))
	}()

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Discovery started"))
}

// handleGetDiscoveredDevices returns all discovered devices
func (s *Server) handleGetDiscoveredDevices(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	devices := s.discovery.GetDevices()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(devices)
}

// handleListPcapDevices returns all pcap devices for debugging
func (s *Server) handleListPcapDevices(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	devices, err := network.ListPcapDevices()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(devices)
}
