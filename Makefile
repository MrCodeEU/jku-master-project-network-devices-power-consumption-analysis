.PHONY: run run-mock run-real build tidy

# Default target runs in mock mode
run: run-real

# Run in mock mode (default)
run-mock:
	cd project && go run main.go -mock=true

# Run with real device (requires .env configuration)
run-real:
	cd project && go run main.go -mock=false

# Build the application
build:
	cd project && go build -o ../bin/app main.go

# Install/Update dependencies
tidy:
	cd project && go mod tidy
