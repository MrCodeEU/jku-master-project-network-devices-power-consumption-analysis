# Power Consumption Test Runner

This is an MVP for automating power consumption tests using a Fritz!Box DECT smart plug.

## Prerequisites

- Go 1.16+

## Running the Application

1.  Navigate to the project directory:
    ```bash
    cd project
    ```

2.  Run the application (Mock mode by default):
    ```bash
    go run main.go
    ```

    To run with real Fritz!Box (not fully implemented yet):
    ```bash
    go run main.go -mock=false -addr=:8080
    ```

3.  Open your browser and go to `http://localhost:8080`.

## Features

-   **Web UI**: Configure test duration and start/stop tests.
-   **Real-time Graph**: Visualizes power consumption in real-time using Server-Sent Events (SSE) and Chart.js.
-   **Mock Mode**: Simulates power consumption data for development and testing without hardware.
-   **Extensible Architecture**:
    -   `internal/fritzbox`: Interface for power meter.
    -   `internal/runner`: Test execution logic.
    -   `internal/server`: Web server and SSE handling.

## TODO

-   Implement real TR-064 communication in `internal/fritzbox/fritzbox.go`.
-   Implement network load generation.
-   Save test results to a report file (CSV/JSON/PDF).
