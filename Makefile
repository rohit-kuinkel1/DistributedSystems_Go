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
.PHONY: run-database run-server run-gateway run-sensor run-mqtt-broker stop-all help
.PHONY: test-rpc-performance test-combined-performance test-http-performance test-mqtt-performance performance-report
.PHONY: setup-mqtt run-mqtt-system

all: build

generate:
	@echo "Generating gRPC code..."
	$(MKDIR) pkg$(PATHSEP)generated$(PATHSEP)rpc
	protoc --go_out=. --go-grpc_out=. pkg/rpc/database.proto

setup-mqtt:
	@echo "Installing MQTT dependencies..."
	go get github.com/eclipse/paho.mqtt.golang

build: generate setup-mqtt
	go build -o bin$(PATHSEP)server$(BINARY_EXT) ./cmd/server
	go build -o bin$(PATHSEP)gateway$(BINARY_EXT) ./cmd/gateway
	go build -o bin$(PATHSEP)sensor$(BINARY_EXT) ./cmd/sensor
	go build -o bin$(PATHSEP)database$(BINARY_EXT) ./cmd/database
	go build -o bin$(PATHSEP)server_32$(BINARY_EXT) ./cmd/server_32
	@echo "Build completed successfully"

run-mqtt-broker:
	@echo "Starting MQTT broker (Mosquitto) in Docker..."
	docker run -d --name mosquitto \
		-p 1883:1883 \
		-p 9001:9001 \
		-v $(PWD)/config/mosquitto.conf:/mosquitto/config/mosquitto.conf \
		eclipse-mosquitto:2.0

run-mqtt-system: stop-all
	@echo "Starting complete MQTT system..."
	@echo "1. Starting MQTT broker..."
	$(MAKE) run-mqtt-broker
	@sleep 3
	@echo "2. Starting database..."
	./bin/database$(BINARY_EXT) -port 50051 -data-limit 1000000 &
	@sleep 2
	@echo "3. Starting HTTP server..."
	./bin/server$(BINARY_EXT) -host localhost -port 8080 -db-addr localhost:50051 &
	@sleep 2
	@echo "4. Starting MQTT gateway..."
	./bin/gateway$(BINARY_EXT) -server-host localhost -server-port 8080 -mqtt-host localhost -mqtt-port 1883 &
	@sleep 2
	@echo "5. Starting MQTT sensors..."
	./bin/sensor$(BINARY_EXT) -mqtt-host localhost -mqtt-port 1883 -instances 3 &
	@echo "Complete MQTT system is running!"
	@echo "View data at: http://localhost:8080"

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
	@echo "Starting MQTT gateway..."
	./bin/gateway$(BINARY_EXT) -server-host localhost -server-port 8080 -mqtt-host localhost -mqtt-port 1883

run-sensor:
	@echo "Starting MQTT sensor simulators..."
	./bin/sensor$(BINARY_EXT) -mqtt-host localhost -mqtt-port 1883 -instances 5

run-gateway-timed:
	@echo "Starting gateway for 30 seconds..."
	./bin/gateway$(BINARY_EXT) -server-host localhost -server-port 8080 -mqtt-host localhost -mqtt-port 1883 -duration 30

stop-all:
	@echo "Stopping all components..."
ifeq ($(OS),Windows_NT)
	taskkill /F /IM server$(BINARY_EXT) 2>nul || echo "No server running"
	taskkill /F /IM server_32$(BINARY_EXT) 2>nul || echo "No server_32 running"
	taskkill /F /IM gateway$(BINARY_EXT) 2>nul || echo "No gateway running"
	taskkill /F /IM database$(BINARY_EXT) 2>nul || echo "No database running"
	taskkill /F /IM sensor$(BINARY_EXT) 2>nul || echo "No sensor running"
	docker stop mosquitto 2>nul || echo "No mosquitto container running"
	docker rm mosquitto 2>nul || echo "No mosquitto container to remove"
else
	pkill -f "server" || echo "No server running"
	pkill -f "server_32" || echo "No server_32 running"
	pkill -f "gateway" || echo "No gateway running"
	pkill -f "database -port 50051" || echo "No database on 50051 running"
	pkill -f "database -port 50052" || echo "No database on 50052 running"
	pkill -f "sensor" || echo "No sensor running"
	docker stop mosquitto 2>/dev/null || echo "No mosquitto container running"
	docker rm mosquitto 2>/dev/null || echo "No mosquitto container to remove"
endif

test-mqtt-performance:
	@echo "Starting MQTT performance test..."
	$(MAKE) run-mqtt-broker
	@sleep 3
	./bin/sensor$(BINARY_EXT) -mqtt-host localhost -mqtt-port 1883 -instances 10 -duration 60 &
	@sleep 65
	@echo "MQTT performance test completed"
	docker stop mosquitto || true
	docker rm mosquitto || true

run-database-dual:
	@echo "Starting dual database servers..."
	./bin/database$(BINARY_EXT) -port 50051 -data-limit 1000000 &
	./bin/database$(BINARY_EXT) -port 50052 -data-limit 1000000 &

run-server-2pc:
	@echo "Starting HTTP server with 2PC support..."
	./bin/server$(BINARY_EXT) -host localhost -port 8080 -db-addr1 localhost:50051 -db-addr2 localhost:50052

run-2pc-system: stop-all
	@echo "Starting complete 2PC system..."
	@echo "1. Starting dual databases..."
	$(MAKE) run-database-dual
	@sleep 3
	@echo "2. Starting HTTP server with 2PC..."
	$(MAKE) run-server-2pc &
	@sleep 2
	@echo "3. Starting MQTT broker..."
	$(MAKE) run-mqtt-broker
	@sleep 2
	@echo "4. Starting MQTT gateway..."
	./bin/gateway$(BINARY_EXT) -server-host localhost -server-port 8080 -mqtt-host localhost -mqtt-port 1883 &
	@sleep 2
	@echo "5. Starting MQTT sensors..."
	./bin/sensor$(BINARY_EXT) -mqtt-host localhost -mqtt-port 1883 -instances 3 &
	@echo "Complete 2PC system is running!"
	@echo "View data at: http://localhost:8080"





#tests
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


test-2pc-performance:
	@echo "Starting dual database servers for 2PC testing..."
	./bin/database$(BINARY_EXT) -port 50051 -data-limit 1000000 &
	./bin/database$(BINARY_EXT) -port 50052 -data-limit 1000000 &
	@sleep 3
	@echo "Running 2PC performance tests..."
	go test -v ./tests/performance/2pc_test.go -timeout 5m
	@echo "Stopping database servers..."
	pkill -f "database -port 50051" || true
	pkill -f "database -port 50052" || true

test-mqtt-performance-detailed:
	@echo "Starting MQTT broker for performance testing..."
	$(MAKE) run-mqtt-broker
	@sleep 3
	@echo "Running detailed MQTT performance tests..."
	go test -v ./tests/performance/mqtt_performance_test.go -timeout 3m
	@echo "Stopping MQTT broker..."
	docker stop mosquitto || true
	docker rm mosquitto || true

test-mqtt-under-load:
	@echo "Starting MQTT load test with system components..."
	$(MAKE) run-mqtt-broker
	@sleep 3
	./bin/database$(BINARY_EXT) -port 50051 -data-limit 1000000 &
	@sleep 2
	./bin/server$(BINARY_EXT) -host localhost -port 8080 -db-addr localhost:50051 &
	@sleep 2
	./bin/gateway$(BINARY_EXT) -server-host localhost -server-port 8080 -mqtt-host localhost -mqtt-port 1883 &
	@sleep 2
	@echo "Running MQTT performance test with full system..."
	go test -v ./tests/performance/mqtt_test.go -timeout 3m
	@echo "Stopping all components..."
	pkill -f "database" || true
	pkill -f "server" || true
	pkill -f "gateway" || true
	docker stop mosquitto || true
	docker rm mosquitto || true

test-system-under-mqtt-load:
	@echo "Testing HTTP/RPC performance while MQTT is under load..."
	$(MAKE) run-mqtt-broker
	@sleep 3
	./bin/database$(BINARY_EXT) -port 50051 -data-limit 1000000 &
	@sleep 2
	./bin/server$(BINARY_EXT) -host localhost -port 8080 -db-addr localhost:50051 &
	@sleep 2
	./bin/gateway$(BINARY_EXT) -server-host localhost -server-port 8080 -mqtt-host localhost -mqtt-port 1883 &
	@sleep 2
	@echo "Starting background MQTT load..."
	./bin/sensor$(BINARY_EXT) -mqtt-host localhost -mqtt-port 1883 -instances 20 -duration 120 &
	@sleep 5
	@echo "Testing HTTP+RPC performance under MQTT load..."
	go test -v ./tests/performance/combined_test.go -timeout 3m
	@echo "Stopping all components..."
	pkill -f "sensor" || true
	pkill -f "database" || true
	pkill -f "server" || true
	pkill -f "gateway" || true
	docker stop mosquitto || true
	docker rm mosquitto || true

test-http-under-mqtt-load:
	@echo "Testing raw HTTP performance while MQTT is under load..."
	$(MAKE) run-mqtt-broker
	@sleep 3
	@echo "Starting raw HTTP server (Task 2)..."
	./bin/server_32$(BINARY_EXT) -host localhost -port 8080 -data-limit 1000000 &
	@sleep 2
	@echo "Starting background MQTT load..."
	./bin/sensor$(BINARY_EXT) -mqtt-host localhost -mqtt-port 1883 -instances 20 -duration 120 &
	@sleep 5
	@echo "Testing raw HTTP performance under MQTT load..."
	go test -v ./tests/performance/http_test.go -timeout 3m
	@echo "Stopping components..."
	pkill -f "server_32" || true
	pkill -f "sensor" || true
	docker stop mosquitto || true
	docker rm mosquitto || true

test-rpc-under-mqtt-load:
	@echo "Testing pure RPC performance while MQTT is under load..."
	$(MAKE) run-mqtt-broker
	@sleep 3
	@echo "Starting database service..."
	./bin/database$(BINARY_EXT) -port 50051 -data-limit 1000000 &
	@sleep 2
	@echo "Starting background MQTT load..."
	./bin/sensor$(BINARY_EXT) -mqtt-host localhost -mqtt-port 1883 -instances 20 -duration 120 &
	@sleep 5
	@echo "Testing pure RPC performance under MQTT load..."
	go test -v ./tests/performance/rpc_test.go -timeout 3m
	@echo "Stopping components..."
	pkill -f "database" || true
	pkill -f "sensor" || true
	docker stop mosquitto || true
	docker rm mosquitto || true

#all tests for 3.4
test-task-34-complete:
	@echo "Running BASELINE performance tests..."
	@echo "----------------------------------------------"
	@echo "1.1 Raw HTTP baseline (Task 2)..."
	$(MAKE) test-http-performance
	@sleep 3
	@echo "1.2 Pure RPC baseline (Task 3)..."
	$(MAKE) test-rpc-performance  
	@sleep 3
	@echo "1.3 HTTP+RPC baseline (Task 3)..."
	$(MAKE) test-combined-performance
	@sleep 3
	@echo "1.4 MQTT throughput baseline (Task 4)..."
	$(MAKE) test-mqtt-performance-detailed
	@sleep 3
	@echo ""
	@echo "Running UNDER MQTT LOAD tests..."
	@echo "-----------------------------------------"
	@echo "2.1 Raw HTTP under MQTT load..."
	$(MAKE) test-http-under-mqtt-load
	@sleep 5
	@echo "2.2 Pure RPC under MQTT load..."
	$(MAKE) test-rpc-under-mqtt-load
	@sleep 5  
	@echo "2.3 HTTP+RPC under MQTT load..."
	$(MAKE) test-system-under-mqtt-load
	@sleep 5


test-2pc-setup:
	@echo "Setting up dual database servers for 2PC testing..."
	./bin/database$(BINARY_EXT) -port 50051 -data-limit 1000000 &
	./bin/database$(BINARY_EXT) -port 50052 -data-limit 1000000 &
	@sleep 3

test-2pc-functional: test-2pc-setup
	@echo "Running 2PC functional tests..."
	go test -v ./tests/functional/2pc_test.go -timeout 2m
	@echo "Stopping database servers..."
	pkill -f "database -port 50051" || true
	pkill -f "database -port 50052" || true

test-2pc-http: test-2pc-setup
	@echo "Running HTTP tests with 2PC redundant storage..."
	go test -v ./tests/functional/http_2pc_test.go -timeout 2m
	@echo "Stopping database servers..."
	pkill -f "database -port 50051" || true
	pkill -f "database -port 50052" || true

test-2pc-all: test-2pc-setup
	@echo "Running all 2PC tests..."
	go test -v ./tests/functional/2pc_test.go ./tests/functional/http_2pc_test.go -timeout 3m
	@echo "Stopping database servers..."
	pkill -f "database -port 50051" || true
	pkill -f "database -port 50052" || true

test-2pc-integration: 
	@echo "Running full 2PC integration test..."
	$(MAKE) run-2pc-system &
	@sleep 10
	@echo "Testing HTTP POST to 2PC system..."
	curl -X POST http://localhost:8080/data \
		-H "Content-Type: application/json" \
		-d '{"sensorId":"integration-test","timestamp":"2025-01-01T12:00:00Z","value":42.5,"unit":"°C"}'
	@echo ""
	@echo "Testing HTTP GET from 2PC system..."
	curl -X GET http://localhost:8080/data/integration-test
	@echo ""
	@echo "Stopping 2PC system..."
	$(MAKE) stop-all

#test targets for each test case
test-2pc-successful:
	@echo "Testing successful 2PC transactions..."
	$(MAKE) test-2pc-setup
	go test -v ./tests/functional/ -run Test2PCSuccessfulTransaction -timeout 1m
	pkill -f "database" || true

test-2pc-failed:
	@echo "Testing failed 2PC transactions..."
	./bin/database$(BINARY_EXT) -port 50051 -data-limit 1000000 &
	@sleep 2
	go test -v ./tests/functional/ -run Test2PCFailedTransaction -timeout 1m
	pkill -f "database" || true

test-2pc-consistency:
	@echo "Testing 2PC data consistency..."
	$(MAKE) test-2pc-setup
	go test -v ./tests/functional/ -run Test2PCDataConsistency -timeout 1m
	pkill -f "database" || true

test-2pc-uniqueness:
	@echo "Testing 2PC transaction ID uniqueness..."
	$(MAKE) test-2pc-setup
	go test -v ./tests/functional/ -run Test2PCTransactionIDUniqueness -timeout 1m
	pkill -f "database" || true

test-2pc-concurrent:
	@echo "Testing 2PC concurrent transactions..."
	$(MAKE) test-2pc-setup
	go test -v ./tests/functional/ -run Test2PCConcurrentTransactions -timeout 2m
	pkill -f "database" || true

test-task-35-complete:
	@echo "START TEST"
	@echo "=========================================="
	@echo "1. Testing successful 2PC transactions..."
	$(MAKE) test-2pc-successful
	@sleep 2
	@echo ""
	@echo "2. Testing failed 2PC transactions..."
	$(MAKE) test-2pc-failed
	@sleep 2
	@echo ""
	@echo "3. Testing 2PC data consistency..."
	$(MAKE) test-2pc-consistency
	@sleep 2
	@echo ""
	@echo "4. Testing HTTP with redundant storage..."
	$(MAKE) test-2pc-http
	@sleep 2
	@echo ""
	@echo "5. Testing transaction ID uniqueness..."
	$(MAKE) test-2pc-uniqueness
	@sleep 2
	@echo ""
	@echo "6. Testing concurrent transactions..."
	$(MAKE) test-2pc-concurrent
	@sleep 2
	@echo ""
	@echo "=========================================="
	@echo "COMPLETE TEST"

clean-2pc-tests:
	@echo "Cleaning up 2PC test processes..."
	pkill -f "database -port 50051" || true
	pkill -f "database -port 50052" || true
	pkill -f "server" || true
	docker stop mosquitto || true
	docker rm mosquitto || true

clean:
ifeq ($(OS),Windows_NT)
	if exist bin $(RM) bin
	if exist pkg\generated $(RM) pkg\generated
else
	$(RM) bin
	$(RM) pkg/generated
endif
	@echo "Clean completed successfully"

docker-build:
	docker-compose build

docker-run:
	docker-compose up

test: clean build test-functional test-performance
