       COMPREHENSIVE PROJECT ARCHITECTURE ANALYSIS

       OVERVIEW

       This is a Power Consumption Test Runner system - a Go backend with a web UI frontend designed to measure power consumption of network devices while optionally subjecting them to network load stress tests. The system supports configurable network load generation, per-interface       
       throughput ramping, power measurement via Fritz!Box API, and comprehensive data collection.

       ---
       1. WEB UI ARCHITECTURE

       HTML Templates

       - Primary Interface: project/web/templates/index.html
         - Full-featured test control and configuration interface
         - Real-time power/throughput charting with Chart.js
         - Test history management (IndexedDB storage)
         - Device discovery UI
       - Secondary Interface: project/web/templates/analysis.html
         - Historical test analysis and visualization
         - More advanced analysis features (indicated by chartjs-plugin-zoom, boxplot charts)

       Form Configuration Elements (index.html)

       The UI captures test settings through forms with these field groups:

       Basic Test Settings:
       - test_name - User-defined name for the test
       - device_name - Name of device under test (e.g., "FritzBox 7530")
       - duration - Test duration (e.g., "60s", "1m")
       - poll_interval - Power meter polling interval (dropdown: 1s to 120s, default 60s)
       - pre_test_time - Baseline measurement before load (e.g., "2m")
       - post_test_time - Baseline measurement after load (e.g., "2m")
       - power_y_min - Chart Y-axis minimum for power graph

       Load Generation Settings (conditional on load_enabled checkbox):
       - load_enabled - Checkbox to enable/disable network load
       - target_ip - Target device IP address
       - target_port - Target port (default 80)
       - protocol - Dropdown: "udp" (default), "tcp", "layer2"
       - packet_size - Packet size in bytes (default 1400, optimal for no fragmentation)
       - target_mac - MAC address (shown only for Layer 2 protocol)

       Per-Interface Configuration (dynamic):
       For each selected interface checkbox:
       - workers_[iface] - Number of worker threads (1-64, default 10)
       - throughput_[iface] - Target throughput in Mbps (0 = unlimited)
       - ramp_[iface] - Number of ramp-up steps (0 = no ramping)
       - pretime_[iface] - Per-interface pre-delay before start
       - rampduration_[iface] - Duration of ramping phase

       ---
       2. JAVASCRIPT/FRONTEND LOGIC (app.js - 1,770 lines)

       Key Features Implemented in Frontend:

       Configuration Persistence:
       - CONFIG_STORAGE_KEY = 'PowerTestConfig'
       - Uses localStorage for basic settings
       - Uses IndexedDB (PowerTestDB) for test history storage (local client-side only)
       - Auto-saves config before test starts

       Interface Management:
       - /interfaces endpoint called to fetch available network interfaces
       - Dynamically builds per-interface config cards with checkboxes
       - Stores and restores interface-specific settings

       Real-time Data Visualization:
       - Power Consumption Chart: Line chart showing power (mW) vs time
       - Throughput Chart: Total throughput + per-interface throughput datasets
       - Expected Throughput Preview: Shows expected load profile based on configuration
       - Annotation system for marking phases and events

       Test Execution Flow:
       1. Form submission → POST /start with FormData
       2. Opens EventSource to /events for SSE streaming
       3. Receives DataPoints containing: timestamp, power, throughput, phase, events
       4. Updates charts in real-time
       5. Collects data for CSV export
       6. On completion: event: done message closes connection

       Test History Management:
       - IndexedDB local storage (client-side only)
       - Load test data into charts
       - Download CSV from saved tests
       - Delete tests

       Progress Tracking:
       - Calculates total test duration from: preTestTime + duration + postTestTime
       - Builds event timeline (phase changes, ramp steps, interface starts)
       - Updates progress bar, time remaining, current phase, upcoming events

       Advanced Features:
       - Device discovery (ARP scanning via /discover, /discovered-devices)
       - Pcap device listing (/pcap-devices)
       - Connection testing: Fritzbox (/test-fritzbox), Target device (/test-target)
       - Custom markers during active test (/marker)
       - Protocol-aware UI (Layer 2 shows MAC field, hides IP/Port for Layer 2)

       ---
       3. LOAD GENERATOR SYSTEM

       Core Load Generator Interface (loadgen.go)

       Config Structure:
       type Config struct {
           TargetIP         string              // Target IP to send packets to
           TargetPort       int                 // Target port
           Protocol         string              // "udp", "tcp", or "layer2"
           PacketSize       int                 // Size of each packet in bytes
           TargetMAC        string              // For Layer 2 mode
           InterfaceConfigs []InterfaceConfig   // Per-interface configurations
       }

       type InterfaceConfig struct {
           Name             string              // Interface name (empty = OS routing)
           Workers          int                 // Number of worker threads
           TargetThroughput float64             // Target Mbps (0 = unlimited)
           RampSteps        int                 // Number of ramp steps (0 = no ramping)
           PreTime          time.Duration       // Delay before interface starts
           RampDuration     time.Duration       // Duration of ramping (0 = automatic)
       }

       Three Protocol Modes:
       1. UDP (runUDPWorkerWithConfig) - Connectionless, creates UDP connections
       2. TCP (runTCPWorkerWithConfig) - Connection-oriented, establishes TCP connections
       3. Layer 2 (StartLayer2) - Raw Ethernet frames using pcap library

       Throughput Rate Limiting:
       - Per-worker packet delays calculated from target throughput
       - Formula: delay = (TargetThroughput * 1,000,000 / 8) / workers / packet_size
       - Uses PreciseSleep (Windows high-resolution timers + spin-wait)
       - 5% compensation for syscall overhead
       - Batching optimization: sends 10 packets before sleeping to reduce overhead

       Throughput Tracking:
       - Atomic counters for thread-safe byte counting (no lock per packet)
       - Lock only taken every 1+ second to calculate Mbps
       - Per-interface throughput tracking in InterfaceThroughput structs
       - Methods:
         - GetThroughput() - Returns total Mbps
         - GetThroughputByInterface() - Returns map of interface→Mbps
         - GetTargetThroughputByInterface() - Returns target Mbps per interface
         - SetInterfaceTargetThroughput(ifaceName, mbps) - Dynamic update (for ramping)

       Layer 2 Support:
       - Uses gopacket + pcap libraries for raw Ethernet frame generation
       - Maps friendly interface names to pcap device names
       - Per-interface byte counters for throughput calculation

       ---
       4. TEST RUNNER & ORCHESTRATION (runner.go)

       Test Configuration & Phases

       TestConfig Structure:
       type TestConfig struct {
           Duration     time.Duration       // Main load test duration
           Interval     time.Duration       // Power meter polling interval
           PreTestTime  time.Duration       // Baseline before load
           PostTestTime time.Duration       // Baseline after load
           Description  string              // Test description
           TestName     string              // User-defined name
           DeviceName   string              // Device under test name
           LoadEnabled  bool                // Enable load generation
           LoadConfig   loadgen.Config      // Load generation settings
       }

       Three Test Phases:
       const (
           PhasePreTest  Phase = "pre"      // Baseline measurements (no load)
           PhaseLoad     Phase = "load"     // Active load generation
           PhasePostTest Phase = "post"     // Recovery baseline (no load)
       )

       Power Measurement Integration

       - RunTest() calls r.meter.GetCurrentPower() every Interval seconds
       - Measurement is INDEPENDENT from load generation:
         - Power readings happen in collectData() loop regardless of load state
         - Works during all phases (pre, load, post)
         - Always sampled at configured interval

       Data Collection Flow

       Per Data Point Collected:
       type DataPoint struct {
           Timestamp                   time.Time
           PowerMW                     float64               // From power meter
           ThroughputMbps              float64               // Total from load gen
           ThroughputByInterface       map[string]float64    // Per-interface
           TargetThroughputByInterface map[string]float64    // Per-interface target
           Phase                       Phase                 // Current phase
           Events                      []Event               // Phase changes, ramps, etc.
       }

       Event System

       Event Types:
       - EventPhaseChange - Phase transition (pre→load→post)
       - EventRampStep - Throughput ramp step completed
       - EventInterfaceStart - Interface load generation started
       - EventInterfaceStop - Interface stopped
       - EventCustom - User-added markers during test

       Load Generation Orchestration

       1. Pre-Test Phase: No load, measure baseline power for PreTestTime
       2. Load Phase:
         - For each interface in config:
             - Wait for PreTime (per-interface delay)
           - Start load generation worker
           - If ramping enabled: launch runInterfaceRamping() goroutine
         - Measure power every Interval seconds during Duration
         - Ramping progressively increases target via SetInterfaceTargetThroughput()
       3. Post-Test Phase: Stop load, measure recovery baseline for PostTestTime

       Custom Markers

       - AddCustomMarker(message) - Queues event during active test
       - Used to annotate events in timeline (e.g., "Connected wireless device")
       - Appears in both charts and CSV

       ---
       5. POWER MEASUREMENT SYSTEM (fritzbox.go)

       PowerMeter Interface

       type PowerMeter interface {
           GetCurrentPower() (float64, error)  // Returns mW
           TestConnection() error               // Validates connection
       }

       Two Implementations:

       MockPowerMeter:
       - Generates random power data with fluctuations
       - Base: 5000 mW + random changes (-500 to +500)
       - Used for testing/development

       RealPowerMeter (Fritz!Box via TR-064):
       type RealPowerMeter struct {
           Session *soap.SoapSession  // SOAP connection to Fritz!Box
           AIN     string             // Actor Identification Number (device ID)
       }

       Power Measurement Details:
       - Connects to Fritz!Box via TR-064 SOAP protocol (gofritz library)
       - Queries gateway.GetSpecificDeviceInfos(session, AIN)
       - Returns power in centiwatts (0.01W units)
       - Converts to milliwatts: power_cw * 10 = power_mW
       - Returned value: power consumption in milliwatts (mW)

       Coupling with Load Generation:
       - INDEPENDENT: Power meter runs on fixed polling interval
       - Measures power regardless of whether load is running
       - Works during all phases
       - No direct coupling or feedback loop

       ---
       6. HTTP BACKEND & ROUTES (server.go)

       Primary Routes:
       ┌─────────────────────┬─────────────┬───────────────────────────────────┐
       │        Route        │   Method    │              Purpose              │
       ├─────────────────────┼─────────────┼───────────────────────────────────┤
       │ /                   │ GET         │ Serve index.html (main UI)        │
       ├─────────────────────┼─────────────┼───────────────────────────────────┤
       │ /analysis           │ GET         │ Serve analysis.html               │
       ├─────────────────────┼─────────────┼───────────────────────────────────┤
       │ /start              │ POST        │ Start test with form config       │
       ├─────────────────────┼─────────────┼───────────────────────────────────┤
       │ /stop               │ POST        │ Stop active test                  │
       ├─────────────────────┼─────────────┼───────────────────────────────────┤
       │ /marker             │ POST        │ Add custom marker during test     │
       ├─────────────────────┼─────────────┼───────────────────────────────────┤
       │ /test-fritzbox      │ POST        │ Test Fritz!Box connectivity       │
       ├─────────────────────┼─────────────┼───────────────────────────────────┤
       │ /test-target        │ POST        │ Test target device connectivity   │
       ├─────────────────────┼─────────────┼───────────────────────────────────┤
       │ /interfaces         │ GET         │ List available network interfaces │
       ├─────────────────────┼─────────────┼───────────────────────────────────┤
       │ /events             │ GET         │ SSE stream for real-time data     │
       ├─────────────────────┼─────────────┼───────────────────────────────────┤
       │ /tests              │ GET         │ List saved tests (database)       │
       ├─────────────────────┼─────────────┼───────────────────────────────────┤
       │ /tests/{id}         │ GET         │ Get test details                  │
       ├─────────────────────┼─────────────┼───────────────────────────────────┤
       │ /tests/delete/{id}  │ DELETE/POST │ Delete test                       │
       ├─────────────────────┼─────────────┼───────────────────────────────────┤
       │ /discover           │ POST        │ Start ARP network discovery       │
       ├─────────────────────┼─────────────┼───────────────────────────────────┤
       │ /discovered-devices │ GET         │ List discovered devices           │
       ├─────────────────────┼─────────────┼───────────────────────────────────┤
       │ /pcap-devices       │ GET         │ List pcap-compatible devices      │
       ├─────────────────────┼─────────────┼───────────────────────────────────┤
       │ /static/*           │ GET         │ Serve static files (JS, CSS)      │
       └─────────────────────┴─────────────┴───────────────────────────────────┘
       Test Start Handler (handleStart)

       Reads Form Values:
       test_name, device_name, duration, poll_interval,
       pre_test_time, post_test_time,
       load_enabled, target_ip, target_port, protocol, target_mac, packet_size,
       interfaces[], workers_[iface], throughput_[iface], ramp_[iface],
       pretime_[iface], rampduration_[iface]

       Creates:
       - loadgen.Config from form values
       - runner.TestConfig with all parameters
       - Launches test goroutine that:
         - Opens event channel to updateChan
         - Calls runner.RunTest(ctx, config, updateChan)
         - Forwards DataPoints to SSE broker
         - On completion: saves to database

       SSE Broker (Broker struct)

       - Manages WebSocket-like connections for real-time updates
       - Broadcasts JSON-serialized DataPoints to all connected clients
       - Sends event: done message when test finishes

       Data Persistence

       - Server has SQLite database (tests.db)
       - Saves complete test to database when test finishes:
         - TestName, DeviceName, Timestamp
         - Config (JSON)
         - Data points (JSON array)
         - Summary statistics (calculated from data)

       ---
       7. OVERALL DATA FLOW

       Test Execution Timeline:

       [UI Form] → POST /start
           ↓
       [Server] Parse form → Create TestConfig + LoadConfig
           ↓
       [Server] Spawn test goroutine
           ├→ [Runner.RunTest()]
           │   ├→ Phase 1: Pre-test (collect baseline power, no load)
           │   │   └→ Every Interval: meter.GetCurrentPower() → DataPoint
           │   │
           │   ├→ Phase 2: Load test (main duration)
           │   │   ├→ For each interface: spawn load worker with delay
           │   │   ├→ If ramping: spawn ramp goroutine (updates target Mbps every step)
           │   │   ├→ LoadGen.Start(loadCtx, config)
           │   │   │   ├→ For UDP/TCP: spawn workers that send packets
           │   │   │   ├→ Each worker: calculate delay from target throughput
           │   │   │   ├→ Send packet → update atomic byte counter
           │   │   │   └→ Sleep for calculated delay
           │   │   │
           │   │   └→ Every Interval: meter.GetCurrentPower() → DataPoint
           │   │       ├→ power_mw from meter
           │   │       ├→ throughput_mbps from loadGen.GetThroughput()
           │   │       ├→ throughput_by_interface from loadGen.GetThroughputByInterface()
           │   │       ├→ phase = "load"
           │   │       └→ events = [phase change, ramp steps, custom markers, etc.]
           │   │
           │   └→ Phase 3: Post-test (collect recovery baseline, no load)
           │       └→ Every Interval: meter.GetCurrentPower() → DataPoint
           │
           ├→ [SSE Broker] Forward each DataPoint as JSON
           │   └→ [Client] Receive via EventSource('/events')
           │       ├→ Update power chart
           │       ├→ Update throughput chart (total + per-interface)
           │       ├→ Store in collectedData[]
           │       └→ Update progress UI
           │
           └→ [Database] Save complete test record
               └→ config, data[], summary stats

       Client-Side Data Flow:

       SSE DataPoint → Parse JSON
           ├→ Power chart.add(timestamp, power_mw)
           ├→ Throughput chart.add(timestamp, throughput_mbps)
           ├→ Per-interface chart.add(timestamp, throughput_by_interface[iface])
           ├→ Add annotations for events
           ├→ Store in collectedData[] for CSV export
           └→ Update progress bar, phase status, upcoming events

       ---
       8. KEY ARCHITECTURAL INSIGHTS

       Load Generation & Power Measurement COUPLING:

       - NOT TIGHTLY COUPLED - they operate independently
       - Power meter samples at fixed poll_interval (default 60s)
       - Load generator runs continuously with per-packet rate limiting
       - Both feed into same DataPoint structure at sampling time

       Test Settings/Configuration:

       - Global settings: duration, poll_interval, pre/post test times
       - Load settings: target IP, port, protocol, packet size
       - Per-interface settings: workers, target throughput, ramp steps/duration, pre-delay
       - All settings sent as form data from UI → parsed in backend

       Ramping Implementation:

       - If RampSteps > 0: progressively increase target throughput
       - Divided into equal steps over RampDuration
       - Uses SetInterfaceTargetThroughput() to update worker delays dynamically
       - Workers recalculate delays during batching to pick up new target

       Three Modes of Operation:

       1. Power measurement only (load_enabled = false)
       2. Load generation only (no power meter, throughput only)
       3. Combined (simultaneous load generation + power measurement) ← Primary use case

       ---
       9. KEY FILES SUMMARY
       ┌──────────────────────────┬─────────────────────┬───────────────────────────────────────────────────────────────┐
       │           File           │       Purpose       │                      Key Types/Functions                      │
       ├──────────────────────────┼─────────────────────┼───────────────────────────────────────────────────────────────┤
       │ main.go                  │ Entry point         │ Initializes PowerMeter, LoadGenerator, Runner, Server         │
       ├──────────────────────────┼─────────────────────┼───────────────────────────────────────────────────────────────┤
       │ server/server.go         │ HTTP handlers       │ Routes, test orchestration, SSE broker, database persistence  │
       ├──────────────────────────┼─────────────────────┼───────────────────────────────────────────────────────────────┤
       │ runner/runner.go         │ Test orchestration  │ TestConfig, RunTest(), phase management, event queuing        │
       ├──────────────────────────┼─────────────────────┼───────────────────────────────────────────────────────────────┤
       │ loadgen/loadgen.go       │ Load generation     │ NetworkLoadGenerator, protocol workers, throughput tracking   │
       ├──────────────────────────┼─────────────────────┼───────────────────────────────────────────────────────────────┤
       │ fritzbox/fritzbox.go     │ Power metering      │ PowerMeter interface, RealPowerMeter (TR-064), MockPowerMeter │
       ├──────────────────────────┼─────────────────────┼───────────────────────────────────────────────────────────────┤
       │ database/database.go     │ Persistence         │ TestRecord, TestSummary, SQLite schema                        │
       ├──────────────────────────┼─────────────────────┼───────────────────────────────────────────────────────────────┤
       │ network/interfaces.go    │ Interface discovery │ GetAvailableInterfaces()                                      │
       ├──────────────────────────┼─────────────────────┼───────────────────────────────────────────────────────────────┤
       │ network/discovery.go     │ ARP discovery       │ Device scanning, hostname resolution                          │
       ├──────────────────────────┼─────────────────────┼───────────────────────────────────────────────────────────────┤
       │ web/templates/index.html │ Main UI             │ Forms, charts, test history, device discovery                 │
       ├──────────────────────────┼─────────────────────┼───────────────────────────────────────────────────────────────┤
       │ web/static/app.js        │ Frontend logic      │ Configuration, charts, SSE handling, test history             │
       └──────────────────────────┴─────────────────────┴───────────────────────────────────────────────────────────────┘