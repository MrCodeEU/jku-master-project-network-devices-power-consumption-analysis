package server

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"project/internal/network"
	"project/internal/runner"
)

type Server struct {
	runner *runner.Runner
	broker *Broker
	mu     sync.Mutex
	cancel context.CancelFunc
}

func NewServer(r *runner.Runner) *Server {
	return &Server{
		runner: r,
		broker: NewBroker(),
	}
}

func (s *Server) Start(addr string) error {
	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("web/static"))))
	http.HandleFunc("/", s.handleIndex)
	http.HandleFunc("/start", s.handleStart)
	http.HandleFunc("/stop", s.handleStop)
	http.HandleFunc("/test-fritzbox", s.handleTestFritzbox)
	http.HandleFunc("/test-target", s.handleTestTarget)
	http.HandleFunc("/interfaces", s.handleGetInterfaces)
	http.HandleFunc("/events", s.broker.ServeHTTP)

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

	workers, _ := strconv.Atoi(r.FormValue("workers"))
	if workers == 0 {
		workers = 8
	}

	packetSize, _ := strconv.Atoi(r.FormValue("packet_size"))
	if packetSize == 0 {
		packetSize = 1400
	}

	// Parse selected interfaces
	r.ParseForm()
	interfaces := r.Form["interfaces"]
	if len(interfaces) == 0 {
		// If no interfaces selected, check for comma-separated value
		ifaceStr := r.FormValue("interfaces")
		if ifaceStr != "" {
			interfaces = strings.Split(ifaceStr, ",")
		}
	}

	// Parse throughput target and ramping
	targetThroughput, _ := strconv.ParseFloat(r.FormValue("target_throughput"), 64)
	rampSteps, _ := strconv.Atoi(r.FormValue("ramp_steps"))

	config := runner.TestConfig{
		Duration:         duration,
		Interval:         pollInterval,
		PreTestTime:      preTestTime,
		PostTestTime:     postTestTime,
		Description:      "Web UI Test",
		LoadEnabled:      loadEnabled,
		TargetIP:         targetIP,
		TargetPort:       targetPort,
		Protocol:         protocol,
		Workers:          workers,
		PacketSize:       packetSize,
		Interfaces:       interfaces,
		TargetThroughput: targetThroughput,
		RampSteps:        rampSteps,
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
			// TODO: Save report to file
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
