ifeq ($(OS),Windows_NT)
	RM := rmdir /s /q
	MKDIR := mkdir
	BINARY_EXT := .exe
	PATHSEP := \\
else
	RM := rm -rf
	MKDIR := mkdir -p
	BINARY_EXT :=
	PATHSEP := /
endif

#ensure bin directory exists before building
$(shell $(MKDIR) bin 2>/dev/null)

.PHONY: build test-functional test-performance clean docker-build docker-run all test generate
.PHONY: run-database run-server run-gateway run-sensor stop-all help

all: build

#create a new generate target for protobuf/gRPC
generate:
	@echo "Generating gRPC code..."
	$(MKDIR) pkg$(PATHSEP)generated$(PATHSEP)rpc
	protoc --go_out=. --go-grpc_out=. pkg/rpc/database.proto

#update build to depend on generate
build: generate
	go build -o bin$(PATHSEP)server$(BINARY_EXT) ./cmd/server
	go build -o bin$(PATHSEP)gateway$(BINARY_EXT) ./cmd/gateway
	go build -o bin$(PATHSEP)sensor$(BINARY_EXT) ./cmd/sensor
	go build -o bin$(PATHSEP)database$(BINARY_EXT) ./cmd/database
	@echo "Build completed successfully"

# Simple run commands (no build dependencies)
run-database:
	@echo "Starting database server on port 50051..."
	./bin/database$(BINARY_EXT) -port 50051 -data-limit 1000000

run-server:
	@echo "Starting HTTP server on port 8080..."
	./bin/server$(BINARY_EXT) -host localhost -port 8080 -db-addr localhost:50051

run-gateway:
	@echo "Starting gateway with 5 sensor instances..."
	./bin/gateway$(BINARY_EXT) -server-host localhost -server-port 8080 -instances 5

run-sensor:
	@echo "Starting sensor simulator..."
	./bin/sensor$(BINARY_EXT)

run-gateway-timed:
	@echo "Starting gateway for 30 seconds..."
	./bin/gateway$(BINARY_EXT) -server-host localhost -server-port 8080 -instances 3 -duration 30

stop-all:
	@echo "Stopping all components..."
ifeq ($(OS),Windows_NT)
	taskkill /F /IM server$(BINARY_EXT) 2>nul || echo "No server running"
	taskkill /F /IM gateway$(BINARY_EXT) 2>nul || echo "No gateway running"
	taskkill /F /IM database$(BINARY_EXT) 2>nul || echo "No database running"
	taskkill /F /IM sensor$(BINARY_EXT) 2>nul || echo "No sensor running"
else
	pkill -f "server" || echo "No server running"
	pkill -f "gateway" || echo "No gateway running"
	pkill -f "database" || echo "No database running"
	pkill -f "sensor" || echo "No sensor running"
endif

help:
	@echo "  make run-database    - Start database (port 50051)"
	@echo "  make run-server      - Start HTTP server (port 8080)"
	@echo "  make run-gateway     - Start gateway (5 sensors)"
	@echo "  make run-sensor      - Start sensor simulator"
	@echo ""
	@echo "Utility:"
	@echo "  make stop-all        - Stop all running components"
	@echo "  make help           - Show this help"
	@echo ""
	@echo "Quick Setup in 3 terminals:"
	@echo "  Terminal 1: make run-database"
	@echo "  Terminal 2: make run-server"
	@echo "  Terminal 3: make run-gateway"

test-functional:
	go test -v ./tests/functional/...

test-performance:
	go test -v ./tests/performance/...

test-rpc-performance:
	@echo "Starting database service..."
	./bin/database -port 50051 -data-limit 1000000 &
	@sleep 2
	@echo "Running RPC performance tests..."
	go test -v ./tests/performance/rpc_test.go -timeout 1m
	@echo "Stopping database service..."
	pkill -f "database -port 50051" || true

test-system-performance: build
	@echo "Starting complete system test..."
	docker-compose up -d database
	@sleep 5
	./bin/server -host localhost -port 8080 -db-addr localhost:50051 &
	@sleep 2
	./bin/gateway -server-host localhost -server-port 8080 -instances 5 -duration 60 &
	@sleep 65
	@echo "Collecting performance metrics..."
	curl -s http://localhost:8080/performance/rpc | jq '.'
	@echo "Stopping services..."
	pkill -f "server" || true
	pkill -f "gateway" || true
	docker-compose down

performance-report: test-performance test-rpc-performance
	@echo "Generating performance comparison report..."
	@echo "HTTP Performance Results:"
	@cat tests/performance/http_performance_results.txt
	@echo ""
	@echo "RPC Performance Results:"
	@cat tests/performance/rpc_performance_results.txt

clean:
ifeq ($(OS),Windows_NT)
	if exist bin $(RM) bin
	if exist pkg\generated $(RM) pkg\generated
else
	$(RM) bin
	$(RM) pkg/generated
endif
	@echo "Clean completed successfully"

test: clean build test-functional test-performance
	@echo "All tests completed successfully"

docker-build:
	docker-compose build

docker-run:
	docker-compose up