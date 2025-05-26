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
.PHONY: test-rpc-performance test-combined-performance test-http-performance performance-report

all: build

#create a new generate target for protobuf/gRPC
generate:
	@echo "Generating gRPC code..."
	$(MKDIR) pkg$(PATHSEP)generated$(PATHSEP)rpc
	protoc --go_out=. --go-grpc_out=. pkg/rpc/database.proto

#update build to depend on generate and include server_32
build: generate
	go build -o bin$(PATHSEP)server$(BINARY_EXT) ./cmd/server
	go build -o bin$(PATHSEP)gateway$(BINARY_EXT) ./cmd/gateway
	go build -o bin$(PATHSEP)sensor$(BINARY_EXT) ./cmd/sensor
	go build -o bin$(PATHSEP)database$(BINARY_EXT) ./cmd/database
	go build -o bin$(PATHSEP)server_32$(BINARY_EXT) ./cmd/server_32
	@echo "Build completed successfully"

# Simple run commands (no build dependencies)
run-database:
	@echo "Starting database server on port 50051..."
	./bin/database$(BINARY_EXT) -port 50051 -data-limit 1000000

run-server:
	@echo "Starting HTTP server on port 8080..."
	./bin/server$(BINARY_EXT) -host localhost -port 8080 -db-addr localhost:50051

run-server-32:
	@echo "Starting raw HTTP server on port 8080..."
	./bin/server_32$(BINARY_EXT) -host localhost -port 8080 -data-limit 1000000

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
	taskkill /F /IM server_32$(BINARY_EXT) 2>nul || echo "No server_32 running"
	taskkill /F /IM gateway$(BINARY_EXT) 2>nul || echo "No gateway running"
	taskkill /F /IM database$(BINARY_EXT) 2>nul || echo "No database running"
	taskkill /F /IM sensor$(BINARY_EXT) 2>nul || echo "No sensor running"
else
	pkill -f "server" || echo "No server running"
	pkill -f "server_32" || echo "No server_32 running"
	pkill -f "gateway" || echo "No gateway running"
	pkill -f "database" || echo "No database running"
	pkill -f "sensor" || echo "No sensor running"
endif

help:
	@echo "Build & Run:"
	@echo "  make build               - Build all components"
	@echo "  make run-database        - Start database (port 50051)"
	@echo "  make run-server          - Start HTTP+RPC server (port 8080)"
	@echo "  make run-server-32       - Start raw HTTP server (port 8080)"
	@echo "  make run-gateway         - Start gateway (5 sensors)"
	@echo "  make run-sensor          - Start sensor simulator"
	@echo ""
	@echo "Performance Tests:"
	@echo "  make test-http-performance      - Raw HTTP performance test (Task 2)"
	@echo "  make test-performance           - HTTP+RPC performance test (Task 3)"
	@echo "  make test-rpc-performance       - RPC performance test"
	@echo "  make test-combined-performance  - Combined HTTP+RPC load test"
	@echo "  make performance-report         - Run all performance tests and generate report"
	@echo ""
	@echo "Utility:"
	@echo "  make stop-all            - Stop all running components"
	@echo "  make help               - Show this help"
	@echo ""
	@echo "Performance Test Setup:"
	@echo "  For Raw HTTP:     make run-server-32, then make test-http-performance"
	@echo "  For HTTP+RPC:     make run-database, then make run-server, then make test-performance"
	@echo "  For Combined:     make run-database, then make test-combined-performance"

test-functional:
	go test -v ./tests/functional/...

test-performance:
	go test -v ./tests/performance/...

test-http-performance:
	@echo "Starting raw HTTP server..."
	./bin/server_32$(BINARY_EXT) -host localhost -port 8080 -data-limit 1000000 &
	@sleep 2
	@echo "Running raw HTTP performance tests..."
	go test -v ./tests/performance/http_test.go -timeout 2m
	@echo "Stopping raw HTTP server..."
	pkill -f "server_32" || true

test-rpc-performance:
	@echo "Starting database service..."
	./bin/database$(BINARY_EXT) -port 50051 -data-limit 1000000 &
	@sleep 2
	@echo "Running RPC performance tests..."
	go test -v ./tests/performance/rpc_test.go -timeout 2m
	@echo "Stopping database service..."
	pkill -f "database -port 50051" || true

test-combined-performance:
	@echo "Starting database service..."
	./bin/database$(BINARY_EXT) -port 50051 -data-limit 1000000 &
	@sleep 2
	@echo "Running combined HTTP+RPC performance tests..."
	go test -v ./tests/performance/combined_test.go -timeout 6m
	@echo "Stopping database service..."
	pkill -f "database -port 50051" || true

test-system-performance: build
	@echo "Starting complete system test..."
	docker-compose up -d database
	@sleep 5
	./bin/server$(BINARY_EXT) -host localhost -port 8080 -db-addr localhost:50051 &
	@sleep 2
	./bin/gateway$(BINARY_EXT) -server-host localhost -server-port 8080 -instances 5 -duration 60 &
	@sleep 65
	@echo "Collecting performance metrics..."
	curl -s http://localhost:8080/performance/rpc | jq '.'
	@echo "Stopping services..."
	pkill -f "server" || true
	pkill -f "gateway" || true
	docker-compose down

performance-report: test-http-performance test-performance test-rpc-performance test-combined-performance
	@echo "Generating comprehensive performance comparison report..."
	@echo ""
	@echo "=========================================="
	@echo "PERFORMANCE COMPARISON REPORT"
	@echo "=========================================="
	@echo ""
	@echo "1. Raw HTTP Performance (Task 2 - Local Storage):"
	@echo "--------------------------------------------------"
	@cat tests/performance/raw_http_performance_results.txt
	@echo ""
	@echo "2. HTTP+RPC Performance (Task 3 - Database via RPC):"
	@echo "-----------------------------------------------------"
	@cat tests/performance/http_performance_results.txt
	@echo ""
	@echo "3. Pure RPC Performance:"
	@echo "------------------------"
	@cat tests/performance/rpc_performance_results.txt
	@echo ""
	@echo "4. Combined HTTP+RPC Load Test:"
	@echo "-------------------------------"
	@cat tests/performance/combined_http_rpc_performance_results.txt
	@echo ""
	@echo "=========================================="
	@echo "ANALYSIS SUMMARY"
	@echo "=========================================="
	@echo "Compare the results to understand:"
	@echo "- Raw HTTP vs HTTP+RPC overhead"
	@echo "- RPC performance characteristics"
	@echo "- Performance degradation under load"
	@echo "- System bottlenecks and scaling limits"

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