# Power Consumption Test Runner

A comprehensive web-based application for measuring power consumption and network throughput of devices under test. Supports Layer 2 (raw Ethernet), TCP, and UDP load generation with real-time monitoring and detailed analysis capabilities.

## Features

### Load Generation
- **Multiple Protocol Support**: UDP, TCP, and Layer 2 (raw Ethernet frames)
- **Per-Interface Configuration**: Independent worker count, throughput targets, and ramping for each network interface
- **Precise Rate Control**: High-resolution timing on Windows for accurate throughput targeting
- **Progressive Ramping**: Gradually increase load over configurable steps

### Power Measurement
- **Real-Time Monitoring**: Live power consumption tracking via FritzBox smart plug
- **Baseline Comparison**: Pre-test and post-test baseline measurements
- **Phase-Based Analysis**: Separate statistics for idle and load phases

### Data Management
- **SQLite Database**: Persistent storage with searchable test metadata
- **CSV Export**: Compatible with external analysis tools
- **IndexedDB**: Browser-based storage for offline access
- **Test Metadata**: Configurable test names and device identifiers

### Analysis Tools
- **Comprehensive Statistics**: Power, throughput, efficiency, and per-phase metrics
- **Interactive Visualizations**: Real-time charts with Chart.js
- **Excel Export**: Formatted reports with summary statistics and raw data
- **Manual Event Markers**: Annotate tests with custom events

### Network Discovery
- **ARP Scanning**: Automatic device discovery on local networks
- **Multi-Interface Support**: Scan all available interfaces simultaneously
- **MAC Address Detection**: Essential for Layer 2 testing

## Requirements

### System Requirements
- Go 1.21 or later
- Windows 10 1803+ (for high-resolution timers) or Linux
- Administrator/root privileges (for Layer 2 packet capture)

### Dependencies
- **Hardware**: FritzBox router with DECT 200 smart plug (or compatible TR-064 device)
- **Network**: WinPcap or Npcap installed (Windows) or libpcap (Linux) for Layer 2 support

### Go Packages
- `github.com/google/gopacket` - Packet manipulation
- `github.com/mattn/go-sqlite3` - Database support
- `github.com/nitram509/gofritz` - FritzBox integration
- `github.com/joho/godotenv` - Configuration management

## Installation

1. Clone the repository:
```bash
git clone <repository-url>
cd PR/project
```

2. Install dependencies:
```bash
go mod download
```

3. Create a `.env` file with your FritzBox credentials:
```env
FRITZ_URL=http://192.168.1.1
FRITZ_USER=your_username
FRITZ_PASSWORD=your_password
FRITZ_AIN=your_device_ain
DB_PATH=tests.db
```

4. Build the application:
```bash
go build -o power-test.exe .
```

## Usage

### Starting the Server

```bash
# With real power meter
./power-test.exe

# With mock power meter (for testing without hardware)
./power-test.exe -mock

# Custom port
./power-test.exe -addr :9090
```

Access the web interface at `http://localhost:8080`

### Running a Test

1. **Configure Test Parameters**:
   - Set test name and device under test
   - Choose test duration and polling interval
   - Optional: Configure pre/post-test baseline periods

2. **Select Protocol**:
   - **UDP**: Fast, connectionless, suitable for high throughput testing
   - **TCP**: Connection-oriented, reliable delivery
   - **Layer 2**: Raw Ethernet frames, bypasses network stack

3. **Configure Network Interfaces**:
   - Select interfaces to use for load generation
   - Set worker count (8-12 recommended for 1 Gbps)
   - Configure target throughput per interface
   - Optional: Enable progressive ramping

4. **Layer 2 Specific**:
   - Click "Discover Devices" to scan the network
   - Select target device or manually enter MAC address
   - Ensure running with administrator privileges

5. **Start Test**:
   - Click "Start Test" to begin
   - Monitor real-time power and throughput data
   - Add custom markers during test execution
   - Download CSV or save to database when complete

### Analyzing Results

1. Navigate to the Analysis page (`/analysis`)
2. Load test data from:
   - Database: Browse and select saved tests
   - CSV: Upload exported test files
3. Review:
   - Summary statistics (average/max power and throughput)
   - Phase-based analysis with standard deviations
   - Interactive charts with zoom and pan
   - Efficiency metrics (Mbps per Watt)
4. Export to Excel for further analysis

## Architecture

### Backend (Go)
- **Server** (`internal/server`): HTTP API, SSE streaming, database endpoints
- **Runner** (`internal/runner`): Test orchestration, phase management
- **LoadGen** (`internal/loadgen`): UDP/TCP/Layer2 packet generation with rate control
- **Network** (`internal/network`): Interface enumeration and device discovery
- **FritzBox** (`internal/fritzbox`): Power meter integration via TR-064
- **Database** (`internal/database`): SQLite CRUD operations

### Frontend (JavaScript)
- Single-page application with real-time updates via SSE
- Chart.js for data visualization
- IndexedDB for local storage
- ExcelJS for report generation

### Data Flow
```
User Config → Server → Runner → LoadGen + PowerMeter
                ↓
          SSE Streaming → Browser → Charts + Storage
                ↓
           Database → Analysis Page → Excel Export
```

## Configuration

### Interface Selection
Network interfaces must be configured individually. Leave interface field empty to use OS routing.

### Throughput Control
- **Target Throughput**: Set to 0 for unlimited, or specify Mbps
- **Workers**: More workers = higher potential throughput but less precise rate control
- **Packet Size**: 1400-1472 bytes optimal (avoids IP fragmentation)

### Ramping
Configure progressive load increase:
- **Ramp Steps**: Number of incremental increases (e.g., 4 steps = 25%, 50%, 75%, 100%)
- **Ramp Duration**: Time to complete all steps
- **Pre-Delay**: Stagger interface startup times

## API Endpoints

### Test Control
- `POST /start` - Start a new test
- `POST /stop` - Stop running test
- `POST /marker` - Add custom event marker
- `GET /events` - SSE stream for real-time updates

### Data Management
- `GET /tests` - List all saved tests
- `GET /tests/{id}` - Get specific test data
- `DELETE /tests/delete/{id}` - Delete test

### Network
- `GET /interfaces` - List available network interfaces
- `POST /discover` - Start device discovery
- `GET /discovered-devices` - Get discovery results

### Diagnostics
- `POST /test-fritzbox` - Verify power meter connection
- `POST /test-target` - Verify target device connectivity

## Troubleshooting

### Layer 2 Issues
- **"Failed to open pcap"**: Install WinPcap/Npcap and run as administrator
- **No packets sent**: Verify interface name is correct and not virtual
- **Permission denied**: Requires elevated privileges for raw socket access

### Power Measurement
- **Connection failed**: Check FritzBox credentials and network connectivity
- **No data**: Verify AIN (Actor Identification Number) is correct
- **Mock mode**: Use `-mock` flag for testing without hardware

### Performance
- **Low throughput**: Increase worker count or use larger packet sizes
- **High CPU usage**: Reduce worker count or target throughput
- **Rate control issues**: Use fewer workers with longer delays for better precision

## License

This project is licensed under the MIT License.

## Acknowledgments

- gopacket for packet manipulation
- FritzBox TR-064 API for power measurement
- Chart.js for data visualization
- ExcelJS for report generation
