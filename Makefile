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

.PHONY: build test-all test-2pc-performance test-2pc-functional clean docker-build docker-run stop-all
.DEFAULT_GOAL := build

# ==============================================
# BUILD TARGET - Does everything needed to build
# ==============================================
build: generate setup-deps compile
	@echo "Build completed successfully"

generate:
	@echo "Generating gRPC code..."
	$(MKDIR) pkg$(PATHSEP)generated$(PATHSEP)rpc
	protoc --go_out=. --go-grpc_out=. pkg/rpc/database.proto

setup-deps:
	@echo "Installing dependencies..."
	go get github.com/eclipse/paho.mqtt.golang
	go mod tidy

compile:
	@echo "Compiling binaries..."
	go build -o bin$(PATHSEP)server$(BINARY_EXT) ./cmd/server
	go build -o bin$(PATHSEP)gateway$(BINARY_EXT) ./cmd/gateway
	go build -o bin$(PATHSEP)sensor$(BINARY_EXT) ./cmd/sensor
	go build -o bin$(PATHSEP)database$(BINARY_EXT) ./cmd/database
	go build -o bin$(PATHSEP)server_32$(BINARY_EXT) ./cmd/server_32

# ==============================================
# TEST-ALL TARGET - Complete test suite
# ==============================================
test-all: build
	@echo "Running all test suites"
	@echo "=================================================================="
	@echo ""
	@echo "FUNCTIONAL TESTS"
	@echo "-------------------"
	@$(MAKE) test-functional-all
	@echo ""
	@echo "PERFORMANCE TESTS"  
	@echo "--------------------"
	@$(MAKE) test-performance-all
	@echo ""
	@echo "2PC TESTS"
	@echo "------------"
	@$(MAKE) test-2pc-all
	@echo ""
	@echo "TEST SUITE FINISHED"
	@echo "================================"

#functional tests
test-functional-all:
	@echo "Testing HTTP/RPC functionality..."
	go test -v ./tests/functional/http_test.go -timeout 2m
	@echo "Testing 2PC functionality..."
	@$(MAKE) start-dual-db
	@sleep 3
	go test -v ./tests/functional/2pc_test.go ./tests/functional/http_2pc_test.go -timeout 3m
	@$(MAKE) stop-components

#performance tests  
test-performance-all:
	@echo "2: HTTP Performance..."
	@$(MAKE) test-http-perf
	@sleep 2
	@echo "3: RPC Performance..."
	@$(MAKE) test-rpc-perf
	@sleep 2
	@echo "4: MQTT Performance..."
	@$(MAKE) test-mqtt-perf
	@sleep 2

# ==============================================
# INDIVIDUAL FUNCTIONAL TESTS (Internal)
# ==============================================
test-2pc-functional:
	@echo "2PC FUNCTIONAL TESTS"
	@echo "-----------------------"
	@$(MAKE) start-dual-db
	@sleep 3
	@echo "Testing 2PC core functionality..."
	@go test -v ./tests/functional/2pc_test.go -timeout 3m
	@echo "Testing HTTP with 2PC storage..."
	@go test -v ./tests/functional/http_2pc_test.go -timeout 3m
	@$(MAKE) stop-components


# ==============================================
# INDIVIDUAL PERFORMANCE TESTS (Internal)
# ==============================================
test-http-perf:
	@./bin/server_32$(BINARY_EXT) -host localhost -port 8080 &
	@sleep 2
	@go test -v ./tests/performance/http_test.go -timeout 3m
	@pkill -f "server_32" || true

test-rpc-perf:
	@./bin/database$(BINARY_EXT) -port 50051 &
	@sleep 2
	@go test -v ./tests/performance/rpc_test.go -timeout 3m
	@pkill -f "database -port 50051" || true

test-mqtt-perf:
	@docker run -d --name mosquitto -p 1883:1883 eclipse-mosquitto:2.0 || true
	@sleep 3
	@go test -v ./tests/performance/mqtt_performance_test.go -timeout 3m
	@docker stop mosquitto 2>/dev/null || true
	@docker rm mosquitto 2>/dev/null || true

test-2pc-perf:
	@$(MAKE) start-dual-db
	@sleep 3
	go test -v ./tests/performance/2pc_performance_test.go -timeout 10m
	@$(MAKE) stop-components


# ==============================================
# SYSTEM STARTUP COMMANDS
# ==============================================
run-2pc-system: stop-all
	@echo "Starting 2PC system..."
	@./bin/database$(BINARY_EXT) -port 50051 -data-limit 1000000 &
	@./bin/database$(BINARY_EXT) -port 50052 -data-limit 1000000 &
	@sleep 3
	@./bin/server$(BINARY_EXT) -host localhost -port 8080 -db-addr1 localhost:50051 -db-addr2 localhost:50052 &
	@sleep 2
	@echo "2PC system running - visit http://localhost:8080"

run-mqtt-system: stop-all  
	@echo "Starting MQTT system..."
	@docker run -d --name mosquitto -p 1883:1883 eclipse-mosquitto:2.0
	@sleep 3
	@./bin/database$(BINARY_EXT) -port 50051 -data-limit 1000000 &
	@sleep 2
	@./bin/server$(BINARY_EXT) -host localhost -port 8080 -db-addr localhost:50051 &
	@sleep 2
	@./bin/gateway$(BINARY_EXT) -server-host localhost -server-port 8080 -mqtt-host localhost -mqtt-port 1883 &
	@sleep 2
	@./bin/sensor$(BINARY_EXT) -mqtt-host localhost -mqtt-port 1883 -instances 3 &
	@echo "MQTT system running - visit http://localhost:8080"

# ==============================================
# UTILITY COMMANDS
# ==============================================
start-dual-db:
	@./bin/database$(BINARY_EXT) -port 50051 -data-limit 1000000 &
	@./bin/database$(BINARY_EXT) -port 50052 -data-limit 1000000 &

stop-components:
	@pkill -f "database" || true
	@pkill -f "server" || true
	@pkill -f "gateway" || true
	@pkill -f "sensor" || true

stop-all:
	@echo "Stopping all components..."
ifeq ($(OS),Windows_NT)
	@taskkill /F /IM server$(BINARY_EXT) 2>nul || echo ""
	@taskkill /F /IM server_32$(BINARY_EXT) 2>nul || echo ""
	@taskkill /F /IM gateway$(BINARY_EXT) 2>nul || echo ""
	@taskkill /F /IM database$(BINARY_EXT) 2>nul || echo ""
	@taskkill /F /IM sensor$(BINARY_EXT) 2>nul || echo ""
else
	@pkill -f "server" 2>/dev/null || true
	@pkill -f "server_32" 2>/dev/null || true
	@pkill -f "gateway" 2>/dev/null || true
	@pkill -f "database" 2>/dev/null || true
	@pkill -f "sensor" 2>/dev/null || true
endif
	@docker stop mosquitto 2>/dev/null || true
	@docker rm mosquitto 2>/dev/null || true
	@echo "All components stopped"

clean:
	@echo "Cleaning build artifacts..."
ifeq ($(OS),Windows_NT)
	@if exist bin $(RM) bin
	@if exist pkg\generated $(RM) pkg\generated
else
	@$(RM) bin
	@$(RM) pkg/generated
endif
	@$(RM) *_performance_results.txt 2>/dev/null || true
	@echo "Clean completed"

# ==============================================
# DOCKER COMMANDS
# ==============================================
docker-build:
	@echo "Building Docker images..."
	@docker-compose build

docker-run:
	@echo "Starting with Docker Compose..."
	@docker-compose up